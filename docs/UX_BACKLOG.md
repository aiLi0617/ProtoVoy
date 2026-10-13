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
| UIA-004 | P2 | Character glyphs used as control icons | open |
| UIA-005 | P1 | Hit targets below 36px / 44px | open |
| UIA-006 | P1 | Excessive 9–10px type | open |
| UIA-007 | P1 | Favorite star invisible until hover | open |
| UIA-008 | P2 | Hard-coded colors bypass tokens | open |
| UIA-009 | P2 | Raw z-index numbers vs tokens | open |
| UIA-010 | P1 | Tabs missing keyboard model outside HTTP | open |
| UIA-011 | P1 | Dialogs missing shared focus trap | open |
| UIA-012 | P2 | Dirty create dialogs close on backdrop | open |
| UIA-013 | P1 | Automation rail has no dedicated view | open |
| UIA-014 | P1 | Project settings “克隆项目” has no `onClick` (project cards elsewhere may clone) | open |
| UIA-015 | P2 | Environment / scripts share one settings category | open |
| UIA-016 | P1 | Native buttons missing `type="button"` | open |
| UIA-017 | P1 | Dangerous delete confirm inconsistent | open |
| UIA-018 | P2 | Forms use div+click instead of submit | open |
| UIA-019 | P2 | Field label patterns inconsistent | open |
| UIA-020 | P1 | Field errors not fully aria-linked | open |
| UIA-021 | P2 | Loading / success feedback inconsistent | open |
| UIA-022 | P1 | WS messages / full tree not virtualized | open |
| UIA-023 | P1 | Fixed multi-column layouts break narrow viewports | open |
| UIA-024 | P1 | Project / collection / request terminology drift | open |
| UIA-025 | P2 | Primary run/connect actions inconsistent | open |
| UIA-026 | P2 | Status badge language mixed | open |
| UIA-027 | P2 | Disabled opacity hurts contrast | open |
| UIA-028 | P2 | Custom menus lack keyboard model | open |
| UIA-029 | P2 | `100vh` + overflow hidden on mobile | open |
| UIA-030 | P1 | Design docs marked complete while UX debt remains | open |
| UIA-031 | P2 | No component gallery / theme matrix | open |
| UIA-032 | P2 | E2E thin on keyboard / i18n / contrast | open |
| UIA-033 | P2 | Empty states not using shared EmptyState | open |
| UIA-034 | P2 | ⌘K shown on Windows | open |
| UIA-035 | P1 | Settings / tools discovery scattered | open |
| UIA-036 | P2 | No per-workbench acceptance matrix | open |
| UIA-037 | P1 | `html lang` not synced on boot | open |
| UIA-038 | P1 | Lifecycle tablist accessible name is escape soup | open |
| UIA-039 | P1 | Light theme contrast failures on tool pages | open |
| UIA-040 | P1 | Mobile clips HTTP/SSE/WS internal fields | open |
| UIA-041 | P1 | Agent errors are opaque “Failed to fetch” | open |
| UIA-042 | P2 | Capture proxy text concatenation | open |
| UIA-043 | P2 | Mobile keeps permanent project rail | open |
| UIA-044 | P2 | Status bar shows internal IDs | open |
| UIA-045 | P2 | Mobile top bar loses project context | open |
| UIA-046 | P2 | Tool workbenches lack mobile task flow | open |

## Notes

- Do not mark [UI_UX_DESIGN_SYSTEM.md](./UI_UX_DESIGN_SYSTEM.md) rows as “done” without a closed UIA/ISS and test evidence (UIA-030).
- Closed historical IA walkthrough: [archive/UX_WALKTHROUGH.md](./archive/UX_WALKTHROUGH.md) (not SoT).
- Detail and evidence for each ID remain in the dated audit archive; update **this** index when status changes.
