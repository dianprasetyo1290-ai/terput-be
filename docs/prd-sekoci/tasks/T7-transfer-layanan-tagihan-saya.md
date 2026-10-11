# T7: Transfer & Withdrawal Siswa (78), Permintaan Layanan Orang Tua (81), saklar Tagihan Saya (82)

**Branch:** `feat/T7-transfer-layanan` · **Migrasi:** `academic/0005_transfers.*`, `core/0002_service_requests.*`, `billing/0004_student_bill_visibility.*`, `identity/0016_*` (jika perlu)
**Pembagian gelombang:** **T7a** = §1 (menu 78) + §3 (menu 82), gelombang 1. **T7b** = §2 (menu 81), gelombang 2, karena butuh service `core` hasil T1.
**Prasyarat:** kontrak T1 (approval, audit, notifikasi). Boleh **paralel** dengan T1: uji client core dengan `httptest` fake. Sentuh billing hanya di file baru plus satu baris registrasi route (T6 juga bekerja di billing).

---

## 1. Menu 78: Transfer & Withdrawal Siswa (service `academic`)
PRD: FR-78-001..007, BR-STU-01..03, BR-GEN-07, OQ-07, AC-78-01..03.

### 1.1 Migrasi `academic/0005_transfers.up.sql`
```
student_status_requests(id, school_id, student_user_id, kind CHECK IN('class_transfer','unit_transfer','withdrawal'),
  from_classroom_id NULL, to_classroom_id NULL, to_school_id NULL, effective_date date NOT NULL, reason text NOT NULL,
  attachments jsonb default '[]',        -- object_key + nama file (pola upload yang sudah ada)
  finance_snapshot jsonb NULL,           -- ringkasan saat diajukan (FR-78-002), tidak berubah setelahnya
  refund_amount_idr bigint NULL, status CHECK IN('pending_approval','approved','rejected','cancelled','applied'),
  approval_id NULL, applied_at NULL, requested_by, idempotency_key, UNIQUE(school_id, idempotency_key))
academic_settings(school_id PK, post_exit_bill_policy CHECK IN('void','keep') default 'void',   -- OQ-07
  parent_grace_days int default 30)
```
Status siswa: perlebar CHECK `class_enrollments.status` dengan `'withdrawn'` (pola 0013). **Jangan hapus baris** (BR-STU-01).

### 1.2 Alur
1. `POST /student-status-requests` (admin, walikelas untuk kelasnya) → ambil ringkasan keuangan dari billing `GET /internal/students/{id}/financial-summary?school_id=` (**baru di billing**: tagihan terbuka, belum jatuh tempo, deposit, pembayaran yang dapat dikembalikan; perluas `InternalStudentOutstanding` yang sudah ada, jangan membuat duplikat). Simpan sebagai snapshot. Tampilkan di UI (**AC-78-01**: tunggakan 800.000).
2. Kirim ke core approval `student_withdrawal` / `student_transfer`, `amount_idr` = tunggakan, flags `has_outstanding`, `has_deposit`. Flow yang dikonfigurasi admin wajib memuat tahap keuangan (BR-STU-02). Status siswa **tidak berubah** sebelum approved.
3. Callback approved → `approved`. Job harian (atau langsung jika `effective_date <= today`) menerapkan:
   - **class_transfer:** tutup enrollment lama, buka enrollment baru per tanggal efektif.
   - **unit_transfer (AC-78-02):** enrollment lama `transferred_out`. Di sekolah tujuan, buat/aktifkan siswa lewat identity `POST /internal/students/create-or-link` (sudah ada) untuk `to_school_id`. Billing: tagihan sebelum tanggal efektif tetap di sekolah asal, tagihan setelahnya diperlakukan sesuai policy. Generator tagihan berikutnya mengikuti sekolah baru (BR-STU-03).
   - **withdrawal:** enrollment `withdrawn`. Billing `POST /internal/students/{id}/exit` `{effective_date, policy}` (**baru**): hentikan generate tagihan berikutnya, tagihan jatuh tempo setelah tanggal efektif di-`void` (status baru di CHECK billing, dengan audit) atau dibiarkan (`keep`) (FR-78-005).
   - **Refund (FR-78-006, AC-78-03):** jika `refund_amount_idr > 0`, panggil endpoint refund billing yang sudah ada (migrasi `0002_refunds`, cari handler-nya) lewat endpoint internal baru yang membungkus handler itu, dengan idempotency `withdrawal:<id>`.
   - **Akses (FR-78-007):** identity `POST /internal/users/{id}/deactivate` (dibuat T2. Jika T2 belum merge, tambahkan di T7 dengan kontrak **persis sama**: koordinasikan lewat handoff). Akun orang tua: job identity menonaktifkan setelah `parent_grace_days` jika tidak ada anak aktif lain.
4. Semua langkah idempoten (`applied_at`), diaudit ke core.

### 1.3 Frontend
| Path | View | Peran | Label |
|---|---|---|---|
| `akademik/mutasi-siswa` | `views/academic/StudentStatusRequestsView.vue` (daftar + filter jenis/status) | admin, walikelas, kepala_sekolah | Akademik / Pindah & Keluar Siswa |
| `akademik/mutasi-siswa/baru` | `StudentStatusRequestFormView.vue`: pilih siswa → jenis → (kelas/sekolah tujuan) → tanggal efektif → alasan → lampiran. Panel kanan "Ringkasan Keuangan" (dimuat begitu siswa dipilih), dengan nominal refund diisi keuangan | sama | — |
| `akademik/mutasi-siswa/:id` | detail + `ApprovalTimeline` + status penerapan | sama | — |
Pengaturan `post_exit_bill_policy` & `parent_grace_days` ada di halaman pengaturan akademik yang sudah ada (cari dulu), atau tab baru jika belum ada.

### 1.4 Catatan peran
"Staf keuangan" = `finance_manager` (T1). Jika T1 belum merge, pakai konstanta peran yang sama. Jangan membuat nama peran lain.

---

## 2. Menu 81: Permintaan Layanan Orang Tua (service `core`)
### 2.1 Migrasi `core/0002_service_requests.up.sql`
```
service_request_categories(id, school_id, name, description, handler_role text NOT NULL, sla_hours int NULL,
  requires_student bool default true, active bool, deleted_at)
service_requests(id, school_id, category_id, requester_user_id, student_user_id NULL, subject, description,
  attachments jsonb, status CHECK IN('submitted','accepted','in_progress','done','rejected'),
  assignee_user_id NULL, result_attachments jsonb default '[]', created_at, updated_at)
service_request_events(id, school_id, request_id, from_status, to_status, note, actor_user_id, created_at)
```
Unit (`school_id`) = sekolah anak (AC-81-01). Orang tua dengan anak di 2 sekolah memilih anak, lalu school mengikuti anak (verifikasi lewat identity `/internal/guardians/{id}/students/{sid}/access`).

### 2.2 Endpoint
| Method & path | Peran |
|---|---|
| `CRUD /service-request-categories` | admin |
| `POST /parent/service-requests` (multipart) · `GET /parent/service-requests` · `GET /parent/service-requests/{id}` | orang_tua (hanya miliknya, anak terverifikasi) |
| `GET /service-requests?status=` (daftar kerja) | user dengan `handler_role` kategori itu di sekolah itu |
| `POST /service-requests/{id}/status` `{to, note}` · `POST /{id}/result` (unggah dokumen hasil) | handler. Transisi valid: submitted→accepted/rejected, accepted→in_progress, in_progress→done/rejected. `rejected` wajib note. `done` boleh dengan lampiran |
| `GET /parent/service-requests/{id}/files/{key}` | orang_tua pemilik (URL bertanda tangan pendek) |
Setiap perubahan status → notifikasi ke orang tua (FR-81-004, **AC-81-02**). Request baru → notifikasi ke semua user `handler_role` di sekolah itu. Penyimpanan file: salin modul storage dari billing (`internal/storage`) ke core.

### 2.3 Frontend
| Path | View | Peran | Label |
|---|---|---|---|
| `orang-tua/layanan` · `/baru` · `/:id` | `views/parent/ServiceRequestsView.vue`, `ServiceRequestFormView.vue`, `ServiceRequestDetailView.vue` (timeline status, unduh dokumen hasil) | orang_tua | Layanan Sekolah |
| `layanan/permintaan` · `/:id` | `views/core/ServiceRequestInboxView.vue`, `ServiceRequestWorkView.vue` (ubah status + catatan + unggah hasil) | peran handler (tampil di nav jika user punya peran handler kategori mana pun: endpoint `GET /service-requests/can-handle`) | Permintaan Layanan |
| `pengaturan/kategori-layanan` | `views/core/ServiceRequestCategoriesView.vue` | admin | Pengaturan / Kategori Layanan |

---

## 3. Menu 82: Tagihan Saya, saklar per jenjang (billing + web)
- Migrasi `billing/0004_student_bill_visibility.up.sql`: `student_bill_visibility(school_id, grade_level text, enabled bool, PRIMARY KEY(school_id, grade_level))`. `grade_level` = nilai `classrooms.grade_level` di academic. Tidak ada baris → **nonaktif** (FR-82-001 Should/Add-on, aman secara default).
- `GET/PUT /admin/student-bill-visibility` (admin): daftar grade_level diambil dari academic internal (cari endpoint kelas yang sudah ada).
- `GET /me/bills` dan `/me/payments` yang sudah ada: tambahkan cek. Ambil grade_level siswa (academic internal, enrollment aktif) → kalau tidak aktif, **403** `"Tagihan tidak ditampilkan untuk jenjang ini"`. Tambahkan `GET /me/bills/visibility` → `{enabled}` untuk menu.
- Frontend: `student.routes.ts` route `siswa/keuangan` (sudah ada, `StudentFinanceView.vue`) diganti label **Tagihan Saya**. Tampilkan di nav hanya jika `visibility.enabled` (cek sekali saat login, simpan di store yang ada) (**AC-82-01**). Pastikan view **tidak punya tombol bayar** (**AC-82-02**, BR-GEN-04). Pengaturan: tab di halaman pengaturan keuangan, tabel jenjang × saklar.

## 4. Bukti handoff
AC-78-01..03, AC-81-01/02, AC-82-01/02 sebagai test handler; test transisi status layanan (transisi invalid → 422); test penerapan mutasi idempoten (jalan 2× → satu efek). Playwright: (a) withdrawal dengan tunggakan → approval keuangan → status berubah di tanggal efektif; (b) orang tua ajukan surat keterangan → admin selesaikan dengan lampiran → orang tua unduh; (c) siswa SD tanpa menu Tagihan Saya, siswa SMK punya menu tanpa tombol bayar.
