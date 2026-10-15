# Handoff QA-XSVC: cross-service connections (QA-IDN §9 and QA-CORE §9)

Base `feat/QA-XSVC` @ `ed4677a` (from `integrasi/qa-fix`).
`feat/QA-XSVC` is checked out in another worktree, so the commits are on branch
`worktree-agent-ac946b534e5e51dee`. That branch is a direct continuation of `ed4677a`,
so `feat/QA-XSVC` can be fast-forwarded to it:
`git -C <wt QA-XSVC> merge --ff-only worktree-agent-ac946b534e5e51dee`.

Nothing was pushed or deployed, and no server was touched. `services/billing` was not touched.

## 1. Status

| Item | Status | Change | Main files | Evidence |
|---|---|---|---|---|
| IDN §9: request type `role_conflict_exception` in the Approval Center | FIXED | New request type `role_conflict_exception` ("Pengecualian Konflik Peran (SoD)") in `KnownRequestTypes`, listed in `GET /approval-request-types`. It may only be submitted by `source_service=identity` (`RequestTypeSources`/`SourceAllowed`); any other source gets 422. On approve/reject, core's existing generic callback mechanism POSTs `{approval_id, source_id, request_type, decision, decided_by, comment}` to the `callback_url` that identity sends (`{IDENTITY_INTERNAL_URL}/internal/role-conflict-exceptions/approval-callback`), with `X-Internal-Key` and `Idempotency-Key: approval:<id>`. Identity already handled this callback idempotently (QA-IDN). The demo seed flow is added. The web label falls back to a local value if the core list is not loaded. | core `model/domain.go`, `handler/internal.go`, `scripts/seed-approval-flows.sql`, `web/src/services/core/requestTypes.ts` | `TestRoleConflictExceptionApproval` (approve/reject/replay/maker≠approver/source pin, fake identity callback server), `TestSourceAllowed`; identity side: `TestSoDExceptionApprovalFlow` |
| IDN §9: role×kind matrix | NO CHANGE NEEDED | The matrix in `model/exception_access.go` covers *exception kinds*, not request types. A failed callback of this type becomes kind `callback`, which admin and superadmin can already handle. Approval itself goes through the flow steps (the approver role per flow). | — | `TestRoleConflictExceptionApproval` (callback_status done) |
| IDN §9: admission NIK masking | FIXED | `model.MaskNIK`: `3201********0001`, idempotent. Applied to the guardian applicant list (`GET /guardian/applicants`, `/guardian/dashboard`). The staff list/detail was already masked (BUG-121). The web staff view `ApplicationDetailView.vue` now calls `maskNik()` so even raw data is never shown. The guardian's own detail (`GET /guardian/applicants/{id}`, the edit form) stays full, following the identity pattern (full detail for the data owner). | admission `model/ppdb.go`, `handler/guardian.go`, `web/src/utils/format.ts`, `ApplicationDetailView.vue` | `TestMaskNIK`, `TestNIKMaskedInLists` |
| CORE §9: correlation middleware in identity, ops, hr | FIXED | MANIFEST `scripts/sync-go-common.sh` + identity/ops/hr (`correlation.go`, `exception_report.go`, plus tests). `CorrelationMiddleware` in the three routers. `X-Correlation-ID` is added to CORS allow/expose. Outgoing HTTP clients use `CorrelationTransport`: ops client, asset/budget/payable accounting clients, procurement jobs; hr client, payroll journal; identity integrations `httpClient`. Identity also has `StampCorrelationHeader` so the existing audit, which reads `r.Header`, gets the id. | `scripts/sync-go-common.sh`, `services/{identity,ops,hr}/internal/common/*`, three routers/main | `common/correlation_test.go` (per service), `TestStampCorrelationHeaderMintsID` |
| CORE §9: ExceptionReporter, identity | FIXED | A failed save of a user, orang_tua or kelas import (including academic classroom/enrol errors and the class history tx) is reported to core as kind `import`, key `import:identity:<jenis>:<unit>:<file>`, asynchronously, so the user response is not delayed. | `handler/import_report.go`, `import_users.go`, `import_kinds.go`, `cmd/server/main.go` | `TestReportImportFailure` |
| CORE §9: ExceptionReporter, ops | FIXED (+ latent bug) | Vendor sync to Accounting: after 5 attempts the row becomes `failed` and is reported as `dead_letter`, critical, key `dead_letter:ops:vendor_accounting_sync:<vendor>`. Bug fixed: a network error used to leave the row `pending` without counting attempts (infinite retry, never reported). It is now counted like a non-404 error. | `procurement/jobs.go`, `cmd/server/main.go` | `TestVendorSyncDeadLetter` (accounting 500 and network down) |
| CORE §9: ExceptionReporter, hr | FIXED | A failed payroll journal post is reported as kind `journal_posting`, critical, key `journal_posting:hr:<journal>`, summary `{journal_id, reference, period}`. | `payroll/journal.go`, `payroll/store.go`, `handler/router.go` | `TestReportJournalFailure` |
| Extra: web build blocked | FIXED | `RoleSwitcher.vue` had a duplicate `akuntan` key (left over from the QA-IDN merge), so vue-tsc failed with TS1117 and `npm run build` failed. The duplicate line was removed. | `web/src/components/identity/RoleSwitcher.vue` | vue-tsc/build below |
| Extra: ops procurement test cleanup | FIXED | `setupTestDB` cleanup did not drop the facility tables, so a second test in the package failed with "relation buildings already exists". The cleanup now drops them. | `ops/internal/procurement/handler_test.go` | ops procurement test ok |

Commits (7):
- `6baaac6` feat(core): role_conflict_exception
- `1ae1823` fix(admission): NIK masking
- `3ff1dae` chore(common): sync to identity/ops/hr
- `a7bf5a4` feat(identity)
- `e6aad2f` feat(ops)
- `6cf60ce` feat(hr)
- `97dce3e` fix(web): RoleSwitcher

## 2. New migrations and endpoints

- **Migrations:** none.
- **Endpoints:** no new endpoints.
  - `GET /approval-request-types` (core) now includes `role_conflict_exception`.
  - `POST /internal/approvals` (core) rejects `role_conflict_exception` from any source other than identity (422).
- **New outgoing calls:**
  - identity, ops and hr → core `POST /internal/exceptions` (X-Internal-Key, Idempotency-Key).
  - core → identity `POST /internal/role-conflict-exceptions/approval-callback`. This is the existing callback mechanism, with the URL taken from identity's request.

## 3. Verification (real output)

Test DBs: `skci_qa_xsvc_{core,identity,hr,admission,ops}` on localhost, migrated up, then down 1, then up.

```
===== core
ok  skci/core/internal/common | handler | model | servicedesk | store
===== identity
ok  skci/identity/internal/common | handler | rules | security | store | userimport | whitelabel
===== hr
ok  skci/hr/internal/ats/calc | client | common | crypto | handler | jobs | model | payroll | payroll/calc | training/calc
===== admission
ok  skci/admission/internal/client | conversion | handler | model | otp | simplepdf
== sync check
SYNC_OK            (scripts/sync-go-common.sh --check)
```
`go build ./...` and `go vet ./...` are clean for core, identity, hr, admission and ops.

ops tests apply migrations themselves and drop tables during cleanup, so `go test ./...` in parallel on one DB collides ("relation already exists"). This was already the case before this task. Each package was run on a fresh DB:
```
ok skci/ops/internal/asset        [empty db]
ok skci/ops/internal/budget       [empty db]
ok skci/ops/internal/campus3d     [empty db]
ok skci/ops/internal/common       [empty db]
ok skci/ops/internal/facility     [empty db]
ok skci/ops/internal/handler      [migrated db]
ok skci/ops/internal/inventory    [empty db]
ok skci/ops/internal/library      [empty db]
ok skci/ops/internal/payable      [empty db]
ok skci/ops/internal/pdfdoc       [empty db]
ok skci/ops/internal/procurement  [empty db]
ok skci/ops/internal/tax          [empty db]
ok skci/ops/internal/transport    [empty db]
```

Web (`npm ci --legacy-peer-deps`):
```
npx vue-tsc -b            -> OK (before the RoleSwitcher fix: TS1117 duplicate key)
npm run build             -> ✓ built in 11.32s; check-dist: no loopback URLs in dist
npm run audit:endpoints   -> 1281 calls, 1278 ok, 0 missing, 0 wrong method, 3 dynamic
```

## 4. Server configuration (not run)

1. **Approval flow per unit:** create the `role_conflict_exception` flow in the Approval Center (Pengaturan → Alur Persetujuan), for example step 1 kepala_yayasan, SLA 48 hours, escalation to superadmin. The demo option is `scripts/seed-approval-flows.sql` (flow 3). Without a flow, a submission from identity is refused by core, and identity marks the request `failed` with the reason.
2. **identity:**
   - `IDENTITY_INTERNAL_URL` must be reachable from core (default `http://identity:8081`), because it is the callback base.
   - `CORE_URL` must point to core.
   - `SEKOCI_INTERNAL_KEY` must match core.
3. **ops and hr:** `CORE_URL` and `SEKOCI_INTERNAL_KEY` are already required in production. Make sure core is reachable so exceptions get through.
4. **nginx/edge:** if a proxy filters headers, allow and forward `X-Correlation-ID`.

## 5. Assumptions and decisions (conservative)

- `role_conflict_exception` is pinned to source `identity`, so other services cannot open SoD exceptions.
- **No default flow in code:** core leaves flows to each school. Only the demo seed was added.
- **The web label** comes from core, with a local fallback.
- **NIK in the staff detail stays masked (BUG-121).** The task allowed full NIK in the detail view for authorized roles. Only the data owner (the guardian) sees it in full.
- **The ops vendor sync dead letter changes key and severity.** It used to be `vendor_sync_ex`/warning through `PostException`; it is now `dead_letter:ops:vendor_accounting_sync:<id>`/critical through the shared ExceptionReporter. Open items reported under the old key in production will not be deduplicated against the new key.
- **The hr `journal_posting` report has no `retry_url`.** Retries stay in the Jurnal Payroll menu, which already exists.
- The identity academic/exam clients do not receive correlation: their methods take no ctx. Changing the signatures was out of scope.

## 6. Open questions

- Should a vendor sync that hits 404 "accounting not ready" also stop at 5 attempts? It currently stops at `pending`, attempts 5, without a report (old behaviour, unchanged).
- Should the ops tests be made safe to run in parallel, for example with a per-package schema? This is outside the QA-XSVC scope.
- A live identity→core run across two processes was not done. The contract is tested on both sides (core with a fake identity, identity with a fake core).
