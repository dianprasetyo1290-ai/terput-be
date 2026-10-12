# T24: Service `insight`: metric dictionary & read model, BI & dashboard kustom, AI Assistant, analitik prediktif

**Menu PRD:** 152, 153, 154 (BR-AI-01, BR-AI-02, BR-GEN-01, IF-ADV-04, IF-ADV-06, NFR-ADV-01/02/04, OQ-35, OQ-44) · **Branch:** `feat/T24-insight` · **Service:** `insight` **baru**, port **8096** (paket `internal/metrics`, `internal/bi`, `internal/assistant`, `internal/predict`) · **Migrasi:** `services/insight/migrations/0001–0009` (seluruh rentang milik T24)
**Prasyarat:** gelombang 4 di-merge. T1 (audit), T9 (peran + feature key). **T23 berjalan paralel** dan memakai kontrak metrik §4.1 milik task ini, jadi kontrak itu **tidak boleh diubah** tanpa persetujuan reviewer.
**Feature key:** `bi` (152), `ai_assistant` (153), `predictive` (154). **Peran:** `bi_analyst` (baru), serta `kepala_yayasan` (Pengurus/Admin Yayasan), `kepala_sekolah` (Kepala Unit), `finance_manager`, `admin` (Finance/School Admin), `superadmin` (= Platform Admin FullCircle, kelola panduan produk global). AI Assistant: semua peran login sesuai hak aksesnya. Nama peran & feature key sesuai **T9 §2** (feature key = allowlist `FeatureKeys` identity).
**Baca PRD:** Bagian 4 menu 152–154, §3 (Custom Dashboard memakai read model & metric dictionary; AI log & prediksi), §6 alur Asisten AI, §7 IF-ADV-04/06, §8 NFR-ADV-01/02/04. Bagian 1 §8 (Read model analitik), §17 (metric dictionary + governance metrik), menu 51–53 (dashboard eksekutif, nilai konsolidasi dibedakan dari nilai unit), BR-GEN-01/06, CC-04, CC-10.

## 1. Acuan kode
- **Kerangka service:** salin `services/billing/` (layout, `internal/common`, Dockerfile). Audit outbox + forward job dari `services/hr/migrations/0001_hris.up.sql` + `services/hr/internal/jobs`; runner dari `services/core/internal/jobs/runner.go`.
- **Adapter penyedia AI:** tiru pola `services/notification/internal/provider/provider.go` (`New(name, production)`; `unconfigured` menolak semua panggilan dengan pesan jelas; `dev` dilarang di production; nama tak dikenal → error saat start).
- **Rumus yang sudah ada (jangan ditulis ulang di insight):** `services/billing/internal/handler/summary.go` (`FinanceSummary`: billed/paid/outstanding, bucket aging, debitur per siswa, status via `model.DeriveBillStatus`), attendance `StudentAttendanceSummaryDashboard`/`StudentAttendanceSummary`, admission `AdminDashboard`, identity `dashboard.go` (hitung siswa/guru aktif; cakupan yayasan via `schools.foundation_id`). Ekstrak bagian hitungnya menjadi fungsi murni di service pemilik lalu panggil dari handler lama **dan** endpoint `metric-facts` baru (§4.2), sehingga dashboard standar (menu 17, 26, 51–53) dan analitik memakai rumus yang sama (Bagian 1 §17).
- **Grafik:** belum ada library chart. Bar ala `views/billing/FinanceDashboardView.vue` (div + Tailwind). Buat **satu** `components/insight/MetricChart.vue` (bar/line SVG, label teks + tabel alternatif untuk aksesibilitas) tanpa dependensi baru. `StatCard.vue`/`DashboardHero.vue` dipakai ulang.
- **Pendaftaran service** (hanya tambah baris): `.env.example` (`INSIGHT_PORT=8096`, `INSIGHT_DB_NAME`), `docker-compose.yml` (+ `insight-migrate`), `docker-compose.local.yml`, `docker-compose.server.yml`, `web/src/api/client.ts` (`insight`), `web/.env*` (`VITE_INSIGHT_URL=http://localhost:8096`), `web/nginx.conf` (`location /api/insight/` → `http://insight:8096/`), `db-init/` bila perlu.
- **Config env:** `PORT, DATABASE_URL, SEKOCI_JWT_SECRET, SEKOCI_INTERNAL_KEY, IDENTITY_URL, BILLING_URL, ATTENDANCE_URL, ADMISSION_URL, HR_URL, STUDENTLIFE_URL, NOTIFICATION_URL, APP_PUBLIC_URL` (tautan di email laporan), `METRIC_REFRESH_DEFAULT_MINUTES, JOB_INTERVAL_SECONDS, AI_PROVIDER` (default kosong = `unconfigured`), `AI_TIMEOUT_SECONDS, AI_MAX_CONTEXT_CHARS, SENTRY_DSN`. Kredensial penyedia AI hanya lewat env/secret, tidak ke DB.

## 2. Migrasi (tabel ber-tenant: `id uuid PK`, `school_id uuid NOT NULL`, `created_at/updated_at`)
**`0001_insight_base`**: `audit_events` (salin skema hr).

**`0002_metric_dictionary`** (FR-152-001, Bagian 1 §17: satu definisi per metrik, tingkat produk, **tanpa** `school_id`)
```
metric_definitions(key text PK, name, description, formula text NOT NULL, unit CHECK IN('percent_bp','idr','count','ratio_milli','days'),
  owner_service text NOT NULL, data_owner text NOT NULL, source_metric text NOT NULL,   -- nama metrik di GET /internal/metric-facts service pemilik
  allowed_group_by text[] NOT NULL DEFAULT '{}', additive bool NOT NULL,                 -- boleh dijumlah lintas unit (count/idr) atau dihitung ulang dari pembilang/penyebut (rasio)
  refresh_minutes int NULL, limitations text, version int NOT NULL DEFAULT 1, status CHECK IN('active','deprecated'))
metric_school_params(school_id, metric_key FK, params jsonb NOT NULL, effective_from date, effective_to date NULL,
  UNIQUE(school_id, metric_key, effective_from))
```
Seed (rumus disalin dari PRD, bukan angka bisnis): `collection_rate` (pembayaran teralokasi ÷ tagihan jatuh tempo periode sama, billing), `outstanding_idr` (tagihan jatuh tempo − terbayar, billing, group_by `aging_bucket`), `billed_idr`, `paid_idr` (billing, group_by `month`), `ppdb_conversion` (selesai daftar ulang ÷ total pendaftar, admission), `ppdb_applicants` (admission, group_by `wave`), `attendance_rate` ((hadir + terlambat) ÷ hari efektif, attendance, group_by `month|grade|class`; param per sekolah `late_counts_as_present`, default `true` sesuai rumus PRD, ditandai "perlu konfirmasi per klien"), `active_students`, `active_teachers` (identity), `active_staff` (hr), `verified_achievements` (prestasi terverifikasi, studentlife T21, group_by `level`). Bucket aging mengikuti bucket yang sudah ada di `FinanceSummary`.
```
metric_snapshots(school_id, metric_key FK, period_kind CHECK IN('day','month','term','academic_year'), period_start date, period_end date,
  group_by text NOT NULL DEFAULT '', dim_value text NOT NULL DEFAULT '', numerator bigint NULL, denominator bigint NULL, value bigint NULL,
  source_as_of timestamptz NOT NULL, computed_at timestamptz NOT NULL,
  PRIMARY KEY(school_id, metric_key, period_kind, period_start, group_by, dim_value))       -- read model
metric_refresh_runs(id, school_id, metric_key, period_start, status CHECK IN('ok','failed'), error text NULL, started_at, finished_at)
```
**`0003_bi`** (FR-152-001..004)
```
dashboards(id, school_id, owner_user_id, title, scope_kind CHECK IN('unit','foundation'), shared_roles text[] NOT NULL DEFAULT '{}', deleted_at)
dashboard_widgets(id, school_id, dashboard_id FK, sort, title, metric_key FK, chart CHECK IN('stat','bar','line','table'),
  group_by text NULL, period jsonb NOT NULL)            -- {kind:'relative', value:'current_academic_year'|'last_12_months'|'this_month'} atau {from,to}
saved_reports(id, school_id, owner_user_id, title, columns jsonb NOT NULL, filters jsonb NOT NULL, shared_roles text[], deleted_at)
scheduled_reports(id, school_id, saved_report_id FK, frequency CHECK IN('daily','weekly','monthly'), weekday int NULL, month_day int NULL,
  send_hour int CHECK(send_hour BETWEEN 0 AND 23), recipient_user_ids uuid[] NOT NULL, active bool, next_run_at timestamptz, last_run_at NULL, created_by)
scheduled_report_runs(id, school_id, schedule_id FK, recipient_user_id, status CHECK IN('sent','skipped','failed'), reason text NULL,
  deliveries jsonb NULL, idempotency_key text NOT NULL, UNIQUE(school_id, idempotency_key))
analytics_scope_grants(id, school_id /*sekolah pemberi*/, user_id, granted_school_id, granted_by, revoked_at NULL,
  UNIQUE(user_id, granted_school_id) WHERE revoked_at IS NULL)
bi_export_tokens(id, school_id, name, token_hash text UNIQUE, metric_keys text[] NOT NULL, school_ids uuid[] NOT NULL,
  expires_at timestamptz NOT NULL, revoked_at NULL, last_used_at NULL, created_by)       -- token mentah hanya tampil sekali
```
**`0004_assistant`** (FR-153-001..005)
```
assistant_settings(school_id PK, log_retention_days int NULL)     -- NULL = simpan sampai sekolah menetapkan kebijakan (PRD: retensi per klien)
help_articles(id, school_id uuid NULL /*NULL = panduan produk global*/, title, body_md, tags text[], route_name NULL,
  status CHECK IN('draft','published'), updated_by)              -- isi ditulis superadmin (Platform Admin), migrasi TIDAK mengarang konten panduan
assistant_intents(id, school_id NULL, key, phrases text[] NOT NULL, tool_key text NOT NULL, roles text[] NOT NULL)   -- seed: frasa pemicu tool §4.3
assistant_conversations(id, school_id, user_id, active_role, started_at, last_message_at)
assistant_messages(id, school_id, conversation_id FK, sender CHECK IN('user','assistant'), content text, ai_generated bool NOT NULL,
  sources jsonb NOT NULL DEFAULT '[]', tool_calls jsonb NOT NULL DEFAULT '[]', provider text NULL, latency_ms int, created_at)
assistant_actions(id, school_id, conversation_id FK, message_id FK, action_key, payload jsonb, preview jsonb,
  status CHECK IN('proposed','confirmed','executed','cancelled','failed','expired'), proposed_by uuid NOT NULL,
  confirmed_at NULL, executed_at NULL, result jsonb NULL, expires_at timestamptz NOT NULL, idempotency_key text NOT NULL,
  UNIQUE(school_id, idempotency_key))
```
**`0005_predict`** (FR-154-001..004, OQ-44)
```
insight_settings(school_id PK, predictive_enabled bool NOT NULL DEFAULT false, enabled_by NULL, enabled_at NULL, review_note text NULL)
risk_rules(id, school_id, model CHECK IN('arrears_risk','absence_risk','ppdb_forecast','cashflow_forecast'), version int,
  params jsonb NOT NULL,          -- bobot, batas normalisasi, ambang low/medium/high, jendela hari, horizon evaluasi: SEMUA diisi sekolah
  effective_from date NOT NULL, created_by, status CHECK IN('active','retired'), UNIQUE(school_id, model, version))
prediction_runs(id, school_id, model, rule_id FK, run_at, status CHECK IN('ok','failed','skipped'), reason text NULL)
predictions(id, school_id, run_id FK, model, subject_kind CHECK IN('student','wave','month'), subject_id text,
  score int NULL, level CHECK IN('low','medium','high') NULL, projected_value bigint NULL, factors jsonb NOT NULL,
  review_label text NOT NULL DEFAULT 'perlu_tinjauan')
prediction_reviews(id, school_id, prediction_id FK, reviewed_by, note text NOT NULL, created_at)     -- catatan keputusan manusia
prediction_accuracy(id, school_id, model, rule_id, period_start, period_end, measure CHECK IN('precision_bp','recall_bp','mape_bp'),
  value_bp int, sample_size int, computed_at)
```
Tidak ada seed `risk_rules` (bobot/ambang = keputusan sekolah). `scripts/seed-insight-risk-rules.sql` sebagai contoh demo lokal saja, diberi komentar "contoh, bukan rekomendasi". Tanpa aturan aktif → 409 "Aturan prediksi belum dikonfigurasi".

## 3. Fungsi murni + vektor
| Fungsi | Vektor |
|---|---|
| `ResolveScope(claims, grants, foundationSchools, requested []string) ([]schoolID, error)` | superadmin: yang diminta; `kepala_yayasan`: semua sekolah yayasannya; `bi_analyst`: sekolah sendiri + grant aktif (hanya sekolah satu yayasan); `kepala_sekolah`/`admin`/`finance_manager`: sekolah sendiri. Permintaan di luar scope **dipotong**, bukan ditolak. **AC-152-01** dashboard buatan kepala_yayasan dibuka kepala_sekolah SD → hanya `[SD]` (FR-152-002) |
| `Aggregate(def, rows) (perUnit, consolidated)` | `additive` → jumlah; rasio → Σpembilang ÷ Σpenyebut (bukan rata-rata persen). Konsolidasi dikembalikan terpisah dari nilai unit (menu 53). Vektor AC-52-01: tagihan 100.000.000, teralokasi 85.000.000 → 8500 bp |
| `ResolvePeriod(spec, today, terms)` | `current_academic_year`, `last_12_months`, `this_month` dengan zona `Asia/Jakarta` |
| `IsStale(snapshot, def, now)` | lewat `refresh_minutes` (atau default env) → refresh on-demand |
| `NextRun(schedule, now)` | harian/mingguan/bulanan, `month_day` 31 di Februari → hari terakhir bulan |
| `ClassifyIntent(text, intents, role) (tool, confidence)` | token overlap ternormalisasi (huruf kecil, buang tanda baca); tool di luar `roles` → tidak terpilih |
| `RetrieveHelp(text, articles, k)` | artikel published dengan skor kecocokan tertinggi; skor 0 → "Panduan untuk topik ini belum tersedia" (FR-153-001) |
| `Minimize(payload) payload` | **NFR-ADV-01** buang `name, nisn, nik, email, phone, address, student_user_id`, ganti ID dengan alias `S1, S2…`; hanya angka agregat + label generik yang tersisa. Test: tak ada substring data pribadi di output |
| `ArrearsRiskScore(facts, params) (score, level, topFactors)` | faktor terdokumentasi: `overdue_bill_count`, `max_days_overdue`, `outstanding_ratio_bp`, `late_payment_count`; tiap faktor dinormalisasi `min(nilai/cap,1)`, skor = Σ bobot×faktor (0–100), level dari ambang di `params`; `topFactors` = 3 kontribusi terbesar. **AC-154-01** skor tinggi → faktor penjelas ada (FR-154-001) |
| `AbsenceRiskScore(facts, params)` | `absence_rate_bp`, `consecutive_absent_days`, `unexcused_count`, `trend_delta_bp` (jendela ini vs sebelumnya), pola sama |
| `ProjectApplicants(currentCum, dayIndex, prevCurve)` | proyeksi = kumulatif sekarang × total tahun lalu ÷ kumulatif tahun lalu pada hari ke-sama; kurva tahun lalu kosong / kumulatif 0 → `nil` + "data historis tidak cukup" (FR-154-002) |
| `ProjectCashflow(dueByMonth, collectionRateBp)` | proyeksi bulan = tagihan jatuh tempo × rata-rata `collection_rate` N bulan terakhir (N di `params`) ÷ 10000, pembulatan rupiah di satu fungsi |
| `Precision/Recall(flaggedHigh, actualOverdue)` dan `MAPE(projected, actual) bp` | himpunan kosong → `nil`, bukan 0 (FR-154-004) |

## 4. Integrasi & job
### 4.1 Kontrak metrik untuk task lain (T23 dan berikutnya) — `X-Internal-Key`
| Method & path | Hasil |
|---|---|
| `GET /internal/metrics?status=active` | `[{key, name, unit, allowed_group_by, owner_service, formula, limitations}]` |
| `GET /internal/metrics/{key}/value?school_id=&from=&to=[&group_by=]` | `200 {key, unit, value, numerator, denominator, as_of, computed_at, rows?:[{dim, value, numerator, denominator}]}` dari read model (refresh on-demand bila basi). `404` key tak dikenal. `422` group_by tak diizinkan. `503 {message}` sumber gagal dan belum ada snapshot. Tidak pernah mengembalikan 0 palsu |
Metrik baru (mis. milik `ops`/`studentlife`) ditambah lewat migrasi insight berikutnya + endpoint §4.2 di service pemiliknya; T23 tidak perlu berubah.

### 4.2 Endpoint `metric-facts` di service pemilik (milik T24, hanya tambah handler, tanpa migrasi)
Semua: `GET /internal/metric-facts?metric=&school_id=&from=&to=[&group_by=]` → `{metric, numerator, denominator, value, unit, rows:[…], as_of}`, integer, `school_id` wajib.
| Service | `metric` |
|---|---|
| billing | `collection_rate`, `outstanding_idr`, `billed_idr`, `paid_idr` (rumus diekstrak dari `FinanceSummary`) |
| attendance | `attendance_rate` (param `late_counts_as_present` dikirim insight) |
| admission | `ppdb_conversion`, `ppdb_applicants` (+ `group_by=wave_day` untuk kurva kumulatif per hari sejak gelombang dibuka) |
| identity | `active_students`, `active_teachers` |
| hr | `active_staff` |
| studentlife | `verified_achievements`: **tidak** menambah endpoint baru; adapter insight membaca `GET /internal/achievements/summary` yang sudah disediakan T21 dan memetakannya ke bentuk `metric-facts` (kalau parameter T21 berbeda, ikuti T21 dan catat di handoff) |
Data per siswa untuk prediksi (hanya dibaca saat job, tidak disimpan mentah): billing `GET /internal/risk-facts/arrears?school_id=&as_of=` → `[{student_user_id, overdue_bill_count, max_days_overdue, billed_idr, outstanding_idr, late_payment_count}]`; attendance `GET /internal/risk-facts/absence?school_id=&from=&to=` → `[{student_user_id, effective_days, absent_days, unexcused_count, consecutive_absent_days}]`.
Identity (dipakai juga T23): `GET /internal/schools/{id}/foundation-schools` → `[{id, name, level}]` sekolah aktif yang `foundation_id`-nya sama (SQL mengikuti `dashboard.go`).

### 4.3 AI Assistant (BR-AI-01, FR-153-*, IF-ADV-04, OQ-35)
Alur: pertanyaan → `ClassifyIntent` → tool → **pemeriksaan hak akses oleh service sumber** → jawaban + sumber → konfirmasi bila aksi → log (PRD §6).
- **Hak akses penanya:** tool data memanggil endpoint **publik** service sumber dengan **JWT penanya** (header `Authorization` diteruskan), bukan internal key, sehingga otorisasi server yang sudah ada berlaku. Tool awal: `help_search` (lokal), `metric_lookup` (lokal, lewat `ResolveScope`), `my_bills` (billing `/me/bills`), `child_bills` (billing `/parent/children/{id}/bills`; `id` hanya boleh dari identity `/me/children` milik penanya), `arrears_summary` (billing `/admin/finance/summary`), `draft_announcement`, `draft_message`, `summarize_report`. 403/404 dari sumber → jawaban "Anda tidak memiliki akses ke data tersebut" tanpa detail (**AC-153-01**).
- **Aksi (FR-153-004):** registry tertutup: `create_announcement_draft` → identity `POST /announcements` (tersimpan sebagai draf, tidak dipublikasikan) dan `send_payment_reminders` → billing endpoint baru `POST /admin/bills/reminders` `{student_user_ids[], message}` (milik T24, peran admin/finance_manager dicek billing, Idempotency-Key, kirim lewat notification ke wali tertaut). Asisten hanya membuat `assistant_actions(status='proposed')` berisi pratinjau penerima + isi. Eksekusi **hanya** lewat `POST /assistant/actions/{id}/confirm` oleh `proposed_by` yang sama sebelum `expires_at`, memakai JWT pengguna itu (**AC-153-02**).
- **Penyedia AI:** paket `internal/assistant/provider`: `unconfigured` (default; tool `draft_*`/`summarize_*` → 503 "Penyedia AI belum dikonfigurasi") dan `dev` (template deterministik, dilarang di production). Adapter penyedia nyata ditambah **setelah OQ-35 diputuskan**, dengan syarat: hanya menerima output `Minimize`, tanpa pelatihan model (opsi no-training/zero-retention penyedia diaktifkan), lokasi pemrosesan sesuai keputusan UU PDP. Help dan data lookup tetap berjalan tanpa penyedia.
- **Penanda & log (FR-153-005):** setiap jawaban menyimpan `ai_generated` (true bila teks berasal dari penyedia), `sources` (artikel/endpoint/metrik + `as_of`), `tool_calls`. Job harian menghapus percakapan lebih tua dari `log_retention_days` (bila diisi).
- **Kinerja (NFR-ADV-02):** panggilan penyedia dibatasi `AI_TIMEOUT_SECONDS`. Lewat batas → kembalikan hasil tool/sumber tanpa teks AI + pesan "Jawaban AI belum tersedia, coba lagi". Jalur help/data tidak menunggu penyedia.

### 4.4 Prediksi (BR-AI-02, OQ-44)
- Saklar `insight_settings.predictive_enabled` default **mati**, terpisah dari feature key. Mati → endpoint prediksi 409 "Analitik prediktif belum diaktifkan" dan job tidak berjalan. Mengaktifkan wajib `review_note` (catatan tinjauan etika/keadilan) dan diaudit.
- Job harian (advisory lock per model): ambil fakta §4.2 → hitung → `predictions` dengan `review_label='perlu_tinjauan'`. Job **tidak** memanggil endpoint tulis apa pun (billing/notification/identity). Test memakai fake client yang menghitung panggilan → harus nol (**AC-154-01**, FR-154-003).
- Akurasi (FR-154-004): job menilai run lama setelah horizon di `params` lewat → `prediction_accuracy`. UI menampilkan akurasi periode sebelumnya + "Tanggal kalibrasi aturan terakhir" (= `risk_rules.effective_from`, karena tidak ada model ML yang dilatih) + waktu run terakhir.

### 4.5 Job lain & audit
- Refresh read model: per metrik × sekolah ber-feature `bi` atau dipakai T23, interval `refresh_minutes`/default env. Gagal → `metric_refresh_runs.failed` + exception core (`kind='import'`, idempotency per metrik-periode-hari).
- Laporan terjadwal (FR-152-003): saat `next_run_at` lewat, untuk **tiap penerima** hitung ulang `ResolveScope(penerima)`. Penerima tanpa peran BI di sekolah itu → `skipped` + alasan. Kirim `POST /internal/notifications` `{type:'scheduled_report', channels:['in_app','email'], title, body: ringkasan agregat + tautan APP_PUBLIC_URL}` (`Idempotency-Key: sched:<id>:<recipient>:<run_date>`). Simpan `deliveries` dari respons agar UI bisa menampilkan "email dinonaktifkan penerima" (preferensi notification dihormati).
- Ekspor (FR-152-004, IF-ADV-06, CC-04, BR-GEN-06): hanya angka agregat dari read model (tanpa baris per orang), mengikuti filter, setiap ekspor dan setiap tarikan token diaudit. Token: hash sha256 di DB, scope metrik + sekolah, wajib kedaluwarsa, bisa dicabut.

## 5. Endpoint (JWT, scope `school_id` dari claims; data lintas sekolah selalu lewat `ResolveScope`)
| Method & path | Peran |
|---|---|
| `GET /metrics` (kamus) · `GET /metrics/{key}` | semua peran BI |
| `PUT /metrics/{key}/description` (deskripsi, keterbatasan) · `PUT /metrics/{key}/school-params` | superadmin (kamus), admin (param sekolahnya) |
| `POST /bi/query` `{metric_key, period, group_by?, school_ids?}` → `{per_unit[], consolidated, as_of, computed_at}` | bi_analyst, kepala_yayasan, kepala_sekolah, finance_manager, admin, superadmin |
| `CRUD /bi/dashboards` (+ widgets) · `GET /bi/dashboards/{id}/data` | buat/ubah: bi_analyst, kepala_yayasan, superadmin; lihat: pemilik + `shared_roles` (angka tetap per scope pembuka) |
| `CRUD /bi/reports` · `GET /bi/reports/{id}/run` · `POST /bi/reports/{id}/export?format=csv\|xlsx` | sama dengan dashboard |
| `CRUD /bi/schedules` · `GET /bi/schedules/{id}/runs` | bi_analyst, kepala_yayasan, superadmin |
| `CRUD /bi/export-tokens` (token mentah sekali tampil) · `GET /bi/feed/{metric_key}.csv` (header `Authorization: Bearer <token>`, tanpa JWT) | bi_analyst, kepala_yayasan, superadmin · token |
| `GET/POST/DELETE /bi/scope-grants` | kepala_yayasan (sekolah yayasannya), superadmin |
| `POST /assistant/messages` `{conversation_id?, text}` · `GET /assistant/conversations` · `GET /assistant/conversations/{id}` | semua peran login (sekolah ber-feature `ai_assistant`) |
| `POST /assistant/actions/{id}/confirm` · `/cancel` | `proposed_by` saja |
| `CRUD /assistant/help-articles` · `CRUD /assistant/intents` | superadmin (global); admin (artikel khusus sekolahnya) |
| `GET/PUT /assistant/settings` (retensi) | admin, superadmin |
| `GET/PUT /predict/settings` (saklar + review_note) · `CRUD /predict/rules` | kepala_sekolah, kepala_yayasan, superadmin |
| `GET /predict/arrears?level=` · `GET /predict/absence?level=` · `GET /predict/ppdb` · `GET /predict/cashflow` · `GET /predict/accuracy` · `POST /predict/{id}/reviews` | kepala_sekolah, finance_manager, admin, kepala_yayasan, bi_analyst (arrears: juga finance_manager/admin; absence: tanpa finance_manager) |
| billing `POST /admin/bills/reminders` | admin, finance_manager (dicek di billing) |

## 6. Frontend
`web/src/services/insight/{metrics.ts,bi.ts,assistant.ts,predict.ts,types.ts}`, `web/src/router/domains/insight.routes.ts`. Semua layar agregat menampilkan "Diperbarui: <as_of>" dan membedakan baris **Konsolidasi** dari baris unit.
| Path | View (`views/insight/…`) | Peran | navGroup / navLabel | meta.feature |
|---|---|---|---|---|
| `analitik/dashboard` · `/:id` | `DashboardsView.vue`, `DashboardView.vue` (grid widget `MetricChart`/`StatCard`, mode ubah: tambah widget pilih metrik dari kamus, group_by, periode, bagikan ke peran) | bi_analyst, kepala_yayasan, kepala_sekolah, finance_manager, admin | Analitik / Dashboard Kustom | `bi` |
| `analitik/laporan` · `/:id` | `ReportBuilderView.vue` (kolom metrik × dimensi, filter, pratinjau, ekspor CSV/XLSX) | sama | Analitik / Pembuat Laporan | `bi` |
| `analitik/jadwal` | `SchedulesView.vue` (frekuensi, penerima autocomplete, riwayat kirim per penerima dengan badge teks) | bi_analyst, kepala_yayasan | Analitik / Laporan Terjadwal | `bi` |
| `analitik/kamus-metrik` | `MetricDictionaryView.vue` (rumus, pemilik data, sumber, refresh, keterbatasan) | semua peran BI | Analitik / Kamus Metrik | `bi` |
| `analitik/ekspor` | `ExportTokensView.vue` (buat token: dialog tampil sekali + tombol salin, cabut, riwayat tarikan) | bi_analyst, kepala_yayasan | Analitik / Ekspor BI Eksternal | `bi` |
| `analitik/akses` | `ScopeGrantsView.vue` | kepala_yayasan | Analitik / Akses Analis | `bi` |
| `analitik/prediksi/tunggakan` · `/ketidakhadiran` | `ArrearsRiskView.vue`, `AbsenceRiskView.vue` (tabel skor + level berteks + 3 faktor penjelas, badge "Perlu tinjauan", dialog "Catat tinjauan"; **tanpa** tombol tindakan ke siswa) | lihat §5 | Analitik / Risiko Tunggakan, Risiko Ketidakhadiran | `predictive` |
| `analitik/prediksi/proyeksi` | `ForecastView.vue` (tab Pendaftar PPDB per gelombang / Arus Kas per bulan, akurasi periode lalu, tanggal kalibrasi aturan) | kepala_sekolah, finance_manager, kepala_yayasan, bi_analyst | Analitik / Proyeksi | `predictive` |
| `analitik/prediksi/pengaturan` | `PredictSettingsView.vue` (saklar + catatan tinjauan wajib, editor aturan per model) | kepala_sekolah, kepala_yayasan | Analitik / Pengaturan Prediksi | `predictive` |
| `asisten/riwayat` | `AssistantHistoryView.vue` | semua peran login | Akun / Riwayat Asisten | `ai_assistant` |
| `pengaturan/panduan-asisten` | `HelpArticlesView.vue` | superadmin, admin | Pengaturan / Panduan Asisten | `ai_assistant` |
Komponen global `components/insight/AssistantPanel.vue` dipasang di `AppLayout.vue` (tambah baris, tampil hanya jika `ai_assistant` aktif): tombol ikon ber-`aria-label`, panel percakapan, badge "Dihasilkan AI" pada jawaban AI, chip sumber yang bisa diklik, kartu aksi berisi pratinjau penerima & pesan + tombol **Konfirmasi**/**Batal** (dialog konfirmasi), draf yang bisa diedit sebelum disalin/dipakai (FR-153-003).

## 7. Bukti handoff
- Test tabel §3 dengan vektor di atas. **AC-152-01**: kepala_yayasan membuat dashboard ber-`scope_kind='foundation'` → kepala_sekolah SD membuka `/bi/dashboards/{id}/data` → semua angka dan konsolidasi hanya SD; `school_ids` milik SMP di body dipotong.
- **AC-153-01**: orang tua A menanyakan tunggakan siswa milik orang tua B → fake billing 403 / id tidak ada di `/me/children` → jawaban menolak, tanpa angka. **AC-153-02**: aksi pengingat → status `proposed`, fake billing **nol** panggilan sampai `confirm`; confirm oleh user lain → 403; confirm dua kali → satu eksekusi (idempotensi).
- **AC-154-01**: skor tinggi dengan faktor penjelas tampil, fake billing/notification/identity mencatat nol panggilan tulis selama job; saklar mati → 409.
- Test kontrak §4.1 (dipakai T23): 200/404/422/503, tanpa 0 palsu. Test `Minimize` (NFR-ADV-01). Test kinerja NFR-ADV-04: seed snapshot satu tahun ajaran (12 bulan × semua metrik × 3 sekolah) di DB `TEST_PG_DSN`, `POST /bi/query` dan `GET /bi/dashboards/{id}/data` < 5 detik. NFR-ADV-02: provider fake lambat → respons tetap kembali dalam batas timeout dengan sumber.
- Test ekspor & token: agregat saja, diaudit, token kedaluwarsa/dicabut → 401. Laporan terjadwal: penerima tanpa wewenang → `skipped`.
- Test endpoint baru di service sumber (billing/attendance/admission/identity/hr `metric-facts`, `risk-facts`, `foundation-schools`, `bills/reminders`) di paket test masing-masing service. Handler lama (`FinanceSummary` dll.) tetap lulus test lamanya setelah ekstraksi fungsi.
- Cakupan: FR-152-001..004, FR-153-001..005, FR-154-001..004 (13 FR), AC-152-01, AC-153-01, AC-153-02, AC-154-01 (4 AC). Migrasi up → down 1 → up di `skci_t24_insight`.
- **Playwright**: kepala_yayasan membuat dashboard (collection rate per unit + konsolidasi) dan membagikannya ke kepala_sekolah → login kepala_sekolah SD hanya melihat SD. bi_analyst membuat laporan terjadwal + ekspor CSV. Orang tua bertanya tagihan anak sendiri (dijawab + sumber) lalu anak orang lain (ditolak). Admin meminta pengingat tunggakan → kartu konfirmasi → Batal → tidak ada notifikasi; ulangi → Konfirmasi → notifikasi wali terbentuk. Pengaturan prediksi diaktifkan dengan catatan → daftar risiko tampil dengan badge "Perlu tinjauan" tanpa tombol tindakan. Peran siswa membuka `/analitik/dashboard` → unauthorized.
