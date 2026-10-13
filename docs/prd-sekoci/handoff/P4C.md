# Handoff P4C: Laporan Kehadiran (Menu 43 Attendance Report)

## 1. Ringkasan
- **Task:** P4C, Phase 4 menu 43: FR-43-001 (M), FR-43-002 (S), FR-43-003 (S, add-on), AC-43-01.
- **Branch:** `feat/P4C-laporan-absensi`
- **Commit:** `e682fc8` (backend attendance), `02d775c` (web), ditambah commit handoff ini.
- **Hasil:** semua agregasi kehadiran sekarang dihitung di server. Ada rekap harian, bulanan, semester, dan rentang tanggal per siswa, kelas, dan unit, lengkap dengan jumlah dan persentase hadir, terlambat, sakit, izin, dan tanpa_keterangan. Juga ada tren per hari/minggu/bulan, perbandingan antar kelas, laporan per mata pelajaran, dan ekspor Excel (.xlsx). Nama siswa, kelas, unit, dan mapel di-resolve di server, dan tidak ada batas 500 baris.

## 2. File
### Baru
- `services/attendance/internal/model/report_attendance.go`: tipe `AttendanceCounts`, `AttendancePercentages`, `AttendanceRecapRow`, `AttendanceTrendPoint`.
- `services/attendance/internal/store/report_attendance.go`: query (`AttendanceRecapStudentRows`, `AttendanceTrend`, `AttendanceSubjectRows`) dan agregasi murni (`ComputePercentages`, `RollupRecap`, `RollupSubject`, `RankClasses`, `SumCounts`, `SortRecapRows`).
- `services/attendance/internal/client/report_lookup.go`: `LookupUsers` (batch 200 lewat identity `POST /users/search`), `SchoolName`, `ClassroomName`, `SubjectName`, `TermName`. Semuanya aman dipanggil dengan receiver nil.
- `services/attendance/internal/handler/report_attendance.go`: parsing periode, kontrol role, resolusi nama, paginasi, dan 4 endpoint JSON.
- `services/attendance/internal/handler/report_attendance_export.go`: ekspor xlsx (excelize, yang memang sudah jadi dependency).
- Test: `store/report_attendance_test.go`, `handler/report_attendance_test.go`.
- `web/src/services/attendance-report.service.ts`

### Diubah
- `services/attendance/internal/handler/router.go`: hanya menambah 5 baris route.
- `web/src/views/attendance/admin/AttendanceReportView.vue`: ditulis ulang.
- `web/src/router/domains/attendance.routes.ts`: menambah route `walikelas/rekap-presensi`.

`store.go`, `sessions.go`, dan `import.go` tidak disentuh. Tidak ada migrasi baru.

## 3. Endpoint (JWT; role admin, superadmin, kepala_sekolah, walikelas)
Walikelas wajib mengirim `classroom_id` miliknya sendiri (`requireOwnClassroom`). Role non-superadmin dikunci ke `school_id` dari JWT.

Parameter yang sama untuk semua endpoint: `period=daily|monthly|semester|range` (default monthly), `date`, `month`, `term_id`, `from`, `to` (rentang maksimal 400 hari), `academic_year_id`, `classroom_id`, `student_id`.

| Endpoint | Param khusus | Data |
|---|---|---|
| `GET /admin/attendance-report/recap` | `scope=student\|class\|unit`, `page`, `per_page` (opsional; tanpa `per_page` semua baris dikembalikan) | `{scope, period, summary, rows}` + meta `{page, per_page, total}` |
| `GET /admin/attendance-report/trend` | `granularity=day\|week\|month` | `{period, granularity, points}` |
| `GET /admin/attendance-report/class-comparison` | (tidak ada) | `{period, summary, rows}`; baris memuat `rank` dan `delta_vs_avg` |
| `GET /admin/attendance-report/subjects` | `group=subject\|class\|student`, `subject_id`, paginasi | `{group, period, summary, rows}`; baris memuat `sessions` |
| `GET /admin/attendance-report/export` | `report=recap\|comparison\|subjects\|trend`, `format=xlsx`, ditambah scope/group/granularity | file .xlsx berisi semua baris dan baris TOTAL |

Aturan perhitungan:
- `total` = hadir + terlambat + sakit + izin + tanpa_keterangan. `belum_presensi` dilaporkan, tapi tidak masuk penyebut.
- `kehadiran` = hadir + terlambat.
- `tidak_hadir` = sakit + izin + tanpa_keterangan.
- Persentase dibulatkan ke 1 desimal.
- `terlambat` dibandingkan sebagai teks biasa (`status = 'terlambat'`), jadi nilainya 0 sampai migrasi enum dari P4A masuk. Tidak ada ketergantungan pada urutan merge.

## 4. Keputusan & batasan
- **Unit = sekolah (`school_id`).** Satu yayasan bisa punya banyak sekolah/unit. Untuk admin, hasilnya 1 baris unit. Untuk superadmin tanpa `school_id`, hasilnya semua unit.
- **PDF = `window.print()` dan CSS print** (tombol "Cetak / Simpan PDF"). Ini mengikuti konvensi repo (lihat `services/report/README.md`, bagian "no Go PDF library"). Saat mencetak, sidebar dan topbar disembunyikan lewat `body:has(.attendance-report-page)`.
- **Grafik:** web belum punya library chart, jadi grafik tren dibuat dengan SVG inline (satu seri %kehadiran, crosshair dan tooltip) disertai tabel data.
- **Mapel:** `student_attendances` hanya punya 1 baris per siswa per hari (migrasi 0003). Karena itu kehadiran per mapel diturunkan dari status harian siswa pada setiap `class_session` mapel tersebut di kelasnya. Satu sesi dihitung satu pertemuan, dan sesi hasil import (tanpa subject) diabaikan. Ada dua batasan:
  - Siswa di kelas paralel Agama ikut terhitung di sesi agama lain.
  - Status "terlambat di jam ke-3" belum bisa dibedakan per jam pelajaran.
  Keduanya baru bisa diperbaiki kalau presensi disimpan per sesi.
- Nama yang gagal di-resolve tampil sebagai "Tidak diketahui", bukan sebagai UUID mentah.
- Endpoint lama `/admin/student-attendance/export` (CSV) tidak diubah. UI baru sudah tidak memakainya.

## 5. Verifikasi
- `go build ./... && go vet ./...`: OK.
- `TEST_PG_DSN=postgres://tia-nurul@localhost:5432/p4c_attendance go test ./...`: semua PASS.
  - Test DB menambahkan `terlambat` ke CHECK constraint, tapi hanya di DB `p4c_attendance`.
  - Yang dicakup: AC-43-01 (17/1/1/1), rollup unit, grup mapel, ranking, tren, paginasi, resolusi nama (fake identity/academic), dan round-trip xlsx.
- `npx vue-tsc --noEmit`, `npx vue-tsc -b`, dan `npx vite build`: OK.
- Belum diuji di browser dengan stack lengkap.

## 6. Catatan merge
- Setelah P4A merge (enum `terlambat`), tidak ada yang perlu diubah.
- `router.go` hanya bertambah satu blok route setelah `teacher-attendance/export`. Kalau terjadi konflik, cukup sisipkan blok itu kembali.
