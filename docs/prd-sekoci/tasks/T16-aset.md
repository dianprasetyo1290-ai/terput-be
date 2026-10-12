# T16: Register aset, label QR, mutasi & pelepasan di service `ops` (penyusutan dari Accounting Engine)

**Menu PRD:** 107, 108 (Bagian 3 Grup 16; BR-AST-01, IF-OPS-01/02/04) · **Branch:** `feat/T16-aset` · **Service:** `ops` port 8093 (paket `internal/asset`) · **Migrasi:** `services/ops/migrations/0020_asset.*` (nomor 0020–0029 milik T16) · **Gelombang:** 5
**Prasyarat:** gelombang 4 di-merge: T15 (`goods_receipts`, `goods_receipt_lines.asset_handoff_at`, hook `procurement.OnGoodsReceived`, `item_categories`), T9 (`rooms`, peran & feature key). **T17 di gelombang yang sama**: endpoint aset di Accounting Engine (Laravel) dibangun T17 sesuai kontrak §4.2 di bawah. T16 koding terhadap kontrak itu dan mengujinya dengan `httptest` fake.
**Feature key (usulan, final mengikuti T9 §2):** `asset`. **Peran (final mengikuti T9 §2):** `asset_officer`, `kepala_sekolah` (= Kepala Unit), `finance_manager`, `finance_admin` (= "Accounting" di PRD), `kepala_yayasan` (baca lintas unit).
**Batas tanggung jawab (Lampiran PRD):** SEKOCI menyimpan data **fisik** aset; nilai buku, akumulasi, dan penyusutan **dihitung Accounting Engine**. Tidak ada rumus penyusutan di Go.

## 1. Donor / acuan kode
Tidak ada donor. Acuan:
| Kebutuhan | Tiru dari |
|---|---|
| Approval, callback, request type | T15 §1 (kontrak core T1 §3; tambah `asset_disposal`, `asset_transfer` di `services/core/internal/model/domain.go` bila belum didaftarkan T9) |
| Client accounting + exception | `services/billing/internal/client/integrations.go` (`PostJSON` dengan Idempotency-Key, `CreateException`) |
| Outbox + job retry backoff | pola `payroll_journals` T3 (`services/hr/internal/payroll`) |
| Unggah foto | `services/billing/internal/storage/` |
| Label QR (frontend) | paket `qrcode` yang sudah ada di `web/package.json`. Pemindai (IF-OPS-04: keyboard wedge + kamera): **pakai komponen pemindai yang dibuat T12/T18** di `web/src/components` (cari `scan`), jangan buat salinan |
| Data ruang | tabel `rooms` T9 (BR-FAC-02: satu-satunya sumber ruang) |

## 2. Migrasi `0020_asset.up.sql`
```
asset_categories(id, school_id, code, name,
  useful_life_months int NULL CHECK(>0),        -- tidak ada default: wajib diisi sebelum aset kategori ini bisa diaktifkan
  dep_start CHECK IN('acquisition_month','next_month') NULL,
  asset_acc_code text NULL, accum_dep_acc_code text NULL, dep_expense_acc_code text NULL,
  deleted_at, UNIQUE(school_id, code) WHERE deleted_at IS NULL)
ALTER TABLE item_categories ADD COLUMN asset_category_id uuid NULL REFERENCES asset_categories(id);   -- barang tracking=asset → kategori aset
asset_settings(school_id PK, code_format text NOT NULL /* token {cat},{yyyy},{seq:N} */, interunit_acc_code text NULL,
  disposal_gain_acc_code NULL, disposal_loss_acc_code NULL, proceeds_acc_code NULL)
assets(id, school_id, code, name, category_id, status CHECK IN('candidate','active','disposed','transferred_out'),
  condition CHECK IN('baik','rusak_ringan','rusak_berat') NULL,
  acquisition_cost_idr bigint CHECK(>=0), acquired_on date, residual_idr bigint NOT NULL DEFAULT 0,
  room_id uuid NULL, custodian_user_id uuid NULL, serial_no NULL, photo_object_key NULL,
  qr_token text NOT NULL UNIQUE /* acak 128-bit, bukan id */,
  source CHECK IN('goods_receipt','manual','interunit_transfer'), gr_line_id uuid NULL, unit_seq int NULL, po_id NULL, vendor_id NULL,
  transferred_from_asset_id uuid NULL, offset_acc_code text NULL /* akun lawan untuk perolehan non-PO */,
  activated_at NULL, activated_by NULL, deleted_at,
  UNIQUE(school_id, code) WHERE deleted_at IS NULL)
CREATE UNIQUE INDEX one_candidate_per_unit ON assets(gr_line_id, unit_seq) WHERE gr_line_id IS NOT NULL   -- idempoten backfill
asset_movements(id, school_id, asset_id, kind CHECK IN('location','custodian','condition','unit'),
  from_value jsonb, to_value jsonb, effective_date date NOT NULL, reason,
  status CHECK IN('done','pending_approval','approved','rejected'), approval_id NULL, to_school_id uuid NULL,
  created_by, created_at)                      -- riwayat, tidak di-UPDATE menimpa (FR-108-001)
asset_disposals(id, school_id, asset_id, kind CHECK IN('dijual','dihibahkan','rusak','hilang'), disposed_on date, proceeds_idr bigint DEFAULT 0,
  reason NOT NULL, status CHECK IN('pending_approval','approved','rejected','cancelled'), approval_id NULL, created_by)
CREATE UNIQUE INDEX one_open_disposal ON asset_disposals(asset_id) WHERE status='pending_approval'
asset_accounting_events(id, school_id, asset_id, kind CHECK IN('acquisition','transfer','disposal'), source_id uuid,
  reference text UNIQUE,        -- asset:acq:<asset_id> | asset:xfer:<movement_id> | asset:disp:<disposal_id>
  payload jsonb NOT NULL, status CHECK IN('pending','posted','failed'), attempts int default 0, error NULL, trans_nos text[], posted_at NULL)
asset_book_values(asset_id PK, school_id, cost_idr, accumulated_idr, book_value_idr, last_period char(7), as_of timestamptz)   -- cache dari accounting
asset_depreciation_runs(school_id, period char(7), status CHECK IN('pending','done','failed'), error NULL, PRIMARY KEY(school_id, period))
```
"Pelepasan menunggu approval" **bukan** status aset: aset tetap `active` (AC-108-01), ditandai lewat `asset_disposals.status='pending_approval'`.

## 3. Fungsi murni + vektor (`internal/asset/calc.go`)
| Fungsi | Vektor |
|---|---|
| `CandidatesFromReceipt(ev procurement.GoodsReceivedEvent) []Candidate` | **AC-105-02/FR-107-002** 1 baris laptop qty 2 harga 12.000.000 → 2 kandidat (`unit_seq` 1,2), masing-masing biaya 12.000.000 |
| `CanActivate(asset, category) []string` | **AC-107-01** tanpa `room_id` atau `custodian_user_id` → `["lokasi wajib","penanggung jawab wajib"]` ; kategori tanpa `useful_life_months`/akun → "kategori aset belum lengkap" ; lengkap → `[]` |
| `AcquisitionPayload(asset, category, settings)` | sumber `goods_receipt` → `post_journal=false` (GL dari tagihan vendor T17) ; sumber `manual` → `post_journal=true` + `offset_acc_code` wajib |
| `MovementKind(before, after) []kind` | ruang berubah → `location`; unit (school) berubah → `unit` (perlu approval + kirim ke accounting, FR-108-004) |
| `CanDispose(asset, openDisposal)` | status bukan `active` atau sudah ada pengajuan → galat |
| `ApplyDisposalDecision(asset, decision)` | **AC-108-01** pending → aset `active`; approved → `disposed` + event `disposal` ; rejected → tetap `active` |
| `FormatAssetCode(format, cat, year, seq)` | `{cat}-{yyyy}-{seq:4}`, LPT, 2026, 7 → `LPT-2026-0007` |
| `ScanView(asset, role)` | **AC-107-02** asset_officer/kepala_sekolah → detail lengkap + nilai buku ; peran staf lain → nama, kode, ruang, kondisi saja |

## 4. Integrasi & job
### 4.1 Dari T15 (penerimaan barang)
- `asset.OnGoodsReceived(ctx, tx, ev procurement.GoodsReceivedEvent) error` **persis** dengan signature T15 §4; didaftarkan di `main.go`: `procurement.OnGoodsReceived = asset.OnGoodsReceived`. Membuat kandidat (`status='candidate'`, `source='goods_receipt'`, kategori dari `item_categories.asset_category_id`; kosong → kandidat tetap dibuat dengan `category_id` NULL dan wajib diisi sebelum aktivasi).
- **Backfill** saat start + job harian: baris `goods_receipt_lines` ber-tracking `asset` dengan `asset_handoff_at IS NULL` (penerimaan sebelum T16 aktif) → panggil fungsi yang sama, lalu set `asset_handoff_at`. Idempoten oleh `UNIQUE(gr_line_id, unit_seq)`.
### 4.2 Kontrak ke Accounting Engine (dibangun T17 di Laravel, IF-OPS-01/02)
Semua `X-Internal-Key`, `Idempotency-Key = reference`. Respons ulang dengan reference sama → 200 + hasil lama.
| Endpoint accounting | Body / respons |
|---|---|
| `POST /api/internal/assets` | `{school_id, asset_ref:<asset_id>, code, name, acquired_on, cost_idr, residual_idr, useful_life_months, dep_start, asset_acc_code, accum_dep_acc_code, dep_expense_acc_code, post_journal:bool, offset_acc_code?, reference:"asset:acq:<id>"}` → `201 {asset_ref, trans_no?}` |
| `POST /api/internal/assets/{asset_ref}/transfer` | `{school_id, to_school_id, to_asset_ref, effective_date, interunit_acc_code_from, interunit_acc_code_to, reference:"asset:xfer:<movement_id>"}` → `{trans_no_out, trans_no_in}` (FR-108-004) |
| `POST /api/internal/assets/{asset_ref}/dispose` | `{school_id, disposed_on, kind, proceeds_idr, proceeds_acc_code?, gain_acc_code, loss_acc_code, reference:"asset:disp:<id>"}` → `{trans_no, book_value_idr}` |
| `POST /api/internal/assets/depreciate` | `{school_id, period:"2026-10"}` → `{assets:int, total_idr, trans_no?}`; idempoten per aset+periode |
| `GET /api/internal/assets?school_id=&refs=a,b` | `[{asset_ref, cost_idr, accumulated_idr, book_value_idr, last_period, schedule:[{period, amount_idr}]}]` (FR-108-003) |
### 4.3 Alur
- **Aktivasi (FR-107-001/002/004, AC-107-01):** `POST /assets/{id}/activate` → `CanActivate` → `active`, `qr_token`, lalu `asset_accounting_events(acquisition, pending)` dalam satu transaksi. Pengirim (job tiap menit + percobaan langsung setelah commit) memanggil §4.2; sukses → `posted`; gagal setelah batas percobaan (config) → `failed` + `CreateException(kind='journal_posting', retry_url=/internal/assets/events/{id}/retry)`. Aset tetap aktif; badge "Belum terkirim ke akuntansi".
- **Mutasi (FR-108-001):** lokasi/penanggung jawab/kondisi → baris `asset_movements(done)` + update kolom aset, dengan `effective_date`. Mutasi **antar unit** → core `asset_transfer` (asumsi konservatif karena memindahkan nilai antar entitas; catat di handoff); approved → aset asal `transferred_out`, aset baru di `to_school_id` (`source='interunit_transfer'`, `transferred_from_asset_id`), event `transfer`.
- **Pelepasan (FR-108-002, AC-108-01):** `POST /assets/{id}/disposals` → core `asset_disposal` (`amount_idr` = nilai buku cache bila ada). Callback approved → `disposed` + event `disposal`. Rejected → tidak ada perubahan.
- **Penyusutan (BR-AST-01):** job bulanan (tanggal dari config, zona sekolah) per sekolah ber-feature `asset` + `accounting` → `POST …/depreciate` periode bulan lalu, catat `asset_depreciation_runs`; gagal → exception `journal_posting`. Setelah itu refresh `asset_book_values` dari `GET /api/internal/assets`.
- Sekolah tanpa feature `accounting` → event tetap `pending`, UI menampilkan "Modul akuntansi tidak aktif" (accounting membalas 409).

## 5. Endpoint
| Method & path | Peran |
|---|---|
| `CRUD /assets/categories`, `GET/PUT /assets/settings` | asset_officer (tulis); finance_manager, finance_admin (baca + akun) |
| `GET /assets?status=&category_id=&room_id=&room_ids=&custodian_user_id=&q=&page=&per_page=` · `GET /assets/{id}` | asset_officer, kepala_sekolah, finance_manager, finance_admin; kepala_yayasan baca lintas unit yayasannya; **juga dipakai T25** |
| `POST /assets` (manual) · `PUT /assets/{id}` (kandidat/data fisik) · `POST /assets/{id}/activate` · `POST /assets/{id}/photo` | asset_officer |
| `GET /assets/{id}/label` (data label: kode, nama, URL scan) · `GET /assets/labels?ids=` (cetak massal) | asset_officer |
| `GET /assets/scan/{qr_token}` | semua pengguna login sekolah yang sama; isi dibatasi `ScanView` (FR-107-003) |
| `POST /assets/{id}/movements` · `GET /assets/{id}/movements` | asset_officer (tulis); baca seperti `GET /assets` |
| `POST /assets/{id}/disposals` · `POST /assets/disposals/{id}/cancel` · `GET /assets/disposals?status=` | asset_officer (tulis); finance_manager (baca) |
| `GET /assets/{id}/depreciation` (cache `asset_book_values` + jadwal dari accounting, "Diperbarui: as_of") | asset_officer, finance_manager, finance_admin |
| `POST /internal/assets/events/{id}/retry` · `POST /internal/approval-callbacks` (tambah case) | internal key |

**Kontrak baca untuk T25 (3D Campus), stabil, ubah hanya dengan reviewer:**
```
GET /assets?room_ids=<uuid,uuid>&status=active&per_page=200
→ data: [{id, code, name, category:{id, name}, room_id, condition, status, custodian:{user_id, name}|null, photo_url|null}], meta:{page, per_page, total}
GET /assets/summary-by-room?room_ids=<uuid,…>        (baru, untuk overlay per ruang)
→ data: [{room_id, total, by_condition:{baik, rusak_ringan, rusak_berat, tanpa_kondisi}, by_category:[{category_id, name, count}], pending_disposal:int}]
GET /assets/{id}  → detail di atas + acquired_on, movements terakhir (5), book_value_idr hanya untuk peran keuangan/asset_officer
```
Peran yang boleh memanggil dua endpoint daftar/ringkasan: daftar baca di atas + `facility_officer` (nama final T9 §2). Hanya status `active` yang dihitung di ringkasan; `room_ids` maksimal 500 (limit teknis dari config).

## 6. Frontend
`services/asset/*.ts`, `router/domains/ops-asset.routes.ts`, `meta.feature: 'asset'`, navGroup **Aset**.
| Path | View (`views/asset/…`) | Peran | Label |
|---|---|---|---|
| `aset/register` · `/:id` | `AssetsView.vue` (tab Aktif / Calon Aset (dari penerimaan, badge jumlah) / Dilepas; filter kategori, ruang, kondisi), `AssetView.vue` (tab Data / Riwayat Mutasi / Penyusutan / Label; badge teks status akuntansi; tombol Aktifkan dengan daftar kekurangan dari `CanActivate`) | asset_officer, kepala_sekolah, finance_manager, finance_admin | Register Aset |
| `aset/label` | `AssetLabelsView.vue` (pilih aset → lembar label QR siap cetak, `@media print`) | asset_officer | Cetak Label |
| `aset/scan` · `aset/scan/:token` | `AssetScanView.vue` (input pemindai keyboard wedge + kamera; hasil sesuai hak akses) | semua staf ber-akun | Pindai Aset |
| `aset/mutasi` | `AssetMovementsView.vue` (dialog pindah ruang/penanggung jawab/kondisi/unit dengan tanggal efektif; mutasi antar unit menampilkan `ApprovalTimeline`) | asset_officer | Mutasi Aset |
| `aset/pelepasan` | `AssetDisposalsView.vue` (ajukan pelepasan, alasan wajib, dialog konfirmasi; status "Menunggu approval" sementara aset tetap tercantum Aktif) | asset_officer, finance_manager | Pelepasan Aset |
| `aset/penyusutan` | `DepreciationView.vue` (tabel nilai perolehan / akumulasi / nilai buku per aset dari accounting, "Diperbarui: <waktu>", status run bulanan) | asset_officer, finance_manager, finance_admin | Penyusutan |
| `aset/pengaturan` | `AssetSettingsView.vue` (kategori aset: masa manfaat, awal penyusutan, akun; format kode; akun antar unit & pelepasan) | asset_officer, finance_manager | Pengaturan Aset |
Rupiah via formatter bersama, rata kanan `tabular-nums`.

## 7. Bukti handoff
Vektor §3 hijau. Test wajib: **AC-107-01** (aktivasi → event acquisition terkirim ke fake accounting dengan reference `asset:acq:<id>`; kirim ulang → 1 kali di fake), **AC-107-02** (scan oleh dua peran berbeda → isi berbeda; token tak dikenal → 404; sekolah lain → 404), **AC-108-01** (aset `active` selama pending; approved → `disposed` + event; callback ganda tidak membuat event kedua), AC-105-02 ujung ke ujung dengan T15 (penerimaan 2 laptop → 2 calon aset) + backfill idempoten, mutasi antar unit lewat approval, kegagalan accounting → exception `journal_posting` + retry sukses, penyusutan bulanan dipanggil sekali per periode. Cakupan FR: FR-107-001..004, FR-108-001..004.
Playwright: PO laptop (T15) → terima 2 → Register Aset tab Calon Aset tampil 2 → lengkapi ruang + penanggung jawab → Aktifkan → badge "Terkirim ke akuntansi" → cetak label → buka `aset/scan/<token>` sebagai guru (tampilan terbatas) → ajukan pelepasan "rusak" → aset masih Aktif → finance_manager setujui di Approval Center → status Dilepas → tab Penyusutan menampilkan nilai buku dari accounting.
