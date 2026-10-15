import { useEffect, useState } from "react";
import { EmptyState } from "./Feedback";
import { Button, Checkbox, InlineAlert, StatusBadge } from "./Components";

export interface CollectionRunCase { requestId: string; name: string; protocolId: string; passed: boolean; status?: number | null; durationMs: number; error?: string | null; failedAssertions: string[] }
export interface CollectionEnvironmentOption { id: string; name: string }
export interface CollectionRunProgress { runId: string; completed: number; total: number; state: "running" | "cancelling" }
export interface CollectionRunnerProps {
  collectionId: string;
  onRun: (collectionId: string, failFast: boolean, environmentId: string | undefined, runId: string) => Promise<CollectionRunCase[]>;
  onGetProgress?: (runId: string) => Promise<CollectionRunProgress>;
  onCancel?: (runId: string) => Promise<void>;
  onListEnvironments?: () => Promise<CollectionEnvironmentOption[]>;
  environmentScope?: string;
}

export function CollectionRunner({ collectionId, onRun, onGetProgress, onCancel, onListEnvironments, environmentScope }: CollectionRunnerProps) {
  const [failFast, setFailFast] = useState(false);
  const [running, setRunning] = useState(false);
  const [cases, setCases] = useState<CollectionRunCase[]>([]);
  const [error, setError] = useState("");
  const [environmentId, setEnvironmentId] = useState("");
  const [environments, setEnvironments] = useState<CollectionEnvironmentOption[]>([]);
  const [runId, setRunId] = useState("");
  const [progress, setProgress] = useState<CollectionRunProgress | null>(null);
  useEffect(() => {
    if (!onListEnvironments) return;
    let active = true;
    void onListEnvironments()
      .then((items) => { if (active) setEnvironments(items); })
      .catch(() => { if (active) setEnvironments([]); });
    return () => { active = false; };
  }, [collectionId, environmentScope]);
  useEffect(() => {
    if (!running || !runId || !onGetProgress) return;
    let active = true;
    const poll = async () => {
      try { const next = await onGetProgress(runId); if (active) setProgress(next); } catch { /* The POST result remains authoritative. */ }
    };
    void poll();
    const timer = window.setInterval(() => void poll(), 250);
    return () => { active = false; window.clearInterval(timer); };
  }, [running, runId, onGetProgress]);
  async function run() {
    const nextRunId = crypto.randomUUID();
    setRunId(nextRunId); setProgress({ runId: nextRunId, completed: 0, total: 0, state: "running" }); setRunning(true); setError(""); setCases([]);
    try { setCases(await onRun(collectionId, failFast, environmentId || undefined, nextRunId)); }
    catch (value) { setError(value instanceof Error ? value.message : String(value)); }
    finally { setRunning(false); setRunId(""); }
  }
  async function cancel() {
    if (!runId || !onCancel) return;
    setProgress((current) => current ? { ...current, state: "cancelling" } : current);
    try { await onCancel(runId); } catch (value) { setError(value instanceof Error ? value.message : String(value)); }
  }
  const passed = cases.filter((item) => item.passed).length;
  return <section className="collection-runner"><div className="collection-runner-header"><div><small>COLLECTION RUNNER</small><h2>集合运行报告</h2></div><div className="collection-runner-actions"><label className="collection-runner-environment"><span>运行环境</span><select aria-label="集合运行环境" value={environmentId} disabled={running} onChange={(event) => setEnvironmentId(event.target.value)}><option value="">按请求配置</option>{environments.map((environment) => <option key={environment.id} value={environment.id}>{environment.name}</option>)}</select></label><Checkbox label="失败即停" checked={failFast} disabled={running} onChange={(event) => setFailFast(event.target.checked)} />{running && onCancel ? <Button variant="danger" disabled={progress?.state === "cancelling"} onClick={() => void cancel()}>{progress?.state === "cancelling" ? "正在取消…" : "取消运行"}</Button> : <Button variant="primary" disabled={!collectionId} onClick={() => void run()}>运行当前集合</Button>}</div></div>{running && progress ? <div className="collection-runner-progress" role="status" aria-live="polite"><span>{progress.state === "cancelling" ? "正在取消" : "运行中"}</span><strong>{progress.completed}/{progress.total || "—"}</strong><progress aria-label="集合运行进度" max={Math.max(1, progress.total)} value={progress.completed} /></div> : null}{error && <InlineAlert tone="danger" title="运行失败">{error}</InlineAlert>}{cases.length === 0 && !running && !error ? <EmptyState title="尚未运行" description="选择集合后点击运行，报告会出现在这里。" /> : null}{cases.length > 0 && <><div className="collection-runner-summary">{passed}/{cases.length} 通过 · {cases.reduce((sum, item) => sum + item.durationMs, 0)} ms</div><div>{cases.map((item) => <div key={item.requestId} className="collection-runner-case"><StatusBadge tone={item.passed ? "success" : "danger"}>{item.passed ? "PASS" : "FAIL"}</StatusBadge><span>{item.protocolId}</span><strong>{item.name}</strong><span>{item.status ?? "—"}</span><span>{item.durationMs} ms</span><small>{item.error ?? item.failedAssertions.join(", ")}</small></div>)}</div></> }</section>;
}
