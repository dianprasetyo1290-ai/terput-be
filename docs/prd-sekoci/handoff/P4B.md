# Handoff P4B: Akademik Kelas & Penugasan Guru (Menu 39–40)

## 1. Ringkasan
- **Task:** P4B, Phase 4 Operational: Class Management (menu 39) dan Teacher/Class Assignment (menu 40).
- **Branch:** `feat/P4B-akademik-kelas`.
- **Cakupan PRD:** FR-39-001, FR-39-002, FR-39-003, FR-40-001, FR-40-002, AC-39-01, AC-40-01. FR-39-004 (tautan ke master ruang, menu 116) tidak termasuk task ini.
- **Hasil:**
  - Kelas punya **kapasitas**. Pendaftaran yang melebihi kapasitas **ditolak** (HTTP 422) dengan pesan: "Kelas 7A sudah penuh (30/30 siswa). Naikkan kapasitas kelas atau pilih kelas lain."
  - **Riwayat perpindahan kelas** (`class_enrollment_history`) diisi oleh trigger DB pada setiap perubahan `class_enrollments`, sehingga semua penulis tercakup.
  - `ApplyClassTransfer` tidak lagi menimpa kelas asal. Enrollment dipindahkan dengan satu UPDATE dan tercatat satu baris `transfer` (dari → ke, request_id, alasan, aktor).
  - **Beban mengajar**: `classroom_subjects.hours_per_week` dan endpoint `/teaching-workload` (jam/minggu per guru, slot jadwal, pasangan kelas×mapel tanpa guru).
  - UI: kapasitas di Data Kelas, tab **Riwayat** (detail kelas), riwayat per siswa (Data Kelas › Siswa), dan panel **Beban Mengajar & Penugasan Guru** di Matriks Pelajaran.

## 2. Daftar File
### Baru
- `services/academic/migrations/0006_class_management.{up,down}.sql`
- `services/academic/internal/store/enrollment_history.go`: konteks perubahan (`setEnrollmentContext`), `CapacityError`, `checkCapacity` (FOR UPDATE), query riwayat
- `services/academic/internal/store/workload.go`: `TeachingWorkload`
- `services/academic/internal/store/class_management_test.go`, `services/academic/internal/handler/class_management_test.go`
- `web/src/components/academic/EnrollmentHistoryTable.vue`

### Diubah
- Store: `classroom.go` (capacity, student_count, `ClassroomPatch`, tx + tag riwayat di add/remove/delete), `classroom_subject.go` (hours_per_week, un-assign guru), `transfer.go` (transfer satu UPDATE + cek kapasitas; withdrawal/unit transfer diberi tag), `promotion.go` (tag `promotion`, cek kapasitas, aktor)
- Handler: `classroom.go`, `classroom_subject.go`, `promotion.go`, `transfer.go` (cek kapasitas saat membuat pengajuan class_transfer, 422 saat apply), `admission_enrollment_test.go` (fake driver dapat 2 kolom baru)
- `cmd/server/main.go` (3 route), `internal/model/model.go`, `README.md`
- Web: `academic.service.ts`, `types.ts`, `ClassroomListView.vue`, `ClassroomDetailView.vue`, `MatriksPelajaranView.vue`

## 3. Migrasi & Endpoint
**Migrasi 0006** (diuji up → down → up):
- `classrooms.capacity INT NULL` (NULL = tanpa batas, > 0)
- `classroom_subjects.hours_per_week SMALLINT NOT NULL DEFAULT 0` (0–60)
- Tabel `class_enrollment_history` (sengaja tanpa FK, agar riwayat tetap ada saat kelas dihapus), fungsi `log_class_enrollment_change()` dan trigger `trg_class_enrollment_history` (AFTER INSERT/UPDATE/DELETE). Tiap enrollment yang sudah ada diberi satu baris `enroll` awal.

| Method | Path | Keterangan |
|---|---|---|
| GET | `/classrooms/{id}/enrollment-history` | riwayat masuk/keluar kelas |
| GET | `/students/{id}/enrollment-history` | riwayat kelas satu siswa, lintas tahun |
| GET | `/teaching-workload?academic_year_id=` | `{summary, teachers[], unassigned[]}` |
| POST/PUT | `/classrooms`, `/classrooms/{id}` | field `capacity` (PUT: 0 = hapus batas; ditolak 422 bila < jumlah siswa aktif) |
| POST/PUT | `/classrooms/{id}/subjects`, `/classroom-subjects/{id}` | field `hours_per_week`; PUT `teacher_user_id: null` sekarang benar-benar melepas guru (sebelumnya diabaikan oleh COALESCE) |

Aksi riwayat: `enroll, move, remove, status_change, transfer, promotion, admission, withdrawal, unit_transfer, classroom_deleted, student_deleted`.

## 4. Cakupan & Test
| Kebutuhan | Bukti | Test |
|---|---|---|
| FR-39-001 kapasitas | store `checkCapacity`, handler 422 | `TestClassroomCapacityDB`, `TestAddStudentCapacityAndHistoryHTTP` |
| FR-39-002 / AC-39-01 tepat satu wali kelas | kolom tunggal, update menggantikan | `TestHomeroomExactlyOnePerClassDB` |
| FR-39-003 riwayat | trigger + tag konteks | `TestEnrollmentHistoryAddTransferRemoveDB`, `TestEnrollmentHistoryOtherWritersDB` (move, promotion, withdrawal) |
| FR-40-002 / AC-40-01 | `TeachingWorkload` | `TestTeachingWorkloadDB`, `TestUpdateClassroomSubjectTeacherNullHTTP` |

Verifikasi: `go build ./... && go vet ./...` OK, `go test ./...` OK (dengan `ACADEMIC_TEST_DATABASE_URL=postgres://…/p4b_academic`), `npx vue-tsc --noEmit` OK, `npx vite build` OK. Smoke test HTTP pada server lokal: create kelas dengan kapasitas, tambah siswa melebihi kapasitas mendapat 422, endpoint riwayat dan workload berjalan.

## 5. Catatan / Keputusan
- **Kapasitas ditegakkan** pada: tambah siswa manual, apply transfer kelas, dan kenaikan kelas (atomik; satu siswa gagal berarti seluruh batch dibatalkan). Kapasitas juga dicek lebih awal saat membuat pengajuan class_transfer. Kapasitas **tidak** ditegakkan pada upsert saga admission (`/internal/admission/enrollments/upsert`), karena saga harus tetap idempotent dan PPDB punya kuota sendiri. Perubahan itu tetap tercatat di riwayat (`enroll`/`move`).
- Kapasitas tidak bisa diturunkan di bawah jumlah siswa aktif saat ini (422).
- Trigger memakai `set_config(..., is_local=true)`. Nilainya hilang saat commit, jadi aman untuk pool koneksi. Penulis tanpa tag mendapat aksi turunan.
- Jam/minggu adalah rencana (diisi admin di Data Kelas). `scheduled_periods` adalah jumlah slot Jadwal Pelajaran nyata, ditampilkan berdampingan agar ketidaksesuaian terlihat.
- Nama aktor di UI: guru/siswa diambil dari direktori. Akun admin tampil sebagai "Admin/Staf", dan aktor null (job/callback internal) sebagai "Sistem".
- Tidak ada halaman detail siswa di domain akademik, jadi riwayat per siswa ditempatkan di Data Kelas › tab Siswa (tombol "Riwayat").
- GET dilayani lewat cache Redis 15 detik (sama seperti route lain), sehingga riwayat atau workload bisa tertinggal hingga 15 detik setelah perubahan.
- DB uji lokal: `p4b_academic` (Postgres :5432).
