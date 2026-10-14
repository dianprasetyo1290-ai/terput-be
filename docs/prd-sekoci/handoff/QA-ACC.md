# Handoff QA-ACC: Akuntansi (menu 32–38, import COA/saldo awal)

Branch `feat/QA-ACC` (base b984872). Service `services/accounting` (Laravel 12 + Livewire 3) plus a small change in the `web` sidebar.

## Status per temuan

| BUG | Status | Files changed (main) | Proof (test) |
|---|---|---|---|
| BUG-059 Import COA & saldo awal | **PARTIAL** | `app/Support/Importer.php`, `app/Livewire/Pages/Import/*`, view `import/import.blade.php`, route `/import` | `QaAccReportsTest::test_coa_import_*`, `test_opening_balance_import_*`, `test_import_page_requires_permission_and_confirmation` |
| BUG-060 COA hard delete | FIXED | `ChartOfAccount.php` + view, `app/Support/Accounts.php` (`deactivationBlockers`) | `QaAccJournalCoaTest::test_deactivate_is_refused_*`, `test_unused_account_is_deactivated_*`, `test_auditor_cannot_deactivate` |
| BUG-061 RBAC | FIXED | `app/Support/AccountingRoles.php`, `SsoController.php` (ROLE_MAP), migration `2026_10_14_000001`, seeders, `routes/web.php`, `routes/api.php`, `authorize()` in HJT/Balance/EOY/WithTransactionHeader/Detail, `web/src/router/domains/accounting.routes.ts` | `QaAccRbacTest` (6 tests), `SsoJwtTest`, `EndOfYearUiTest::test_end_of_year_requires_permission` |
| BUG-062 Approval & jurnal pembalik di UI | FIXED | `app/Livewire/Traits/WithJournalWorkflow.php`, `JT.php`, `JV.php` + views, `app/Support/Journals.php` (maker-checker) | `QaAccJournalCoaTest::test_maker_submits_checker_approves`, `test_reject_returns_to_draft`, `test_reversal_from_ui_*`, `test_finance_manager_cannot_submit_*` |
| BUG-063 Neraca saldo | FIXED | `app/Support/Statements.php::trialBalance`, `Report/TrialBalance/*`, route `/report/trial-balance` | `QaAccReportsTest::test_trial_balance_counts_posted_only_and_balances` |
| BUG-064 Laporan keuangan standar | FIXED | `Statements::build/activity/position/cashFlow/netAssetChanges`, `Report/Statements/*`, route `/report/statements` | `QaAccReportsTest::test_financial_statements_tie_out`, `test_statements_page_for_executive` |
| BUG-065 Konsolidasi & eliminasi | FIXED | `Statements::consolidation`, `Report/Consolidation/*`, route `/report/consolidation`, `Ledger::balances(..., $division)` | `QaAccReportsTest::test_consolidation_eliminates_interunit_accounts` |
| BUG-140 Dashboard KPI/status/posting gagal | FIXED | `Home.php`, `partials/home-kpi.blade.php`, `AutoPosting/*` (`/autoposting`) | `QaAccReportsTest::test_dashboard_shows_kpis_*`, `test_auto_posting_page` |
| BUG-141 Template COA per unit | **PARTIAL** | via CSV import (`/import`) | see BUG-059 |
| BUG-142 Form COA (induk, saldo normal, aktif) | FIXED | `ChartOfAccount.php` + view, `Accounts::parentProblems` | `test_coa_form_saves_parent_*`, `test_parent_must_be_same_group_*`, `test_normal_balance_is_locked_once_used` |
| BUG-143 Posting ke akun induk | FIXED | `Accounts::postingProblems`, `WithTransactionDetail::saveDetail`, `Journals::transition`, `AutoJournalService` | `test_parent_inactive_and_control_accounts_are_not_postable`, `test_auto_posting_to_parent_account_is_refused` |
| BUG-144 Template & jurnal berulang | **DEFERRED** | – | – |
| BUG-145 Nilai negatif / tanpa keterangan | FIXED | `WithTransactionDetail::saveDetail`, `Journals::transition` (re-check on submit/approve) | `test_journal_line_rejects_negative_amount_and_empty_description` |
| BUG-146 Akun kontrol piutang 1201 | FIXED (needs config) | `mst_account.is_control` (migration `2026_10_14_000002`), `Accounts::postingProblems` | `test_parent_inactive_and_control_accounts_are_not_postable`, `test_approval_rechecks_lines` |
| BUG-147 Tutup buku cek draft & posting gagal | FIXED | `EndOfMonth/EOM.php` + view | `QaAccReportsTest::test_eom_*` (4 tests) |
| BUG-148 Drilldown buku besar | **PARTIAL** | `JournalDetail/*` (`/journal/{no}`), GL view links, `QueryGeneral.php`/`ReportGeneralLedger.php` (posted only) | `test_journal_drilldown_open_and_closed`, `test_general_ledger_detail_links_to_journal_and_skips_drafts` |
| BUG-178 Buka kembali periode | **DEFERRED** | – | – |

### What was done

- **RBAC (061).** The Sekoci role now maps to `Accounting` / `FinanceManager` / `Auditor` / `Executive` and is always re-synced on SSO, so the active role decides access.
  - Executive is kepala_sekolah/kepala_yayasan. It sees only the dashboard, financial reports and consolidation. COA, journals and the GL return 403.
  - Auditor is read-only.
  - FinanceManager is read-only plus approve/reject/reverse.
  - Accounting can do everything except user/role admin.
  - The routes `/z`, `/summarygeneralledger`, the report generator, `report/{id}`, `/form-balance` and `/api/search-*` were previously open and now carry permissions.
  - Livewire write actions call `authorize()`.
- **COA (060/142/143/146).**
  - The form now has parent, normal balance, active, kas/bank, inter-unit and control fields.
  - The code is immutable on edit (it used to delete and re-create the account).
  - Group and normal balance are locked once the account has activity.
  - Delete is replaced by Nonaktifkan, which is refused with a list of reasons: balance, open journals, auto-posting mapping, transaction type, active children.
  - Journal lines are rejected for inactive accounts, parent accounts and control accounts (manual entries only).
  - Changes are recorded in `acc_audit_log`.
- **Workflow (062).**
  - Per-status actions in the journal list:
    - Draft: Ajukan
    - Pending: Setujui/Tolak
    - Posted: Jurnal Pembalik
  - Maker-checker: the maker or submitter cannot approve or reject, and a reversal must be booked by someone other than the original maker.
  - A reversal journal cannot itself be reversed.
  - `/journalvalidation` is now the approval queue; it used to return 403 for every role.
- **End of Month (147).**
  - Blocks while draft/pending journals or failed auto-postings exist in the period, and lists them.
  - "Data Transaksi" is informational only, so an empty month can be closed.
  - Server side: `authorize`, the checks are re-run, a stale page or second run is refused (locked `expectedPeriod` plus a `lockForUpdate` on `parameter`), and an audit entry `period.close` is written.
  - Before this fix, EOM also moved drafts into history and counted them in balances.
- **Reports (063/064/065).** Everything is built from `Ledger::balances` (posted only) and now supports a per-unit filter.
  - Trial balance: opening, movement, ending, and a balanced indicator.
  - Four statements, each compared with the previous period and showing the period, unit, generation time and a SEMENTARA banner while the period is open.
  - The cash flow uses the direct method, so kas↔bank transfers cancel out.
  - Consolidation: per-unit columns, elimination of `is_interunit` accounts, and a warning when the remainder is not zero.
  - The trial balance, the statements and the net-asset changes are cross-checked against each other in the tests.
- **Dashboard (140).** KPIs for revenue, expense, surplus, kas & bank, receivables (control accounts) and net assets; journal counts per status; up to 5 failed postings with a link to `/autoposting`.
- **GL (148).** The `trans_no` in the GL detail links to `/journal/{no}?date=`, which shows head, lines, source/reference, reversal links and the auto-posting log. The GL query for the open period now skips drafts/pending (BR-ACC-03).
- **Import (059).** `/import` takes a CSV for COA and for the opening balances of the active year.
  - It is all-or-nothing and has a preview.
  - Opening balances must balance.
  - The import replaces the year's whole periode-0 set, with explicit confirmation when one already exists.

### PARTIAL / DEFERRED, with a short design

- **BUG-059 piutang per siswa (expected total Rp150.000.000).** The per-student sub-ledger lives in SEKOCI billing (BR-ACC-06), not in accounting. The import here only carries the total of the control account 1201 in the opening balance.
  - Design: the opening per-student receivables are imported in billing (outstanding opening bills per NIS). After that, billing sends one opening journal (`source=billing`, reference `billing:opening:<year>`) to `/api/internal/journals`, or the total is entered as the 1201 opening balance here.
  - A reconciliation check (sum of billing outstanding = 1201 balance) belongs on the accounting dashboard. **That needs a billing endpoint, which belongs to the QA-BILL area.**
- **BUG-141 template COA per unit.** In the current data model one COA is shared per school, and units are divisions (`mst_division`). Balances and journals are separated per unit through `div_code`, so the "COA per unit" in the TC is not this schema's model.
  - Current solution: a per-level (TK/SD/SMK) template is kept as a CSV and imported via `/import`; an example is on the page.
  - Real design, if a COA per unit is required: add a `mst_account.unit_scope` column (nullable = all units), a `coa_templates(id, name)` + `coa_template_lines` table, and a "Salin template ke unit" action that creates `unit_scope` accounts. The journal form then filters accounts by the line's `div_code`.
- **BUG-144 template & recurring journals (DEFERRED).**
  - Design: tables `acc_journal_templates(id, name, trans_code, created_by)` and `acc_journal_template_lines(template_id, acc_code, div_code, description, debit, credit)`, plus `acc_recurring(id, template_id, day_of_month, next_run date, active, last_trans_no)`.
  - A "Buat dari template" action in the JT page creates a **draft** that follows the normal workflow.
  - A scheduler job `accounting:recurring` (per tenant, daily) creates a draft for every `next_run <= today` inside the open period, is idempotent on `(recurring_id, period)`, and records failures in `acc_auto_postings` with `kind=recurring`.
  - It was not done because it needs a per-tenant scheduler and its own UI. Faking it was not acceptable.
- **BUG-148 list of source payments.** The drilldown shows `reference` and `acc_auto_postings.source_ids`. The billing service currently sends the reference only, not the payment IDs.
  - Design: `AutoJournalService` stores `source_ids` (payment/bill IDs) from the payload, and the drilldown page calls a billing endpoint (`GET /api/billing/payments?ids=`) through the internal key to list payments: number, student, amount. **This depends on QA-BILL.**
- **BUG-178 reopen period (DEFERRED).**
  - Design: a `acc_period_reopen_requests(id, year, period, reason, requested_by, approved_by, status, created_at, decided_at)` table.
  - The request is made by Accounting and approved by FinanceManager (maker-checker, with a new permission `editperiod_reopen`).
  - On approval, inside one transaction with a lock on `parameter`, and only for the most recent closed period:
    - move `this_head/this_detail` of that period back to `trans_*`
    - zero `mst_saldo_news` for that period
    - set `parameter.period` back
    - write `AuditLog::record('period.reopen')`
  - It is refused when a later period already has postings or the year has gone through EOY.
  - It was not done because it reverses EOM, the riskiest operation on the books, and it needs approval from the business owner.

## New migrations (tenant, raw `IF NOT EXISTS`)

- `2026_10_14_000001_accounting_rbac_roles.php`
  - Adds the modules journal_validation, trial_balance, consolidation, auto_posting and import_data, plus their permissions.
  - Roles Accounting, FinanceManager, Auditor and Executive are created or synced. Admin/Accounting only gain the new permissions (additive). The legacy `Finance` role is narrowed to dashboard + reports.
  - Skipped when the tenant has no permission seed yet. down() is a no-op.
- `2026_10_14_000002_coa_control_flag_and_audit.php`: adds `mst_account.is_control boolean default false` and creates table `acc_audit_log`. down() drops both.

## New endpoints and pages (Laravel, inside auth + TenantSchema)

| Path | Permission |
|---|---|
| `/journalvalidation` (approval queue; already existed, was 403) | viewjournal_validation (approve: editjournal_validation) |
| `/autoposting` | viewauto_posting |
| `/import` | viewimport_data (run: createimport_data) |
| `/journal/{transNo}?date=` | viewgeneral_ledger |
| `/report/trial-balance` | viewtrial_balance |
| `/report/statements` | viewfinancial_report |
| `/report/consolidation` | viewconsolidation |
| `/api/search-accounts`, `/api/search-divisions` (moved from api.php, previously unauthenticated) | viewjournal_transaction |

Workflow permissions:
- `editjournal_transaction` = submit
- `editjournal_validation` = approve/reject/reverse
- `createend_of_month` = EOM
- `createend_of_year` = EOY

The SPA (`accounting.routes.ts`) has new pages for approval, posting otomatis, neraca saldo, laporan keuangan, konsolidasi and import. Role sets are EXEC/READ/ADMIN; COA, journals and GL are hidden from kepala_*.

## Verification (real output)

```
# full suite (docker run --rm p3b-php:8.4 + postgres:16 in its own network, no ports)
OK, but some tests were skipped!
Tests: 140, Assertions: 429, Skipped: 20.

# QA-ACC tests only
OK (35 tests, 177 assertions)

# migrate up -> rollback 3 -> up (fresh DB, ModulesSeeder in between)
  2026_10_14_000001_accounting_rbac_roles ........................ 0.68ms DONE   (rollback)
  2026_10_14_000001_accounting_rbac_roles ...................... 267.65ms DONE   (up)
  2026_10_14_000002_coa_control_flag_and_audit .................. 35.74ms DONE
{"roles":["Accounting","Admin","Auditor","Executive","Finance","FinanceManager","SuperAdmin"],
 "auditor_perms":13,"executive":["viewfinancial_report","viewconsolidation"],"is_control":true,"acc_audit_log":true}

# web
npm ci OK; npx vue-tsc -b OK; npm run build: ✓ built in 3.90s
```

Skipped: 9 cases were already skipped at base by Jetstream feature flags. The other 11 belong to 5 Jetstream scaffold classes (Authentication, ProfileInformation, DeleteAccount, Example, PasswordConfirmation), now marked skipped with a reason in their `setUp`. 8 of those 11 were already failing at base b984872, because they assume a `users` table with `name`, a password login and a `dashboard` route.

There is also a test-only fix: `tests/TestCase.php` calls `withoutVite()`, and `UserFactory` writes `user_name`.

## Server configuration steps (do NOT run from this task)

1. Deploy the image, then migrate each tenant schema: `php artisan accounting:tenants`, then `DB_SEARCH_PATH=tenant_<hex>,public php artisan migrate --force` for each school. **Do not** run `db:seed`; the migration installs the roles itself.
2. In each school, open COA and tick **Akun kontrol** on the student receivables account (e.g. 1201) so manual journals to it are refused (BUG-146). There is deliberately no automatic backfill, because the code differs per school.
3. Tick **Antar unit** on the inter-unit receivable/payable accounts so consolidation can eliminate them (BUG-065).
4. Check that kas/bank accounts have **Kas/Bank** ticked. The 2026-10-08 migration backfilled this from the account name, but the cash flow and the KPIs depend on it.
5. Open the roles in the accounting app: users who were mapped to `Finance` are moved to `Executive` automatically on their next SSO.

## Assumptions and open questions

- **akuntan role in identity.** `ROLE_MAP` accepts `akuntan`/`accounting` → Accounting, but the role CHECK in the identity service, and the `Role` type in the web, have no such role yet. Until identity adds one (QA-IDN / owner decision), daily accounting work is done by `admin`. finance_manager and auditor already exist in identity.
- **Opening-balance import replaces the whole periode-0 set** for the active year. This is the conservative choice, so the result is exactly the file and stays balanced. It requires explicit confirmation when opening balances already exist.
- **Reversal date.** The reversal is dated today, clamped to the open period, as in the original `Journals::reverse`.
- **The Executive role sees the dashboard KPIs** (the summary only, without the journal list). This follows the BUG-061 expectation "dashboard + laporan".
- **Consolidation** uses one shared COA with per-unit `div_code` columns. When a line has a `div_code` that is not in `mst_division`, a "Tanpa unit" column appears so the total still ties out.
- The legacy `/report/financial` (Report Generator) still exists. It is labelled "Laporan Kustom" in the SPA; the standard statements are at `/report/statements`.
