# T4: ATS di service `hr`: permintaan rekrutmen, lowongan & karier publik, pipeline, wawancara & scorecard, offer & hiring

**Menu PRD:** 88–92 (BR-REC-01..03, IF-HR-06, OQ-25) · **Branch:** `feat/T4-ats` · **Service:** `hr` (paket `internal/ats`) · **Migrasi:** `services/hr/migrations/0020_ats.*` (nomor 0020–0029 milik T4)
**Prasyarat:** T1, T2 di-merge (positions + salary range, headcount, employees). **Feature key:** `ats`.

## 1. Donor: ATS-TIA (stack sama chi + pgx, porting paling mudah)
| Fitur | File donor | Penyesuaian |
|---|---|---|
| Requisition + keputusan | `ATS-TIA/backend/internal/joborder/{requisition.go,requisition_test.go,validate.go}` | `company_id/tenant` + RLS → `school_id` di WHERE. `decideRequisitionHandler` **dibuang**: keputusan lewat core approval callback |
| Lowongan publik + form lamaran | `careers/careers.go`, `applicant/{applicant.go,documents.go,questions.go}` | URL publik per yayasan/sekolah: slug sekolah dari identity |
| Pipeline, tahap, perpindahan, otomasi, bulk | `pipeline/{stages.go,move.go,board.go,bulk.go,automation.go,events.go}` + test | grants/authz ATS-TIA → peran Sekoci (§4) |
| Jadwal wawancara, reschedule, undangan | `interview/{schedule.go,reschedule.go,notify.go,form.go,status.go}` + test | email lewat service notification, bukan SMTP sendiri |
| Scorecard/asesmen | `assessment/master.go`, `candidate/screening.go`, migrasi `00012_scorecard_offer.sql`, `00027_interview_template_form.sql` | bobot dalam bp |
| Offer | `candidate/{offer.go,offer_test.go,offer_flow_test.go}`, `decision/decision.go` | approval core + BR-REC-03 |
| UI (alur dan field saja) | `ATS-TIA/frontend/src/pages/{JobOrderRequest,JobOrders,JobOrderDetail,Careers,Pipeline,CandidateDetail,Schedule,InterviewForms,EmailTemplates}.tsx` | tulis ulang ke Vue + components/ui. **Jangan** bawa branding/teks BRI/TIA, peran RPO/HCBP, invoice/billing ATS-TIA |

Yang **tidak** diambil: multi-company, RPO vendor, invoice, SLA monitor, analytics manajemen. Itu YAGNI untuk sekolah.

## 2. Migrasi `0020_ats.up.sql`
```
requisitions(id, school_id, position_id FK positions, department_id NULL, quantity int CHECK(quantity>0),
  reason CHECK IN('replacement','addition'), replaces_employee_id NULL, qualifications text, budget_idr bigint NULL,
  exceeds_headcount bool NOT NULL, status CHECK IN('draft','pending_approval','approved','rejected','filled','cancelled'),
  filled_count int default 0, approval_id NULL, requested_by uuid, idempotency_key, UNIQUE(school_id,idempotency_key))
job_postings(id, school_id, requisition_id FK NOT NULL,             -- BR-REC-01 di level DB
  slug text, title, description, qualifications, employment_type, closes_on date,
  status CHECK IN('draft','open','closed'), closed_reason NULL, published_at, UNIQUE(school_id, slug))
pipeline_stages(id, school_id, code, name, sort, kind CHECK IN('normal','offer','hired','rejected'),
  notify_template_key text NULL, deleted_at)                         -- tahap bisa dikonfigurasi (FR-90-001)
candidates(id, school_id, full_name, email, email_norm, phone_enc bytea, consent_processing_at timestamptz NOT NULL,
  consent_talent_pool bool, anonymized_at NULL, UNIQUE(school_id, email_norm))
applications(id, school_id, posting_id, candidate_id, stage_id, status CHECK IN('active','rejected','withdrawn','hired'),
  cv_object_key, answers jsonb, source text, access_token_hash text,       -- tautan kandidat tanpa akun
  UNIQUE(posting_id, candidate_id))                                         -- FR-90-003: ganda → 409
application_events(id, school_id, application_id, from_stage NULL, to_stage, note, actor_user_id NULL, created_at)
application_documents(id, school_id, application_id, kind, object_key, file_name)
scorecard_templates(id, school_id, position_id NULL, name)
scorecard_criteria(id, school_id, template_id, name, weight_bp int CHECK(weight_bp>0))   -- jumlah per template = 10000 (dicek di aplikasi)
interviews(id, school_id, application_id, kind CHECK IN('interview','microteaching','test'), starts_at, ends_at,
  location_or_link, template_id, status CHECK IN('scheduled','done','cancelled','rescheduled'))
interview_panel(id, school_id, interview_id, user_id, UNIQUE(interview_id,user_id))
interview_scores(id, school_id, interview_id, panel_user_id, criterion_id, score int CHECK(score BETWEEN 0 AND 100),
  recommendation text NULL, submitted_at NULL, UNIQUE(interview_id, panel_user_id, criterion_id))
offers(id, school_id, application_id, position_id, start_date, employment_type, base_salary_enc bytea,
  out_of_range bool, status CHECK IN('draft','pending_approval','approved','sent','accepted','declined','rejected','expired'),
  approval_id, document_object_key, responded_at, response_note, employee_id NULL)
ats_settings(school_id PK, retention_months int default 6, career_page_enabled bool default false, career_intro text)
```

## 3. Fungsi murni + vektor
| Fungsi | Vektor |
|---|---|
| `ExceedsHeadcount(vacant, requested) bool` | **AC-88-01** kosong 1, minta 2 → true → kirim flag `exceeds_headcount` ke core |
| `WeightedScore(criteria, panelAverages) int` (bp, dibulatkan setengah ke atas) | **AC-91-01** 60%/40%, 80/90 → 84 |
| `VisibleScores(viewer, scores)` | **AC-91-02** belum submit → hanya miliknya |
| `SalaryOutOfRange(base, min, max)` | **AC-92-01** 5,5 jt vs 4–5 jt → true → flag `salary_out_of_range` |
| `ShouldClose(posting, today, filled)` | **AC-89-02** closes_on 31 Okt, 1 Nov → close |
| `ShouldAnonymize(candidate, lastActivity, months)` | **AC-90-02** ditolak, tanpa talent pool, > 6 bln → true |

## 4. Endpoint & peran
| Method & path | Peran | Catatan |
|---|---|---|
| `GET/POST /ats/requisitions`, `POST /{id}/submit`, `/{id}/cancel` | kepala_sekolah (ajukan), hr_manager, hr_admin, recruiter (baca) | submit → hitung formasi via data T2 → core approval `requisition` dengan flags |
| `GET/POST/PUT /ats/postings`, `POST /{id}/publish`, `/{id}/close` | hr_admin, recruiter | create tanpa requisition `approved` → 422 (AC-89-01) |
| **Publik (tanpa JWT, rate limit per IP)** `GET /public/careers/{schoolSlug}` · `GET /public/careers/{schoolSlug}/{postingSlug}` · `POST /public/careers/{schoolSlug}/{postingSlug}/apply` (multipart) | — | ditolak jika posting tidak `open`, sekolah tidak punya fitur `ats`, atau `career_page_enabled=false`. Validasi file: MIME + ukuran dari setting. Checkbox persetujuan wajib, talent pool opsional (FR-89-003). Respons berisi tautan status bertoken |
| `GET /public/applications/status?token=` · `GET/POST /public/offers/{token}` (lihat & terima/tolak) | kandidat via token | token acak 32 byte, simpan hash, kedaluwarsa |
| `GET /ats/postings/{id}/board` · `POST /ats/applications/{id}/move` `{to_stage_id, note}` · `POST /ats/applications/bulk-move` | recruiter, hr_admin, kepala_sekolah (baca + catatan) | move → event + notifikasi kandidat dengan template tahap (AC-90-01), Idempotency-Key `ats:move:<event_id>` |
| `CRUD /ats/stages`, `/ats/scorecard-templates` | hr_admin | |
| `POST /ats/interviews` · `/{id}/reschedule` · `/{id}/cancel` | recruiter | undangan ke kandidat (email) + panel (in-app/email) |
| `GET /ats/interviews/mine` · `PUT /ats/interviews/{id}/scores` · `POST /{id}/submit` | anggota panel (siapa pun yang ditugaskan) | otorisasi = ada di `interview_panel`, bukan peran |
| `GET /ats/applications/{id}/summary` | recruiter, hr_admin, kepala_sekolah | skor tertimbang + rekomendasi (FR-91-004) |
| `POST /ats/offers` · `/{id}/submit` · `/{id}/send` | hr_admin (buat), approval hr_manager | send hanya jika `approved`. Dokumen offer PDF digenerate (gofpdf seperti slip T3) |
| Accept (publik) | kandidat | dalam satu transaksi: offer `accepted` → **buat employee status `candidate`** di tabel T2 (panggil store T2, satu service), `requisition.filled_count++` lalu `filled` jika kuota terpenuhi, posting `closed`. Emit hook onboarding (T5: fungsi `onboarding.StartForEmployee` dipanggil jika paket T5 ada. Sampai T5 merge, siapkan interface kecil satu-fungsi yang nil-safe) (AC-92-02) |
| `CRUD /ats/settings` · `GET /ats/talent-pool` | hr_admin | |

**Job:** tutup posting kedaluwarsa (AC-89-02), anonimisasi kandidat (AC-90-02): nama → "Kandidat terhapus", email/telepon/file dihapus dari storage, dan diaudit.

## 5. Frontend
Admin: `router/domains/ats.routes.ts`, `meta.feature: 'ats'`, navGroup **Rekrutmen**.
| Path | View (`views/ats/…`) | Peran | Label |
|---|---|---|---|
| `rekrutmen/permintaan` | `RequisitionsView.vue` (+ dialog form, badge "Melebihi formasi", ApprovalTimeline) | kepala_sekolah, hr_admin, hr_manager | Permintaan Rekrutmen |
| `rekrutmen/lowongan` · `/:id` | `PostingsView.vue`, `PostingDetailView.vue` (tab Detail / Pelamar (board) / Wawancara; tombol salin tautan bagikan) | hr_admin, recruiter | Lowongan |
| `rekrutmen/kandidat/:id` | `ApplicationDetailView.vue` (profil, CV viewer, timeline event, skor, offer) | hr_admin, recruiter, kepala_sekolah | — |
| `rekrutmen/wawancara` | `MyInterviewsView.vue` (jadwal saya + form scorecard) | semua yang punya panel (tampil di nav jika `/ats/interviews/mine` tidak kosong) | Wawancara Saya |
| `rekrutmen/pengaturan` | `AtsSettingsView.vue` (tab Tahap / Scorecard / Template Email (template_key notification) / Halaman Karier / Retensi) | hr_admin | Pengaturan Rekrutmen |
| `rekrutmen/talent-pool` | `TalentPoolView.vue` | hr_admin, recruiter | Talent Pool |

**Board pipeline:** kolom per tahap, kartu kandidat. Pindah lewat menu aksi "Pindah ke…" (dialog + catatan). Drag-and-drop **opsional** dan harus punya alternatif keyboard. Di mobile, tampilkan list dikelompokkan per tahap.

Publik (layout tanpa sidebar, tanpa login, route `meta.public: true` mengikuti route publik PPDB yang sudah ada): `karier/:schoolSlug` (daftar lowongan), `karier/:schoolSlug/:postingSlug` (detail + form lamaran: data diri, unggah CV dengan progress, pertanyaan, 2 checkbox persetujuan), `karier/status` (token), `karier/offer` (token: lihat offer PDF, Terima/Tolak dengan konfirmasi). Halaman publik harus ringan, responsif di 375px, dan punya meta title.

## 6. Bukti handoff
Vektor §3, AC-88-01, 89-01/02, 90-01/02, 91-01/02, 92-01/02. Test port dari donor tetap hijau (sebutkan file mana yang dipindah). Test publik: lamaran ganda 409, posting tertutup 404/410, token salah 404. Playwright alur penuh: requisition → approval → lowongan → apply publik → board → wawancara + skor 2 panel → offer → approval → kandidat terima → pegawai `candidate` muncul di Master Pegawai.
