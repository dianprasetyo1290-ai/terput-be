# Handoff QA-IDN: identity, tahun ajaran/semester, menu 1–5, keamanan NFR

Source: UAT of 2026-10-14 (39 findings, `agent-tasks/QA-IDN.md`).
Branch `feat/QA-IDN`, base `b984872`.
Nothing was pushed or deployed, and no server was touched.

## 1. Status per BUG

Status values: FIXED, PARTIAL, DEFERRED (with a design in section 6), NOT-A-CODE-BUG.

| BUG | Sev | Status | Change | Main files | Evidence (test) |
|---|---|---|---|---|---|
| 005 | Kritis | NOT-A-CODE-BUG (server config) | Added an example edge nginx config with TLS 1.2/1.3, HTTP→HTTPS redirect, HSTS and security headers. Server steps are in section 5. At-rest encryption: TOTP secrets now use AES-GCM. Other PII is in section 6. | `docs/deploy/edge-nginx-tls-hsts.example.conf` | manual review only (server-side) |
| 012 | Tinggi | FIXED | Each unit now has `accounting_entity_code` and `settlement_bank_name/_account_number/_account_name`. The settlement account must be complete (all fields) with 5–20 digits. Only superadmin may change these fields (403 otherwise). | mig 0020, `handler/schools.go`, `rules/rules.go`, `SekolahView.vue` | `TestSchoolValidation`, `rules_test.go` |
| 013 | Tinggi | FIXED | `code` and `level` are required, and the code format is validated. The server returns 400 with field errors and the UI marks the fields with `*`. | `schools.go`, `rules.go`, `SekolahView.vue`, `IdentitasSekolahView.vue` | `TestSchoolValidation` |
| 014 | Tinggi | FIXED | Deactivating a unit that has active users returns 409 `{requires_confirmation, impact:{active_students,active_teachers,active_users}}`. It only proceeds with `confirm=true` and a `reason`, and it is audited. Data stays (it is only `is_active`). | `schools.go` (setSchoolActive), `SekolahView.vue` | `TestSchoolValidation` (deactivate step) |
| 015 | Tinggi | FIXED | `academic_years.locked_at/locked_by/lock_reason`. A locked year cannot be edited or deleted (409). `set-current` auto-locks years that ended before the new year. Lock is admin (not allowed on the current year). Unlock is superadmin only, with a reason of at least 10 characters, and is audited. The academic service rejects term edits and bootstrap for a locked year (409). | mig 0020, `academic_years.go`, academic `term.go`, `TahunAkademikView.vue` | `TestAcademicYearDatesAndLock`, academic `term_update_test.go` |
| 016 | Tinggi | FIXED | Duplicate detection on NISN, NIK and normalized phone when the value changes. The server returns 409 with the list of duplicates; names from other units are hidden. The user can save anyway with `confirm_duplicate=true`, which is audited. Added a NIK column. | mig 0020, `users.go` (checkDuplicates), `store/duplicates.go`, `PenggunaView.vue` | `TestUserValidationAndDuplicates` |
| 017 | Tinggi | FIXED | `user_school_roles.student_status`: calon/aktif/lulus/keluar, backfilled. `POST /students/{id}/status` and `GET /students/{id}/status-history`. Status `lulus` and `keluar` set the role to `inactive`, which also cuts off login. | mig 0020, `student_status.go`, `PenggunaView.vue` | `TestStudentStatusFlow` |
| 018 | Tinggi | FIXED | Import `dry_run=true` returns a preview `{total, valid, invalid, rows, errors[row,issue]}` and saves nothing. The UI shows the preview before saving. | `import_users.go`, `userimport/`, `ImportUsersDialog.vue` | `TestImportUsersFlow`, `userimport_test.go` |
| 019 | Tinggi | DEFERRED | Migration control totals and sign-off. Design in section 6. | – | – |
| 020 | Tinggi | FIXED | Every committed import is recorded in `import_batches` and `import_batch_items`. `GET /users/import-batches` lists them. `POST /users/import-batches/{id}/rollback` takes a reason and works all-or-nothing: it is refused (409, with the affected rows listed) if any account has logged in, holds another role, has a guardian link, or is managed by HR. Rolled-back accounts are soft-deleted and their emails released. | mig 0021, `import_users.go`, `ImportUsersDialog.vue` | `TestImportUsersFlow` |
| 021 | Tinggi | DEFERRED | Importing parents/relations and classes from the old system. Design in section 6. | – | – |
| 022 | Tinggi | FIXED | Sessions are stored in `user_sessions` (JWT `sid`). Delete, deactivate or password change ends all sessions. nginx `auth_request /_session_check` checks every `/api/<svc>/` call against identity, so an old token is rejected at once (401) in all Go services. | mig 0019, `session.go`, `web/nginx.conf` | `TestTokenRejectedAfterUserDeleted`, `nginx -t` |
| 023 | Tinggi | PARTIAL | `role_conflicts` table (15 seed pairs). Assigning a forbidden combination returns 409. Superadmin manages the pairs and can list existing violations (`/security/role-conflicts/violations`). **Not done:** the exception approval flow via core approval, and cleaning up existing accounts (for example the admin with 41 roles), which is an operational step. | mig 0019, `security_admin`/`users.go` | `TestAssignRoleRejectsForbiddenCombination` |
| 024 | Tinggi | FIXED | Removing or deactivating a role takes effect immediately: session-check rejects a token whose `active_role` is no longer active. | `session.go`, `nginx.conf` | `TestTokenRejectedAfterRoleRemoved` |
| 025 | Tinggi | FIXED | Account locks after 5 failed logins for 15 minutes (423 with a message). Idle timeout is 30 minutes on the server. Failed logins are written to `login_events` and the audit log. Admin unlock is available. The SPA also logs out on idle, because polling would otherwise keep the session alive. | mig 0019, `auth.go`, `useIdleLogout.ts` | `TestLockoutAfterFailedLogins`, `TestIdleTimeoutAndLogoutAll` |
| 045 | Tinggi | PARTIAL | Within identity, about 29 `err.Error()` leaks were replaced with `WriteDBError`/`SafeDBMessage`. Constraint names map to Indonesian field messages, and unique violations return 409. Field errors are shown in the UI. Other services are handled by their own QA agents. | `common/dberror.go`, identity handlers | `dberror_test.go` |
| 047 | Tinggi | FIXED | TOTP MFA (RFC 6238) is required for the roles in `security_policy.mfa_roles`: superadmin, finance_manager, payroll_officer, hr_manager. The superadmin can add roles via `PUT /security/policy`. First login runs enrollment (secret plus otpauth URL); later logins ask for a 6-digit code. Admin can reset MFA. | mig 0019, `auth.go`, `security/`, `LoginView.vue` | `TestMFAEnrollAndVerify` |
| 068 | Sedang | FIXED | `principal_name` and `principal_user_id` on the unit. | mig 0020, `schools.go`, both school views | `TestSchoolValidation` |
| 069 | Sedang | FIXED | Duplicate unit code returns 409 "Kode unit sudah dipakai" (field `code`). No SQL text is exposed. | `common/dberror.go` | `dberror_test.go`, `TestSchoolValidation` |
| 070 | Sedang | FIXED | `academic_years.start_date/end_date`. Required on create (defaults to 1 Jul–30 Jun from the label). End must be after start, maximum 2 years. Existing rows were backfilled. | mig 0020, `academic_years.go`, `TahunAkademikView.vue` | `TestAcademicYearDatesAndLock` |
| 071 | Sedang | FIXED | Semester `end_date` must be after `start_date` (academic `validateTermDates`, plus a CHECK NOT VALID in mig 0008). | academic `term_rules.go`, mig academic 0008 | `term_rules_test.go`, `term_update_test.go` |
| 072 | Sedang | FIXED | A semester must fall inside the academic-year range and must not overlap its sibling semester. If identity is unreachable the server returns 503 and saves nothing. | academic `term.go`, `client/identity.go` | `term_rules_test.go`, `term_update_test.go` |
| 073 | Sedang | DEFERRED | Reference master data (religion, parent occupation, program). Design in section 6. | – | – |
| 074 | Sedang | FIXED | NISN must be 10 digits and NIK 16 digits, otherwise 400 with field errors. DB CHECK constraints are NOT VALID. | `rules.go`, `users.go`, mig 0020 | `rules_test.go`, `TestUserValidationAndDuplicates` |
| 075 | Sedang | FIXED | Status transitions are controlled. `lulus` is final; only kepala_sekolah or superadmin can correct it, with `correction=true` and a reason of at least 10 characters. `keluar` requires a reason. History is kept and audited. | `rules.StudentTransition`, `student_status.go` | `rules_test.go`, `TestStudentStatusFlow` |
| 076 | Sedang | FIXED | Import accepts `.xlsx` and `.csv` (comma or semicolon, with or without BOM). Columns are found by header and aliases, in any order. A `mapping` JSON field lets the user map columns, and the UI shows a dropdown when a column is missing. | `userimport/`, `ImportUsersDialog.vue` | `userimport_test.go` |
| 077 | Sedang | PARTIAL | The "ID Sumber" column is stored in `import_batch_items.source_ref` (indexed), mapping the source ID to the new user ID. This covers user import only. Parents, classes and finance depend on BUG-021/019. | mig 0021, template | `TestImportUsersFlow` |
| 078 | Sedang | DEFERRED | Trial migration mode. Design in section 6. | – | – |
| 079 | Sedang | DEFERRED | Read-only historical archive. Design in section 6. | – | – |
| 080 | Sedang | FIXED | A missing required column (Nama, Email, Peran when the role is not fixed) rejects the whole file with 400 `missing_columns`. | `userimport.MapColumns` | `TestMapColumns`, `TestImportUsersFlow` |
| 081 | Sedang | FIXED | More than 1000 rows returns 400 ("pecah file"). The file size limit is 10 MB. Any invalid row returns 422 and saves nothing, unless the user explicitly chooses `skip_invalid=true`. Saving is one transaction. | `import_users.go` | `TestImportUsersFlow` |
| 082 | Sedang | DEFERRED | Configurable permission matrix per module and feature. Design in section 6. | – | – |
| 083 | Sedang | FIXED | `GET /auth/sessions`, `DELETE /auth/sessions/{sid}`, `POST /auth/sessions/revoke-all` and `GET /auth/login-history`. Admin variants are `/users/{id}/sessions` and `/users/{id}/login-history`. UI is on the "Ubah Password" page. | mig 0019, `session.go`, `AccountSecurityPanel.vue` | `TestIdleTimeoutAndLogoutAll` |
| 084 | Sedang | FIXED | Email format is validated on create (`rules.ValidEmail`) and on import. | `users.go`, `rules.go` | `TestUserValidationAndDuplicates`, `TestValidateRows` |
| 085 | Sedang | FIXED | Reset link: `forgot` always returns 200 (no enumeration). The token is single-use, stored as SHA-256, with a 30-minute TTL. `GET /auth/password/reset/{token}` returns 404 for an unknown link and 410 for a used or expired one, with a "minta tautan baru" message. A successful reset ends all sessions. The link is sent through the notification service (email). The UI has a "Lupa kata sandi?" option and a `/reset-password` page. | mig 0019, `password_reset.go`, `LoginView.vue`, `ResetPasswordView.vue` | `TestPasswordResetLink` |
| 119 | Sedang | DEFERRED | Data-subject rights (UU PDP). Design in section 6. NIK is now hidden from the directory view. | – | – |
| 120 | Sedang | DEFERRED | Break-glass access. Design in section 6. | – | – |
| 150 | Rendah | FIXED | `GET /academic-years` and `/academic-years/{id}` return 403 for `kepala_yayasan`. | `academic_years.go` | `TestAcademicYearDatesAndLock` |
| 151 | Rendah | FIXED | Unsupported formats get the message "Format file tidak didukung. Gunakan file .xlsx atau .csv sesuai template" (in Indonesian). | `userimport.ReadTable` | `TestReadTableFormats` |

Summary (39 findings):
- 27 FIXED
- 3 PARTIAL (023, 045, 077)
- 8 DEFERRED (019, 021, 073, 078, 079, 082, 119, 120)
- 1 NOT-A-CODE-BUG (005: an example config is in the repo; the server work is in section 5)

## 2. Commits (`git log b984872..HEAD`)

```
856fc20 feat(web): school master fields, deactivation impact confirm, academic-year dates and lock
b165523 feat(web): MFA login step, forgot/reset password, sessions, idle logout, import preview
fada9a0 fix(identity): user import xlsx/csv with header mapping, preview, batch rollback
e3e3931 feat(academic): validate semester dates against academic year and lock
33a9e65 feat(identity): master-data validation, academic period lock, student status
8e7e395 feat(web,deploy): nginx auth_request session check for all Go services; TLS/HSTS example
0aa9be7 feat(identity): sessions, token revocation, lockout, TOTP MFA, reset link, SoD
```

## 3. Migrations

Each migration was tested up → down 1 → up on `skci_qa_idn_identity` and `skci_qa_idn_academic`.

| Service | No. | Contents |
|---|---|---|
| identity | 0019_security_sessions | `security_policy` (1 row); `users.failed_login_count/locked_until/tokens_valid_after/mfa_secret_enc/mfa_enabled_at`; `user_sessions`; `login_events`; `mfa_challenges`; `password_reset_tokens`; `role_conflicts` (+15 seed pairs) |
| identity | 0020_master_data_validation | `schools`: principal_*, accounting_entity_code, settlement_*, plus a CHECK on the account number. `users.nik` with NISN/NIK CHECKs **NOT VALID** and indexes on nisn, nik and phone. `academic_years`: start/end dates (backfilled from the label), locked_*. `user_school_roles.student_status` (backfilled for siswa). `student_status_history` |
| identity | 0021_import_batches | `import_batches`, `import_batch_items` (with email and source_ref) |
| academic | 0008_term_dates_check | CHECK `end_date > start_date` on terms, **NOT VALID** (old bad data on the server, such as Semester 1 2027/2028, is not blocked) |

After deploy:
1. Clean up old data that breaks the rules (NISN that is not 10 digits, inverted semester dates).
2. Then run `ALTER TABLE … VALIDATE CONSTRAINT …`.

Example query to find bad NISN rows:

```sql
SELECT id, nisn FROM users WHERE nisn IS NOT NULL AND nisn !~ '^[0-9]{10}$';
```

## 4. New and changed endpoints (identity unless noted)

**Public**
- `POST /auth/mfa/verify`
- `GET /auth/session-check` (used by nginx auth_request)
- `POST /auth/password/forgot`
- `GET /auth/password/reset/{token}`
- `POST /auth/password/reset`

**JWT, own account**
- `POST /auth/logout`
- `GET /auth/sessions`
- `DELETE /auth/sessions/{sid}`
- `POST /auth/sessions/revoke-all {keep_current}`
- `GET /auth/login-history`

**Superadmin**
- `GET|PUT /security/policy`
- `GET|POST /security/role-conflicts`
- `DELETE /security/role-conflicts/{a}/{b}`
- `GET /security/role-conflicts/violations`

**Admin, user management**
- `GET /users/{id}/sessions`
- `POST /users/{id}/sessions/revoke-all`
- `GET /users/{id}/login-history`
- `POST /users/{id}/unlock`
- `POST /users/{id}/mfa/reset`

**Import**
- `POST /users/import` (multipart). Fields:
  - `file`
  - `role?`
  - `school_id?`
  - `mapping?`
  - `dry_run?`
  - `skip_invalid?`
- `GET /users/import-batches`
- `POST /users/import-batches/{id}/rollback {reason}`
- The template gains an "ID Sumber" column.

**Academic years**
- `GET /academic-years/{id}` (also callable with the internal key, for the academic service)
- `POST /academic-years/{id}/lock {reason}`
- `POST /academic-years/{id}/unlock {reason}`
- Create and update accept `start_date` and `end_date`.

**Students**
- `POST /students/{id}/status {status, reason, correction}`
- `GET /students/{id}/status-history`

**Other changes**
- `POST /schools/{id}/deactivate {confirm, reason}`: returns 409 with the impact when unconfirmed.
- `PUT /users/{id}` accepts `nik` and `confirm_duplicate`, and returns 409 `requires_confirmation`.
- Academic: `PUT /terms/{id}` validates dates and the year lock. `POST /academic-years/{id}/terms/bootstrap` rejects a locked year.

## 5. Server configuration steps (not done; operator approval needed)

1. **TLS/HSTS (BUG-005).**
   - Install a certificate on the edge box (Let's Encrypt/certbot, or the IDCH certificate) for the demo/production domain.
   - Use the blocks in `docs/deploy/edge-nginx-tls-hsts.example.conf`: 80 → 301 HTTPS, `ssl_protocols TLSv1.2 TLSv1.3`, HSTS, and security headers.
   - Start HSTS with a short `max-age` and only add `includeSubDomains`/`preload` once every subdomain is on HTTPS.
   - Open port 443 in the firewall.
   - Check with `nginx -t` and `curl -I https://…` (look for the `strict-transport-security` header), then run an SSL Labs scan.
2. **Identity environment variables:**
   - `PUBLIC_APP_URL`: the SPA base URL **including the path prefix**, for example `https://<domain>/teratai`. Reset links become `<PUBLIC_APP_URL>/reset-password?token=…`.
   - `MFA_ENCRYPTION_KEY`: a long random string (`openssl rand -base64 48`). It must never change once someone has enrolled. If it is empty, the key is derived from `SEKOCI_JWT_SECRET`, so rotating the JWT secret would break MFA.
   - `NOTIFICATION_URL` (already set in compose).
3. **Before deploy:**
   - Back up the identity and academic databases.
   - Run migrations identity 0019–0021 and academic 0008.
   - Rebuild identity, academic and web. `web/nginx.conf` now contains `auth_request`.
4. **Day-one operational effects:**
   - Accounts with MFA roles (superadmin and the others listed) are asked to enroll TOTP at their next login. Prepare an authenticator app for each.
   - Tokens issued before the deploy have no `sid`. They are still checked for a deleted account and for an inactive role, but they are not subject to the idle timeout or session revocation until they expire (at most 24 hours).
   - Option: `UPDATE users SET tokens_valid_after = now();` right after deploy forces everyone to log in again. **This is a PO/operator decision.**
   - `auth_request` makes identity a dependency of every API: if identity is down, other APIs fail too. Monitor identity.
   - Existing role violations (for example admin@teratai.sch.id holding 41 roles) are not removed automatically. Superadmin should review `GET /security/role-conflicts/violations` and remove the extra roles by hand.
5. **Sending reset links** needs the notification service's email channel to be active for the user's unit. Superadmin accounts with no unit cannot receive the email; another superadmin resets them with `POST /users/{id}/reset-password`.

## 6. Designs for DEFERRED items

- **BUG-019 (control totals and migration sign-off).**
  - Tables: `migration_runs(id, school_id, mode trial|production, status, created_by, signed_off_by, signed_off_at)` and `migration_control_totals(run_id, category, metric(count|sum_amount), source_value, target_value, diff, explanation)`.
  - Source values are entered by the operator or come from the file; target values are computed by each service through an internal endpoint (billing: outstanding amounts, academic: number of classes, and so on).
  - `POST /migration-runs/{id}/sign-off` is rejected (409) while any `diff ≠ 0` has no `explanation`.
  - Approval goes through the core approval center (QA-CORE).
- **BUG-021 (parents and classes from the old system).**
  - Extend `userimport` with sheet types: `orang_tua` (columns: Nama, Email/HP, ID Sumber Siswa, Hubungan) creates the account plus a `guardian_links` row via the existing student `source_ref`; `kelas` (Nama Kelas, Tingkat, Wali Kelas email, ID Sumber Siswa[]) calls the academic service's internal create/enroll endpoint.
  - Use the same preview/commit/batch flow, with `import_batch_items` extended by `entity_type`.
- **BUG-073 (reference master data).**
  - Table `reference_values(id, category religion|gender|parent_occupation|program, code, label, sort, active, school_id NULL=global)`, with CRUD for superadmin/admin.
  - Migrate `users.religion` free text by mapping it to a code, keeping the old text in `extra`.
  - Forms take dropdowns from `GET /references?category=`.
- **BUG-078 (trial migration).**
  - Use a separate environment: restore the database to a trial schema (`skci_trial_*`) on the same box, with an env flag `MIGRATION_TRIAL=true` shown as a UI banner, and imports with `migration_runs.mode=trial`.
  - Remove trial data with a database drop; do not mix it with production.
- **BUG-079 (read-only archive).**
  - Table `archive_records(id, school_id, category, source_ref, payload jsonb, imported_at)` with a GIN index on payload, `tsvector` search, and read-only endpoints `GET /archive?category=&q=`.
  - Per-category decisions are recorded in `migration_runs`.
- **BUG-082 (configurable permission matrix).**
  - Table `role_permissions(school_id NULL, role, module, feature, can_view, can_edit, can_approve)`, seeded from the current `meta.role` and `requireRole` values.
  - The identity session-check or `/me/permissions` sends the matrix to the SPA, and each service checks via a shared middleware (with caching).
  - Large scope: every service has to adopt it.
- **BUG-119 (UU PDP data-subject rights).**
  - Table `data_subject_requests(id, school_id, subject_user_id, type access|correction|deletion, status, requested_by, due_at = +3x24h per UU PDP, handled_by, resolution)`. Requests go through the approval center.
  - Access requests produce a JSON/PDF export per service through internal endpoints.
  - Deletion means anonymizing PII and keeping financial transactions (legal retention, [OQ-10]).
  - Mask NIK in PPDB (admission/QA-PPDB).
- **BUG-120 (break-glass).**
  - Table `break_glass_grants(id, user_id, role, school_id, reason, approved_by, starts_at, expires_at, revoked_at)`.
  - The grant creates a temporary `user_school_roles` row. A ticker job revokes it when `expires_at` passes, then calls `revokeAll`.
  - All audit events during the grant are tagged, and a summary is sent to the YA (kepala_yayasan) through notification when the grant ends.
- **BUG-023 (SoD exception, rest of the work).**
  - `role_conflict_exceptions(user_id, role_a, role_b, approval_id, expires_at)`. AssignUserRole accepts `exception_request=true`, which creates an approval in core, and the assignment is applied when the approval callback arrives.

## 7. Verification (real output, 2026-10-14)

```
$ cd services/identity && go vet ./... && TEST_PG_DSN=postgres://localhost/skci_qa_idn_identity?sslmode=disable go test ./... -count=1
identity vet OK
ok  	skci/identity/internal/common	0.405s
ok  	skci/identity/internal/handler	5.742s
ok  	skci/identity/internal/rules	0.340s
ok  	skci/identity/internal/security	0.338s
ok  	skci/identity/internal/store	0.620s
ok  	skci/identity/internal/userimport	0.397s
ok  	skci/identity/internal/whitelabel	0.537s

$ cd services/academic && go vet ./... && TEST_PG_DSN=postgres://localhost/skci_qa_idn_academic?sslmode=disable go test ./... -count=1
academic vet OK
ok  	skci/academic/internal/common	0.374s
ok  	skci/academic/internal/handler	0.578s
ok  	skci/academic/internal/model	0.351s
ok  	skci/academic/internal/store	0.472s

$ cd web && npm ci --legacy-peer-deps && npx vue-tsc -b && npm run build   (node:20-alpine in Docker)
vue-tsc EXIT=0
✓ built in 18.91s

migrate identity: 19/u 20/u 21/u ; down 1 ; up OK — academic: 8/u ; down 1 ; up OK
nginx -t (web/nginx.conf, docker nginx:alpine + --add-host for the upstreams): syntax is ok / test is successful
```

Notes:
- `npm ci` without `--legacy-peer-deps` fails with ERESOLVE (`@sentry/vue` vs `pinia@4`). This is pre-existing at base; the Dockerfile already uses `--legacy-peer-deps`.
- `gofmt -l` flags a few files that were already unformatted at base (`announcements.go`, `internal_conversion.go`, `model.go`, academic `client/identity.go`). They were left alone.

## 8. Assumptions and open questions

1. **MFA roles.** The PRD lists "Yayasan Admin, Finance Admin, Accounting". The system has no roles with exactly those names. The default covers superadmin, finance_manager, payroll_officer and hr_manager. Superadmin can add `kepala_yayasan` or an accounting role through `PUT /security/policy` without changing code. **The PO needs to confirm the mapping.**
2. **One platform-wide security policy.** Accounts are global across units, so the policy is a single row. Values follow the PRD: 5 failures, 15-minute lock, 30-minute idle timeout.
3. **Imported user passwords.** Imported users get a random password that is not shown, which is the existing behavior. They sign in through "Lupa kata sandi" or an admin reset. Consider adding an option to hand out credentials.
4. **Graduated students.** Changing a student to `lulus` or `keluar` also sets the siswa role to `inactive`, so the student can no longer log in. If alumni still need portal access (for example for transcripts), this needs a decision.
5. **Import rollback** deletes accounts (soft delete, with the email changed to `import-rollback-<id>@invalid.local`). It is refused entirely if even one account has been used.
6. **`kepala_yayasan` and academic years.** `kepala_yayasan` no longer has access to `/academic-years` (BUG-150). No `kepala_yayasan` page in the SPA was found calling that endpoint directly. If an executive page needs a year filter, use an internal path.
7. **The academic service needs identity** for term validation. When identity is unreachable it returns 503 instead of saving without validation.
8. **Locked years.** Only the academic-year and semester tables enforce the lock. Grades, attendance and other data from a locked year are not yet checked by their own services. Follow-up: have each service check `locked_at` via `GET /academic-years/{id}`.
9. **Client-side idle logout** uses user input (mouse, keyboard, touch, scroll) shared between tabs via localStorage. The server stays the final guard.
