# T6: Payment gateway NICEPay di service `billing`: checkout, metode, transaksi & webhook, kuitansi, kasir, settlement, rekonsiliasi, dashboard

**Menu PRD:** 26–31 (BR-FIN-01..05, BR-PAY-01/02, IF-PG-*, AC-IF-01/02/03, NFR-PERF-03, NFR-SEC-05, NFR-REL-04) · **Branch:** `feat/T6-payment` · **Service:** `billing` · **Migrasi:** `services/billing/migrations/0003_gateway.*` (nomor 0003 milik T6, 0004 milik T7)
**Prasyarat:** kontrak T1 (exception, approval, audit, jurnal accounting §7b). Boleh dikerjakan **paralel** dengan T1: tulis client sesuai kontrak T1 §3 dan uji dengan `httptest` fake. Integrasi nyata diuji setelah T1 merge.
**Feature key:** `payment_gateway`. Jika mati, alur upload bukti transfer yang lama tetap berjalan tanpa berubah.
**Driver DB:** ikuti billing (`database/sql`), jangan campur pgx.

## 1. Donor: senb-v1 (NICEPay SNAP, sudah teruji sandbox & produksi)
| Ambil | Dari | Penyesuaian |
|---|---|---|
| Klien SNAP: token B2B, signature asimetris/simetris, VA, QRIS, e-wallet, status, cancel VA, verifikasi notifikasi | `senb-v1/backend/internal/payment/nicepay/client.go` + `client_test.go` | salin ke `services/billing/internal/nicepay/`. **`Amount(float64)` → `Amount(int64 rupiah)`** = `fmt.Sprintf("%d.00", v)`; hapus semua float. Kredensial (`Config`) dari env, **tanpa default**. Hapus kode spesifik toko (cartData item SeNB → item tagihan) |
| Callback: verifikasi signature → cek status ke API (jangan percaya body) → update idempoten | `senb-v1/backend/internal/usecase/payment_usecase.go` (`Callback`) + `payment_usecase_test.go`, `handler/payment_handler.go` (`Callback`, kode respons SNAP) | chi, dan alokasi ke `bill_payment_allocations` lewat fungsi billing yang sudah ada (`model.ValidateAllocations`, `recomputeBillStatusQ`) |

Tidak membuat interface `Provider` (satu implementasi = YAGNI, GEMINI.md §2). Nama paket `nicepay` cukup.

## 2. Migrasi `0003_gateway.up.sql`
```
payment_methods(id, school_id, code text /* 'va_bni','va_mandiri','qris','ewallet_ovo',… */, kind CHECK IN('va','qris','ewallet'),
  provider_code text /* bankCd / mitraCd NICEPay */, label text, enabled bool,
  fee_kind CHECK IN('flat','percent'), fee_flat_idr bigint default 0, fee_bp int default 0,
  fee_bearer CHECK IN('parent','school'),                    -- BR-PAY-02 per metode
  valid_minutes int NOT NULL, sort int, UNIQUE(school_id, code))
gateway_transactions(id, school_id, tx_no text NOT NULL /* 'TX-' + urutan per sekolah */, payer_user_id, student_user_id,
  method_id, bills_amount_idr bigint, fee_idr bigint, total_idr bigint, fee_bearer,
  status CHECK IN('pending','success','failed','expired','cancelled'),
  provider_ref text NULL, va_number text NULL, qr_content text NULL, redirect_url NULL, expires_at, paid_at NULL,
  settlement_batch_id NULL, idempotency_key, UNIQUE(school_id, tx_no), UNIQUE(school_id, idempotency_key), UNIQUE(provider_ref))
gateway_transaction_bills(transaction_id, bill_id, amount_idr bigint, PRIMARY KEY(transaction_id, bill_id))
gateway_notifications(id, school_id NULL, provider_ref, signature_valid bool, raw_hash text, body_masked jsonb,
  processed_at NULL, result text, received_at, UNIQUE(provider_ref, raw_hash))       -- BR-PAY-01: notifikasi sama diproses sekali
receipts(id, school_id, receipt_no text, source CHECK IN('gateway','cashier'), source_id, object_key, sent_at NULL, resend_count int,
  UNIQUE(school_id, receipt_no))
receipt_sequences(school_id, year int, last int, PRIMARY KEY(school_id, year))            -- nomor unik tanpa celah, ambil dengan UPDATE … RETURNING
cashier_payments(id, school_id, student_user_id, method CHECK IN('cash','transfer'), amount_idr, received_by, note,
  status CHECK IN('recorded','corrected','cancelled'), corrected_by_id NULL, idempotency_key)
payment_corrections(id, school_id, source CHECK IN('gateway','cashier','manual'), source_id, kind CHECK IN('reassign','cancel'),
  target_student_user_id NULL, reason, status, approval_id)                               -- BR-FIN-05, AC-29-04
settlement_batches(id, school_id, provider, channel, settled_on date, gross_idr, fee_idr, net_idr, bank_account text,
  status CHECK IN('reported','matched','mismatch'), source_file_key NULL, UNIQUE(school_id, provider, channel, settled_on))
settlement_items(id, school_id, batch_id, provider_ref, amount_idr, fee_idr, matched_transaction_id NULL)
reconciliation_exceptions(id, school_id, kind CHECK IN('missing_in_settlement','missing_in_sekoci','amount_mismatch','bank_unmatched'),
  ref, detail jsonb, status CHECK IN('open','resolved'), resolution_note, adjustment_approval_id NULL)
bank_statement_lines(id, school_id, account text, posted_on date, amount_idr bigint, description, matched_ref NULL, import_id)
gateway_settings(school_id PK, auto_allocation CHECK IN('earliest_due','component_priority','payer_choice') default 'earliest_due',
  receipt_deadline_minutes int default 5, sub_account_mode bool default false, sub_account_id text NULL)
```
**Deposit (BR-FIN-04)**: cek dulu apakah billing sudah punya mekanisme kelebihan bayar. Kalau belum ada, tambahkan `student_deposits(school_id, student_user_id, balance_idr, PRIMARY KEY(...))` + `deposit_movements` di migrasi yang sama, dan alokasikan deposit otomatis saat tagihan terbit (`generateBills`).

## 3. Fungsi murni + vektor
| Fungsi | Vektor |
|---|---|
| `Fee(method, amount) int64` | flat & percent (floor) |
| `Totals(bills, method)` | **AC-28-02** QRIS ditanggung orang tua: total = 1.000.000 + fee, alokasi tagihan 1.000.000; ditanggung sekolah: total 1.000.000, fee dicatat beban |
| `Allocate(amount, openBills, rule)` | BR-FIN-03 earliest_due; sisa → deposit (BR-FIN-04); payer_choice = tagihan yang dipilih |
| `NextReceiptNo(school, year)` | unik, tanpa celah, aman konkuren (test 50 goroutine) |
| `Reconcile(successTx, settlementItems)` | **AC-31-01** TX-012 tanpa pasangan → `missing_in_settlement`; selisih nominal → mismatch |
| `MethodShare(txs)` | **AC-26-01** VA 40 / QRIS 10 → 8000 bp / 2000 bp |

## 4. Endpoint
| Method & path | Peran | Perilaku |
|---|---|---|
| `GET /payment-methods` (aktif) · `CRUD /admin/payment-methods` | orang_tua (baca); admin, finance_manager (tulis) | AC-28-01 |
| `POST /parent/checkout/preview` `{student_user_id, bill_ids, method_id}` | orang_tua (anak sendiri, pakai `requireChild` yang ada) | ringkasan + fee sebelum konfirmasi (FR-28-002) |
| `POST /parent/checkout` (+ header `Idempotency-Key`) | orang_tua | satu transaksi untuk banyak tagihan (**AC-27-01**: 500k+300k → 1 VA 800k). Tagihan yang sedang punya transaksi `pending` → 409 dengan tautan transaksi itu |
| `GET /parent/transactions/{id}` | orang_tua | halaman hasil + instruksi (FR-27-002). Status `pending` dan sudah lewat `expires_at` → cek status ke API lalu set `expired` |
| `POST /parent/transactions/{id}/renew` | orang_tua | **AC-27-02**: expired → cancel VA lama (best effort) + buat transaksi baru |
| **Webhook** `POST /webhooks/nicepay` (tanpa JWT, path di luar grup auth) | — | (1) baca body maks 64KB; (2) verifikasi signature → tidak valid: 401, simpan `gateway_notifications(signature_valid=false)`, `POST core /internal/audit` module `security` (**AC-IF-01**), status tidak berubah; (3) dedup `(provider_ref, raw_hash)`; (4) **re-query `Status`** ke NICEPay; (5) dalam satu transaksi DB `SELECT … FOR UPDATE` → kalau sudah `success`, return 200 tanpa alokasi (**AC-IF-02**), selain itu alokasi + `success`; (6) enqueue kuitansi. Target ≤ 60 detik (NFR-PERF-03). Gagal internal → 500 (NICEPay retry) + `/internal/exceptions` kind `webhook` |
| `GET /admin/transactions?status=&method=&from=&to=&q=` · `/{id}` | admin, finance_manager | FR-29-001 |
| `POST /admin/receipts/{id}/resend` | admin, finance_manager | FR-29-003 |
| `POST /cashier/payments` · `GET /cashier/payments` · `GET /cashier/payments/{id}/receipt.pdf` | admin, kasir (peran admin, cek apakah ada peran kasir khusus di identity; kalau tidak ada, pakai admin) | **AC-29-03**: alokasi BR-FIN-03 + nomor kuitansi unik |
| `POST /admin/payment-corrections` · callback approval | admin (ajukan), finance_manager (setujui via core `payment_correction`) | **AC-29-04**: asal ditandai `corrected`, tidak dihapus; reverse alokasi pakai `bill_payment_reversals` yang sudah ada |
| `POST /admin/settlements/import` (CSV laporan settlement NICEPay) · `GET /admin/settlements` | finance_manager | FR-30-001 |
| `POST /admin/reconciliation/run?date=` (juga job harian) · `GET /admin/reconciliation/exceptions` · `POST /…/{id}/resolve` | finance_manager | resolve dengan jurnal penyesuaian → approval `recon_adjustment` (FR-31-004) |
| `POST /admin/bank-statements/import` · `POST /admin/bank-reconciliation/run` | finance_manager, accounting | AC-31-02 (cocok berdasarkan nominal + tanggal ± toleransi dari setting + referensi) |
| `GET /admin/payment-dashboard?from=&to=` | finance_manager, admin | FR-26-001: count & nilai per status, distribusi metode, tren harian, status settlement |

**Kuitansi:** PDF (gofpdf, template mengikuti T3 slip bila sudah ada, kalau belum buat sederhana), simpan di storage billing, kirim lewat `/internal/notifications` (in-app + email) dengan lampiran tautan. Job tiap menit: transaksi `success` tanpa kuitansi → buat + kirim (AC-29-02 ≤ 5 menit).
**Jurnal:** harian per sekolah, kirim ke `POST /api/internal/journals` (T1 §7b) dengan reference `pgw:<school>:<date>` (debit kas/bank gateway + beban fee jika ditanggung sekolah, kredit piutang). Akun dari tabel mapping billing; kalau belum ada, buat `gateway_accounts(school_id PK, clearing_acc, fee_expense_acc, receivable_acc)`. Gagal → exception `journal_posting` + retry (**AC-IF-03**).
**Settlement sub-akun (FR-30-002, Should):** kalau `sub_account_mode`, kirim `sub_account_id` di request NICEPay. Kalau API NICEPay di donor tidak mendukungnya, tulis di handoff sebagai item terbuka. Jangan mengarang parameter API.

## 5. Frontend
| Path | View | Peran | Catatan |
|---|---|---|---|
| (perluas) `views/parent/TagihanListView.vue` & `TagihanDetailView.vue` | — | orang_tua | checkbox pilih tagihan (hanya yang belum lunas) + sticky bar "Bayar (n) · Rp…" → `ParentCheckoutView`. Tombol bayar hanya jika fitur aktif |
| `orang-tua/checkout` | `views/parent/ParentCheckoutView.vue` | orang_tua | ringkasan tagihan, pilih metode (kartu radio dengan logo **teks**, tanpa mengunduh logo bank yang tidak resmi), rincian fee, tombol "Bayar" (disabled saat proses) |
| `orang-tua/transaksi/:id` | `views/parent/ParentTransactionView.vue` | orang_tua | status besar (Menunggu/Berhasil/Gagal/Kedaluwarsa, ikon + teks), VA dengan tombol salin, QR (render dari `qr_content` dengan lib QR yang sudah ada di repo, cek dulu, kalau tidak ada pakai `qrcode` npm versi tetap), countdown kedaluwarsa, polling 5 detik selama pending (berhenti saat tab tersembunyi), tombol "Buat kode baru" bila kedaluwarsa, unduh kuitansi bila sukses |
| `keuangan/transaksi` | `views/billing/TransactionsView.vue` | admin, finance_manager | daftar + filter + detail drawer + kirim ulang kuitansi + ajukan koreksi |
| `keuangan/kasir` | `views/billing/CashierView.vue` | admin | cari siswa → tagihan terbuka → nominal (input rupiah) → metode → simpan → cetak kuitansi (print CSS) |
| `keuangan/metode-pembayaran` | `views/billing/PaymentMethodsView.vue` | admin, finance_manager | aktif/nonaktif, fee, penanggung, masa berlaku |
| `keuangan/settlement` | `views/billing/SettlementView.vue` | finance_manager | import + daftar batch |
| (perluas) `PaymentReconciliationView.vue` | — | finance_manager | tab Gateway vs Settlement / Mutasi Bank, daftar pengecualian + resolve |
| (perluas) `PaymentDashboardView.vue` | — | finance_manager, admin | StatCard per status, bar distribusi metode (persen + angka, ada tabel alternatif), tren, settlement |
Semua route baru `meta.feature: 'payment_gateway'`. Perluasan view lama harus menyembunyikan bagian gateway saat fitur mati.

## 6. Env
`NICEPAY_BASE_URL, NICEPAY_CLIENT_ID, NICEPAY_PARTNER_ID, NICEPAY_CHANNEL_ID, NICEPAY_PRIVATE_KEY, NICEPAY_PUBLIC_KEY, NICEPAY_CLIENT_SECRET, NICEPAY_CALLBACK_URL` (nama mengikuti `Config` donor). Di `.env.example`, isi kosong + komentar sandbox. **Jangan** salin kredensial dari repo senb-v1.

## 7. Bukti handoff
Test donor `client_test.go` dan callback test hijau setelah port. Vektor §3; AC-26-01, 27-01/02, 28-01/02, 29-01..04, 31-01/02, AC-IF-01/02/03; test float tidak ada (`grep -rn "float" internal/nicepay` kosong kecuali parsing respons). Playwright dengan **NICEPay sandbox** jika kredensial tersedia di `.env` lokal; kalau tidak, server mock lokal `scripts/mock-nicepay` yang mengirim webhook bertanda tangan valid. Tulis mana yang dipakai.
