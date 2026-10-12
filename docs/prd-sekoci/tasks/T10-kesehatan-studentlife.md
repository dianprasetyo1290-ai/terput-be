# T10: Kesehatan Sekolah + kerangka service `studentlife`: profil kesehatan, kunjungan UKS, imunisasi & skrining

**Menu PRD:** 127, 128, 129 (Bagian 3 §5 Grup 22, BR-HLT-01, NFR-OPS-03, OQ-31) · **Branch:** `feat/T10-kesehatan` · **Service:** `studentlife` **baru**, port **8094**, DB `<prefix>_studentlife` (paket `internal/sensitive`, `internal/consent`, `internal/health`)
**Migrasi:** `studentlife/0001`–`0009` · `attendance/0009_health_absence` (**cadangan baru**, belum ada di tabel PARALLEL-v2: orkestrator mencatatnya sebelum gelombang 3 dimulai) · tidak menyentuh migrasi identity.
**Prasyarat:** gelombang 3, paralel dengan T9. Peran dan feature key **dibuat T9** (`identity/0017`). Nama yang dipakai di sini adalah snake_case dari tabel "Peran Tambahan" PRD: `petugas_uks` (peran), `health` (feature key). **Nama final mengikuti T9 §2**: kalau berbeda, ganti satu konstanta Go (`internal/common/roles.go`) dan satu konstanta TS, bukan string yang tersebar.
**Dipakai ulang oleh:** T20 (paket `sensitive` + `consent`, migrasi 0010–0019), T21 (`consent` boleh dipakai untuk BR-EVT-01), T22 (kerangka service §1.2, migrasi 0030–0039).
**Feature key:** `health`. Semua route, endpoint, dan job hanya aktif jika fitur menyala untuk sekolah (cek `GET {IDENTITY_URL}/schools/{id}/features`, cache 60 detik per `(school_id, feature_key)`; tiru `services/hr/internal/client/client.go` `IsHrisEnabled`, tetapi generik per key karena T20/T22 memakai key lain).

## 1. Acuan kode & kerangka service
### 1.1 Acuan
| Kebutuhan | Tiru dari | Catatan |
|---|---|---|
| Layout service | `services/billing/` (`cmd/server/main.go`, `internal/{common,config,handler,store,model,client,storage}`) | salin `common/{auth,response,http,internal,sentry}.go`, ganti nama package |
| Job berkala | `services/hr/internal/jobs/runner.go` (ticker + `pg_try_advisory_lock`) | salin |
| Enkripsi kolom | `services/hr/internal/crypto/{crypto.go,crypto_test.go}` | salin **utuh beserta test** ke `internal/sensitive/crypto.go`. Kunci dari env `STUDENTLIFE_ENC_KEY` (32 byte base64) + `STUDENTLIFE_BLIND_KEY`, **tanpa default** di config (gagal start jika kosong, pola `services/hr/internal/config/config.go`) |
| Klien internal + akses wali | `services/billing/internal/client/client.go` (`GuardianHasAccess`, header `X-Internal-Key`) | salin, jangan impor lintas modul |
| Status sakit siswa | `services/attendance/internal/store/leave.go` (`ReviewLeaveRequest` menerapkan BR-ATT-02 ke `student_attendances`; `ApprovedLeaveTypes` dipakai saat sesi guru menyemai roster) | endpoint internal baru §4.2 memakai jalur yang sama, tidak membuat jalur kedua |
| Wali kelas | academic `classrooms.homeroom_teacher_user_id`, `GET /students/{id}/classroom` → `{classroom_id}`, `GET /classrooms/{id}` (grup InternalOrJWT) | |
| Relasi wali | identity `guardian_links` + `GET /internal/guardians/{g}/students/{s}/access` | daftar wali per siswa belum ada → §4.3 |
| Notifikasi | notification `POST /internal/notifications` (Idempotency-Key wajib, `internal.go`) | |
| Approval | core `POST /internal/approvals` + callback (T1 §3), konstanta `KnownRequestTypes` di `services/core/internal/model/domain.go` | |
| Unggah file | `services/billing/internal/storage/local.go` | `STORAGE_ROOT` |
| Route + menu | `web/src/router/domains/hr.routes.ts` (`meta.feature`), `walikelas.routes.ts`, `parent.routes.ts` | |
| Riwayat approval di UI | `web/src/components/core/ApprovalTimeline.vue` | pakai, jangan salin |

### 1.2 Kerangka service `studentlife`
```
services/studentlife/
  cmd/server/main.go                 # config, DB, router, runner job
  internal/common/                   # salinan billing + roles.go (konstanta peran & feature key, lihat header)
  internal/config/config.go          # PORT, DATABASE_URL, SEKOCI_JWT_SECRET, SEKOCI_INTERNAL_KEY, IDENTITY_URL, ACADEMIC_URL,
                                     # ATTENDANCE_URL, NOTIFICATION_URL, CORE_URL, OPS_URL (opsional, T15), STORAGE_ROOT,
                                     # STUDENTLIFE_ENC_KEY, STUDENTLIFE_BLIND_KEY, STUDENTLIFE_PUBLIC_INTERNAL_URL, JOB_INTERVAL_SECONDS
  internal/client/client.go          # identity, academic, attendance, notification, core, ops
  internal/sensitive/                # crypto.go (+test), accesslog.go, policy.go — DIPAKAI ULANG T20
  internal/consent/                  # persetujuan orang tua generik — DIPAKAI ULANG T20 (rujukan), boleh T21
  internal/health/                   # model.go (fungsi murni), store.go, handler.go
  internal/jobs/runner.go
  internal/storage/local.go
  migrations/  Dockerfile  docker-compose.yml  README.md  .env.example
```
Daftarkan (hanya tambah baris): `.env.example` (`STUDENTLIFE_PORT=8094`, `STUDENTLIFE_DB_NAME=teratai_studentlife`, `STUDENTLIFE_ENC_KEY`, `STUDENTLIFE_BLIND_KEY` nilai dev), `docker-compose.yml` (`studentlife-migrate` + `studentlife`, pola blok `hr`), `docker-compose.local.yml` (service + `POSTGRES_MULTIPLE_DATABASES` + `${STUDENTLIFE_DB_NAME:-teratai_studentlife}`), `docker-compose.server.yml` (`studentlife: { <<: *lim, ports: !reset [] }`), `web/src/api/client.ts` (`studentlife`), `web/.env.example` (`VITE_STUDENTLIFE_URL=http://localhost:8094`), `web/.env` (`/api/studentlife`), `web/nginx.conf` (`location /api/studentlife/ { proxy_pass http://studentlife:8094/; ... }`, pola blok `/api/hr/`).

Router: `/healthz`; grup JWT (+ cek feature per prefix: `/health/*` → `health`; T20/T22 menambah prefix-nya sendiri; `/consents/*` dan `/sensitive/*` generik, cek fitur per subjek/modul); grup `InternalOnly` untuk `/internal/*`.

## 2. Migrasi
Semua tabel: `id uuid PK default gen_random_uuid()`, `school_id uuid NOT NULL`, `created_at`/`updated_at timestamptz`. Kolom `*_enc bytea` = AES-256-GCM dengan AAD `"<tabel>:<school_id>:<id baris>"` (ciphertext tidak bisa dipindah ke baris lain). Isi terenkripsi berupa JSON yang di-marshal sebelum `Encrypt`.

**`0001_studentlife_base`** (dipakai T20/T22)
```
sensitive_access_logs(id bigserial PK, school_id, module text CHECK(module IN('health','counseling')),
  entity_type text, entity_id uuid, student_id uuid NULL, actor_user_id uuid, actor_role text,
  action text CHECK(action IN('view','create','update','print','deny')), reason text NULL, ip_hash text NULL, created_at)
  INDEX(school_id, created_at DESC), (school_id, student_id, created_at DESC)
confidential_access_policies(id, school_id, module CHECK IN('health','counseling'), role text NOT NULL,
  granted_by uuid, granted_at, revoked_by uuid NULL, revoked_at NULL,
  UNIQUE(school_id, module, role) WHERE revoked_at IS NULL)        -- OQ-31: tanpa baris = kepala_sekolah TIDAK punya akses
parent_consents(id, school_id, subject_type text NOT NULL,         -- 'health_program' (T10), 'referral' (T20), T21 boleh menambah lewat migrasinya
  subject_id uuid NOT NULL, student_id uuid NOT NULL,
  status text NOT NULL CHECK(status IN('pending','granted','declined','revoked')),
  decided_by_guardian_id uuid NULL, decided_at NULL, note text NULL, requested_by uuid,
  UNIQUE(subject_type, subject_id, student_id))
```
**`0002_health`**
```
health_profiles(id, school_id, student_id uuid NOT NULL,
  profile_enc bytea NOT NULL,       -- {blood_type, allergies[{name, reaction, is_alert}], conditions[{name, note, is_alert}], routine_medications[{name, dose, schedule}]}
  alert_enc bytea NULL,             -- turunan: hanya item is_alert=true (tampilan wali kelas), ditulis ulang setiap profil berubah
  emergency_enc bytea NULL,         -- [{name, relationship, phone}]
  verified_by uuid NULL, verified_at NULL, version int NOT NULL DEFAULT 1,
  UNIQUE(school_id, student_id))
health_profile_changes(id, school_id, student_id, profile_id NULL, submitted_by uuid /* wali */,
  proposed_enc bytea NOT NULL,      -- payload utuh usulan (bukan diff)
  base_version int NOT NULL, status CHECK IN('pending','approved','rejected','superseded'),
  approval_id uuid NULL, idempotency_key, UNIQUE(school_id, idempotency_key))
uks_medicines(id, school_id, name, unit text NOT NULL, ops_item_id uuid NULL /* barang di menu 102 (T15) */, active bool, deleted_at)
clinic_visits(id, school_id, student_id, visited_at timestamptz NOT NULL, left_at NULL,
  details_enc bytea NOT NULL,       -- {complaint, examination, action, note}
  outcome text NOT NULL CHECK(outcome IN('back_to_class','sent_home','referred')),
  referred_to_enc bytea NULL, handled_by uuid NOT NULL,
  attendance_sync CHECK IN('not_needed','pending','done','skipped','failed') NOT NULL,
  parent_notified_at NULL, idempotency_key, UNIQUE(school_id, idempotency_key))
clinic_visit_addenda(id, school_id, visit_id FK, note_enc bytea, created_by)   -- kunjungan tidak diubah setelah disimpan; koreksi = addendum
clinic_visit_medications(id, school_id, visit_id FK, medicine_id FK, qty numeric(10,2) CHECK(qty>0))
inventory_issue_outbox(id, school_id, visit_medication_id UNIQUE, ops_item_id, qty, status CHECK IN('pending','sent','skipped','failed'),
  attempts int default 0, last_error text NULL)   -- FR-128-003, dikirim job ke T15
```
**`0003_health_programs`**
```
health_programs(id, school_id, kind CHECK IN('immunization','screening'), name, description,
  starts_on date, ends_on date, requires_consent bool NOT NULL, consent_deadline date NULL,
  status CHECK IN('draft','published','completed','cancelled'), created_by, CHECK(ends_on >= starts_on))
health_program_targets(id, school_id, program_id FK, target_kind CHECK IN('classroom','grade_level','student'), target_ref text)
health_program_participants(id, school_id, program_id FK, student_id, consent_id uuid NULL REFERENCES parent_consents,
  result_status CHECK IN('not_recorded','done','absent','excluded') default 'not_recorded',
  result_enc bytea NULL, needs_follow_up bool default false, follow_up_enc bytea NULL, follow_up_done_at NULL,
  recorded_by NULL, recorded_at NULL, UNIQUE(program_id, student_id))
```
**`attendance/0009_health_absence`**
```
ALTER TABLE leave_requests ADD COLUMN source text NOT NULL DEFAULT 'parent' CHECK(source IN('parent','uks')),
  ADD COLUMN source_ref uuid NULL, ADD COLUMN sick_since timestamptz NULL;
CREATE UNIQUE INDEX ON leave_requests(source, source_ref) WHERE source_ref IS NOT NULL;
```
`0004`–`0009` dicadangkan untuk perbaikan T10. Tidak ada seed data kesehatan; obat UKS diisi dari UI.

## 3. Fungsi murni (`internal/health/model.go`, `internal/sensitive`, `internal/consent`) + vektor uji
| Fungsi | Isi | Vektor (→ AC/FR) |
|---|---|---|
| `sensitive.Encrypt/Decrypt/BlindIndex` | salinan hr | round-trip; kunci salah gagal; **AAD baris lain gagal decrypt**; plaintext kosong → nil |
| `sensitive.CanAccess(module, viewer, subject, policies) Level` | `Full` / `AlertOnly` / `None`. Modul `health`: `petugas_uks` → Full; wali terverifikasi siswa → Full; peran yang punya baris aktif di `confidential_access_policies` → Full; wali kelas siswa → AlertOnly; lainnya → None (termasuk `kepala_sekolah` tanpa kebijakan, `admin`, `guru`). Fungsi menerima fakta (`isGuardian`, `isHomeroom`) yang sudah diambil handler, tanpa I/O | **AC-127-01**, FR-127-003; kepala_sekolah tanpa kebijakan → None (OQ-31); dengan kebijakan → Full; superadmin tanpa kebijakan → None |
| `health.AlertView(profile)` | ambil hanya `allergies`/`conditions` dengan `is_alert=true` + kontak darurat | **AC-127-01**: profil {alergi kacang (alert), riwayat asma (bukan alert)} → hanya alergi kacang + kontak darurat |
| `health.ValidateProfile(p)` | golongan darah ∈ {A,B,AB,O,tidak_diketahui} + rhesus opsional; nama item wajib; telepon kontak darurat pola angka | FR-127-001 |
| `health.ApplyChange(current, change)` | tolak (`superseded`) jika `change.base_version != current.version`; jika lolos versi +1 dan `alert_enc` diturunkan ulang | FR-127-002: perubahan disetujui saat versi berubah → superseded |
| `health.VisitEffects(outcome) {NotifyParent, MarkSick}` | `back_to_class` → {false,false}; `sent_home` → {true,true}; `referred` → {true,true} (siswa meninggalkan sekolah; dicatat sebagai asumsi di handoff) | **AC-128-01**, FR-128-002 |
| `health.SickNote(since, tz)` | `"Dipulangkan dari UKS pukul 10.15 WIB"` / `"Dirujuk dari UKS pukul …"`, zona dari config sekolah | AC-128-01 ("sejak jam tersebut") |
| `consent.Decide(c, decision, guardianID)` | hanya dari `pending` (atau `granted` → `revoked` selama belum dipakai). `declined`/`revoked` final kecuali dibuat ulang petugas | dua wali: keputusan pertama yang berlaku, kedua → 409 |
| `health.CanRecordResult(program, participant, consent)` | jika `requires_consent` dan consent ≠ `granted` → error "Persetujuan orang tua belum tersedia" | **AC-129-01**, FR-129-002 |
| `health.ResolveTargets(targets, rosters)` | himpunan unik siswa dari kelas/tingkat/siswa | FR-129-001: siswa di dua target → satu peserta |
| `health.ProgramRecap(participants)` | jumlah per `result_status`, consent (`pending/granted/declined`), `needs_follow_up` — angka saja, tanpa identitas | FR-129-003 |

## 4. Integrasi & job
**4.1 Akses + log (NFR-OPS-03).** Setiap handler yang membaca atau mengubah isi `*_enc` memanggil `CanAccess` lalu menulis satu baris `sensitive_access_logs` **dalam request yang sama**; gagal menulis log → 500 dan data tidak dikirim (fail closed). Akses ditolak → baris `action='deny'` + respons **404** (tidak membocorkan keberadaan data). Mutasi juga dikirim ke core `POST /internal/audit` **tanpa plaintext** (`"changed":["allergies"]`). Plaintext kesehatan tidak boleh muncul di log aplikasi, audit, notifikasi, maupun `summary` approval.
**Ekspor umum:** studentlife tidak menyediakan ekspor per siswa untuk data kesehatan. Rekap program hanya angka. Tidak ada endpoint `/internal/*` yang mengembalikan isi terdekripsi (kontrak untuk T23/T24: hanya agregat tanpa identitas, kalau nanti dibutuhkan). Test grep membuktikan ekspor identity/report/academic tidak menyentuh studentlife.

**4.2 Kontrak attendance (milik T10, dikerjakan di `services/attendance`).** `POST /internal/student-absences/health` (X-Internal-Key, grup baru `InternalOnly` di `router.go`):
`{school_id, student_id, date, sick_since, source_ref /* visit id */, outcome, actor_user_id}` →
- insert `leave_requests(type='sakit', status='approved', source='uks', source_ref, sick_since, requested_by=reviewed_by=actor, reason=SickNote, classroom_id dari academic bila tersedia)`; lalu UPDATE `student_attendances` tanggal itu ke `sakit` + `catatan` (pola `ReviewLeaveRequest`). Hari yang belum disemai otomatis ikut lewat `ApprovedLeaveTypes` yang sudah ada.
- `source_ref` sama → `200 {applied:true}` tanpa perubahan (idempoten). Sudah ada izin/sakit `approved` lain yang menutup tanggal itu → `200 {applied:false, reason:"sudah ada izin/sakit disetujui"}` (tidak menimpa).
- Granularitas presensi siswa saat ini **harian** (`UNIQUE(classroom_id, session_date, student_id)`): "sejak jam tersebut" disimpan di `sick_since` dan ditampilkan di catatan, status hari itu menjadi `sakit`. Daftar izin wali (`ListChildLeaveRequests`) menampilkan baris `source='uks'` dengan label "Dicatat UKS".
studentlife memanggilnya setelah kunjungan tersimpan (`attendance_sync='pending'` → `done/skipped`); gagal → job ulang, setelah batas percobaan (`JOB_MAX_ATTEMPTS` env) → core `POST /internal/exceptions` (`kind='callback'`, payload tanpa data kesehatan).

**4.3 Kontrak identity (milik T9 §4.1, gelombang yang sama: T10 koding sesuai kontrak ini dan uji dengan fake `httptest`, jangan membuat salinan).** `GET /internal/students/{studentUserId}/guardians?school_id=` (InternalOnly, file baru `internal/handler/internal_student_guardians.go`, tambah satu baris route di `cmd/server/main.go`) → `[{guardian_user_id, relationship, is_primary}]` dari `guardian_links` berstatus `active`. Dipakai T10 (notifikasi), T20 (notifikasi disiplin, consent rujukan), T22. Jika T13 menulis endpoint setara di gelombang yang sama, Claude menyatukannya saat merge.

**4.4 Notifikasi.** Kunjungan `sent_home`/`referred` → ke setiap wali aktif: `POST /internal/notifications` `type:'health_visit'`, judul "Ananda <nama> dipulangkan dari UKS" / "dirujuk", isi **tanpa diagnosis** (hanya jam + hasil + tautan portal), Idempotency-Key `studentlife:visit:<visit_id>:<guardian_id>`. Program `published` dengan `requires_consent` → permintaan persetujuan ke wali (`studentlife:consent:<consent_id>`).

**4.5 Approval (FR-127-002).** Usulan perubahan profil dari wali → core `POST /internal/approvals` `request_type='health_profile_update'` (tambahkan ke `KnownRequestTypes` + label "Perubahan profil kesehatan siswa" di `services/core/internal/model/domain.go`, hanya tambah baris), `summary:{student_name}` saja, `maker_user_id`=wali, `callback_url={STUDENTLIFE_PUBLIC_INTERNAL_URL}/internal/approval-callbacks`, `idempotency_key='health_profile_update:<change_id>'`. Admin mengonfigurasi alur dengan `approver_kind='role'`, `approver_role='petugas_uks'`. Core 422 (alur belum ada) → tampilkan pesan ke wali, **jangan auto-approve**. Callback idempoten: `approved` → `ApplyChange` (atau `superseded`), `rejected` → status saja. Petugas UKS melihat isi usulan di halaman verifikasi studentlife (isi terenkripsi tidak pernah ke core) lalu memutuskan lewat endpoint core `POST /approvals/{id}/approve|reject`.

**4.6 Stok obat (FR-128-003, kontrak untuk T15 gelombang 4).** `POST {OPS_URL}/internal/inventory/issues` `{school_id, item_id, qty, ref_type:'clinic_visit', ref_id, idempotency_key:'studentlife:med:<visit_medication_id>'}` → 201/200 (ulang). Stok kurang → 422. T15 wajib menyediakan endpoint ini persis. Di T10: `OPS_URL` kosong atau 404 → outbox tetap `pending` (UI: badge "Belum tersinkron ke Inventory"); obat tanpa `ops_item_id` → `skipped`. Penyimpanan kunjungan tidak pernah gagal karena inventory.

**Job (runner, advisory lock):** (1) sinkron kunjungan `attendance_sync='pending'`; (2) kirim `inventory_issue_outbox` pending; (3) pengingat consent: wali yang belum memutuskan pada hari `consent_deadline` (sekali, key memuat tanggal); (4) program lewat `ends_on` → peserta `not_recorded` tetap tampil di daftar tindak lanjut petugas (tidak diubah otomatis).

## 5. Endpoint (JWT, `school_id` dari `EffectiveSchoolID`; prefix `/health` kecuali yang ditandai generik)
| Method & path | Peran | Catatan |
|---|---|---|
| `GET /health/students/{id}/profile` | petugas_uks, wali (anak sendiri), peran berkebijakan | Full; dicatat `view` |
| `PUT /health/students/{id}/profile` | petugas_uks | langsung berlaku + `verified_*`; FR-127-001 |
| `GET /health/alerts?classroom_id=` | walikelas (homeroom kelas itu, dicek ke academic), petugas_uks | AlertOnly per siswa; **AC-127-01** |
| `POST /health/students/{id}/profile-changes` · `GET /health/profile-changes?status=` · `GET /health/profile-changes/{id}` | wali (buat/lihat milik sendiri); petugas_uks (antrian + isi usulan vs data saat ini) | FR-127-002, §4.5 |
| `GET/POST /health/visits` · `GET /health/visits/{id}` · `POST /health/visits/{id}/addenda` | petugas_uks | FR-128-001 (keluhan, pemeriksaan, tindakan, obat, hasil); POST header `Idempotency-Key`; respons memuat status sinkron presensi & notifikasi |
| `GET /health/students/{id}/visits` | petugas_uks, wali (anak sendiri) | |
| `CRUD /health/medicines` | petugas_uks | FR-128-003 |
| `CRUD /health/programs` (+targets) · `POST /health/programs/{id}/publish` | petugas_uks | publish → bentuk peserta + consent |
| `GET /health/programs/{id}/participants` · `PUT …/participants/{pid}/result` · `POST …/participants/{pid}/follow-up-done` | petugas_uks | result → `CanRecordResult` (422 **AC-129-01**) |
| `GET /health/programs/{id}/recap` | petugas_uks, peran berkebijakan | agregat |
| `GET /consents/my?subject_type=` · `POST /consents/{id}/decide` `{decision, note}` | orang_tua (`GuardianHasAccess` ke siswa consent) | **generik** (paket `consent`), di luar prefix `/health`: fitur dicek per `subject_type` lewat registri di `main.go` (`health_program` → `health`; T20 mendaftarkan `referral` → `counseling`). Respons memuat ringkasan subjek dari hook `consent.Describe(subject_type)` |
| `GET/POST/DELETE /sensitive/access-policies?module=` | admin, superadmin | **generik** (paket `sensitive`), modul hanya bisa dipilih jika fitur modulnya menyala (`health` → `health`, `counseling` → feature T20); diaudit |
| `GET /sensitive/access-logs?module=&student_id=&actor=&from=&to=` | admin, auditor, superadmin | log akses NFR-OPS-03 (tanpa isi data), generik |
| `POST /internal/approval-callbacks` | X-Internal-Key | dispatch per `request_type` (T20 menambah tipe-nya) |

## 6. Frontend
`web/src/services/studentlife/{health.ts,consent.ts,sensitive.ts,types.ts}` (`consent.ts`/`sensitive.ts` generik, dipakai ulang T20) → `api('studentlife')`; `web/src/router/domains/studentlife.routes.ts` (didaftarkan di `router/index.ts`, tambah baris). Semua route `meta.feature: 'health'`. Landing `petugas_uks` → `kesehatan/kunjungan` di `DashboardRedirectView.vue` (tambah cabang jika T9 belum).

| Path | Name | View (`views/studentlife/health/…`) | Peran | navGroup / navLabel |
|---|---|---|---|---|
| `kesehatan/kunjungan` · `kesehatan/kunjungan/baru` · `/:id` | health-visits, health-visit-form, health-visit-detail | `VisitsView.vue`, `VisitFormView.vue`, `VisitDetailView.vue` | petugas_uks | Kesehatan Sekolah / Kunjungan UKS |
| `kesehatan/siswa` · `/:studentId` | health-students, health-student | `HealthStudentsView.vue` (cari siswa), `HealthProfileView.vue` (tab Profil / Kunjungan / Program) | petugas_uks | Kesehatan Sekolah / Profil Kesehatan |
| `kesehatan/verifikasi` · `/:id` | health-changes | `ProfileChangesView.vue`, `ProfileChangeDetailView.vue` (data saat ini vs usulan + `ApprovalTimeline` + tombol Setujui/Tolak ke core) | petugas_uks | Kesehatan Sekolah / Verifikasi Perubahan |
| `kesehatan/program` · `/:id` | health-programs, health-program | `ProgramsView.vue`, `ProgramDetailView.vue` (tab Info & Sasaran / Peserta & Persetujuan / Hasil / Rekap) | petugas_uks | Kesehatan Sekolah / Imunisasi & Skrining |
| `kesehatan/obat` | health-medicines | `MedicinesView.vue` | petugas_uks | Kesehatan Sekolah / Obat UKS |
| `pengaturan/akses-data-sensitif` | sensitive-access | `views/studentlife/SensitiveAccessView.vue` (tab Kebijakan Akses / Log Akses; dipakai T20 untuk modul konseling) | admin, superadmin (log: + auditor) | Pengaturan / Akses Data Sensitif |
| `walikelas/peringatan-medis` | walikelas-health-alerts | `views/walikelas/HealthAlertsView.vue` | walikelas | Wali Kelas / Peringatan Medis |
| `orang-tua/kesehatan` | parent-health | `views/parent/HealthView.vue` (pilih anak; tab Profil (form usulan perubahan + badge "Menunggu verifikasi UKS") / Kunjungan / Persetujuan Program) | orang_tua | Portal / Kesehatan Anak |

Detail: **VisitFormView** – pilih siswa (autocomplete), jam datang (default sekarang, server), keluhan/pemeriksaan/tindakan (textarea berlabel), obat (baris obat + jumlah), hasil (radio berteks: Kembali ke kelas / Pulang / Dirujuk + tujuan rujukan). Ringkasan efek sebelum simpan: "Orang tua akan diberi tahu dan presensi hari ini menjadi Sakit sejak 10.15". Tombol simpan disabled selama request; `Idempotency-Key` dibuat sekali per form. Kartu peringatan medis siswa muncul di atas form. **HealthAlertsView** – tabel siswa kelas wali dengan badge berteks "Alergi"/"Kondisi khusus" + kontak darurat; tanpa riwayat lain. **ProgramDetailView › Hasil** – baris tanpa consent `granted` menampilkan badge "Menunggu persetujuan" dan input hasil dinonaktifkan (server tetap memvalidasi). Tiga state (memuat/kosong/galat) di semua halaman; data sensitif hanya dari respons server, tidak di-cache di `localStorage`.

## 7. Bukti handoff
- Test tabel semua fungsi §3; test crypto (salinan hr + AAD lintas baris).
- Test handler: **AC-127-01** (walikelas → hanya alert + kontak darurat; walikelas kelas lain → 404 + log `deny`; kepala_sekolah tanpa kebijakan → 404, dengan kebijakan → 200), FR-128-001 (kunjungan tersimpan terenkripsi, addendum tanpa mengubah aslinya), **AC-128-01** (fake attendance + fake notification: satu panggilan masing-masing, ulang simpan dengan Idempotency-Key sama → tidak dobel), **AC-129-01** (422 tanpa consent, 200 setelah wali `granted`), FR-127-002 (callback approved menerapkan, callback ulang idempoten, versi berubah → superseded), FR-128-003 (outbox: OPS_URL kosong → pending, fake ops 201 → sent).
- Test attendance: endpoint §4.2 (insert + update baris hari itu, idempoten, tidak menimpa izin approved), `ApprovedLeaveTypes` mengembalikan sakit UKS untuk sesi yang disemai setelahnya.
- Bukti NFR-OPS-03: `SELECT` mentah di DB menunjukkan `*_enc` bukan teks; setiap GET sensitif menambah baris `sensitive_access_logs`; `grep -rn "log\.\|slog\." internal | grep -i "allerg\|complaint\|diagnos"` kosong.
- Migrasi studentlife dan attendance up/down/up (DB baru per GEMINI §7b).
- Playwright (setelah merge gelombang): admin menyalakan `health` + membuat alur `health_profile_update` → wali mengusulkan alergi baru → petugas UKS menyetujui → wali kelas melihat peringatan alergi saja → petugas mencatat kunjungan "Pulang" → wali menerima notifikasi dan presensi anak hari itu "Sakit" dengan catatan jam → petugas membuat program imunisasi butuh persetujuan → input hasil terkunci sampai wali setuju.
