# Handoff Task T7a: Transfer & Withdrawal Siswa (Menu 78) & Saklar Tagihan Saya (Menu 82)

## 1. Ringkasan Eksekutif
Task T7a mengimplementasikan:
1. **Menu 78 (Transfer & Withdrawal Siswa):** Layanan pengelolaan status mutasi siswa (pindah kelas internal, pindah unit yayasan, dan pengunduran diri/withdrawal) pada service `academic`, terintegrasi dengan service `billing` (penarikan ringkasan keuangan beku, perlakuan tagihan paska keluar, dan refund deposit), service `core` (approval berjenjang dengan flags keuangan dan audit mutasi), serta service `identity` (deaktivasi peran siswa dan tautan wali dengan timestamp audit `deactivated_at`).
2. **Menu 82 (Tagihan Saya & Saklar Visibilitas per Jenjang):** Pengaturan visibilitas tagihan siswa per jenjang pendidikan pada service `billing`. Siswa pada jenjang yang dinonaktifkan (misal: SD) disaring dari navigasi sidebar dan diblokir dengan status `403 Forbidden` pada endpoint `/me/bills`. Siswa pada jenjang aktif (misal: SMK) dapat melihat rincian tagihan secara read-only tanpa tombol bayar (BR-GEN-04).

---

## 2. Daftar Berkas (File List)

### Berkas Baru (New Files)
- `services/academic/migrations/0005_transfers.up.sql`
- `services/academic/migrations/0005_transfers.down.sql`
- `services/academic/internal/model/transfer.go`
- `services/academic/internal/model/transfer_test.go`
- `services/academic/internal/store/transfer.go`
- `services/academic/internal/client/billing.go`
- `services/academic/internal/client/core.go`
- `services/academic/internal/handler/transfer.go`
- `services/academic/internal/handler/transfer_test.go`
- `services/billing/migrations/0004_student_bill_visibility.up.sql`
- `services/billing/migrations/0004_student_bill_visibility.down.sql`
- `services/billing/internal/model/visibility.go`
- `services/billing/internal/model/visibility_test.go`
- `services/billing/internal/handler/transfer_billing.go`
- `services/billing/internal/handler/transfer_billing_test.go`
- `services/identity/migrations/0016_deactivation_timestamp.up.sql`
- `services/identity/migrations/0016_deactivation_timestamp.down.sql`
- `services/identity/internal/handler/deactivate_test.go`
- `web/src/services/academic/transfer.service.ts`
- `web/src/services/billing/visibility.service.ts`
- `web/src/views/academic/StudentStatusRequestsView.vue`
- `web/src/views/academic/StudentStatusRequestFormView.vue`
- `web/src/views/academic/StudentStatusRequestDetailView.vue`
- `web/src/views/billing/BillingSettingsView.vue`

### Berkas Diubah (Modified Files)
- `services/academic/cmd/server/main.go` (registrasi route transfer & academic settings)
- `services/academic/internal/client/identity.go` (penambahan method `DeactivateUser`)
- `services/academic/internal/common/auth.go` (helper test `WithClaims`)
- `services/academic/internal/handler/handler.go` (injeksi client `Billing` & `Core`)
- `services/billing/cmd/server/main.go` (registrasi route `/me/bills/visibility`, admin visibility, dan internal exit/refund)
- `services/billing/internal/client/client.go` (penambahan `StudentGradeLevel` dan `SchoolGradeLevels`)
- `services/billing/internal/handler/app.go` (ekspansi interface `ClassRoster`)
- `services/billing/internal/handler/bills.go` (pengecekan `student_billing_exits` saat generate bills)
- `services/billing/internal/handler/parent.go` (guard `checkStudentBillVisibility` pada `/me/bills` & `/me/payments`)
- `services/billing/internal/handler/teacher_scope_test.go` (stub `ClassRoster` terupdate)
- `services/identity/cmd/server/main.go` (registrasi route `/internal/users/{id}/deactivate`)
- `services/identity/internal/handler/internal_conversion.go` (handler `InternalDeactivateUser`)
- `web/src/router/domains/academic.routes.ts` (route `akademik/mutasi-siswa`, `baru`, `:id`)
- `web/src/router/domains/billing.routes.ts` (route `keuangan/pengaturan-tagihan`)
- `web/src/router/domains/student.routes.ts` (label `'Tagihan Saya'` & `feature: 'student_bills'`)
- `web/src/services/academic/types.ts` (tipe status requests & academic settings)
- `web/src/services/billing/billing.types.ts` (tipe `VisibilityItem` & `MyBillVisibility`)
- `web/src/stores/features.ts` (evaluasi feature flag `'student_bills'` via `getMyBillVisibility`)

---

## 3. Migrasi & Endpoint Baru

### 3.1 Migrasi Database
1. `academic/0005_transfers`:
   - Tabel `student_status_requests` (`id`, `school_id`, `student_user_id`, `kind`, `from_classroom_id`, `to_classroom_id`, `to_school_id`, `effective_date`, `reason`, `attachments`, `finance_snapshot`, `refund_amount_idr`, `status`, `approval_id`, `applied_at`, `requested_by`, `idempotency_key`, `created_at`, `updated_at`).
   - Tabel `academic_settings` (`school_id` PK, `post_exit_bill_policy` CHECK IN ('void','keep') DEFAULT 'void', `parent_grace_days` INT DEFAULT 30, `updated_at`).
   - Pelebaran CHECK `class_enrollments.status` menyertakan `'withdrawn'`.
2. `billing/0004_student_bill_visibility`:
   - Tabel `student_bill_visibility` (`school_id`, `grade_level`, `enabled`, PRIMARY KEY (`school_id`, `grade_level`)).
   - Tabel `student_billing_exits` (`school_id`, `student_user_id`, `effective_date`, `policy`, `created_at`, PRIMARY KEY (`school_id`, `student_user_id`)).
   - Penyesuaian `bill_refunds` (`student_bill_id` nullable untuk refund withdrawal, ditambah `idempotency_key`).
3. `identity/0016_deactivation_timestamp`:
   - Penambahan kolom `deactivated_at timestamptz` pada tabel `user_school_roles` dan `guardian_links`.

### 3.2 Endpoint Baru
| Service | Method | Path | Auth / Role | Keterangan |
|---|---|---|---|---|
| `academic` | `POST` | `/student-status-requests` | JWT (`admin`, `superadmin`, `walikelas`, `kepala_sekolah`) | Pengajuan mutasi siswa + capture snapshot keuangan |
| `academic` | `GET` | `/student-status-requests` | JWT (`admin`, `superadmin`, `walikelas`, `kepala_sekolah`) | Daftar mutasi siswa (filter jenis & status) |
| `academic` | `GET` | `/student-status-requests/{id}` | JWT (`admin`, `superadmin`, `walikelas`, `kepala_sekolah`) | Detail permohonan mutasi |
| `academic` | `POST` | `/student-status-requests/{id}/apply` | JWT (`admin`, `superadmin`) | Eksekusi penerapan mutasi yang sudah disetujui |
| `academic` | `GET` | `/student-status-requests/financial-summary` | JWT (`admin`, `superadmin`, `walikelas`) | Ringkasan keuangan realtime siswa |
| `academic` | `GET` | `/academic-settings` | JWT (`admin`, `superadmin`) | Ambil pengaturan kebijakan akademik |
| `academic` | `PUT` | `/academic-settings` | JWT (`admin`, `superadmin`) | Perbarui kebijakan akademik |
| `academic` | `POST` | `/internal/student-status-requests/approval-callback` | Internal (`X-Internal-Key`) | Webhook callback persetujuan dari Core |
| `academic` | `POST` | `/internal/student-status-requests/apply-pending` | Internal (`X-Internal-Key`) | Cron trigger penerapan mutasi tanggal efektif |
| `billing` | `GET` | `/admin/student-bill-visibility` | JWT (`admin`, `superadmin`) | Daftar status saklar visibilitas per jenjang |
| `billing` | `PUT` | `/admin/student-bill-visibility` | JWT (`admin`, `superadmin`) | Simpan pengaturan saklar per jenjang |
| `billing` | `GET` | `/me/bills/visibility` | JWT (`siswa`) | Ambil status aktif/nonaktif tagihan siswa pemanggil |
| `billing` | `GET` | `/internal/students/{id}/financial-summary` | Internal (`X-Internal-Key`) | Rincian tagihan terbuka, tempo, dan deposit |
| `billing` | `POST` | `/internal/students/{id}/exit` | Internal (`X-Internal-Key`) | Catat exit & terapkan policy void/keep tagihan |
| `billing` | `POST` | `/internal/students/{id}/refund` | Internal (`X-Internal-Key`) | Pengajuan refund dana siswa dengan idempotency key |
| `identity` | `POST` | `/internal/users/{id}/deactivate` | Internal (`X-Internal-Key`) | Deaktivasi role sekolah & tautan wali siswa |

---

## 4. Tabel Cakupan FR & AC

| Kode Spec | Deskripsi Kebutuhan | Bukti Implementasi & Pengujian | Status |
|---|---|---|---|
| **FR-78-001** | Pengajuan mutasi siswa (pindah kelas, pindah unit, keluar) | `TestValidateCreateStatusRequest` & `TestAC78_01_WithdrawalWithArrears`, UI `StudentStatusRequestFormView.vue` | **Lolos** |
| **FR-78-002** | Capture ringkasan keuangan saat pengajuan (tidak berubah setelahnya) | `InternalStudentFinancialSummary` billing, disimpan di `finance_snapshot`, `TestAC78_01_WithdrawalWithArrears` | **Lolos** |
| **FR-78-003** | Persetujuan berjenjang lewat Core Approval dengan flags keuangan | `DetermineApprovalFlags`, pengiriman approval di `CreateStatusRequest`, `TestDetermineApprovalFlags` | **Lolos** |
| **FR-78-004** | Penerapan mutasi tanpa menghapus riwayat enrollment lama | `ApplyClassTransfer`, `ApplyUnitTransfer` (`transferred_out`), `TestAC78_02_UnitTransferWithoutHistoryLoss` | **Lolos** |
| **FR-78-005** | Kebijakan tagihan pasca keluar (`void` / `keep`) | `InternalStudentExit` membatalkan tagihan belum jatuh tempo, `TestInternalStudentExit_VoidPolicy` | **Lolos** |
| **FR-78-006** | Pengembalian deposit/kelebihan bayar dengan idempotensi | `InternalStudentRefund`, `TestAC78_03_WithdrawalDepositRefund`, `TestInternalStudentRefund_Idempotency` | **Lolos** |
| **FR-78-007** | Deaktivasi akses akun siswa dan pencatatan timestamp | `InternalDeactivateUser` di identity, `TestInternalDeactivateUser` membuktikan `status='inactive'` & `deactivated_at IS NOT NULL` | **Lolos** |
| **AC-78-01** | Siswa withdrawal dengan tunggakan Rp 800.000 memuat ringkasan keuangan & flag `has_outstanding` | `TestAC78_01_WithdrawalWithArrears` (status pending approval, amount 800.000, flag `has_outstanding=true`) | **Lolos** |
| **AC-78-02** | Unit transfer memindahkan siswa ke unit baru tanpa menghapus riwayat lama | `TestAC78_02_UnitTransferWithoutHistoryLoss` (enrollment lama berstatus `transferred_out`, baru diaktifkan di unit tujuan) | **Lolos** |
| **AC-78-03** | Withdrawal dengan kelebihan bayar/deposit memicu refund idempoten | `TestAC78_03_WithdrawalDepositRefund` (refund sebesar Rp 200.000 tercatat dengan idempotency key `withdrawal:<id>`) | **Lolos** |
| **BR-STU-01** | Riwayat siswa tidak dihapus secara destruktif | Tidak ada `DELETE` statement pada enrollment; status transisi memakai `'withdrawn'` dan `'transferred_out'` | **Lolos** |
| **BR-STU-02** | Workflow persetujuan memuat tahap verifikasi keuangan | Flag `has_outstanding` disematkan pada approval request ke Core | **Lolos** |
| **BR-STU-03** | Tagihan berikutnya mengikuti unit/sekolah baru | `student_billing_exits` menyaring siswa keluar dari generator tagihan di sekolah asal | **Lolos** |
| **OQ-07** | Konfigurasi `post_exit_bill_policy` & `parent_grace_days` | Tabel `academic_settings`, endpoint `GET/PUT /academic-settings`, UI Tab Pengaturan di `StudentStatusRequestsView.vue` | **Lolos** |
| **FR-82-001** | Saklar visibilitas tagihan per jenjang (default nonaktif) | Tabel `student_bill_visibility`, fungsi murni `IsGradeBillVisible`, `TestIsGradeBillVisible` | **Lolos** |
| **AC-82-01** | Siswa jenjang SD dengan tagihan dinonaktifkan diblokir (403 Forbidden) dan disembunyikan dari nav | `TestStudentBillVisibility_AC82_01_SD_Forbidden` mengembalikan 403, Pinia store `features.ts` menyaring navigasi sidebar | **Lolos** |
| **AC-82-02** | Siswa jenjang SMK dengan visibilitas aktif dapat membaca tagihan (200 OK) tanpa tombol bayar | `TestStudentBillVisibility_AC82_02_SMK_Allowed` (200 OK), `StudentFinanceView.vue` terverifikasi read-only tanpa tombol aksi pembayaran | **Lolos** |
| **BR-GEN-04** | Pembayaran tagihan hanya dapat dilakukan oleh orang tua | `StudentFinanceView.vue` menegaskan pembayaran dilakukan orang tua via Portal Orang Tua | **Lolos** |

---

## 5. Bukti Hasil Verifikasi (Terminal Outputs Asli)

### 5.1 Migrasi Database Lokal (Homebrew PostgreSQL)
```
$ migrate -path services/academic/migrations -database "postgres://localhost/skci_t7a_academic?sslmode=disable" down 1 && migrate -path services/academic/migrations -database "postgres://localhost/skci_t7a_academic?sslmode=disable" up
5/d transfers (20.669792ms)
5/u transfers (26.580209ms)

$ migrate -path services/billing/migrations -database "postgres://localhost/skci_t7a_billing?sslmode=disable" down 1 && migrate -path services/billing/migrations -database "postgres://localhost/skci_t7a_billing?sslmode=disable" up
4/d student_bill_visibility (22.612166ms)
4/u student_bill_visibility (21.351042ms)

$ migrate -path services/identity/migrations -database "postgres://localhost/skci_t7a_identity?sslmode=disable" down 1 && migrate -path services/identity/migrations -database "postgres://localhost/skci_t7a_identity?sslmode=disable" up
16/d deactivation_timestamp (15.859791ms)
16/u deactivation_timestamp (15.054584ms)
```

### 5.2 Backend Go Test & Vet
```
$ cd services/academic && go vet ./... && go test -v ./internal/handler -run "Test"
=== RUN   TestUpsertAdmissionEnrollmentPendingPlacement
--- PASS: TestUpsertAdmissionEnrollmentPendingPlacement (0.00s)
=== RUN   TestUpsertAdmissionEnrollmentIdempotentUpdatesClassroom
--- PASS: TestUpsertAdmissionEnrollmentIdempotentUpdatesClassroom (0.00s)
=== RUN   TestUpsertAdmissionEnrollmentRejectsOtherSchoolClassroom
--- PASS: TestUpsertAdmissionEnrollmentRejectsOtherSchoolClassroom (0.00s)
=== RUN   TestUpsertAdmissionEnrollmentRejectsOtherYearClassroom
--- PASS: TestUpsertAdmissionEnrollmentRejectsOtherYearClassroom (0.00s)
=== RUN   TestUpsertAdmissionEnrollmentRequiresIdentifiers
--- PASS: TestUpsertAdmissionEnrollmentRequiresIdentifiers (0.00s)
=== RUN   TestAC78_01_WithdrawalWithArrears
--- PASS: TestAC78_01_WithdrawalWithArrears (0.13s)
=== RUN   TestAC78_02_UnitTransferWithoutHistoryLoss
--- PASS: TestAC78_02_UnitTransferWithoutHistoryLoss (0.02s)
=== RUN   TestAC78_03_WithdrawalDepositRefund
--- PASS: TestAC78_03_WithdrawalDepositRefund (0.02s)
=== RUN   TestIdempotentApply
--- PASS: TestIdempotentApply (0.02s)
PASS
ok  	skci/academic/internal/handler	0.650s

$ cd services/academic && go test -v ./internal/model
=== RUN   TestValidateCreateStatusRequest
--- PASS: TestValidateCreateStatusRequest (0.00s)
=== RUN   TestIsValidStatusTransition
--- PASS: TestIsValidStatusTransition (0.00s)
=== RUN   TestDetermineApprovalFlags
--- PASS: TestDetermineApprovalFlags (0.00s)
=== RUN   TestShouldApplyImmediately
--- PASS: TestShouldApplyImmediately (0.00s)
PASS
ok  	skci/academic/internal/model	0.316s

$ cd services/billing && go vet ./... && go test -v ./internal/handler -run "TestStudentBillVisibility|TestInternalStudent"
=== RUN   TestStudentBillVisibility_AC82_01_SD_Forbidden
--- PASS: TestStudentBillVisibility_AC82_01_SD_Forbidden (0.01s)
=== RUN   TestStudentBillVisibility_AC82_02_SMK_Allowed
--- PASS: TestStudentBillVisibility_AC82_02_SMK_Allowed (0.02s)
=== RUN   TestAdminStudentBillVisibility
--- PASS: TestAdminStudentBillVisibility (0.01s)
=== RUN   TestInternalStudentExit_VoidPolicy
--- PASS: TestInternalStudentExit_VoidPolicy (0.01s)
=== RUN   TestInternalStudentRefund_Idempotency
--- PASS: TestInternalStudentRefund_Idempotency (0.01s)
PASS
ok  	skci/billing/internal/handler	0.304s

$ cd services/identity && go vet ./... && go test -v ./internal/handler -run "TestInternalDeactivateUser"
=== RUN   TestInternalDeactivateUser
--- PASS: TestInternalDeactivateUser (0.03s)
PASS
ok  	skci/identity/internal/handler	0.339s
```

### 5.3 Frontend TypeScript & Vite Build
```
$ cd web && npx vue-tsc -b && npm run build
> web@0.0.0 build
> vue-tsc -b && vite build

dist/assets/BillingSettingsView-EZDFB4ih.js               5.44 kB │ gzip:   2.12 kB
dist/assets/StudentStatusRequestsView-DBnMXd1B.js        11.79 kB │ gzip:   3.89 kB
dist/assets/StudentStatusRequestDetailView-rJvjcqIj.js   12.24 kB │ gzip:   3.71 kB
dist/assets/StudentStatusRequestFormView-D_PPc1zA.js     13.25 kB │ gzip:   4.20 kB
dist/assets/index-V8hZBstW.js                            88.10 kB │ gzip:  27.88 kB
✓ built in 1.31s
```

---

## 6. Catatan Integrasi Lintas-Task & Asumsi
1. **Approval Timeline (T1 Core):** Sesuai aturan mode paralel (PARALLEL.md §7b), `StudentStatusRequestDetailView.vue` menampilkan badge penanda status approval sementara `[ApprovalTimeline: T1]`. Sambungkan komponen visual `ApprovalTimeline` dari T1 setelah rebase/merge gelombang 1.
2. **Deaktivasi User (T2 HR/Identity):** Sesuai spesifikasi task T7 §1.2 butir 3, endpoint `POST /internal/users/{id}/deactivate` telah diimplementasikan pada `services/identity` dan didukung oleh migrasi `0016_deactivation_timestamp`. Jika T2 juga memperkenalkan endpoint serupa, kontrak keduanya identik (idempotent UPDATE `status='inactive'` dan `deactivated_at = now()`).
3. **Penyimpanan Nilai Uang:** Seluruh nilai rupiah menggunakan tipe `bigint` / `int64` (kolom `amount_idr`, `refund_amount_idr`, `net_idr`, `outstanding_idr`) tanpa `float` sesuai aturan wajib GEMINI.md §4.
