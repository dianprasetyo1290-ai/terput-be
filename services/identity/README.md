# identity-service

Owns auth/credentials, users, roles, schools, foundations, organization/position
(jabatan), academic years, and the bulk "Import Template Sekoci" school
onboarding flow, for the Sekoci Go+Vue rewrite. Port `8081`, database
`skci_identity`. See `../../docs/CONVENTIONS.md` for the shared JWT/response
envelope/multi-tenancy rules this service (and its 5 siblings) follow.

## Run locally

```bash
# 1. copy env (docker-compose.yml already has working defaults baked in,
#    .env.example is for running `go run` directly against the compose db)
cp .env.example .env

# 2. start postgres + the service
docker compose up -d --build

# 3. run migrations (installs golang-migrate if you don't have it: `brew install golang-migrate`)
migrate -path migrations \
  -database "postgres://sekoci:change-me@localhost:5481/skci_identity?sslmode=disable" \
  up

# 4. sanity check
curl http://localhost:8081/healthz
# {"status":"ok"}
```

A seed superadmin account is created by migration `0002_seed`:
`superadmin@sekoci.local` / `Sekoci@2026`.

To run outside Docker (e.g. against a local Postgres):
```bash
go run ./cmd/server
```

Tear down: `docker compose down` (add `-v` to also drop the postgres volume).

## curl examples

**Login:**
```bash
curl -s -X POST http://localhost:8081/auth/login \
  -H "Content-Type: application/json" \
  -d '{"email":"superadmin@sekoci.local","password":"Sekoci@2026"}'
# => {"data":{"token":"<jwt>","user":{...}},"message":"","errors":null}
TOKEN=$(curl -s -X POST http://localhost:8081/auth/login -H "Content-Type: application/json" \
  -d '{"email":"superadmin@sekoci.local","password":"Sekoci@2026"}' | jq -r .data.token)
```

**Create a foundation (Data Yayasan):**
```bash
curl -s -X POST http://localhost:8081/foundations \
  -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" \
  -d '{"name":"Yayasan Cerdas Bangsa","address":"Jl. Merdeka No 1","year_founded":1998,"year_joined":2020}'
```

**Create a school (Data Sekolah, manual add):**
```bash
curl -s -X POST http://localhost:8081/schools \
  -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" \
  -d '{"name":"SD Cerdas 1","level":"SD","city":"Jakarta"}'
```

**List users (Data Pengguna — superadmin sees all schools, `?school_id=` optional):**
```bash
curl -s "http://localhost:8081/users?role=guru" -H "Authorization: Bearer $TOKEN"
```

**Switch role without logout** (for an account holding multiple roles, e.g. a
guru who is also tagged Kepala Sekolah):
```bash
curl -s -X POST http://localhost:8081/auth/switch-role \
  -H "Authorization: Bearer $GURU_TOKEN" -H "Content-Type: application/json" \
  -d '{"role":"kepala_sekolah"}'
```

**Impersonate a school's admin:**
```bash
curl -s -X POST "http://localhost:8081/schools/$SCHOOL_ID/impersonate" -H "Authorization: Bearer $TOKEN"
```

**Sekoci bulk import — download template, dry run, then import:**
```bash
curl -s "http://localhost:8081/schools/import/template" -H "Authorization: Bearer $TOKEN" -o template.xlsx
curl -s -X POST http://localhost:8081/schools/import/dry-run -H "Authorization: Bearer $TOKEN" \
  -F "file=@filled_template.xlsx"
curl -s -X POST http://localhost:8081/schools/import -H "Authorization: Bearer $TOKEN" \
  -F "file=@filled_template.xlsx" -F "academic_year=2026/2027" -F "school_name=SMP Harapan Bangsa"
```

## Endpoint map

| Area | Endpoints |
|---|---|
| Auth | `POST /auth/login`, `POST /auth/switch-role`, `GET /auth/me` |
| Dashboard | `GET /dashboard` (superadmin) |
| Foundations | `GET/POST /foundations`, `GET/PUT/DELETE /foundations/{id}` (superadmin) |
| Schools | `GET/POST /schools`, `GET/PUT/DELETE /schools/{id}`, `POST /schools/{id}/activate\|deactivate\|assign-admin\|impersonate` |
| Import | `GET /schools/import/template`, `POST /schools/import/dry-run`, `POST /schools/import` |
| Users (Data Pengguna) | `GET/POST /users`, `GET/PUT /users/{id}`, `POST /users/{id}/reset-password`, `POST /users/{id}/roles`, `GET /users/export` |
| Students/Teachers | `GET /students`, `GET /teachers`, `POST /teachers/{id}/activate\|deactivate` |
| Positions (Data Jabatan / Peran) | `GET/POST /positions`, `PUT/DELETE /positions/{id}`, `POST /positions/{id}/assign`, `DELETE /position-assignments/{id}`, `GET /organization` |
| Academic years (Tahun Akademik) | `GET/POST /academic-years`, `POST /academic-years/{id}/set-current` |
| Announcements (Informasi) | `GET /announcements` (visible to me), `GET /announcements/all` (admin mgmt), `POST/PUT/DELETE /announcements[/{id}]` |
| Ops | `GET /healthz` |

All routes except `/healthz` and `/auth/login` require `Authorization: Bearer <jwt>`.

## What was tested end-to-end (with the stack actually running)

Ran `docker compose up -d --build`, applied migrations with `golang-migrate`,
then via curl/jq against the live containers:
- `GET /healthz`
- `POST /auth/login` (seeded superadmin)
- `POST /foundations`, `POST /schools` (with `foundation_id` link)
- `GET /dashboard` (per-school student counts, verified they don't mix across 2 schools)
- `GET /users` (Data Pengguna list with joined roles)
- `POST /users` (create admin + guru accounts), `POST /schools/{id}/assign-admin`
- `POST /schools/{id}/impersonate`, then `PUT /schools/{id}` as the impersonated admin token (Identitas Sekolah edit)
- `POST /academic-years`, `POST /academic-years/{id}/set-current`
- `POST /positions`, `POST /positions/{id}/assign` (verified the jabatan-only role mirrors into the JWT `roles` claim), `GET /organization`
- `POST /auth/switch-role` (guru account holding an extra `kepala_yayasan` tag switching active_role with no re-login — verified the reissued JWT)
- `POST /users/{id}/reset-password`
- `POST /announcements` with `target_roles`, verified `GET /announcements` returns it for a `guru` token and not for an untargeted `admin` token
- `GET /schools/import/template` (downloaded, confirmed valid .xlsx)
- `POST /schools/import/dry-run` and `POST /schools/import` against a hand-built 6-sheet .xlsx (including one deliberately duplicated guru row) — confirmed the duplicate was silently dropped, the school/academic-year/guru/siswa/jabatan rows were created, Struktur Organisasi rows correctly mirrored "Kepala Sekolah" into the JWT-visible role set, and Data Kelas / Data mata pelajaran rows landed in `import_staged_rows` (kind='kelas'/'mata_pelajaran', dispatched=false) rather than being silently discarded
- `GET /users/export?role=guru` (CSV)
- `docker compose down` after verification

`go build ./...` and `go vet ./...` both pass with no errors or warnings.

## Assumptions

Made where the spec was ambiguous, to keep moving without blocking on the
other 5 parallel agents:

1. **`school_levels` not modeled as its own table.** The old Laravel schema
   has a separate `school_levels` lookup table; this service just uses a free
   text `schools.level` column (e.g. "SD"/"SMP"/"SMA"). Simpler, and nothing
   in the spec calls for level-specific behavior beyond a label.
2. **Two distinct "role" concepts, deliberately not merged:**
   - The **7 system roles** (superadmin/admin/guru/siswa +
     walikelas/kepala_sekolah/kepala_yayasan) live in `user_school_roles` and
     drive the JWT `active_role`/`roles` claims — these are what
     `switch-role` operates over.
   - **Data Jabatan / Peran** (admin creates arbitrary jabatan labels like
     "Bendahara", "Wali Kelas 7A") live in `positions` +
     `position_assignments` and only feed the "Organisasi Sekolah" list.
     The spec explicitly says Peran and Data Jabatan are "the same concept,
     implement once" — done as one `positions` CRUD + assignment.
   - **Bridge between the two:** when an admin assigns a jabatan whose name
     canonicalizes (case/whitespace-insensitive) to exactly "Walikelas" /
     "Wali Kelas" / "Kepala Sekolah" / "Kepala Yayasan", it's also mirrored
     into `user_school_roles` so it shows up in the JWT `roles` claim per-spec
     ("Kepala Sekolah appears as a dropdown option... in evaluation-service").
     **Limitation:** a per-class variant like "Walikelas 7A" does NOT
     auto-mirror (it doesn't exactly match "Walikelas") — admin can still tag
     it manually via `POST /users/{id}/roles`. Exact-match mirroring was the
     simplest rule that satisfies the one example given in the spec without
     guessing at fuzzy-matching semantics.
3. **`Claims.SchoolID` is `""`, not `null`, for superadmin tokens.** The
   verbatim `Claims` struct from CONVENTIONS.md declares `SchoolID string`
   (not `*string`), so there's no way to encode JSON `null` — an empty string
   is the closest fit and is what `resolveSchoolScope()` checks for.
4. **Data Siswa / Data Guru reuse the generic Data Pengguna handlers**
   (`GET /students` / `GET /teachers` are `GET /users?role=siswa|guru` under
   the hood; edit and reset-password use the same `PUT /users/{id}` /
   `POST /users/{id}/reset-password` as Data Pengguna) rather than
   duplicating parallel CRUD. Guru status aktif/nonaktif lives on the
   `user_school_roles.status` column (that role assignment, not the account,
   is deactivated — satisfies "deactivating a guru should NOT delete them").
5. **Import form fields:** the spec's "Buat sekolah" field is the multipart
   field `school_name` (optional; overrides the name from the "Data
   Identitas Sekolah" sheet), "Tahun Akademik" is the multipart field
   `academic_year` (required on `POST /schools/import`, not required for
   `dry-run` since dry-run never touches academic years). "Download Template"
   generates a blank 6-sheet .xlsx with a documented header row per sheet
   (see `internal/handler/import.go` for the exact column layout) since no
   real Sekoci template file was available in the reference tree to copy
   column-for-column.
6. **Import dedup key varies by sheet** (spec only says "duplicate rows,
   first kept, not documented column-by-column): guru/siswa dedup by email;
   Struktur Organisasi dedups by (jabatan, guru email) pair; Data Kelas /
   Data mata pelajaran dedup by full-row equality since they have no obvious
   natural key at this layer (academic-service, which actually owns that
   data, may want a smarter key once it exists).
7. **Data Kelas / Data mata pelajaran are staged, not dispatched.**
   `import_staged_rows` holds them as raw JSON (`kind`, `payload`,
   `dispatched=false`). `internal/handler/import.go` documents the intended
   HTTP hand-off (`POST http://localhost:8082/internal/import-rows` with the
   `X-Internal-Key` header from CONVENTIONS.md) as a TODO — not implemented
   since academic-service doesn't exist yet and this service can't block on
   a sibling agent's output.
8. **Announcements with `school_id = NULL`** are treated as a superadmin
   broadcast visible to every school; admin-created announcements are always
   scoped to the admin's own school. Not explicitly specified, but the
   spec's "publish to all roles or a subset of roles" reads as role-scoping,
   not school-scoping, and someone (superadmin) has to be able to reach every
   school for org-wide announcements.
9. **`requireRole()` treats superadmin as a superset of every other role** —
   e.g. superadmin can call admin-only endpoints directly (not just via
   impersonation). This matches "superadmin can do everything admin can, plus
   cross-school things admin can't" and avoids forcing every superadmin
   action through impersonation.
10. **No self-service "change my password" endpoint.** The spec only
    describes *admin* resetting a student's/user's password and the
    default-password-visibility flag; a siswa/guru changing their own
    password isn't mentioned in identity's section, so it's out of scope
    here. `password_changed` currently only flips via `POST
    /users/{id}/reset-password` (which sets it back to `false` since that's
    a fresh default). If a self-service change-password flow is wanted later,
    it's a small addition (`PUT /auth/password`, `password_changed=true`).
11. **Voucher generation, class rosters, subject matrix**: explicitly
    exam-service's and academic-service's domain per the spec; not built
    here even though `positions`/`users` tables could technically support
    fragments of it.
12. **Seed superadmin** (`superadmin@sekoci.local` / `Sekoci@2026`) ships in
    migration `0002_seed` purely so the service is immediately usable after
    `migrate up` with zero manual bootstrap steps — remove/change this in
    any shared/staging environment.
