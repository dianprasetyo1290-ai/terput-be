# report-service

Part of the Sekoci Go+Vue rewrite. Owns E-raport (report cards) and Leger
(classroom rekap). Port `8086`, database `skci_report`. See
`../../docs/CONVENTIONS.md` for the shared JWT/response-envelope/
multi-tenancy/academic-year conventions every sibling service (identity,
academic, attendance, exam, evaluation) follows.

This is an **aggregator**, not a source of truth for student/classroom/exam
data — it owns only report-configuration/override state (`report_cards`,
`report_card_subjects`, `report_grade_bands`) and pulls everything else
(student names, classroom/term info, attendance summaries, exam scores)
from sibling services over internal HTTP. See "Expected upstream
contracts" below for exactly what it expects those services to expose.

## Data model

- **`report_cards`** — one row per `(student_id, classroom_id, term_id)`.
  Header + status envelope: `status` (`draft`|`finalized`), `finalized_at/by`,
  `unlocked_at/by`, a cached header snapshot (student name/NISN, classroom
  name, term name) and a cached attendance summary (hadir/sakit/izin/tanpa
  keterangan), each with a `*_synced_at`/`*_sync_error` so the UI can show
  "last synced" / "could not reach attendance-service" honestly.
- **`report_card_subjects`** — one row per `(report_card_id, subject_id)`.
  `computed_score` is the aggregated (average) value pulled from
  exam-service; `override_score`, if set, is what an admin manually
  adjusted it to — **both are kept**, never destroyed by a regenerate, so
  the UI can show "computed vs overridden". `final_score` (API-only, not a
  DB column) = `override_score` if set, else `computed_score`.
  `grade_label` (predikat A/B/C/D) is resolved at read time against
  `report_grade_bands`.
- **`report_grade_bands`** — optional per-school predikat thresholds
  (`min_score`–`max_score` → `label`). If a school has none, a built-in
  Kurikulum-Merdeka-style default (90-100=A, 80-89.99=B, 70-79.99=C,
  else=D) is used. This table is the "report configuration" surface the
  task description mentions; there was no spec-mandated shape for it, so
  this is a simplest-reasonable-call (see Assumptions).
- **Leger is not a table.** `GET /classrooms/{id}/leger?term_id=` is a pure
  read-model: it loads all `report_cards` for the classroom+term and
  reshapes their subject rows into a student × subject grid. Nothing is
  separately entered.

## Run locally

```bash
cp .env.example .env   # adjust if needed
docker compose up -d --build
# wait for postgres healthcheck, then apply migrations from the host:
DATABASE_URL="postgres://sekoci:change-me@localhost:5486/skci_report?sslmode=disable" \
  go run ./cmd/migrate up

curl http://localhost:8086/healthz
# {"status":"ok"}
```

To roll back: `go run ./cmd/migrate down`. To check applied version:
`go run ./cmd/migrate version`.

Tear down: `docker compose down` (add `-v` to also drop the postgres volume).

### Running the Go binary outside Docker

```bash
go build ./... && go run ./cmd/server
# reads DATABASE_URL / PORT / SEKOCI_JWT_SECRET / SEKOCI_INTERNAL_KEY /
# IDENTITY_SERVICE_URL / ACADEMIC_SERVICE_URL / ATTENDANCE_SERVICE_URL /
# EXAM_SERVICE_URL from env or .env
```

### Minting a test JWT

Every route except `/healthz` requires `Authorization: Bearer <JWT>` signed
with `SEKOCI_JWT_SECRET` (default `change-me`). In
real usage identity-service issues this at login; for local testing without
identity-service running, use the included helper:

```bash
go run ./cmd/mintjwt admin   11111111-1111-1111-1111-111111111111 22222222-2222-2222-2222-222222222222
go run ./cmd/mintjwt siswa   33333333-3333-3333-3333-333333333333 22222222-2222-2222-2222-222222222222
go run ./cmd/mintjwt guru    55555555-5555-5555-5555-555555555555 22222222-2222-2222-2222-222222222222
# args: role sub school_id
```

## Expected upstream contracts

report-service calls four sibling services over plain internal HTTP with
`X-Internal-Key: <SEKOCI_INTERNAL_KEY>` (default `change-me`).
All calls have a 3s timeout and degrade to partial/null data on failure
(see "Resilience" below) rather than failing the request. At the time this
was written the other five services were being built in parallel by
sibling agents; the shapes below were reconciled where their code already
existed (identity/academic/attendance had partial implementations; exam
did not yet have a scores endpoint), and are flagged `[guess]` where not.

**identity-service** (`IDENTITY_SERVICE_URL`, default `:8081`)
- `GET /users/{id}` → `{id, name, nisn, school_id}` — used for report-card
  header (student name/NISN). **Gap found**: as of this writing,
  identity-service's `/users/{id}` sits behind `JWTMiddleware` only (no
  `X-Internal-Key` bypass), so report-service's internal-only calls to it
  will currently get a 401. attendance-service already solved this exact
  problem for its own report-service integration point (see
  `attendance/internal/common/internal_auth.go`'s `InternalOrJWTMiddleware`
  — accepts either a real JWT or `X-Internal-Key`, and passes `claims=nil`
  on the internal path). identity-service should adopt the same pattern
  for `GET /users/{id}` (or expose an `/internal/users/{id}` variant) for
  this integration to actually work end-to-end.

**academic-service** (`ACADEMIC_SERVICE_URL`, default `:8082`) — confirmed
against real code (`academic/internal/handler/classroom.go`, `term.go`):
- `GET /classrooms/{id}` → `{id, school_id, academic_year_id, name, grade_level}`
- `GET /terms/{id}` → `{id, school_id, academic_year_id, name}`
- `GET /classrooms/{id}/students` → `[{id, student_user_id, status}]` (the
  raw `ClassEnrollment` rows — **no embedded student name/NISN**, so
  report-service always resolves those from identity-service separately).
  report-service passes `?term_id=` defensively but academic-service's
  current implementation ignores it (enrollment is classroom-scoped, not
  term-scoped).
- `GET /classrooms/{id}/subjects?term_id=` → `[{id, name, code}]` `[guess — endpoint exists as ListClassroomSubjects but response shape not independently verified]`
- Same auth gap as identity: these routes are JWT-only as of this writing;
  no `X-Internal-Key` bypass exists yet on academic-service either.

**attendance-service** (`ATTENDANCE_SERVICE_URL`, default `:8083`) —
confirmed against real code (`attendance/internal/handler/report.go`,
`router.go`) and **this is the one sibling that already fully supports the
integration**:
- `GET /students/{id}/attendance-summary?term_id=&academic_year_id=` →
  `{hadir, sakit, izin, tanpa_keterangan}` (all ints). Reachable via
  `X-Internal-Key` (claims-free, unscoped by school — trusts the internal
  caller) or a normal JWT. This is the reference implementation the other
  two services' internal-auth gap above should copy.

**exam-service** (`EXAM_SERVICE_URL`, default `:8084`) — exam-service had
exams/exam_types CRUD built but no scores/results endpoint yet, so this is
mostly `[guess]`, reconciled only where real code existed:
- `GET /exams?classroom_id=&term_id=` → `[{id, subject_id, classroom_id, term_id, exam_type_id, title}]`
  — confirmed shape (minus scoring fields) against real `model.Exam`.
- `GET /exam-types?academic_year_id=` → `[{id, school_id, academic_year_id, name, mapping_rapor}]`
  — confirmed against real `model.ExamType`. **Important**: `mapping_rapor`
  lives on `ExamType`, not embedded on `Exam` — report-service fetches both
  lists and joins client-side via `exam_type_id`. An exam whose type has an
  empty `mapping_rapor` (e.g. old system's "Test Kemampuan Akademik") is
  excluded from report aggregation entirely, matching the legacy
  `exam_types.grade_mapping` semantics found in the old Laravel schema.
- `GET /exams/{id}/scores` → `[{student_id, score}]` — `[guess, endpoint
  did not exist yet]`. Given verbatim by the report-service task spec
  itself, so kept as specified rather than reconciled against exam-service
  code.

## Aggregation rule (subject computed_score)

For each subject assigned to the classroom, `computed_score` = the average
of that student's scores across every exam in that classroom+term whose
`ExamType.mapping_rapor` is non-empty and whose `subject_id` matches. This
is the "simplest reasonable rule" called out in the task spec, made
overridable per-subject via `PATCH .../subjects/{subject_id}`.

## Resilience

Every upstream call in `internal/upstream` returns a typed
`*upstream.UnavailableError` (wraps `upstream.ErrUnavailable`) on timeout,
connection failure, or non-2xx status — handlers never let this become a
500. Specifically in `POST /report-cards/generate`:

- Roster (`ListClassroomStudents`) failing is the *one* hard dependency —
  without it there's no way to know which students to generate cards for,
  so generate returns **200** with `created:0, refreshed:0` and a
  `warnings` object explaining what failed (not a crash, not a 500).
- Every other integration point (identity header, attendance summary,
  exam scores, exam types, classroom/term names, subject list) degrades
  independently per student/subject: the `report_cards`/
  `report_card_subjects` rows still get created with whatever fields
  succeeded and `null` + a `sync_error` string for whatever didn't.
- Verified end-to-end locally (see "What was tested" below) with a stub
  academic-service standing in for the real one while
  identity/attendance/exam stayed down — confirmed a report card gets
  created with classroom/term name populated but null student name,
  null attendance, and a subject row with `computed_score: null,
  sync_error: "exam-service unavailable"`.
- A finalized report card additionally skips the attendance/computed-score
  refresh entirely on regenerate (header snapshot still refreshes) so a
  locked report never silently changes under an admin's feet — they must
  `PUT /unlock` first.

## curl examples

```bash
ADMIN_TOKEN=$(go run ./cmd/mintjwt admin 11111111-1111-1111-1111-111111111111 22222222-2222-2222-2222-222222222222)
SISWA_TOKEN=$(go run ./cmd/mintjwt siswa 33333333-3333-3333-3333-333333333333 22222222-2222-2222-2222-222222222222)
CLASSROOM=77777777-7777-7777-7777-777777777777
TERM=88888888-8888-8888-8888-888888888888

# Generate/refresh report cards for a whole classroom for a term
curl -s -X POST "http://localhost:8086/report-cards/generate?classroom_id=$CLASSROOM&term_id=$TERM" \
  -H "Authorization: Bearer $ADMIN_TOKEN"

# Get one report card (full detail: header + subjects + attendance)
curl -s http://localhost:8086/report-cards/<id> -H "Authorization: Bearer $ADMIN_TOKEN"

# A student's own report cards (siswa can only fetch their own — 403 otherwise)
curl -s "http://localhost:8086/students/<student_id>/report-cards?term_id=$TERM" \
  -H "Authorization: Bearer $SISWA_TOKEN"

# Admin overrides one subject's score (only while draft; 409 once finalized)
curl -s -X PATCH "http://localhost:8086/report-cards/<id>/subjects/<subject_id>" \
  -H "Authorization: Bearer $ADMIN_TOKEN" -H "Content-Type: application/json" \
  -d '{"override_score": 88.5, "notes": "manual adjustment"}'

# Clear an override back to the computed value
curl -s -X PATCH "http://localhost:8086/report-cards/<id>/subjects/<subject_id>" \
  -H "Authorization: Bearer $ADMIN_TOKEN" -H "Content-Type: application/json" \
  -d '{"clear": true}'

# Finalize (locks it) / unlock (admin explicit reopen)
curl -s -X POST "http://localhost:8086/report-cards/<id>/finalize" -H "Authorization: Bearer $ADMIN_TOKEN"
curl -s -X PUT  "http://localhost:8086/report-cards/<id>/unlock"   -H "Authorization: Bearer $ADMIN_TOKEN"

# Leger: classroom x term rekap grid (admin/guru/superadmin only)
curl -s "http://localhost:8086/classrooms/$CLASSROOM/leger?term_id=$TERM" -H "Authorization: Bearer $ADMIN_TOKEN"
```

## What was tested (this session, local dev, no prod risk)

- `go build ./...` and `go vet ./...` clean; `gofmt -l .` clean.
- `docker compose up -d --build` → both containers healthy; `go run
  ./cmd/migrate up` applied migration 0001 successfully (`version=1
  dirty=false`).
- `GET /healthz` → `{"status":"ok"}`, no auth required.
- `POST /report-cards/generate` **with all four upstreams down** →
  returned `200` with `created:0`, an explicit `warnings` object per
  failed call, *not* a crash/500. Proved the "no roster = graceful, not a
  hard failure" path explicitly.
- `POST /report-cards/generate` **with a stub academic-service up on
  :8082 and identity/attendance/exam still down** (the realistic "not all
  6 up together" scenario from the task brief) → created 1 report card
  with `classroom_name`/`term_name` populated, `student_name: null`,
  `attendance: {..all null, sync_error: "..."}`, and one subject row
  (`Matematika`) with `computed_score: null, sync_error: "exam-service
  unavailable"`. This is the literal "nulled/partial subject data instead
  of crashing" behavior the task asked to prove.
- `GET /report-cards/{id}` as the owning `siswa` → 200; as a *different*
  `siswa` → 404 (not 403, to avoid confirming the ID belongs to someone
  else).
- `GET /students/{id}/report-cards` as a different `siswa` → 403.
- `PATCH .../subjects/{subject_id}` as `admin` → sets `override_score`,
  `final_score` resolves to it, `grade_label` computed (`88.5` → `B`
  against the default bands); as `guru` → 403.
- `POST .../finalize` → status becomes `finalized`; a second `finalize` →
  409; `PATCH` override afterward → 409 telling the caller to unlock
  first; `PUT .../unlock` on a non-finalized card → 409; on the finalized
  one → status back to `draft`.
- `GET /classrooms/{id}/leger` as `guru` → 200 grid with the one
  student/subject/score; as `siswa` → 403.
- `POST /report-cards/generate` and `PATCH .../subjects/...` as `siswa` →
  403 (role gate enforced server-side, not just documented).
- `docker compose down` → clean teardown, no orphaned containers/volumes
  left running.

## Assumptions

- **Role gates**: `generate`, `PATCH override`, `finalize`, `unlock` are
  restricted to `admin`/`superadmin` (the spec only says "admin needs to
  be able to..."; `guru` gets read access only via `leger`/`GET
  report-cards`, not write access, since the spec doesn't mention guru
  input here — unlike academic-service's grade-entry-by-subject-teacher
  flows). `leger` is `admin`/`guru`/`superadmin` (not `siswa` — a
  classroom-wide grid isn't appropriate for a single student to see).
  `siswa` can only ever read their own report card(s).
- **Multi-tenancy on `GET /report-cards/{id}`**: a wrong-school lookup
  returns `404`, not `403`, to avoid confirming a given UUID exists in
  another school's data.
- **PDF export**: per the task instructions, this service returns
  structured JSON only (`GET /report-cards/{id}` is already the "PDF-ready
  structured data" — headers, subject rows, attendance, status). No Go PDF
  library was added; rendering is the Vue frontend's job.
- **Predikat/grade bands**: not specified by the client doc at all; added
  a small overridable `report_grade_bands` table (per-school) with a
  hardcoded Kurikulum-Merdeka-style A/B/C/D default (90/80/70 cutoffs) used
  when a school hasn't configured its own. This is the "report
  configuration" surface referenced in the task description.
- **Regenerate vs. override**: `POST /report-cards/generate` is safe to
  call repeatedly — it never clobbers `override_score`/`notes` on existing
  subject rows (only `computed_score` and its sync metadata), and it
  fully skips attendance/score refresh on any already-`finalized` card.
- **Header snapshot caching**: student/classroom/term names are cached
  onto `report_cards` at generate time (not fetched live on every `GET`)
  so the detail/list/leger endpoints stay fast and don't multiply upstream
  load — a `header_synced_at` timestamp is exposed so a stale cache is at
  least visible, not silently wrong.
- **`report_card_subjects` rows only exist for subjects academic-service
  currently reports for the classroom** — if academic-service is down
  during a generate call, no subject rows get created/refreshed that run
  (existing ones, if any, are left untouched); there's no way to know the
  subject list without it.
- Two identity/academic **auth gaps flagged, not fixed** (out of scope —
  can't touch sibling directories): `GET /users/{id}` and the
  `/classrooms/*` read endpoints this service depends on are currently
  JWT-only in their respective services, with no `X-Internal-Key` bypass
  like attendance-service already implemented for exactly this purpose.
  Until those are added, report-service's identity/academic enrichment
  calls will 401 against the real services (verified working correctly
  against attendance-service, and against a stub standing in for
  academic-service, in this session's testing).
