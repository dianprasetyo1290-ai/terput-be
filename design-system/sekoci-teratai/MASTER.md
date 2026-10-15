# SEKOCI Teratai — Design System (MASTER)

Source of truth for the in-app UI (web/src). Page files in `pages/` override
this file for one page only. Since 2026-10-15 the look follows the **FC Design
System** (design-component-fc-main), ported to Vue + Tailwind. Icons stay
lucide-vue-next. Light mode, WCAG AA.

## Style
- Grey canvas, white cards **without borders**: a thin layered shadow defines the edge.
- Pills everywhere you click or read a state: buttons, status badges, search inputs.
- Solid colored figure cards for key numbers (3–5 per row, never more).
- Black tab bar with a white pill for the active tab.
- Color carries meaning (status), never decoration. One primary action per screen.
- Icons: lucide-vue-next, 16–20px, `aria-hidden="true"` when decorative. No emoji as icons.

## Tokens (web/src/tailwind.css @theme + CSS vars in web/src/style.css)
| Token | Hex | Use |
|---|---|---|
| background / `--dash-canvas` | #F3F4F6 | page canvas |
| surface | #FFFFFF | cards, tables, dialogs |
| on-surface / `--dash-text` | #161616 | headings, body text |
| on-surface-variant / `--dash-sub` | #5C5C5C | secondary text, table headers |
| muted | #6F6F6F | captions, placeholders only |
| border / `--dash-line` | #E8E8E8 | row dividers, input borders |
| surface-container-low / `--dash-muted` | #F4F4F4 | row hover, neutral pills, notes |
| primary / `--dash-accent` | #1E6BF6 | primary buttons, links, active nav |
| primary-light | #EAF1FE | active nav background |
| ink / `--dash-ink` | #161616 | tab bar, ink buttons, hero badge |
| tone success / warning / error / info / neutral | #15803D / #B45309 / #DC2626 / #1E6BF6 / #262626 | solid status pills and figures |

Shadows: `--dash-shadow` (cards; Tailwind `shadow-md`), `--dash-shadow-sm` (buttons, small
controls; Tailwind `shadow-sm`). Radius: cards 20px (`--dash-radius-card`), dialogs 22px,
inputs 14px, buttons/pills 999px.

Rules: no raw hex in views; use tokens or the `fc-*` classes. Status = color + text.

## Typography
- Font **Roboto Flex** (variable 400–800), fallback system-ui. Body 14px, line-height 1.5.
- Page title 26px/700, card title 15px/700, table header 11px/600 uppercase +0.04em,
  captions 12px. Numbers: `tabular-nums` (`fc-num`).

## Global classes (web/src/style.css)
- Cards: `fc-card` (padded 18px), `fc-card-flush` (for tables), `fc-card-head` +
  `fc-card-title` (lucide icon + title) + `fc-count` (row count pill), `fc-card-body`.
  Existing `rounded-xl … shadow-sm` cards automatically adopt the FC card look.
- Tables: `fc-table`; numeric cells `fc-num`; actions `td.fc-actions` with an
  `inline-flex gap-1.5` wrapper (never stacked buttons); secondary line `fc-sub`.
- Status: `<Badge variant>` (solid pill) or `.fc-status[data-tone]`.
- Figures: `fc-figures` grid with `fc-figure[data-tone]` (`fc-figure-label`, `-value`, `-hint`);
  `StatCard` renders one.
- Tabs: `fc-tabs` / `fc-tab[aria-selected]` or `<Tabs>` components (same look).
- Forms: `fc-form` grid, `fc-field`, `fc-label`, `fc-help`, `fc-span` (full width);
  fields `fc-input` / `fc-select` / `fc-textarea` (bare inputs get the same style).
- Filters: `fc-filterbar` (single white pill bar; search left, filters, reset).
- Buttons: `<Button>` variants default (blue) / ink / secondary (white + shadow) /
  outline / destructive / ghost / link; legacy `.btn .btn-*` map to the same look.

## Page anatomy
1. `<PageHeader title description>` with actions slot (max 1 primary).
2. Optional `fc-filterbar`.
3. Optional `fc-figures` row (3–5 figures).
4. Content: `fc-card-flush` with `fc-card-head` (icon + title + count) and an `fc-table`,
   horizontal scroll inside the card on small screens, never page-level.
5. States: `LoadingState` (skeleton), `EmptyState`, inline `Alert variant="destructive"`.
- Dialogs: `<Dialog>`; confirmations/prompts only via `confirmDialog` / `alertDialog` /
  `promptDialog` (components/ui). **Native alert/confirm/prompt are forbidden.**

## Interaction & accessibility
- Visible focus ring on all interactive elements; `cursor-pointer` on clickables;
  transitions 150ms; respect prefers-reduced-motion.
- Icon-only buttons need `aria-label`. Loading buttons show a spinner and are disabled.
- Check at 1366×768 and 390×844; no horizontal page scroll.

## Do not change during redesign
Behavior, API calls, routes, props/emits, data-testids, form field names, permission checks.
