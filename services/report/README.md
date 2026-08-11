# report-service

Part of the Sekoci Go+Vue rewrite. Owns E-raport (report cards) and Leger
(classroom rekap). Port `8086`, database `skci_report`. See
`../../docs/CONVENTIONS.md` for the shared JWT/response-envelope/
multi-tenancy/academic-year conventions every sibling service (identity,
academic, attendance, exam, evaluation) follows.

This is an **aggregator**, not a source of truth for student/classroom/exam
data — it owns only report-configuration/override state (`report_cards`,
`report_card_subjects`, `report_grade_bands`, `report_assessment_types`,
`report_weight_settings`, `formative_grades`, `report_templates`) and pulls
everything else (student names, classroom/term info, attendance summaries,
exam scores) from sibling services over internal HTTP. See "Expected
upstream contracts" below for exactly what it expects those services to
expose.

## Print/PDF, Leger semester+kelas cascade, Import Template (migration 0008)

This section documents the 2026-08-11 print-template session on top of
migration 0007's dynamic-report-structure work. Three things were built,
against 3 reference .xlsx files (`Format Rapor Tengah Semester (3
Bulan).xlsx`, `Format Rapor Akhir semester (6 bulan).xlsx`, its Halaman 2)
that define the real government-school report card's visual layout.

### 1. Leger: cascading "Pilih Semester" + "Pilih Kelas"

`web/src/components/report/SemesterKelasPicker.vue` (new) replaces
`ClassroomTermPicker` on `LegerView.vue` only (Generate Rapor keeps its own
existing classroom+term picker unchanged). No backend/migration change was
needed for this — it's built entirely on fields that already existed:

- "tingkat" (grade level 1/2/3) = academic-service's `Classroom.grade_level`
  (free text, e.g. `"X"/"XI"/"XII"` or `"10"/"11"/"12"`). The distinct
  `grade_level` values seen across every classroom the school has (across
  **every** academic year, not just the current one — a grade-1/2/3 cohort
  typically spans 3 different years as students are promoted), sorted
  ascending, become tingkat 1/2/3 in order. This happens to sort correctly
  for both labeling conventions above since both are prefix-ordered
  strings.
- "which half of the semester pair" = academic-service's `Term.order`
  (`1` = Ganjil, `2` = Genap) within the selected classroom's own
  `academic_year_id`. Semester 1/3/5 (odd) → order 1, 2/4/6 (even) → order
  2. `TermOption` (`report.types.ts`) gained an `order?: number` field —
  it was already in the real API JSON response, just never typed before.
- Picking Semester N computes `tingkatIndex = ceil(N/2)`, filters classrooms
  to that tingkat's `grade_level`, and once a kelas is picked, resolves
  `term_id` by fetching that classroom's terms and matching `order`.

`GET /classrooms/{id}/leger` itself is unchanged (still `?term_id=` +
`?report_type=`) — the cascade only changes how the frontend arrives at a
`classroom_id`/`term_id` pair before calling it.

### 2. Print/PDF rebuild (F4, letterhead, dynamic per-school content)

"PDF" in this codebase has always meant the browser's native
`window.print()` + `@media print` CSS (see `ReportCardDetailView.vue`,
evaluation's `LaporanCetakView.vue`) — there is no Go PDF library anywhere
in this repo, and none was added. The rebuild is a **new, separate** view,
`web/src/views/report/admin/ReportCardPrintView.vue` (route
`rapor/:id/cetak`, name `report-print`), reachable from
`ReportCardDetailView.vue`'s new "Cetak Rapor (Format Resmi)" button.
`ReportCardDetailView.vue`'s own on-screen 7-column table and its existing
"Cetak / Simpan PDF" button are **unchanged** — the task was explicit that
the on-screen edit table already matches the bug report's spec and print
rendering is a separate concern from it.

The print view matches the reference templates' structure: letterhead
(logo + name + address) → "LAPORAN HASIL BELAJAR" title → report-type
subtitle → "TAHUN AJARAN" → student info block (Nama/NISN/Nama
Sekolah/Alamat left, Fase/Kelas/Semester/Tahun Ajaran right — Fase has no
source data anywhere in the system and renders as a literal "-", not
fabricated) → "I. INTRAKURIKULER" grouped subject table → (semester type
only) "D. JUMLAH DAN RATA-RATA NILAI" summary → signature block
(date/Orang Tua-Wali/Wali Kelas/Kepala Sekolah). **F4 paper size** is set
via `@page { size: 215mm 330mm; margin: 15mm; }` inside a `@media print`
block — the only place a browser print pipeline reads page dimensions
from.

- `report_type=bayangan` (the quarterly report) renders the Tengah Semester
  layout: NO / MATA PELAJARAN / NILAI FORMATIF (30%) / NILAI PRAKTIK (40%)
  / **NILAI PTS (30%)** / NILAI AKHIR. The template's own label is "PTS"
  (Penilaian Tengah Semester), not "STS" — this is display-only, the
  underlying `sts_score` field/bucket name is unchanged.
- `report_type=semester` renders the Akhir Semester layout: NO / MATA
  PELAJARAN / NILAI AKHIR / CAPAIAN KOMPETENSI, where the two narrative
  sub-lines reuse Deskripsi Rapor's existing fields rather than adding new
  ones: **Capaian Pembelajaran = `final_note`** (the existing Capaian SAS
  narrative) and **Kemajuan Belajar = `mid_note`** (the existing Capaian
  STS narrative, read as "progress during the semester"). No new DB
  columns for this.
- Subject grouping ("A. Mata Pelajaran Umum / B. Kejuruan / ...") reuses
  academic-service's existing `subjects.subject_group` field (already used
  for religion-elective grouping) — resolved **live**, not cached, via
  `enrichSubjectGroups` in `handler/report_card.go`
  (`ReportCardSubject.SubjectGroup`, API-only, same "computed at read time,
  not stored" pattern as `GradeLabel`). Schools that haven't set a
  `subject_group` on any subject get a flat, ungrouped table instead of an
  assumed 4-way SMK split.
- **II. KOKURIKULER** is intentionally **omitted** — there is no
  co-curricular-activity data model anywhere in this codebase to render;
  building one would be a new feature, not a print-formatting change. Left
  as a `ponytail:` comment in `ReportCardPrintView.vue`.
- Halaman 2's "Konsentrasi Keahlian continuation + signature page" isn't
  reproduced as a literal second physical page — the print view renders
  every subject group in one continuous flow (the browser paginates a long
  table naturally) with `break-inside: avoid` on each group's table, which
  achieves the same effect without hardcoding a literal group-name string
  no school is guaranteed to use.

**Dynamic school info** (the multi-tenancy requirement — every school must
see its own identity, never the reference school's): `GET
/report-cards/{id}` now resolves a `school` field
(`model.SchoolInfo`: name/address/city/province/logo) **live** from
identity-service's already-internal-key-reachable `GET /schools/{id}`
(`upstream.IdentityClient.GetSchool`, `fetchSchoolInfo` in
`handler/report_card.go`) — deliberately **not** cached onto `report_cards`
like the other header-snapshot fields (student_name/classroom_name/etc.).
Reasoning: that caching pattern exists specifically to avoid multiplying
upstream calls across list/per-row reads (Leger, generate's N-student
loop); `GetReportCard` is a single-card read, so one extra call per open of
the print view is cheap, and it means every read always reflects the
school's current name/address rather than whatever was cached at the last
`generate` run. Same non-cached rationale for `SubjectGroup` above. Kepala
Sekolah (`principal_name`) and Wali Kelas (`homeroom_teacher_name`) were
already resolved dynamically per-school before this session (identity's
Organisasi Sekolah / academic's homeroom assignment) and are unchanged.
Akreditasi was **not** added — identity-service's `schools` table has no
such field, and adding one is data-entry plumbing out of scope here.

### 3. Import Template (upload/store, migration 0008)

New table `report_templates` (school_id, template_type, file_name,
content_type, file_size, file_data BYTEA, uploaded_by, uploaded_at; UNIQUE
`(school_id, template_type)`), storing the raw bytes **directly in
Postgres**, not a filesystem path or object store — no prior convention
for storing a raw uploaded file exists anywhere in this codebase (compare
`services/evaluation`'s dimensi_lulusan import, which parses an uploaded
.xlsx into DB rows immediately and never persists the file itself), and
report-service's whole architecture is otherwise "just Postgres" — adding
a filesystem volume or MinIO/S3 dependency for one small (<10MB),
rarely-written admin file-of-record per school would be new infrastructure
for very little payoff. Reconsider if templates need CDN serving or grow
past a few MB.

New endpoints (admin/superadmin write, staff read, all JWT-gated):
```
GET  /settings/report-templates                    -- list metadata (no file bytes) for this school
POST /settings/report-templates                     -- multipart: template_type + file (.xlsx, max 10MB)
GET  /settings/report-templates/{type}/download      -- raw file, admin verification/record-keeping
```
`template_type` is one of `tengah_semester` / `akhir_semester` /
`akhir_semester_hal2`, matching the 3 reference files.

**Deliberate scope cut** (explicitly out of scope per the task, called out
here and as a `ponytail:` comment at the top of
`handler/report_template.go`): this is **upload/storage/replace only**.
The uploaded `.xlsx` is never parsed or rendered from — actual print
output still comes from the hardcoded-but-dynamic-per-school layout in
`ReportCardPrintView.vue` described above. Building a generic "render PDF
from an arbitrary uploaded template" engine was explicit YAGNI for this
pass. Upgrade path if a school genuinely needs a custom layout: parse the
uploaded sheet's cell positions the same way `evaluation`'s
dimensi_lulusan import reads a fixed header row (already uses `excelize`,
if that dependency is ever added to report-service), and feed the result
into the print view instead of/alongside the hardcoded template.

Frontend: `ReportCardGenerateView.vue` gained an "Import Template" button
opening a popup — pick a rapor-type, see the currently-uploaded file (name
+ date, with a Download button reusing the blob+`URL.createObjectURL`
pattern already used elsewhere in this codebase for authenticated
downloads), upload a replacement.

## Report structure (Sekolah GPM 2026-08-05 revision)

This section documents the 2026-08-11 dynamic-report-structure work
(migration `0007_dynamic_report_settings`) on top of the original design
below.

1. **Leger is view/print only.** The old "Detail" drill-down button/route
   was removed from `LegerView.vue` — Leger is a read-only classroom x
   term grid with a print button, nothing else. `GET .../leger` gained a
   `?report_type=` filter (default `semester`) so it never mixes Rapot
   Bayangan rows into the semester grid.
2. **Generate Rapor's columns are now**: Mata Pelajaran, Nilai Formatif,
   Nilai STS, Nilai SAS, Capaian STS, Capaian SAS, Nilai Akhir. The old
   "Override"/"Sinkronisasi" columns are gone; correcting Nilai Akhir is
   now an inline edit affordance on that cell (still the same
   `PATCH .../subjects/{subject_id}` override, just a different UI), and a
   sync problem shows as an inline warning next to the subject name
   instead of an always-visible column.
3. **Rapot Bayangan** (`report_type=bayangan`) is a second, quarterly
   report sharing `report_cards`/`report_card_subjects` and the whole
   draft/finalize/unlock lifecycle with the regular semester report —
   only the weight formula and which columns get populated differ (see
   `calcSubjectScores`-equivalent logic in `GenerateReportCards`). Its
   columns: Mata Pelajaran, Nilai Formatif, Nilai Praktik, Nilai STS,
   Capaian STS, Nilai Sementara. `period_label` (e.g. `"Triwulan 1"`)
   disambiguates multiple bayangan reports within one term, since
   academic-service's `Term` only models ganjil/genap, not quarters.
   Nilai Sementara uses all three weighted components
   (formatif/praktik/sts) per the client's explicit weight list, even
   though the client's own prose only says "formatif dan sts" — treated
   as the source text's own inconsistency (Nilai Praktik is listed as its
   own report column, so it isn't dropped).
4. **Dynamic, admin-configurable report structure** — the architectural
   core of this revision, see "Dynamic report configuration" below.
5. **Penilaian Formatif (guru)** is now a dropdown-per-tipe-penilaian
   picker instead of a spreadsheet add/remove-column grid: pick one
   `report_assessment_types` row, get one score column for the roster,
   save per student. STS/SAS-tagged types auto-suggest a score from
   exam-service (still fully editable — deliberately, so a guru can raise
   a low score) via `GET /formative`'s `assessment_type_id` param.

### Dynamic report configuration (`report_assessment_types` / `report_weight_settings`)

Two per-school tables replace every previously-hardcoded weight:

- **`report_assessment_types`** — the "tipe penilaian" an admin defines
  (tugas, praktek, STS, SAS, ...). Each row has a `bucket`
  (`formatif`|`praktik`|`sts`|`sas` — which report column it feeds),
  its own `weight_percent` (pools multiple types sharing a bucket, e.g.
  Tugas 60% + Kuis 40% → one Nilai Formatif), and `exam_linked` (marks
  STS/SAS-style types whose score can auto-populate from exam-service).
  This is also what powers the Penilaian Formatif dropdown — same table,
  two consumers (admin CRUD screen, guru picker).
- **`report_weight_settings`** — one row per school, the top-level
  formula weights for Nilai Akhir (`akhir_formatif_weight` /
  `akhir_sts_weight` / `akhir_sas_weight`) and Nilai Sementara
  (`sementara_formatif_weight` / `sementara_praktik_weight` /
  `sementara_sts_weight`). A school with no row gets illustrative
  defaults (`model.DefaultWeightSettings`), same fallback pattern as
  `report_grade_bands`.
- Both are **per-school only, not per-academic-year** — the simplest
  shape that satisfies "diatur di Admin"; a school's grading policy
  changing mid-year was judged unlikely enough not to warrant threading
  `academic_year_id` through every read here too (YAGNI — add it if a
  real school needs different weights across years).
- `model.WeightedAverage` (in `internal/model/model.go`) is the one
  calculation function both Nilai Akhir and Nilai Sementara go through:
  a normalized weighted average that **excludes** a missing bucket
  (e.g. no STS exam yet) rather than treating it as zero, renormalizing
  the remaining weights to fill the gap.
- **Design note for whoever builds Wali Kelas's scoped Generate Rapor
  next**: `GenerateReportCards` (handler/report_card.go) already takes
  `report_type`/`period_label` as query params and loads weights once per
  call via `h.d.Store.GetWeightSettings(schoolID)` — a Wali Kelas-scoped
  variant should reuse this same handler/calc path with an added
  classroom-ownership check, not reimplement the scoring, since the
  bucket-pooling (`GetFormativeBucketScores`) and STS/SAS exam
  aggregation are the parts worth not duplicating.
- Item 5's STS/SAS auto-populate (`resolveExamBucketScore`, same file)
  deliberately adds **no new exam-service endpoint** — it composes
  exam-service's existing `GET /exams?classroom_id=&term_id=`,
  `GET /exam-types?academic_year_id=`, and `GET /exams/{id}/scores`
  (already used by `GenerateReportCards`) client-side, filtering to one
  student. Checked exam-service's real routes (`internal/handler/scores.go`,
  `router.go`) first — no per-student endpoint exists, but the
  classroom-wide one already carries everything needed, so adding a new
  endpoint would have been redundant.

## Data model

- **`report_cards`** — one row per `(student_id, classroom_id, term_id,
  report_type, period_label)`. `report_type` (`semester`|`bayangan`,
  default `semester`) and `period_label` (quarterly label, `""` for
  semester rows) were added in migration 0007 — see "Report structure"
  above. Header + status envelope: `status` (`draft`|`finalized`),
  `finalized_at/by`, `unlocked_at/by`, a cached header snapshot (student
  name/NISN, classroom name, term name) and a cached attendance summary
  (hadir/sakit/izin/tanpa keterangan), each with a `*_synced_at`/
  `*_sync_error` so the UI can show "last synced" / "could not reach
  attendance-service" honestly.
- **`report_card_subjects`** — one row per `(report_card_id, subject_id)`.
  `computed_score` is Nilai Akhir (semester rows) or Nilai Sementara
  (bayangan rows) — an admin-configurable weighted formula over
  `formatif_score`/`sts_score`/`sas_score` (semester) or
  `formatif_score`/`praktik_score`/`sts_score` (bayangan), see "Dynamic
  report configuration" above. `override_score`, if set, is what an admin
  manually adjusted it to — **both are kept**, never destroyed by a
  regenerate, so the UI can show "computed vs overridden". `final_score`
  (API-only, not a DB column) = `override_score` if set, else
  `computed_score`. `grade_label` (predikat for the final score),
  `sts_label`/`sas_label` (Capaian STS/Capaian SAS — the same
  `report_grade_bands` mechanism applied to `sts_score`/`sas_score`
  individually) are all resolved at read time, not stored.
- **`report_assessment_types`** / **`report_weight_settings`** — see
  "Dynamic report configuration" above.
- **`formative_grades`** — one row per `(term_id, classroom_subject_id,
  student_id, assessment_type_id)`, the guru's Penilaian Formatif entry
  (item 5). Replaced the old `order_index` spreadsheet-column shape in
  migration 0007; `formative_headers` (the old per-column display name
  table) was dropped entirely, superseded by
  `report_assessment_types.name`.
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

# Leger: classroom x term rekap grid (admin/guru/superadmin only), scoped
# to one report_type (default "semester")
curl -s "http://localhost:8086/classrooms/$CLASSROOM/leger?term_id=$TERM&report_type=semester" -H "Authorization: Bearer $ADMIN_TOKEN"

# Rapot Bayangan: same generate endpoint, report_type=bayangan requires period_label
curl -s -X POST "http://localhost:8086/report-cards/generate?classroom_id=$CLASSROOM&term_id=$TERM&report_type=bayangan&period_label=Triwulan%201" \
  -H "Authorization: Bearer $ADMIN_TOKEN"

# Item 4: dynamic report settings (admin CRUD)
curl -s http://localhost:8086/settings/assessment-types -H "Authorization: Bearer $ADMIN_TOKEN"
curl -s -X POST http://localhost:8086/settings/assessment-types -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"code":"tugas","name":"Tugas","bucket":"formatif","weight_percent":60,"exam_linked":false,"sort_order":1}'
curl -s http://localhost:8086/settings/weights -H "Authorization: Bearer $ADMIN_TOKEN"
curl -s -X PUT http://localhost:8086/settings/weights -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"akhir_formatif_weight":30,"akhir_sts_weight":30,"akhir_sas_weight":40,"sementara_formatif_weight":30,"sementara_praktik_weight":40,"sementara_sts_weight":30}'

# Item 5: Penilaian Formatif (guru) -- dropdown-per-tipe-penilaian.
# GET auto-suggests a score for exam_linked types (STS/SAS) from
# exam-service when the guru hasn't saved their own value yet.
curl -s "http://localhost:8086/formative?term_id=$TERM&classroom_subject_id=<cs_id>&classroom_id=$CLASSROOM&assessment_type_id=<type_id>&subject_id=<subject_id>&academic_year_id=<ay_id>" \
  -H "Authorization: Bearer $GURU_TOKEN"
curl -s -X PUT http://localhost:8086/formative -H "Authorization: Bearer $GURU_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"term_id":"'"$TERM"'","classroom_subject_id":"<cs_id>","student_id":"<student_id>","assessment_type_id":"<type_id>","score":85}'

# Import Template (migration 0008) -- upload/list/download reference .xlsx
# files, storage/record-keeping only (see "Print/PDF, Leger cascade,
# Import Template" section above for the scope-cut note).
curl -s http://localhost:8086/settings/report-templates -H "Authorization: Bearer $ADMIN_TOKEN"
curl -s -X POST http://localhost:8086/settings/report-templates -H "Authorization: Bearer $ADMIN_TOKEN" \
  -F "template_type=tengah_semester" -F "file=@Format Rapor Tengah Semester (3 Bulan).xlsx"
curl -s http://localhost:8086/settings/report-templates/tengah_semester/download \
  -H "Authorization: Bearer $ADMIN_TOKEN" -o downloaded.xlsx

# Official-format print/PDF -- GET /report-cards/{id} now includes a live
# `school` field (name/address/city/province/logo); the F4/letterhead
# rendering itself is entirely client-side (ReportCardPrintView.vue,
# window.print()), report-service returns structured JSON only, unchanged
# from the original "no Go PDF library" decision.
curl -s http://localhost:8086/report-cards/<id> -H "Authorization: Bearer $ADMIN_TOKEN"
```

## What was tested (2026-08-11 print-template session: Leger cascade, print/PDF rebuild, Import Template)

- `go build ./...`, `go vet ./...`, `gofmt -l .` clean. `npx vue-tsc -b`
  clean (0 errors); `npx vite build` succeeded (`ReportCardPrintView`,
  `SemesterKelasPicker` bundled without error).
- `docker compose up -d --build` → healthy; `go run ./cmd/migrate up` →
  `version=8 dirty=false`. Verified via `psql \d report_templates`: table
  exists with the expected columns, `UNIQUE (school_id, template_type)`,
  and the `template_type` CHECK constraint.
- Import Template: `POST /settings/report-templates` (multipart,
  `template_type=tengah_semester` + a real `.xlsx`-named file) → `200`
  with metadata (no file bytes in the response). `GET
  /settings/report-templates` → list shows the one uploaded row. `GET
  /settings/report-templates/tengah_semester/download` → downloaded bytes
  `diff`ed byte-identical against the originally uploaded file.
- Dynamic school info (`GET /report-cards/{id}`'s new `school` field):
  inserted a `report_cards` row directly via SQL (no academic/exam/
  identity services running) → `GET` returned `200` with no `school` key
  at all (graceful degrade, not a crash) when identity-service is
  unreachable. Then ran report-service via `go run ./cmd/server` on the
  host (pointed at the dockerized Postgres) against a throwaway Python
  stub standing in for identity-service's `GET /schools/{id}` (returning a
  deliberately *non*-GPM school name/address/city/province, "SMK Uji Coba
  Mandiri") — `GET /report-cards/{id}` came back with
  `"school":{"name":"SMK Uji Coba Mandiri",...}`, proving the enrichment
  is genuinely per-school/dynamic, not hardcoded, and doesn't crash either
  way. Test report card row deleted afterward.
- Not exercised end-to-end in this session (no academic/exam services
  running alongside report-service, same constraint as the 2026-08-11
  dynamic-report-structure session below): the Leger semester+kelas
  cascade against real classroom/term data, `enrichSubjectGroups` against
  a real `subject_group` value, and the print view's rendered output in an
  actual browser. `vue-tsc`/`vite build` passing is the fallback proof the
  new Vue components/types are internally consistent; the backend
  enrichment's live-fetch behavior above is proven directly.
- `docker compose down` → clean teardown.

## What was tested (2026-08-11 dynamic-report-structure session)

- `go build ./...`, `go vet ./...`, `gofmt -l .` clean. `npx vue-tsc -b`
  and `npx vite build` (web/) clean.
- `docker compose up -d --build` → healthy; `go run ./cmd/migrate up` →
  `version=7 dirty=false` (migration 0007 applied cleanly on top of the
  existing 0001-0006 chain).
- `POST /settings/assessment-types` created 5 types (tugas/kuis formatif,
  praktek praktik, sts/sas exam_linked); a duplicate `code` → `409`
  (fixed a bug caught here: the driver is pgx/v5/stdlib so Postgres
  errors surface as `*pgconn.PgError`, not `lib/pq`'s `*pq.Error` — an
  initial `err.(*pq.Error)` type assertion silently never matched and
  fell through to a generic 500; replaced with `errors.As` against
  `pgconn.PgError`).
- `PUT /formative` (guru) saved Tugas=85 (weight 60) and Kuis=70 (weight
  40) for one student; verified via direct SQL join that
  `GetFormativeBucketScores`' weighted-average math is exactly
  `(85*60+70*40)/100 = 79.0` for the pooled formatif bucket.
- `DELETE /settings/assessment-types/{id}` on a type still referenced by
  `formative_grades` → `409` (FK `ON DELETE` default RESTRICT, not a
  crash); `PUT` to rename/reweight a type → `200`.
- `POST /report-cards/generate` (semester, all 4 upstreams down) → `200`,
  `created:0`, per-integration `warnings` — same documented graceful
  pattern as before, confirms the new `report_type`/weight-loading logic
  doesn't regress the base path.
- `POST /report-cards/generate?report_type=bayangan` without
  `period_label` → `400`; with it → `200`; `report_type=bogus` → `400`.
- `PUT /settings/weights` → `200`, persisted (`GET` after showed
  `updated_by`/`updated_at` set).
- `GET /classrooms/{id}/leger?report_type=semester` → `200`, empty grid.
- Verified schema directly via `psql`: `report_cards.report_type`/
  `period_label` columns exist with the expected `UNIQUE
  (student_id, classroom_id, term_id, report_type, period_label)`
  constraint (replacing the old 3-column one) and `report_type` CHECK;
  `report_card_subjects.praktik_score` exists.
- `docker compose down` → clean teardown.
- Not exercised end-to-end (no academic-service/exam-service running
  alongside report-service in this session — see other three parallel
  workstreams): the roster-populated generate path, STS/SAS auto-populate
  actually resolving a real exam score, and the Vue views against a live
  backend. The graceful-degradation path above and the direct-SQL/unit
  verification of the weighted-average math are the fallback proof for
  the scoring logic; the original 2026-07 session already proved the
  roster-populated path works once academic/identity/exam are up (see
  below), and this session's changes to that path are additive
  (report_type param, new bucket-based score calc) not structural.

## What was tested (original 2026-07 session, local dev, no prod risk)

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

## Wali Kelas role (Bug Report SEKOCI GPM 5 Agustus 2026)

`active_role == "walikelas"` can now call `POST /report-cards/generate`,
`GET /classrooms/{id}/subjects/{subject_id}/description`,
`PATCH /report-cards/{id}/subjects/{subject_id}/notes`, and read
`GET /report-cards/{id}` / `GET /students/{id}/report-cards` — the same
Generate & Deskripsi Rapor endpoints admin/guru already use, restricted to
their own homeroom classroom via `isWaliKelasOwner` (report_card.go), which
re-checks `classrooms.homeroom_teacher_user_id` against academic-service's
`GET /classrooms/{id}` live on every call (that response already carried
`homeroom_teacher_user_id`, used for the Wali Kelas header field — nothing
new needed on the academic side). `finalize`/`unlock`/`PATCH override`
remain admin/superadmin-only — the spec only asks wali kelas to generate and
write descriptions, not lock/override scores.

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
