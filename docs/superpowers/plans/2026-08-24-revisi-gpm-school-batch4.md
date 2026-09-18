# Revisi GPM School (Batch 4) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Ship 4 independent GPM School revision items against the Sekoci Go-rewrite (`skci`): a printable monthly attendance grid + real Excel import, a per-class filter on the live exam monitor, a cleaned-up + 3-way rapot type selector, and moving "+Tambah Pengguna" onto the Data Siswa/Data Guru pages.

**Architecture:** Backend is 6 independent Go microservices (chi router, pgx/`database/sql`, golang-migrate) under `services/`; frontend is one Vue 3 + TypeScript SPA under `web/src`, one thin `*.service.ts` per backend service. Every task below either touches exactly one service + its frontend service file, or is frontend-only.

**Tech Stack:** Go 1.25 (chi, pgx v5, golang-migrate, `github.com/xuri/excelize/v2` for Excel), Vue 3 `<script setup>` + TypeScript, native `<input type="month">`/`<input type="date">` (no date-picker library), browser `window.print()` for print output (no PDF library).

**Spec:** User's revision notes (2026-08-24, GPM School, verbal via chat — no docx this round) plus 3 scope-clarifying answers captured in-conversation:
1. Absensi Excel import currently doesn't exist at all → build a **real** import endpoint (not just a display fix), plus a monthly 1–N day grid to replace the day-by-day print flow.
2. "KOKURIKULER" does not exist anywhere in the codebase (confirmed via repo-wide grep) → **out of scope**, nothing to hide.
3. Rapot 3-way type selector (Tengah Semester / Semester 1 / Semester 2) → add to **both** the admin generate view and the wali kelas generate view.

## Global Constraints

- No Go test suite exists anywhere in this repo (`find . -name "*_test.go"` is empty) — do not introduce a new test framework mid-feature; verify backend changes by running the service locally and hitting the endpoint with `curl`, matching how every other handler in this codebase was built.
- Every attendance-service SQL change must preserve the existing guru daily check-in flow (`class_sessions` created via `POST /sessions/checkin`, `source` defaults to `'checkin'`) — the import feature is strictly additive, never touches non-import rows.
- Frontend: follow existing per-file conventions exactly — Vue `<script setup lang="ts">`, `ref`/`computed`/`watch`, blob downloads via the existing `downloadBlob` helper in `attendance.service.ts`, multipart uploads via `FormData` + `'Content-Type': 'multipart/form-data'` header (see `exam.service.ts`'s `importQuestions`).
- Money/security paths are untouched by this batch — no new auth logic, only new `admin`/`superadmin`-gated endpoints reusing the existing `requireRole` helper.

---

## Task Group A: Exam Monitoring — per-class filter (frontend only)

### Task 1: Add a client-side kelas filter to the live monitor roster

**Files:**
- Modify: `web/src/views/exam/MonitorView.vue`

**Interfaces:**
- Consumes: `monitor.value.roster: RosterRow[]` (each row already carries `classroom_id` — confirmed live in the backend response, `services/exam/internal/handler/monitor.go` lines 81-105), `classroomNames: Record<string, string>` (already populated by `loadSiblingsAndClassroomNames`, line 56-57).
- Produces: nothing consumed by later tasks — this group is self-contained.

Root cause (confirmed by research): the existing "Kelas" dropdown (line 200-205) switches between **sibling Exam records** (the old one-exam-per-class fan-out), not a filter within one exam's roster. Since exams now support `exam_classrooms` many-to-many (one Exam row spanning several classrooms), that dropdown never renders (`siblings.length` is 0 or 1) and `monitor.roster` returns the full union of every targeted class in one flat, ever-scrolling table. The fix is a second, independent filter that narrows the *already-loaded* `monitor.roster` by `classroom_id` — no backend or type change needed.

- [ ] **Step 1: Add the filter state and computed roster**

In `web/src/views/exam/MonitorView.vue`, right after the existing `classroomNames` ref declaration (line 47), add:

```ts
// ---- Kelas filter within THIS exam's own roster (item 6, GPM 2026-08-24):
// distinct from the sibling-exam switcher above — this narrows the roster
// of a single exam that spans multiple classrooms (exam_classrooms
// many-to-many), which is the actual "endless scroll" case since the
// sibling switcher only ever applies to the older one-class-per-exam data.
const classFilter = ref('')

const classOptions = computed(() => {
  const ids = new Set((monitor.value?.roster ?? []).map((r) => r.classroom_id))
  return [...ids].map((id) => ({ id, name: classroomNames.value[id] || id }))
})

const filteredRoster = computed(() => {
  const roster = monitor.value?.roster ?? []
  if (!classFilter.value) return roster
  return roster.filter((r) => r.classroom_id === classFilter.value)
})
```

- [ ] **Step 2: Reset the filter whenever the underlying exam changes**

In the existing `watch(() => route.params.examId as string, ...)` block (line 104-111), reset `classFilter.value = ''` alongside setting `examId.value`:

```ts
watch(
  () => route.params.examId as string,
  (id) => {
    examId.value = id
    classFilter.value = ''
    loadAll()
  },
  { immediate: true },
)
```

- [ ] **Step 3: Render the filter dropdown, only when there's more than one class to filter**

In the template, right after the existing sibling `<label v-if="siblings.length > 1" class="classroom-filter">...</label>` block (line 200-205), add a sibling label:

```vue
<label v-if="classOptions.length > 1" class="classroom-filter">
  Filter Kelas
  <select v-model="classFilter">
    <option value="">Semua Kelas</option>
    <option v-for="c in classOptions" :key="c.id" :value="c.id">{{ c.name }}</option>
  </select>
</label>
```

- [ ] **Step 4: Render `filteredRoster` instead of `monitor.roster`**

Change `<tr v-for="row in monitor.roster" :key="row.attempt_id || row.user_id">` (line 245) to:

```vue
<tr v-for="row in filteredRoster" :key="row.attempt_id || row.user_id">
```

And change the empty-state row condition `<tr v-if="monitor.roster.length === 0">` (line 269) to:

```vue
<tr v-if="filteredRoster.length === 0">
```

- [ ] **Step 5: Manual verification**

Run the exam service + frontend locally (`docker compose up exam-service` or the repo's existing dev script, then `npm run dev` in `web/`). Create (or use an existing) exam whose `classroom_ids` spans 2+ classes, open Pantau Ujian, confirm:
- "Filter Kelas" dropdown appears listing each class actually present in the roster.
- Selecting one class narrows the table to just that class's students; "Semua Kelas" restores the full list.
- The pre-existing sibling "Kelas" dropdown (only visible for old one-class-per-exam siblings, if any exist in test data) still behaves as before — switching exams, not filtering.

- [ ] **Step 6: Commit**

```bash
git add web/src/views/exam/MonitorView.vue
git commit -m "feat(exam): add per-class filter to live exam monitor roster"
```

---

## Task Group B: Rapot — remove percentage labels + 3-way type selector (frontend only)

### Task 2: Remove "(30%)"/"(40%)" labels from the Rapot Bayangan print header

**Files:**
- Modify: `web/src/views/report/admin/ReportCardPrintView.vue:204-206`

The weighting logic (backend `WeightSettings`/`DefaultWeightSettings`, `services/report/internal/model/model.go:195-217`, applied in `services/report/internal/handler/report_card.go:476-500`) is admin-configurable and never reads this label text — this is a pure template edit, zero backend impact.

- [ ] **Step 1: Edit the three `<th>` labels**

```vue
              <tr v-if="isBayangan">
                <th class="col-no">No</th>
                <th>Mata Pelajaran</th>
                <th>Nilai Formatif</th>
                <th>Nilai Praktik</th>
                <th>Nilai PTS</th>
                <th>Nilai Akhir</th>
              </tr>
```

- [ ] **Step 2: Manual verification**

Open a Rapot Tengah Semester (Bayangan) print view in the browser, confirm the three columns read "Nilai Formatif" / "Nilai Praktik" / "Nilai PTS" with no percentage suffix, and confirm the actual computed values are unchanged (still reflect the real weighting).

- [ ] **Step 3: Commit**

```bash
git add web/src/views/report/admin/ReportCardPrintView.vue
git commit -m "fix(report): drop hardcoded percentage labels from Rapot Bayangan print header"
```

### Task 3: Add the 3-way "Jenis Rapor" selector to the admin generate view

**Files:**
- Modify: `web/src/views/report/admin/ReportCardGenerateView.vue`

**Interfaces:**
- Consumes: `TermOption.order` (`web/src/services/report.types.ts:232`, already typed — 1 = Ganjil/Semester 1, 2 = Genap/Semester 2), `ClassroomTermPicker`'s emitted `update:termId` (unchanged), `generateReportCards(classroomId, termId, reportType, periodLabel?)` and `getLeger(classroomId, termId, reportType)` (`report.service.ts:27`, `:94` — both already accept `reportType: 'semester' | 'bayangan'`, no backend/signature change needed).
- Produces: nothing consumed by later tasks.

No backend or API-shape change is needed: `report_type` stays the existing 2-value enum (`'semester' | 'bayangan'`); "Semester 1" vs "Semester 2" is expressed purely by which term (`order === 1` vs `order === 2`) gets passed alongside `reportType: 'semester'`. This task adds a 3rd frontend-only concept, `jenisRapor`, that derives both `reportType` and (for the two Semester options) auto-picks the matching term.

- [ ] **Step 1: Replace the `reportType` ref with a `jenisRapor` ref + derived `reportType`**

Replace the existing declaration (currently around line ~120, near `const reportType = ref<'semester' | 'bayangan'>('semester')`):

```ts
// Item 3 (GPM 2026-08-24): the "Jenis Rapor" picker now exposes exactly the
// 3 named options the client asked for. Backend/report_type stays a 2-value
// enum ('semester' | 'bayangan') -- Semester 1 vs 2 is expressed by WHICH
// term gets sent alongside 'semester', not by a 3rd enum value. Picking
// "Semester 1"/"Semester 2" auto-selects the term whose academic-service
// Order (1=Ganjil, 2=Genap) matches; "Tengah Semester" leaves the term
// picker as a manual choice, since UTS can fall in either semester.
type JenisRapor = 'tengah_semester' | 'semester_1' | 'semester_2'
const jenisRapor = ref<JenisRapor>('semester_1')
const reportType = computed<'semester' | 'bayangan'>(() =>
  jenisRapor.value === 'tengah_semester' ? 'bayangan' : 'semester',
)
const BAYANGAN_PERIOD_LABEL = 'Tengah Semester'
```

(Keep `BAYANGAN_PERIOD_LABEL` — it's still read by `onGenerate` below.)

- [ ] **Step 2: Auto-select the matching term when Semester 1/2 is picked**

`ClassroomTermPicker` resolves `classroomId` → `academic_year_id` → terms internally but doesn't expose that term list to its parent. Rather than changing a shared component two other views depend on, replicate the one lookup here using `report.service.ts`'s already-exported `listClassrooms`/`listTerms`:

```ts
import { listClassrooms as listClassroomsForYear, listTerms } from '../../../services/report.service'
import type { TermOption } from '../../../services/report.types'

const termsForAutoSelect = ref<TermOption[]>([])

watch(classroomId, async (id) => {
  termsForAutoSelect.value = []
  if (!id) return
  const classrooms = await listClassroomsForYear()
  const classroom = classrooms.find((c) => c.id === id)
  if (!classroom) return
  termsForAutoSelect.value = await listTerms(classroom.academic_year_id)
})

function onJenisRaporChange() {
  generateResult.value = null
  if (jenisRapor.value === 'tengah_semester') {
    loadList()
    return
  }
  const wantOrder = jenisRapor.value === 'semester_1' ? 1 : 2
  const match = termsForAutoSelect.value.find((t) => t.order === wantOrder)
  if (match) {
    termId.value = match.id
  }
  loadList()
}
```

- [ ] **Step 3: Update the template select**

Replace the existing 2-option select (line ~228-231):

```vue
      <div class="report-type-row">
        <label>
          Jenis Rapor
          <select v-model="jenisRapor" @change="onJenisRaporChange">
            <option value="tengah_semester">Rapot Tengah Semester</option>
            <option value="semester_1">Rapot Semester 1</option>
            <option value="semester_2">Rapot Semester 2</option>
          </select>
        </label>
      </div>
```

`reportType` is now a computed, so every existing call site that reads `reportType.value` (`onGenerate`, `loadList`) keeps working unchanged — `computed()` refs support `.value` reads exactly like `ref()`.

- [ ] **Step 4: Manual verification**

Run the frontend locally, open Generate Rapor (admin), pick a classroom:
- Default "Rapot Semester 1" auto-selects the Ganjil term.
- Switching to "Rapot Semester 2" auto-selects the Genap term.
- Switching to "Rapot Tengah Semester" leaves the term picker on whatever was last selected, generate still works and produces a Bayangan-type rapor (verify via the resulting print view's `isBayangan`-gated header).

- [ ] **Step 5: Commit**

```bash
git add web/src/views/report/admin/ReportCardGenerateView.vue
git commit -m "feat(report): 3-way Jenis Rapor selector (Tengah Semester/Semester 1/Semester 2) on admin generate view"
```

### Task 4: Add the same 3-way selector to the wali kelas generate view

**Files:**
- Modify: `web/src/views/walikelas/GenerateRaporView.vue`

**Interfaces:**
- Consumes: same `generateReportCards`/`getLeger`/`listTerms`/`TermOption.order` as Task 3. This view already loads `terms.value` directly in `onMounted` (line 70) — no lookup-plumbing needed here, unlike Task 3.
- Produces: nothing.

- [ ] **Step 1: Add `jenisRapor` state and derived `reportType`**

Near the existing `const termId = ref('')` (line 27):

```ts
// Item 3 (GPM 2026-08-24): same 3-way picker as the admin generate view —
// see ReportCardGenerateView.vue for the full rationale. This view already
// loads `terms` up front in onMounted, so no extra lookup is needed here.
type JenisRapor = 'tengah_semester' | 'semester_1' | 'semester_2'
const jenisRapor = ref<JenisRapor>('semester_1')
const reportType = computed<'semester' | 'bayangan'>(() =>
  jenisRapor.value === 'tengah_semester' ? 'bayangan' : 'semester',
)
const BAYANGAN_PERIOD_LABEL = 'Tengah Semester'

function onJenisRaporChange() {
  generateResult.value = null
  if (jenisRapor.value === 'tengah_semester') return
  const wantOrder = jenisRapor.value === 'semester_1' ? 1 : 2
  const match = terms.value.find((t) => t.order === wantOrder)
  if (match) termId.value = match.id
}
```

- [ ] **Step 2: Pass `reportType`/`periodLabel` through to `generateReportCards` and `getLeger`**

Currently both calls omit these args (defaulting to `'semester'` server-side). Update `onGenerate` (line 81-94):

```ts
async function onGenerate() {
  if (!canGenerate.value || !myClassroom.value) return
  generating.value = true
  generateError.value = ''
  generateResult.value = null
  try {
    generateResult.value = await generateReportCards(
      myClassroom.value.classroom_id,
      termId.value,
      reportType.value,
      reportType.value === 'bayangan' ? BAYANGAN_PERIOD_LABEL : undefined,
    )
    await loadList()
  } catch (e: any) {
    generateError.value = e?.response?.data?.message || 'Gagal men-generate rapor. Coba lagi.'
  } finally {
    generating.value = false
  }
}
```

And `loadList` (line 39-53):

```ts
async function loadList() {
  if (!myClassroom.value || !termId.value) {
    leger.value = null
    return
  }
  loadingList.value = true
  listError.value = ''
  try {
    leger.value = await getLeger(myClassroom.value.classroom_id, termId.value, reportType.value)
  } catch {
    listError.value = 'Gagal memuat daftar rapor untuk term ini.'
  } finally {
    loadingList.value = false
  }
}
watch([termId, reportType], loadList)
```

(Note the `watch` target changes from bare `termId` to `[termId, reportType]` so switching Jenis Rapor alone — without the term itself changing, i.e. the Tengah Semester case — still reloads the list.)

- [ ] **Step 3: Update the template**

Replace the existing single "Semester" `<select>` (line 117-122) with two fields:

```vue
        <label class="term-field">
          Jenis Rapor
          <select v-model="jenisRapor" @change="onJenisRaporChange">
            <option value="tengah_semester">Rapot Tengah Semester</option>
            <option value="semester_1">Rapot Semester 1</option>
            <option value="semester_2">Rapot Semester 2</option>
          </select>
        </label>
        <label class="term-field">
          Semester
          <select v-model="termId">
            <option v-for="t in terms" :key="t.id" :value="t.id">{{ t.name }}</option>
          </select>
        </label>
```

- [ ] **Step 4: Manual verification**

As wali kelas, open Generate Rapor: confirm the 3-option Jenis Rapor selector works the same way as the admin view (Steps 4 of Task 3), scoped to the wali kelas's own single classroom.

- [ ] **Step 5: Commit**

```bash
git add web/src/views/walikelas/GenerateRaporView.vue
git commit -m "feat(report): 3-way Jenis Rapor selector on wali kelas generate view"
```

---

## Task Group C: Data Pengguna — relocate "+Tambah Pengguna" (frontend only)

### Task 5: Move the add-user button onto Data Siswa/Data Guru, make Data Pengguna read-only

**Files:**
- Modify: `web/src/views/identity/shared/PenggunaView.vue:644`

**Interfaces:**
- Consumes: `isGenericView`/`isSiswaView`/`isGuruView` computeds (already defined, lines 32-34), `openCreate()` (line 358) and `createForm.roles` (line 345/353, already auto-fixed to `[props.role]` when `props.role` is set) — no logic change needed, this is a template-only edit.

The create form already fully supports being opened with a fixed role (confirmed: `createForm.roles = props.role ? [props.role] : []`), so moving the button is a pure `v-if`/label change on one `<button>`.

- [ ] **Step 1: Replace the button's guard and label**

Change (line 644):

```vue
        <button v-if="isGenericView" class="btn btn-primary" @click="openCreate">+ Tambah Pengguna</button>
```

to:

```vue
        <button v-if="isSiswaView" class="btn btn-primary" @click="openCreate">+ Tambah Siswa</button>
        <button v-if="isGuruView" class="btn btn-primary" @click="openCreate">+ Tambah Guru</button>
```

- [ ] **Step 2: Update the comment above it that documented the old (now-reversed) behavior**

The comment at lines 639-640 currently reads:

```vue
        <!-- "+ Tambah" / "Impor" only on Data Pengguna: siswa/guru accounts are
        created there and show up here automatically via the role filter. -->
```

Replace with:

```vue
        <!-- Item 4 (GPM 2026-08-24): Data Pengguna is now view+search only —
        "+ Tambah" moved onto Data Siswa/Data Guru below. "Unduh Template" /
        "Import" (bulk .csv/.xlsx) stay Data Pengguna-only; those aren't part
        of this revision. -->
```

- [ ] **Step 3: Manual verification**

Log in as admin, visit:
- Data Pengguna: confirm no "+ Tambah Pengguna" button, search-by-name still works, "Unduh Template"/"Import"/"Export CSV" still present (unchanged, out of scope for this revision).
- Data Siswa: confirm "+ Tambah Siswa" appears, opens the create form, submitting creates a user with `role: siswa` and the new user shows up back on Data Pengguna via its role filter.
- Data Guru: same check with "+ Tambah Guru" / `role: guru`.

- [ ] **Step 4: Commit**

```bash
git add web/src/views/identity/shared/PenggunaView.vue
git commit -m "fix(identity): move +Tambah button onto Data Siswa/Data Guru, Data Pengguna is view+search only"
```

---

## Task Group D: Attendance — monthly grid + real Excel import

This group touches `services/attendance` (backend) then `web/src` (frontend); tasks D1→D5 are sequential (each depends on the previous).

### Task 6: Migration — allow import-created sessions without a real schedule slot

**Files:**
- Create: `services/attendance/migrations/0007_import_sessions.up.sql`
- Create: `services/attendance/migrations/0007_import_sessions.down.sql`

`class_sessions.schedule_id` is currently `NOT NULL` (it's the academic-service jam-pelajaran slot the guru checked into) and `guru_id` is `NOT NULL` too. A bulk Excel import of historical attendance isn't tied to one schedule slot, so import-created sessions need `schedule_id` nullable; `guru_id` stays filled with the importing admin's own user id (a real FK value, just not "the guru who taught that slot"). A new `source` column distinguishes these from the guru's own check-in flow, and a **partial** unique index (`WHERE source = 'import'`) gives the import handler an idempotent upsert target — the existing `class_sessions_schedule_date_unique` constraint is untouched and still governs real check-ins.

- [ ] **Step 1: Write the up migration**

```sql
-- 0007_import_sessions.up.sql
-- Admin Excel import of historical attendance (client revision, GPM School
-- 2026-08-24): bulk-imported days aren't tied to a real schedule_id slot,
-- so it must become nullable. `source` distinguishes import-created rows
-- from the guru's own check-in flow; the partial unique index is the
-- idempotent upsert target the import handler relies on (re-uploading the
-- same month must not create duplicate sessions).
ALTER TABLE class_sessions ALTER COLUMN schedule_id DROP NOT NULL;

ALTER TABLE class_sessions
  ADD COLUMN source TEXT NOT NULL DEFAULT 'checkin' CHECK (source IN ('checkin', 'import'));

CREATE UNIQUE INDEX class_sessions_import_classroom_date_unique
  ON class_sessions (classroom_id, session_date) WHERE source = 'import';
```

- [ ] **Step 2: Write the down migration**

```sql
-- 0007_import_sessions.down.sql
DROP INDEX IF EXISTS class_sessions_import_classroom_date_unique;
ALTER TABLE class_sessions DROP COLUMN source;
ALTER TABLE class_sessions ALTER COLUMN schedule_id SET NOT NULL;
```

- [ ] **Step 3: Run the migration against the local dev DB and verify**

```bash
cd services/attendance
go run ./cmd/server &   # or however the repo normally runs migrations on boot — check cmd/server/main.go
```

Confirm via `psql` (or the project's usual DB access) that `class_sessions.schedule_id` is nullable, `source` exists with the default, and `class_sessions_import_classroom_date_unique` exists:

```sql
\d class_sessions
```

- [ ] **Step 4: Commit**

```bash
git add services/attendance/migrations/0007_import_sessions.up.sql services/attendance/migrations/0007_import_sessions.down.sql
git commit -m "feat(attendance): migration for Excel-import-created class_sessions"
```

### Task 7: Store layer (grid pivot, import upsert)

**Files:**
- Modify: `services/attendance/internal/model/model.go`
- Modify: `services/attendance/internal/store/store.go`

**Interfaces:**
- Consumes: migration 0007's `source` column and `class_sessions_import_classroom_date_unique` index (Task 6); `student_attendances_classroom_date_student_unique` (pre-existing, migration 0003).
- Produces: `model.StudentAttendanceGridRow`, `Store.StudentAttendanceMonthlyGrid`, `Store.FindOrCreateImportSession`, `Store.UpsertStudentAttendanceImport` — all consumed by Task 8's handlers.

Note: this task's own code only needs the Go standard library (`strconv`, `fmt`, `strings`, `time`, `context`, `database/sql`) — the `excelize` dependency is added in Task 8 instead, alongside the code that actually imports it. (Amended after Task 7's first pass: adding it here via `go get` + `go mod tidy` before any code imports it let `go mod tidy` prune the requirement right back out, since nothing used it yet — and `go mod tidy` runs module-wide, not scoped to this task's files, so it also swept up unrelated changes from other uncommitted `.go` files in the module. Neither problem occurs once the dependency is added together with real usage.)

- [ ] **Step 1: Add the grid row model**

In `services/attendance/internal/model/model.go`, right after `StudentMonthlyAttendance` (after line 135):

```go
// StudentAttendanceGridRow is one student's full-month day-by-day status,
// pivoted for the printable monthly grid (client revision, GPM School
// 2026-08-24, item 1: replace the day-by-day print flow with one printable
// table per month, columns 1..N). Days with no recorded status (a holiday,
// or a day nobody has imported/marked yet) are simply absent from the map.
type StudentAttendanceGridRow struct {
	StudentID string            `json:"student_id"`
	Days      map[string]string `json:"days"` // day-of-month ("1".."31") -> status
}
```

- [ ] **Step 2: Add `strconv` to store.go's imports**

In `services/attendance/internal/store/store.go`, add `"strconv"` to the existing import block (currently `context`, `database/sql`, `encoding/json`, `errors`, `fmt`, `strings`, `time`).

- [ ] **Step 3: Add the grid pivot query**

In `store.go`, right after `StudentAttendanceMonthlySummary` (after line ~825, its closing `}`):

```go
// StudentAttendanceMonthlyGrid backs the "Presensi Siswa" admin monthly
// grid print view (client revision, item 1): every recorded status for one
// classroom/month, pivoted so each row is one student and each key in Days
// is a day-of-month. Unlike StudentAttendanceMonthlySummary (counts only),
// this preserves which specific day each status fell on.
func (s *Store) StudentAttendanceMonthlyGrid(ctx context.Context, schoolID, academicYearID, classroomID, month string) ([]model.StudentAttendanceGridRow, error) {
	if month == "" {
		month = time.Now().Format("2006-01")
	}
	where := []string{"sa.classroom_id = $1", "to_char(sa.session_date, 'YYYY-MM') = $2"}
	args := []interface{}{classroomID, month}
	add := func(cond string, val interface{}) {
		args = append(args, val)
		where = append(where, fmt.Sprintf(cond, len(args)))
	}
	if schoolID != "" {
		add("sa.school_id = $%d", schoolID)
	}
	if academicYearID != "" {
		add("sa.academic_year_id = $%d", academicYearID)
	}
	query := fmt.Sprintf(`
		SELECT sa.student_id, extract(day from sa.session_date)::int, sa.status
		FROM student_attendances sa
		WHERE %s
		ORDER BY sa.student_id, sa.session_date
	`, strings.Join(where, " AND "))
	rows, err := s.DB.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	byStudent := map[string]*model.StudentAttendanceGridRow{}
	var order []string
	for rows.Next() {
		var studentID, status string
		var day int
		if err := rows.Scan(&studentID, &day, &status); err != nil {
			return nil, err
		}
		g, ok := byStudent[studentID]
		if !ok {
			g = &model.StudentAttendanceGridRow{StudentID: studentID, Days: map[string]string{}}
			byStudent[studentID] = g
			order = append(order, studentID)
		}
		g.Days[strconv.Itoa(day)] = status
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	out := make([]model.StudentAttendanceGridRow, 0, len(order))
	for _, id := range order {
		out = append(out, *byStudent[id])
	}
	return out, nil
}

// FindOrCreateImportSession returns the id of the "import" class_session for
// (classroom_id, session_date), creating it on first use. Import-created
// sessions carry no real schedule_id (see migration 0007) -- guru_id is set
// to the importing admin's own user id, source='import' is what the partial
// unique index keys on, so re-importing the same date is idempotent instead
// of piling up duplicate sessions.
func (s *Store) FindOrCreateImportSession(ctx context.Context, schoolID, academicYearID, termID, classroomID, sessionDate, importedBy string) (string, error) {
	var id string
	err := s.DB.QueryRowContext(ctx, `
		INSERT INTO class_sessions (school_id, academic_year_id, term_id, classroom_id, guru_id, session_date, source)
		VALUES ($1, $2, NULLIF($3, ''), $4, $5, $6, 'import')
		ON CONFLICT (classroom_id, session_date) WHERE source = 'import'
		DO UPDATE SET updated_at = now()
		RETURNING id
	`, schoolID, academicYearID, termID, classroomID, importedBy, sessionDate).Scan(&id)
	return id, err
}

// UpsertStudentAttendanceImport writes/overwrites one student's status for
// one imported day. Conflict target matches student_attendances_classroom_
// date_student_unique (migration 0003) -- the same constraint the guru's own
// daily marking flow relies on -- so re-uploading the same month safely
// overwrites rather than duplicating.
func (s *Store) UpsertStudentAttendanceImport(ctx context.Context, classSessionID, schoolID, academicYearID, termID, classroomID, studentID, sessionDate, status, markedBy string) error {
	_, err := s.DB.ExecContext(ctx, `
		INSERT INTO student_attendances (class_session_id, school_id, academic_year_id, term_id, classroom_id, student_id, session_date, status, marked_by)
		VALUES ($1, $2, $3, NULLIF($4, ''), $5, $6, $7, $8, $9)
		ON CONFLICT (classroom_id, session_date, student_id)
		DO UPDATE SET status = EXCLUDED.status, class_session_id = EXCLUDED.class_session_id, marked_by = EXCLUDED.marked_by, updated_at = now()
	`, classSessionID, schoolID, academicYearID, termID, classroomID, studentID, sessionDate, status, markedBy)
	return err
}
```

- [ ] **Step 4: Build to catch typos**

```bash
cd services/attendance && go build ./...
```

Expected: builds clean.

- [ ] **Step 5: Commit**

```bash
git add services/attendance/internal/model/model.go services/attendance/internal/store/store.go
git commit -m "feat(attendance): store layer for monthly attendance grid + Excel import upsert"
```

### Task 8: Handlers — monthly grid, import template download, Excel import

**Files:**
- Modify: `services/attendance/internal/handler/admin.go` (add grid handler)
- Create: `services/attendance/internal/handler/import.go`

**Interfaces:**
- Consumes: `Store.StudentAttendanceMonthlyGrid`/`FindOrCreateImportSession`/`UpsertStudentAttendanceImport` (Task 7), `Client.GetRoster(ctx, classroomID) ([]RosterStudent, error)` (`internal/client/client.go:155`, returns `{StudentID, Name}`), `requireRole`, `scopeFromRequest`, `academicYearFromRequest`, `h.requireOwnClassroom` (all pre-existing helpers in `internal/handler/handler.go`).
- Produces: `Handler.StudentAttendanceMonthlyGridAdmin`, `Handler.AdminAttendanceImportTemplate`, `Handler.AdminAttendanceImport` — wired to routes in Task 9.

- [ ] **Step 1: Add the excelize dependency**

Moved here from Task 7: this task's `import.go` (Step 3 below) is the first code that actually imports `excelize`, so adding the dependency here — where it's immediately used — means `go mod tidy` won't prune it back out the way it did when Task 7 tried to add it before any code referenced it.

```bash
cd services/attendance
go get github.com/xuri/excelize/v2
```

Do NOT run `go mod tidy` yet — run it only after Step 3 (`import.go`) exists and actually imports the package, and only if `go build ./...` (Step 4) needs it to resolve a `go.sum` gap. Also be aware `go mod tidy` operates on the whole module, not just this task's files — if `git status` shows other uncommitted `.go` files in this service (peer-session WIP), a `tidy` run can pick up their imports into `go.mod`/`go.sum` too. If that happens, isolate `go.mod`/`go.sum` the same way as any other file with mixed changes before committing (Step 5): stage/verify only the entries this task's own code actually needs.

- [ ] **Step 2: Add the grid handler to admin.go**

In `services/attendance/internal/handler/admin.go`, right after `StudentAttendanceSummaryAdmin` (after its closing `}`):

```go
// StudentAttendanceMonthlyGridAdmin serves the "Presensi Siswa" admin
// monthly grid print view (client revision, GPM School 2026-08-24, item 1):
// replaces the day-by-day print flow with one printable table per month
// (columns = day 1..N). Requires a specific classroom_id (unlike the
// counts-only summary above, which can aggregate "Semua Kelas") because the
// roster merge below needs one classroom to enumerate students from.
func (h *Handler) StudentAttendanceMonthlyGridAdmin(w http.ResponseWriter, r *http.Request) {
	claims := common.FromCtx(r)
	if !requireRole(w, claims, "admin", "superadmin", "walikelas") {
		return
	}
	q := r.URL.Query()
	classroomID := q.Get("classroom_id")
	if claims.ActiveRole == "walikelas" && !h.requireOwnClassroom(w, r, claims, classroomID) {
		return
	}
	if classroomID == "" {
		common.WriteError(w, http.StatusBadRequest, "classroom_id is required")
		return
	}
	grid, err := h.Store.StudentAttendanceMonthlyGrid(r.Context(), scopeFromRequest(claims, r), academicYearFromRequest(r), classroomID, q.Get("month"))
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	roster, err := h.Client.GetRoster(r.Context(), classroomID)
	if err != nil {
		common.WriteError(w, http.StatusBadGateway, "could not load classroom roster")
		return
	}
	byStudent := map[string]map[string]string{}
	for _, g := range grid {
		byStudent[g.StudentID] = g.Days
	}
	type gridRow struct {
		StudentID   string            `json:"student_id"`
		StudentName string            `json:"student_name"`
		Days        map[string]string `json:"days"`
	}
	out := make([]gridRow, 0, len(roster))
	for _, s := range roster {
		days := byStudent[s.StudentID]
		if days == nil {
			days = map[string]string{}
		}
		out = append(out, gridRow{StudentID: s.StudentID, StudentName: s.Name, Days: days})
	}
	common.WriteJSON(w, http.StatusOK, out, nil)
}
```

- [ ] **Step 3: Create import.go with the template-download and import handlers**

```go
// services/attendance/internal/handler/import.go
//
// Admin Excel import of historical attendance (client revision, GPM School
// 2026-08-24, item 1): "does the school have to import every day?" -- no.
// AdminAttendanceImportTemplate hands back a ready-to-fill .xlsx for one
// classroom/month (one row per roster student, one column per day);
// AdminAttendanceImport reads it back, matching students by exact name
// against the same roster, and upserts one student_attendances row per
// non-empty day cell via the store functions from Task 7.
package handler

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/xuri/excelize/v2"

	"skci/attendance/internal/common"
)

// attendanceStatusCodes: the single-letter codes schools fill into the
// template, matching the Indonesian school convention (Hadir/Sakit/Izin/
// Alpa) rather than asking them to type the full enum value.
var attendanceStatusCodes = map[string]string{
	"H": "hadir", "S": "sakit", "I": "izin", "A": "tanpa_keterangan",
}

func daysInMonth(month string) (int, error) {
	t, err := time.Parse("2006-01", month)
	if err != nil {
		return 0, fmt.Errorf("month must be YYYY-MM")
	}
	return time.Date(t.Year(), t.Month()+1, 0, 0, 0, 0, 0, time.UTC).Day(), nil
}

// AdminAttendanceImportTemplate: GET /admin/student-attendance/import-template
// ?classroom_id=&month=YYYY-MM
func (h *Handler) AdminAttendanceImportTemplate(w http.ResponseWriter, r *http.Request) {
	claims := common.FromCtx(r)
	if !requireRole(w, claims, "admin", "superadmin") {
		return
	}
	q := r.URL.Query()
	classroomID := q.Get("classroom_id")
	month := q.Get("month")
	if classroomID == "" || month == "" {
		common.WriteError(w, http.StatusBadRequest, "classroom_id and month are required")
		return
	}
	days, err := daysInMonth(month)
	if err != nil {
		common.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	roster, err := h.Client.GetRoster(r.Context(), classroomID)
	if err != nil {
		common.WriteError(w, http.StatusBadGateway, "could not load classroom roster")
		return
	}

	f := excelize.NewFile()
	sheet := "Presensi"
	f.SetSheetName(f.GetSheetName(0), sheet)
	f.SetCellValue(sheet, "A1", "Nama Siswa")
	for d := 1; d <= days; d++ {
		col, _ := excelize.ColumnNumberToName(d + 1)
		f.SetCellValue(sheet, col+"1", strconv.Itoa(d))
	}
	for i, s := range roster {
		row := i + 2
		f.SetCellValue(sheet, fmt.Sprintf("A%d", row), s.Name)
	}

	w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=presensi_%s_%s.xlsx", classroomID, month))
	_ = f.Write(w)
}

// AdminAttendanceImport: POST /admin/student-attendance/import -- multipart
// form fields classroom_id, academic_year_id, month=YYYY-MM, term_id
// (optional), file=.xlsx matching the template above. Aborts on the first
// unrecognized student name or status code (matches exam-service's
// ImportQuestions convention: fail fast with a row-numbered message rather
// than partially importing).
func (h *Handler) AdminAttendanceImport(w http.ResponseWriter, r *http.Request) {
	claims := common.FromCtx(r)
	if !requireRole(w, claims, "admin", "superadmin") {
		return
	}
	if err := r.ParseMultipartForm(10 << 20); err != nil {
		common.WriteError(w, http.StatusBadRequest, "invalid multipart form: "+err.Error())
		return
	}
	classroomID := r.FormValue("classroom_id")
	month := r.FormValue("month")
	termID := r.FormValue("term_id")
	academicYearID := r.FormValue("academic_year_id")
	if classroomID == "" || month == "" || academicYearID == "" {
		common.WriteError(w, http.StatusBadRequest, "classroom_id, academic_year_id and month are required")
		return
	}
	days, err := daysInMonth(month)
	if err != nil {
		common.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	file, _, err := r.FormFile("file")
	if err != nil {
		common.WriteError(w, http.StatusBadRequest, "file is required (multipart field 'file')")
		return
	}
	defer file.Close()

	f, err := excelize.OpenReader(file)
	if err != nil {
		common.WriteError(w, http.StatusBadRequest, "could not parse xlsx: "+err.Error())
		return
	}
	defer f.Close()
	sheets := f.GetSheetList()
	if len(sheets) == 0 {
		common.WriteError(w, http.StatusBadRequest, "file has no sheets")
		return
	}
	records, err := f.GetRows(sheets[0])
	if err != nil || len(records) < 2 {
		common.WriteError(w, http.StatusBadRequest, "file must contain a header row plus at least one data row")
		return
	}

	roster, err := h.Client.GetRoster(r.Context(), classroomID)
	if err != nil {
		common.WriteError(w, http.StatusBadGateway, "could not load classroom roster")
		return
	}
	byName := map[string]string{} // lower(trim(name)) -> student_id
	for _, s := range roster {
		byName[strings.ToLower(strings.TrimSpace(s.Name))] = s.StudentID
	}

	header := records[0]
	dayCol := map[int]int{} // day-of-month -> column index
	for i, cell := range header {
		d, err := strconv.Atoi(strings.TrimSpace(cell))
		if err == nil && d >= 1 && d <= days {
			dayCol[d] = i
		}
	}

	schoolID := scopeFromRequest(claims, r)
	imported, skipped := 0, 0
	sessionIDCache := map[int]string{} // day -> class_session id, one find-or-create per day not per row

	for rowNum, row := range records[1:] {
		if len(row) == 0 || strings.TrimSpace(row[0]) == "" {
			continue
		}
		studentID, ok := byName[strings.ToLower(strings.TrimSpace(row[0]))]
		if !ok {
			common.WriteError(w, http.StatusBadRequest, fmt.Sprintf("row %d: siswa %q tidak ditemukan di kelas ini", rowNum+2, row[0]))
			return
		}
		for day, col := range dayCol {
			if col >= len(row) {
				continue
			}
			code := strings.ToUpper(strings.TrimSpace(row[col]))
			if code == "" {
				skipped++
				continue
			}
			status, ok := attendanceStatusCodes[code]
			if !ok {
				common.WriteError(w, http.StatusBadRequest, fmt.Sprintf("row %d, tanggal %d: kode %q tidak dikenal (pakai H/S/I/A)", rowNum+2, day, code))
				return
			}
			sessionDate := fmt.Sprintf("%s-%02d", month, day)
			sessionID, cached := sessionIDCache[day]
			if !cached {
				sessionID, err = h.Store.FindOrCreateImportSession(r.Context(), schoolID, academicYearID, termID, classroomID, sessionDate, claims.UserID)
				if err != nil {
					common.WriteError(w, http.StatusInternalServerError, err.Error())
					return
				}
				sessionIDCache[day] = sessionID
			}
			if err := h.Store.UpsertStudentAttendanceImport(r.Context(), sessionID, schoolID, academicYearID, termID, classroomID, studentID, sessionDate, status, claims.UserID); err != nil {
				common.WriteError(w, http.StatusInternalServerError, err.Error())
				return
			}
			imported++
		}
	}

	common.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"imported_count": imported,
		"skipped_count":  skipped,
	}, nil)
}
```

- [ ] **Step 4: Build**

```bash
cd services/attendance && go build ./...
```

Expected: builds clean.

- [ ] **Step 5: Commit**

```bash
git add services/attendance/go.mod services/attendance/go.sum services/attendance/internal/handler/admin.go services/attendance/internal/handler/import.go
git commit -m "feat(attendance): handlers for monthly grid, import template, and Excel import"
```

### Task 9: Wire the 3 new routes

**Files:**
- Modify: `services/attendance/internal/handler/router.go`

- [ ] **Step 1: Add the routes**

In the existing `// admin` block (after line 62, `pr.Get("/admin/student-attendance/export", h.StudentAttendanceExport)`):

```go
		pr.Get("/admin/student-attendance/monthly-grid", h.StudentAttendanceMonthlyGridAdmin)
		pr.Get("/admin/student-attendance/import-template", h.AdminAttendanceImportTemplate)
		pr.Post("/admin/student-attendance/import", h.AdminAttendanceImport)
```

- [ ] **Step 2: Build and smoke-test with curl**

```bash
cd services/attendance && go build ./... && go run ./cmd/server &
# with a real admin JWT and existing classroom_id/academic_year_id:
curl -H "Authorization: Bearer $TOKEN" \
  "http://localhost:8083/admin/student-attendance/import-template?classroom_id=$CLASSROOM_ID&month=2026-08" \
  -o /tmp/template.xlsx
file /tmp/template.xlsx   # expect: Microsoft Excel 2007+
```

Fill in a couple of H/S/I/A cells in `/tmp/template.xlsx`, then:

```bash
curl -H "Authorization: Bearer $TOKEN" \
  -F "classroom_id=$CLASSROOM_ID" -F "academic_year_id=$YEAR_ID" -F "month=2026-08" \
  -F "file=@/tmp/template.xlsx" \
  http://localhost:8083/admin/student-attendance/import
# expect: {"data":{"imported_count":N,"skipped_count":M}}
curl -H "Authorization: Bearer $TOKEN" \
  "http://localhost:8083/admin/student-attendance/monthly-grid?classroom_id=$CLASSROOM_ID&month=2026-08"
# expect: the imported days present in each student's "days" map
```

- [ ] **Step 3: Commit**

```bash
git add services/attendance/internal/handler/router.go
git commit -m "feat(attendance): wire monthly-grid/import-template/import routes"
```

### Task 10: Frontend — service functions + PresensiSiswaView.vue UI

**Files:**
- Modify: `web/src/services/attendance.service.ts`
- Modify: `web/src/views/attendance/admin/PresensiSiswaView.vue`

**Interfaces:**
- Consumes: `GET /admin/student-attendance/monthly-grid`, `GET /admin/student-attendance/import-template`, `POST /admin/student-attendance/import` (Task 9).

- [ ] **Step 1: Add types + service functions**

In `web/src/services/attendance.service.ts`, right after `StudentMonthlyAttendance` (after line 128):

```ts
// One row per student per classroom per month, day-by-day (not just
// totals) — backs the printable monthly grid (client revision, item 1).
export interface AttendanceGridRow {
  student_id: string
  student_name: string
  days: Record<string, AttendanceStatus> // day-of-month ("1".."31") -> status
}
```

And after `exportStudentAttendanceCsv` (after line 348):

```ts
export async function adminStudentAttendanceGrid(params: {
  classroom_id: string
  month: string
  academic_year_id?: string
}) {
  const res = await attendance().get<Envelope<AttendanceGridRow[]>>(
    '/admin/student-attendance/monthly-grid',
    { params },
  )
  return res.data.data
}

export async function downloadImportTemplate(params: {
  classroom_id: string
  month: string
}) {
  const res = await attendance().get('/admin/student-attendance/import-template', {
    params,
    responseType: 'blob',
  })
  return res.data as Blob
}

export async function importStudentAttendance(formData: FormData) {
  const res = await attendance().post('/admin/student-attendance/import', formData, {
    headers: { 'Content-Type': 'multipart/form-data' },
  })
  return res.data as { data: { imported_count: number; skipped_count: number } }
}
```

- [ ] **Step 2: Build the frontend to catch type errors**

```bash
cd web && npm run build 2>&1 | tail -40
```

Expected: no new TypeScript errors from `attendance.service.ts`.

- [ ] **Step 3: Commit**

```bash
git add web/src/services/attendance.service.ts
git commit -m "feat(attendance): frontend service functions for monthly grid + Excel import"
```

### Task 11: Frontend UI — day-grid table + import form on PresensiSiswaView.vue

**Files:**
- Modify: `web/src/views/attendance/admin/PresensiSiswaView.vue`

**Interfaces:**
- Consumes: `adminStudentAttendanceGrid`, `downloadImportTemplate`, `importStudentAttendance`, `downloadBlob` (all from Task 10 / pre-existing in `attendance.service.ts`).

Behavior: when a specific classroom is selected and no `date` filter is applied, show the new day-grid (columns 1..N) instead of the old counts-only summary — this directly answers "tampilkan absensi nya dari tanggal 1-30, biar bisa print perbulan." When **no** classroom is selected ("Semua Kelas"), keep the existing counts-only summary (a day-grid across multiple classes at once isn't a printable table). The date-filtered detail view is untouched.

- [ ] **Step 1: Add grid state + loader**

In the `<script setup>` block, after `const summaryRows = ref<StudentMonthlyAttendance[]>([])` (line 48), add:

```ts
// Day-by-day grid (client revision, item 1): shown instead of summaryRows
// when a specific classroom is picked and no Tanggal filter is applied.
const gridRows = ref<AttendanceGridRow[]>([])
```

And update the import to include the new type/function:

```ts
import {
  adminListStudentAttendance,
  adminStudentAttendanceSummary,
  adminStudentAttendanceGrid,
  downloadImportTemplate,
  importStudentAttendance,
  downloadBlob,
  type StudentAttendance,
  type StudentMonthlyAttendance,
  type AttendanceGridRow,
  type AttendanceStatus,
} from '../../../services/attendance.service'
```

Add a `daysInSelectedMonth` computed, right after `currentMonth()`:

```ts
const daysInSelectedMonth = computed(() => {
  const m = filters.value.month || currentMonth()
  const [y, mo] = m.split('-').map(Number)
  return new Date(y, mo, 0).getDate() // day 0 of next month = last day of this month
})
const gridDayColumns = computed(() =>
  Array.from({ length: daysInSelectedMonth.value }, (_, i) => String(i + 1)),
)
const statusAbbrev: Record<AttendanceStatus, string> = {
  belum_presensi: '', hadir: 'H', sakit: 'S', izin: 'I', tanpa_keterangan: 'A',
}
```

(`computed` must be added to the existing `import { ref, onMounted, watch } from 'vue'` → `import { ref, computed, onMounted, watch } from 'vue'`.)

- [ ] **Step 2: Branch `search()` to load the grid when a classroom is selected**

Replace the `else` branch of `search()` (currently lines 96-101):

```ts
    } else if (filters.value.classroom_id) {
      // Classroom selected, no Tanggal: printable monthly grid.
      params.month = filters.value.month || currentMonth()
      gridRows.value = await adminStudentAttendanceGrid({
        classroom_id: filters.value.classroom_id,
        month: params.month,
        academic_year_id: filters.value.academic_year_id || undefined,
      })
      summaryRows.value = []
      rows.value = []
    } else {
      // "Semua Kelas", no Tanggal: counts-only summary (unchanged).
      params.month = filters.value.month || currentMonth()
      summaryRows.value = await adminStudentAttendanceSummary(params)
      gridRows.value = []
      rows.value = []
    }
```

- [ ] **Step 3: Add the grid table to the template**

Replace the existing default-view `<div v-if="!filters.date" class="card">...</div>` block (lines 217-246) with a 3-way branch:

```vue
    <!-- Classroom selected, no Tanggal: printable monthly grid -->
    <div v-if="!filters.date && filters.classroom_id" class="card grid-card">
      <table class="data-table grid-table">
        <thead>
          <tr>
            <th class="col-name">Siswa</th>
            <th v-for="d in gridDayColumns" :key="d">{{ d }}</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="r in gridRows" :key="r.student_id">
            <td class="col-name">{{ r.student_name }}</td>
            <td v-for="d in gridDayColumns" :key="d">{{ statusAbbrev[r.days[d]] ?? (r.days[d] || '') }}</td>
          </tr>
          <tr v-if="!loading && !gridRows.length">
            <td :colspan="gridDayColumns.length + 1" class="muted">Tidak ada data presensi untuk bulan ini.</td>
          </tr>
          <tr v-if="loading">
            <td :colspan="gridDayColumns.length + 1" class="muted">Memuat...</td>
          </tr>
        </tbody>
      </table>
      <p class="hint">H = Hadir, S = Sakit, I = Izin, A = Tanpa Keterangan. Kosong = tidak ada catatan hari itu.</p>
    </div>

    <!-- "Semua Kelas", no Tanggal: counts-only summary (unchanged) -->
    <div v-else-if="!filters.date" class="card">
      <table class="data-table">
        <thead>
          <tr>
            <th>Siswa</th>
            <th>Kelas</th>
            <th>Hadir</th>
            <th>Sakit</th>
            <th>Izin</th>
            <th>Tanpa Keterangan</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="r in summaryRows" :key="r.classroom_id + '-' + r.student_id">
            <td>{{ studentNames.get(r.student_id) ?? r.student_id }}</td>
            <td>{{ classroomNames.get(r.classroom_id) ?? r.classroom_id }}</td>
            <td>{{ r.hadir }}</td>
            <td>{{ r.sakit }}</td>
            <td>{{ r.izin }}</td>
            <td>{{ r.tanpa_keterangan }}</td>
          </tr>
          <tr v-if="!loading && !summaryRows.length">
            <td colspan="6" class="muted">Tidak ada data presensi untuk bulan ini.</td>
          </tr>
          <tr v-if="loading">
            <td colspan="6" class="muted">Memuat...</td>
          </tr>
        </tbody>
      </table>
    </div>
```

- [ ] **Step 4: Add Import UI (state, handlers, buttons/form)**

Script additions, after the existing `onExport` function:

```ts
const importFileInput = ref<HTMLInputElement | null>(null)
const importFile = ref<File | null>(null)
const importing = ref(false)
const importError = ref('')
const importResult = ref<{ imported_count: number; skipped_count: number } | null>(null)

async function onDownloadTemplate() {
  if (!filters.value.classroom_id) {
    error.value = 'Pilih kelas terlebih dahulu untuk mengunduh template.'
    return
  }
  const blob = await downloadImportTemplate({
    classroom_id: filters.value.classroom_id,
    month: filters.value.month || currentMonth(),
  })
  downloadBlob(blob, `template_presensi_${filters.value.month || currentMonth()}.xlsx`)
}

function onImportFileChange(e: Event) {
  importFile.value = (e.target as HTMLInputElement).files?.[0] ?? null
}

async function onImportSubmit() {
  if (!filters.value.classroom_id || !filters.value.academic_year_id || !importFile.value) return
  importing.value = true
  importError.value = ''
  importResult.value = null
  try {
    const fd = new FormData()
    fd.append('classroom_id', filters.value.classroom_id)
    fd.append('academic_year_id', filters.value.academic_year_id)
    fd.append('month', filters.value.month || currentMonth())
    fd.append('file', importFile.value)
    const res = await importStudentAttendance(fd)
    importResult.value = res.data
    importFile.value = null
    if (importFileInput.value) importFileInput.value.value = ''
    await search()
  } catch (e: any) {
    importError.value = e?.response?.data?.message || 'Gagal mengimpor file presensi.'
  } finally {
    importing.value = false
  }
}
```

Template additions, in `.filter-actions` (after the existing "Export Excel" button):

```vue
        <button class="btn btn-outline" :disabled="!filters.classroom_id" @click="onDownloadTemplate">
          Unduh Template Import
        </button>
        <button class="btn btn-outline" :disabled="!filters.classroom_id" @click="importFileInput?.click()">
          Import Excel
        </button>
        <input
          ref="importFileInput"
          type="file"
          accept=".xlsx"
          style="display: none"
          @change="(e) => { onImportFileChange(e); onImportSubmit() }"
        />
```

And a result/error line right after the existing `<p v-if="error" class="error">{{ error }}</p>`:

```vue
    <p v-if="importing" class="muted">Mengimpor...</p>
    <p v-if="importError" class="error">{{ importError }}</p>
    <p v-if="importResult" class="success">
      {{ importResult.imported_count }} baris berhasil diimpor, {{ importResult.skipped_count }} sel kosong dilewati.
    </p>
```

- [ ] **Step 5: Add print support (native browser print, no PDF library)**

Add a "Print" button next to the grid table (inside `.grid-card`, before the closing `</div>`):

```vue
      <button class="btn btn-primary no-print" @click="window.print()">Print</button>
```

Add print styles in the `<style scoped>` block:

```css
.success {
  color: var(--sekoci-success);
  font-size: 0.85rem;
}
.grid-table {
  font-size: 0.78rem;
}
.grid-table .col-name {
  text-align: left;
  white-space: nowrap;
}
.hint {
  font-size: 0.78rem;
  color: var(--sekoci-text-muted);
  margin-top: 0.5rem;
}
@media print {
  .filters,
  .no-print {
    display: none !important;
  }
}
```

- [ ] **Step 6: Manual verification**

Run `npm run build` (or `npm run dev`) in `web/`, run `services/attendance` locally, then in the browser:
1. Open Presensi Siswa (admin), pick a classroom + month → confirm the day-grid renders with columns 1..N.
2. Click "Unduh Template Import" → confirm an `.xlsx` downloads with the roster names and day columns.
3. Fill in a few H/S/I/A cells in that file, click "Import Excel", select it → confirm the success message shows a non-zero `imported_count` and the grid refreshes with those statuses visible.
4. Click "Print" → confirm the browser print preview shows only the grid table (filters/buttons hidden via `.no-print`/`@media print`).
5. Switch back to "Semua Kelas" → confirm the old counts-only summary still renders unchanged.
6. Pick a specific `Tanggal` → confirm the untouched detail table still renders.

- [ ] **Step 7: Commit**

```bash
git add web/src/views/attendance/admin/PresensiSiswaView.vue
git commit -m "feat(attendance): monthly day-grid view + Excel import UI on Presensi Siswa"
```

---

## Execution Notes

- Groups A, B, C, and D are fully independent of each other — safe to dispatch in parallel (e.g. 4 subagents, one per group).
- Within Group D, tasks 6→7→8→9→10→11 are strictly sequential (migration → store → handler → route → frontend service → frontend UI).
- Within Group B, B1 is independent; B2 and B3 touch different files but B3's pattern directly mirrors B2 — do Task 3 first so Task 4's subagent (if run separately) can reference the finished B2 diff.
