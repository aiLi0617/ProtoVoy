use std::collections::HashMap;
use std::future::Future;

use core_domain::{ExecutionEvent, ExecutionState, RequestEnvelope};
use serde::{Deserialize, Serialize};
use tokio_util::sync::CancellationToken;

use crate::{ExecutionEngine, VariableScope};

#[derive(Debug, Clone)]
pub struct CollectionRunItem {
    pub request_id: String,
    pub request: RequestEnvelope,
    pub scope: VariableScope,
}

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq)]
#[serde(rename_all = "camelCase")]
pub struct CollectionRunCase {
    pub request_id: String,
    pub name: String,
    pub protocol_id: String,
    pub passed: bool,
    pub status: Option<u16>,
    pub duration_ms: u64,
    pub error: Option<String>,
    pub failed_assertions: Vec<String>,
}

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq)]
#[serde(rename_all = "camelCase")]
pub struct CollectionRunResult {
    pub cases: Vec<CollectionRunCase>,
    pub cancelled: bool,
}

#[derive(Debug, Clone, Default)]
pub struct CollectionCaseOutcome {
    pub passed: bool,
    pub status: Option<u16>,
    pub duration_ms: u64,
    pub error: Option<String>,
    pub failed_assertions: Vec<String>,
    pub extracted_variables: HashMap<String, String>,
}

/// Shared collection orchestration for hosts that need to keep their own execution-history
/// integration. The host callback executes one request; ordering, fail-fast, cancellation between
/// cases, and run-local variable propagation remain centralized here.
pub async fn orchestrate_collection<F, Fut>(
    items: Vec<CollectionRunItem>,
    fail_fast: bool,
    cancellation: CancellationToken,
    mut execute: F,
) -> CollectionRunResult
where
    F: FnMut(RequestEnvelope, VariableScope) -> Fut,
    Fut: Future<Output = CollectionCaseOutcome>,
{
    let mut cases = Vec::new();
    let mut collection_variables = HashMap::new();
    for item in items {
        if cancellation.is_cancelled() {
            return CollectionRunResult {
                cases,
                cancelled: true,
            };
        }
        let mut scope = item.scope;
        scope.collection.extend(collection_variables.clone());
        let name = item.request.name.clone();
        let protocol_id = item.request.protocol_id.0.clone();
        let outcome = execute(item.request, scope).await;
        collection_variables.extend(outcome.extracted_variables);
        let case = CollectionRunCase {
            request_id: item.request_id,
            name,
            protocol_id,
            passed: outcome.passed,
            status: outcome.status,
            duration_ms: outcome.duration_ms,
            error: outcome.error,
            failed_assertions: outcome.failed_assertions,
        };
        let failed = !case.passed;
        cases.push(case);
        if fail_fast && failed {
            break;
        }
    }
    CollectionRunResult {
        cases,
        cancelled: false,
    }
}

pub async fn orchestrate_collection_to_completion<F, Fut>(
    items: Vec<CollectionRunItem>,
    fail_fast: bool,
    execute: F,
) -> CollectionRunResult
where
    F: FnMut(RequestEnvelope, VariableScope) -> Fut,
    Fut: Future<Output = CollectionCaseOutcome>,
{
    orchestrate_collection(items, fail_fast, CancellationToken::new(), execute).await
}

/// Runs a collection sequentially with variables extracted by one request available to
/// subsequent requests for this run only. Extracted values are never written to an environment.
pub async fn run_collection(
    engine: &ExecutionEngine,
    items: Vec<CollectionRunItem>,
    fail_fast: bool,
    cancellation: CancellationToken,
) -> CollectionRunResult {
    let mut cases = Vec::new();
    let mut collection_variables = HashMap::new();

    for item in items {
        if cancellation.is_cancelled() {
            return CollectionRunResult {
                cases,
                cancelled: true,
            };
        }

        let mut scope = item.scope;
        scope.collection.extend(collection_variables.clone());
        let request_name = item.request.name.clone();
        let protocol_id = item.request.protocol_id.0.clone();
        let execution = engine.execute_with_scope(item.request, scope).await;

        let case = match execution {
            Ok((execution_id, mut events, handle)) => {
                let mut collected = Vec::new();
                let mut cancelled = false;
                loop {
                    tokio::select! {
                        _ = cancellation.cancelled(), if !cancelled => {
                            cancelled = true;
                            engine.cancel(&execution_id);
                        }
                        event = events.recv() => match event {
                            Some(event) => collected.push(event),
                            None => break,
                        }
                    }
                }

                let result = handle.await;
                if cancelled {
                    cases.push(CollectionRunCase {
                        request_id: item.request_id,
                        name: request_name,
                        protocol_id,
                        passed: false,
                        status: None,
                        duration_ms: 0,
                        error: Some("collection run cancelled".into()),
                        failed_assertions: Vec::new(),
                    });
                    return CollectionRunResult {
                        cases,
                        cancelled: true,
                    };
                }

                match result {
                    Ok(Ok(summary)) => {
                        let failed_assertions = collected
                            .iter()
                            .filter_map(|event| match event {
                                ExecutionEvent::AssertionResult(result) if !result.passed => {
                                    Some(result.name.clone())
                                }
                                _ => None,
                            })
                            .collect::<Vec<_>>();
                        for variables in collected.iter().filter_map(|event| match event {
                            ExecutionEvent::VariablesExtracted { variables } => Some(variables),
                            _ => None,
                        }) {
                            collection_variables.extend(variables.clone());
                        }
                        CollectionRunCase {
                            request_id: item.request_id,
                            name: request_name,
                            protocol_id,
                            passed: summary.state == ExecutionState::Completed
                                && failed_assertions.is_empty(),
                            status: summary.status,
                            duration_ms: summary.duration_ms,
                            error: None,
                            failed_assertions,
                        }
                    }
                    Ok(Err(error)) => CollectionRunCase {
                        request_id: item.request_id,
                        name: request_name,
                        protocol_id,
                        passed: false,
                        status: None,
                        duration_ms: 0,
                        error: Some(error.to_string()),
                        failed_assertions: Vec::new(),
                    },
                    Err(error) => CollectionRunCase {
                        request_id: item.request_id,
                        name: request_name,
                        protocol_id,
                        passed: false,
                        status: None,
                        duration_ms: 0,
                        error: Some(format!("execution task terminated unexpectedly: {error}")),
                        failed_assertions: Vec::new(),
                    },
                }
            }
            Err(error) => CollectionRunCase {
                request_id: item.request_id,
                name: request_name,
                protocol_id,
                passed: false,
                status: None,
                duration_ms: 0,
                error: Some(error.to_string()),
                failed_assertions: Vec::new(),
            },
        };

        let failed = !case.passed;
        cases.push(case);
        if fail_fast && failed {
            break;
        }
    }

    CollectionRunResult {
        cases,
        cancelled: false,
    }
}

pub async fn run_collection_to_completion(
    engine: &ExecutionEngine,
    items: Vec<CollectionRunItem>,
    fail_fast: bool,
) -> CollectionRunResult {
    run_collection(engine, items, fail_fast, CancellationToken::new()).await
}

#[cfg(test)]
mod tests {
    use std::sync::{Arc, Mutex};

    use async_trait::async_trait;
    use chrono::Utc;
    use core_domain::{ExecutionId, ExecutionSummary, ProtocolId};
    use event_stream::EventSink;

    use crate::{DriverDescriptor, DriverError, ProtocolDriver, ValidationReport};

    use super::*;

    struct RecordingDriver {
        targets: Arc<Mutex<Vec<String>>>,
    }

    #[async_trait]
    impl ProtocolDriver for RecordingDriver {
        fn descriptor(&self) -> DriverDescriptor {
            DriverDescriptor {
                protocol_id: "collection-test".into(),
                version: "1".into(),
                display_name: "Collection test".into(),
                capabilities: vec![],
            }
        }

        fn validate(&self, _request: &RequestEnvelope) -> ValidationReport {
            ValidationReport::ok()
        }

        async fn execute(
            &self,
            request: RequestEnvelope,
            mut events: EventSink,
            _cancel: CancellationToken,
            execution_id: ExecutionId,
        ) -> Result<ExecutionSummary, DriverError> {
            self.targets
                .lock()
                .unwrap_or_else(|error| error.into_inner())
                .push(request.target.clone());
            events
                .emit(ExecutionEvent::ResponseChunk {
                    content_type: Some("text/plain".into()),
                    size: 5,
                    preview: Some("value".into()),
                    data_base64: None,
                    done: true,
                })
                .await;
            Ok(ExecutionSummary {
                execution_id,
                request_id: request.id.0,
                protocol_id: request.protocol_id.0,
                state: ExecutionState::Completed,
                started_at: Utc::now(),
                finished_at: Utc::now(),
                duration_ms: 1,
                bytes_received: 5,
                status: Some(200),
            })
        }
    }

    fn item(id: &str, target: &str) -> CollectionRunItem {
        let mut request = RequestEnvelope::http_get(id, target);
        request.protocol_id = ProtocolId("collection-test".into());
        CollectionRunItem {
            request_id: id.into(),
            request,
            scope: VariableScope::default(),
        }
    }

    #[tokio::test]
    async fn carries_extracted_variables_without_mutating_environment_scope() {
        let targets = Arc::new(Mutex::new(Vec::new()));
        let mut engine = ExecutionEngine::new();
        engine.register(Arc::new(RecordingDriver {
            targets: Arc::clone(&targets),
        }));
        let mut first = item("first", "https://example.test/first");
        first.request.post_scripts = vec!["variables.next = response.body;".into()];
        let second = item("second", "https://example.test/{{next}}");

        let result = run_collection(
            &engine,
            vec![first, second],
            false,
            CancellationToken::new(),
        )
        .await;

        assert!(!result.cancelled);
        assert!(result.cases.iter().all(|case| case.passed));
        assert_eq!(
            *targets.lock().unwrap_or_else(|error| error.into_inner()),
            vec![
                "https://example.test/first".to_string(),
                "https://example.test/value".to_string()
            ]
        );
    }

    #[tokio::test]
    async fn fail_fast_stops_after_the_first_failed_case() {
        let targets = Arc::new(Mutex::new(Vec::new()));
        let mut engine = ExecutionEngine::new();
        engine.register(Arc::new(RecordingDriver {
            targets: Arc::clone(&targets),
        }));

        let result = run_collection(
            &engine,
            vec![
                item("broken", "https://example.test/{{missing}}"),
                item("skipped", "https://example.test/skipped"),
            ],
            true,
            CancellationToken::new(),
        )
        .await;

        assert_eq!(result.cases.len(), 1);
        assert!(!result.cases[0].passed);
        assert!(targets
            .lock()
            .unwrap_or_else(|error| error.into_inner())
            .is_empty());
    }
}
