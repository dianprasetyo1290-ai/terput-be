# exam-service (Sekoci CBT / Tes Basis Komputer engine)

Go microservice owning bank soal, exam authoring, vouchers, and the
anti-cheat attempt state machine for the Sekoci Go+Vue rewrite. Independent
module, own Postgres (`skci_exam`), port `8084`. Built against
[`../../docs/CONVENTIONS.md`](../../docs/CONVENTIONS.md) — same JWT claims,
response envelope, multi-tenancy (`school_id`) and academic-year scoping
rules as the other 5 services.

## Run locally

### Docker Compose (recommended)

```bash
cd services/exam
docker compose up -d --build   # starts postgres, runs migrations (migrate/migrate image), then the app
curl http://localhost:8084/healthz
docker compose down            # stop everything (add -v to also drop the pgdata volume)
```

### Manual (Go directly, Postgres elsewhere)

```bash
cp .env.example .env           # edit DATABASE_URL etc. as needed
createdb skci_exam              # or run against your own Postgres
migrate -path migrations -database "$DATABASE_URL" up   # golang-migrate CLI
go run ./cmd/server
```

### Minting a test JWT

identity-service issues real tokens; for standalone curl-testing this
service without it running, use the bundled dev helper (mirrors the
`cmd/mintjwt` pattern used by the sibling academic-service):

```bash
go run ./cmd/mintjwt admin   # or: guru / siswa / superadmin
# optional extra args: go run ./cmd/mintjwt guru <user_id> <school_id>
```

## Data model

`questions` (Bank Soal, one row per soal) live inside `question_folders`
(Kategori), and are attached to one or more `exams` via the `exam_questions`
join table (which carries the exam-specific display order and the
score-normalized point value — see "Score normalization" below). This keeps
soal reusable across exams and makes the guru-ownership rule a single
`WHERE created_by = ?` on `questions`.

```
exam_types            question_folders           exams
    |                        |                      |
    | exam_type_id           | folder_id            |
    v                        v                      v
  exams  ------------->  exam_questions  <--------  questions --> answer_options (PG/BS)
    |                        |                      \--> essay_answer_options (essay/tebak_gambar)
    | exam_id                | exam_question_id
    v                        v
exam_tokens (voucher)   exam_attempt_answers
    |                        ^
    | token_id               | attempt_id
    v                        |
exam_attempts -----------------
```

Every table carries `school_id` + `academic_year_id` per CONVENTIONS.md.

## Attempt state machine

```
                     (voucher bulk-/single-issued)
                                |
                                v
                         [belum_mulai] <---------------------+
                          |         |                        |
             admin Larang |         | siswa POST /enter      | admin Buka Larangan
             Ujian        |         | (starts, builds        |
             (Alasan      |         |  question_order)       |
              required)   v         v                        |
                     [dilarang]   [aktif] <--------+          |
                          ^          |  ^           |          |
                          |          |  | guru/admin| siswa/system
                          +----------+  | Buka      | Ulangi/  
                                        | Blokir    | Buka Blokir
                     siswa focus-lost   | ("Ulangi")|
                     event OR admin/guru|           |
                     Blokir (manual)    |           |
                                        v           |
                                    [blokir] --------+
                                        
                     siswa POST /attempt/submit (from aktif)
                                        |
                                        v
                                   [selesai]
```

| State | Meaning | Entered via | Left via |
|---|---|---|---|
| `belum_mulai` | Voucher issued, student hasn't opened the exam (or a `dilarang` ban was lifted) | Token (bulk-)issuance; `Buka Larangan` | `POST /exams/{id}/enter` (siswa) → `aktif`; `Larang Ujian` (admin) → `dilarang` |
| `aktif` | Student is actively answering | `enter` (first time or resume); `Buka Blokir`/"Ulangi" | `focus-lost` event or manual `Blokir` → `blokir`; `submit` → `selesai` |
| `blokir` | Blocked (anti-cheat auto, or admin/guru manual mid-exam) | Focus-lost event (student client); `POST .../blokir` (admin/guru) | `POST .../buka-blokir` ("Ulangi", admin/guru) → `aktif`, resuming at `current_position` with all prior answers intact |
| `dilarang` | Banned from ever starting, admin-only, only valid pre-start | `POST .../larang` (admin, requires `alasan`) — **only from `belum_mulai`** | `POST .../buka-larangan` (admin) → `belum_mulai`, same voucher re-enabled, no regeneration |
| `selesai` | Done, graded | `POST /exams/{id}/attempt/submit` (siswa, from `aktif`) | terminal |

Every already-submitted `exam_attempt_answers` row is preserved across every
transition above — block/unblock never deletes or resets answers, and
`current_position` (the resume pointer) only ever advances forward when an
answer is submitted, so a student blocked while viewing question 6 resumes
exactly at question 6.

## Score normalization

Guru enters a raw `poin` per soal (bank-soal level). When an exam is
published (`POST /exams/{id}/publish`), `exam_questions.points` for every
attached question is proportionally rescaled so the exam's total is exactly
100: `scaled_i = round2(raw_i / sum(raw) * 100)`, with the **last** question
(by `order_index`) absorbing the rounding residue (`100 - sum of the
already-scaled ones`) so the total is always exactly `100.00`, never
`99.98`/`100.02`. The exam is locked (no further question add/remove/import)
once published. Verified in testing: two soal at raw `10`/`10` → published as
`50.00`/`50.00`.

## Shuffle (Acak Soal)

When `exams.is_shuffled = true`, each student's `exam_attempts.question_order`
(a JSON array of `exam_questions.id`) is built as an independent random
permutation the first time they enter. **Answer-option order within a
question is never shuffled** (`answer_options.order_index` is fixed at
creation). The number shown to the student (`GET
/exams/{id}/attempt/questions/{position}`'s `no` field) is always the
1-indexed position in *that student's* `question_order` array — i.e. always
sequential 1..N regardless of which underlying question sits there.

## Key endpoints

See `internal/handler/router.go` for the full list. Highlights:

- `POST/GET/PUT/DELETE /exam-types` — Kelola Tipe Ujian (admin CRUD, guru read-only via GET)
- `POST/GET/PUT/DELETE /question-folders` — Bank Soal Kategori (admin)
- `POST/GET /question-folders/{folderId}/questions` — soal inside a folder; guru's GET is scoped to `created_by = self`, admin sees all
- `GET /question-import-template?type=` / `POST /exams/{id}/questions/import` — CSV (and `.xlsx` via excelize) bulk soal import
- `POST/GET/PUT/DELETE /exams`, `POST /exams/{id}/questions`, `POST /exams/{id}/publish`
- `POST /exams/{id}/tokens`, `POST /exams/{id}/tokens/bulk`, `POST /tokens/{tokenId}/regenerate`
- `GET /exams/{id}/monitor?classroom_id=` — live roster + 4 counters, always computed fresh
- `GET /exams/{id}/attempts/{attemptId}` — detail/correction view
- `POST /exams/{id}/attempts/{attemptId}/{larang|buka-larangan|blokir|buka-blokir}`
- `PATCH /exams/{id}/attempts/{attemptId}/answers/{answerId}` — essay-only manual override
- `GET /exams/{id}/scores` — report-service integration point
- `GET /me/exams?status=today|upcoming|history`, `POST /exams/{id}/enter`, `GET /exams/{id}/attempt`, `GET /exams/{id}/attempt/questions/{position}`, `POST /exams/{id}/attempt/answers`, `POST /exams/{id}/attempt/submit`, `POST /exams/{id}/attempt/focus-lost`

## curl walkthrough

All commands assume `BASE=http://localhost:8084` and tokens minted via
`cmd/mintjwt` as `$ADMIN` / `$GURU` / `$SISWA`.

```bash
# 1. Kelola Tipe Ujian
curl -s -X POST $BASE/exam-types -H "Authorization: Bearer $ADMIN" -H "Content-Type: application/json" \
  -d '{"academic_year_id":"<ay-uuid>","name":"Sumatif Akhir Semester","mapping_rapor":"sas"}'

# 2. Bank Soal: folder (admin), then a pilihan_ganda soal (guru)
curl -s -X POST $BASE/question-folders -H "Authorization: Bearer $ADMIN" -H "Content-Type: application/json" \
  -d '{"academic_year_id":"<ay-uuid>","name":"UJIAN SUMATIF AKHIR JENJANG 2026/2027 Kelas X"}'

curl -s -X POST $BASE/question-folders/<folder-id>/questions -H "Authorization: Bearer $GURU" -H "Content-Type: application/json" -d '{
  "academic_year_id": "<ay-uuid>", "question_type": "pilihan_ganda", "content": "Ibukota Indonesia adalah?",
  "poin": 10,
  "answer_options": [
    {"content":"Jakarta","is_correct":true}, {"content":"Bandung"}, {"content":"Surabaya"}, {"content":"Medan"}, {"content":"Semarang"}
  ]
}'

# 3. Buat Ujian Baru (guru), attach the soal, publish (normalizes points)
curl -s -X POST $BASE/exams -H "Authorization: Bearer $GURU" -H "Content-Type: application/json" -d '{
  "academic_year_id":"<ay-uuid>", "term_id":"<term-uuid>", "exam_type_id":"<exam-type-uuid>",
  "title":"Sumatif Akhir Semester - IPS Kelas X", "classroom_id":"<classroom-uuid>", "subject_id":"<subject-uuid>",
  "start_at":"2026-08-01T08:00:00Z", "end_at":"2026-08-01T10:00:00Z", "is_shuffled": true
}'
curl -s -X POST $BASE/exams/<exam-id>/questions -H "Authorization: Bearer $GURU" -H "Content-Type: application/json" -d '{"question_id":"<question-id>"}'
curl -s -X POST $BASE/exams/<exam-id>/publish -H "Authorization: Bearer $GURU"

# 4. Generate a voucher for a student
curl -s -X POST $BASE/exams/<exam-id>/tokens -H "Authorization: Bearer $GURU" -H "Content-Type: application/json" \
  -d '{"user_id":"<student-uuid>","classroom_id":"<classroom-uuid>"}'

# 5. Student enters with the voucher code
curl -s -X POST $BASE/exams/<exam-id>/enter -H "Authorization: Bearer $SISWA" -H "Content-Type: application/json" -d '{"code":"<code>"}'

# 6. Student submits an answer to question 1
curl -s $BASE/exams/<exam-id>/attempt/questions/1 -H "Authorization: Bearer $SISWA"   # fetch options (no answer key leaked)
curl -s -X POST $BASE/exams/<exam-id>/attempt/answers -H "Authorization: Bearer $SISWA" -H "Content-Type: application/json" \
  -d '{"exam_question_id":"<eq-id>","selected_option_id":"<option-id>"}'

# 7. Anti-cheat: focus-lost auto-blocks
curl -s -X POST $BASE/exams/<exam-id>/attempt/focus-lost -H "Authorization: Bearer $SISWA"

# 8. Admin/guru unblocks ("Ulangi") — resumes without wiping answers
curl -s -X POST $BASE/exams/<exam-id>/attempts/<attempt-id>/buka-blokir -H "Authorization: Bearer $GURU"

# 9. Live monitor (roster + 4 counters)
curl -s "$BASE/exams/<exam-id>/monitor" -H "Authorization: Bearer $GURU"
```

## Testing performed (this session)

Ran the full stack via `docker compose up -d --build` (Postgres 16 +
`migrate/migrate` one-shot migration container + the Go app), then exercised
end-to-end with curl using JWTs from `cmd/mintjwt`:

1. Created an exam type (admin), a question folder (admin), a `pilihan_ganda`
   soal and an `essay` soal (both as guru).
2. Verified the guru-only-sees-own-soal rule: a second guru's `GET
   /question-folders/{id}/questions` returned an empty list for the first
   guru's soal, while admin's same call returned both.
3. Created a draft exam (guru), attached both soal (raw poin 10 + 10),
   published it — confirmed `exam_questions.points` was rescaled to exactly
   `50.00`/`50.00` (sum 100).
4. Bulk-issued a voucher for a classroom/student list; confirmed an
   `exam_attempts` row was pre-created in `belum_mulai`.
5. Confirmed the siswa dashboard (`GET /me/exams?status=today`) surfaced the
   exam once its window opened.
6. Student entered with the voucher code → `aktif`, `question_order` built.
7. Fetched question 1 as the student — confirmed answer options came back
   **without** `is_correct` (found and fixed a real bug here: the first pass
   of `GetExamQuestion` didn't join `answer_options`/`essay_answer_options` at
   all, so nothing was returned — fixed in `internal/store/exams.go`).
8. Submitted a correct pilihan_ganda answer, then fired `focus-lost` — attempt
   auto-transitioned to `blokir` with `current_position` pointing at the next
   unanswered question; confirmed a further answer-submit attempt was
   rejected with 409.
9. Admin `monitor` endpoint showed the blocked student correctly bucketed
   under `sedang_ujian` (1/1).
10. `Buka Blokir` ("Ulangi") resumed to `aktif` without touching the already-
    saved Q1 answer; submitted the essay answer with mismatched
    case/whitespace (`"  tokyo "` vs opsi `"Tokyo"`) and confirmed
    case-insensitive/trim auto-grading marked it correct.
11. Submitted the exam — `score: 100`, `correct_count: 2`.
12. Confirmed `monitor` counters flipped to `selesai: 1` and the roster row
    showed `jawaban_benar: "2/2"`, `nilai: 100`.
13. Hit `GET /exams/{id}/scores` (report-service-facing) — confirmed it
    returns `mapping_rapor` from the exam's exam_type alongside per-student
    scores.
14. Tested `Larang Ujian` on a second (not-yet-started) student: required
    `alasan` enforced, exact spec message returned, guru correctly forbidden
    (403, admin-only), then `Buka Larangan` reversed it back to
    `belum_mulai` re-using the same voucher code.
15. Tested the essay manual-override endpoint (`PATCH .../answers/{answerId}`)
    — flipped a correct essay answer to incorrect; confirmed the same
    endpoint's underlying check would reject a `tebak_gambar` answer (code
    path verified by inspection: `eq.Question.QuestionType != "essay"` guard).
16. Downloaded the CSV import template for `pilihan_ganda` and `tebak_gambar`,
    confirmed a bad `type` query param 400s.
17. Round-tripped the downloaded `pilihan_ganda` template through `POST
    /exams/{id}/questions/import` on a second draft exam — confirmed the
    `correct_option: b` column correctly flagged `option_b`
    (`is_correct: true`) among the 5 created answer_options, and the question
    was attached to the exam.
18. Ran `go build ./...` and `go vet ./...` clean at every stage; `docker
    compose down` at the end left no dangling containers.

Not separately exercised in this pass (lower-risk, same code path as CSV):
`.xlsx` import via `excelize.OpenReader` — the parser is wired and the
library builds/links correctly, but no `.xlsx` fixture was round-tripped in
this session.

## Assumptions

1. **Cross-service IDs are trusted, not validated.** `classroom_id`,
   `subject_id`, `term_id`, `exam_type_id`, and `academic_year_id` on exams
   (and `folder`/`question` academic-year scoping) are taken as-is from the
   caller (admin-web is expected to source them from identity/academic's own
   dropdown endpoints). Per the task's own suggested simplification, guru
   "picks a kelas they teach" is **not** round-tripped against
   academic-service's classroom-subject-teacher matrix — only JWT
   `active_role`/`school_id` + the passed IDs are trusted.
2. **No "current academic year" resolution.** exam-service doesn't own
   `academic_years` (identity does) and doesn't call out to resolve "current"
   — every list endpoint that's academic-year-scoped requires an explicit
   `?academic_year_id=` query param; there's no server-side default.
3. **Bank Soal is a reusable table, not exam-exclusive.** `questions` live in
   `question_folders` and are attached to one or more exams via the
   `exam_questions` join (which carries the exam-specific display order and
   the score-normalized `points`). This is a normalization choice — the spec
   doesn't say explicitly whether a soal can be reused across exams — but it
   keeps "guru only sees their own soal" a single `created_by` predicate and
   avoids duplicating soal content per exam.
4. **Score normalization**: proportional rescale of raw `poin` to sum to
   exactly 100 on publish (`scaled_i = round(raw_i/sum*100, 2)`), last
   question absorbs rounding residue. Exam locks (no add/remove/import
   questions) once published.
5. **Attempt rows are pre-created at voucher-issue time** (single or bulk),
   in `belum_mulai`, specifically so `Larang Ujian` has something to act on
   before a student ever opens the exam, and so the live monitor roster can
   list "belum mengerjakan" students. A student who was never issued a
   voucher never appears anywhere in exam-service (no row to show).
6. **Bulk voucher issuance takes an explicit `student_ids` array** in the
   request body rather than exam-service calling out to academic-service's
   roster endpoint itself — keeps this service's write path independent of
   academic-service's availability/route shape; admin-web is expected to
   source the list from academic-service's classroom roster.
7. **Monitor counters/roster are computed only from `exam_attempts` rows for
   that classroom** — i.e. only students who hold a voucher. They are *not*
   reconciled against academic-service's actual classroom headcount (would
   require a live cross-service call this service doesn't make). In practice
   this reconciles correctly as long as bulk-generate was run against the
   full classroom roster, which is the expected admin-web flow.
8. **The 4 monitor counters group 5 states into 4 buckets**: `blokir` is
   counted inside `sedang_ujian` alongside `aktif` (spec lists only 4
   counter labels — Belum mengerjakan / Sedang Ujian / Selesai / Dilarang
   Ujian — for a 5-state machine), since a blocked student is still
   mid-attempt, just paused pending guru/admin action.
9. **Essay auto-grading**: case-insensitive, whitespace-trimmed exact match
   against any of the guru's `essay_options`. No fuzzy/partial matching.
10. **Manual override is essay-only**, enforced server-side (`PATCH
    .../answers/{answerId}` 400s if the underlying question isn't `essay`),
    per spec's explicit tebak_gambar exclusion.
11. **`current_position`** is the 0-indexed pointer to the next
    question to resume at; it advances (monotonically, never regresses) only
    when an answer is submitted for the question at that position, which is
    what makes "blocked while viewing Q6 resumes at Q6" work — the pointer
    only moves past a question once it's actually answered.
12. **CSV/XLSX import** auto-creates (or reuses) a folder named `"Import -
    <exam title>"` owned by the calling guru/admin when no `folder_id` form
    field is supplied. `.xlsx` support uses `github.com/xuri/excelize`
    reading the first sheet's rows; `.csv` uses the stdlib `encoding/csv`.
13. **Identity-service student-name lookups are best-effort.** The monitor
    roster calls `GET {IDENTITY_URL}/internal/users/{id}` with a 2s timeout
    and silently falls back to an empty `nama_siswa` string on any error
    (service down, route not yet built by the sibling agent, timeout) —
    exam-service's own state never depends on this call succeeding.
14. **`question_order` is stored as JSONB**, not a native Postgres `uuid[]`,
    for reliable scanning through plain `database/sql` (`pgx/v5/stdlib`)
    without extra driver-side array configuration.
15. **Regenerating a token never touches the linked attempt** — only the code
    (and optionally the expiry) changes; per spec "admin can regenerate"
    without any other stated side effect.
16. **`total_points`/`question_count`** on an exam are always computed
    live via a `SUM`/`COUNT` subquery at read time, never cached, matching
    the same "always fresh, never stale" requirement called out for the
    monitor counters.
17. **`question_type` is immutable** after a soal is created (an `UpdateQuestion`
    request's `question_type` is ignored/pinned to the existing value),
    since `answer_options` vs `essay_answer_options` shapes are type-specific
    and changing type would leave orphaned option rows.
18. Response envelope, JWT middleware, and port/DB-name are copied verbatim
    from `docs/CONVENTIONS.md` and were not modified.
