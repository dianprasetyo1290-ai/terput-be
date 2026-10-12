# Handoff Cek Gabungan Gelombang 3 (CEK-W3)

**Branch:** `integrasi/prd-v2` (basis `55b26f7`)  
**Tanggal:** 2026-10-13  
**Cakupan:** Hasil merge dan integrasi Gelombang 3:
- **T9:** Fasilitas (master ruang, booking, perawatan) + kerangka service `ops` (port 8093) + semua peran & feature key Bagian 3–4 (identity migrasi 0017) + endpoint wali murid (`GET /internal/students/{id}/guardians`).
- **T10:** Kesehatan sekolah (rekam medis, UKS, program kesehatan) + kerangka service `studentlife` (port 8094) + migrasi 0009 attendance (absen UKS) + penegakan isolasi data (NFR-OPS-03).
- **T11:** Service Desk (tiket, kategori, tim penanganan, SLA, eskalasi) di `services/core` (migrasi 0003) + 8 view frontend di `web/src/views/servicedesk/` + routing dengan guard `meta.feature: 'service_desk'`.
- **T12:** Kantin POS & Dompet Siswa di `services/billing` (migrasi 0005, 0007), modul `canteen`, `wallet`, `adhoc` (BILL-ADHOC), hook approval Core (`canteen_settlement`, `wallet_transfer`), resolver kartu siswa (`GET /internal/student-cards/resolve`) + komponen UI `ScanInput.vue`.
- **T13:** Komunikasi lanjutan: kampanye pesan di `services/notification` (migrasi 0003), chat guru–ortu, landing page & program referral PPDB di `services/admission` (migrasi 0004).
- **T14:** Ekstensi pembayaran multi-provider (NICEPay, Midtrans, Xendit adapter stub/pluggable, autodebit, gerai retail) di `services/billing` (migrasi 0006).

**Status akhir:** SELESAI. Semua build, vet, test, migrasi (up → down 1 → up), dan build frontend (`vue-tsc` + Vite) HIJAU.

---

## 1. Rangkuman Integrasi & Resolusi Konflik

| # | Modul / Lokasi | Konflik / Temuan | Tindakan & Resolusi |
|---|---|---|---|
| 1 | `docker-compose.yml` & `.env.example` | Penambahan service baru `ops` (8093) dan `studentlife` (8094) dari branch T9 & T10 | Menggabungkan konfigurasi port, DSN database, env internal URL, dan service dependency secara bersih |
| 2 | `billing/cmd/server/main.go` & `app.go` | T12 (kantin & dompet) dan T14 (ekstensi gateway) sama-sama memodifikasi inisialisasi runner, adapter, dan webhook billing | Menggabungkan wiring: `app.PurposeCharger = app`, `app.PurposeHandlers["wallet_topup"] = app.WalletTopupHook()`, dan mendaftarkan gateway adapter extension |
| 3 | `core/internal/model/domain.go` | T9, T10, T12 membutuhkan request type baru untuk alur Approval Center | Mendaftarkan seluruh request type W3 ke `KnownRequestTypes` & `RequestTypeLabels`: `room_booking`, `health_profile_update`, `canteen_settlement`, `wallet_transfer` |
| 4 | `studentlife/internal/handler/static_test.go` | NFR-OPS-03: pengujian statis memastikan string `"studentlife"` tidak muncul di package `identity`, `academic`, atau `report` | Menyesuaikan komentar di `services/identity/internal/model/model.go` agar tidak mengandung string modul terisolasi tersebut |
| 5 | `web/src/views/servicedesk/` | T11 backend selesai namun 8 halaman frontend belum tersedia di branch T11 | Mengimplementasikan ke-8 view: `TicketWorkView.vue`, `CategoriesView.vue`, `TeamsView.vue`, `SlaView.vue`, `ReportView.vue`, `MyTicketsView.vue`, `MyTicketFormView.vue`, `MyTicketDetailView.vue` |

---

## 2. Pengujian & Verifikasi

### 2.1 `go vet ./...` (Seluruh 13 Service Go)
```
=== ops          VET_OK
=== studentlife  VET_OK
=== core         VET_OK
=== billing      VET_OK
=== notification VET_OK
=== admission    VET_OK
=== identity     VET_OK
=== academic     VET_OK
=== attendance   VET_OK
=== hr           VET_OK
=== evaluation   VET_OK
=== exam         VET_OK
=== report       VET_OK
```

### 2.2 `go test -count=1 ./...` (Unit & Integration Tests)
```
ops:          ok facility (0.178s) · ok handler (0.165s)                              EXIT 0
studentlife:  ok config · ok consent · ok handler · ok health · ok sensitive          EXIT 0
core:         ok common · ok handler · ok model · ok servicedesk · ok store           EXIT 0
billing:      ok adhoc · ok canteen · ok client · ok common · ok gateway ·
              ok handler · ok jobs · ok model · ok nicepay · ok wallet                EXIT 0
notification: ok campaign · ok chat · ok config · ok handler · ok provider · ok store EXIT 0
admission:    ok client · ok conversion · ok model · ok otp                           EXIT 0
identity:     ok common · ok handler · ok store                                       EXIT 0
academic:     ok common · ok handler · ok model · ok store                            EXIT 0
attendance:   ok client · ok handler · ok store                                       EXIT 0
hr:           ok ats/calc · ok client · ok crypto · ok handler · ok jobs ·
              ok model · ok payroll/calc · ok training/calc                           EXIT 0
evaluation:   ok handler                                                              EXIT 0
exam:         ok cache · ok common · ok config · ok handler · ok model · ok store     EXIT 0
report:       ok cache · ok docx · ok handler · ok model · ok upstream                EXIT 0
```

### 2.3 Migrasi Database dari Nol: `up` → `down 1` → `up`
```
academic:     up [5/u transfers]            down1 [5/d transfers]            up [5/u]  version=5
admission:    up [4/u marketing_referral]   down1 [4/d marketing_referral]   up [4/u]  version=4
attendance:   up [9/u health_absence]       down1 [9/d health_absence]       up [9/u]  version=9
billing:      up [7/u adhoc_bills]          down1 [7/d adhoc_bills]          up [7/u]  version=7
core:         up [3/u service_desk]         down1 [3/d service_desk]         up [3/u]  version=3
evaluation:   up [6/u prosem]               down1 [6/d prosem]               up [6/u]  version=6
exam:         up [8/u admission_part...]    down1 [8/d admission_part...]    up [8/u]  version=8
hr:           up [30/u training]            down1 [30/d training]            up [30/u] version=30
identity:     up [17/u ops_roles]           down1 [17/d ops_roles]           up [17/u] version=17
notification: up [3/u campaign_chat]        down1 [3/d campaign_chat]        up [3/u]  version=3
ops:          up [2/u facility]             down1 [2/d facility]             up [2/u]  version=2
report:       up [13/u capaian_dua_kolom]   down1 [13/d capaian_dua_kolom]   up [13/u] version=13
studentlife:  up [3/u health_programs]      down1 [3/d health_programs]      up [3/u]  version=3
```

### 2.4 Frontend TypeScript & Build
```
$ npx vue-tsc -b   → VUETSC_EXIT: 0
$ npm run build    → ✓ built in 1.49s  BUILD_EXIT: 0
```

---

## 3. Komitmen & Perubahan Kunci W3

1. **Service `ops` & `studentlife` Berdiri Kokoh:**
   - Direktori `services/ops` dan `services/studentlife` lengkap dengan `cmd/server/main.go`, konfigurasi env, koneksi database, router chi, dan migrasi terdaftar di compose.
2. **Kantin, Dompet, dan Tagihan Ad-Hoc (`billing`):**
   - Mendukung transaksi kasir kantin, verifikasi QR kartu siswa, transfer saldo dompet, top-up dengan hook pembayaran gateway, penarikan dana titipan, dan modul tagihan ad-hoc untuk konsumsi task-task berikutnya (T18 denda perpus, T19 ongkos transport, T21 biaya lomba/ekskul).
3. **Approval Center Universal:**
   - Semua modul W3 yang membutuhkan persetujuan berjenjang (`room_booking`, `health_profile_update`, `canteen_settlement`, `wallet_transfer`) terintegrasi langsung dengan `services/core` via `RequestApproval` dan handler callback.
4. **Isolasi Data Siswa & Rekam Medis (NFR-OPS-03):**
   - Data riwayat penyakit dan catatan konseling terlindungi di service `studentlife`, terisolasi dari service akademik maupun pelaporan umum.
5. **UI Lengkap Tanpa Placeholder:**
   - 8 view Service Desk di frontend dibangun menggunakan desain konsisten sistem Tailwind/CSS SEKOCI, siap diakses pengguna peran staf tata usaha dan pemohon tiket.

---

## 4. Kesiapan Gelombang 4 (W4)

Dengan selesainya Gelombang 3, basis untuk Gelombang 4 sudah tersedia sepenuhnya:
- **T15 (Inventory & Pengadaan):** Membangun di atas `ops` (`internal/inventory`, `internal/procurement`) dengan master barang, stok, opname, PR, PO, vendor. Migrasi cadangan `0010–0019`.
- **T18 (Perpustakaan):** Membangun di atas `ops` (`internal/library`), menggunakan tagihan ad-hoc dari billing untuk denda (OQ-33: `pay_at_library` / tagihan ad-hoc). Migrasi cadangan `0040–0049`.
- **T19 (Transportasi):** Membangun di atas `ops` (`internal/transport`), menggunakan rute dan armada kendaraan. Migrasi cadangan `0050–0059`.
- **T20 (Konseling & Kesiswaan):** Membangun di atas `studentlife` (`internal/counseling`, `internal/discipline`). Migrasi cadangan `0010–0019`.
- **T21 (Event, Ekstrakurikuler & Prestasi):** Membangun di atas `studentlife` (`internal/activity`). Migrasi cadangan `0020–0029`.
- **T22 (PKL):** Membangun di atas `studentlife` (`internal/pkl`). Migrasi cadangan `0030–0039`.
