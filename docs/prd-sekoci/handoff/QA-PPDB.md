# Handoff QA-PPDB — perbaikan 44 temuan PPDB (menu 8–16)

Branch `feat/QA-PPDB` (worktree `skci-teratai-wt/QA-PPDB`), basis `b984872`. Cakupan: service `services/admission` + view/API PPDB di `web/` (admin, kepala sekolah/yayasan, portal calon orang tua). Migrasi milik area ini: `0005_qa_ppdb`, `0006_qa_ppdb_round2`.

Commit (urut):

| Commit | Isi |
|---|---|
| 2191c61 | migrasi 0005 + aturan murni PPDB (syarat diskon, cicilan per pendaftar, tenggat, kuota, unggahan) |
| 1f79502 | tagihan bertahap per pendaftar, promo bersyarat, kunci kuota, gating dokumen, notifikasi, magic bytes |
| 0e7d65b | approval hasil seleksi via core, pengumuman terjadwal, naik cadangan, approval diskon khusus, job gugur |
| 92fae08 | tenggat & cek dampak gelombang, kelayakan/kuota jalur, masa promo, masking NIK, jadwal tes, funnel dashboard |
| 50c6d53 | bukti pendaftaran PDF; compose menyambungkan core approval + SMTP |
| 5dce814 | putaran 2: form builder, jenjang gelombang, refund, sesi SEKOCI |
| 3e6ea2c | putaran 2: scope formula, ranking terakhir, bukti bayar admin, filter antrean, opsi dashboard, refund di detail, status CBT, pesan galat Indonesia |
| 2347281 | web admin: semua view tersambung ke endpoint nyata |
| 059ac0a | web portal orang tua: field dinamis, jadwal, PDF, tagihan per tahap, refund, notifikasi, login SEKOCI, keluar |

## Status per temuan

Semua 44 temuan berstatus **FIXED**. Tidak ada DEFERRED atau PARTIAL. Beberapa butuh langkah konfigurasi server (lihat bagian "Langkah server") karena datanya ada di server atau butuh kredensial pihak luar. Kodenya sudah lengkap.

| BUG | Status | File utama | Bukti (test) |
|---|---|---|---|
| 003 cicilan bocor antar pendaftar | FIXED | model/ppdb.go (PlanInstallments, SettleInstallments), handler/charges.go, 0005 `charge_installments` | TestInstallmentsArePerApplicant, TestSettleInstallmentsPerApplicant, TestReregistrationMinimumBoundary |
| 011 diskon tanpa syarat/duplikat | FIXED (+data) | model/ppdb.go DiscountConditions, handler/charges.go, financeconfig.go (tolak promo aktif bernama sama) | TestChargeDiscountsConditionalAndNotDuplicated, TestDiscountConditions |
| 030 dashboard Yayasan/funnel | FIXED | common/http.go DashboardAuth, handler/dashboard.go (funnel, tren, per gelombang/jalur + kuota, keuangan, opsi filter), DashboardView.vue, route role | TestDashboardFunnel, TestBuildFunnel, TestDashboardOptionsForLeaders |
| 031 kuota tidak ditegakkan | FIXED | handler/application.go (lock jalur FOR UPDATE), model QuotaAndWaitlist | TestSubmitEnforcesTrackQuota, TestQuotaAndWaitlist |
| 032 diskon khusus tanpa approval | FIXED (+config core) | handler/approvals.go (`discount_special` via core), ApplicationDetailView | TestCreateApproval, TestSelectionApprovalAnnouncementAndWaitlist |
| 033 siap tes walau dokumen ditolak | FIXED | handler/application.go MarkEligible + model DocumentIssue | TestDocumentIssue, TestNewApplicationChecksAndNumbers |
| 034 hasil seleksi tanpa approval | FIXED (+config core) | handler/approvals.go (`selection_result`), decision.go, SelectionView tab Persetujuan | TestSelectionApprovalAnnouncementAndWaitlist |
| 035 naik cadangan | FIXED | handler/decision.go PromoteWaitlist, 0005 `waitlist_promotions`, SelectionView tab Cadangan | TestSelectionApprovalAnnouncementAndWaitlist |
| 036 hasil terlihat sebelum jadwal | FIXED | handler/announcements.go + jobs.go (publishDueAnnouncements), GetDecision hanya keputusan `published` | TestSelectionApprovalAnnouncementAndWaitlist |
| 037 tidak ada notifikasi | FIXED (+SMTP) | handler/notifications.go (`guardian_notifications` + email), portal Ringkasan menampilkan & menandai dibaca | TestRefundRecordAndProcess, TestDropUnpaidForms |
| 038 tagihan daftar ulang tidak terbit | FIXED | handler/charges.go issueCharge stage `reregistration` saat offered | TestInstallmentsArePerApplicant |
| 039 minimum & tenggat daftar ulang | FIXED | model ReregistrationMinimum/Deadline, conversion.go gating | TestReregistrationMinimumBoundary, TestReregistrationDeadline |
| 040 gugur & refund daftar ulang | FIXED | jobs.go dropIfOverdue, refunds.go (catat/proses/bebaskan), PaymentReviewView tab Pengembalian dana, portal tab Biaya | TestRefundRecordAndProcess, TestApplicationDetailRefunds, TestDeadlineDrops |
| 041 gugur formulir tidak dibayar | FIXED | jobs.go (payment_end), tagihan dibatalkan + notifikasi | TestDropUnpaidForms |
| 042 unggahan hanya Content-Type | FIXED | model DetectUpload (whitelist ekstensi + magic bytes), handler/uploads.go | TestDetectUpload |
| 091 biaya per gelombang/unit | FIXED | financeconfig.go (wave_id, entrance_fee), FinanceConfigView | TestValidateFeeScheme |
| 092 masa berlaku promo/voucher | FIXED | promos.go + model PromoActive (valid_from/until, kuota, komponen), voucher kedaluwarsa ditolak | TestPromoActiveBoundary, TestPromoQuotaAndWindow |
| 093 gelombang per jenjang | FIXED (+data) | 0006 waves.education_level/entry_grade, waves.go, ConfigView, discovery | TestWaveWarningsPerLevel, TestValidateWaveLevel |
| 094 jalur per gelombang + kelayakan | FIXED | admin.go track eligibility_config & wave_ids, model Eligibility | TestEligibility, TestEligibilityIssues |
| 095 field formulir dapat diatur | FIXED | 0006 `admission_form_fields`, formfields.go, ConfigView tab Field formulir, ApplicantFormView merender field dinamis | TestFormFieldsBlockSubmit, TestValidateFormField, TestNormalizeCustomValues, TestMissingFormFields |
| 096 dokumen wajib per jalur & tahap | FIXED | admin.go requirements track_id + stage | TestDocumentUploadAllowed |
| 097 BR-PPDB-01/02 | FIXED | model ApplyDiscounts (snapshot, cap, floor) + masa promo | TestApplyDiscountsCapAndFloor, TestApplyDiscountPolicy* |
| 098 batas bayar & daftar ulang | FIXED | waves.go payment_end/reregistration_end/waitlist days | TestValidateWaveSchedule |
| 099 HTTP 500 + bocor SQL | FIXED | helpers.go writeErr (pemetaan constraint → 422 Indonesia), validasi kuota | TestErrorResponse |
| 100 tipe dokumen berbahaya | FIXED | admin.go requirement accepted_types whitelist (PDF/JPG/PNG) | TestDocumentUploadAllowed |
| 101 ekspor Excel | FIXED | WorkQueueView exportExcel (semua halaman, filter aktif) | build web |
| 102 riwayat aktivitas | FIXED | audit tertaut ke application_id + actor_name, detail memuat `activity` | TestActorLabel |
| 103 data orang tua / status draft | FIXED | admin.go detail (guardian, program, status tahap), dokumen draft tidak bisa direview | TestNIKAndScore |
| 104 notifikasi verifikasi dokumen | FIXED | documents.go ReviewDocument → notify | TestDocumentIssue |
| 105 jadwal tes/wawancara | FIXED | 0005 `selection_schedules`, schedules.go, detail admin + portal (tab Status) | TestValidateSchedule |
| 106 rentang nilai | FIXED | ranking.go UpsertScore (0..max_score), CHECK score>=0 | TestNIKAndScore |
| 107 verifikasi menunggu bayar formulir | FIXED | documents.go/application.go gating minimum_paid formulir | TestNewApplicationChecksAndNumbers |
| 108 akun orang tua SEKOCI | FIXED | sekoci_session.go POST /public/session/sekoci (JWT identity peran orang_tua → sesi wali, tertaut identity_user_id), tombol di OtpView | TestSekociSessionExchange |
| 109 BR-PPDB-06 | FIXED | charges.go tagihan formulir saat submit dengan diskon/cicilan benar | TestChargeDiscountsConditionalAndNotDuplicated |
| 110 syarat usia/kelayakan | FIXED | model Eligibility, application.go | TestEligibility |
| 121 NIK tanpa masking | FIXED | admin.go MaskNIK di detail/list | TestNIKAndScore |
| 155 gelombang tumpang tindih | FIXED | waves.go peringatan overlap per jenjang | TestWaveWarningsPerLevel |
| 156 ubah gelombang tanpa cek dampak | FIXED | waves.go WaveScheduleChanged + konfirmasi `confirm_impact` | TestWaveScheduleChanged |
| 157 filter status pembayaran | FIXED | admin.go `payment_status`, WorkQueueView | TestPaymentStatusOf |
| 158 preview dokumen | FIXED | DocumentReviewView pratinjau blob + penanganan file rusak | build web |
| 159 optimistic lock review | FIXED | documents.go lock_version → 409 | TestDocumentIssue |
| 160 versi dokumen tertimpa | FIXED | 0005 `application_document_versions`, riwayat versi di review | TestDocumentUploadAllowed |
| 161 formula per jalur | FIXED | ranking.go CreateFormula wajib wave+track satu periode, ListFormulas memfilter, SelectionView | TestFormulaRequiresScope |
| 162 bukti PDF + nomor per unit/gelombang | FIXED | proof.go + simplepdf, `registration_counters` | TestRegistrationProof, TestRegistrationNumber, TestRenderStructure |

## Migrasi

- `0005_qa_ppdb` mencakup:
  - cicilan per tagihan (`charge_installments`) dan remap alokasi lama;
  - origin diskon, stage voucher/diskon manual + approval_id, kuota promo, discount_mode;
  - bookkeeping gugur (dropped_at, drop_reason), tenggat gelombang, `registration_counters`;
  - versi dokumen + lock_version;
  - `selection_approvals`, status keputusan (draft/pending_approval/approved/published/superseded), pengumuman;
  - `waitlist_promotions`, `approval_callback_events`, `guardian_notifications`, `selection_schedules`;
  - CHECK score>=0.
- `0006_qa_ppdb_round2` mencakup:
  - `admission_form_fields` dan `applicants.custom_fields`;
  - `admission_waves.education_level/entry_grade`;
  - `admission_periods.refund_policy_bp/refund_admin_fee_idr`;
  - `admission_refunds` (UNIQUE application+stage, sehingga idempoten).
- Diverifikasi up → down 1 → up di DB uji `skci_qa_ppdb_admission` (versi akhir 6).

## Endpoint baru/berubah (admission)

Publik/wali:

- `POST /public/session/sekoci` (JWT identity, peran orang_tua).
- `GET /public/discovery`, sekarang dengan field-field berikut:
  - `form_fields`;
  - `period_id`/`status`/`education_level`/`entry_grade` pada gelombang;
  - `period_id` pada jalur.
- `GET /guardian/notifications`, `POST /guardian/notifications/{id}/read`.
- `GET /guardian/applications/{id}/schedules`, `GET /guardian/applications/{id}/proof.pdf`.
- `GET /guardian/applications/{id}/charge?stage=registration|reregistration`, dengan field `stage_label`, `min_payment`, `due_at`, `payable`, `blocked_reason`.
- `GET /guardian/applications/{id}`, dengan field `form_answers`, `refunds`, `drop_reason`.
- `GET /guardian/applications/{id}/cbt`, dengan field `configured`.

Admin (admin/superadmin):

- Konfigurasi: `/admin/config/form-fields` (GET/POST/PUT).
- Detail pendaftar:
  - `/admin/applications/{id}/special-discounts` (GET/POST → core);
  - `/admin/applications/{id}/schedules` (GET/POST);
  - `/admin/applications/{id}/proof.pdf`.
- Dokumen: `/admin/documents/{id}/versions`, `/admin/documents/{id}/download?version=`.
- Pembayaran dan refund:
  - `/admin/payments/{id}/proof`;
  - `/admin/payments?status=pending_review`, sekarang juga memuat status `submitted`;
  - `/admin/refunds` (GET), `/admin/refunds/{id}/process`, `/admin/refunds/{id}/waive`.
- Seleksi:
  - `/admin/selection/formulas?wave_id&track_id`;
  - `GET /admin/selection/rankings?wave_id&track_id` (run final terakhir + keputusan);
  - `/admin/selection/approvals` (GET/POST → core `selection_result`);
  - `/admin/selection/announcements` (GET/POST), `/admin/selection/announcements/{id}/cancel`;
  - `/admin/selection/waitlist`, `/admin/selection/waitlist/promote`.
- Konversi: `POST /admin/conversions` (mulai konversi, header Idempotency-Key).

Dashboard dan internal:

- `GET /admin/dashboard` untuk admin, superadmin, kepala_sekolah dan kepala_yayasan. Responsnya memuat `options` (periode/gelombang/jalur), sehingga pimpinan tidak perlu endpoint konfigurasi yang khusus admin.
- `POST /internal/approval-callbacks` (callback core).

## Verifikasi (output nyata, 2026-10-14)

```
$ migrate up / down 1 / up  (skci_qa_ppdb_admission)
no change
6/d qa_ppdb_round2 (74.092958ms)
6/u qa_ppdb_round2 (70.5135ms)
6

$ gofmt -l . ; go vet ./... && go test ./... -count=1   (TEST_PG_DSN diset)
ok  	skci/admission/internal/client	0.458s
ok  	skci/admission/internal/conversion	0.405s
ok  	skci/admission/internal/handler	1.834s
ok  	skci/admission/internal/model	0.386s
ok  	skci/admission/internal/otp	0.370s
ok  	skci/admission/internal/simplepdf	0.350s

$ go test ./internal/handler -run 'TestFormulaRequiresScope|TestLatestRanking|TestPaymentQueuePending|TestDashboardOptions|TestApplicationDetailRefunds' -v
--- PASS: TestFormulaRequiresScope (0.08s)
--- PASS: TestLatestRanking (0.09s)
--- PASS: TestPaymentQueuePendingIncludesSubmitted (0.09s)
--- PASS: TestDashboardOptionsForLeaders (0.08s)
--- PASS: TestApplicationDetailRefunds (0.06s)

$ cd web && npm ci && npx vue-tsc -b && npm run build
ci=0 / tsc-ok / ✓ built in 8.69s  (hanya peringatan ukuran chunk > 500 kB yang sudah ada)
```

## Langkah server (TIDAK dijalankan; untuk deployer)

1. Backup DB admission. Jalankan migrasi admission sampai versi 6.
2. Env `admission`:
   - `CORE_URL` (approval center) dan `ADMISSION_INTERNAL_URL` (URL yang dipakai core untuk callback `/internal/approval-callbacks`);
   - `SEKOCI_INTERNAL_KEY`, `SEKOCI_JWT_SECRET` (sama dengan identity; dibutuhkan login SEKOCI orang tua);
   - `SMTP_HOST`, `SMTP_PORT`, `SMTP_USER`, `SMTP_PASSWORD`, `SMTP_FROM_NAME`, `SMTP_FROM_ADDRESS` (email notifikasi; tanpa SMTP notifikasi tetap tampil di portal dengan email_status `skipped`);
   - `ADMISSION_JOB_INTERVAL` (default 1m: gugur formulir/daftar ulang, publikasi pengumuman);
   - `EXAM_URL` dan `EXAM_PUBLIC_URL`. Tanpa keduanya, portal menampilkan "Layanan ujian CBT belum dikonfigurasi" dan tombol Masuk CBT tidak ditampilkan.
3. Core (Approval Center): buat flow untuk request_type `discount_special` dan `selection_result` (Selection Committee) per sekolah. Tanpa flow, pengajuan ditolak dengan pesan dari core yang tampil di UI.
4. Data demo server:
   - nonaktifkan aturan promo duplikat (Early Bird ×2, Saudara Kandung ×2, Beasiswa Tahfidz ×2);
   - simpan ulang "Saudara Kandung 10%" lewat UI (UI mengonversi persen → basis poin);
   - isi `education_level` gelombang sesuai unit (data SD/SMA pada unit SMP perlu dibersihkan admin);
   - isi kebijakan refund periode.
5. Pendaftar lama berstatus draft dengan dokumen terverifikasi (contoh Ryoto Ogawa): review dokumen ulang setelah formulir dikirim; aturan baru mencegah kasus baru.

## Kontrak ke service lain (Claude menyambungkan)

- **accounting**: pengembalian dana PPDB dicatat di `admission_refunds` (status paid, method, reference, paid_at). Untuk posting jurnal, accounting dapat menarik `GET /admin/refunds?status=paid` (JWT admin) atau menambahkan event. Belum ada consumer; posting jurnal bukan bagian service admission.
- **identity**: login SEKOCI orang tua memakai JWT identity dengan `active_role=orang_tua` dan `school_id`. Admission menautkan guardian lewat `guardians.identity_user_id` (partial unique). Identity perlu menyediakan akun orang tua (dibuat oleh saga konversi melalui `IDENTITY_URL`). Tidak ada perubahan API identity yang dibutuhkan.
- **core**: callback `POST {ADMISSION_INTERNAL_URL}/internal/approval-callbacks` dengan `X-Internal-Key`, payload `{approval_id, source_id, request_type, decision, decided_by, comment, decided_at}`; idempoten lewat `approval_callback_events`.

## Asumsi

- Bobot formula dalam persen (disimpan ×100 sebagai basis poin), nilai komponen 0..`max_score` (default 10000).
- Rekomendasi ranking `accepted`/`waitlisted`. API keputusan menerima `accepted|offered`.
- Field formulir bawaan (biodata) hanya diatur wajib/label; field custom divalidasi tipe saat simpan dan wajibnya saat kirim.
- Lokasi CBT "Online" karena tes CBT memang daring. Tes/wawancara tatap muka memakai jadwal per pendaftar.

## Catatan / risiko

- Bila `offerSeat` gagal saat publikasi pengumuman (mis. kuota berubah), job mencoba ulang setiap tick sampai berhasil atau pengumuman dibatalkan admin. Galatnya tercatat di log.
- Pesan galat API kini seluruhnya berbahasa Indonesia. Klien lain yang mencocokkan teks Inggris lama (tidak ditemukan di repo) perlu menyesuaikan.

## Putaran 2

Target pemilik: tidak ada tombol mati, tidak ada temuan di server.

Yang dikerjakan di putaran 2, di luar perbaikan putaran 1:

**Selesai dari status DEFERRED/PARTIAL**

- BUG-095: form builder.
- BUG-093: jenjang/kelas pada gelombang.
- BUG-040: refund (proses transfer/tunai, bebaskan, notifikasi, saldo `refund_due_idr`).
- BUG-108: login SEKOCI orang tua.

**Audit tombol, view admin**

- Seleksi: tab Formula (gelombang/jalur eksplisit), Nilai (input per komponen), Ranking & keputusan (muat ranking final, pratinjau, tetapkan, keputusan per entri, terapkan rekomendasi), Persetujuan, Pengumuman, Cadangan. Semuanya memanggil endpoint yang ada.
- Pembayaran:
  - bukti bayar sebelumnya tidak bisa dibuka (tautan tanpa JWT); kini pratinjau blob lewat endpoint baru;
  - filter default "Perlu review" sebelumnya tidak pernah cocok dengan status `submitted`; sudah diperbaiki;
  - tab baru Pengembalian dana.
- Konfigurasi: tab Field formulir; kebijakan refund periode; jenjang/kelas gelombang.
- Konversi: daftar "Siap dikonversi" + tombol Mulai konversi (sebelumnya tidak ada cara memulai konversi dari UI). Detail pendaftar juga punya tombol Konversi menjadi siswa.
- Detail pendaftar: tombol transisi hanya tampil untuk status yang diizinkan backend.
- Audit dan antrean menampilkan pesan galat dari server.

**Audit tombol, kepala_sekolah/kepala_yayasan**

- Hanya menu Dashboard PPDB yang tampil (route role = DashboardAuth).
- Pintasan/tugas khusus admin disembunyikan.
- Filter memakai `options` dari respons dashboard.
- Diuji: pimpinan 200 di dashboard, 403 di endpoint admin; guru 403.

**Audit tombol, portal calon orang tua**

- Formulir: field dinamis per periode; gelombang dibatasi yang `open` pada periode terpilih (dengan jenjang/kelas); jalur per periode.
- Detail: jadwal tes/wawancara, unduh bukti PDF, tagihan per tahap (pendaftaran/daftar ulang) dengan minimum dan jatuh tempo, info gugur + refund, jawaban formulir.
- CBT: state "belum dikonfigurasi" yang jelas.
- Ringkasan: notifikasi + tandai dibaca; hapus calon siswa yang belum mendaftar.
- Navigasi: tombol Keluar.
- Login: tombol "Masuk dengan akun SEKOCI" bila sesi SEKOCI orang tua aktif.
- Semua galat memakai pesan server (`apiError`).

**Lain-lain**: pesan galat backend yang berbahasa Inggris diterjemahkan.

**Tes baru**: TestFormFieldsBlockSubmit, TestWaveWarningsPerLevel, TestRefundRecordAndProcess, TestSekociSessionExchange, TestFormulaRequiresScope, TestLatestRanking, TestPaymentQueuePendingIncludesSubmitted, TestDashboardOptionsForLeaders, TestApplicationDetailRefunds, serta tes model ppdb_round2.
