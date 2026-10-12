# T23: Service `quality`: QA & akreditasi (standar, bukti, evaluasi diri) + pelaporan pemerintah (Dapodik, laporan regulasi)

**Menu PRD:** 142, 143, 144, 145, 146 (BR-QA-01, BR-GOV-01, IF-ADV-01, OQ-36, OQ-37) · **Branch:** `feat/T23-quality` · **Service:** `quality` **baru**, port **8095** (paket `internal/qa`, `internal/govreport`) · **Migrasi:** `services/quality/migrations/0001–0009` (seluruh rentang milik T23)
**Prasyarat:** gelombang 4 di-merge. T1 (approval, audit), T9 (peran + feature key). **T24 berjalan paralel**: nilai indikator otomatis dibaca dari metric dictionary milik T24 lewat kontrak §4.1, uji dengan `httptest` fake. Endpoint identity `GET /internal/schools/{id}/foundation-schools` juga milik T24 (§4.4), pakai fake.
**Feature key:** `quality` (menu 142–144) dan `gov_reporting` (menu 145–146). **Peran:** `qa_coordinator`, `operator_dapodik` (peran baru), serta `kepala_sekolah` (Kepala Unit), `kepala_yayasan` (Pengurus/Admin Yayasan), `admin` (School Admin), `superadmin`. Nama peran & feature key sudah sesuai **T9 §2** (sumber kebenaran; feature key = allowlist `FeatureKeys` di `services/identity/internal/store/school_features.go`, ditambahkan T9).
**Baca PRD:** Bagian 4 §2–§4, menu 142–146, §6 (alur Siklus mutu & Sinkronisasi Dapodik), §7 IF-ADV-01, Lampiran (indikator mutu = metric dictionary; data siswa/rombel/PTK milik menu 03/39/83). Bagian 1 §17 (metric dictionary), CC-04, CC-10, BR-GEN-01/06/07.

## 1. Acuan kode
- **Kerangka service:** salin layout `services/billing/` (`cmd/server/main.go`, `internal/{common,config,handler,store,model,client,storage}`), `Dockerfile`, `.env.example`. `internal/common` disalin, bukan diimpor.
- **Audit:** outbox `audit_events` + job forward ke core `POST /internal/audit`, tiru `services/hr/migrations/0001_hris.up.sql` (tabel `audit_events`, kolom `forwarded_at`) dan `services/hr/internal/jobs`. Runner job (ticker + `pg_try_advisory_lock`) tiru `services/core/internal/jobs/runner.go`.
- **Approval:** kontrak T1 §3. Bentuk callback yang nyata ada di `services/core/internal/jobs/effects.go` (`CallbackPayload`: `approval_id, source_id, request_type, decision, decided_by, comment, decided_at`, header `Idempotency-Key: approval:<id>`).
- **Unggah berkas bukti:** `services/billing/internal/storage/local.go` (checksum sha256, `STORAGE_ROOT`).
- **XLSX:** `github.com/xuri/excelize/v2 v2.11.0` (versi yang sudah dipakai identity/hr). Jangan menambah library lain.
- **Sumber data Dapodik (dibaca, bukan disalin):** identity `users` (`name, nisn, birth_date, birth_place, religion`), academic `classrooms` + `class_enrollments` + `GET /terms`, hr `employees` (`full_name, gender, birth_date, nik_enc/nik_bidx`). Endpoint internal baru di §4.3.
- **Pendaftaran service** (GEMINI.md §1, hanya tambah baris): `.env.example` (`QUALITY_PORT=8095`, `QUALITY_DB_NAME`), `docker-compose.yml` (+ `quality-migrate`), `docker-compose.local.yml`, `docker-compose.server.yml` (ikuti pola `hr`), `web/src/api/client.ts` (`quality`), `web/.env*` (`VITE_QUALITY_URL=http://localhost:8095`), `web/nginx.conf` (`location /api/quality/` → `http://quality:8095/`), `db-init/` bila DB dibuat di sana.
- **Config env:** `PORT, DATABASE_URL, SEKOCI_JWT_SECRET, SEKOCI_INTERNAL_KEY, QUALITY_INTERNAL_URL` (alamat service ini yang bisa dijangkau core, mis. `http://quality:8095`; dipakai menyusun `callback_url`. **Jangan** meniru `hr/internal/client/client.go:214` yang menunjuk ke `CORE_URL` atau `billing/.../gateway_corrections.go:72` yang memakai `localhost`), `CORE_URL, IDENTITY_URL, ACADEMIC_URL, HR_URL, INSIGHT_URL, NOTIFICATION_URL, STORAGE_ROOT, EVIDENCE_MAX_BYTES, IMPORT_MAX_ROWS, QUALITY_HASH_KEY` (HMAC untuk NIK di baris impor Dapodik; kosong di production → service gagal start), `JOB_INTERVAL_SECONDS, SENTRY_DSN`.

## 2. Migrasi (semua tabel: `id uuid PK default gen_random_uuid()`, `school_id uuid NOT NULL`, `created_at/updated_at timestamptz`)
**`0001_quality_base`**: `audit_events` (salin skema hr).

**`0002_qa_framework`** (FR-142-001/002/003, OQ-37: **tanpa** seed instrumen)
```
qa_instruments(id, school_id, code, name, issuer text, version_label, score_min int, score_max int CHECK(score_max>score_min),
  status CHECK IN('draft','active','retired'), effective_from date, deleted_at,
  UNIQUE(school_id, code, version_label))                         -- versi baru = baris baru, versi lama tetap utuh
qa_standards(id, school_id, instrument_id FK, sort, code, name, weight_bp int NOT NULL CHECK(weight_bp>=0))
qa_items(id, school_id, standard_id FK, sort, code, statement text)                          -- butir
qa_indicators(id, school_id, item_id FK, sort, code, name, weight_bp int NOT NULL CHECK(weight_bp>=0),
  unit CHECK IN('percent_bp','count','ratio_milli','idr','days','score'), direction CHECK IN('higher_better','lower_better'),
  source_kind CHECK IN('manual','metric'), metric_key text NULL, rubric jsonb NOT NULL,       -- [{min_value bigint, score int}] dari instrumen
  CHECK((source_kind='metric') = (metric_key IS NOT NULL)), deleted_at)
qa_targets(id, school_id, indicator_id FK, academic_year_id uuid, target_value bigint NOT NULL, owner_user_id uuid NOT NULL,
  UNIQUE(school_id, indicator_id, academic_year_id))
```
**`0003_qa_evidence`** (FR-143-001..004)
```
qa_evidence(id, school_id, kind CHECK IN('file','sekoci_report','link'), title, object_key NULL, mime NULL, size_bytes NULL, checksum NULL,
  report_ref jsonb NULL,          -- {route_name, params, label} → dibuka ulang lewat router SPA, hak akses dicek route guard + server sumber
  url NULL, version int NOT NULL DEFAULT 1, supersedes_id uuid NULL, uploaded_by uuid,
  status CHECK IN('pending','verified','rejected'), verified_by NULL, verified_at NULL, reject_reason NULL, deleted_at,
  CHECK((kind='file') = (object_key IS NOT NULL)), CHECK((kind='sekoci_report') = (report_ref IS NOT NULL)))
qa_evidence_links(evidence_id FK, indicator_id FK, academic_year_id uuid, locked_at timestamptz NULL,
  PRIMARY KEY(evidence_id, indicator_id, academic_year_id))
```
**`0004_qa_self_evaluation`** (FR-144-001..004, BR-QA-01)
```
qa_self_evaluations(id, school_id, instrument_id FK, academic_year_id, period_label text, period_from date, period_to date,
  status CHECK IN('draft','pending_finalization','finalized','pending_reopen'), approval_id uuid NULL, reopen_approval_id uuid NULL,
  finalized_at NULL, finalized_by NULL, version int NOT NULL DEFAULT 0,
  UNIQUE(school_id, instrument_id, academic_year_id, period_label))
qa_self_eval_items(id, school_id, self_evaluation_id FK, indicator_id FK, target_value bigint NULL /*salinan saat dinilai*/,
  actual_value bigint NULL, value_source CHECK IN('manual','metric'), metric_as_of timestamptz NULL, metric_fetched_at timestamptz NULL,
  score int NULL, below_target bool NOT NULL DEFAULT false, note text, assessed_by uuid, UNIQUE(self_evaluation_id, indicator_id))
qa_action_plans(id, school_id, self_eval_item_id FK, action text NOT NULL, owner_user_id uuid NOT NULL, due_date date NOT NULL,
  status CHECK IN('open','in_progress','done','cancelled'), done_at NULL, deleted_at)
qa_self_eval_versions(id, school_id, self_evaluation_id FK, version int, snapshot jsonb NOT NULL, finalized_at, finalized_by,
  UNIQUE(self_evaluation_id, version))                             -- hasil final lama tetap tersimpan setelah dibuka kembali
```
**`0005_gov_dapodik`** (FR-145-001..004, BR-GOV-01, OQ-36)
```
dapodik_field_mappings(id, school_id, entity CHECK IN('student','rombel','ptk'), dapodik_column text, sekoci_field text,
  is_match_key bool NOT NULL DEFAULT false, match_priority int NULL, normalize CHECK IN('none','trim_upper','digits','date'),
  UNIQUE(school_id, entity, dapodik_column))
dapodik_imports(id, school_id, entity, file_name, file_checksum, row_count int, imported_by, status CHECK IN('parsed','compared','closed'))
dapodik_import_rows(id, school_id, import_id FK, row_no int, data jsonb NOT NULL)   -- kolom NIK disimpan sebagai {hmac, last4}, TIDAK mentah
dapodik_diffs(id, school_id, import_id FK, entity, sekoci_ref text NULL, import_row_id uuid NULL,
  kind CHECK IN('field_mismatch','missing_in_dapodik','missing_in_sekoci'), field text NULL, sekoci_value text NULL, dapodik_value text NULL,
  status CHECK IN('open','decided'), decision CHECK IN('update_dapodik','update_sekoci','ignore') NULL, decision_reason text NULL,
  decided_by NULL, decided_at NULL)
dapodik_sync_logs(id, school_id, entity, action CHECK IN('import','compare','decide','export'), actor_user_id, import_id NULL, summary jsonb)
```
**`0006_gov_reports`** (FR-146-001..003)
```
regulatory_templates(id, school_id, code, name, recipient text, frequency CHECK IN('monthly','semester','annual','adhoc'),
  columns jsonb NOT NULL,         -- [{label, metric_key, group_by NULL}] → setiap angka bersumber dari metric dictionary T24
  export_format CHECK IN('xlsx','csv'), status CHECK IN('active','inactive'), deleted_at, UNIQUE(school_id, code))
regulatory_reports(id, school_id, template_id FK, period_from date, period_to date,
  status CHECK IN('draft','pending_approval','approved','rejected','submitted'), approval_id NULL,
  template_snapshot jsonb NULL, data_snapshot jsonb NULL, snapshot_at NULL,       -- dibekukan saat diajukan, tidak pernah di-UPDATE lagi
  submitted_at NULL, submitted_by NULL, submission_note NULL, export_object_key NULL, idempotency_key text NOT NULL,
  UNIQUE(school_id, idempotency_key), UNIQUE(template_id, period_from, period_to) WHERE status <> 'rejected')
```
Status "terlambat" RTL **bukan** kolom: `due_date < today(Asia/Jakarta) AND status IN('open','in_progress')`. `.down.sql` men-drop berurutan terbalik.

## 3. Fungsi murni (`internal/qa/model`, `internal/govreport/model`) + vektor
| Fungsi | Vektor (dipetakan ke AC/FR) |
|---|---|
| `ScoreFromRubric(value, rubric) int` | nilai di batas `min_value` tepat → skor tingkat itu; di bawah semua batas → `score_min` instrumen (FR-144-001) |
| `StandardScore(items []{score, weight_bp}) milli` dan `InstrumentScore(standards)` | bobot 0 diabaikan; semua bobot 0 → error "bobot instrumen belum diisi"; pembulatan **half-up** di satu fungsi (FR-144-001) |
| `BelowTarget(actual, target, direction) bool` | **AC-144-01** target 2 (`ratio_milli` 2000), capaian 1,4 (1400), `higher_better` → `true`; `lower_better` dibalik; actual nil → false |
| `FinalizeBlockers(items, plans) []Blocker` | **AC-144-01** item `below_target` tanpa RTL aktif → blocker "Rencana tindak lanjut wajib untuk indikator di bawah target"; item tanpa skor → blocker (FR-144-002/003) |
| `Completeness(totalIndicators, withVerifiedEvidence) bp` | **AC-143-01** 10 & 7 → 7000 bp (tampil 70%); total 0 → 0 tanpa panic (FR-143-003) |
| `IsLocked(evalStatus) bool` | `finalized`, `pending_finalization`, `pending_reopen` → terkunci; `draft` → tidak (BR-QA-01, FR-143-004) |
| `UnitCompatible(indicatorUnit, metricUnit) bool` | `percent_bp` vs `idr` → false → 422 saat menautkan (FR-142-003) |
| `Normalize(value, rule) string` | `digits`: "00 12-34" → "001234"; `date`: "31/07/2010" & "2010-07-31" → sama; `trim_upper` |
| `MatchRecords(sekoci, dapodik, mapping) (pairs, onlySekoci, onlyDapodik)` | kunci dicoba berurutan `match_priority`; siswa: NISN lalu (nama ternormalisasi + tanggal lahir) |
| `DiffFields(pair, mapping) []Diff` | **AC-145-01** NISN beda, nama+tgl lahir sama → satu `field_mismatch` field `nisn`, dua sisi tercatat; **tidak ada** panggilan tulis ke identity (BR-GOV-01) |
| `HashSensitive(key, v)` / `Last4(v)` | NIK yang sama → HMAC sama; nilai mentah tidak pernah ada di output |
| `BuildRegulatorySnapshot(template, values) Snapshot` | **AC-146-01** snapshot = salinan nilai + `as_of` per kolom; metrik yang gagal → baris berlabel "sumber tidak tersedia", bukan 0 (FR-146-001/003) |

## 4. Integrasi & job
### 4.1 Kontrak metric dictionary (milik **T24**, dikonsumsi T23)
Kepemilikan: definisi, rumus, dan read model metrik **hanya** ada di service `insight` (T24 §2 `metric_definitions`, `metric_snapshots`). T23 **tidak** menghitung metrik sendiri dan tidak membaca DB service lain; T23 menyimpan `metric_key` di indikator lalu bertanya ke insight. Semua dengan `X-Internal-Key`:
| Method & path | Hasil |
|---|---|
| `GET {INSIGHT_URL}/internal/metrics?status=active` | `[{key, name, unit, allowed_group_by[], owner_service, formula, limitations}]`. Dipakai dropdown "Sumber data otomatis" di editor indikator |
| `GET {INSIGHT_URL}/internal/metrics/{key}/value?school_id=&from=YYYY-MM-DD&to=YYYY-MM-DD[&group_by=]` | `200 {key, unit, value, numerator, denominator, as_of, computed_at, rows?:[{dim, value}]}`. `404` key tidak dikenal. `503 {message}` sumber tidak tersedia/belum disegarkan. Nilai integer sesuai `unit` (`percent_bp`, `idr`, `count`, `ratio_milli`, `days`) |
- `as_of` = waktu data sumber terakhir, ditampilkan sebagai "Diperbarui: …" (**AC-142-01**, CC-10). `503` → tampil "Sumber metrik tidak tersedia", nilai **tidak** diisi 0.
- Kunci metrik awal yang dijamin T24: `attendance_rate`, `collection_rate`, `outstanding_idr`, `ppdb_conversion`, `ppdb_applicants`, `active_students`, `active_teachers`, `active_staff`, `billed_idr`, `paid_idr` (daftar resmi di T24 §2). Indikator yang belum punya metrik (mis. rasio buku per siswa) memakai `source_kind='manual'`.
- Periode: evaluasi diri memakai `period_from/to`. UI mengisinya dari academic `GET /terms?academic_year_id=` (semester), boleh diubah manual.

### 4.2 Approval lewat core (BR-QA-01, FR-144-003, FR-146-002)
Tambahkan ke `services/core/internal/model/domain.go` (hanya tambah baris konstanta + label + `KnownRequestTypes`): `qa_self_eval_finalize` "Finalisasi Evaluasi Diri", `qa_self_eval_reopen` "Pembukaan Kembali Evaluasi Diri", `regulatory_report` "Persetujuan Laporan Regulasi". Tidak ada flow default (keputusan sekolah); tanpa flow → tampilkan pesan 422 core apa adanya.
- **Finalisasi:** `POST /qa/self-evaluations/{id}/finalize` → `FinalizeBlockers` kosong, lalu `POST {CORE_URL}/internal/approvals` (`source_service='quality'`, `source_id=<id>`, `summary` = skor per standar + jumlah indikator di bawah target, `idempotency_key=qa-finalize:<id>:<version>`, `callback_url={QUALITY_INTERNAL_URL}/internal/approval-callbacks`). Status → `pending_finalization` (sudah terkunci).
- **Callback** `POST /internal/approval-callbacks` (idempoten via `Idempotency-Key`): finalize `approved` → `finalized`, `version+1`, tulis `qa_self_eval_versions` (snapshot item + skor), set `qa_evidence_links.locked_at` untuk indikator instrumen itu pada `academic_year_id` itu; `rejected` → `draft`. Reopen `approved` → `draft` + `locked_at = NULL` (versi lama tetap); `rejected` → `finalized`. Report `approved`/`rejected` → status laporan.
- **Penguncian (BR-QA-01, FR-143-004):** saat `IsLocked`, semua mutasi item, RTL yang terkait penilaian, dan link/ganti/hapus bukti yang `locked_at IS NOT NULL` → **409** "Evaluasi diri sudah difinalisasi; ajukan pembukaan kembali". Dicek di store dalam transaksi yang sama (`SELECT … FOR UPDATE` pada evaluasi). RTL tetap boleh diperbarui statusnya (`open→done`) setelah final karena itu tindak lanjut, bukan penilaian.

### 4.3 Endpoint internal baru di service sumber (milik T23, hanya tambah handler, tanpa migrasi)
| Service | Endpoint (`X-Internal-Key`, `school_id` wajib) | Isi |
|---|---|---|
| identity | `GET /internal/dapodik/students?school_id=` | siswa aktif: `user_id, name, nisn, birth_date, birth_place, religion` |
| academic | `GET /internal/dapodik/rombel?school_id=&academic_year_id=` | `classroom_id, name, grade_level, homeroom_teacher_user_id, student_count`, + peta `student_user_id → classroom` |
| hr | `GET /internal/dapodik/ptk?school_id=` | `employee_id, full_name, gender, birth_date, position, employment_status, nik_last4` (tanpa NIK mentah) |
| hr | `POST /internal/dapodik/ptk/match` `{school_id, niks[]}` | `{nik_index → employee_id}` dihitung lewat blind index hr; body tidak boleh di-log |
Daftar `sekoci_field` yang boleh dipetakan = field hasil endpoint di atas, satu konstanta Go per entitas (FR-145-001).

### 4.4 Lain-lain
- **Perbandingan antar unit (FR-144-004):** daftar sekolah satu yayasan dari identity `GET /internal/schools/{id}/foundation-schools` (milik T24 §4). Hanya `kepala_yayasan`/`superadmin`.
- **Dapodik (OQ-36, IF-ADV-01):** impor file CSV/XLSX hasil unduhan operator dari Dapodik → parse (batas `IMPORT_MAX_ROWS`) → `MatchRecords` + `DiffFields` → `dapodik_diffs`. **Tidak ada** web service ke Dapodik dan **tidak ada** penulisan otomatis ke identity/academic/hr maupun ke Dapodik (BR-GOV-01). Keputusan per diff wajib `decision_reason`. `update_sekoci` hanya memberi tautan ke menu pemilik data (Data Siswa / Kelas / Pegawai), dan perubahan dilakukan manual di sana. Perbandingan ulang memverifikasinya. Ekspor (FR-145-004) = XLSX/CSV berisi diff `update_dapodik`, kolom mengikuti `dapodik_column` pemetaan. Setiap impor/banding/keputusan/ekspor → `dapodik_sync_logs` + audit.
- **Notifikasi** (`POST {NOTIFICATION_URL}/internal/notifications`): hasil verifikasi bukti ke pengunggah, penugasan RTL ke owner. **Job harian:** pengingat RTL yang mendekati/lewat tenggat (`Idempotency-Key: qa-ap:<id>:<tanggal>`). Job forward audit.
- **Audit:** setiap mutasi (instrumen, target, bukti, verifikasi, penilaian, finalisasi, keputusan diff, ekspor, pengajuan/penyerahan laporan) menulis `audit_events`. Ekspor mengikuti filter aktif dan diaudit (CC-04, BR-GEN-06).

## 5. Endpoint (JWT, scope `school_id` dari claims, superadmin `?school_id=`)
| Method & path | Peran |
|---|---|
| `CRUD /qa/instruments` (+ `/{id}/standards`, `/items`, `/indicators`), `POST /qa/instruments/{id}/new-version` (salin ke versi baru), `POST /qa/instruments/import?dry_run=` (XLSX: standar/butir/indikator/bobot/rubrik), `GET /qa/instruments/import/template` | qa_coordinator (tulis), kepala_sekolah (baca) |
| `GET /qa/metrics` (proxy daftar §4.1) · `GET /qa/indicators/{id}/value?from=&to=` | qa_coordinator, kepala_sekolah, owner indikator |
| `CRUD /qa/targets?academic_year_id=` | qa_coordinator, kepala_sekolah |
| `GET /qa/evidence?indicator_id=&standard_id=&status=` · `POST /qa/evidence` (multipart / `report_ref` / `url`, `indicator_ids[]`) · `POST /qa/evidence/{id}/new-version` · `DELETE /qa/evidence/{id}` (soft) | qa_coordinator; owner target indikator terkait (unggah/versi bukti miliknya) |
| `POST /qa/evidence/{id}/verify` `{decision:'verified'\|'rejected', reason}` · `GET /qa/completeness?instrument_id=&academic_year_id=` | qa_coordinator (verifikasi), kepala_sekolah (baca) |
| `CRUD /qa/self-evaluations` · `PUT /qa/self-evaluations/{id}/items/{indicatorId}` · `POST …/refresh-metrics` · `POST …/finalize` · `POST …/reopen` `{reason}` | qa_coordinator, kepala_sekolah |
| `CRUD /qa/action-plans` · `POST /qa/action-plans/{id}/status` | qa_coordinator (semua), owner (milik sendiri) |
| `GET /qa/reports/self-evaluation/{id}` (`?format=xlsx`) · `GET /qa/reports/accreditation?instrument_id=&academic_year_id=` · `GET /qa/reports/comparison?instrument_id=&academic_year_id=` | qa_coordinator, kepala_sekolah (unitnya); comparison: kepala_yayasan, superadmin |
| `CRUD /gov/dapodik/mappings?entity=` · `POST /gov/dapodik/imports` (multipart, `entity`) · `GET /gov/dapodik/imports/{id}/diffs?kind=&status=` · `POST /gov/dapodik/diffs/{id}/decide` `{decision, reason}` · `POST /gov/dapodik/imports/{id}/export?format=` · `GET /gov/dapodik/logs` | operator_dapodik, admin |
| `CRUD /gov/report-templates` | admin |
| `POST /gov/reports` `{template_id, period_from, period_to}` (Idempotency-Key) · `GET /gov/reports/{id}/preview` (nilai live) · `POST /gov/reports/{id}/submit-for-approval` (bekukan snapshot) · `POST /gov/reports/{id}/mark-submitted` `{note}` · `GET /gov/reports/{id}/export?format=` · `GET /gov/reports?status=` | admin (tulis), kepala_sekolah, kepala_yayasan (baca unit/yayasannya) |
| `POST /internal/approval-callbacks` | X-Internal-Key |
Orang yang bukan owner/peran di atas → 403 lewat API langsung (AC-CC-03).

## 6. Frontend
`web/src/services/quality/{qa.ts,gov.ts,types.ts}`, `web/src/router/domains/quality.routes.ts` (spread di `router/index.ts`). Semua halaman tiga state; agregat menampilkan "Diperbarui: …".
| Path | View (`views/quality/…`) | Peran | navGroup / navLabel | meta.feature |
|---|---|---|---|---|
| `mutu/instrumen` · `/:id` | `InstrumentsView.vue`, `InstrumentEditorView.vue` (pohon standar → butir → indikator, bobot, rubrik, sumber data: Manual / Metrik + dropdown kamus metrik, dialog impor XLSX dengan hasil dry-run, tombol "Versi baru") | qa_coordinator, kepala_sekolah | Penjaminan Mutu / Standar & Indikator | `quality` |
| `mutu/target` | `TargetsView.vue` (tahun ajaran × indikator: target, penanggung jawab) | qa_coordinator, kepala_sekolah | Penjaminan Mutu / Target Indikator | `quality` |
| `mutu/indikator/:id` | `IndicatorDetailView.vue` (nilai otomatis + "Diperbarui", target, bukti tertaut) | qa_coordinator, kepala_sekolah, owner | (tidak di nav) | `quality` |
| `mutu/bukti` | `EvidenceView.vue` (unggah berkas / tautan laporan SEKOCI / URL, multi-indikator, versi, badge teks Menunggu/Terverifikasi/Ditolak, kelengkapan per standar dengan progress bar + angka %, ikon kunci untuk bukti terkunci) | qa_coordinator, owner indikator | Penjaminan Mutu / Repositori Bukti | `quality` |
| `mutu/evaluasi-diri` · `/:id` | `SelfEvaluationsView.vue`, `SelfEvaluationView.vue` (tabel indikator: target, capaian, sumber, skor, badge "Di bawah target", RTL inline; skor per standar; daftar blocker sebelum Finalisasi; `ApprovalTimeline` (T1); banner terkunci + tombol "Ajukan buka kembali" dengan alasan) | qa_coordinator, kepala_sekolah | Penjaminan Mutu / Evaluasi Diri | `quality` |
| `mutu/tindak-lanjut` | `ActionPlansView.vue` (filter owner/status, badge Terlambat berteks) | qa_coordinator, owner | Penjaminan Mutu / Rencana Tindak Lanjut | `quality` |
| `mutu/laporan` | `QaReportsView.vue` (tab Laporan Evaluasi Diri / Persiapan Akreditasi / Perbandingan Unit (khusus yayasan), ekspor XLSX, cetak) | qa_coordinator, kepala_sekolah, kepala_yayasan | Penjaminan Mutu / Laporan Akreditasi | `quality` |
| `pelaporan/dapodik` · `/impor/:id` | `DapodikView.vue` (unggah file per entitas, riwayat impor), `DapodikDiffView.vue` (tabel perbedaan per field: nilai SEKOCI vs Dapodik, dialog Putuskan: Perbarui Dapodik / Perbarui SEKOCI (tautan ke menu pemilik) / Abaikan + alasan wajib, ekspor data pembaruan) | operator_dapodik, admin | Pelaporan Pemerintah / Sinkronisasi Dapodik | `gov_reporting` |
| `pelaporan/dapodik/pemetaan` | `DapodikMappingView.vue` | operator_dapodik, admin | Pelaporan Pemerintah / Pemetaan Dapodik | `gov_reporting` |
| `pelaporan/dapodik/log` | `DapodikLogView.vue` | operator_dapodik, admin | Pelaporan Pemerintah / Log Sinkronisasi | `gov_reporting` |
| `pelaporan/regulasi/template` | `RegulatoryTemplatesView.vue` (kolom = label + metrik dari kamus) | admin | Pelaporan Pemerintah / Template Laporan | `gov_reporting` |
| `pelaporan/regulasi` · `/:id` | `RegulatoryReportsView.vue`, `RegulatoryReportView.vue` (pratinjau live sebelum diajukan; setelah diajukan menampilkan **snapshot** + "Data per: <snapshot_at>"; `ApprovalTimeline`; ekspor; tandai diserahkan; riwayat) | admin, kepala_sekolah, kepala_yayasan | Pelaporan Pemerintah / Laporan Regulasi | `gov_reporting` |

## 7. Bukti handoff
- Test tabel semua fungsi §3, termasuk **AC-142-01** (fake insight mengembalikan `attendance_rate` + `as_of` → respons `/qa/indicators/{id}/value` memuat nilai & `as_of`; fake 503 → tidak ada angka 0), **AC-143-01**, **AC-144-01** (finalize ditolak 422 dengan blocker, lalu sukses setelah RTL dibuat), **AC-145-01** (diff tercatat, fake identity mencatat **nol** panggilan tulis), **AC-146-01** (ajukan → fake insight diubah → GET laporan tetap snapshot lama).
- Test handler: BR-QA-01 (setelah callback `approved`, ubah item/ganti bukti → 409; reopen disetujui → bisa diubah, `qa_self_eval_versions` versi 1 tetap ada), callback idempoten (dua kali → satu versi), maker-checker dihormati core (pengaju tidak muncul sebagai penyetuju), BR-GOV-01 (keputusan tanpa alasan → 422), 403 untuk peran lain dan lintas sekolah, NIK mentah tidak ada di `dapodik_import_rows` maupun audit, ekspor tercatat di audit (CC-04).
- Cakupan: FR-142-001..003, FR-143-001..004, FR-144-001..004, FR-145-001..004, FR-146-001..003 (18 FR) dan AC-142-01, AC-143-01, AC-144-01, AC-145-01, AC-146-01 (5 AC), masing-masing dengan nama test atau langkah UI di tabel cakupan.
- Migrasi up → down 1 → up di DB `skci_t23_quality`; core diuji ulang dari nol setelah penambahan request type.
- **Playwright** (setelah merge gelombang): qa_coordinator impor instrumen XLSX → tautkan indikator ke metrik kehadiran → nilai + "Diperbarui" tampil → set target & owner → owner unggah bukti → QA verifikasi → kelengkapan naik → evaluasi diri dengan indikator di bawah target ditolak finalisasi sampai RTL diisi → ajukan → penyetuju lain menyetujui di Approval Center → banner terkunci tampil. operator_dapodik unggah CSV siswa dengan NISN berbeda → perbedaan tampil → putuskan dengan alasan → ekspor. admin buat laporan regulasi → ajukan → setujui → ekspor → data siswa dikoreksi → laporan lama tetap. Peran guru membuka `/mutu/bukti` → halaman unauthorized.
