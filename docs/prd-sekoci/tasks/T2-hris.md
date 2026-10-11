# T2: Service `hr`: Master Pegawai, Organisasi & Jabatan, Dokumen, Absensi & Cuti, ESS

**Menu PRD:** 83, 84, 85, 86, 87 (Bagian 2 §5 Grup 12, BR-HR-01..05, NFR-HR-*) · **Branch:** `feat/T2-hris` · **Port:** 8092 · **DB:** `<prefix>_hr`
**Prasyarat:** kontrak T1 (§3 approval/exception/audit, peran `hr_admin, hr_manager, pegawai`). Di mode paralel, T1 dikerjakan bersamaan: koding sesuai kontrak dan uji dengan `httptest` fake. **Migrasi:** `hr/0001`–`0009`, `identity/0015`.
**Feature key:** `hris`. Semua route, endpoint, dan job hanya aktif jika fitur ini menyala untuk sekolah (cek `GET {IDENTITY_URL}/schools/{id}/features` dengan X-Internal-Key, cache 60 detik di memori).
**Istilah:** unit = sekolah (`school_id`), yayasan = foundation. Data lintas unit di yayasan dilihat lewat scope yayasan yang sudah ada di identity (`kepala_yayasan`).

## 1. Donor
| Ambil | Dari | Catatan port |
|---|---|---|
| Model pegawai, riwayat, org chart | `virelis/backend/internal/handlers/employees.go` (`EmployeeTimeline`, `OrgChart`), `virelis/.../models/models.go` | gin+GORM → chi+pgx. Field disesuaikan dengan §3 |
| Enkripsi kolom + blind index | `hris-mulnco/backend/internal/usecase/crypto.go` | salin hampir utuh ke `services/hr/internal/crypto/`, beserta test-nya. Kunci dari env `HR_ENC_KEY` (32 byte base64) dan `HR_BLIND_KEY`. **Tidak ada default** di config: service gagal start jika kosong |
| Hari kerja, saldo cuti, cuti/lembur | `virelis/.../handlers/leave.go` (`workingDays`, `CreateLeaveRequest`, `DecideLeaveRequest`, `CancelLeaveRequest`, `MyLeaveBalances`, `LeaveCalendar`) | Keputusan **tidak** lagi diambil di hr: hr mengirim ke core approval lalu menerima callback |
| Check-in, koreksi, statistik | `virelis/.../handlers/attendance.go` (`ClockIn`, geofence, `CreateCorrection`, `DecideCorrection`, `AttendanceStats`) + `attendance_geofence_test.go` | Geofence dibungkus saklar per sekolah (OQ-23, default mati) |

## 2. Kerangka service
Ikuti layout `services/billing` (sama seperti T1 §1). Buat `internal/jobs/runner.go` sendiri sesuai spesifikasi T1 §5 (ticker + `pg_try_advisory_lock`). Di mode paralel core belum ada di worktree-mu, dan kebijakan repo memang salin, bukan impor. Daftarkan di compose, `.env.example` (`HR_PORT=8092`, `HR_DB_NAME`, `HR_ENC_KEY`, `HR_BLIND_KEY`, `CORE_URL`, `IDENTITY_URL`, `NOTIFICATION_URL`, `STORAGE_*` mengikuti pola penyimpanan file yang sudah dipakai billing/admission untuk `object_key`), `client.ts` (`hr`), `VITE_HR_URL`.

## 3. Migrasi `0001_hris.up.sql`
Semua tabel memakai `id uuid`, `school_id uuid NOT NULL`, `created_at`/`updated_at`. Tabel master memakai soft delete `deleted_at`.

**Organisasi (84)**
```
departments(id, school_id, code, name, parent_id NULL FK self, active, deleted_at, UNIQUE(school_id,code) WHERE deleted_at IS NULL)
positions(id, school_id, code, name, level int, job_description text, department_id FK NULL,
  salary_min_idr bigint NULL, salary_max_idr bigint NULL,          -- dipakai BR-REC-03 di T4
  CHECK(salary_min_idr IS NULL OR salary_max_idr >= salary_min_idr), deleted_at, UNIQUE(school_id,code))
headcount_plans(id, school_id, position_id FK, department_id FK NULL, planned int CHECK(planned>=0),
  effective_from date, effective_to date NULL)                      -- formasi berlaku per tanggal (FR-84-006)
org_change_log(id, school_id, entity_type, entity_id, before jsonb, after jsonb, effective_date, actor_user_id)
```
**Pegawai (83)**
```
employees(id, school_id, user_id uuid NULL UNIQUE,                 -- akun identity (guru/pegawai)
  employee_no text NOT NULL, full_name text NOT NULL, email text NULL, phone_enc bytea NULL,
  nik_enc bytea, nik_bidx text, npwp_enc bytea NULL, npwp_bidx text NULL,
  ptkp_status text NOT NULL REFERENCES ptkp_statuses(code),   -- master di T3, buat tabel kode di sini: TK/0..K/3 (kode saja, tanpa nominal)
  bank_code text NULL, bank_account_enc bytea NULL, bank_account_name text NULL,
  birth_date date NULL, gender text NULL CHECK(gender IN('L','P')), address_enc bytea NULL,
  hire_date date NOT NULL, exit_date date NULL, exit_reason text NULL,
  status text NOT NULL CHECK(status IN('candidate','active','exited')),
  UNIQUE(school_id, employee_no), UNIQUE(school_id, nik_bidx))
employment_assignments(id, school_id, employee_id FK, position_id FK, department_id FK NULL,
  supervisor_employee_id uuid NULL FK employees,                    -- atasan langsung (FR-84-003)
  employment_type text CHECK(employment_type IN('tetap','kontrak','honorer','bukan_pegawai')),
  grade text NULL, base_salary_enc bytea NULL,                      -- gaji pokok terenkripsi
  contract_end date NULL, effective_from date NOT NULL, effective_to date NULL,
  EXCLUDE USING gist (employee_id WITH =, daterange(effective_from, effective_to, '[)') WITH &&))   -- butuh btree_gist
```
- Riwayat (BR-HR-02): perubahan jabatan/gaji/status **menutup** baris aktif (`effective_to = tanggal efektif`) lalu menambah baris baru. Tidak boleh UPDATE di tempat dan tidak boleh DELETE.
- `current_assignment(employee_id, on date)` adalah fungsi Go di store, bukan view (ingat gotcha view `SELECT t.*`).

**Dokumen (85)**
```
document_types(id, school_id, code, name, has_expiry bool, deleted_at)
required_documents(id, school_id, employment_type, document_type_id, UNIQUE(school_id,employment_type,document_type_id))
employee_documents(id, school_id, employee_id, document_type_id, version int, object_key, file_name, mime, size_bytes,
  expires_on date NULL, uploaded_by uuid, requested_by_hr bool default false, superseded_at NULL,
  UNIQUE(employee_id, document_type_id, version))
document_requests(id, school_id, employee_id, document_type_id, note, status CHECK IN('open','fulfilled','cancelled'))
```
**Absensi, cuti, lembur (86/87)**
```
work_schedules(id, school_id, name, position_id NULL, department_id NULL,
  days smallint[] NOT NULL,       -- 1=Senin..7=Minggu
  start_time time, end_time time, late_tolerance_minutes int default 0, effective_from date, effective_to NULL)
holidays(id, school_id, date, name, UNIQUE(school_id,date))
employee_attendance(id, school_id, employee_id, work_date date,
  status text CHECK(status IN('hadir','terlambat','izin','sakit','cuti','dinas_luar','alpa')),
  check_in_at timestamptz NULL, check_out_at timestamptz NULL, lat numeric(9,6) NULL, lng numeric(9,6) NULL,
  source text CHECK(source IN('ess','manual','import','leave')), import_batch_id NULL, note,
  UNIQUE(employee_id, work_date))
attendance_import_batches(id, school_id, file_name, column_map jsonb, total int, ok int, failed int, errors jsonb, created_by)
attendance_import_profiles(id, school_id, name, column_map jsonb, date_format text)   -- IF-HR-05: pemetaan kolom mesin disimpan, bukan hardcode
leave_types(id, school_id, code, name, has_quota bool, annual_quota_days numeric(5,1) NULL,
  accrual text CHECK(accrual IN('none','monthly')), carry_over_max_days numeric(5,1) default 0,
  requires_document bool default false, deleted_at)
leave_balances(id, school_id, employee_id, leave_type_id, year int, entitled numeric(5,1), carried numeric(5,1),
  used numeric(5,1) default 0, UNIQUE(employee_id, leave_type_id, year), CHECK(used >= 0))
leave_requests(id, school_id, employee_id, leave_type_id, start_date, end_date, days numeric(5,1), reason,
  attachment_key NULL, status CHECK IN('pending','approved','rejected','cancelled'), approval_id uuid NULL,
  idempotency_key, UNIQUE(school_id, idempotency_key))
overtime_requests(id, school_id, employee_id, work_date, start_at timestamptz, end_at timestamptz, minutes int, reason,
  status ..., approval_id, idempotency_key)
attendance_corrections(id, school_id, employee_id, work_date, before jsonb, after jsonb, reason, status, approval_id)
attendance_periods(id, school_id, period char(7) /* '2026-10' */, status CHECK IN('open','sent','locked'),
  sent_at, sent_by, UNIQUE(school_id, period))
```
**ESS (87)**
```
profile_change_requests(id, school_id, employee_id, field text CHECK(field IN('address','phone','bank_account','ptkp_status')),
  new_value_enc bytea, status, approval_id, applied_at NULL)
```
**Konfigurasi per sekolah** (OQ, tanpa hardcode):
```
hr_settings(school_id PK, contract_reminder_days int default 30, document_reminder_days int default 30,
  geolocation_enabled bool default false, geofence jsonb NULL /* {lat,lng,radius_m} */, timezone text default 'Asia/Jakarta')
```

## 4. Aturan & fungsi murni (`internal/model`, test tabel wajib)
| Fungsi | Isi | Test (minimal) |
|---|---|---|
| `WorkingDays(start, end, schedule.days, holidays) float64` | hitung hari kerja inklusif, lewati libur & hari non-jadwal | rentang lintas akhir pekan, libur nasional, start>end → error |
| `CanTakeLeave(balance, days)` | BR-HR-04: hanya jika `has_quota` dan `entitled+carried-used >= days` | **AC-86-01** (saldo 2, minta 3 → ditolak, pesan "Saldo cuti tidak mencukupi (sisa 2 hari)") |
| `ApplyLeaveDecision` | approved → `used += days` **dalam transaksi yang sama** dengan perubahan status + baris `employee_attendance(status='cuti', source='leave')` per hari kerja; cancelled setelah approved → kembalikan saldo + hapus baris absensi cuti | **AC-86-02** (12 → 10), cancel → 12 lagi |
| `AttendanceStatus(checkIn, schedule)` | hadir / terlambat (lewat toleransi) | batas tepat menit toleransi |
| `MaskEmployee(emp, viewerRoles, viewerUserID)` | BR-HR-03: tampil penuh hanya untuk `hr_admin, hr_manager, payroll_officer` atau pemilik data. Lainnya tersamar (`••••1234`) | **AC-83-04** (kepala_sekolah → tersamar) |
| `DuplicateCheck(nik_bidx, email)` | FR-83-004: kembalikan kandidat duplikat; create diblok 409 kecuali `force=true` + alasan (diaudit) | NIK sama, email sama beda kapitalisasi |
| `DocumentCompleteness(required, docs, today)` | daftar dokumen wajib yang belum ada **atau** kedaluwarsa | **AC-85-02** → `["kontrak"]` |
| `Accrue(leaveType, monthsWorked)` | akrual bulanan = `annual_quota/12 * bulan`, dibulatkan ke 0,5 ke bawah | 12 hari, 5 bulan → 5,0 |
| `CarryOver(prevRemaining, max)` | `min(sisa, max)` | |

## 5. Integrasi
- **Approval (T1):** create leave/overtime/correction/profile_change → `POST {CORE_URL}/internal/approvals` (`request_type` = `leave|overtime|attendance_correction|profile_change`, `subject_user_id` = user pegawai, `callback_url` = `{HR_PUBLIC_INTERNAL_URL}/internal/approval-callbacks`, `idempotency_key` = `<type>:<id>`). Jika core menolak (422 karena alur belum dikonfigurasi) → kembalikan pesan itu ke UI, **jangan auto-approve**.
- **Callback** `POST /internal/approval-callbacks`: dispatch per `request_type`, idempoten (kalau status sudah final, langsung 200).
- **Atasan langsung untuk core:** `GET /internal/employees/by-user/{userId}/supervisor?school_id=` → `{user_id}` dari `supervisor_employee_id` assignment aktif (AC-84-01). Atasan tanpa akun → 404 dengan pesan.
- **Identity (BR-HR-01, FR-83-006):** tambahkan di identity (handler baru `internal_employees.go`, pola `internalUpsertUser`):
  - `POST /internal/employees/create-or-link` `{school_id, email, full_name, role:'guru'|'pegawai'}` → `{user_id}`.
  - `POST /internal/users/{id}/deactivate` `{school_id, effective_at}`: kalau tanggalnya hari ini atau sudah lewat, langsung nonaktif. Kalau di masa depan, simpan di kolom baru `users.deactivate_at` dan Login menolak setelah tanggal itu. Lebih sederhana dan pasti daripada job terpisah.
  - Saat fitur `hris` aktif, handler update user di identity (`users.go`) **menolak** perubahan nama/email/unit untuk user yang punya `employees.user_id` (identity menyimpan flag `managed_by_hr bool` yang diisi oleh create-or-link) dengan 409 "Data dikelola dari HRIS" (AC-83-02). Data Guru di frontend menampilkan badge "Dikelola HRIS" dan field read-only.
- **Notifikasi:** pengingat kontrak/dokumen/status pengajuan → `/internal/notifications` dengan Idempotency-Key `hr:<jenis>:<id>:<tanggal>`.
- **Audit:** setiap create/update/offboarding/unlock → `/internal/audit` (field sensitif sudah dimasking oleh core, tetapi hr **tidak boleh** mengirim plaintext NIK/rekening/gaji sama sekali: kirim `"changed": ["bank_account"]`).
- **Payroll (T3):** `POST /hr/attendance-periods/{period}/send` mengubah status ke `sent` lalu `locked`. Setelah itu, perubahan absensi pada periode tersebut membuat `attendance_corrections` + approval (AC-86-04).

## 6. Job (runner T1, advisory lock)
1. Pengingat kontrak berakhir H-`contract_reminder_days` (AC-83-01): kirim sekali per kontrak (idempotency key memuat tanggal kontrak).
2. Pengingat dokumen kedaluwarsa H-`document_reminder_days` ke HR + pegawai (AC-85-01).
3. Akrual cuti bulanan + carry-over saat ganti tahun (idempoten per `(employee, type, year, month)`).
4. Offboarding terjadwal: pegawai `exit_date <= today` → status `exited`.

## 7. Endpoint (JWT, semua `school_id` dari `EffectiveSchoolID`)
| Method & path | Peran | Catatan |
|---|---|---|
| `GET/POST /employees`, `GET/PUT /employees/{id}` | hr_admin, hr_manager (tulis: hr_admin) | list: filter unit/jabatan/status/teks + pagination. Respons lewat `MaskEmployee` |
| `POST /employees/{id}/assignments` | hr_admin | perubahan jabatan/gaji/atasan dengan `effective_from` |
| `GET /employees/{id}/timeline` | hr_admin, hr_manager | riwayat assignment + perubahan (FR-83-002) |
| `POST /employees/{id}/offboard` | hr_admin | `{exit_date, reason}` → identity deactivate (AC-83-03) |
| `GET /employees/export` | hr_admin, hr_manager | CSV ber-masking sesuai peran, diaudit |
| `CRUD /departments`, `/positions`, `/headcount-plans` | hr_admin (baca: hr_manager, kepala_sekolah) | |
| `GET /org-chart?department_id=` | sama | pohon dari supervisor (FR-84-005) |
| `GET /headcount?on=` | sama | planned / filled / vacant per jabatan (AC-84-02). Juga `GET /internal/headcount/{positionId}` untuk T4 |
| `CRUD /document-types`, `/required-documents` | hr_admin | |
| `GET/POST /employees/{id}/documents` · `GET /documents/{id}/download` | hr_admin + pemilik | download pakai URL bertanda tangan pendek (pola admission) dan diaudit |
| `CRUD /work-schedules`, `/holidays`, `/leave-types` | hr_admin | |
| `GET /attendance?period=&employee_id=` · `PUT /attendance/{id}` | hr_admin | PUT di periode locked → buat koreksi + approval |
| `POST /attendance/import` (multipart + `profile_id`) | hr_admin | parse CSV/XLSX (pakai lib xlsx yang sudah ada di repo, cek `go.mod` service lain dulu), hasil per baris: ok/galat (AC-86-03: NIK/no pegawai tak dikenal → galat, baris lain tetap masuk) |
| `GET /attendance/recap?period=&format=csv` | hr_admin, hr_manager | FR-86-005 |
| `POST /attendance-periods/{period}/send` | hr_admin | FR-86-006 |
| `GET /leave-requests` (antrian HR) | hr_admin, hr_manager | |
| **ESS** `GET /me/employee` · `GET /me/dashboard` | pegawai, guru (yang punya employees.user_id) | FR-87-001: sisa cuti, pengajuan pending, slip terbaru (T3, tampilkan kosong sampai T3 ada), pelatihan wajib (T5) |
| `POST /me/attendance/check-in` · `/check-out` | sama | waktu dari **server**, lokasi hanya jika `geolocation_enabled` (OQ-23) dan diverifikasi geofence |
| `GET/POST /me/leave-requests` · `POST /me/leave-requests/{id}/cancel` · `GET /me/leave-balances` | sama | |
| `GET/POST /me/overtime-requests` | sama | |
| `POST /me/profile-change-requests` | sama | AC-87-01: nilai lama tetap dipakai sampai approval callback `approved` diterapkan |
| `GET /me/documents` · `POST /me/documents` (hanya untuk `document_requests` terbuka) | sama | |

## 8. Frontend
`web/src/services/hr/{employees.ts,org.ts,documents.ts,attendance.ts,leave.ts,ess.ts,types.ts}`, `web/src/router/domains/hr.routes.ts`, semuanya dengan `meta.feature: 'hris'`.

| Path | Name | View (`views/hr/…`) | Peran | navGroup / label |
|---|---|---|---|---|
| `hr/pegawai` | hr-employees | `EmployeesView.vue` | hr_admin, hr_manager | SDM / Master Pegawai |
| `hr/pegawai/baru` & `hr/pegawai/:id` | hr-employee-form, hr-employee-detail | `EmployeeFormView.vue`, `EmployeeDetailView.vue` | hr_admin | — |
| `hr/organisasi` | hr-org | `OrganizationView.vue` (tab Bagian / Jabatan / Formasi / Bagan) | hr_admin, hr_manager, kepala_sekolah | SDM / Organisasi & Jabatan |
| `hr/dokumen` | hr-documents | `DocumentsView.vue` (tab Kedaluwarsa / Belum Lengkap / Jenis & Wajib) | hr_admin | SDM / Dokumen Pegawai |
| `hr/absensi` | hr-attendance | `AttendanceView.vue` (tab Harian / Import / Rekap / Periode) | hr_admin, hr_manager | SDM / Absensi & Cuti |
| `hr/cuti` | hr-leave | `LeaveAdminView.vue` (tab Pengajuan / Jenis Cuti / Saldo / Jadwal & Libur) | hr_admin, hr_manager | SDM / Absensi & Cuti |
| `pegawai` | ess-home | `ess/EssDashboardView.vue` | pegawai, guru* | Pegawai / Beranda Pegawai |
| `pegawai/kehadiran` | ess-attendance | `ess/EssAttendanceView.vue` | sama | Pegawai / Kehadiran Saya |
| `pegawai/cuti` | ess-leave | `ess/EssLeaveView.vue` (cuti + lembur) | sama | Pegawai / Cuti & Lembur |
| `pegawai/profil` | ess-profile | `ess/EssProfileView.vue` | sama | Pegawai / Profil & Dokumen |

\* guru melihat menu Pegawai **hanya** jika `GET /me/employee` 200 (store `useEmployeeStore`, dicek sekali setelah login). Kalau 404, menu disembunyikan, tidak menampilkan error.

Detail layar:
- **EmployeesView:** tabel (No, Nama, Jabatan, Unit, Status kepegawaian, Status (badge teks), Kontrak berakhir) + filter + pencarian (debounce 300ms) + Ekspor CSV. Empty state dengan CTA "Tambah Pegawai".
- **EmployeeFormView:** seksi Identitas (NIK 16 digit, NPWP 15/16 digit, validasi pola), Kontak, Rekening (kode bank dari master), Penugasan awal (jabatan, bagian, atasan (autocomplete pegawai), jenis, gaji pokok (input rupiah), kontrak berakhir). Simpan → kalau 409 duplikat, tampilkan dialog daftar kandidat duplikat + pilihan "Tetap simpan" dengan alasan wajib.
- **EmployeeDetailView:** header (nama, jabatan, badge status, kelengkapan dokumen), tab Data, Riwayat (timeline vertikal per tanggal efektif), Dokumen (versi, kedaluwarsa, unggah versi baru, minta dokumen ke pegawai), Absensi bulan ini, Cuti. Tombol "Ubah Penugasan" (dialog + tanggal efektif) dan "Proses Keluar" (dialog konfirmasi, alasan wajib). Field sensitif ditampilkan sesuai respons server (server sudah masking, frontend tidak memutuskan).
- **OrganizationView › Bagan:** pohon dari `/org-chart` dengan indent + garis (CSS saja, tanpa lib chart), node berisi nama + jabatan, bisa diciutkan, dan bisa dinavigasi dengan keyboard.
- **Formasi:** tabel jabatan × planned/terisi/kosong, dengan badge "Penuh" bila kosong=0 (AC-84-02).
- **AttendanceView › Import:** pilih profil kolom (atau buat baru: unggah contoh → petakan kolom NIK/no pegawai, tanggal, jam masuk, jam keluar), unggah file, tabel hasil (ok/galat per baris, galat dengan nomor baris + alasan), unduh CSV galat.
- **Periode:** daftar bulan dengan status Terbuka / Terkirim / Terkunci + tombol "Kirim ke Payroll" (dialog konfirmasi yang menjelaskan efek penguncian).
- **EssDashboardView:** kartu StatCard: sisa cuti tahunan, pengajuan menunggu, slip terbaru, pelatihan wajib belum selesai. Tombol besar Check-in / Check-out (status jelas, jam server, disabled saat proses, minimal 44px). Kalau geolokasi aktif dan izin browser ditolak, tampilkan pesan cara mengaktifkan, tidak gagal diam-diam.
- **EssLeaveView:** form cuti (jenis, tanggal mulai/selesai → hitung hari kerja via endpoint pratinjau `GET /me/leave-requests/preview?…`, tampilkan sisa saldo; submit disabled jika saldo kurang), daftar pengajuan + `ApprovalTimeline` (komponen T1) di detail, tombol batalkan.
- **EssProfileView:** data diri (milik sendiri tampil penuh), form permintaan perubahan (alamat/telepon/rekening/PTKP) dengan label "Menunggu persetujuan HR", dokumen saya + unggah untuk permintaan HR.

## 9. Seed
- Migrasi: `ptkp_statuses` kode TK/0, TK/1, TK/2, TK/3, K/0, K/1, K/2, K/3 (hanya kode dan label, nominal PTKP ada di T3).
- **Tidak** ada pegawai, jenis cuti, atau jadwal bawaan. Sediakan `scripts/seed-hr-demo.sql` untuk lokal saja.

## 10. Bukti handoff
Test semua fungsi §4; test handler AC-83-01..04, AC-84-01..02, AC-85-01..02, AC-86-01..04, AC-87-01; test crypto (encrypt/decrypt, blind index deterministik, kunci salah gagal); grep membuktikan tidak ada plaintext NIK/rekening di log (`grep -rn "nik\|account" internal | grep -i log`). Playwright: HR membuat pegawai → pegawai login ESS → ajukan cuti → atasan setujui di Kotak Masuk Approval → saldo berkurang.
