# admission-service

Sumber kebenaran **PPDB + portal orang tua Sekolah Teratai**: konfigurasi
periode/gelombang/jalur, sesi wali berbasis OTP, applicant & application,
berkas, biaya/diskon/voucher, pembayaran, CBT, seleksi/ranking, daftar ulang,
dan konversi applicant → siswa.

Port `8088`, database `teratai_admission` (host dev `5487`). Lihat
`../../docs/CONVENTIONS.md` untuk aturan JWT/envelope/multi-tenant bersama, dan
`/Users/.../plans/teratai-ppdb-parent-portal.md` untuk keputusan produk.

## Menjalankan

```bash
cp .env.example .env
docker compose up -d --build          # postgres (5487) + migrate + service

# migrasi manual (tanpa container):
migrate -path migrations \
  -database "postgres://teratai:change-me@localhost:5487/teratai_admission?sslmode=disable" up

curl http://localhost:8088/healthz    # {"status":"ok"}
```

`migrate down` mengembalikan skema persis ke kondisi sebelum `0002`
(`0002_admission_portal.down.sql`). Kalau mau menjalankan service di luar
Docker: `go run ./cmd/server`.

## Auth

Tiga jalur autentikasi:

| Area | Middleware | Kredensial |
|---|---|---|
| Publik | — | tanpa auth |
| `?/guardian/**` | `common.GuardianAuth` | JWT sesi wali (HS256, `scope=admission:guardian`) via `Authorization: Bearer` **atau** header `X-Guardian-Token` |
| `/admin/**` | `common.AdminAuth` | JWT identity, `active_role` ∈ `admin|superadmin` |

Superadmin tidak punya `school_id` di token; tenant diambil dari
`?school_id=` (SPA mengirim otomatis). Admin sekolah selalu di-scope ke
`school_id` dari JWT.

## Peta endpoint (kontrak frontend `web/src/services/admission/index.ts`)

**Public**
`GET /public/discovery`, `POST /public/otp/{request,verify,resend}`

**Guardian** (semua butuh token sesi wali)
`GET /guardian/dashboard` ·
`GET/POST /guardian/applicants`, `GET/PUT/DELETE /guardian/applicants/{id}` ·
`POST /guardian/applications`, `GET /guardian/applications/{id}`,
`POST /guardian/applications/{id}/submit` ·
`GET/POST /guardian/applications/{id}/documents`,
`DELETE /guardian/applications/{id}/documents/{docId}` ·
`GET /guardian/applications/{id}/charge`,
`POST /guardian/applications/{id}/charge/voucher` ·
`GET/POST /guardian/applications/{id}/payments` ·
`GET /guardian/applications/{id}/cbt`, `POST .../cbt/enter` ·
`GET .../decision`, `POST .../decision/respond` ·
`GET .../reregistration`

**Admin**
`GET /admin/dashboard` ·
`GET /admin/applications`, `GET /admin/applications/{id}`,
`POST /admin/applications/{id}/{request_correction,mark_eligible,reject}` ·
`GET/POST/PUT /admin/config/{periods,waves,tracks,document-requirements}[/{id}]` ·
`GET /admin/documents`, `POST /admin/documents/{id}/review` ·
`GET/POST/PUT /admin/finance/{fee-schemes,discount-rules,vouchers}[/{id}]` ·
`GET /admin/payments`, `POST /admin/payments/{id}/{review,reversal}` ·
`GET/POST/PUT /admin/selection/formulas[/{id}]`,
`GET /admin/selection/scores`, `PUT /admin/selection/scores/{applicationId}`,
`POST /admin/selection/rankings/preview`, `POST /admin/selection/rankings`,
`POST /admin/selection/decisions/{applicationId}` ·
`GET /admin/conversions`, `POST /admin/conversions/{id}/retry` ·
`GET /admin/audit-events`

Operasi finansial & konversi menerima header `Idempotency-Key`. Submit/transisi
memakai optimistic lock `{version}` dan `{reason,version}`.

## Konversi applicant → siswa (saga)

`internal/conversion` menjalankan langkah berurutan yang idempoten:

```
guardian (identity) → student (identity) → guardian link (identity) → enrollment (academic, opsional)
```

- Adaptor mengirim **data nyata** dari guardians/applicants dengan
  `external_ref` stabil (id wali/admission + applicant) dan `conversion_ref`
  (nomor pendaftaran) ke path identity yang benar:
  `POST /internal/guardians/upsert`, `POST /internal/students/create-or-link`,
  `POST /internal/guardian-links`.
- State setiap langkah (`guardian_step`, `student_step`, `link_step`,
  `enrollment_step`, `current_step`, id hasil) dipersist ke `student_conversions`
  sehingga kegagalan parsial bisa di-resume tanpa membuat user/link ganda.
- `POST /internal/admission/enrollments/upsert` milik academic masih boleh
  belum ada: 404 ditandai sebagai langkah `skipped`, konversi tetap `completed`.
- Identity mewajibkan `email` pada upsert user; bila wali hanya punya nomor HP,
  alamat deterministik `<nomor>@wali.ppdb.teratai.local` dibuat (dan untuk siswa
  `<applicant_id>@siswa.ppdb.teratai.local`) agar upsert tetap idempoten.

## Pengujian

Logic murni diuji tanpa DB:

```bash
go test ./...     # model (state machine, diskon, ranking), otp, conversion, client
go vet ./...
gofmt -l .
```

- `internal/model`: tabel transisi application & payment, `Outstanding`,
  validasi alokasi/reversal, engine diskon (urutan prioritas + tie-break,
  cap, zero floor, snapshot `base_before/after`), ranking berbobot + tie-break
  deterministik + immutability formula.
- `internal/otp`: hash deterministik, TTL, attempt cap, single-use, cooldown.
- `internal/conversion`: happy path, resume tanpa duplikasi, enrollment 404
  ditoleransi.

## Asumsi

1. **Satu sekolah.** Endpoint publik (`/public/*`) tidak menerima `school_id`
   dari SPA, jadi tenant diresolve dari `ADMISSION_SCHOOL_ID` atau periode
   terbuka terbaru.
2. **Token wali di header.** SPA menyimpan token sesi di `sessionStorage`; agar
   tetap aman, backend menerima token dari `Authorization` maupun
   `X-Guardian-Token` (frontend saat ini belum memasang interceptor penulisnya).
3. **Voucher menyimpan hash**, bukan kode plaintext; daftar admin hanya
   menampilkan `code_hint` ter-mask.
4. **Biaya = integer rupiah** (tanpa float). Berat formula disimpan basis points
   (10000 = 100%); form admin mengirim persen.
5. **Bukti transfer & berkas** masuk object storage privat (driver lokal saat
   ini), hanya diakses lewat endpoint berotorisasi.
