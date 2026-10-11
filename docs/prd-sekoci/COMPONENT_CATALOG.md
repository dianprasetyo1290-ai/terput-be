# SEKOCI Component Catalog
## Reusable UI Component Specification

**Document ID:** SEKOCI-UX-COMP-001  
**Version:** 1.0  
**Status:** Component Reference  
**Product:** SEKOCI School Management SaaS  
**Related source of truth:** `DESIGN.md`  
**Component foundation:** shadcn/ui  
**Charting library:** Apache ECharts

---

## 1. Purpose

This document defines the reusable UI component inventory and implementation guidance for SEKOCI. It complements `DESIGN.md`; it does not replace the product-wide design principles, design tokens, responsive standards, accessibility requirements, or interaction rules defined there.

The catalog describes component intent, variants, states, responsive behavior, accessibility expectations, and usage guidance. Business rules, workflows, permissions, status transitions, and acceptance criteria remain governed by the approved PRD/SRS.

## 2. Source-of-Truth Hierarchy

1. **PRD / SRS:** requirements, business rules, workflow, roles, permissions, and acceptance criteria.
2. **`DESIGN.md`:** shared visual language, design tokens, responsive behavior, interaction principles, and accessibility.
3. **`COMPONENT_CATALOG.md`:** reusable component specifications and composition guidance.
4. **Approved prototype:** screen-level composition and reviewed user flows.
5. **Implementation:** production behavior aligned with all approved sources.

If a conflict is found, do not silently invent a resolution. Record the conflict and clarify it with the product owner.

## 3. Component Principles

- Reuse existing components before introducing a new pattern.
- Use shadcn/ui as the primary component foundation and apply SEKOCI design tokens.
- Keep component APIs and behavior consistent across portals.
- Prefer composition over duplicated, screen-specific implementations.
- Keep visual variants limited and purposeful.
- Every interactive component must define relevant states, keyboard behavior, accessible naming, and responsive behavior.
- Do not use color as the only way to communicate status.
- Hide or disable actions according to role and record state, while enforcing authorization on the server.
- Avoid one-off colors, arbitrary radii, inconsistent icon styles, excessive shadows, and unnecessary animations.
- Do not add a component solely because it appears in another product; a component must support a traceable requirement or a validated user need.

## 4. Shared Design Tokens

Use the values in `DESIGN.md` as the authoritative token source. The following values are repeated here for quick reference and must not drift independently.

### 4.1 Colors

| Token | Value | Intended use |
|---|---|---|
| Primary | `#2563EB` | Primary actions, active navigation, links, focus-related emphasis |
| Primary Dark | `#1E40AF` | Stronger primary emphasis where specified |
| Primary Light | `#EFF6FF` | Subtle primary surfaces and selected states |
| Background | `#F8FAFC` | Application background |
| Surface | `#FFFFFF` | Cards, panels, dialogs, tables |
| Border | `#E2E8F0` | Dividers, outlines, table borders |
| Text | `#0F172A` | Primary text |
| Secondary | `#64748B` | Supporting text and secondary icons |
| Muted | `#94A3B8` | Muted labels and disabled visuals |

Semantic colors must use the shared tokens defined in `DESIGN.md`; do not invent new semantic values at component level.

### 4.2 Shape and Spacing

- Controls: 6px radius.
- Cards: 8px radius.
- Dialogs: 10px radius.
- Containers: 12px radius.
- Spacing scale: 4, 8, 12, 16, 20, 24, 32, 40, 48.
- Shadows: subtle; prioritize borders and spacing over elevated card shadows.

### 4.3 Iconography

- Default to a consistent outline icon family.
- Use neutral/slate icons for general navigation and actions; use SEKOCI blue for active states.
- Reserve semantic colors for meaningful statuses.
- Do not give every module a different bright icon color.
- Interactive icons require accessible names or tooltips.
- Do not use emoji as operational UI icons.

## 5. Standard Component Specification

Each component entry uses the following fields:

- **Purpose:** the user need it supports.
- **Foundation:** recommended shadcn/ui component or implementation pattern.
- **Variants:** permitted visual or behavioral variants.
- **States:** states that must be designed and tested.
- **Responsive behavior:** expected behavior across desktop, tablet, and mobile.
- **Accessibility:** minimum interaction and assistive-technology requirements.
- **Usage rules:** guidance to keep usage consistent.

Where a requirement is not applicable, mark it N/A rather than inventing behavior.

---

## 6. Actions and Feedback

### 6.1 Button

**Purpose:** Trigger an action or navigate to a related workflow.

**Foundation:** shadcn/ui Button.

**Variants**
- `primary`: one clear primary action per context where possible.
- `secondary`: lower-emphasis action.
- `outline`: action with visible border.
- `ghost`: low-emphasis contextual action.
- `destructive`: delete, reject, or other high-impact action when allowed.
- `link`: navigation-style action.

**Sizes**
- `sm`: dense tables or compact controls.
- `default`: standard forms and page actions.
- `lg`: prominent actions in simplified flows.
- `icon`: icon-only control, only when its meaning is clear and it has an accessible name.

**States**
- Default, hover, focus-visible, active, disabled, loading, and success feedback where appropriate.

**Responsive behavior**
- Avoid overflowing action groups.
- On mobile, stack or wrap primary/secondary actions; preserve a clear primary action.
- Provide usable touch targets; do not make the interactive area as small as the icon glyph.

**Accessibility**
- Use semantic buttons for actions.
- Loading state must prevent accidental duplicate submission where relevant.
- Destructive actions must have clear labels and confirmation when required by `DESIGN.md` or the PRD.

**Usage rules**
- Use concise verb-first labels such as `Simpan`, `Tambah Pendaftar`, `Ekspor`, or `Ajukan`.
- Do not use multiple equally prominent primary actions in one local context without a clear reason.

### 6.2 Icon Button

**Purpose:** Provide a compact action such as search, notification, or row menu.

**Foundation:** Button with icon.

**Variants:** Default, ghost, outline, destructive where appropriate.

**States:** Default, hover, focus-visible, active, disabled, loading if relevant.

**Responsive behavior:** Preserve touch target size on mobile.

**Accessibility:** Provide an accessible name; add a tooltip on desktop when the action is not self-evident.

**Usage rules:** Do not use ambiguous icons for irreversible actions without a clear label or confirmation.

### 6.3 Link

**Purpose:** Navigate to another page or resource.

**Foundation:** Semantic link styled with SEKOCI tokens.

**Variants:** Default, muted, inline, external.

**States:** Default, hover, focus-visible, visited if relevant.

**Responsive behavior:** Long links must wrap without breaking the layout.

**Accessibility:** Link text must describe its destination or action.

### 6.4 Badge / Status Badge

**Purpose:** Show a compact status, category, or count.

**Foundation:** shadcn/ui Badge, extended with SEKOCI semantic tokens.

**Variants**
- `neutral`
- `info`
- `success`
- `warning`
- `error`
- `outline`

**States:** Static by default; interactive only when explicitly designed as a filter or action.

**Responsive behavior:** Labels may wrap or use a defined compact treatment; never truncate a critical status without an accessible alternative.

**Accessibility:** Always include readable text. Do not communicate status through color alone.

**Usage rules**
- Use for statuses such as `Aktif`, `Menunggu`, `Lunas`, `Terlambat`, or approved workflow statuses.
- Actual business status vocabulary must come from the PRD/SRS.
- Do not make badges excessively rounded or overly colorful.

### 6.5 Alert / Inline Message

**Purpose:** Explain important information, validation issues, or system feedback.

**Foundation:** shadcn/ui Alert and form message patterns.

**Variants:** `info`, `success`, `warning`, `error`.

**States:** Visible, dismissible where appropriate, persistent where required.

**Responsive behavior:** Text wraps naturally; actions move below the message if needed.

**Accessibility:** Use appropriate semantics and do not rely on color alone. Announce important dynamic errors/statuses when implemented in a live interface.

**Usage rules:** Explain what happened and what the user can do next. Avoid vague messages such as “Something went wrong” without a next step.

### 6.6 Toast / Sonner Notification

**Purpose:** Confirm short-lived actions or communicate a non-blocking result.

**Foundation:** shadcn/ui-compatible toast solution, such as Sonner if selected by the implementation team.

**Variants:** Success, error, warning, info.

**States:** Entering, visible, dismissed.

**Responsive behavior:** Keep within mobile viewport and away from persistent navigation/critical controls.

**Accessibility:** Announce meaningful feedback without unexpectedly moving focus.

**Usage rules:** Use for brief feedback; use an inline alert for persistent errors and a dialog for decisions requiring a response.

### 6.7 Tooltip

**Purpose:** Explain an icon-only action or provide concise supplementary context.

**Foundation:** shadcn/ui Tooltip.

**Variants:** Standard; placement follows available space.

**States:** Hidden, visible, keyboard-focus visible.

**Responsive behavior:** Do not depend on hover alone for mobile-critical information.

**Accessibility:** Support keyboard focus and avoid making essential instructions available only in a tooltip.

---

## 7. Navigation and Layout Components

### 7.1 Application Shell

**Purpose:** Provide the shared frame for authenticated SEKOCI portals.

**Foundation:** Composed layout using shared header, navigation, and content container.

**Parts**
- Product identity.
- Global header.
- Sidebar or mobile navigation.
- Breadcrumb/page context where applicable.
- Main content region.
- Optional contextual panel.

**States:** Normal, sidebar collapsed, mobile navigation open, loading shell, access-restricted.

**Responsive behavior**
- Desktop: persistent or collapsible sidebar and full header.
- Tablet: collapsible navigation and adaptive content grid.
- Mobile: compact header and accessible navigation drawer or bottom navigation where appropriate to the portal.

**Accessibility:** Provide a skip link, landmarks, logical focus order, and visible focus.

**Usage rules:** All portals reuse the same foundational layout language. Navigation contents vary by authorized role; visual foundations do not.

### 7.2 Sidebar Navigation

**Purpose:** Navigate between authorized modules and menus.

**Foundation:** Navigation menu/list with collapsible groups.

**Variants:** Expanded, collapsed, mobile drawer.

**States:** Default, hover, active, expanded group, disabled/unavailable if required.

**Responsive behavior:** Desktop sidebar; tablet collapsible sidebar; mobile drawer or portal-specific compact navigation.

**Accessibility:** Use semantic links/buttons, announce expanded state, and keep keyboard navigation predictable.

**Usage rules**
- Active menu must be obvious without relying only on color.
- Use a restrained outline icon family with consistent sizing.
- Do not show unauthorized menu items as if available.
- Preserve context when navigating within a module where the product architecture supports it.

### 7.3 Top Header

**Purpose:** Expose global context and utilities.

**Possible elements:** Breadcrumb/page context, unit selector, academic-year selector, global search, notifications, help, profile menu.

**Foundation:** Composed header using shared controls.

**States:** Normal, search active, notifications open, profile menu open, mobile compact.

**Responsive behavior:** Prioritize high-value controls; move secondary controls into menus on smaller screens.

**Accessibility:** Every icon-only control needs an accessible name; menus must support keyboard interaction.

**Usage rules:** Only include controls relevant to the current portal and role. Do not overload the header with module-specific actions.

### 7.4 Breadcrumb

**Purpose:** Show location within the application hierarchy.

**Foundation:** shadcn/ui Breadcrumb.

**Variants:** Standard; compact on narrow viewports.

**States:** Current item and navigable ancestor items.

**Responsive behavior:** Collapse or truncate ancestors on mobile while preserving accessible names.

**Accessibility:** Mark the current page semantically.

### 7.5 Tabs

**Purpose:** Switch between related content within the same context.

**Foundation:** shadcn/ui Tabs.

**Variants:** Underline, segmented only if specified by the design system.

**States:** Active, inactive, focus-visible, disabled, loading content if needed.

**Responsive behavior:** Allow horizontal scrolling or a suitable compact pattern when tabs do not fit; avoid wrapping into confusing multiple rows.

**Accessibility:** Follow the selected tab keyboard and ARIA behavior provided by the chosen component foundation.

**Usage rules:** Use tabs for peer-level content, not to hide unrelated workflows or replace primary navigation.

### 7.6 Dropdown Menu

**Purpose:** Display contextual actions or secondary navigation.

**Foundation:** shadcn/ui Dropdown Menu.

**Variants:** Standard, row action, profile menu.

**States:** Closed, open, focused item, disabled item.

**Responsive behavior:** Position within the viewport and avoid clipping.

**Accessibility:** Keyboard navigation and focus management must be supported.

**Usage rules:** Destructive actions should be visually distinct and confirmed where necessary.

### 7.7 Command / Global Search

**Purpose:** Search menus, records, or supported commands.

**Foundation:** shadcn/ui Command plus a search input, depending on the workflow.

**States:** Idle, typing, results, no results, loading, error.

**Responsive behavior:** Full-width or dialog/sheet on mobile.

**Accessibility:** Provide a visible label or accessible name, announce result count where appropriate, and support keyboard selection.

**Usage rules:** Search scope and accessible records must respect role and unit scope.

### 7.8 Pagination

**Purpose:** Navigate large result sets.

**Foundation:** Shared pagination component.

**Parts:** Current page, previous/next, page-size selector, total record context.

**States:** First page, middle page, last page, disabled controls, loading.

**Responsive behavior:** Simplify page-number display on mobile while retaining previous/next and current context.

**Accessibility:** Label controls and identify the current page.

**Usage rules:** Keep filters and sorting when moving between pages unless the workflow explicitly specifies otherwise.

---

## 8. Forms and Data Entry

### 8.1 Form Field Wrapper

**Purpose:** Keep labels, controls, helper text, and validation consistent.

**Foundation:** shadcn/ui Form patterns with the chosen form validation library.

**Parts:** Label, required indicator where applicable, input/control, helper text, error message.

**States:** Default, focus, filled, error, disabled, read-only, required.

**Responsive behavior:** Stack fields on mobile; use multi-column layouts only where they improve comprehension.

**Accessibility:** Associate labels, instructions, and errors with their fields. Required status must not be communicated by color alone.

**Usage rules:** Place validation feedback close to the affected field and preserve valid input after errors where possible.

### 8.2 Input

**Purpose:** Capture short text, numbers, codes, or other supported values.

**Foundation:** shadcn/ui Input.

**Variants:** Standard, search, prefix/suffix where required by the design.

**States:** Default, focus-visible, filled, error, disabled, read-only.

**Responsive behavior:** Fit the available container width; prevent horizontal overflow.

**Accessibility:** Use appropriate input type, label, and autocomplete behavior where applicable.

### 8.3 Textarea

**Purpose:** Capture multi-line text such as notes or revision reasons.

**Foundation:** shadcn/ui Textarea.

**Variants:** Standard; auto-resize only where it improves the workflow.

**States:** Default, focus, filled, error, disabled, read-only.

**Responsive behavior:** Full width on mobile.

**Accessibility:** Provide label, character limit guidance when applicable, and an associated error.

### 8.4 Select / Combobox

**Purpose:** Choose one value from a known set or search a larger set.

**Foundation:** shadcn/ui Select for short fixed lists; Combobox for searchable or large lists.

**States:** Closed, open, selected, no results, disabled, error, loading where data-driven.

**Responsive behavior:** Must fit the viewport; use a mobile-appropriate interaction without losing selected value or search capability.

**Accessibility:** Keyboard selection, accessible label, selected state, and meaningful empty-result feedback.

**Usage rules:** Do not use a Select for a long list that requires search if a Combobox is more appropriate.

### 8.5 Checkbox

**Purpose:** Select independent options or consent/confirmation where applicable.

**Foundation:** shadcn/ui Checkbox.

**States:** Unchecked, checked, indeterminate, disabled, focus-visible.

**Responsive behavior:** Touch-friendly spacing and readable label wrapping.

**Accessibility:** Label the checkbox and explain group-level selection behavior.

### 8.6 Radio Group

**Purpose:** Choose one option from a small, mutually exclusive set.

**Foundation:** shadcn/ui Radio Group.

**States:** Unselected, selected, disabled, focus-visible, error if applicable.

**Responsive behavior:** Stack options when horizontal layouts become crowded.

**Accessibility:** Provide group label and clear option labels.

### 8.7 Switch

**Purpose:** Toggle a setting that takes effect immediately or follows a clearly defined save pattern.

**Foundation:** shadcn/ui Switch.

**States:** On, off, disabled, focus-visible, pending if persistence is asynchronous.

**Responsive behavior:** Keep label and switch aligned and touch-accessible.

**Accessibility:** Include a descriptive label; do not use a switch for choosing among more than two values.

**Usage rules:** Make it clear whether the change saves immediately or requires a separate Save action.

### 8.8 Date Picker / Calendar

**Purpose:** Select a date or supported date range.

**Foundation:** shadcn/ui Calendar/Popover or a date-range pattern.

**States:** Closed, open, selected, range selected, unavailable date, invalid range, disabled.

**Responsive behavior:** Avoid calendar clipping; use a mobile-appropriate sheet or layout where needed.

**Accessibility:** Support keyboard operation, labels, and understandable date formats.

**Usage rules:** Use Indonesian locale conventions where required by the product; enforce actual date constraints from the PRD/business rules.

### 8.9 File Upload

**Purpose:** Submit documents or import files.

**Foundation:** Shared upload/dropzone pattern with native file input support.

**States:** Idle, drag-over, uploading, validation error, failed, uploaded, removed, retry.

**Responsive behavior:** Drag-and-drop is an enhancement, never the only upload method. The upload control must be usable by touch and keyboard.

**Accessibility:** Provide a labeled file picker, accepted format/size guidance, progress/status announcements, and error messages.

**Usage rules:** Accepted formats, size limits, document requirements, and retention behavior must come from approved requirements.

### 8.10 Form Action Footer

**Purpose:** Keep Save, Submit, Cancel, or Back actions clear on long forms.

**Foundation:** Shared action group; sticky footer only when it improves usability.

**States:** Normal, unsaved changes, submitting, success, validation failure.

**Responsive behavior:** Stack or wrap actions on mobile; ensure sticky actions do not obscure fields or keyboard focus.

**Accessibility:** Preserve logical focus order and visible labels.

---

## 9. Data Display Components

### 9.1 Data Table

**Purpose:** Display and manage structured operational records.

**Foundation:** shadcn/ui Table plus a reusable data-table implementation.

**Capabilities, when required by the PRD**
- Search.
- Filters and active filter chips.
- Sorting.
- Pagination and page-size selection.
- Column visibility.
- Row selection.
- Bulk actions.
- Row-level actions.
- Export.
- Loading, empty, and error states.

**States:** Loading, populated, empty, no filter results, error, selected rows, disabled actions.

**Responsive behavior**
- Desktop: compact, aligned columns with clear headers.
- Tablet: adjust density and allow controlled horizontal scrolling if needed.
- Mobile: use prioritized record cards or a mobile-specific row layout for important workflows; horizontal scrolling is acceptable where necessary but must remain usable.
- Never shrink text until data becomes unreadable.

**Accessibility:** Use proper table semantics, meaningful headers, keyboard-accessible actions, and visible focus.

**Usage rules**
- Keep column widths and alignment consistent.
- Use status badges for status fields.
- Do not expose actions unavailable to the current role or record state.
- Export must respect permissions, unit scope, and active filters.

### 9.2 Table Toolbar

**Purpose:** Group search, filters, column controls, export, and primary table actions.

**Foundation:** Composed toolbar using shared inputs, buttons, and dropdowns.

**States:** Default, filters active, bulk selection active, loading.

**Responsive behavior:** Wrap or collapse secondary controls on mobile; keep the primary task discoverable.

**Accessibility:** Every control needs a label; do not rely on placeholder text alone.

### 9.3 Filter Chip

**Purpose:** Show an active filter and optionally allow it to be removed.

**Foundation:** Badge/button combination.

**States:** Active, removable, disabled if relevant.

**Responsive behavior:** Wrap to additional lines or move into a horizontally scrollable region with accessible controls.

**Accessibility:** Provide a clear remove-filter label.

### 9.4 Empty State

**Purpose:** Explain why a region has no content and what the user can do next.

**Foundation:** Shared empty-state component.

**Parts:** Short title, explanation, optional restrained icon, relevant next action.

**Variants:** No records, no search results, no permission/content, not configured.

**Responsive behavior:** Stack content and action on mobile.

**Accessibility:** Use meaningful text; decorative icons should not be the only explanation.

**Usage rules:** Explain what is empty, why it may be empty, and the next useful action. Do not show an irrelevant illustration.

### 9.5 Skeleton / Loading State

**Purpose:** Indicate content is being loaded without causing abrupt layout shifts.

**Foundation:** shadcn/ui Skeleton and shared loading patterns.

**Variants:** Table rows, KPI cards, chart panel, form fields, detail sections.

**States:** Loading only.

**Responsive behavior:** Match the expected content layout at each breakpoint.

**Accessibility:** Avoid announcing every decorative skeleton; expose loading state through appropriate page/region semantics.

**Usage rules:** Use skeletons for content regions when appropriate and avoid indefinite loading without feedback.

### 9.6 Error State

**Purpose:** Explain a failed load or action and provide a recovery path.

**Foundation:** Alert plus retry/action controls.

**Variants:** Inline field error, section error, page error, action error.

**States:** Error, retrying, recovered.

**Responsive behavior:** Keep message and recovery action visible on small screens.

**Accessibility:** Identify the problem and action; associate form errors with affected fields.

### 9.7 Confirmation Dialog

**Purpose:** Confirm a destructive, irreversible, or high-impact action.

**Foundation:** shadcn/ui Alert Dialog.

**Variants:** Destructive, standard confirmation.

**States:** Open, submitting, failed, confirmed/closed.

**Responsive behavior:** Fit within viewport; use a full-screen or sheet-like pattern only if appropriate to the implementation.

**Accessibility:** Move focus into the dialog, trap focus while open, label the dialog, and restore focus after closing.

**Content requirements:** State the action, affected object, consequence, and clear confirm/cancel labels.

### 9.8 Dialog

**Purpose:** Handle a short focused interaction that does not need a full page.

**Foundation:** shadcn/ui Dialog.

**Variants:** Short form, information, confirmation where suitable.

**States:** Open, closed, submitting, error.

**Responsive behavior:** Keep content within viewport; consider a sheet or full-page pattern on mobile for complex content.

**Accessibility:** Correct dialog semantics and focus management.

**Usage rules:** Do not put long or multi-step workflows in a small dialog.

### 9.9 Sheet / Drawer

**Purpose:** Show contextual details or quick editing while preserving list or page context.

**Foundation:** shadcn/ui Sheet or Drawer.

**Variants:** Right-side detail, mobile bottom sheet, contextual editor.

**States:** Open, closed, loading, error, unsaved changes.

**Responsive behavior:** Side drawer on desktop when appropriate; mobile sheet or full-page navigation based on task complexity.

**Accessibility:** Focus management, accessible title/description, keyboard dismissal where safe, and unsaved-change handling where required.

**Usage rules:** Use full pages for complex workflows, large details, or multi-step operations.

### 9.10 Avatar / User Menu

**Purpose:** Identify the signed-in user and expose account actions.

**Foundation:** shadcn/ui Avatar and Dropdown Menu.

**Variants:** Image, initials fallback, neutral placeholder.

**States:** Loaded image, fallback, menu open.

**Responsive behavior:** Compact in mobile header.

**Accessibility:** Provide meaningful alternative text when the image conveys identity; decorative avatars may use empty alternative text if the name is adjacent.

**Usage rules:** Do not depend on avatar color to identify a user or role.

### 9.11 Progress / Stepper

**Purpose:** Show progress through a multi-step workflow, such as application or document submission.

**Foundation:** Shared stepper pattern.

**Variants:** Horizontal, vertical, compact mobile.

**States:** Not started, current, completed, blocked, error when applicable.

**Responsive behavior:** Convert to a vertical or compact sequence on mobile if horizontal labels would be crowded.

**Accessibility:** Communicate step name and state in text, not color alone.

**Usage rules:** Step order and state transitions must follow the approved business workflow.

---

## 10. Dashboard and Analytics Components

### 10.1 KPI Card

**Purpose:** Summarize a meaningful operational metric.

**Parts:** Label, value, optional trend/comparison, optional contextual detail, optional icon.

**Variants:** Standard, compact, attention-required only where justified.

**States:** Loading, populated, empty/unavailable, error.

**Responsive behavior:** Multi-column grid on desktop; fewer columns or stacked cards on tablet/mobile.

**Accessibility:** Label and value must be readable by assistive technology; trend must include textual meaning where relevant.

**Usage rules**
- A KPI must answer a meaningful business question.
- Do not invent metrics or comparisons.
- Avoid using a unique bright icon color for each card.
- Negative/positive trend semantics must be defined by the metric, not assumed from red/green alone.

### 10.2 Chart Container

**Purpose:** Provide a consistent frame for Apache ECharts visualizations.

**Foundation:** Shared chart panel wrapping Apache ECharts.

**Parts:** Title, optional description, optional filters/period selector, chart, optional legend, supporting context.

**States:** Loading, populated, empty, error, no filtered data.

**Responsive behavior**
- Resize the chart with its container.
- Preserve readable axis labels and legend.
- Reposition or simplify legends when space is limited.
- Avoid clipping tooltips and controls on mobile.
- Do not depend on hover-only interactions for essential values.

**Accessibility:** Provide a clear title and a text summary or equivalent data access where appropriate. Ensure color contrast and distinguishable series.

**Usage rules**
- Apache ECharts is the preferred charting library.
- Select chart type according to the question being answered.
- Use ECharts examples as reference, not as a reason to add unnecessary visual complexity.
- Provide explicit units, date ranges, legends, and filter context where relevant.
- Do not use 3D charts or decorative animation unless a validated requirement justifies it.

### 10.3 Supported Chart Patterns

| Pattern | Suitable purpose | Notes |
|---|---|---|
| Line | Trends over time | Label time range and series |
| Bar / column | Category comparison | Use consistent category ordering |
| Stacked bar | Composition across categories | Keep segments distinguishable |
| Donut / pie | Part-to-whole for a small number of categories | Prefer direct labels/legend; avoid too many slices |
| Area | Trends where magnitude or accumulation matters | Avoid obscuring overlapping series |
| Progress | Completion against a defined target | Show numeric context |
| Comparison | Current versus previous period or target | Explain the comparison basis |

Only show chart types supported by the actual data and user need.

### 10.4 Chart Filter / Period Selector

**Purpose:** Change the period or dimension shown in a chart.

**Foundation:** Select/Combobox or date range controls.

**States:** Default, selected, loading, invalid range, disabled.

**Responsive behavior:** Stack controls or move secondary filters into a compact toolbar on mobile.

**Accessibility:** Use visible labels and preserve selected values.

**Usage rules:** Filter scope must be clear and consistent with the displayed data.

### 10.5 Activity Feed

**Purpose:** Summarize recent meaningful events.

**Parts:** Event label, affected record/context, timestamp, optional status, optional link to details.

**States:** Loading, populated, empty, error.

**Responsive behavior:** Stack event content and wrap long descriptions.

**Accessibility:** Provide text for event type and status; do not use a colored dot alone.

**Usage rules:** Show only activity the user is authorized to see.

### 10.6 Notification Center

**Purpose:** Present notifications and their read/unread state.

**Parts:** Message, type, timestamp, read state, destination/action where relevant.

**States:** Unread, read, loading, empty, error.

**Responsive behavior:** Drawer or full page on mobile when appropriate.

**Accessibility:** Provide meaningful labels and announce important new notifications appropriately.

**Usage rules:** Notification channels, preferences, retention, and delivery behavior must follow the PRD/SRS.

### 10.7 Progress Bar

**Purpose:** Show progress toward a known total or target.

**Foundation:** Shared progress component.

**States:** Normal, complete, indeterminate only for ongoing work where suitable, error if the process failed.

**Responsive behavior:** Fill available width while retaining readable numeric context.

**Accessibility:** Provide accessible name and value; include text or numeric context when useful.

---

## 11. Domain-Specific Component Patterns

These are reusable UI patterns, not a declaration that every feature is in scope. Use them only where the approved PRD/SRS requires them.

### 11.1 Applicant Status Summary

**Purpose:** Summarize an applicant's current stage and outstanding next action.

**Parts:** Current status, progress stepper, outstanding requirements, primary next action where authorized.

**States:** Must map to approved PPDB workflow states.

**Responsive behavior:** Stack status and next action on mobile.

**Usage rules:** Do not invent workflow transitions or show actions outside the user's permissions.

### 11.2 Document Verification Item

**Purpose:** Present one required document and its verification result.

**Parts:** Document name, submission state, verification state, reviewer feedback, permitted action.

**States:** Not submitted, submitted, under review, verified, revision required, rejected, or other PRD-defined states.

**Responsive behavior:** Use a stacked document row/card on mobile; ensure preview/download controls remain usable.

**Accessibility:** Status and action labels must be explicit; document previews need appropriate titles and keyboard-accessible controls.

**Usage rules:** Document requirements, state transitions, and reviewer authority come from the PRD/SRS.

### 11.3 Payment Summary

**Purpose:** Show invoice amount and payment status.

**Parts:** Invoice/reference, amount, status, due date, payment method or receipt where authorized and available.

**States:** Use the payment states approved in the PRD/SRS.

**Responsive behavior:** Stack amounts and status on mobile.

**Accessibility:** Amount and status must be readable as text.

**Security:** Never display payment credentials or sensitive secrets in the UI.

### 11.4 Academic Record Summary

**Purpose:** Summarize grades, attendance, schedules, or academic progress for an authorized user.

**Parts:** Relevant period, class/subject context, metric/value, optional trend or detail link.

**States:** Loading, available, empty/not yet published, error, access denied.

**Responsive behavior:** Prioritize the most relevant information and use mobile-friendly lists/cards.

**Usage rules:** Visibility and publication rules must follow the approved requirements and role permissions.

### 11.5 Role / Permission Indicator

**Purpose:** Clarify role or access context when needed.

**Foundation:** Badge or profile metadata.

**States:** Current role, selected role if role switching is supported, restricted access.

**Responsive behavior:** Compact in header/profile menu.

**Accessibility:** Use explicit role text.

**Usage rules:** A UI indicator does not replace server-side authorization.

---

## 12. Responsive Behavior Matrix

The breakpoints must use the authoritative values in `DESIGN.md` and the implementation theme. The table below defines expected behavior, not fixed breakpoint numbers.

| Component | Desktop | Tablet | Mobile |
|---|---|---|---|
| Application shell | Full or collapsible sidebar | Collapsible sidebar | Drawer or compact portal navigation |
| Header | Full utility controls | Prioritized controls | Compact header and overflow menu |
| KPI cards | Multi-column grid | Reduced columns | Stacked or two-column when readable |
| Data table | Full columns | Reduced density / controlled scroll | Priority cards or mobile row pattern |
| Forms | Multi-column where appropriate | Adaptive columns | Single column |
| Dialog | Centered dialog | Fit available viewport | Compact dialog, sheet, or full page by complexity |
| Drawer | Side drawer | Side drawer sized to viewport | Sheet or full page by task complexity |
| Tabs | Full tab labels | Scroll/compact if necessary | Horizontal scroll or suitable compact pattern |
| Charts | Full analytics | Adaptive layout | Resize, simplify labels, retain accessible values |
| Action groups | Inline | Wrap as needed | Stack or wrap; preserve primary action |
| Filters | Toolbar row | Wrap | Compact filter panel or sheet |
| Stepper | Horizontal | Compact horizontal | Vertical or compact sequence |

## 13. Component State Matrix

Interactive components must specify the states relevant to their behavior.

| State | Required consideration |
|---|---|
| Default | Baseline appearance and behavior |
| Hover | Pointer feedback; never the only feedback |
| Focus-visible | Keyboard focus is clearly visible |
| Active / selected | Current selection is clear |
| Disabled | Unavailable action is visually and semantically disabled |
| Loading | Progress is communicated; duplicate actions are prevented when necessary |
| Empty | Explain absence and next step |
| Error | Explain problem and recovery action |
| Success | Confirm meaningful completion |
| Read-only | Value remains readable; editing is unavailable |
| Permission denied | Do not expose unauthorized data or actions |
| Responsive | Layout and controls remain usable at supported viewport sizes |

Not every state applies to every component. Specify applicable states instead of adding meaningless variants.

## 14. Accessibility Checklist

For each reusable interactive component, verify:

- Semantic HTML and accessible role.
- Accessible name and description where needed.
- Keyboard operation and visible focus.
- Appropriate focus management for dialogs, sheets, and menus.
- Label, helper text, and validation association for form controls.
- Status and errors are not communicated through color alone.
- Sufficient contrast using the approved design tokens.
- Touch usability on mobile.
- Responsive zoom/reflow without losing essential content.
- Dynamic loading and feedback are announced appropriately.
- Reduced-motion preferences are respected when animation is present.

The product-wide accessibility target is defined in `DESIGN.md`; validate components against that target during implementation and testing.

## 15. Component API and Implementation Guidance

The exact API depends on the chosen front-end stack. Keep reusable component APIs predictable and typed where the stack supports typing.

Recommended conventions:

- Use explicit variants and sizes rather than arbitrary per-screen styles.
- Prefer semantic props such as `variant`, `size`, `disabled`, `loading`, and `aria-label`.
- Keep domain behavior out of generic UI primitives.
- Compose domain components from shared primitives.
- Keep validation and business rules in the appropriate application/domain layer.
- Avoid embedding role authorization only in the visual component; enforce access on the server.
- Keep design tokens centralized and avoid hard-coded one-off colors.
- Reuse the same component implementation across portals unless a documented use case requires a distinct composition.
- Keep chart configuration and data formatting reusable, but do not force unrelated charts into an identical visualization.
- Do not assume the implementation framework, routing library, form library, or table library until the technical architecture has approved it.

## 16. Naming and Documentation Convention

Use consistent component names in code and documentation. Suggested conceptual groups:

```text
components/
├── ui/                 # shared primitives based on shadcn/ui
├── layout/             # application shell, header, sidebar
├── navigation/         # breadcrumb, tabs, pagination, menus
├── forms/              # field wrapper, validation, upload
├── tables/             # data table, toolbar, filters
├── dashboards/         # KPI cards, chart container, activity feed
├── feedback/           # alert, toast, empty, loading, error
├── overlays/            # dialog, alert dialog, sheet, drawer
├── status/             # badges, progress, stepper
└── domain/              # PPDB and other requirement-backed patterns
```

This is a conceptual structure. Adapt it to the approved repository and technical architecture without changing the component standards.

For each newly added component, document:

1. Component name and purpose.
2. Requirement/use case it supports.
3. Foundation or primitive used.
4. Variants and sizes.
5. Applicable states.
6. Responsive behavior.
7. Accessibility requirements.
8. Example usage.
9. Related components.
10. Owner/review status if the team uses component governance.

## 17. Governance: Adding or Changing Components

Before adding a new shared component:

1. Check whether an existing component can support the need through composition or a documented variant.
2. Confirm the user need or requirement.
3. Define variants, states, responsive behavior, and accessibility.
4. Confirm it follows `DESIGN.md`.
5. Review whether it introduces a new token or visual pattern.
6. Update this catalog and the approved prototype where relevant.
7. Validate it across the portals that use it.

Avoid changing a shared component in a way that unexpectedly breaks other portals. Document breaking changes and review affected screens.

## 18. Definition of Done

A component is ready for reuse when:

- It follows `DESIGN.md` tokens and patterns.
- Its purpose and applicable requirements are clear.
- Variants and applicable states are documented.
- Desktop, tablet, and mobile behavior is defined.
- Keyboard, focus, labels, and accessibility are validated.
- Loading, empty, error, disabled, and permission-related behavior is handled where applicable.
- It does not expose unauthorized actions or sensitive data.
- It has been reviewed in at least one realistic screen composition.
- Documentation and examples are updated.

## 19. Related Artifacts

- `DESIGN.md` — product-wide design and interaction source of truth.
- `COMPONENT_CATALOG.md` — reusable component reference.
- `SCREEN_CATALOG.md` — optional screen inventory and requirement traceability.
- PRD / SRS — business and functional source of truth.
- Approved prototype — screen-level visual and interaction reference.

**Final principle:** one SEKOCI design system, one consistent component language, and role-appropriate experiences across every authorized portal.
