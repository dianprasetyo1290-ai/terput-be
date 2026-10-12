# T13: Komunikasi lanjutan: kampanye WhatsApp tersegmentasi, chat guru–orang tua, landing page & referral PPDB

**Menu PRD:** 147, 148, 149 (BR-COM-01/02, BR-REF-01, IF-ADV-02, NFR-ADV-06, OQ-38/43) · **Branch:** `feat/T13-komunikasi-lanjutan` · **Service:** `notification` (paket `internal/campaign`, perluasan chat yang ada), `admission` (paket `internal/marketing`) · **Migrasi:** `services/notification/migrations/0003_campaign_chat.*` dan `services/admission/migrations/0004_marketing_referral.*` (nomor milik T13)
**Prasyarat:** T1 (audit/exception), menu 09/15/16 (admission, sudah ada), chat `0002_chat` (sudah ada, **diperluas**, tidak dibuat ulang). Paralel dengan T9 (peran + feature key). Tidak ada ketergantungan ke T12/T14.
**Feature key (final, T9 §2):** `campaign` (147), `teacher_parent_chat` (148), `ppdb_marketing` (149). **Peran:** `humas` (baru, T9), plus yang sudah ada: `admin`, `guru`, `walikelas`, `orang_tua`, `kepala_sekolah` (moderasi = "Kepala Unit"). Nama peran final mengikuti **T9 §2**.

## 1. Acuan kode (tanpa donor eksternal)
| Kebutuhan | Pakai / tiru |
|---|---|
| Adapter kanal | `services/notification/internal/provider/provider.go` (`Provider`, `NoopProvider`, `UnconfiguredProvider`, `New`). WhatsApp = adapter tambahan di paket ini, **bukan** paket baru |
| Kirim + ledger pengiriman | `notification_deliveries` (0001), `InternalSendNotification` (idempoten). Kampanye memakai ledger sendiri (§2) karena BR-COM-01 berbeda dari notifikasi transaksional |
| Chat | `0002_chat` + `internal/handler/chat.go` + `internal/store/chat.go` (`authorizeThread`, peserta, `chat_message_reads`). Status baca, peserta, dan arsip **sudah ada**. Yang ditambah: jenis guru–ortu, cek relasi, lampiran nyata, notifikasi, jam layanan, laporan |
| Cek relasi guru–siswa & wali–anak (fail closed) | salin pola `services/billing/internal/client/client.go` `TeacherStudentIDs` (wali kelas atau `teaching-assignments`) dan `GuardianHasAccess` ke `services/notification/internal/client/` |
| Storage lampiran | salin `services/billing/internal/storage/local.go` ke notification (belum punya storage) |
| Job berkala | salin `services/hr/internal/jobs/runner.go` ke `services/notification/internal/jobs/` |
| Promo/voucher menu 09 | `discount_vouchers` + `hashVoucher` (`admission/internal/handler/otp.go`) + `ApplyVoucher` (finance.go) |
| Titik "daftar ulang selesai" | `runConversion` (`admission/internal/handler/conversion.go`): transisi `reregistration → converted` (BR-PPDB-04/05 tuntas) |
| Halaman publik PPDB | `web/src/router/domains/admission.routes.ts` (`admissionPublicRoutes`, `meta.public`), `resolveSchoolID` di `admission/internal/handler/public.go` |

## 2. Migrasi
### `notification/0003_campaign_chat.up.sql`
```
-- Persetujuan promosi (BR-COM-01, OQ-43: tanpa baris = belum setuju = TIDAK dikirim)
marketing_consents(id, school_id, subject_key text /* 'user:<uuid>' | 'phone:<sha256>' | 'email:<sha256>' */, user_id NULL,
  channel CHECK IN('whatsapp','email','in_app'), status CHECK IN('opted_in','opted_out'), source CHECK IN('preferences','ppdb_form','unsubscribe_link','wa_keyword','admin_import'),
  changed_at, UNIQUE(school_id, subject_key, channel))
marketing_consent_events(id, school_id, subject_key, channel, status, source, actor_user_id NULL, created_at)   -- riwayat append-only
-- Segmen & kampanye (FR-147-*)
campaign_segments(id, school_id, name, kind CHECK IN('parents','ppdb_applicants','alumni_parents'), filter jsonb /* grade_levels, classroom_ids, period_id, statuses */,
  created_by, deleted_at)
wa_templates(id, school_id, name, provider_template_id text, language text, body_preview text, variables jsonb,
  approval_status CHECK IN('pending','approved','rejected'), deleted_at)          -- diisi humas sesuai status di penyedia; kirim hanya 'approved'
campaigns(id, school_id, name, segment_id, channel CHECK IN('whatsapp','email','in_app'), template_id NULL, subject NULL, body NULL,
  scheduled_at NULL, status CHECK IN('draft','scheduled','sending','sent','cancelled','failed'), created_by, sent_at NULL,
  CHECK(channel <> 'whatsapp' OR template_id IS NOT NULL))
campaign_recipients(id, school_id, campaign_id, subject_key, user_id NULL, contact_masked text, contact_hash text,
  status CHECK IN('queued','excluded','sent','delivered','read','replied','failed'), exclude_reason NULL CHECK IN('no_consent','opted_out','no_contact','quota'),
  provider_message_id NULL, last_error NULL, sent_at, delivered_at, read_at, replied_at, UNIQUE(campaign_id, subject_key))   -- NFR-ADV-06
campaign_settings(school_id PK, monthly_wa_quota int NULL)       -- NULL = tanpa kuota (sub-fitur "kuota", tanpa angka bawaan)
-- Chat guru–ortu (FR-148-*): perluasan 0002, tanpa menghapus kolom/baris lama
ALTER chat_threads ADD kind text NOT NULL DEFAULT 'general' CHECK(kind IN('general','teacher_parent')),
  ADD student_user_id uuid NULL, ADD teacher_user_id uuid NULL, ADD guardian_user_id uuid NULL, ADD archived_at timestamptz NULL,
  ADD CHECK(kind='general' OR (student_user_id IS NOT NULL AND teacher_user_id IS NOT NULL AND guardian_user_id IS NOT NULL))
CREATE UNIQUE INDEX ON chat_threads(school_id, teacher_user_id, guardian_user_id, student_user_id) WHERE kind='teacher_parent' AND closed_at IS NULL
ALTER chat_messages ADD attachment_name text NULL, ADD attachment_mime text NULL, ADD attachment_size bigint NULL,
  ADD outside_service_hours bool NOT NULL DEFAULT false, ADD school_id uuid NULL      -- diisi dari thread, untuk query audit
chat_settings(school_id PK, service_start time NULL, service_end time NULL, service_days int[] NULL /* 1=Senin..7 */, max_attachment_bytes bigint NOT NULL DEFAULT 5242880)  -- jam NULL = tanpa jam layanan; batas 5 MB mengikuti FR-16-002
chat_message_reports(id, school_id, message_id, reporter_user_id, reason, status CHECK IN('open','reviewed','dismissed'), reviewed_by NULL, review_note NULL, reviewed_at NULL,
  UNIQUE(message_id, reporter_user_id))
```
Pesan **tidak pernah dihapus** (tidak ada endpoint delete; FR-148-004, NFR-ADV-06). Retensi mengikuti kebijakan klien: tidak ada purge otomatis di task ini (dicatat di handoff).

### `admission/0004_marketing_referral.up.sql`
```
landing_pages(id, school_id, period_id, wave_id NULL, slug text, title, template_code CHECK IN('hero_cta','program_grid','faq_testimonial'),
  content jsonb /* section sesuai template, tanpa HTML mentah */, status CHECK IN('draft','published','archived'), published_at NULL, created_by, deleted_at,
  UNIQUE(school_id, slug))                                                                    -- FR-149-001
landing_visits(id, school_id, landing_page_id NULL, visitor_hash text, utm_source NULL, utm_medium NULL, utm_campaign NULL, referral_code_id NULL,
  visited_on date, created_at, UNIQUE(school_id, landing_page_id, visitor_hash, visited_on))  -- 1 kunjungan/pengunjung/halaman/hari, tanpa IP mentah
referral_programs(id, school_id, period_id /* periode tempat voucher berlaku */, reward_kind CHECK IN('fixed','percent'), reward_value bigint CHECK(reward_value>0),
  reward_cap_idr NULL, reward_valid_days int CHECK(reward_valid_days>0), priority int, active bool, created_by)   -- nilai diisi admin, TANPA default
referral_codes(id, school_id, code text, owner_user_id uuid /* identity */, owner_kind CHECK IN('parent','alumni'), status CHECK IN('active','revoked'),
  UNIQUE(school_id, code), UNIQUE(school_id, owner_user_id))
application_attributions(application_id PK REFERENCES applications(id), school_id, landing_page_id NULL, referral_code_id NULL,
  utm_source NULL, utm_medium NULL, utm_campaign NULL, created_at)                            -- FR-149-002
referral_rewards(id, school_id, referral_code_id, application_id UNIQUE, program_id NULL,
  status CHECK IN('pending_reregistration','issued','void'), void_reason NULL CHECK IN('self_referral','application_withdrawn','no_active_program','code_revoked'),
  voucher_id NULL REFERENCES discount_vouchers(id), voucher_code text NULL /* hanya ditampilkan ke pemilik kode & admin */, issued_at NULL,
  idempotency_key text UNIQUE)                                                                -- BR-REF-01
```

## 3. Fungsi murni + vektor
| Fungsi | Vektor → AC/FR |
|---|---|
| `campaign.Eligible(recipients, consents, channel)` → `(send, excluded)` | **AC-147-01** 500 penerima: 480 `opted_in`, 20 `opted_out` → 480 dikirim, 20 `excluded/opted_out`. Tanpa baris consent → `excluded/no_consent` (OQ-43). Kontak kosong → `no_contact` (FR-147-003) |
| `campaign.ApplyQuota(send, usedThisMonth, quota *int)` | quota NULL → semua; sisa kuota 10 dari 15 → 10 kirim, 5 `excluded/quota` |
| `campaign.Stats(recipients)` | terkirim/terbaca/dibalas/gagal/dikecualikan; `read` dan `replied` juga dihitung sebagai `sent` (FR-147-004) |
| `campaign.RenderTemplate(tpl, vars)` | variabel kurang → error; template status bukan `approved` → error (FR-147-002) |
| `chat.CanStartTeacherParent(teacherStudents, guardianHasChild)` | **AC-148-01** guru Matematika tidak mengajar 9B → siswa 9B tidak ada di set → tolak 403 "Anda tidak mengajar atau mewalikan siswa ini". Wali kelas siswa → boleh. Ortu yang bukan wali siswa → tolak (BR-COM-02) |
| `chat.ServiceWindow(settings, now, loc)` → `(outside bool, notice string)` | **AC-148-02** 07.00–17.00, pukul 21.00 WIB → outside, notice berisi jam layanan; 16.59 → inside; jam NULL → inside (FR-148-003) |
| `marketing.Attribute(visit, refCode, utm)` | ref valid & aktif → `referral_code_id`; ref tidak dikenal → diabaikan tanpa menggagalkan pendaftaran (FR-149-002) |
| `marketing.RewardDecision(appStatus, referrerUserID, applicantGuardianUserID, program, code)` | **AC-149-02** status `submitted`/`offered`/`reregistration` → `pending_reregistration`. **AC-149-01** `converted` + program aktif → `issued`. Pemilik kode = wali pendaftar → `void/self_referral`. `withdrawn` → `void/application_withdrawn` (BR-REF-01, FR-149-003) |
| `marketing.Funnel(visits, applications, groupBy)` | per sumber (utm_source / `referral`) dan per kode: kunjungan, pendaftaran (submitted+), konversi (`converted`), rasio dalam bp (FR-149-004) |
| `marketing.NewCode(rand)` | 8 karakter dari alfabet tanpa huruf/angka mirip (0/O, 1/I), unik per sekolah (retry saat bentrok UNIQUE) |

## 4. Integrasi & job
### 4.1 Adapter WhatsApp (IF-ADV-02, OQ-38)
- Env baru di `notification/internal/config`: `WHATSAPP_PROVIDER` (default **kosong = nonaktif**), plus kredensial penyedia saat OQ-38 diputuskan. Nilai `noop` diperbolehkan untuk dev/uji. Production + `dev` → gagal start (pola yang ada).
- `provider.NewWhatsApp(name)` mengembalikan `TemplateSender { SendTemplate(ctx, to, providerTemplateID, lang, vars) (msgID, error); ParseStatusWebhook(r) ([]StatusEvent, error) }`. Untuk saat ini hanya `noop` + `disabled`. **Jangan mengarang API penyedia**; adapter nyata ditambah setelah OQ-38 diputuskan.
- Nonaktif → membuat kampanye `channel='whatsapp'` ditolak `409 "Kanal WhatsApp belum dikonfigurasi"`. Kampanye email/in-app tetap jalan.
- Webhook status/balasan: `POST /webhooks/whatsapp` (tanpa JWT, verifikasi lewat adapter). Disabled → 404. Kata kunci berhenti (dikonfigurasi di adapter) → consent `opted_out`, `source='wa_keyword'`.

### 4.2 Segmen (FR-147-001): resolver di notification, data dibaca dari pemiliknya (tidak disalin)
| kind | Sumber |
|---|---|
| `parents` | academic `GET /classrooms?school_id=` + `/classrooms/{id}/students` (filter jenjang/kelas) → identity `GET /internal/students/{id}/guardians?school_id=` (**milik T9 §4.1**, gelombang yang sama: koding sesuai kontrak, uji dengan fake, jangan membuat salinan) → `GET /users/{id}` untuk email/telepon |
| `ppdb_applicants` | admission **baru** `GET /internal/marketing/segment-guardians?school_id=&period_id=&statuses=` → `{guardian_id, identity_user_id?, name, phone?, email?}` dari `guardians` + `guardian_contacts` terverifikasi |
| `alumni_parents` | academic: siswa dengan `class_enrollments.status='graduated'`. Kalau tidak ada endpoint baca dengan filter status, T13 menambah `GET /internal/enrollments?school_id=&status=graduated` di academic (file baru + 1 baris route, read-only) dan mencatatnya di handoff |
Kontak disimpan di `campaign_recipients` dalam bentuk **masked + hash**, tidak mentah. Kontak mentah dibaca ulang saat pengiriman.

### 4.3 Consent
- Portal (semua peran, termasuk `orang_tua`): toggle "Terima info promosi" per kanal di halaman preferensi yang ada → `PUT /marketing-consents`.
- Formulir PPDB: checkbox persetujuan (default **tidak** dicentang) → admission memanggil notification `POST /internal/marketing-consents {school_id, subject_key, channel, status, source:'ppdb_form'}`.
- Setiap email kampanye memuat tautan berhenti bertoken HMAC (`GET /public/unsubscribe?token=`) → `opted_out`.
- Notifikasi transaksional dan pengumuman (`/internal/notifications`) **tidak membaca** `marketing_consents`. Ada tes regresi untuk ini (BR-COM-01).

### 4.4 Kampanye & job (`internal/jobs`, interval env `CAMPAIGN_JOB_INTERVAL_SECONDS`)
1. `campaign_dispatch`: kampanye `scheduled` yang sudah jatuh tempo → `sending` → resolve segmen → `Eligible` → `ApplyQuota` → insert `campaign_recipients` (UNIQUE = idempoten bila job diulang) → kirim per batch: WA lewat `TemplateSender`; email lewat `Provider`; in-app lewat `CreateNotification` dengan `type='campaign'`. Selesai → `sent`.
2. Status in-app `read` diambil dari `notifications.read_at`. Status WA dari webhook. Email: `sent`/`failed`.
3. Audit: buat/jadwalkan/batal kampanye → `POST core /internal/audit` (module `communication`).

### 4.5 Chat guru–ortu (FR-148-*)
- `POST /chat/teacher-parent/threads {student_user_id, guardian_user_id?}`. Guru/wali kelas: cek `TeacherStudentIDs` (academic) + guardian benar wali siswa (identity), lalu thread `kind='teacher_parent'` dengan 2 peserta. Ortu: `{student_user_id, teacher_user_id}` → cek `GuardianHasAccess` + guru ada di set pengajar siswa. Thread aktif dengan trio yang sama → kembalikan yang ada (200).
- **Pengetatan thread `general` (BR-COM-02, fail closed):** `CreateThread` lama yang pesertanya memuat pasangan `orang_tua` ↔ `guru`/`walikelas` wajib lolos cek relasi yang sama. Kalau tidak lolos → 403. Thread admin ↔ ortu tidak berubah. Perubahan perilaku ini dicatat di handoff.
- Pesan: `POST /chat/threads/{id}/messages` diperluas. (a) Lampiran lewat `POST /chat/threads/{id}/attachments` (multipart; mime PDF/JPG/PNG; batas `max_attachment_bytes`) → `attachment_key` privat, unduh lewat `GET /chat/messages/{id}/attachment` (cek peserta). (b) `ServiceWindow` → `outside_service_hours` + respons `meta.service_notice`. (c) Notifikasi in-app ke peserta lain (`CreateNotification`, `Idempotency-Key: chat:<message_id>:<user_id>`). Status "terkirim" = pesan tersimpan, "dibaca" = `chat_message_reads` (sudah ada).
- Laporan: `POST /chat/messages/{id}/report {reason}` (peserta) → notifikasi ke `kepala_sekolah` sekolah itu. Moderator hanya membaca **thread yang punya laporan** lewat endpoint moderasi, bukan semua chat. Setiap pembukaan dicatat `core /internal/audit`.
- Arsip: `POST /chat/threads/{id}/archive` (peserta) mengisi `archived_at`. Pesan tetap tersimpan.

### 4.6 Landing page & referral (admission)
- Auth baru di `admission/internal/common/http.go`: `RoleAuth(secret, roles...)` (pola `AdminAuth`) untuk grup `humas`/`admin`/`superadmin` dan grup `orang_tua` (identity JWT). `AdminAuth` lama tidak diubah.
- Publik: `GET /public/landing/{slug}` (published saja, `resolveSchoolID`) dan `POST /public/landing/{slug}/visits {visitor_id, utm_*, ref}` (visitor_id acak dari browser, disimpan sebagai hash). CTA mengarah ke `/ppdb/otp?lp=<slug>&ref=<code>&utm_*`. SPA menyimpan atribusi di `sessionStorage` (try/catch), lalu mengirimnya bersama `POST /guardian/applications` (field opsional `attribution`, diperluas di `application.go`) → `application_attributions`. Kode referral valid → `referral_rewards(status='pending_reregistration', idempotency_key='ref:<application_id>')`.
- Kode referral (FR-149-003): `GET /me/referral` (orang_tua) membuat kode saat pertama dipanggil (idempoten) setelah cek kelayakan: punya anak aktif (identity children + academic enrollment aktif) → `parent`; hanya anak `graduated` → `alumni`; selain itu → 403.
- **Reward (BR-REF-01):** di `runConversion`, setelah `UPDATE applications SET status='converted'` berhasil, panggil `marketing.OnConverted(ctx, schoolID, applicationID)`. Fungsi ini idempoten (`referral_rewards.idempotency_key`): `RewardDecision`; jika `issued`, buat `discount_vouchers` (`code_hash = hashVoucher(code)`, `usage_limit 1`, `valid_from now`, `valid_until now + reward_valid_days`, `period_id/kind/value/cap/priority` dari program), simpan `voucher_id` + `voucher_code`, lalu notifikasi pemilik kode lewat notification `/internal/notifications` (`Idempotency-Key: referral-reward:<id>`). Gagal notifikasi tidak membatalkan reward. Tidak ada program aktif → `void/no_active_program` + audit. Transisi `withdrawn` → `void`.
- Audit semua mutasi ke `audit_events` admission yang sudah ada.

## 5. Endpoint
| Method & path | Peran | FR/AC |
|---|---|---|
| **notification** `CRUD /campaign-segments` · `GET /campaign-segments/{id}/preview` (jumlah total, eligible, dikecualikan per alasan) | humas, admin | FR-147-001/003 |
| `CRUD /wa-templates` | humas, admin | FR-147-002 |
| `CRUD /campaigns` · `POST /campaigns/{id}/schedule {scheduled_at}` · `POST /campaigns/{id}/send-now` · `POST /campaigns/{id}/cancel` · `GET /campaigns/{id}/stats` · `GET /campaigns/{id}/recipients?status=` | humas, admin | FR-147-002/004, AC-147-01 |
| `GET/PUT /campaign-settings` | admin | kuota |
| `GET/PUT /marketing-consents` (milik sendiri) · `POST /internal/marketing-consents` · `GET /public/unsubscribe` · `POST /webhooks/whatsapp` | semua peran · internal · publik · penyedia | BR-COM-01 |
| `POST /chat/teacher-parent/threads` · `GET /chat/teacher-parent/contacts?student_user_id=` (guru yang boleh dihubungi ortu / wali yang boleh dihubungi guru) | guru, walikelas, orang_tua | FR-148-001, AC-148-01 |
| `POST /chat/threads/{id}/attachments` · `GET /chat/messages/{id}/attachment` · `POST /chat/threads/{id}/archive` | peserta | FR-148-002 |
| `POST /chat/messages/{id}/report` · `GET /chat/moderation/reports?status=` · `GET /chat/moderation/reports/{id}` (thread terkait) · `POST /chat/moderation/reports/{id}/review` | peserta · kepala_sekolah, admin | FR-148-004 |
| `GET/PUT /chat-settings` | admin | FR-148-003 |
| **admission** `CRUD /admin/marketing/landing-pages` · `POST …/{id}/publish` · `POST …/{id}/archive` | humas, admin | FR-149-001 |
| `CRUD /admin/marketing/referral-programs` · `GET /admin/marketing/referral-codes` · `POST …/{id}/revoke` · `GET /admin/marketing/referral-rewards?status=` | admin (program), humas (baca) | FR-149-003, AC-149-01/02 |
| `GET /admin/marketing/report?period_id=&from=&to=&group_by=source\|referral_code` | humas, admin | FR-149-004 |
| `GET /me/referral` (kode + daftar referral + status + voucher yang sudah terbit) | orang_tua | FR-149-003, AC-149-02 |
| `GET /public/landing/{slug}` · `POST /public/landing/{slug}/visits` | publik | FR-149-001/002 |
| `GET /internal/marketing/segment-guardians` | internal (notification) | FR-147-001 |

## 6. Frontend
| Path | View | Peran | Label · navGroup · `meta.feature` |
|---|---|---|---|
| `komunikasi/kampanye` · `/:id` | `views/campaign/CampaignsView.vue`, `CampaignEditorView.vue` (langkah: segmen → kanal & template → jadwal → ringkasan: "500 penerima, 480 dikirim, 20 dikecualikan (berhenti berlangganan)") + tab Statistik (StatCard terkirim/terbaca/dibalas/gagal + tabel penerima, kontak masked) | humas, admin | Kampanye · **Komunikasi** · `campaign` |
| `komunikasi/segmen` · `komunikasi/template-wa` | `SegmentsView.vue`, `WaTemplatesView.vue` (badge status teks) | humas, admin | Segmen Penerima / Template WhatsApp · Komunikasi · `campaign` |
| (perluas) `NotificationPreferencesView.vue` + versi portal ortu | toggle "Terima info promosi" per kanal | semua | — |
| (perluas) `views/notification/ChatThreadsView.vue`, `ChatThreadView.vue`, `views/parent/ChatListView.vue`, `ChatThreadView.vue` | tombol "Mulai chat dengan orang tua/guru" (pilih siswa → kontak yang diizinkan), lampiran (input file berlabel, batas ukuran ditampilkan), centang terkirim/dibaca dengan teks alternatif, banner jam layanan, menu "Laporkan pesan" (dialog alasan), tombol arsip | guru, walikelas, orang_tua | (route lama) · fitur `teacher_parent_chat` untuk tombol baru |
| `komunikasi/moderasi-chat` | `views/notification/ChatModerationView.vue` | kepala_sekolah, admin | Moderasi Chat · Komunikasi · `teacher_parent_chat` |
| `ppdb/admin/landing-page` · `/:id` | `views/admission/admin/LandingPagesView.vue`, `LandingPageEditorView.vue` (pilih template, isi section, pratinjau, salin tautan + tautan dengan UTM) | humas, admin | Landing Page · **PPDB** · `ppdb_marketing` |
| `ppdb/admin/referral` | `ReferralView.vue` (tab Program / Kode / Reward: badge "Menunggu daftar ulang" / "Terbit" / "Batal") | admin, humas | Referral · PPDB · `ppdb_marketing` |
| `ppdb/admin/laporan-pemasaran` | `MarketingReportView.vue` (tabel funnel per sumber/kode + "Diperbarui: …") | humas, admin | Laporan Pemasaran · PPDB · `ppdb_marketing` |
| `/ppdb/l/:slug` (publik, `admissionPublicRoutes`) | `views/admission/public/MarketingLandingView.vue` (render template dari `content`, CTA ke OTP dengan atribusi) | publik | — |
| `orang-tua/referral` | `views/parent/ParentReferralView.vue` (kode + tombol salin/bagikan, daftar referral + status, voucher terbit) | orang_tua | Referral PPDB · Portal · `ppdb_marketing` |
Catatan: `admissionRoutes` lama sengaja tanpa `meta.feature`; route **baru** T13 tetap wajib `meta.feature`. Peran `humas` ditambah ke `meta.role` route baru saja. Tiga state + aksesibilitas §5 GEMINI.

## 7. Bukti handoff
Vektor §3. Tes AC wajib: **AC-147-01** (handler + fake segmen 500/20 → 480 baris `sent`, 20 `excluded`), **AC-148-01** (fake academic: guru tidak mengajar 9B → 403, tidak ada thread dibuat; jalur `CreateThread` general juga 403), **AC-148-02** (jam 21.00 WIB → 201 + `meta.service_notice`, `outside_service_hours=true`), **AC-149-01** (konversi sukses → voucher di `discount_vouchers`, panggil `OnConverted` 2x → satu voucher), **AC-149-02** (pendaftar baru bayar formulir → `pending_reregistration` di laporan & `/me/referral`). Tes lain: notifikasi transaksional tetap terkirim ke pengguna `opted_out`; WA nonaktif → 409; lampiran > batas → 413/422; moderator tanpa laporan → 403; pesan tidak bisa dihapus; migrasi notification 0001–0003 dan admission 0001–0004 up/down/up di DB baru.
FR tercakup: FR-147-001..004, FR-148-001..004, FR-149-001..004 (12). AC: AC-147-01, AC-148-01/02, AC-149-01/02 (5).
**Playwright (setelah merge):** humas buat segmen ortu kelas tertentu → pratinjau (dikecualikan per alasan) → kampanye email terjadwal → job → statistik. Guru A (tidak mengajar 9B) tidak menemukan ortu siswa 9B di kontak. Wali kelas 9B chat ortu → ortu balas pukul di luar jam layanan (atur jam layanan sempit) → banner tampil → ortu laporkan pesan → kepala_sekolah melihat di Moderasi. Humas terbitkan landing page → buka `/ppdb/l/<slug>?ref=<kode ortu A>&utm_source=ig` → daftar sebagai pendaftar B → laporan referral "Menunggu daftar ulang" → admin selesaikan daftar ulang + konversi → ortu A melihat voucher di `orang-tua/referral` → laporan pemasaran menunjukkan 1 kunjungan, 1 pendaftaran, 1 konversi untuk sumber `ig`.
