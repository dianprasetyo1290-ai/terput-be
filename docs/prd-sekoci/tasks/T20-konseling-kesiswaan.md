# T20: Konseling & Kesiswaan di service `studentlife`: catatan konseling, poin & pembinaan disiplin, rujukan & tindak lanjut

**Menu PRD:** 130, 131, 132 (Bagian 3 §5 Grup 23, BR-CNS-01, BR-DIS-01, NFR-OPS-03, OQ-31) · **Branch:** `feat/T20-konseling` · **Service:** `studentlife` (paket `internal/counseling`, `internal/discipline`) · **Migrasi:** `studentlife/0010`–`0019` (milik T20)
**Prasyarat:** gelombang 4. T10 di-merge (kerangka `studentlife`, paket `internal/sensitive` dan `internal/consent`, endpoint identity daftar wali `GET /internal/students/{id}/guardians`, `POST /internal/approval-callbacks`). T9 di-merge (peran + feature key).
**Peran:** `konselor`, `kesiswaan` (baru, T9) + `walikelas`, `orang_tua` (sudah ada). **Feature key (final T9 §2):** `counseling` (130, 132), `discipline` (131). Nama final peran dan feature key **mengikuti T9 §2** (kalau T9 memisahkan disiplin menjadi key sendiri, route/endpoint `/discipline/*` memakai key itu). Ganti lewat konstanta di `internal/common/roles.go` + satu konstanta TS.

## 1. Acuan kode
| Kebutuhan | Tiru / pakai | Catatan |
|---|---|---|
| Enkripsi kolom, log akses, kebijakan akses | `services/studentlife/internal/sensitive` (T10) | **pakai**, jangan salin. Modul `counseling` sudah ada di CHECK `sensitive_access_logs` & `confidential_access_policies` (T10 0001) |
| Persetujuan orang tua | `services/studentlife/internal/consent` + tabel `parent_consents` (T10) | `subject_type='referral'` |
| Callback approval | handler `POST /internal/approval-callbacks` (T10) | tambah cabang `discipline_appeal` |
| Kebijakan akses UI + log | `views/studentlife/SensitiveAccessView.vue` (T10) | tambah tab/filter modul `counseling` |
| Wali kelas, kelas, tahun ajaran | academic `GET /students/{id}/classroom`, `GET /classrooms/{id}` (homeroom), identity `schools.current_academic_year_id` | |
| Unggah bukti | `internal/storage/local.go` (T10) | prefix `discipline/` |
| Approval | core T1 §3, `KnownRequestTypes` di `services/core/internal/model/domain.go` | |
| Job | `internal/jobs/runner.go` (T10) | daftarkan job baru, jangan buat runner kedua |

## 2. Migrasi (skema ringkas)
Kolom `*_enc` = `sensitive.Encrypt` dengan AAD `"<tabel>:<school_id>:<id>"` seperti T10.

**`0010_counseling`** (130)
```
counseling_categories(id, school_id, name, active, deleted_at, UNIQUE(school_id, name) WHERE deleted_at IS NULL)
counseling_cases(id, school_id, student_id, category_id, opened_by, status CHECK IN('open','monitoring','closed'),
  disclose_to_homeroom bool NOT NULL DEFAULT false,      -- AC-130-01: keterangan "dalam pendampingan BK" hanya jika konselor mengizinkan
  opened_at, closed_at NULL, close_reason_enc bytea NULL)
counseling_case_members(case_id FK, user_id, member_role CHECK IN('counselor','kesiswaan'), added_by, added_at, removed_at NULL,
  PRIMARY KEY(case_id, user_id))                         -- "konselor yang menangani" (BR-CNS-01)
counseling_sessions(id, school_id, case_id FK NULL, kind CHECK IN('individual','group'), category_id,
  counselor_user_id, scheduled_at timestamptz, held_at NULL,
  status CHECK IN('scheduled','held','cancelled','no_show'),
  notes_enc bytea NULL, follow_up_plan_enc bytea NULL, reminder_sent_at NULL)
counseling_session_students(session_id FK, student_id, PRIMARY KEY(session_id, student_id))   -- kelompok
counseling_settings(school_id PK, reminder_days_before int NOT NULL DEFAULT 0 CHECK(>=0))      -- 0 = hari H
```
**`0011_referrals`** (132)
```
referrals(id, school_id, case_id FK, student_id, target_kind CHECK IN('internal','external'),
  target_name_enc bytea, target_contact_enc bytea NULL, reason_enc bytea NOT NULL,
  parent_summary_enc bytea NOT NULL,                     -- ringkasan yang boleh dibaca wali saat memberi persetujuan, ditulis konselor
  status CHECK IN('awaiting_consent','active','completed','declined','cancelled'),
  consent_id uuid NULL REFERENCES parent_consents, outcome_enc bytea NULL, created_by)
case_tasks(id, school_id, case_id FK, title text NOT NULL,   -- judul tampil ke penanggung jawab non-anggota: UI memperingatkan agar tanpa detail rahasia
  assignee_user_id, due_date date, done_at NULL, done_by NULL, note_enc bytea NULL, created_by)
```
**`0012_discipline`** (131)
```
discipline_types(id, school_id, code, name, kind CHECK IN('violation','merit'), points int NOT NULL CHECK(points>0),
  active, deleted_at, UNIQUE(school_id, code) WHERE deleted_at IS NULL)
discipline_thresholds(id, school_id, points int NOT NULL CHECK(points>0), action_name text NOT NULL,
  notify_parent bool NOT NULL DEFAULT true, effective_from date NOT NULL, effective_to date NULL, deleted_at)
discipline_settings(school_id PK, accumulation_scope CHECK IN('academic_year','all_time') NOT NULL DEFAULT 'academic_year')
discipline_records(id, school_id, student_id, academic_year_id, type_id FK, kind, points int /* salinan saat dicatat */,
  occurred_on date, description text, evidence jsonb NOT NULL DEFAULT '[]' /* [{object_key,file_name,mime}] */,
  reporter_user_id, status CHECK IN('active','appealed','annulled'), annulled_reason NULL, idempotency_key,
  UNIQUE(school_id, idempotency_key))
discipline_actions(id, school_id, student_id, academic_year_id NULL, threshold_id FK, triggered_by_record_id FK,
  points_at_trigger int, status CHECK IN('open','done','cancelled'), assignee_user_id NULL, notes text NULL, done_at NULL,
  UNIQUE(student_id, threshold_id, academic_year_id) WHERE status <> 'cancelled')   -- BR-DIS-01 sekali per ambang per periode
discipline_appeals(id, school_id, record_id FK, guardian_user_id, reason text NOT NULL,
  status CHECK IN('pending','approved','rejected'), approval_id NULL, idempotency_key, UNIQUE(school_id, idempotency_key),
  UNIQUE(record_id) WHERE status='pending')
```
Data disiplin **tidak** termasuk NFR-OPS-03 (hanya kesehatan & konseling), jadi tidak dienkripsi, tetapi aksesnya tetap per peran/kepemilikan. Tidak ada seed jenis pelanggaran, poin, atau ambang: semuanya diisi sekolah dari UI (BR-GEN-08). Angka 50/45/10 di AC-131-01 hanya vektor uji.

## 3. Fungsi murni + vektor uji
| Fungsi | Isi | Vektor (→ AC/FR) |
|---|---|---|
| `sensitive.CanAccess('counseling', …)` (tambahan di paket T10) | `Full`: anggota aktif kasus / konselor sesi, atau peran ber-kebijakan aktif. `StatusOnly`: wali kelas siswa jika ada kasus `open/monitoring` dengan `disclose_to_homeroom=true`. Lainnya `None` (termasuk `konselor` yang bukan anggota, `kepala_sekolah` tanpa kebijakan (OQ-31), `admin`, `orang_tua` untuk catatan) | **AC-130-01**; FR-130-002; FR-132-003; konselor lain → None; kebijakan `konselor` ditambahkan → semua konselor Full |
| `counseling.HomeroomStatus(cases)` | `dalam_pendampingan` jika ada kasus aktif yang diizinkan, selain itu `tidak_ada_informasi` (tidak membedakan "tidak ada kasus" dan "tidak diizinkan") | AC-130-01 kedua cabang |
| `counseling.ValidateSession(s)` | `group` ≥ 2 siswa, `individual` tepat 1; `held` wajib `held_at` | FR-130-001 |
| `counseling.ReminderDue(session, settings, today, tz)` | `scheduled` dan `date(scheduled_at) - reminder_days_before == today` dan belum dikirim | FR-130-003 (0 → hari H; 1 → H-1) |
| `discipline.Accumulate(records, scope, yearID)` | jumlah `points` record `kind='violation'` berstatus `active/appealed` pada periode. `merit` dijumlah terpisah dan **tidak** mengurangi (PRD tidak mengatur saling hapus; dicatat di handoff) | annulled tidak dihitung |
| `discipline.CrossedThresholds(before, after, thresholds(on date), triggered)` | ambang dengan `before < points <= after` yang belum punya aksi aktif; urut naik | **AC-131-01**: ambang 50, 45 + 10 → [50]; 40 → 100 dengan ambang 50/75/100 → tiga; 50 → 60 dengan 50 sudah terpicu → []; ambang tak berlaku pada tanggal kejadian → diabaikan |
| `discipline.CanAppeal(record, guardian)` | status `active`, belum ada banding pending | FR-131-004 |
| `referral.InitialStatus(consent)` | consent belum `granted` → `awaiting_consent`; `granted` → `active`; `declined` → `declined` | **AC-132-01**, FR-132-001 |
| `counseling.CanClose(case, tasks, reason)` | semua tugas selesai, atau alasan penutupan diisi | FR-132-003 |
| `counseling.TaskDisplayStatus(task, today)` | belum / selesai / terlambat (`due_date < today` dan belum selesai) | FR-132-002 |

## 4. Integrasi & job
- **Akses + log (NFR-OPS-03):** setiap baca/tulis isi `*_enc` konseling/rujukan memakai `sensitive.CanAccess` + baris `sensitive_access_logs(module='counseling')` dalam request yang sama (fail closed). Ditolak → 404 + log `deny`. Audit ke core tanpa plaintext. Tidak ada ekspor per siswa, tidak ada endpoint `/internal/*` yang mengembalikan isi konseling. Notifikasi ke konselor/penanggung jawab tidak memuat isi catatan (hanya "Sesi lanjutan dengan <nama> hari ini pukul …").
- **Disiplin → orang tua (FR-131-002, BR-DIS-01):** dalam **satu transaksi** simpan record → hitung `Accumulate` sebelum/sesudah → `CrossedThresholds` → insert `discipline_actions`. Setelah commit: notifikasi ke setiap wali aktif (identity `GET /internal/students/{id}/guardians`, kontrak T10 §4.3): `type:'discipline_record'` key `studentlife:discipline:<record_id>:<guardian_id>`; untuk aksi dengan `notify_parent`: `type:'discipline_action'` key `studentlife:discipline_action:<action_id>:<guardian_id>`. Notifikasi gagal tidak membatalkan record (job ulang memakai key yang sama). Kesiswaan officer juga diberi notifikasi aksi baru.
- **Banding (FR-131-004):** wali → `discipline_appeals` → core `POST /internal/approvals` `request_type='discipline_appeal'` (tambahkan ke `KnownRequestTypes` + label "Keberatan catatan pelanggaran", hanya tambah baris), `summary:{student_name, type_name, points}`, `maker_user_id`=wali, `subject_user_id`=siswa, `idempotency_key='discipline_appeal:<appeal_id>'`, callback ke `/internal/approval-callbacks`. Record → `appealed` (tetap dihitung). Alur dikonfigurasi admin (default yang disarankan di README: peran `kesiswaan`). Core 422 → tampilkan pesan, jangan auto-approve. Callback idempoten: `approved` → record `annulled` (+ alasan dari komentar), aksi yang dipicu record itu **tidak** dihapus tetapi ditandai untuk ditinjau kesiswaan (badge "Perlu ditinjau: catatan dibatalkan"); `rejected` → record kembali `active`. Wali diberi notifikasi hasilnya.
- **Rujukan (FR-132-001):** buat rujukan → `parent_consents(subject_type='referral', status='pending')` → notifikasi wali (`studentlife:consent:<consent_id>`), isi hanya tujuan + `parent_summary`. Keputusan wali lewat endpoint generik T10 `GET /consents/my` · `POST /consents/{id}/decide`. T20 mendaftarkan di `main.go`: `subject_type='referral'` → feature `counseling`, `consent.Describe` (tujuan + `parent_summary` terdekripsi, dicatat di access log sebagai `view` oleh wali), dan hook `consent.OnDecided` untuk memindah status rujukan dalam transaksi yang sama.
- **Job (runner T10):** (1) pengingat sesi lanjutan (`ReminderDue`), key `studentlife:session:<id>:<tanggal>`; (2) pengingat tugas tindak lanjut pada `due_date` dan sehari setelah terlambat, ke `assignee_user_id`, key memuat tanggal; (3) ulang notifikasi disiplin yang gagal.

## 5. Endpoint (JWT, `school_id` dari `EffectiveSchoolID`)
| Method & path | Peran | Catatan |
|---|---|---|
| `CRUD /counseling/categories` | konselor | |
| `GET /counseling/cases?status=&student_id=` · `POST /counseling/cases` · `GET/PUT /counseling/cases/{id}` | konselor, kesiswaan (hanya kasus tempat ia anggota) | pembuat otomatis anggota; `disclose_to_homeroom` hanya diubah anggota `counselor` |
| `POST/DELETE /counseling/cases/{id}/members` | anggota counselor | |
| `POST /counseling/cases/{id}/close` | anggota counselor | `CanClose` |
| `GET/POST /counseling/sessions` · `PUT /counseling/sessions/{id}` | konselor (sesi miliknya / kasus yang ia tangani) | FR-130-001/003 |
| `GET /counseling/students/{id}/status` | walikelas (homeroom siswa) | **AC-130-01**, hanya `HomeroomStatus` |
| `GET/POST /counseling/cases/{id}/referrals` · `PUT /counseling/referrals/{id}` (selesai/batal + hasil) | anggota kasus | **AC-132-01** |
| `GET/POST /counseling/cases/{id}/tasks` · `POST /counseling/tasks/{id}/done` | anggota kasus; `done` juga oleh assignee | FR-132-002 |
| `GET /counseling/my/tasks` | siapa pun yang jadi assignee | hanya judul + nama siswa + tenggat |
| `CRUD /discipline/types` · `CRUD /discipline/thresholds` · `GET/PUT /discipline/settings` | kesiswaan | FR-131-001 |
| `GET /discipline/records?classroom_id=&student_id=` · `POST /discipline/records` (multipart bukti + `Idempotency-Key`) | kesiswaan (unit); walikelas (kelasnya sendiri) | FR-131-002; respons memuat total poin + aksi yang terbentuk |
| `GET /discipline/students/{id}/summary` | kesiswaan, walikelas (kelasnya), orang_tua (anaknya, `GuardianHasAccess`) | poin pelanggaran, poin prestasi, aksi |
| `GET /discipline/actions?status=` · `PUT /discipline/actions/{id}` | kesiswaan | |
| `GET /discipline/records/{id}/evidence/{n}` | kesiswaan, walikelas (kelasnya), pelapor | file bukti; orang tua tidak (data siswa lain mungkin ada di foto) — dicatat sebagai asumsi |
| `POST /discipline/records/{id}/appeals` · `GET /discipline/my/appeals` | orang_tua | FR-131-004 |
| `GET/POST/DELETE /sensitive/access-policies?module=counseling` · `GET /sensitive/access-logs?module=counseling` | admin, superadmin (log: + auditor) | endpoint generik T10 dipakai ulang |

## 6. Frontend
`web/src/services/studentlife/{counseling.ts,discipline.ts}` + tipe di `types.ts`; route ditambahkan ke `web/src/router/domains/studentlife.routes.ts` (T10). Route konseling/disiplin `meta.feature: 'counseling'`. Landing `konselor` → `bk/kasus`, `kesiswaan` → `kesiswaan/pelanggaran` (cabang di `DashboardRedirectView.vue` jika T9 belum).

| Path | Name | View (`views/studentlife/…`) | Peran | navGroup / navLabel |
|---|---|---|---|---|
| `bk/kasus` · `bk/kasus/:id` | counseling-cases, counseling-case | `counseling/CasesView.vue`, `counseling/CaseDetailView.vue` (tab Sesi / Rujukan / Tugas Tindak Lanjut / Anggota; header status + saklar "Izinkan wali kelas melihat status pendampingan") | konselor, kesiswaan | Konseling / Kasus & Catatan |
| `bk/jadwal` | counseling-schedule | `counseling/SessionsView.vue` (daftar per tanggal, sesi individu/kelompok, catat hasil) | konselor | Konseling / Jadwal Sesi |
| `bk/tugas` | counseling-tasks | `counseling/MyTasksView.vue` | semua staf yang punya tugas (menu tampil hanya jika ada) | Konseling / Tugas Saya |
| `bk/kategori` | counseling-categories | `counseling/CategoriesView.vue` | konselor | Konseling / Kategori |
| `kesiswaan/pelanggaran` | discipline-records | `discipline/RecordsView.vue` (catat pelanggaran/prestasi: siswa, jenis, tanggal, bukti, deskripsi; pratinjau "Total menjadi 55 poin, memicu: Pemanggilan orang tua") | kesiswaan | Kesiswaan / Catatan Poin |
| `kesiswaan/pembinaan` | discipline-actions | `discipline/ActionsView.vue` (badge berteks Terbuka / Selesai / Perlu ditinjau) | kesiswaan | Kesiswaan / Tindakan Pembinaan |
| `kesiswaan/pengaturan` | discipline-settings | `discipline/SettingsView.vue` (tab Jenis & Poin / Ambang / Periode Akumulasi) | kesiswaan | Kesiswaan / Pengaturan Poin |
| `walikelas/poin-disiplin` | walikelas-discipline | `views/walikelas/DisciplineView.vue` (tabel siswa kelas: poin, aksi; catat pelanggaran; kolom "BK": badge "Dalam pendampingan" hanya bila diizinkan) | walikelas | Wali Kelas / Poin Disiplin |
| `orang-tua/kesiswaan` | parent-discipline | `views/parent/DisciplineView.vue` (tab Poin & Pembinaan (ajukan keberatan + `ApprovalTimeline`) / Persetujuan Rujukan) | orang_tua | Portal / Kesiswaan Anak |
| `pengaturan/akses-data-sensitif` | (T10) | `SensitiveAccessView.vue` + filter modul Konseling | admin, superadmin, auditor | Pengaturan / Akses Data Sensitif |

Catatan UI: form catatan konseling dan rujukan memakai textarea berlabel dengan peringatan "Catatan rahasia: hanya terlihat oleh konselor yang menangani"; form tugas memperingatkan judul terlihat oleh penanggung jawab. Tidak ada cache data konseling di `localStorage`. Tiga state di semua halaman, badge selalu berteks, dialog konfirmasi untuk tutup kasus dan batal rujukan.

## 7. Bukti handoff
- Test tabel semua fungsi §3.
- Test handler: **AC-130-01** (walikelas → `GET /counseling/cases*` 404 + log `deny`; `/students/{id}/status` → `tidak_ada_informasi` lalu `dalam_pendampingan` setelah saklar dinyalakan; konselor non-anggota 404; kepala_sekolah 404 sampai kebijakan diberikan), FR-130-003 (job pengingat sekali per sesi), **AC-131-01** (ambang 50, 45 + 10 → satu `discipline_actions` + fake notification menerima record + action untuk setiap wali; ulang dengan Idempotency-Key sama → tidak dobel), FR-131-004 (callback approved → annulled + total turun + aksi bertanda tinjau; callback ulang idempoten; core 422 diteruskan), **AC-132-01** (rujukan baru `awaiting_consent`; consent granted → `active`; declined → `declined`), FR-132-002/003 (tugas terlambat, tutup kasus diblok tanpa alasan).
- Bukti NFR-OPS-03: `notes_enc`/`reason_enc` bukan teks di DB; setiap GET catatan menambah log; grep log aplikasi tidak memuat isi catatan.
- Migrasi studentlife semua dari nol up/down 1/up.
- Playwright: kesiswaan mengatur ambang → wali kelas mencatat pelanggaran yang melewati ambang → orang tua melihat notifikasi + tindakan, mengajukan keberatan → kesiswaan menyetujui di Kotak Masuk Approval → poin turun; guru BK membuat kasus + sesi + rujukan → status menunggu persetujuan → orang tua setuju → rujukan aktif; wali kelas hanya melihat badge pendampingan setelah diizinkan.
