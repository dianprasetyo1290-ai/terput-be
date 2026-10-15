# Handoff QA-ACC: Akuntansi (menu 32–38, import COA/saldo awal)

Branch `feat/QA-ACC` (base b984872). Service `services/accounting` (Laravel 12 + Livewire 3) plus a small change in the `web` sidebar.

## Status per temuan

| BUG | Status | Files changed (main) | Proof (test) |
|---|---|---|---|
| BUG-059 Import COA & saldo awal | FIXED (round 2: receivables control vs billing + sign-off; needs BILLING_INTERNAL_URL + the billing contract below) | `app/Support/Importer.php`, `app/Livewire/Pages/Import/*`, view `import/import.blade.php`, route `/import` | `QaAccReportsTest::test_coa_import_*`, `test_opening_balance_import_*`, `test_import_page_requires_permission_and_confirmation` |
| BUG-060 COA hard delete | FIXED | `ChartOfAccount.php` + view, `app/Support/Accounts.php` (`deactivationBlockers`) | `QaAccJournalCoaTest::test_deactivate_is_refused_*`, `test_unused_account_is_deactivated_*`, `test_auditor_cannot_deactivate` |
| BUG-061 RBAC | FIXED | `app/Support/AccountingRoles.php`, `SsoController.php` (ROLE_MAP), migration `2026_10_14_000001`, seeders, `routes/web.php`, `routes/api.php`, `authorize()` in HJT/Balance/EOY/WithTransactionHeader/Detail, `web/src/router/domains/accounting.routes.ts` | `QaAccRbacTest` (6 tests), `SsoJwtTest`, `EndOfYearUiTest::test_end_of_year_requires_permission` |
| BUG-062 Approval & jurnal pembalik di UI | FIXED | `app/Livewire/Traits/WithJournalWorkflow.php`, `JT.php`, `JV.php` + views, `app/Support/Journals.php` (maker-checker) | `QaAccJournalCoaTest::test_maker_submits_checker_approves`, `test_reject_returns_to_draft`, `test_reversal_from_ui_*`, `test_finance_manager_cannot_submit_*` |
| BUG-063 Neraca saldo | FIXED | `app/Support/Statements.php::trialBalance`, `Report/TrialBalance/*`, route `/report/trial-balance` | `QaAccReportsTest::test_trial_balance_counts_posted_only_and_balances` |
| BUG-064 Laporan keuangan standar | FIXED | `Statements::build/activity/position/cashFlow/netAssetChanges`, `Report/Statements/*`, route `/report/statements` | `QaAccReportsTest::test_financial_statements_tie_out`, `test_statements_page_for_executive` |
| BUG-065 Konsolidasi & eliminasi | FIXED | `Statements::consolidation`, `Report/Consolidation/*`, route `/report/consolidation`, `Ledger::balances(..., $division)` | `QaAccReportsTest::test_consolidation_eliminates_interunit_accounts` |
| BUG-140 Dashboard KPI/status/posting gagal | FIXED | `Home.php`, `partials/home-kpi.blade.php`, `AutoPosting/*` (`/autoposting`) | `QaAccReportsTest::test_dashboard_shows_kpis_*`, `test_auto_posting_page` |
| BUG-141 Template COA per unit | FIXED (round 2) | `app/Support/CoaTemplates.php`, `mst_account.unit_scope`, ChartOfAccount + Division | `QaAccUnitCoaTest` (7) |
| BUG-142 Form COA (induk, saldo normal, aktif) | FIXED | `ChartOfAccount.php` + view, `Accounts::parentProblems` | `test_coa_form_saves_parent_*`, `test_parent_must_be_same_group_*`, `test_normal_balance_is_locked_once_used` |
| BUG-143 Posting ke akun induk | FIXED | `Accounts::postingProblems`, `WithTransactionDetail::saveDetail`, `Journals::transition`, `AutoJournalService` | `test_parent_inactive_and_control_accounts_are_not_postable`, `test_auto_posting_to_parent_account_is_refused` |
| BUG-144 Template & jurnal berulang | FIXED (round 2) | `JournalTemplates`, `RecurringJournals`, command `accounting:recurring-journals`, scheduler, `/journaltemplate` | `QaAccTemplateReopenTest` |
| BUG-145 Nilai negatif / tanpa keterangan | FIXED | `WithTransactionDetail::saveDetail`, `Journals::transition` (re-check on submit/approve) | `test_journal_line_rejects_negative_amount_and_empty_description` |
| BUG-146 Akun kontrol piutang 1201 | FIXED (needs config) | `mst_account.is_control` (migration `2026_10_14_000002`), `Accounts::postingProblems` | `test_parent_inactive_and_control_accounts_are_not_postable`, `test_approval_rechecks_lines` |
| BUG-147 Tutup buku cek draft & posting gagal | FIXED | `EndOfMonth/EOM.php` + view | `QaAccReportsTest::test_eom_*` (4 tests) |
| BUG-148 Drilldown buku besar | FIXED (round 2: down to the billing source; source list needs the billing contract below) | `JournalDetail/*` (`/journal/{no}`), GL view links, `QueryGeneral.php`/`ReportGeneralLedger.php` (posted only) | `test_journal_drilldown_open_and_closed`, `test_general_ledger_detail_links_to_journal_and_skips_drafts` |
| BUG-178 Buka kembali periode | FIXED (round 2) | `app/Support/PeriodReopen.php`, `/period-reopen` | `QaAccTemplateReopenTest` |

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

### PARTIAL / DEFERRED, with a short design (round 1; all closed in round 2, see below)

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


## Putaran 2 (round 2)

Goal from the owner: no dead buttons and no server findings. Every round-1 PARTIAL/DEFERRED item is now FIXED, and every accounting page and Livewire action was audited for each role.

### Status

| BUG | Status | Main changes | Test proof |
|---|---|---|---|
| 059 (accounting side) | FIXED, needs config | `ReceivableReconciliation` on the "Kontrol Total Piutang (Migrasi)" card of `/import`. It compares control accounts (group 1, `is_control`), opening and ledger balances, against billing's receivables summary; sign-off needs a note of ≥10 chars when they differ, and is audited as `migration.signoff`. When billing is unreachable or not set up, the card says "belum dikonfigurasi" and sign-off is refused. | `QaAccSourcesTest` |
| 141 | FIXED | Shared template saved from the COA, then copied to a unit. A unit account has code `<code>.<UNIT>` (≤15 chars), `unit_scope=<UNIT>`, and parents mapped to unit copies. Posting another unit's line to it is refused (manual and auto journals). Division: code read-only on edit, optional template copy on create, delete refused while in use (the old delete CASCADE-deleted journal lines). | `QaAccUnitCoaTest` |
| 144 | FIXED, needs image rebuild | Templates (manual, or copied from a journal), drafts from a template, monthly schedules (day 1–28) producing DRAFTS. The run log has retry and skip. `accounting:recurring-journals` runs over every tenant schema, `hourlyAt(5)`, `withoutOverlapping`, `onOneServer`. The run is idempotent (unique per schedule+date). | `QaAccTemplateReopenTest` |
| 178 | FIXED | Only the last closed period of the active year can be reopened, and only while the active period has no journals. A reason (≥10 chars) is required. Maker-checker: approved by a different user with `editperiod_reopen` (FinanceManager, SuperAdmin). Request, cancel, reject and approve are all audited. Approving moves `this_*` back to `trans_*` and sets the period back. | `QaAccTemplateReopenTest` |
| 148 | FIXED, needs config | JournalDetail has a "Transaksi Sumber" card for auto journals. It loads lazily from billing and shows a readable error or "belum dikonfigurasi". `AutoJournalService` stores `source_refs`. | `QaAccSourcesTest` |
| Button and page audit | FIXED | See below. | `QaAccRoleSmokeTest`, `QaAccPermissionsGuardTest` |

### Button and page audit (all 6 roles)

`QaAccRoleSmokeTest` covers SuperAdmin, Admin, Accounting, FinanceManager, Auditor and Executive. For each role it:
- opens every authenticated GET page and expects 200, or 403 when the route permission is missing;
- checks every `wire:click`/`wire:submit` on each page: the handler must exist on a rendered component, and the role must hold the permission that `LivewireGuard` requires;
- opens every sidebar link the role sees and expects 200.

Last run: SuperAdmin 37 pages / 89 buttons, Admin 33/81, Accounting 31/72, FinanceManager 18/23, Auditor 18/23, Executive 5/8. No problems.

Fixed along the way:
- **Central guard.** `app/Support/LivewireGuard.php` is hooked on Livewire `call` in `AppServiceProvider`. Every public method of a mapped component needs `view<module>`. Writes need `create|edit<module>`, deletes need `delete<module>`. Workflow methods (JT/JV/template drafts/reopen approve) keep their own `authorize()`. Livewire internals (`_startUpload` …) are not checked. Many old pages had no `authorize()` at all.
- **History journal (HJT) is read-only.** It used to allow editing and deleting journals of closed periods. Corrections now go through Buka Kembali Periode or Penyesuaian Tahun Lalu.
- **Opening balance form.** It no longer inserts rows on render. It saves periode 0 of the active year only, with `editbalance`, and is audited (`balance.opening`).
- **Pages that returned 500:**
  - `/periode`: string arithmetic.
  - `/balance`: `vw_balance` was only created by a seeder. New migration `2026_10_21_000004_ensure_vw_balance_view`.
  - `/permissions` opened bare: now redirects to Peran.
  - `/settingwebsite` with no row.
  - `/report`, `/generator/*/detail` with no or unknown id: now redirect to their list.
- **Dead buttons:**
  - Permissions `cancel`/`closeViewModalFileKet` did not exist (added); its "Show N items" picker did nothing (removed).
  - The Report Row Generator sort/limit methods did not exist (added).
  - Financial Report "Show N" was ignored.
  - HJV "Process" called a method that never existed. The page now lists the closed periods instead.
  - EOM's form was still rendered for roles without `createend_of_month`.
- **Permission escalation.** Admin could grant itself `editperiod_reopen`, the checker half of maker-checker. Now only a SuperAdmin can toggle it, an Admin's "save" keeps its current state, and only a SuperAdmin can edit the SuperAdmin role.
- **SPA.** `web/src/router/domains/accounting.routes.ts` adds Template & Jurnal Berulang and Buka Kembali Periode (READ roles).

### Contract for billing (QA-BILL / billing owner)

All calls are internal, header `X-Internal-Key: <INTERNAL_API_KEY>`, envelope `{data: ...}`.

1. `POST` journal request to accounting (existing `JournalRequest`): new optional field `source_refs` = `{ "<type>": ["<id>", ...] }`, for example `{"payment": ["9f..."], "invoice": ["..."]}`. Accounting stores it with the journal.
2. `GET {BILLING_INTERNAL_URL}/internal/journal-sources?school_id=<uuid>&reference=<journal reference>` returns `{data: {reference, items: [{type, id, number, student_name, nis, unit, amount_idr, date, status}]}}`. Accounting renders this list on `/journal/{no}`. A 404 is treated as "no sources".
3. `GET {BILLING_INTERNAL_URL}/internal/receivables/summary?school_id=<uuid>&as_of=<YYYY-MM-DD>` returns `{data: {outstanding_idr, students, as_of}}`. This feeds the 059 control total.

Until billing serves (2) and (3), those cards show a readable "belum dikonfigurasi / tidak tersedia" message. Nothing breaks.

### Server configuration (do NOT run from this task)

1. Set `BILLING_INTERNAL_URL` for accounting (already added to `docker-compose.yml`: `http://billing:${BILLING_PORT}`). `INTERNAL_API_KEY` must match billing's.
2. Rebuild the accounting image. supervisord now has `[program:scheduler]` (`php artisan schedule:work`) for recurring journals.
3. Migrate every tenant schema (`php artisan accounting:provision <school_id>` without `--seed`, or `DB_SEARCH_PATH=tenant_x,public php artisan migrate --force`). Migration `2026_10_21_000001` calls `AccountingRoles::install(additive: true)`, which adds the new permissions (`*journal_template`, `*period_reopen`) to existing roles. **Never** run ModulesSeeder or a bare `db:seed` on a live tenant, because they truncate.
4. Back up before deploying (see the PROD notes in the repo memory).

### Decisions (conservative defaults)

- HJT is read-only. Closed periods change only through reopen or PYA.
- The opening balance form edits periode 0 of the active year only. Periods 1–13 come from journals.
- Unit account code format is `<code>.<UNIT>`, max 15 chars (copying is refused if any code would be longer).
- Reopen covers the last closed period only, needs an empty active period, a reason of ≥10 chars and a different approver, and is not available across End of Year.
- Recurring journals create drafts only, never posted journals, on days 1–28.
- The SPA has no separate "akuntan" role mapped onto the Accounting pages. Accounting users come in through SSO `ROLE_MAP`. **Open item:** confirm that READ/ADMIN in the SPA matches who should see each menu.

### Review items

- `LivewireGuard` method classification (WRITE/DELETE lists): a new write method that is not listed only needs `view<module>` unless the page calls `authorize()` itself.
- The smoke test checks the initial render of each page, not buttons that appear only after opening a modal or form. Those methods are still covered by the guard.
- `Balance::save` refers to legacy `act_` columns. Its form is commented out in the view, and the method is guarded.

### Verification (round 2)

```
# full suite (docker run --rm p3b-php:8.4 + postgres:16, own network, no ports)
Tests: 182, Assertions: 542, Skipped: 20.  OK
# QaAccRoleSmokeTest: 6 roles, 0 problems (SuperAdmin 37 pages/89 buttons ... Executive 5/8)
# fresh DB skci_qa_acc_mig1: migrate -> ModulesSeeder -> migrate:rollback --step=4 -> migrate: all DONE
# web (node:20-alpine, like the Dockerfile: npm ci --legacy-peer-deps): vue-tsc -b exit 0, npm run build "built in 2m 16s"
```
Note: plain `npm ci` fails with ERESOLVE (@sentry/vue peer pinia 2.x|3.x vs pinia 4). This is an existing condition, not from this round; the Dockerfile already uses `--legacy-peer-deps`.
