# notification-service

Fase 3 — **Notifikasi & Chat untuk Portal Orang Tua** di fork Teratai.
Milik satu domain: kotak masuk notifikasi, preferensi per-kanal, pengiriman
internal antar-service (billing/admission → wali/siswa), template pesan, status
& retry pengiriman, dan chat tenant-scoped antara sekolah ↔ wali/siswa.

Port `8089`, database `teratai_notification` (host dev `5489`). Lihat
`../../docs/CONVENTIONS.md` untuk aturan JWT/envelope/multi-tenant bersama.

Service ini **independen**: modul Go sendiri, DB sendiri, tidak ada Go package
bersama. Ia tidak memiliki data pengguna — `user_id` adalah UUID dari
identity-service, dan kontak email/wa di-resolve lewat internal HTTP
(`X-Internal-Key`) saat benar-benar dibutuhkan.

## Menjalankan

```bash
cp .env.example .env
docker compose up -d --build          # postgres (5489) + migrate + service

# migrasi manual (tanpa container):
go run ./cmd/migrate up
go run ./cmd/migrate down    # rollback satu langkah
go run ./cmd/migrate version

curl http://localhost:8089/healthz    # {"status":"ok"}
```

Jalankan binary di luar Docker: `go run ./cmd/server` (membaca env/.env).

## Autentikasi

| Area | Middleware | Kredensial |
|---|---|---|
| `GET /healthz` | — | tanpa auth |
| Portal (inbox, preferensi, chat) | `common.JWTMiddleware` | JWT identity HS256 (`Authorization: Bearer`) |
| Internal (kirim, template, retry) | `common.InternalOnlyMiddleware` | header `X-Internal-Key` |

Superadmin tidak punya `school_id` di token; ia harus mengirim `?school_id=`
pada route tenant. Semua query lain di-scope ke `school_id` (dan `user_id`
untuk data pribadi) dari klaim JWT — ini yang menjaga satu sekolah tidak pernah
melihat data sekolah lain.

## Endpoint

### Portal (JWT)

| Method | Path | Catatan |
|---|---|---|
| GET | `/notifications` | Milik pemanggil saja. Filter `?unread=true`, `?type=`, `?page=&per_page=` |
| GET | `/notifications/unread-count` | `{"data":{"count":N}}` untuk badge inbox |
| POST | `/notifications/{id}/read` | **Idempoten** (`read_at = COALESCE(read_at, now())`) |
| POST | `/notifications/read-all` | Menandai semua belum-dibaca, returns `{"marked":N}` |
| GET | `/notifications/{id}/deliveries` | Status per-kanal milik notifikasi sendiri |
| GET | `/notification-preferences` | 3 kanal efektif (stored + default) |
| PUT | `/notification-preferences` | `{"preferences":[{"channel":"email","enabled":true}]}` |
| GET | `/chat/threads` | Hanya thread tempat pemanggil peserta |
| POST | `/chat/threads` | `{"subject":..,"participants":[{"user_id","role"}],"participant_user_ids":[..]}` |
| GET | `/chat/threads/{id}/messages` | Wajib peserta (lihat otorisasi) |
| POST | `/chat/threads/{id}/messages` | `{"body":..,"attachment_key":..}` |
| POST | `/chat/threads/{id}/read` | Read receipt idempoten, returns `{"marked":N}` |

### Internal (`X-Internal-Key`)

| Method | Path | Catatan |
|---|---|---|
| POST | `/internal/notifications` | Kirim notifikasi; **`Idempotency-Key` wajib** |
| POST | `/internal/notifications/{id}/retry` | Retry hanya delivery `failed` |
| GET | `/internal/notification-templates` | `?school_id=&key=` |
| POST | `/internal/notification-templates` | `version<=0` → versi berikutnya |
| GET/PUT/DELETE | `/internal/notification-templates/{id}` | CRUD template terbatas admin internal |

### Mengirim notifikasi dari service lain

```bash
curl -X POST http://localhost:8089/internal/notifications \
  -H "X-Internal-Key: change-me" \
  -H "Idempotency-Key: billing:inv-123:paid" \
  -H "Content-Type: application/json" \
  -d '{
        "school_id":"11111111-1111-1111-1111-111111111111",
        "user_id":"33333333-3333-3333-3333-333333333333",
        "type":"billing.invoice.paid",
        "title":"Tagihan Lunas",
        "body":"Pembayaran Anda telah diterima.",
        "data":{"invoice_id":"inv-123"},
        "channels":["in_app","email"]
      }'
# 201 pertama; 200 (id sama, tanpa baris baru) pada retry dengan key yang sama
```

Retry dengan `Idempotency-Key` sama mengembalikan notifikasi yang sudah ada
(`200`, `created=false`) dan **tidak** menambah notifikasi/delivery baru. Kirim
ulang yang benar-benar baru memakai key baru → `201`.

## Model data

- **`notifications`** — `school_id, user_id, type, title, body, data jsonb,
  idempotency_key, read_at, created_at`. Index
  `(school_id, user_id, read_at)` untuk inbox + unread count. Unique
  `(school_id, idempotency_key)` menegakkan idempotensi internal send.
- **`notification_preferences`** — `(school_id, user_id, channel)` unique,
  `channel ∈ in_app|email|whatsapp`, `enabled`. Tidak ada baris = default
  (`in_app` on, `email`/`whatsapp` off).
- **`notification_templates`** — `(school_id, key, version, channel)` unique,
  `subject`, `body`. Versi baru = baris baru.
- **`notification_deliveries`** — `(notification_id, channel)` unique, `status
  ∈ pending|sent|failed`, `attempts`, `last_error`, `provider_message_id`.
- **`chat_threads`** / **`chat_thread_participants`** / **`chat_messages`** /
  **`chat_message_reads`** — thread tenant-scoped, peserta sebagai satu-satunya
  pihak yang boleh baca/tulis, receipt per `(message, user)`.

## Env

| Var | Default | Catatan |
|---|---|---|
| `PORT` | `8089` | |
| `APP_ENV` | `development` | `production` menolak dev default |
| `DATABASE_URL` | `postgres://teratai:change-me@localhost:5489/teratai_notification?sslmode=disable` | compose standalone pakai host `notification-db` |
| `SEKOCI_JWT_SECRET` | `change-me` | wajib eksplisit & bukan dev di production |
| `SEKOCI_INTERNAL_KEY` | `change-me` | idem |
| `CORS_ALLOWED_ORIGINS` | `http://localhost:5173..5178` | |
| `NOTIFICATION_PROVIDER` | `dev` (dev) / `unconfigured` (prod) | `dev` dilarang di production |
| `IDENTITY_URL` | `http://localhost:8081` | untuk resolve email/telepon |
| `SENTRY_DSN` | — | opsional, kosong = nonaktif |
| `SENTRY_ENVIRONMENT` | `development` | |
| `POSTGRES_USER/PASSWORD/DB_NAME/DB_HOST_PORT` | `teratai`/`change-me`/`teratai_notification`/`5489` | hanya untuk `docker-compose.yml` |

### Aturan `APP_ENV=production`

`config.Load()` gagal start bila `SEKOCI_JWT_SECRET` atau `SEKOCI_INTERNAL_KEY`
kosong atau masih memuat `dev-`/`change-me`, atau `NOTIFICATION_PROVIDER=dev`.
Default provider di production adalah `unconfigured`, yang **menolak** setiap
kirim email/whatsapp dengan error eksplisit (tercatat di
`notification_deliveries.last_error`) sampai provider nyata dikonfigurasi.

## Pengiriman, status & retry

`internal/provider` adalah adapter keluar yang dapat di-inject:

- `dev` / `noop` → no-op yang mencatat `provider_message_id` sintetis. Di
  production service **menolak start** bila provider adalah no-op (`dev` sudah
  ditolak `config.Load`, `noop` ditolak saat startup).
- `unconfigured` → menolak setiap kirim dengan pesan jelas (production default).

`in_app` selalu tersedia dan langsung `sent` (barisnya sendiri adalah pesan
inbox). `email`/`whatsapp` hanya dikirim bila preferensi user mengaktifkannya;
alamat di-resolve dari identity-service, dan ketiadaan alamat dicatat sebagai
delivery `failed` (bukan error 500). Retry lewat
`POST /internal/notifications/{id}/retry` hanya menyentuh delivery `failed`.

## Pengujian

```bash
gofmt -l .      # bersih
go vet ./...    # bersih
go test ./...   # hijau
```

Uji yang **selalu jalan** (tanpa DB, pakai fake store in-memory):

- `internal/handler`: otorisasi thread lintas-user/lintas-sekolah 403/404,
  idempotensi mark-read, idempotensi pengiriman internal (`Idempotency-Key`),
  unread count + filter `?unread=true`, isolasi daftar thread, read receipt
  idempoten.
- `internal/provider`: `dev` ditolak di production, `unconfigured` menolak kirim,
  no-op aman di dev, nama provider tak dikenal ditolak.
- `internal/config`: default dev, penolakan dev default di production, config
  production eksplisit diterima.

Uji store terhadap Postgres asli (dilewati otomatis bila tak ada DB):

```bash
NOTIFICATION_TEST_DATABASE_URL="postgres://teratai:change-me@localhost:5489/teratai_notification?sslmode=disable" \
  go test ./internal/store/ -v
```

Menguji idempotensi `(school_id, idempotency_key)` di SQL, idempotensi
mark-read (`COALESCE`), dan scoping peserta chat.

## Asumsi

1. **`Idempotency-Key` wajib** pada `POST /internal/notifications`. Caller
   antar-service (billing/admission) memang selalu punya kunci bisnis stabil
   (mis. `invoice_id` + event); mewajibkannya lebih aman daripada diam-diam
   membuat pesan ganda.
2. **Channel default `in_app`.** Bila caller tidak menyebut `channels`, hanya
   inbox yang dibuat. Email/WhatsApp opt-in lewat preferensi user.
3. **`in_app` tidak lewat provider.** Baris `notification_deliveries` itu
   sendiri adalah bukti pesan inbox; `provider_message_id = "in_app"`.
4. **Chat: otorisasi = peserta.** Thread sekolah lain → `404` (tidak membocorkan
   keberadaannya); thread sekolah sama tapi bukan peserta → `403`.
5. **Template hanya untuk konten `in_app`** pada fase ini; `FindNotificationTemplate`
   memakai versi tertinggi per `(key, channel)`.
6. **Kontak pengguna bukan milik service ini.** Resolusi email/telepon
   best-effort ke identity-service; kegagalan = delivery `failed`, bukan
   kegagalan request.
7. **IDs & baca**: id notifikasi/thread/pesan UUID (`gen_random_uuid()`).
   `chat_messages.attachment_key` menyimpan *kunci storage privat*, bukan URL
   publik — service ini tidak pernah menyimpan token/kode rahasia dalam bentuk
   plaintext.

## Wiring (di luar ownership service ini)

`docker-compose.yml` root, `nginx`, dan `.env` di root di-wire oleh integrator
(mengikuti pola service lain): tambahkan blok `notification-migrate` +
`notification` dengan `NOTIFICATION_DB_NAME=teratai_notification`,
`NOTIFICATION_PORT=8089`, dan `IDENTITY_URL: http://identity:${IDENTITY_PORT}`.
Service ini tidak menyentuh file di luar `services/notification/**`.
