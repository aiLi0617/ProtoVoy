//! Multi-scope `{{var}}` resolution (P0).
//!
//! Lookup order (later scopes override earlier):
//! global → environment → collection → request.
//! Dynamic tokens: `{{$uuid}}`, `{{$timestamp}}`, `{{$isoTimestamp}}`.

use std::collections::HashMap;

use chrono::Utc;
use core_domain::{
    GlobalParameter, GlobalParameterLocation, HttpPayload, MultipartPart, ProtocolPayload,
    RequestEnvelope,
};
use regex::Regex;
use thiserror::Error;
use uuid::Uuid;

#[derive(Debug, Error, Clone, PartialEq, Eq)]
pub enum ResolveError {
    #[error("unresolved variable `{{{{{0}}}}}`")]
    Unresolved(String),
}

impl ResolveError {
    pub fn reference(&self) -> String {
        match self {
            Self::Unresolved(name) => name.clone(),
        }
    }
}

#[derive(Debug, Clone, Default)]
pub struct VariableScope {
    pub global: HashMap<String, String>,
    /// Request parameters inherited by every HTTP request. Request-local values win.
    pub global_parameters: Vec<GlobalParameter>,
    pub environment: HashMap<String, String>,
    pub collection: HashMap<String, String>,
    pub request: HashMap<String, String>,
    /// Resolved secret values keyed by `secret_ref` name (never persisted).
    pub secrets: HashMap<String, String>,
}

impl VariableScope {
    pub fn merged(&self) -> HashMap<String, String> {
        let mut out = HashMap::new();
        out.extend(self.global.clone());
        out.extend(self.environment.clone());
        out.extend(self.collection.clone());
        out.extend(self.request.clone());
        out
    }

    pub fn resolve_template(&self, input: &str) -> Result<String, ResolveError> {
        resolve_template(input, &self.merged())
    }
}

/// Replace `{{name}}` placeholders. Unknown names become `ResolveError`.
pub fn resolve_template(
    input: &str,
    vars: &HashMap<String, String>,
) -> Result<String, ResolveError> {
    let re = Regex::new(r"\{\{\s*([^{}]+?)\s*\}\}").expect("static regex");
    let mut unresolved = Vec::new();
    let resolved = re.replace_all(input, |caps: &regex::Captures| {
        let key = caps[1].trim();
        if let Some(value) = resolve_dynamic(key) {
            return value;
        }
        match vars.get(key) {
            Some(v) => v.clone(),
            None => {
                unresolved.push(key.to_string());
                caps[0].to_string()
            }
        }
    });
    if let Some(name) = unresolved.into_iter().next() {
        return Err(ResolveError::Unresolved(name));
    }
    Ok(resolved.into_owned())
}

fn resolve_dynamic(key: &str) -> Option<String> {
    match key {
        "$uuid" => Some(Uuid::new_v4().to_string()),
        "$timestamp" => Some(Utc::now().timestamp().to_string()),
        "$isoTimestamp" => Some(Utc::now().to_rfc3339()),
        _ => None,
    }
}

/// Apply variable resolution to target / headers / body.
pub fn resolve_request(
    mut request: RequestEnvelope,
    scope: &VariableScope,
) -> Result<RequestEnvelope, ResolveError> {
    let vars = {
        let mut merged = scope.merged();
        merged.extend(request.variables.clone());
        merged
    };

    request.target = resolve_template(&request.target, &vars)?;
    if let Some(proxy) = request.proxy.clone() {
        request.proxy = Some(resolve_template(&proxy, &vars)?);
    }

    apply_global_parameters(&mut request, &scope.global_parameters, &vars)?;

    match &mut request.payload {
        ProtocolPayload::Http(HttpPayload {
            headers,
            body,
            body_encoding,
            body_source: _,
            multipart,
            method: _,
            follow_redirects: _,
        }) => {
            for (k, v) in headers.iter_mut() {
                *k = resolve_template(k, &vars)?;
                *v = resolve_template(v, &vars)?;
            }
            if body_encoding != "base64" {
                if let Some(b) = body.as_mut() {
                    *b = resolve_template(b, &vars)?;
                }
            }
            for part in multipart {
                part.name = resolve_template(&part.name, &vars)?;
                if !part.base64 {
                    part.value = resolve_template(&part.value, &vars)?;
                }
                if let Some(file_name) = &mut part.file_name {
                    *file_name = resolve_template(file_name, &vars)?;
                }
                if let Some(content_type) = &mut part.content_type {
                    *content_type = resolve_template(content_type, &vars)?;
                }
            }
        }
        ProtocolPayload::Sse(payload) => {
            for (name, value) in &mut payload.headers {
                *name = resolve_template(name, &vars)?;
                *value = resolve_template(value, &vars)?;
            }
            if let Some(value) = &mut payload.last_event_id {
                *value = resolve_template(value, &vars)?;
            }
        }
        ProtocolPayload::Tcp(payload) => {
            payload.data = resolve_template(&payload.data, &vars)?;
            if let Some(value) = &mut payload.delimiter {
                *value = resolve_template(value, &vars)?;
            }
            if let Some(value) = &mut payload.server_name {
                *value = resolve_template(value, &vars)?;
            }
            if let Some(value) = &mut payload.ca_cert_ref {
                *value = resolve_template(value, &vars)?;
            }
        }
        ProtocolPayload::Udp(payload) => {
            payload.data = resolve_template(&payload.data, &vars)?;
        }
        ProtocolPayload::Graphql(payload) => {
            payload.query = resolve_template(&payload.query, &vars)?;
            for (name, value) in &mut payload.headers {
                *name = resolve_template(name, &vars)?;
                *value = resolve_template(value, &vars)?;
            }
        }
        ProtocolPayload::Websocket(payload) => {
            for (name, value) in &mut payload.headers {
                *name = resolve_template(name, &vars)?;
                *value = resolve_template(value, &vars)?;
            }
            for message in &mut payload.messages {
                message.data = resolve_template(&message.data, &vars)?;
            }
        }
        ProtocolPayload::Grpc(payload) => {
            for (name, value) in &mut payload.metadata {
                *name = resolve_template(name, &vars)?;
                *value = resolve_template(value, &vars)?;
            }
            if let Some(value) = &mut payload.message_json {
                *value = resolve_template(value, &vars)?;
            }
        }
        ProtocolPayload::Raw(_) => {}
    }

    Ok(request)
}

fn apply_global_parameters(
    request: &mut RequestEnvelope,
    parameters: &[GlobalParameter],
    vars: &HashMap<String, String>,
) -> Result<(), ResolveError> {
    let ProtocolPayload::Http(payload) = &mut request.payload else {
        return Ok(());
    };
    for parameter in parameters.iter().filter(|parameter| parameter.enabled) {
        let name = resolve_template(parameter.name.trim(), vars)?;
        if name.is_empty() {
            continue;
        }
        let value = resolve_template(&parameter.value, vars)?;
        match parameter.location {
            GlobalParameterLocation::Header => {
                if !payload
                    .headers
                    .iter()
                    .any(|(existing, _)| existing.eq_ignore_ascii_case(&name))
                {
                    payload.headers.push((name, value));
                }
            }
            GlobalParameterLocation::Cookie => {
                let cookie_index = payload
                    .headers
                    .iter()
                    .position(|(header, _)| header.eq_ignore_ascii_case("cookie"));
                let already_present = cookie_index
                    .map(|index| cookie_contains(&payload.headers[index].1, &name))
                    .unwrap_or(false);
                if !already_present {
                    if let Some(index) = cookie_index {
                        let separator = if payload.headers[index].1.trim().is_empty() {
                            ""
                        } else {
                            "; "
                        };
                        payload.headers[index].1.push_str(separator);
                        payload.headers[index]
                            .1
                            .push_str(&format!("{name}={value}"));
                    } else {
                        payload
                            .headers
                            .push(("Cookie".into(), format!("{name}={value}")));
                    }
                }
            }
            GlobalParameterLocation::Query => {
                if let Ok(mut url) = reqwest::Url::parse(&request.target) {
                    let exists = url.query_pairs().any(|(key, _)| key == name);
                    if !exists {
                        url.query_pairs_mut().append_pair(&name, &value);
                        request.target = url.into();
                    }
                }
            }
            GlobalParameterLocation::Body => apply_body_parameter(payload, &name, &value),
        }
    }
    Ok(())
}

fn cookie_contains(header: &str, name: &str) -> bool {
    header.split(';').any(|part| {
        part.split_once('=')
            .map(|(key, _)| key.trim() == name)
            .unwrap_or(false)
    })
}

fn apply_body_parameter(payload: &mut HttpPayload, name: &str, value: &str) {
    if !payload.multipart.is_empty() {
        if !payload.multipart.iter().any(|part| part.name == name) {
            payload.multipart.push(MultipartPart {
                name: name.into(),
                value: value.into(),
                file_name: None,
                content_type: None,
                base64: false,
            });
        }
        return;
    }
    let content_type = payload
        .headers
        .iter()
        .find(|(header, _)| header.eq_ignore_ascii_case("content-type"))
        .map(|(_, value)| value.to_ascii_lowercase())
        .unwrap_or_default();
    if content_type.contains("application/x-www-form-urlencoded") {
        let body = payload.body.get_or_insert_with(String::new);
        let encoded_name = form_urlencode_component(name);
        let exists = body.split('&').any(|pair| {
            pair.split_once('=')
                .map(|(key, _)| key == name || key == encoded_name)
                .unwrap_or(false)
        });
        if !exists {
            if !body.is_empty() {
                body.push('&');
            }
            body.push_str(&encoded_name);
            body.push('=');
            body.push_str(&form_urlencode_component(value));
        }
        return;
    }
    let body_is_empty = payload
        .body
        .as_deref()
        .is_none_or(|body| body.trim().is_empty());
    let parsed_body = payload
        .body
        .as_deref()
        .filter(|body| !body.trim().is_empty())
        .and_then(|body| serde_json::from_str::<serde_json::Value>(body).ok());
    let inferred_json = content_type.is_empty()
        && matches!(parsed_body.as_ref(), Some(serde_json::Value::Object(_)));
    if body_is_empty || content_type.contains("application/json") || inferred_json {
        let parsed = parsed_body.unwrap_or_else(|| serde_json::json!({}));
        if let serde_json::Value::Object(mut object) = parsed {
            if !object.contains_key(name) {
                object.insert(name.into(), serde_json::Value::String(value.into()));
                payload.body = serde_json::to_string(&object).ok();
                if !payload
                    .headers
                    .iter()
                    .any(|(header, _)| header.eq_ignore_ascii_case("content-type"))
                {
                    payload
                        .headers
                        .push(("Content-Type".into(), "application/json".into()));
                }
            }
        }
    }
}

fn form_urlencode_component(value: &str) -> String {
    value
        .bytes()
        .map(|byte| match byte {
            b'A'..=b'Z' | b'a'..=b'z' | b'0'..=b'9' | b'-' | b'_' | b'.' | b'~' => {
                (byte as char).to_string()
            }
            b' ' => "+".into(),
            _ => format!("%{byte:02X}"),
        })
        .collect()
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn resolves_nested_scopes_and_dynamics() {
        let mut scope = VariableScope::default();
        scope
            .environment
            .insert("host".into(), "example.com".into());
        scope.request.insert("path".into(), "users".into());
        let out = scope
            .resolve_template("https://{{host}}/{{path}}?t={{$timestamp}}")
            .expect("resolve");
        assert!(out.starts_with("https://example.com/users?t="));
        assert!(!out.contains("{{"));
    }

    #[test]
    fn unresolved_variable_errors() {
        let scope = VariableScope::default();
        let err = scope.resolve_template("{{missing}}").unwrap_err();
        assert!(matches!(err, ResolveError::Unresolved(name) if name == "missing"));
    }

    #[test]
    fn resolves_http_envelope() {
        let mut req = RequestEnvelope::http_get("t", "https://{{host}}/{{id}}");
        req.variables.insert("host".into(), "api.test".into());
        req.variables.insert("id".into(), "42".into());
        req.payload = ProtocolPayload::Http(HttpPayload {
            method: "GET".into(),
            headers: vec![("X-Token".into(), "{{token}}".into())],
            body: None,
            body_encoding: "text".into(),
            body_source: None,
            multipart: vec![],
            follow_redirects: true,
        });
        let mut scope = VariableScope::default();
        scope.environment.insert("token".into(), "abc".into());
        let resolved = resolve_request(req, &scope).expect("ok");
        assert_eq!(resolved.target, "https://api.test/42");
        match resolved.payload {
            ProtocolPayload::Http(p) => assert_eq!(p.headers[0].1, "abc"),
            _ => panic!("expected http"),
        }
    }

    #[test]
    fn applies_typed_global_parameters_without_overriding_request_values() {
        let mut req = RequestEnvelope::http_get("t", "https://example.test/items?local=1");
        req.payload = ProtocolPayload::Http(HttpPayload {
            method: "POST".into(),
            headers: vec![
                ("X-Local".into(), "request".into()),
                ("Cookie".into(), "session=request".into()),
            ],
            body: Some("{\"local\":true}".into()),
            body_encoding: "text".into(),
            body_source: None,
            multipart: vec![],
            follow_redirects: true,
        });
        let parameter = |location, name: &str, value: &str| GlobalParameter {
            location,
            name: name.into(),
            value: value.into(),
            enabled: true,
            description: String::new(),
        };
        let scope = VariableScope {
            global: HashMap::from([("token".into(), "secret".into())]),
            global_parameters: vec![
                parameter(GlobalParameterLocation::Header, "X-Global", "{{token}}"),
                parameter(GlobalParameterLocation::Header, "X-Local", "global"),
                parameter(GlobalParameterLocation::Cookie, "region", "cn"),
                parameter(GlobalParameterLocation::Cookie, "session", "global"),
                parameter(GlobalParameterLocation::Query, "locale", "zh-CN"),
                parameter(GlobalParameterLocation::Body, "global", "yes"),
                parameter(GlobalParameterLocation::Body, "local", "false"),
            ],
            ..VariableScope::default()
        };

        let resolved = resolve_request(req, &scope).expect("resolve");
        assert!(resolved.target.contains("locale=zh-CN"));
        let ProtocolPayload::Http(payload) = resolved.payload else {
            panic!("http")
        };
        assert!(payload
            .headers
            .contains(&("X-Global".into(), "secret".into())));
        assert!(payload
            .headers
            .contains(&("X-Local".into(), "request".into())));
        assert!(payload
            .headers
            .iter()
            .any(|(name, value)| name == "Cookie" && value == "session=request; region=cn"));
        let body: serde_json::Value =
            serde_json::from_str(payload.body.as_deref().unwrap()).unwrap();
        assert_eq!(body["local"], true);
        assert_eq!(body["global"], "yes");
    }
}
