# Handoff QA-WEB: web build config, shared layout, HR/Payroll/ATS/Training errors

Branch `feat/QA-WEB` (base b984872). No migrations were used; the reserved hr numbers 0031 and 0032 are still free.

## Bug status

| BUG | Status | Files | Evidence |
|---|---|---|---|
| BUG-046: API calls go to localhost:PORT in production | FIXED (server config also needs cleanup, see below) | `web/src/api/client.ts` (`serviceURL`/`serviceBaseURL`), `stores/auth.ts`, `services/admission/guardianClient.ts` (wrong dev port 8087 fixed), `pwa/branding.ts`, `web/scripts/check-dist.mjs`, `package.json`, `web/Dockerfile` | Production now defaults to `withAppBase('/api/<svc>')`. `check-dist` fails the build if `dist` contains `http(s)://localhost\|127.0.0.1\|0.0.0.0:<port>`: the old code fails it, the new code passes. |
| BUG-066: payroll/inputs and training/mandatory return 500 (`e.nip`, `pos.title`) | FIXED | `services/hr/internal/payroll/store.go`, `training/store.go` | `TestQAWeb_ListEndpointsAgainstSchema` (24 list endpoints against the real schema). Putting `e.nip` back makes the test fail. |
| BUG-067 + role sweep: payroll_officer, recruiter, training_admin, kepala_sekolah and finance_manager get 403 on their own modules | FIXED | `services/hr/internal/handler/router.go`, `common/auth.go` | `TestQAWeb_RoleMatrix` (31 role×endpoint cases). |
| BUG-149: superadmin without a school gets a 500 with an SQL uuid "" error; SQL errors leak to the client | FIXED | `common/auth.go` (`EffectiveSchoolID` honours `?school_id=` and validates the UUID), `common/response.go` (`InternalError`), 93 call sites in handler/* | `TestQAWeb_SuperadminWithoutSchoolIsBadRequestNotSQL` and `TestQAWeb_InternalErrorHidesDriverMessage`. |
| ESS /api/hr/me/* 404 for accounts without an employee row; guru/pegawai get 403 on attendance, training and catalog | FIXED | `ess.go`, router, `web/src/services/hr/ess.ts`, `training.service.ts`, `components/hr/EssNotEmployeeState.vue`, 7 ESS views | A 404 now shows "Akun Anda belum terdaftar sebagai pegawai". The role-matrix test covers the guru `/me/attendance` 404 and `/me/training/catalog` 200. |
| BUG-179: /api/identity/public/branding 404 | FIXED (code) | `services/identity/internal/whitelabel/handler.go`, `web/src/pwa/branding.ts` | `TestHandler_PublicBrandingWithoutBrandingIs200Null` |
| BUG-179: /api/identity/me 404 | NOT-A-CODE-BUG | none | No code calls a bare `/api/identity/me` (the endpoint is `/api/identity/auth/me`). The server is likely running an old bundle or identity build. Gets fixed by a rebuild and redeploy. |
| BUG-180: superadmin inbox returns 400 | FIXED | `web/src/stores/approvalBadge.ts` | The badge does not call the inbox for a superadmin without an `activeSchoolId`. |
| BUG-181: unit chips and year are hardcoded; /assets/* deep links 404 | FIXED | `AppLayout.vue`, `composables/useCurrentYear.ts`, `web/vite.config.ts` (`assetsDir: 'static'`), `web/nginx.conf` | Chips are removed and the year shows only the `is_current` value (else "Tahun ajaran belum diatur"). The bundle is under `/static/`, so `/assets/*` reaches the SPA fallback. |
| BUG-168: no data freshness indicator | PARTIAL | `components/dashboard/DashboardHero.vue` + Yayasan, identity admin, superadmin and insight dashboards | The finance dashboard belongs to the billing area and was not touched. |
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
- Freshness on the finance dashboard is left to the billing area (QA-BILL).
- An ESS account without an employee row is treated as a valid state (404 → empty-state UI), not as an error.
