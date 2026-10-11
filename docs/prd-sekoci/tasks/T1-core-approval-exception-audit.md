# T1: Service `core`: Approval Center, Integration Exception Queue, Audit Trail pusat, peran baru

**Menu PRD:** 06, 79, 80 (+ peran untuk Bagian 2) · **Branch:** `feat/T1-core` · **Port:** 8091 · **DB:** `<prefix>_core` (ikuti pola nama DB di `.env.example`)
**Baca PRD:** Bagian 1 §9 (BR-GEN-09, BR-INT-01), menu 06, 07 (FR-07-002), 79, 80.
**Prasyarat:** working tree bersih (GEMINI.md §7.0).

Task ini membangun fondasi yang dipakai T2–T7. Kontrak internal (§3) **tidak boleh diubah** tanpa persetujuan reviewer, karena task lain bergantung padanya.

---

## 1. Kerangka service

Salin layout `services/billing/` menjadi `services/core/`:
```
services/core/
  cmd/server/main.go            # config, pool pgx, router, job runner (§5)
  internal/common/              # salin auth.go, response.go, http.go, internal.go, sentry.go dari billing (ganti nama package saja)
  internal/config/config.go     # env: PORT, DATABASE_URL, SEKOCI_JWT_SECRET, SEKOCI_INTERNAL_KEY, NOTIFICATION_URL, IDENTITY_URL, HR_URL (opsional), JOB_INTERVAL_SECONDS (default 300), SENTRY_DSN
  internal/handler/{router.go,approvals.go,approval_flows.go,delegations.go,exceptions.go,audit.go,internal.go,helpers.go}
  internal/store/{db.go,approvals.go,flows.go,exceptions.go,audit.go}
  internal/model/domain.go      # konstanta status + fungsi murni (resolve tahap, mask)
  internal/client/client.go     # panggil notification, identity, hr, dan callback sumber
  migrations/0001_core.up.sql / .down.sql
  Dockerfile, docker-compose.yml, README.md, .env.example
```
Daftarkan juga di `docker-compose.yml` (+ `core-migrate`), `docker-compose.local.yml`, `.env.example` (`CORE_PORT=8091`, `CORE_DB_NAME`), `web/src/api/client.ts` (`core`), `web/.env*` (`VITE_CORE_URL`). Untuk server, ikuti pola `/api/<svc>` yang sudah ada.

## 2. Migrasi `0001_core.up.sql`

Semua tabel memakai `id uuid PK default gen_random_uuid()`, `school_id uuid NOT NULL`, serta `created_at`/`updated_at timestamptz`.

```sql
-- Konfigurasi alur (FR-79-001/002)
approval_flows(id, school_id, request_type text NOT NULL, name text NOT NULL, active bool NOT NULL DEFAULT true,
  created_by uuid NOT NULL, created_at, updated_at, deleted_at,
  UNIQUE(school_id, request_type) WHERE deleted_at IS NULL)   -- partial unique index
approval_flow_steps(id, school_id, flow_id FK, seq int NOT NULL CHECK(seq>0),
  approver_kind text NOT NULL CHECK(approver_kind IN('role','direct_supervisor','user')),
  approver_role text NULL, approver_user_id uuid NULL,
  min_amount_idr bigint NULL,          -- tahap hanya berlaku jika amount >= nilai ini
  condition_flag text NULL,            -- tahap hanya berlaku jika flag ini ada di request.flags
  sla_hours int NULL CHECK(sla_hours>0),
  escalate_to_role text NULL,
  UNIQUE(flow_id, seq),
  CHECK((approver_kind='role') = (approver_role IS NOT NULL)),
  CHECK((approver_kind='user') = (approver_user_id IS NOT NULL)))

-- Permintaan (instance)
approval_requests(id, school_id, request_type text, source_service text, source_id text,
  title text NOT NULL, summary jsonb NOT NULL,        -- ringkasan untuk ditampilkan, TANPA data sensitif mentah
  amount_idr bigint NULL, flags text[] NOT NULL DEFAULT '{}',
  maker_user_id uuid NOT NULL, subject_user_id uuid NULL,   -- subject = orang yang dibicarakan (mis. pegawai yang cuti), dipakai untuk direct_supervisor
  status text NOT NULL CHECK(status IN('pending','approved','rejected','cancelled')),
  current_seq int NULL, callback_url text NOT NULL, correlation_id text NOT NULL,
  idempotency_key text NOT NULL, flow_snapshot jsonb NOT NULL,   -- salinan tahap yang berlaku saat dibuat
  decided_at timestamptz NULL, callback_status text NOT NULL DEFAULT 'pending' CHECK(callback_status IN('pending','done','failed')),
  UNIQUE(school_id, idempotency_key))
approval_steps(id, school_id, request_id FK, seq int, approver_kind, approver_role, approver_user_id,
  status text CHECK(status IN('waiting','pending','approved','rejected','skipped','escalated')),
  due_at timestamptz NULL, decided_by uuid NULL, decided_as_delegate_of uuid NULL, comment text NULL, decided_at timestamptz NULL,
  UNIQUE(request_id, seq))

-- Delegasi (FR-79-005)
approval_delegations(id, school_id, from_user_id uuid, to_user_id uuid, valid_from timestamptz, valid_to timestamptz,
  created_at, revoked_at NULL, CHECK(valid_to>valid_from), CHECK(from_user_id<>to_user_id))

-- Exception queue (FR-80-*)
integration_exceptions(id, school_id, source_service text, kind text CHECK(kind IN('webhook','journal_posting','notification','dead_letter','import','callback')),
  severity text CHECK(severity IN('normal','critical')), correlation_id text, error_message text,
  attempts int NOT NULL DEFAULT 0, max_attempts int NOT NULL,
  payload_summary jsonb NOT NULL,       -- WAJIB sudah dimasking oleh pengirim; core memasking ulang (defense in depth)
  retry_url text NULL,                  -- endpoint internal idempoten milik service sumber
  status text CHECK(status IN('open','retrying','resolved')), assignee_user_id uuid NULL,
  resolved_by uuid NULL, resolution_reason text NULL, resolved_at timestamptz NULL,
  idempotency_key text NOT NULL, UNIQUE(school_id, idempotency_key))

-- Audit pusat (FR-06-*)
audit_events(id bigserial PK, school_id uuid NULL /* null = event global superadmin */, service text, module text,
  entity_type text, entity_id text, action text, actor_user_id uuid NULL, actor_role text NULL,
  before_data jsonb NULL, after_data jsonb NULL, reason text NULL, correlation_id text NULL,
  ip_hash text NULL, created_at timestamptz NOT NULL DEFAULT now())
INDEX audit_events(school_id, created_at DESC), (school_id, module, created_at DESC), (actor_user_id, created_at DESC)
```
File `.down.sql` men-drop semuanya dengan urutan terbalik.

## 3. Kontrak internal (dipakai task lain, semuanya `X-Internal-Key`)

| Method & path | Body / hasil | Dipakai oleh |
|---|---|---|
| `POST /internal/approvals` | `{school_id, request_type, source_service, source_id, title, summary, amount_idr?, flags?, maker_user_id, subject_user_id?, callback_url, correlation_id, idempotency_key}` → `201 {id, status, current_seq}`. Panggilan ulang dengan idempotency_key sama → `200` + data yang sama. Kalau tidak ada flow aktif untuk `request_type` → `422 "alur approval <type> belum dikonfigurasi"` (bukan auto-approve). | T2–T7 |
| `GET /internal/approvals?source_service=&source_id=` | riwayat tahap (FR-79-007) | T2–T7 (ditampilkan di halaman transaksi asal) |
| `POST /internal/approvals/{id}/cancel` | `{actor_user_id, reason}`, hanya untuk status pending | sumber (mis. pegawai membatalkan cuti) |
| Callback **ke** sumber: `POST {callback_url}` | `{approval_id, source_id, request_type, decision:'approved'\|'rejected', decided_by, comment, decided_at}` + header `Idempotency-Key: approval:<id>`. Sumber **wajib idempoten**. Jika gagal setelah `max_attempts` → buat `integration_exceptions(kind='callback')`. | — |
| `POST /internal/exceptions` | `{school_id, source_service, kind, severity, correlation_id, error_message, payload_summary, retry_url?, max_attempts, idempotency_key}`. Idempotency_key sama → `attempts += 1` dan update error terakhir, tidak membuat baris baru. | T3, T6 |
| `POST /internal/audit` | `{school_id, service, module, entity_type, entity_id, action, actor_user_id, actor_role, before, after, reason, correlation_id}` → `202`. **Mask dulu field sensitif** (§4.3). | semua service baru + identity |

Tambahkan `internal/client` kecil di setiap service yang memakai kontrak ini. **Salin** file client (jangan impor lintas modul), pola `services/billing/internal/client/client.go`.

## 4. Logika (fungsi murni di `internal/model`, wajib dites tabel)

### 4.1 `ResolveSteps(flow []FlowStep, amountIDR *int64, flags []string) []Step`
- Tahap ikut jika `(min_amount_idr == nil || (amount != nil && *amount >= min))` **dan** `(condition_flag == nil || flag ada di flags)`.
- Urutan mengikuti `seq`. Kalau hasilnya kosong → error "tidak ada tahap berlaku" (422).
- Test: AC-79-02 (refund 6 jt: dua tahap. refund 1 jt: satu tahap), condition_flag `exceeds_headcount` (AC-88-01), `salary_out_of_range` (AC-92-01).

### 4.2 Siapa boleh memutuskan tahap (`CanDecide`)
- `role`: user punya peran itu (active_role atau `roles[]`) di school yang sama.
- `direct_supervisor`: core memanggil `GET {HR_URL}/internal/employees/by-user/{subject_user_id}/supervisor` (dibuat di T2). Jika HR belum aktif (HR_URL kosong / 404) → **422** saat request dibuat, dengan pesan jelas. Jangan diam-diam fallback ke peran lain.
- `user`: user_id cocok.
- Delegasi: user B boleh bertindak untuk A jika ada `approval_delegations` aktif saat ini. Simpan `decided_as_delegate_of = A`.
- **Maker-checker (BR-GEN-09, AC-79-01):** `decider == maker_user_id` → **403**, berlaku di tahap mana pun dan juga lewat delegasi. Item seperti ini tetap tampil di inbox dengan label "Menunggu penyetuju lain", tanpa tombol aksi.
- Tolak tanpa komentar (`trim(comment)==""`) → **422** (AC-79-03).

### 4.3 Masking (`MaskSensitive(map) map`)
Kunci yang dimasking (konstanta di satu tempat): `nik, npwp, account_number, bank_account, phone, email, salary, base_salary_idr, gross_idr, net_idr`.
- Untuk string, tampilkan 4 karakter terakhir saja (`••••1234`). Email: `a•••@domain`. Nilai uang diganti `"•••"`.
- Dipakai di `/internal/audit`, `/internal/exceptions`, dan respons GET exception (AC-80-03). Test wajib.

### 4.4 Transisi (dijalankan dalam satu transaksi DB dengan `SELECT … FOR UPDATE` pada request)
- approve tahap N → tahap N+1 berstatus `pending` (+`due_at = now + sla_hours`). Kalau tidak ada tahap berikutnya → request `approved`, lalu jalankan callback.
- reject → request `rejected`, sisa tahap `skipped`, lalu callback.
- Setiap transisi: tulis `audit_events` dan kirim notifikasi (FR-07-002) ke penyetuju berikut / ke maker lewat `POST {NOTIFICATION_URL}/internal/notifications` dengan `Idempotency-Key: approval:<id>:<seq>:<status>`, `type: 'approval'`, `data: {approval_id, source_service, source_id}`.

## 5. Job berkala (pola baru, buat **sekali** di core lalu disalin ke `hr` di T2)

`internal/jobs/runner.go`: `time.NewTicker(cfg.JobInterval)` + `pg_try_advisory_lock(<hash nama job>)` supaya aman kalau container lebih dari satu. Job di core:
1. **Eskalasi (FR-79-006, AC-79-04):** tahap `pending` dengan `due_at < now()` → status `escalated`, buat tahap pengganti untuk `escalate_to_role`, lalu kirim notifikasi ke dua pihak. Tahap tanpa `escalate_to_role` tidak dieskalasi.
2. **Retry callback:** request dengan `callback_status='failed'`/`pending` yang gagal. Backoff `attempts²` menit, maksimal `max_attempts`, lalu masuk exception.

## 6. Endpoint publik (JWT)

| Method & path | Peran | Keterangan |
|---|---|---|
| `GET /approval-flows` · `POST` · `PUT /{id}` · `DELETE /{id}` (soft) | superadmin, admin | CRUD flow + steps dalam satu payload. Validasi: seq berurutan mulai 1, role valid (ambil daftar dari identity `GET /internal/roles` atau konstanta peran yang sama dengan CHECK identity) |
| `GET /approval-request-types` | superadmin, admin | daftar `request_type` yang dikenal. Konstanta Go di `model`, satu sumber: `discount_special, fee_change, bill_adjustment, payment_correction, scholarship, refund, journal, recon_adjustment, selection_result, student_transfer, student_withdrawal, leave, overtime, attendance_correction, profile_change, requisition, offer, payroll_component_change, payroll_run, payroll_input_change, training_paid` |
| `GET /approvals/inbox?status=pending\|decided&page=` | semua peran login | yang bisa diputuskan user ini (termasuk lewat delegasi) + item buatan sendiri ditandai `can_decide=false` |
| `GET /approvals/mine` | semua | permintaan yang dibuat user |
| `GET /approvals/{id}` | maker, penyetuju terkait, admin | detail + tahap |
| `POST /approvals/{id}/approve` · `/reject` | penyetuju | body `{comment}` |
| `GET/POST/DELETE /approval-delegations` | semua (milik sendiri) | delegasi ke user dengan peran setara (validasi: `to_user` punya minimal satu peran penyetuju yang sama) |
| `GET /exceptions?kind=&status=&severity=&page=` | superadmin, admin: semua kind; finance_manager: `webhook`, `journal_posting`; payroll_officer: `journal_posting` dengan source_service `hr`; auditor: baca saja | FR-80-005: filter kind per peran **di server** |
| `POST /exceptions/{id}/retry` | sama | panggil `retry_url` (internal key), `attempts+1`. Sukses → `resolved` + reason "retry berhasil" |
| `POST /exceptions/{id}/assign` `{user_id}` · `/resolve` `{reason}` | sama | reason wajib (AC-80-02). Semua aksi diaudit |
| `GET /audit?user_id=&module=&from=&to=&page=` | superadmin, `auditor` | FR-06-003. Ekspor CSV mengikuti filter + diaudit (CC-04) |

## 7. Peran baru (migrasi identity `0014_hr_roles.up.sql`)

Perlebar `CHECK (role IN (...))` mengikuti pola `0013_parent_portal.up.sql`: tambahkan `hr_admin, hr_manager, payroll_officer, recruiter, training_admin, auditor, finance_manager, pegawai`. (`pegawai` = staf non-guru yang memakai ESS. Guru tetap `guru`. Pewawancara **bukan** peran: siapa pun yang ditugaskan sebagai panel di T4 boleh menilai. Kepala Unit = `kepala_sekolah`. Accounting memakai peran accounting yang sudah ada di Laravel.) File `.down.sql` mengembalikan CHECK lama. **Sebelum** CHECK diperketat, cek dulu: down gagal bila masih ada baris dengan peran baru, beri pesan yang jelas.
Tambahkan peran yang sama di daftar peran frontend (cari konstanta peran di `web/src/stores/auth.ts` / tipe role) dan label Indonesia-nya: HR Admin, HR Manager, Petugas Payroll, Rekruter, Admin Pelatihan, Auditor, Manajer Keuangan, Pegawai.

Identity juga harus mengirim **event keamanan** (FR-06-002) ke `POST /internal/audit` (module `security`, action `login_success|login_failed|account_locked|role_changed`) dari `services/identity/internal/handler/auth.go` (`Login`) dan dari handler ubah peran. Kirim **async** dengan timeout pendek. Kalau core mati, login tidak boleh gagal: catat error ke Sentry saja.

## 7b. Endpoint jurnal internal di accounting (Laravel), fondasi T3 & T6

Accounting belum punya pintu masuk jurnal otomatis: tabel `acc_posting_maps`/`acc_auto_postings` dan kolom `trans_head.source/reference/status` sudah ada (migrasi `2026_10_08_000001_…`), tetapi belum ada kode yang mengisinya. Buat **satu** endpoint yang dipakai payroll (T3) dan payment gateway (T6):

`POST /api/internal/journals` (routes/api.php), dengan middleware baru `InternalKey` yang membandingkan header `X-Internal-Key` dengan `config('app.sekoci_internal_key')` secara constant-time (`hash_equals`). Kunci kosong → tolak semua.
```json
{ "school_id": "uuid", "source": "payroll|payment_gateway", "reference": "payroll:<run_id>:accrual",
  "trans_date": "2026-10-31", "description": "Payroll Oktober 2026 SD",
  "lines": [ {"acc_code": "6101", "div_code": null, "description": "Gaji pokok", "debit_idr": 150000000, "credit_idr": 0}, … ] }
```
- Tenant: `Tenancy::activate($schoolId)` (pola `SsoController`). Kalau `accountingEnabled` false → 409 `"modul akuntansi tidak aktif"`.
- Validasi: minimal 2 baris; `sum(debit) == sum(credit)` dalam **integer rupiah**, bukan float; setiap `acc_code` ada di `mst_account` dengan `is_active`. Pelanggaran → 422 dengan daftar akun yang salah.
- **Idempoten (BR-INT-01, AC-98-02):** tambah migrasi unique index `trans_head(source, reference) WHERE reference IS NOT NULL`. Kalau reference sudah ada → **200** + `trans_no` yang lama, tidak membuat jurnal kedua.
- Insert `trans_head` (status `posted`, `source`, `reference`, `trans_user` = user sistem per tenant, cari pola `resolveUserCode`) + `trans_detail` dalam satu `DB::transaction`. Nomor `trans_no` mengikuti pola `generateTransNo` di `PriorYearAdjustmentService` dengan prefix per source (`PRL`, `PGW`) dan `ensureTransactionType`. Ekstrak ke `app/Services/AutoJournalService.php`, jangan disalin di controller.
- Respons `201 {trans_no}`. Test PHPUnit: seimbang, tidak seimbang → 422, reference ganda → 200 dengan trans_no sama, kunci salah → 401.
- Di sisi Go, client kecil `PostJournal(ctx, req) (transNo string, err error)` **disalin** ke service pemanggil (T3 dan T6), bukan dibagi lewat import.

## 8. Frontend

`web/src/services/core/{approvals.ts,exceptions.ts,audit.ts,types.ts}` dan `web/src/router/domains/core.routes.ts` (daftarkan di `router/index.ts`, cukup tambah satu import + spread).

| Path | Name | View | meta.role | navGroup / navLabel |
|---|---|---|---|---|
| `approval/inbox` | `approval-inbox` | `views/core/ApprovalInboxView.vue` | semua peran kecuali siswa, orang_tua | Approval / Kotak Masuk Approval |
| `approval/saya` | `approval-mine` | `views/core/ApprovalMineView.vue` | sama | Approval / Pengajuan Saya |
| `approval/{id}` | `approval-detail` | `views/core/ApprovalDetailView.vue` | sama | (tidak di nav) |
| `pengaturan/alur-approval` | `approval-flows` | `views/core/ApprovalFlowsView.vue` | superadmin, admin | Pengaturan / Alur Approval |
| `pengaturan/delegasi` | `approval-delegations` | `views/core/DelegationsView.vue` | penyetuju | Approval / Delegasi |
| `integrasi/antrean-galat` | `exception-queue` | `views/core/ExceptionQueueView.vue` | superadmin, admin, auditor | Sistem / Antrean Galat Integrasi |
| `audit` | `audit-trail` | `views/core/AuditTrailView.vue` | superadmin, auditor | Sistem / Audit Trail |

Detail per layar:
- **ApprovalInboxView**: tab "Menunggu" / "Sudah diputuskan". Tabel: jenis (label Indonesia dari map `requestTypeLabel` di `services/core/types.ts`), judul, pengaju, nominal (formatter rupiah bersama), tahap `n/total`, jatuh tempo SLA (badge merah bila lewat), tombol Detail. Item milik sendiri berlabel "Menunggu penyetuju lain". Badge jumlah pending ditampilkan di sidebar lewat store kecil `useApprovalBadge` (polling 60 detik, berhenti saat tab tersembunyi).
- **ApprovalDetailView**: ringkasan `summary` (render key → value generik, label dari map), timeline tahap (penyetuju, keputusan, waktu, komentar, "atas nama X" bila delegasi). Tombol Setujui / Tolak membuka `ui/dialog`. Textarea komentar wajib saat Tolak (tombol nonaktif + pesan inline). Kedua tombol nonaktif saat request berjalan.
- **Komponen bersama** `components/core/ApprovalTimeline.vue` (props: `sourceService`, `sourceId`). **Dipakai ulang** oleh T2–T7 di halaman transaksi asal (FR-79-007).
- **ApprovalFlowsView**: daftar flow per jenis. Editor tahap berupa tabel yang bisa ditambah/hapus/urutkan: jenis penyetuju (select), peran/user, min nominal (input rupiah), flag kondisi (select dari konstanta), SLA jam, eskalasi ke peran. Validasi klien sama dengan server, tetapi server tetap sumber kebenaran.
- **ExceptionQueueView**: filter jenis/status/severity. Baris bisa diperluas untuk menampilkan correlation ID (tombol salin), galat, percobaan, dan payload (sudah tersamar). Aksi Coba Lagi / Tugaskan / Tandai Selesai (dialog alasan wajib).
- **AuditTrailView**: filter pengguna (autocomplete dari identity), modul, rentang tanggal. Tabel waktu, pengguna, modul, aksi, entitas. Baris diperluas menampilkan diff before/after (komponen `components/core/JsonDiff.vue` sederhana: key berubah disorot). Tombol Ekspor CSV mengikuti filter.

## 9. Seed

Migrasi **tidak** membuat flow default (flow adalah keputusan sekolah). Tambahkan `scripts/seed-approval-flows.sql` sebagai contoh (leave → direct_supervisor; refund → admin, lalu kepala_sekolah bila ≥ 5.000.000) yang dijalankan manual untuk demo lokal.

## 10. Bukti yang diminta di handoff

- Test tabel: `ResolveSteps`, `CanDecide` (maker-checker, delegasi, peran), `MaskSensitive`, transisi approve/reject/eskalasi, idempotensi `POST /internal/approvals` dan `/internal/exceptions`.
- Test handler: AC-79-01..04, AC-80-01..03, AC-06-01 (pakai pola test handler di `services/notification/internal/handler/notifications_test.go`).
- Playwright: admin membuat flow → user A mengajukan (lewat endpoint uji / curl internal) → A tidak bisa menyetujui → B menyetujui → timeline tampil.
