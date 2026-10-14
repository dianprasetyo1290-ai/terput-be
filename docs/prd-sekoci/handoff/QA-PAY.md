# Handoff QA-PAY: Payment gateway (menu 26–30) & Mutasi Siswa (menu 78)

Branch `feat/QA-PAY` (base `b984872`). Ada 3 commit kode dan 1 commit handoff ini. Tidak ada push, deploy, atau SSH.

## Status per temuan

| BUG | Status | File utama | Bukti |
|---|---|---|---|
| BUG-004 [Kritis] Webhook tanpa verifikasi tanda tangan (adapter Sim aktif) | FIXED | `services/billing/internal/config/{gateway,config}.go`, `gateway/sim.go`, `handler/gateway_webhook.go`, `handler/gateway_charge.go`, `handler/autodebit.go`, `handler/gateway_providers.go`, `.env.example` | `TestWebhookFailClosed`, `TestSimWebhookRequiresSignature`, `TestSimulatorGuards` |
| BUG-001 [Kritis] Metode checkout kosong / kredensial NICEPay belum ada | NOT-A-CODE-BUG + UI/UX FIXED | `handler/gateway_methods.go`, `handler/gateway_checkout.go`, `web/src/views/billing/PaymentMethodsView.vue`, `web/src/views/parent/ParentCheckoutView.vue`, `gateway.types.ts` | `TestPaymentMethodReadiness`, `TestMethodReadyReason` |
| BUG-008 [Kritis] Status lunas tidak sesuai nominal terbayar | PARTIAL (jalur gateway terverifikasi aman; akar masalah di QA-BILL) | Tidak ada perubahan; dicek di `handler/gateway_webhook.go` `recomputeBillStatusInTx` | lihat catatan di bawah |
| BUG-057 [Tinggi] Pengajuan menu 78 tidak sampai ke Approval Center, tidak bisa dibatalkan | FIXED (kode) + perlu konfigurasi server | `services/academic/internal/handler/transfer.go`, `client/core.go`, `store/transfer.go`, `cmd/server/main.go`, `web/.../StudentStatusRequestDetailView.vue` | `TestMenu78SubmitFailClosed`, `TestMenu78ActiveRequestLifecycle` |
| BUG-137 [Sedang] Pengajuan ganda untuk siswa sama diterima | FIXED | migrasi academic `0007`, `store/transfer.go`, `handler/transfer.go`, `StudentStatusRequestFormView.vue` | `TestMenu78ActiveRequestLifecycle` (409 + `existing_request_id` + `link`) |
| BUG-176 [Rendah] Tanggal efektif tidak divalidasi terhadap tanggal masuk | FIXED | `model/transfer.go` `ValidateEffectiveDateAfterEnrollment`, `store.GetActiveStudentEnrollment` (+`enrolled_at`) | `TestValidateEffectiveDateAfterEnrollment`, `TestMenu78ActiveRequestLifecycle` |
| BUG-177 [Rendah] Super Admin bisa akses Mutasi Siswa | FIXED | `model/transfer.go` (matriks peran), `handler/transfer.go`, `web/src/router/domains/academic.routes.ts`, `StudentStatusRequestsView.vue` | `TestMenu78RoleMatrix`, `TestMenu78SuperadminForbidden` |

## Rincian perubahan

### BUG-004: webhook fail-closed
- Adapter Sim hanya dipakai bila `PAYMENT_SIMULATOR_ENABLED=true` dan `APP_ENV` bukan production. Di production, `config.Load` menolak start bila flag ini aktif. Sim butuh `PAYMENT_SIM_WEBHOOK_SECRET` dengan panjang minimal 16 karakter, dan setiap webhook Sim wajib membawa `X-SIGNATURE` = hex HMAC-SHA256(secret, body).
- Sim tidak pernah menjadi fallback. Bila NICEPay tidak punya kredensial, provider dianggap tidak tersedia: webhook dijawab 503, transaksi tetap `pending`.
- Webhook yang ditolak (401 untuk tanda tangan salah, 503 untuk provider tidak tersedia) dicatat ke `gateway_notifications` dengan `signature_valid=false` dan `result=invalid_signature|provider_unavailable`. Penolakan juga dikirim ke audit core (module `security`, action `webhook_*`) dan ditulis ke log.
- Salinan palsu tidak bisa memblokir notifikasi asli. Insert yang valid memakai `ON CONFLICT ... DO UPDATE ... WHERE signature_valid=false`.
- Webhook mandate autodebit memakai jalur penolakan yang sama.
- Membuat provider `adapter=sim` lewat API ditolak bila simulator tidak diizinkan.
- Perbaikan sampingan: `newTxNo` sekarang berformat `TX-<yymmddhhmmss><6 digit acak crypto>`. Suffix lama `Nanosecond()%1000` selalu 0 di macOS, sehingga terjadi tabrakan unik yang muncul sebagai 409 "sedang diproses" di test.

### BUG-001: checkout kosong
Penyebab di server adalah **konfigurasi**: sekolah belum punya baris `payment_methods` dan env `NICEPAY_*` kosong. Perubahan di kode:
- `GET /payment-methods` (orang tua) hanya mengembalikan metode yang siap dipakai, yaitu metode yang punya route dan providernya punya kredensial. Hasilnya selalu array JSON.
- Preview dan checkout pada metode yang belum siap dijawab 503 dengan pesan "Pembayaran online belum tersedia: Kredensial payment gateway untuk provider nicepay belum terpasang di server..." dan tidak membuat transaksi. Metode yang tidak punya route tetap dijawab 422.
- `GET /admin/payment-methods` menambahkan field `ready` dan `not_ready_reason`. UI admin (menu Metode Pembayaran) menampilkan kolom "Kesiapan" dan banner peringatan.
- UI orang tua menampilkan empty state "Pembayaran online belum tersedia" dengan saran transfer manual, serta tombol "Coba Lagi" saat galat.

### BUG-008: jalur webhook/checkout
Jalur gateway tidak menandai lunas tanpa uang masuk:
- Webhook hanya diproses setelah tanda tangan valid (BUG-004).
- Settlement membuat `bill_payments`/alokasi sesuai nominal transaksi, lalu `recomputeBillStatusInTx` menghitung `paid = Σ alokasi verified − Σ reversal`, dan status diturunkan oleh `model.DeriveBillStatus(net, paid)`. Bila `paid <= 0` hasilnya `unpaid`, bila `paid >= net` hasilnya `paid`.

Akar BUG-008 (kasir, rekonsiliasi, tagihan) menjadi wilayah QA-BILL, jadi file tersebut tidak disentuh di sini.

### BUG-057 / 137 / 176 / 177: menu 78 (academic)
- **Submit fail-closed.** Client core selalu di-wire lewat `handler.New` → `client.NewCoreClient()` (env `CORE_BASE_URL`, `ACADEMIC_INTERNAL_URL`, `SEKOCI_INTERNAL_KEY`). Baris pengajuan hanya dipertahankan bila core menerima approval. Bila gagal, baris dihapus dan respons dipilih sebagai berikut:
  - core 4xx selain 401/403 → **422** dengan pesan core, misalnya "alur approval student_withdrawal belum dikonfigurasi", ditambah petunjuk untuk mengatur menu Alur Approval.
  - core 5xx, 401/403 (internal key salah), atau tidak terjangkau → **502**.
  - client core tidak ada → **503**.
- `CoreClient.SubmitApproval` sekarang mengembalikan `*client.CoreError{Status, Message}`, dan ada metode baru `CancelApproval`.
- **Endpoint baru `POST /student-status-requests/{id}/cancel`** dengan body opsional `{ "reason": "..." }`:
  - Hanya pengaju (`requested_by == sub`) yang boleh membatalkan, dan hanya saat status `pending_approval`. Selain pengaju dijawab 403, status lain dijawab 409.
  - Approval di core dibatalkan dulu lewat `/internal/approvals/{id}/cancel`. Bila core menjawab 409, respons 409 dan status lokal tidak berubah. Bila core tidak bisa dihubungi, respons 502.
  - Setelah itu dilakukan CAS `pending_approval → cancelled` dan audit `request_cancelled`.
  - UI: tombol "Batalkan Pengajuan" dengan dialog konfirmasi di halaman detail.
- **Satu pengajuan aktif per siswa.** Status aktif adalah `pending_approval` atau `approved`. Pengecekan dilakukan di handler, ditambah unique index parsial `uq_student_status_requests_active` untuk menangani race. Duplikat dijawab 409 dengan `data: {existing_request_id, existing_status, existing_kind, link: "/akademik/mutasi-siswa/<id>"}`, dan form menampilkan tautan "Lihat pengajuan yang sedang berjalan". Replay dengan `idempotency_key` yang sama tetap 201 dan mengembalikan baris asli.
- **Tanggal efektif ≥ tanggal masuk kelas aktif** (`class_enrollments.enrolled_at`, dibandingkan per tanggal WIB). Bila dilanggar, respons 422 "tanggal efektif tidak boleh sebelum tanggal masuk siswa di kelas aktif (YYYY-MM-DD)".
- **Callback approval** kini dijaga transisi status:
  - Replay keputusan yang sama dijawab 200 (idempoten).
  - Keputusan setelah pengajuan dibatalkan atau sudah diputus berbeda dijawab 409.
  - Update memakai compare-and-set.
- **Matriks peran.** API dan route SPA memakai aturan berikut:

  | Akses | Peran |
  |---|---|
  | Lihat (list/detail/ringkasan keuangan) | admin, walikelas, kepala_sekolah |
  | Ajukan / batalkan | admin, walikelas |
  | Terapkan | admin |

  Superadmin ditolak 403 di API dan menu tidak tampil. Tombol "Ajukan Mutasi" disembunyikan untuk kepala_sekolah.
- Halaman detail dan form kini menampilkan pesan galat dari server (`apiErrorMessage`), bukan lagi "Request failed with status code ...".

## Migrasi & endpoint baru
- **academic `0007_status_request_active_unique`**
  - up: pengajuan aktif ganda yang sudah ada dibatalkan (yang tertua dipertahankan), lalu dibuat `CREATE UNIQUE INDEX uq_student_status_requests_active ON student_status_requests (school_id, student_user_id) WHERE status IN ('pending_approval','approved')`.
  - down: `DROP INDEX`. Baris yang dibatalkan oleh up tidak dikembalikan.
- billing: **tidak ada migrasi baru**. Nomor 0010 tidak terpakai.
- Endpoint baru: `POST /api/academic/student-status-requests/{id}/cancel` (JWT).
- Field respons baru: `ready` dan `not_ready_reason` pada `GET /admin/payment-methods`.
- Env baru (billing):
  - `PAYMENT_SIMULATOR_ENABLED` (default false, dilarang di production)
  - `PAYMENT_SIM_WEBHOOK_SECRET` (minimal 16 karakter bila simulator aktif)

## Verifikasi
```
# academic  (TEST_PG_DSN=postgres://localhost/skci_qa_pay_academic?sslmode=disable)
migrate up → 7/u ; down 1 → 7/d ; up → 7/u ; version → 7
go vet ./...            → bersih
go test ./... -count=1  → ok common, handler, model, store
  (baru: TestMenu78SuperadminForbidden, TestMenu78SubmitFailClosed, TestMenu78ActiveRequestLifecycle,
   TestMenu78RoleMatrix, TestIsActiveStatusRequest, TestValidateEffectiveDateAfterEnrollment, TestCheckCancelStatusRequest)

# billing   (TEST_PG_DSN=postgres://localhost/skci_qa_pay_billing?sslmode=disable)
go vet ./...            → bersih
go test ./... -count=1  → ok adhoc, canteen, client, common, config, gateway, handler, jobs, model, nicepay, wallet
  (baru: TestWebhookFailClosed, TestPaymentMethodReadiness, TestMethodReadyReason, TestSimWebhookRequiresSignature, TestSimulatorGuards)

# web
npm ci ; npx vue-tsc -b → tanpa galat ; npm run build → exit 0 (✓ built; hanya peringatan ukuran chunk yang sudah ada)
```

## Langkah konfigurasi server (JANGAN dieksekusi oleh agen, untuk tim ops)
1. **Kredensial NICEPay (billing):** isi `NICEPAY_BASE_URL`, `NICEPAY_CLIENT_ID`, `NICEPAY_CHANNEL_ID`, `NICEPAY_CLIENT_SECRET`, `NICEPAY_PRIVATE_KEY`, `NICEPAY_PUBLIC_KEY`, `NICEPAY_STORE_ID`, `NICEPAY_CALLBACK_URL`, `NICEPAY_RETURN_URL` (lihat `.env.example`; provider lain memakai prefix `<credential_ref>_*`) di env billing produksi, lalu restart billing. Pastikan `PAYMENT_SIMULATOR_ENABLED` **tidak** di-set di production, karena service akan menolak start.
2. **Metode pembayaran per sekolah:** sebagai admin sekolah, buka menu Metode Pembayaran, tambahkan metode (VA per bank, QRIS, dan seterusnya), lalu pastikan kolom "Kesiapan" menunjukkan "Siap dipakai". Bila masih "Belum siap", baca alasannya (kredensial atau routing).
3. **Alur approval menu 78 (core, menu 79 Alur Approval):** per sekolah, buat flow untuk `student_transfer` dan `student_withdrawal`. Sertakan langkah keuangan (bendahara/keuangan) untuk flag `has_outstanding`/`has_deposit` sesuai AC-78-01. Tanpa flow, pengajuan sekarang ditolak 422 dengan pesan jelas.
4. **Wiring academic → core:** cek env academic:
   - `CORE_BASE_URL`: URL internal core, default `http://core:8091`.
   - `ACADEMIC_INTERNAL_URL`: URL academic yang dapat dijangkau core untuk callback, default `http://academic:8082`.
   - `SEKOCI_INTERNAL_KEY`: harus sama dengan core.

   Bila salah, pengajuan dijawab 502 dan alasannya ada di log academic (`submit approval: ...`).
5. **Migrasi:** jalankan migrasi academic sampai versi 7 saat deploy. Up akan membatalkan pengajuan aktif ganda lama, jadi backup DB dulu.
6. **Pengajuan lama yang tertahan `pending_approval` tanpa `approval_id`:** setelah flow dikonfigurasi, pengaju dapat membatalkannya lewat tombol Batalkan (tanpa `approval_id`, core tidak dipanggil) lalu mengajukan ulang.
7. **Dev/Playwright dengan simulator:** set `PAYMENT_SIMULATOR_ENABLED=true` dan `PAYMENT_SIM_WEBHOOK_SECRET=<≥16 karakter>` (di luar production), lalu tanda tangani setiap webhook dengan `X-SIGNATURE: hex(HMAC-SHA256(secret, body))`. Alternatifnya, pakai `scripts/mock-nicepay` (RSA, jalur NICEPay asli).

## Asumsi (default konservatif)
- "Tanggal masuk" diartikan sebagai `enrolled_at` enrollment kelas **aktif**. Bila kenaikan kelas dijalankan terlambat, tanggal efektif sebelum tanggal kenaikan akan ditolak. Ini lebih ketat dan aman.
- "Pengajuan aktif" adalah `pending_approval` atau `approved` yang belum diterapkan.
- Pembatalan hanya oleh pengaju, tidak oleh admin lain. Alasan bersifat opsional, dengan default "Dibatalkan oleh pengaju".
- Kepala sekolah hanya bisa melihat menu 78. Persetujuannya berjalan lewat Approval Center.
- Endpoint `/academic-settings` (kebijakan tagihan pasca keluar) **tidak** diubah dan masih admin/superadmin, karena dipakai juga di luar menu 78.
- Bila core tidak punya approval-nya (404 saat cancel), pembatalan lokal tetap dilanjutkan.

## Pertanyaan terbuka / untuk review
- Apakah peran keuangan/bendahara perlu melihat menu 78? Saat ini mereka hanya terlibat lewat Approval Center.
- Belum ada UI admin untuk provider dan routing gateway (T14 backend-only), sehingga konfigurasi provider dilakukan via API atau seed. Perlu tiket UI terpisah?
- Format `tx_no` berubah menjadi 18 digit setelah `TX-`. Perlu dicek bila ada laporan atau rekonsiliasi yang mem-parsing panjangnya.
- Respons GET menu 78 masih di-cache Redis selama 15 detik (`respCache`), jadi daftar bisa terlambat ≤15 detik setelah batal atau ajukan. Perilaku ini sudah ada sebelumnya dan tidak diubah.
- Walikelas masih bisa melihat seluruh pengajuan sekolah di list. Scoping per kelas belum diminta.
