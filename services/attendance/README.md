# attendance-service

Part of the Sekoci Go+Vue rewrite. Owns student/teacher *presensi* (daily
attendance) and the *Agenda Pembelajaran Harian* (daily teaching agenda).
Port `8083`, database `skci_attendance`. See `../../docs/CONVENTIONS.md` for
the shared JWT/response-envelope/multi-tenancy/academic-year conventions
every sibling service (identity, academic, exam, evaluation, report) follows.

This service stores **events**, not master data: it never owns users,
classrooms, or schedules — those belong to `identity`-service and
`academic`-service respectively. It references their records by UUID only
and enriches responses with names/rosters via best-effort internal HTTP
calls (`X-Internal-Key` header) when those services are reachable.

## Data model

- **`class_sessions`** — one row per `(schedule_id, session_date)`. Created
  when a guru self-checks-in for a schedule slot ("presensi guru"). Also
  carries the Agenda Pembelajaran Harian fields (`pertemuan`, `pembahasan`,
  `materi_pertemuan`, `bentuk_kegiatan`) filled in right after check-in, plus
  (client request, Sekolah GPM) a session-level `catatan`, a `tugas_diberikan`
  +`tugas_jenis` ("individu"/"kelompok") checkbox, `checked_out_at` (jam
  keluar, alongside `checked_in_at` as jam masuk), and "pergantian guru"
  substitute-teacher tracking (`original_guru_id`, `substituted_at`,
  `substituted_by` — `guru_id` itself always holds whoever is *currently*
  covering the session). This single table doubles as the "Presensi Guru"
  admin record, since a guru's attendance *is* their check-in on a session.
- **`student_attendances`** — one row per `(class_session_id, student_id)`,
  status `hadir|sakit|izin|tanpa_keterangan` (default `hadir`), optional
  `catatan`. This is what the admin "Presensi Siswa" view and the
  report-service summary endpoint read from.

## Run locally

```bash
cp .env.example .env   # adjust if needed
docker compose up -d --build
# wait for postgres healthcheck, then apply migrations from the host:
DATABASE_URL="postgres://sekoci:change-me@localhost:5483/skci_attendance?sslmode=disable" \
  go run ./cmd/migrate up

curl http://localhost:8083/healthz
# {"status":"ok"}
```

To roll back: `go run ./cmd/migrate down`. To check applied version:
`go run ./cmd/migrate version`.

Tear down: `docker compose down` (add `-v` to also drop the postgres volume).

### Running the Go binary outside Docker

```bash
go build ./... && go run ./cmd/server
# reads DATABASE_URL / PORT / SEKOCI_JWT_SECRET / SEKOCI_INTERNAL_KEY /
# IDENTITY_SERVICE_URL / ACADEMIC_SERVICE_URL from env or .env
```

## Auth for curl examples

Every route except `/healthz` requires `Authorization: Bearer <JWT>` signed
with `SEKOCI_JWT_SECRET` (default `change-me`), with
claims shaped per CONVENTIONS.md:

```json
{"sub":"<user_id>","school_id":"<school_id>","active_role":"guru","roles":["guru"],"exp":...}
```

In real usage identity-service issues this token at login. For local testing
without identity-service running, mint one yourself, e.g. with a throwaway
Go script using `github.com/golang-jwt/jwt/v5` and the shared dev secret, or
any JWT HS256 tool — the important part is signing with
`change-me` and including `sub`/`school_id`/
`active_role`/`roles`/`exp`.

## curl examples (guru daily flow)

```bash
GURU_TOKEN="<jwt with active_role=guru>"
ADMIN_TOKEN="<jwt with active_role=admin>"
SCHOOL=11111111-1111-1111-1111-111111111111
SCHEDULE=66666666-6666-6666-6666-666666666666   # academic-service schedule id
CLASSROOM=77777777-7777-7777-7777-777777777777  # academic-service classroom id
YEAR=88888888-8888-8888-8888-888888888888       # academic-service academic_year id
STUDENT=33333333-3333-3333-3333-333333333333    # identity-service user id

# 1. guru self check-in for today's schedule slot
curl -X POST "http://localhost:8083/sessions/checkin?academic_year_id=$YEAR" \
  -H "Authorization: Bearer $GURU_TOKEN" -H "Content-Type: application/json" \
  -d "{\"schedule_id\":\"$SCHEDULE\",\"classroom_id\":\"$CLASSROOM\",\"slot_start_time\":\"07:00\",\"slot_end_time\":\"08:30\"}"
# -> { "data": { "id": "<session_id>", ... }, "message": "Presensi berhasil dicatat", ... }
SESSION_ID="<paste session id from response>"

# 2. fill Agenda Pembelajaran Harian for that session -- catatan and the
#    tugas checkbox (tugas_diberikan/tugas_jenis) are part of the same step
curl -X PUT "http://localhost:8083/sessions/$SESSION_ID/agenda" \
  -H "Authorization: Bearer $GURU_TOKEN" -H "Content-Type: application/json" \
  -d '{"pertemuan":"12","pembahasan":"Trigonometri","materi_pertemuan":"Sin Cos Tan","bentuk_kegiatan":["teori","praktikum"],"catatan":"ada tugas yang harus diselesaikan hari itu juga","tugas_diberikan":true,"tugas_jenis":"kelompok"}'

# 2b. jam keluar (clock-out) for this pertemuan, once the guru is done
curl -X PUT "http://localhost:8083/sessions/$SESSION_ID/checkout" -H "Authorization: Bearer $GURU_TOKEN"

# 3a. "Pilih semua" — mark every roster student hadir in one call
curl -X POST "http://localhost:8083/sessions/$SESSION_ID/students/bulk" \
  -H "Authorization: Bearer $GURU_TOKEN" -H "Content-Type: application/json" \
  -d "{\"student_ids\":[\"$STUDENT\"],\"status\":\"hadir\"}"

# 3b. override one student's status via "aksi -> ubah status"
curl -X PUT "http://localhost:8083/sessions/$SESSION_ID/students/$STUDENT" \
  -H "Authorization: Bearer $GURU_TOKEN" -H "Content-Type: application/json" \
  -d '{"status":"sakit","catatan":"demam"}'

# roster proxy (best-effort, calls academic-service; falls back to whatever
# is already recorded if academic-service is unreachable)
curl "http://localhost:8083/sessions/$SESSION_ID/roster" -H "Authorization: Bearer $GURU_TOKEN"

# pergantian guru — the guru assigned to a slot (or an admin) hands it off
# to another guru who will cover it instead. Same schedule_id/session_date/
# academic_year_id resolution as checkin above; guru_id on the resulting
# session becomes the substitute's, original_guru_id keeps the audit trail.
# The substitute then does their OWN checkin (step 1) for the same
# schedule_id/date -- normally rejected as "not their slot", but allowed
# here because this call already named them as covering it.
SUBSTITUTE_GURU=44444444-4444-4444-4444-444444444444
curl -X POST "http://localhost:8083/sessions/substitute-guru?academic_year_id=$YEAR" \
  -H "Authorization: Bearer $GURU_TOKEN" -H "Content-Type: application/json" \
  -d "{\"schedule_id\":\"$SCHEDULE\",\"classroom_id\":\"$CLASSROOM\",\"substitute_guru_id\":\"$SUBSTITUTE_GURU\"}"
# -> { "data": { ..., "guru_id": "<substitute>", "original_guru_id": "<caller>", "substituted_at": "..." }, "message": "Pergantian guru berhasil dicatat", ... }
```

## curl examples (admin)

```bash
# Presensi Siswa — per classroom, filterable by academic year (incl. past
# years)/bulan/hari/tanggal
curl "http://localhost:8083/admin/student-attendance?academic_year_id=$YEAR&classroom_id=$CLASSROOM&month=2026-07" \
  -H "Authorization: Bearer $ADMIN_TOKEN"

# CSV export/print feed
curl "http://localhost:8083/admin/student-attendance/export?academic_year_id=$YEAR" \
  -H "Authorization: Bearer $ADMIN_TOKEN" -o presensi_siswa.csv

# Presensi Guru — per guru, with agenda context
curl "http://localhost:8083/admin/teacher-attendance?academic_year_id=$YEAR" \
  -H "Authorization: Bearer $ADMIN_TOKEN"

# "guru sedang mengajar" realtime-ish monitor (poll this)
curl "http://localhost:8083/dashboard/active-teachers" -H "Authorization: Bearer $ADMIN_TOKEN"

# dashboard charts
curl "http://localhost:8083/dashboard/student-attendance-summary?academic_year_id=$YEAR" -H "Authorization: Bearer $ADMIN_TOKEN"
curl "http://localhost:8083/dashboard/teacher-attendance-summary?academic_year_id=$YEAR" -H "Authorization: Bearer $ADMIN_TOKEN"
```

## curl example (report-service integration)

`report-service` calls this endpoint over internal HTTP for e-raport
attendance summaries:

```bash
curl "http://localhost:8083/students/$STUDENT/attendance-summary?term_id=<term_id>" \
  -H "X-Internal-Key: change-me"
# -> { "data": { "hadir": 1, "sakit": 1, "izin": 0, "tanpa_keterangan": 0 }, ... }
```

It also accepts a normal user JWT (admin/guru/the student themself) instead
of the internal key — see Assumptions below.

## Endpoints

| Method | Path | Role | Notes |
|---|---|---|---|
| GET | `/healthz` | none | |
| POST | `/sessions/checkin` | guru | step 1: self presensi for a schedule slot/date (jam masuk) |
| POST | `/sessions/substitute-guru` | guru (assigned to the slot) / admin | pergantian guru: reassign who's covering a schedule slot/date |
| GET | `/sessions/{id}` | guru (own) / admin | |
| PUT | `/sessions/{id}/agenda` | guru (own) | step 2: Agenda Pembelajaran Harian, incl. `catatan`/`tugas_diberikan`/`tugas_jenis` |
| PUT | `/sessions/{id}/checkout` | guru (own) | jam keluar (clock-out) for this pertemuan |
| GET | `/sessions/{id}/roster` | guru (own) | proxies academic-service roster + current statuses |
| GET | `/sessions/{id}/students` | guru (own) / admin | |
| POST | `/sessions/{id}/students/bulk` | guru (own) | "Pilih semua" |
| PUT | `/sessions/{id}/students/{student_id}` | guru (own) | "ubah status" override |
| GET | `/sessions` | admin | filters: `guru_id, classroom_id, academic_year_id, term_id, date, month, day` |
| GET | `/admin/student-attendance` | admin | Presensi Siswa |
| GET | `/admin/student-attendance/export` | admin | CSV |
| GET | `/admin/teacher-attendance` | admin | Presensi Guru |
| GET | `/admin/teacher-attendance/export` | admin | CSV |
| GET | `/dashboard/active-teachers` | admin | "guru sedang mengajar" |
| GET | `/dashboard/student-attendance-summary` | admin | chart data |
| GET | `/dashboard/teacher-attendance-summary` | admin | chart data |
| GET | `/students/{id}/attendance-summary` | internal-key or JWT | for report-service e-raport |

All list endpoints support `?page=&per_page=`; scoping (`school_id`,
`academic_year_id`) follows CONVENTIONS.md's multi-tenancy/academic-year
rules.

## What was actually tested (2026-07-18, local docker compose)

1. `go build ./...` and `go vet ./...` — clean.
2. `docker compose up -d --build` — postgres + attendance service started,
   postgres healthcheck passed.
3. `go run ./cmd/migrate up` against the compose postgres (host port 5483) —
   applied `0001_init` (`class_sessions`, `student_attendances`,
   `schema_migrations` tables confirmed via `psql \dt`).
4. `GET /healthz` — `{"status":"ok"}`.
5. Minted guru/admin/siswa JWTs with `github.com/golang-jwt/jwt/v5`, HS256,
   signed with the shared dev secret `change-me`
   (matching the claims shape from CONVENTIONS.md).
6. Full guru daily flow end-to-end: `POST /sessions/checkin` ->
   `PUT /sessions/{id}/agenda` -> `POST /sessions/{id}/students/bulk`
   (pilih semua hadir) -> `PUT /sessions/{id}/students/{student_id}`
   (override to sakit + catatan) -> `GET /sessions/{id}/students` confirms
   the final per-student state.
7. Admin views: `GET /dashboard/active-teachers` (confirmed a guru who
   checked into a same-day, not-yet-ended slot shows up, and does NOT show
   up when the session_date doesn't match the server's current date — i.e.
   the "aktif until slot end-time" window works), `GET
   /dashboard/student-attendance-summary`, `GET
   /dashboard/teacher-attendance-summary`, `GET /admin/student-attendance`,
   `GET /admin/teacher-attendance`, `GET
   /admin/student-attendance/export` (CSV content verified).
8. Report-service integration: `GET /students/{id}/attendance-summary`
   called with `X-Internal-Key: change-me` and no JWT —
   correct aggregate counts returned.
9. Negative/security tests: guru JWT hitting an admin-only route -> 403;
   no token -> 401; a second guru trying to check into / edit the agenda of
   a schedule slot already owned by another guru -> 403 both times; a siswa
   JWT hitting the guru check-in endpoint -> 403; the report-service
   endpoint with neither a valid JWT nor the internal key -> 401.
10. `docker compose down` — clean teardown, containers/network removed.

## Wali Kelas role (Bug Report SEKOCI GPM 5 Agustus 2026)

`active_role == "walikelas"` was added to `requireRole` on
`/admin/student-attendance`, `/admin/student-attendance/summary`,
`/dashboard/active-teachers`, and `/dashboard/student-attendance-summary` —
their read-only "Presensi Siswa" recap, "Guru Sedang Mengajar" widget, and
per-semester attendance charts. Unlike admin/superadmin (unfiltered or
optionally filtered), a walikelas caller MUST pass `?classroom_id=` and it
is independently re-verified against academic-service
(`requireOwnClassroom` in handler.go, via `GET /classrooms/{id}` — the
existing `ClassroomSummary` client struct now also decodes
`homeroom_teacher_user_id`) rather than trusted from the query string —
`403` on a classroom they don't own. `DashboardStudentSummary` gained a
`classroomID` filter param for this (empty string = unfiltered, admin's
prior behavior unchanged).

## Assumptions

Ambiguities in the spec were resolved as follows, simplest-reasonable-call
per the task instructions (no other agent to ask, working in parallel):

- **`ClassSession` keyed on `(schedule_id, session_date)`, upserted on
  check-in.** A guru re-hitting check-in for the same slot/day is treated as
  idempotent (returns the existing row); a *different* guru hitting an
  already-checked-in slot is rejected with 403 ("schedule slot is not
  theirs"). The spec doesn't say what happens on a schedule mix-up (e.g. two
  teachers assigned by mistake) — first check-in wins.
- **Cross-service enrichment is best-effort, not a hard dependency.**
  `identity`/`academic`-service may not be running yet (all 6 services are
  being built in parallel, this one first). Name/roster/schedule lookups
  degrade gracefully: `GET /sessions/{id}/roster` falls back to whatever
  attendance is already recorded if academic-service is unreachable;
  `GET /dashboard/active-teachers` just omits `guru_name`/`classroom_name`
  if identity/academic can't be reached. Only the **check-in** endpoint has a
  harder dependency: it *tries* `GET /schedules/{id}` on academic-service to
  resolve `classroom_id`/`slot_start_time`/`slot_end_time`/`term_id`, but
  falls back to the same fields supplied directly in the request body if
  that call fails — since the guru's own client already has the schedule
  detail on screen to render the check-in button in the first place. This
  keeps the service testable/usable standalone before academic-service
  exists, without silently trusting the client's word about ownership: the
  check-in still checks `sched.GuruID == claims.UserID` and rejects with 403
  if academic-service *is* reachable and disagrees.
- **`academic_year_id` is required, not inferred.** attendance-service
  doesn't own `academic_years` and has no notion of "the school's current
  year" (that's academic-service's data). Per CONVENTIONS.md, admin GET
  endpoints default to "no year filter" (all years) rather than guessing;
  the guru check-in endpoint *requires* `?academic_year_id=` (or an
  `X-Academic-Year-Id` header) explicitly, since `academic_year_id` is
  `NOT NULL` on `class_sessions` and there's nowhere safe to default it
  from. The frontend is expected to resolve "current year" from
  academic-service and pass it through.
- **`term_id` is optional and denormalized.** Added to both tables (not
  strictly required by CONVENTIONS.md's academic-year rule, which only
  mandates `academic_year_id`) because the report-service integration
  endpoint explicitly needs a *per-semester* summary, and a school year
  typically has two terms. It's populated from academic-service's schedule
  lookup at check-in time when available, or passed directly in the
  check-in request body otherwise.
- **`/students/{id}/attendance-summary` accepts either
  `X-Internal-Key` or a normal JWT**, via
  `common.InternalOrJWTMiddleware` (in addition to, not instead of, the two
  snippets pasted verbatim from CONVENTIONS.md into `auth.go`/`response.go`).
  The task instructions say to mount the JWT middleware on "all routes
  except /healthz," but CONVENTIONS.md's cross-service section also says
  report-service reaches other services via `X-Internal-Key`, which implies
  a backend-to-backend call with no end-user JWT in hand. Supporting both on
  this one route satisfies both statements: report-service can call it
  directly with the shared key, while a logged-in admin/guru/the student
  themself can also hit it directly with their own token. When called via
  the internal key, `common.FromCtx(r)` returns `nil` — handlers on that
  route are written to tolerate that.
- **"Aktif" window** = checked in today (`session_date = CURRENT_DATE`,
  server/DB time) AND `slot_end_time` (if known) hasn't passed yet. If a
  guru never supplies/academic-service never resolves a `slot_end_time`,
  the session is treated as active all day once checked in (fail-open, since
  under-reporting "who's teaching" seems worse than a stale entry lingering
  for a few extra hours in a local-dev context).
- **Day-of-week filter (`?day=`)** accepts either English (`Monday`) or
  Indonesian (`Senin`) weekday names; Indonesian names are mapped to English
  before being compared against Postgres' `to_char(date,'FMDay')`, since the
  container runs under the `C`/English locale.
- **CSV export only**, no PDF generation — the task explicitly says "CSV/PDF
  export endpoint is enough, actual print formatting can be left to the
  frontend," so only CSV was implemented (simpler, no extra PDF library
  dependency, and the frontend can print a CSV-driven table just as well as
  a server-rendered PDF for this stage of the rewrite).
- **Bentuk Kegiatan** stored as a JSONB array of five fixed slugs (`teori`,
  `praktikum`, `formatif`, `sumatif_tengah_semester`,
  `sumatif_akhir_jenjang`) validated server-side against that fixed set,
  matching the checkbox options named verbatim in the spec.
- **Teacher "attendance status"** (sakit/izin/tanpa_keterangan) is
  student-only per the spec's wording — a guru's own presensi is binary
  (checked in or not), so `/dashboard/teacher-attendance-summary` only has a
  meaningful `hadir` (check-in count) bucket; the other three status buckets
  are always `0` in that response, kept in the same `MonthlyStatusCount`
  shape as the student chart purely so both dashboard chart endpoints are
  drop-in interchangeable on the frontend.
- **Pergantian guru reassigns `guru_id` itself** (2026-08-11, Sekolah GPM
  request), rather than adding a separate `substitute_guru_id` column that
  every admin/dashboard query would then need to know to also check.
  `original_guru_id`/`substituted_at`/`substituted_by` are the audit trail;
  `guru_id` is always "whoever is actually covering this session right now",
  so every existing consumer of `class_sessions.guru_id` (admin's Presensi
  Guru, the dashboard's Guru Aktif Mengajar, `/me/sessions/today`) picks up
  the substitute automatically with zero changes. The tradeoff:
  `AssignSubstitute` reuses `CheckIn`'s exact academic-service
  resolve-with-fallback pattern, so it *also* fails open (trusts the caller
  is the slot's assigned guru) when academic-service is unreachable — same
  documented limitation `CheckIn` already has, not a new one.
- **The substitute still does their own check-in.** `AssignSubstitute` only
  records who's covering the slot (creating a `class_sessions` row with no
  `checked_in_at` if none existed yet); `CheckIn`'s existing
  "schedule slot is not assigned to this guru" 403 gained one extra
  condition — it's skipped when a `class_sessions` row already names the
  caller as `guru_id` for that `(schedule_id, session_date)`, i.e. a
  substitution was already recorded for them.
- **`catatan`/`tugas_diberikan`/`tugas_jenis` live on the agenda step**
  (`PUT /sessions/{id}/agenda`), not a separate endpoint — the client's
  examples for both ("ada tugas yang harus diselesaikan hari itu juga", and
  the tugas checkbox) are session-level facts recorded at the same point in
  the guru's flow as `pertemuan`/`pembahasan`/`bentuk_kegiatan` already are,
  so extending that one PUT avoids a parallel form/request just for two more
  fields. `tugas_jenis` is only persisted when `tugas_diberikan` is true
  (silently cleared otherwise) since an orphaned type with no task given
  isn't meaningful.
- Go module uses `go 1.25` (not the exact local toolchain `1.26.4`) so the
  Docker build can use the published `golang:1.25-alpine` image without
  triggering an in-container toolchain auto-download.
