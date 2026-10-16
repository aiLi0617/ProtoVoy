import type { ReactNode } from "react";
import { createPortal } from "react-dom";
import type { HttpRunResult } from "./HttpWorkbench";
import { SplitPane } from "./WorkbenchFrame";
import { RovingTabList } from "./RovingTabList";
import { useInterfaceNameTarget } from "./InterfaceNameTarget";
import { RequestLoadingOverlay } from "./RequestLoadingOverlay";
import { Button, TextInput } from "./Components";
import { useI18n } from "./i18n";

interface ProtocolWorkbenchLayoutProps {
  id: string; protocol: string; name: string; target: string; targetLabel: string;
  actionLabel: string; loadingLabel?: string; loading: boolean; result: HttpRunResult | null;
  responseTitle: string; emptyResponse: string; message?: string; controls?: ReactNode; children: ReactNode;
  onNameChange: (value: string) => void; onTargetChange: (value: string) => void;
  onRun: () => void; onSave?: () => void; onCancel?: () => void;
}

export function ProtocolWorkbenchLayout({ id, protocol, name, target, targetLabel, actionLabel, loadingLabel, loading, result, responseTitle, emptyResponse, message, controls, children, onNameChange, onTargetChange, onRun, onSave, onCancel }: ProtocolWorkbenchLayoutProps) {
  const { t } = useI18n();
  const nameTarget = useInterfaceNameTarget();
  const metrics = result ? <span className="protocol-response-metrics">{result.summary.durationMs} ms · {result.summary.bytesReceived} B</span> : null;
  const nameInput = <TextInput aria-label={t("field.interfaceName")} className="protocol-name-input" value={name} onChange={(event) => onNameChange(event.target.value)} />;
  return <>{nameTarget ? createPortal(nameInput, nameTarget) : null}<div className="protocol-workbench-layout" aria-busy={loading || undefined}>
      <div className="protocol-commandbar">
        <span className="protocol-command-badge">{protocol}</span>
        {nameTarget ? null : nameInput}
        <TextInput aria-label={targetLabel} className="protocol-target-input" value={target} onChange={(event) => onTargetChange(event.target.value)} />
        <Button variant="primary" className="protocol-run-button" loading={loading} onClick={onRun}>{loading ? loadingLabel ?? t("status.running") : actionLabel}</Button>
        {onSave ? <Button variant="secondary" className="protocol-save-button" disabled={loading} onClick={onSave}>{t("action.save")}</Button> : null}
        {loading && onCancel ? <Button variant="danger" className="protocol-cancel-button" onClick={onCancel}>{t("action.cancel")}</Button> : null}
      </div>
      <div className="protocol-workbench-split">
        <SplitPane id={`${id}-workbench`} direction="vertical" minPrimary={180} minSecondary={140} primaryLabel={t("region.requestConfig")} secondaryLabel={t("region.responseInspector")} secondaryActions={metrics}
          primary={<div className="protocol-request-pane"><div className="request-interaction-content protocol-request-interaction-content" inert={loading ? true : undefined}>{controls ? <RovingTabList className="protocol-mode-tabs" ariaLabel={t("workbench.operationMode", { protocol })}>{controls}</RovingTabList> : null}<div className="protocol-request-content">{children}</div></div><RequestLoadingOverlay active={loading}/></div>}
          secondary={<div className="protocol-response-pane"><div className="protocol-response-tabs"><button type="button" className="is-active">{responseTitle}</button></div><pre className={`protocol-response-body${result ? " has-result" : " is-empty"}`}>{result?.preview ?? emptyResponse}</pre></div>}
        />
      </div>
      {message ? <div className="protocol-status" role="status" aria-live="polite">{message}</div> : null}
  </div></>;
}
