# T12: Kantin, POS & dompet siswa di service `billing`: tenant, POS (luring 30 menit), uang saku & tabungan, laporan & bagi hasil + kontrak bersama tagihan ad-hoc (BILL-ADHOC) & kartu siswa

**Menu PRD:** 123, 124, 125, 126 (BR-WAL-01/02, NFR-OPS-01/02/04, IF-OPS-01/03/04, OQ-29/30) · **Branch:** `feat/T12-kantin-dompet` · **Service:** `billing` (paket `internal/canteen`, `internal/wallet`, `internal/adhoc`) · **Migrasi:** `services/billing/migrations/0005_canteen_wallet.*` dan `0007_adhoc_bills.*` (0005 + 0007 milik T12; 0006 milik T14). **Tanpa** migrasi Laravel (alasan di §4.5).
**Prasyarat:** T6 (sudah di `integrasi/prd-v2`), T1 (approval, exception, jurnal). **Paralel** dengan T9 (peran + feature key) dan T14 (ekstensi gateway, service sama): kontrak di §4.1 dan pembagian file di §4.6 **mengikat**. **Driver DB:** `database/sql` seperti billing.
**Feature key (final, T9 §2):** `canteen` (menu 123, 124, 126) dan `wallet` (menu 125; pembayaran dompet di POS butuh keduanya, kalau `wallet` mati POS hanya tunai). BILL-ADHOC dan kartu siswa **tidak** digerbang fitur (infrastruktur bersama). **Peran (final, T9 §2):** `canteen_admin`, `canteen_operator` (baru, dibuat T9), plus yang sudah ada: `admin`, `finance_manager`, `orang_tua`, `siswa`. Konstanta peran hanya di satu file (§4.6).
**Kepemilikan tambahan (keputusan orkestrator):** (a) kontrak tagihan ad-hoc **BILL-ADHOC** (§2b, §4.7), dipakai T18 (denda), T19 (transport), T21 (event/ekskul) di gelombang 4; (b) komponen pemindai bersama `web/src/components/ui/ScanInput.vue` dan endpoint `POST /internal/student-cards/resolve` (§4.8), dipakai ulang perpustakaan, transport, dan absensi.

## 1. Acuan kode (tanpa donor eksternal)
| Kebutuhan | Tiru / pakai dari |
|---|---|
| Uang, idempotensi, alokasi, kuitansi, nomor tanpa celah | `services/billing/migrations/0003_gateway.up.sql` (`receipt_sequences`, `UNIQUE(school_id, idempotency_key)`), `internal/handler/receipts.go` (`atomicNextReceiptNo`, `generateReceiptDocument`) dipakai apa adanya |
| Jurnal seimbang | `internal/model/gateway.go` (`JournalLine`, `ValidateJournal`) dipakai ulang; `client.PostJournal` (`internal/client/integrations.go`) |
| Outbox jurnal + penanda job harian | tabel `gateway_journal_postings` dan `gateway_job_runs` (0003) **dipakai ulang**, bukan tabel baru. Saat ini belum ada worker yang membaca outbox: T12 yang membuatnya (§4.4) |
| Job berkala | salin **verbatim** `services/hr/internal/jobs/runner.go` (+ test `hashJobName`) ke `services/billing/internal/jobs/`. T14 menyalin file yang sama; saat merge pilih salah satu (identik) |
| Refund menu 25 | `bill_refunds` (0002 + 0004: `student_bill_id` boleh NULL, `idempotency_key`), `InternalStudentRefund`/`InternalStudentExit` di `internal/handler/transfer_billing.go` |
| Cek wali–anak, fitur, approval, notifikasi | `Identity.GuardianHasAccess`, `client.FeatureEnabled`, `client.RequestApproval`, `client.Notify`, `client.CreateException` |
| Storage bukti | `internal/storage/local.go` |
| QR | npm `qrcode` (sudah ada di `web/package.json`). Pemindai kamera: `BarcodeDetector` bawaan browser; keyboard wedge = input teks biasa. Dependensi pemindai baru hanya jika wajib untuk iOS: versi tetap + alasan di handoff |

## 2. Migrasi `0005_canteen_wallet.up.sql`
```
-- Tenant & skema (FR-123-001/002)
canteen_tenants(id, school_id, code text, name, location_label text, room_id uuid NULL /* master ruang T9, BR-FAC-02 */,
  owner_name, owner_phone, bank_name, bank_account_no, bank_account_name,          -- sensitif: dimasking di audit (MaskSensitive T1)
  status CHECK IN('active','inactive'), is_open bool default false, open_changed_at NULL, deleted_at, UNIQUE(school_id, code))
canteen_tenant_schemes(id, school_id, tenant_id, kind CHECK IN('revenue_share','rent'),
  share_bp int NULL CHECK(share_bp BETWEEN 0 AND 10000),          -- bagian sekolah
  share_base CHECK IN('wallet','all') NULL,                        -- dasar bagi hasil, WAJIB diisi admin (tanpa default)
  rent_idr bigint NULL CHECK(rent_idr>=0),                         -- sewa per bulan kalender
  effective_from date NOT NULL, effective_to date NULL, created_by,
  CHECK((kind='revenue_share') = (share_bp IS NOT NULL AND share_base IS NOT NULL)), CHECK((kind='rent') = (rent_idr IS NOT NULL)),
  EXCLUDE/trigger: tidak boleh tumpang tindih periode per tenant)   -- riwayat baris baru, bukan UPDATE (BR-HR-02)
canteen_tenant_operators(tenant_id, user_id, school_id, active bool, created_at, PRIMARY KEY(tenant_id, user_id))   -- FR-123-003
-- Menu & POS (FR-124-*)
canteen_menu_items(id, school_id, tenant_id, name, category, price_idr bigint CHECK(price_idr>0), active bool, deleted_at)
canteen_daily_stock(item_id, stock_date date, qty_start int, qty_sold int default 0, PRIMARY KEY(item_id, stock_date))  -- opsional per item
canteen_sale_sequences(tenant_id, sale_date date, last int, PRIMARY KEY(tenant_id, sale_date))
canteen_sales(id, school_id, tenant_id, sale_no text, pay_method CHECK IN('wallet','cash'), total_idr bigint CHECK(total_idr>0),
  student_user_id NULL, wallet_movement_id NULL, operator_user_id, device_id text, client_txn_id uuid NOT NULL,
  recorded_at timestamptz /* jam di perangkat */, synced_at timestamptz, offline bool, late_sync bool default false,
  status CHECK IN('completed','voided'), void_reason NULL, voided_by NULL, voided_at NULL, settlement_id NULL,
  UNIQUE(school_id, tenant_id, client_txn_id),                    -- FR-124-005: sinkron ulang tidak menggandakan
  UNIQUE(tenant_id, sale_no), CHECK((pay_method='wallet') = (student_user_id IS NOT NULL)), CHECK(pay_method='wallet' OR NOT offline))
canteen_sale_items(sale_id, item_id, name_snapshot, qty int CHECK(qty>0), unit_price_idr bigint, line_total_idr bigint)
canteen_settlements(id, school_id, tenant_id, period_from date, period_to date, scheme_id,
  wallet_sales_idr, cash_sales_idr, school_share_idr, tenant_payout_idr bigint /* boleh negatif = tenant berutang */,
  status CHECK IN('draft','pending_approval','approved','rejected','paid'), approval_id NULL, paid_at NULL, paid_ref NULL,
  created_by, UNIQUE(tenant_id, period_from, period_to) WHERE status <> 'rejected')
-- Dompet (FR-125-*)
wallet_settings(school_id PK, max_allowance_balance_idr NULL, max_savings_balance_idr NULL,   -- OQ-29: NULL = tanpa batas
  pos_offline_max_minutes int NOT NULL DEFAULT 30)                                                -- NFR-OPS-02 [30]
wallets(id, school_id, student_user_id, kind CHECK IN('allowance','savings'), balance_idr bigint NOT NULL default 0 CHECK(balance_idr>=0),
  daily_limit_idr bigint NULL CHECK(daily_limit_idr>0) /* hanya allowance, diatur ortu; NULL = belum ada batas */,
  status CHECK IN('active','closing','closed'), UNIQUE(school_id, student_user_id, kind), CHECK(kind='allowance' OR daily_limit_idr IS NULL))
wallet_movements(id, school_id, wallet_id, kind CHECK IN('topup_gateway','topup_cashier','pos_sale','pos_void','savings_deposit',
  'savings_withdrawal','transfer_in','transfer_out','refund_exit'), amount_idr bigint CHECK(amount_idr<>0) /* + masuk, - keluar */,
  balance_after_idr bigint CHECK(balance_after_idr>=0), ref_type text, ref_id uuid, created_by, idempotency_key, created_at,
  UNIQUE(school_id, idempotency_key))                                                            -- append-only, tidak ada UPDATE/DELETE
student_cards(id, school_id, student_user_id, token_hash text UNIQUE, status CHECK IN('active','revoked'), issued_by, revoked_at NULL,
  UNIQUE(school_id, student_user_id) WHERE status='active')   -- OQ-30 QR statis; kartu siswa BERSAMA (POS, perpustakaan, transport), bukan khusus dompet
wallet_topups(id, school_id, student_user_id, wallet_kind, amount_idr bigint CHECK(amount_idr>0), channel CHECK IN('gateway','cashier'),
  gateway_transaction_id NULL, receipt_id NULL, status CHECK IN('pending','success','failed','expired'), requested_by, idempotency_key,
  UNIQUE(school_id, idempotency_key))
savings_requests(id, school_id, wallet_id, kind CHECK IN('deposit','withdrawal'), amount_idr, proof_object_key NULL, requested_by, requested_role,
  status CHECK IN('pending_parent','approved','rejected','completed'), parent_user_id NULL, parent_decided_at NULL, completed_by NULL, completed_at NULL)
wallet_accounts(school_id PK, allowance_liability_acc, savings_liability_acc, cash_acc, tenant_payable_acc,
  canteen_revenue_acc, rent_revenue_acc NULL, tenant_receivable_acc NULL)    -- tanpa default: kode akun milik COA sekolah
wallet_reconciliations(id, school_id, run_on date, wallet_kind, wallet_total_idr, ledger_idr, diff_idr, status CHECK IN('matched','mismatch'),
  UNIQUE(school_id, run_on, wallet_kind))
-- Perluasan tabel lama (aditif)
ALTER bill_refunds ADD wallet_id uuid NULL REFERENCES wallets(id)          -- refund saldo dompet lewat menu 25
ALTER receipts: CHECK source IN('gateway','cashier','wallet')               -- kuitansi top-up kasir
ALTER gateway_journal_postings ADD source text NOT NULL DEFAULT 'gateway'  -- outbox dipakai bersama
```
### 2b. Migrasi `0007_adhoc_bills.up.sql` (BILL-ADHOC)
```
ALTER student_bills ALTER scheme_id DROP NOT NULL,
  ADD source_module text NULL CHECK(source_module IN('library','transport','event','extracurricular')),   -- daftar ditambah lewat migrasi pemilik kontrak
  ADD source_ref text NULL,
  ADD CHECK((scheme_id IS NOT NULL) <> (source_module IS NOT NULL)),           -- tagihan skema XOR tagihan ad-hoc
  ADD CHECK((source_module IS NULL) = (source_ref IS NULL))
CREATE UNIQUE INDEX student_bills_source_uq ON student_bills(school_id, source_module, source_ref) WHERE source_module IS NOT NULL
bill_adjustments(id, school_id, bill_id, kind CHECK IN('prorate','adjust'), delta_idr bigint CHECK(delta_idr<>0), reason, source_ref_version text,
  idempotency_key, created_at, UNIQUE(school_id, idempotency_key))            -- riwayat append-only, net_idr dihitung ulang
```
`period_year/period_month` tagihan ad-hoc diisi dari `due_date` (Asia/Jakarta). UNIQUE lama `(school_id, student_user_id, period_year, period_month, scheme_id)` tidak berlaku untuk baris `scheme_id NULL` (perilaku NULL Postgres), jadi tagihan skema tetap terlindungi. `.down.sql`: tolak (RAISE) jika sudah ada baris ad-hoc, supaya tidak ada data yang hilang diam-diam; tanpa baris ad-hoc → kembalikan NOT NULL.
**Audit query lama:** semua query billing yang `JOIN billing_schemes` ke `student_bills` (daftar tagihan admin/ortu/siswa, aging, tunggakan, ringkasan) diubah menjadi `LEFT JOIN` dengan label `COALESCE(scheme.name, items)`. T12 mengubah file milik non-gateway (`bills.go`, `parent.go`, `summary.go`, `admin.go`, `internal.go`, `discounts.go`, `refunds.go`, `transfer_billing.go`: setiap pemindaian `scheme_id` memakai `sql.NullString`). Query di `gateway_*.go` milik T14: lihat T14 §4.1 K3.

Indeks: `canteen_sales(school_id, tenant_id, recorded_at)`, `wallet_movements(wallet_id, created_at)` (dipakai hitung belanja hari ini, NFR-OPS-01). `.down.sql` membalik semua ALTER tanpa menghapus data tabel lama.

## 3. Fungsi murni + vektor (`internal/wallet`, `internal/canteen`, tes tabel)
| Fungsi | Vektor → AC/FR |
|---|---|
| `wallet.CanSpend(balance, dailyLimit *int64, spentToday, amount) error` | **AC-124-01** limit 25.000, terpakai 20.000, beli 10.000 → `ErrDailyLimit` ("Melebihi batas jajan harian"). Saldo 5.000 beli 10.000 → `ErrInsufficient`. Limit NULL → hanya cek saldo (BR-WAL-01, FR-124-003) |
| `wallet.SpentToday(movements, now, loc)` | hanya `pos_sale` dikurangi `pos_void` di tanggal Asia/Jakarta yang sama; transaksi 23.59 vs 00.01 beda hari |
| `wallet.CanTopup(balance, amount, max *int64)` | max NULL → boleh; melewati max → tolak (OQ-29) |
| `canteen.OfflineAccept(recordedAt, now, maxMinutes)` | ≤ 30 menit → `offline`; > 30 → tetap diterima, `late_sync=true` (NFR-OPS-02) |
| `canteen.MergeSync(existing, batch)` | batch berisi `client_txn_id` yang sudah ada → tidak ditambah, dikembalikan sebagai `duplicate` (FR-124-005) |
| `canteen.CanVoid(sale)` | sudah `voided` / sudah punya `settlement_id` → tolak; alasan kosong → 422 (FR-124-004) |
| `canteen.Settle(scheme, walletSales, cashSales, periodFrom, periodTo)` | **AC-126-01** wallet 10.000.000, `share_bp` 1000, base `wallet` → sekolah 1.000.000, tenant 9.000.000. Pembulatan: bagian sekolah **dibulatkan ke bawah** ke rupiah (satu fungsi, dicatat di handoff). `share_base='all'` dengan tunai besar → payout negatif. `rent` → periode wajib satu bulan kalender, sekolah = `rent_idr`. Skema berganti di tengah periode → error "pecah periode" (FR-126-002) |
| `canteen.SalesSummary(sales, groupBy)` | per tenant / item / hari / metode, `voided` dikecualikan (FR-126-001) |
| `wallet.DailyJournal(agg, acc, gatewayAcc)` | per kind: top-up gateway (Dr clearing = total−fee, Dr beban fee bila ditanggung sekolah, Cr titipan), top-up kasir & setoran (Dr kas, Cr titipan), POS wallet (Dr titipan uang saku, Cr utang tenant), void (kebalikan), penarikan & refund (Dr titipan, Cr kas), transfer antar-kind (Dr titipan asal, Cr titipan tujuan; kind sama → tanpa baris). Lolos `ValidateJournal` (FR-125-006, IF-OPS-01) |
| `canteen.SettlementJournals(s, acc)` | approval: Dr utang tenant min(share, walletSales) + Dr piutang tenant (selisih), Cr pendapatan bagi hasil/sewa. Paid: Dr utang tenant, Cr kas = payout (payout ≤ 0 → tanpa jurnal paid) (FR-126-003) |
| `wallet.Reconcile(walletTotal, ledger)` | sama → `matched`; beda 1 rupiah → `mismatch` + diff (NFR-OPS-04) |
| `wallet.ExitDisposition(wallets)` | **AC-125-02** tabungan 350.000 → satu refund 350.000; saldo 0 → tidak ada refund (FR-125-005) |
| `adhoc.Adjust(bill, paid, newNet)` | net 300.000 dibayar 300.000, prorata jadi 200.000 → net 200.000, kelebihan 100.000 → `student_deposits` (BR-FIN-04 T6). Net baru < 0 → error. Net baru > dibayar → status `partial`. Tagihan `void` → error |
| `adhoc.ValidateCreate(req)` | item kosong / `amount_idr ≤ 0` / `source_module` tidak dikenal → 422. Total = Σ item |

## 4. Integrasi & job
### 4.1 Kontrak dengan T14 (gelombang sama, service sama). Disalin juga di T14 §4.1
- **Data:** T14 (0006) menambah `gateway_transactions.purpose` (`'bills'` | `'wallet_topup'`, default `'bills'`) dan `purpose_ref uuid`. T12 **tidak** menyentuh tabel/handler `gateway_*`.
- **K1 T12 → T14 (buat charge top-up):** T12 mendeklarasikan di `handler/wallet_topup.go` interface `purposeCharger { CreatePurposeCharge(ctx, schoolID, payerUserID, studentUserID, methodID, purpose, purposeRef, idempotencyKey string, amountIDR int64) (gatewayTxID string, err error) }` dan field `App.PurposeCharger` (nil-safe → `503 "Top-up online belum tersedia"`). T14 mengimplementasikannya (lewat router provider, fee `model.Totals`, `bills_amount_idr` = nominal top-up, tanpa baris `gateway_transaction_bills`). Parameter primitif supaya kedua branch kompilasi sendiri.
- **K2 T14 → T12 (hasil pembayaran):** T12 menyediakan `func (a *App) WalletTopupHook() walletTopupHook` dengan method `OnGatewaySuccess(ctx, tx *sql.Tx, schoolID, purposeRef, gatewayTxID string) error` dan `OnGatewayClosed(ctx, tx *sql.Tx, schoolID, purposeRef, gatewayTxID, status string) error`. Dipanggil webhook T14 **di dalam transaksi DB yang sama** setelah `FOR UPDATE` dan sebelum commit. Success: kredit dompet idempoten (`idempotency_key = wallet-topup:<topup_id>`), `wallet_topups.status='success'`, notifikasi ortu (`Idempotency-Key: wallet-topup:<id>`) **setelah** commit (**AC-125-01**). Closed (`failed`/`expired`/`cancelled`): status top-up mengikuti.
- **Jurnal:** jurnal harian gateway T6/T14 hanya memuat `purpose='bills'`; jurnal top-up dibuat T12 (§4.4).
- **Wiring:** satu baris di `main.go` (`app.PurposeCharger = …`, `app.PurposeHandlers["wallet_topup"] = app.WalletTopupHook()`) ditambahkan orkestrator saat merge. Sebelum itu masing-masing menguji dengan fake.

### 4.2 Kontrak lain
| Pihak | Kontrak |
|---|---|
| identity (T12 menambah, file baru `identity/internal/handler/internal_role_accounts.go` + 1 baris route) | `POST /internal/role-accounts/create-or-link {school_id, email, full_name, role}`; whitelist `role ∈ {canteen_operator, pembimbing_industri}` (pembimbing_industri dipakai T22 gelombang 4; endpoint ini satu-satunya pembuat akun peran eksternal, sertakan `default_password` sekali di respons bila akun baru); pola `InternalCreateOrLinkEmployee` **tanpa** `managed_by_hr`. Kalau T9 §2 sudah menyediakan endpoint setara, pakai itu dan jangan buat. Tes DB dijalankan setelah rebase ke T9 (CHECK peran milik T9 0017) |
| ops / T9 | validasi `room_id` lewat endpoint baca ruang T9 (lihat T9 §5). `OPS_URL` kosong → simpan tanpa validasi, catat di handoff |
| **T25 (3D peta kantin, menu 156)** | `GET /canteen/directory` (JWT semua peran sekolah, fitur aktif) dan `GET /internal/canteen/directory?school_id=` (X-Internal-Key). Respons `data: [{tenant_id, code, name, location_label, room_id, is_open, open_changed_at, menu: [{item_id, name, category, price_idr, available}]}]`, `meta: {generated_at}`. Hanya tenant `active` dan item `active`. **Tanpa** data pemilik/rekening. Tanpa cache, sehingga harga terbaru langsung tampil (AC-156-01, BR-3D-01) |
| academic (T7) | `GET /internal/students/{id}/financial-summary` diperluas (aditif): `wallet_allowance_idr`, `wallet_savings_idr`. `POST /internal/students/{id}/exit`: setelah kebijakan tagihan, untuk tiap dompet bersaldo → `bill_refunds(wallet_id, student_bill_id NULL, idempotency_key='wallet-exit:<student>:<kind>')`, dompet `closing` (**AC-125-02**). `ProcessRefund` dengan `wallet_id` → movement `refund_exit` (bukan `bill_payment_reversals`), dompet `closed` saat saldo 0 |
| core | approval `canteen_settlement` (maker `canteen_admin`, penyetuju sesuai flow, maker-checker oleh core). Callback `POST /internal/canteen/settlements/{id}/callback` idempoten. Approval `wallet_transfer` untuk pindah saldo ke dompet saudara. Exception `journal_posting` (gagal posting/rekonsiliasi selisih, severity `high`) |
| notification | top-up sukses, penarikan tabungan menunggu persetujuan ortu, saldo di bawah batas tidak dibuat (tidak ada di PRD) |
| **T18 / T19 / T21 (gelombang 4)** | BILL-ADHOC §4.7 dan kartu siswa §4.8. Mereka mengoding sesuai kontrak ini tanpa menyentuh tabel billing |

### 4.3 Aturan proses
- **POS dompet (online saja):** `POST /pos/sales` → resolve `card_token` (hash) → `SELECT … FOR UPDATE` dompet allowance → `CanSpend` → movement `pos_sale` + `canteen_sales` dalam satu transaksi. Target ≤ 2 detik dengan 300 transaksi/15 menit per unit (**NFR-OPS-01**). Tunai: boleh luring.
- **POS luring (FR-124-005, NFR-OPS-02):** antrean IndexedDB per perangkat (`device_id` acak per browser), tiap penjualan punya `client_txn_id` (UUID v4) + `recorded_at`. Sinkron otomatis saat online (`POST /pos/sales/sync`, batch ≤ 100, idempoten lewat UNIQUE). UI menolak penjualan **dompet** saat luring dan menolak penjualan tunai baru setelah luring melebihi `pos_offline_max_minutes`. Akses storage dibungkus try/catch.
- **Top-up (BR-WAL-02, IF-OPS-03, FR-125-002):** hanya lewat gateway (K1) atau kasir (`admin`/`kasir`, kuitansi `source='wallet'`, nomor dari `atomicNextReceiptNo`). Tidak ada endpoint yang menambah saldo langsung.
- **Tabungan (FR-125-004):** setoran oleh admin dengan bukti (unggah) → langsung `completed`. Penarikan oleh admin/siswa → `pending_parent` → ortu menyetujui di portal → admin menyerahkan dana + bukti → `completed`. Penarikan yang diajukan ortu sendiri langsung `approved`. **Penyimpangan tercatat:** persetujuan ortu tidak lewat core karena penyetuju adalah wali siswa yang bersangkutan (per instance), sedangkan flow core berbasis peran. Persetujuan disimpan di `savings_requests` + audit.
- **Kartu siswa:** POS memakai `ScanInput` (§4.8). Kode hasil pindai di-hash lalu dicari di `student_cards` aktif; kartu dicabut → "Kartu tidak berlaku".
- **Akses operator (AC-123-01):** semua endpoint POS/laporan untuk `canteen_operator` memfilter `tenant_id IN (canteen_tenant_operators aktif milik user)`. Parameter `tenant_id` lain → 403.

### 4.4 Job (runner §1, `GATEWAY_JOB_INTERVAL_SECONDS`, penanda `gateway_job_runs`)
1. `wallet_journal:<school>` untuk tanggal D (setelah 00.00 WIB D+1): agregasi movement D → `DailyJournal` → outbox `gateway_journal_postings(source='wallet', reference='wal:<school>:<D>')`.
2. `canteen_journal`: approval settlement → `cts:<settlement_id>:share`, paid → `cts:<settlement_id>:paid`.
3. `journal_outbox`: kirim `pending` ke `POST /api/internal/journals` (source dari kolom), backoff, gagal maks `journal_max_attempts` → exception `journal_posting` (**AC-IF-03** pola T6). Referensi sama = idempoten di accounting.
4. `wallet_recon:<school>` setelah jurnal D `posted`: total saldo per kind vs saldo akun titipan (§4.5) → `wallet_reconciliations`. Selisih → exception + notifikasi `finance_manager` (**NFR-OPS-04**).
5. Top-up gateway `pending` yang transaksinya sudah `expired` → ikut status (cadangan jika K2 tidak terpanggil).

### 4.5 Accounting (Laravel)
- Tambah route `GET /api/internal/accounts/balances?school_id=&acc_codes[]=` (middleware `internal.key` yang sudah ada) di `routes/api.php` + `app/Http/Controllers/InternalBalanceController.php`: `Tenancy::activate`, lalu `Ledger::balances(tahun, periode aktif)` → `ending` per akun, dikonversi ke rupiah bulat (string). Hanya jurnal **posted** (BR-ACC-03).
- **Tidak ada migrasi Laravel.** Akun dana titipan adalah akun COA biasa per tenant schema yang dibuat akuntan lewat menu 33. Pemetaan kodenya disimpan di `wallet_accounts` (billing), mengikuti pola `gateway_accounts`. Kode akun tidak di-seed (tidak dikarang). Mapping kosong → job jurnal membuat exception "akun dompet belum dipetakan" dan tidak memakai fallback hardcode.

### 4.7 Kontrak BILL-ADHOC (internal, X-Internal-Key, scope `school_id` di body/query, file `handler/bills_adhoc.go` + logika murni `internal/adhoc/`)
| Endpoint | Body / respons | Perilaku |
|---|---|---|
| `POST /internal/bills/adhoc` | `{school_id, student_user_id, source_module, source_ref, items:[{name, amount_idr}], due_date}` → `201 {bill_id, net_idr, status}` | Membuat `student_bills` (`scheme_id NULL`) + `bill_items`. Idempoten per `(school_id, source_module, source_ref)`: panggilan ulang dengan isi sama → `200` + bill yang sama; isi berbeda → `409` (pakai `adjust`). Alokasi deposit otomatis mengikuti `generateBills` T6 bila ada saldo deposit |
| `POST /internal/bills/adhoc/adjust` | `{school_id, source_module, source_ref, kind:'prorate'\|'adjust', items:[…] \| new_net_idr, reason, idempotency_key}` → `{bill_id, net_idr, status, deposit_idr}` | `adhoc.Adjust`. Baris `bill_adjustments`, `bill_items` lama tidak dihapus (item penyesuaian bertanda). Kelebihan bayar → `student_deposits` + `deposit_movements` (T6). Tagihan dengan transaksi gateway `pending` → `409` |
| `POST /internal/bills/{id}/void` | `{school_id, reason}` | Hanya jika belum ada pembayaran terverifikasi. Sudah dibayar → `409` (pakai `adjust` ke 0 supaya kelebihan masuk deposit). Idempoten |
| `GET /internal/bills/by-source?school_id=&source_module=&source_ref=` | `{bill_id, net_idr, paid_idr, outstanding_idr, status, due_at}` / 404 | Pemilik modul membaca status lunas tanpa menyalin data |
Semua mutasi ditulis ke `billing_audit_events` (actor `internal:<source_module>`). Tagihan ad-hoc tampil dan dapat dibayar di alur tagihan yang sama (ortu/siswa/kasir/gateway). Fixture `httptest` untuk konsumen gelombang 4 ditulis di handoff T12.

### 4.8 Pemindai & kartu siswa bersama (IF-OPS-04, OQ-30)
- `web/src/components/ui/ScanInput.vue`: props `modelValue: string`, `placeholder?: string`, `camera?: boolean`; emit `update:modelValue`, `scan(code: string)`. Mode keyboard wedge: input berlabel, fokus otomatis, `scan` dipicu Enter. Mode kamera (`camera=true`): tombol ikon `aria-label="Pindai dengan kamera"`, `BarcodeDetector` (format `qr_code`, `code_128`, `ean_13`). Kalau tidak didukung, tombol disembunyikan dengan teks "Kamera tidak didukung, gunakan pemindai atau ketik kode". Stream kamera dihentikan saat unmount/tutup. Tanpa logika domain.
- `POST /internal/student-cards/resolve {school_id, code}` → `200 {student_user_id, card_id}` / `404` (tidak dikenal atau dicabut). Hash kode sama dengan saat penerbitan. Dipakai T18/T19 (absensi perjalanan) di gelombang 4. Penerbitan/pencabutan kartu: endpoint admin di §5.

### 4.6 Pembagian file dengan T14 (billing)
| | T12 (milik) | T14 (milik) |
|---|---|---|
| Paket Go | `internal/canteen/`, `internal/wallet/`, `internal/adhoc/` | `internal/gateway/` |
| Handler baru | `canteen_tenants.go`, `canteen_pos.go`, `canteen_reports.go`, `canteen_settlements.go`, `canteen_directory.go`, `bills_adhoc.go`, `student_cards.go`, `wallet_parent.go`, `wallet_admin.go`, `wallet_topup.go`, `wallet_savings.go`, `jobs_wallet.go`, `journal_outbox.go` | `gateway_providers.go`, `gateway_routing.go`, `gateway_charge.go`, `autodebit.go`, `retail.go`, `jobs_gateway_ext.go` |
| Handler lama yang diubah | `transfer_billing.go`, `refunds.go` (cabang `wallet_id` di `ProcessRefund`), `LEFT JOIN`/`NullString` skema di `bills.go`/`parent.go`/`summary.go`/`admin.go`/`internal.go`/`discounts.go` (BILL-ADHOC) | semua `gateway_*.go` yang ada, `internal/nicepay/` |
| Lain | `config/wallet.go`, `common/roles_canteen.go` (konstanta `RoleCanteenAdmin`, `RoleCanteenOperator`), `client/accounting_balance.go`, `client/identity_accounts.go` | `config/gateway.go`, `common/roles_payment.go` |
| Bersama (hanya tambah baris) | `cmd/server/main.go` (blok route "T12"), `handler/app.go` (field baru), `internal/jobs/runner.go` (salinan identik) | sama, blok "T14" |
Dilarang: T12 menyentuh `gateway_*.go`/`internal/model/gateway.go`; T14 menyentuh `canteen_*`/`wallet_*`/`bills_adhoc.go`/`student_cards.go`/`refunds.go`/`transfer_billing.go`/`receipts.go`/`bills.go`/`parent.go`/`summary.go`/`admin.go`/`internal.go`/`discounts.go`.

## 5. Endpoint (cek fitur di server: `canteen` untuk `/canteen/*`, `/pos/*`; `wallet` untuk dompet/top-up/tabungan; scope `school_id` dari claims)
| Method & path | Peran | Catatan |
|---|---|---|
| `CRUD /canteen/tenants` · `POST /canteen/tenants/{id}/schemes` (baris riwayat baru) · `GET …/schemes` | canteen_admin, admin | FR-123-001/002. Rekening tampil dimasking kecuali di form edit |
| `POST /canteen/tenants/{id}/operators {email, full_name}` · `DELETE …/operators/{userId}` | canteen_admin | FR-123-003 (identity §4.2) |
| `CRUD /canteen/tenants/{id}/menu-items` · `PUT …/menu-items/{id}/stock` | canteen_admin; canteen_operator (tenant sendiri) | FR-124-001 |
| `GET /pos/session` (tenant milik operator, menu, `pos_offline_max_minutes`) · `PUT /pos/open-state {is_open}` | canteen_operator | status buka untuk T25 |
| `POST /pos/sales` `{tenant_id, client_txn_id, pay_method, card_token?, items[]}` | canteen_operator | FR-124-002/003, AC-124-01, NFR-OPS-01 |
| `POST /pos/sales/sync` `{device_id, sales[]}` → `{accepted[], duplicate[], rejected[]}` | canteen_operator | FR-124-005, NFR-OPS-02 |
| `POST /pos/sales/{id}/void {reason}` · `GET /pos/sales/{id}/receipt` | canteen_operator (tenant sendiri), canteen_admin | FR-124-004, struk |
| `GET /canteen/reports/sales?tenant_id=&from=&to=&group_by=tenant\|item\|day\|method` | canteen_admin, admin, finance_manager (semua tenant); canteen_operator (paksa tenant sendiri) | FR-126-001, **AC-123-01** |
| `POST /canteen/settlements {tenant_id, period_from, period_to}` (draft + kalkulasi) · `POST …/{id}/submit` (→ core) · `POST …/{id}/pay {paid_ref}` · `GET /canteen/settlements` | canteen_admin (buat/ajukan); finance_manager (bayar); canteen_operator (baca miliknya) | FR-126-002/003, AC-126-01 |
| `GET /canteen/directory` · `GET /internal/canteen/directory` | semua peran sekolah · internal | kontrak T25 (gabungan) |
| `GET /internal/canteen/tenants?school_id=` → `[{tenant_id, code, name, location_label, room_id, is_open, open_changed_at}]` · `GET /internal/canteen/tenants/{id}/menu?school_id=` → `[{item_id, name, category, price_idr, available}]` | internal (T25 / ops) | **kontrak T25** menu 156 (FR-156-001/002, AC-156-01): hanya tenant/item aktif, tanpa data pemilik/rekening, tanpa cache |
| `GET /parent/children/{sid}/wallets` · `PUT …/wallets/allowance/limit {daily_limit_idr\|null}` · `GET …/wallets/{kind}/movements` | orang_tua (`GuardianHasAccess`) | FR-125-001/003 |
| `POST /parent/children/{sid}/wallets/{kind}/topups {method_id, amount_idr}` (+`Idempotency-Key`) → `{topup_id, transaction_id}` | orang_tua | FR-125-002, K1; frontend lanjut ke `orang-tua/transaksi/:id` (T6) |
| `GET /parent/savings-requests` · `POST …/{id}/decision {approve\|reject}` · `POST /parent/children/{sid}/savings-withdrawals` | orang_tua | FR-125-004 |
| `GET /me/wallets` · `GET /me/wallets/{kind}/movements` · `GET /me/student-card` (QR) · `POST /me/savings-withdrawals` | siswa | FR-125-001/004 |
| `GET /admin/wallets?q=&status=` | admin, canteen_admin | FR-125-001 |
| `POST /admin/student-cards {student_user_id}` (terbit/ganti, kartu lama dicabut; kode ditampilkan sekali + QR) · `POST /admin/student-cards/{id}/revoke` · `GET /admin/student-cards?student_user_id=` | admin, canteen_admin | OQ-30, kartu bersama (tanpa gerbang fitur) |
| `POST /internal/student-cards/resolve` | internal | §4.8 |
| `POST /internal/bills/adhoc` · `POST /internal/bills/adhoc/adjust` · `POST /internal/bills/{id}/void` · `GET /internal/bills/by-source` | internal (T18/T19/T21) | §4.7 BILL-ADHOC |
| `POST /cashier/wallet-topups` · `POST /admin/savings-requests` (setoran/penarikan + bukti) · `POST /admin/savings-requests/{id}/complete` | admin, kasir, finance_manager (`RequireCashier`) | FR-125-002/004 |
| `POST /admin/wallets/{id}/close {disposition:'refund'\|'transfer', target_student_user_id?}` | admin (ajukan), approval `wallet_transfer` untuk transfer | FR-125-005 (lulus/keluar manual) |
| `GET/PUT /admin/wallet-settings` · `GET/PUT /admin/wallet-accounts` · `GET /admin/wallet-reconciliations` | finance_manager, admin | OQ-29, NFR-OPS-04 |
| `POST /internal/canteen/settlements/{id}/callback` · `POST /internal/wallets/transfer-callback` | internal (core) | idempoten |

## 6. Frontend (`web/src/router/domains/canteen.routes.ts`, service `web/src/services/canteen/`; `meta.feature: 'canteen'` untuk `kantin/*`, `'wallet'` untuk route dompet)
| Path | View | Peran | Label · navGroup |
|---|---|---|---|
| `kantin/tenant` · `/:id` | `views/canteen/TenantsView.vue`, `TenantDetailView.vue` (tab Profil / Skema (riwayat) / Operator / Menu) | canteen_admin, admin | Tenant Kantin · **Kantin** |
| `kantin/pos` | `views/canteen/PosView.vue`: grid menu besar (target sentuh ≥ 44px), keranjang, `ScanInput` (camera) untuk kartu siswa, pilihan Dompet/Tunai, badge teks **Daring/Luring (n antre)**, sisa menit luring, struk cetak (print CSS), void dengan dialog alasan, saklar Buka/Tutup | canteen_operator | POS · Kantin |
| `kantin/laporan` | `views/canteen/SalesReportView.vue` (filter tenant/periode/metode, tabel + total, "Diperbarui: …") | canteen_admin, admin, finance_manager, canteen_operator | Laporan Kantin · Kantin |
| `kantin/penyelesaian` · `/:id` | `views/canteen/SettlementsView.vue`, `SettlementDetailView.vue` (rincian bagi hasil, status approval, tombol Bayar) | canteen_admin, finance_manager, canteen_operator (baca) | Bagi Hasil Tenant · Kantin |
| `keuangan/dompet` | `views/wallet/WalletAdminView.vue` (cari siswa, saldo 2 jenis, terbit/cabut kartu QR, setoran/penarikan + unggah bukti, tutup dompet) | admin, canteen_admin | Dompet Siswa · Keuangan |
| `keuangan/kasir` (perluas `CashierView.vue`, tab "Top-up Dompet", tampil hanya jika fitur aktif) | — | admin, kasir | — |
| `keuangan/dompet/rekonsiliasi` | `views/wallet/WalletReconciliationView.vue` + form pemetaan akun + pengaturan batas saldo | finance_manager, admin | Rekonsiliasi Dompet · Keuangan |
| `orang-tua/dompet` · `/:studentId` | `views/parent/ParentWalletView.vue` (kartu saldo uang saku & tabungan, atur batas harian, riwayat, Top-up → pilih metode → `orang-tua/transaksi/:id`, persetujuan penarikan) | orang_tua | Dompet Anak · Portal |
| `siswa/dompet` | `views/student/StudentWalletView.vue` (saldo, riwayat, QR kartu dari `qrcode`, ajukan penarikan tabungan) | siswa | Dompet Saya · (navGroup siswa yang ada) |
`components/ui/ScanInput.vue` (§4.8) milik T12, dipakai ulang task lain tanpa disalin. Tiga state (memuat/kosong/galat) di semua view, format rupiah dari `src/utils/format.ts`, badge berteks.

## 7. Bukti handoff
Vektor §3; **AC-123-01** (operator tenant A memanggil laporan dengan `tenant_id` B → 403 dan respons tanpa `tenant_id` hanya berisi A), **AC-124-01**, **AC-125-01** (fake K1 + panggil `OnGatewaySuccess` → saldo +200.000, notifikasi terkirim sekali walau hook dipanggil 2x), **AC-125-02** (exit → `bill_refunds` 350.000 `wallet_id` terisi, idempoten), **AC-126-01**. Tes wajib lain: sinkron batch yang sama 2x → tidak ada duplikat (FR-124-005); 50 goroutine belanja dompet yang sama → saldo tidak negatif; uji beban 300 penjualan dompet (`TEST_PG_DSN`) dengan p95 ≤ 2 detik dicatat (NFR-OPS-01); `DailyJournal` + `Reconcile` dengan fake accounting (NFR-OPS-04); tidak ada float (`grep -rn float internal/wallet internal/canteen` kosong); migrasi 0005 + 0007 up/down/up di DB baru bersama 0001–0004 (dan 0006 setelah rebase T14). **BILL-ADHOC:** create 2x isi sama → 1 tagihan, isi beda → 409; adjust 300.000→200.000 setelah lunas → deposit 100.000; void tagihan lunas → 409; tagihan ad-hoc muncul di daftar tagihan ortu & bisa dibayar kasir; tagihan skema lama tetap tampil (regresi LEFT JOIN). **Kartu:** resolve kode dicabut → 404. **ScanInput:** `vue-tsc` + uji komponen (Enter memicu `scan`).
FR tercakup: FR-123-001..003, FR-124-001..005, FR-125-001..006, FR-126-001..003 (17). AC: AC-123-01, AC-124-01, AC-125-01/02, AC-126-01 (5).
**Playwright (setelah merge gelombang, bersama T14):** canteen_admin buat tenant + skema 10% + operator + menu → admin terbitkan kartu QR siswa → ortu atur batas 25.000 dan top-up 200.000 (mock NICEPay, webhook sukses) → saldo 200.000 → operator POS: dompet 20.000 sukses, 10.000 ditolak "Melebihi batas jajan harian" → set offline (DevTools offline) jual tunai 2x → online → laporan menampilkan 2 tunai tanpa duplikat → void 1 transaksi dompet, saldo kembali → buat bagi hasil, ajukan, finance_manager setujui di Approval Center, bayar → operator tenant B tidak melihat data tenant A.
