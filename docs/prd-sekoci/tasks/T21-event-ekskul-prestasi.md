# T21: Event, ekstrakurikuler & prestasi di service `studentlife`

**Menu PRD:** 133, 134, 135, 136 (BR-EVT-01, BR-GEN-09, lampiran "Anggaran", "Penerbitan tagihan non-SPP", "Prestasi siswa", "Ruang") · **Gelombang:** 4 · **Branch:** `feat/T21-event-ekskul-prestasi` · **Service:** `studentlife` (paket `internal/activity`) · **Migrasi:** `services/studentlife/migrations/0020_activity.*` (nomor 0020–0029 milik T21)
**Prasyarat:** gelombang 3 di-merge: T10 (kerangka `studentlife`, paket `consent`, endpoint wali identity), T9 (peran, feature key, master ruang & booking menu 116–117). Kontrak tagihan ad-hoc billing = **BILL-ADHOC milik T12** (billing 0007, gelombang 3, sudah di-merge; detail di T18 §4.3). Cek anggaran = hook nil-safe ke T17 (gelombang 5). **Feature key (final T9 §2):** `event` (133–134), `extracurricular` (135), `achievement` (136). **Peran:** `event_coordinator`, `pembina_ekskul`, `kesiswaan`, + `kepala_sekolah`, `orang_tua`, `siswa`. Nama final **mengikuti T9 §2**.

## 1. Acuan kode
- Kerangka `services/studentlife` dari T10 (pola `services/billing`). Paket `internal/activity/` (`model.go` fungsi murni, `store.go`, `handler.go`, `jobs.go`).
- Approval: core `POST /internal/approvals` (kontrak T1 §3, lihat `services/core/internal/handler/internal.go`), callback `POST /internal/approval-callbacks` (payload `approval_id, source_id, request_type, decision, …` dari `services/core/internal/jobs/effects.go`). Tambahkan **satu** konstanta `ReqTypeEvent = "event"` (label "Kegiatan/Event") ke `services/core/internal/model/domain.go` (`RequestTypeLabels` + `KnownRequestTypes`, penambahan baris) kecuali T9 sudah mendaftarkan request type Bagian 3–4.
- Akses orang tua → anak: identity `GET /internal/guardians/{gid}/students/{sid}/access` dan `/internal/guardians/{gid}/children` (sudah ada). Siswa → wali: identity `GET /internal/students/{id}/guardians` (**milik T10 §4.3**, jangan buat ulang).
- Persetujuan orang tua digital: paket generik `internal/consent` + tabel `parent_consents` dari **T10** (`subject_type` baru `'event_registration'`), endpoint wali generik `GET /consents/my` · `POST /consents/{id}/decide`. T21 mendaftarkan di `main.go`: `event_registration` → feature `activities`, hook `consent.Describe` (nama event, tanggal, biaya, `consent_text`) dan `consent.OnDecided` (pindah status registrasi dalam transaksi yang sama). Konstanta peran/feature di `internal/common/roles.go` (T10).
- Target siswa per jenjang/kelas: academic `GET /classrooms?school_id=`, `GET /classrooms/{id}/students` (pola `services/billing/internal/client/client.go`).
- Notifikasi: notification `POST /internal/notifications` (Idempotency-Key). Audit: core `/internal/audit`. Lampiran (bukti prestasi): modul `internal/storage` di `studentlife` (dari T10; jika belum ada, salin dari billing).
- Dashboard 51–53: `web/src/views/executive/YayasanDashboardView.vue`, `web/src/views/identity/admin/DashboardView.vue` (data dari identity `fetchDashboard`). T21 hanya menambah kartu, tidak mengubah endpoint identity.

## 2. Migrasi `0020_activity.up.sql`
```
act_event_kinds(id, school_id, name, deleted_at)                                                  -- jenis event = master, bukan enum karangan
act_events(id, school_id, name, kind_id, description, starts_at, ends_at,
  room_id uuid NULL, booking_id uuid NULL, location_text NULL,     -- room_id → booking T9; location_text untuk lokasi luar sekolah
  target jsonb NOT NULL,           -- {grade_levels:[], classroom_ids:[], include_staff:bool}  (FR-133-001)
  quota int NULL, fee_idr bigint NOT NULL default 0 CHECK(fee_idr >= 0),
  requires_parent_consent bool default false, consent_text text NULL,
  budget_line_id uuid NULL, budget_amount_idr bigint NULL,
  registration_opens_at, registration_closes_at,
  status CHECK IN('draft','pending_approval','approved','rejected','published','cancelled','completed'),
  approval_id NULL, published_at NULL, created_by, deleted_at, idempotency_key, UNIQUE(school_id, idempotency_key))
act_event_committee(event_id, user_id, school_id, role_label text, PRIMARY KEY(event_id, user_id))
act_event_registrations(id, school_id, event_id, student_user_id, registered_by, registered_via CHECK IN('siswa','orang_tua','event_coordinator'),
  status CHECK IN('pending_consent','confirmed','declined','cancelled'),
  consent_id uuid NULL REFERENCES parent_consents(id),     -- T10; isi teks persetujuan dibekukan saat consent dibuat (consent_text_snapshot)
  consent_text_snapshot NULL,
  bill_id uuid NULL, attended_at NULL, attendance_by NULL, created_at,
  UNIQUE(event_id, student_user_id) WHERE status <> 'cancelled')
act_ecc_periods(id, school_id, academic_year_id, name, opens_at, closes_at)                       -- periode pendaftaran ekskul
act_extracurriculars(id, school_id, name, description, schedule jsonb,   -- [{weekday,start,end}]
  room_id uuid NULL, location_text NULL, quota int, fee_idr bigint default 0,
  full_policy CHECK IN('reject','waitlist') NOT NULL default 'reject',    -- AC-135-01 "sesuai pengaturan", default konservatif
  active bool, deleted_at)
act_ecc_coaches(ecc_id, user_id, school_id, PRIMARY KEY(ecc_id, user_id))
act_ecc_members(id, school_id, ecc_id, period_id, student_user_id, status CHECK IN('active','waitlisted','left','rejected'),
  waitlisted_at NULL, joined_at NULL, left_at NULL, bill_id uuid NULL, UNIQUE(ecc_id, period_id, student_user_id))
act_ecc_sessions(id, school_id, ecc_id, held_on date, note, created_by, UNIQUE(ecc_id, held_on))
act_ecc_attendance(session_id, member_id, school_id, status CHECK IN('hadir','izin','sakit','alpa'), PRIMARY KEY(session_id, member_id))
act_ecc_assessments(id, school_id, ecc_id, period_id, member_id, predicate text, description text, assessed_by, UNIQUE(member_id, period_id))
act_achievements(id, school_id, student_user_id, activity_name, organizer NULL,
  level CHECK IN('sekolah','kecamatan','kabupaten_kota','provinsi','nasional','internasional'),
  rank_label text, achieved_on date, evidence jsonb default '[]', event_id NULL, ecc_id NULL,
  submitted_by, submitted_via CHECK IN('orang_tua','siswa','pembina_ekskul','kesiswaan'),
  status CHECK IN('pending','verified','rejected'), verified_by NULL, verified_at NULL, reject_reason NULL,
  created_at, deleted_at, CHECK(verified_by IS NULL OR verified_by <> submitted_by))               -- BR-GEN-09
```
Ekskul berbiaya ditagih **sekali per periode** saat keanggotaan aktif (PRD tidak menyebut tagihan bulanan) → asumsi, catat di handoff. Predikat penilaian = teks bebas pembina (PRD tidak menetapkan skala).

## 3. Fungsi murni (`internal/activity/model.go`) + vektor
| Fungsi | Vektor (→ AC/FR) |
|---|---|
| `NeedsApproval(event)` | `fee_idr > 0` atau `budget_line_id` terisi → true (FR-133-003); event gratis tanpa anggaran → langsung bisa dipublikasi |
| `ApprovalFlags(event, budgetResult)` | `has_fee`, `uses_budget`, `over_budget` (BR-BUD-02 dari hasil hook) |
| `InTarget(event, studentGrade, classroomID)` | siswa di luar sasaran → tidak bisa mendaftar (FR-134-001) |
| `EventSeatDecision(quota, taken)` | `taken` = `confirmed` + `pending_consent`; kuota 30, terisi 30 → tolak "Kuota penuh"; `quota NULL` → bebas |
| `RegistrationStatus(event, via, consentGiven)` | event butuh persetujuan, didaftar siswa → `pending_consent`; didaftar orang tua dengan centang persetujuan → `confirmed`; tidak butuh persetujuan → `confirmed` (BR-EVT-01) |
| `CanIssueEventBill(reg, event)` | **AC-134-01**: fee 750.000, `confirmed` → true, nominal 750.000; `pending_consent`/`declined` → false (BR-EVT-01) |
| `CanMarkAttendance(reg, event, today)` | hanya `confirmed`, hanya pada tanggal event (`Asia/Jakarta`); `pending_consent` → false (BR-EVT-01, FR-134-004) |
| `EccJoinDecision(quota, active, policy)` | **AC-135-01**: kuota 20, aktif 20, `reject` → tolak; `waitlist` → `waitlisted` |
| `PromoteWaitlist(members)` | anggota keluar → `waitlisted` tertua menjadi `active` (lalu tagihan bila berbiaya) |
| `InPeriod(period, now)` | pendaftaran ekskul di luar periode → tolak (FR-135-002) |
| `VisibleAchievement(a)` | **AC-136-01**: `pending` → tidak tampil di portal & dashboard; hanya `verified` (FR-136-002) |
| `CanVerify(a, userID)` | verifikator ≠ pengaju (BR-GEN-09) |
| `AchievementSummary(list)` | jumlah per tingkat & per bulan dari prestasi `verified` saja (FR-136-003) |

## 4. Integrasi & job
### 4.1 Lokasi event lewat booking T9 (FR-133-002, AC-133-01)
Memakai kontrak booking **T9 §4/§5** (menu 117). Ekspektasi minimum (nama field final mengikuti T9):
```
POST {OPS_URL}/internal/bookings     Idempotency-Key: event:<event_id>:<starts_at>
{ school_id, room_id, starts_at, ends_at, participants, purpose, source_service:"studentlife", source_type:"event", source_id, requested_by }
→ 201 { id, status }  |  409 { conflict:true, nearest_free_slots:[…] }  |  422 (melebihi kapasitas, BR-FAC-01)
POST {OPS_URL}/internal/bookings/{id}/cancel  { school_id, reason }
```
Simpan event: buat UUID event → panggil booking (`source_id` = UUID itu) → 409/422 → **tidak ada insert**, respons 409 "Lokasi sudah dipesan pada waktu tersebut" + slot terdekat (event tidak tersimpan dengan lokasi itu). Booking sukses → insert event + `booking_id`; insert gagal → cancel booking (kompensasi). Ubah waktu/ruang = cancel booking lama + booking baru. Event dibatalkan → cancel booking. `participants` = `quota` (atau jumlah sasaran bila kuota kosong). Pemilihan ruang memakai `GET {OPS_URL}/rooms?bookable=true` (JWT, T9). Ekskul memakai `room_id` sebagai informasi (jadwal tetap, tanpa booking per sesi).

### 4.2 Approval & cek anggaran (FR-133-003, lampiran "Anggaran")
- Hook nil-safe di `main.go`: `activity.Budget` diisi klien HTTP hanya bila env `BUDGET_CHECK_URL` di-set (default kosong → `nil` → cek dilewati, field anggaran disembunyikan di UI kecuali feature anggaran T17 aktif). Kontrak untuk T17 (gelombang 5):
```
POST {BUDGET_CHECK_URL}/internal/budget/check   { school_id, budget_line_id, amount_idr, source_service:"studentlife", source_type:"event", source_id }
→ 200 { available_idr, over_budget:bool, policy:"extra_approval"|"reject" }      (BR-BUD-02)
```
  `over_budget` + `reject` → 422 "Melebihi sisa anggaran"; `extra_approval` → flag `over_budget` ke approval.
- `POST /activities/events/{id}/submit`: `NeedsApproval` → core `POST /internal/approvals` `{request_type:"event", source_service:"studentlife", source_id, amount_idr: budget_amount_idr ?? fee_idr, flags, maker_user_id, callback_url: {STUDENTLIFE_URL}/internal/approval-callbacks, idempotency_key:"event:<id>"}` → `pending_approval`. Callback `approved` → `approved`, `rejected` → `rejected` (booking dibatalkan). Idempoten per `approval_id`. Penyetuju diatur di Approval Center (tidak ada tabel approval di studentlife).

### 4.3 Publikasi (FR-133-004)
`POST /activities/events/{id}/publish` (status `approved`, atau `draft` bila tidak butuh approval) → `published`, tampil di kalender kegiatan dan portal siswa/orang tua sesuai `target`; notifikasi ke siswa sasaran (key `event:<id>:published:<user>`). Tidak ada kalender sekolah terpusat di repo → kalender = view kegiatan milik T21.

### 4.4 Tagihan lewat billing (FR-134-003, FR-135-002, AC-134-01)
Kontrak **BILL-ADHOC T18 §4.3**. Event: saat registrasi menjadi `confirmed` dan `fee_idr > 0` → `POST /internal/bills/adhoc` `{source_module:"event", source_ref:<registration_id>, items:[{name:<nama event>, component_code:"event_fee", amount_idr:fee_idr}]}`, `bill_id` disimpan. Pembatalan sebelum bayar → `void`. Ekskul: anggota `active` + `fee_idr > 0` → `source_module:"extracurricular"`, `source_ref:<member_id>`, `component_code:"extracurricular_fee"`. Gagal → registrasi tetap, exception core (`dead_letter`, retry `POST /internal/activities/bills/{kind}/{id}`).

### 4.5 Persetujuan orang tua (FR-134-002, BR-EVT-01)
Siswa mendaftar event `requires_parent_consent` → registrasi `pending_consent` + `parent_consents(subject_type='event_registration', subject_id=<registration_id>, status='pending')` → notifikasi ke setiap wali aktif (identity `GET /internal/students/{id}/guardians`, T10 §4.3), key `studentlife:consent:<consent_id>:<guardian_id>`. Wali memutuskan lewat `POST /consents/{id}/decide` (T10) → hook `OnDecided`: `granted` → `confirmed` → tagihan (§4.4); `declined` → `declined` (kursi dilepas). Orang tua yang mendaftarkan sendiri dengan centang persetujuan: registrasi + consent dibuat lalu `consent.Decide(granted)` oleh wali yang sama dalam satu transaksi. Dua wali: keputusan pertama berlaku (aturan T10).

### 4.6 Data prestasi untuk dashboard 51–53 (FR-136-003)
`GET /activities/achievements/summary?school_id=&from=&to=` (JWT) dan `GET /internal/achievements/summary?school_ids=&from=&to=` (untuk report/insight T24) → hanya `verified`: total, per tingkat, 5 terbaru (nama siswa, kegiatan, tingkat, peringkat). Data `pending` tidak pernah ikut (AC-136-01).

### 4.7 Job harian `activity_daily`
Registrasi `pending_consent` lewat `registration_closes_at` → `cancelled` (kursi dilepas); event lewat `ends_at` → `completed`.

## 5. Endpoint (studentlife, JWT, scope `school_id`, feature per menu `event`/`extracurricular`/`achievement` → 403 bila mati)
| Method & path | Peran |
|---|---|
| `CRUD /activities/event-kinds` | event_coordinator, admin |
| `GET /activities/events?status=&from=&to=` · `POST` · `PUT /{id}` · `DELETE /{id}` (soft, draft saja) · `POST /{id}/submit` · `POST /{id}/publish` · `POST /{id}/cancel` | event_coordinator (event miliknya/panitia), kepala_sekolah (lihat) |
| `GET /activities/events/{id}/registrations` · `POST /activities/events/{id}/registrations` (daftarkan siswa) · `POST /activities/registrations/{id}/attendance` | event_coordinator, panitia event |
| `GET /activities/calendar?from=&to=` | semua peran sekolah (hanya event `published` sesuai sasaran untuk siswa/orang tua) |
| `GET /me/activities/events` · `POST /me/activities/events/{id}/register` · `DELETE /me/activities/registrations/{id}` | siswa (dirinya) |
| `GET /parent/activities/events?student_id=` · `POST /parent/activities/events/{id}/register {student_id, consent:true}` (keputusan persetujuan atas pendaftaran siswa lewat `POST /consents/{id}/decide` generik T10) | orang_tua (anak terverifikasi) |
| `CRUD /activities/extracurriculars` (+coaches) · `CRUD /activities/ecc-periods` | kesiswaan, admin |
| `GET /activities/extracurriculars/{id}/members` · `POST …/members/{mid}/status` · `POST …/sessions` + absensi · `PUT …/assessments` | pembina_ekskul (ekskul yang dibina), kesiswaan |
| `GET /me/activities/extracurriculars` · `POST /me/activities/extracurriculars/{id}/join` · `POST …/leave` | siswa |
| `GET /parent/activities/extracurriculars?student_id=` · `POST …/{id}/join {student_id}` | orang_tua |
| `GET /activities/achievements?status=` · `POST` (multipart bukti) · `POST /{id}/verify` · `POST /{id}/reject {reason}` | kesiswaan (verifikasi), pembina_ekskul (ajukan) |
| `GET /parent/activities/achievements?student_id=` · `POST /parent/activities/achievements` (multipart) · `GET /me/activities/achievements` · `POST /me/activities/achievements` | orang_tua, siswa (pengajuan → `pending`; daftar menampilkan status berteks) |
| `GET /activities/achievements/summary` | kepala_sekolah, admin, kepala_yayasan, superadmin |
| `POST /internal/approval-callbacks` · `GET /internal/achievements/summary` · `POST /internal/activities/bills/{kind}/{id}` | internal |

## 6. Frontend
`meta.feature` per menu: `'event'` (133–134), `'extracurricular'` (135), `'achievement'` (136), navGroup **Kegiatan** (staf) dan **Kegiatan** di portal. Route `web/src/router/domains/activity.routes.ts` (daftarkan di `router/index.ts`, tambah baris). Service `web/src/services/activity/` memanggil `api('studentlife')`.
| Path | View (`views/activity/…`) | Peran | Label |
|---|---|---|---|
| `kegiatan/event` · `/baru` · `/:id` | `EventsView.vue`, `EventFormView.vue` (pemilih ruang T9 + pesan bentrok & slot terdekat, sasaran, kuota, biaya, persetujuan ortu + teks, pos anggaran bila tersedia, panitia), `EventDetailView.vue` (`ApprovalTimeline` T1, peserta + status persetujuan/tagihan berteks, absensi hari-H) | event_coordinator, kepala_sekolah | Event |
| `kegiatan/kalender` | `CalendarView.vue` (bulan/agenda, responsif) | semua peran sekolah | Kalender Kegiatan |
| `kegiatan/ekskul` · `/:id` | `ExtracurricularsView.vue`, `ExtracurricularDetailView.vue` (tab Anggota & Daftar Tunggu / Sesi & Kehadiran / Penilaian) | kesiswaan, pembina_ekskul | Ekstrakurikuler |
| `kegiatan/prestasi` | `AchievementsView.vue` (antrean verifikasi, pratinjau bukti, verifikasi/tolak dengan alasan) | kesiswaan, pembina_ekskul | Prestasi Siswa |
| `orang-tua/kegiatan` · `/:id` | `parent/ParentActivitiesView.vue` (tab Event / Ekskul / Prestasi, pilih anak; dialog persetujuan digital menampilkan teks persetujuan + biaya, memakai `services/studentlife/consent.ts` milik T10), `parent/ParentEventDetailView.vue` | orang_tua | Kegiatan Anak |
| `siswa/kegiatan` | `student/StudentActivitiesView.vue` (event sasaran, ekskul, prestasi saya + status "Menunggu verifikasi") | siswa | Kegiatan |
Kartu `components/activity/AchievementSummaryCard.vue` ditambahkan ke `YayasanDashboardView.vue` dan `identity/admin/DashboardView.vue` (tampil hanya bila feature `achievement` aktif; "Diperbarui: …"). Tiga state + aksesibilitas sesuai GEMINI §5.

## 7. Bukti handoff
- Vektor §3 (test tabel) termasuk batas kuota (n−1, n), tanggal event vs hari ini, maker-checker prestasi.
- **AC-133-01**: fake booking T9 mengembalikan 409 → event tidak tersimpan (cek tabel kosong), respons berisi slot terdekat.
- **AC-134-01**: event Rp750.000 butuh persetujuan → siswa daftar (`pending_consent`, fake billing **tidak** dipanggil, absensi ditolak) → orang tua setuju → fake billing menerima `adhoc` 750.000 dengan Idempotency-Key `event:<registration_id>`; callback/aksi diulang → satu tagihan.
- **AC-135-01**: kuota 20 terisi 20 → `reject` 422, `waitlist` → `waitlisted`; anggota keluar → promosi + tagihan.
- **AC-136-01**: prestasi `pending` tidak ada di `/summary`; setelah verifikasi oleh user lain → ada; pengaju memverifikasi sendiri → 403.
- FR-133-003: event berbiaya → fake core menerima `request_type:"event"`; callback `rejected` → booking dibatalkan. Hook anggaran `nil` → dilewati; fake `over_budget+reject` → 422.
- FR-133-001..004, FR-134-001..004, FR-135-001..003, FR-136-001..003: tabel cakupan di handoff. Test otorisasi: orang tua anak lain → 404, pembina ekskul lain → 403, siswa di luar sasaran → 422, feature mati → 403.
- Playwright: coordinator buat study tour di aula yang sudah dibooking → ditolak → pilih ruang/tanggal lain → ajukan → kepala sekolah setujui → publikasi → siswa daftar → orang tua setujui di portal → tagihan Rp750.000 muncul di tagihan orang tua → absensi hari-H; orang tua unggah sertifikat juara 2 → belum tampil di dashboard → kesiswaan verifikasi → tampil di kartu prestasi dashboard yayasan.
