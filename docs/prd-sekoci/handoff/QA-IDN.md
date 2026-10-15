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
| 019 | Tinggi | FIXED (putaran 2) | Migration runs (`migration_runs`): trial or production. A production run requires a signed-off trial for the same unit. Control totals per category (`migration_control_totals`, generated `diff`). Sekoci-side counts are recomputed automatically for siswa/guru/orang_tua/relasi_ortu/kelas/anggota_kelas, and the operator enters the source figure. Every difference needs an explanation (≥10 characters) before sign-off. Sign-off is maker-checker: the run's creator cannot sign it (409). Runs can be discarded with a reason. UI: Manajemen → Migrasi Data Lama. | mig 0024, `migration_runs.go`, `MigrasiView.vue` | `TestMigrationRunTrialSignOffAndImports`, `TestUnexplainedDifferences`, `TestValidateTotalInput` |
| 020 | Tinggi | FIXED | Every committed import is recorded in `import_batches` and `import_batch_items`. `GET /users/import-batches` lists them. `POST /users/import-batches/{id}/rollback` takes a reason and works all-or-nothing: it is refused (409, with the affected rows listed) if any account has logged in, holds another role, has a guardian link, or is managed by HR. Rolled-back accounts are soft-deleted and their emails released. | mig 0021, `import_users.go`, `ImportUsersDialog.vue` | `TestImportUsersFlow` |
| 021 | Tinggi | FIXED (putaran 2) | Import `kind=orang_tua`: creates or reuses the parent account, adds the `orang_tua` role (SoD checked) and creates the guardian link to the student. The student is resolved by ID Sumber (from the source map) or NISN. All rows for one parent are rejected together if any row is bad. Rollback removes the links, the granted roles and the new accounts. Import `kind=kelas`: classroom and enrolment through academic (`FindOrCreateClassroom`/`EnrollStudent`). Wali kelas must be an active guru. It is safe to rerun after a 502, and returns 503 "belum dikonfigurasi" without an academic client. Templates: `/users/import-template?kind=`. | mig 0024, `import_kinds.go`, `import_users.go`, `MigrasiView.vue` | `TestMigrationRunTrialSignOffAndImports`, `TestValidateGuardianRows`, `TestValidateClassRows`, `TestMapKindColumns` |
| 022 | Tinggi | FIXED | Sessions are stored in `user_sessions` (JWT `sid`). Delete, deactivate or password change ends all sessions. nginx `auth_request /_session_check` checks every `/api/<svc>/` call against identity, so an old token is rejected at once (401) in all Go services. | mig 0019, `session.go`, `web/nginx.conf` | `TestTokenRejectedAfterUserDeleted`, `nginx -t` |
| 023 | Tinggi | FIXED (putaran 2) | A forbidden combination returns 409 `{exception_allowed}`. The UI (Data Pengguna/Data Peran) then offers "Ajukan pengecualian" (reason ≥20 characters, optional end date). The request goes to the core Approval Center (`request_type=role_conflict_exception`), and the role is granted only after the approved callback. If core is unreachable or refuses, the status becomes `failed` and shows the reason; the request can be revoked and filed again. Exceptions can be revoked. Clean-up: `POST /security/role-conflicts/violations/resolve` (superadmin) runs the same clean-up as migration 0022 (which already ran it once on deploy): for each violating account (for example the admin with 41 roles), the role involved in the most conflicts is removed repeatedly until no unexcepted pair is left. Each removed role is written to `role_sod_cleanup_log` and can be restored manually. UI: Pengaturan → Keamanan & Tata Kelola. | mig 0022, `sod_exceptions.go`, `KeamananView.vue`, `SoDExceptionDialog.vue` | `TestSoDExceptionApprovalFlow`, `TestSoDResolveViolationsCleanup`, `TestWithoutExcepted` |
| 024 | Tinggi | FIXED | Removing or deactivating a role takes effect immediately: session-check rejects a token whose `active_role` is no longer active. | `session.go`, `nginx.conf` | `TestTokenRejectedAfterRoleRemoved` |
| 025 | Tinggi | FIXED | Account locks after 5 failed logins for 15 minutes (423 with a message). Idle timeout is 30 minutes on the server. Failed logins are written to `login_events` and the audit log. Admin unlock is available. The SPA also logs out on idle, because polling would otherwise keep the session alive. | mig 0019, `auth.go`, `useIdleLogout.ts` | `TestLockoutAfterFailedLogins`, `TestIdleTimeoutAndLogoutAll` |
| 045 | Tinggi | FIXED (putaran 2) | Identity no longer sends any raw or English error text: all `WriteError`/`WriteDBError` messages are in Indonesian (`requireRole` → "Akses ditolak: hanya untuk peran …", token messages, claim, import upload). In the web, every identity view and Tahun Akademik shows the server's message through `utils/apiError.ts` instead of a generic error. Other services are handled by their own QA agents. | identity handlers, `common/auth.go`, `utils/apiError.ts`, identity views | `go test ./...` (identity), `dberror_test.go` |
| 047 | Tinggi | FIXED | TOTP MFA (RFC 6238) is required for the roles in `security_policy.mfa_roles`: superadmin, finance_manager, payroll_officer, hr_manager. The superadmin can add roles via `PUT /security/policy`. First login runs enrollment (secret plus otpauth URL); later logins ask for a 6-digit code. Admin can reset MFA. | mig 0019, `auth.go`, `security/`, `LoginView.vue` | `TestMFAEnrollAndVerify` |
| 068 | Sedang | FIXED | `principal_name` and `principal_user_id` on the unit. | mig 0020, `schools.go`, both school views | `TestSchoolValidation` |
| 069 | Sedang | FIXED | Duplicate unit code returns 409 "Kode unit sudah dipakai" (field `code`). No SQL text is exposed. | `common/dberror.go` | `dberror_test.go`, `TestSchoolValidation` |
| 070 | Sedang | FIXED | `academic_years.start_date/end_date`. Required on create (defaults to 1 Jul–30 Jun from the label). End must be after start, maximum 2 years. Existing rows were backfilled. | mig 0020, `academic_years.go`, `TahunAkademikView.vue` | `TestAcademicYearDatesAndLock` |
| 071 | Sedang | FIXED | Semester `end_date` must be after `start_date` (academic `validateTermDates`, plus a CHECK NOT VALID in mig 0008). | academic `term_rules.go`, mig academic 0008 | `term_rules_test.go`, `term_update_test.go` |
| 072 | Sedang | FIXED | A semester must fall inside the academic-year range and must not overlap its sibling semester. If identity is unreachable the server returns 503 and saves nothing. | academic `term.go`, `client/identity.go` | `term_rules_test.go`, `term_update_test.go` |
| 073 | Sedang | FIXED (putaran 2) | `reference_values` (agama, jenis_kelamin, pekerjaan_ortu, program): national seed plus unit-specific items, which can be switched on or off. Only superadmin may change national items. Religion on a user must come from the list (400 otherwise). The Agama dropdown in Data Pengguna reads from it. UI: Manajemen → Data Referensi. | mig 0022, `references.go`, `ReferensiView.vue`, `PenggunaView.vue` | `TestReferencesAndReligionValidation`, `TestValidateReference` |
| 074 | Sedang | FIXED | NISN must be 10 digits and NIK 16 digits, otherwise 400 with field errors. DB CHECK constraints are NOT VALID. | `rules.go`, `users.go`, mig 0020 | `rules_test.go`, `TestUserValidationAndDuplicates` |
| 075 | Sedang | FIXED | Status transitions are controlled. `lulus` is final; only kepala_sekolah or superadmin can correct it, with `correction=true` and a reason of at least 10 characters. `keluar` requires a reason. History is kept and audited. | `rules.StudentTransition`, `student_status.go` | `rules_test.go`, `TestStudentStatusFlow` |
| 076 | Sedang | FIXED | Import accepts `.xlsx` and `.csv` (comma or semicolon, with or without BOM). Columns are found by header and aliases, in any order. A `mapping` JSON field lets the user map columns, and the UI shows a dropdown when a column is missing. | `userimport/`, `ImportUsersDialog.vue` | `userimport_test.go` |
| 077 | Sedang | FIXED (putaran 2) | `source_id_map (school, entity_type, source_ref) → entity_id` for siswa, guru, orang_tua and kelas. An ID Sumber that has already been migrated is rejected (no duplicates on re-import). Parent and class imports resolve students through it. Finance data belongs to the billing/accounting agents (contract in Putaran 2). | mig 0024, `import_users.go` (`rejectMigratedRefs`), `import_kinds.go` | `TestMigrationRunTrialSignOffAndImports` |
| 078 | Sedang | FIXED (putaran 2) | Trial mode: every import (users, orang_tua, kelas, arsip) in a trial run is validated and counted, then rolled back. The batch is recorded with `trial=true` and `category_counts`. Trial students can be referenced by later trial files (`trial_refs`). | mig 0024, `import_users.go` (`commitImport`), `MigrasiView.vue` | `TestMigrationRunTrialSignOffAndImports` |
| 079 | Sedang | FIXED (putaran 2) | `archive_records` is read-only: a trigger blocks UPDATE and DELETE. Rows are imported from any xlsx/csv per category (up to 5000 rows) and searched through full-text search. Viewers: admin, kepala_sekolah, kepala_yayasan and superadmin. UI: Arsip Data Lama, with import from Migrasi Data Lama. | mig 0024, `archive.go`, `ArsipView.vue` | `TestArchiveRows`, `TestMigrationRunTrialSignOffAndImports` |
| 080 | Sedang | FIXED | A missing required column (Nama, Email, Peran when the role is not fixed) rejects the whole file with 400 `missing_columns`. | `userimport.MapColumns` | `TestMapColumns`, `TestImportUsersFlow` |
| 081 | Sedang | FIXED | More than 1000 rows returns 400 ("pecah file"). The file size limit is 10 MB. Any invalid row returns 422 and saves nothing, unless the user explicitly chooses `skip_invalid=true`. Saving is one transaction. | `import_users.go` | `TestImportUsersFlow` |
| 082 | Sedang | FIXED (putaran 2) | Permission matrix per module and action, with defaults in code and overrides per role, global or per unit (`role_permissions`). Identity enforces it through `a.permit` (announcements, guardians, privacy_requests, …). `GET /me/permissions` drives the UI buttons, and `GET /internal/permissions` is available for other services. UI: Keamanan → Matriks Hak Akses (superadmin). | mig 0022, `permissions.go`, `KeamananView.vue`, `InformasiKelolaView.vue` | `TestPermissionMatrixOverridesDefaults`, `TestValidatePermissionUpdate` |
| 083 | Sedang | FIXED | `GET /auth/sessions`, `DELETE /auth/sessions/{sid}`, `POST /auth/sessions/revoke-all` and `GET /auth/login-history`. Admin variants are `/users/{id}/sessions` and `/users/{id}/login-history`. UI is on the "Ubah Password" page. | mig 0019, `session.go`, `AccountSecurityPanel.vue` | `TestIdleTimeoutAndLogoutAll` |
| 084 | Sedang | FIXED | Email format is validated on create (`rules.ValidEmail`) and on import. | `users.go`, `rules.go` | `TestUserValidationAndDuplicates`, `TestValidateRows` |
| 085 | Sedang | FIXED | Reset link: `forgot` always returns 200 (no enumeration). The token is single-use, stored as SHA-256, with a 30-minute TTL. `GET /auth/password/reset/{token}` returns 404 for an unknown link and 410 for a used or expired one, with a "minta tautan baru" message. A successful reset ends all sessions. The link is sent through the notification service (email). The UI has a "Lupa kata sandi?" option and a `/reset-password` page. | mig 0019, `password_reset.go`, `LoginView.vue`, `ResetPasswordView.vue` | `TestPasswordResetLink` |
| 119 | Sedang | FIXED (putaran 2) | UU PDP requests (`data_subject_requests`): access, correction or deletion, for oneself or a linked child. The due date is 3×24 hours and is flagged when overdue. The admin processes the request: access produces a JSON export, correction applies the requested fields, deletion anonymises the account (with explicit confirmation). Data that must be kept by law is retained. Everything is audited. UI: Akun → Data Pribadi Saya (all roles); Manajemen → Permintaan Data Pribadi. | mig 0022, `privacy.go`, `DataPribadiView.vue`, `DataPribadiKelolaView.vue` | `TestDataSubjectRequestFlow`, `TestValidateCorrections` |
| 120 | Sedang | FIXED (putaran 2) | Break-glass (`break_glass_grants`): superadmin grants a temporary role for 15–1440 minutes, with a reason ≥20 characters and an incident number. It cannot be given to oneself or as superadmin. The session-check honours the expiry. A summary is sent to kepala_yayasan through notification and can be resent. The grant can be ended early. UI: Pengaturan → Akses Darurat (superadmin, kepala_yayasan). | mig 0022, `break_glass.go`, `BreakGlassView.vue` | `TestBreakGlassGrantAndExpiry`, `TestValidateBreakGlass` |
| 150 | Rendah | FIXED | `GET /academic-years` and `/academic-years/{id}` return 403 for `kepala_yayasan`. | `academic_years.go` | `TestAcademicYearDatesAndLock` |
| 089 | Sedang | FIXED (putaran 2) | Announcements can target units (`target_school_ids`, superadmin) and classes (`target_class_ids`, max 50). Class membership comes from academic `GET /internal/user-classrooms` and reaches siswa, their parents and the wali kelas. Every role can be targeted, including orang_tua. | mig 0023, `announcements.go`, academic `user_classrooms.go`, `InformasiKelolaView.vue` | `TestAnnouncementTargetingAndSchedule`, `TestAnnouncementVisibleTo`, academic `TestInternalUserClassrooms` |
| 153 | Sedang | FIXED (putaran 2) | An audience is mandatory: either `audience_all` or at least one role (422 otherwise). Publishing immediately requires `confirm_send_now`; without it the server returns 409 `requires_confirmation` and the UI asks "Kirim sekarang?". | `announcements.go`, `InformasiKelolaView.vue` | `TestValidateAnnouncementRequest`, `TestNeedsSendNowConfirmation` |
| 154 | Sedang | FIXED (putaran 2) | Scheduled publishing with `publish_at` (up to 1 year ahead). Status is draf, terjadwal or terbit, and an announcement is only visible after `publish_at`. | mig 0023, `announcements.go`, `InformasiKelolaView.vue` | `TestAnnouncementTargetingAndSchedule` |
| 151 | Rendah | FIXED | Unsupported formats get the message "Format file tidak didukung. Gunakan file .xlsx atau .csv sesuai template" (in Indonesian). | `userimport.ReadTable` | `TestReadTableFormats` |

Summary after putaran 2 (42 findings, including 089/153/154):
- 41 FIXED
- 0 PARTIAL
- 0 DEFERRED
- 1 NOT-A-CODE-BUG (005: an example config is in the repo; the server work is in section 5)

Section 6 (designs) is kept for history; every design there has now been implemented, as described in section 9.

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
| identity | 0022_governance | `role_conflict_exceptions`, `role_sod_cleanup_log` (and a one-time SoD clean-up), `break_glass_grants`, `data_subject_requests`, `role_permissions` (overrides), `reference_values` (+ national seed); adds role `akuntan` to the role CHECK |
| identity | 0023_announcement_targeting | `announcements.audience_all/target_school_ids/target_class_ids/publish_at` (existing rows: audience_all = no target_roles) |
| identity | 0024_migration_tools | `migration_runs`, `migration_control_totals`, `source_id_map`, `archive_records` (read-only trigger, GIN tsvector); `import_batches.kind/run_id/trial/category_counts/trial_refs`; `import_batch_items.entity_type/entity_id/created_account` (PK now includes entity_type) |
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

## 9. Putaran 2 (2026-10-15)

All items that were DEFERRED or PARTIAL are now FIXED (see section 1). BUG-089/153/154 (announcements) were added.

### Commits

```
3ec81e4 fix(web): tampilkan pesan server, bukan galat generik, di view identity dan Tahun Akademik (BUG-045)
e2998ee feat(web): Kelola Informasi — audiens wajib, target unit/kelas, jadwal terbit, konfirmasi kirim sekarang (BUG-089/153/154)
862a079 feat(web): ajukan pengecualian SoD saat peran ditolak; Agama dari Data Referensi (BUG-023/073)
f81af60 feat(web): halaman tata kelola identity — keamanan/SoD, akses darurat, data pribadi (UU PDP), referensi, migrasi data lama, arsip
7ffa29d fix(identity): keuangan/pengawas membaca siswa unitnya; GET /internal/users per peran
fee979b feat(web): peran akuntan di tipe Role, pemilih peran dan pengalihan dasbor
75dfeb3 fix(identity): tidak ada galat mentah/berbahasa Inggris ke pengguna (BUG-045)
9710f2e fix(identity): migrasi data lama — percobaan, control total + sign-off, impor orang tua/kelas, peta ID sumber, arsip read-only (BUG-019/021/077/078/079)
12c3e8f feat(identity): announcements require an audience, target units/classes and can be scheduled
a8278bb feat(identity): SoD exceptions via core, SoD clean-up, break-glass, UU PDP requests, references, permission matrix, akuntan role
e19b01f fix(academic): internal user-classrooms lookup for class-targeted announcements (BUG-089)
```

### Migrations

- identity 0022, 0023 and 0024: each tested up → down 1 → up.
- academic needs no new migration. `GET /internal/user-classrooms` only reads existing tables.

### New web pages (identity.routes.ts; navRoles match the backend gates)

| Path | Roles | Purpose |
|---|---|---|
| `/keamanan` | superadmin, admin | Pengecualian SoD and Pelanggaran tabs (admin+superadmin); Log Pembersihan, Bersihkan pelanggaran and Matriks Hak Akses (superadmin only) |
| `/keamanan/akses-darurat` | superadmin, kepala_yayasan | Break-glass: grant, end and resend summary (superadmin); list (kepala_yayasan) |
| `/data-pribadi` | every role | My UU PDP requests; orang tua can file for a linked child |
| `/data-pribadi/kelola` | admin, superadmin | Process, finish or reject requests; export download |
| `/manajemen/referensi` | admin, superadmin | Data Referensi |
| `/manajemen/migrasi` | admin, superadmin | Migration runs, kind imports (users/orang_tua/kelas/arsip) with trial, control totals, sign-off |
| `/arsip` | admin, kepala_sekolah, kepala_yayasan, superadmin | Read-only archive search |

Changed pages:
- **Informasi and Ubah Password:** now open to every role. Both endpoints were already role-agnostic, and orang tua and other roles are now announcement targets.
- **Kelola Informasi:**
  - Audience is mandatory ("Semua peran" or roles, including orang_tua).
  - Unit targets (superadmin) and class targets (current academic year).
  - `publish_at` schedule and a "Kirim sekarang?" confirmation on 409.
  - Status shown as draf, terjadwal or terbit.
  - Buttons are hidden according to `/me/permissions`.
- **Data Pengguna and Data Peran:** a 409 `exception_allowed` opens the "Ajukan pengecualian" dialog. The Agama dropdown reads from Data Referensi.
- **Button audit:** every identity view and Tahun Akademik shows the server's message (`utils/apiError.ts`) instead of a generic "Gagal …", so a 403/409/422 reason is readable.

### Coordinator extra (from QA-BILL)

- `GET /students`, `GET /users?role=siswa` and `GET /users/{id}` (student detail):
  - finance_manager, kepala_yayasan, kepala_sekolah and auditor now read students of their own active unit.
  - Responses carry identifiers only: name, email, NISN, roles/status. No NIK, phone, address, birth date, religion or password.
  - A student of another unit, or a non-student, returns 404.
  - `GET /students` no longer returns 403 to an X-Internal-Key caller.
  - Test: `TestOversightRolesReadStudents`.
- `GET /internal/users?role=<role>&school_id=<uuid>[&include_global=true]` (X-Internal-Key only) returns `[{id, name, email, school_id}]`, the active holders of that role. Test: `TestInternalUsersByRole`.

### Cross-service contracts (for other agents / operator)

1. **core (Approval Center):**
   - Add request type `role_conflict_exception` (const `ReqTypeRoleConflictException`) with an approval flow (suggested approver: kepala_yayasan or superadmin).
   - Identity submits it to `CORE_URL` the same way as other approvals.
   - Core must call `POST {IDENTITY_INTERNAL_URL}/internal/role-conflict-exceptions/approval-callback` with X-Internal-Key and `{approval_id, source_id (exception id), request_type, decision: approved|rejected, decided_by, comment}`. The callback is idempotent.
   - Until the type exists in core, submissions are stored with `status=failed` and core's refusal shown as the status message. Nothing is granted.
2. **Identity env var:** set `IDENTITY_INTERNAL_URL` (default `http://identity:8081`), the callback base URL identity sends to core.
3. **academic:** `GET /internal/user-classrooms?user_ids=` (committed in e19b01f) is used by identity announcements for class targeting. If academic is unreachable, class-targeted announcements are hidden; identity fails closed.
4. **QA-ACC:** add `akuntan` to the role list in `accounting.routes.ts` (ALL). Identity already accepts the role (mig 0022) and the web Role type has it. DashboardRedirect sends akuntan to `/akuntansi/jurnal`.
5. **admission:** mask NIK in its own lists, the same as the identity directory (identity no longer exposes NIK in directory views).
6. **Other services:** may read `GET /internal/permissions?role=&school_id=` (X-Internal-Key) to honour the permission matrix for their modules.
7. **billing/core alerts:** use `GET /internal/users?role=akuntan|finance_manager&school_id=` to find recipients.
8. **Finance migration figures (BUG-019/077):** billing/accounting can add their own control totals to a run through `PUT /migration-runs/{id}/totals`, for example category `tagihan`. The UI accepts any category. Their own import of source IDs is theirs to do.

### Verification (2026-10-15)

- `go build ./... && go vet` (identity): ok.
- `TEST_PG_DSN=…skci_qa_idn_identity go test ./... -count=1` (identity): all packages ok.
- academic `go test ./internal/handler -run TestInternalUserClassrooms`: ok (round 2 run).
- Migrations identity 0022/0023/0024: up → down 1 → up ok.
- Web `vue-tsc -b`: no errors (exit 0). `npm run build`: exit 0, with only the existing chunk-size warning. Both ran in Docker node:20-alpine.
