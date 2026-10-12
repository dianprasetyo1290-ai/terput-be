# T22: PKL di service `studentlife`: mitra & kuota, penempatan & absensi, jurnal, penilaian & sertifikat

**Menu PRD:** 139, 140, 141 (Bagian 3 §5 Grup 26, BR-PKL-01, alur "PKL" §6) · **Branch:** `feat/T22-pkl` · **Service:** `studentlife` (paket `internal/pkl`) · **Migrasi:** `studentlife/0030`–`0039` (milik T22) · identity: **handler saja**, tanpa migrasi
**Prasyarat:** gelombang 4. T10 di-merge (kerangka `studentlife`, storage, runner, klien identity/notification/academic). T9 di-merge (peran + feature key).
**Peran:** `koordinator_pkl`, `pembimbing_industri` (eksternal) dari T9 + `siswa`, `guru`/`walikelas` (sebagai pembimbing sekolah per penempatan, bukan peran baru; jika T9 membuat peran khusus pembimbing sekolah, peran itu ikut diberi akses dengan aturan kepemilikan yang sama). **Feature key:** `pkl`. Nama final peran dan feature key **mengikuti T9 §2**, dipakai lewat konstanta `internal/common/roles.go` + satu konstanta TS.
**Batas:** kehadiran PKL dicatat terpisah dari presensi sekolah (Lampiran PRD: menu 140 tidak menulis ke menu 41/69). T22 **tidak** memanggil attendance.

## 1. Acuan kode
| Kebutuhan | Tiru / pakai | Catatan |
|---|---|---|
| Kerangka, runner, storage, klien | `services/studentlife` (T10 §1.2) | daftarkan paket baru di `main.go`, jangan buat runner kedua |
| Akun eksternal | identity `internal/handler/internal_employees.go` (`InternalCreateOrLinkEmployee`: `GenDefaultPassword`, `password_changed=false`, upsert `user_school_roles`) dan `InternalDeactivateUser` | endpoint baru §4.1 meniru pola ini |
| Kelas & siswa | academic `GET /students/{id}/classroom`, `GET /classrooms/{id}/students`, `GET /classrooms?academic_year_id=` | jurusan **belum ada** di academic (tidak ada kolom jurusan) → master jurusan di studentlife §2 |
| PDF tanpa dependensi baru | `services/billing/internal/handler/receipts.go` (`buildReceiptPDF`, `escapePDF`, simpan via `Storage.Put`) | salin pola ke `internal/pkl/certificate_pdf.go` |
| Unggah dokumen + URL unduh | `internal/storage/local.go` (T10) + pola unduh `services/hr/internal/handler/documents.go` | |
| Menu bersyarat (tampil hanya jika punya data) | `web/src/stores/employee.ts` (menu Pegawai untuk guru) | pola untuk menu PKL siswa & pembimbing sekolah |

## 2. Migrasi (skema ringkas)
**`0030_pkl_master`** (139)
```
majors(id, school_id, code, name, active, deleted_at, UNIQUE(school_id, code) WHERE deleted_at IS NULL)     -- jurusan SMK, mis. kode diisi sekolah
classroom_majors(classroom_id uuid PK, school_id, major_id FK)        -- pemetaan kelas academic → jurusan; jurusan siswa = jurusan kelas aktifnya
pkl_partners(id, school_id, name, field text, address, contact_name, contact_phone, contact_email, active, deleted_at)
pkl_partner_mous(id, school_id, partner_id FK, mou_number text, valid_from date NOT NULL, valid_to date NOT NULL,
  object_key, file_name, mime, uploaded_by, CHECK(valid_to >= valid_from))     -- riwayat MoU, tidak ditimpa
pkl_periods(id, school_id, academic_year_id, name, starts_on, ends_on, status CHECK IN('planning','ongoing','closed'),
  CHECK(ends_on >= starts_on))
pkl_quotas(id, school_id, partner_id FK, period_id FK, major_id FK, quota int NOT NULL CHECK(quota >= 0),
  UNIQUE(partner_id, period_id, major_id))
industry_mentors(id, school_id, partner_id FK, user_id uuid NOT NULL /* akun identity */, full_name, email, phone, job_title,
  active bool, deactivated_at NULL, UNIQUE(school_id, partner_id, user_id))
pkl_settings(school_id PK, certificate_prefix text NULL)
```
**`0031_pkl_placement`** (140)
```
pkl_placements(id, school_id, period_id FK, partner_id FK, major_id FK, student_id,
  school_mentor_user_id uuid NOT NULL, industry_mentor_id FK NULL, starts_on, ends_on,
  status CHECK IN('planned','active','completed','cancelled'), cancel_reason NULL, created_by, idempotency_key,
  UNIQUE(school_id, idempotency_key), UNIQUE(period_id, student_id) WHERE status <> 'cancelled')
pkl_attendance(id, school_id, placement_id FK, work_date date, status CHECK IN('hadir','izin','sakit'),
  check_in_at timestamptz NULL, check_out_at NULL,           -- jam dari server
  note text NULL, attachment_key NULL,                       -- surat izin/sakit
  confirmation CHECK IN('pending','confirmed','rejected') DEFAULT 'pending', confirmed_by NULL, confirmed_at NULL, reject_reason NULL,
  UNIQUE(placement_id, work_date))
```
**`0032_pkl_assessment`** (141)
```
pkl_journals(id, school_id, placement_id FK, entry_date, activity text NOT NULL, photos jsonb NOT NULL DEFAULT '[]',
  mentor_note text NULL, mentor_note_by NULL, UNIQUE(placement_id, entry_date))
pkl_final_reports(id, school_id, placement_id FK, version int, object_key, file_name, mime, uploaded_at, UNIQUE(placement_id, version))
pkl_schemes(id, school_id, period_id FK UNIQUE, industry_weight_bp int, school_weight_bp int,
  CHECK(industry_weight_bp >= 0 AND school_weight_bp >= 0 AND industry_weight_bp + school_weight_bp = 10000))
pkl_components(id, school_id, scheme_id FK, assessor CHECK IN('industry','school'), name, weight_bp int CHECK(weight_bp > 0), sort)
pkl_scores(id, school_id, placement_id FK, component_id FK, score_x100 int CHECK(score_x100 BETWEEN 0 AND 10000),
  scored_by, scored_at, UNIQUE(placement_id, component_id))
pkl_results(placement_id PK, school_id, industry_x100 int, school_x100 int, final_x100 int, computed_at,
  finalized_by NULL, finalized_at NULL)
pkl_certificates(id, school_id, placement_id UNIQUE, certificate_no text, object_key, issued_by, issued_at,
  UNIQUE(school_id, certificate_no))
```
Nilai disimpan sebagai bilangan bulat ×100 (85 → 8500), persentase dalam basis poin (GEMINI §4). Tidak ada seed jurusan, mitra, kuota, bobot, atau komponen: angka di AC (kuota 4, bobot 70/30, nilai 85/80) hanya vektor uji.

## 3. Fungsi murni (`internal/pkl/model.go`) + vektor uji
| Fungsi | Isi | Vektor (→ AC/FR) |
|---|---|---|
| `MouCovering(mous, period) (*Mou, error)` | MoU berlaku jika `valid_from <= period.starts_on` **dan** `valid_to >= period.ends_on` (seluruh periode tertutup; pilihan konservatif, dicatat di handoff). Error memuat tanggal berakhir MoU terakhir | **AC-139-01**: MoU s.d. 31 Des, periode Januari → error "MoU mitra sudah tidak berlaku (berakhir 31 Desember …)"; MoU baru yang menutup Januari → lolos |
| `CanPlace(quota *Quota, filled int, mouErr error)` | BR-PKL-01: tanpa baris kuota → tolak "Kuota mitra untuk jurusan ini belum diatur"; `filled >= quota` → tolak "Kuota mitra <X> jurusan <Y> periode ini penuh (4/4)"; MoU tidak berlaku → tolak | **AC-140-01** (kuota 4, terisi 4 → ditolak), FR-140-001 |
| `CountFilled(placements)` | status `planned/active/completed` (bukan `cancelled`) | batal tidak dihitung |
| `CanRecordAttendance(placement, today)` | status `active`, `starts_on <= today <= ends_on`, satu baris per hari | FR-140-002: di luar rentang → tolak; hari yang sama dua kali → 409 |
| `AttendanceRecap(rows)` | per siswa / per mitra: jumlah hadir, izin, sakit, terkonfirmasi, menunggu, ditolak | FR-140-003 |
| `ValidateScheme(scheme, components)` | bobot industri + sekolah = 10000; jumlah `weight_bp` komponen per assessor = 10000 (assessor berbobot 0 boleh tanpa komponen) | FR-141-002 |
| `AssessorScore(components, scores, assessor) (int, error)` | `Σ score_x100 × weight_bp / 10000`, semua komponen assessor wajib terisi | komponen kurang → error "Penilaian <industri/sekolah> belum lengkap" |
| `FinalScore(industryX100, schoolX100, scheme) int` | `(industry × iw + school × sw) / 10000`, pembulatan setengah ke atas ditulis eksplisit di satu fungsi | **AC-141-01**: iw 7000, sw 3000, 8500 & 8000 → 8350 (tampil "83,5"); kasus pembulatan .5 |
| `FormatScore(x100)` | `id-ID`, buang nol di belakang koma | 8350 → "83,5", 8000 → "80" |
| `CanIssueCertificate(placement, result)` | penempatan `completed` + hasil `finalized_at` terisi | FR-141-003 |
| `CertificateNo(prefix, year, seq)` | `[prefix]<tahun>/<urut 4 digit>`, urut per sekolah per tahun lewat `SELECT … FOR UPDATE` di store | prefix kosong |
| `Scope(viewer, placement)` | `koordinator_pkl` → semua di sekolahnya; pembimbing sekolah → `school_mentor_user_id = viewer`; pembimbing industri → `industry_mentors.user_id = viewer`; siswa → `student_id = viewer` | FR-139-003: pembimbing industri lain → tidak ada akses (404) |

## 4. Integrasi & job
**4.1 Akun pembimbing industri eksternal (FR-139-003) — kontrak identity milik T22.** Tambahkan di identity (file baru `internal/handler/internal_external_users.go`, satu baris route di grup `InternalOnlyMiddleware` di `cmd/server/main.go`):
`POST /internal/role-accounts/create-or-link` `{school_id, email, full_name, role:"pembimbing_industri"}` → `200 {user_id, created, default_password?}` — **milik T12** (gelombang 3, sudah di-merge; whitelist sudah memuat `pembimbing_industri`). T22 tidak membuat endpoint identity baru.
- `role` hanya dari daftar putih peran eksternal (saat ini `pembimbing_industri`, konstanta di identity). Peran lain → 400. Tidak mengisi `managed_by_hr`.
- Email sudah ada → hanya tambah/aktifkan baris `user_school_roles` (orang yang sama bisa membimbing di dua sekolah; satu akun, peran per sekolah). Email baru → buat user dengan `GenDefaultPassword` + `password_changed=false` (mekanisme ganti sandi pertama yang sudah ada).
- `default_password` hanya dikembalikan saat `created=true`. studentlife meneruskannya **sekali** ke layar koordinator untuk diserahkan ke pembimbing; tidak disimpan di studentlife, tidak masuk log/audit/notifikasi.
- Penonaktifan memakai `POST /internal/users/{id}/deactivate` `{school_id, reason:'pkl_mentor_inactive'}` yang sudah ada.
Model akses: pembimbing industri adalah user identity biasa dengan peran `pembimbing_industri` di sekolah itu; **semua** pembatasan data dilakukan studentlife lewat `Scope` (hanya siswa di penempatan yang ia bimbing: nama, kelas, jurusan, penempatan, absensi, jurnal, laporan, komponen penilaian industri). Peran ini tidak boleh lolos ke endpoint service lain: tidak dimasukkan ke daftar peran staf (`ALL_STAFF_ROLES` di `web/src/router/domains/core.routes.ts` dan cek peran di backend lain), sidebar hanya menu PKL.
Siklus akun: koordinator menambah pembimbing di mitra → identity create-or-link → baris `industry_mentors`. Nonaktif ketika (a) koordinator menonaktifkan pembimbing, atau (b) koordinator menutup periode (`closed`) dan pembimbing tidak punya penempatan `planned/active` atau penilaian industri yang belum lengkap di periode lain. Dijalankan saat aksi tersebut, bukan berdasarkan jumlah hari (tidak ada angka di PRD).

**4.2 Notifikasi** (`POST /internal/notifications`, key memuat id + tanggal): penempatan dibuat → siswa, pembimbing sekolah, pembimbing industri (`studentlife:pkl_placement:<id>:<user>`); absensi hari sebelumnya yang masih menunggu konfirmasi → ringkasan harian ke pembimbing industri (job, `studentlife:pkl_confirm:<mentor>:<tanggal>`); absensi ditolak → siswa; penilaian lengkap → koordinator; sertifikat terbit → siswa.
**4.3 Audit:** create/update mitra, MoU, kuota, penempatan, nilai, finalisasi, sertifikat → core `POST /internal/audit`. Penempatan dan sertifikat memakai `idempotency_key`.
**4.4 Job (runner T10):** (1) ringkasan konfirmasi absensi untuk pembimbing industri; (2) penempatan `planned` → `active` pada `starts_on`, `active` → `completed` setelah `ends_on` (zona sekolah); (3) hitung ulang `pkl_results` (belum final) saat skor berubah dilakukan langsung di handler, bukan job.
**4.5 Konkurensi BR-PKL-01:** buat penempatan dalam transaksi: `SELECT … FROM pkl_quotas WHERE … FOR UPDATE` → `CountFilled` → `MouCovering` → `CanPlace` → insert. Dua koordinator menempatkan siswa kelima bersamaan → satu gagal.
Tidak ada approval core di menu 139–141 (PRD tidak mensyaratkan).

## 5. Endpoint (JWT, `school_id` dari `EffectiveSchoolID`, prefix `/pkl`)
| Method & path | Peran | Catatan |
|---|---|---|
| `CRUD /pkl/majors` · `PUT /pkl/classroom-majors` (bulk) | koordinator_pkl | |
| `CRUD /pkl/partners` · `GET/POST /pkl/partners/{id}/mous` · `GET /pkl/mous/{id}/file` | koordinator_pkl | FR-139-001; list memuat status MoU (Berlaku / Berakhir / Belum ada) |
| `CRUD /pkl/periods` · `POST /pkl/periods/{id}/close` | koordinator_pkl | tutup → §4.1 (b) |
| `GET/PUT /pkl/periods/{id}/quotas` (matriks mitra × jurusan, terisi/sisa) | koordinator_pkl | FR-139-002 |
| `GET/POST /pkl/partners/{id}/mentors` · `POST /pkl/mentors/{id}/deactivate` | koordinator_pkl | FR-139-003; respons POST memuat `default_password` sekali |
| `GET /pkl/placements/check?period_id=&partner_id=&student_id=` | koordinator_pkl | pratinjau MoU/kuota untuk peringatan di form (**AC-139-01**) |
| `GET/POST /pkl/placements` · `PUT /pkl/placements/{id}` · `POST /pkl/placements/{id}/cancel` | koordinator_pkl | POST `Idempotency-Key`; 422 dengan pesan `CanPlace` (**AC-140-01**) |
| `GET /pkl/placements/{id}` · `GET /pkl/placements/{id}/attendance` · `/journals` · `/final-reports` · `/scores` | koordinator, pembimbing sekolah & industri terkait (`Scope`) | |
| `POST /pkl/attendance/{id}/confirm` · `/reject` `{reason}` | pembimbing industri penempatan itu | FR-140-002 |
| `GET /pkl/recap?period_id=&partner_id=&format=csv` | koordinator_pkl; pembimbing sekolah (siswanya) | FR-140-003 |
| `GET/PUT /pkl/periods/{id}/scheme` (+components) | koordinator_pkl | `ValidateScheme`; dikunci setelah ada skor |
| `PUT /pkl/placements/{id}/scores` `{assessor, items[]}` | pembimbing industri (assessor `industry`), pembimbing sekolah (`school`) | FR-141-002; assessor salah → 403 |
| `POST /pkl/placements/{id}/journals/{jid}/note` | pembimbing sekolah & industri terkait | catatan pembimbing |
| `POST /pkl/placements/{id}/finalize` · `POST /pkl/placements/{id}/certificate` · `GET /pkl/certificates/{id}/file` | koordinator_pkl (unduh: + siswa pemilik) | FR-141-003 |
| `GET /pkl/my/supervision` | guru/walikelas (pembimbing sekolah), pembimbing_industri | daftar penempatan yang dibimbing; 200 kosong = menu disembunyikan |
| Siswa: `GET /pkl/me` · `POST /pkl/me/attendance/check-in` · `/check-out` · `POST /pkl/me/attendance/leave` (izin/sakit + lampiran) · `GET/POST/PUT /pkl/me/journals` (multipart foto) · `POST /pkl/me/final-report` · `GET /pkl/me/result` | siswa dengan penempatan | 404 jika tidak punya penempatan |

## 6. Frontend
`web/src/services/studentlife/pkl.ts` + tipe di `types.ts`; route di `web/src/router/domains/studentlife.routes.ts`, semua `meta.feature: 'pkl'`. Landing `koordinator_pkl` → `pkl/penempatan`, `pembimbing_industri` → `pkl/bimbingan` (cabang `DashboardRedirectView.vue` jika T9 belum). Menu PKL untuk siswa/guru memakai store `usePklStore` (dimuat sekali setelah login, pola `stores/employee.ts`): tampil hanya jika `GET /pkl/me` / `GET /pkl/my/supervision` berisi data; 404/kosong menyembunyikan menu tanpa error.

| Path | Name | View (`views/studentlife/pkl/…`) | Peran | navGroup / navLabel |
|---|---|---|---|---|
| `pkl/mitra` · `pkl/mitra/:id` | pkl-partners, pkl-partner | `PartnersView.vue` (badge berteks status MoU), `PartnerDetailView.vue` (tab Profil / MoU (riwayat + unggah) / Pembimbing Industri (tambah akun → dialog sekali tampil kata sandi awal + tombol salin) / Kuota) | koordinator_pkl | PKL / Mitra & Lokasi |
| `pkl/periode` · `pkl/periode/:id` | pkl-periods, pkl-period | `PeriodsView.vue`, `PeriodDetailView.vue` (tab Kuota (matriks mitra × jurusan, terisi/sisa) / Skema Penilaian / Tutup Periode) | koordinator_pkl | PKL / Periode & Kuota |
| `pkl/jurusan` | pkl-majors | `MajorsView.vue` (master + pemetaan kelas → jurusan) | koordinator_pkl | PKL / Jurusan |
| `pkl/penempatan` · `pkl/penempatan/:id` | pkl-placements, pkl-placement | `PlacementsView.vue` (form: periode, siswa, mitra → panel peringatan dari `/placements/check` "MoU sudah tidak berlaku" / "Kuota penuh 4/4", tombol simpan disabled bila tidak lolos), `PlacementDetailView.vue` (tab Absensi / Jurnal / Laporan / Penilaian / Sertifikat) | koordinator_pkl | PKL / Penempatan |
| `pkl/rekap` | pkl-recap | `RecapView.vue` (per siswa / per mitra, unduh CSV, "Diperbarui: …") | koordinator_pkl, pembimbing sekolah | PKL / Rekap Kehadiran |
| `pkl/bimbingan` · `pkl/bimbingan/:placementId` | pkl-supervision, pkl-supervision-detail | `SupervisionView.vue` (daftar siswa bimbingan + jumlah absensi menunggu), `SupervisionDetailView.vue` (konfirmasi/tolak absensi massal, baca jurnal + catatan, isi komponen nilai sesuai assessor) | pembimbing_industri; guru/walikelas yang jadi pembimbing sekolah | PKL / Bimbingan PKL |
| `siswa/pkl` | student-pkl | `views/student/PklView.vue` (tab Hari Ini (tombol Check-in/Check-out besar ≥44px, jam server, ajukan izin/sakit) / Jurnal (form harian + unggah foto) / Laporan Akhir / Nilai & Sertifikat) | siswa (punya penempatan) | Portal Siswa / PKL |

Tiga state di semua halaman, badge berteks (Menunggu / Terkonfirmasi / Ditolak), dialog konfirmasi untuk batal penempatan, tutup periode, finalisasi, dan nonaktifkan pembimbing. Kata sandi awal tidak disimpan di store/`localStorage`.

## 7. Bukti handoff
- Test tabel semua fungsi §3 (termasuk pembulatan `FinalScore` dan `MouCovering` batas tanggal).
- Test handler: **AC-139-01** (`/placements/check` memperingatkan + POST 422 dengan pesan MoU), **AC-140-01** (kuota 4 terisi 4 → 422; batal satu → bisa lagi; dua request paralel ke slot terakhir → tepat satu 201), **AC-141-01** (skor industri 85 & sekolah 80 dengan bobot 70/30 → `final_x100=8350`, respons "83,5"), FR-139-003 (pembimbing industri A tidak bisa membaca penempatan pembimbing B → 404; tidak bisa mengisi assessor `school` → 403; fake identity: create-or-link dipanggil sekali, email lama → `created=false`), FR-140-002/003 (check-in di luar periode ditolak, konfirmasi, rekap), FR-141-001 (jurnal + foto, laporan versi), FR-141-003 (sertifikat PDF diawali `%PDF-`, nomor unik, ditolak sebelum finalisasi).
- Test identity: endpoint §4.1 (peran di luar daftar putih → 400, `default_password` hanya saat dibuat, peran ditambah ke sekolah kedua).
- Migrasi studentlife dari nol up/down 1/up.
- Playwright: koordinator membuat jurusan + mitra + MoU + kuota (4) + akun pembimbing industri → menempatkan 4 siswa, siswa kelima ditolak → siswa check-in & isi jurnal → pembimbing industri login (ganti sandi pertama) hanya melihat siswanya, konfirmasi absensi, isi nilai → pembimbing sekolah isi nilai → koordinator finalisasi (83,5) → sertifikat terunduh oleh siswa; MoU kedaluwarsa memunculkan peringatan.
