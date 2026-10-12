# T19: Transportasi di service `ops`: rute & halte, kendaraan & kru, alokasi & absensi perjalanan, tagihan transport

**Menu PRD:** 119, 120, 121, 122 (BR-TRS-01, IF-OPS-04, alur "Antar-jemput" Bagian 3 §6) · **Branch:** `feat/T19-transport` · **Service:** `ops` (paket `internal/transport`), sedikit `hr` dan `billing` (lihat §4) · **Migrasi:** `services/ops/migrations/0050_transport.*` (nomor 0050–0059 milik T19)
**Prasyarat:** gelombang 3 di-merge: **T9** (kerangka `ops`, peran `transport_officer`/`pendamping_transport`/`pengemudi`, endpoint identity guardians), T12 (kartu QR siswa, opsional, lihat §4). **Feature key:** `transport`.

## 1. Donor / acuan kode
Tidak ada donor. Pola:
- Paket domain: `services/ops/internal/facility` (T9) → `internal/transport/{handler,model,store}.go` + `calc_test.go`, mount di `ops/internal/handler/router.go` dengan `requireFeature("transport")`.
- Job pengingat dokumen: `services/hr/internal/jobs/jobs.go` `DocumentReminders` (pola idempotency per dokumen + tanggal).
- Kepemilikan orang tua → anak: identity `GET /internal/guardians/{guardianUserId}/students/{studentUserId}/access` (pola `services/attendance/internal/client/client.go`).
- Penerbitan tagihan: `services/billing/internal/handler/bills.go` `generateBills` (transaksi, idempoten, audit) sebagai acuan gaya untuk endpoint charge di §4.
- Nomor dokumen & audit: `doc_sequences`, `audit_events` milik T9 `0001_ops_base`.

## 2. Migrasi `0050_transport.up.sql`
Semua tabel: `id uuid PK`, `school_id uuid NOT NULL`, `created_at/updated_at`. Siswa = `student_user_id` (identity), pegawai = `employee_id` (hr), tanpa FK lintas DB.
```
transport_settings(school_id PK, prorata_policy CHECK IN('daily','full_month') NOT NULL DEFAULT 'daily',
  doc_reminder_days int NOT NULL DEFAULT 30 CHECK(doc_reminder_days>0),          -- [30] dari FR-120-003, bisa diubah per sekolah
  notify_parent_on_trip bool NOT NULL DEFAULT true, component_code text NOT NULL DEFAULT 'TRANSPORT', bill_due_day int NULL CHECK(bill_due_day BETWEEN 1 AND 31))
transport_zones(id, school_id, name, deleted_at, UNIQUE(school_id, name) WHERE deleted_at IS NULL)
transport_routes(id, school_id, code, name, status CHECK IN('active','inactive'), deleted_at, UNIQUE(school_id, code) WHERE deleted_at IS NULL)
transport_stops(id, school_id, route_id FK, seq int CHECK(seq>0), name, address NULL, zone_id FK NULL,
  pickup_time time NULL, dropoff_time time NULL, deleted_at, UNIQUE(route_id, seq) WHERE deleted_at IS NULL)          -- FR-119-001
transport_vehicles(id, school_id, plate_no, name, capacity int NOT NULL CHECK(capacity>0), status CHECK IN('active','maintenance','inactive'),
  deleted_at, UNIQUE(school_id, plate_no) WHERE deleted_at IS NULL)
transport_vehicle_documents(id, school_id, vehicle_id FK, kind CHECK IN('stnk','uji_kir','asuransi','lainnya'), number, valid_until date NOT NULL,
  object_key NULL, deleted_at)                                                                                       -- FR-120-001
transport_crew(id, school_id, kind CHECK IN('driver','attendant'), employee_id uuid NULL, user_id uuid NULL,
  external_name text NULL, phone text NULL, license_no NULL, license_valid_until date NULL, deleted_at,
  CHECK(employee_id IS NOT NULL OR external_name IS NOT NULL))                                                        -- FR-120-002 (pegawai menu 83 atau pihak luar)
transport_route_assignments(id, school_id, route_id FK, vehicle_id FK, driver_crew_id FK, attendant_crew_id FK NULL,
  valid_from date NOT NULL, valid_to date NULL, CHECK(valid_to IS NULL OR valid_to>=valid_from))                       -- riwayat, bukan UPDATE menimpa
transport_fares(id, school_id, zone_id NULL, route_id NULL, monthly_amount_idr bigint NOT NULL CHECK(monthly_amount_idr>=0),
  effective_from date NOT NULL, effective_to date NULL, CHECK((zone_id IS NULL) <> (route_id IS NULL)))               -- FR-122-001
transport_allocations(id, school_id, student_user_id, route_id FK, stop_id FK, start_date date NOT NULL, end_date date NULL,
  end_reason NULL, created_by, CHECK(end_date IS NULL OR end_date>=start_date))
UNIQUE INDEX transport_allocations_one_open ON transport_allocations(school_id, student_user_id) WHERE end_date IS NULL
transport_trips(id, school_id, route_id FK, trip_date date, direction CHECK IN('jemput','antar'), vehicle_id, driver_crew_id, attendant_crew_id NULL,
  status CHECK IN('planned','running','done'), UNIQUE(route_id, trip_date, direction))
transport_trip_events(id, school_id, trip_id FK, student_user_id, stop_id NULL, event CHECK IN('board','alight','absent'),
  source CHECK IN('scan','manual'), recorded_by uuid, recorded_at timestamptz, client_event_id text NOT NULL,
  UNIQUE(trip_id, student_user_id, event), UNIQUE(school_id, client_event_id))                                       -- FR-121-002, aman diulang
transport_absence_notices(id, school_id, student_user_id, notice_date date, direction CHECK IN('jemput','antar','both'), reason NULL,
  created_by uuid, cancelled_at NULL, UNIQUE(student_user_id, notice_date, direction) WHERE cancelled_at IS NULL)     -- FR-121-004
transport_charges(id, school_id, student_user_id, period_year int, period_month int CHECK(period_month BETWEEN 1 AND 12),
  items jsonb NOT NULL, amount_idr bigint NOT NULL CHECK(amount_idr>=0), version int NOT NULL DEFAULT 1,
  bill_id uuid NULL, status CHECK IN('issued','adjusted','void','failed'), last_error NULL,
  UNIQUE(school_id, student_user_id, period_year, period_month))                                                      -- satu tagihan transport per siswa per bulan
```
Kapasitas **bukan** kolom terhitung: diturunkan dari kendaraan pada `transport_route_assignments` yang berlaku di tanggal itu.

## 3. Fungsi murni (`internal/transport/model.go`) + vektor
| Fungsi | Vektor |
|---|---|
| `StudentsByStop(stops, allocations, date) []StopRoster` (urut `seq`, alokasi aktif di tanggal itu) | **AC-119-01** halte "Perumahan A" dengan 6 alokasi aktif → tampil 6 siswa; alokasi berakhir kemarin tidak ikut (FR-119-002) |
| `CanAllocate(capacity, activeOnRoute int, newCount int) error` | **AC-121-01** kapasitas 15, terisi 15, tambah 1 → `ErrCapacityFull` "Kapasitas kendaraan penuh (15/15)"; 14 → ok (BR-TRS-01) |
| `CheckVehicleChange(newCapacity, peakAllocated)` | ganti ke kendaraan kapasitas 12 saat terisi 15 → 422 (BR-TRS-01 tidak bisa dilewati dengan ganti kendaraan) |
| `DocReminderDue(validUntil, today, days) (due bool, daysLeft int)` | **AC-120-01** uji berlaku s/d 30 Nov, hari ini 31 Okt, days 30 → due, sisa 30 hari; 29 Okt → belum; lewat masa berlaku → due + label "kedaluwarsa" (berlaku juga untuk SIM `license_valid_until`, FR-120-003) |
| `Manifest(roster, events, notices, direction)` | siswa dengan pemberitahuan "tidak ikut" → status `tidak_ikut` (FR-121-004); sudah `board` → tombol "Turun" saja |
| `ValidateTripEvent(prev []Event, ev)` | `alight` tanpa `board` → 422; `board` dua kali dengan `client_event_id` sama → 200 tanpa duplikat |
| `FareFor(fares, routeID, zoneID, date)` (fare rute menang atas zona; pakai `effective_from<=date<effective_to`) | tarif zona 600.000 + tarif rute 650.000 → 650.000; tanpa tarif → error "Tarif belum diatur" (siswa dilewati, tampil di pratinjau) |
| `ProrateIDR(monthly, daysUsed, daysInPeriod) int64` (satu-satunya titik pembulatan: setengah ke atas ke rupiah penuh, dicatat di handoff) | **AC-122-01** 600.000, 15/30 → 300.000; 600.000, 30/30 → 600.000; 0 hari → 0 |
| `ComputeCharge(segments, fares, policy, period) (items, total)` (segmen = alokasi yang beririsan dengan bulan, hari dihitung inklusif) | `daily`: berhenti setelah 15 dari 30 hari → 300.000 (AC-122-01); pindah rute tgl 16 (rute A 600.000 15 hari + rute B 800.000 15 hari) → dua item 300.000 + 400.000 (FR-122-003); `full_month`: satu tarif penuh dari segmen dengan hari terbanyak (seri → segmen terakhir) |
| `ChargeDiff(issued, recomputed)` | sama → tidak ada aksi; beda → adjust dengan `version+1` |

## 4. Integrasi & job
- **Billing (menu 19, FR-122-002/003):** endpoint yang ada (`POST /internal/bills/generate`) berbasis skema SPP dengan nominal tetap dan tidak mendukung prorata, jadi transport memakai kontrak **charge non-SPP** berikut (X-Internal-Key, juga dibutuhkan T18 denda dan T21 event/ekskul):
  - `POST /internal/bills/adhoc` `{school_id, student_user_id, source:'transport', source_ref:'transport:<student>:<yyyy-mm>', period_year, period_month, due_date?, items:[{code, name, amount_idr}], idempotency_key}` → `201 {bill_id, status, net_idr}`; key sama → `200` data sama. Tidak menerapkan aturan diskon SPP. Siswa yang sudah keluar (`student_billing_exits`) → 409.
  - `POST /internal/bills/adhoc/adjust` `{school_id, source, source_ref, items, reason, idempotency_key}` → billing menghitung ulang `net_idr`: tagihan belum dibayar → diganti; jumlah baru < yang sudah dibayar → selisih masuk `student_deposits` (BR-FIN-04, tabel T6); jumlah 0 dan belum ada pembayaran → `void`. Diaudit di billing.
  - **Pemilik:** T12 (kontrak BILL-ADHOC, migrasi `billing/0007_adhoc_bills`, gelombang 3, sudah di-merge saat T19 mulai). Field `source` di atas = `source_module` di T12. T19 tidak membangun endpoint billing.
  - Penerbitan: `POST /transport/billing/issue {period_year, period_month}` (manual) → per siswa `ComputeCharge` → simpan `transport_charges` → panggil charge dengan `idempotency_key = 'transport:<student>:<yyyy-mm>:v<version>'`. Gagal → status `failed` + `POST core /internal/exceptions` (kind `callback`, retry_url `ops /internal/transport/charges/{id}/retry`). Item memakai `component_code` dari `transport_settings` supaya mapping COA billing (BR-FIN-06) berlaku; posting akuntansi tetap milik billing, ops **tidak** menjurnal.
  - Perubahan alokasi (mulai/pindah/berhenti) pada bulan yang sudah ditagih → hitung ulang otomatis dalam transaksi yang sama dengan perubahan → `ChargeDiff` → `/internal/bills/adhoc/adjust` (versi baru). Diaudit.
- **HR (FR-120-002):** tambahkan di hr (kode saja, tanpa migrasi, hanya tambah baris di router): `GET /internal/employees?school_id=&q=&status=active` → `[{id, user_id, full_name, position_name}]` tanpa data sensitif. Dipakai dialog "Pilih pegawai" untuk kru. HR mati/kosong → kru pihak luar tetap bisa diisi.
- **Identity:** daftar siswa `GET /students?school_id=` (sudah ada); wali siswa `GET /internal/students/{id}/guardians` (dibuat T9 §4.1); cek akses ortu `/internal/guardians/{g}/students/{s}/access` (sudah ada).
- **Notifikasi (FR-121-003, AC-121-02):** setiap `board`/`alight` tersimpan → kirim ke semua wali aktif, `type:'transport'`, judul "Nara turun di Halte Rumah 15.42", `Idempotency-Key: transport:<trip_id>:<student>:<event>`. Dikirim setelah commit, async; gagal kirim tidak membatalkan absensi (dicatat ke Sentry, notification punya retry sendiri). Saklar `notify_parent_on_trip`.
- **Pemindai (IF-OPS-04):** pendamping memindai kartu QR siswa (keyboard wedge atau kamera). Resolusi kode → siswa memakai kontrak T12 (pemilik kartu QR siswa, OQ-30): `POST {BILLING_URL}/internal/student-cards/resolve {school_id, code}` → `{student_user_id, status:'active'|'revoked'}`. Kartu dicabut → 422 "Kartu tidak aktif". Siswa tidak ada di manifest perjalanan → 422 "Siswa tidak terdaftar di rute ini". Kontrak belum tersedia → mode manual (pilih dari manifest) tetap jalan dan dicatat di handoff. Komponen input: `components/ui/ScanInput.vue` milik T12 (props `modelValue`, `placeholder`, `camera?: boolean`, emit `scan(code)`); kalau belum ada setelah rebase, buat dengan kontrak ini dan catat di handoff.
- **Job harian (zona sekolah):** (1) pengingat dokumen kendaraan & SIM (FR-120-003, AC-120-01) ke semua user `transport_officer` sekolah, `Idempotency-Key: transport-doc:<doc_id|crew_id>:<valid_until>`; (2) buat `transport_trips` hari ini (jemput + antar) untuk rute aktif dengan assignment berlaku (`ON CONFLICT DO NOTHING`); (3) forward audit (job T9).

## 5. Endpoint (JWT + `requireFeature("transport")`; `admin`, `superadmin` selalu boleh)
| Method & path | Peran |
|---|---|
| `CRUD /transport/zones`, `/transport/routes` (+ `PUT /routes/{id}/stops` urutan halte sekaligus), soft delete | transport_officer |
| `GET /transport/routes/{id}/roster?date=&direction=` (FR-119-002, AC-119-01) | transport_officer, pendamping_transport/pengemudi (rute yang ditugaskan) |
| `CRUD /transport/vehicles` (+ `documents`, unggah file), `CRUD /transport/crew`, `POST /transport/routes/{id}/assignments` | transport_officer |
| `GET /transport/expiring?within_days=` (dokumen & SIM) | transport_officer |
| `POST /transport/allocations` `{student_user_ids[], route_id, stop_id, start_date}` (semua atau tidak sama sekali; 422 kapasitas) · `POST /transport/allocations/{id}/end` `{end_date, reason}` · `POST /transport/allocations/{id}/move` `{route_id, stop_id, effective_date}` · `GET /transport/allocations?route_id=&student=` | transport_officer |
| `GET /transport/my-trips?date=` · `GET /transport/trips/{id}/manifest` · `POST /transport/trips/{id}/events` `{student_user_id?, code?, stop_id, event, client_event_id}` · `POST /transport/trips/{id}/start` · `/finish` | pendamping_transport, pengemudi (hanya trip yang kru-nya `user_id` = dirinya) |
| `GET /transport/trips?date=&route_id=` (rekap absensi perjalanan) | transport_officer |
| `CRUD /transport/fares` · `GET/PUT /transport/settings` | transport_officer (fares), admin, finance_manager |
| `GET /transport/billing/preview?period=` · `POST /transport/billing/issue` · `GET /transport/billing?period=&status=` | admin, finance_manager (issue); transport_officer (baca) |
| Ortu: `GET /me/transport/children` (alokasi, halte, jam, status perjalanan hari ini) · `GET /me/transport/children/{studentId}/events?from=&to=` · `POST /me/transport/absences` `{student_user_id, notice_date, direction, reason}` · `DELETE /me/transport/absences/{id}` (sebelum tanggal) | orang_tua (cek akses wali per siswa di server) |
| `POST /internal/transport/charges/{id}/retry` (X-Internal-Key) | core exception retry |
Semua mutasi diaudit; list `?page=&per_page=` + `meta`.

## 6. Frontend
`web/src/services/ops/{transport.ts,types.ts}`, `web/src/router/domains/transport.routes.ts`. Semua route `meta.feature: 'transport'`. navGroup **Transportasi** (staf) dan **Portal** (ortu).
| Path | View (`views/transport/…`) | Peran | navLabel |
|---|---|---|---|
| `transportasi/rute` · `/:id` | `RoutesView.vue`, `RouteDetailView.vue` (halte berurutan dengan tombol naik/turun ber-`aria-label`, jam jemput/antar, zona; tab "Siswa per Halte" dengan jumlah per halte; pilihan arah jemput/antar) | transport_officer | Rute & Halte |
| `transportasi/kendaraan` | `VehiclesView.vue` (kapasitas, dokumen, badge berteks "Berlaku 30 hari lagi" / "Kedaluwarsa") | transport_officer | Kendaraan |
| `transportasi/kru` | `CrewView.vue` (pengemudi/pendamping; pilih pegawai HR atau isi pihak luar; SIM) + penugasan ke rute | transport_officer | Pengemudi & Pendamping |
| `transportasi/alokasi` | `AllocationsView.vue` (pilih siswa multi, rute, halte, tanggal mulai; bar kapasitas "12/15" dengan angka; pindah/berhenti) | transport_officer | Alokasi Siswa |
| `transportasi/perjalanan` | `TripsView.vue` (rekap per tanggal: naik, turun, tidak ikut, belum tercatat) | transport_officer | Absensi Perjalanan |
| `transportasi/tarif` | `FaresView.vue` (per zona/rute + berlaku mulai, kebijakan prorata, kode komponen) | transport_officer, admin, finance_manager | Tarif Transport |
| `transportasi/tagihan` | `TransportBillingView.vue` (pratinjau per periode: siswa, segmen, hari, tarif, nominal; siswa tanpa tarif ditandai; tombol Terbitkan dengan dialog konfirmasi; status per siswa + tautan tagihan) | admin, finance_manager, transport_officer (baca) | Tagihan Transport |
| `transportasi/tugas-saya` | `AttendantTripView.vue` (mobile-first 375px: pilih perjalanan hari ini, manifest per halte, `ScanInput` + tombol Naik/Turun per siswa ≥44px, badge "Tidak ikut", antrean kirim ulang bila jaringan putus memakai `client_event_id`) | pendamping_transport, pengemudi | Perjalanan Hari Ini |
| `orang-tua/antar-jemput` | `views/parent/ParentTransportView.vue` (per anak: rute, halte, jam, status hari ini naik/turun + jam, riwayat; form "Tidak ikut" tanggal + arah) | orang_tua | Antar-Jemput |
`DashboardRedirectView`: `transport_officer` → `/transportasi/rute`, `pendamping_transport`/`pengemudi` → `/transportasi/tugas-saya`.

## 7. Bukti handoff
Vektor §3 + test handler: **AC-119-01**, **AC-120-01** (job dua kali → satu notifikasi), **AC-121-01** (alokasi massal 2 siswa saat sisa 1 → keduanya ditolak), **AC-121-02** (notifikasi ke wali dengan Idempotency-Key; event ulang tidak mengirim dua kali), **AC-122-01** (charge 300.000 lewat fake billing `httptest`), FR-122-003 pindah rute setelah tagihan terbit → adjust versi 2, FR-121-004 pemberitahuan ortu muncul di manifest, ortu tidak bisa melihat/menulis untuk siswa bukan anaknya (403), pendamping tidak bisa mencatat di trip rute lain (403), kartu dicabut → 422. Playwright: transport_officer membuat zona, rute 3 halte, bus kapasitas 2, tarif 600.000 → alokasi 2 siswa, siswa ke-3 ditolak "Kapasitas kendaraan penuh" → ortu menandai anak tidak ikut besok → pendamping membuka manifest (badge Tidak ikut), mencatat naik/turun siswa lain → ortu siswa itu melihat notifikasi → finance_manager pratinjau & terbitkan tagihan bulan ini → tagihan tampil di `/orang-tua/tagihan`.
