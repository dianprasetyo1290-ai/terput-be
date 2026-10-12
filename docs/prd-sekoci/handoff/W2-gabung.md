# Handoff Cek Gabungan Gelombang 2 (CEK-W2)

**Branch:** `fix/W2-temuan` (dari `integrasi/prd-v2`, basis `3f84be7`)
**Tanggal:** 2026-10-13
**Cakupan:** hasil merge T3 (payroll), T4 (ATS), T5 (pelatihan), T7b (layanan orang tua) + temuan §1–§3 di `W2-temuan-awal.md`
**Status akhir:** SELESAI. Semua build, test, migrasi, dan build frontend hijau. Sisa masalah ada di §6.

Di luar scope (dikerjakan agen lain): T11 (hook service desk di `CreateParentServiceRequest`) dan T14 (jurnal gateway billing, temuan §4).

---

## 1. Kerusakan yang ditemukan

| # | Lokasi | Kerusakan | Dampak |
|---|---|---|---|
| 1a | `hr/internal/client/client.go` | `callback_url` diisi `CORE_URL + /internal/approval-callbacks`, padahal route itu milik hr | callback dikirim ke core sendiri, hr tidak pernah menerima keputusan |
| 1b | `hr/internal/handler/callbacks.go` | membaca `resource_type/resource_id/notes`, sedangkan core mengirim `source_id/request_type/...` (`core/internal/jobs/effects.go` `CallbackPayload`, `decision` = `req.Status` = `approved`/`rejected`) | callback asli dari core selalu 400 |
| 1c | `hr/internal/handler/ess.go` | `request_type` = `leave_request`/`overtime_request`. Core hanya menerima `model.KnownRequestTypes` (`leave`, `overtime`, `profile_change`, ...) | core menolak 422, jadi approval tidak pernah dibuat. Temuan baru, tidak tercatat di W2-temuan-awal |
| 1d | `hr/internal/handler/ess.go` | `subject_user_id` = user atasan | penyetuju menjadi atasan dari atasan |
| 1e | `hr/internal/handler/ess.go` | `appID, _ :=` membuang error, dan approval hanya dibuat jika pegawai punya atasan | gagal tanpa jejak, cuti/lembur `pending` selamanya |
| 1f | `hr/internal/handler/ess.go` | perubahan profil (`profile_change`) tidak membuat approval sama sekali | pengajuan profil tidak pernah diputuskan |
| 1g | T3 `payroll` / T4 `ats` | `SubmitRun`/`SubmitRequisition` hanya mengubah status ke `pending_approval` tanpa memanggil core. `FinalizeRun` tidak mengecek status | payroll/requisition tidak pernah bisa `approved`. Finalize payroll bisa melewati approval |
| 1h | mock test hr | fake core mengembalikan `{approval_id}` (bukan `{id}`) dan string non-UUID | test lolos walau kontrak salah |
| 3a | `billing/internal/handler/gateway_corrections.go` | `callback_url` = `http://localhost:<port>/...` | di docker, core tidak menjangkau billing |
| 3b | sama | error `RequestApproval` dibuang. Callback: decode error diabaikan, tidak idempoten (keputusan `rejected` yang datang terlambat menimpa `approved`), error saat menerapkan keputusan tetap dijawab 200 | koreksi menggantung, retry core tidak jalan |
| 3c | `academic/internal/handler/transfer.go` (T7a) | `callback_url` relatif `/internal/student-status-requests/approval-callback`. Core mewajibkan `http(s)://` | approval mutasi siswa selalu 422 dan error-nya dibuang |
| 3d | `docker-compose.yml` | `HR_PUBLIC_INTERNAL_URL` (hr), `CORE_URL`/`BILLING_INTERNAL_URL` (billing), `HR_URL` (core), `CORE_URL` (identity) tidak diset. Identity default ke `http://localhost:8091` | billing tanpa core (koreksi auto-apply), core tidak bisa resolve `direct_supervisor`, audit keamanan identity hilang |
| 2 | `core/internal/handler/service_request.go` (T7b) | `CreateParentServiceRequest` tidak mengecek peran, tidak memverifikasi wali–siswa, tidak menotifikasi staf | orang tua bisa mengajukan atas nama siswa lain. Staf tidak tahu ada permintaan |
| W1 | `web/src/services/core/audit.ts` (T1) | memanggil `GET /audit/export` (tidak ada di core) dan mengirim `start_date/end_date/actor_user_id`, sedangkan core membaca `from/to/user_id` | ekspor CSV 404, filter tanggal diabaikan |

Tidak ditemukan import ganda, route ganda (web maupun chi), atau placeholder "setelah rebase". Semua path API `hr`/`core` yang dipanggil frontend ada di router backend, kecuali `/audit/export` (sudah diperbaiki).

## 2. Perbaikan

### §1 hr approval (`fb72236`)
- `client.Client.CallbackURL` diisi di `main.go` dari `cfg.HRPublicInternalURL + "/internal/approval-callbacks"`. `CreateApproval` selalu mengisi `source_service=hr`, `callback_url`, `idempotency_key=<type>:<id>` (default), dan `correlation_id`.
- Konstanta `ReqTypeLeave/Overtime/ProfileChange/Requisition/PayrollRun` mengikuti `core/internal/model`.
- ESS cuti/lembur/profil: insert dan pembuatan approval berjalan dalam satu transaksi. Jika core menolak, transaksi di-rollback, error dicatat di log, dan pesan core dikembalikan ke UI (`422 "Approval Center menolak pengajuan: alur approval ... belum dikonfigurasi"`, atau `502` jika core tidak terjangkau). Tidak ada auto-approve (T2 §approval). Gerbang "hanya jika ada atasan" dihapus: alur approval di core yang menentukan penyetuju. `subject_user_id` = `maker_user_id` = user pemohon.
  - Catatan pola T1: tidak dibuat item exception queue, karena `kind` exception di core tidak punya jenis untuk kegagalan pembuatan approval dan baris sumber tidak disimpan, jadi tidak ada yang perlu di-retry. Error terlihat oleh pengguna dan tercatat di log.
- `callbacks.go` membaca payload core asli, lalu dispatch per `request_type`: `leave`, `overtime`, `profile_change`, `payroll_run`, `requisition`. Respons: `400` untuk payload tidak lengkap atau format lama, `422` untuk jenis yang tidak ditangani, `404` jika sumber tidak ditemukan, `5xx` saat gagal (core akan retry, lalu masuk exception setelah `max_attempts`).
- Idempoten: `ApplyLeaveDecisionTx`, `ApplyOvertimeDecisionTx`, `ApplyProfileChangeRequestTx`, `ApplyRunDecision`, dan `ApplyRequisitionDecision` hanya mengubah baris yang masih `pending`/`pending_approval`. Callback ulang menjawab 200 tanpa perubahan, jadi saldo cuti tidak terpotong dua kali.
- T3: `POST /payroll/runs/{id}/submit` membuat approval `payroll_run` dengan `amount_idr` = total net dan idempotency `payroll_run:<id>:<calculated_at>` (run yang dihitung ulang mendapat approval baru), lalu menyimpan `approval_id`. `FinalizeRun` mensyaratkan status `approved`, selain itu `409`. Frontend (`RunDetailView`) memang sudah hanya menampilkan tombol finalize saat `approved`.
- T4: `POST /ats/requisitions/{id}/submit` membuat approval `requisition` dengan flag `exceeds_headcount` (AC-88-01) dan `amount_idr` = `budget_idr`.

### §3 callback localhost (`f4d41a1`, `8c8179b`, `012384a`, `9686e95`)
- billing: callback = `cfg.Gateway.BillingInternalURL` (env `BILLING_INTERNAL_URL`, sudah ada di config T6 tetapi belum dipakai; default dev `http://localhost:<PORT>` hanya jika env kosong). Correction + approval dibuat dalam satu transaksi, dan penolakan core dikembalikan (`422`/`502`). Callback: payload tidak valid → 400, sudah final → 200 no-op, gagal menerapkan → 5xx.
- academic: `CoreClient.CallbackBaseURL` dari env `ACADEMIC_INTERNAL_URL` (default `http://academic:8082`, mengikuti default docker `CORE_BASE_URL` di file yang sama). Error `SubmitApproval` dicatat di log.
- `.env.example` (tambah baris): `HR_PUBLIC_INTERNAL_URL`, `BILLING_INTERNAL_URL`, `ACADEMIC_INTERNAL_URL`.
- `docker-compose.yml` (tambah baris): hr `HR_PUBLIC_INTERNAL_URL=http://hr:${HR_PORT}`. billing `CORE_URL`, `BILLING_INTERNAL_URL=http://billing:${BILLING_PORT}`. academic `CORE_BASE_URL`, `ACADEMIC_INTERNAL_URL`. core `HR_URL`. identity `CORE_URL`. `docker-compose.local.yml` tidak meng-override environment, jadi tidak diubah.
- Hasil grep `localhost` lain di kode Go non-test: hanya default config/CORS dan `identity/internal/handler/app.go` (`CoreURL` default yang di-override `CORE_URL` di `main.go`, sekarang diset di compose).

### §2 layanan orang tua (`5c9e026`)
- `POST /parent/service-requests` hanya untuk peran `orang_tua` (403 untuk peran lain).
- Jika `student_user_id` diisi (string kosong dianggap tidak diisi), core memanggil identity `GET /internal/guardians/{g}/students/{s}/access?school_id=` lewat `client.GuardianHasStudent`. 404 → `403 "Anda bukan wali aktif siswa ini"`. Jika identity tidak terkonfigurasi atau gagal, hasilnya `503` (fail closed).
- Setelah dibuat, notifikasi `type=service_request` dikirim di background (`h.bg`) ke semua pemegang `handler_role` kategori lewat `UsersByRole` + `NotifyUsers` (idempotency `service_request:<id>:submitted:<user>`), mengikuti pola notifikasi approval.

### W1 audit (`9fefc5d`)
- `auditService` memetakan filter ke `user_id/from/to`. Ekspor memakai `GET /audit?format=csv`.

## 3. Test baru
- `hr/internal/client/client_test.go`: kontrak body ke core (subjek = pemohon, callback hr, idempotency) dan pemetaan error 422/502.
- `hr/internal/handler/handler_test.go` (DB): cuti dengan payload core asli dikirim 2× (saldo tetap 2 hari). Lembur ditolak core → 422 dan tidak tersimpan. Callback `rejected`, lalu `approved` terlambat (tetap rejected). Payload lama → 400. Jenis tak dikenal → 422. Sumber tak dikenal → 404. Profil via `profile_change`. `payroll_run`: submit → finalize sebelum approve 409 → callback 2× → approved. `requisition`: submit → callback → approved.
- `billing/internal/handler/gateway_corrections_test.go` (DB): callback_url = `http://billing:8090/internal/corrections/<id>/callback`. Callback approved 2× idempoten. `rejected` terlambat tidak menimpa.
- `core/internal/handler/service_request_test.go`: siswa bukan anaknya → 403. Peran guru → 403. Wali sah → 201 + 1 notifikasi ke staf `tata_usaha`. Identity mati → 503.

## 4. Output verifikasi (asli)

### 4.1 `go build ./... && go vet ./...`
```
=== academic      VET_OK
=== accounting (no go.mod, Laravel — tidak diuji, tanpa php di toolchain)
=== admission     VET_OK
=== attendance    VET_OK
=== billing       VET_OK
=== core          VET_OK
=== evaluation    VET_OK
=== exam          VET_OK
=== hr            VET_OK
=== identity      VET_OK
=== notification  VET_OK
=== report        VET_OK
```

### 4.2 `TEST_PG_DSN=postgres://localhost/skci_w2_<svc> go test -count=1 ./...`
```
academic:     ok common · ok handler · ok model · ok store                         EXIT 0
admission:    ok client · ok conversion · ok model · ok otp                        EXIT 0
attendance:   ok client · ok handler                                               EXIT 0
billing:      ok client · ok common · ok handler · ok model · ok nicepay           EXIT 0
core:         ok common · ok handler · ok model · ok store                         EXIT 0
evaluation:   ok handler                                                           EXIT 0
exam:         ok cache · ok common · ok config · ok handler · ok model · ok store  EXIT 0
hr:           ok ats/calc · ok client · ok crypto · ok handler · ok jobs · ok model · ok payroll/calc · ok training/calc   EXIT 0
identity:     ok common · ok handler · ok store                                    EXIT 0
notification: ok config · ok handler · ok provider · ok store                      EXIT 0
report:       ok cache · ok docx · ok handler · ok model · ok upstream             EXIT 0
```
`go test -v ./internal/handler/` hr: `--- PASS: TestHandlerE2E` (berjalan, tidak di-skip). billing: `--- PASS: TestCorrectionApprovalViaCore`, `--- PASS: TestAdminCreateCorrection`.

### 4.3 Migrasi dari nol: `up` → `down 1` → `up` (DB baru `skci_w2_<svc>`)
```
academic:     up[5/u transfers]               down1[5/d transfers]               up[5/u transfers]               version=5
admission:    up[3/u prd_gaps]                down1[3/d prd_gaps]                up[3/u prd_gaps]                version=3
attendance:   up[8/u leave_requests]          down1[8/d leave_requests]          up[8/u leave_requests]          version=8
billing:      up[4/u student_bill_visibility] down1[4/d student_bill_visibility] up[4/u student_bill_visibility] version=4
core:         up[2/u service_requests]        down1[2/d service_requests]        up[2/u service_requests]        version=2
evaluation:   up[6/u prosem]                  down1[6/d prosem]                  up[6/u prosem]                  version=6
exam:         up[8/u admission_participants]  down1[8/d admission_participants]  up[8/u admission_participants]  version=8
hr:           up[30/u training]               down1[30/d training]               up[30/u training]               version=30
identity:     up[16/u deactivation_timestamp] down1[16/d deactivation_timestamp] up[16/u deactivation_timestamp] version=16
notification: up[2/u chat]                    down1[2/d chat]                    up[2/u chat]                    version=2
report:       up[13/u capaian_dua_kolom]      down1[13/d capaian_dua_kolom]      up[13/u capaian_dua_kolom]      version=13
```
Tambahan: migrasi gelombang 2 hr di-down 4 lalu up lagi:
```
30/d training · 20/d ats · 11/d statutory_seed · 10/d payroll
10/u payroll · 11/u statutory_seed · 20/u ats · 30/u training
```

### 4.4 Frontend
```
$ npx vue-tsc -b      → VUETSC_EXIT:0   (sebelum dan sesudah perbaikan audit.ts)
$ npm run build       → ✓ built in 1.85s   BUILD_EXIT:0
```

## 5. File yang diubah
- hr: `cmd/server/main.go`, `internal/client/client.go` (+`client_test.go`), `internal/handler/{callbacks,ess,router,handler_test}.go`, `internal/store/{leave,ess}.go`, `internal/payroll/{handler,store}.go`, `internal/ats/{handler,store}.go`
- billing: `internal/config/config.go`, `internal/handler/gateway_corrections.go` (+`gateway_corrections_test.go`)
- academic: `internal/client/core.go`, `internal/handler/transfer.go`
- core: `internal/client/client.go`, `internal/handler/service_request.go` (+`service_request_test.go`), `internal/handler/fakestore_test.go`
- web: `src/services/core/audit.ts`
- file bersama (hanya tambah baris): `.env.example`, `docker-compose.yml`

## 6. Sisa masalah / pertanyaan terbuka
1. **T5 `training_paid` belum tersambung ke core.** `calc.NeedsApproval` ada, tetapi belum ada alur enrol mandiri yang membuat `pending_approval`. `POST /training/assignments` (admin) langsung membuat penugasan. Perlu endpoint enrol pegawai + approval `training_paid` + dispatch di `callbacks.go`. Ini fitur baru, jadi tidak dikerjakan di CEK.
2. **T4 offer** (`offer`, `salary_out_of_range`, AC-92-01) belum melewati core. `POST /ats/offers` tidak membuat approval. Endpoint `submit/send` offer belum ada.
3. **T3 perubahan komponen/statutori/input setelah cutoff** (`payroll_component_change`, `payroll_input_change`) belum lewat approval.
4. hr masih punya `POST /leave-requests/{id}/approve|reject` dan `/overtime-requests/{id}/approve|reject` (keputusan langsung oleh hr_admin, melewati core dan maker-checker). Sekarang aman terhadap callback ganda (keduanya hanya mengubah baris `pending`), tetapi melanggar GEMINI §4 "approval lewat core". Perlu keputusan pemilik: dihapus, atau dibiarkan sebagai override HR yang diaudit.
5. Alur approval `leave`, `overtime`, `profile_change`, `payroll_run`, `requisition`, `payment_correction`, `student_transfer/withdrawal` harus **dikonfigurasi di Approval Center** per sekolah. Tanpa itu, pengajuan sekarang ditolak dengan pesan 422 yang jelas (sesuai kontrak), bukan diam-diam menggantung.
6. Academic (T7a) masih menyimpan `student_status_request` walau core menolak (hanya di-log). Penyelarasan ke pola transaksi seperti hr/billing perlu disepakati dengan pemilik T7a.
7. Billing di compose belum menerima `NOTIFICATION_URL`/`ACCOUNTING_URL`. Diserahkan ke T14 (jurnal gateway) supaya tidak bentrok.
8. Uji UI Playwright per gelombang belum dijalankan (mode paralel melarang dev server di port tetap).
