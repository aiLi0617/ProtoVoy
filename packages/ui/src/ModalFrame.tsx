import { useRef, type FormEventHandler, type ReactNode, type RefObject } from "react";
import { useStackedDialogFocus } from "./useStackedDialogFocus";

export interface ModalFrameProps {
  id?: string;
  open: boolean;
  onClose: () => void;
  children: ReactNode;
  className: string;
  overlayClassName?: string;
  ariaLabel?: string;
  ariaLabelledBy?: string;
  ariaDescribedBy?: string;
  initialFocusRef?: RefObject<HTMLElement | null>;
  closeOnBackdrop?: boolean;
  closeOnEscape?: boolean;
  role?: "dialog" | "alertdialog";
  as?: "div" | "form";
  onSubmit?: FormEventHandler<HTMLFormElement>;
}

/** Headless modal shell for product-specific dialog and drawer layouts. */
export function ModalFrame({ id, open, onClose, children, className, overlayClassName = "dialog-backdrop", ariaLabel, ariaLabelledBy, ariaDescribedBy, initialFocusRef, closeOnBackdrop = true, closeOnEscape = true, role = "dialog", as = "div", onSubmit }: ModalFrameProps) {
  const dialogRef = useRef<HTMLElement>(null);
  useStackedDialogFocus(open, dialogRef, closeOnEscape ? onClose : () => undefined, initialFocusRef);
  if (!open) return null;
  return <div className={overlayClassName} role="presentation" onMouseDown={() => { if (closeOnBackdrop) onClose(); }}>
    {as === "form"
      ? <form id={id} ref={(node) => { dialogRef.current = node; }} tabIndex={-1} className={className} role={role} aria-modal="true" aria-label={ariaLabel} aria-labelledby={ariaLabelledBy} aria-describedby={ariaDescribedBy} onMouseDown={(event) => event.stopPropagation()} onSubmit={onSubmit}>{children}</form>
      : <div id={id} ref={(node) => { dialogRef.current = node; }} tabIndex={-1} className={className} role={role} aria-modal="true" aria-label={ariaLabel} aria-labelledby={ariaLabelledBy} aria-describedby={ariaDescribedBy} onMouseDown={(event) => event.stopPropagation()}>{children}</div>}
  </div>;
}
