# billing-service

Sumber kebenaran **SPP / tagihan sekolah** untuk Portal Orang Tua Sekolah
Teratai: skema tagihan (SPP bulanan, uang gedung, seragam, kegiatan),
penagihan massal per periode, beasiswa/diskon berurutan dengan cap, verifikasi
bukti transfer, koreksi lewat reversal, tunggakan, dan ekspor rekonsiliasi.

Port `8090`, database `teratai_billing` (host dev `5490`). Lihat
`../../docs/CONVENTIONS.md` untuk aturan JWT/envelope/multi-tenant bersama.

## Menjalankan

```bash
cp .env.example .env
docker compose up -d --build          # postgres (5490) + migrate + service

# migrasi manual (tanpa container):
migrate -path migrations \
  -database "postgres://teratai:change-me@localhost:5490/teratai_billing?sslmode=disable" up

curl http://localhost:8090/healthz    # {"status":"ok"}
```

`go run ./cmd/server` juga jalan langsung (butuh Postgres di `DATABASE_URL`).

## Auth

Tiga jalur autentikasi:

| Area | Middleware | Kredensial |
|---|---|---|
| `/parent/**` | `common.JWTMiddleware` + `RequireParent` | JWT identity, `active_role=orang_tua`, `school_id` wajib |
| `/admin/**` | `common.JWTMiddleware` + `RequireAdmin` | JWT identity, `active_role` ∈ `admin|superadmin` |
| `/internal/**` | `common.InternalAuth` | header `X-Internal-Key` |

Superadmin tanpa `school_id` di token memilih tenant lewat `?school_id=`.
Admin sekolah selalu di-scope ke `school_id` dari JWT.

**Kepemilikan anak (fail closed).** Setiap request wali memverifikasi
`guardianUserId = sub` benar-benar wali dari `studentUserId` lewat identity:

```
GET {IDENTITY_URL}/internal/guardians/{guardianUserId}/students/{studentUserId}/access?school_id=<uuid>
X-Internal-Key: <SEKOCI_INTERNAL_KEY>
```

Jika identity tidak menjawab 2xx, tidak terkonfigurasi, atau menjawab "tidak
punya akses", request wali dijawab **404** (tidak membocorkan id siswa lain).
Kontrak ini diasumsikan; billing tidak menyentuh DB identity.

## Peta endpoint

**Wali (Portal Orang Tua)**
- `GET /parent/children/{studentUserId}/bills` — daftar tagihan + outstanding
- `GET /parent/children/{studentUserId}/bills/{billId}` — detail (items, snapshot diskon, pembayaran)
- `POST /parent/children/{studentUserId}/bills/{billId}/payments` — multipart unggah bukti
- `GET /parent/children/{studentUserId}/payments` — riwayat pembayaran
- `GET /parent/children/{studentUserId}/payments/{paymentId}/proof` — stream bukti (privat, ber-otorisasi)

**Admin (school-scoped)**
- `GET/POST /admin/schemes`, `GET/PUT/DELETE /admin/schemes/{id}` — konfigurasi skema + komponen
- `POST /admin/bills/generate` — generate massal idempoten per (skema, siswa, periode)
- `GET /admin/bills` — daftar tagihan (`?student_user_id=&status=&scheme_id=&period_year=&period_month=`)
- `GET /admin/arrears` — daftar tunggakan (outstanding > 0)
- `GET /admin/payments` — antrean verifikasi (`?status=`)
- `POST /admin/payments/{id}/review` — verify/reject (+ alokasi)
- `POST /admin/payments/{id}/reversal` — koreksi beralasan (append-only)
- `GET /admin/payments/{id}/proof` — stream bukti
- `GET /admin/reconciliation.csv` — ekspor rekonsiliasi CSV

**Internal (X-Internal-Key)**
- `POST /internal/bills/generate` — pemicu scheduler/layanan lain (idempoten per periode+student)
- `GET /internal/students/{studentUserId}/outstanding?school_id=`

Operasi tulis menerima header `Idempotency-Key`.

## Aturan keuangan

- **Uang = integer rupiah (bigint).** Tidak ada float di mana pun; persen diskon
  disimpan basis points (10000 = 100%).
- **Diskon/beasiswa berurutan dengan cap.** Diurutkan naik berdasarkan
  `priority`, lalu `created_at`, lalu `id` (tie-break stabil). Tiap langkah
  menyimpan snapshot audit `base_before`/`requested`/`applied`/`base_after`;
  cap membatasi nilai yang benar-benar diterapkan; lantainya nol.
- **Outstanding = net tersnapshot − total alokasi verified + total reversal**,
  tidak pernah negatif (kelebihan bayar → 0).
- **Pembayaran verified bersifat immutable.** Status hanya `pending → verified`
  atau `pending → rejected`. Koreksi selalu lewat `bill_payment_reversals`
  beralasan; reversal memulihkan persis sebesar yang dibalik.
- **Generate idempoten** lewat `UNIQUE(school_id, student_user_id, period_year,
  period_month, scheme_id)` + planner murni yang membuang kandidat yang sudah ada
  (dan duplikat dalam batch). Menjalankan periode yang sama dua kali → `generated=0`.
- `billing_audit_events` append-only untuk setiap aksi finansial.

## Keamanan bukti transfer

Bukti transfer hanya disimpan di **object storage privat** (`STORAGE_ROOT`),
tidak pernah base64 di DB. Metadata yang tersimpan: `object_key`, `file_name`,
`mime`, `size_bytes`, `sha256`. Tipe MIME dibatasi (`application/pdf`,
`image/jpeg`, `image/png`), ukuran dibatasi `MAX_UPLOAD_BYTES`, dan checksum
sha256 dihitung saat streaming. File hanya keluar lewat endpoint
ber-otorisasi (wali pemilik / admin sekolah) dengan `Cache-Control: private,
no-store`.

## Pengujian

Logic murni diuji tanpa DB:

```bash
go test ./...
go vet ./...
gofmt -l .
```

- `internal/model`: status tagihan, state machine pembayaran, alokasi,
  outstanding + invarian reversal, engine diskon (urutan/cap/snapshot/lantai
  nol), planner generate idempoten, jatuh tempo akhir bulan.
- `internal/client`: kontrak identity access (path/header), gagal-tertutup
  pada 4xx/5xx, body rusak, dan saat tidak terkonfigurasi.
- `internal/handler`: otorisasi wali lintas siswa ditolak (path HTTP + unit),
  fail-closed saat identity error, role admin ditolak di endpoint wali.

## Asumsi

1. **Identity menyediakan endpoint access** di atas. Selama belum ada, semua
   request wali gagal-tertutup (404) — tidak ada kebocoran data siswa.
2. **Daftar siswa per kelas dikirim eksplisit** (`student_user_ids`). Billing
   tidak memanggil academic untuk resolusi keanggotaan kelas; SPA/scheduler
   mengirim id siswa yang sudah diresolusi.
3. **Satu pembayaran menetapkan satu tagihan.** Jika `allocations` tidak
   dikirim saat verify, pembayaran dialokasikan penuh ke tagihan tersebut.
4. Skema diarsipkan (soft), tidak dihapus keras, agar tagihan lama tetap
   mereferensikan skemanya.
