# Handoff QA-WEB: web build config, shared layout, HR/Payroll/ATS/Training errors

Branch `feat/QA-WEB` (base b984872). Round 1 used no migrations. Round 2 used hr `0031_payroll_engine` (up → down 1 → up OK on `skci_qa_web_hr`); hr 0032–0033 are still free. Quality needed no migration (its next free number is 0007).

## Bug status

| BUG | Status | Files | Evidence |
|---|---|---|---|
| BUG-046: API calls go to localhost:PORT in production | FIXED (server config also needs cleanup, see below) | `web/src/api/client.ts` (`serviceURL`/`serviceBaseURL`), `stores/auth.ts`, `services/admission/guardianClient.ts` (wrong dev port 8087 fixed), `pwa/branding.ts`, `web/scripts/check-dist.mjs`, `package.json`, `web/Dockerfile` | Production now defaults to `withAppBase('/api/<svc>')`. `check-dist` fails the build if `dist` contains `http(s)://localhost\|127.0.0.1\|0.0.0.0:<port>`: the old code fails it, the new code passes. |
| BUG-066: payroll/inputs and training/mandatory return 500 (`e.nip`, `pos.title`) | FIXED | `services/hr/internal/payroll/store.go`, `training/store.go` | `TestQAWeb_ListEndpointsAgainstSchema` (24 list endpoints against the real schema). Putting `e.nip` back makes the test fail. |
| BUG-067 + role sweep: payroll_officer, recruiter, training_admin, kepala_sekolah and finance_manager get 403 on their own modules | FIXED | `services/hr/internal/handler/router.go`, `common/auth.go` | `TestQAWeb_RoleMatrix` (31 role×endpoint cases). |
| BUG-149: superadmin without a school gets a 500 with an SQL uuid "" error; SQL errors leak to the client | FIXED | `common/auth.go` (`EffectiveSchoolID` honours `?school_id=` and validates the UUID), `common/response.go` (`InternalError`), 93 call sites in handler/* | `TestQAWeb_SuperadminWithoutSchoolIsBadRequestNotSQL` and `TestQAWeb_InternalErrorHidesDriverMessage`. |
| ESS /api/hr/me/* 404 for accounts without an employee row; guru/pegawai get 403 on attendance, training and catalog | FIXED | `ess.go`, router, `web/src/services/hr/ess.ts`, `training.service.ts`, `components/hr/EssNotEmployeeState.vue`, 7 ESS views | A 404 now shows "Akun Anda belum terdaftar sebagai pegawai". The role-matrix test covers the guru `/me/attendance` 404 and `/me/training/catalog` 200. |
| BUG-179: /api/identity/public/branding 404 | FIXED (code) | `services/identity/internal/whitelabel/handler.go`, `web/src/pwa/branding.ts` | `TestHandler_PublicBrandingWithoutBrandingIs200Null` |
| BUG-179: /api/identity/me 404 | FIXED | `web/src/stores/auth.ts` (e1e8b18) | `fetchMe` called the non-existent `/me`; it now calls `/auth/me`. Takes effect after a web rebuild and redeploy. |
| BUG-180: superadmin inbox returns 400 | FIXED | `web/src/stores/approvalBadge.ts` | The badge does not call the inbox for a superadmin without an `activeSchoolId`. |
| BUG-181: unit chips and year are hardcoded; /assets/* deep links 404 | FIXED | `AppLayout.vue`, `composables/useCurrentYear.ts`, `web/vite.config.ts` (`assetsDir: 'static'`), `web/nginx.conf` | Chips are removed and the year shows only the `is_current` value (else "Tahun ajaran belum diatur"). The bundle is under `/static/`, so `/assets/*` reaches the SPA fallback. |
| BUG-168: no data freshness indicator | FIXED (my scope) | `DashboardHero.vue` `updatedAt` + Yayasan, unit/admin/kepala sekolah, superadmin, insight, SDM (`generated_at`), ESS, wali kelas, siswa, guru, orang tua and Ujian Saya dashboards | Every dashboard in my scope shows "Diperbarui: <waktu>" after a successful load. The finance/payment dashboards belong to QA-BILL. |
| BUG-169: a11y labels | FIXED | `AppLayout.vue` (search), `PenggunaView.vue` (religion select) | aria-label |
| BUG-170: supported browsers not documented | FIXED | `docs/SUPPORTED_BROWSERS.md` | Chrome/Edge 111, Firefox 128, Safari/iOS 16.4; screen widths 375/768/1024/1440. |
| hr_admin gets 500 on /pelatihan/wajib and /payroll/input | FIXED | (BUG-066) | Role-matrix test, hr_admin cases |

## New endpoints (service hr)

- `GET /me/attendance`: attendance for the logged-in employee (404 if the user has no employee row).
- `GET /me/training/catalog`: published course catalog for all staff.
- `GET /me/training/courses/{courseId}/materials`: course materials for the employee.
- `GET /leave-types` and `GET /document-types`: open to every authenticated user (they were HR-only before).

## Role changes (router.go)

| Group | Roles | Used for |
|---|---|---|
| rolesHR | superadmin, hr_admin, hr_manager | employees, onboarding templates, offers create/submit/send, ATS settings, scorecard POST |
| rolesOrgRead | HR + kepala_sekolah | org-chart, departments, positions (GET) |
| rolesTrainingManage | superadmin, training_admin, hr_admin | write access to courses, assignments and mandatory |
| rolesTrainingRead | Manage + hr_manager + kepala_sekolah | read access to training and compliance |
| rolesRequisitionWrite / Read | HR + kepala_sekolah / + recruiter | requisitions |
| rolesRecruiting / CandidateRead | HR + recruiter / + kepala_sekolah | postings, applications, talent pool |
| rolesPayrollOperate / Read | HR + payroll_officer / + finance_manager | payroll inputs and runs / components, runs, payslips, journals (read) |

Admin `ListMaterials` is now school-scoped through GetCourse.

## Verification

- `services/hr`: `go vet ./...` is clean. `TEST_PG_DSN=postgres://localhost/skci_qa_web_hr go test -count=1 ./...` passes all packages (ats/calc, client, crypto, handler, jobs, model, payroll/calc, training/calc).
- `services/identity`: `go vet ./...` is clean and `go test -count=1 ./...` passes (common, handler, store, whitelabel).
- hr migrations on `skci_qa_web_hr`: up → down 1 (30/d training) → up (30/u training) all OK.
- `web`: `npx vue-tsc -b` is clean. `npm run build` succeeds and prints `check-dist: no loopback URLs in dist`. The only warning is the existing chunk-size warning.

## Server config steps (for whoever deploys)

1. Remove every `VITE_*_URL=http://localhost:...` from the server's web build env (or leave them unset). Without them, the build uses `/api/<svc>` under `VITE_BASE`. Rebuild web with `VITE_BASE=/teratai/`. If the env is still wrong, `check-dist` will fail the build.
2. On the edge/host proxy, remove any `location /assets/` special case. The bundle is now under `/teratai/static/`; make sure that path reaches the web container.
3. Redeploy identity so that `public/branding` returns 200 `branding:null`. Rebuild web, because the old bundle is what calls `/api/identity/me`.
4. Data: make sure each school has exactly one academic year with `is_current = true`. Otherwise the sidebar shows "Tahun ajaran belum diatur".

## Assumptions and open questions

- Following the PRD T5 RBAC, hr_manager can now only read training (write access is limited to superadmin, training_admin and hr_admin). If hr_manager should also be able to manage training, add it to `rolesTrainingManage`.
- finance_manager can read payroll components, runs, payslips and journals but not inputs. Payroll operations stay with HR and payroll_officer.
- Freshness on the finance/payment dashboards is left to the billing area (QA-BILL).
- An ESS account without an employee row is treated as a valid state (404 → empty-state UI), not as an error.

## Putaran 2

Scope: every module not owned by another agent (HR/payroll/ATS/training/ESS, academic, the student/teacher/walikelas/pengawas/parent portals, ops, insight, quality, studentlife, report/dapodik, dashboards). Goal: no dead buttons, and every button calls an existing endpoint with a role the backend allows.

### Status

| Item | Status | Evidence |
|---|---|---|
| Web calls without a backend route | FIXED | `npm run audit:endpoints`: 1150 calls, 1147 ok, 0 wrong method, **2 missing**. Both missing calls are core approvals (`GET /approvals/source/{svc}/{id}`, `POST /approvals/{id}/decide`) and belong to **QA-CORE**. Before this round there were 21 (19 in quality). |
| Endpoint and role audit scripts | DONE | `web/scripts/audit-endpoints.mjs` (`npm run audit:endpoints`). `web/scripts/audit-roles.mjs` regenerates `scripts/role-matrix.json`, which maps each SPA route and its roles to the backend calls its view makes. |
| Payroll: mock calculation, missing statutory/tax/PDF/journal features | FIXED | b57d0c4 + 746ce22. The real engine uses encrypted salary, components/inputs and effective-dated BPJS/TER tables, with the Pasal 17 December/exit reconciliation. Missing parameters return 422, unmapped components 422, and a duplicate regular run 409. Payslip PDFs carry a sha256. The balanced accrual journal goes to accounting, and there is a journal retry. Also: tax reports (PPh21 monthly/annual, BPJS, reconciliation), remittances and component account mapping. Test: `TestPayrollEngineE2E`. |
| ATS offer salary stored in plain text | FIXED | 7f76e91: encrypted like employee salaries. |
| ATS "Unduh Berkas CV" did nothing; the public form asked for a file *name* | FIXED | fae7405 + b401398. Public apply takes multipart with a PDF CV (max 5 MB, `%PDF-` magic). The object key is assigned by the server, and a client-sent `cv_object_key` is ignored. `GET /ats/applications/{id}/cv` is for candidate readers. Test: `TestATS_CVUploadAndDownload` (non-PDF 400, download 200, no CV 404, pegawai 403). |
| SDM dashboard mock data; /dashboard landing | FIXED | d2ac24a: `GET /dashboard` (hr) gives real data, and `/dashboard` redirects each role to its own landing page. |
| Header bell hardcoded PPDB items | FIXED | 4879933: real unread count and latest notifications. |
| Ekskul member status and bookable rooms called wrong routes | FIXED | ce9dde2 |
| Raw SQL/driver text in error bodies (ops, academic, attendance, evaluation, exam, insight, quality, report, studentlife, hr) | FIXED | ac32afe. d1900cf: transport allocation shows the real error instead of 25P02. |
| Quality: 19 missing endpoints, fake import/reports, school-scope 500s | FIXED | eb17a1e + 9ff60a3 + 0de6cd4, details below. Test: `TestQAEndpointsE2E`. |
| Ops purchase-order and stock-opname PDFs were corrupt (wrong /Length and xref, unescaped text, capped at 25 lines) | FIXED | 0d17e2b: new `internal/pdfdoc` writer with pagination. Test: `TestBuildIsWellFormed`. |
| Parent "Unduh Dokumen" on a service request did nothing | FIXED | b401398: resolves the linked ticket (`/service-desk/service-requests/{id}/ticket`) and downloads `/me/tickets/{ticket}/files/{key}` with the JWT. |
| BUG-168 freshness | FIXED (scope) | See table above. |

### Quality / gov reporting (services/quality)

- **Bug fix:** queries used `(school_id = $2 OR $2 = '')` against a uuid column, so every GetInstrument/GetSelfEvaluation/GetTemplate/GetIndicator call returned 500 (`invalid input syntax for type uuid: ""`). They now use `school_id::text = $2`. Routes that need a school sit behind `requireSchool`, which returns 400 when a superadmin has picked no school.
- **New endpoints:**
  - instrument `new-version` (copies the whole tree)
  - standard, item and indicator PUT/DELETE, plus POST under the instrument
  - evidence `DELETE`, `new-version` and `download`
  - `refresh-metrics` (pulls insight metrics; unavailable ones are skipped and reported in `X-Refresh-Message`)
  - `reports/self-evaluation/{id}` (JSON or `?format=xlsx`)
  - action-plan PUT/DELETE
  - report-template GET/PUT/DELETE (409 while a draft/pending/approved report uses the template)
- **Real implementations:**
  - XLSX instrument import, with dry-run validation; a real run creates a draft instrument in one transaction.
  - Accreditation report: weighted standard scores from the latest EDS, plus completeness.
  - Comparison report: latest finalized EDS of the same instrument code at every foundation school.
  - The Approval Center callback for `regulatory_report` now actually updates the report status (it was a no-op).
- **Integrity rules:**
  - Instrument structure can't be edited once an EDS using it has left draft (409); the fix is "Versi baru".
  - Indicators are soft-deleted.
  - Locked evidence can't be deleted.
- **RBAC** (`internal/handler/router.go`):
  - write = qa_coordinator, admin
  - manage (targets, EDS, create RTL) = + kepala_sekolah
  - read = + kepala_yayasan, auditor
  - verify evidence = qa_coordinator, admin
  - comparison = kepala_yayasan (+ superadmin)
  - Dapodik = operator_dapodik, admin
  - gov templates/reports: write = admin; read = admin, kepala_sekolah, kepala_yayasan; template list is also open to operator_dapodik
  - Evidence upload and RTL list/update/status are open to every authenticated user. Ownership is checked in the handler: non-managers only see and change RTL items they own; evidence is changed by its uploader or a verifier.
- **Web:**
  - `QaReportsView` drops all its mock numbers and rows, and shows the comparison tab only to kepala_yayasan/superadmin.
  - Instrument and editor write buttons are hidden for read-only roles.
  - Verify is shown to coordinator/admin, and delete to the uploader or a verifier.
  - Regulatory create/submit/mark-submitted are shown to admin only.
  - The RTL route is opened to guru, walikelas and pegawai.
  - Errors use `apiErrorMessage`. Blob downloads read the server message through `blobOrError` / `UserFacingError`.

### Button audit method

- Ran a static scan of in-scope `views/**` for `<Button>`/`<button>` with no `@click`, `type=submit` or link wrapper. The remaining hits are either disabled status badges (ESS "Sudah Terdaftar") or buttons inside a clickable row whose row handler fires (quality "Buka"/"Lihat Perbedaan", calendar "Rincian").
- Grepped for "segera hadir"/"coming soon": only legitimate empty states remain (3D model not uploaded, sensor data, score not yet final).

### Config / decisions

- `ACCOUNTING_URL` (hr): payroll journals post to `{ACCOUNTING_URL}/api/internal/journals` (X-Internal-Key). If it is empty, the journal stays pending with "integrasi akuntansi belum dikonfigurasi" and can be retried from Jurnal Payroll. The variable is added to `docker-compose.yml` only; production .env was not touched.
- The TER tables in hr 0031 are marked **PERLU VALIDASI KONSULTAN PAJAK** (PMK 168/2023 transcription).
- Kepala sekolah gets read-only access to QA instruments (PRD). Foundation comparison is restricted to kepala_yayasan and superadmin.
- `kepala_sekolah`/`kepala_yayasan` are checked against `active_role` (same as other services). A principal must switch roles to reach role-gated QA actions.

### Verification (round 2)

- `services/quality`: `go vet ./...` is clean. `QUALITY_TEST_DATABASE_URL=…/skci_qa_web_quality go test ./...` passes (`TestQAEndpointsE2E`).
- `services/hr`: `go vet ./...` is clean. `TEST_PG_DSN=…/skci_qa_web_hr go test -count=1 ./...` passes. Migration 0031 up → down 1 → up is OK.
- `services/ops`: `go vet ./...` is clean and `go test ./...` passes.
- `web`: `npx vue-tsc -b` is clean. `npm run build` succeeds with `check-dist: no loopback URLs`. `npm run audit:endpoints` reports 2 missing calls (QA-CORE).

### For other agents

- **QA-CORE:** implement `GET /approvals/source/{service}/{sourceId}` and `POST /approvals/{id}/decide` in core (called from `web/src/services/core/approvals.ts`), or change the web calls to the existing routes.
