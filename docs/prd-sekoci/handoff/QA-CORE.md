# Handoff QA-CORE: Approval Center, Antrean Galat, Audit, Notifikasi

Area: service `core` (menu 6 Audit Trail, 79 Approval Center, 80 Antrean Galat Integrasi) dan service `notification` (menu 7 Notifikasi & Template). Ada juga BUG-165 (menu 50).
Branch `feat/QA-CORE` berbasis `b984872`. Tidak ada push, deploy, atau akses server.

## 1. Status per bug

| BUG | Status | File utama | Bukti |
|---|---|---|---|
| BUG-002 Delegasi approval ke siswa | FIXED | core `handler/delegations.go`, `model/delegation.go`, `config/config.go` | Peran pendelegasi diambil dari JWT dan peran penerima dari identity. Irisan keduanya pada peran penyetuju harus ada, kalau tidak ditolak 422. Identity tidak tersedia memberi 503, bukan diterima tanpa cek. Test: `TestCreateDelegation_PeerRoleRequired`, `TestCreateDelegation_IdentityUnavailable`, `TestSharedApproverRoles` |
| BUG-026 Mutasi pengguna/unit/relasi/import tidak diaudit | FIXED (sisi core) + kontrak identity | core `handler/audit.go`, `store/audit.go`; web `AuditTrailView.vue` | Putaran 2: audit trail bisa difilter per service sumber dan beberapa aksi sekaligus, pilihan diambil dari `GET /audit/facets`. Pemanggilan `POST /internal/audit` dari identity disambungkan Claude sesuai §4. Test: `TestAudit_SourceFilterAndFacets` |
| BUG-027 Login gagal tidak diaudit | FIXED (sisi core) + kontrak identity | sama | Filter `service=identity&module=security&action=login_failed,account_locked` siap di audit trail. Pemanggil di identity mengikuti kontrak §4. |
| BUG-028 Approval tanpa notifikasi | FIXED | core `jobs/effects.go`, `handler/internal.go` | Penyetuju tahap (tanpa pembuat) dan pengaju menerima notifikasi untuk permintaan baru, disetujui, ditolak, dan eskalasi. Kirim yang gagal atau tanpa penerima masuk antrean galat `kind=notification`, jadi tidak lagi gagal diam-diam. Test: `TestApprovalNotifications`, `TestApprovalNotificationFailure_RaisesException`. Lihat juga §5: di server, `NOTIFICATION_URL`, `IDENTITY_URL` dan `SEKOCI_INTERNAL_KEY` harus benar. |
| BUG-029 WA/email tidak terkirim walau preferensi aktif | FIXED (kode), perlu konfigurasi server | notification `model/channels.go`, `provider/channels.go`, `handler/internal.go`, `handler/delivery_policy.go`, `config/config.go`, `cmd/server/main.go` | Kanal kini ditentukan kebijakan `ResolveChannels`, bukan pemanggil: in_app selalu ada, ditambah kanal eksternal yang diaktifkan pengguna. Adapter nyata: SMTP (STARTTLS/implicit TLS) dan gateway WA HTTP. Bila provider belum dikonfigurasi, delivery tercatat `failed` (bukan sukses palsu) dan ada laporan ke antrean galat. Test: `TestSend_PreferenceAddsChannel`, `TestSend_FailuresReachExceptionQueue`, `provider/channels_test.go` (SMTP palsu di port acak, gateway httptest) |
| BUG-087 Auditor tanpa batas waktu | FIXED | core migrasi `0004_audit_access_grants`, `handler/audit_grants.go`, `handler/audit.go`, `store/audit_grants.go`; web `AuditAccessGrantsView.vue` | Auditor hanya bisa membaca audit dan antrean galat selama ada masa akses aktif (maksimal 180 hari, alasan wajib, bisa dicabut, diaudit). Test: `TestAuditorAccessWindow`, `TestCreateAuditGrant`, `TestPGStore_AuditGrants` (PG) |
| BUG-088 Tidak ada menu admin template | FIXED | notification `handler/admin_templates.go`, `model/template_render.go`, `handler/internal.go`; web `NotificationTemplatesView.vue` | Endpoint JWT admin/superadmin. Setiap simpan menjadi versi baru dan versi terbaru yang dipakai saat kirim. Variabel tak dikenal atau kurung tak seimbang ditolak 422. Ada pemilih variabel, dan setiap perubahan diaudit. Test: `TestAdminTemplates` (termasuk email nyata dirender dari versi terbaru) |
| BUG-089 Pengumuman per unit/kelas | DIALIHKAN ke QA-IDN (putaran 2) | - | Sesuai arahan koordinator putaran 2, pengumuman (089/153/154) dikerjakan QA-IDN. |
| BUG-090 Preferensi kanal diabaikan | FIXED | notification `model/channels.go`, `handler/internal.go` | Tipe wajib (`bill_*`, `invoice_*`, `security_*`, atau `data.mandatory=true`) tetap dikirim lewat email+WA walau dimatikan. Untuk tipe lain, kanal yang dimatikan dilewati. Test: `TestSend_MandatoryIgnoresOptOut`, `TestSend_OptOutRespected`, `TestResolveChannels` |
| BUG-111 Pembuat = satu-satunya penyetuju | FIXED | core `model/routing.go`, `handler/internal.go` | Tahap dialihkan ke `escalate_to_role` bila ada pemegang selain pembuat. Bila tidak ada, permintaan ditolak 422 "tidak ada penyetuju selain pembuat permintaan" sehingga tidak macet. Test: `TestRerouteMakerOnlyStep`, `TestMakerOnlyApprover` |
| BUG-112 Admin akses Approval Center | FIXED (dengan catatan) | core `handler/approval_flows.go`, `handler/helpers.go`; web `core.routes.ts` | CRUD alur approval dan bypass "lihat semua permintaan" kini khusus superadmin. Admin tetap bisa memakai inbox untuk tahap yang memang ditugaskan ke perannya. Test: `TestApprovalFlows_SuperadminOnly`, `TestGetApproval_AdminNoBypass`. **Catatan:** FR-79-001 menyebut admin sebagai pengelola konfigurasi, jadi ini konflik PRD vs TC (§8). |
| BUG-113 Antrean tidak menerima notifikasi/import gagal | FIXED | shared `shared/go/common/exception_report.go`; core `handler/internal.go`, `model/severity.go`; notification `campaign/service.go`, `cmd/server/main.go` | Putaran 2: (a) intake core menerima dialek pelapor yang sudah ada (severity `warning/low/medium/high/...`, tanpa `payload_summary`, correlation hanya di header) alih-alih 422; (b) paket bersama `ExceptionReporter` dengan `ReportImportFailure` dan `ReportDeadLetter`; (c) kampanye WA/email notification yang gagal kirim kini dilaporkan `kind=dead_letter`. Test: `TestInternalException_AcceptsReporterDialects`, `TestNormalizeSeverity`, `TestExceptionReporter_Report`, `TestNormalizeExceptionReport`, `TestCampaignFailuresReportDeadLetter` |
| BUG-114 Admin melihat posting jurnal | FIXED | core `model/exception_access.go`, `handler/exceptions.go` | Matriks peran × jenis: admin tidak melihat `journal_posting` dan filter eksplisit memberi 403. Accounting (finance_manager/payroll_officer) melihat posting. Test: `TestListExceptions_AdminCannotSeeJournalPosting`, `TestExceptionActions_KindAccess`, `TestExceptionKindsFor` |
| BUG-115 Tanpa notifikasi item kritis | FIXED | core `jobs/effects.go`, `model/exception_access.go` | Item kritis baru dikirim ke peran penanggung jawab jenisnya, dan penugasan dikirim ke penerima tugas. Pembuatan berulang dengan idempotency key yang sama hanya menaikkan attempts, jadi tidak ada notifikasi berulang. Test: `TestCriticalException_NotifiesResponsibleRoles`, `TestExceptionResponsibleRoles` |
| BUG-116 Ekspor tidak diaudit | FIXED (sisi core) + kontrak service lain | core `handler/audit.go` | Ekspor audit dicatat `audit_exported`. Audit trail bisa memfilter `action=data_exported,audit_exported` per service. Ekspor di service lain memanggil kontrak §4. |
| BUG-117 Correlation ID lintas modul | FIXED (core+notification) + paket untuk service lain | `shared/go/common/correlation.go`, `scripts/sync-go-common.sh`; core+notification `internal/common/correlation.go` (salinan generated), `router.go` | Putaran 2: kode kanonik dipindah ke `shared/go/common` dan disalin ke service lewat `scripts/sync-go-common.sh` (`--check` untuk CI). Middleware terpasang di core dan notification. Pemasangan di service lain lihat §9. Test: `TestCorrelationMiddleware`, `TestCorrelationTransport` (shared dan salinan) |
| BUG-152 Rentang tanggal terbalik | FIXED | core `model/audit_range.go`, `handler/audit.go`; web `AuditTrailView.vue` | `from > to` memberi 422 "rentang tanggal tidak valid", format salah memberi 400. `to` tanggal-saja mencakup sehari penuh di zona sekolah. Test: `TestParseAuditRange`, `TestListAudit_DateRange` |
| BUG-153 Pengumuman tanpa audiens | DIALIHKAN ke QA-IDN (putaran 2) | - | Dikerjakan QA-IDN. |
| BUG-154 Pengumuman tak bisa dijadwalkan | DIALIHKAN ke QA-IDN (putaran 2) | - | Dikerjakan QA-IDN. |
| BUG-165 Panduan & FAQ orang tua | FIXED | core migrasi `0005_help_articles`, `model/help_article.go`, `store/help_articles.go`, `handler/help_articles.go`; web `HelpArticlesAdminView.vue`, `parent/PanduanView.vue`, `services/core/help.ts` | Konten dikelola admin: artikel bawaan platform yang netral (cara memakai portal, tanpa kanal/VA/batas waktu karangan) dan artikel sekolah yang bisa menggantikan atau menyembunyikan bawaan. Halaman `orang-tua/panduan` dan `siswa/panduan`; menu admin Pengaturan → Panduan & FAQ. Mutasi diaudit (`module=help_center`). Test: `TestHelpArticles`, `TestValidateHelpArticle`, `TestMergeHelpArticles`, `TestPGStore_HelpArticles` |
| BUG-166 Ambang nominal tumpang tindih | FIXED | core `model/thresholds.go`, `handler/approval_flows.go` | Per `condition_flag`, `min_amount_idr` harus > 0 dan naik ketat antar tahap, kalau tidak ditolak 422. Test: `TestValidateAmountThresholds`, `TestCreateFlow_OverlappingThresholds` |
| BUG-167 Retry tanpa kunci | FIXED | core `store/exceptions.go` (`ClaimExceptionForRetry`), `handler/exceptions.go`, `config` | Klaim atomik `open` → `retrying`. Klik kedua mendapat 409 "sedang dicoba ulang". Kunci kedaluwarsa setelah `EXCEPTION_RETRY_LOCK_SECONDS` (300) untuk pekerja yang crash, dan retry gagal mengembalikan status ke `open`. Test: `TestRetryException_ConcurrentClicksLocked`, `TestRetryException_FailureReopens`, `TestPGStore_ClaimExceptionForRetry` (PG, tepat satu pemenang) |

## 2. Migrasi baru

- **core `0004_audit_access_grants`**:
  - tabel `audit_access_grants` (id, school_id, user_id, valid_from, valid_to, reason, granted_by, created_at, revoked_at, revoked_by)
  - CHECK `valid_to > valid_from` dan alasan tidak kosong
  - indeks parsial grant aktif
  - `idx_audit_events_correlation`
  - Down menghapus keduanya.
- notification: **tidak ada migrasi baru**. Tabel `notification_templates` sudah ada, dan slot 0005 tidak dipakai.
- **core `0005_help_articles`** (putaran 2): tabel `help_articles` (school_id NULL = bawaan platform, `replaces_id` → bawaan yang diganti, unik per sekolah+bawaan), plus 6 artikel bawaan netral untuk orang tua. Down menghapus tabel. Slot core 0006 dan notification 0006 tidak dipakai.

## 3. Endpoint baru dan perubahan kontrak

Core (JWT):
- `GET/POST /audit/access-grants`, `DELETE /audit/access-grants/{id}`: superadmin.
- `GET /audit` dan export mendapat filter tambahan `service`, `action`, `entity_id`, `correlation_id`. Rentang tanggal divalidasi (400 untuk format salah, 422 untuk rentang terbalik). Export di atas `AUDIT_EXPORT_MAX_ROWS` memberi 422.
- Auditor mendapat **403** pada `/audit` dan `/exceptions` tanpa masa akses aktif. **Ini perubahan perilaku.**
- `/approval-flows` (CRUD) kini hanya superadmin. Admin mendapat 403.
- `/exceptions`: daftar disaring menurut matriks peran × jenis, dan aksi pada jenis di luar matriks mendapat 403. Retry ganda mendapat 409.

Notification (JWT admin/superadmin):
- `GET /notification-templates?key=` (semua versi, terbaru dulu per key+kanal)
- `GET /notification-templates/variables`
- `POST /notification-templates` (`{key, channel: in_app|email|whatsapp, subject, body}`, selalu versi baru)
- `DELETE /notification-templates/{id}`
- `POST /internal/notifications/{id}/retry` kini memberi **502** bila masih ada delivery yang gagal setelah retry. Sebelumnya selalu 200. Pemanggilnya hanya core (retry antrean galat), yang memang mengharapkan non-2xx sebagai gagal.
- `POST /internal/notifications`: daftar `channels` dari pemanggil kini menjadi *permintaan*. Kanal final mengikuti preferensi pengguna dan tipe wajib. Template terbaru untuk `template_key`/`type` menggantikan teks pemanggil bila berhasil dirender.

Semua service yang disentuh menerima dan mengembalikan header `X-Correlation-ID` (CORS allowed+exposed).

## 4. Kontrak untuk area lain (QA-IDN / QA-BILL / QA-PPDB)

`POST {CORE_URL}/internal/audit` dengan header `X-Internal-Key` dan `X-Correlation-ID` (opsional; core memakainya bila `correlation_id` kosong):
- BUG-027:
  - `{"school_id","module":"security","action":"login_failed","entity_type":"user","entity_id":<user atau username ter-masking>,"actor_user_id":null,"after":{"ip":...,"reason":"wrong_password"}}`
  - `action=account_locked` saat penguncian.
- BUG-026: setiap mutasi pengguna, unit, relasi, atau import memakai `action` create/update/delete dengan `before`/`after`. Field sensitif (password, token, NIK) harus dimasking oleh pengirim.
- BUG-116: `action=data_exported`, `entity_type=<jenis data>`, `after={"filter":{...},"rows":N,"format":"csv|xlsx"}`.
- BUG-113 (import/dead-letter): `POST {CORE_URL}/internal/exceptions` dengan `kind=import|dead_letter`, `severity`, `idempotency_key` stabil, dan `retry_url` bila bisa diulang.

## 5. Langkah konfigurasi server (untuk pemilik repo, TIDAK dijalankan oleh agen ini)

1. Backup DB core lalu jalankan migrasi core sampai versi 4 (`core-migrate` di compose melakukannya otomatis).
2. Setelah deploy, **superadmin harus memberi masa akses** lewat menu Pengaturan → Masa Akses Auditor untuk setiap auditor aktif. Tanpa itu auditor mendapat 403.
3. Agar email benar-benar terkirim, isi variabel berikut di `.env` server (lihat `.env.example`):
   - `NOTIFICATION_EMAIL_PROVIDER=smtp`
   - `SMTP_HOST`, `SMTP_PORT`, `SMTP_USERNAME`, `SMTP_PASSWORD`, `SMTP_FROM`
   - `SMTP_TLS=implicit` hanya untuk port 465
4. Agar WA benar-benar terkirim, isi `NOTIFICATION_WHATSAPP_PROVIDER=http`, `WA_API_URL` (gateway menerima `POST {to, message}` dengan Bearer token dan membalas JSON berisi `id`) dan `WA_API_TOKEN`. Bila kosong, kiriman tercatat gagal dan muncul satu item antrean galat per kanal.
5. `docker-compose.yml` kini meneruskan `CORE_URL`, `NOTIFICATION_INTERNAL_URL` dan variabel SMTP/WA ke kontainer notification. Pastikan `docker-compose.server.yml` (bila menimpa environment) ikut memuatnya.
6. BUG-028: pastikan `NOTIFICATION_URL` dan `IDENTITY_URL` di core serta `SEKOCI_INTERNAL_KEY` identik di core, notification dan identity. Bila salah, notifikasi approval akan muncul sebagai item `notification` di antrean galat (tidak lagi diam).
7. Opsional: `EXCEPTION_RETRY_LOCK_SECONDS` (default 300) dan `AUDIT_EXPORT_MAX_ROWS` (default 10000).
8. Isi template notifikasi melalui menu Komunikasi → Template Notifikasi (admin). Tanpa template, teks dari service pengirim dipakai.

## 6. Verifikasi (output asli)

Toolchain lokal (`~/.local/toolchain/env.sh`), Postgres lokal, DB `skci_qa_core_core` dan `skci_qa_core_notification`.

```
== core
go vet ./...                     -> VET_OK
migrate up                       -> no change ; version -> 4
migrate down 1 / up              -> 4/d audit_access_grants / 4/u audit_access_grants ; version 4
TEST_PG_DSN=... go test -count=1 ./...
ok  skci/core/internal/common
ok  skci/core/internal/handler
ok  skci/core/internal/model
ok  skci/core/internal/servicedesk
ok  skci/core/internal/store        (4 test PG PASS, termasuk klaim retry konkuren)
== notification
go vet ./...                     -> VET_OK
migrate up                       -> 3/u campaign_chat, 4/u push_channel ; version 4 ; down 1 / up OK
go test -count=1 ./...           -> campaign, chat, common, config, handler, model, provider, store: ok
web: npx vue-tsc -b              -> 0 error ; npm run build -> built in 8.71s
```

Catatan: `TestChatParticipantScoping` (notification store, **di luar area ini dan tidak diubah**) gagal bila dijalankan dengan `NOTIFICATION_TEST_DATABASE_URL`. Fixture test-nya berisi UUID tidak valid (37 karakter, `...0e303cce31190`), sehingga Postgres menolak dengan `22P02`. Tanpa variabel itu test tersebut di-skip, sama seperti sebelumnya.

Tidak dijalankan sesuai mode paralel GEMINI.md: `docker compose up/build`, Playwright, dev server.

## 7. Desain untuk item DEFERRED (089/153/154 kini milik QA-IDN)

- **BUG-089/154 (identity announcements)**:
  - Tambah kolom `target_unit_ids UUID[]`, `target_class_ids UUID[]` dan `publish_at TIMESTAMPTZ NULL`.
  - Audiens = gabungan peran ∩ unit ∩ kelas. Keanggotaan kelas diambil dari academic.
  - Job tiap menit menerbitkan `publish_at <= now()`.
  - Simpan dengan `publish_at` lampau atau kosong meminta konfirmasi "kirim sekarang".
- **BUG-153**: validasi 422 bila tidak ada satu pun target.
- **BUG-165**: sudah diimplementasikan di putaran 2 (lihat §9).

## 8. Asumsi dan pertanyaan terbuka

1. **BUG-112 vs FR-79-001**: TC meminta admin tidak bisa mengakses Approval Center, sedangkan PRD menyebut admin pengelola alur. Diambil opsi paling konservatif: konfigurasi alur hanya superadmin, dan admin tetap bisa memutus tahap yang memang ditugaskan ke perannya. Perlu keputusan produk.
2. Tipe notifikasi wajib (`bill_`, `invoice_`, `security_`) ditentukan sebagai kebijakan kode (`model.MandatoryTypePrefixes`). Tipe lain bisa dibuat wajib per kiriman dengan `data.mandatory=true`.
3. Pembagian peran × jenis antrean galat: admin = Finance Admin (webhook, notification, dead_letter, import, callback), finance_manager = webhook + journal_posting, payroll_officer = journal_posting, auditor = baca semua (dengan masa akses), superadmin = semua. Lingkup unit belum diterapkan karena item antrean tidak membawa unit_id.
4. Masa akses auditor maksimal 180 hari per pemberian (README §5 tidak menyebut angka, jadi dipilih yang konservatif).
5. Kontrak gateway WA generik (`POST {to, message}` + Bearer). Bila vendor memakai format lain, cukup menambah adapter di `provider/channels.go`.
6. BUG-165: isi bawaan sengaja netral (cara memakai portal) tanpa nomor rekening, VA, atau batas waktu. Rincian pembayaran adalah konten sekolah. Halaman yang sama dipakai portal siswa (`audience=siswa`, belum ada isi bawaan).

## 9. Putaran 2

Commit: `3abd483`, `1de9dce`, `5d75cbf`, `54816fb`, `2ef4397`, `7eb93de`, `6db2e43` (+ handoff ini).

### Endpoint baru (tambahan, kompatibel)

Core (JWT):
- `GET /approvals/by-source?source_service=&source_id=`: permintaan terbaru untuk transaksi sumber, lengkap dengan langkah. 404 bila belum diajukan atau pemanggil tidak berhak melihat. Dipakai `ApprovalTimeline` di halaman sumber.
- `GET /audit/facets`: daftar service, module, action yang tercatat 365 hari terakhir (superadmin/auditor aktif). `GET /audit` kini menerima `action` berupa daftar koma.
- `GET /help-articles[?audience=orang_tua|siswa]`: artikel terbit untuk pembaca. Orang tua/siswa otomatis mendapat audiensnya; staf bisa pratinjau.
- `GET /help-articles/manage?audience=`, `POST /help-articles`, `PUT /help-articles/{id}`, `DELETE /help-articles/{id}`: admin/superadmin. Artikel bawaan tidak bisa diubah/dihapus (422); buat pengganti dengan `replaces_id`. Pengganti kedua untuk bawaan yang sama → 409.
- Perubahan perilaku kecil: `DELETE /approval-delegations/{id}` atas delegasi yang sudah dicabut atau milik orang lain kini **404** (sebelumnya 200 diam).
- `POST /internal/exceptions` menerima alias severity (`low/info/medium/warning/normal` → normal, `high/error/critical` → critical), `payload_summary` kosong, dan correlation dari header. Severity tak dikenal tetap 422.

Notification (JWT admin/superadmin/humas):
- `GET /notification-channels/status` → `[{channel: email|whatsapp|push|whatsapp_campaign, provider, state: aktif|simulasi|belum_dikonfigurasi, configured}]`. Tanpa kredensial. UI Template Notifikasi dan Preferensi Notifikasi memakai ini untuk menampilkan "belum dikonfigurasi" alih-alih tombol mati.
- Kampanye yang gagal kirim melaporkan `kind=dead_letter` (severity normal), `idempotency_key=dead_letter:notification:campaign:<campaign_id>`. Pesan galat handler notification kini berbahasa Indonesia.

### Paket Go bersama (BUG-117/113), untuk disambungkan Claude ke service lain

Kode kanonik ada di `shared/go/common/` (modul `skci/shared`, punya test sendiri). Setiap service adalah modul Go terpisah dengan konteks Docker sendiri, jadi kode disalin sebagai file generated:

1. Tambahkan `"<service>:correlation.go"` dan/atau `"<service>:exception_report.go"` ke `MANIFEST` di `scripts/sync-go-common.sh`, lalu jalankan skripnya. `exception_report.go` membutuhkan `correlation.go`.
2. Router: `r.Use(middleware.RequestID, common.CorrelationMiddleware, ...)`, tambahkan `common.CorrelationHeader` ke CORS `AllowedHeaders` dan `ExposedHeaders`.
3. Klien HTTP keluar: `Transport: common.CorrelationTransport{Base: http.DefaultTransport}` agar ID diteruskan; audit di core otomatis dicap.
4. Pelapor galat: `rep := common.NewExceptionReporter(cfg.CoreURL, cfg.InternalKey, "<service>")`, lalu
   - `rep.ReportImportFailure(ctx, schoolID, jobID, pesan, ringkasan)` → `kind=import`, key `import:<service>:<job>`;
   - `rep.ReportDeadLetter(ctx, schoolID, topic, messageID, pesan, attempts, ringkasan)` → `kind=dead_letter`, severity critical, key `dead_letter:<service>:<topic>:<msg>`;
   - `rep.Report(ctx, common.ExceptionReport{...})` untuk jenis lain. Laporan dengan key yang sama hanya menaikkan `attempts` di core. Tanpa `CORE_URL`/kunci → `ErrReporterNotConfigured` (bukan sukses palsu).
5. CI: `scripts/sync-go-common.sh --check` gagal bila salinan basi.

Kandidat pemasangan: identity (import pengguna → `ReportImportFailure`), ops (dead-letter procurement, kini sudah diterima core tanpa ubah kode), billing → accounting (correlation untuk posting jurnal).

### Audit tombol per peran (Approval Center, Antrean Galat, Audit, Notifikasi)

- Approval Center (`ALL_STAFF_ROLES`): tombol Setujui/Tolak hanya tampil bila `can_decide`; Tolak wajib komentar (validasi UI + 422 backend). Timeline memakai `by-source` (rute yang sebelumnya dipanggil tidak ada). Alur approval: superadmin saja, sama dengan backend; PUT/DELETE alur kini tersambung. Delegasi: buat/cabut sesuai backend, galat tampil terbaca.
- Antrean galat (superadmin, admin, finance_manager, payroll_officer, auditor): auditor hanya baca (tombol aksi disembunyikan). Coba Ulang hanya muncul bila item punya `retry_url` atau callback approval; 409 tampil sebagai "sedang dicoba ulang". Assign/Resolve memanggil endpoint yang ada; filter jenis mengikuti matriks peran sehingga tidak memicu 403.
- Audit (superadmin, auditor): auditor tanpa masa akses melihat pesan 403 yang jelas. Filter sumber/aksi dari facets; ekspor CSV memakai `apiBlobErrorMessage` untuk galat.
- Notifikasi: inbox/chat/preferensi memakai `apiErrorMessage`; Preferensi menyembunyikan aksi perangkat push bila server belum punya kunci VAPID dan menampilkan "Layanan push belum dikonfigurasi"; Template menandai kanal yang belum dikonfigurasi/simulasi. Portal orang tua (`orang-tua/notifikasi`) sudah memakai endpoint yang benar.

### Verifikasi putaran 2 (output asli, ringkas)

```
scripts/sync-go-common.sh --check          -> SYNC_OK
shared/go: go vet ./... ; go test ./...     -> ok skci/shared/common
core: go vet ./... ; TEST_PG_DSN=... go test ./...
  ok common, handler, model, servicedesk, store
core migrate: up -> no change ; down 1 -> 5/d help_articles ; up -> 5/u help_articles ; version 5
notification: go vet ./... ; NOTIFICATION_TEST_DATABASE_URL=... go test ./...
  ok campaign, chat, common, config, handler, model, provider
  FAIL store: TestChatParticipantScoping (fixture UUID 37 karakter, sudah ada sebelumnya, lihat §6)
web: npx vue-tsc -b -> 0 error ; npm run build -> built in 12.39s
```

`gofmt -l` masih menampilkan `core/internal/model/service_request.go` dan `notification/internal/handler/{fakestore_test.go,push.go,push_test.go}`. Format tersebut sudah ada sebelum QA-CORE dan sengaja tidak diubah agar diff tetap kecil.

### Konfigurasi server tambahan (tidak dijalankan)

1. Jalankan migrasi core sampai versi 5 (`core-migrate`). Artikel bawaan langsung tampil di portal orang tua.
2. Admin sekolah sebaiknya mengganti artikel "Cara melihat dan membayar tagihan" dengan kanal pembayaran sekolahnya lewat Pengaturan → Panduan & FAQ.

## 10. Putaran 3

Commit `63e664c`. Pemicu: QA-BILL kini mengirim kegagalan posting jurnal sebagai item antrean galat kritis.

### Perubahan

- **Matriks peran × jenis** (`core/internal/model/exception_access.go`): peran baru `akuntan` (`common.RoleAkuntan`) melihat dan menindak `webhook` dan `journal_posting`, sama seperti `finance_manager`. Item kritis kedua jenis itu kini juga dikirim ke akuntan. Matriks web (`services/core/types.ts`) disamakan.
- **Penerima notifikasi kritis** (`core/internal/jobs/effects.go`, `client.RoleHolders`):
  - Core memanggil `GET {IDENTITY_URL}/internal/users?role=<peran>&school_id=<sekolah>` dengan `X-Internal-Key`. Respons yang diharapkan adalah envelope `{"data":[{"id":"<uuid>"}...]}`; field `user_id` juga diterima.
  - Bila identity menjawab **404** (rute belum ada), core memakai perilaku lama: `GET /users?role=&school_id=&status=active&per_page=200`. Notifikasi tidak gagal.
  - Galat lain (5xx/timeout) hanya di-log per peran, dan peran lain tetap dikirimi.
  - Pengguna yang memegang beberapa peran penanggung jawab hanya menerima satu notifikasi.
- **Web**:
  - Peran `akuntan` ditambahkan ke tipe `Role` (`types/envelope.ts`) dan diberi label di `RoleSwitcher`.
  - Menu Sistem → Antrean Galat Integrasi kini tampil untuk `akuntan`. `finance_manager` sudah mendapatkannya sejak putaran 1.

### Test

- `TestCriticalException_UsesInternalUserList`: identity palsu melayani `/internal/users` dan menyembunyikan `/users`. Penerima yang diharapkan adalah tepat akuntan dan finance_manager, masing-masing sekali, tanpa admin. Akuntan juga bisa membaca daftar `kind=journal_posting`.
- `TestCriticalException_NotifiesResponsibleRoles`: identity palsu tanpa `/internal/users` (404), sehingga jalur fallback ke `/users` teruji.
- `TestExceptionKindsFor` dan `TestExceptionResponsibleRoles` mencakup akuntan.

### Verifikasi (output asli)

```
core: go vet ./... ; TEST_PG_DSN=... go test -count=1 ./...
  ok common, handler, model, servicedesk, store
web: npx vue-tsc -b -> 0 error ; npm run build -> built in 25.86s
```

### Catatan untuk area lain

- **Identity (QA-IDN)**:
  - Peran `akuntan` harus bisa diberikan (enum/daftar peran identity).
  - `/internal/users` sebaiknya hanya mengembalikan pengguna aktif dengan peran itu di sekolah tersebut.
- **Web/akuntansi**: `DashboardRedirectView` belum punya halaman awal khusus `akuntan`, jadi akuntan mendarat di dasbor umum. Menu akuntansi lain untuk akuntan adalah milik area akuntansi. Core tidak mengubahnya agar tidak bentrok.
