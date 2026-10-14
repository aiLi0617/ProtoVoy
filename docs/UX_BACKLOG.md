# UX backlog (UIA-*)

> Living UX / a11y / visual debt. Source of truth for open UI work.
>
> Historical full audit: [archive/UI_UX_AUDIT_2026-08-31.md](./archive/UI_UX_AUDIT_2026-08-31.md)
>
> Status: `open` · `partial` · `done` · `wontfix`
>
> Design norms (not status): [UI_UX_DESIGN_SYSTEM.md](./UI_UX_DESIGN_SYSTEM.md)
>
> Engineering bugs: [ISSUES.md](./ISSUES.md)

## Index

| ID | Level | Summary | Status |
|----|-------|---------|--------|
| UIA-001 | P1 | Workbench private controls vs shared primitives | open |
| UIA-002 | P1 | Incomplete i18n across workbenches | open |
| UIA-003 | P1 | Protocol / workbench icons collide on `bolt` | open |
| UIA-004 | P2 | Character glyphs used as control icons | done |
| UIA-005 | P1 | Hit targets below 36px / 44px | open |
| UIA-006 | P1 | Excessive 9–10px type | open |
| UIA-007 | P1 | Favorite star invisible until hover | done |
| UIA-008 | P2 | Hard-coded colors bypass tokens | open |
| UIA-009 | P2 | Raw z-index numbers vs tokens | open |
| UIA-010 | P1 | Tabs missing keyboard model outside HTTP | partial |
| UIA-011 | P1 | Dialogs missing shared focus trap | open |
| UIA-012 | P2 | Dirty create dialogs close on backdrop | done |
| UIA-013 | P1 | Automation rail has no dedicated view | done |
| UIA-014 | P1 | Project settings “克隆项目” has no `onClick` (project cards elsewhere may clone) | done |
| UIA-015 | P2 | Environment / scripts share one settings category | done |
| UIA-016 | P1 | Native buttons missing `type="button"` | done |
| UIA-017 | P1 | Dangerous delete confirm inconsistent | partial |
| UIA-018 | P2 | Forms use div+click instead of submit | open |
| UIA-019 | P2 | Field label patterns inconsistent | open |
| UIA-020 | P1 | Field errors not fully aria-linked | open |
| UIA-021 | P2 | Loading / success feedback inconsistent | open |
| UIA-022 | P1 | WS messages / full tree not virtualized | open |
| UIA-023 | P1 | Fixed multi-column layouts break narrow viewports | open |
| UIA-024 | P1 | Project / collection / request terminology drift | open |
| UIA-025 | P2 | Primary run/connect actions inconsistent | open |
| UIA-026 | P2 | Status badge language mixed | partial |
| UIA-027 | P2 | Disabled opacity hurts contrast | partial |
| UIA-028 | P2 | Custom menus lack keyboard model | open |
| UIA-029 | P2 | `100vh` + overflow hidden on mobile | done |
| UIA-030 | P1 | Design docs marked complete while UX debt remains | open |
| UIA-031 | P2 | No component gallery / theme matrix | open |
| UIA-032 | P2 | E2E thin on keyboard / i18n / contrast | open |
| UIA-033 | P2 | Empty states not using shared EmptyState | open |
| UIA-034 | P2 | ⌘K shown on Windows | done |
| UIA-035 | P1 | Settings / tools discovery scattered | open |
| UIA-036 | P2 | No per-workbench acceptance matrix | open |
| UIA-037 | P1 | `html lang` not synced on boot | done |
| UIA-038 | P1 | Lifecycle tablist accessible name is escape soup | done |
| UIA-039 | P1 | Light theme contrast failures on tool pages | open |
| UIA-040 | P1 | Mobile clips HTTP/SSE/WS internal fields | open |
| UIA-041 | P1 | Agent errors are opaque “Failed to fetch” | partial |
| UIA-042 | P2 | Capture proxy text concatenation | done |
| UIA-043 | P2 | Mobile keeps permanent project rail | done |
| UIA-044 | P2 | Status bar shows internal IDs | done |
| UIA-045 | P2 | Mobile top bar loses project context | open |
| UIA-046 | P2 | Tool workbenches lack mobile task flow | partial |

## Notes

- 2026-10-13 audit evidence: UIA-013/014/037/044 closed; UIA-041 is partial because workspace bootstrap errors are actionable, while protocol/tool errors still need a shared structured model. See [PROJECT_AUDIT_2026-10-13.md](./PROJECT_AUDIT_2026-10-13.md).

- 2026-10-14 remediation: UIA-007/012/016/029/034/038/042/043 closed with unit or browser regression coverage. Capture now uses an actionable execution-error mapper, so UIA-041 advances but remains partial until history and every protocol workbench migrate. UIA-046 remains partial: Capture has a narrow-screen task flow, while Gateway/AI and other tool pages still need the same treatment.

- 2026-10-14 follow-up: UIA-010 is partial after migrating Mock, WebSocket, Kafka, MQTT, AMQP and the existing protocol workbenches to roving `tabIndex`/arrow-key behavior, backed by `tabKeyboardModel.test.ts`; full tab/panel ID linkage remains. UIA-015 is closed by the single “项目资源” settings entry. UIA-017 is partial after project scripts, environments and gateway jobs adopted shared danger confirmation; remaining inline case/tool clear actions still need policy review.

- 2026-10-14 icon follow-up: UIA-004 is closed after product controls migrated to the shared Lucide-backed icon abstraction. `controlIconGlyphs.test.ts` rejects character glyphs in direct button labels; protocol payload and traffic-direction symbols remain permitted as data.

- 2026-10-14 contrast follow-up: UIA-027 is partial. Shared buttons, checkboxes and switches now retain full foreground opacity and use semantic muted colors; non-control page sections that use `.is-disabled` still need a case-by-case visual pass.

- 2026-10-14 status-language follow-up: UIA-026 is partial. Capture and Comments now use Chinese state labels; AI、SSO、Gateway and Team still contain English status badges that should move into the locale dictionary rather than receive more page-local replacements.

- Do not mark [UI_UX_DESIGN_SYSTEM.md](./UI_UX_DESIGN_SYSTEM.md) rows as “done” without a closed UIA/ISS and test evidence (UIA-030).
- Closed historical IA walkthrough: [archive/UX_WALKTHROUGH.md](./archive/UX_WALKTHROUGH.md) (not SoT).
- Detail and evidence for each ID remain in the dated audit archive; update **this** index when status changes.
