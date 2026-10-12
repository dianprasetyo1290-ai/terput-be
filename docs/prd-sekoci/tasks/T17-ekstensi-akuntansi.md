# T17: Ekstensi akuntansi di service `ops` + Accounting Engine: anggaran (RAPBS) & realisasi, pajak non-gaji & e-Faktur, utang usaha (3-way match)

**Menu PRD:** 109, 110, 111 (Bagian 3 Grup 17; BR-BUD-01/02, BR-PRC-02, BR-INT-01, IF-OPS-01/02/05, OQ-28) · **Branch:** `feat/T17-ekstensi-akuntansi` · **Service:** `ops` port 8093 (paket `internal/budget`, `internal/tax`, `internal/payable`) + `accounting` (Laravel) · **Migrasi:** `services/ops/migrations/0030_budget.*`, `0031_tax.*`, `0032_tax_seed.*`, `0033_payable.*` (nomor 0030–0039 milik T17) + Laravel `services/accounting/database/migrations/2026_10_20_000001_ops_extension.php` (migrasi bertanggal milik T17; tanggal boleh digeser agar urut setelah migrasi terakhir saat branch dibuat) · **Gelombang:** 5
**Prasyarat:** gelombang 4 di-merge: T15 (`purchase_*`, `goods_*`, view `po_line_match_v` & `pr_open_commitment_v`, `vendors`, `vendor_bank_accounts`, `item_categories.purchase_acc_code/withholding_type_id`, `doc_sequences`, hook `procurement.BudgetCheck`), T1 §7b (`POST /api/internal/journals`, `AutoJournalService`). **T16 di gelombang yang sama** memakai endpoint aset Accounting Engine yang dibangun di sini (§4.3) sesuai kontrak T16 §4.2.
**Feature key (usulan, final mengikuti T9 §2):** `budget` (109), `tax_nonpayroll` (110), `payable` (111); ketiganya juga mensyaratkan feature `accounting` aktif (accounting membalas 409 bila tidak). **Peran (final mengikuti T9 §2):** `finance_manager`, `finance_admin`, `kepala_sekolah` (= Kepala Unit), `kepala_yayasan` (= Pengurus Yayasan, baca lintas unit dalam yayasannya; ikuti pola akses `kepala_yayasan` yang dipakai dashboard yayasan).
Uang `bigint` rupiah, persen dalam basis poin. **Tidak ada float**, juga di sisi PHP (hitung dalam integer, simpan ke kolom numeric accounting).

## 1. Donor / acuan kode
| Kebutuhan | Tiru dari |
|---|---|
| Posting jurnal idempoten | `services/accounting/app/Services/AutoJournalService.php` + `tests/Feature/AutoJournalTest.php`; client Go `PostJournal` dari `services/billing/internal/client/integrations.go` (disalin) |
| Migrasi Laravel per tenant | `2026_10_08_000001_journal_workflow_coa_fields_auto_posting.php` (raw SQL `IF NOT EXISTS`, tanpa schema qualifier, aman di-rerun). **Dilarang** seeder yang `truncate` (lihat `services/accounting/CLAUDE.md`) |
| Tenant | `App\Support\Tenancy::activate($schoolId)` + `SsoController::accountingEnabled` (pola di AutoJournalService) |
| Parameter pajak effective-dated + seed bertanda | T3 §2 `statutory_*` + `0011_statutory_seed` ("PERLU VALIDASI KONSULTAN PAJAK") |
| Template ekspor kolom (format DJP, IF-OPS-05) | T3 `bank_file_templates` / `tax_export_templates` (columns jsonb). Format **tidak** di-hardcode |
| Bucket aging | sama dengan aging piutang billing: `current, d1_30, d31_60, d61_90, d90_plus` (`services/billing/internal/handler/summary.go`) |
| Approval, callback, exception | T15 §1 / T1 §3. Request type baru: `budget`, `vendor_invoice_override`, `vendor_payment` (tambah di `services/core/internal/model/domain.go` bila belum didaftarkan T9) |
| Data rekening vendor | hanya baris `vendor_bank_accounts.status='active'` (T15, AC-106-01) |

## 2. Migrasi
`0030_budget.up.sql`
```
budget_settings(school_id PK, over_budget_policy CHECK IN('extra_approval','reject') NOT NULL DEFAULT 'extra_approval',   -- BR-BUD-02 "sesuai pengaturan klien"
  absorption_warn_bp int NOT NULL DEFAULT 8000)                                                                         -- FR-109-004 [80%]
budgets(id, school_id, name, period_start date, period_end date, version int, status CHECK IN('draft','pending_approval','active','superseded','rejected'),
  revision_of uuid NULL, approval_id NULL, note, created_by, UNIQUE(school_id, period_start, version))
CREATE UNIQUE INDEX one_active_budget ON budgets(school_id, period_start) WHERE status='active'
budget_lines(id, school_id, budget_id, line_key uuid NOT NULL /* stabil lintas versi; KONTRAK T15 budget_line_key */,
  name, acc_code text NOT NULL, div_code text NULL, amount_idr bigint CHECK(>=0), UNIQUE(budget_id, line_key))
budget_actuals_cache(school_id, line_key, period_start, actual_idr bigint, as_of timestamptz, PRIMARY KEY(school_id, line_key, period_start))   -- IF-OPS-02
budget_alerts(school_id, line_key, budget_id, sent_at, PRIMARY KEY(line_key, budget_id))                     -- peringatan serapan 1× per versi
```
`0031_tax.up.sql`
```
tax_settings(school_id PK, is_pkp bool NOT NULL DEFAULT false /* OQ-28 default mati */, withholder_npwp_enc bytea NULL, withholder_name)
withholding_tax_types(id, school_id NULL /* NULL = default nasional */, code, name, article CHECK IN('pph23','pph4_2'), payable_acc_code text NULL, deleted_at)
withholding_tax_rates(id, school_id NULL, type_id, rate_bp int CHECK(>=0), no_npwp_rate_bp int NULL, effective_from date, effective_to date NULL, source_note text NOT NULL)
ALTER TABLE item_categories ADD CONSTRAINT fk_wht FOREIGN KEY (withholding_type_id) REFERENCES withholding_tax_types(id);
withholding_slips(id, school_id, number, invoice_id, vendor_id, type_id, period char(7), dpp_idr, rate_bp, amount_idr, pdf_object_key NULL, created_at)   -- bukti potong
input_vat(id, school_id, invoice_id UNIQUE, faktur_no, faktur_date, dpp_idr, vat_idr, creditable bool, period char(7))       -- PPN masukan
output_tax_invoices(id, school_id, faktur_no, faktur_date, buyer_name, buyer_npwp_enc NULL, dpp_idr, vat_idr, period char(7), source_ref NULL, created_by)  -- hanya bila is_pkp
tax_export_templates(id, school_id, kind CHECK IN('bupot_pph23','bupot_pph4_2','faktur_masukan','faktur_keluaran'), name, delimiter, header_row bool, columns jsonb NOT NULL)
```
`0032_tax_seed.up.sql`: jenis `pph23`/`pph4_2` + tarif nasional (`school_id NULL`) dari regulasi resmi, `source_note` berisi dasar hukum. Komentar di atasnya `-- PERLU VALIDASI KONSULTAN PAJAK (OQ-28)`. Ragu satu angka → **jangan isi**, catat di handoff; pencatatan tagihan dengan jenis itu gagal 422 "tarif <jenis> belum dikonfigurasi". Template ekspor DJP **tidak** di-seed (dikonfigurasi pengguna).
`0033_payable.up.sql`
```
payable_settings(school_id PK, match_tolerance_bp int NOT NULL DEFAULT 200 /* BR-PRC-02 [2%] */, ap_acc_code text NULL, vat_in_acc_code text NULL, default_bank_acc_code text NULL)
vendor_invoices(id, school_id, vendor_id, po_id NOT NULL, vendor_invoice_no, invoice_date, due_date,
  subtotal_idr bigint, vat_idr bigint DEFAULT 0, withholding_idr bigint DEFAULT 0, payable_idr bigint /* subtotal + vat − withholding */,
  faktur_no NULL, faktur_date NULL, match_result jsonb, override_reason NULL,
  status CHECK IN('draft','mismatch','matched','override_pending','matched_override','posted','post_failed','partially_paid','paid','cancelled'),
  approval_id NULL, journal_reference text UNIQUE NULL /* payable:inv:<id> */, trans_no NULL, idempotency_key, UNIQUE(school_id, idempotency_key),
  UNIQUE(school_id, vendor_id, vendor_invoice_no))                       -- tagihan ganda dari vendor yang sama ditolak
vendor_invoice_lines(id, school_id, invoice_id, po_line_id, qty bigint CHECK(>0), unit_price_idr, amount_idr /* DPP */,
  debit_acc_code text NOT NULL /* snapshot dari item_categories.purchase_acc_code */, budget_line_key uuid NULL,
  withholding_type_id NULL, withholding_rate_bp int NULL, withholding_idr bigint DEFAULT 0)
payment_schedules(id, school_id, invoice_id, due_date, amount_idr, status CHECK IN('planned','pending_approval','approved','rejected','paid','cancelled'),
  approval_id NULL, bank_account_id uuid NULL /* snapshot vendor_bank_accounts aktif saat diajukan */, paid_on NULL, bank_acc_code NULL,
  journal_reference text UNIQUE NULL /* payable:pay:<id> */, trans_no NULL)
payable_journals(id, school_id, reference text UNIQUE, kind CHECK IN('invoice','payment'), status CHECK IN('pending','posted','failed'),
  lines jsonb, trans_no NULL, attempts int default 0, error NULL)          -- outbox, pola payroll_journals T3
```
Laravel `2026_10_20_000001_ops_extension.php` (per tenant, raw SQL `IF NOT EXISTS`):
```
acc_vendors(vendor_id uuid PK, code, name, npwp_masked, status, updated_at)                                     -- FR-106-005 (T15)
ALTER TABLE trans_detail, this_detail ADD COLUMN IF NOT EXISTS vendor_id uuid NULL                              -- utang per vendor; End of Month menyalin verbatim, jadi keduanya
fa_assets(asset_ref uuid PK, code, name, acquired_on date, cost numeric(20,2), residual numeric(20,2), opening_accum numeric(20,2) DEFAULT 0,
  useful_life_months int, dep_start varchar(20), asset_acc_code, accum_dep_acc_code, dep_expense_acc_code,
  status varchar(15) /* active|disposed|transferred */, closed_on date NULL)                                     -- untuk T16
fa_asset_events(reference varchar(100) PK, asset_ref, kind varchar(15), trans_no varchar(15) NULL, payload jsonb, created_at)
fa_depreciation(asset_ref, period char(7), amount numeric(20,2), trans_no varchar(15), PRIMARY KEY(asset_ref, period))
```

## 3. Fungsi murni + vektor
Go (`internal/budget/calc.go`, `internal/tax/calc.go`, `internal/payable/calc.go`):
| Fungsi | Vektor |
|---|---|
| `AbsorptionBP(actual, budget) int` (floor) + `FormatPct1(bp)` | **AC-109-01** 11.000.000 / 24.000.000 → 4583 bp → "45,8%" ; budget 0 → 0 (bukan panic) |
| `Commitment(prOpen, poLines, invoicedPosted) int64` | **AC-109-02** PO approved 3.000.000 belum ditagih → komitmen 3.000.000, realisasi tidak berubah ; setelah tagihan 3.000.000 posted → komitmen 0 |
| `Remaining(budget, actual, commitment) int64` + `CheckLines(...) BudgetCheckResult` | **AC-104-01** budget 10.000.000, aktual 7.000.000, komitmen 1.000.000 → sisa 2.000.000; PR 2.500.000 → `Over=true`; policy dari `budget_settings` |
| `CrossedWarn(prevBP, curBP, warnBP) bool` | 7900 → 8100, warn 8000 → true ; 8100 → 8300 → false |
| `WithholdingAmount(dpp, rateBP) int64` (floor rupiah) | **AC-110-01** 5.000.000 × 200 → 100.000 ; bayar ke vendor 4.900.000 |
| `PickRate(rates, onDate, vendorHasNPWP)` | override sekolah menang atas nasional; tanggal tagihan menentukan baris berlaku; tanpa NPWP & `no_npwp_rate_bp` terisi → pakai itu |
| `ThreeWayMatch(lines []{qtyInvoicedCum, qtyAcceptedNet, poUnitPrice, invAmount}, tolBP) MatchResult` | **AC-111-01** diterima 10.000.000, tagihan 10.500.000, tol 200 → `mismatch` (selisih 500 bp) ; 10.200.000 → `matched` (batas inklusif) ; 10.200.001 → `mismatch` ; qty ditagih > qty diterima → `mismatch` "qty melebihi penerimaan" tanpa toleransi |
| `CanSchedule(invoice)` | status selain `posted`/`partially_paid` → galat "Tagihan tidak cocok…/belum diposting" (AC-111-01 "tidak dapat dijadwalkan bayar") |
| `DueDate(invoiceDate, termsDays)` | termin dari PO (FR-111-002) |
| `BuildInvoiceJournal(inv, lines, settings, isPKP) []JournalLine` | AC-110-01: Dr beban 5.000.000 / Cr utang PPh 23 100.000 / Cr utang usaha 4.900.000 (`vendor_id` di baris utang) ; PPN 550.000 & PKP → Dr PPN masukan ; non-PKP → PPN ditambahkan ke baris beban ; selalu seimbang (test) |
| `BuildPaymentJournal(schedule)` | Dr utang usaha / Cr bank, nominal sama |
| `AgingBucket(dueDate, today)` | jatuh tempo besok → `current` ; lewat 45 hari → `d31_60` |
PHP (`app/Services/DepreciationCalculator.php`, fungsi murni + PHPUnit):
| Fungsi | Vektor |
|---|---|
| `monthly(cost, residual, life, index)` garis lurus, integer, sisa pembagian di bulan terakhir | 12.000.000 / 48 → 250.000 ; 10.000.000 / 3 → 3.333.333, 3.333.333, 3.333.334 ; aset `disposed`/habis masa → 0 |
| `firstPeriod(acquired_on, dep_start)` | 2026-10-15, `next_month` → 2026-11 ; `acquisition_month` → 2026-10 |
| `disposalLines(cost, accum, proceeds)` | laba → Cr akun laba ; rugi → Dr akun rugi ; seimbang |
Metode hanya garis lurus dan pelepasan memakai akumulasi yang sudah diposting (tanpa penyusutan parsial): asumsi, tandai "perlu validasi akuntan" di handoff.

## 4. Integrasi & job
### 4.1 Anggaran (BR-BUD-01/02, FR-109-001..004, FR-104-002)
- `budget.Check(ctx, tx, procurement.BudgetCheckInput) (procurement.BudgetCheckResult, error)` dengan signature **persis** T15 §4, didaftarkan di `main.go`: `procurement.BudgetCheck = budget.Check`. Memakai versi `active` yang `period_start..period_end` mencakup hari ini (zona sekolah); aktual dari `budget_actuals_cache`; komitmen = `pr_open_commitment_v` + Σ`po_line_match_v.amount_idr` (PO `approved..received`, tidak `cancelled`; PO `closed` hanya bagian yang diterima) − Σ baris tagihan `posted` per `po_line_id`; `ExcludeSource` dikeluarkan. `budget_line_key` tidak dikenal/tidak aktif → `ErrUnknownBudgetLine` (T15 memetakan ke 422 "Pos anggaran tidak aktif").
- **Sinkron realisasi (IF-OPS-02):** job berkala (interval di config) + tombol "Perbarui" → `POST {ACCOUNTING_URL}/api/internal/ledger/actuals` untuk semua baris versi aktif; tulis `budget_actuals_cache` + `as_of`. Accounting mati → cache lama tetap dipakai, laporan menampilkan "Diperbarui: <as_of>" (CC-10), job membuat exception setelah batas percobaan. Setelah sinkron: `CrossedWarn` → `Notify` finance_manager + kepala_sekolah, `Idempotency-Key: budwarn:<budget_id>:<line_key>`.
- **Penetapan & revisi (FR-109-002):** `submit` → core `budget` (`amount_idr` = total, summary: versi + selisih per pos dibanding versi aktif). Approved → versi baru `active`, versi lama `superseded` (satu transaksi). Revisi = salin versi aktif ke `draft` baru dengan `line_key` sama.
- **UI pemilih pos anggaran di form PR T15:** tambahkan `components/budget/BudgetLinePicker.vue` (memanggil `GET /budget/lines/options`) ke `PurchaseRequestFormView.vue` milik T15 menggantikan input tersembunyi `budget_line_key`. Hanya tambah komponen, jangan menata ulang view T15.
### 4.2 Utang usaha & pajak (BR-PRC-02, FR-110-*, FR-111-*, IF-OPS-01)
- Catat tagihan → hitung PPh per baris (`PickRate` + `WithholdingAmount`), PPN dari faktur (nilai PPN **diinput dari faktur**, tidak dihitung dari tarif), lalu `ThreeWayMatch` terhadap `po_line_match_v` dengan `match_tolerance_bp`. `matched` → langsung buat jurnal tagihan (reference `payable:inv:<id>`, source `payable`) via outbox `payable_journals`; sukses → `posted`, gagal → `post_failed` + `CreateException(kind='journal_posting', retry_url=/internal/payable/journals/{id}/retry)`.
- `mismatch` (AC-111-01): tidak bisa dijadwalkan. Penyelesaian: (a) koreksi tagihan (revisi vendor) → match ulang; (b) `POST …/override` `{reason}` → core `vendor_invoice_override` (`amount_idr` = selisih) → approved → `matched_override` → jurnal.
- Saat posting: buat `withholding_slips` (nomor dari `doc_sequences` T15, kind `bupot`) + PDF bukti potong (FR-110-002), dan `input_vat` (`creditable = is_pkp`) (FR-110-003). Masa pajak = bulan tanggal posting tagihan (asumsi, validasi konsultan pajak).
- **Jadwal bayar (FR-111-002):** default satu jadwal `due_date = DueDate(invoice_date, po.payment_terms_days)` sebesar `payable_idr`, boleh dipecah (Σ = payable). `submit` → core `vendor_payment` (`amount_idr`) dengan snapshot rekening vendor aktif; rekening `pending_approval` tidak pernah dipakai. Approved → finance_admin mencatat `paid_on` + akun bank → jurnal bayar `payable:pay:<schedule_id>` (Dr utang usaha `vendor_id` / Cr bank). Semua jadwal `paid` → tagihan `paid`.
- Idempotensi (FR-111-003, BR-INT-01): reference unik di `payable_journals` **dan** di accounting (`trans_head(source, reference)`); kirim 2× → 1 `trans_no`.
### 4.3 Accounting Engine (Laravel), dikerjakan T17
Semua di `routes/api.php` dengan middleware `internal.key`, logika di `app/Services/*` (bukan closure panjang), tenant via `Tenancy::activate`, 409 bila `accountingEnabled` false, nominal integer rupiah.
| Endpoint | Untuk | Perilaku |
|---|---|---|
| `AutoJournalService`: tambah prefix `payable`→`APV`, `asset`→`AST`; baris boleh membawa `vendor_id` (harus ada di `acc_vendors`, else 422) | T17, T16 | tanpa mengubah perilaku `payroll`/`payment_gateway` (test lama tetap hijau) |
| `PUT /api/internal/vendors/{vendor_id}` `{school_id, code, name, npwp_masked, status}` | T15 FR-106-005 | upsert `acc_vendors`, 200 |
| `POST /api/internal/ledger/actuals` `{school_id, from, to, items:[{key, acc_code, div_code}]}` → `{as_of, items:[{key, actual_idr}]}` | T17 IF-OPS-02 | hanya jurnal `posted`; akun + turunannya (`parent_code`); tanda mengikuti `normal_balance` (D: debit−kredit, K: kredit−debit); gabung `trans_*` dan `this_*` **tanpa hitung ganda** (pelajari End of Month, uji dengan `HistoryJournalTransactionTest` sebagai acuan) |
| `POST /api/internal/assets` · `POST …/{ref}/transfer` · `POST …/{ref}/dispose` · `POST …/depreciate` · `GET /api/internal/assets` | T16 §4.2 (kontrak mengikat, field persis) | simpan `fa_*`; jurnal lewat `AutoJournalService` source `asset`: perolehan hanya bila `post_journal=true` (Dr aset / Cr `offset_acc_code`); penyusutan satu jurnal per tenant per periode reference `asset:dep:<YYYY-MM>` (Dr beban / Cr akumulasi per pasangan akun), baris per aset di `fa_depreciation`; transfer = dua tenant: asal Dr akumulasi + Dr antar-unit / Cr aset, tujuan Dr aset / Cr akumulasi + Cr antar-unit (akun harus `is_interunit`), `fa_assets` tujuan dengan `opening_accum`; dispose memakai `DepreciationCalculator::disposalLines` |
PHPUnit: seimbang/tidak seimbang, reference ganda → 200 `trans_no` sama, kunci salah → 401, actuals tidak menghitung jurnal draft, depresiasi periode yang sama 2× → satu jurnal, transfer antar tenant tidak bocor schema (aktifkan tenant kembali setelah sisi asal).

## 5. Endpoint (ops)
| Method & path | Peran |
|---|---|
| `GET/PUT /budget/settings` | finance_manager |
| `CRUD /budgets` (+ `/lines`) · `POST /budgets/{id}/submit` · `POST /budgets/{id}/revise` | finance_manager (tulis); kepala_sekolah (usul baris unitnya, draft) |
| `GET /budget/report?budget_id=&until=` (per pos: anggaran, realisasi, komitmen, sisa, serapan bp, badge peringatan, `as_of`) · `POST /budget/actuals/refresh` | finance_manager, kepala_sekolah (unitnya), kepala_yayasan (lintas unit, baca) |
| `GET /budget/lines/options?q=` | semua pemohon PR (dipakai picker T15) |
| `GET/PUT /tax/settings` · `CRUD /tax/withholding-types` (+ `/rates`, baris baru effective-dated, tidak update di tempat) · `CRUD /tax/export-templates` | finance_manager (tulis); finance_admin (baca) |
| `GET /tax/withholding-slips?period=` · `GET /tax/withholding-slips/{id}/pdf` · `GET /tax/export?kind=&period=&template_id=` (IF-OPS-05) · `GET /tax/recap?period=` (per jenis & unit, FR-110-004) · `CRUD /tax/output-invoices` (hanya bila `is_pkp`, else 409) · `GET /tax/input-vat?period=` | finance_admin, finance_manager |
| `GET/PUT /payable/settings` | finance_manager |
| `POST /payable/invoices` · `PUT /{id}` (draft/mismatch) · `POST /{id}/match` · `POST /{id}/override` · `POST /{id}/cancel` (belum posted) · `GET /payable/invoices?status=&vendor_id=` · `GET /{id}` (hasil match per baris) | finance_admin (tulis); finance_manager (baca) |
| `POST /payable/invoices/{id}/schedules` · `POST /payable/schedules/{id}/submit` · `POST /payable/schedules/{id}/pay` `{paid_on, bank_acc_code}` · `GET /payable/schedules?due_before=` | finance_admin; approval oleh peran di flow core (mis. finance_manager) |
| `GET /payable/aging?vendor_id=&as_of=` (per vendor & unit, FR-111-004) · `GET /payable/journals?status=` · `POST /internal/payable/journals/{id}/retry` | finance_admin, finance_manager · retry: internal key |
| `POST /internal/approval-callbacks` (tambah case `budget`, `vendor_invoice_override`, `vendor_payment`) | internal key |

## 6. Frontend
`services/budget/*.ts`, `services/tax/*.ts`, `services/payable/*.ts`, `router/domains/ops-finance.routes.ts`, navGroup **Keuangan Lanjutan**, `meta.feature` per menu (`budget` / `tax_nonpayroll` / `payable`). Banner "Modul akuntansi tidak aktif" bila accounting 409.
| Path | View (`views/budget/…`, `views/tax/…`, `views/payable/…`) | Peran | Label |
|---|---|---|---|
| `anggaran` · `/:id` | `BudgetsView.vue` (daftar versi + status), `BudgetEditorView.vue` (grid pos × akun COA × divisi, total, ajukan, revisi, `ApprovalTimeline`) | finance_manager, kepala_sekolah | Anggaran (RAPBS) |
| `anggaran/realisasi` | `BudgetReportView.vue` (kolom Anggaran / Realisasi / Komitmen / Sisa / Serapan dengan progress bar + angka + badge teks "Serapan > 80%"; filter s.d. tanggal; "Diperbarui: <waktu>" + tombol Perbarui) | finance_manager, kepala_sekolah, kepala_yayasan | Realisasi Anggaran |
| `pajak/pemotongan` | `WithholdingView.vue` (tab Bukti Potong / Rekap Masa / Ekspor DJP) | finance_admin, finance_manager | Pajak Non-Gaji |
| `pajak/ppn` | `VatView.vue` (tab PPN Masukan / Faktur Keluaran (tampil hanya bila PKP)) | finance_admin, finance_manager | PPN & e-Faktur |
| `pajak/pengaturan` | `TaxSettingsView.vue` (status PKP, jenis & tarif dengan timeline tanggal berlaku, banner "perlu validasi konsultan pajak" bila `source_note` kosong, template ekspor) | finance_manager | Pengaturan Pajak |
| `utang/tagihan` · `/:id` | `VendorInvoicesView.vue`, `VendorInvoiceView.vue` (pilih PO → baris terisi dari penerimaan; panel hasil 3-way match per baris dengan badge "Cocok"/"Tidak cocok" + alasan; PPh & PPN; tombol Ajukan Penyelesaian) | finance_admin, finance_manager | Tagihan Vendor |
| `utang/jadwal` | `PaymentSchedulesView.vue` (kalender/daftar jatuh tempo, ajukan bayar, catat pembayaran, `ApprovalTimeline`) | finance_admin, finance_manager | Jadwal Pembayaran |
| `utang/aging` | `ApAgingView.vue` (bucket per vendor & unit, "Diperbarui: <waktu>") | finance_admin, finance_manager | Aging Utang |
| `utang/jurnal` | `PayableJournalsView.vue` (status posting, coba lagi) | finance_admin, finance_manager | Jurnal Utang |
Rupiah via formatter bersama, rata kanan `tabular-nums`; aksi posting/pembayaran dengan dialog konfirmasi.

## 7. Bukti handoff
Vektor §3 (Go + PHP) hijau. Test wajib: **AC-109-01** (fake accounting actuals 11.000.000 → laporan 45,8%), **AC-109-02** (PO approved T15 → komitmen 3.000.000, realisasi 0), **AC-110-01** (tagihan jasa → PPh 100.000, payable 4.900.000, jurnal seimbang, bukti potong terbit), **AC-111-01** (10.500.000 → `mismatch`, jadwal bayar 409; override disetujui → bisa dijadwalkan), AC-104-01 nyata lewat `budget.Check` terdaftar (flag `over_budget` sampai fake core; policy `reject` → 422), jurnal tagihan & bayar idempoten (2× kirim → 1 `trans_no`), rekening vendor pending tidak dipakai, PKP mati → endpoint faktur keluaran 409, tarif kosong → 422, PHPUnit §4.3, sync vendor T15 sukses ke Laravel (FR-106-005 ujung ke ujung). Cakupan FR: FR-109-001..004, FR-110-001..004, FR-111-001..004; FR-104-002 (dengan T15); endpoint aset untuk FR-107-004/FR-108-002..004 (dengan T16). Catat di handoff: daftar tarif yang di-seed + sumbernya + yang dikosongkan, asumsi masa pajak, metode penyusutan.
Playwright: finance_manager susun anggaran pos ATK → disetujui → guru ajukan PR melebihi sisa → badge "Melebihi anggaran" + tahap approval tambahan → PO + penerimaan (T15) → realisasi menampilkan komitmen → finance_admin catat tagihan melebihi toleransi (Tidak cocok, jadwal ditolak) → koreksi nominal → Cocok → jurnal posted → jadwal bayar disetujui → dicatat bayar → aging kosong untuk vendor itu → rekap pajak masa menampilkan PPh yang dipotong.
