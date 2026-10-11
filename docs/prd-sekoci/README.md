# PRD SEKOCI v1.0 → skci-teratai: Peta Gap & Urutan Task

Sumber: `source/Master_PRD_SaaS_SEKOCI_Bagian1_v1.0.md` (menu 01–82) dan `source/Master_PRD_SaaS_SEKOCI_Bagian2_v1.0.md` (menu 83–101).
Bagian 3–4 (menu 102–159) **tidak ada di zip**, jadi belum dipetakan.

Aturan kerja: `../../GEMINI.md`. Pengerjaan kode oleh Gemini, review oleh Claude.

## 1. Status per menu

Legenda: **Ada** = sudah ada menu + backend · **Sebagian** = ada, tetapi FR tertentu belum · **Belum** = belum ada sama sekali.
Status 01–77 di bawah masih kasar (dari nama menu dan route). Audit FR per FR dikerjakan di **T8**.

| Menu | Status | Lokasi sekarang / catatan | Task |
|---|---|---|---|
| 01 Organization & Unit | Ada | identity: Data Yayasan, Data Sekolah, Organisasi | T8 |
| 02 Academic Year | Ada | identity/academic: Tahun Akademik | T8 |
| 03 Master Data | Ada | Data Siswa/Guru/Kelas/Mapel. Sinkron dari HRIS (BR-HR-01) masuk T2 | T2, T8 |
| 04 Data Import & Migration | Sebagian | import siswa/sekolah ada, tetapi belum ada import generik + laporan galat | T8 |
| 05 User & Access | Ada | identity: Data Pengguna, Jabatan/Peran. Peran baru ditambahkan di T1 | T1 |
| 06 Audit Trail | Sebagian | admission & billing punya tabel audit sendiri, tetapi belum ada log pusat, log keamanan, dan menu auditor | **T1** |
| 07 Notification | Ada | service notification (in-app/email, template, preferensi). Kanal WhatsApp dicek di T8 | T8 |
| 08–16 PPDB | Ada | service admission (lengkap: gelombang, jalur, berkas, seleksi, daftar ulang, konversi) | T8 |
| 17–25 Finance | Sebagian | service billing: skema, tagihan, diskon, aging, beasiswa, refund. Mapping COA per komponen (BR-FIN-06) dan deposit (BR-FIN-04) dicek di T8 | T8 |
| 26 Payment Dashboard | Sebagian | `PaymentDashboardView.vue` ada, tetapi belum ada data gateway | **T6** |
| 27 Checkout | Belum | baru ada upload bukti transfer manual, belum ada VA/QRIS/e-wallet | **T6** |
| 28 Payment Method | Belum | — | **T6** |
| 29 Transaction | Sebagian | `bill_payments` manual, verifikasi admin. Belum ada webhook, kuitansi otomatis, kasir | **T6** |
| 30 Settlement | Belum | — | **T6** |
| 31 Reconciliation | Sebagian | menu Rekonsiliasi Bank ada. Rekonsiliasi settlement gateway belum | **T6** |
| 32–38 Accounting | Ada | Laravel accounting (SSO, iframe). Posting otomatis dari billing/payroll dicek di T3/T8 | T3, T8 |
| 39–43 Operational | Ada | academic + attendance (kelas, wali, presensi, izin, rekap) | T8 |
| 44–50 Parent Portal | Ada | parent routes, klaim anak, tagihan, presensi, nilai, chat | T8 |
| 51–53 Executive | Sebagian | Dashboard Yayasan ada. Konsolidasi dicek di T8 | T8 |
| 54–64 Student Portal | Sebagian | jadwal, ujian, nilai, presensi ada. Materi/tugas (academic 0004_learning) masih uncommitted | T8 |
| 65–77 Teacher Portal | Ada | presensi, RPM, prota/prosem, ujian, nilai, rapor | T8 |
| 78 Transfer & Withdrawal | Belum | — | **T7** |
| 79 Approval Center | Belum | approval masih tersebar (izin di attendance, keputusan di admission) | **T1** |
| 80 Integration Exception Queue | Belum | — | **T1** |
| 81 Permintaan Layanan Ortu | Belum | — | **T7** |
| 82 Tagihan Saya | Sebagian | siswa `/me/bills` ada, tetapi belum ada saklar per jenjang | **T7** |
| 83–87 HRIS | Belum | — | **T2** |
| 88–92 ATS | Belum | — | **T4** |
| 93–98 Payroll | Belum | — | **T3** |
| 99–101 Pelatihan | Belum | — | **T5** |

## 2. Fitur yang sudah ada di proyek lain (`~/Documents/LMS`)

| Task | Ambil dari | Stack sumber | Yang diambil |
|---|---|---|---|
| T1 Audit | `ATS-TIA/backend/internal/platform/audit/`, `platform/listing/` | chi + pgx (sama) | pola tulis audit + query list/filter |
| T2 HRIS | `virelis/backend/internal/handlers/{employees,leave,attendance}.go`, `virelis/.../models/models.go` | gin + GORM | model pegawai, saldo cuti, hari kerja, koreksi absensi, org chart |
| T2 HRIS | `hris-mulnco/backend/internal/{domain,usecase}/employee`, `.../attendance` | gin + GORM, clean arch | enkripsi kolom AES-GCM + blind index NIK/NPWP (`usecase/crypto.go`) |
| T3 Payroll | `hris-mulnco/backend/internal/usecase/payroll/{pph21.go,pph21_test.go,payroll.go,calculation_test.go}` | gin + GORM, decimal | **PPh 21 TER (PMK 168/2023) + progresif Pasal 17 + rekonsiliasi Desember**, beserta test |
| T3 Payroll | `virelis/backend/internal/utils/payroll.go` (`CalcBPJS`), `handlers/payroll.go` (`GeneratePayrollRun`, `PayrollTransition`, `buildPayslipPDF`, `BankTransferExport`, `TaxSummary`) | gin + GORM, float | BPJS, state machine run, PDF slip, file bank, rekap pajak |
| T4 ATS | `ATS-TIA/backend/internal/{joborder,careers,applicant,pipeline,interview,assessment,decision}` | **chi + pgx (sama)** | requisition, halaman karier, formulir lamaran, pipeline + otomasi tahap, jadwal wawancara, scorecard, offer |
| T4 ATS (UI) | `ATS-TIA/frontend/src` | React | alur layar dan field saja (UI ditulis ulang ke Vue) |
| T5 Pelatihan | `inquira-vue-golang/backend/internal/handlers/{courses,enrollment,progress,quiz,certificates,learning_path}.go` | gin + sqlx | model kursus/enrol/progres/kuis/sertifikat |
| T6 Payment | `senb-v1/backend/internal/payment/nicepay/{client.go,client_test.go}`, `senb-v1/backend/internal/handler/payment_handler.go` | gin + GORM | **klien NICEPay SNAP: VA, QRIS, e-wallet, status, cancel, verifikasi callback** |

Fitur yang tidak punya donor (T1 Approval/Exception, T7) dibuat mengikuti pola service Sekoci sendiri (`billing`, `admission`).

## 3. Urutan & ketergantungan

```
T1 core (approval + exception + audit + peran baru)   ← wajib pertama, dipakai semua task
 ├─ T2 HRIS ─┬─ T3 Payroll
 │           ├─ T4 ATS ──┐
 │           └───────────┴─ T5 Pelatihan & Onboarding
 ├─ T6 Payment gateway (billing)
 └─ T7 Transfer/Withdrawal, Layanan Ortu, Tagihan Saya
T8 audit gap menu 01–77 (laporan saja, boleh paralel kapan pun)
```

T2–T5 berada di satu service baru `hr`. Supaya bisa paralel, nomor migrasi dicadangkan per task (lihat `PARALLEL.md`) dan setiap task punya paket Go sendiri (`internal/payroll`, `internal/ats`, `internal/training`). T3/T4/T5 tetap menunggu T2 di-merge karena butuh tabel pegawai & jabatan.

**Mode paralel (disarankan):** lihat `PARALLEL.md`. Gelombang 1 = T1, T2, T6, T7a, T8. Gelombang 2 = T3, T4, T5, T7b. Satu prompt Gemini menjalankan semuanya (`PARALLEL.md` § Prompt tunggal); Claude me-review **sekali di akhir** di branch `integrasi/prd-v1`.

## 4. Siklus per task

1. Gemini membaca `GEMINI.md` + file task, lalu bekerja di branch `feat/T0X-<slug>`.
2. Gemini menulis `handoff/T0X.md` (format di GEMINI.md §7).
3. Claude me-review diff branch + handoff dengan checklist GEMINI.md §8. Temuan ditulis ke `handoff/T0X-review.md`.
4. Gemini memperbaiki, lalu Claude me-review ulang. Selesai jika semua checklist lolos.

## 5. Keputusan yang masih terbuka (jangan dikarang, tanyakan ke pemilik)

Daftar ini disalin dari PRD dan berdampak langsung ke kode:
- OQ-19/20: angka TER PPh 21, PTKP, BPJS. Seed dari regulasi resmi, ditandai "perlu validasi konsultan pajak". Angka tidak dikarang.
- OQ-22: format file transfer bank. Dibuat sebagai template kolom yang bisa dikonfigurasi. Jangan hardcode satu bank.
- OQ-23: lokasi saat check-in pegawai. Buat saklar per sekolah, default mati.
- OQ-25: masa simpan data kandidat. Buat parameter per sekolah, default 6 bulan.
- OQ-07: perlakuan tagihan setelah tanggal keluar siswa. Buat parameter per sekolah (`void` | `keep`), default `void`.
