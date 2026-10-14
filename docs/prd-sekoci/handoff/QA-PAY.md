# Handoff QA-PAY: Payment gateway (menu 26–30) & Mutasi Siswa (menu 78)

Branch `feat/QA-PAY` (base `b984872`). Putaran 1: 3 commit kode + 1 commit handoff. Putaran 2: 8 commit kode + 1 commit handoff (lihat bagian "Putaran 2"). Tidak ada push, deploy, atau SSH.

## Status per temuan

| BUG | Status | File utama | Bukti |
|---|---|---|---|
| BUG-004 [Kritis] Webhook tanpa verifikasi tanda tangan (adapter Sim aktif) | FIXED | `services/billing/internal/config/{gateway,config}.go`, `gateway/sim.go`, `handler/gateway_webhook.go`, `handler/gateway_charge.go`, `handler/autodebit.go`, `handler/gateway_providers.go`, `.env.example` | `TestWebhookFailClosed`, `TestSimWebhookRequiresSignature`, `TestSimulatorGuards` |
| BUG-001 [Kritis] Metode checkout kosong / kredensial NICEPay belum ada | NOT-A-CODE-BUG + UI/UX FIXED | `handler/gateway_methods.go`, `handler/gateway_checkout.go`, `web/src/views/billing/PaymentMethodsView.vue`, `web/src/views/parent/ParentCheckoutView.vue`, `gateway.types.ts` | `TestPaymentMethodReadiness`, `TestMethodReadyReason` |
| BUG-008 [Kritis] Status lunas tidak sesuai nominal terbayar | FIXED (jalur gateway diverifikasi di sini; akar kasir/tagihan diperbaiki QA-BILL `c365cf2`) | `handler/gateway_webhook.go` `recomputeBillStatusInTx` (tanpa perubahan) | lihat catatan di bawah |
| BUG-057 [Tinggi] Pengajuan menu 78 tidak sampai ke Approval Center, tidak bisa dibatalkan | FIXED (kode) + perlu konfigurasi server | `services/academic/internal/handler/transfer.go`, `client/core.go`, `store/transfer.go`, `cmd/server/main.go`, `web/.../StudentStatusRequestDetailView.vue` | `TestMenu78SubmitFailClosed`, `TestMenu78ActiveRequestLifecycle` |
| BUG-137 [Sedang] Pengajuan ganda untuk siswa sama diterima | FIXED | migrasi academic `0007`, `store/transfer.go`, `handler/transfer.go`, `StudentStatusRequestFormView.vue` | `TestMenu78ActiveRequestLifecycle` (409 + `existing_request_id` + `link`) |
| BUG-176 [Rendah] Tanggal efektif tidak divalidasi terhadap tanggal masuk | FIXED | `model/transfer.go` `ValidateEffectiveDateAfterEnrollment`, `store.GetActiveStudentEnrollment` (+`enrolled_at`) | `TestValidateEffectiveDateAfterEnrollment`, `TestMenu78ActiveRequestLifecycle` |
| BUG-177 [Rendah] Super Admin bisa akses Mutasi Siswa | FIXED | `model/transfer.go` (matriks peran), `handler/transfer.go`, `web/src/router/domains/academic.routes.ts`, `StudentStatusRequestsView.vue` | `TestMenu78RoleMatrix`, `TestMenu78SuperadminForbidden` |
| P2-1 UI admin provider & routing gateway belum ada | FIXED | `web/src/views/billing/PaymentGatewaySettingsView.vue` (baru), `services/billing/gateway-admin.service.ts` (baru), `handler/gateway_providers.go` | `TestProviderOptionsShowReadiness`, vue-tsc/build |
| P2-2 Walikelas melihat semua pengajuan menu 78 | FIXED | `academic store/transfer.go`, `handler/transfer.go`, `internal/cache/cache.go` | `TestMenu78WaliKelasOwnClassOnly` |
| P2-3 Audit tombol (dashboard, checkout, metode, transaksi, settlement, mutasi, menu 81, menu 82) | FIXED | lihat "Putaran 2" | `TestServiceRequestHandlerOnly`, `TestNormalizeAttachments`, `TestMenu78Attachments`, `TestRenewOverduePending` |
| P2-4 Checkout tanpa kredensial NICEPay | FIXED (state "belum dikonfigurasi") | `ParentCheckoutView.vue`, `ParentTransactionView.vue`, `PaymentGatewaySettingsView.vue` | 503 → kartu "Pembayaran online belum dikonfigurasi" |
| P2-5 Autodebit orang tua tanpa UI | FIXED | `web/src/views/parent/ParentAutodebitView.vue` (baru), `services/parent/autodebit.service.ts` | vue-tsc/build |

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
- ~~Walikelas masih bisa melihat seluruh pengajuan sekolah di list.~~ Diperbaiki di putaran 2.
- ~~Belum ada UI admin untuk provider dan routing gateway.~~ Dibuat di putaran 2 (menu Pengaturan Gateway).

## Putaran 2

Target: tidak ada tombol mati dan tidak ada temuan di server. Semua perubahan ada di branch yang sama.

### Commit
| Commit | Isi |
|---|---|
| `6e8864a` | billing + web: halaman Pengaturan Gateway; dashboard memakai bentuk API sebenarnya; perbaikan tombol metode pembayaran dan settlement |
| `35af3a3` | core + web: menu 81 (Layanan Sekolah); hak status/hasil, lampiran sungguhan, pemilih anak, tombol unduh |
| `bd1d79e` | web: klaim anak yang gagal tidak lagi menampilkan tautan palsu |
| `6ed39ab` | academic: walikelas hanya melihat pengajuan kelasnya |
| `f727de9` | academic + web: lampiran menu 78 sungguhan, daftar siswa walikelas, tombol sesuai peran |
| `593b9f0` | billing + web: state "belum dikonfigurasi" di checkout, perpanjang kode yang sudah lewat waktu |
| `f016215` | academic: kunci cache respons memuat peran aktif |
| `a194ee8` | web: halaman Autodebit SPP orang tua |

### Menu Pengaturan Gateway (baru)
Route `/keuangan/pengaturan-gateway` untuk finance_manager, admin, dan superadmin (fitur `payment_gateway`). Halaman ini punya tiga tab.

- **Provider**
  - Superadmin: tabel `/platform/payment-providers` dengan tambah/ubah (kode, adapter, label, aktif, `credential_ref`, kapabilitas) dan badge kesiapan kredensial.
  - Admin/finance_manager: daftar baca-saja dari `/payment-providers/options`. Respons kini membawa `credentials_installed` (tanpa membocorkan secret atau prefix env). Provider tanpa kredensial tampil "Belum dikonfigurasi".
- **Routing** (fitur `payment_multi_provider`)
  - Tambah, ubah, nonaktifkan (dengan dialog konfirmasi), dan pratinjau routing per metode.
  - Bila fitur mati, tab ini menampilkan pesan, bukan galat.
- **Autodebit** (fitur `payment_autodebit`): form pengaturan (retry, interval, jam), tabel mandat dan percobaan debit dengan filter status.

### Audit tombol per view
| View | Temuan | Perbaikan |
|---|---|---|
| Dashboard Pembayaran | Bentuk respons tidak cocok (kartu kosong); bagian transfer manual 403 untuk finance_manager menutupi seluruh halaman; "Lihat semua" menuju halaman khusus admin | Memakai `getGatewayDashboard(from,to)`; galat per bagian; tautan hanya untuk admin/superadmin |
| Metode Pembayaran | Toggle mengirim `{enabled}` saja sehingga fee dan urutan ter-nol-kan; `alert()`; route menolak finance_manager padahal API mengizinkan | Selalu kirim body lengkap; kode/jenis/provider dikunci saat edit; editor kode per provider (multi-provider); opsi retail/autodebit; pesan inline; route menyertakan finance_manager |
| Settlement | `alert()`; provider tidak bisa dipilih | Pesan inline `apiErrorMessage`; pilihan provider (multi-provider); route menyertakan finance_manager |
| Checkout orang tua | Tidak ada state "belum dikonfigurasi" saat 503; konflik 409 membaca field di luar envelope (tombol "Lihat Transaksi Pending" tidak pernah muncul); galat "Request failed…" | Kartu "Pembayaran online belum dikonfigurasi" (daftar kosong atau 503); 409 membaca `data.transaction_id`; pesan server |
| Transaksi orang tua | "Perbarui Transaksi" selalu 400 setelah hitung mundur habis (tidak ada job kedaluwarsa, baris tetap `pending`); status `failed` tanpa tampilan; galat perbarui/salin tidak terlihat | Backend: renew menutup kode `pending` yang lewat `expires_at` lalu checkout ulang (`TestRenewOverduePending`); kartu gagal + perbarui; alert inline |
| Transaksi admin (QA-BILL) | Diaudit baca-saja di `feat/QA-BILL`: list/detail/kirim ulang kuitansi → `/admin/transactions*`, `/admin/receipts/{id}/resend` (RequireFinance) | Tidak diubah |
| Mutasi Siswa (menu 78) | Tab Pengaturan Kebijakan tampil untuk walikelas/kepala_sekolah (API admin-only → simpan 403); "Terapkan" tampil untuk non-admin (403); lampiran hanya nama file tanpa unggah; field `file_name` ≠ `name` di backend; walikelas bisa memilih siswa kelas lain (403 saat kirim); timeline approval memakai `approval_id` sebagai `source_id` | Tab dan tombol sesuai peran; unggah/unduh lampiran sungguhan; walikelas hanya melihat roster kelas perwaliannya; timeline memakai id pengajuan |
| Layanan Sekolah (menu 81) | Orang tua atau staf mana pun bisa mengubah status/hasil permintaan; lampiran hanya nama; tombol unduh mati; anak harus diketik manual | Hanya admin/superadmin atau `handler_role` kategori (403 untuk lainnya); unggah ke storage privat + unduh dengan aturan akses detail; pemilih anak dari `getMyChildren`; konfirmasi hapus kategori dan galat inline |
| Tagihan Saya (menu 82, siswa) | Diaudit baca-saja di `feat/QA-BILL`: `/me/bills`, `/me/payments`, `/me/bills/visibility` ada untuk siswa; satu tombol "Coba Lagi" | Tidak diubah (file QA-BILL) |
| Klaim anak | Saat galat, layanan mengembalikan tautan palsu sehingga UI menyatakan sukses | Galat diteruskan ke UI |

### Matriks peran (menu yang tampil ↔ izin backend)
| Peran | Menu dalam cakupan QA-PAY | Catatan |
|---|---|---|
| admin | Dashboard Pembayaran, Transaksi, Metode Pembayaran, Settlement, Pengaturan Gateway (provider baca-saja), Mutasi Siswa (penuh), Layanan (inbox + kategori) | Semua endpoint RequireFinance/RequirePaymentConfig/menu-78 mengizinkan |
| finance_manager | Dashboard, Transaksi, Metode Pembayaran, Settlement, Pengaturan Gateway | Bagian transfer manual di dashboard butuh `RequireFinanceViewer` dari QA-BILL (lihat kontrak 2) |
| superadmin | Menu keuangan di atas + CRUD provider | Mutasi Siswa sengaja tidak tampil (BUG-177) |
| kepala_sekolah | Mutasi Siswa (lihat saja), Layanan inbox | Tanpa tombol Ajukan/Terapkan/Pengaturan |
| kepala_yayasan | Layanan inbox | Tidak ada menu pembayaran atau mutasi |
| walikelas | Mutasi Siswa (kelas perwalian saja: list/detail/ringkasan keuangan/unduh, ajukan, batalkan milik sendiri), Layanan inbox | 403 untuk pengajuan kelas lain |
| orang_tua | Tagihan, Checkout, Transaksi, Autodebit SPP (fitur `payment_autodebit`), Layanan Sekolah | — |
| siswa | Tagihan Saya (menu 82) | — |

### Endpoint baru putaran 2
- core: `POST /service-request-attachments` (multipart `file`; PDF/JPEG/PNG; 503 bila storage tidak ada) dan `GET /service-requests/{id}/files?key=`.
- academic: `POST /student-status-request-attachments` (multipart `file`; PDF/JPG/PNG ≤10 MB; disimpan di `STORAGE_ROOT/<school>/transfers/<user>/`) dan `GET /student-status-requests/{id}/files?key=`.
- academic: create menolak `attachments[].object_key` yang bukan unggahan pengaju sendiri (422).
- billing: `GET /payment-providers/options` menambah `credentials_installed`.
- billing: `POST /parent/transactions/{id}/renew` kini menerima kode `pending` yang sudah lewat `expires_at` (ditandai `expired` lebih dulu).
- Migrasi: **tidak ada** (billing 0010 dan academic 0010 tidak dipakai).

### Kontrak untuk service/area lain (Claude yang menyambungkan)
1. **QA-BILL: status batch settlement.** `AdminRunReconciliation` (`gateway_reconciliation.go`) tidak pernah memperbarui `settlement_batches.status`, sehingga batch tetap `reported`. Setelah rekonsiliasi sebuah tanggal/provider, jalankan `UPDATE settlement_batches SET status = CASE WHEN <tidak ada exception terbuka> THEN 'matched' ELSE 'mismatch' END, updated_at = now() WHERE school_id = $1 AND settled_on = $2 AND provider = $3`. UI Settlement sudah menampilkan kolom status.
2. **QA-BILL: ringkasan transfer manual untuk finance_manager.** `/admin/finance/summary` di branch ini masih `adminSchool` (403 untuk finance_manager). Setelah merge dengan QA-BILL (`RequireFinanceViewer`) bagian ini terbuka. Dashboard sudah menampilkan galatnya terpisah, jadi halaman tetap terbuka.
3. **QA-CORE: timeline approval.** `web/src/services/core/approvals.ts#getBySource` memanggil `/approvals/source/{svc}/{id}` yang tidak ada. QA-CORE menambah route `GET /approvals/by-source?source_service=&source_id=`. `getBySource` perlu diubah ke route itu agar `ApprovalTimeline` di detail mutasi (dan modul lain) terisi. Detail mutasi sudah mengirim `source_id` = id pengajuan.
4. **Ops: kedaluwarsa transaksi gateway.** Tidak ada job yang menandai `gateway_transactions` `pending` → `expired` setelah `expires_at`. Untuk orang tua, renew sudah menanganinya, tetapi dashboard admin tetap menghitungnya sebagai pending sampai webhook datang. Usulan: job billing berkala `UPDATE … SET status='expired' WHERE status='pending' AND expires_at < now() - interval '5 minutes'` (hati-hati dengan webhook yang terlambat).

### Konfigurasi server tambahan (untuk ops, jangan dieksekusi agen)
- academic: `STORAGE_ROOT` (volume privat yang sama dengan materi pembelajaran) harus dapat ditulis, karena dipakai untuk lampiran menu 78.
- core: storage service desk (`h.ServiceDesk.Files`) dipakai untuk lampiran menu 81. Bila belum ada, unggah dijawab 503 "penyimpanan lampiran belum dikonfigurasi di server".
- Kredensial NICEPay tetap belum ada. Checkout, Pengaturan Gateway, dan Autodebit menampilkan "belum dikonfigurasi" sampai env `NICEPAY_*` diisi.

### Verifikasi putaran 2
```
# billing  go vet ./... bersih; go test ./... -count=1 → semua ok
#          (baru: TestProviderOptionsShowReadiness, TestRenewOverduePending)
# core     go vet ./... bersih; go test ./... -count=1 → ok common, handler, model, servicedesk, store
#          (baru: TestServiceRequestHandlerOnly, TestNormalizeAttachments)
# academic go vet ./... bersih; go test ./... -count=1 → ok common, handler, model, store
#          (baru: TestMenu78WaliKelasOwnClassOnly, TestMenu78Attachments)
# web      npx vue-tsc -b → tanpa galat; npm run build → ✓ built (hanya peringatan ukuran chunk lama)
# merge    git merge-tree feat/QA-PAY dengan QA-BILL/CORE/IDN/WEB/ACC/PPDB → tanpa konflik
#          (hasil merge dengan QA-BILL: metode-pembayaran/settlement/pengaturan-gateway = finance_manager, admin, superadmin)
```
`gofmt -l` masih menampilkan file lama yang tidak disentuh di sini (billing, `core/internal/model/service_request.go`, academic `client/identity.go`, `dapodik.go`, `learning_assignment.go`).

### Asumsi putaran 2 (default konservatif)
- Walikelas dianggap memiliki pengajuan bila kelas asal **atau** tujuan adalah kelas perwaliannya.
- Ringkasan keuangan untuk walikelas hanya untuk siswa yang punya enrollment aktif di kelas perwaliannya.
- Lampiran menu 78 dibatasi PDF/JPG/PNG ≤10 MB (lebih ketat dari materi pembelajaran).
- Kategori layanan menu 81 tetap admin/superadmin. Petugas kategori hanya memproses permintaan.
- Penghapusan provider tidak disediakan (API memang tidak punya DELETE). Provider dinonaktifkan lewat toggle.
- Autodebit orang tua tidak meminta nomor rekening. Aktivasi berjalan di halaman bank lewat `redirect_url` provider.
