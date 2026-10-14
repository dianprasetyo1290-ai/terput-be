# Handoff QA-CORE: Approval Center, Antrean Galat, Audit, Notifikasi

Area: service `core` (menu 6 Audit Trail, 79 Approval Center, 80 Antrean Galat Integrasi) dan service `notification` (menu 7 Notifikasi & Template). Ada juga BUG-165 (menu 50).
Branch `feat/QA-CORE` berbasis `b984872`. Tidak ada push, deploy, atau akses server.

## 1. Status per bug

| BUG | Status | File utama | Bukti |
|---|---|---|---|
| BUG-002 Delegasi approval ke siswa | FIXED | core `handler/delegations.go`, `model/delegation.go`, `config/config.go` | Peran pendelegasi diambil dari JWT dan peran penerima dari identity. Irisan keduanya pada peran penyetuju harus ada, kalau tidak ditolak 422. Identity tidak tersedia memberi 503, bukan diterima tanpa cek. Test: `TestCreateDelegation_PeerRoleRequired`, `TestCreateDelegation_IdentityUnavailable`, `TestSharedApproverRoles` |
| BUG-026 Mutasi pengguna/unit/relasi/import tidak diaudit | PARTIAL (kontrak) | core `handler/internal.go` (correlation default) | Mutasinya terjadi di service **identity** (pemilik QA-IDN). Sisi core sudah siap menerima lewat `POST /internal/audit` (lihat §4). Pemanggilan dari identity menjadi tugas QA-IDN. |
| BUG-027 Login gagal tidak diaudit | PARTIAL (kontrak) | sama | Login ada di identity (QA-IDN). Kontraknya: `module=security`, `action=login_failed` / `account_locked` (§4). |
| BUG-028 Approval tanpa notifikasi | FIXED | core `jobs/effects.go`, `handler/internal.go` | Penyetuju tahap (tanpa pembuat) dan pengaju menerima notifikasi untuk permintaan baru, disetujui, ditolak, dan eskalasi. Kirim yang gagal atau tanpa penerima masuk antrean galat `kind=notification`, jadi tidak lagi gagal diam-diam. Test: `TestApprovalNotifications`, `TestApprovalNotificationFailure_RaisesException`. Lihat juga §5: di server, `NOTIFICATION_URL`, `IDENTITY_URL` dan `SEKOCI_INTERNAL_KEY` harus benar. |
| BUG-029 WA/email tidak terkirim walau preferensi aktif | FIXED (kode), perlu konfigurasi server | notification `model/channels.go`, `provider/channels.go`, `handler/internal.go`, `handler/delivery_policy.go`, `config/config.go`, `cmd/server/main.go` | Kanal kini ditentukan kebijakan `ResolveChannels`, bukan pemanggil: in_app selalu ada, ditambah kanal eksternal yang diaktifkan pengguna. Adapter nyata: SMTP (STARTTLS/implicit TLS) dan gateway WA HTTP. Bila provider belum dikonfigurasi, delivery tercatat `failed` (bukan sukses palsu) dan ada laporan ke antrean galat. Test: `TestSend_PreferenceAddsChannel`, `TestSend_FailuresReachExceptionQueue`, `provider/channels_test.go` (SMTP palsu di port acak, gateway httptest) |
| BUG-087 Auditor tanpa batas waktu | FIXED | core migrasi `0004_audit_access_grants`, `handler/audit_grants.go`, `handler/audit.go`, `store/audit_grants.go`; web `AuditAccessGrantsView.vue` | Auditor hanya bisa membaca audit dan antrean galat selama ada masa akses aktif (maksimal 180 hari, alasan wajib, bisa dicabut, diaudit). Test: `TestAuditorAccessWindow`, `TestCreateAuditGrant`, `TestPGStore_AuditGrants` (PG) |
| BUG-088 Tidak ada menu admin template | FIXED | notification `handler/admin_templates.go`, `model/template_render.go`, `handler/internal.go`; web `NotificationTemplatesView.vue` | Endpoint JWT admin/superadmin. Setiap simpan menjadi versi baru dan versi terbaru yang dipakai saat kirim. Variabel tak dikenal atau kurung tak seimbang ditolak 422. Ada pemilih variabel, dan setiap perubahan diaudit. Test: `TestAdminTemplates` (termasuk email nyata dirender dari versi terbaru) |
| BUG-089 Pengumuman per unit/kelas | DEFERRED | - | Pengumuman milik service **identity** (`identity/announcements`, QA-IDN). Butuh migrasi identity dan data keanggotaan kelas (academic). Desain usulan ada di §7. |
| BUG-090 Preferensi kanal diabaikan | FIXED | notification `model/channels.go`, `handler/internal.go` | Tipe wajib (`bill_*`, `invoice_*`, `security_*`, atau `data.mandatory=true`) tetap dikirim lewat email+WA walau dimatikan. Untuk tipe lain, kanal yang dimatikan dilewati. Test: `TestSend_MandatoryIgnoresOptOut`, `TestSend_OptOutRespected`, `TestResolveChannels` |
| BUG-111 Pembuat = satu-satunya penyetuju | FIXED | core `model/routing.go`, `handler/internal.go` | Tahap dialihkan ke `escalate_to_role` bila ada pemegang selain pembuat. Bila tidak ada, permintaan ditolak 422 "tidak ada penyetuju selain pembuat permintaan" sehingga tidak macet. Test: `TestRerouteMakerOnlyStep`, `TestMakerOnlyApprover` |
| BUG-112 Admin akses Approval Center | FIXED (dengan catatan) | core `handler/approval_flows.go`, `handler/helpers.go`; web `core.routes.ts` | CRUD alur approval dan bypass "lihat semua permintaan" kini khusus superadmin. Admin tetap bisa memakai inbox untuk tahap yang memang ditugaskan ke perannya. Test: `TestApprovalFlows_SuperadminOnly`, `TestGetApproval_AdminNoBypass`. **Catatan:** FR-79-001 menyebut admin sebagai pengelola konfigurasi, jadi ini konflik PRD vs TC (§8). |
| BUG-113 Antrean tidak menerima notifikasi/import gagal | PARTIAL | notification `handler/delivery_policy.go`, `client/integrations.go` | **Notifikasi gagal: FIXED.** Laporannya dikelompokkan supaya antrean tidak banjir: `notify-provider:<kanal>` untuk provider belum dikonfigurasi, `notify-recipient:<user>:<kanal>` untuk tujuan kosong, dan `notify-delivery:<id>:<kanal>` dengan `retry_url` ke endpoint retry. **Import gagal** ada di identity (QA-IDN), dan **dead-letter** perlu diadopsi oleh service pengirim event. Keduanya cukup memanggil `POST /internal/exceptions` dengan `kind=import` / `dead_letter` (sudah didukung core). |
| BUG-114 Admin melihat posting jurnal | FIXED | core `model/exception_access.go`, `handler/exceptions.go` | Matriks peran × jenis: admin tidak melihat `journal_posting` dan filter eksplisit memberi 403. Accounting (finance_manager/payroll_officer) melihat posting. Test: `TestListExceptions_AdminCannotSeeJournalPosting`, `TestExceptionActions_KindAccess`, `TestExceptionKindsFor` |
| BUG-115 Tanpa notifikasi item kritis | FIXED | core `jobs/effects.go`, `model/exception_access.go` | Item kritis baru dikirim ke peran penanggung jawab jenisnya, dan penugasan dikirim ke penerima tugas. Pembuatan berulang dengan idempotency key yang sama hanya menaikkan attempts, jadi tidak ada notifikasi berulang. Test: `TestCriticalException_NotifiesResponsibleRoles`, `TestExceptionResponsibleRoles` |
| BUG-116 Ekspor tidak diaudit | PARTIAL (kontrak) | core `handler/audit.go` | Ekspor audit kini mencatat filter dan correlation. Ekspor siswa, tagihan dan PPDB ada di service lain (QA-IDN/BILL/PPDB): `POST /internal/audit` dengan `action=data_exported` (§4). |
| BUG-117 Correlation ID lintas modul | PARTIAL | core+notification `common/correlation.go`, `router.go`, `client/client.go`; core `store/audit.go`; web `AuditTrailView.vue` | Middleware `X-Correlation-ID` mengadopsi atau membuat ID dan mengembalikannya di respons. Transport meneruskannya ke panggilan keluar, dan audit dicap otomatis. Audit trail kini bisa difilter per `correlation_id` dengan tombol "Telusuri alur". Test: `TestCorrelationMiddleware`, `TestCorrelationTransport`, `TestCorrelationID_StampedOnAudit`, `TestPGStore_AuditFilters`. Rantai billing → accounting perlu memasang middleware/transport yang sama (file `common/correlation.go` bisa disalin apa adanya). |
| BUG-152 Rentang tanggal terbalik | FIXED | core `model/audit_range.go`, `handler/audit.go`; web `AuditTrailView.vue` | `from > to` memberi 422 "rentang tanggal tidak valid", format salah memberi 400. `to` tanggal-saja mencakup sehari penuh di zona sekolah. Test: `TestParseAuditRange`, `TestListAudit_DateRange` |
| BUG-153 Pengumuman tanpa audiens | DEFERRED | - | Validasi ada di identity `announcements` (QA-IDN). Perubahan yang diusulkan: tolak 422 "audiens wajib dipilih" bila `target_roles` kosong. |
| BUG-154 Pengumuman tak bisa dijadwalkan | DEFERRED | - | Identity (QA-IDN). Butuh kolom `publish_at` dan job penerbit. Desain di §7. |
| BUG-165 Panduan & FAQ orang tua | DEFERRED | - | Isi panduan bayar (kanal, VA, batas waktu) harus dari sekolah atau keuangan. Menulis isi karangan akan menyesatkan. Usulan: halaman statis `orang-tua/panduan` yang isinya dikelola admin (pakai pola pengumuman). |
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

## 7. Desain untuk item DEFERRED

- **BUG-089/154 (identity announcements)**:
  - Tambah kolom `target_unit_ids UUID[]`, `target_class_ids UUID[]` dan `publish_at TIMESTAMPTZ NULL`.
  - Audiens = gabungan peran ∩ unit ∩ kelas. Keanggotaan kelas diambil dari academic.
  - Job tiap menit menerbitkan `publish_at <= now()`.
  - Simpan dengan `publish_at` lampau atau kosong meminta konfirmasi "kirim sekarang".
- **BUG-153**: validasi 422 bila tidak ada satu pun target.
- **BUG-165**: halaman panduan orang tua berbasis konten yang dikelola admin. Isi pertama disusun oleh keuangan sekolah.

## 8. Asumsi dan pertanyaan terbuka

1. **BUG-112 vs FR-79-001**: TC meminta admin tidak bisa mengakses Approval Center, sedangkan PRD menyebut admin pengelola alur. Diambil opsi paling konservatif: konfigurasi alur hanya superadmin, dan admin tetap bisa memutus tahap yang memang ditugaskan ke perannya. Perlu keputusan produk.
2. Tipe notifikasi wajib (`bill_`, `invoice_`, `security_`) ditentukan sebagai kebijakan kode (`model.MandatoryTypePrefixes`). Tipe lain bisa dibuat wajib per kiriman dengan `data.mandatory=true`.
3. Pembagian peran × jenis antrean galat: admin = Finance Admin (webhook, notification, dead_letter, import, callback), finance_manager = webhook + journal_posting, payroll_officer = journal_posting, auditor = baca semua (dengan masa akses), superadmin = semua. Lingkup unit belum diterapkan karena item antrean tidak membawa unit_id.
4. Masa akses auditor maksimal 180 hari per pemberian (README §5 tidak menyebut angka, jadi dipilih yang konservatif).
5. Kontrak gateway WA generik (`POST {to, message}` + Bearer). Bila vendor memakai format lain, cukup menambah adapter di `provider/channels.go`.
