# T26: Aplikasi white-label (PWA): merek per sekolah, manifest & ikon, publikasi, notifikasi push

**Menu PRD:** 159 (IF-ADV-05, NFR-ADV-05, OQ-40 = PWA, CC-08, FR-07-006) · **Branch:** `feat/T26-white-label` · **Service:** `identity` (merek, endpoint publik manifest), `notification` (kanal `push`), `billing` (notifikasi tagihan terbit, tanpa migrasi), `web` (PWA)
**Migrasi:** `services/identity/migrations/0018_white_label.*` (milik T26) + **`services/notification/migrations/0004_push_channel.*`**. Nomor notification 0004 **belum** tercantum di tabel PARALLEL-v2. Orkestrator wajib mencadangkannya untuk T26 sebelum gelombang 5 (notification 0003 milik T13 di gelombang 3, dan tidak ada task gelombang 5 lain yang menyentuh notification).
**Prasyarat:** gelombang 4 di-merge. T9 (feature key `white_label` sudah ada di allowlist `FeatureKeys` `services/identity/internal/store/school_features.go`, jadi 0018 **tidak** menambah feature). T13 (notification 0003) sudah di-merge.
**Feature key:** `white_label`. **Peran:** `superadmin` (= Platform Admin FullCircle, bukan peran baru; T9 §2), `kepala_yayasan` (Yayasan Admin), penerima push: semua peran yang memakai portal (`orang_tua`, `siswa`, `guru`, `walikelas`). Nama sesuai **T9 §2**.
**Baca PRD:** Bagian 4 menu 159, §3 (App Branding, App Build per klien), §7 IF-ADV-05, §8 NFR-ADV-05, OQ-40, Lampiran (kanal push milik menu 07, menu 159 menyediakan aplikasinya). Bagian 1 FR-07-001/006, CC-08.

**Ruang lingkup sesuai OQ-40 (PWA):** "build per klien" = versi merek yang diterbitkan, lalu dilayani sebagai manifest + ikon + warna per domain dari satu build web yang sama. Build native **tidak** dikerjakan. Publikasi toko (FR-159-003) dicatat sebagai data listing + `assetlinks.json` agar klien/FullCircle dapat membungkusnya sebagai TWA Google Play di luar SEKOCI. iOS memakai "Tambahkan ke Layar Utama" (App Store native tidak dikerjakan). Batas ini wajib ditulis di handoff sebagai penyimpangan yang disengaja.

## 1. Acuan kode
- **Penyimpanan gambar:** identity menyimpan logo sebagai TEXT/data-URL (`schools.logo`, `0010_school_government_logo.up.sql`). Ikuti pola yang sama untuk ikon/splash, dengan batas ukuran env `BRANDING_IMAGE_MAX_BYTES` dan validasi PNG + dimensi.
- **Cakupan yayasan:** `services/identity/internal/handler/dashboard.go` (`schools.foundation_id`), dipakai untuk membatasi `kepala_yayasan` pada sekolah yayasannya.
- **Saklar fitur:** `store.SchoolFeatureMap` + `meta.feature` di router. Endpoint publik merek hanya mengembalikan data bila `white_label` aktif untuk sekolah itu.
- **Notification:** kanal tertutup `model.ValidChannels` + CHECK di `0001_notifications.up.sql` (3 tabel: `notification_preferences`, `notification_templates`, `notification_deliveries`); `dispatchChannel`/`recipientFor` di `internal/handler/internal.go` (preferensi dihormati, baris kosong = default); adapter `internal/provider/provider.go` (pola `UnconfiguredProvider`).
- **Billing:** `GenerateBills` (`internal/handler/admin.go:372`) dan `InternalGenerateBills` belum memberi tahu wali. Client notifikasi sudah ada: `internal/client/integrations.go` (`Notify`).
- **Web:** `web/index.html` (title & favicon SEKOCI statis), `web/vite.config.ts` (tanpa plugin PWA), `web/public/` (favicon), `views/LoginView.vue` & `layouts/AppLayout.vue` (teks/logo "SEKOCI"), token warna `--primary` di `web/src/style.css`, `stores/features.ts`, `views/notification/NotificationPreferencesView.vue`, `api/client.ts` `withAppBase`.
- **Dependensi baru (satu, wajib dijustifikasi di handoff):** `github.com/SherClockHolmes/webpush-go` di notification untuk enkripsi payload RFC 8291 + VAPID. Implementasi kripto manual berisiko. Web **tidak** menambah `vite-plugin-pwa`: service worker ditulis tangan (`web/public/sw.js`).

## 2. Migrasi
**identity `0018_white_label.up.sql`** (FR-159-001/002/003)
```
app_brandings(id uuid PK, school_id uuid NOT NULL UNIQUE REFERENCES schools(id), app_name text NOT NULL, short_name text NOT NULL CHECK(char_length(short_name)<=12),
  theme_color text NOT NULL CHECK(theme_color ~ '^#[0-9a-fA-F]{6}$'), background_color text NOT NULL CHECK(background_color ~ '^#[0-9a-fA-F]{6}$'),
  icon_192 text NULL, icon_512 text NULL, icon_maskable text NULL, splash text NULL,        -- data URL PNG
  domain text NULL UNIQUE CHECK(domain = lower(domain)),                                     -- host tanpa skema/port
  included_portals text[] NOT NULL DEFAULT '{parent,student,teacher}' CHECK(included_portals <@ ARRAY['parent','student','teacher']),
  privacy_policy_url text NULL, support_email text NULL, updated_by uuid, created_at, updated_at)
app_branding_versions(id uuid PK, school_id uuid NOT NULL, version int NOT NULL, snapshot jsonb NOT NULL,   -- tanpa data URL; ikon disalin ke kolom di bawah
  icon_192 text, icon_512 text, icon_maskable text, splash text, published_by uuid, published_at timestamptz, UNIQUE(school_id, version))
app_store_listings(id uuid PK, school_id uuid NOT NULL, store CHECK IN('google_play','app_store'), account_owner CHECK IN('client','fullcircle'),
  package_id text NULL, sha256_fingerprints text[] NOT NULL DEFAULT '{}', status CHECK IN('not_started','in_review','published','rejected'),
  listing_url text NULL, note text NULL, updated_by uuid, created_at, updated_at, UNIQUE(school_id, store))
```
Draf = `app_brandings`. Yang dilayani ke publik = **versi terbit terbaru** (`app_branding_versions`), sehingga perubahan draf tidak langsung tampil (AC-159-01 "build baru dibuat"). Hex di DB diperbolehkan (aturan "tanpa hex" berlaku untuk komponen). Warna diterapkan sebagai variabel CSS saat runtime. Down: drop berurutan terbalik.

**notification `0004_push_channel.up.sql`** (FR-159-004)
```
-- perlebar CHECK channel di notification_preferences, notification_templates, notification_deliveries → ('in_app','email','whatsapp','push')
push_subscriptions(id uuid PK, school_id uuid NOT NULL, user_id uuid NOT NULL, endpoint text NOT NULL UNIQUE, p256dh text NOT NULL, auth text NOT NULL,
  user_agent text NULL, created_at, last_success_at NULL, revoked_at NULL)
INDEX push_subscriptions(school_id, user_id) WHERE revoked_at IS NULL
```
Down: hapus baris ber-channel `push` di tiga tabel **sebelum** mengembalikan CHECK lama (beri komentar), lalu drop `push_subscriptions`.

## 3. Fungsi murni + vektor
| Fungsi | Vektor |
|---|---|
| `ValidateBranding(b) []FieldError` (Go) | **AC-159-01** "Teratai App" + hijau valid; `short_name` > 12 → galat; hex tidak valid → galat; terbitkan tanpa `privacy_policy_url` → galat "Kebijakan privasi wajib sebelum diterbitkan" (NFR-ADV-05); ikon bukan PNG 192/512 → galat |
| `ContrastRatio(fg, bg)` (Go + TS, rumus WCAG 2.x) | teks putih di `theme_color` < 4,5 → peringatan + teks otomatis gelap. Vektor: `#FFFFFF` vs `#000000` = 21 |
| `BuildManifest(version, baseURL) Manifest` | `name/short_name` = merek, `theme_color`, `background_color`, `display:'standalone'`, `start_url`/`scope` = base app, ikon 192/512/maskable dengan `?v=<version>`; **tidak ada** string "SEKOCI" (AC-159-01) |
| `NormalizeHost(hostHeader)` | `Teratai.Example.ID:443` → `teratai.example.id` |
| `PortalAllowed(role, portals) bool` (TS) | `orang_tua`→parent, `siswa`→student, `guru`/`walikelas`→teacher; peran lain di mode standalone → halaman "Portal ini tersedia di versi web" (FR-159-002) |
| `hexToHsl(hex)` (TS) | `#16A34A` → nilai HSL untuk `--primary` |
| `ShouldPush(prefRow, hasActiveSubscription) bool` (Go, notification) | belum ada baris preferensi + punya langganan → true; preferensi push mati → false (CC-08); tanpa langganan → false |
| `IsGoneStatus(code)` | 404/410 dari layanan push → langganan dicabut (`revoked_at`) |

## 4. Integrasi & job
- **Identity publik (tanpa JWT, aman di-cache singkat):** `GET /public/branding?host=` → versi terbit + `school_id`, hanya jika `white_label` aktif, selain itu `404` (SPA memakai merek default). `GET /public/manifest.webmanifest?host=` (`Content-Type: application/manifest+json`). `GET /public/branding/{schoolId}/{kind}.png?v=` (decode data URL, `Cache-Control` panjang karena ber-versi). `GET /public/assetlinks.json?host=` → Digital Asset Links dari `app_store_listings(google_play)` (`package_id`, `sha256_fingerprints`). Nginx (`web/nginx.conf`, tambah baris): `location = /.well-known/assetlinks.json` → identity `/public/assetlinks.json` dengan `X-Forwarded-Host`. Host diambil dari `X-Forwarded-Host`/`Host`. Pemalsuan header tidak berbahaya karena datanya publik.
- **Identity internal (sudah ada dari T9 §4.1, gelombang 3 — jangan dibuat ulang):** `GET /internal/students/{studentUserId}/guardians?school_id=`, dipakai billing.
- **Notification (FR-159-004, CC-08, IF-ADV-05):**
  - `GET /push/config` → `{enabled, vapid_public_key}`. `POST /push/subscriptions` `{endpoint, keys:{p256dh, auth}}` (upsert per endpoint, sekaligus set preferensi `push=true` untuk user itu = persetujuan eksplisit). `DELETE /push/subscriptions` `{endpoint}`. JWT, semua peran, milik sendiri.
  - `model.ValidChannels` + `ChannelPush`. Push adalah **kanal tambahan menu 07**: di `InternalSendNotification`, setelah kanal yang diminta diproses, `push` ikut didispatch bila `ShouldPush` true, walau pemanggil tidak memintanya (pemanggil lama tidak perlu diubah). **Pengecualian:** notifikasi `type='campaign'` (kampanye promosi T13, dibuat lewat `CreateNotification` di job `campaign_dispatch`) **tidak** pernah mendapat push otomatis, karena persetujuan kampanye dicatat per kanal (BR-COM-01, OQ-43) dan push bukan kanal kampanye. Test wajib. Satu baris `notification_deliveries(channel='push')` per notifikasi. Status gagal per langganan ditulis ke `last_error`.
  - Adapter `internal/provider/webpush.go`: env `WEBPUSH_VAPID_PUBLIC_KEY`, `WEBPUSH_VAPID_PRIVATE_KEY`, `WEBPUSH_SUBJECT`, `WEBPUSH_TTL_SECONDS`. Kunci kosong → delivery `failed` "push belum dikonfigurasi" (pola `UnconfiguredProvider`), tidak panik. Payload hanya `{title, body, url, notification_id}`, tanpa data sensitif.
  - Template notifikasi tetap per channel. Push memakai judul/isi notifikasi in-app.
- **Billing (AC-159-02, menyentuh billing tanpa migrasi):** setelah transaksi `GenerateBills`/`InternalGenerateBills` commit, untuk setiap tagihan **baru**: ambil wali dari identity (§ di atas), lalu `Notify` `{type:'bill_issued', title:'Tagihan baru terbit', body: periode + nominal (formatter rupiah), data:{bill_id, url:'/orang-tua/tagihan/<id>'}, channels:['in_app']}` dengan `Idempotency-Key: bill-issued:<bill_id>:<guardian_id>`. Dijalankan di goroutine dengan timeout. Gagal → Sentry, pembuatan tagihan **tidak** gagal. Push otomatis ikut lewat aturan notification di atas. (Cek path route tagihan ortu yang nyata di `web/src/router/domains/parent*.routes.ts` sebelum menulis `url`.)
- **Audit:** perubahan merek, penerbitan versi, perubahan listing toko → `POST {CORE_URL}/internal/audit` (pola async identity dari T1 §7: kalau core mati, aksi tidak gagal). Data URL tidak dimasukkan ke before/after, cukup `{"changed":["icon_192"]}`.
- **Tanpa job baru.** Pembersihan langganan mati terjadi saat kirim (410/404).

## 5. Endpoint
| Method & path | Peran |
|---|---|
| identity `GET /white-label/brandings?school_id=` · `PUT /white-label/brandings/{schoolId}` (draf) · `POST /white-label/brandings/{schoolId}/publish` · `GET /white-label/brandings/{schoolId}/versions` | superadmin (semua sekolah); kepala_yayasan (hanya sekolah satu yayasan, dicek server); sekolah wajib ber-feature `white_label` |
| identity `PUT /white-label/brandings/{schoolId}/domain` | superadmin saja (domain = urusan instalasi/DNS) |
| identity `GET/PUT /white-label/store-listings/{schoolId}/{store}` | superadmin (tulis), kepala_yayasan (baca) |
| identity `GET /public/branding` · `/public/manifest.webmanifest` · `/public/branding/{schoolId}/{kind}.png` · `/public/assetlinks.json` | publik |
| identity `GET /internal/students/{id}/guardians` | X-Internal-Key |
| notification `GET /push/config` · `POST/DELETE /push/subscriptions` | semua peran login |
| notification `GET/PUT /notification-preferences` (kini menerima `push`) | semua peran login (sudah ada) |

## 6. Frontend
- **Bootstrap PWA** `web/src/pwa/{branding.ts,register.ts,push.ts}` + `stores/branding.ts`: sebelum mount, `GET /public/branding?host=location.host`. Bila ada → set `document.title`, `<meta name="theme-color">`, `<link rel="manifest" href=withAppBase('/api/identity/public/manifest.webmanifest')>`, `<link rel="icon">`, `<link rel="apple-touch-icon">`, dan override `--primary`/`--ring` di `:root` (dari `hexToHsl`). Gagal/404 → merek default tanpa error di layar. `index.html` hanya diberi `<meta name="theme-color">` default dan hook. Title statis tetap sebagai fallback.
- **Service worker** `web/public/sw.js` (didaftarkan di `main.ts` hanya saat `import.meta.env.PROD` dan merek aktif): navigasi network-first dengan fallback `web/public/offline.html` (tanpa merek SEKOCI, judul diambil dari cache merek); **respons API tidak di-cache** (privasi & kesegaran data); event `push` → `showNotification(title, {body, icon, data:{url}})`; `notificationclick` → fokus/buka `data.url`. Versi SW mengikuti versi merek agar ikon/warna baru terambil.
- **Bebas merek SEKOCI (AC-159-01):** `LoginView.vue` dan header `AppLayout.vue` membaca `useBrandingStore`: jika merek aktif, tampilkan ikon + `app_name` + footer "© <tahun> <app_name>" + tautan kebijakan privasi, dan sembunyikan teks/logo SEKOCI. Tanpa merek → tampilan sekarang tidak berubah.
- **Mode standalone:** `matchMedia('(display-mode: standalone)')` + `PortalAllowed` di guard router. Peran di luar `included_portals` diarahkan ke `views/public/WebOnlyPortalView.vue`.
- **Preferensi push:** `NotificationPreferencesView.vue` ditambah baris "Push (aplikasi)" dan tombol "Aktifkan notifikasi di perangkat ini" (`Notification.requestPermission` → `pushManager.subscribe` dengan `vapid_public_key` → `POST /push/subscriptions`). Izin ditolak → pesan cara mengaktifkan di pengaturan browser. Status berteks (Aktif/Nonaktif/Diblokir browser). Tombol nonaktif selama request.

| Path | View | Peran | navGroup / navLabel | meta.feature |
|---|---|---|---|---|
| `pengaturan/white-label` | `views/identity/WhiteLabelView.vue`: pemilih sekolah (yayasan), tab **Merek** (nama, nama pendek, warna dengan `<input type=color>` + label + cek kontras), **Ikon & Splash** (unggah 192/512/maskable/splash + pratinjau), **Domain & Portal** (domain: superadmin; centang portal ortu/siswa/guru), **Publikasi** (riwayat versi, tombol "Terbitkan versi" dengan dialog konfirmasi + daftar galat validasi), **Toko Aplikasi** (status listing per toko, pemilik akun klien/FullCircle, package id, fingerprint, checklist kepatuhan NFR-ADV-05: kebijakan privasi, ikon, nama, kontak dukungan) + komponen `components/identity/AppPreview.vue` (tiruan layar ponsel dengan nama & warna) | superadmin, kepala_yayasan | Pengaturan / Aplikasi White-label | `white_label` |
| `komunikasi/preferensi-notifikasi` | (sudah ada) + baris Push | semua peran | (tetap) | — |
| `portal-web-saja` | `views/public/WebOnlyPortalView.vue` | publik (standalone) | (tidak di nav) | — |

## 7. Bukti handoff
- Test tabel §3 (Go & TS). Test handler identity: publik 404 saat `white_label` mati atau belum ada versi terbit; manifest memakai versi terbit (draf baru tidak bocor); `kepala_yayasan` mengubah sekolah yayasan lain → 403; domain ganda → 409; data URL tidak masuk audit.
- **AC-159-01:** set "Teratai App" + hijau → terbitkan → `GET /public/manifest.webmanifest` berisi nama & `theme_color` tersebut dan tidak memuat "SEKOCI". Playwright: domain merek = host uji (mis. `127.0.0.1`) → halaman login & dashboard ortu menampilkan "Teratai App", warna primer berubah, teks "SEKOCI" tidak ada di layar utama.
- **AC-159-02:** test notification: user punya langganan + preferensi default → `POST /internal/notifications` tanpa `push` di `channels` tetap membuat delivery `push` (provider fake menerima payload tanpa data sensitif); preferensi push dimatikan → tidak ada delivery push; respons 410 → `revoked_at` terisi. Test billing: `GenerateBills` membuat N tagihan → fake notification menerima N×wali panggilan dengan idempotency key unik, dan generate ulang tidak menggandakan notifikasi; fake notification mati → generate tetap 200.
- Migrasi: identity semua dari nol + 0018 up/down/up di `skci_t26_identity`; notification 0001→0004 up/down/up di `skci_t26_notification` (down berhasil walau ada baris push).
- Lighthouse/PWA check manual (dicatat di handoff): manifest valid, SW terdaftar, installable di Chrome Android; uji push di Chrome desktop dengan kunci VAPID dev.
- Cakupan: FR-159-001 (merek), FR-159-002 (versi terbit + portal sesuai modul aktif, PWA per OQ-40), FR-159-003 (listing + assetlinks, sebagian sesuai OQ-40), FR-159-004 (push) = 4 FR. AC-159-01, AC-159-02 = 2 AC. Juga IF-ADV-05 dan NFR-ADV-05 (inspeksi checklist).
- **Playwright** (setelah merge gelombang): superadmin mengaktifkan `white_label` untuk sekolah → kepala_yayasan mengisi merek & ikon → terbitkan → buka host bermerek → login ortu tanpa merek SEKOCI → preferensi notifikasi menampilkan baris Push → admin membuat tagihan → notifikasi in-app "Tagihan baru terbit" muncul untuk ortu (push diverifikasi lewat delivery row, karena Playwright tidak menerima push OS). Peran guru membuka `/pengaturan/white-label` → unauthorized.
