# Teratai Putih School Management System
## Product Design System & UX Specification

**Document ID:** TP-SMS-DESIGN-001  
**Version:** 1.1  
**Status:** Approved Design Direction / Design Source of Truth  
**Product:** Teratai Putih School Management System (TP-SMS)  
**Client:** Teratai Putih Global, Bekasi  
**Product Owner:** David Pambudi  
**Project Manager:** David Pambudi  

---

## 1. Purpose

This document defines the UI/UX design system, interaction principles, visual language, information architecture patterns, reusable components, and screen design standards for TP-SMS.

It is complementary to:

- `PRD_Terpadu_TP-SMS_v2.1.md`
- Future SRS documentation
- UX flows
- Prototype designs
- Development component specifications

## 2. Source of Truth Hierarchy

### Level 1 — Business Requirements

`PRD_Terpadu_TP-SMS_v2.1.md`

Defines business requirements, scope, functional requirements, business rules, acceptance criteria, roles, menus, phases, integrations, and non-functional requirements.

The PRD is the source of truth for **WHAT** the system must do.

### Level 2 — Design Specification

`DESIGN.md`

Defines visual language, UX principles, navigation, layouts, components, interaction patterns, responsive behavior, states, accessibility, and design consistency.

DESIGN.md is the source of truth for **HOW** the product should look and behave.

### Level 3 — Prototype

Stitch / Figma / UI prototype defines screen composition, visual implementation, interaction visualization, and responsive representation.

### Level 4 — Implementation

Implementation must follow:

`PRD → DESIGN.md → Approved Prototype → SRS / Technical Specification`

## 3. Product Context

TP-SMS is an enterprise School Management System for Teratai Putih Global.

The platform covers:

- PPDB
- School Finance
- Payment
- Accounting
- Operational Management
- Parent Portal
- Executive Dashboard
- Student Portal
- Teacher Portal

The PRD defines **10 menu groups, 77 menus, and 8 delivery phases**.

## 4. Product Design Vision

Create a modern enterprise school platform that feels:

- trustworthy
- structured
- professional
- efficient
- transparent
- easy to operate
- scalable across multiple school units

The product should feel closer to an enterprise SaaS platform than a generic education website.

## 5. Design Principles

### 5.1 Clarity

Users must immediately understand where they are, what they are viewing, what needs attention, what action they can perform, and the current status.

### 5.2 Consistency

Identical interaction types must use identical UI patterns across the platform.

### 5.3 Operational Efficiency

Prioritize search, filtering, sorting, bulk actions, keyboard-friendly workflows, readable tables, clear forms, and contextual actions.

### 5.4 Progressive Disclosure

Use tabs, drawers, sheets, dialogs, expandable sections, and detail pages to expose secondary information when needed.

### 5.5 Data Safety

Sensitive student, parent, applicant, document, financial, accounting, and access information must respect role-based access and unit scope.

### 5.6 Traceability

Where relevant, show actor, timestamp, action, status, reference number, and history.

## 6. Visual Design Language

### 6.1 Overall Style

**Light Enterprise SaaS**

Characteristics:

- clean white surfaces
- soft gray application background
- enterprise blue
- subtle borders
- minimal shadows
- strong information hierarchy
- professional typography
- compact but comfortable spacing

### 6.2 Explicitly Avoid

Do NOT use:

- dark mode
- neon colors
- glassmorphism
- excessive gradients
- gaming UI
- excessive animation
- playful consumer-app styling
- SCADA-style dashboards
- excessive rounded cards
- heavy shadows
- overly decorative illustrations

## 7. Design Tokens

### 7.1 Color

Primary:

```text
Primary:       #2563EB
Primary Dark:  #1E40AF
Primary Light: #EFF6FF
```

Neutral:

```text
Background: #F8FAFC
Surface:    #FFFFFF
Border:     #E2E8F0
Text:       #0F172A
Secondary:  #64748B
Muted:      #94A3B8
```

Semantic:

```text
Success: Green
Warning: Amber
Error: Red
Info: Blue
```

Do not introduce arbitrary colors per screen.

## 8. Typography

Use a modern enterprise sans-serif typeface.

Hierarchy:

```text
Display
Page Title
Section Title
Card Title
Body
Body Small
Caption
Label
```

Prioritize readability, hierarchy, compact enterprise layouts, and clear table scanning.

## 9. Spacing System

Use:

```text
4
8
12
16
20
24
32
40
48
```

Guidelines:

- 4–8 px: micro spacing
- 12–16 px: component spacing
- 20–24 px: section spacing
- 32–48 px: major layout spacing

## 10. Border Radius

```text
Controls:   6px
Cards:      8px
Dialogs:    10px
Containers: 12px
```

Avoid highly rounded consumer-app interfaces.

## 11. Shadows

Use shadows sparingly, primarily for dialogs, drawers, popovers, and floating elements.

Cards should rely mainly on white surface, border, and spacing.

## 12. Component Foundation

Use `shadcn/ui` as the primary UI component foundation.

Core components:

- Button
- Input
- Textarea
- Select
- Combobox
- Checkbox
- Radio Group
- Switch
- Date Picker
- Calendar
- Table
- Data Table
- Card
- Badge
- Alert
- Tabs
- Dialog
- Alert Dialog
- Sheet
- Drawer
- Dropdown Menu
- Popover
- Tooltip
- Breadcrumb
- Sidebar
- Command
- Pagination
- Skeleton
- Toast

All components must follow TP-SMS design tokens.

## 13. Application Shell

Primary Admin / Back Office shell:

```text
┌───────────────────────────────────────────────┐
│ Top Header                                    │
├───────────────┬───────────────────────────────┤
│ Sidebar       │ Main Content                  │
│ Navigation    │                               │
└───────────────┴───────────────────────────────┘
```

## 14. Sidebar

The sidebar contains:

- Teratai Putih branding
- Product name
- Organization / Unit context
- Navigation groups
- Active menu state
- Collapsible behavior

Example:

```text
Dashboard

Platform Foundation
  Organization & Unit
  Academic Year
  Master Data
  Data Import & Migration
  User & Access
  Audit Trail
  Notification

PPDB
  PPDB Dashboard
  PPDB Configuration
  Applicant List
  Applicant Detail
  Document Verification
  Selection
  Announcement
  Re-registration
```

Actual navigation must follow the PRD.

## 15. Top Header

May contain:

- Breadcrumb
- Page context
- Unit selector
- Academic Year selector
- Global Search
- Notifications
- Help
- User Profile

Controls are contextual.

## 16. Multi-Unit Context

TP-SMS is a multi-unit system.

The design should communicate:

```text
Organization
    ↓
Unit
    ↓
Academic Year
    ↓
Data
```

Example:

```text
Teratai Putih Global
→ SD
→ 2026/2027
```

Users must understand the scope of the data they are viewing.

## 17. Page Layout Pattern

```text
Breadcrumb
↓
Page Header
↓
Context / Global Filters
↓
Primary Content
↓
Secondary Content
↓
Operational Information
```

## 18. Page Header

Should contain:

- page title
- short description
- breadcrumb
- primary action
- secondary actions where applicable

## 19. Dashboard Pattern

```text
Page Header
↓
Global Filters
↓
KPI Cards
↓
Main Analytics
↓
Secondary Analytics
↓
Operational Tables / Alerts
↓
Recent Activity
```

Every visualization should answer a business question.

## 20. Data Table Pattern

```text
Page Header
↓
Primary Action
↓
Search + Filters
↓
Filter Chips
↓
Data Table
↓
Pagination
```

Tables may support:

- search
- filtering
- sorting
- pagination
- column visibility
- row selection
- bulk actions
- row actions

## 21. Table Density

Use compact but readable tables.

Prioritize scanning, alignment, consistent column widths, meaningful status indicators, and concise actions.

## 22. Row Actions

Common actions:

```text
View
Edit
Verify
Approve
Reject
Archive
Delete
```

Only show actions applicable to the user's role and record state.

## 23. Detail Page Pattern

```text
Breadcrumb
↓
Entity Header
↓
Status + Primary Actions
↓
Summary
↓
Tabs
↓
Detailed Sections
↓
Activity / Audit
```

## 24. Form Pattern

```text
Breadcrumb
↓
Page Header
↓
Form Sections
↓
Review / Summary
↓
Sticky Action Footer
```

Use clear grouping such as:

- Personal Information
- Parent / Guardian
- Academic Information
- Documents
- Financial Information

## 25. Form States

Every field must support:

- Default
- Focus
- Filled
- Error
- Disabled
- Read-only
- Required

Validation must be close to the affected field.

## 26. Approval Pattern

```text
Request
↓
Review
↓
Approve / Reject / Request Revision
↓
Final Status
```

States:

- Draft
- Submitted
- Under Review
- Approved
- Rejected
- Revision Required
- Cancelled

Significant approval actions require confirmation.

## 27. Notification Pattern

Support:

- notification type
- recipient
- channel
- status
- timestamp
- read/unread state
- history

Potential channels include:

- Email
- WhatsApp Business API

Notification preferences must respect user settings except mandatory notifications.

## 28. Audit Trail Pattern

Audit records should show:

- timestamp
- actor
- role
- module
- action
- record/reference
- result

Audit records are read-only.

## 29. Payment Status Pattern

Primary states:

```text
Pending
Paid
Failed
Expired
```

Additional context may include:

- invoice/reference
- amount
- payment method
- expiry
- transaction time
- receipt

Payment credentials must never be displayed or stored by the UI.

## 30. Status System

### Success

- Active
- Paid
- Verified
- Approved
- Completed

### Warning

- Pending
- Waiting
- Needs Attention
- Partial

### Error

- Failed
- Rejected
- Overdue
- Invalid

### Information

- Draft
- Scheduled
- In Progress

### Neutral

- Inactive
- Archived
- Not Started

Never rely only on color. Use text, badge, icon, and supporting context where appropriate.

## 31. PPDB UX

Core Phase 1 journey:

```text
Configuration
↓
Registration
↓
Application
↓
Document Submission
↓
Document Verification
↓
Selection
↓
Announcement
↓
Re-registration
↓
Payment
↓
Payment Confirmation
↓
Receipt
```

The UI must communicate the applicant's current stage.

## 32. Parent PPDB Experience

Parent-facing screens must be simpler than Admin screens.

Prioritize:

- progress
- required actions
- pending tasks
- payment
- documents
- announcement
- re-registration

The parent must understand:

```text
What have I completed?
What is pending?
What do I need to do?
What happens next?
```

## 33. Payment UX

Minimum PPDB payment capability:

- registration/form payment
- re-registration payment
- Virtual Account
- QRIS
- e-wallet
- automatic payment status
- receipt

Flow:

```text
Invoice
↓
Select Payment Method
↓
Payment Instruction
↓
Payment Pending
↓
Gateway Notification
↓
Payment Success
↓
Receipt
```

Do not make manual confirmation the primary payment flow.

## 34. Import & Migration UX

```text
Select Data
↓
Upload
↓
Validate
↓
Review Errors
↓
Confirm Import
↓
Process
↓
Result
```

States:

- Ready
- Validating
- Validation Failed
- Ready to Import
- Importing
- Completed
- Completed with Errors
- Failed

## 35. Empty States

Every major page must define an empty state.

Example:

```text
No applicants found

There are no applicants matching the selected filters.

[Clear Filters]
```

Explain:

1. What is empty
2. Why it may be empty
3. What the user can do next

## 36. Loading States

Use:

- Skeleton
- Loading indicators
- Disabled action states

Use skeletons for tables, cards, dashboard widgets, and detail sections where appropriate.

## 37. Error States

Errors must be actionable.

Example:

```text
Unable to load applicant data.

Please try again. If the problem continues, contact the administrator.

[Try Again]
```

For form errors:

- identify the affected field
- explain the problem
- preserve valid input where possible

## 38. Confirmation & Destructive Actions

High-impact actions require confirmation.

Examples:

- Delete
- Reject
- Deactivate
- Publish
- Finalize
- Cancel transaction
- Confirm import

Dialogs must clearly state the action, consequence, affected object, and confirmation action.

## 39. Responsive Design

### Desktop

- full sidebar
- multi-column forms
- dense tables
- dashboard grid

### Tablet

- collapsible sidebar
- adaptive grid
- reduced density

### Mobile

- stacked content
- simplified navigation
- horizontally scrollable tables where necessary
- large touch targets

Parent Portal receives stronger mobile optimization.

## 40. Accessibility

Requirements:

- sufficient contrast
- visible focus state
- keyboard navigation
- semantic labels
- readable typography
- status not communicated by color alone
- accessible dialogs
- accessible form validation
- accessible tables

## 41. Role-Based UX

Roles include:

- Super Admin
- Yayasan Admin
- Pengurus Yayasan
- School Admin / Unit Admin
- Kepala Sekolah
- PPDB Admin
- Selection Committee
- Finance Admin
- Finance Manager
- Accounting
- Parent
- Student
- Teacher

Users must only see authorized menus, data, and actions.

UI restrictions do not replace server-side authorization.

## 42. Scope Model

Every feature must be classified as:

```text
Module for Terput
Add-on
Campuran
```

### Module for Terput

Core Teratai Putih scope.

### Add-on

Optional functionality.

### Campuran

Contains both core and add-on requirements.

The UI must not accidentally present Add-on functionality as mandatory core scope.

## 43. Phase Model

TP-SMS uses:

```text
Phase 1
Phase 2
Phase 3
Phase 4
Phase 5
Phase 6
Phase 7
Phase 8
```

The product contains 10 menu groups, but delivery is organized into 8 phases.

**Menu group is not the same as delivery phase.**

Do not revert to the old 10-phase structure.

## 44. Phase 1 Design Scope

### Group Menu 1 — Platform Foundation

01 Organization & Unit Management  
02 Academic Year Management  
03 Master Data  
04 Data Import & Migration  
05 User & Access Management  
06 Audit Trail  
07 Notification Management

### Group Menu 2 — PPDB

08 PPDB Dashboard  
09 PPDB Configuration  
10 Applicant List  
11 Applicant Detail  
12 Document Verification  
13 Selection  
14 Announcement  
15 Re-registration  
16 Parent PPDB Portal

Additionally, minimum PPDB payment capability is included in Phase 1.

## 45. Screen Traceability

Every designed screen must be traceable through:

```text
Phase
↓
Menu ID
↓
Menu Name
↓
Scope
↓
Portal
↓
FR ID
↓
Business Rule
↓
Acceptance Criteria
```

Example:

```text
Phase: Phase 1
Menu: 10
Name: Applicant List
Scope: Module for Terput
Portal: Admin / PPDB

FR:
FR-10-001
FR-10-002

AC:
AC-10-01
AC-10-02
```

Do not invent FR or AC IDs. Use the PRD as the source.

## 46. Design-to-Requirement Rule

A screen should exist because a requirement needs it.

Do not create screens merely because competitors have them, they look impressive, they are common in SaaS, or they are visually interesting.

Every major screen must have a traceable purpose.

## 47. UX Interaction Rules

### Primary Action

Use solid primary buttons:

```text
[Save]
[Submit]
[Approve]
[Publish]
```

### Secondary Action

Use outline or secondary buttons:

```text
[Cancel]
[Back]
[Export]
```

### Destructive

Use destructive treatment:

```text
[Delete]
[Reject]
```

## 48. Drawer vs Dialog

Use Drawer / Sheet for:

- contextual information
- quick editing
- secondary detail
- preserving list context

Use Dialog for:

- confirmation
- destructive action
- short focused form
- critical decision

Use full page for:

- complex forms
- complex workflows
- large detail views
- multi-step operations

## 49. Search

Search should be used where users manage significant datasets.

It must be clearly labeled, provide meaningful placeholder text, support relevant fields, and preserve filters where appropriate.

Example:

```text
Search applicant name, registration number...
```

## 50. Filters

Common operational filters:

- Unit
- Academic Year
- Status
- Date
- Program
- Admission Path
- Payment Status
- Verification Status

Use Filter Chips to show active filters.

## 51. Pagination

For large datasets:

- show page size
- show current page
- show total records
- provide previous/next controls

Avoid unnecessarily large single-page datasets.

## 52. Export

Export must respect:

- current scope
- current filters
- authorized records

Export actions must be auditable where required by the PRD.

## 53. Data Visualization

Preferred:

- line chart
- bar chart
- donut chart
- progress
- comparison

Avoid:

- decorative charts
- excessive chart density
- 3D charts
- unnecessary animation

## 54. Dashboard KPI Rules

A KPI should contain:

```text
Label
Value
Trend / Comparison
Supporting Context
```

Example:

```text
Total Applicants
1,248

+12.4% vs previous period
```

KPI must be actionable or informative.

## 55. Realistic Content

Use realistic Indonesian school data.

Example names:

- Aisyah Putri
- Muhammad Rizky
- Nabila Rahma
- Fajar Maulana

Units:

- Teratai Putih — SD
- Teratai Putih — SMP
- Teratai Putih — SMK

Academic year:

- 2026/2027

Currency:

- Indonesian Rupiah (Rp)

Never use lorem ipsum in production-like prototypes.

## 56. Prototype Fidelity

Stitch prototypes must be:

- realistic
- production-oriented
- enterprise-grade
- visually consistent
- sufficiently detailed for stakeholder review

Do not create conceptual wireframes unless explicitly requested.

## 57. Prototype Consistency Rule

When generating a new screen, always reuse:

- same sidebar
- same header
- same typography
- same colors
- same spacing
- same table pattern
- same form pattern
- same status system
- same buttons
- same dialogs
- same responsive behavior

Never create a new visual system for a new menu.

## 58. Stitch Design Rule

When generating screens in Stitch:

Use the existing TP-SMS Design Foundation.

Do not:

- change the color palette
- change typography
- redesign the sidebar
- redesign the header
- introduce a new card style
- introduce a new table style
- introduce a new button language
- introduce dark mode
- introduce unrelated visual patterns

The output must look like it was designed by the same product design team.

## 59. Development Alignment

Recommended reusable component structure:

```text
components/
├── ui/
├── layout/
├── navigation/
├── forms/
├── tables/
├── dashboards/
├── status/
├── dialogs/
├── drawers/
└── domain/
```

Domain components should reuse shared UI components.

## 60. Design Quality Checklist

### Product

- [ ] Correct menu
- [ ] Correct phase
- [ ] Correct scope
- [ ] Correct portal
- [ ] Correct role
- [ ] Requirement traceable

### UX

- [ ] Clear page purpose
- [ ] Clear primary action
- [ ] Clear navigation
- [ ] Clear status
- [ ] Clear validation
- [ ] Clear empty state
- [ ] Clear loading state
- [ ] Clear error state

### UI

- [ ] Correct typography
- [ ] Correct spacing
- [ ] Correct colors
- [ ] Correct components
- [ ] Correct table pattern
- [ ] Correct form pattern
- [ ] Correct responsive behavior

### Security

- [ ] Role-aware
- [ ] Unit-scope aware
- [ ] Sensitive data protected
- [ ] Unauthorized actions hidden/blocked

### Accessibility

- [ ] Keyboard accessible
- [ ] Focus visible
- [ ] Labels meaningful
- [ ] Status not color-only
- [ ] Contrast acceptable

## 61. Definition of Design Done

A screen is Design Done when:

1. It is traceable to the PRD.
2. It uses the approved design system.
3. It uses existing components where applicable.
4. User flow is understandable.
5. Primary action is clear.
6. Error state is defined.
7. Empty state is defined.
8. Loading state is defined.
9. Authorization considerations are defined.
10. Responsive behavior is considered.
11. It does not introduce an inconsistent visual pattern.
12. It is ready for stakeholder review and implementation handoff.

## 62. Final Design Principle

TP-SMS must feel like:

**ONE PLATFORM  
ONE DESIGN SYSTEM  
ONE DATA FOUNDATION  
ONE USER EXPERIENCE**

The product must not feel like separate applications for:

- PPDB
- Finance
- Payment
- Accounting
- Operations
- Parent
- Student
- Teacher

All modules are part of one integrated enterprise platform.

The PRD defines **WHAT** TP-SMS must do.

DESIGN.md defines **HOW** TP-SMS should look and behave.

The prototype must demonstrate both consistently.

---

## 68. Enterprise Iconography Standard

SEKOCI uses a restrained, enterprise-grade icon system. Icons must support recognition and navigation without making the interface feel playful, decorative, or visually noisy.

### 68.1 Visual direction

- Use a consistent outline icon style as the default.
- Prefer a single neutral or primary-blue icon color for navigation and general actions.
- Use semantic colors only when the icon communicates a meaningful state, such as success, warning, error, or payment status.
- Do not assign a different bright color to every module or menu item.
- Avoid mixed icon styles, emoji, 3D icons, filled cartoon icons, gradients, and decorative illustrations in operational interfaces.
- Use consistent stroke weight, optical size, alignment, and bounding box across the product.
- Recommended default: Lucide icons, or an equivalent consistent outline icon library, subject to implementation-stack approval.
- Icon choice must be semantically recognizable and consistent across all portals.

### 68.2 Icon color rules

| Context | Default treatment |
|---|---|
| Sidebar navigation | Neutral slate; SEKOCI blue for active state |
| Primary actions | Inherit the button's foreground color |
| Secondary actions | Neutral slate |
| Dashboard KPI icon | Neutral/blue by default; semantic color only when it adds meaning |
| Success / approved / paid | Success token, paired with a text label |
| Pending / needs attention | Warning token, paired with a text label |
| Failed / rejected / overdue | Error token, paired with a text label |
| Disabled / unavailable | Muted token |
| Decorative or redundant icon | Omit when it adds no information |

### 68.3 Icon interaction and accessibility

- Icons used as buttons must have an accessible name or tooltip.
- Do not use an icon alone for destructive or high-impact actions when the meaning may be ambiguous.
- Preserve visible focus indicators for keyboard navigation.
- Do not communicate status by icon color alone; include text or another clear indicator.
- Use consistent icon sizes: 16 px for dense table actions, 18–20 px for standard controls, and 20–24 px for primary navigation where appropriate.
- Keep touch targets comfortably usable on mobile; the interactive target may be larger than the icon itself.

---

## 69. PPDB Enterprise Screen Pattern

The PPDB portal must follow the shared SEKOCI enterprise design system. It must not introduce a separate visual language.

### 69.1 PPDB dashboard structure

The administrative PPDB dashboard should use this hierarchy:

1. Global application header with product identity, global search, notifications, and user profile.
2. Sidebar navigation with PPDB as the active module and its authorized submenus.
3. Breadcrumb and page title: `PPDB / Dashboard` and `Dashboard PPDB`.
4. Context filters: academic year, admission wave, date range, and other PRD-defined filters.
5. KPI cards for key operational states, such as total applicants, under verification, accepted, rejected, and incomplete applications—only when supported by the approved PRD and available data.
6. Main analytics: applicant trend over time and applicant distribution by school level, admission route, or another relevant dimension.
7. Recent activity feed showing meaningful events, actors or affected records, status, and timestamp where available.
8. Applicant data table with search, filters, sorting, pagination, status badges, row actions, and authorized bulk actions.
9. Supporting summaries, such as admission-route distribution, when they add operational value and do not duplicate the main chart.

Do not treat example KPI values, labels, or sample records in visual mockups as approved business rules or production data. Use the PRD and validated source data as the authority.

### 69.2 Applicant list pattern

The applicant list should include, where defined by the PRD:

- Search by applicant name, registration number, or school.
- Filters for academic year, school level, admission route, application status, document verification status, and registration date.
- Clear active-filter chips and a one-step way to clear filters.
- A compact, readable data table with stable column alignment.
- Pagination, total record count, page-size selection, and current-page context.
- Export actions that respect user permissions, current filters, and data scope.
- Row actions limited to those authorized for the current role and record state.
- Bulk actions only when the workflow and permission rules explicitly support them.

On mobile, do not simply compress the desktop table until the text becomes unreadable. Use a mobile-appropriate row/card presentation, prioritized fields, a contextual detail page or drawer where appropriate, and accessible action controls.

### 69.3 Applicant detail and document verification

Applicant detail should present the applicant identity and registration reference, current application status, progress through the admission workflow, key applicant information, submitted documents, verification status, relevant payment information, and audit/activity history when required by the PRD.

Document verification should make the following clear:

- Which documents are required and which have been submitted.
- The verification status of each document.
- The reviewer action available to the current role.
- The reason for rejection or requested revision, where applicable.
- The timestamp and actor for meaningful verification decisions.
- Confirmation before high-impact actions such as rejecting an application or finalizing a decision.

Use a drawer for quick contextual review when it preserves list context. Use a full detail page for complex workflows, substantial document review, or multi-step decisions.

### 69.4 PPDB status presentation

Use the approved semantic status system and always pair color with text. Example statuses may include `Draft`, `Submitted`, `Under Review`, `Documents Incomplete`, `Revision Required`, `Accepted`, `Rejected`, `Waiting for Re-registration`, and `Completed`; the actual status vocabulary and transitions must come from the PRD/SRS.

Never invent status transitions or expose actions that the current role is not authorized to perform.

### 69.5 PPDB mobile behavior

- Keep the current academic year or admission context visible when it affects the data.
- Stack KPI cards and analytics in a readable order.
- Allow charts to resize and preserve useful labels and tooltips.
- Prioritize applicant identity, registration number, status, and next required action in mobile list items.
- Use full-page flows for complex application tasks and accessible dialogs for short confirmations.
- Ensure upload, document review, filter, pagination, and primary-action controls are usable with touch input.
- Do not hide critical errors, payment status, verification outcomes, or required next steps on mobile.

---

## 70. Prototype Review and Approval Rules

Before a screen is marked ready for stakeholder review or development handoff, verify:

- The screen is traceable to an approved PRD requirement and correct phase/menu.
- The screen uses the shared SEKOCI design tokens and iconography.
- The user role, unit scope, and permitted actions are clear.
- Desktop and mobile states have been considered.
- Loading, empty, error, success, disabled, and permission-denied states are specified where applicable.
- Forms have field-level validation and preserve valid input when errors occur.
- Tables and charts remain usable at supported breakpoints.
- Destructive and high-impact actions include suitable confirmation.
- Important status and chart information is not communicated by color alone.
- Sample content is realistic but is not presented as approved business logic.
- No unsupported menus, actions, status transitions, or requirements have been invented.

A static generated image is a visual reference, not proof that interactions, responsive behavior, accessibility, authorization, or data behavior have been implemented. These must be validated in the interactive prototype and implementation.

---

## 71. Related Design Documents

Keep the following artifacts separate and linked:

- `DESIGN.md` — the product-wide design principles, visual tokens, responsive rules, interaction standards, accessibility, and shared UI patterns.
- `COMPONENT_CATALOG.md` — the detailed inventory of reusable components, variants, states, properties, responsive behavior, and usage examples.
- `SCREEN_CATALOG.md` (optional when the project needs it) — the inventory of screens with role, portal, phase, menu, requirement traceability, and state coverage.
- Approved prototype — the visual reference for actual screen composition and reviewed flows.
- PRD / SRS — the authority for business requirements, functional behavior, business rules, and acceptance criteria.

If these documents conflict, business behavior must follow the PRD/SRS; visual and interaction consistency must follow the approved DESIGN.md and prototype. Any unresolved conflict must be recorded and clarified rather than silently guessed.

