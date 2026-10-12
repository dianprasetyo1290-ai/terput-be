# T14: Ekstensi pembayaran di service `billing`: multi provider + routing + failover, rekonsiliasi per provider, mandat autodebit, gerai retail

**Menu PRD:** 150, 151 (BR-PAY-03/04, IF-ADV-03, OQ-41; melanjutkan BR-PAY-01/02, AC-IF-01/02/03 dari T6) · **Branch:** `feat/T14-ekstensi-pembayaran` · **Service:** `billing` (paket `internal/gateway`) · **Migrasi:** `services/billing/migrations/0006_payment_extensions.*` (nomor 0006 milik T14; 0005 dan 0007 milik T12)
**Prasyarat:** T6 (NICEPay SNAP, sudah di `integrasi/prd-v2`), T1. **Paralel** dengan T12 di service yang sama: kontrak §4.1 dan pembagian file §4.6 **mengikat** (sama dengan T12 §4.1/§4.6). **Driver DB:** `database/sql`.
**Feature key (final, T9 §2):** `payment_multi_provider` (150), `payment_autodebit` (151, termasuk gerai retail). Keduanya membutuhkan `payment_gateway` aktif. Kalau keduanya mati, perilaku T6 **tidak berubah sedikit pun**. **Peran (final, T9 §2):** Platform Admin = `superadmin` yang sudah ada (tidak ada peran baru); `finance_manager`, `admin`, `orang_tua`.
**Prinsip:** refactor tanpa regresi. Semua test T6 (`internal/handler/gateway_test.go`, `internal/nicepay/client_test.go`, `internal/model/gateway_test.go`) tetap hijau **tanpa diubah**. Respons `POST /parent/checkout` untuk konfigurasi lama (hanya NICEPay, tanpa aturan routing) identik.

## 1. Acuan kode
| Kebutuhan | Pakai / tiru |
|---|---|
| Klien NICEPay | `services/billing/internal/nicepay/client.go` (`CreateVA`, `CreateQRIS`, `CreateEWallet`, `Status`, `CancelVA`, `VerifyNotification`). Dibungkus adapter, **tidak** ditulis ulang |
| Alur checkout & webhook | `internal/handler/gateway_checkout.go` (`ParentCheckout`: pembuatan charge sekarang langsung memanggil `a.Nicepay` + cabang simulasi `SIM-`), `gateway_webhook.go` (`NicepayWebhook`: verifikasi → dedup `(provider_ref, raw_hash)` → re-query → `FOR UPDATE` → alokasi → kuitansi → jurnal) |
| Fee, alokasi, rekonsiliasi | `internal/model/gateway.go` (`Totals`, `Allocate`, `Reconcile`): dipanggil per provider, tidak diubah |
| Settlement | `gateway_settlement.go` (`settlement_batches.provider` sudah ada) |
| Approval/exception/notifikasi | `client.RequestApproval`, `client.CreateException`, `client.Notify` |
| Job berkala | salin **verbatim** `services/hr/internal/jobs/runner.go` ke `services/billing/internal/jobs/` (T12 menyalin file yang sama; identik saat merge) |
**Interface `Provider` kini dibenarkan:** IF-ADV-03 + OQ-41 mewajibkan adapter per provider, dan sudah ada dua implementasi nyata: `nicepay` serta `sim` (cabang simulasi yang sudah ada di `ParentCheckout`, dipindah menjadi adapter, ditolak saat `APP_ENV=production`). Ini mencabut keputusan "tanpa interface" di T6 §1. Catat di handoff.

## 2. Migrasi `0006_payment_extensions.up.sql`
```
-- Provider (instalasi klien, dikelola superadmin; FR-150-001). Kredensial TIDAK di DB.
payment_providers(code text PK /* 'nicepay', … */, adapter CHECK IN('nicepay','sim'), label, enabled bool,
  credential_ref text /* prefix env/secret manager, mis. 'NICEPAY' → NICEPAY_* */, consecutive_failures int default 0,
  last_failure_at NULL, created_at, updated_at)
payment_provider_capabilities(provider_code, capability CHECK IN('va','qris','ewallet','retail','autodebit'), PRIMARY KEY(provider_code, capability))
payment_method_provider_codes(method_id REFERENCES payment_methods(id), provider_code, provider_method_code text, PRIMARY KEY(method_id, provider_code))
  -- seed: INSERT … SELECT id, 'nicepay', provider_code FROM payment_methods (kolom lama dipertahankan)
-- Routing per metode & unit (unit = sekolah; FR-150-002/003, BR-PAY-03)
payment_routing_rules(id, school_id, method_kind CHECK IN('va','qris','ewallet','retail'), method_code text NULL /* NULL = semua metode jenis ini */,
  primary_provider, fallback_provider NULL, failover_enabled bool NOT NULL DEFAULT false, active bool,
  UNIQUE(school_id, method_kind, method_code) WHERE active, CHECK(fallback_provider IS NULL OR fallback_provider <> primary_provider))
-- Perluasan T6 (aditif, backfill 'nicepay')
ALTER payment_methods: CHECK kind IN('va','qris','ewallet','retail')
ALTER gateway_transactions ADD provider text NOT NULL DEFAULT 'nicepay', ADD failover_from text NULL,
  ADD purpose text NOT NULL DEFAULT 'bills' CHECK(purpose IN('bills','wallet_topup')), ADD purpose_ref uuid NULL,
  ADD mandate_id uuid NULL, ADD pay_code text NULL /* kode bayar gerai */, ADD retail_outlet text NULL,
  DROP CONSTRAINT UNIQUE(provider_ref) → ADD UNIQUE(provider, provider_ref)       -- ref unik per provider (BR-PAY-03)
  ADD CHECK(purpose='bills' OR purpose_ref IS NOT NULL)
ALTER gateway_notifications ADD provider text NOT NULL DEFAULT 'nicepay'; UNIQUE(provider, provider_ref, raw_hash) menggantikan yang lama
ALTER reconciliation_exceptions ADD provider text NOT NULL DEFAULT 'nicepay'; UNIQUE(school_id, provider, kind, ref) menggantikan yang lama  -- FR-150-004
-- Autodebit (FR-151-001..003, BR-PAY-04)
autodebit_settings(school_id PK, max_retries int NOT NULL DEFAULT 2 /* FR-151-002 [2] */, retry_interval_days int NULL /* wajib diisi sebelum autodebit diaktifkan, tanpa default */,
  run_hour smallint NULL /* jam WIB job debit; wajib diisi */)
autodebit_mandates(id, school_id, guardian_user_id, student_user_id, provider, provider_mandate_ref NULL, account_masked text NULL,
  status CHECK IN('pending_activation','active','cancelled','failed'), max_amount_idr bigint NULL, activated_at NULL,
  cancelled_at NULL, cancelled_by NULL, cancel_reason NULL, idempotency_key, UNIQUE(school_id, idempotency_key))
autodebit_mandate_schemes(mandate_id, scheme_id REFERENCES billing_schemes(id), PRIMARY KEY(mandate_id, scheme_id))   -- "jenis tagihan" = skema tagihan
autodebit_attempts(id, school_id, mandate_id, bill_id, attempt_no int CHECK(attempt_no>=1), scheduled_for date,
  status CHECK IN('scheduled','processing','success','failed','skipped'), skip_reason NULL, gateway_transaction_id NULL, failure_reason NULL,
  idempotency_key text, UNIQUE(school_id, idempotency_key) /* 'ad:<mandate>:<bill>:<attempt_no>' */, UNIQUE(mandate_id, bill_id, attempt_no))
```
`.down.sql` mengembalikan constraint lama hanya jika semua baris berprovider `nicepay`; kalau tidak, `RAISE` (tidak menghapus data).

## 3. Fungsi murni + vektor (`internal/gateway`, tes tabel)
| Fungsi | Vektor → AC/FR |
|---|---|
| `Route(rules, method, providers) (primary, fallback, error)` | **AC-150-01** QRIS→A, VA→B, pilih VA → B. Aturan `method_code` spesifik mengalahkan aturan per jenis. Tanpa aturan → `nicepay` bila aktif (kompatibel T6). Provider tujuan nonaktif/tanpa kapabilitas → error "Metode belum dirutekan" (FR-150-002, BR-PAY-03) |
| `ShouldFailover(err, rule)` | timeout / error jaringan / HTTP 5xx → true **jika** `failover_enabled` dan fallback punya kapabilitas. 4xx bisnis (nominal ditolak) → false. Fallback = provider yang sama → false (FR-150-003) |
| `ProviderCode(method, provider, mappings)` | `nicepay` tanpa mapping → `payment_methods.provider_code` lama; provider lain tanpa mapping → error |
| `GroupForReconcile(txs, items)` | memisah per provider sebelum `model.Reconcile`. TX provider A tidak pernah dicocokkan dengan settlement provider B (FR-150-004) |
| `DueDebits(mandates, bills, attempts, today, settings)` | tagihan jatuh tempo ≤ hari ini, skema ada di mandat, belum lunas, mandat `active` → attempt 1. `max_amount_idr` terlampaui → `skipped` (orang tua dinotifikasi) |
| `NextRetry(attempt, settings)` | **AC-151-01** gagal attempt 1 → attempt 2 pada `+retry_interval_days`; gagal attempt 3 (1 + 2 retry) → berhenti + notifikasi "debit gagal, bayar manual" (FR-151-002) |
| `CanDebit(mandate, at)` | **AC-151-02** dibatalkan 20 Okt → tagihan November (jatuh tempo setelahnya) **tidak** didebit; attempt `scheduled` milik mandat itu → `skipped/mandate_cancelled`. Attempt yang sudah `processing` tetap diselesaikan (sudah di provider) (FR-151-003, BR-PAY-04) |
| `RetailExpiry(method, now)` | kode bayar berlaku `valid_minutes` metode retail; lewat → `expired` (FR-151-004) |

## 4. Integrasi & job
### 4.1 Kontrak dengan T12 (salinan T12 §4.1)
- **Data:** `gateway_transactions.purpose` (`bills`|`wallet_topup`) + `purpose_ref` dibuat **T14** di 0006.
- **K1 (T14 mengimplementasikan untuk T12):** method `func (a *App) CreatePurposeCharge(ctx context.Context, schoolID, payerUserID, studentUserID, methodID, purpose, purposeRef, idempotencyKey string, amountIDR int64) (gatewayTxID string, err error)` di `gateway_charge.go`. Memakai jalur yang sama dengan checkout tagihan (routing, failover, `model.Totals`). `bills_amount_idr = amountIDR`, tanpa `gateway_transaction_bills`, `purpose_ref` wajib. Idempoten per `idempotency_key`.
- **K2 (T14 memanggil hook milik T12):** T14 mendeklarasikan `type PurposeHandler interface { OnGatewaySuccess(ctx context.Context, tx *sql.Tx, schoolID, purposeRef, gatewayTxID string) error; OnGatewayClosed(ctx context.Context, tx *sql.Tx, schoolID, purposeRef, gatewayTxID, status string) error }` dan field `App.PurposeHandlers map[string]PurposeHandler`. Di webhook, setelah `FOR UPDATE` dan status menjadi `success`: `purpose='bills'` → alokasi seperti T6; selain itu → panggil handler purpose dalam transaksi yang sama (handler tidak ada → rollback + 500 + exception `webhook`, supaya provider mengirim ulang). Status `failed`/`expired`/`cancelled` → `OnGatewayClosed`. Kuitansi tetap dibuat untuk semua purpose.
- **Jurnal:** jurnal gateway (bills) hanya untuk `purpose='bills'`. Top-up dijurnal T12.
- **K3 (BILL-ADHOC milik T12, 0007):** `student_bills.scheme_id` menjadi nullable. Query di `gateway_*.go` saat ini tidak `JOIN billing_schemes`, tetapi T14 wajib memastikan semua scan bill di file miliknya tahan `scheme_id NULL` (`sql.NullString`), sehingga tagihan ad-hoc (denda, transport, event) bisa di-checkout, didebit, dan dibayar di gerai. Mandat autodebit hanya mencakup tagihan skema (`autodebit_mandate_schemes`). Tagihan ad-hoc **tidak** didebit otomatis (dicatat sebagai batasan).
- **Wiring** `app.PurposeCharger = app` dan `app.PurposeHandlers["wallet_topup"] = app.WalletTopupHook()` di `main.go` ditambahkan orkestrator saat merge. Sebelum itu T14 menguji K2 dengan handler fake.

### 4.2 Adapter (`internal/gateway/provider.go`)
```
type Provider interface {
  Code() string
  Capabilities() []string
  CreateCharge(ctx, ChargeRequest{RefNo, AmountIDR, Kind, ProviderMethodCode, ValidFor, Buyer…}) (Charge{ProviderRef, VANumber, QRContent, RedirectURL, PayCode, ExpiresAt}, error)
  QueryStatus(ctx, ref) (Status, error)
  Cancel(ctx, ref) error
  VerifyWebhook(headers, body) (Notification{ProviderRef, PartnerRef}, error)
  // kapabilitas autodebit (boleh ErrUnsupported)
  CreateMandate(ctx, MandateRequest) (MandateSetup{ProviderMandateRef, RedirectURL}, error)
  Debit(ctx, DebitRequest) (Charge, error)
  CancelMandate(ctx, ref) error
}
```
- `nicepay`: membungkus `internal/nicepay` (VA, QRIS, e-wallet). Kapabilitas `retail` dan `autodebit` **hanya** diaktifkan jika API-nya ada di dokumentasi resmi NICEPay SNAP yang tersedia. **Jangan mengarang parameter API.** Kalau tidak tersedia → `ErrUnsupported`, kapabilitas tidak di-seed, dan dicatat sebagai item terbuka OQ-41 di handoff.
- `sim`: memindahkan cabang simulasi `SIM-` yang sudah ada, mendukung semua kapabilitas (untuk dev, uji, dan mock Playwright). Ditolak di production.
- Registry dibangun di `main.go` dari `payment_providers` + env `<credential_ref>_*`. Kredensial NICEPay tetap `NICEPAY_*` (T6). Kredensial provider lain disuntik lewat secret manager → env, **tidak pernah** masuk DB/log (FR-150-001).
- Hapus fallback kode akun hardcode di blok jurnal `gateway_webhook.go` (`"1101-01"`, dst., pelanggaran GEMINI §3). Mapping `gateway_accounts` kosong → exception `journal_posting`, tanpa jurnal tebakan.

### 4.3 Alur
- **Checkout (FR-150-002/003):** `ParentCheckout` dan `CreatePurposeCharge` → `createCharge(ctx, in)` (satu fungsi di `gateway_charge.go`): `Route` → `CreateCharge` di primary → error yang `ShouldFailover` → `CreateCharge` di fallback, `failover_from=primary`, `consecutive_failures++`, lalu `POST core /internal/exceptions` kind `webhook` (kategori integrasi gateway yang terlihat oleh `finance_manager` di filter T1), `payload_summary.event='provider_failover'`, idempotency `failover:<tx_no>` (**AC-150-02**). Tanpa failover → 502 dengan pesan seperti T6. Satu transaksi = satu provider; renew (`/renew`) me-route ulang dan membuat transaksi baru (BR-PAY-03).
- **Webhook:** `POST /webhooks/{provider}` (alias lama `/webhooks/nicepay` tetap). Adapter `VerifyWebhook` → langkah T6 (dedup per provider, re-query `QueryStatus` ke provider **transaksi itu**, `FOR UPDATE`, K2). Signature tidak valid → 401 + audit `security` (AC-IF-01 tetap).
- **Settlement & rekonsiliasi (FR-150-004):** import settlement wajib memilih `provider` (format CSV per adapter; NICEPay = format T6). `POST /admin/reconciliation/run?date=&provider=` (kosong = semua provider, dijalankan per provider). Exception menyimpan `provider`. Dashboard pembayaran T6 mendapat breakdown per provider.
- **Gerai retail (FR-151-004):** metode `kind='retail'` di `payment_methods`, dirutekan seperti metode lain ke provider berkapabilitas `retail`. Charge mengembalikan `pay_code` + `retail_outlet` + `expires_at`. Halaman transaksi menampilkan kode, gerai, dan masa berlaku. Pembayaran masuk lewat webhook seperti VA.
- **Mandat (FR-151-001/003):** ortu memilih anak + skema tagihan + provider berkapabilitas `autodebit` → `CreateMandate` → `pending_activation` (redirect ke provider bila ada) → aktivasi lewat webhook/callback provider → `active`. Batal: `POST …/cancel` → `CancelMandate` (best effort; gagal di provider tetap `cancelled` di SEKOCI + exception) → attempt `scheduled` milik mandat menjadi `skipped`. Audit setiap perubahan mandat.

### 4.4 Job (`jobs_gateway_ext.go`, interval `GATEWAY_JOB_INTERVAL_SECONDS`, penanda harian `gateway_job_runs`)
1. `autodebit_schedule:<school>` (setelah `run_hour` WIB): `DueDebits` → insert attempt (UNIQUE = idempoten) → `Debit` (gateway transaction `purpose='bills'`, `mandate_id`, bill tunggal) → hasil sinkron atau via webhook → alokasi T6. Gagal → `NextRetry` + notifikasi ortu (`Idempotency-Key: autodebit:<attempt_id>`) (**AC-151-01**).
2. `retail_expiry`: transaksi retail `pending` lewat `expires_at` → `QueryStatus` → `expired`.
3. `provider_health`: reset `consecutive_failures` setelah charge sukses (dicatat untuk dashboard; tidak mematikan provider otomatis).

### 4.5 Approval
Perubahan routing rule dan aktivasi provider tidak memerlukan approval di PRD → cukup audit (`core /internal/audit`, module `payment`). Tidak membuat tabel approval.

### 4.6 Pembagian file dengan T12
Lihat T12 §4.6 (mengikat). Ringkas: T14 memiliki `internal/gateway/`, semua `gateway_*.go` yang sudah ada, `internal/nicepay/`, `gateway_providers.go`, `gateway_routing.go`, `gateway_charge.go`, `autodebit.go`, `retail.go`, `jobs_gateway_ext.go`, `config/gateway.go`, `common/roles_payment.go`. T14 **tidak** menyentuh `canteen_*`, `wallet_*`, `bills_adhoc.go`, `student_cards.go`, `refunds.go`, `transfer_billing.go`, `receipts.go`, `bills.go`, `parent.go`, `summary.go`, `admin.go`, `internal.go`, `discounts.go`. Bersama (hanya tambah baris): `cmd/server/main.go` (blok "T14"), `handler/app.go`, `internal/jobs/runner.go` (salinan identik).

## 5. Endpoint
| Method & path | Peran | FR/AC |
|---|---|---|
| `GET/POST/PUT /platform/payment-providers` (+ kapabilitas; **tanpa** field kredensial, hanya `credential_ref` + status "kredensial terpasang: ya/tidak") | superadmin | FR-150-001 |
| `CRUD /admin/payment-routing-rules` · `POST /admin/payment-routing-rules/preview {method_id}` → provider yang akan dipakai | finance_manager, admin (sekolah sendiri); superadmin (semua) | FR-150-002/003, AC-150-01 |
| `CRUD /admin/payment-methods` (perluas: `kind='retail'`, mapping kode per provider) | finance_manager, admin | FR-151-004 |
| `POST /parent/checkout` (perilaku §4.3, respons tambah `provider`, `pay_code`, `retail_outlet`, `purpose`) · `GET /parent/transactions/{id}` | orang_tua | AC-150-01/02, FR-151-004 |
| `POST /webhooks/{provider}` · `POST /webhooks/nicepay` (alias) | provider | BR-PAY-01/03 |
| `POST /admin/settlements/import` (+`provider`) · `POST /admin/reconciliation/run?provider=` · `GET /admin/reconciliation/exceptions?provider=` | finance_manager | FR-150-004 |
| `GET /parent/autodebit/mandates` · `POST /parent/autodebit/mandates {student_user_id, scheme_ids[], provider?, max_amount_idr?}` (+`Idempotency-Key`) · `POST /parent/autodebit/mandates/{id}/cancel {reason?}` · `GET /parent/autodebit/attempts?mandate_id=` | orang_tua (`GuardianHasAccess`) | FR-151-001/003, AC-151-02 |
| `GET /admin/autodebit/mandates?status=` · `GET /admin/autodebit/attempts?status=failed` · `GET/PUT /admin/autodebit/settings` | admin, finance_manager | FR-151-002, AC-151-01 |
| `POST /webhooks/{provider}/mandates` (aktivasi/pembatalan dari provider, jika adapter mendukung) | provider | FR-151-001 |

## 6. Frontend
| Path | View | Peran | Label · navGroup · `meta.feature` |
|---|---|---|---|
| `platform/provider-pembayaran` | `views/billing/PaymentProvidersView.vue` (daftar, aktif/nonaktif, kapabilitas berupa badge teks, status kredensial, jumlah gagal beruntun) | superadmin | Provider Pembayaran · **Platform** · `payment_multi_provider` |
| `keuangan/routing-pembayaran` | `views/billing/PaymentRoutingView.vue` (tabel jenis/metode → utama/cadangan/failover, dialog uji "metode X akan ke provider Y") | finance_manager, admin | Routing Pembayaran · Keuangan · `payment_multi_provider` |
| (perluas) `SettlementView.vue`, `PaymentReconciliationView.vue`, `PaymentDashboardView.vue`, `TransactionsView.vue` | pilihan/filter provider, kolom provider + tanda "failover", breakdown per provider. Disembunyikan bila fitur mati | finance_manager, admin | — |
| (perluas) `PaymentMethodsView.vue` | jenis "Gerai retail", mapping kode per provider | finance_manager, admin | — |
| (perluas) `views/parent/ParentCheckoutView.vue`, `ParentTransactionView.vue` | metode gerai (label teks, tanpa logo tidak resmi); halaman transaksi: kode bayar + gerai + countdown + tombol salin; label "Top-up dompet" bila `purpose='wallet_topup'` (daftar tagihan disembunyikan) | orang_tua | — |
| `orang-tua/autodebit` | `views/parent/ParentAutodebitView.vue` (daftar mandat + status badge teks, tambah mandat: pilih anak → centang jenis tagihan → provider → batas opsional → lanjut ke provider; batal dengan dialog konfirmasi "berlaku untuk debit berikutnya"; riwayat attempt) | orang_tua | Autodebit · Portal · `payment_autodebit` |
| `keuangan/autodebit` | `views/billing/AutodebitAdminView.vue` (tab Mandat / Debit gagal / Pengaturan: jumlah retry, interval, jam jalan) | admin, finance_manager | Autodebit · Keuangan · `payment_autodebit` |
Tiga state, aksesibilitas, dan format rupiah sesuai GEMINI §5.

## 7. Bukti handoff
Vektor §3. Tes AC wajib: **AC-150-01** (dua fake provider A/B; checkout VA → `gateway_transactions.provider='B'`), **AC-150-02** (A mengembalikan timeout, failover aktif → transaksi di cadangan, `failover_from='A'`, fake core menerima satu exception `provider_failover`; failover mati → 502 tanpa transaksi), **AC-151-01** (fake `Debit` gagal → attempt 2 terjadwal, notifikasi terkirim; gagal ke-3 → berhenti), **AC-151-02** (mandat dibatalkan 20 Okt → job 1 Nov tidak membuat attempt untuk tagihan November). Tes lain: regresi T6 hijau tanpa diubah; respons checkout konfigurasi lama sama (golden JSON); dedup webhook per provider (ref sama di dua provider tidak bentrok); K1 + K2 dengan handler fake (success memanggil hook sekali di transaksi yang sama, hook error → rollback + 500); tagihan `scheme_id NULL` bisa di-checkout (K3); rekonsiliasi provider A tidak memakai settlement B; kredensial tidak muncul di respons/log/audit (`grep` test); migrasi 0006 up/down/up di DB baru bersama 0001–0004 (dan 0005/0007 setelah rebase T12).
FR tercakup: FR-150-001..004, FR-151-001..004 (8). AC: AC-150-01/02, AC-151-01/02 (4).
**Playwright (setelah merge, adapter `sim` + mock NICEPay):** superadmin aktifkan provider `sim` → finance_manager rute QRIS→nicepay, VA→sim → ortu checkout VA (provider sim) dan QRIS (nicepay) → webhook sukses keduanya → rekonsiliasi per provider tanpa exception silang → matikan nicepay dengan failover ke sim → checkout QRIS tercatat failover + item di Integration Exception Queue → ortu buat mandat autodebit SPP → job debit gagal (sim dipaksa gagal) → notifikasi + attempt 2 → ortu batalkan mandat → attempt terjadwal `skipped` → checkout metode gerai menampilkan kode bayar + masa berlaku.
