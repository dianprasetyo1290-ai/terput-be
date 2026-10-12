# T11: Service Desk di service `core`: tiket, kategori, SLA & eskalasi

**Menu PRD:** 137, 138 (BR-SD-01, lampiran "Tiket layanan" & "Perbaikan fasilitas") · **Gelombang:** 3 · **Branch:** `feat/T11-service-desk` · **Service:** `core` (paket `internal/servicedesk`) · **Migrasi:** `services/core/migrations/0003_service_desk.*` (hanya nomor 0003)
**Prasyarat:** T1 + T7b sudah di-merge (`core` 0001/0002 ada). Paralel dengan T9 (gelombang 3): konversi ke perintah kerja (menu 118) dikoding sesuai kontrak §4.3, diuji dengan `httptest` fake. **Feature key:** `service_desk`. **Peran:** `servicedesk_agent`, `servicedesk_manager` (+ pelapor: `orang_tua`, `siswa`, peran staf). Nama peran/feature final **mengikuti T9 §2**; bila beda, ganti konstanta saat rebase dan catat di handoff.

## 1. Acuan kode
- Layout & helper: `services/core/internal/{common,client,config,jobs}` yang sudah ada. Paket baru `internal/servicedesk/` berisi `model.go` (fungsi murni), `store.go` (SQL, `*sql.DB` sama dengan store core), `handler.go`. Router: tambah grup di `internal/handler/router.go` (penambahan baris saja).
- Menu 81 yang sudah ada (T7b): `migrations/0002_service_requests.up.sql`, `internal/model/service_request.go` (`ValidateStatusTransition`), `internal/handler/service_request.go` (`CreateParentServiceRequest`, `UpdateServiceRequestStatus`, `UpdateServiceRequestResult`). **Jangan ubah skema/kontrak 0002**; hanya tambah hook nil-safe (§4.1).
- Job: `internal/jobs/runner.go` (`withAdvisoryLock`, ticker). Tambah satu job di `RunOnce`, pola sama dengan `runEscalation`.
- Notifikasi: `client.NotifyUsers` + `client.UsersByRole` (core `internal/client/client.go`), `type: 'service_desk'`.
- Saklar fitur: pola cache 60 dtk `services/hr/internal/client/client.go` (`IsHrisEnabled`) → `client.FeatureEnabled(ctx, schoolID, "service_desk")` memanggil identity `GET /schools/{id}/features`.
- Lampiran: core **belum** punya modul storage (T7b menyimpan `attachments` jsonb saja). Salin `services/billing/internal/storage/` ke `services/core/internal/storage/` (satu salinan, dipakai tiket **dan** menu 81 ke depan); env `STORAGE_*` mengikuti billing.
- Approval: tidak ada approval di menu 137–138 (PRD tidak mewajibkan). Jangan membuat tabel approval.

## 2. Migrasi `0003_service_desk.up.sql`
```
sd_teams(id, school_id, name, manager_user_id NULL, deleted_at)                         -- "supervisor" tim (AC-138-01)
sd_team_members(team_id, user_id, school_id, PRIMARY KEY(team_id, user_id))
sd_categories(id, school_id, name, description, team_id NOT NULL, is_facility bool default false,   -- FR-137-004
  form_schema jsonb default '[]',          -- [{key,label,type CHECK text|textarea|select|date|number,required,options[]}] (FR-137-001)
  reporter_kinds text[] NOT NULL,          -- subset {'orang_tua','siswa','pegawai'} (FR-137-002)
  service_request_category_id uuid NULL UNIQUE REFERENCES service_request_categories(id),  -- pemetaan BR-SD-01
  is_fallback bool default false, active bool, deleted_at,
  UNIQUE(school_id) WHERE is_fallback AND deleted_at IS NULL)
sd_business_hours(school_id, weekday int CHECK(weekday BETWEEN 1 AND 7), opens_at time, closes_at time,
  PRIMARY KEY(school_id, weekday), CHECK(closes_at > opens_at))                         -- tidak ada baris = hari tutup
sd_holidays(id, school_id, date, name, UNIQUE(school_id, date))
sd_sla_policies(id, school_id, category_id NULL, priority CHECK IN('low','normal','high','urgent'),
  response_value int, response_unit CHECK IN('business_hours','business_days'),
  resolution_value int, resolution_unit CHECK IN('business_hours','business_days'),
  warn_bp int NOT NULL CHECK(warn_bp BETWEEN 1 AND 9999),   -- ambang "mendekati", diisi manager (tanpa default)
  effective_from date, effective_to date NULL, deleted_at)  -- FR-138-001, BR-GEN-08
sd_escalation_rules(id, school_id, policy_id, trigger CHECK IN('warning','breach'), level int,
  target_kind CHECK IN('team_manager','role','user'), target_role NULL, target_user_id NULL,
  UNIQUE(policy_id, trigger, level))                                                     -- FR-138-002
sd_ticket_sequences(school_id, year int, last int, PRIMARY KEY(school_id, year))
sd_tickets(id, school_id, number text, category_id, team_id, priority, subject, description, form_answers jsonb,
  channel CHECK IN('parent_portal','student_portal','employee_portal','agent'),
  reporter_user_id, student_user_id NULL,
  service_request_id uuid NULL UNIQUE REFERENCES service_requests(id),                  -- BR-SD-01
  status CHECK IN('new','assigned','in_progress','resolved','closed','rejected'),
  assignee_user_id NULL, sla_snapshot jsonb NULL,          -- salinan policy saat tiket dibuat (perubahan policy tidak retroaktif)
  response_due_at, resolution_due_at, first_responded_at NULL, resolved_at NULL, closed_at NULL,
  response_breached_at NULL, resolution_breached_at NULL, escalation_level int default 0,
  work_order_id uuid NULL, work_order_number text NULL,    -- AC-137-01
  satisfaction_score smallint NULL CHECK(satisfaction_score BETWEEN 1 AND 5), satisfaction_comment NULL, rated_at NULL,
  idempotency_key, UNIQUE(school_id, idempotency_key), UNIQUE(school_id, number), created_at, updated_at)
sd_ticket_events(id, school_id, ticket_id, kind CHECK IN('status','comment','assign','escalation','sla_breach','work_order','satisfaction'),
  from_status NULL, to_status NULL, body NULL, internal bool default false,             -- komentar internal tidak tampil ke pelapor
  attachments jsonb default '[]', actor_user_id NULL, dedupe_key text NULL, created_at,
  UNIQUE(ticket_id, dedupe_key))                                                         -- job eskalasi idempoten
```
Indeks: `(school_id, status, resolution_due_at)`, `(school_id, assignee_user_id)`, `(school_id, reporter_user_id)`. Down: drop tabel `sd_*` saja (tabel 0002 tidak disentuh). Skala kepuasan 1–5 adalah asumsi UI (PRD tidak menyebut skala) → catat di handoff.

## 3. Fungsi murni (`internal/servicedesk/model.go`) + vektor
| Fungsi | Vektor (→ AC/FR) |
|---|---|
| `ValidTransition(from, to, note)` | new→assigned/rejected, assigned→in_progress/rejected, in_progress→resolved/rejected, resolved→closed/in_progress (dibuka ulang pelapor), `rejected` tanpa note → error. Lainnya → `ErrInvalidTransition` (422) (FR-137-003) |
| `DueAt(start, value, unit, cal Calendar) time.Time` | **AC-138-01**: jam layanan Sen–Jum 07:00–15:00, target 2 `business_days`, dibuat Senin 10:00 → Rabu 10:00. Dibuat Jumat 14:00 → Selasa 14:00. Dibuat Sabtu → mulai Senin 07:00 → Rabu 07:00. Libur Selasa → digeser 1 hari kerja. `business_hours` 4, dibuat 13:00 → besok 09:00. Zona `Asia/Jakarta` |
| `SLAState(dueAt, start, now, warnBp)` | `ok` / `warning` (waktu kerja terpakai ≥ warnBp) / `breached` (now > dueAt). **AC-138-01**: 2 hari kerja + 1 menit → `breached` |
| `ResolvePolicy(policies, categoryID, priority, onDate)` | policy kategori mengalahkan policy umum (`category_id NULL`); di luar `effective_from/to` diabaikan; tidak ada → `nil` (tiket tanpa SLA, tetap dibuat) (FR-138-001) |
| `NextEscalations(ticket, state, rules)` | `warning` → aturan level berikut yang belum dijalankan; `breached` → aturan `breach`. Tidak ada aturan → kosong. Sudah dijalankan (`escalation_level`) → tidak diulang (FR-138-002) |
| `ValidateForm(schema, answers)` | field wajib kosong, tipe `number`/`date` salah, opsi `select` di luar daftar → daftar galat per field (FR-137-001) |
| `CanReport(category, reporterKind)` | kategori tanpa `pegawai` menolak tiket pegawai (FR-137-002) |
| `ToServiceRequestStatus(ticketStatus)` | new→submitted, assigned→accepted, in_progress→in_progress, resolved/closed→done, rejected→rejected (BR-SD-01) |
| `CanRate(ticket, userID)` | hanya pelapor, status resolved/closed, belum pernah menilai (FR-137-005) |
| `ComplianceBp(met, total)` | 8 dari 10 → 8000 bp; total 0 → `nil` (tampil "—") (FR-138-003) |
| `TicketNumber(year, seq)` | 2026, 7 → `SD-2026-000007` |
| `WorkOrderPriority(p)` | pemetaan prioritas tiket → enum prioritas perintah kerja T9 (satu fungsi; sesuaikan nama enum saat rebase) |

## 4. Integrasi & job
### 4.1 BR-SD-01: menu 81 sebagai tiket (tanpa merusak T7b)
- Hook nil-safe `ServiceDesk *servicedesk.Service` di `Handler` (core). Di akhir `CreateParentServiceRequest`, setelah SR tersimpan: `if h.ServiceDesk != nil { h.ServiceDesk.FromServiceRequest(ctx, sr) }`.
- `FromServiceRequest`: (1) `FeatureEnabled(school, "service_desk")` mati → tidak melakukan apa pun (alur 81 tetap). (2) Kategori tiket = `sd_categories` yang memetakan `service_request_category_id`, atau kategori `is_fallback`. Keduanya tidak ada → tidak membuat tiket, log info (alur 81 tetap). (3) Buat tiket `channel='parent_portal'`, `service_request_id=sr.id`, idempotency `sr:<id>` (unik `service_request_id`). Gagal → SR tetap di alur 81, kirim `POST /internal/exceptions` core (kind `dead_letter`, `retry_url` = `POST /internal/service-desk/from-service-request/{id}`).
- Sinkron satu arah, **dalam transaksi yang sama** dengan perubahan status tiket: update `service_requests.status = ToServiceRequestStatus(...)` + insert `service_request_events` (note "Sinkron dari tiket SD-…"). Dokumen hasil yang dilampirkan agent saat `resolved` juga ditulis ke `service_requests.result_attachments`, jadi halaman orang tua T7b tetap benar.
- Guard di `UpdateServiceRequestStatus` dan `UpdateServiceRequestResult`: SR yang punya tiket terbuka → **409** `"Permintaan ini dikelola di Service Desk (SD-…)"`. Guard memakai keberadaan tiket, **bukan** status feature (feature dimatikan belakangan tidak meninggalkan tiket yatim).
- Notifikasi: SR bertiket tidak mengirim notifikasi 81 terpisah. Satu perubahan status = satu notifikasi (dari tiket).

### 4.2 Penerimaan tiket & penugasan (FR-137-002/003)
Tiket baru → `team_id` dari kategori, `status='new'`, SLA dihitung (`ResolvePolicy` + `DueAt`), snapshot disimpan. Notifikasi ke semua anggota tim. Penugasan oleh agent/manager (`assigned`). `first_responded_at` = komentar publik/transisi pertama oleh agent. Setiap transisi/komentar publik → notifikasi pelapor (key `sd:<ticket>:<event_id>:<user>`). Audit `audit_events` (module `service_desk`) untuk setiap mutasi.

### 4.3 Kontrak ke T9: tiket → perintah kerja (FR-137-004, AC-137-01)
Env baru di core config: `OPS_URL` (port 8093). Hanya kategori `is_facility=true`, status bukan closed/rejected, belum punya `work_order_id`.
```
POST {OPS_URL}/internal/work-orders            X-Internal-Key, Idempotency-Key: sd_ticket:<ticket_id>:work_order
{ school_id, source_service:"core", source_type:"sd_ticket", source_id:<ticket_id>, source_ref:<SD-number>,
  title, description, room_id?:uuid, asset_id?:uuid, priority, reporter_user_id, attachments:[{name,key}] }
→ 201/200 { id, number, status }        (200 = idempotent replay, data sama)
GET  {OPS_URL}/internal/work-orders/{id}?school_id=   → { id, number, status, assignee_name, completed_at }
```
Hasil disimpan ke `work_order_id/number` + event `work_order` (tautan di detail tiket). Status perintah kerja ditampilkan dengan pull `GET` saat detail dibuka (tanpa callback). Nama field/enum final mengikuti T9 §4; T9 menulis kontrak ini di file task-nya. Gagal panggil → 502 ke agent + exception `kind='callback'`. Test memakai fake `httptest`. `room_id` dipilih di form lewat `GET {OPS_URL}/rooms` (JWT, T9 menu 116), opsional.

### 4.4 Job SLA & eskalasi (FR-138-002, AC-138-01)
`core_service_desk_sla_job` di `Runner.RunOnce` (advisory lock, interval `JOB_INTERVAL_SECONDS` yang sudah ada). Untuk tiket belum resolved: hitung `SLAState` respons & penyelesaian → set `*_breached_at` sekali (`WHERE … IS NULL`), event `sla_breach`, jalankan `NextEscalations`: notifikasi target (`team_manager` = `sd_teams.manager_user_id`; `role` lewat `UsersByRole`), naikkan `escalation_level`, event `escalation` dengan `dedupe_key = <trigger>:<level>` (dijalankan 2× → satu efek). Tidak mengubah assignee otomatis.

## 5. Endpoint (core, JWT, semua cek `school_id` + feature `service_desk` → 403 bila mati)
| Method & path | Peran |
|---|---|
| `CRUD /service-desk/teams` (+members) · `CRUD /service-desk/categories` (form builder, pemetaan kategori 81, fallback) | servicedesk_manager, admin |
| `GET/PUT /service-desk/business-hours` · `CRUD /service-desk/holidays` · `CRUD /service-desk/sla-policies` (+escalation rules) | servicedesk_manager, admin |
| `GET /service-desk/tickets?status=&category_id=&team_id=&assignee=me&sla=warning\|breached&page=&per_page=` | agent (hanya kategori timnya), manager (semua) |
| `GET /service-desk/tickets/{id}` · `POST …/{id}/assign {user_id}` · `POST …/{id}/status {to, note}` · `POST …/{id}/comments` (multipart, `internal`) | agent tim kategori, manager |
| `POST /service-desk/tickets/{id}/work-order` | agent/manager, kategori `is_facility` |
| `POST /service-desk/tickets` (atas nama pelapor, `channel='agent'`) | agent, manager |
| `GET /service-desk/reports?from=&to=&group_by=category\|team` → jumlah, terpenuhi/melanggar, `compliance_bp`, rata-rata kepuasan (×100, int), rata-rata menit penyelesaian | servicedesk_manager, kepala_sekolah |
| `GET /me/tickets/categories` · `POST /me/tickets` (multipart) · `GET /me/tickets` · `GET /me/tickets/{id}` (tanpa komentar internal) · `POST /me/tickets/{id}/comments` · `POST /me/tickets/{id}/reopen` · `POST /me/tickets/{id}/satisfaction {score, comment}` | pelapor: orang_tua (siswa terverifikasi via identity `/internal/guardians/{id}/students/{sid}/access`), siswa, staf |
| `GET /me/tickets/{id}/files/{key}` | pelapor pemilik / agent (URL bertanda tangan pendek) |
| `POST /internal/service-desk/from-service-request/{id}` | internal (retry exception) |

## 6. Frontend
`meta.feature: 'service_desk'`, navGroup **Service Desk** (agent/manager) dan **Layanan** (pelapor). Route di `web/src/router/domains/servicedesk.routes.ts` (daftarkan di `router/index.ts`, tambah baris). Service `web/src/services/servicedesk/` memanggil `api('core')`.
| Path | View (`views/servicedesk/…`) | Peran | Label |
|---|---|---|---|
| `service-desk/tiket` · `/:id` | `TicketInboxView.vue` (filter status/kategori/SLA, badge teks "Mendekati SLA"/"Melanggar SLA"), `TicketWorkView.vue` (timeline, komentar publik/internal, assign, ubah status + catatan, unggah, tombol "Jadikan Perintah Kerja" + tautan nomor PK) | servicedesk_agent, servicedesk_manager | Tiket |
| `service-desk/kategori` | `CategoriesView.vue` (tim, form builder field, jenis pelapor, flag fasilitas, pemetaan kategori layanan ortu, fallback) | servicedesk_manager, admin | Kategori Tiket |
| `service-desk/tim` | `TeamsView.vue` | servicedesk_manager, admin | Tim |
| `service-desk/sla` | `SlaView.vue` (tab Kebijakan SLA / Jam Layanan / Hari Libur / Eskalasi) | servicedesk_manager, admin | SLA & Eskalasi |
| `service-desk/laporan` | `ReportView.vue` (tabel per kategori/tim: jumlah, % kepatuhan + angka, kepuasan; "Diperbarui: …"; ekspor CSV) | servicedesk_manager, kepala_sekolah | Laporan Service Desk |
| `layanan/tiket` · `/baru` · `/:id` | `MyTicketsView.vue`, `MyTicketFormView.vue` (field dinamis dari `form_schema`), `MyTicketDetailView.vue` (timeline publik, komentar, buka ulang, dialog penilaian 1–5 setelah selesai) | orang_tua, siswa, staf | Tiket Layanan |
Detail menu 81 orang tua (`ServiceRequestDetailView.vue`, T7b): bila SR punya tiket, tampilkan banner "Diproses sebagai tiket SD-…" + tautan ke `layanan/tiket/:id` (penambahan saja). Inbox 81 (`ServiceRequestWorkView.vue`): tombol ubah status disembunyikan + pesan 409 ditampilkan bila bertiket. Tiga state + aksesibilitas sesuai GEMINI §5.

## 7. Bukti handoff
- Vektor §3 (test tabel), khusus `DueAt` dengan akhir pekan, libur, di luar jam layanan.
- **AC-137-01**: test handler dengan fake ops → `work_order_id/number` tersimpan, panggilan kedua idempoten (fake menerima Idempotency-Key sama), kategori non-fasilitas → 422.
- **AC-138-01**: test job dengan jam tetap → tiket ditandai `resolution_breached_at`, eskalasi ke `manager_user_id` tim, job dijalankan 2× → satu event + satu notifikasi.
- **BR-SD-01**: feature aktif → `POST /parent/service-requests` membuat SR + tiket tertaut; ubah status tiket → status SR ikut berubah; `POST /service-requests/{id}/status` → 409; feature mati → alur 81 lama utuh (test T7b yang ada tetap hijau). Tanpa pemetaan & tanpa fallback → tanpa tiket.
- FR-137-001..005, FR-138-001..003: tabel cakupan di handoff. Test otorisasi: agent tim lain → 403/404, orang tua lain → 404, komentar internal tidak muncul di `/me/tickets/{id}`, feature mati → 403.
- Playwright (setelah merge gelombang): (a) manager buat tim + kategori "Tagihan" (SLA 2 hari kerja) + kategori "Fasilitas" → orang tua ajukan lewat menu 81 → tiket muncul di inbox agent → agent selesaikan → orang tua melihat status "Selesai" di menu 81 dan memberi nilai; (b) pegawai lapor "AC kelas 8A mati" → agent "Jadikan Perintah Kerja" → nomor PK tampil dan perintah kerja ada di menu 118 (T9); (c) laporan menampilkan kepatuhan & kepuasan.
