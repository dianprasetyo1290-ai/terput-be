# SEKOCI Teratai — Design System (MASTER)

Source of truth for the in-app UI (web/src). Page files in `pages/` override
this file for one page only. Chosen 2026-10-15: SEKOCI blue, Minimalism &
Swiss style for enterprise dashboards, light mode, WCAG AA.

## Style
- Clean, functional, grid-based, generous but dense-enough spacing (dashboard density).
- One accent per screen; color carries meaning (status), never decoration.
- Flat surfaces: white cards on a slate-50 background, 1px borders, `shadow-sm` at most.
- Icons: lucide-vue-next only, 16–20px, `aria-hidden="true"` when decorative. No emoji as icons.
- Avoid: gradients on content, glassmorphism, colored page backgrounds, heavy shadows,
  ALL CAPS body text, more than 2 font weights in one block.

## Color tokens (Tailwind theme names in web/src/tailwind.css)
| Token | Hex | Use |
|---|---|---|
| primary | #2563EB | main buttons, links, active nav, focus ring |
| primary-hover | #1D4ED8 | hover/pressed of primary |
| primary-light | #EFF6FF | selected rows, active nav background, info chips |
| accent | #EA580C | rare, the single most important CTA on a page |
| background | #F8FAFC | app/page background |
| surface | #FFFFFF | cards, tables, dialogs |
| border | #E2E8F0 | card/table/input borders |
| on-surface | #0F172A | headings, primary text |
| on-surface-variant | #475569 | secondary text, labels (4.5:1 on white) |
| muted | #64748B | captions, placeholders only (not body text) |
| success / success-light | #15803D / #F0FDF4 | paid, approved, active |
| warning / warning-light | #B45309 / #FFFBEB | pending, due soon |
| error / error-light | #B91C1C / #FEF2F2 | failed, overdue, destructive |
| info / info-light | #0369A1 | informational notes |

Rules: no raw hex in components; prefer the tokens above (`bg-primary`, `text-on-surface-variant`,
`border-border`, …). Existing `slate-*` utilities are acceptable for neutrals; replace `indigo-*`,
`violet-*`, `purple-*` primaries with `primary` tokens. Status always = color + text/icon.

## Typography
- Font: **Plus Jakarta Sans** (400/500/600/700), fallback system-ui. Numbers in tables: `tabular-nums`.
- Scale: page title 24px/700 (`text-2xl font-bold`), section title 18px/600, card title 16px/600,
  body 14px/400 (`text-sm`) with line-height 1.5, captions 12px/500. Never below 12px.

## Spacing & shape
- 4px grid. Page padding 24px desktop / 16px mobile. Gap between cards 16–24px.
- Radius: 8px inputs/buttons (`rounded-lg`), 12px cards/dialogs (`rounded-xl`), chips `rounded-full`.
- Controls min height 36px (buttons, inputs, selects); touch targets ≥ 40px on mobile.

## Page anatomy (every feature page)
1. `<PageHeader title description>` with actions slot (primary action right-aligned, max 1 primary).
2. Optional filter bar: a single row card (search left, filters, reset) wrapping on mobile.
3. Content: `Card` with table/list, or a responsive grid of `StatCard`s on top for dashboards.
4. States: `LoadingState` (skeleton), `EmptyState` (icon + title + hint + optional action),
   inline `Alert variant="destructive"` for load errors (message from apiErrorMessage).
- Tables: sticky-free simple header `bg-slate-50 text-xs font-semibold text-on-surface-variant`,
  rows `hover:bg-slate-50`, numeric columns right-aligned, actions in the last column,
  horizontal scroll inside the card on small screens (`overflow-x-auto`), never page-level.
- Forms: visible label above every field, helper text below, error under the field in error color,
  required marked with `*`, primary submit right in dialog footer, Cancel as outline.
- Dialogs: max-w-lg (forms) / max-w-2xl (detail), title + description, footer actions.
- Status badges: `Badge` with semantic variant (success/warning/destructive/secondary/info).

## Interaction & accessibility
- Visible focus ring (`focus-visible:ring-2 ring-primary/40`) on all interactive elements.
- `cursor-pointer` on clickable things, hover transitions 150–200ms, respect prefers-reduced-motion.
- Icon-only buttons need `aria-label`. Disabled buttons explain why (title/tooltip or helper text).
- Loading: buttons show spinner + disabled while submitting; never double-submit.
- Responsive check at 375, 768, 1024, 1440px; no horizontal page scroll.

## Do not change during redesign
Behavior, API calls, routes, props/emits, data-testids, form field names, permission checks.
Redesign is presentational only.
