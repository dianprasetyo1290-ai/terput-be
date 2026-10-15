# Handoff QA-BILL — billing, keuangan (menu 17–25, 29 kasir, 31 rekonsiliasi), refund, jurnal dari billing

Branch `feat/QA-BILL`, basis `b984872`. Semua temuan di QA-BILL.md berstatus **FIXED**; tidak ada DEFERRED.
Area di luar billing ditulis sebagai kontrak (bagian "Kontrak ke area lain").

## Tabel status

| BUG | Status | File utama | Bukti (tes) |
|---|---|---|---|
| BUG-006 alokasi manual kasir | FIXED | handler/gateway_cashier.go | TestCashierManualAllocationAndDeposit |
| BUG-007 pengingat tagihan | FIXED | handler/reminders.go, model/reminder.go, job `bill_reminders` | TestBillReminders, model/reminder_test.go |
| BUG-009 impor mutasi bank 500 | FIXED | handler/gateway_reconciliation.go, model/bankstatement.go | TestBankStatementImportAndMatch, bankstatement_test.go |
| BUG-010 jurnal tagihan/pembayaran + Exception Queue | FIXED | handler/journal_queue.go, journal_outbox.go | TestBillingJournalsQueuedIdempotentAndBalanced, TestJournalOutbox* |
| BUG-043 refund lewat Approval Center, akses FM | FIXED | handler/refunds.go | TestRefundApprovalCenter |
| BUG-044 aturan nominal approval untuk refund | FIXED | handler/refunds.go (amount_idr ke core) | TestRefundApprovalCenter |
| BUG-048 dashboard keuangan untuk Yayasan | FIXED | common/auth.go (RequireFinanceViewer), web billing.routes.ts | common tests, TestFinanceReports |
| BUG-049 FM 403 di skema/tagihan/refund/diskon | FIXED | common/auth.go, handler/*.go | TestSchemeTariffControls, TestRefundApprovalCenter |
| BUG-050 tagihan terbit tanpa mapping COA | FIXED | handler/bills.go (SchemeIssueMapping) | TestGenerateBillsGuards |
| BUG-051 perubahan tarif tanpa approval | FIXED | handler/schemes.go (`fee_change`) | TestSchemeTariffControls |
| BUG-052 termin tagihan | FIXED | handler/bill_changes.go, model/installment.go | TestBillTermsAnnualAndChanges |
| BUG-053 batal/sesuaikan tagihan via approval | FIXED | handler/bill_changes.go (`bill_adjustment`) | TestBillTermsAnnualAndChanges |
| BUG-054 tagihan untuk non-siswa / tingkat salah | FIXED | handler/bills.go | TestGenerateBillsGuards |
| BUG-055 pengelolaan & pengajuan beasiswa | FIXED (putaran 2) | handler/discount_programs.go, model/discount_program.go, ScholarshipView.vue | TestDiscountProgramApproval, discount_program_test.go |
| BUG-056 pengecualian rekonsiliasi maker-checker | FIXED | handler/recon_exceptions.go (`recon_adjustment`) | TestReconExceptionMakerChecker |
| BUG-058 nama placeholder di kuitansi kasir | FIXED | handler/gateway_cashier.go, receipts.go | TestCashierManualAllocationAndDeposit |
| BUG-086 audit perubahan tarif lengkap | FIXED | handler/schemes.go (auditBoth before/after) | TestSchemeTariffControls |
| BUG-118 peringatan akuntan + lock retry | FIXED (putaran 2) | handler/journal_outbox.go, summary.go, FinanceDashboardView.vue | TestJournalOutboxRetriesWithBackoffThenRaisesException |
| BUG-122 impor riwayat pembayaran | FIXED (putaran 2) | handler/payment_import.go, model/payment_import.go, PaymentImportCard.vue | TestImportPaymentHistory, payment_import_test.go |
| BUG-123 laporan penerimaan + validasi rentang | FIXED | handler/summary.go, finance_reports.go | TestFinanceReports, finance_report_test.go |
| BUG-124 periode berlaku tarif | FIXED | handler/schemes.go | TestSchemeTariffControls |
| BUG-125 dimensi program/angkatan/jalur | FIXED | handler/schemes.go, SchemeListView.vue | TestBillTermsAnnualAndChanges |
| BUG-126 cicilan per komponen | FIXED | model/installment.go | installment_test.go, TestBillTermsAnnualAndChanges |
| BUG-127 tarif ganda ditolak | FIXED | model/scheme.go | scheme_test.go, TestSchemeTariffControls |
| BUG-128 billing tahunan sekaligus | FIXED | handler/bills.go (generate-annual) | TestBillTermsAnnualAndChanges |
| BUG-129 invoice PDF | FIXED | handler/invoice.go | TestLedgerUnallocatedInvoice, TestBuildTextPDFPaging |
| BUG-130 kartu piutang lengkap | FIXED | handler/ledger.go, model/ledger.go | TestLedgerUnallocatedInvoice, ledger_test.go |
| BUG-131 deposit dialokasikan otomatis | FIXED | handler/bills.go (applyDepositToBill) | TestGenerateBillsGuards |
| BUG-132 dana belum teralokasi | FIXED | handler/ledger.go (UnallocatedFunds) | TestLedgerUnallocatedInvoice |
| BUG-133 log tindak lanjut | FIXED | handler/reminders.go (followups) | TestBillReminders |
| BUG-134 aging untuk Yayasan | FIXED | common/auth.go, AgingReceivableView.vue | TestFinanceReports |
| BUG-135 refund: notifikasi + referensi transfer | FIXED | handler/refunds.go | TestRefundApprovalCenter |
| BUG-136 rekonsiliasi settlement harian + status batch | FIXED | handler/jobs_settlement.go, gateway_reconciliation.go | TestSettlementBatchStatus, TestGatewayExpiryAndSettlementJobs |
| BUG-138 paginasi server | FIXED | handler/bills.go, arrears, refunds, ledger | TestFinanceReports |
| BUG-139 validasi format mutasi bank | FIXED | model/bankstatement.go | bankstatement_test.go |
| BUG-163 batas kirim ulang kuitansi | FIXED | model/receipt_resend.go | receipt_resend_test.go, TestCorrectionAndResendRules |
| BUG-164 online tidak boleh lewat koreksi | FIXED | model/correction.go | correction_test.go, TestCorrectionAndResendRules |
| BUG-171 kode komponen duplikat 500 | FIXED | handler/schemes.go (422) | TestSchemeTariffControls |
| BUG-172 tarif nol tanpa penanda gratis | FIXED | model/scheme.go (`is_free`) | scheme_test.go |
| BUG-173 gagal notifikasi ke Exception Queue | FIXED | handler/reminders.go (raiseNotificationFailure) | TestBillReminders |
| BUG-174 status lewat jatuh tempo | FIXED | model/billstatus.go | TestBillStatusFollowsLedger |
| BUG-175 tren aging bulanan | FIXED | handler/finance_reports.go | TestFinanceReports |
| Tambahan koordinator: status settlement_batches | FIXED | gateway_reconciliation.go (`matched`/`mismatch`) | TestSettlementBatchStatus |
| Tambahan koordinator: job kedaluwarsa transaksi gateway | FIXED | jobs_settlement.go (`gateway_expiry`) | TestGatewayExpiryAndSettlementJobs |
| Putaran 3: refund mutasi/keluar dipisah deposit vs kelebihan bayar | FIXED | handler/student_refund.go, transfer_billing.go, refunds.go, model/refund.go | TestStudentRefundDepositSplit, TestSplitStudentRefund, TestRefundJournal |
| Putaran 3: pengajuan refund ditolak core → Exception Queue | FIXED | handler/student_refund.go (reportRefundApprovalFailure) | TestInternalStudentRefund_Idempotency |
| Putaran 4: refund saldo dompet (wallet_id) diproses tuntas | FIXED | handler/wallet_refund.go, wallet_admin.go, refunds.go, wallet/wallet.go, RefundView.vue | TestWalletRefundFlow, TestClosureForRefund, TestWalletRefundJournal |

## Migrasi

Hanya `services/billing/migrations/0009_billing_controls.{up,down}.sql` (0010 milik QA-PAY; 0011 tidak dipakai). Isinya:
- `billing_reminder_settings`, `bill_reminder_log`, `bill_followups`
- `billing_schemes` (approval, periode berlaku, program/cohort/admission_track), `billing_scheme_components` (`is_free`, kind `annual`, `installment_terms`, `installment_interval_months`), `billing_scheme_changes`
- `bill_refunds` (approval, transfer_ref), `reconciliation_exceptions` (approval), `gateway_settings.receipt_resend_cooldown_minutes`
- `bill_installments`, `bill_adjustments` kind `cancel`, `bill_change_requests`
- `bill_discounts` (status, valid_from/valid_to, approval, idempotency_key), `bill_payments.source` ditambah `import`, `payment_imports`
- Putaran 3: `bill_refunds.deposit_idr` (CHECK 0 ≤ deposit_idr ≤ amount_idr) dan tabel `bill_refund_parts` (refund_id, student_bill_id, amount_idr; UNIQUE per refund+tagihan)

Siklus up → down 1 → up lulus (versi 9, tidak dirty).

## Endpoint baru

Admin (JWT):
- `/admin/schemes/{id}/changes` (GET)
- Pengingat:
  - `/admin/bills/reminders` (POST)
  - `/admin/bills/reminder-log` (GET)
  - `/admin/billing/reminder-settings` (GET/PUT)
- `/admin/billing/followups` (GET/POST)
- Tagihan:
  - `/admin/bills/generate-annual` (POST)
  - `/admin/bills/{id}` (GET)
  - `/admin/bills/{id}/installments` (PUT)
  - `/admin/bills/{id}/change-requests` (POST)
  - `/admin/bill-change-requests` (GET)
  - `/admin/bills/{id}/invoice.pdf` (GET)
- Laporan:
  - `/admin/finance/aging` (GET)
  - `/admin/finance/receipts` (GET)
  - `/admin/finance/unallocated` (GET)
  - `/admin/students/{id}/ledger` (GET)
- Diskon/beasiswa:
  - `/admin/discounts` (POST, 202 + `Idempotency-Key`)
  - `/admin/discounts/{id}/deactivate` (POST)
- Impor pembayaran:
  - `/admin/payments/import` (POST multipart `file`, `source_name`, `dry_run`)
  - `/admin/payments/imports` (GET)

Ortu/siswa:
- `/parent/children/{studentUserId}/bills/{billId}/invoice.pdf`
- `/me/bills/{id}/invoice.pdf`

Internal (callback Approval Center):
- `/internal/schemes/changes/{id}/callback`
- `/internal/refunds/{id}/callback`
- `/internal/reconciliation/exceptions/{id}/callback`
- `/internal/bills/change-requests/{id}/callback`
- `/internal/discounts/{id}/callback`
- `/internal/bills/generate-annual`

Job baru: `bill_reminders`, `gateway_expiry`, `settlement_reconciliation`.

`/admin/finance/summary` sekarang juga mengembalikan `journal_failed_count` dan `journal_failed_since`.

## Verifikasi (dijalankan 2026-10-15)

```
migrate down 1 → 9/d billing_controls; up → 9/u billing_controls; version → 9
go vet ./...  → VET_OK
go test ./... (tanpa DB) → semua paket ok
TEST_PG_DSN=…skci_qa_bill_billing go test -count=1 -p 1 ./... →
  ok adhoc, canteen, client, common, gateway, jobs, model, nicepay, wallet
  FAIL handler: hanya TestPurposeChargeAndHook ("transaksi dengan Idempotency-Key ini sedang diproses")
                dan TestAdhocBillCheckout (fixture: "inconsistent types deduced for parameter $1")
web: npx vue-tsc -b → tanpa galat; npm run build → ✓ built in 24.57s
```

Dua tes yang gagal ada di `gateway_ext_test.go` (area QA-PAY). Keduanya sudah gagal sebelum perubahan QA-BILL:
- TestAdhocBillCheckout: fixture memakai `$1` sebagai uuid sekaligus text. Perbaikannya `$1::uuid` / `$1::text`.
- TestPurposeChargeAndHook: bergantung pada urutan dan sisa data.

TestParentCheckoutFlow kadang gagal (intermittent). Semua tes QA-BILL lulus.

## Langkah konfigurasi server (TIDAK dijalankan)

1. Env billing:
   - `CORE_URL` (Approval Center, exception, audit)
   - `BILLING_INTERNAL_URL` (dasar callback approval dan retry_url)
   - `NOTIFICATION_URL`, `ACCOUNTING_URL`, `ACADEMIC_URL`, `IDENTITY_URL`
   - kunci internal yang sudah ada
2. Jalankan migrasi billing sampai versi 9 (backup dulu).
3. Daftarkan request_type berikut di Approval Center core per sekolah, beserta aturan nominal dan approver-nya:
   - `fee_change`
   - `refund`
   - `recon_adjustment`
   - `payment_correction`
   - `bill_adjustment`
   - `discount_award`
4. Isi data per sekolah:
   - Mapping akun di menu Mapping Akun Jurnal (komponen, piutang, diskon, deposit, kas/bank). Tanpa mapping ini tagihan sengaja ditolak (BUG-050).
   - `billing_reminder_settings` (H-x/H/H+x) lewat UI Tunggakan → Pengaturan Pengingat.
   - `gateway_settings.receipt_resend_cooldown_minutes` (default 10).
   - `gateway_settings.journal_max_attempts` (default 5).
   - Toleransi rekonsiliasi bank.
5. Tinjau `kind` komponen skema (bulanan/tahunan/sekali bayar) dan termin bawaannya sebelum generate berikutnya. Data lama dianggap `monthly`/`one_time`.
6. Program diskon lama tetap aktif dengan status `active`, kecuali yang `active=false` (menjadi `inactive`). Program baru harus lewat approval.
7. Impor riwayat pembayaran:
   - Pembayaran impor (source `import`) tidak dijurnal. Diasumsikan sudah dibukukan sistem lama dan masuk akuntansi sebagai saldo awal.
   - Pastikan saldo awal piutang di akuntansi sudah sesuai sebelum impor.

## Kontrak ke area lain (perlu disambungkan)

- **identity (QA-IDN)**: `GET /students` masih 403 untuk `finance_manager` dan `kepala_yayasan`. Menu Detail Siswa, Refund (nama) dan Beasiswa (pilih siswa) membutuhkannya.
  - Kontrak: izinkan baca-saja `/students` (id, name, nis) untuk finance_manager/kepala_yayasan pada sekolahnya.
  - Sementara itu UI menampilkan pesan yang jelas, dan Beasiswa jatuh ke input ID.
- **core (QA-CORE)**:
  - Exception `journal_posting` setelah percobaan terakhir sekarang `severity=critical`.
  - Kontrak: peran akuntan (`accounting`/akuntan baru dari QA-IDN) dimasukkan ke `allowedExceptionKinds` untuk `journal_posting`, dan menu "Antrean Galat Integrasi" ditampilkan untuk finance_manager/akuntan.
  - Opsional: notifikasi in-app ke pemegang peran akuntan untuk exception `critical`. Ini butuh endpoint identity "users by role" yang belum ada.
- **academic**:
  - Atribut siswa `program`/`cohort`/`admission_track` untuk mencocokkan tarif berdimensi.
  - Pemanggilan `/internal/bills/generate-annual` saat siswa aktif di tengah tahun.
- **QA-PAY**:
  - `gateway_webhook.go:364` masih hardcode "Sekolah Teratai Putih". Ganti dengan `SchoolName` dari identity.
  - `InternalStudentRefund` sebaiknya lewat core approval seperti `/admin/refunds`.
  - transfer_billing: deposit pindahan bernilai 0. Perlu dipastikan sesuai PRD.

## Asumsi

- Persentase disimpan dalam basis poin (10000 = 100%) dan uang dalam rupiah bulat bigint.
- Periode berlaku program diskon dianggap berlaku bila beririsan dengan bulan tagihan.
- Impor riwayat bersifat all-or-nothing:
  - nominal per baris ≤ sisa tagihan
  - siswa dengan >1 tagihan pada periode yang sama wajib mengisi `bill_id`
  - impor ulang baris yang sama dilewati (idempotensi hash baris)
- Retry manual jurnal yang bertabrakan dengan posting yang sedang jalan dijawab 409 (tidak menunggu lalu memposting dua kali).
- Auditor tidak mendapat menu billing, karena backend tidak memberi akses keuangan ke auditor. Menu dan izin sudah selaras.

## Putaran 2

- **BUG-055 → FIXED**:
  - Program beasiswa dibuat di aplikasi dengan status `pending_approval`, lalu menjadi `active` lewat Approval Center `discount_award` (maker ≠ approver, replay 200, audit).
  - Program punya periode berlaku dan bisa dinonaktifkan dengan alasan.
  - Generate tagihan hanya menerapkan program aktif dalam periode.
  - UI Beasiswa punya form pengajuan, status, periode dan tombol Nonaktifkan.
- **BUG-122 → FIXED**:
  - Impor CSV riwayat pembayaran dengan pratinjau (dry run), all-or-nothing dan idempoten.
  - Status tagihan dihitung ulang, dan setiap impor diaudit.
  - Ada kartu Impor Riwayat Pembayaran di Verifikasi Pembayaran.
- **BUG-118/010 → FIXED**:
  - Retry manual memakai `FOR UPDATE NOWAIT` (409 bila sedang diproses).
  - Exception setelah percobaan terakhir bertanda `critical`.
  - Dashboard Keuangan menampilkan peringatan selama ada jurnal gagal.
- **BUG-136 + tambahan koordinator**:
  - Status `settlement_batches` menjadi `matched`/`mismatch` saat rekonsiliasi.
  - Job `gateway_expiry` mengakhiri transaksi pending yang lewat `expires_at` (idempoten, diaudit).
  - Job `settlement_reconciliation` berjalan harian dengan alert mismatch dan alert settlement yang belum datang.
- **BUG-129/130/132**: invoice PDF (admin/ortu/siswa), kartu piutang (saldo berjalan, deposit, termin) dan dana belum teralokasi (umur FIFO).
- **Audit tombol per peran**:
  - Setiap view keuangan, kasir, rekonsiliasi, refund, diskon, beasiswa dan aging ditelusuri dari panggilan service sampai guard backend.
  - Menu FINANCE_STAFF (admin, finance_manager, superadmin) hanya memanggil endpoint staff/viewer.
  - Menu FINANCE_VIEWERS (+kepala_sekolah, kepala_yayasan) hanya memanggil endpoint viewer: Dashboard (summary, receipts) dan Aging (aging, unallocated). Tautan ke Detail Siswa disembunyikan untuk viewer.
  - Kasir dan Rekonsiliasi memakai RequireCashier/RequireFinance(OrAccounting), yang mencakup admin dan FM.
  - Metode Pembayaran, Settlement dan Pengaturan Tagihan hanya untuk admin. Backend juga mengizinkan admin.
  - Auditor tidak punya menu billing.
  - Satu-satunya 403 yang tersisa berasal dari identity `/students` (kontrak QA-IDN di atas), dan UI sudah menampilkan pesan yang terbaca.

## Putaran 3

Merge `feat/QA-PAY` ke `feat/QA-BILL` bersih (tanpa konflik).

- **Refund mutasi/keluar (Menu 78, `POST /internal/students/{id}/refund`)**:
  - Refund tidak lagi ditempel ke tagihan terakhir. Nominal dipecah oleh `model.SplitStudentRefund`:
    1. porsi **deposit** lebih dulu, sebesar saldo `student_deposits` dikurangi porsi deposit refund lain yang masih pending;
    2. sisanya dari **saldo terbayar tagihan**, tagihan terbaru lebih dulu, dikurangi refund pending.
  - Hasilnya disimpan di `bill_refunds.deposit_idr` dan `bill_refund_parts` (`student_bill_id` = NULL).
  - Nominal yang melebihi deposit + saldo terbayar dijawab 422 tanpa menyimpan baris.
  - Respons 201, ringkasan approval dan audit memuat `deposit_idr` dan `bill_parts`.
- **Proses refund (`POST /admin/refunds/{id}/process`)**:
  - Porsi deposit mengurangi `student_deposits` (dikunci `FOR UPDATE`) dan menulis `deposit_movements` (−nominal, ref `refund:<id>`), sehingga muncul di kartu piutang.
  - Porsi deposit diaudit `deposit_refunded` dengan saldo before/after dan **tidak membalik pembayaran tagihan**.
  - Porsi tagihan membalik pembayaran tagihan masing-masing (terbaru lebih dulu), lalu status tagihan dihitung ulang.
  - Bila deposit sudah terpakai saat pembayaran keluar: 422, refund tetap `approved`, saldo tidak berubah.
  - Refund Menu 25 lama (`student_bill_id` terisi) tetap berjalan seperti sebelumnya.
  - Refund saldo dompet (`wallet_id`): lihat Putaran 4 (sekarang diproses tuntas).
- **Jurnal refund harian**: Dr akun refund (porsi tagihan) + Dr akun deposit siswa (porsi deposit) / Cr kas (total). Refund siswa tanpa `student_bill_id` kini ikut dijurnal. Bila ada porsi deposit, akun `deposit_acc` wajib dipetakan.
- **Refund pending** (bagian `bill_refund_parts`) ikut dihitung di `refundableQ` (Menu 25) dan `refundable_idr` financial summary.
- **Pengajuan ditolak/gagal di core** (alur `refund` belum dikonfigurasi, core tidak terjangkau): selain log dan penghapusan baris, billing mengirim item ke Integration Exception Queue core:
  - kind `callback`: kegagalan serah-terima antar layanan; daftar kind core adalah webhook/journal_posting/notification/dead_letter/import/callback.
  - severity `critical`: core hanya menerima `normal`/`critical`, jadi "high" dipetakan ke `critical`.
  - idempotency `refund-approval:<idempotency_key>`, payload berisi siswa, nominal, alasan dan request_type.
  - Bila core sama sekali tidak dikonfigurasi, hanya log yang ditulis.

Verifikasi putaran 3 (2026-10-15):

```
migrate down 1 / up / down 1 / up → versi 9, tidak dirty
go vet ./... → OK
go test ./... (tanpa DB) → semua paket ok
TEST_PG_DSN=…skci_qa_bill_billing go test -count=1 ./... → semua paket ok (termasuk handler; TestPurposeChargeAndHook dan TestAdhocBillCheckout kini lulus setelah merge QA-PAY)
web: tidak ada perubahan di putaran 3
```

## Putaran 4

Refund saldo dompet (FR-125-005: sisa saldo dikembalikan lewat menu 78/25, jadi ada di PRD; tombol tidak disembunyikan) kini diproses tuntas di billing:

- **Pengajuan** (`AdminCloseWallet`, disposisi `refund`):
  - Penutupan berstatus `pending_approval`, dompet `closing`. Refund Menu 25 (`wallet_id`) diajukan ke Approval Center core dengan request type `refund`, sama seperti refund lain: nominal ikut dikirim, maker = admin, callback `/internal/refunds/{id}/callback`.
  - Bila core menolak atau gagal: baris refund dihapus, penutupan `rejected`, dompet kembali `active`, dan item masuk Exception Queue (kind `callback`, severity `critical`).
- **Keputusan**: callback core dan keputusan lokal lama menyelaraskan penutupan dan dompet lewat `wallet.ClosureForRefund`:
  - requested → pending_approval/closing
  - approved → approved/closing
  - rejected → rejected/active (dompet bisa dipakai lagi)
  - processed → completed/closed
- **Proses** (`POST /admin/refunds/{id}/process`):
  - Dompet dikunci, lalu mutasi `refund_exit` sebesar −nominal ditulis (idempoten `wallet-refund:<refund_id>`), sehingga saldo menjadi 0. Penutupan menjadi `completed` dan dompet `closed`.
  - Audit `wallet_refunded` dicatat dengan saldo/status before/after.
  - Bila saldo dompet sudah berubah sejak pengajuan: 422, tidak ada yang berubah. Refund tersebut perlu ditolak, lalu dompet ditutup ulang.
- **Jurnal**: mutasi `refund_exit` dijurnal oleh jurnal harian dompet sebagai Dr liabilitas dompet (uang saku/tabungan) / Cr kas dompet. Jurnal refund tagihan tetap mengecualikan `wallet_id`, jadi tidak dobel.
- **UI Refund (menu 25)**:
  - Kolom/baris "Sumber" menampilkan "Saldo dompet uang saku/tabungan", "Deposit siswa", "Deposit + tagihan terbayar siswa" atau tagihan.
  - Riwayat proses dan form proses menjelaskan efeknya, misalnya "saldo dompet dikosongkan dan dompet ditutup".
  - `GET /admin/refunds` menambahkan `deposit_idr`, `wallet_id` dan `wallet_kind`.
- **Temuan untuk koordinator (tidak diubah)**:
  - Handler dompet T12 (`/admin/wallets`, `/admin/wallets/{id}/close`, `/internal/wallets/transfer-callback`, dst.) belum di-mount di `cmd/server/main.go` di branch mana pun, dan belum ada UI penutupan dompet. Endpoint penutupan sudah dites langsung lewat router tes.
  - Mount rute dan UI dompet adalah keputusan area T12/QA-PAY.

Verifikasi putaran 4 (2026-10-15):

```
migrate down 1 / up → versi 9, tidak dirty
go vet ./... → OK
go test ./... (tanpa DB) → semua paket ok
TEST_PG_DSN=…skci_qa_bill_billing go test -count=1 ./... → semua paket ok
web: npx vue-tsc -b && npm run build → vue-tsc tanpa galat; ✓ built in 20.64s
```
