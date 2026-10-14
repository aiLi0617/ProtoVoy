import { CodeGenerator } from "./CodeGenerator";
import type { HttpWorkbenchRequest } from "./HttpWorkbench";

function formatBytes(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(bytes < 10 * 1024 ? 1 : 0)} KB`;
  return `${(bytes / 1024 / 1024).toFixed(1)} MB`;
}

function requestBodySize(request: HttpWorkbenchRequest): number {
  return new TextEncoder().encode(request.body ?? "").length;
}

export function HttpLiveRequestView({ request }: { request: HttpWorkbenchRequest }) {
  const contentType = request.headers.find(([header]) => header.toLowerCase() === "content-type")?.[1] ?? formatBytes(requestBodySize(request));
  return <div className="http-live-request">
    <section className="http-live-url-section"><h4>请求 URL:</h4><div className="http-request-summary" data-method={request.method.toUpperCase()}><strong>{request.method}</strong><code title={request.url}>{request.url}</code></div></section>
    <section className="http-live-data-section"><h4>Header:</h4><div className="http-request-data" role="table" aria-label="实时请求 Header">{request.headers.length ? <><div className="http-request-data-heading" role="row"><strong role="columnheader">名称</strong><span role="columnheader">值</span></div>{request.headers.map(([name, value], index) => <div role="row" key={`${name}-${index}`}><strong role="cell">{name}</strong><span role="cell">{value}</span></div>)}</> : <p>（空）</p>}</div></section>
    <section className="http-live-data-section"><h4>Body <span>{contentType}</span></h4><pre className={`http-request-body${request.body ? "" : " is-empty"}`}>{request.body || "（空）"}</pre></section>
    <section className="http-live-code-section"><CodeGenerator request={request}/></section>
  </div>;
}
