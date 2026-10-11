# T5: Pelatihan & Onboarding di service `hr`: katalog, onboarding, penugasan & kepatuhan

**Menu PRD:** 99, 100, 101 (BR-TRN-01) · **Branch:** `feat/T5-training` · **Service:** `hr` (paket `internal/training`) · **Migrasi:** `services/hr/migrations/0030_training.*` (nomor 0030–0039 milik T5)
**Prasyarat:** T1, T2 di-merge. T4 **tidak** wajib: hook `onboarding.StartForEmployee` dipanggil T4 lewat interface nil-safe, jadi T5 cukup menyediakan fungsinya dan mendaftarkannya di `main.go`. **Feature key:** `training`.

## 1. Donor
`inquira-vue-golang/backend/internal/handlers/{courses.go,enrollment.go,progress.go,quiz.go,certificates.go,learning_path.go,categories.go}` + `categories_test.go` (gin + sqlx → chi + driver hr). Yang diambil: model kursus/modul/materi, enrol, progres per materi, kuis + penilaian, sertifikat. **Tidak** diambil: SCORM, xAPI, diskusi, review, payment, KSAT, sesi live. Cek juga apakah academic `0004_learning` (materi/tugas siswa) punya komponen UI unggah file atau pemutar video yang bisa dipakai ulang sebelum membuat yang baru.

## 2. Migrasi `0030_training.up.sql`
```
training_categories(id, school_id, name, deleted_at)
competencies(id, school_id, code, name, deleted_at)
courses(id, school_id, title, objective, duration_minutes int, provider text, kind CHECK IN('online','offline'),
  category_id, cost_idr bigint default 0, quota int NULL, passing_score int NULL CHECK(passing_score BETWEEN 0 AND 100),
  status CHECK IN('draft','published','archived'), deleted_at)
course_competencies(course_id, competency_id, PRIMARY KEY(course_id, competency_id))
course_materials(id, school_id, course_id, sort, kind CHECK IN('file','link','video'), title, object_key NULL, url NULL)
course_sessions(id, school_id, course_id, starts_at, ends_at, location, quota int)            -- tatap muka (FR-99-003)
quiz_questions(id, school_id, course_id, sort, prompt, options jsonb, correct_index int)       -- kunci TIDAK pernah dikirim ke pegawai
mandatory_trainings(id, school_id, position_id, course_id, due_days int, UNIQUE(position_id, course_id))   -- BR-TRN-01
training_assignments(id, school_id, employee_id, course_id, session_id NULL, source CHECK IN('manual','position','unit','onboarding','self'),
  due_date date NULL, status CHECK IN('pending_approval','not_started','in_progress','completed','rejected','cancelled'),
  approval_id NULL, started_at, completed_at, score int NULL, certificate_doc_id uuid NULL,
  UNIQUE(employee_id, course_id) WHERE status NOT IN ('cancelled','rejected'))
material_progress(assignment_id, material_id, done_at, PRIMARY KEY(assignment_id, material_id))
quiz_attempts(id, school_id, assignment_id, answers jsonb, score int, passed bool, created_at)
onboarding_templates(id, school_id, name, position_id NULL, department_id NULL, deleted_at)
onboarding_template_tasks(id, school_id, template_id, sort, title, category CHECK IN('dokumen','akun','perangkat','orientasi','pelatihan'),
  owner_kind CHECK IN('role','supervisor','employee'), owner_role NULL, due_offset_days int)     -- relatif ke tanggal mulai
onboarding_plans(id, school_id, employee_id UNIQUE, template_id, start_date, status CHECK IN('active','completed'))
onboarding_tasks(id, school_id, plan_id, title, category, owner_user_id NULL, owner_role NULL, due_date, done_at NULL, done_by NULL)
```
Status `terlambat` **bukan** kolom: dihitung `due_date < today AND status <> 'completed'` (FR-101-003).

## 3. Fungsi murni + vektor
| Fungsi | Vektor |
|---|---|
| `QuizScore(questions, answers) int` dan `Passed(score, passing)` | **AC-101-01** passing 70, nilai 80 → completed |
| `DisplayStatus(assignment, today)` | belum mulai / berjalan / selesai / terlambat |
| `Compliance(assigned, completed) bp` | **AC-101-02** 40 & 30 → 7500 bp (tampil 75%) |
| `CanEnroll(course, session, taken)` | **AC-99-01** kuota 20, terisi 20 → "Kuota penuh" |
| `NeedsApproval(course)` = `cost_idr > 0` | **AC-99-02** 1.500.000 → `pending_approval` → core `training_paid` (penyetuju default atasan langsung) |
| `BuildOnboarding(template, startDate, supervisor)` | **AC-100-01** template 8 tugas → 8 tugas dengan owner + tenggat |

## 4. Integrasi & job
- `onboarding.StartForEmployee(ctx, tx, employeeID)` dipanggil oleh: T4 (offer accepted) dan T2 (create pegawai baru; tambahkan panggilan di handler create T2 yang nil-safe). Template dipilih dengan urutan jabatan+bagian → jabatan → default sekolah. Tidak ada template → tidak membuat apa pun (log info, bukan error).
- **BR-TRN-01:** saat assignment T2 dibuat/diubah ke jabatan X, assign semua `mandatory_trainings` jabatan X yang belum dimiliki (`source='position'`). Lakukan di **satu** fungsi yang dipanggil dari store assignment T2.
- **Job harian:** (1) **AC-100-02** pada `start_date`: panggil identity `POST /internal/employees/create-or-link` (dari T2), set `employees.status='active'`, lalu assign pelatihan wajib; (2) pengingat tugas onboarding & pelatihan yang mendekati/lewat tenggat (notifikasi, Idempotency-Key per tanggal).
- **FR-101-005:** sertifikat yang diunggah atau digenerate saat selesai → simpan sebagai `employee_documents` (T2) dengan jenis `sertifikat_pelatihan` (dibuat lewat migrasi jika belum ada untuk sekolah, `has_expiry=false`).

## 5. Endpoint
| Method & path | Peran |
|---|---|
| `CRUD /training/courses` (+ materials, sessions, questions, competencies), `CRUD /training/categories`, `/training/competencies`, `/training/mandatory` | training_admin |
| `POST /training/assignments` `{course_id, target:{employee_ids?, position_id?, department_id?}, due_date}` | training_admin |
| `GET /training/assignments?…` · `GET /training/compliance?position_id=&department_id=&course_id=` | training_admin, hr_manager, kepala_sekolah (unitnya) |
| `CRUD /onboarding/templates` (+tasks) · `GET /onboarding/plans` · `GET /onboarding/plans/{id}` | hr_admin |
| `POST /onboarding/tasks/{id}/done` | pemilik tugas (owner_user_id, peran owner_role, atau pegawai sendiri untuk `employee`) |
| ESS: `GET /me/training/catalog` · `POST /me/training/enroll` · `GET /me/training/assignments` · `POST /me/training/assignments/{id}/materials/{mid}/done` · `GET /me/training/assignments/{id}/quiz` (tanpa kunci) · `POST …/quiz` · `POST …/certificate` (unggah) · `GET /me/onboarding` | pegawai/guru ber-employee |

## 6. Frontend
`meta.feature: 'training'`, navGroup **Pelatihan** (admin) dan **Pegawai** (ESS).
| Path | View (`views/training/…`) | Peran | Label |
|---|---|---|---|
| `pelatihan/katalog` · `/:id` | `CoursesView.vue`, `CourseEditorView.vue` (tab Info / Materi (urutkan, unggah) / Jadwal / Kuis / Kompetensi) | training_admin | Katalog Pelatihan |
| `pelatihan/wajib` | `MandatoryView.vue` (matriks jabatan × pelatihan wajib) | training_admin | Pelatihan Wajib |
| `pelatihan/penugasan` | `AssignmentsView.vue` (dialog tugaskan per pegawai/jabatan/unit, tabel status dengan badge teks) | training_admin | Penugasan |
| `pelatihan/kepatuhan` | `ComplianceView.vue` (tabel % per unit × jabatan × pelatihan, progress bar + angka) | training_admin, hr_manager, kepala_sekolah | Kepatuhan |
| `sdm/onboarding` · `/:id` | `OnboardingPlansView.vue`, `OnboardingPlanView.vue` (checklist per kategori, owner, tenggat, badge terlambat) + `OnboardingTemplatesView.vue` | hr_admin | SDM / Onboarding |
| `pegawai/pelatihan` · `/:assignmentId` | `ess/EssTrainingView.vue` (tab Wajib / Katalog / Riwayat), `ess/EssCourseView.vue` (materi berurutan + tandai selesai, kuis satu halaman, hasil + sertifikat) | pegawai/guru | Pegawai / Pelatihan Saya |
| `pegawai/onboarding` | `ess/EssOnboardingView.vue` | pegawai baru (tampil jika plan aktif) | Pegawai / Onboarding |
Isi kartu "Pelatihan wajib belum selesai" di `EssDashboardView` (T2).

## 7. Bukti handoff
Vektor §3, AC-99-01/02, AC-100-01/02, AC-101-01/02; test kunci jawaban tidak bocor di respons ESS; test BR-TRN-01 saat pindah jabatan. Playwright: buat kursus + kuis → jadikan wajib untuk jabatan → pegawai baru di jabatan itu otomatis mendapat penugasan → kerjakan kuis lulus → kepatuhan naik.
