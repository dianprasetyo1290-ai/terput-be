# T9: Fasilitas (master ruang, booking, perawatan) + kerangka service `ops` + semua peran & feature key Bagian 3–4

**Menu PRD:** 116, 117, 118 (BR-FAC-01, BR-FAC-02) + tabel "Peran Tambahan" Bagian 3 §2 & Bagian 4 §2 · **Branch:** `feat/T9-facility-ops` · **Service:** `ops` baru, port **8093** (paket `internal/facility`), `identity`, sedikit `core` · **Migrasi:** `services/ops/migrations/0001_ops_base.*` + `0002_facility.*` (nomor 0001–0009 milik T9) · `services/identity/migrations/0017_ops_roles.*` (satu-satunya migrasi peran Bagian 3–4)
**Prasyarat:** T1–T8 sudah di-merge di `integrasi/prd-v2`. **Feature key:** `facility`.
**Dipakai oleh:** T15–T19, T25 (kerangka `ops`), semua T10–T26 (nama peran & feature key §2), T11/T16/T21/T25 (kontrak §4.2). Kontrak di §2 dan §4.2 **tidak boleh diubah** tanpa persetujuan reviewer.

## 1. Donor / acuan kode
Tidak ada donor eksternal. Kerangka & pola:
- Layout service: salin `services/billing/` → `services/ops/` (`cmd/server/main.go`, `internal/{common,config,handler,store,model,client,storage}`, `Dockerfile`, `docker-compose.yml`, `README.md`, `.env.example`). `internal/common/{auth,response,http,internal,sentry}.go` **disalin** (ganti nama package), jangan impor lintas modul. `internal/storage/local.go` disalin dari billing (foto kerusakan, nanti dipakai T18/T25).
- Paket domain per task meniru `services/hr/internal/training/{handler,model,store}.go` + mount di `internal/handler/router.go` (pola `hr/internal/handler/router.go`). T9 membuat `internal/facility`; task berikutnya menambah paket sendiri (`inventory`, `procurement`, `asset`, `budget`, `payable`, `library`, `transport`, `campus3d`).
- Job berkala: salin `services/hr/internal/jobs/runner.go` (ticker + `pg_try_advisory_lock`). Audit: tabel `audit_events` lokal dengan `forwarded_at` + job forward ke core `POST /internal/audit` (salin pola `hr` migrasi 0001 baris `audit_events` dan `hr/internal/client` `PostAudit`).
- Approval: client `POST /internal/approvals` + endpoint callback `POST /internal/approval-callbacks` (pola `hr/internal/handler/router.go`, kontrak T1 §3).
- Saklar fitur di backend: middleware `requireFeature(key)` per grup route (pola `hr/internal/handler/helpers.go` `requireHrisFeature`, tetapi generik dengan parameter key). Baca identity `GET /schools/{id}/features` (X-Internal-Key), cache in-memory TTL `FEATURE_CACHE_SECONDS` (env). Fitur mati → 403 `"Modul <label> dinonaktifkan untuk sekolah ini"`.
- Peran: `services/identity/migrations/0014_hr_roles.{up,down}.sql` (CHECK + down yang gagal jelas), `services/identity/internal/common/helpers.go` (`AllRoles`), `services/core/internal/common/auth.go` (`AllRoles`). Feature key: `services/identity/internal/store/school_features.go` (`FeatureKeys`).

### Kerangka `ops` (wajib, sebelum fitur)
Config env (`internal/config/config.go`): `OPS_PORT` (fallback `PORT`, default 8093), `DATABASE_URL`, `SEKOCI_JWT_SECRET`, `SEKOCI_INTERNAL_KEY`, `CORS_ALLOWED_ORIGINS`, `CORE_URL`, `IDENTITY_URL`, `NOTIFICATION_URL`, `HR_URL`, `BILLING_URL`, `OPS_PUBLIC_INTERNAL_URL` (dasar `callback_url` approval), `STORAGE_ROOT` (default `./data/private/ops`), `MAX_UPLOAD_BYTES`, `JOB_INTERVAL_SECONDS`, `FEATURE_CACHE_SECONDS`, `SENTRY_DSN`.
Daftarkan (hanya tambah baris): `.env.example` (`OPS_PORT=8093`, `OPS_DB_NAME=teratai_ops`), `docker-compose.yml` (`ops` + `ops-migrate` + volume `ops_files`), `docker-compose.local.yml` (+ `POSTGRES_MULTIPLE_DATABASES` tambah `${OPS_DB_NAME:-teratai_ops}`), `web/src/api/client.ts` (`ops: withAppBase(import.meta.env.VITE_OPS_URL ?? 'http://localhost:8093')`), `web/.env` & `web/.env.example` (`VITE_OPS_URL`), `web/nginx.conf` (`location /api/ops/ { proxy_pass http://ops:8093/; … }` salin blok `/api/hr/`). `GET /healthz` publik.

## 2. Migrasi

### 2a. Identity `0017_ops_roles.up.sql`: semua peran baru Bagian 3–4
Perlebar `user_school_roles_role_check` = daftar 0014 + 25 peran di bawah. `.down.sql`: `RAISE EXCEPTION` bila masih ada baris dengan peran baru (pola 0014), lalu kembalikan CHECK 0014. Tambahkan konstanta yang sama di `identity/internal/common/helpers.go` (`AllRoles`), `core/internal/common/auth.go` (`AllRoles`), `web/src/types/envelope.ts` (union `Role`), label di `components/identity/RoleSwitcher.vue` + `layouts/AppLayout.vue`, daftar pilihan peran di form pengguna identity (cari `'training_admin'`), dan `ALL_STAFF_ROLES` di `router/domains/core.routes.ts` (kecuali `pembimbing_industri`).

**Tabel peran final (dipakai persis oleh T10–T26):**
| Peran PRD | `role` | Label UI | Task utama |
|---|---|---|---|
| Inventory Officer | `inventory_officer` | Petugas Inventaris | T15 |
| Procurement Officer | `procurement_officer` | Petugas Pengadaan | T15, T17 |
| Asset Officer | `asset_officer` | Petugas Aset | T16, T25 |
| Pustakawan | `pustakawan` | Pustakawan | T18 |
| Facility Officer | `facility_officer` | Petugas Fasilitas | T9, T25 |
| Teknisi | `teknisi` | Teknisi | T9 |
| Transport Officer | `transport_officer` | Petugas Transportasi | T19 |
| Pendamping | `pendamping_transport` | Pendamping Antar-Jemput | T19 |
| Pengemudi | `pengemudi` | Pengemudi | T19 |
| Canteen Admin | `canteen_admin` | Admin Kantin | T12, T25 |
| Operator tenant | `canteen_operator` | Operator Tenant Kantin | T12 |
| Petugas UKS | `petugas_uks` | Petugas UKS | T10 |
| Guru BK / Konselor | `konselor` | Guru BK / Konselor | T20 |
| Kesiswaan Officer | `kesiswaan` | Petugas Kesiswaan | T20, T21 |
| Event Coordinator | `event_coordinator` | Koordinator Event | T21, T9 |
| Pembina ekskul | `pembina_ekskul` | Pembina Ekskul | T21 |
| Service Desk Agent | `servicedesk_agent` | Agen Service Desk | T11 |
| Service Desk Manager | `servicedesk_manager` | Manajer Service Desk | T11 |
| Koordinator PKL | `koordinator_pkl` | Koordinator PKL | T22 |
| Pembimbing sekolah | `pembimbing_pkl` | Pembimbing PKL Sekolah | T22 |
| Pembimbing industri (eksternal) | `pembimbing_industri` | Pembimbing Industri | T22 |
| QA Coordinator / Tim Penjaminan Mutu | `qa_coordinator` | Koordinator Mutu | T23 |
| Operator Dapodik | `operator_dapodik` | Operator Dapodik | T23 |
| Humas / Marketing | `humas` | Humas / Marketing | T13 |
| BI Analyst | `bi_analyst` | Analis BI | T24 |
| Platform Admin (FullCircle) | — (pakai `superadmin` yang sudah lintas sekolah) | — | T14, T26 |

Catatan: Kepala Unit = `kepala_sekolah`; Finance Admin = `admin` / `finance_manager`; Pemohon = peran staf mana pun; Guru tetap `guru`. Guru yang ditugaskan per kegiatan (pembina ekskul, pembimbing PKL) boleh juga diberi hak lewat penugasan di task pemiliknya (cek kepemilikan data), peran di atas dipakai untuk akun yang memang khusus mengelola.

### 2b. Feature key Bagian 3–4 (identity, kode, bukan migrasi)
`school_features` tidak punya daftar di DB (tidak ada baris = mati, migrasi 0012). Allowlist ada di `identity/internal/store/school_features.go` `FeatureKeys`: **tambahkan semua key di bawah sekaligus** (urutan seperti tabel). Tambah juga `FeatureRequires = map[string][]string{"campus3d": {"facility"}}`: `PUT /schools/{id}/features/campus3d` enabled=true saat `facility` mati → 422 `"Aktifkan modul Fasilitas terlebih dahulu"` (fungsi murni `MissingRequirements(key, enabled map)`, dites). Label Indonesia di `web/src/services/identity/features.ts` (`featureLabels`, satu sumber).

**Tabel feature key final (dipakai persis di `meta.feature` dan `requireFeature`):**
| `feature_key` | Label | Menu | Task |
|---|---|---|---|
| `facility` | Fasilitas | 116–118 | T9 |
| `health` | Kesehatan Sekolah | 127–129 | T10 |
| `service_desk` | Service Desk | 137–138 | T11 |
| `canteen` | Kantin & POS | 123, 124, 126 | T12 |
| `wallet` | Dompet & Tabungan Siswa | 125 | T12 |
| `campaign` | Kampanye & Broadcast | 147 | T13 |
| `teacher_parent_chat` | Chat Guru–Orang Tua | 148 | T13 |
| `ppdb_marketing` | Landing Page & Referral PPDB | 149 | T13 |
| `payment_multi_provider` | Multi Payment Provider | 150 | T14 |
| `payment_autodebit` | Autodebit & Gerai Retail | 151 | T14 |
| `inventory` | Inventaris & Stok | 102–103 | T15 |
| `procurement` | Pengadaan & Vendor | 104–106 | T15 |
| `asset` | Aset | 107–108 | T16 |
| `budget` | Anggaran (RAPBS) | 109 | T17 |
| `tax_nonpayroll` | Pajak Non-Gaji & e-Faktur | 110 | T17 |
| `payable` | Utang Usaha | 111 | T17 |
| `library` | Perpustakaan | 112–115 | T18 |
| `transport` | Transportasi | 119–122 | T19 |
| `counseling` | Konseling & Rujukan | 130, 132 | T20 |
| `discipline` | Poin & Pembinaan Disiplin | 131 | T20 |
| `event` | Event & Persetujuan Orang Tua | 133–134 | T21 |
| `extracurricular` | Ekstrakurikuler | 135 | T21 |
| `achievement` | Prestasi Siswa | 136 | T21 |
| `pkl` | PKL | 139–141 | T22 |
| `quality` | QA & Akreditasi | 142–144 | T23 |
| `gov_reporting` | Pelaporan Pemerintah (Dapodik) | 145–146 | T23 |
| `bi` | BI & Dashboard Kustom | 152 | T24 |
| `ai_assistant` | AI Assistant | 153 | T24 |
| `predictive` | Analitik Prediktif | 154 | T24 |
| `campus3d` | Kampus 3D | 155–158 | T25 |
| `white_label` | Aplikasi White-label | 159 | T26 |

### 2c. Ops `0001_ops_base.up.sql`
```
audit_events(id bigserial PK, school_id uuid, module text, entity_type, entity_id, action, actor_user_id, actor_role,
  before_data jsonb, after_data jsonb, reason, correlation_id, created_at, forwarded_at timestamptz NULL)   -- salin hr 0001
INDEX audit_events(id) WHERE forwarded_at IS NULL
doc_sequences(school_id, prefix text, year int, last int, PRIMARY KEY(school_id, prefix, year))           -- nomor dokumen ops (WO-2026-0001), dipakai ulang T15–T19
CREATE EXTENSION IF NOT EXISTS btree_gist;                                                                  -- exclusion constraint booking
```

### 2d. Ops `0002_facility.up.sql`
Semua tabel: `id uuid PK default gen_random_uuid()`, `school_id uuid NOT NULL`, `created_at/updated_at`. Unit pemilik = `school_id` (unit sekolah = baris `schools` identity).
```
buildings(id, school_id, code, name, address NULL, sort int, deleted_at, UNIQUE(school_id, code) WHERE deleted_at IS NULL)
floors(id, school_id, building_id FK, level int, name, deleted_at, UNIQUE(building_id, level) WHERE deleted_at IS NULL)
rooms(id, school_id, floor_id FK, code, name,
  kind CHECK IN('kelas','lab','aula','lapangan','perpustakaan','ruang_serbaguna','kantin','kantor','uks','gudang','lainnya'),
  capacity int NOT NULL CHECK(capacity>=0),
  usage CHECK IN('bookable','fixed'),                              -- FR-116-002: fixed = penggunaan tetap (kelas), tidak bisa dipesan
  requires_approval bool NOT NULL DEFAULT false,                   -- FR-117-003
  status CHECK IN('active','maintenance','inactive'),
  available_from time NULL, available_to time NULL,                -- jam pakai ruang; NULL = sepanjang hari
  notes, deleted_at, UNIQUE(school_id, code) WHERE deleted_at IS NULL,
  CHECK(usage='bookable' OR requires_approval=false), CHECK((available_from IS NULL)=(available_to IS NULL)))
room_facilities(id, school_id, room_id FK, name, qty int NOT NULL DEFAULT 1 CHECK(qty>0))       -- fasilitas pendukung
room_fixed_schedules(id, school_id, room_id FK, weekday smallint CHECK(weekday BETWEEN 1 AND 7), starts_time time, ends_time time,
  label text NOT NULL, valid_from date NOT NULL, valid_to date NULL, CHECK(ends_time>starts_time))  -- jadwal tetap ruang bookable (mis. praktikum)
room_bookings(id, school_id, room_id FK, starts_at timestamptz, ends_at timestamptz, purpose text NOT NULL, participants int CHECK(participants>0),
  requester_user_id uuid NOT NULL, source CHECK IN('portal','internal'), source_service text NULL, source_ref text NULL,
  status CHECK IN('pending_approval','confirmed','rejected','cancelled'), approval_id uuid NULL, cancel_reason NULL, cancelled_by NULL,
  idempotency_key text NOT NULL, UNIQUE(school_id, idempotency_key), CHECK(ends_at>starts_at),
  EXCLUDE USING gist (room_id WITH =, tstzrange(starts_at, ends_at, '[)') WITH &&) WHERE (status IN ('pending_approval','confirmed')))  -- BR-FAC-01 anti-race
work_orders(id, school_id, code text NOT NULL, source CHECK IN('report','preventive','ticket'),
  room_id uuid NULL, asset_id uuid NULL, title, description, priority CHECK IN('rendah','sedang','tinggi','darurat'),
  status CHECK IN('dilaporkan','ditugaskan','diproses','menunggu_material','selesai','ditutup','dibatalkan'),
  reporter_user_id uuid NOT NULL, assignee_user_id uuid NULL, completion_note NULL, completed_at NULL,
  schedule_id uuid NULL, scheduled_for date NULL, ticket_ref text NULL, callback_url text NULL, purchase_request_ref text NULL,
  idempotency_key text NULL, UNIQUE(school_id, code), UNIQUE(school_id, idempotency_key),
  UNIQUE(schedule_id, scheduled_for),                                -- AC-118-01 idempoten per jadwal
  CHECK(room_id IS NOT NULL OR asset_id IS NOT NULL))
work_order_photos(id, school_id, work_order_id FK, object_key, mime, size_bytes, sha256, uploaded_by, created_at)
work_order_events(id, school_id, work_order_id FK, from_status NULL, to_status, note NULL, actor_user_id, created_at)   -- riwayat status
work_order_costs(id, school_id, work_order_id FK, kind CHECK IN('material','jasa','lainnya'), description, amount_idr bigint CHECK(amount_idr>=0), created_by, created_at)
maintenance_schedules(id, school_id, room_id NULL, asset_id NULL, title, priority, interval_unit CHECK IN('day','week','month'), interval_count int CHECK(interval_count>0),
  next_due_date date NOT NULL, default_assignee_user_id NULL, active bool DEFAULT true, deleted_at, CHECK(room_id IS NOT NULL OR asset_id IS NOT NULL))
```
Down: drop terbalik. Tidak ada data demo di migrasi.

## 3. Fungsi murni (`internal/facility/model.go`) + vektor
| Fungsi | Vektor |
|---|---|
| `Bookable(room, now) bool` = `usage='bookable' && status='active' && deleted_at nil` | **AC-116-01** aula `maintenance` → tidak tampil di pencarian booking; ruang `fixed` (kelas) → tidak tampil (FR-116-002) |
| `Overlaps(a, b Interval)` setengah terbuka `[start,end)` | 10–12 vs 12–13 → tidak bentrok; 10–12 vs 11–13 → bentrok |
| `ExpandFixed(schedules, day, loc) []Interval` (Asia/Jakarta, hormati `valid_from/to`) | jadwal Senin 07–09 → muncul hanya di Senin dalam masa berlaku |
| `CheckBooking(room, req, busy) error` | **AC-117-01** lab dipesan 10.00–12.00, guru minta 11.00–13.00 → `ErrConflict`; peserta 41 > kapasitas 40 → `ErrOverCapacity` (BR-FAC-01); di luar `available_from/to` → `ErrOutsideHours`; bentrok jadwal tetap → `ErrConflict` |
| `NearestFreeSlots(busy, req, window, n=2) []Interval` (durasi sama, hari yang sama, dalam jam pakai ruang) | **AC-117-01** → `[12.00–14.00, 08.00–10.00]` (terdekat lebih dulu); hari penuh → kosong + pesan "tidak ada slot kosong hari ini" |
| `NeedsApproval(room)` = `requires_approval` | true → status `pending_approval` + core `room_booking` (FR-117-003); false → `confirmed` |
| `NextDue(date, unit, count)` (akhir bulan di-clamp) | **AC-118-01** 2026-01-15 + 3 bulan → 2026-04-15; 2026-01-31 + 1 bulan → 2026-02-28 |
| `DueSchedules(schedules, today)` | `next_due_date <= today && active` → dibuat WO; jadwal lewat 2 periode (job mati) → satu WO untuk periode tertua lalu maju satu periode per run (tidak menumpuk) |
| `CanTransition(from, to, actorKind)` | dilaporkan→ditugaskan (facility_officer); ditugaskan→diproses/menunggu_material→selesai (teknisi penerima); selesai→ditutup (facility_officer/pelapor); selain itu 422 |
| `WOTotalCost(costs) int64` | 150.000 + 75.000 → 225.000 (FR-118-004) |
| `MissingRequirements(key, enabled)` (identity) | campus3d tanpa facility → `["facility"]` |

## 4. Integrasi & job
### 4.1 Integrasi keluar
- **Approval (FR-117-003):** tambahkan `room_booking` ("Pemesanan Ruang") ke `core/internal/model/domain.go` (konstanta + `RequestTypeLabels` + `KnownRequestTypes`) dan `requestTypeLabel` di `web/src/services/core/types.ts`. Booking ke ruang `requires_approval` → `POST /internal/approvals` `{request_type:'room_booking', source_service:'ops', source_id:<booking_id>, title:"Pemesanan <ruang> <tgl jam>", summary:{ruang, waktu, tujuan, peserta}, maker_user_id: requester, callback_url: OPS_PUBLIC_INTERNAL_URL+'/internal/approval-callbacks', idempotency_key:'room_booking:'+id}`. Core 422 (flow belum dikonfigurasi) → booking **gagal dibuat** dengan pesan core (bukan auto-approve). Slot `pending_approval` ikut memblokir. Callback idempoten: approved → `confirmed`, rejected → `rejected` (slot bebas). Batal saat pending → `POST /internal/approvals/{id}/cancel`.
- **Notifikasi** (`POST {NOTIFICATION_URL}/internal/notifications`, `Idempotency-Key` wajib): keputusan booking ke pemohon (`booking:<id>:<status>`); WO ditugaskan ke teknisi (`wo:<id>:assigned:<assignee>`); WO selesai ke pelapor (`wo:<id>:selesai`).
- **FR-118-004 permintaan pembelian:** hook nil-safe di paket facility: `type PurchaseRequester interface { CreateFromWorkOrder(ctx, in WOPurchaseInput) (prRef string, err error) }` dengan `WOPurchaseInput{SchoolID, RequesterUserID, WorkOrderID, WorkOrderCode, Lines []{Description string; Qty int; EstAmountIDR int64}}`. T15 (gelombang 4) mendaftarkannya di `main.go`. Hook nil atau fitur `procurement` mati → 409 `"Modul pengadaan belum aktif"`, tombol "Buat Permintaan Pembelian" disembunyikan. Sukses → simpan `purchase_request_ref`, status WO `menunggu_material`.
- **Aset di WO (FR-118-001):** hook nil-safe `type AssetLookup interface { Exists(ctx, schoolID, assetID string) (roomID string, ok bool, err error) }` didaftarkan T16 (gelombang 5). Hook nil → `asset_id` ditolak 422 `"Modul aset belum aktif"`; isian aset disembunyikan di UI. Jadwal perawatan per aset mengikuti aturan sama.
- **Identity (dibuat T9, dipakai T10/T19/T20/T21):** `GET /internal/students/{studentUserId}/guardians?school_id=` → `[{guardian_user_id, relationship, is_primary}]` hanya link `active`, grup `InternalOnlyMiddleware` (tanpa migrasi). Test: school lain → kosong.

### 4.2 Kontrak internal yang **disediakan** T9 (X-Internal-Key, wajib `school_id`)
BR-FAC-02: semua modul membaca ruang dari sini; tidak ada tabel ruang lain di service mana pun. Paket `facility` juga mengekspor fungsi baca in-process (`ListRooms`, `GetRoom`, `ListBuildings`, `BusyIntervals`, `ActiveWorkOrders`) untuk paket lain di `ops` (T16, T25): pakai fungsi ini, **bukan** HTTP ke diri sendiri dan bukan query langsung ke tabel facility.
| Method & path | Body / hasil | Dipakai |
|---|---|---|
| `GET /internal/facility/buildings` | `[{id, code, name, floors:[{id, level, name}]}]` | T25 |
| `GET /internal/facility/rooms?kind=&status=&bookable=&ids=&q=` | `[{id, code, name, kind, capacity, usage, status, requires_approval, building:{id,name}, floor:{id,level,name}, facilities:[{name,qty}]}]` | T16 (lokasi aset), T21 (lokasi event), T25, modul pembelajaran/menu 39 (FR-116-003) |
| `GET /internal/facility/rooms/{id}` | satu ruang (404 bila beda sekolah/terhapus) | T16, T21, T25 |
| `GET /internal/facility/availability?room_ids=&from=&to=` | `[{room_id, busy:[{starts_at, ends_at, kind:'booking'\|'fixed', label, booked_by_name?, booking_id?}]}]` (booking `confirmed` + `pending_approval` + jadwal tetap) | T21, T25 (AC-157-01) |
| `POST /internal/facility/bookings` | `{room_id, starts_at, ends_at, purpose, participants, requester_user_id, source_service, source_ref, idempotency_key}` → `201 {id, status}`; key sama → `200` data sama; bentrok → `409 {error, nearest_slots:[…]}`; kapasitas/ruang tak bisa dipesan → `422` | T21 (event + lokasi, alur Bagian 3 §6) |
| `POST /internal/facility/bookings/{id}/cancel` | `{actor_user_id, reason}` | T21 |
| `POST /internal/facility/work-orders` | `{room_id?, asset_id?, title, description, priority, reporter_user_id, ticket_ref, callback_url?, idempotency_key}` → `201 {id, code, status}` (`source='ticket'`) | T11 (konversi tiket → WO, lampiran Bagian 3) |
| `GET /internal/facility/work-orders?status=active&room_ids=&ticket_ref=` | `[{id, code, room_id, asset_id, title, priority, status, assignee_name, updated_at}]`; `active` = bukan `selesai/ditutup/dibatalkan` | T11, T25 (AC-158-01) |
| Callback **dari** ops ke `callback_url` WO | `{work_order_id, ticket_ref, status, completion_note}` + `Idempotency-Key: wo:<id>:<status>`; gagal → `POST core /internal/exceptions` kind `callback` | T11 |

### 4.3 Job (runner `ops`)
1. **Perawatan berkala (FR-118-003, AC-118-01):** harian (zona sekolah) → `DueSchedules` → buat WO `source='preventive'` (`ON CONFLICT (schedule_id, scheduled_for) DO NOTHING`), assign ke `default_assignee_user_id` (status `ditugaskan` + notifikasi) atau `dilaporkan` bila kosong, lalu `next_due_date = NextDue(...)`. Dalam satu transaksi.
2. **Forward audit** ke core (pola hr), batch, tandai `forwarded_at`.

## 5. Endpoint publik (JWT + `requireFeature("facility")`; `admin`, `superadmin` selalu boleh)
| Method & path | Peran |
|---|---|
| `CRUD /facility/buildings`, `/facility/floors`, `/facility/rooms` (+ `facilities`, `fixed-schedules`); `DELETE` = soft | facility_officer |
| `GET /facility/rooms?bookable=true&from=&to=&min_capacity=&kind=` (hanya `Bookable`, AC-116-01) · `GET /facility/rooms/{id}/calendar?from=&to=` (FR-117-004) | semua staf (ALL_STAFF_ROLES) |
| `POST /facility/bookings` `{room_id, starts_at, ends_at, purpose, participants}` → 201 / 409 `{nearest_slots}` / 422 | semua staf (guru, event_coordinator, dst.) |
| `GET /facility/bookings/mine` · `POST /facility/bookings/{id}/cancel` `{reason}` | pemohon (milik sendiri) |
| `GET /facility/bookings?room_id=&status=&from=&to=` · `POST /facility/bookings/{id}/cancel` (siapa pun, alasan wajib) | facility_officer, kepala_sekolah (baca) |
| `POST /facility/work-orders` (multipart: data + foto ≤ `MAX_UPLOAD_BYTES`, mime image/jpeg/png/webp, sha256) (FR-118-001) | semua staf (pelapor) |
| `GET /facility/work-orders/mine` | pelapor |
| `GET /facility/work-orders?status=&priority=&room_id=&assignee=` · `GET /{id}` (+ foto lewat endpoint ber-auth, bukan URL publik) | facility_officer, kepala_sekolah (baca); teknisi (hanya yang ditugaskan padanya); pelapor (miliknya) |
| `POST /facility/work-orders/{id}/assign` `{assignee_user_id}` (penerima wajib punya peran `teknisi` di sekolah ini, cek identity) · `/close` · `/cancel` `{reason}` | facility_officer |
| `POST /facility/work-orders/{id}/status` `{to, note}` (selesai → `completion_note` wajib) · `POST/DELETE /{id}/costs` (FR-118-004) | teknisi penerima, facility_officer |
| `POST /facility/work-orders/{id}/purchase-request` `{lines}` | facility_officer |
| `CRUD /facility/maintenance-schedules` | facility_officer |
| `POST /internal/approval-callbacks` (X-Internal-Key) | core |
Semua mutasi menulis `audit_events`. List `?page=&per_page=` + `meta`.

## 6. Frontend
`web/src/services/ops/{facility.ts,types.ts}` (pakai `api('ops')`), `web/src/router/domains/facility.routes.ts` (daftarkan di `router/index.ts`: satu import + spread). Semua route `meta.feature: 'facility'`, navGroup **Fasilitas**.
| Path | View (`views/facility/…`) | Peran | navLabel |
|---|---|---|---|
| `fasilitas/ruang` | `RoomsView.vue` (tab Gedung & Lantai / Ruang: filter jenis/status, badge berteks "Dalam perbaikan", dialog fasilitas pendukung & jadwal tetap) | facility_officer | Master Ruang |
| `fasilitas/booking` (query `?room_id=` didukung, dipakai T25) | `BookingView.vue` (cari ruang tersedia: tanggal, jam, peserta, jenis → daftar ruang; kalender minggu per ruang FR-117-004; form pesan; galat 409 menampilkan "Bentrok dengan …" + tombol slot terdekat yang mengisi ulang form) | semua staf | Pesan Ruang |
| `fasilitas/booking-saya` | `MyBookingsView.vue` (status + `ApprovalTimeline` sourceService `ops`) | semua staf | Pemesanan Saya |
| `fasilitas/pemesanan` | `BookingsAdminView.vue` | facility_officer, kepala_sekolah | Semua Pemesanan |
| `fasilitas/lapor-kerusakan` | `ReportDamageView.vue` (pilih gedung→lantai→ruang, foto, prioritas) + daftar laporan saya | semua staf | Lapor Kerusakan |
| `fasilitas/perawatan` · `/:id` | `WorkOrdersView.vue`, `WorkOrderDetailView.vue` (timeline status, foto, tugaskan teknisi, biaya + total, tombol Buat Permintaan Pembelian bila hook aktif) | facility_officer, kepala_sekolah | Perintah Kerja |
| `fasilitas/perawatan-berkala` | `MaintenanceSchedulesView.vue` (interval, berikutnya, penanggung jawab) | facility_officer | Perawatan Berkala |
| `teknisi/tugas` | `TechnicianTasksView.vue` (mobile-first: daftar WO saya, ubah status, catatan, biaya) | teknisi | Tugas Perawatan |
- Superadmin: `components/identity/SchoolFeaturesDialog.vue` dipanggil dari `views/identity/superadmin/SekolahView.vue` (tombol "Fitur Add-on" per sekolah): daftar semua key dari `GET /schools/{id}/features` dengan `featureLabels`, saklar per baris, galat 422 dependensi tampil inline. Kolom "Akuntansi" yang ada dibiarkan.
- `views/DashboardRedirectView.vue`: `facility_officer` → `/fasilitas/perawatan`, `teknisi` → `/teknisi/tugas`. Peran baru lain diberi landing oleh task pemiliknya.

## 7. Bukti handoff
- Kerangka: `go vet`/`go test` ops hijau, migrasi ops 0001–0002 dan identity 0017 up/down/up di DB baru; down 0017 gagal dengan pesan jelas bila ada baris peran baru; `/healthz` ops; middleware `requireFeature` (fitur mati → 403) dites; `IsValidRole` menerima 25 peran baru; `FeatureStatuses` memuat 31 key baru; `MissingRequirements` dites.
- Vektor §3 + test handler: **AC-116-01**, **AC-117-01** (409 + `nearest_slots`), BR-FAC-01 kapasitas, race dua booking paralel → satu sukses (exclusion constraint), FR-117-003 (pending → callback approved/rejected, idempoten dua kali), **AC-118-01** (job dua kali di hari yang sama → satu WO), teknisi lain tidak bisa mengubah WO yang bukan miliknya (403), akses lintas sekolah ke `/internal/facility/*` dan `/facility/*` ditolak, hook procurement/asset nil → 409/422.
- Kontrak §4.2 dites dengan `httptest` (bentuk JSON persis seperti tabel).
- Playwright: superadmin menyalakan `facility` → facility_officer membuat gedung/lantai/lab (kapasitas 40) + aula status perbaikan → guru tidak melihat aula; guru memesan lab 10–12 → guru kedua memesan 11–13 ditolak dan memilih slot 12–14 → pelapor melapor AC rusak dengan foto → facility_officer menugaskan teknisi → teknisi menyelesaikan dengan catatan → pelapor melihat status selesai.
