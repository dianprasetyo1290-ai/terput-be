# T15: Inventory & Pengadaan di service `ops`: barang, stok, opname, PR, PO, penerimaan, vendor

**Menu PRD:** 102, 103, 104, 105, 106 (Bagian 3 Grup 16; BR-INV-01, BR-PRC-01, BR-BUD-02 via hook; IF-OPS-01 vendor) · **Branch:** `feat/T15-inventory-pengadaan` · **Service:** `ops` port 8093 (paket `internal/inventory`, `internal/procurement`) · **Migrasi:** `services/ops/migrations/0010_inventory.*`, `0011_procurement.*` (nomor 0010–0019 milik T15) · **Gelombang:** 4
**Prasyarat:** gelombang 3 di-merge: T9 (kerangka `ops`, master ruang, **semua peran & feature key** di identity 0017), T1 (approval/exception/audit, endpoint jurnal accounting). T16/T17 **belum** ada: anggaran dan aset disambung lewat hook nil-safe (§4), bukan import paket.
**Feature key (usulan, nama final mengikuti T9 §2):** `inventory` (102–103), `procurement` (104–106). Jika T9 §2 memakai satu key per grup, pakai key grup itu.
**Peran (snake_case dari PRD Bagian 3 "Peran Tambahan", nama final mengikuti T9 §2):** `inventory_officer`, `procurement_officer`, `finance_manager`, `finance_admin`, `kepala_sekolah` (= Kepala Unit), pemohon = `guru`/`pegawai`/staf ber-akun. Kuantitas barang `bigint` dalam satuan barang (pecahan tidak didukung, catat di handoff). Uang `bigint` rupiah.

## 1. Donor / acuan kode
Tidak ada donor di `~/Documents/LMS`. Ikuti pola Sekoci:
| Kebutuhan | Tiru dari |
|---|---|
| Layout, auth, envelope, client internal | kerangka `ops` dari T9 (asal `services/billing/internal/{common,client,config}`) |
| Approval | `services/core/internal/handler/internal.go` (`POST /internal/approvals`, kontrak T1 §3). Callback **ke** ops berisi `{approval_id, source_id, request_type, decision, decided_by, comment, decided_at}` + `Idempotency-Key: approval:<id>` (`services/core/internal/jobs/effects.go`). **Jangan** meniru nama field `resource_type/resource_id` di `services/hr/internal/handler/callbacks.go`: itu tidak cocok dengan payload core |
| Request type baru | tambah konstanta + entri `KnownRequestTypes` di `services/core/internal/model/domain.go` (hanya tambah baris; lewati jika T9 §2 sudah mendaftarkan) |
| Exception, notifikasi, PostJournal | `services/billing/internal/client/integrations.go` (`CreateException`, `Notify`, `PostJournal`), disalin, bukan diimpor |
| Enkripsi kolom + blind index (rekening, NPWP vendor) | `services/hr/internal/crypto/crypto.go` (salin ke `ops/internal/crypto` jika T9 belum menyalin) |
| PDF (PO, berita acara opname) | builder PDF pure-Go `buildReceiptPDF` di `services/billing/internal/handler/receipts.go`. Satu helper di `ops/internal/common/pdf.go` (cek dulu apakah T9/T18/T19 sudah membuatnya) |
| Penyimpanan berkas (dokumen legal vendor) | `services/billing/internal/storage/` |

## 2. Migrasi
`0010_inventory.up.sql`
```
item_categories(id, school_id, code, name, tracking CHECK IN('stock','asset','none'),   -- stock → menu 102, asset → calon aset 107, none → jasa/non-stok
  purchase_acc_code text NULL,          -- akun debit saat tagihan vendor diposting (dipakai T17)
  withholding_type_id uuid NULL,        -- default jenis PPh non-gaji (FK ditambah T17)
  deleted_at, UNIQUE(school_id, code) WHERE deleted_at IS NULL)
items(id, school_id, code, name, category_id, unit text NOT NULL, min_stock bigint NOT NULL DEFAULT 0 CHECK(min_stock>=0),
  status CHECK IN('active','inactive'), deleted_at, UNIQUE(school_id, code) WHERE deleted_at IS NULL)        -- FR-102-001
warehouses(id, school_id, code, name, room_id uuid NULL /* master ruang T9, BR-FAC-02 */, status, deleted_at)    -- FR-102-002
stock_balances(school_id, warehouse_id, item_id, qty bigint NOT NULL CHECK(qty >= 0), updated_at,
  PRIMARY KEY(warehouse_id, item_id))                                                                        -- BR-INV-01 di level DB
stock_movements(id, school_id, warehouse_id, item_id, direction CHECK IN('in','out'), qty bigint CHECK(qty>0),
  kind CHECK IN('receipt','issue','transfer_in','transfer_out','adjustment','return_to_vendor'),
  ref_type text NOT NULL, ref_id uuid NOT NULL, ref_no text NOT NULL,            -- referensi dokumen wajib (FR-102-003)
  transfer_id uuid NULL, balance_after bigint NOT NULL, note, created_by, created_at,
  idempotency_key, UNIQUE(school_id, idempotency_key))
-- trigger: tolak UPDATE/DELETE pada stock_movements (mutasi tidak dapat dihapus, BR-INV-01)
stock_counts(id, school_id, warehouse_id, number, status CHECK IN('counting','submitted','pending_approval','approved','rejected','adjusted','adjust_failed','cancelled'),
  started_at, started_by, submitted_at, approval_id NULL, report_object_key NULL, error NULL)
CREATE UNIQUE INDEX one_open_count ON stock_counts(warehouse_id) WHERE status IN ('counting')
stock_count_lines(id, school_id, count_id, item_id, system_qty bigint NOT NULL /* dibekukan saat mulai, FR-103-001 */,
  counted_qty bigint NULL, note, UNIQUE(count_id, item_id))
doc_sequences(school_id, kind text, year int, last int, PRIMARY KEY(school_id, kind, year))   -- nomor PR/PO/GR/SO
inventory_settings(school_id PK, doc_number_format jsonb)   -- format nomor per jenis dokumen, mis. {"po":"PO-{yyyy}-{seq:4}"}
audit_events(...)   -- pola admission 0001 (jika T9 belum membuatnya di ops)
```
`0011_procurement.up.sql`
```
vendors(id, school_id, code, name, npwp_enc bytea NULL, npwp_bidx text NULL, address, categories text[],
  payment_terms_days int NULL, status CHECK IN('active','inactive','blocked'), duplicate_ack_by uuid NULL, deleted_at)  -- FR-106-001
vendor_contacts(id, school_id, vendor_id, name, phone, email, is_primary bool)
vendor_documents(id, school_id, vendor_id, kind text, object_key, expires_on date NULL, uploaded_by, created_at)
vendor_bank_accounts(id, school_id, vendor_id, bank_name, account_no_enc bytea, account_no_bidx text, account_holder,
  status CHECK IN('pending_approval','active','rejected','superseded'), approval_id NULL, requested_by, activated_at)
CREATE UNIQUE INDEX one_active_bank ON vendor_bank_accounts(vendor_id) WHERE status='active'      -- AC-106-01
vendor_accounting_sync(vendor_id PK, school_id, payload_hash text, status CHECK IN('pending','synced','failed'),
  attempts int default 0, last_error NULL, synced_at NULL)                                         -- outbox FR-106-005
vendor_evaluations(id, school_id, vendor_id, goods_receipt_id UNIQUE, on_time bool /* dihitung */, quality_score smallint CHECK(1..5),
  price_score smallint CHECK(1..5), note, created_by, created_at)                                   -- FR-106-004

purchase_requests(id, school_id, number, requester_user_id, reason, needed_by date NULL,
  status CHECK IN('draft','pending_approval','approved','rejected','cancelled','partially_ordered','ordered'),
  total_est_idr bigint, over_budget bool default false, budget_check jsonb NULL /* hasil hook, disimpan apa adanya */,
  approval_id NULL, idempotency_key, UNIQUE(school_id, idempotency_key))
purchase_request_lines(id, school_id, pr_id, item_id NULL, description text NOT NULL, qty bigint CHECK(qty>0), unit text,
  est_unit_price_idr bigint CHECK(>=0), budget_line_key uuid NULL /* KONTRAK T17: budget_lines.line_key, tanpa FK di T15 */,
  qty_ordered bigint NOT NULL DEFAULT 0)
purchase_orders(id, school_id, number, vendor_id, payment_terms_days int NOT NULL, expected_date date NULL,
  status CHECK IN('draft','pending_approval','approved','rejected','sent','partially_received','received','closed','cancelled'),
  total_idr bigint, approval_id NULL, pdf_object_key NULL, sent_at NULL, closed_reason NULL, idempotency_key, UNIQUE(school_id, idempotency_key))
purchase_order_lines(id, school_id, po_id, pr_line_id NULL, item_id NULL, description, qty bigint CHECK(qty>0), unit_price_idr bigint CHECK(>=0),
  amount_idr bigint GENERATED ALWAYS AS (qty*unit_price_idr) STORED, budget_line_key uuid NULL /* diwarisi dari PR line */,
  withholding_type_id uuid NULL, warehouse_id uuid NULL /* wajib jika tracking=stock */)
goods_receipts(id, school_id, number, po_id, received_on date, received_by, note, idempotency_key, UNIQUE(school_id, idempotency_key))
goods_receipt_lines(id, school_id, gr_id, po_line_id, qty_received bigint, qty_accepted bigint, qty_rejected bigint, reject_reason NULL,
  asset_handoff_at timestamptz NULL,   -- diisi setelah hook aset sukses; NULL + tracking=asset = antrean backfill T16
  CHECK(qty_received = qty_accepted + qty_rejected))
goods_returns(id, school_id, gr_line_id, qty bigint CHECK(qty>0), reason, returned_on, created_by)   -- retur setelah diterima, hanya tracking=stock

-- KONTRAK BACA untuk T17 (3-way match & komitmen). Nama kolom tidak boleh diubah tanpa reviewer.
CREATE VIEW po_line_match_v AS  -- per baris PO: dipesan, diterima (accepted − retur), harga
  SELECT pol.school_id, pol.po_id, pol.id AS po_line_id, po.vendor_id, po.status AS po_status, pol.item_id, pol.budget_line_key,
         pol.withholding_type_id, pol.qty AS qty_ordered, pol.unit_price_idr, pol.amount_idr,
         COALESCE(acc.qty,0) - COALESCE(ret.qty,0) AS qty_accepted_net, ...;
CREATE VIEW pr_open_commitment_v AS  -- PR approved/partially_ordered: sisa yang belum menjadi PO
  SELECT prl.school_id, prl.budget_line_key, pr.id AS pr_id, (prl.qty - prl.qty_ordered) * prl.est_unit_price_idr AS amount_idr ...;
```
Status "di bawah minimum" **bukan** kolom: `qty < min_stock`.

## 3. Fungsi murni + vektor (`internal/inventory/calc.go`, `internal/procurement/calc.go`)
| Fungsi | Vektor |
|---|---|
| `ApplyMovement(balance, direction, qty) (after int64, err)` | **AC-102-01** saldo 5, out 8 → `ErrInsufficientStock` "Stok tidak mencukupi (tersedia 5)" ; saldo 5, out 5 → 0 |
| `CrossedBelowMin(before, after, min) bool` | **AC-102-02** min 20, 25 → 15 → true ; 15 → 10 → false (sudah di bawah, tidak spam) ; min 0 → selalu false |
| `CountVariance(systemQty, countedQty) int64` | **AC-103-01** 12 vs 10 → −2 ; belum dihitung → tidak masuk penyesuaian |
| `PRTotal(lines) int64` | qty × est_unit_price, jumlah tanpa float |
| `ApprovalFlags(budgetResult *BudgetCheckResult) ([]string, error)` | hook nil → `[]` ; **AC-104-01** sisa 2.000.000, PR 2.500.000, policy `extra_approval` → `["over_budget"]` ; policy `reject` → `ErrOverBudget` (422 "Melebihi sisa anggaran pos …") |
| `PRStatusAfterOrder(lines) status` | semua `qty_ordered = qty` → `ordered`; sebagian → `partially_ordered` |
| `POReceiptStatus(lines []{ordered, acceptedNet}) (status, remaining map)` | **AC-105-01** 10 dipesan, terima 6 → `partially_received`, sisa 4 ; 10/10 → `received` ; retur 2 dari 10 → kembali `partially_received` sisa 2 |
| `SplitGoodsReceipt(lines, itemTracking) (stockMoves, assetLines, none)` | **AC-105-02** 2 laptop `asset` → 2 unit di `assetLines` (hook dipanggil 1× dengan qty 2), tanpa mutasi stok ; kursi `stock` qty_accepted 6 → mutasi `in` 6, qty_rejected tidak masuk stok |
| `VendorDuplicates(npwpBidx, accountBidx, existing)` | NPWP sama atau rekening sama → daftar kandidat duplikat (FR-106-002) |
| `OnTime(expectedDate, receivedOn) *bool` | expected kosong → nil |
| `FormatDocNumber(format, year, seq)` | `PO-{yyyy}-{seq:4}`, 2026, 12 → `PO-2026-0012` |

## 4. Integrasi & job
**Mutasi stok (BR-INV-01, FR-102-003):** semua mutasi lewat **satu** fungsi `inventory.Move(ctx, tx, MoveInput)` yang `SELECT … FOR UPDATE` baris `stock_balances`, memanggil `ApplyMovement`, menulis `stock_movements` (+`balance_after`) dan audit. Transfer = dua mutasi dengan `transfer_id` sama dalam satu transaksi. Gudang dengan opname `counting` menolak mutasi: 409 "Gudang sedang stock opname". Koreksi = mutasi balik, tidak pernah UPDATE/DELETE.
**Stok minimum (FR-102-005):** jika `CrossedBelowMin` → `Notify` ke semua user `inventory_officer` sekolah itu, `Idempotency-Key: stockmin:<movement_id>`.
**Opname (FR-103-001..004):** mulai → salin `stock_balances` ke `stock_count_lines.system_qty`. Simpan hasil → `submitted` (stok tidak berubah). Ajukan → core `stock_adjustment` (`amount_idr` null, summary jumlah item selisih). Callback approved → mutasi `adjustment` per baris (delta = counted − system) dalam satu transaksi; jika ada yang membuat saldo negatif → rollback, status `adjust_failed` + pesan, tidak sebagian. Berita acara PDF disimpan ke storage setelah `adjusted` (juga bisa dicetak saat `submitted` sebagai draf).
**Approval (BR-PRC-01):** request type baru `purchase_request`, `purchase_order`, `stock_adjustment`, `vendor_bank_change`. PR/PO mengirim `amount_idr` = total, sehingga tahap per nilai memakai `min_amount_idr` flow core; "per unit" = flow per `school_id`. Maker-checker ditegakkan core. Callback ops: `POST /internal/approval-callbacks` (dari kerangka T9; tambah `case` per request type), idempoten per `approval_id`.

**Hook nil-safe ke task lain** (`internal/procurement/hooks.go`, didaftarkan di `cmd/server/main.go`):
```go
// Didaftarkan T17: procurement.BudgetCheck = budget.Check
type BudgetCheckInput struct{ SchoolID string; Lines []BudgetCheckLine; ExcludeSource string /* "pr:<id>" atau "po:<id>" */ }
type BudgetCheckLine struct{ BudgetLineKey string; AmountIDR int64 }
type BudgetCheckResult struct {
    Policy string                 // "extra_approval" | "reject"
    Lines  []BudgetLineResult     // {BudgetLineKey, BudgetIDR, ActualIDR, CommitmentIDR, RemainingIDR, RequestIDR, Over bool}
    Over   bool; AsOf time.Time
}
var BudgetCheck func(ctx context.Context, q pgx.Tx, in BudgetCheckInput) (BudgetCheckResult, error)

// Didaftarkan T16: procurement.OnGoodsReceived = asset.OnGoodsReceived
type GoodsReceivedLine struct{ GRLineID, POLineID, POID, PONumber, VendorID, ItemID, ItemName, ItemCategoryID string
    QtyAccepted, UnitPriceIDR int64; ReceivedOn time.Time }
type GoodsReceivedEvent struct{ SchoolID, GoodsReceiptID, GRNumber string; Lines []GoodsReceivedLine } // hanya baris tracking=asset
var OnGoodsReceived func(ctx context.Context, tx pgx.Tx, ev GoodsReceivedEvent) error
```
- `BudgetCheck` dipanggil saat **submit PR** dan **submit PO** (PO: `ExcludeSource` = PR asalnya supaya komitmen tidak dihitung dua kali), hanya untuk baris dengan `budget_line_key`. Nil → tanpa cek, `budget_check` NULL. Hasil disimpan di `purchase_requests.budget_check` dan dikirim di `summary` approval (FR-104-002). `Over` + policy `extra_approval` → flag `over_budget` (flow core memakai `condition_flag='over_budget'`); `reject` → 422 dan PR tetap `draft`.
- `OnGoodsReceived` dipanggil **di dalam transaksi** penerimaan untuk baris aset. Sukses → `asset_handoff_at = now()`. Error → rollback seluruh penerimaan. Nil → baris tetap `asset_handoff_at IS NULL` (T16 punya backfill). Retur setelah diterima hanya untuk `stock`; barang aset yang cacat dicatat sebagai `qty_rejected` saat penerimaan.

**Vendor ke Accounting Engine (FR-106-005, IF-OPS-01):** setiap create/ubah vendor atau aktivasi rekening → upsert `vendor_accounting_sync(status='pending')`. Job tiap menit mengirim `PUT {ACCOUNTING_URL}/api/internal/vendors/{vendor_id}` `{school_id, code, name, npwp_masked, status}` (header `X-Internal-Key`, `Idempotency-Key: vendor:<id>:<payload_hash>`; **tidak** mengirim rekening mentah). Endpoint ini dibangun T17 (§4.2 T17). Sampai T17 di-merge accounting membalas 404 → tetap `pending`, log info, **tanpa** exception. Galat lain setelah batas percobaan (config) → `CreateException(kind='dead_letter', retry_url=/internal/vendors/{id}/sync)`. Sekolah tanpa feature `accounting` → tidak dikirim.
**Rekening vendor (FR-106-003, AC-106-01):** rekening baru = baris `pending_approval` + core `vendor_bank_change`; yang `active` tetap dipakai pembayaran (T17 membaca hanya `status='active'`). Approved → baris lama `superseded`, baru `active`. Nilai rekening tidak pernah masuk log/audit mentah (mask 4 digit terakhir).
**Jurnal:** T15 **tidak** memposting jurnal. Utang diakui T17 saat tagihan vendor diposting; aset dikirim T16.

## 5. Endpoint
| Method & path | Peran |
|---|---|
| `CRUD /inventory/item-categories`, `/inventory/items`, `/inventory/warehouses` | inventory_officer (tulis); kepala_sekolah, procurement_officer (baca) |
| `GET /inventory/stock?warehouse_id=&item_id=&below_min=` · `GET /inventory/stock-card?item_id=&warehouse_id=&from=&to=` (FR-102-004, saldo awal + mutasi + saldo akhir) | inventory_officer, kepala_sekolah |
| `POST /inventory/movements` `{kind:'issue'|'receipt', warehouse_id, item_id, qty, ref_no, note}` (Idempotency-Key wajib) · `POST /inventory/transfers` `{from, to, lines}` | inventory_officer |
| `POST /inventory/counts` `{warehouse_id}` · `GET /inventory/counts/{id}` · `PUT /inventory/counts/{id}/lines` · `POST …/submit` · `POST …/request-adjustment` · `GET …/report.pdf` · `POST …/cancel` | inventory_officer; `report.pdf` + baca juga finance_manager |
| `CRUD /procurement/vendors` (+`/contacts`, `/documents` unggah) · `POST /procurement/vendors/check-duplicate` · `POST /procurement/vendors/{id}/bank-accounts` (→ approval) · `GET …/evaluations` | procurement_officer (tulis); finance_admin, finance_manager (baca, rekening ter-mask) |
| `POST /internal/vendors/{id}/sync` | internal key (retry dari exception) |
| `POST /procurement/requests` · `PUT /{id}` (draft) · `POST /{id}/submit` · `POST /{id}/cancel` · `GET /procurement/requests?mine=1` · `GET /{id}` (timeline status FR-104-004) | pemohon: semua pengguna ber-akun staf (guru, pegawai, peran operasional); daftar semua: procurement_officer, kepala_sekolah |
| `POST /procurement/orders` `{vendor_id, pr_line_ids[], lines:[{pr_line_id, qty, unit_price_idr, warehouse_id}], payment_terms_days}` · `PUT /{id}` · `POST /{id}/submit` · `GET /{id}/pdf` · `POST /{id}/mark-sent` · `POST /{id}/close` `{reason}` · `POST /{id}/cancel` | procurement_officer; baca finance_manager, inventory_officer |
| `POST /procurement/orders/{id}/receipts` `{received_on, lines:[{po_line_id, qty_received, qty_accepted, qty_rejected, reject_reason}]}` · `GET /procurement/receipts?po_id=` · `POST /procurement/receipts/lines/{id}/returns` | inventory_officer, procurement_officer |
| `POST /procurement/receipts/{id}/evaluation` | procurement_officer |
| `POST /internal/approval-callbacks` | internal key (core) |
Validasi lain: qty diterima kumulatif ≤ qty dipesan − diterima (422 "Melebihi sisa PO"); PO hanya dari baris PR `approved`/`partially_ordered`; vendor `blocked/inactive` tidak bisa dipilih; PO `sent` tidak bisa diubah.
Kirim PO ke vendor (FR-105-002): PDF dibuat saat PO `approved`; "Kirim" = unduh PDF + `mark-sent`. Notification hanya mendukung `user_id` (tidak ada email eksternal): catat di handoff, **jangan** mengubah service notification.

## 6. Frontend
`services/inventory/*.ts`, `services/procurement/*.ts`, `router/domains/ops-inventory.routes.ts` (daftarkan di `router/index.ts`). `meta.feature: 'inventory'` (102–103) / `'procurement'` (104–106), navGroup **Inventaris & Pengadaan**; menu pemohon di navGroup **Pegawai**.
| Path | View (`views/inventory/…`, `views/procurement/…`) | Peran | Label |
|---|---|---|---|
| `inventaris/barang` | `ItemsView.vue` (tab Barang / Kategori (tracking + akun) / Gudang) | inventory_officer | Master Barang |
| `inventaris/stok` · `/kartu/:itemId` | `StockView.vue` (filter gudang, badge teks "Di bawah minimum"), `StockCardView.vue` | inventory_officer, kepala_sekolah | Stok & Kartu Stok |
| `inventaris/mutasi` | `MovementsView.vue` (dialog barang masuk/keluar/transfer, pesan galat stok dari API apa adanya) | inventory_officer | Mutasi Stok |
| `inventaris/opname` · `/:id` | `StockCountsView.vue`, `StockCountView.vue` (lembar hitung, kolom selisih bertanda +/−, tombol Ajukan Penyesuaian, unduh berita acara, `ApprovalTimeline` T1) | inventory_officer, finance_manager | Stock Opname |
| `pengadaan/permintaan` · `/:id` | `PurchaseRequestsView.vue`, `PurchaseRequestFormView.vue` (baris barang/jasa; field **pos anggaran** = input `budget_line_key` tersembunyi sampai T17 menambahkan pemilih; badge "Melebihi anggaran"; stepper status sampai PO) | procurement_officer, kepala_sekolah | Permintaan Pembelian |
| `pegawai/permintaan-pembelian` | memakai ulang `PurchaseRequestsView.vue` dengan `mine` | guru, pegawai, staf | Pegawai / Permintaan Pembelian |
| `pengadaan/po` · `/:id` | `PurchaseOrdersView.vue`, `PurchaseOrderView.vue` (buat dari PR approved, pratinjau PDF, tab Penerimaan dengan sisa per baris, `ApprovalTimeline`) | procurement_officer, finance_manager, inventory_officer | Purchase Order |
| `pengadaan/penerimaan/:poId` | `GoodsReceiptView.vue` (input diterima/diterima baik/ditolak + alasan; info "N calon aset akan dibuat" untuk baris aset) | inventory_officer, procurement_officer | Penerimaan Barang |
| `pengadaan/vendor` · `/:id` | `VendorsView.vue`, `VendorView.vue` (tab Profil / Kontak / Dokumen / Rekening (badge "Menunggu approval") / Evaluasi; dialog peringatan duplikat) | procurement_officer, finance_admin, finance_manager | Vendor |
Komponen `ApprovalTimeline` dari T1: pakai ulang, jangan disalin. Tabel lebar di-scroll di wrapper; angka kuantitas dan rupiah rata kanan `tabular-nums`.

## 7. Bukti handoff
Vektor §3 hijau. Test wajib: **AC-102-01** (API 422 + saldo tetap), **AC-102-02** (notifikasi 1× dengan fake notification), **AC-103-01** (stok tidak berubah sebelum callback approved; berubah −2 setelahnya; callback diulang tidak menggandakan), **AC-104-01** (fake `BudgetCheck`: flag `over_budget` terkirim ke fake core; policy reject → 422; hook nil → tanpa flag), **AC-105-01**, **AC-105-02** (fake `OnGoodsReceived` menerima qty 2; hook error → penerimaan rollback; hook nil → `asset_handoff_at` NULL), **AC-106-01** (rekening lama tetap `active` sampai callback). Plus: trigger tolak UPDATE/DELETE `stock_movements`, CHECK `qty>=0`, mutasi ditolak saat opname, penerimaan melebihi sisa PO → 422, sync vendor 404 → tetap pending tanpa exception, rekening tidak muncul di audit. Cakupan FR: FR-102-001..005, FR-103-001..004, FR-104-001..004 (FR-104-002 lewat fake; uji nyata di T17), FR-105-001..004, FR-106-001..005 (FR-106-005 E2E di T17).
Playwright (setelah merge gelombang): inventory_officer buat barang + gudang → barang masuk → keluar melebihi stok ditolak → guru ajukan PR → kepala_sekolah setujui di Approval Center → procurement_officer buat PO dari PR, setujui, unduh PDF → inventory_officer terima 6 dari 10 (status "Diterima sebagian", sisa 4) → stok naik 6 → opname dengan selisih −2 → setujui → kartu stok menunjukkan mutasi penyesuaian.
