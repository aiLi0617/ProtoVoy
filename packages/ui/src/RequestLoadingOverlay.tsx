import { useI18n } from "./i18n";

export interface RequestLoadingOverlayProps {
  active: boolean;
  label?: string;
}

export function RequestLoadingOverlay({ active, label }: RequestLoadingOverlayProps) {
  const { t } = useI18n();
  if (!active) return null;
  const accessibleLabel = label ?? t("request.loading");
  return <div className="request-loading-overlay" role="status" aria-live="polite" aria-label={accessibleLabel}>
    <div className="request-loading-overlay-card">
      <span className="request-loading-indicator" aria-hidden="true"><span className="request-loading-spinner"/></span>
      <span className="request-loading-copy"><strong>{accessibleLabel}</strong><small>{t("request.loadingHint")}</small></span>
    </div>
  </div>;
}
