---
name: SEKOCI Integrated Operations
colors:
  surface: '#FFFFFF'
  surface-dim: '#d2d9f4'
  surface-bright: '#faf8ff'
  surface-container-lowest: '#ffffff'
  surface-container-low: '#f2f3ff'
  surface-container: '#eaedff'
  surface-container-high: '#e2e7ff'
  surface-container-highest: '#dae2fd'
  on-surface: '#131b2e'
  on-surface-variant: '#434655'
  inverse-surface: '#283044'
  inverse-on-surface: '#eef0ff'
  outline: '#737686'
  outline-variant: '#c3c6d7'
  surface-tint: '#0053db'
  primary: '#004ac6'
  on-primary: '#ffffff'
  primary-container: '#2563eb'
  on-primary-container: '#eeefff'
  inverse-primary: '#b4c5ff'
  secondary: '#505f76'
  on-secondary: '#ffffff'
  secondary-container: '#d0e1fb'
  on-secondary-container: '#54647a'
  tertiary: '#2c4bb9'
  on-tertiary: '#ffffff'
  tertiary-container: '#4865d4'
  on-tertiary-container: '#eff0ff'
  error: '#DC2626'
  on-error: '#ffffff'
  error-container: '#ffdad6'
  on-error-container: '#93000a'
  primary-fixed: '#dbe1ff'
  primary-fixed-dim: '#b4c5ff'
  on-primary-fixed: '#00174b'
  on-primary-fixed-variant: '#003ea8'
  secondary-fixed: '#d3e4fe'
  secondary-fixed-dim: '#b7c8e1'
  on-secondary-fixed: '#0b1c30'
  on-secondary-fixed-variant: '#38485d'
  tertiary-fixed: '#dde1ff'
  tertiary-fixed-dim: '#b8c4ff'
  on-tertiary-fixed: '#001453'
  on-tertiary-fixed-variant: '#173bab'
  background: '#F8FAFC'
  on-background: '#131b2e'
  surface-variant: '#dae2fd'
  primary-light: '#EFF6FF'
  border: '#E2E8F0'
  muted: '#94A3B8'
  success: '#16A34A'
  success-light: '#F0FDF4'
  warning: '#D97706'
  warning-light: '#FFFBEB'
  error-light: '#FEF2F2'
  info: '#0284C7'
  info-light: '#F0F9FF'
typography:
  display:
    fontFamily: Inter
    fontSize: 32px
    fontWeight: '700'
    lineHeight: 40px
    letterSpacing: -0.02em
  display-mobile:
    fontFamily: Inter
    fontSize: 24px
    fontWeight: '700'
    lineHeight: 32px
    letterSpacing: -0.015em
  page-title:
    fontFamily: Inter
    fontSize: 24px
    fontWeight: '600'
    lineHeight: 32px
    letterSpacing: -0.015em
  page-title-mobile:
    fontFamily: Inter
    fontSize: 20px
    fontWeight: '600'
    lineHeight: 28px
    letterSpacing: -0.01em
  section-title:
    fontFamily: Inter
    fontSize: 18px
    fontWeight: '600'
    lineHeight: 24px
    letterSpacing: -0.01em
  card-title:
    fontFamily: Inter
    fontSize: 16px
    fontWeight: '600'
    lineHeight: 24px
    letterSpacing: -0.005em
  body-default:
    fontFamily: Inter
    fontSize: 14px
    fontWeight: '400'
    lineHeight: 20px
  body-medium:
    fontFamily: Inter
    fontSize: 14px
    fontWeight: '500'
    lineHeight: 20px
  body-small:
    fontFamily: Inter
    fontSize: 13px
    fontWeight: '400'
    lineHeight: 18px
  label:
    fontFamily: Inter
    fontSize: 12px
    fontWeight: '500'
    lineHeight: 16px
    letterSpacing: 0.01em
  caption:
    fontFamily: Inter
    fontSize: 11px
    fontWeight: '400'
    lineHeight: 14px
    letterSpacing: 0.01em
rounded:
  sm: 0.125rem
  DEFAULT: 0.25rem
  md: 0.375rem
  lg: 0.5rem
  xl: 0.75rem
  full: 9999px
spacing:
  gutter: 1.5rem
  gutter-mobile: 1rem
  margin: 2rem
  margin-mobile: 1rem
  space-2xs: 0.25rem
  space-xs: 0.5rem
  space-sm: 0.75rem
  space-md: 1rem
  space-lg: 1.25rem
  space-xl: 1.5rem
  space-2xl: 2rem
  space-3xl: 2.5rem
  space-4xl: 3rem
---

## Brand & Style

This design system embodies an institutional, high-density, enterprise SaaS aesthetic engineered specifically for administrative multi-unit school operations. Its personality is utilitarian, dependable, orderly, and calm. Rather than relying on decorative consumer trends, the system establishes trust through rigorous alignment, information clarity, and predictable interactions.

### Design Principles
- **Clarity Over Embellishment:** Form follows data density. Every stroke, color token, and spacing value serves an operational purpose.
- **Structural Integrity:** Boundaries and groupings are established through clean 1px hairline borders (`#E2E8F0`) and calibrated white-space rhythms, never through aggressive drop shadows, glassmorphism, or heavy skeuomorphism.
- **Accessible & Cognitive Restraint:** Status is never communicated by color alone. High-contrast slate typography anchors every view, while brand blue directs attention exclusively toward key actions and verified system states.
- **Strict Light Mode Enterprise Standard:** The interface operates solely within an intentional, high-legibility light mode tailored for daylight administrative productivity and continuous daily operations.

## Colors

The color palette reflects institutional authority, functional restraint, and rigorous accessibility.

### Key Roles
- **Primary (`#2563EB`)**: The signature enterprise blue reserved for key call-to-action controls (`[Save]`, `[Submit]`, `[Approve]`), active navigation tabs, focused selection rings, and validated interactive indicators.
- **Primary Dark / Tertiary (`#1E40AF`)**: Deep corporate blue used for hover and active-pressed states on primary controls, ensuring tactile feedback without introducing secondary hues.
- **Primary Light (`#EFF6FF`)**: Subtle azure wash deployed strictly for active sidebar items, selected data table rows, and highlighted operational cards.
- **Secondary (`#64748B`)**: Neutral slate dedicated to metadata, secondary labels, column sorting handles, and default icon states.
- **Neutral / Text (`#0F172A`)**: Near-black slate delivering high WCAG AAA contrast for body copy, data cells, headers, and primary administrative summaries.

### Functional Surfaces & Semantic Accents
- **Canvas & Surface**: Application shell background rests on `#F8FAFC`, while actionable cards, popovers, modals, and data grids rely on `#FFFFFF` with hairline `#E2E8F0` framing.
- **Semantic Badges & Indicators**:
  - `Success` (`#16A34A` / bg `#F0FDF4`): Completed, Verified, Active, Paid.
  - `Warning` (`#D97706` / bg `#FFFBEB`): Pending, Partial, Needs Attention.
  - `Error` (`#DC2626` / bg `#FEF2F2`): Rejected, Overdue, Critical, Destructive triggers.
  - `Info` (`#0284C7` / bg `#F0F9FF`): Draft, Scheduled, In Progress.

## Typography

The typography uses Inter across all levels to optimize micro-legibility within information-dense grids, complex multi-step forms, and data visualizations. 

### Typographic Hierarchy Rules
- **Display & Page Titles:** Reserved exclusively for high-level dashboard summaries, fiscal totals, and primary module headings. Features subtle negative tracking (`-0.02em` to `-0.015em`) to maintain tightness at larger sizes.
- **Card & Section Titles:** Employs medium to semi-bold weights (`600`) to anchor scanning zones across multi-column dashboard widgets.
- **Body & Data Grid:** The default reading size is `14px` (`body-default`), pairing `20px` line heights to accommodate dense data displays while preventing text collisions.
- **Labels & Microcopy:** Labels for form headers and badge items are rendered in `12px` medium (`fontWeight: 500`). Timestamps, auditing footers, and auxiliary metadata use `11px` caption typography to maintain a clear visual hierarchy.

## Layout & Spacing

The platform is constructed on a strict 4px mathematical rhythm. Spacing values correspond directly to:
- `4px` (`space-2xs`): Icon-to-text gaps, inline pill padding, dense tag alignments.
- `8px` (`space-xs`): Stacked form field gaps, control interiors, table action gaps.
- `12px` (`space-sm`): Input padding, secondary action gaps, compact container padding.
- `16px` (`space-md`): Standard module margins, control container padding, input row spacing.
- `20px` (`space-lg`): Drawer inner padding, multi-field form clusters.
- `24px` (`space-xl`): Card interior padding, widget separation.
- `32px` (`space-2xl`): Major section breaks, canvas gutters.
- `40px` (`space-3xl`) & `48px` (`space-4xl`): Application shell wrappers, primary view padding.

### Layout Architecture
- **Desktop (1280px+):** Fixed 240px or 280px left navigation sidebar, 64px persistent top navigation header containing multi-unit organization context (`Organization` -> `Unit` -> `Academic Year`), and a fluid 12-column workspace with `24px` gutters and `32px` outer padding.
- **Tablet (768px - 1023px):** Collapsed icon-only navigation rail, 2-column card layouts, 16px gutters, and preserved touch targets (minimum 44px).
- **Mobile (< 768px):** Off-canvas drawer navigation, single-column stacked forms, table views transformed into structured card lists, and a condensed `16px` margin.

## Elevation & Depth

This system intentionally departs from consumer glassmorphism, heavy blur, and skeuomorphic drop shadows. Depth is established through **tonal separation** and **low-contrast perimeter lines**.

### Surface Elevation Hierarchy
- **Canvas Ground (`#F8FAFC`):** The foundational plane for the viewport content area.
- **Level 1 — Inset & Hairline Panels (`#FFFFFF` + 1px `#E2E8F0` Border):** Standard content cards, data tables, metrics panels, and primary navigation sidebars. No drop shadows are applied.
- **Level 2 — Floating Overlays (Dropdowns, Popovers, Date Pickers):** Background `#FFFFFF`, border 1px `#E2E8F0`, accompanied by a minimal ambient shadow: `box-shadow: 0 4px 6px -1px rgba(15, 23, 42, 0.06), 0 2px 4px -2px rgba(15, 23, 42, 0.04)`.
- **Level 3 — Modal Dialogs & Action Sheets:** Background `#FFFFFF`, 1px `#E2E8F0`, ambient shadow: `box-shadow: 0 20px 25px -5px rgba(15, 23, 42, 0.08), 0 8px 10px -6px rgba(15, 23, 42, 0.04)`. Accompanied by a non-blurred solid backdrop: `rgba(15, 23, 42, 0.45)`.
- **Level 4 — Toasts & Floating Notifications:** Background `#FFFFFF`, border 1px `#E2E8F0`, high-contrast left status accent bar (3px width), shadow: `box-shadow: 0 10px 15px -3px rgba(15, 23, 42, 0.08)`.

## Shapes

The design system uses a calibrated, functional border radius tier. Curved pill shapes and circular elements are avoided to maintain an enterprise software feel.

### Border Radius Tiers
- **6px (`rounded-md` equivalent in standard libraries):** Interactive controls including buttons, text inputs, selects, switches, filter badges, and table checkboxes.
- **8px (`rounded-lg`):** Content cards, KPI overview tiles, analytical charts, and list items.
- **10px:** Alert dialogs, standard confirmation modals, and contextual floating menus.
- **12px (`rounded-xl`):** Primary canvas wrappers, large slide-out panels, and multi-unit tenancy switcher cards.

All shapes maintain crisp geometric boundaries defined by 1px solid `#E2E8F0` borders. Focus states introduce a clean 2px `#2563EB` offset ring (`box-shadow: 0 0 0 2px #FFFFFF, 0 0 0 4px #2563EB`).

## Components

### Buttons
- **Primary:** Background `#2563EB`, text `#FFFFFF`, radius 6px, padding `8px 16px`, height 36px (desktop). Hover: `#1E40AF`. Active: `#1D4ED8`. Focus: 2px offset outline.
- **Secondary / Outline:** Background `#FFFFFF`, border 1px `#E2E8F0`, text `#0F172A`. Hover: `#F8FAFC` with border `#CBD5E1`.
- **Destructive:** Background `#DC2626`, text `#FFFFFF`. Hover: `#B91C1C`.
- **Ghost / Tertiary:** Background transparent, text `#64748B`. Hover: `#F1F5F9`, text `#0F172A`.

### Status Badges & Chips
- Compact dimensions: height 22px, border radius 6px, padding `2px 8px`, typography `12px` medium.
- Paired with an explicit leading 6px colored dot indicator or 14px Lucide outline icon to ensure accessibility without relying on color alone.
- Variants:
  - *Success:* Background `#F0FDF4`, border `#BBF7D0`, text `#15803D`.
  - *Warning:* Background `#FFFBEB`, border `#FDE68A`, text `#B45309`.
  - *Error:* Background `#FEF2F2`, border `#FECACA`, text `#B91C1C`.
  - *Info:* Background `#EFF6FF`, border `#BFDBFE`, text `#1D4ED8`.
  - *Neutral:* Background `#F8FAFC`, border `#E2E8F0`, text `#475569`.

### Input Fields & Selects
- Height 36px, background `#FFFFFF`, border 1px `#E2E8F0`, radius 6px, padding `8px 12px`, typography `14px` (`#0F172A`).
- Placeholder text: `#94A3B8`.
- Focus state: Border `#2563EB`, ring 2px `rgba(37, 99, 235, 0.2)`.
- Error state: Border `#DC2626`, helper message in `#DC2626` at `12px`.

### Tables & Data Grids
- Outer wrapper enclosed in 1px `#E2E8F0` border with 8px radius. Header background `#F8FAFC`, text `#64748B` uppercase `11px` bold, height 40px, border-bottom 1px `#E2E8F0`.
- Rows: Background `#FFFFFF`, height 48px (standard) or 40px (compact administrative), border-bottom 1px `#F1F5F9`. Hover: `#F8FAFC`. Selected: `#EFF6FF`.

### Cards & KPI Tiles
- Background `#FFFFFF`, border 1px `#E2E8F0`, radius 8px, padding 20px.
- KPI layout: Top label in `12px` muted uppercase text, Lucide 20px outline icon in `#64748B` top-right, value in `24px` bold slate (`#0F172A`), followed by a bottom comparison trend badge.

### Multi-Unit Tenancy Context Header
- Anchored at the top-left of the application bar.
- Interactive nested breadcrumb trigger containing Organization -> Unit -> Academic Year (e.g., "Foundation Al-Azhar / High School Unit / 2024-2025 Odd").
- Displays active unit icon, chevron indicator, and switches with a structured 10px-radius dropdown list.