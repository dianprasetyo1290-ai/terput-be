# T18: Perpustakaan di service `ops`: katalog & OPAC, keanggotaan, sirkulasi & denda, laporan

**Menu PRD:** 112, 113, 114, 115 (BR-LIB-01, BR-LIB-02, OQ-33, OQ-34, IF-OPS-04, lampiran "Penerbitan tagihan non-SPP") · **Gelombang:** 4 · **Branch:** `feat/T18-perpustakaan` · **Service:** `ops` (paket `internal/library`) · **Migrasi:** `services/ops/migrations/0040_library.*` (nomor 0040–0049 milik T18)
**Prasyarat:** gelombang 3 di-merge: T9 (kerangka `ops`, peran, feature key), T12 (komponen pemindai, bila ada). Kontrak tagihan ad-hoc billing §4.3 (lihat status pemilik di sana). **Feature key:** `library`. **Peran:** `pustakawan`; anggota = `siswa` + staf ber-akun; laporan juga `kepala_sekolah`. Nama final **mengikuti T9 §2**.

## 1. Acuan kode
- Kerangka `services/ops` dari T9 (pola `services/billing`: `cmd/server/main.go`, `internal/{common,config,handler,store,client,storage}`). Paket baru `internal/library/` (`model.go` fungsi murni, `store.go`, `handler.go`, `jobs.go`).
- Impor Excel: pola tiga langkah identity `GET /import/template` · `POST /import/dry-run` · `POST /import` (`services/identity/internal/handler/import.go`, excelize sudah dipakai di identity/report/hr → boleh ditambahkan ke `ops`, catat di handoff).
- Data siswa aktif & kelas: identity `GET /users?role=siswa&school_id=&status=active` (pola `UsersByRole` di `services/core/internal/client/client.go`), academic `GET /students/{id}/classroom` + `GET /classrooms/{id}` (pola `StudentGradeLevel` di `services/billing/internal/client/client.go`). Pegawai = user identity aktif ber-peran staf (status identity diturunkan T2/T7 lewat `/internal/users/{id}/deactivate`).
- Notifikasi: `POST {NOTIFICATION_URL}/internal/notifications` + `Idempotency-Key` (pola `client.Notify` billing). Audit: `POST {CORE_URL}/internal/audit`.
- Pemindai (IF-OPS-04): pakai komponen pemindai bersama dari T12 (POS) bila sudah ada. Jika tidak ada, komponennya milik **T12**: `web/src/components/ui/ScanInput.vue` (jangan membuat salinan; spesifikasi berikut untuk konsistensi) (input fokus otomatis untuk keyboard wedge, Enter = kirim; tombol kamera memakai `BarcodeDetector` bila didukung browser, selain itu kamera disembunyikan) dan T19 memakai komponen yang sama. Tanpa dependensi baru.
- OQ-34: e-book **tidak** dikerjakan.

## 2. Migrasi `0040_library.up.sql`
```
lib_settings(school_id PK, fine_policy CHECK IN('pay_at_library','bill') default 'pay_at_library',   -- OQ-33 / BR-LIB-02
  reminder_days_before int NOT NULL default 2 CHECK(reminder_days_before >= 0))                    -- FR-114-002 [2]
lib_member_categories(id, school_id, name, applies_to CHECK IN('student','employee'), is_default bool,
  max_loans int, loan_days int, max_renewals int, reservation_hold_days int,                     -- BR-LIB-01, diisi pustakawan (tanpa default)
  deleted_at, UNIQUE(school_id, applies_to) WHERE is_default AND deleted_at IS NULL)
lib_fine_rates(id, school_id, member_category_id NULL, late_fee_per_day_idr bigint,
  lost_fee_idr bigint NULL,                 -- NULL = pakai replacement_price_idr judul
  effective_from date, effective_to date NULL)                                                     -- FR-114-004, BR-GEN-08
lib_members(id, school_id, kind CHECK IN('student','employee'), user_id, category_id, member_code text,  -- kode di kartu anggota
  status CHECK IN('active','pending_close','inactive'), pending_reason NULL, closed_at NULL, synced_at,
  UNIQUE(school_id, user_id), UNIQUE(school_id, member_code))
lib_collection_categories(id, school_id, code, name, deleted_at)
lib_titles(id, school_id, title, author, publisher, isbn NULL, publish_year int NULL, category_id,
  replacement_price_idr bigint NULL, search tsvector GENERATED (title, author, publisher, isbn), deleted_at)
lib_copies(id, school_id, title_id, barcode, shelf_location, status CHECK IN('available','on_loan','on_hold','lost','damaged','withdrawn'),
  deleted_at, UNIQUE(school_id, barcode))                                                          -- FR-112-001
lib_loans(id, school_id, copy_id, member_id, classroom_id NULL, classroom_name NULL,   -- snapshot kelas untuk FR-115-001
  loaned_at, due_date date, returned_at NULL, renew_count int default 0, status CHECK IN('active','returned','lost'),
  loaned_by, returned_by NULL, idempotency_key, UNIQUE(school_id, idempotency_key),
  UNIQUE(copy_id) WHERE status = 'active')
lib_reservations(id, school_id, title_id, member_id, status CHECK IN('waiting','ready','fulfilled','cancelled','expired'),
  held_copy_id NULL, ready_until NULL, created_at,
  UNIQUE(title_id, member_id) WHERE status IN ('waiting','ready'))                                  -- FR-114-003
lib_fines(id, school_id, loan_id, member_id, kind CHECK IN('late','lost'), late_days int NULL, amount_idr bigint,
  rate_snapshot jsonb, settlement CHECK IN('pay_at_library','bill'),
  status CHECK IN('open','paid','billed','waived','void'), bill_id uuid NULL,
  paid_at NULL, received_by NULL, waived_by NULL, waive_reason NULL, created_at,
  UNIQUE(loan_id, kind))                                                                           -- denda idempoten per pinjaman
lib_visits(id, school_id, member_id, visited_on date, visited_at, UNIQUE(member_id, visited_on))   -- sub-fitur kunjungan (115)
-- audit: tidak ada tabel lokal, setiap mutasi dikirim ke core POST /internal/audit
```
Status "terlambat" **bukan** kolom: `status='active' AND due_date < today(Asia/Jakarta)`. Mutasi eksemplar/pinjaman tidak dihapus (soft delete judul/eksemplar via `deleted_at`, eksemplar dipinjam tidak bisa dihapus).

## 3. Fungsi murni (`internal/library/model.go`) + vektor
| Fungsi | Vektor (→ AC/FR) |
|---|---|
| `AvailableCopies(copies)` | **AC-112-01**: 3 eksemplar, 3 `on_loan` → 0. `on_hold` untuk anggota lain tidak dihitung tersedia (FR-112-003) |
| `CanBorrow(member, category, activeLoans, hasOverdue, hasOpenLibraryFine)` | **AC-114-01**: batas 3, sedang pinjam 3 → tolak "Batas pinjam tercapai". Ada pinjaman terlambat → tolak (BR-LIB-01). Denda `open` + `pay_at_library` → tolak; denda `billed` tidak memblokir (ditangani keuangan). Anggota `pending_close`/`inactive` → tolak |
| `DueDate(loanDate, loanDays)` | hari kalender, tanggal `Asia/Jakarta` |
| `LateDays(dueDate, returnedOn)` | kembali tepat/lebih awal → 0; terlambat 4 hari → 4 |
| `LateFine(days, rate)` | **AC-114-02**: Rp1.000/hari × 4 → Rp4.000 (`int64`, tanpa float) |
| `LostFine(rate, title)` | `lost_fee_idr` bila diisi, selain itu `replacement_price_idr`; keduanya kosong → galat "Tarif kehilangan belum diatur" |
| `ResolveRate(rates, categoryID, onDate)` | tarif kategori mengalahkan tarif umum; di luar periode efektif diabaikan |
| `Settlement(policy, memberKind)` | `bill` hanya untuk anggota `student` (tagihan siswa); anggota pegawai selalu `pay_at_library` (BR-LIB-02) |
| `CanRenew(loan, category, today, titleHasWaitingReservation)` | `renew_count < max_renewals`, belum terlambat, tidak ada reservasi menunggu → boleh; due baru = `DueDate(today, loan_days)` (FR-114-003) |
| `CloseMembership(member, activeLoans, openFines)` | **AC-113-01**: siswa keluar 1 Juli, masih 1 pinjaman → `pending_close` + alasan "Masih meminjam 1 buku"; tanpa pinjaman & denda terbuka → `inactive` (FR-113-003) |
| `ReminderKind(loan, today, daysBefore)` | due − 2 hari = hari ini → `due_soon`; hari pertama terlambat → `overdue`; lainnya → none (FR-114-002) |
| `NextReservation(queue)` | FIFO `created_at`; eksemplar kembali → reservasi teratas `ready`, eksemplar `on_hold`, `ready_until = today + reservation_hold_days` |

## 4. Integrasi & job
### 4.1 Sirkulasi (FR-114-001)
`POST /library/loans {member_code, copy_barcode}` (dari pemindai): dalam satu transaksi `SELECT … FOR UPDATE` anggota + eksemplar → `CanBorrow` → insert loan + eksemplar `on_loan` + snapshot kelas. Idempotency-Key dari klien (scan ganda tidak membuat dua pinjaman). `POST /library/returns {copy_barcode}` → tutup loan, hitung `LateFine` → buat `lib_fines` bila > 0 dengan `settlement = Settlement(...)`, jalankan `NextReservation`, cek `CloseMembership` bila anggota `pending_close`. Lapor hilang `POST /library/loans/{id}/lost` → loan `lost`, eksemplar `lost`, denda `lost`.

### 4.2 Keanggotaan otomatis (FR-113-001/003)
Job harian `library_member_sync` (advisory lock, pola `services/core/internal/jobs/runner.go`): siswa aktif & staf aktif yang belum jadi anggota → anggota dengan kategori `is_default` sesuai `applies_to` (tidak ada kategori default → lewati + tampilkan peringatan di halaman anggota). User yang sudah tidak aktif → `CloseMembership`. Tombol "Sinkronkan sekarang" memanggil fungsi yang sama.

### 4.3 Kontrak BILL-ADHOC (billing): denda ditagihkan (BR-LIB-02, FR-114-004)
Billing saat ini hanya punya tagihan berbasis skema (`student_bills.scheme_id NOT NULL`, `POST /internal/bills/generate`); belum ada endpoint tagihan non-SPP ad-hoc. Kontrak berikut dipakai **T18, T19 (menu 122), T21 (134/135)**:
```
POST {BILLING_URL}/internal/bills/adhoc           X-Internal-Key, Idempotency-Key: <source_module>:<source_ref>
{ school_id, student_user_id, source_module:"library"|"transport"|"event"|"extracurricular",
  source_ref:<id sumber>, title, due_at, items:[{name, component_code, amount_idr}] }
→ 201 { bill_id, status:"unpaid", net_idr }     (200 + data sama bila source sudah pernah ditagih)
POST {BILLING_URL}/internal/bills/{id}/void      { school_id, source_module, source_ref, reason }  → 409 bila sudah ada pembayaran terverifikasi
GET  {BILLING_URL}/internal/bills/by-source?school_id=&source_module=&source_refs=a,b
→ [{ source_ref, bill_id, status:"unpaid"|"partial"|"paid"|"void", net_idr, paid_idr }]
```
Implementasi billing **milik T12** (gelombang 3, migrasi `billing/0007_adhoc_bills`; T18 hanya memakai, tidak membangun) → `scheme_id DROP NOT NULL`, kolom `source_module text NULL`, `source_ref text NULL`, `UNIQUE(school_id, source_module, source_ref) WHERE source_module IS NOT NULL`, `CHECK(scheme_id IS NOT NULL OR source_module IS NOT NULL)`; diskon/beasiswa SPP **tidak** diterapkan otomatis; `component_code` (`library_fine`, `event_fee`, `extracurricular_fee`, …) dipakai mapping COA (BR-FIN-06). Semua query billing yang `JOIN billing_schemes` wajib dicek agar tagihan ad-hoc tetap tampil di portal orang tua/kasir/aging (`LEFT JOIN` + nama dari `bill_items`). Selama endpoint belum tersedia, T18 menguji dengan fake `httptest`.
- Denda `settlement='bill'` → panggil `adhoc` (`source_ref = fine_id`, item `Denda perpustakaan: <judul> (<n> hari)`) → `status='billed'`, `bill_id`. Gagal → denda tetap `open` + exception core (`kind='dead_letter'`, retry ke `POST /internal/library/fines/{id}/bill`).
- Status bayar: job harian + saat laporan dibuka memanggil `by-source` → `paid` bila billing `paid`. Pembayaran di perpustakaan: `POST /library/fines/{id}/pay` (pustakawan, kuitansi sederhana) → `paid`. Hapus/keringanan denda: `POST /library/fines/{id}/waive {reason}` → `waived` (+ `void` di billing bila `billed`), maker-checker tidak diwajibkan PRD; audit wajib.

### 4.4 Job harian `library_daily`
(1) Pengingat `ReminderKind` → notifikasi anggota (key `lib:loan:<id>:<kind>`); (2) reservasi `ready` lewat `ready_until` → `expired`, eksemplar ke reservasi berikutnya atau `available`; (3) sinkron status denda `billed`; (4) `CloseMembership` untuk anggota `pending_close`.

## 5. Endpoint (ops, JWT, scope `school_id`, feature `library` → 403 bila mati)
| Method & path | Peran |
|---|---|
| `GET/PUT /library/settings` · `CRUD /library/member-categories` · `CRUD /library/fine-rates` · `CRUD /library/collection-categories` | pustakawan, admin |
| `CRUD /library/titles` (+copies) · `GET /library/import/template` · `POST /library/import/dry-run` · `POST /library/import` (FR-112-002) | pustakawan |
| `GET /library/copies/{barcode}/label` (data barcode untuk cetak) | pustakawan |
| `GET /library/members?status=` · `GET /library/members/{id}` (pinjaman, denda, riwayat) · `PUT /library/members/{id}` (kategori) · `POST /library/members/sync` · `GET /library/members/{id}/card` | pustakawan |
| `POST /library/loans` · `POST /library/returns` · `POST /library/loans/{id}/renew` · `POST /library/loans/{id}/lost` · `POST /library/visits {member_code}` | pustakawan |
| `GET /library/fines?status=` · `POST /library/fines/{id}/pay` · `POST /library/fines/{id}/waive` | pustakawan |
| `GET /library/reports/loans?from=&to=&group_by=classroom\|collection_category` · `GET /library/reports/popular?from=&to=` · `GET /library/reports/overdue` · `GET /library/reports/fines?from=&to=` (terbit vs dibayar) · `GET /library/reports/visits` · `?format=xlsx` untuk ekspor | pustakawan, kepala_sekolah |
| OPAC: `GET /library/opac?q=&category_id=` (judul + jumlah tersedia, tanpa data peminjam) · `GET /library/opac/{titleId}` | siswa, guru, staf ber-anggota |
| `GET /me/library` (keanggotaan, pinjaman aktif + tenggat, denda) · `POST /me/library/loans/{id}/renew` · `POST /me/library/reservations {title_id}` · `DELETE /me/library/reservations/{id}` | anggota (hanya miliknya) |
| `POST /internal/library/fines/{id}/bill` | internal (retry) |

## 6. Frontend
`meta.feature: 'library'`, navGroup **Perpustakaan**. Route `web/src/router/domains/library.routes.ts` (daftarkan di `router/index.ts`, tambah baris). Service `web/src/services/library/` memanggil `api('ops')`.
| Path | View (`views/library/…`) | Peran | Label |
|---|---|---|---|
| `perpustakaan/sirkulasi` | `CirculationView.vue` (tab Pinjam / Kembali / Kunjungan; `ScanInput` untuk kartu anggota lalu eksemplar; panel anggota: pinjaman aktif, terlambat, denda; galat BR-LIB-01 ditampilkan sebagai alert berteks) | pustakawan | Sirkulasi |
| `perpustakaan/katalog` · `/:id` | `CatalogView.vue`, `TitleEditorView.vue` (data judul + tabel eksemplar barcode/rak/status, cetak label) + dialog impor Excel (template → dry-run galat per baris → impor) | pustakawan | Katalog |
| `perpustakaan/anggota` · `/:id` | `MembersView.vue` (filter status, badge "Belum dapat ditutup" + alasan), `MemberDetailView.vue` (cetak kartu, riwayat) | pustakawan | Anggota |
| `perpustakaan/denda` | `FinesView.vue` (terbuka/ditagihkan/lunas, bayar di tempat, keringanan dengan dialog konfirmasi) | pustakawan | Denda |
| `perpustakaan/laporan` | `ReportsView.vue` (tab Peminjaman / Terpopuler / Keterlambatan / Denda / Kunjungan, filter periode, ekspor, "Diperbarui: …") | pustakawan, kepala_sekolah | Laporan Perpustakaan |
| `perpustakaan/pengaturan` | `SettingsView.vue` (kebijakan denda OQ-33, hari pengingat, kategori anggota, tarif denda) | pustakawan, admin | Pengaturan Perpustakaan |
| `perpustakaan/opac` · `/:id` | `OpacView.vue` (cari + "x dari y tersedia" berteks, tombol reservasi bila 0 tersedia), `MyLibraryView.vue` tab "Pinjaman Saya" (tenggat, perpanjang, denda) | siswa, guru, walikelas, pegawai | Perpustakaan |
Tiga state, aksesibilitas, responsif sesuai GEMINI §5; input pemindai punya `<label for>` dan bisa diketik manual.

## 7. Bukti handoff
- Vektor §3 (test tabel) termasuk batas: tepat jatuh tempo (0 hari), perpanjangan maksimum, tarif berganti di tengah periode.
- **AC-112-01** (OPAC 0 tersedia), **AC-113-01** (job sinkron → `pending_close`, kembalikan buku → `inactive`), **AC-114-01** (pinjam ke-4 → 422), **AC-114-02** (denda Rp4.000 tercatat), **AC-115-01** (5 anggota terlambat tampil dengan judul + lama terlambat) sebagai test handler.
- BR-LIB-02: `fine_policy='bill'` → fake billing menerima `POST /internal/bills/adhoc` dengan Idempotency-Key `library:<fine_id>`; dijalankan 2× → satu tagihan; anggota pegawai → tidak ditagihkan. Test scan ganda (Idempotency-Key sama) → satu pinjaman. Test otorisasi: siswa tidak bisa `POST /library/loans`, OPAC tidak membocorkan peminjam, sekolah lain → 404, feature mati → 403.
- FR-112-001..003, FR-113-001..003, FR-114-001..004, FR-115-001..003: tabel cakupan di handoff.
- Playwright: pustakawan impor 3 eksemplar "Fisika Dasar" → atur kategori siswa (maks 3) + tarif Rp1.000/hari → pinjam lewat input pemindai (ketik barcode + Enter) → siswa melihat 0 tersedia di OPAC & membuat reservasi → pengembalian terlambat (tanggal diatur via data uji) → denda Rp4.000 → dengan kebijakan `bill` denda muncul sebagai tagihan di portal orang tua.
