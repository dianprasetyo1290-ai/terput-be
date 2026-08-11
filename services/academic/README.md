# academic-service

Owns everything "under" an academic year in the Sekoci rewrite: terms/semesters,
classrooms (Data Kelas), subjects (Data Mata Pelajaran), the
classroom×subject×guru mapping (+ the "Matriks Pelajaran" enable/disable
flag), schedules (Jadwal Pelajaran), class enrollments, and grade promotion
(Kenaikan Kelas). Port `8082`, DB `skci_academic`. See
`../../docs/CONVENTIONS.md` for the shared JWT/envelope/multi-tenancy rules
this service follows.

identity-service owns `schools`, `users`, and a bare `academic_years` table
(id, school_id, label, is_current). This service references those by UUID
only — no cross-DB foreign keys — and resolves details (a user's name, the
school's current academic year) via internal HTTP calls documented in
`internal/client/identity.go`.

## Running locally

```bash
cp .env.example .env   # only needed if you run `go run ./cmd/server` outside docker
docker compose up -d --build

# apply migrations (golang-migrate CLI isn't vendored into the image; the
# simplest local path is to pipe the SQL straight into the postgres
# container — equivalent to `migrate -path migrations -database "$DATABASE_URL" up`
# if you have the `migrate` CLI installed):
docker exec -i skci_academic_pg psql -U sekoci -d skci_academic < migrations/0001_init.up.sql

curl http://localhost:8082/healthz
# {"status":"ok"}
```

To tear down: `docker compose down` (add `-v` to also drop the postgres
volume).

### Minting a test JWT

There's no login here (identity-service issues tokens); for local testing
without identity running, `cmd/mintjwt` is a small dev-only helper that
signs a token with the shared dev secret (`change-me`).
It is **not** part of the server image (the Dockerfile only builds
`./cmd/server`).

```bash
go run ./cmd/mintjwt admin
# eyJhbGciOi...   (active_role=admin, school_id=22222222-2222-2222-2222-222222222222)

go run ./cmd/mintjwt guru <guru-user-id>
# mints a token for that specific user id with active_role=guru — useful for
# testing GET /me/schedule
```

## Example curls

```bash
TOKEN=$(go run ./cmd/mintjwt admin)
AY=33333333-3333-3333-3333-333333333333   # a fake academic_year_id for testing

# 1. Bootstrap Semester 1 + Semester 2 for an academic year
curl -X POST "http://localhost:8082/academic-years/$AY/terms/bootstrap" \
  -H "Authorization: Bearer $TOKEN"

# 2. Create a classroom
curl -X POST "http://localhost:8082/classrooms?academic_year_id=$AY" \
  -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" \
  -d '{"name":"X IPA 1","grade_level":"10"}'
# => note the returned "id" as $CLASSROOM_ID

# 3. Create a subject
curl -X POST "http://localhost:8082/subjects" \
  -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" \
  -d '{"name":"Matematika"}'
# => note the returned "id" as $SUBJECT_ID

# 4. Assign the subject + a guru to the classroom (Data Kelas mapping)
curl -X POST "http://localhost:8082/classrooms/$CLASSROOM_ID/subjects" \
  -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" \
  -d "{\"subject_id\":\"$SUBJECT_ID\",\"teacher_user_id\":\"<guru-user-id>\"}"
# => note the returned "id" as $CS_ID (classroom_subject_id)

# 5. Create a jam-pelajaran slot for that classroom (Monday, period 1)
curl -X POST "http://localhost:8082/classrooms/$CLASSROOM_ID/schedule-periods" \
  -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" \
  -d '{"day_of_week":1,"period_no":1,"start_time":"07:00","end_time":"08:00"}'
# => note the returned "id" as $SP_ID

# 6. Put the subject into that slot (Jadwal Pelajaran)
curl -X POST "http://localhost:8082/classrooms/$CLASSROOM_ID/schedules" \
  -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" \
  -d "{\"schedule_period_id\":\"$SP_ID\",\"classroom_subject_id\":\"$CS_ID\"}"
```

## Endpoints

All routes below require `Authorization: Bearer <jwt>` except `/healthz`.
List/detail endpoints are scoped to the caller's `school_id`
(superadmin can override via `?school_id=`) and, where noted, to
`academic_year_id` (defaults to the school's current year — see
"Assumptions" below for exactly how that default is resolved).

| Method | Path | Notes |
|---|---|---|
| GET | `/healthz` | no auth |
| POST | `/academic-years/{academic_year_id}/terms/bootstrap` | creates Semester 1 + 2 |
| GET | `/terms?academic_year_id=` | |
| GET | `/terms/{id}` | |
| PUT | `/terms/{id}` | edit name/start_date/end_date |
| POST | `/classrooms?academic_year_id=` | |
| GET | `/classrooms?academic_year_id=&page=&per_page=` | |
| GET / PUT / DELETE | `/classrooms/{id}` | PUT edits name/grade_level/wali kelas |
| POST | `/classrooms/{id}/students` | `{student_user_id}` |
| GET | `/classrooms/{id}/students` | roster |
| DELETE | `/classrooms/{id}/students/{student_user_id}` | |
| POST | `/classrooms/{id}/subjects` | `{subject_id, teacher_user_id, is_active}` |
| GET | `/classrooms/{id}/subjects` | |
| GET | `/classrooms/{id}/subject-matrix` | same data, "Matriks Pelajaran" naming; exam-service consumes this |
| PUT / DELETE | `/classroom-subjects/{id}` | edit teacher / toggle `is_active` |
| GET | `/wali-kelas?academic_year_id=` | derived from `classrooms.homeroom_teacher_user_id`, not separately stored |
| POST | `/subjects` | |
| GET | `/subjects?page=&per_page=` | |
| GET / PUT / DELETE | `/subjects/{id}` | |
| POST | `/classrooms/{id}/schedule-periods` | jam pelajaran |
| GET | `/classrooms/{id}/schedule-periods?day=` | |
| PUT / DELETE | `/schedule-periods/{id}` | |
| POST | `/classrooms/{id}/schedules` | jadwal slot; rejects `classroom_subject_id` not assigned to this classroom |
| GET | `/classrooms/{id}/schedules?day=` | |
| PUT / DELETE | `/schedules/{id}` | |
| GET | `/me/schedule?day=` | caller's own teaching schedule (guru) |
| POST | `/students/{id}/subject-enrollments` | `{classroom_subject_id, is_enrolled}` — resolves elective variant |
| GET | `/students/{id}/active-subjects?academic_year_id=` | for exam-service |
| POST | `/grade-promotion` | kenaikan kelas, see below |
| GET | `/dashboard/counts?academic_year_id=` | `{siswa, guru, kelas}` |

### Grade promotion (Kenaikan Kelas)

```json
POST /grade-promotion
{
  "source_academic_year_id": "...",
  "target_academic_year_id": "...",
  "mappings": [
    {"from_classroom_id": "X-IPA-1-id", "to_classroom_id": "XI-IPA-1-id"}
  ]
}
```

For each mapping, copies the source classroom's active roster into new
`class_enrollments` rows under the target classroom/year. Graduating classes
(12/9) are excluded simply by omitting them from `mappings` — the source
year's data is never modified or deleted, so it stays intact and isolated
from the new year (this is the client's core pain point being fixed).

## Wali Kelas role (Bug Report SEKOCI GPM 5 Agustus 2026)

`active_role == "walikelas"` can now call the same admin-facing endpoints a
few of Jadwal Kelas/Matriks Pelajaran/Kenaikan Kelas already exposed —
`GET /classrooms/{id}/schedules`, `GET /classrooms/{id}/schedule-periods`,
`GET/POST /classrooms/{id}/subject-matrix*`, `POST /grade-promotion` — but
every one of them is now guarded to their own homeroom classroom via
`requireOwnClassroomIfWaliKelas` (handler.go), resolved live against
`classrooms.homeroom_teacher_user_id` (still the only place this assignment
is stored, see the existing Wali Kelas section below). Any other
`active_role` is completely unaffected — the guard no-ops for them. A
walikelas request naming a classroom they don't own gets `403`.
`POST /grade-promotion` checks every distinct `from_classroom_id` in the
payload, not just one, since a wali kelas processes their whole roster in
one call.

## Assumptions

- **Flattened classroom model.** The reference Laravel schema splits
  `classrooms` (grade grouping, e.g. "Kelas 10") from `sub_classrooms`
  (sections, e.g. "10 IPA 1"). This service uses a single `classrooms` table
  (e.g. name = "X IPA 1" directly) since the spec's Data Kelas flow ("admin
  creates classroom, names it, assigns wali kelas...") describes one
  create-a-classroom step, not two nested ones. `grade_level` is a plain text
  column for filtering/reporting (e.g. to spot grade 9/12 at promotion time),
  not a separate table.
- **Subjects are not academic-year-scoped**, only school-scoped — matching
  the reference schema (`subjects` there has no `academic_year_id` either).
  A subject name like "Matematika" persists across years; year-scoping
  happens one level down, in `classroom_subjects`.
- **Resolving the "current academic year" default.** CONVENTIONS.md says
  default list queries should filter to the school's current year via
  `?academic_year_id=` override. identity-service (built in parallel, not
  fully implemented at the time this was written) is known from its own
  migration to have `academic_years.is_current BOOLEAN`. This service's
  `internal/client/identity.go` assumes identity exposes
  `GET /academic-years?school_id=<id>` returning the envelope shape from
  CONVENTIONS.md with `is_current` per row, and picks the row where
  `is_current = true`. If identity is unreachable, returns non-2xx, or has no
  current year for that school, every endpoint that needs a default
  `academic_year_id` **fails closed with 400** asking the caller to pass
  `?academic_year_id=` explicitly, rather than guessing — verified in step
  19 of end-to-end testing below.
- **Term bootstrap is admin-triggered, not event-driven.** Rather than
  identity-service pushing a webhook/event when an academic year is created,
  admin (or identity-service itself, via an internal HTTP call) explicitly
  calls `POST /academic-years/{id}/terms/bootstrap` after creating the year.
  Simpler for local dev with no message bus; idempotent (`ON CONFLICT DO
  NOTHING` on `(academic_year_id, order)`, so calling it twice is safe).
- **classroom_subjects.is_active doubles as the subject matrix.** Rather than
  a separate matrix table, "Matriks Pelajaran" enable/disable is the same
  `is_active` flag already on the classroom↔subject↔guru mapping — one less
  table, and `GET /classrooms/{id}/subject-matrix` is just a differently-named
  view of `GET /classrooms/{id}/subjects`.
- **Religion-elective variant resolution.** `subjects.subject_group` (e.g.
  "Agama") tags mutually-exclusive subject variants. `classroom_subjects`
  can list several active variants for one classroom (Agama Islam, Agama
  Katolik, ...); `student_subject_enrollments` picks which variant each
  individual student is in. `GET /students/{id}/active-subjects` resolves
  this: if a student has an explicit `is_enrolled=true` row for a
  subject_group, only that variant is returned; if no student in that group
  has been explicitly split yet, all active variants are returned (so the
  feature degrades to "show everything" until someone actually does the
  split) — this fallback is a judgment call, not from the spec.
- **guru count for the dashboard** is computed locally (distinct
  `teacher_user_id` across `classroom_subjects` + `homeroom_teacher_user_id`
  across `classrooms`, for the given year) rather than asking
  identity-service, since identity doesn't scope "guru" by academic-year
  teaching assignment — that's data this service owns.
- **`schedule_periods.start_time`/`end_time` are `TEXT` ("HH:MM"), not SQL
  `TIME`.** Pure simplification to keep pgx/database-sql scanning
  straightforward (no time-of-day arithmetic happens in SQL here); validated
  as non-empty at the handler layer.
- **Hard vs soft delete.** Reference tables (`classrooms`, `subjects`,
  `classroom_subjects`) keep `deleted_at` soft-delete like the old system;
  `class_enrollments`, `schedule_periods`, `schedules`,
  `student_subject_enrollments` are hard-deleted since they're
  join/roster-style rows with no audit requirement called out in the spec.
- **No cross-DB foreign keys** to identity's `users`/`academic_years`, per
  CONVENTIONS.md — `student_user_id`, `teacher_user_id`,
  `homeroom_teacher_user_id`, and `academic_year_id` are bare UUID columns.

## What was actually tested end-to-end

Ran `docker compose up -d --build`, applied `migrations/0001_init.up.sql`
directly against the `skci_academic_pg` container via `psql`, confirmed
`GET /healthz` → `{"status":"ok"}`, then — using a JWT minted by
`cmd/mintjwt` and signed with the shared dev secret — exercised, in order:

1. `POST /academic-years/{ay}/terms/bootstrap` → created Semester 1 + 2
2. `GET /terms?academic_year_id=` → both terms returned
3. `POST /classrooms?academic_year_id=` → created "X IPA 1"
4. `PUT /classrooms/{id}` → set `homeroom_teacher_user_id` (wali kelas)
5. `GET /wali-kelas?academic_year_id=` → reflected the wali kelas immediately (no separate storage, confirmed live-derived)
6. `POST /subjects` → created "Matematika"
7. `POST /classrooms/{id}/subjects` → assigned subject + guru to the classroom
8. `POST /classrooms/{id}/schedule-periods` → created a Monday period-1 slot
9. `POST /classrooms/{id}/schedules` → assigned the subject into that slot
   9b. Negative test: same call with a random `classroom_subject_id` not
       belonging to the classroom → correctly rejected with 400
10. `POST /classrooms/{id}/students` → enrolled a student
11. `GET /students/{id}/active-subjects` → returned the assigned subject
12. `GET /classrooms/{id}/subject-matrix` → returned the matrix row
13. `GET /dashboard/counts` → `{siswa:1, guru:1, kelas:1}`
14. `GET /me/schedule?day=1` (as the guru) → returned their one teaching slot
    with subject/classroom/time joined in
15–16. Created a second classroom under a **different** fake academic year,
    then `POST /grade-promotion` from year 1 → year 2 → `students_moved: 1`
17–18. Confirmed the promoted roster exists under year 2's classroom, and
    that year 1's dashboard counts (`siswa:1`) were **unchanged** — i.e. the
    promotion was additive and the two years' data stayed isolated, which is
    the spec's central correctness requirement
19. Called a scoped list endpoint with **no** `?academic_year_id=` while
    identity-service wasn't running → got a clean `400` with an explanatory
    message instead of a crash or wrong data
20. Called an endpoint with no `Authorization` header → `401 missing token`

Then `docker compose down`. `go build ./...`, `go vet ./...`, and
`gofmt -l .` are all clean.
