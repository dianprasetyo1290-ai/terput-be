# Master PRD — SEKOCI Integrated School Operating Platform (SaaS)

**Bagian 1 dari 4 · Kerangka produk dan modul inti (menu 01–82)**

Product Requirements Document untuk produk SaaS SEKOCI: ruang lingkup 33 grup menu dan 159 menu, kebutuhan fungsional, aturan bisnis, kriteria penerimaan Given/When/Then, kebutuhan non-fungsional, dan roadmap phase.

| Atribut | Nilai |
|---|---|
| Document ID | SEKOCI-MPRD-001 |
| Versi | 1.0 — Bagian 1 (draft) |
| Tanggal | 10/10/2026 |
| Status | Draft, menunggu review |
| Produk | SEKOCI Integrated School Operating Platform (SaaS, single-tenant per klien) |
| Product Owner | David Pambudi |
| Project Manager | David Pambudi |
| Disusun oleh | PT FullCircle Solusi Indonesia |
| Klasifikasi | Rahasia — internal tim |

## Daftar Isi

- [Kontrol Dokumen](#kontrol-dokumen)
  - [Riwayat versi](#riwayat-versi)
  - [Struktur dokumen](#struktur-dokumen)
  - [Log keputusan](#log-keputusan)
- [1. Ringkasan Eksekutif](#1-ringkasan-eksekutif)
- [2. Latar Belakang & Masalah](#2-latar-belakang--masalah)
- [3. Tujuan & KPI](#3-tujuan--kpi)
- [4. Prinsip Produk](#4-prinsip-produk)
- [5. Standar, Konvensi & Definisi](#5-standar-konvensi--definisi)
  - [5.1 Mutu kebutuhan](#51-mutu-kebutuhan)
  - [5.2 Konvensi](#52-konvensi)
  - [5.3 Definisi](#53-definisi)
- [6. Ruang Lingkup & Katalog Menu](#6-ruang-lingkup--katalog-menu)
  - [6.1 Prinsip pembagian scope](#61-prinsip-pembagian-scope)
  - [6.2 Grup menu](#62-grup-menu)
  - [6.3 Katalog 159 menu](#63-katalog-159-menu)
- [7. Arsitektur Portal & Model Peran](#7-arsitektur-portal--model-peran)
  - [7.1 Portal](#71-portal)
  - [7.2 Peran](#72-peran)
- [8. Entitas Data Inti & System of Record](#8-entitas-data-inti--system-of-record)
  - [8.1 Batas system of record](#81-batas-system-of-record)
- [9. Aturan Bisnis](#9-aturan-bisnis)
- [10. Kebutuhan Lintas Modul](#10-kebutuhan-lintas-modul)
- [11. Kebutuhan Fungsional per Grup Menu & Menu](#11-kebutuhan-fungsional-per-grup-menu--menu)
  - [Grup Menu 1 — Platform Foundation](#grup-menu-1--platform-foundation)
  - [Grup Menu 2 — PPDB](#grup-menu-2--ppdb)
  - [Grup Menu 3 — Finance](#grup-menu-3--finance)
  - [Grup Menu 4 — Payment](#grup-menu-4--payment)
  - [Grup Menu 5 — Accounting](#grup-menu-5--accounting)
  - [Grup Menu 6 — Operational](#grup-menu-6--operational)
  - [Grup Menu 7 — Parent Portal](#grup-menu-7--parent-portal)
  - [Grup Menu 8 — Executive Dashboard](#grup-menu-8--executive-dashboard)
  - [Grup Menu 9 — Student Portal](#grup-menu-9--student-portal)
  - [Grup Menu 10 — Teacher Portal](#grup-menu-10--teacher-portal)
  - [Grup Menu 11 — Core Extensions](#grup-menu-11--core-extensions)
  - [Grup Menu 12–33](#grup-menu-1233)
- [12. Alur Bisnis End-to-End](#12-alur-bisnis-end-to-end)
- [13. Integrasi & Antarmuka](#13-integrasi--antarmuka)
- [14. Migrasi & Kualitas Data](#14-migrasi--kualitas-data)
- [15. Kebutuhan Non-Fungsional](#15-kebutuhan-non-fungsional)
- [16. Keamanan, Privasi & Kendala](#16-keamanan-privasi--kendala)
  - [15.1 Kontrol & segregation of duties](#151-kontrol--segregation-of-duties)
  - [15.2 Kendala desain & kepatuhan](#152-kendala-desain--kepatuhan)
- [17. Pelaporan, Analitik & Governance Metrik](#17-pelaporan-analitik--governance-metrik)
- [18. Roadmap Phase & Release Gate](#18-roadmap-phase--release-gate)
  - [18.1 Roadmap phase](#181-roadmap-phase)
  - [18.2 Release gate (berlaku untuk setiap phase)](#182-release-gate-berlaku-untuk-setiap-phase)
- [19. Verifikasi, Traceability & Definition of Ready/Done](#19-verifikasi-traceability--definition-of-readydone)
  - [18.1 Metode verifikasi](#181-metode-verifikasi)
  - [18.2 Klasifikasi cacat](#182-klasifikasi-cacat)
  - [18.3 Rantai traceability](#183-rantai-traceability)
  - [18.4 Definition of Ready](#184-definition-of-ready)
  - [18.5 Definition of Done](#185-definition-of-done)
- [20. Asumsi, Dependensi & Risiko](#20-asumsi-dependensi--risiko)
  - [19.1 Asumsi](#191-asumsi)
  - [19.2 Dependensi](#192-dependensi)
  - [19.3 Risiko](#193-risiko)
- [21. Pertanyaan Terbuka](#21-pertanyaan-terbuka)
- [22. Persetujuan Dokumen](#22-persetujuan-dokumen)
- [Lampiran — Batas Tanggung Jawab Antar-Menu](#lampiran--batas-tanggung-jawab-antar-menu)

## Kontrol Dokumen

### Riwayat versi

| Versi | Tanggal | Pemilik | Perubahan |
|---|---|---|---|
| 1.0 — Bagian 1 | 10/10/2026 | PT FullCircle Solusi Indonesia | Master PRD produk SaaS: kerangka umum, katalog 159 menu, kebutuhan menu 01–82, release gate G0–G5, roadmap phase 1–16 |

### Struktur dokumen

| Bagian | Isi |
|---|---|
| Bagian 1 | Kerangka umum produk (bab 1–10, 12–22) dan kebutuhan fungsional grup menu 1–11 (menu 01–82) |
| Bagian 2 | Grup menu 12–15: HRIS, Rekrutmen (ATS), Payroll, Pelatihan Pegawai (menu 83–101) |
| Bagian 3 | Grup menu 16–26: Inventory & Aset sampai PKL (menu 102–141) |
| Bagian 4 | Grup menu 27–33: QA & Akreditasi sampai Aplikasi White-label (menu 142–159) |

PRD implementasi per klien (misalnya PRD Implementasi Teratai Putih) mengambil subset menu dari dokumen ini dan menambahkan keputusan, konfigurasi, serta label scope khusus klien.

### Log keputusan

| ID | Keputusan | Dampak |
|---|---|---|
| D-01 | Bahasa Indonesia, istilah teknis tetap bahasa Inggris | Seluruh dokumen |
| D-02 | Dokumen dipisah: Master PRD SaaS SEKOCI dan PRD implementasi per klien | Struktur dokumen |
| D-03 | Model SaaS single-tenant: setiap yayasan klien di-deploy terpisah | NFR-TEN-01, SAD |
| D-04 | Label scope produk: Core dan Add-on | Bab 6 dan 11 |
| D-05 | Penomoran kebutuhan per menu 01–159 (contoh: FR-09-001) | Bab 11 dan Bagian 2–4 |
| D-06 | Accounting Engine menjadi mesin akuntansi; software akuntansi lama klien digantikan melalui migrasi dan parallel run | Grup menu 5, menu 04 |
| D-07 | Pembayaran minimal untuk tagihan PPDB tersedia di Phase 1 | Menu 27–29 |
| D-08 | Roadmap Phase 1–8 untuk modul inti dan Phase 9–16 untuk domain lanjutan | Bab 18 |
| D-09 | Satu model release gate G0–G5 berlaku untuk setiap phase | Bab 18 |
| D-10 | Approval terpusat dan dapat dikonfigurasi lintas modul | Menu 79, BR-GEN-09 |
| D-11 | Antrean exception di UI untuk semua kegagalan integrasi | Menu 80, BR-INT-01 |
| D-12 | Alur pindah dan keluar siswa termasuk Core | Menu 78, BR-STU-01–03 |
| D-13 | Permintaan layanan orang tua (Core) dan tampilan tagihan siswa (Add-on, dapat diaktifkan per sekolah) | Menu 81, 82 |
| D-14 | Semua domain lanjutan dirinci penuh per menu | Bagian 2–4 |
| D-15 | Role & Permission Matrix per menu sebagai dokumen terpisah | Bab 7 |
| D-16 | Format dokumen Word dan Markdown | — |

## 1. Ringkasan Eksekutif

SEKOCI adalah platform operasional sekolah berbasis SaaS untuk yayasan dan sekolah multi-unit. Platform menyatukan PPDB, keuangan sekolah, pembayaran digital, akuntansi, operasional akademik, layanan mandiri orang tua, siswa, dan guru, pelaporan eksekutif, serta domain lanjutan seperti HR, payroll, inventory, perpustakaan, fasilitas, transportasi, kantin, kesehatan, kesiswaan, akreditasi, analitik, dan 3D Campus.

Produk disusun dalam 33 grup menu dan 159 menu. Modul inti (Core) membentuk satu rantai data dari pendaftaran sampai laporan keuangan per unit; modul Add-on dapat diaktifkan per klien. Setiap yayasan klien berjalan di lingkungan terpisah (single-tenant) dengan konfigurasi sesuai kebijakannya.

## 2. Latar Belakang & Masalah

Banyak yayasan dan sekolah menjalankan operasional dengan beberapa aplikasi terpisah untuk akademik, akuntansi, dan pembayaran, ditambah pengecekan manual. Akibatnya data tidak terhubung, staf menginput dan mencocokkan data berulang kali, dan pimpinan tidak memiliki satu tampilan menyeluruh.

- PPDB, tagihan, dan pembayaran tidak terhubung otomatis.
- Pembayaran sebagian dan cicilan harus dicek manual.
- Jurnal penerimaan tidak terpisah otomatis per unit.
- HR, payroll, aset, dan layanan siswa lain dikelola di spreadsheet atau aplikasi terpisah.
- Orang tua harus memakai beberapa aplikasi dan kanal komunikasi.

## 3. Tujuan & KPI

| ID | Tujuan | Hasil yang diharapkan | KPI indikatif |
|---|---|---|---|
| G1 | Satu platform | Klien dapat menghentikan aplikasi terpisah yang digantikan | Jumlah sistem terpisah yang masih dipakai klien |
| G2 | PPDB digital end-to-end | PPDB berjalan dari pendaftaran sampai siswa aktif dengan tagihan | % pendaftar online |
| G3 | Keuangan terintegrasi | Tagihan, pembayaran, dan jurnal dalam satu rantai transaksi | % pembayaran terkonfirmasi otomatis; % jurnal otomatis; jam rekonsiliasi; selisih rekonsiliasi |
| G4 | Layanan mandiri | Orang tua, siswa, guru, dan pegawai menyelesaikan tugas rutin secara digital | % akun aktif per peran |
| G5 | Visibilitas | Pimpinan unit dan yayasan melihat KPI terpercaya | Adopsi dashboard; kesegaran data |
| G6 | Keterlacakan | Perubahan keuangan dan operasional dapat diaudit | Kelengkapan audit trail |
| G7 | Multi-unit | Unit dan yayasan beroperasi dengan pemisahan terkontrol | Laporan per unit dan konsolidasi tersedia |
| G8 | Produk dapat diskalakan ke banyak klien | Klien baru dapat diaktifkan melalui konfigurasi tanpa perubahan kode | Waktu onboarding klien baru; jumlah change request khusus klien |

Target angka KPI ditetapkan per klien setelah baseline diukur saat discovery.

## 4. Prinsip Produk

- Satu sumber data utama untuk data induk dan transaksi keuangan.
- Akses berbasis peran dan dibatasi per unit untuk setiap aksi yang dilindungi.
- Multi-unit sebagai perhatian utama arsitektur.
- Otomatisasi untuk proses billing, pembayaran, akuntansi, dan notifikasi yang deterministik.
- Layanan mandiri untuk alur kerja orang tua, siswa, dan guru.
- Data akuntansi yang sudah diposting dapat ditelusuri dan selalu seimbang.
- Perubahan material pada data induk, akses, dan keuangan dapat diaudit.
- Integrasi berbasis API untuk payment gateway, Accounting Engine, dan sistem lama.
- Aturan sekolah dikonfigurasi, bukan ditulis tetap di kode.
- Pengalaman pengguna enterprise SaaS yang ringan, dengan status, validasi, dan tampilan responsif yang jelas.
- Satu produk untuk banyak klien: perbedaan antar sekolah diatasi dengan konfigurasi, bukan versi kode terpisah.
- Setiap modul dapat diaktifkan per klien sesuai paket (Core / Add-on).

## 5. Standar, Konvensi & Definisi

### 5.1 Mutu kebutuhan

Dokumen ini mengikuti struktur rekayasa kebutuhan yang selaras dengan ISO/IEC/IEEE 29148:2018: aktor yang jelas, kebutuhan fungsional, aturan bisnis, pertimbangan data dan antarmuka, kebutuhan mutu, traceability, dan kriteria penerimaan yang dapat diverifikasi. Pernyataan ini adalah keselarasan, bukan klaim sertifikasi.

| Mutu | Penerapan |
|---|---|
| Benar | Setiap kebutuhan menggambarkan perilaku produk atau kendala yang dimaksud |
| Tidak ambigu | Ditulis sebagai "Sistem harus …" dengan aktor dan cakupan yang jelas |
| Tunggal | Satu pernyataan memuat satu kebutuhan; aturan bersama ditulis sekali di bab 9 dan dirujuk |
| Konsisten | Konvensi status, peran, unit, dan ID yang sama di seluruh dokumen |
| Dapat diverifikasi | Pernyataan terukur dan kriteria Given/When/Then |
| Dapat ditelusuri | ID kebutuhan terhubung ke tujuan, test case, dan UAT |

### 5.2 Konvensi

| Konvensi | Arti |
|---|---|
| FR-[menu]-[urut] | Kebutuhan fungsional, contoh FR-09-006 = menu 09, kebutuhan ke-6 |
| AC-[menu]-[urut] | Kriteria penerimaan (Given/When/Then) untuk menu tersebut |
| CC-xx / AC-CC-xx | Kebutuhan dan kriteria lintas modul yang berlaku untuk semua menu |
| BR-[domain]-xx | Aturan bisnis: GEN, PPDB, FIN, PAY, ACC, ATT |
| IF-xx / NFR-xx | Kebutuhan antarmuka / non-fungsional |
| Prio | M = Must (wajib untuk phasenya); S = Should (penting, dapat bergeser); C = Could (opsional) |
| Scope | Core = termasuk cakupan sekolah klien sesuai penawaran; Add-on = modul tambahan dengan penawaran terpisah |
| Phase | Phase 1–16 sesuai bab 18; "—" = belum dijadwalkan |
| [ … ] | Nilai TBD atau rujukan ke pertanyaan terbuka (OQ) di bab 20 |
| Verif. | T = Test, D = Demonstration, I = Inspection, A = Analysis |

### 5.3 Definisi

| Istilah | Arti |
|---|---|
| PPDB | Penerimaan Peserta Didik Baru |
| Unit | Satuan pendidikan/cabang di bawah yayasan (SD, SMP, SMK), dengan entitas akuntansi sendiri |
| Pendaftar | Calon siswa yang terdaftar di PPDB dan belum berstatus siswa aktif |
| Tagihan | Kewajiban pembayaran satu komponen biaya untuk satu siswa/pendaftar |
| Alokasi | Pembagian nominal satu pembayaran ke satu atau lebih tagihan |
| Payment gateway | Penyedia jasa pembayaran berlisensi Bank Indonesia yang memproses VA, QRIS, dan e-wallet |
| Accounting Engine | Sistem akuntansi existing milik FullCircle yang menjalankan menu di Grup Menu 5 |
| Settlement | Penerusan dana dari payment gateway ke rekening sekolah |
| COA | Chart of Accounts / bagan akun |
| Core | Cakupan yang termasuk harga langganan atau lisensi sesuai penawaran |
| Add-on | Modul tambahan di luar Core, ditawarkan terpisah |

## 6. Ruang Lingkup & Katalog Menu

### 6.1 Prinsip pembagian scope

| Kategori | Definisi |
|---|---|
| Core | Modul paket dasar yang membentuk rantai PPDB → billing → pembayaran → akuntansi per unit → absensi → portal orang tua → dashboard, beserta ekstensi inti (menu 78–81) |
| Add-on | Modul yang diaktifkan dan dijual terpisah per klien |
| Konfigurasi | Pengaturan di fitur yang sudah ada; tidak memerlukan perubahan kode |
| Change Request | Perubahan perilaku khusus klien yang tidak dapat dicapai melalui konfigurasi |

### 6.2 Grup menu

| Grup | Nama | Menu | Phase | Dirinci di |
|---|---|---|---|---|
| 1 | Platform Foundation | 01–07 | Phase 1 | Bagian 1 |
| 2 | PPDB | 08–16 | Phase 1 | Bagian 1 |
| 3 | Finance | 17–25 | Phase 2 | Bagian 1 |
| 4 | Payment | 26–31 | Phase 1–2 | Bagian 1 |
| 5 | Accounting | 32–38 | Phase 3 | Bagian 1 |
| 6 | Operational | 39–43 | Phase 4 | Bagian 1 |
| 7 | Parent Portal | 44–50 | Phase 5 | Bagian 1 |
| 8 | Executive Dashboard | 51–53 | Phase 6 | Bagian 1 |
| 9 | Student Portal | 54–64 | Phase 7 | Bagian 1 |
| 10 | Teacher Portal | 65–77 | Phase 8 | Bagian 1 |
| 11 | Core Extensions | 78–82 | Phase 1–7 | Bagian 1 |
| 12 | HRIS | 83–87 | Phase 9 | Bagian 2 |
| 13 | Rekrutmen (ATS) | 88–92 | Phase 14 | Bagian 2 |
| 14 | Payroll | 93–98 | Phase 9 | Bagian 2 |
| 15 | Pelatihan Pegawai | 99–101 | Phase 14 | Bagian 2 |
| 16 | Inventory, Pengadaan & Aset | 102–108 | Phase 11 | Bagian 3 |
| 17 | Ekstensi Akuntansi | 109–111 | Phase 11 | Bagian 3 |
| 18 | Perpustakaan | 112–115 | Phase 12 | Bagian 3 |
| 19 | Fasilitas | 116–118 | Phase 12 | Bagian 3 |
| 20 | Transportasi | 119–122 | Phase 12 | Bagian 3 |
| 21 | Kantin & Dompet Siswa | 123–126 | Phase 10 | Bagian 3 |
| 22 | Kesehatan Sekolah | 127–129 | Phase 13 | Bagian 3 |
| 23 | Konseling & Kesiswaan | 130–132 | Phase 13 | Bagian 3 |
| 24 | Event & Ekstrakurikuler | 133–136 | Phase 10 | Bagian 3 |
| 25 | Service Desk | 137–138 | Phase 10 | Bagian 3 |
| 26 | PKL | 139–141 | Phase 13 | Bagian 3 |
| 27 | QA & Akreditasi | 142–144 | Phase 13 | Bagian 4 |
| 28 | Pelaporan Pemerintah | 145–146 | Phase 13 | Bagian 4 |
| 29 | Komunikasi Lanjutan | 147–149 | Phase 15 | Bagian 4 |
| 30 | Ekstensi Pembayaran | 150–151 | Phase 15 | Bagian 4 |
| 31 | Analitik & AI | 152–154 | Phase 15 | Bagian 4 |
| 32 | 3D Campus | 155–158 | Phase 16 | Bagian 4 |
| 33 | Aplikasi White-label | 159 | Phase 15 | Bagian 4 |

### 6.3 Katalog 159 menu

Menu berlabel Campuran memiliki kebutuhan Core dan Add-on; label per kebutuhan tercantum di bab 11 dan Bagian 2–4.

| No | Menu | Grup menu | Scope | Phase | Dirinci di |
|---|---|---|---|---|---|
| 01 | Organization & Unit Management | 1. Platform Foundation | Core | Phase 1 | Bagian 1 |
| 02 | Academic Year Management | 1. Platform Foundation | Core | Phase 1 | Bagian 1 |
| 03 | Master Data | 1. Platform Foundation | Campuran | Phase 1 | Bagian 1 |
| 04 | Data Import & Migration | 1. Platform Foundation | Core | Phase 1 | Bagian 1 |
| 05 | User & Access Management | 1. Platform Foundation | Core | Phase 1 | Bagian 1 |
| 06 | Audit Trail | 1. Platform Foundation | Core | Phase 1 | Bagian 1 |
| 07 | Notification Management | 1. Platform Foundation | Core | Phase 1 | Bagian 1 |
| 08 | PPDB Dashboard | 2. PPDB | Core | Phase 1 | Bagian 1 |
| 09 | PPDB Configuration | 2. PPDB | Core | Phase 1 | Bagian 1 |
| 10 | Applicant List | 2. PPDB | Core | Phase 1 | Bagian 1 |
| 11 | Applicant Detail | 2. PPDB | Core | Phase 1 | Bagian 1 |
| 12 | Document Verification | 2. PPDB | Core | Phase 1 | Bagian 1 |
| 13 | Selection | 2. PPDB | Core | Phase 1 | Bagian 1 |
| 14 | Announcement | 2. PPDB | Core | Phase 1 | Bagian 1 |
| 15 | Re-registration | 2. PPDB | Core | Phase 1 | Bagian 1 |
| 16 | Parent PPDB Portal | 2. PPDB | Core | Phase 1 | Bagian 1 |
| 17 | Finance Dashboard | 3. Finance | Core | Phase 2 | Bagian 1 |
| 18 | Fee Structure | 3. Finance | Core | Phase 2 | Bagian 1 |
| 19 | Billing | 3. Finance | Core | Phase 2 | Bagian 1 |
| 20 | Student Billing Detail | 3. Finance | Core | Phase 2 | Bagian 1 |
| 21 | Payment Allocation | 3. Finance | Core | Phase 2 | Bagian 1 |
| 22 | Receivable | 3. Finance | Core | Phase 2 | Bagian 1 |
| 23 | Aging | 3. Finance | Core | Phase 2 | Bagian 1 |
| 24 | Scholarship | 3. Finance | Core | Phase 2 | Bagian 1 |
| 25 | Refund | 3. Finance | Core | Phase 2 | Bagian 1 |
| 26 | Payment Dashboard | 4. Payment | Core | Phase 2 | Bagian 1 |
| 27 | Checkout | 4. Payment | Core | Phase 1/2 | Bagian 1 |
| 28 | Payment Method | 4. Payment | Campuran | Phase 1 | Bagian 1 |
| 29 | Transaction | 4. Payment | Core | Phase 1/2 | Bagian 1 |
| 30 | Settlement | 4. Payment | Core | Phase 2 | Bagian 1 |
| 31 | Reconciliation | 4. Payment | Core | Phase 2/3 | Bagian 1 |
| 32 | Accounting Dashboard | 5. Accounting | Core | Phase 3 | Bagian 1 |
| 33 | Chart of Accounts (COA) | 5. Accounting | Core | Phase 3 | Bagian 1 |
| 34 | Journal | 5. Accounting | Core | Phase 3 | Bagian 1 |
| 35 | General Ledger | 5. Accounting | Core | Phase 3 | Bagian 1 |
| 36 | Trial Balance | 5. Accounting | Core | Phase 3 | Bagian 1 |
| 37 | Financial Reports | 5. Accounting | Core | Phase 3 | Bagian 1 |
| 38 | Consolidation | 5. Accounting | Core | Phase 3 | Bagian 1 |
| 39 | Class Management | 6. Operational | Campuran | Phase 4 | Bagian 1 |
| 40 | Teacher/Class Assignment | 6. Operational | Add-on | Phase 4 | Bagian 1 |
| 41 | Attendance | 6. Operational | Core | Phase 4 | Bagian 1 |
| 42 | Leave | 6. Operational | Campuran | Phase 4 | Bagian 1 |
| 43 | Attendance Report | 6. Operational | Campuran | Phase 4 | Bagian 1 |
| 44 | Parent Dashboard | 7. Parent Portal | Campuran | Phase 5 | Bagian 1 |
| 45 | My Children | 7. Parent Portal | Core | Phase 5 | Bagian 1 |
| 46 | Child Profile | 7. Parent Portal | Core | Phase 5 | Bagian 1 |
| 47 | Academic | 7. Parent Portal | Add-on | Phase 5 | Bagian 1 |
| 48 | Attendance & Leave | 7. Parent Portal | Core | Phase 5 | Bagian 1 |
| 49 | Parent Finance | 7. Parent Portal | Core | Phase 5 | Bagian 1 |
| 50 | Parent Information & Settings | 7. Parent Portal | Core | Phase 5 | Bagian 1 |
| 51 | Yayasan Dashboard | 8. Executive Dashboard | Core | Phase 6 | Bagian 1 |
| 52 | Unit Dashboard | 8. Executive Dashboard | Campuran | Phase 6 | Bagian 1 |
| 53 | Consolidated Dashboard | 8. Executive Dashboard | Campuran | Phase 6 | Bagian 1 |
| 54 | Student Dashboard | 9. Student Portal | Add-on | Phase 7 | Bagian 1 |
| 55 | Student Profile | 9. Student Portal | Add-on | Phase 7 | Bagian 1 |
| 56 | Schedule | 9. Student Portal | Add-on | Phase 7 | Bagian 1 |
| 57 | Learning Materials | 9. Student Portal | Add-on | Phase 7 | Bagian 1 |
| 58 | Assignment | 9. Student Portal | Add-on | Phase 7 | Bagian 1 |
| 59 | Examination | 9. Student Portal | Add-on | Phase 7 | Bagian 1 |
| 60 | Grade / Nilai | 9. Student Portal | Add-on | Phase 7 | Bagian 1 |
| 61 | Attendance | 9. Student Portal | Add-on | Phase 7 | Bagian 1 |
| 62 | Announcement | 9. Student Portal | Add-on | Phase 7 | Bagian 1 |
| 63 | Calendar | 9. Student Portal | Add-on | Phase 7 | Bagian 1 |
| 64 | Student Settings | 9. Student Portal | Add-on | Phase 7 | Bagian 1 |
| 65 | Teacher Dashboard | 10. Teacher Portal | Add-on | Phase 8 | Bagian 1 |
| 66 | Teacher Profile | 10. Teacher Portal | Add-on | Phase 8 | Bagian 1 |
| 67 | Teaching Schedule | 10. Teacher Portal | Add-on | Phase 8 | Bagian 1 |
| 68 | My Classes | 10. Teacher Portal | Campuran | Phase 4/8 | Bagian 1 |
| 69 | Attendance Management | 10. Teacher Portal | Campuran | Phase 4/8 | Bagian 1 |
| 70 | Learning Material | 10. Teacher Portal | Add-on | Phase 8 | Bagian 1 |
| 71 | Assignment | 10. Teacher Portal | Add-on | Phase 8 | Bagian 1 |
| 72 | Examination | 10. Teacher Portal | Add-on | Phase 8 | Bagian 1 |
| 73 | Grade Management | 10. Teacher Portal | Add-on | Phase 8 | Bagian 1 |
| 74 | Student Performance | 10. Teacher Portal | Add-on | Phase 8 | Bagian 1 |
| 75 | Announcement & Communication | 10. Teacher Portal | Add-on | Phase 8 | Bagian 1 |
| 76 | Teacher Reports | 10. Teacher Portal | Add-on | Phase 8 | Bagian 1 |
| 77 | Teacher Settings | 10. Teacher Portal | Add-on | Phase 8 | Bagian 1 |
| 78 | Transfer & Withdrawal Siswa | 11. Core Extensions | Core | Phase 2 | Bagian 1 |
| 79 | Approval Center | 11. Core Extensions | Core | Phase 1 | Bagian 1 |
| 80 | Integration Exception Queue | 11. Core Extensions | Core | Phase 1 | Bagian 1 |
| 81 | Permintaan Layanan Orang Tua | 11. Core Extensions | Core | Phase 5 | Bagian 1 |
| 82 | Tagihan Saya | 11. Core Extensions | Add-on | Phase 7 | Bagian 1 |
| 83 | Master Pegawai | 12. HRIS | Add-on | Phase 9 | Bagian 2 |
| 84 | Organisasi & Jabatan | 12. HRIS | Add-on | Phase 9 | Bagian 2 |
| 85 | Dokumen Pegawai | 12. HRIS | Add-on | Phase 9 | Bagian 2 |
| 86 | Absensi & Cuti Pegawai | 12. HRIS | Add-on | Phase 9 | Bagian 2 |
| 87 | Employee Self-Service | 12. HRIS | Add-on | Phase 9 | Bagian 2 |
| 88 | Permintaan Rekrutmen | 13. Rekrutmen (ATS) | Add-on | Phase 14 | Bagian 2 |
| 89 | Lowongan & Publikasi | 13. Rekrutmen (ATS) | Add-on | Phase 14 | Bagian 2 |
| 90 | Pipeline Kandidat | 13. Rekrutmen (ATS) | Add-on | Phase 14 | Bagian 2 |
| 91 | Wawancara & Asesmen | 13. Rekrutmen (ATS) | Add-on | Phase 14 | Bagian 2 |
| 92 | Offer & Hiring | 13. Rekrutmen (ATS) | Add-on | Phase 14 | Bagian 2 |
| 93 | Komponen & Pengaturan Gaji | 14. Payroll | Add-on | Phase 9 | Bagian 2 |
| 94 | Input Payroll | 14. Payroll | Add-on | Phase 9 | Bagian 2 |
| 95 | Payroll Run & Approval | 14. Payroll | Add-on | Phase 9 | Bagian 2 |
| 96 | Slip Gaji | 14. Payroll | Add-on | Phase 9 | Bagian 2 |
| 97 | PPh 21 & BPJS | 14. Payroll | Add-on | Phase 9 | Bagian 2 |
| 98 | Jurnal Payroll | 14. Payroll | Add-on | Phase 9 | Bagian 2 |
| 99 | Katalog Pelatihan | 15. Pelatihan Pegawai | Add-on | Phase 14 | Bagian 2 |
| 100 | Onboarding Pegawai | 15. Pelatihan Pegawai | Add-on | Phase 14 | Bagian 2 |
| 101 | Penugasan & Penyelesaian Pelatihan | 15. Pelatihan Pegawai | Add-on | Phase 14 | Bagian 2 |
| 102 | Master Barang & Stok | 16. Inventory, Pengadaan & Aset | Add-on | Phase 11 | Bagian 3 |
| 103 | Stock Opname | 16. Inventory, Pengadaan & Aset | Add-on | Phase 11 | Bagian 3 |
| 104 | Purchase Request | 16. Inventory, Pengadaan & Aset | Add-on | Phase 11 | Bagian 3 |
| 105 | Purchase Order & Penerimaan | 16. Inventory, Pengadaan & Aset | Add-on | Phase 11 | Bagian 3 |
| 106 | Vendor | 16. Inventory, Pengadaan & Aset | Add-on | Phase 11 | Bagian 3 |
| 107 | Register Aset | 16. Inventory, Pengadaan & Aset | Add-on | Phase 11 | Bagian 3 |
| 108 | Penyusutan & Mutasi Aset | 16. Inventory, Pengadaan & Aset | Add-on | Phase 11 | Bagian 3 |
| 109 | Anggaran (RAPBS) & Realisasi | 17. Ekstensi Akuntansi | Add-on | Phase 11 | Bagian 3 |
| 110 | Pajak & e-Faktur | 17. Ekstensi Akuntansi | Add-on | Phase 11 | Bagian 3 |
| 111 | Utang Usaha Lanjutan | 17. Ekstensi Akuntansi | Add-on | Phase 11 | Bagian 3 |
| 112 | Katalog Perpustakaan | 18. Perpustakaan | Add-on | Phase 12 | Bagian 3 |
| 113 | Keanggotaan Perpustakaan | 18. Perpustakaan | Add-on | Phase 12 | Bagian 3 |
| 114 | Sirkulasi & Denda | 18. Perpustakaan | Add-on | Phase 12 | Bagian 3 |
| 115 | Laporan Perpustakaan | 18. Perpustakaan | Add-on | Phase 12 | Bagian 3 |
| 116 | Master Ruang & Fasilitas | 19. Fasilitas | Add-on | Phase 12 | Bagian 3 |
| 117 | Booking & Cek Bentrok | 19. Fasilitas | Add-on | Phase 12 | Bagian 3 |
| 118 | Permintaan Perawatan | 19. Fasilitas | Add-on | Phase 12 | Bagian 3 |
| 119 | Rute & Halte | 20. Transportasi | Add-on | Phase 12 | Bagian 3 |
| 120 | Kendaraan & Pengemudi | 20. Transportasi | Add-on | Phase 12 | Bagian 3 |
| 121 | Alokasi & Absensi Perjalanan | 20. Transportasi | Add-on | Phase 12 | Bagian 3 |
| 122 | Tagihan Transport | 20. Transportasi | Add-on | Phase 12 | Bagian 3 |
| 123 | Tenant Kantin | 21. Kantin & Dompet Siswa | Add-on | Phase 10 | Bagian 3 |
| 124 | Menu & POS | 21. Kantin & Dompet Siswa | Add-on | Phase 10 | Bagian 3 |
| 125 | Tabungan & Uang Saku Digital | 21. Kantin & Dompet Siswa | Add-on | Phase 10 | Bagian 3 |
| 126 | Laporan Transaksi Kantin | 21. Kantin & Dompet Siswa | Add-on | Phase 10 | Bagian 3 |
| 127 | Profil Kesehatan | 22. Kesehatan Sekolah | Add-on | Phase 13 | Bagian 3 |
| 128 | Kunjungan UKS/Klinik | 22. Kesehatan Sekolah | Add-on | Phase 13 | Bagian 3 |
| 129 | Imunisasi & Skrining | 22. Kesehatan Sekolah | Add-on | Phase 13 | Bagian 3 |
| 130 | Catatan Konseling | 23. Konseling & Kesiswaan | Add-on | Phase 13 | Bagian 3 |
| 131 | Poin & Pembinaan Disiplin | 23. Konseling & Kesiswaan | Add-on | Phase 13 | Bagian 3 |
| 132 | Rujukan & Tindak Lanjut | 23. Konseling & Kesiswaan | Add-on | Phase 13 | Bagian 3 |
| 133 | Manajemen Event | 24. Event & Ekstrakurikuler | Add-on | Phase 10 | Bagian 3 |
| 134 | Pendaftaran & Persetujuan Orang Tua | 24. Event & Ekstrakurikuler | Add-on | Phase 10 | Bagian 3 |
| 135 | Ekstrakurikuler | 24. Event & Ekstrakurikuler | Add-on | Phase 10 | Bagian 3 |
| 136 | Prestasi Siswa | 24. Event & Ekstrakurikuler | Add-on | Phase 10 | Bagian 3 |
| 137 | Tiket & Kategori | 25. Service Desk | Add-on | Phase 10 | Bagian 3 |
| 138 | SLA & Eskalasi | 25. Service Desk | Add-on | Phase 10 | Bagian 3 |
| 139 | Mitra & Lokasi PKL | 26. PKL | Add-on | Phase 13 | Bagian 3 |
| 140 | Penempatan & Absensi PKL | 26. PKL | Add-on | Phase 13 | Bagian 3 |
| 141 | Dokumentasi & Penilaian PKL | 26. PKL | Add-on | Phase 13 | Bagian 3 |
| 142 | Standar & Indikator | 27. QA & Akreditasi | Add-on | Phase 13 | Bagian 4 |
| 143 | Repositori Bukti | 27. QA & Akreditasi | Add-on | Phase 13 | Bagian 4 |
| 144 | Evaluasi Diri & Laporan Akreditasi | 27. QA & Akreditasi | Add-on | Phase 13 | Bagian 4 |
| 145 | Sinkronisasi Dapodik | 28. Pelaporan Pemerintah | Add-on | Phase 13 | Bagian 4 |
| 146 | Laporan Regulasi | 28. Pelaporan Pemerintah | Add-on | Phase 13 | Bagian 4 |
| 147 | WhatsApp Broadcast & Kampanye | 29. Komunikasi Lanjutan | Add-on | Phase 15 | Bagian 4 |
| 148 | Chat Guru–Orang Tua | 29. Komunikasi Lanjutan | Add-on | Phase 15 | Bagian 4 |
| 149 | Landing Page & Referral PPDB | 29. Komunikasi Lanjutan | Add-on | Phase 15 | Bagian 4 |
| 150 | Multi Payment Provider | 30. Ekstensi Pembayaran | Add-on | Phase 15 | Bagian 4 |
| 151 | Autodebit & Gerai Retail | 30. Ekstensi Pembayaran | Add-on | Phase 15 | Bagian 4 |
| 152 | BI & Dashboard Lanjutan | 31. Analitik & AI | Add-on | Phase 15 | Bagian 4 |
| 153 | AI Assistant | 31. Analitik & AI | Add-on | Phase 15 | Bagian 4 |
| 154 | Analitik Prediktif | 31. Analitik & AI | Add-on | Phase 15 | Bagian 4 |
| 155 | Peta Kampus 3D | 32. 3D Campus | Add-on | Phase 16 | Bagian 4 |
| 156 | Peta Kantin & Tenant | 32. 3D Campus | Add-on | Phase 16 | Bagian 4 |
| 157 | Peta Lab, Perpustakaan & Ruang Serbaguna | 32. 3D Campus | Add-on | Phase 16 | Bagian 4 |
| 158 | Facility Digital Twin | 32. 3D Campus | Add-on | Phase 16 | Bagian 4 |
| 159 | Aplikasi White-label | 33. Aplikasi White-label | Add-on | Phase 15 | Bagian 4 |

## 7. Arsitektur Portal & Model Peran

### 7.1 Portal

| Portal | Aktor utama | Tanggung jawab |
|---|---|---|
| Admin / Back Office | Super Admin, Yayasan Admin, School Admin, Finance, Accounting, PPDB, Academic | Administrasi dan kontrol |
| Parent Portal | Orang tua/wali, calon orang tua | PPDB, anak, kehadiran, keuangan, pembayaran, komunikasi |
| Student Portal (Add-on) | Siswa | Pembelajaran dan informasi akademik pribadi |
| Teacher Portal | Guru | Absensi (Core); pengajaran, materi, tugas, ujian, nilai (Add-on) |
| Executive / Yayasan | Pengurus Yayasan, Yayasan Admin, Kepala Sekolah | Pemantauan strategis dan laporan konsolidasi |

### 7.2 Peran

| Peran | Tujuan | Lingkup data |
|---|---|---|
| Super Admin | Konfigurasi platform dan administrasi keamanan | Seluruh platform |
| Yayasan Admin | Administrasi dan pelaporan tingkat yayasan | Semua unit |
| Pengurus Yayasan | Pemantauan eksekutif yayasan | Semua unit, hanya baca |
| School Admin / Unit Admin | Administrasi tingkat unit | Unitnya |
| Kepala Sekolah | Pemantauan eksekutif unit dan approval | Unitnya |
| PPDB Admin | Konfigurasi dan pemrosesan PPDB | Unitnya |
| Selection Committee | Penilaian dan approval hasil seleksi | Unitnya |
| Finance Admin | Operasional billing, piutang, kasir, dan pembayaran | Unitnya |
| Finance Manager | Pengawasan keuangan dan approval | Unit atau yayasan |
| Accounting | COA, jurnal, buku besar, neraca saldo, laporan, rekonsiliasi | Unit atau yayasan |
| Academic Admin | Kelas, penugasan guru, absensi, administrasi akademik | Unitnya |
| Teacher | Absensi kelas; layanan mandiri pengajaran (add-on) | Kelas/mapel yang ditugaskan |
| Parent / Guardian | Layanan mandiri keluarga | Anak yang tertaut |
| Student | Layanan mandiri siswa (add-on) | Data sendiri |
| Authorized Auditor | Membaca audit trail | Sesuai penugasan |

Peran untuk grup menu 12–33 (misalnya HR Admin, Payroll Officer, Librarian, Facility Officer, Transport Officer, Canteen Operator, Nurse, Counselor) didefinisikan di Bagian 2–4. Hak akses rinci per menu tercantum di dokumen Role & Permission Matrix.

## 8. Entitas Data Inti & System of Record

SEKOCI menjadi sumber data utama untuk organisasi, orang, PPDB, tagihan, pembayaran, absensi, dan pembelajaran. Accounting Engine menjadi sumber data utama untuk jurnal, buku besar, dan laporan keuangan.

| Entitas | Atribut / relasi utama | Sumber data utama |
|---|---|---|
| Organization / Unit | Yayasan → unit; entitas akuntansi; rekening settlement | SEKOCI |
| Academic Year / Semester | Periode per unit; satu aktif | SEKOCI |
| User / Role / Permission | Pengguna, peran, hak akses per modul, fitur, unit | SEKOCI |
| Student / Parent / Teacher / Staff | Siswa (calon/aktif/lulus/keluar); orang tua ↔ siswa (n:n) | SEKOCI |
| Class / Subject / Room | Kelas per unit dan tahun ajaran; wali kelas; mapel dan ruang (add-on) | SEKOCI |
| Applicant | Nomor pendaftaran, gelombang, jalur, dokumen, status; dikonversi menjadi Student | SEKOCI |
| Fee Component / Tariff / Promo | Komponen, tarif per dimensi, mapping COA; promo | SEKOCI |
| Invoice | Komponen, nominal, potongan, jatuh tempo, terbayar, status | SEKOCI |
| Payment / Allocation / Settlement / Reconciliation | Transaksi, alokasi ke tagihan, batch settlement, hasil pencocokan | SEKOCI |
| COA / Journal / Journal Line / Ledger | Akun, jurnal, baris jurnal, saldo per unit | Accounting Engine |
| Attendance / Leave | Status harian per siswa; pengajuan izin | SEKOCI |
| Assignment / Exam / Grade (add-on) | Tugas, ujian, nilai | SEKOCI |
| Notification / Announcement | Template, kanal, status baca | SEKOCI |
| Audit Log | Pengguna, waktu, modul, nilai sebelum dan sesudah | SEKOCI |
| Transfer / Withdrawal | Siswa, jenis, unit asal & tujuan, tanggal efektif, ringkasan keuangan, status approval | SEKOCI |
| Approval Request | Jenis, transaksi asal, tahap, penyetuju, keputusan, komentar, waktu | SEKOCI |
| Integration Exception | Sumber, correlation ID, galat, percobaan, status, penanggung jawab | SEKOCI |
| Service Request | Kategori, pengaju, unit, penanggung jawab, status, lampiran | SEKOCI |

### 8.1 Batas system of record

| Domain / data | Pemilik data (system of record) | Konsumen utama | Kontrol minimum |
|---|---|---|---|
| Organisasi, unit, tahun ajaran, pengguna, scope akses | SEKOCI Platform Foundation | Semua modul | ID stabil, RBAC di server, scope unit |
| Identitas & lifecycle siswa/orang tua | SEKOCI Student Master | PPDB, kelas, portal, keuangan, laporan | Pencegahan duplikasi, riwayat status, relasi orang tua–anak |
| Catatan akademik & kelas | SEKOCI Academic | Portal, absensi, laporan | Versi per tahun ajaran, otorisasi, riwayat perubahan |
| Struktur biaya, tagihan, piutang siswa | SEKOCI School Finance | Checkout, alokasi, portal orang tua, Accounting Engine | ID tagihan stabil, tanggal berlaku, aturan alokasi, saldo terekonsiliasi |
| Status & bukti transaksi pembayaran | SEKOCI Payment (bukti dari provider) | Keuangan, kuitansi, rekonsiliasi | Verifikasi webhook, idempotensi, referensi provider |
| COA, jurnal, buku besar, laporan keuangan | Accounting Engine | Keuangan, rekonsiliasi, laporan eksekutif | Mapping akun disetujui, posting tidak ganda, kontrol periode |
| Settlement provider / mutasi bank | File provider / bank, diproses SEKOCI Payment | Payment, Finance, Accounting | Control totals, exception queue, approval penyesuaian |
| Data dashboard | Read model analitik SEKOCI | Yayasan dan unit | Definisi metrik, freshness, scope akses, drilldown ke sumber |

Tidak ada dua modul yang berwenang mengubah fakta yang sama tanpa kontrak dan mekanisme rekonsiliasi.

## 9. Aturan Bisnis

Aturan ditulis sekali di sini dan dirujuk oleh kebutuhan fungsional. Nilai dalam [ ] dikonfigurasi per unit dan ditetapkan per klien.

| ID | Aturan |
|---|---|
| BR-GEN-01 | Pengguna hanya dapat mengakses data dalam lingkup unit yang ditugaskan, kecuali diberi akses tingkat yayasan secara eksplisit. |
| BR-GEN-02 | Hanya satu tahun ajaran/periode yang aktif per unit pada satu waktu. |
| BR-GEN-03 | Orang tua hanya dapat melihat anak yang tertaut ke akunnya. |
| BR-GEN-04 | Siswa hanya dapat melihat data akademik dan kehadirannya sendiri. |
| BR-GEN-05 | Guru hanya dapat mengelola kelas/mapel yang ditugaskan, kecuali didelegasikan. |
| BR-GEN-06 | Ekspor data sensitif mengikuti hak akses laporan sumbernya dan tercatat di audit trail. |
| BR-GEN-07 | Data bisnis yang memerlukan riwayat dinonaktifkan, bukan dihapus fisik. |
| BR-GEN-08 | Aturan sekolah (promo, alokasi, minimum daftar ulang, penanggung biaya, batas absensi) dikonfigurasi per unit, bukan ditulis tetap di kode. |
| BR-PPDB-01 | Promo hanya diterapkan jika tanggal penerbitan tagihan berada dalam masa berlaku promo dan kuotanya belum habis. Tagihan yang sudah terbit tidak berubah ketika promo berakhir. |
| BR-PPDB-02 | Jika lebih dari satu promo memenuhi syarat, sistem mengikuti aturan kombinasi (digabung atau dipilih yang terbesar) dengan batas maksimum potongan per pendaftar [per klien]. |
| BR-PPDB-03 | Diskon di luar aturan promo wajib disetujui kepala unit sebelum diterapkan dan tercatat di audit trail. |
| BR-PPDB-04 | Daftar ulang dianggap selesai ketika total pembayaran daftar ulang mencapai minimum yang dikonfigurasi [per klien]. |
| BR-PPDB-05 | Pendaftar yang selesai daftar ulang dikonversi otomatis menjadi siswa aktif dengan NIS baru, tercatat di unitnya, dan memicu billing tahunan (menu 19). |
| BR-PPDB-06 | Formulir pendaftaran aktif dan dapat diproses hanya setelah tagihan formulir lunas. |
| BR-FIN-01 | Status tagihan: belum bayar (terbayar = 0), sebagian (0 < terbayar < nilai), lunas (terbayar ≥ nilai), lewat jatuh tempo (belum lunas setelah tanggal jatuh tempo). |
| BR-FIN-02 | Tagihan yang dibayar sebagian tetap terbuka sampai lunas atau disesuaikan melalui approval. |
| BR-FIN-03 | Pembayaran tanpa pilihan tagihan dialokasikan ke tagihan dengan jatuh tempo paling awal [default, per klien]; alternatif: prioritas komponen atau pilihan orang tua. |
| BR-FIN-04 | Kelebihan bayar disimpan sebagai deposit dan dialokasikan otomatis ke tagihan berikutnya. |
| BR-FIN-05 | Koreksi dan pembatalan pembayaran memerlukan approval; transaksi asal tetap tersimpan dengan status dikoreksi/dibatalkan. |
| BR-FIN-06 | Komponen biaya wajib dipetakan ke akun pendapatan dan piutang unit sebelum dapat dipakai menerbitkan tagihan. |
| BR-PAY-01 | Notifikasi dari payment gateway diproses secara idempoten agar tidak terjadi transaksi ganda. |
| BR-PAY-02 | Penanggung biaya transaksi (orang tua atau sekolah) diatur per metode pembayaran [per klien]. |
| BR-ACC-01 | Jurnal yang sudah diposting tidak dapat diubah langsung; koreksi melalui jurnal pembalik atau penyesuaian. |
| BR-ACC-02 | Jurnal tidak dapat diposting kecuali total debit sama dengan total kredit. |
| BR-ACC-03 | Neraca saldo dihasilkan dari saldo buku besar yang sudah diposting untuk lingkup dan periode yang dipilih. |
| BR-ACC-04 | Konsolidasi mengeliminasi transaksi antar-unit yang sudah disetujui tanpa perhitungan ganda. |
| BR-ACC-05 | Setiap tagihan dan pembayaran dijurnal ke entitas unit siswa, menggunakan akun yang dipetakan di komponen biaya. |
| BR-ACC-06 | Posting ke Accounting Engine berupa ringkasan harian per unit dan per komponen dengan referensi ke transaksi rinci. Piutang per siswa dikelola SEKOCI; akun kontrol piutang dikelola Accounting Engine. |
| BR-ACC-07 | Tidak ada jurnal baru maupun perubahan pada periode yang sudah ditutup. |
| BR-ATT-01 | Koreksi absensi setelah dikunci memerlukan otorisasi dan tercatat di audit trail. |
| BR-GEN-09 | Semua approval dijalankan melalui Approval Center (menu 79). Pembuat permintaan tidak dapat menyetujui permintaannya sendiri pada tahap mana pun (maker-checker). |
| BR-STU-01 | Perubahan status siswa (pindah, keluar, lulus) tidak menghapus riwayat akademik maupun keuangan; semua perubahan tercatat dengan tanggal efektif. |
| BR-STU-02 | Pengajuan keluar memerlukan persetujuan keuangan atas tagihan terbuka, deposit, dan refund sebelum status berubah. |
| BR-STU-03 | Pada perpindahan antar unit, tagihan dan saldo sebelum tanggal efektif tetap tercatat di unit asal; tagihan sesudah tanggal efektif terbit di unit tujuan. |
| BR-INT-01 | Retry atas kegagalan integrasi tidak boleh membuat transaksi, alokasi, atau jurnal ganda; setiap item yang melewati batas retry masuk Integration Exception Queue (menu 80). |
| BR-ATT-02 | Izin yang disetujui mengubah status kehadiran siswa pada tanggal terkait menjadi izin atau sakit. |

## 10. Kebutuhan Lintas Modul

Kebutuhan berikut berlaku untuk semua menu di bab 11 dan tidak ditulis ulang per menu.

| ID | Kebutuhan | Prio |
|---|---|---|
| CC-01 | Sistem harus menampilkan setiap menu hanya kepada pengguna yang memiliki izin, dengan data dan aksi sesuai lingkup unitnya (BR-GEN-01). Pemeriksaan di sisi server diatur NFR-SEC-02; pencatatan audit diatur menu 06. | M |
| CC-02 | Sistem harus memvalidasi field wajib dan aturan bisnis sebelum menyimpan atau mengirim data, dan tidak menyimpan transaksi yang tidak valid. | M |
| CC-03 | Sistem harus menyimpan ID bisnis yang stabil, pembuat, pengubah, dan waktunya pada setiap record, serta unit pada record yang spesifik unit. | M |
| CC-04 | Sistem harus menerapkan BR-GEN-06 untuk setiap ekspor, dan ekspor harus mengikuti filter yang sedang aktif. | M |
| CC-05 | Sistem harus memungkinkan setiap pengguna mengubah kata sandi, melihat sesi aktif, dan keluar dari semua sesi. | M |
| CC-06 | Sistem harus menampilkan status memuat, data kosong, dan galat yang jelas pada setiap halaman. | M |
| CC-07 | Sistem harus menerapkan BR-GEN-07 untuk penghapusan data yang memiliki riwayat. | M |
| CC-09 | Sistem harus menyematkan correlation ID pada setiap alur yang melintasi beberapa modul (tagihan, transaksi, alokasi, jurnal) sehingga satu alur dapat ditelusuri dari awal sampai akhir. | M |
| CC-10 | Sistem harus menampilkan waktu pembaruan terakhir data (freshness timestamp) pada setiap dashboard dan laporan agregat. | S |
| CC-08 | Sistem harus memungkinkan setiap pengguna mengatur preferensi kanal notifikasinya; preferensi tersebut dihormati sesuai FR-07-006. | S |

**Kriteria penerimaan lintas modul**

| ID | Skenario | Given / When / Then |
|---|---|---|
| AC-CC-01 | Akses sah | **Given** pengguna memiliki izin untuk sebuah menu<br>**When** pengguna membuka menu tersebut<br>**Then** sistem menampilkan data dan aksi sesuai lingkup unit pengguna |
| AC-CC-02 | Validasi | **Given** data wajib kosong atau tidak valid<br>**When** pengguna menyimpan atau mengirim data<br>**Then** sistem menolak operasi, menunjukkan field bermasalah, dan tidak menyimpan transaksi tidak valid |
| AC-CC-03 | Akses tidak sah | **Given** pengguna tidak memiliki izin untuk sebuah aksi<br>**When** pengguna mencoba aksi tersebut, termasuk melalui URL atau API langsung<br>**Then** sistem menolak aksi dan tidak menampilkan data yang dilindungi |
| AC-CC-04 | Ekspor mengikuti filter | **Given** pengguna memfilter daftar pendaftar gelombang 1 jalur reguler<br>**When** pengguna mengekspor data<br>**Then** file hanya berisi data sesuai filter dan kegiatan ekspor tercatat di audit trail |
| AC-CC-05 | Preferensi notifikasi | **Given** pengguna menonaktifkan kanal email<br>**When** pengumuman baru dikirim<br>**Then** pengguna menerima pengumuman di kanal aktif lainnya, tetapi notifikasi tagihan tetap terkirim di semua kanal wajib |

## 11. Kebutuhan Fungsional per Grup Menu & Menu

Setiap menu memuat portal, peran, scope, phase, tujuan, fitur dan sub-fitur, kebutuhan fungsional, dan kriteria penerimaan. Kebutuhan lintas modul (bab 10) dan aturan bisnis (bab 9) berlaku tanpa ditulis ulang.

### Grup Menu 1 — Platform Foundation

*Phase 1 · seluruh menu Core*

#### 01. Organization & Unit Management

| Atribut | Nilai |
|---|---|
| **Portal** | Admin / Back Office |
| **Peran** | Super Admin; Yayasan Admin; School Admin |
| **Scope** | Core |
| **Phase** | Phase 1 |
| **Tujuan** | Mengelola hierarki yayasan dan unit/cabang sebagai dasar pemisahan data, transaksi, dan pelaporan. |
| **Fitur** | Organization, Foundation, Unit, Multi-unit |
| **Sub-fitur** | Create/edit unit, code/name/type, address/contact, head, activate/deactivate, hierarchy, configuration |

| ID | Kebutuhan | Prio | Scope | Phase |
|---|---|---|---|---|
| FR-01-001 | Sistem harus menyimpan hierarki yayasan → unit dengan kode, nama, jenis/jenjang (SD, SMP, SMK), alamat, kontak, dan kepala unit. | M | Core | Phase 1 |
| FR-01-002 | Sistem harus mengikat setiap data siswa dan setiap transaksi ke tepat satu unit. | M | Core | Phase 1 |
| FR-01-003 | Sistem harus menyimpan pengaturan per unit yang dipakai modul lain, minimal: entitas akuntansi terkait dan rekening settlement. | M | Core | Phase 1 |

| ID | Skenario | Given / When / Then |
|---|---|---|
| AC-01-01 | Transaksi terikat ke unit siswa | **Given** siswa "Andi" terdaftar di unit "SMP"<br>**When** tagihan untuk Andi diterbitkan<br>**Then** tagihan tercatat di unit "SMP" dan hanya muncul di laporan unit SMP serta konsolidasi yayasan |

#### 02. Academic Year Management

| Atribut | Nilai |
|---|---|
| **Portal** | Admin / Back Office |
| **Peran** | Super Admin; School Admin; Academic Admin |
| **Scope** | Core |
| **Phase** | Phase 1 |
| **Tujuan** | Mengelola tahun ajaran, semester, dan periode akademik per unit. |
| **Fitur** | Academic year, Semester, Academic period |
| **Sub-fitur** | Create year, dates, semester, active period, lock, history |

| ID | Kebutuhan | Prio | Scope | Phase |
|---|---|---|---|---|
| FR-02-001 | Sistem harus mengelola tahun ajaran dan semester per unit beserta tanggal mulai dan selesainya. | M | Core | Phase 1 |
| FR-02-002 | Sistem harus menerapkan BR-GEN-02 saat tahun ajaran diaktifkan. | M | Core | Phase 1 |
| FR-02-003 | Sistem harus mengunci periode akademik yang sudah selesai sehingga data di dalamnya hanya dapat diubah melalui otorisasi. | S | Core | Phase 1 |

| ID | Skenario | Given / When / Then |
|---|---|---|
| AC-02-01 | Hanya satu tahun ajaran aktif | **Given** tahun ajaran 2026/2027 aktif di unit "SD"<br>**When** admin mengaktifkan tahun ajaran 2027/2028 di unit "SD"<br>**Then** tahun ajaran 2026/2027 berubah menjadi tidak aktif |

#### 03. Master Data

| Atribut | Nilai |
|---|---|
| **Portal** | Admin / Back Office |
| **Peran** | Super Admin; School Admin; Academic Admin |
| **Scope** | Campuran (lihat per kebutuhan) |
| **Phase** | Phase 1 |
| **Tujuan** | Mengelola data induk orang dan referensi. Data kelas dikelola di menu 39. |
| **Fitur** | Student, Parent, Teacher, Staff, Class, Subject, Room, Program, Reference |
| **Sub-fitur** | CRUD, search/filter, import/export, validation, duplicate detection, history |

| ID | Kebutuhan | Prio | Scope | Phase |
|---|---|---|---|---|
| FR-03-001 | Sistem harus menyimpan data siswa dengan status calon, aktif, lulus, atau keluar. | M | Core | Phase 1 |
| FR-03-002 | Sistem harus memungkinkan satu akun orang tua/wali terhubung ke lebih dari satu siswa, termasuk siswa di unit berbeda. | M | Core | Phase 1 |
| FR-03-003 | Sistem harus menyimpan data guru dan staf beserta unit tugasnya. | M | Core | Phase 1 |
| FR-03-004 | Sistem harus menyimpan data program dan data referensi (agama, jenis kelamin, pekerjaan orang tua, dan sejenisnya). | M | Core | Phase 1 |
| FR-03-005 | Sistem harus mendeteksi kemungkinan duplikat siswa atau orang tua berdasarkan NIK/NISN dan nomor ponsel sebelum data disimpan. | S | Core | Phase 1 |
| FR-03-006 | Sistem harus menyimpan data mata pelajaran. Data ruang dikelola di menu 116. | M | Add-on | Phase 4 |

| ID | Skenario | Given / When / Then |
|---|---|---|
| AC-03-01 | Satu akun orang tua, dua anak di unit berbeda | **Given** "Ibu Sari" memiliki anak di unit "SD" dan unit "SMP"<br>**When** admin menautkan kedua anak ke akun Ibu Sari<br>**Then** Ibu Sari dapat melihat dan memilih kedua anaknya di portal orang tua |
| AC-03-02 | Deteksi duplikat | **Given** siswa dengan NISN 0012345678 sudah tersimpan<br>**When** admin menyimpan siswa baru dengan NISN yang sama<br>**Then** sistem menampilkan peringatan kemungkinan duplikat sebelum data disimpan |

#### 04. Data Import & Migration

| Atribut | Nilai |
|---|---|
| **Portal** | Admin / Back Office |
| **Peran** | Super Admin; School Admin |
| **Scope** | Core |
| **Phase** | Phase 1 |
| **Tujuan** | Memindahkan data dari sistem sekolah sebelumnya, software akuntansi sebelumnya, dan file Excel secara terkontrol. |
| **Fitur** | Bulk import, Data migration, Import validation |
| **Sub-fitur** | Excel/CSV, template, mapping, validation, error report, preview, history, rollback |

| ID | Kebutuhan | Prio | Scope | Phase |
|---|---|---|---|---|
| FR-04-001 | Sistem harus menerima import dari file Excel (.xlsx) dan CSV menggunakan template yang disediakan, dengan pemetaan kolom. | M | Core | Phase 1 |
| FR-04-002 | Sistem harus menampilkan preview hasil validasi sebelum data disimpan, dan menolak baris yang tidak valid dengan pesan kesalahan per nomor baris. | M | Core | Phase 1 |
| FR-04-003 | Sistem harus menyimpan riwayat setiap batch import dan memungkinkan rollback satu batch selama data batch tersebut belum dipakai transaksi. | S | Core | Phase 1 |
| FR-04-004 | Sistem harus mengimport data siswa, orang tua, dan kelas dari sistem sekolah sebelumnya. | M | Core | Phase 1 |
| FR-04-005 | Sistem harus mengimport COA, saldo awal, dan piutang berjalan per siswa dari software akuntansi sebelumnya per tanggal cutover. | M | Core | Phase 3 |
| FR-04-006 | Sistem harus mengimport riwayat pembayaran tahun berjalan dari payment aggregator sebelumnya bila diperlukan untuk saldo piutang [OQ-09]. | S | Core | Phase 2 |
| FR-04-007 | Sistem harus menyimpan pemetaan ID sumber ke ID baru sehingga setiap data hasil migrasi dapat ditelusuri ke sumbernya. | M | Core | Phase 1 |
| FR-04-008 | Sistem harus mendukung migrasi percobaan (trial) di lingkungan terpisah sebelum migrasi produksi. | M | Core | Phase 1 |
| FR-04-009 | Sistem harus menghasilkan control totals sebelum dan sesudah migrasi (jumlah dan nilai tagihan, pembayaran, alokasi, saldo piutang, jurnal) untuk direkonsiliasi dan disetujui pemilik data. | M | Core | Phase 1 |
| FR-04-010 | Sistem harus menyimpan data historis yang tidak dimigrasi sebagai arsip baca-saja yang dapat dicari, sesuai keputusan per kategori data. | S | Core | Phase 1 |

| ID | Skenario | Given / When / Then |
|---|---|---|
| AC-04-01 | Import dengan baris tidak valid | **Given** file import berisi 100 baris siswa dan 3 baris tanpa NIS<br>**When** admin mengimport file tersebut<br>**Then** preview menunjukkan 97 baris valid dan 3 baris ditolak beserta nomor barisnya<br>**And** setelah dikonfirmasi, hanya 97 baris yang tersimpan |
| AC-04-02 | Saldo awal dari software akuntansi sebelumnya | **Given** total piutang di software akuntansi sebelumnya per tanggal cutover Rp150.000.000<br>**When** data saldo awal diimport<br>**Then** total piutang per siswa di sistem sama dengan Rp150.000.000 |
| AC-04-03 | Control totals tidak cocok | **Given** total piutang sumber Rp150.000.000 dan hasil migrasi Rp149.500.000<br>**When** pemilik data meninjau hasil migrasi<br>**Then** sign-off migrasi tidak dapat diberikan sampai selisih Rp500.000 dijelaskan atau diperbaiki |

#### 05. User & Access Management

| Atribut | Nilai |
|---|---|
| **Portal** | Admin / Back Office |
| **Peran** | Super Admin |
| **Scope** | Core |
| **Phase** | Phase 1 |
| **Tujuan** | Mengelola pengguna, peran, dan hak akses per modul, fitur, dan unit. |
| **Fitur** | User, Role, Permission, Access control |
| **Sub-fitur** | Create/edit, activate, reset, role assignment, module/feature/unit permissions, session/login history |

| ID | Kebutuhan | Prio | Scope | Phase |
|---|---|---|---|---|
| FR-05-001 | Sistem harus memungkinkan admin membuat, mengaktifkan, menonaktifkan, dan mereset kata sandi pengguna. | M | Core | Phase 1 |
| FR-05-002 | Sistem harus menetapkan satu atau lebih peran dan satu atau lebih unit kepada setiap pengguna. | M | Core | Phase 1 |
| FR-05-003 | Sistem harus mengatur hak akses per modul dan per fitur untuk setiap peran. | M | Core | Phase 1 |
| FR-05-004 | Sistem harus menampilkan riwayat login dan sesi aktif setiap pengguna. | S | Core | Phase 1 |

| ID | Skenario | Given / When / Then |
|---|---|---|
| AC-05-01 | Staf keuangan tidak dapat melihat unit lain | **Given** staf keuangan hanya ditugaskan di unit "SD"<br>**When** staf tersebut membuka daftar tagihan<br>**Then** hanya tagihan unit "SD" yang ditampilkan |

#### 06. Audit Trail

| Atribut | Nilai |
|---|---|
| **Portal** | Admin / Back Office |
| **Peran** | Super Admin; Authorized auditor |
| **Scope** | Core |
| **Phase** | Phase 1 |
| **Tujuan** | Menyediakan jejak aktivitas pengguna, perubahan data, dan kejadian keamanan. |
| **Fitur** | User activity, Data change history, Security log |
| **Sub-fitur** | Login, create/update/delete, before/after, timestamp, user/module, export |

| ID | Kebutuhan | Prio | Scope | Phase |
|---|---|---|---|---|
| FR-06-001 | Sistem harus mencatat setiap perubahan data keuangan, konfigurasi biaya, promo, hak akses, dan approval: pengguna, waktu, modul, nilai sebelum, dan nilai sesudah. | M | Core | Phase 1 |
| FR-06-002 | Sistem harus mencatat kejadian keamanan: login berhasil, login gagal, penguncian akun, dan perubahan hak akses. | M | Core | Phase 1 |
| FR-06-003 | Sistem harus menyediakan pencarian dan filter log berdasarkan pengguna, modul, dan rentang waktu bagi peran auditor. | M | Core | Phase 1 |

| ID | Skenario | Given / When / Then |
|---|---|---|
| AC-06-01 | Perubahan tarif tercatat | **Given** tarif SPP unit "SMK" sebesar Rp500.000<br>**When** staf keuangan mengubah tarif menjadi Rp550.000<br>**Then** log mencatat pengguna, waktu, modul, nilai Rp500.000, dan nilai Rp550.000 |

#### 07. Notification Management

| Atribut | Nilai |
|---|---|
| **Portal** | Admin / Back Office |
| **Peran** | Super Admin; School Admin |
| **Scope** | Core |
| **Phase** | Phase 1 |
| **Tujuan** | Mesin notifikasi dan pengumuman untuk seluruh portal. Portal lain hanya menampilkan notifikasi yang dikirim dari sini. |
| **Fitur** | Notification, Announcement, Notification preference |
| **Sub-fitur** | In-app, email, payment, attendance, PPDB, approval, templates, read/unread, history |

| ID | Kebutuhan | Prio | Scope | Phase |
|---|---|---|---|---|
| FR-07-001 | Sistem harus mengirim notifikasi melalui notifikasi aplikasi, email, dan WhatsApp untuk setiap kejadian yang ditetapkan pada kebutuhan menu terkait. | M | Core | Phase 1 |
| FR-07-002 | Sistem harus memberi tahu penyetuju ketika ada permintaan approval, dan memberi tahu pengaju ketika permintaan disetujui atau ditolak. | M | Core | Phase 1 |
| FR-07-003 | Sistem harus menyediakan template notifikasi per jenis kejadian yang dapat diubah oleh admin. | M | Core | Phase 1 |
| FR-07-004 | Sistem harus memungkinkan admin membuat pengumuman untuk audiens tertentu (unit, kelas, atau peran). | M | Core | Phase 1 |
| FR-07-005 | Sistem harus menyimpan status baca/belum dibaca dan riwayat pengiriman setiap notifikasi. | S | Core | Phase 1 |
| FR-07-006 | Sistem harus menghormati preferensi kanal notifikasi yang diatur pengguna, kecuali untuk notifikasi wajib (tagihan dan keamanan). | S | Core | Phase 1 |

| ID | Skenario | Given / When / Then |
|---|---|---|
| AC-07-01 | Notifikasi permintaan approval | **Given** staf keuangan mengajukan pembatalan tagihan<br>**When** pengajuan dikirim<br>**Then** Finance Manager menerima notifikasi permintaan approval<br>**And** setelah disetujui, staf keuangan menerima notifikasi hasilnya |
| AC-07-02 | Template yang diubah langsung dipakai | **Given** admin mengubah template notifikasi jatuh tempo<br>**When** pengingat jatuh tempo berikutnya dikirim<br>**Then** isi notifikasi memakai template terbaru |

### Grup Menu 2 — PPDB

*Phase 1 · seluruh menu Core*

#### 08. PPDB Dashboard

| Atribut | Nilai |
|---|---|
| **Portal** | Admin / PPDB |
| **Peran** | PPDB Admin; School Admin; Yayasan Admin |
| **Scope** | Core |
| **Phase** | Phase 1 |
| **Tujuan** | Memantau perkembangan PPDB per unit, gelombang, dan jalur. |
| **Fitur** | Applicant summary, Registration monitoring, PPDB financial summary |
| **Sub-fitur** | Applicants by unit/path, trend, payment, verification, selection, re-registration, revenue, conversion |

| ID | Kebutuhan | Prio | Scope | Phase |
|---|---|---|---|---|
| FR-08-001 | Sistem harus menampilkan jumlah pendaftar per unit, gelombang, dan jalur beserta tren hariannya. | S | Core | Phase 1 |
| FR-08-002 | Sistem harus menampilkan jumlah pendaftar per tahap: daftar, bayar formulir, terverifikasi, diterima, dan selesai daftar ulang. | S | Core | Phase 1 |
| FR-08-003 | Sistem harus menampilkan ringkasan penerimaan PPDB (uang formulir dan daftar ulang) serta tingkat konversi PPDB. | S | Core | Phase 1 |

| ID | Skenario | Given / When / Then |
|---|---|---|
| AC-08-01 | Funnel PPDB | **Given** unit "SD" memiliki 120 pendaftar, 100 bayar formulir, 80 diterima, 60 selesai daftar ulang<br>**When** kepala unit membuka dashboard PPDB unit SD<br>**Then** dashboard menampilkan 120, 100, 80, dan 60 per tahap serta konversi 50% |

#### 09. PPDB Configuration

| Atribut | Nilai |
|---|---|
| **Portal** | Admin / PPDB |
| **Peran** | PPDB Admin |
| **Scope** | Core |
| **Phase** | Phase 1 |
| **Tujuan** | Mengatur gelombang, jalur, kuota, formulir, dokumen, biaya, dan promo PPDB tanpa perubahan kode. |
| **Fitur** | Admission path, Program & unit, Registration form, Promo & discount |
| **Sub-fitur** | Path, quota, unit/program, fields, required fields, documents, period/fee, promo/discount, eligibility |

| ID | Kebutuhan | Prio | Scope | Phase |
|---|---|---|---|---|
| FR-09-001 | Sistem harus mengelola gelombang pendaftaran per unit dan jenjang, masing-masing dengan tanggal buka dan tanggal tutup. | M | Core | Phase 1 |
| FR-09-002 | Sistem harus mengelola jalur pendaftaran per gelombang beserta syarat kelayakannya. | M | Core | Phase 1 |
| FR-09-003 | Sistem harus menerapkan kuota per unit, jenjang, dan jalur, dan menolak pendaftaran baru bila kuota penuh. | S | Core | Phase 1 |
| FR-09-004 | Sistem harus memungkinkan admin mengatur field formulir dan menandai field wajib. | M | Core | Phase 1 |
| FR-09-005 | Sistem harus mengelola daftar dokumen wajib per jalur untuk tahap pendaftaran dan tahap daftar ulang. | M | Core | Phase 1 |
| FR-09-006 | Sistem harus mengelola biaya formulir serta biaya daftar ulang dan uang pangkal per gelombang, jalur, dan unit. | M | Core | Phase 1 |
| FR-09-007 | Sistem harus mengelola promo dengan jenis potongan (nominal atau persen), komponen biaya yang dikenai, masa berlaku, syarat, dan kuota. | M | Core | Phase 1 |
| FR-09-008 | Sistem harus menerapkan BR-PPDB-01 dan BR-PPDB-02 setiap kali tagihan PPDB (formulir maupun daftar ulang) diterbitkan. | M | Core | Phase 1 |
| FR-09-009 | Sistem harus menerapkan promo berbasis kode voucher yang dimasukkan pendaftar. | S | Core | Phase 1 |
| FR-09-010 | Sistem harus menerapkan BR-PPDB-03 untuk diskon khusus di luar aturan promo. | M | Core | Phase 1 |

| ID | Skenario | Given / When / Then |
|---|---|---|
| AC-09-01 | Pendaftaran di luar periode gelombang | **Given** gelombang 1 unit "SD" dibuka 1 Oktober dan ditutup 30 November<br>**When** calon orang tua membuka formulir unit SD pada 1 Desember<br>**Then** gelombang 1 tidak dapat dipilih |
| AC-09-02 | Kuota jalur penuh | **Given** kuota jalur "prestasi" unit "SMP" 20 dan sudah terisi 20<br>**When** calon orang tua mendaftar di jalur tersebut<br>**Then** pendaftaran ditolak dengan pesan bahwa kuota penuh |
| AC-09-03 | Biaya formulir sesuai gelombang | **Given** biaya formulir gelombang 1 Rp200.000 dan gelombang 2 Rp300.000<br>**When** pendaftar mendaftar di gelombang 2<br>**Then** tagihan formulir terbit sebesar Rp300.000 |
| AC-09-04 | Promo lewat masa berlaku | **Given** promo "Early Bird" berlaku 1–31 Oktober<br>**When** tagihan daftar ulang diterbitkan pada 2 November<br>**Then** promo "Early Bird" tidak diterapkan |
| AC-09-05 | Tagihan lama tidak berubah setelah promo berakhir | **Given** tagihan diterbitkan 20 Oktober dengan promo "Early Bird"<br>**When** tanggal melewati 31 Oktober<br>**Then** nominal tagihan tersebut tidak berubah |
| AC-09-06 | Batas maksimum potongan | **Given** batas potongan per pendaftar Rp2.000.000 dan pendaftar memenuhi dua promo senilai total Rp2.500.000<br>**When** tagihan diterbitkan<br>**Then** total potongan yang diterapkan Rp2.000.000 |
| AC-09-07 | Kode voucher tidak valid | **Given** kode "OPENHOUSE" tidak terdaftar<br>**When** pendaftar memasukkan kode tersebut<br>**Then** sistem menolak kode dengan pesan "kode tidak valid" |
| AC-09-08 | Diskon khusus memerlukan approval | **Given** admin mengajukan diskon khusus Rp3.000.000 di luar aturan promo<br>**When** kepala unit belum menyetujui<br>**Then** diskon belum diterapkan<br>**And** setelah disetujui, diskon diterapkan dan approval tercatat di audit trail |

#### 10. Applicant List

| Atribut | Nilai |
|---|---|
| **Portal** | Admin / PPDB |
| **Peran** | PPDB Admin |
| **Scope** | Core |
| **Phase** | Phase 1 |
| **Tujuan** | Daftar kerja seluruh pendaftar untuk diproses admin PPDB. |
| **Fitur** | Applicant management, Search/filter |
| **Sub-fitur** | List, registration number, status, payment, unit/program/path, search, filter, export |

| ID | Kebutuhan | Prio | Scope | Phase |
|---|---|---|---|---|
| FR-10-001 | Sistem harus menampilkan daftar pendaftar dengan nomor pendaftaran, nama, unit, program, jalur, gelombang, status tahap, dan status pembayaran. | M | Core | Phase 1 |
| FR-10-002 | Sistem harus menyediakan pencarian berdasarkan nama atau nomor pendaftaran dan filter berdasarkan unit, gelombang, jalur, dan status. | M | Core | Phase 1 |
| FR-10-003 | Sistem harus mengekspor daftar pendaftar ke Excel. | S | Core | Phase 1 |

| ID | Skenario | Given / When / Then |
|---|---|---|
| AC-10-01 | Filter status pembayaran | **Given** terdapat 50 pendaftar, 12 di antaranya belum membayar formulir<br>**When** admin memfilter status "belum bayar formulir"<br>**Then** daftar menampilkan 12 pendaftar tersebut |

#### 11. Applicant Detail

| Atribut | Nilai |
|---|---|
| **Portal** | Admin / PPDB |
| **Peran** | PPDB Admin |
| **Scope** | Core |
| **Phase** | Phase 1 |
| **Tujuan** | Satu halaman lengkap untuk satu pendaftar. |
| **Fitur** | Applicant profile, Registration data, Parent data, Academic data |
| **Sub-fitur** | Personal, parent, previous school, program, status, payment, documents, activity history |

| ID | Kebutuhan | Prio | Scope | Phase |
|---|---|---|---|---|
| FR-11-001 | Sistem harus menampilkan data pribadi, data orang tua, sekolah asal, program, status tahap, ringkasan pembayaran, dan dokumen seorang pendaftar dalam satu halaman. | M | Core | Phase 1 |
| FR-11-002 | Sistem harus menampilkan riwayat aktivitas pendaftar: perubahan status, verifikasi dokumen, pembayaran, dan pengumuman. | M | Core | Phase 1 |

| ID | Skenario | Given / When / Then |
|---|---|---|
| AC-11-01 | Riwayat aktivitas | **Given** dokumen pendaftar diverifikasi pada 10 Oktober<br>**When** admin membuka detail pendaftar<br>**Then** riwayat menampilkan kejadian verifikasi beserta pengguna dan waktunya |

#### 12. Document Verification

| Atribut | Nilai |
|---|---|
| **Portal** | Admin / PPDB |
| **Peran** | PPDB Admin |
| **Scope** | Core |
| **Phase** | Phase 1 |
| **Tujuan** | Memeriksa dokumen persyaratan pendaftar. |
| **Fitur** | Document submission, Verification |
| **Sub-fitur** | Upload, preview, verify, reject, revision, notes, history, status |

| ID | Kebutuhan | Prio | Scope | Phase |
|---|---|---|---|---|
| FR-12-001 | Sistem harus menampilkan preview setiap dokumen tanpa perlu mengunduhnya. | M | Core | Phase 1 |
| FR-12-002 | Sistem harus memungkinkan admin menerima, menolak, atau meminta revisi setiap dokumen dengan catatan, dan memberi tahu pendaftar. | M | Core | Phase 1 |
| FR-12-003 | Sistem harus menandai pendaftar "terverifikasi" ketika seluruh dokumen wajib berstatus diterima. | M | Core | Phase 1 |

| ID | Skenario | Given / When / Then |
|---|---|---|
| AC-12-01 | Permintaan revisi dokumen | **Given** foto akta kelahiran pendaftar buram<br>**When** admin memilih "minta revisi" dengan catatan "foto kurang jelas"<br>**Then** pendaftar menerima notifikasi beserta catatan dan dapat mengunggah ulang dokumen |

#### 13. Selection

| Atribut | Nilai |
|---|---|
| **Portal** | Admin / PPDB |
| **Peran** | PPDB Admin; Selection Committee |
| **Scope** | Core |
| **Phase** | Phase 1 |
| **Tujuan** | Menjadwalkan, menilai, dan menetapkan hasil seleksi. |
| **Fitur** | Selection management, Assessment |
| **Sub-fitur** | Criteria, test/interview/academic score, scoring, pass/fail, approval, history |

| ID | Kebutuhan | Prio | Scope | Phase |
|---|---|---|---|---|
| FR-13-001 | Sistem harus mengelola kriteria seleksi per jalur beserta komponen nilainya (tes, wawancara, nilai akademik) dan bobotnya. | M | Core | Phase 1 |
| FR-13-002 | Sistem harus menjadwalkan tes atau wawancara dan memberi tahu pendaftar jadwalnya. | S | Core | Phase 1 |
| FR-13-003 | Sistem harus menghitung skor akhir dari komponen nilai dan bobot yang dikonfigurasi. | M | Core | Phase 1 |
| FR-13-004 | Sistem harus menyimpan status akhir: diterima, cadangan, atau tidak diterima. | M | Core | Phase 1 |
| FR-13-005 | Sistem harus mewajibkan approval Selection Committee atas hasil seleksi sebelum hasil dapat diumumkan. | M | Core | Phase 1 |

| ID | Skenario | Given / When / Then |
|---|---|---|
| AC-13-01 | Skor akhir terhitung | **Given** bobot tes 60% dan wawancara 40%<br>**When** pendaftar mendapat nilai tes 80 dan wawancara 90<br>**Then** skor akhir pendaftar 84 |
| AC-13-02 | Hasil belum disetujui | **Given** hasil seleksi gelombang 1 belum disetujui Selection Committee<br>**When** admin mencoba mempublikasikan pengumuman<br>**Then** publikasi ditolak dengan pesan bahwa hasil belum disetujui |

#### 14. Announcement

| Atribut | Nilai |
|---|---|
| **Portal** | Admin / PPDB |
| **Peran** | PPDB Admin |
| **Scope** | Core |
| **Phase** | Phase 1 |
| **Tujuan** | Mengumumkan hasil seleksi kepada pendaftar. |
| **Fitur** | Result announcement, Notification |
| **Sub-fitur** | Generate/publish, acceptance/rejection, applicant notification, history |

| ID | Kebutuhan | Prio | Scope | Phase |
|---|---|---|---|---|
| FR-14-001 | Sistem harus mempublikasikan hasil seleksi pada waktu yang dijadwalkan dan tidak menampilkannya sebelum waktu tersebut. | M | Core | Phase 1 |
| FR-14-002 | Sistem harus mengirim notifikasi hasil (diterima, cadangan, tidak diterima) kepada setiap pendaftar saat publikasi. | M | Core | Phase 1 |

| ID | Skenario | Given / When / Then |
|---|---|---|
| AC-14-01 | Pengumuman terjadwal | **Given** pengumuman dijadwalkan 1 Februari pukul 10.00<br>**When** waktu menunjukkan 09.59<br>**Then** hasil belum terlihat oleh pendaftar<br>**And** pada pukul 10.00 hasil terlihat dan notifikasi terkirim |

#### 15. Re-registration

| Atribut | Nilai |
|---|---|
| **Portal** | Admin / PPDB |
| **Peran** | PPDB Admin |
| **Scope** | Core |
| **Phase** | Phase 1 |
| **Tujuan** | Menuntaskan daftar ulang dan mengubah pendaftar menjadi siswa aktif. |
| **Fitur** | Re-registration, Student conversion |
| **Sub-fitur** | Status, required docs, payment verification, confirmation, student creation, completion |

| ID | Kebutuhan | Prio | Scope | Phase |
|---|---|---|---|---|
| FR-15-001 | Sistem harus menerbitkan tagihan daftar ulang dan uang pangkal untuk pendaftar berstatus diterima. | M | Core | Phase 1 |
| FR-15-002 | Sistem harus menampilkan status kelengkapan dokumen daftar ulang per pendaftar. | S | Core | Phase 1 |
| FR-15-003 | Sistem harus menandai daftar ulang selesai sesuai BR-PPDB-04. | M | Core | Phase 1 |
| FR-15-004 | Sistem harus menjalankan konversi siswa sesuai BR-PPDB-05. | M | Core | Phase 1 |

| ID | Skenario | Given / When / Then |
|---|---|---|
| AC-15-01 | Tagihan daftar ulang dengan promo | **Given** uang pangkal Rp10.000.000 dan promo "Early Bird" 10% sedang berlaku<br>**When** tagihan daftar ulang diterbitkan<br>**Then** tagihan uang pangkal sebesar Rp9.000.000 |
| AC-15-02 | Pembayaran minimum tercapai | **Given** minimum daftar ulang 50% dari tagihan Rp9.000.000<br>**When** pendaftar telah membayar Rp4.500.000<br>**Then** status daftar ulang menjadi "selesai" |
| AC-15-03 | Konversi menjadi siswa aktif | **Given** status daftar ulang "Citra" adalah "selesai"<br>**When** sistem memproses konversi<br>**Then** Citra berstatus siswa aktif dengan NIS baru dan tercatat di unitnya |

#### 16. Parent PPDB Portal

| Atribut | Nilai |
|---|---|
| **Portal** | Parent Portal |
| **Peran** | Applicant / Parent |
| **Scope** | Core |
| **Phase** | Phase 1 |
| **Tujuan** | Tempat calon orang tua mendaftar, membayar, dan memantau PPDB. |
| **Fitur** | Registration, Applicant profile, Documents, Selection, Payment, Re-registration |
| **Sub-fitur** | Form, upload, status, payment, result, announcement, re-registration, notification |

| ID | Kebutuhan | Prio | Scope | Phase |
|---|---|---|---|---|
| FR-16-001 | Sistem harus menyediakan formulir pendaftaran publik dan pembuatan akun calon orang tua menggunakan email dan nomor ponsel. | M | Core | Phase 1 |
| FR-16-002 | Sistem harus menerima isian data calon siswa dan orang tua serta upload dokumen PDF, JPG, atau PNG dengan ukuran maksimal [5] MB per file. | M | Core | Phase 1 |
| FR-16-003 | Sistem harus membuat nomor pendaftaran unik per unit dan gelombang serta bukti pendaftaran dalam PDF. | M | Core | Phase 1 |
| FR-16-004 | Sistem harus menerbitkan tagihan formulir saat pendaftaran dikirim dan menerapkan BR-PPDB-06. | M | Core | Phase 1 |
| FR-16-005 | Sistem harus menampilkan status pendaftaran per tahap: formulir, pembayaran, verifikasi, seleksi, pengumuman, dan daftar ulang. | M | Core | Phase 1 |
| FR-16-006 | Sistem harus mempertahankan akun calon orang tua sebagai akun orang tua setelah anak menjadi siswa aktif. | M | Core | Phase 1 |

| ID | Skenario | Given / When / Then |
|---|---|---|
| AC-16-01 | File melebihi batas ukuran | **Given** batas ukuran file [5] MB<br>**When** pendaftar mengunggah file 8 MB<br>**Then** unggahan ditolak dengan pesan batas ukuran |
| AC-16-02 | Nomor pendaftaran unik | **Given** dua pendaftar mengirim formulir unit "SD" gelombang 1 pada detik yang sama<br>**When** sistem membuat nomor pendaftaran<br>**Then** kedua nomor berbeda dan bukti pendaftaran PDF dapat diunduh |
| AC-16-03 | Formulir aktif setelah dibayar | **Given** tagihan formulir pendaftar berstatus "belum bayar"<br>**When** pembayaran tagihan formulir berstatus "sukses"<br>**Then** status formulir menjadi "aktif" dan pendaftar masuk tahap verifikasi |
| AC-16-04 | Akun berlanjut setelah daftar ulang | **Given** calon orang tua mendaftar dengan akun sari@email.com<br>**When** anaknya menjadi siswa aktif<br>**Then** orang tua masuk dengan akun yang sama dan melihat data siswa aktif |

### Grup Menu 3 — Finance

*Phase 2 · seluruh menu Core*

#### 17. Finance Dashboard

| Atribut | Nilai |
|---|---|
| **Portal** | Admin / Finance |
| **Peran** | Finance Admin; Finance Manager |
| **Scope** | Core |
| **Phase** | Phase 2 |
| **Tujuan** | Ringkasan operasional keuangan sekolah: tagihan, penerimaan, dan piutang. |
| **Fitur** | Billing overview, Collection, Receivable, Financial performance |
| **Sub-fitur** | Billing, collection, outstanding, rate, aging, scholarship, refund, trend |

| ID | Kebutuhan | Prio | Scope | Phase |
|---|---|---|---|---|
| FR-17-001 | Sistem harus menampilkan total tagihan, total terkumpul, total outstanding, collection rate, ringkasan aging, beasiswa, dan refund per unit dan periode. | M | Core | Phase 2 |
| FR-17-002 | Sistem harus menghasilkan laporan penerimaan per periode, unit, komponen biaya, dan metode pembayaran. | M | Core | Phase 2 |

| ID | Skenario | Given / When / Then |
|---|---|---|
| AC-17-01 | Laporan penerimaan per metode | **Given** pada 1 Oktober unit "SD" menerima Rp5.000.000 via VA dan Rp1.000.000 tunai<br>**When** staf keuangan membuka laporan penerimaan 1 Oktober unit SD per metode<br>**Then** laporan menampilkan VA Rp5.000.000 dan tunai Rp1.000.000 |

#### 18. Fee Structure

| Atribut | Nilai |
|---|---|
| **Portal** | Admin / Finance |
| **Peran** | Finance Admin; Finance Manager |
| **Scope** | Core |
| **Phase** | Phase 2 |
| **Tujuan** | Mengatur komponen biaya dan tarif. Biaya PPDB diatur di menu 09; beasiswa di menu 24. |
| **Fitur** | Fee configuration, Discount, Fee history |
| **Sub-fitur** | Per unit/class/program, annual/monthly, installment, discount, scholarship, effective period, approval, history |

| ID | Kebutuhan | Prio | Scope | Phase |
|---|---|---|---|---|
| FR-18-001 | Sistem harus mengelola master komponen biaya (SPP, seragam, buku, kegiatan, dan lainnya) dengan frekuensi tahunan, bulanan, atau sekali bayar. | M | Core | Phase 2 |
| FR-18-002 | Sistem harus menyimpan tarif setiap komponen per unit, kelas/tingkat, program, angkatan, dan jalur. | M | Core | Phase 2 |
| FR-18-003 | Sistem harus menyimpan skema cicilan default per komponen. | M | Core | Phase 2 |
| FR-18-004 | Sistem harus menerapkan BR-FIN-06 sebelum komponen dapat dipakai menerbitkan tagihan. | M | Core | Phase 2 |
| FR-18-005 | Sistem harus menyimpan periode berlaku setiap tarif dan memakai tarif yang berlaku pada tanggal penerbitan tagihan. | M | Core | Phase 2 |
| FR-18-006 | Sistem harus mewajibkan approval Finance Manager untuk perubahan tarif. | S | Core | Phase 2 |

| ID | Skenario | Given / When / Then |
|---|---|---|
| AC-18-01 | Tarif berbeda per unit | **Given** tarif SPP unit "SD" Rp400.000 dan unit "SMP" Rp500.000<br>**When** tagihan SPP diterbitkan untuk siswa SMP<br>**Then** nominal tagihan Rp500.000 |
| AC-18-02 | Tarif baru berlaku sesuai periode | **Given** tarif SPP Rp500.000 berlaku hingga 30 Juni dan Rp550.000 mulai 1 Juli<br>**When** tagihan SPP Juli diterbitkan<br>**Then** nominal tagihan Rp550.000 |
| AC-18-03 | Komponen tanpa mapping akun | **Given** komponen "Biaya Buku" belum dipetakan ke akun unit "SMK"<br>**When** staf keuangan menerbitkan tagihan Biaya Buku untuk unit SMK<br>**Then** penerbitan ditolak dengan pesan mapping akun belum lengkap |

#### 19. Billing

| Atribut | Nilai |
|---|---|
| **Portal** | Admin / Finance |
| **Peran** | Finance Admin |
| **Scope** | Core |
| **Phase** | Phase 2 |
| **Tujuan** | Menerbitkan dan mengelola tagihan. |
| **Fitur** | Billing generation, Invoice, Installment |
| **Sub-fitur** | Recurring/individual/bulk, schedule, due date, partial, installment, discount, adjustment, cancellation, history |

| ID | Kebutuhan | Prio | Scope | Phase |
|---|---|---|---|---|
| FR-19-001 | Sistem harus menerbitkan tagihan satu tahun ajaran sekaligus untuk siswa yang baru berstatus aktif. | M | Core | Phase 2 |
| FR-19-002 | Sistem harus menerbitkan tagihan berulang terjadwal, tagihan massal per unit/kelas/angkatan, dan tagihan individual. | M | Core | Phase 2 |
| FR-19-003 | Sistem harus memungkinkan satu tagihan dibagi menjadi beberapa termin dengan jatuh tempo masing-masing. | M | Core | Phase 2 |
| FR-19-004 | Sistem harus menyediakan invoice PDF untuk setiap tagihan. | M | Core | Phase 2 |
| FR-19-005 | Sistem harus memberi tahu orang tua ketika tagihan baru terbit. | M | Core | Phase 2 |
| FR-19-006 | Sistem harus mewajibkan approval untuk penyesuaian nominal atau pembatalan tagihan, dan menyimpan nilai asalnya. | M | Core | Phase 2 |

| ID | Skenario | Given / When / Then |
|---|---|---|
| AC-19-01 | Billing tahunan untuk siswa baru | **Given** siswa "Citra" baru aktif di unit "SD" dan komponen tahunan unit SD adalah SPP 12 bulan dan Biaya Kegiatan<br>**When** billing tahunan diproses<br>**Then** seluruh tagihan tahun ajaran berjalan untuk Citra terbit sekaligus |
| AC-19-02 | Tagihan dibagi tiga termin | **Given** tagihan uang pangkal Rp9.000.000<br>**When** staf keuangan membaginya menjadi 3 termin<br>**Then** terbentuk 3 termin masing-masing Rp3.000.000 dengan jatuh tempo berbeda |
| AC-19-03 | Tagihan massal per kelas | **Given** kelas "7A" berisi 30 siswa aktif<br>**When** staf keuangan menerbitkan tagihan "Study Tour" untuk kelas 7A<br>**Then** 30 tagihan terbit, satu per siswa |
| AC-19-04 | Pembatalan tagihan | **Given** tagihan Rp500.000 salah terbit<br>**When** staf keuangan mengajukan pembatalan<br>**Then** tagihan tetap aktif sampai disetujui<br>**And** setelah disetujui, tagihan berstatus "dibatalkan" dan nilai asalnya tetap tersimpan |

#### 20. Student Billing Detail

| Atribut | Nilai |
|---|---|
| **Portal** | Admin / Finance |
| **Peran** | Finance Admin; Finance Manager |
| **Scope** | Core |
| **Phase** | Phase 2 |
| **Tujuan** | Kartu piutang satu siswa. |
| **Fitur** | Student financial profile, Billing detail, Payment history |
| **Sub-fitur** | Outstanding/paid/partial, installment, discount, scholarship, refund, balance, payment history |

| ID | Kebutuhan | Prio | Scope | Phase |
|---|---|---|---|---|
| FR-20-001 | Sistem harus menampilkan kartu piutang per siswa: seluruh tagihan, termin, potongan, beasiswa, pembayaran, alokasi, refund, deposit, dan saldo akhir. | M | Core | Phase 2 |
| FR-20-002 | Sistem harus menampilkan status setiap tagihan sesuai BR-FIN-01. | M | Core | Phase 2 |

| ID | Skenario | Given / When / Then |
|---|---|---|
| AC-20-01 | Status pembayaran sebagian | **Given** tagihan SPP Rp1.000.000 berstatus "belum bayar"<br>**When** pembayaran Rp600.000 dialokasikan ke tagihan tersebut<br>**Then** status tagihan "sebagian" dengan sisa Rp400.000 |
| AC-20-02 | Tagihan menjadi lunas | **Given** tagihan SPP Rp1.000.000 dengan terbayar Rp600.000<br>**When** pembayaran Rp400.000 dialokasikan<br>**Then** status tagihan menjadi "lunas" |

#### 21. Payment Allocation

| Atribut | Nilai |
|---|---|
| **Portal** | Admin / Finance |
| **Peran** | Finance Admin; Accounting |
| **Scope** | Core |
| **Phase** | Phase 2 |
| **Tujuan** | Mencocokkan pembayaran dengan tagihan. |
| **Fitur** | Payment matching, Invoice allocation |
| **Sub-fitur** | Allocate, partial/multi-invoice, overpayment, unallocated, adjustment, history |

| ID | Kebutuhan | Prio | Scope | Phase |
|---|---|---|---|---|
| FR-21-001 | Sistem harus mengalokasikan pembayaran secara otomatis sesuai BR-FIN-03, termasuk ke beberapa tagihan sekaligus. | M | Core | Phase 2 |
| FR-21-002 | Sistem harus memungkinkan Finance Admin mengalokasikan pembayaran secara manual. | M | Core | Phase 2 |
| FR-21-003 | Sistem harus memproses kelebihan bayar sesuai BR-FIN-04. | S | Core | Phase 2 |
| FR-21-004 | Sistem harus menampilkan daftar dana yang belum teralokasi beserta umurnya. | M | Core | Phase 2 |
| FR-21-005 | Sistem harus mewajibkan approval untuk koreksi alokasi dan menyimpan riwayat alokasi. | M | Core | Phase 2 |

| ID | Skenario | Given / When / Then |
|---|---|---|
| AC-21-01 | Alokasi ke jatuh tempo paling awal | **Given** aturan alokasi "jatuh tempo paling awal" dan siswa memiliki SPP September Rp1.000.000, SPP Oktober Rp1.000.000, dan Biaya Ujian Rp500.000 (jatuh tempo November)<br>**When** orang tua membayar Rp1.200.000 tanpa memilih tagihan<br>**Then** SPP September lunas Rp1.000.000, SPP Oktober sebagian Rp200.000, dan Biaya Ujian Rp0 |
| AC-21-02 | Kelebihan bayar menjadi deposit | **Given** seluruh tagihan siswa yang sudah terbit Rp1.000.000<br>**When** orang tua membayar Rp1.200.000<br>**Then** Rp200.000 tersimpan sebagai deposit dan dialokasikan otomatis saat tagihan berikutnya terbit |

#### 22. Receivable

| Atribut | Nilai |
|---|---|
| **Portal** | Admin / Finance |
| **Peran** | Finance Admin; Finance Manager |
| **Scope** | Core |
| **Phase** | Phase 2 |
| **Tujuan** | Memantau dan menindaklanjuti piutang lintas siswa. |
| **Fitur** | Receivable monitoring, Collection |
| **Sub-fitur** | Outstanding invoice/student/unit, due/overdue, status, follow-up, history |

| ID | Kebutuhan | Prio | Scope | Phase |
|---|---|---|---|---|
| FR-22-001 | Sistem harus menampilkan piutang outstanding per tagihan, siswa, dan unit dengan penanda jatuh tempo dan lewat jatuh tempo. | M | Core | Phase 2 |
| FR-22-002 | Sistem harus mengirim pengingat otomatis [H-3], pada hari jatuh tempo, dan [H+3] untuk tagihan yang belum lunas. | S | Core | Phase 2 |
| FR-22-003 | Sistem harus mencatat tindak lanjut penagihan (tanggal, kanal, catatan, petugas) per siswa. | S | Core | Phase 2 |

| ID | Skenario | Given / When / Then |
|---|---|---|
| AC-22-01 | Pengingat sebelum jatuh tempo | **Given** tagihan SPP jatuh tempo 10 Oktober dan belum lunas<br>**When** tanggal menunjukkan 7 Oktober<br>**Then** orang tua menerima pengingat jatuh tempo |

#### 23. Aging

| Atribut | Nilai |
|---|---|
| **Portal** | Admin / Finance |
| **Peran** | Finance Admin; Finance Manager |
| **Scope** | Core |
| **Phase** | Phase 2 |
| **Tujuan** | Analisis umur piutang. |
| **Fitur** | Aging analysis |
| **Sub-fitur** | Not due, 1–30, 31–60, 61–90, >90, by student/unit/invoice, trend |

| ID | Kebutuhan | Prio | Scope | Phase |
|---|---|---|---|---|
| FR-23-001 | Sistem harus mengelompokkan piutang ke kelompok belum jatuh tempo, 1–30, 31–60, 61–90, dan lebih dari 90 hari, per siswa, unit, dan tagihan. | M | Core | Phase 2 |
| FR-23-002 | Sistem harus menampilkan tren saldo aging per bulan. | S | Core | Phase 2 |

| ID | Skenario | Given / When / Then |
|---|---|---|
| AC-23-01 | Aging tunggakan | **Given** tagihan Rp1.000.000 jatuh tempo 45 hari lalu dan belum lunas<br>**When** staf keuangan membuka laporan aging<br>**Then** tagihan tersebut tampil di kelompok 31–60 hari |

#### 24. Scholarship

| Atribut | Nilai |
|---|---|
| **Portal** | Admin / Finance |
| **Peran** | Finance Admin; School Admin |
| **Scope** | Core |
| **Phase** | Phase 2 |
| **Tujuan** | Beasiswa dan keringanan untuk siswa aktif. Promo PPDB diatur di menu 09. |
| **Fitur** | Scholarship management |
| **Sub-fitur** | Type, eligibility, application, approval, amount, period, billing allocation, history |

| ID | Kebutuhan | Prio | Scope | Phase |
|---|---|---|---|---|
| FR-24-001 | Sistem harus mengelola jenis beasiswa dan keringanan (termasuk potongan saudara kandung) beserta syarat, nominal atau persen, dan periodenya. | S | Core | Phase 2 |
| FR-24-002 | Sistem harus menerapkan beasiswa ke tagihan siswa hanya setelah pengajuan disetujui. | S | Core | Phase 2 |

| ID | Skenario | Given / When / Then |
|---|---|---|
| AC-24-01 | Beasiswa setelah approval | **Given** siswa mengajukan beasiswa 50% SPP<br>**When** kepala unit menyetujui<br>**Then** tagihan SPP berikutnya siswa tersebut terbit 50% lebih rendah |

#### 25. Refund

| Atribut | Nilai |
|---|---|
| **Portal** | Admin / Finance |
| **Peran** | Finance Admin; Finance Manager |
| **Scope** | Core |
| **Phase** | Phase 2 |
| **Tujuan** | Pengembalian dana yang terkontrol. |
| **Fitur** | Refund management |
| **Sub-fitur** | Request, reason, amount, approval, processing, status, history, accounting impact |

| ID | Kebutuhan | Prio | Scope | Phase |
|---|---|---|---|---|
| FR-25-001 | Sistem harus mencatat pengajuan refund dengan alasan, nominal, dan tagihan terkait. | S | Core | Phase 2 |
| FR-25-002 | Sistem harus memproses refund hanya setelah approval Finance Manager, mengurangi saldo terbayar tagihan terkait, dan memicu jurnal refund (menu 34). | S | Core | Phase 2 |

| ID | Skenario | Given / When / Then |
|---|---|---|
| AC-25-01 | Refund mengurangi saldo terbayar | **Given** pendaftar mengundurkan diri setelah membayar uang pangkal Rp4.500.000<br>**When** refund Rp4.500.000 disetujui dan diproses<br>**Then** saldo terbayar tagihan uang pangkal menjadi Rp0 dan transaksi refund tercatat dengan referensi approval |

### Grup Menu 4 — Payment

*Phase 2 · seluruh menu Core. Pembayaran minimal untuk tagihan PPDB (VA, QRIS, e-wallet, status otomatis, kuitansi) ditarik ke Phase 1.*

#### 26. Payment Dashboard

| Atribut | Nilai |
|---|---|
| **Portal** | Admin / Finance |
| **Peran** | Finance Admin; Finance Manager |
| **Scope** | Core |
| **Phase** | Phase 2 |
| **Tujuan** | Memantau transaksi dan settlement pembayaran digital. |
| **Fitur** | Payment monitoring, Transaction monitoring, Settlement monitoring |
| **Sub-fitur** | Total/success/pending/failed, volume, method distribution, trend, settlement |

| ID | Kebutuhan | Prio | Scope | Phase |
|---|---|---|---|---|
| FR-26-001 | Sistem harus menampilkan jumlah dan nilai transaksi sukses, pending, gagal, dan kedaluwarsa, distribusi per metode, tren, dan status settlement per unit. | M | Core | Phase 2 |

| ID | Skenario | Given / When / Then |
|---|---|---|
| AC-26-01 | Distribusi metode | **Given** hari ini terdapat 40 transaksi VA dan 10 transaksi QRIS yang sukses<br>**When** Finance Manager membuka dashboard pembayaran<br>**Then** distribusi metode menampilkan VA 80% dan QRIS 20% |

#### 27. Checkout

| Atribut | Nilai |
|---|---|
| **Portal** | Parent Portal |
| **Peran** | Parent / Guardian |
| **Scope** | Core |
| **Phase** | Phase 1/2 |
| **Tujuan** | Alur bayar orang tua. Phase 1 untuk tagihan PPDB; Phase 2 untuk semua tagihan. |
| **Fitur** | Billing selection, Payment processing |
| **Sub-fitur** | Select invoice, summary, method, confirmation, pay now, processing, success/failure |

| ID | Kebutuhan | Prio | Scope | Phase |
|---|---|---|---|---|
| FR-27-001 | Sistem harus memungkinkan orang tua memilih satu atau lebih tagihan, melihat ringkasan, memilih metode, dan menerima instruksi pembayaran dalam satu transaksi. | M | Core | Phase 1 |
| FR-27-002 | Sistem harus menampilkan halaman hasil pembayaran: berhasil, menunggu, gagal, atau kedaluwarsa. | M | Core | Phase 1 |
| FR-27-003 | Sistem harus menandai kode bayar yang melewati masa berlaku sebagai kedaluwarsa dan memungkinkan orang tua membuat kode baru. | M | Core | Phase 1 |

| ID | Skenario | Given / When / Then |
|---|---|---|
| AC-27-01 | Membayar dua tagihan sekaligus | **Given** orang tua memiliki tagihan SPP Rp500.000 dan Biaya Buku Rp300.000<br>**When** orang tua memilih kedua tagihan dan metode VA<br>**Then** sistem membuat satu transaksi Rp800.000 dan menampilkan nomor VA beserta instruksinya |
| AC-27-02 | Kode bayar kedaluwarsa | **Given** masa berlaku VA transaksi TX-011 telah lewat<br>**When** orang tua membuka tagihan terkait<br>**Then** TX-011 tampil "kedaluwarsa" dan orang tua dapat membuat kode bayar baru |

#### 28. Payment Method

| Atribut | Nilai |
|---|---|
| **Portal** | Parent Portal |
| **Peran** | Parent / Guardian |
| **Scope** | Campuran (lihat per kebutuhan) |
| **Phase** | Phase 1 |
| **Tujuan** | Pilihan metode pembayaran melalui payment gateway berlisensi. |
| **Fitur** | Bank payment, Digital payment, Retail payment |
| **Sub-fitur** | VA, bank transfer, QRIS, e-wallet, card, Indomaret, Alfamart, saved methods |

| ID | Kebutuhan | Prio | Scope | Phase |
|---|---|---|---|---|
| FR-28-001 | Sistem harus menyediakan pembayaran melalui Virtual Account multi-bank, QRIS, dan e-wallet. | M | Core | Phase 1 |
| FR-28-002 | Sistem harus menerapkan BR-PAY-02 dan menampilkan biaya transaksi kepada orang tua sebelum konfirmasi bila biaya dibebankan kepada orang tua. | S | Core | Phase 1 |
| FR-28-003 | Sistem harus menyediakan pembayaran kartu kredit/debit. | S | Add-on | — |
| FR-28-004 | Sistem harus menyediakan pembayaran di gerai retail (Indomaret, Alfamart). | S | Add-on | — |
| FR-28-005 | Sistem harus menyimpan metode pembayaran favorit orang tua. | S | Add-on | — |

| ID | Skenario | Given / When / Then |
|---|---|---|
| AC-28-01 | Pilihan metode inti | **Given** orang tua memiliki tagihan yang belum lunas<br>**When** orang tua membuka pilihan metode<br>**Then** tersedia Virtual Account, QRIS, dan e-wallet |
| AC-28-02 | Biaya ditanggung orang tua | **Given** biaya QRIS diatur ditanggung orang tua<br>**When** orang tua membayar tagihan Rp1.000.000 dengan QRIS<br>**Then** total bayar Rp1.000.000 ditambah biaya QRIS, dan tagihan teralokasi Rp1.000.000 |

#### 29. Transaction

| Atribut | Nilai |
|---|---|
| **Portal** | Admin / Finance / Accounting |
| **Peran** | Finance Admin; Finance Manager; Accounting |
| **Scope** | Core |
| **Phase** | Phase 1/2 |
| **Tujuan** | Catatan seluruh transaksi pembayaran, online maupun tunai. |
| **Fitur** | Transaction management |
| **Sub-fitur** | List/detail, reference, invoice/student, amount/method/time, status, receipt, refund, resend, history |

| ID | Kebutuhan | Prio | Scope | Phase |
|---|---|---|---|---|
| FR-29-001 | Sistem harus menampilkan daftar dan detail transaksi: referensi gateway, tagihan, siswa, nominal, metode, waktu, dan status. | M | Core | Phase 1 |
| FR-29-002 | Sistem harus memperbarui status transaksi (pending, sukses, gagal, kedaluwarsa) berdasarkan notifikasi payment gateway tanpa tindakan manual. | M | Core | Phase 1 |
| FR-29-003 | Sistem harus mengirim kuitansi digital paling lambat [5] menit setelah transaksi sukses dan memungkinkan kuitansi dikirim ulang. | M | Core | Phase 1 |
| FR-29-004 | Sistem harus mencatat pembayaran tunai atau transfer bank di kasir dan mencetak kuitansi bernomor unik. | M | Core | Phase 2 |
| FR-29-005 | Sistem harus menerapkan BR-FIN-05 untuk koreksi dan pembatalan transaksi. | M | Core | Phase 2 |

| ID | Skenario | Given / When / Then |
|---|---|---|
| AC-29-01 | Status sukses otomatis | **Given** transaksi TX-010 berstatus "pending"<br>**When** payment gateway mengirim notifikasi sukses untuk TX-010<br>**Then** status TX-010 menjadi "sukses" dan pembayaran dialokasikan ke tagihan terkait |
| AC-29-02 | Kuitansi terkirim | **Given** transaksi TX-013 sukses pukul 10.00<br>**When** waktu menunjukkan 10.05<br>**Then** orang tua sudah menerima kuitansi digital TX-013 |
| AC-29-03 | Pembayaran tunai di kasir | **Given** orang tua membayar tunai Rp500.000 di kasir unit "SD"<br>**When** kasir mencatat pembayaran<br>**Then** kuitansi bernomor unik tercetak dan pembayaran teralokasi sesuai BR-FIN-03 |
| AC-29-04 | Koreksi memerlukan approval | **Given** pembayaran Rp500.000 salah dicatat ke siswa lain<br>**When** kasir mengajukan koreksi<br>**Then** koreksi berlaku setelah disetujui dan transaksi asal tetap tersimpan dengan status "dikoreksi" |

#### 30. Settlement

| Atribut | Nilai |
|---|---|
| **Portal** | Admin / Finance |
| **Peran** | Finance Admin; Finance Manager |
| **Scope** | Core |
| **Phase** | Phase 2 |
| **Tujuan** | Memantau dana yang diteruskan payment gateway ke rekening sekolah. |
| **Fitur** | Settlement management |
| **Sub-fitur** | Batch, channel, amount/date, bank/gateway, status, history |

| ID | Kebutuhan | Prio | Scope | Phase |
|---|---|---|---|---|
| FR-30-001 | Sistem harus menampilkan batch settlement per kanal dengan nominal, tanggal, rekening tujuan, biaya, dan status. | M | Core | Phase 2 |
| FR-30-002 | Sistem harus mengarahkan settlement ke rekening unit siswa bila payment gateway mendukung sub-akun. | S | Core | Phase 2 |

| ID | Skenario | Given / When / Then |
|---|---|---|
| AC-30-01 | Settlement ke rekening unit | **Given** payment gateway mendukung sub-akun dan unit "SMP" memiliki rekening sendiri<br>**When** orang tua siswa SMP membayar tagihan<br>**Then** dana settlement diarahkan ke rekening unit SMP |

#### 31. Reconciliation

| Atribut | Nilai |
|---|---|
| **Portal** | Admin / Finance / Accounting |
| **Peran** | Finance Admin; Accounting |
| **Scope** | Core |
| **Phase** | Phase 2/3 |
| **Tujuan** | Mencocokkan transaksi dengan settlement gateway (Phase 2) dan mutasi bank (Phase 3). |
| **Fitur** | Bank reconciliation, Gateway reconciliation, Adjustment |
| **Sub-fitur** | Import reports, matching, unmatched/missing, difference, adjustment, status, audit |

| ID | Kebutuhan | Prio | Scope | Phase |
|---|---|---|---|---|
| FR-31-001 | Sistem harus merekonsiliasi transaksi sukses dengan laporan settlement gateway setiap hari secara otomatis. | M | Core | Phase 2 |
| FR-31-002 | Sistem harus mengimport mutasi rekening bank per unit dan mencocokkannya dengan jurnal kas/bank. | M | Core | Phase 3 |
| FR-31-003 | Sistem harus menampilkan item yang tidak cocok, hilang, atau berselisih dalam laporan pengecualian. | M | Core | Phase 2 |
| FR-31-004 | Sistem harus mewajibkan approval untuk jurnal penyesuaian hasil rekonsiliasi. | M | Core | Phase 2 |

| ID | Skenario | Given / When / Then |
|---|---|---|
| AC-31-01 | Transaksi tanpa pasangan settlement | **Given** TX-012 sukses tanggal 1 Oktober dan laporan settlement 1 Oktober tidak memuatnya<br>**When** rekonsiliasi harian berjalan<br>**Then** TX-012 tercatat di laporan pengecualian |
| AC-31-02 | Rekonsiliasi bank | **Given** mutasi rekening unit "SD" berisi 50 transaksi<br>**When** akuntan menjalankan rekonsiliasi bank<br>**Then** transaksi yang cocok ditandai "cocok" dan sisanya ditampilkan untuk ditindaklanjuti |

### Grup Menu 5 — Accounting

*Phase 3 · Core. Dijalankan oleh Accounting Engine existing dan ditampilkan di dalam platform.*

#### 32. Accounting Dashboard

| Atribut | Nilai |
|---|---|
| **Portal** | Accounting / Finance |
| **Peran** | Finance Admin; Finance Manager; Accounting |
| **Scope** | Core |
| **Phase** | Phase 3 |
| **Tujuan** | Ringkasan kondisi keuangan akuntansi per unit. |
| **Fitur** | Financial overview, Accounting monitoring |
| **Sub-fitur** | Revenue, expense, surplus/deficit, assets, liabilities, cash/bank, receivable, journal status, trend |

| ID | Kebutuhan | Prio | Scope | Phase |
|---|---|---|---|---|
| FR-32-001 | Sistem harus menampilkan pendapatan, beban, surplus/defisit, aset, liabilitas, kas/bank, dan piutang dari buku besar per unit dan periode. | M | Core | Phase 3 |
| FR-32-002 | Sistem harus menampilkan jumlah jurnal per status (draft, menunggu approval, posted, reversed) dan posting otomatis yang gagal; item yang gagal ditangani di menu 80. | M | Core | Phase 3 |

| ID | Skenario | Given / When / Then |
|---|---|---|
| AC-32-01 | Posting gagal terlihat | **Given** 2 posting otomatis unit "SD" gagal dikirim<br>**When** akuntan membuka dashboard akuntansi<br>**Then** dashboard menampilkan 2 posting gagal dengan tautan ke detailnya |

#### 33. Chart of Accounts (COA)

| Atribut | Nilai |
|---|---|
| **Portal** | Accounting / Finance |
| **Peran** | Accounting; Finance Manager |
| **Scope** | Core |
| **Phase** | Phase 3 |
| **Tujuan** | Bagan akun per unit. |
| **Fitur** | Account management, Account hierarchy |
| **Sub-fitur** | Code/name/type/category, parent, normal balance, unit mapping, status, CRUD, import/export, history |

| ID | Kebutuhan | Prio | Scope | Phase |
|---|---|---|---|---|
| FR-33-001 | Sistem harus mengelola akun dengan kode, nama, tipe, kategori, akun induk, saldo normal, dan status aktif. | M | Core | Phase 3 |
| FR-33-002 | Sistem harus menyediakan template COA sekolah yang dapat disalin dan disesuaikan per unit. | M | Core | Phase 3 |

| ID | Skenario | Given / When / Then |
|---|---|---|
| AC-33-01 | COA unit baru dari template | **Given** template COA sekolah tersedia<br>**When** admin membuat unit baru "SMK"<br>**Then** unit SMK memiliki COA hasil salinan template yang dapat diubah |

#### 34. Journal

| Atribut | Nilai |
|---|---|
| **Portal** | Accounting / Finance |
| **Peran** | Accounting; Finance Admin |
| **Scope** | Core |
| **Phase** | Phase 3 |
| **Tujuan** | Jurnal otomatis dari transaksi sekolah dan jurnal manual. |
| **Fitur** | Manual journal, Auto journal, Approval, Posting |
| **Sub-fitur** | Create/edit, draft/approval/posted/reversed, debit/credit, reference, attachment, template, recurring, adjustment, history |

| ID | Kebutuhan | Prio | Scope | Phase |
|---|---|---|---|---|
| FR-34-001 | Sistem harus membentuk jurnal otomatis saat tagihan terbit, mengikuti BR-ACC-05 dan BR-ACC-06. | M | Core | Phase 3 |
| FR-34-002 | Sistem harus membentuk jurnal otomatis untuk setiap penerimaan (online dan tunai), mengikuti BR-ACC-05 dan BR-ACC-06. | M | Core | Phase 3 |
| FR-34-003 | Sistem harus membentuk jurnal otomatis untuk diskon, beasiswa, refund, dan biaya transaksi sesuai mapping akun. | M | Core | Phase 3 |
| FR-34-004 | Sistem harus memungkinkan akuntan membuat jurnal manual, termasuk pengeluaran kas, dengan referensi dan lampiran. | M | Core | Phase 3 |
| FR-34-005 | Sistem harus menjalankan alur jurnal draft → approval → posted → reversed, dan menerapkan BR-ACC-01 serta BR-ACC-02. | M | Core | Phase 3 |
| FR-34-006 | Sistem harus menyediakan template jurnal dan jurnal berulang terjadwal. | S | Core | Phase 3 |
| FR-34-007 | Sistem harus menyediakan tutup buku per unit dan periode, dan menerapkan BR-ACC-07. | M | Core | Phase 3 |

| ID | Skenario | Given / When / Then |
|---|---|---|
| AC-34-01 | Jurnal saat tagihan terbit | **Given** tagihan SPP Oktober Rp500.000 diterbitkan untuk siswa unit "SMP"<br>**When** posting harian dibuat<br>**Then** entitas SMP menerima jurnal debit Piutang SPP Rp500.000 dan kredit akun sesuai mapping |
| AC-34-02 | Jurnal penerimaan per unit | **Given** pada 1 Oktober siswa unit "SD" membayar total Rp3.000.000 dan siswa unit "SMP" Rp2.000.000<br>**When** posting harian 1 Oktober dibuat<br>**Then** entitas SD menerima jurnal penerimaan Rp3.000.000 dan entitas SMP Rp2.000.000 |
| AC-34-03 | Jurnal tidak seimbang | **Given** akuntan menyusun jurnal manual dengan debit Rp1.000.000 dan kredit Rp900.000<br>**When** akuntan memposting jurnal<br>**Then** posting ditolak dengan pesan debit dan kredit tidak seimbang |
| AC-34-04 | Koreksi jurnal yang sudah posted | **Given** jurnal JV-0101 sudah berstatus posted<br>**When** akuntan mencoba mengubah nominalnya<br>**Then** perubahan langsung tidak diizinkan dan sistem menawarkan jurnal pembalik |
| AC-34-05 | Periode terkunci | **Given** periode September unit "SD" sudah ditutup<br>**When** akuntan membuat jurnal bertanggal 15 September<br>**Then** jurnal ditolak dengan pesan bahwa periode sudah dikunci |

#### 35. General Ledger

| Atribut | Nilai |
|---|---|
| **Portal** | Accounting / Finance |
| **Peran** | Accounting; Finance Manager |
| **Scope** | Core |
| **Phase** | Phase 3 |
| **Tujuan** | Buku besar per akun. |
| **Fitur** | General ledger, Account ledger, Transaction drilldown |
| **Sub-fitur** | Opening, debit/credit, running balance, period/unit, journal reference, source, drilldown, export |

| ID | Kebutuhan | Prio | Scope | Phase |
|---|---|---|---|---|
| FR-35-001 | Sistem harus menampilkan saldo awal, mutasi debit/kredit, dan saldo berjalan per akun, unit, dan periode. | M | Core | Phase 3 |
| FR-35-002 | Sistem harus menyediakan drilldown dari baris buku besar ke jurnal dan ke transaksi sumbernya (tagihan atau pembayaran). | M | Core | Phase 3 |

| ID | Skenario | Given / When / Then |
|---|---|---|
| AC-35-01 | Drilldown ke transaksi sumber | **Given** baris buku besar akun Kas berasal dari posting penerimaan 1 Oktober<br>**When** akuntan membuka baris tersebut<br>**Then** sistem menampilkan jurnal dan daftar pembayaran sumbernya |

#### 36. Trial Balance

| Atribut | Nilai |
|---|---|
| **Portal** | Accounting / Finance |
| **Peran** | Accounting; Finance Manager |
| **Scope** | Core |
| **Phase** | Phase 3 |
| **Tujuan** | Neraca saldo untuk memastikan pembukuan seimbang. |
| **Fitur** | Trial balance, Balance validation |
| **Sub-fitur** | Opening, debit/credit mutation, ending, hierarchy, unit/period, totals, difference, balanced, export/print |

| ID | Kebutuhan | Prio | Scope | Phase |
|---|---|---|---|---|
| FR-36-001 | Sistem harus menghasilkan neraca saldo sesuai BR-ACC-03 per unit, konsolidasi, periode, dan hierarki akun. | M | Core | Phase 3 |
| FR-36-002 | Sistem harus menampilkan total debit, total kredit, selisih, dan indikator seimbang. | M | Core | Phase 3 |

| ID | Skenario | Given / When / Then |
|---|---|---|
| AC-36-01 | Neraca saldo seimbang | **Given** seluruh jurnal Oktober unit "SMP" sudah posted<br>**When** akuntan membuka neraca saldo Oktober unit SMP<br>**Then** total debit sama dengan total kredit dan indikator menunjukkan seimbang |

#### 37. Financial Reports

| Atribut | Nilai |
|---|---|
| **Portal** | Accounting / Finance |
| **Peran** | Accounting; Finance Manager; Yayasan |
| **Scope** | Core |
| **Phase** | Phase 3 |
| **Tujuan** | Laporan keuangan per unit untuk yayasan. |
| **Fitur** | Laba Rugi, Neraca, Arus Kas, Perubahan Aset Neto, Supporting reports |
| **Sub-fitur** | Revenue/expense, assets/liabilities/net assets, cash flow, beginning/ending cash, comparison, GL/supporting, export/print |

| ID | Kebutuhan | Prio | Scope | Phase |
|---|---|---|---|---|
| FR-37-001 | Sistem harus menghasilkan laporan aktivitas (laba rugi), laporan posisi keuangan (neraca), laporan arus kas, dan laporan perubahan aset neto per unit dan periode, dalam format yang disepakati akuntan yayasan [per klien]. | M | Core | Phase 3 |
| FR-37-002 | Sistem harus menampilkan perbandingan dengan periode sebelumnya dan laporan pendukung dari buku besar. | S | Core | Phase 3 |

| ID | Skenario | Given / When / Then |
|---|---|---|
| AC-37-01 | Laporan per unit | **Given** periode Oktober unit "SD" sudah ditutup<br>**When** Finance Manager membuka laporan keuangan Oktober unit SD<br>**Then** keempat laporan tersedia dengan periode, unit, dan waktu pembuatan tercantum |

#### 38. Consolidation

| Atribut | Nilai |
|---|---|
| **Portal** | Yayasan / Accounting |
| **Peran** | Accounting; Yayasan Admin; Pengurus Yayasan |
| **Scope** | Core |
| **Phase** | Phase 3 |
| **Tujuan** | Laporan keuangan konsolidasi yayasan dari seluruh unit. |
| **Fitur** | Consolidated financial data, Unit consolidation, Elimination, Adjustment |
| **Sub-fitur** | Unit reporting, consolidated revenue/expense/assets/liabilities/net assets, inter-unit, elimination, adjustment, consolidated journal/report, history |

| ID | Kebutuhan | Prio | Scope | Phase |
|---|---|---|---|---|
| FR-38-001 | Sistem harus menghasilkan laporan konsolidasi yayasan dari entitas seluruh unit. | M | Core | Phase 3 |
| FR-38-002 | Sistem harus mengidentifikasi transaksi antar-unit dan menerapkan BR-ACC-04 [ketersediaan fitur eliminasi di Accounting Engine perlu dikonfirmasi, OQ-02]. | M | Core | Phase 3 |
| FR-38-003 | Sistem harus menyediakan jurnal penyesuaian konsolidasi beserta riwayatnya. | S | Core | Phase 3 |

| ID | Skenario | Given / When / Then |
|---|---|---|
| AC-38-01 | Konsolidasi dengan eliminasi | **Given** unit "SD" mencatat piutang Rp10.000.000 ke unit "SMP" dan SMP mencatat utang yang sama, dan eliminasi sudah disetujui<br>**When** laporan konsolidasi yayasan dibuat<br>**Then** piutang dan utang antar-unit tersebut tidak muncul dalam laporan konsolidasi |

### Grup Menu 6 — Operational

*Phase 4 · Core, dengan bagian akademik sebagai add-on*

#### 39. Class Management

| Atribut | Nilai |
|---|---|
| **Portal** | Admin / Academic |
| **Peran** | School Admin; Academic Admin |
| **Scope** | Campuran (lihat per kebutuhan) |
| **Phase** | Phase 4 |
| **Tujuan** | Data kelas (rombel) per unit dan tahun ajaran. |
| **Fitur** | Class management, Class assignment |
| **Sub-fitur** | Create/edit, year/unit/level, room/capacity, students, homeroom, status, history |

| ID | Kebutuhan | Prio | Scope | Phase |
|---|---|---|---|---|
| FR-39-001 | Sistem harus mengelola kelas per unit, tahun ajaran, dan tingkat, beserta kapasitas dan daftar siswanya. | M | Core | Phase 4 |
| FR-39-002 | Sistem harus menetapkan tepat satu wali kelas untuk setiap kelas. | M | Core | Phase 4 |
| FR-39-003 | Sistem harus menyimpan riwayat perpindahan siswa antar kelas. | S | Core | Phase 4 |
| FR-39-004 | Sistem harus menautkan kelas ke ruang dari master ruang (menu 116). | S | Add-on | Phase 4 |

| ID | Skenario | Given / When / Then |
|---|---|---|
| AC-39-01 | Satu wali kelas per kelas | **Given** kelas "7A" sudah memiliki wali kelas<br>**When** admin menetapkan guru lain sebagai wali kelas 7A<br>**Then** wali kelas berganti dan kelas 7A tetap memiliki tepat satu wali kelas |

#### 40. Teacher/Class Assignment

| Atribut | Nilai |
|---|---|
| **Portal** | Admin / Academic / Teacher |
| **Peran** | Academic Admin; School Admin; Teacher |
| **Scope** | Add-on |
| **Phase** | Phase 4 |
| **Tujuan** | Penugasan guru ke mata pelajaran dan kelas. Penugasan wali kelas ada di menu 39. |
| **Fitur** | Teacher assignment, Class assignment, Subject assignment |
| **Sub-fitur** | Teacher/class/subject, teaching hours, workload, multiple classes, schedule, history, unassigned detection |

| ID | Kebutuhan | Prio | Scope | Phase |
|---|---|---|---|---|
| FR-40-001 | Sistem harus menugaskan guru ke mata pelajaran dan kelas, termasuk lebih dari satu kelas. | M | Add-on | Phase 4 |
| FR-40-002 | Sistem harus menghitung jam mengajar dan beban kerja per guru, serta mendeteksi kelas atau mata pelajaran yang belum memiliki guru. | S | Add-on | Phase 4 |

| ID | Skenario | Given / When / Then |
|---|---|---|
| AC-40-01 | Kelas tanpa guru mata pelajaran | **Given** kelas 8B belum memiliki guru Matematika<br>**When** admin akademik membuka penugasan<br>**Then** kelas 8B ditandai belum lengkap untuk mata pelajaran Matematika |

#### 41. Attendance

| Atribut | Nilai |
|---|---|
| **Portal** | Admin / Academic / Teacher |
| **Peran** | Academic Admin; Teacher |
| **Scope** | Core |
| **Phase** | Phase 4 |
| **Tujuan** | Pengelolaan absensi di back office: status, koreksi, import, penguncian, dan notifikasi. Input oleh guru (harian dan per mata pelajaran) ada di menu 69. |
| **Fitur** | Daily attendance, Class attendance, Subject attendance |
| **Sub-fitur** | Present/late/absent/excused/sick, manual, import/edit/save, history, lock |

| ID | Kebutuhan | Prio | Scope | Phase |
|---|---|---|---|---|
| FR-41-001 | Sistem harus mendukung status kehadiran hadir, terlambat, izin, sakit, dan alpa. | M | Core | Phase 4 |
| FR-41-002 | Sistem harus memungkinkan Academic Admin menginput, mengimport, dan mengoreksi absensi harian kelas. | M | Core | Phase 4 |
| FR-41-003 | Sistem harus mengunci absensi harian setelah batas waktu yang dikonfigurasi dan menerapkan BR-ATT-01 [per klien]. | M | Core | Phase 4 |
| FR-41-004 | Sistem harus mengirim notifikasi kepada orang tua ketika anaknya tercatat alpa. | M | Core | Phase 4 |

| ID | Skenario | Given / When / Then |
|---|---|---|
| AC-41-01 | Notifikasi alpa | **Given** siswa "Dani" tercatat alpa tanggal 6 Oktober<br>**When** absensi disimpan<br>**Then** orang tua Dani menerima notifikasi ketidakhadiran |
| AC-41-02 | Koreksi setelah dikunci | **Given** absensi 6 Oktober kelas 7A sudah terkunci<br>**When** wali kelas mengubah status Dani dari alpa menjadi sakit<br>**Then** perubahan menunggu otorisasi Academic Admin dan tercatat di audit trail |

#### 42. Leave

| Atribut | Nilai |
|---|---|
| **Portal** | Admin / Academic / Parent / Teacher |
| **Peran** | School Admin; Academic Admin; Parent; Teacher |
| **Scope** | Core |
| **Phase** | Phase 4 |
| **Tujuan** | Persetujuan izin siswa. Pengajuan oleh orang tua ada di menu 48; cuti guru dan staf ada di menu 86. |
| **Fitur** | Leave request, Leave approval |
| **Sub-fitur** | Student/teacher/staff, type/period/duration/reason, attachment, approval/rejection, history/status |

| ID | Kebutuhan | Prio | Scope | Phase |
|---|---|---|---|---|
| FR-42-001 | Sistem harus memungkinkan wali kelas atau Academic Admin menyetujui atau menolak pengajuan izin siswa beserta alasannya. | S | Core | Phase 4 |
| FR-42-002 | Sistem harus menerapkan BR-ATT-02 ketika izin disetujui. | S | Core | Phase 4 |

| ID | Skenario | Given / When / Then |
|---|---|---|
| AC-42-01 | Izin disetujui | **Given** orang tua mengajukan izin untuk 7 Oktober dengan keterangan acara keluarga<br>**When** wali kelas menyetujui pengajuan<br>**Then** status kehadiran siswa tanggal 7 Oktober tercatat "izin" dan orang tua menerima notifikasi |

#### 43. Attendance Report

| Atribut | Nilai |
|---|---|
| **Portal** | Admin / Academic |
| **Peran** | School Admin; Academic Admin |
| **Scope** | Campuran (lihat per kebutuhan) |
| **Phase** | Phase 4 |
| **Tujuan** | Rekap dan analisis kehadiran siswa. Laporan kehadiran guru dan staf ada di menu 86. |
| **Fitur** | Attendance summary, Student report, Class report, Subject report, Teacher/staff report |
| **Sub-fitur** | Attendance/late/absence %, trend, class comparison, student detail, Excel/PDF/print |

| ID | Kebutuhan | Prio | Scope | Phase |
|---|---|---|---|---|
| FR-43-001 | Sistem harus menghasilkan rekap kehadiran harian, bulanan, dan semester per siswa, kelas, dan unit, termasuk persentase hadir, terlambat, dan tidak hadir. | M | Core | Phase 4 |
| FR-43-002 | Sistem harus menampilkan tren kehadiran dan perbandingan antar kelas, dan mengekspor laporan ke Excel atau PDF. | S | Core | Phase 4 |
| FR-43-003 | Sistem harus menghasilkan laporan kehadiran per mata pelajaran. | S | Add-on | Phase 4 |

| ID | Skenario | Given / When / Then |
|---|---|---|
| AC-43-01 | Rekap bulanan | **Given** Dani hadir 17 hari, terlambat 1 hari, sakit 1 hari, dan alpa 1 hari pada Oktober<br>**When** wali kelas membuka rekap Oktober<br>**Then** rekap Dani menampilkan hadir 17, terlambat 1, sakit 1, alpa 1 |

### Grup Menu 7 — Parent Portal

*Phase 5 · Core, kecuali menu 47 (add-on). Sebelum Phase 5, orang tua membayar melalui menu 16 dan 27.*

#### 44. Parent Dashboard

| Atribut | Nilai |
|---|---|
| **Portal** | Parent Portal |
| **Peran** | Parent / Guardian |
| **Scope** | Campuran (lihat per kebutuhan) |
| **Phase** | Phase 5 |
| **Tujuan** | Halaman awal orang tua untuk semua anak. |
| **Fitur** | Family overview, Child overview, Financial summary, Academic summary |
| **Sub-fitur** | Multi-child, child status, attendance, schedule, outstanding, payment, announcement, notification, exam, quick action |

| ID | Kebutuhan | Prio | Scope | Phase |
|---|---|---|---|---|
| FR-44-001 | Sistem harus menampilkan ringkasan per anak: status, kehadiran hari ini, total tagihan outstanding, pembayaran terakhir, pengumuman, dan notifikasi terbaru. | M | Core | Phase 5 |
| FR-44-002 | Sistem harus menyediakan aksi cepat: bayar tagihan, ajukan izin, dan lihat kuitansi. | S | Core | Phase 5 |
| FR-44-003 | Sistem harus menampilkan ringkasan jadwal hari ini dan ujian terdekat anak, bersumber dari menu 47. | S | Add-on | Phase 5 |

| ID | Skenario | Given / When / Then |
|---|---|---|
| AC-44-01 | Ringkasan dua anak | **Given** Ibu Sari memiliki dua anak dan anak pertama memiliki tagihan outstanding Rp400.000<br>**When** Ibu Sari membuka dashboard<br>**Then** dashboard menampilkan kedua anak dan tagihan outstanding Rp400.000 pada anak pertama |

#### 45. My Children

| Atribut | Nilai |
|---|---|
| **Portal** | Parent Portal |
| **Peran** | Parent / Guardian |
| **Scope** | Core |
| **Phase** | Phase 5 |
| **Tujuan** | Daftar anak yang tertaut ke akun orang tua. |
| **Fitur** | Child management, Child switching |
| **Sub-fitur** | Child list, profile, unit/level/class, homeroom, student ID, status, switch |

| ID | Kebutuhan | Prio | Scope | Phase |
|---|---|---|---|---|
| FR-45-001 | Sistem harus menampilkan daftar anak yang tertaut sesuai BR-GEN-03, beserta unit, tingkat, kelas, wali kelas, NIS, dan status. | M | Core | Phase 5 |
| FR-45-002 | Sistem harus memungkinkan orang tua berpindah konteks antar anak tanpa keluar dari aplikasi. | M | Core | Phase 5 |

| ID | Skenario | Given / When / Then |
|---|---|---|
| AC-45-01 | Berpindah antar anak | **Given** Ibu Sari memiliki anak di unit "SD" dan "SMP"<br>**When** Ibu Sari memilih anak di unit SMP<br>**Then** seluruh menu menampilkan data anak di unit SMP |

#### 46. Child Profile

| Atribut | Nilai |
|---|---|
| **Portal** | Parent Portal |
| **Peran** | Parent / Guardian |
| **Scope** | Core |
| **Phase** | Phase 5 |
| **Tujuan** | Profil anak untuk orang tua (hanya baca). |
| **Fitur** | Student profile, Parent relationship |
| **Sub-fitur** | Personal, ID, class/unit/program, homeroom, parent/contact, status |

| ID | Kebutuhan | Prio | Scope | Phase |
|---|---|---|---|---|
| FR-46-001 | Sistem harus menampilkan data pribadi, NIS/NISN, kelas, unit, program, wali kelas, kontak orang tua, dan status anak. | M | Core | Phase 5 |

| ID | Skenario | Given / When / Then |
|---|---|---|
| AC-46-01 | Tidak dapat melihat anak lain | **Given** siswa "Eko" tidak tertaut ke akun Ibu Sari<br>**When** Ibu Sari membuka tautan profil Eko secara langsung<br>**Then** akses ditolak dan data Eko tidak ditampilkan |

#### 47. Academic

| Atribut | Nilai |
|---|---|
| **Portal** | Parent Portal |
| **Peran** | Parent / Guardian |
| **Scope** | Add-on |
| **Phase** | Phase 5 |
| **Tujuan** | Informasi akademik anak bagi orang tua. Bergantung pada add-on e-Rapor/LMS (Grup Menu 9–10). |
| **Fitur** | Schedule, Learning materials, Examination, Grades, Report card |
| **Sub-fitur** | Daily/weekly, subject/teacher/room, materials, exam, scores, report, attendance, notes, download |

| ID | Kebutuhan | Prio | Scope | Phase |
|---|---|---|---|---|
| FR-47-001 | Sistem harus menampilkan jadwal, materi, jadwal dan hasil ujian, nilai, serta catatan guru untuk anak yang dipilih. | S | Add-on | Phase 5 |
| FR-47-002 | Sistem harus menyediakan unduhan rapor anak setelah rapor dipublikasikan. | S | Add-on | Phase 5 |

| ID | Skenario | Given / When / Then |
|---|---|---|
| AC-47-01 | Nilai ujian terlihat | **Given** guru telah mempublikasikan nilai ujian tengah semester<br>**When** orang tua membuka menu akademik<br>**Then** nilai ujian tengah semester anak ditampilkan |

#### 48. Attendance & Leave

| Atribut | Nilai |
|---|---|
| **Portal** | Parent Portal |
| **Peran** | Parent / Guardian |
| **Scope** | Core |
| **Phase** | Phase 5 |
| **Tujuan** | Kehadiran anak dan pengajuan izin oleh orang tua. |
| **Fitur** | Attendance monitoring, Leave request |
| **Sub-fitur** | Today/history, status, percentage/calendar, submit leave, reason/attachment, status/approval |

| ID | Kebutuhan | Prio | Scope | Phase |
|---|---|---|---|---|
| FR-48-001 | Sistem harus menampilkan kehadiran hari ini, riwayat, persentase, dan kalender kehadiran anak. | M | Core | Phase 5 |
| FR-48-002 | Sistem harus memungkinkan orang tua mengajukan izin atau sakit dengan alasan dan lampiran, serta memantau status persetujuannya. | S | Core | Phase 5 |

| ID | Skenario | Given / When / Then |
|---|---|---|
| AC-48-01 | Riwayat kehadiran | **Given** anak tercatat sakit tanggal 6 Oktober<br>**When** orang tua membuka riwayat kehadiran<br>**Then** tanggal 6 Oktober tampil dengan status "sakit" |

#### 49. Parent Finance

| Atribut | Nilai |
|---|---|
| **Portal** | Parent Portal |
| **Peran** | Parent / Guardian |
| **Scope** | Core |
| **Phase** | Phase 5 |
| **Tujuan** | Seluruh informasi keuangan anak bagi orang tua. Proses bayar memakai menu 27–28. |
| **Fitur** | Billing, Payment, Transaction history |
| **Sub-fitur** | Active/outstanding/paid/partial/overdue, invoice, installment, checkout, methods, confirmation, result, history, receipt |

| ID | Kebutuhan | Prio | Scope | Phase |
|---|---|---|---|---|
| FR-49-001 | Sistem harus menampilkan tagihan aktif dengan status belum bayar, sebagian, lunas, atau lewat jatuh tempo, beserta termin cicilan dan sisa tagihan. | M | Core | Phase 5 |
| FR-49-002 | Sistem harus menampilkan riwayat transaksi dan menyediakan unduhan invoice dan kuitansi. | M | Core | Phase 5 |

| ID | Skenario | Given / When / Then |
|---|---|---|
| AC-49-01 | Melihat sisa tagihan | **Given** anak memiliki tagihan SPP Rp1.000.000 dengan terbayar Rp600.000<br>**When** orang tua membuka menu keuangan<br>**Then** tagihan tampil "sebagian" dengan sisa Rp400.000 dan tombol bayar tersedia |

#### 50. Parent Information & Settings

| Atribut | Nilai |
|---|---|
| **Portal** | Parent Portal |
| **Peran** | Parent / Guardian |
| **Scope** | Core |
| **Phase** | Phase 5 |
| **Tujuan** | Notifikasi, pengumuman, kontak, dan bantuan bagi orang tua. Kata sandi, sesi, dan preferensi notifikasi diatur di CC-05 dan CC-08. |
| **Fitur** | Notification, Announcement, Profile, Account settings |
| **Sub-fitur** | School/academic/payment/attendance/exam/event notifications, profile, contact, password, preferences, security, session/logout |

| ID | Kebutuhan | Prio | Scope | Phase |
|---|---|---|---|---|
| FR-50-001 | Sistem harus menampilkan daftar notifikasi dan pengumuman yang dikirim dari menu 07 untuk orang tua tersebut. | M | Core | Phase 5 |
| FR-50-002 | Sistem harus memungkinkan orang tua memperbarui data kontaknya. | S | Core | Phase 5 |
| FR-50-003 | Sistem harus menyediakan panduan penggunaan dan FAQ di dalam aplikasi. | S | Core | Phase 1 |

| ID | Skenario | Given / When / Then |
|---|---|---|
| AC-50-01 | Panduan tersedia | **Given** orang tua baru pertama kali masuk<br>**When** orang tua membuka menu bantuan<br>**Then** panduan cara membayar tagihan dapat dibaca |

### Grup Menu 8 — Executive Dashboard

*Phase 6 · Core untuk indikator yang datanya tersedia di Grup Menu 1–7. Indikator akademik bergantung pada add-on; beberapa indikator belum memiliki sumber data (OQ-18).*

#### 51. Yayasan Dashboard

| Atribut | Nilai |
|---|---|
| **Portal** | Executive / Yayasan |
| **Peran** | Pengurus Yayasan; Yayasan Admin |
| **Scope** | Core |
| **Phase** | Phase 6 |
| **Tujuan** | Snapshot total yayasan saat ini. Perbandingan antar unit dan tren ada di menu 53; indikator per unit di menu 52. |
| **Fitur** | Executive KPI, Academic performance, Financial performance, Attendance, Strategic performance |
| **Sub-fitur** | Units, students, teachers/staff, revenue/expense/surplus, growth, scores, graduation, achievement, attendance, unit finance, strategic indicators, news, agenda |

| ID | Kebutuhan | Prio | Scope | Phase |
|---|---|---|---|---|
| FR-51-001 | Sistem harus menampilkan total yayasan saat ini: jumlah unit, siswa aktif, guru/staf, pendapatan, beban, surplus, dan kehadiran rata-rata. | M | Core | Phase 6 |

| ID | Skenario | Given / When / Then |
|---|---|---|
| AC-51-01 | Ringkasan yayasan | **Given** yayasan memiliki 3 unit dengan total 1.800 siswa aktif<br>**When** Pengurus Yayasan membuka dashboard<br>**Then** dashboard menampilkan 3 unit dan 1.800 siswa aktif |

#### 52. Unit Dashboard

| Atribut | Nilai |
|---|---|
| **Portal** | Executive / Unit |
| **Peran** | Kepala Sekolah; Unit Admin |
| **Scope** | Campuran (lihat per kebutuhan) |
| **Phase** | Phase 6 |
| **Tujuan** | Indikator satu unit untuk kepala sekolah. |
| **Fitur** | Unit KPI, Academic, Student, Financial, Operational performance |
| **Sub-fitur** | Students, teachers/staff, average score, attendance, revenue/expense, growth, class distribution, achievement, graduation, budget vs actual, teacher/facility, parent satisfaction, agenda |

| ID | Kebutuhan | Prio | Scope | Phase |
|---|---|---|---|---|
| FR-52-001 | Sistem harus menampilkan jumlah siswa, guru/staf, distribusi kelas, pertumbuhan, kehadiran, pendapatan, beban, collection rate, dan pendaftar PPDB untuk unit. | M | Core | Phase 6 |
| FR-52-002 | Sistem harus menampilkan rata-rata nilai, prestasi, dan kelulusan. | S | Add-on | Phase 6 |
| FR-52-003 | Sistem harus menampilkan anggaran vs realisasi. | S | Add-on | Phase 6 |
| FR-52-004 | Sistem harus menampilkan indikator guru/fasilitas, kepuasan orang tua, dan agenda unit [sumber data belum ada, OQ-18]. | S | Add-on | Phase 6 |

| ID | Skenario | Given / When / Then |
|---|---|---|
| AC-52-01 | Collection rate unit | **Given** total tagihan jatuh tempo Oktober unit "SD" Rp100.000.000 dan pembayaran teralokasi Rp85.000.000<br>**When** kepala unit membuka dashboard Oktober<br>**Then** collection rate tampil 85% |

#### 53. Consolidated Dashboard

| Atribut | Nilai |
|---|---|
| **Portal** | Executive / Yayasan |
| **Peran** | Pengurus Yayasan; Yayasan Admin |
| **Scope** | Campuran (lihat per kebutuhan) |
| **Phase** | Phase 6 |
| **Tujuan** | Perbandingan antar unit dan tren tingkat yayasan. Angka keuangan bersumber dari menu 38. |
| **Fitur** | Consolidated KPI, Unit comparison, Consolidated financial, Consolidated academic, Strategic monitoring |
| **Sub-fitur** | Revenue/expense/surplus, students, staff, units, trends, unit comparisons, growth, attendance, graduation, achievement, satisfaction, digitalization, agenda |

| ID | Kebutuhan | Prio | Scope | Phase |
|---|---|---|---|---|
| FR-53-001 | Sistem harus menampilkan perbandingan antar unit dan tren untuk pendapatan, beban, surplus, siswa, staf, pertumbuhan, kehadiran, dan collection rate. | S | Core | Phase 6 |
| FR-53-002 | Sistem harus menampilkan perbandingan kelulusan dan prestasi antar unit. | S | Add-on | Phase 6 |
| FR-53-003 | Sistem harus menampilkan indikator strategis, berita, digitalisasi, kepuasan, dan agenda tingkat yayasan [sumber data belum ada, OQ-18]. | S | Add-on | Phase 6 |

| ID | Skenario | Given / When / Then |
|---|---|---|
| AC-53-01 | Perbandingan antar unit | **Given** yayasan memiliki unit "SD", "SMP", dan "SMK"<br>**When** Yayasan Admin membuka dashboard konsolidasi<br>**Then** indikator ketiga unit tampil berdampingan beserta totalnya, dan nilai konsolidasi dibedakan dari nilai unit |

### Grup Menu 9 — Student Portal

*Phase 7 · Add-on (bundel e-Rapor/LMS). Ditulis di level fitur; dirinci saat add-on dipesan.*

#### 54. Student Dashboard

| Atribut | Nilai |
|---|---|
| **Portal** | Student Portal |
| **Peran** | Student |
| **Scope** | Add-on |
| **Phase** | Phase 7 |
| **Tujuan** | Halaman awal siswa. |
| **Fitur** | Academic overview, Daily activity, Notification |
| **Sub-fitur** | Today schedule, assignments, exams, attendance, latest grade, announcements, calendar, notifications |

| ID | Kebutuhan | Prio | Scope | Phase |
|---|---|---|---|---|
| FR-54-001 | Sistem harus menampilkan jadwal hari ini, tugas yang belum dikumpulkan, ujian terdekat, kehadiran, nilai terbaru, dan pengumuman. | S | Add-on | Phase 7 |

| ID | Skenario | Given / When / Then |
|---|---|---|
| AC-54-01 | Tugas belum dikumpulkan | **Given** siswa memiliki 2 tugas yang belum dikumpulkan<br>**When** siswa membuka dashboard<br>**Then** dashboard menampilkan 2 tugas tersebut beserta tenggatnya |

#### 55. Student Profile

| Atribut | Nilai |
|---|---|
| **Portal** | Student Portal |
| **Peran** | Student |
| **Scope** | Add-on |
| **Phase** | Phase 7 |
| **Tujuan** | Profil siswa (hanya baca). |
| **Fitur** | Student profile, Account |
| **Sub-fitur** | Personal, ID, class/unit/program, homeroom, parent info, account |

| ID | Kebutuhan | Prio | Scope | Phase |
|---|---|---|---|---|
| FR-55-001 | Sistem harus menampilkan data pribadi, NIS, kelas, unit, program, wali kelas, dan info orang tua milik siswa sendiri sesuai BR-GEN-04. | S | Add-on | Phase 7 |

| ID | Skenario | Given / When / Then |
|---|---|---|
| AC-55-01 | Hanya data sendiri | **Given** siswa A masuk ke portal<br>**When** siswa A membuka profil<br>**Then** hanya data siswa A yang ditampilkan |

#### 56. Schedule

| Atribut | Nilai |
|---|---|
| **Portal** | Student Portal |
| **Peran** | Student |
| **Scope** | Add-on |
| **Phase** | Phase 7 |
| **Tujuan** | Jadwal pelajaran siswa. |
| **Fitur** | Daily schedule, Weekly schedule |
| **Sub-fitur** | Subject, teacher, room, time, detail |

| ID | Kebutuhan | Prio | Scope | Phase |
|---|---|---|---|---|
| FR-56-001 | Sistem harus menampilkan jadwal harian dan mingguan berisi mata pelajaran, guru, ruang, dan waktu. | S | Add-on | Phase 7 |

| ID | Skenario | Given / When / Then |
|---|---|---|
| AC-56-01 | Jadwal mingguan | **Given** jadwal kelas 7A sudah disusun<br>**When** siswa kelas 7A membuka jadwal mingguan<br>**Then** jadwal Senin–Jumat kelas 7A ditampilkan |

#### 57. Learning Materials

| Atribut | Nilai |
|---|---|
| **Portal** | Student Portal |
| **Peran** | Student |
| **Scope** | Add-on |
| **Phase** | Phase 7 |
| **Tujuan** | Akses materi dari guru (menu 70). |
| **Fitur** | Subject materials, Learning resources |
| **Sub-fitur** | List, subject, teacher, file, link, download, history |

| ID | Kebutuhan | Prio | Scope | Phase |
|---|---|---|---|---|
| FR-57-001 | Sistem harus menampilkan materi yang dipublikasikan untuk kelas siswa, per mata pelajaran, dengan unduhan file dan tautan. | S | Add-on | Phase 7 |

| ID | Skenario | Given / When / Then |
|---|---|---|
| AC-57-01 | Materi baru | **Given** guru mempublikasikan materi untuk kelas 7A<br>**When** siswa 7A membuka materi<br>**Then** materi tersebut tampil dan dapat diunduh |

#### 58. Assignment

| Atribut | Nilai |
|---|---|
| **Portal** | Student Portal |
| **Peran** | Student |
| **Scope** | Add-on |
| **Phase** | Phase 7 |
| **Tujuan** | Mengerjakan dan mengumpulkan tugas (dibuat di menu 71). |
| **Fitur** | Assignment management, Submission |
| **Sub-fitur** | List/detail, due date, instruction, upload, submission, status, late, feedback |

| ID | Kebutuhan | Prio | Scope | Phase |
|---|---|---|---|---|
| FR-58-001 | Sistem harus memungkinkan siswa melihat instruksi tugas, mengunggah jawaban sebelum atau sesudah tenggat, dan melihat status serta umpan balik. | S | Add-on | Phase 7 |

| ID | Skenario | Given / When / Then |
|---|---|---|
| AC-58-01 | Pengumpulan terlambat | **Given** tenggat tugas 10 Oktober pukul 23.59<br>**When** siswa mengunggah jawaban 11 Oktober<br>**Then** pengumpulan diterima dan ditandai "terlambat" |

#### 59. Examination

| Atribut | Nilai |
|---|---|
| **Portal** | Student Portal |
| **Peran** | Student |
| **Scope** | Add-on |
| **Phase** | Phase 7 |
| **Tujuan** | Informasi ujian siswa (dikelola di menu 72). |
| **Fitur** | Exam schedule, Exam information, Exam result |
| **Sub-fitur** | Date, subject, time, room, instruction, status, result |

| ID | Kebutuhan | Prio | Scope | Phase |
|---|---|---|---|---|
| FR-59-001 | Sistem harus menampilkan jadwal, ruang, instruksi, status, dan hasil ujian setelah dipublikasikan [pelaksanaan ujian online, OQ-06]. | S | Add-on | Phase 7 |

| ID | Skenario | Given / When / Then |
|---|---|---|
| AC-59-01 | Hasil belum dipublikasikan | **Given** nilai ujian Matematika belum dipublikasikan guru<br>**When** siswa membuka hasil ujian<br>**Then** status tampil "belum tersedia" |

#### 60. Grade / Nilai

| Atribut | Nilai |
|---|---|
| **Portal** | Student Portal |
| **Peran** | Student |
| **Scope** | Add-on |
| **Phase** | Phase 7 |
| **Tujuan** | Nilai siswa (dihitung di menu 73). |
| **Fitur** | Grade overview, Grade history |
| **Sub-fitur** | Assignment, exam, midterm, final, subject, semester, history |

| ID | Kebutuhan | Prio | Scope | Phase |
|---|---|---|---|---|
| FR-60-001 | Sistem harus menampilkan nilai tugas, ujian, tengah semester, dan akhir per mata pelajaran dan semester beserta riwayatnya. | S | Add-on | Phase 7 |

| ID | Skenario | Given / When / Then |
|---|---|---|
| AC-60-01 | Nilai per semester | **Given** nilai semester ganjil sudah dipublikasikan<br>**When** siswa memilih semester ganjil<br>**Then** nilai seluruh mata pelajaran semester ganjil ditampilkan |

#### 61. Attendance

| Atribut | Nilai |
|---|---|
| **Portal** | Student Portal |
| **Peran** | Student |
| **Scope** | Add-on |
| **Phase** | Phase 7 |
| **Tujuan** | Kehadiran siswa sendiri (data dari menu 41 dan 69). |
| **Fitur** | Attendance monitoring |
| **Sub-fitur** | Today/history, status, percentage, calendar |

| ID | Kebutuhan | Prio | Scope | Phase |
|---|---|---|---|---|
| FR-61-001 | Sistem harus menampilkan kehadiran hari ini, riwayat, persentase, dan kalender kehadiran siswa sendiri. | S | Add-on | Phase 7 |

| ID | Skenario | Given / When / Then |
|---|---|---|
| AC-61-01 | Persentase kehadiran | **Given** siswa hadir 18 dari 20 hari pada Oktober<br>**When** siswa membuka kehadiran Oktober<br>**Then** persentase kehadiran tampil 90% |

#### 62. Announcement

| Atribut | Nilai |
|---|---|
| **Portal** | Student Portal |
| **Peran** | Student |
| **Scope** | Add-on |
| **Phase** | Phase 7 |
| **Tujuan** | Pengumuman untuk siswa (dikirim dari menu 07 dan 75). |
| **Fitur** | School announcement, Academic announcement, Activity |
| **Sub-fitur** | List/detail, date/category, attachment, read/unread |

| ID | Kebutuhan | Prio | Scope | Phase |
|---|---|---|---|---|
| FR-62-001 | Sistem harus menampilkan pengumuman sekolah dan akademik yang ditujukan kepada siswa, dengan kategori, lampiran, dan status baca. | S | Add-on | Phase 7 |

| ID | Skenario | Given / When / Then |
|---|---|---|
| AC-62-01 | Pengumuman kelas | **Given** guru mengirim pengumuman ke kelas 7A<br>**When** siswa 7A membuka pengumuman<br>**Then** pengumuman tampil dengan status "belum dibaca" |

#### 63. Calendar

| Atribut | Nilai |
|---|---|
| **Portal** | Student Portal |
| **Peran** | Student |
| **Scope** | Add-on |
| **Phase** | Phase 7 |
| **Tujuan** | Kalender akademik dan kegiatan siswa. |
| **Fitur** | Academic calendar, Personal academic calendar |
| **Sub-fitur** | Exam, assignment deadline, event, holiday, activity, detail |

| ID | Kebutuhan | Prio | Scope | Phase |
|---|---|---|---|---|
| FR-63-001 | Sistem harus menampilkan kalender berisi ujian, tenggat tugas, acara, hari libur, dan kegiatan. | S | Add-on | Phase 7 |

| ID | Skenario | Given / When / Then |
|---|---|---|
| AC-63-01 | Tenggat di kalender | **Given** tugas Bahasa Indonesia bertenggat 10 Oktober<br>**When** siswa membuka kalender Oktober<br>**Then** tenggat tugas tampil pada 10 Oktober |

#### 64. Student Settings

| Atribut | Nilai |
|---|---|
| **Portal** | Student Portal |
| **Peran** | Student |
| **Scope** | Add-on |
| **Phase** | Phase 7 |
| **Tujuan** | Pengaturan akun siswa. |
| **Fitur** | Account, Notification, Security |
| **Sub-fitur** | Profile, password, preferences, session, logout |

*Tidak ada kebutuhan spesifik; seluruh kebutuhan menu ini tercakup oleh CC-05 dan CC-08.*

### Grup Menu 10 — Teacher Portal

*Phase 8 · Add-on, kecuali bagian Core di menu 68 dan 69 untuk absensi harian (diusulkan dikirim di Phase 4, OQ-16).*

#### 65. Teacher Dashboard

| Atribut | Nilai |
|---|---|
| **Portal** | Teacher Portal |
| **Peran** | Teacher |
| **Scope** | Add-on |
| **Phase** | Phase 8 |
| **Tujuan** | Halaman awal guru. |
| **Fitur** | Teaching overview, Today's activity, Class overview, Attendance summary, Assignment summary, Notification |
| **Sub-fitur** | Schedule, classes, subjects, students, attendance, unassessed work, exams, announcements, notifications, quick actions |

| ID | Kebutuhan | Prio | Scope | Phase |
|---|---|---|---|---|
| FR-65-001 | Sistem harus menampilkan jadwal mengajar hari ini, kelas, absensi yang belum diisi, tugas yang belum dinilai, ujian terdekat, dan notifikasi. | S | Add-on | Phase 8 |

| ID | Skenario | Given / When / Then |
|---|---|---|
| AC-65-01 | Absensi belum diisi | **Given** guru belum mengisi absensi kelas 7A hari ini<br>**When** guru membuka dashboard<br>**Then** kelas 7A tampil di daftar absensi yang belum diisi |

#### 66. Teacher Profile

| Atribut | Nilai |
|---|---|
| **Portal** | Teacher Portal |
| **Peran** | Teacher |
| **Scope** | Add-on |
| **Phase** | Phase 8 |
| **Tujuan** | Profil guru. |
| **Fitur** | Teacher profile, Teaching information |
| **Sub-fitur** | Name, teacher ID, unit, subjects, position, classes, contact, photo, account |

| ID | Kebutuhan | Prio | Scope | Phase |
|---|---|---|---|---|
| FR-66-001 | Sistem harus menampilkan profil guru: nama, ID, unit, jabatan, mata pelajaran, kelas, kontak, dan foto. | S | Add-on | Phase 8 |

| ID | Skenario | Given / When / Then |
|---|---|---|
| AC-66-01 | Profil tampil | **Given** guru mengajar Matematika di kelas 7A dan 7B<br>**When** guru membuka profil<br>**Then** mata pelajaran dan kedua kelas tersebut tampil |

#### 67. Teaching Schedule

| Atribut | Nilai |
|---|---|
| **Portal** | Teacher Portal |
| **Peran** | Teacher |
| **Scope** | Add-on |
| **Phase** | Phase 8 |
| **Tujuan** | Jadwal mengajar. |
| **Fitur** | Daily schedule, Weekly schedule, Teaching calendar |
| **Sub-fitur** | Class, subject, room, time, period, conflict indicator |

| ID | Kebutuhan | Prio | Scope | Phase |
|---|---|---|---|---|
| FR-67-001 | Sistem harus menampilkan jadwal mengajar harian dan mingguan, serta menandai jadwal yang bentrok. | S | Add-on | Phase 8 |

| ID | Skenario | Given / When / Then |
|---|---|---|
| AC-67-01 | Jadwal bentrok | **Given** guru dijadwalkan di kelas 7A dan 8B pada jam yang sama<br>**When** guru membuka jadwal<br>**Then** kedua jadwal ditandai bentrok |

#### 68. My Classes

| Atribut | Nilai |
|---|---|
| **Portal** | Teacher Portal |
| **Peran** | Teacher |
| **Scope** | Campuran (lihat per kebutuhan) |
| **Phase** | Phase 4/8 |
| **Tujuan** | Kelas dan siswa yang diampu guru. |
| **Fitur** | Class list, Student list |
| **Sub-fitur** | Assigned classes, subject, student count, list, profile, class detail, attendance, performance |

| ID | Kebutuhan | Prio | Scope | Phase |
|---|---|---|---|---|
| FR-68-001 | Sistem harus menampilkan kelas yang menjadi tanggung jawab guru beserta daftar siswanya, sesuai BR-GEN-05. | M | Core | Phase 4 |
| FR-68-002 | Sistem harus menampilkan mata pelajaran yang diampu, profil siswa, dan ringkasan kinerja per kelas. | S | Add-on | Phase 8 |

| ID | Skenario | Given / When / Then |
|---|---|---|
| AC-68-01 | Hanya kelas sendiri | **Given** guru adalah wali kelas 7A<br>**When** guru membuka My Classes<br>**Then** hanya kelas 7A dan siswanya yang tampil |

#### 69. Attendance Management

| Atribut | Nilai |
|---|---|
| **Portal** | Teacher Portal |
| **Peran** | Teacher |
| **Scope** | Campuran (lihat per kebutuhan) |
| **Phase** | Phase 4/8 |
| **Tujuan** | Input absensi oleh guru. Status, koreksi, dan penguncian ada di menu 41. |
| **Fitur** | Student attendance, Attendance history |
| **Sub-fitur** | Class/date, statuses, bulk/individual, notes, edit, history, submit/lock |

| ID | Kebutuhan | Prio | Scope | Phase |
|---|---|---|---|---|
| FR-69-001 | Sistem harus memungkinkan wali kelas mencatat absensi harian kelasnya secara massal maupun per siswa, dengan catatan, lalu mengirimkannya. | M | Core | Phase 4 |
| FR-69-002 | Sistem harus memungkinkan guru mencatat absensi per mata pelajaran. | S | Add-on | Phase 8 |

| ID | Skenario | Given / When / Then |
|---|---|---|
| AC-69-01 | Pencatatan absensi harian | **Given** wali kelas 7A membuka absensi tanggal 6 Oktober<br>**When** wali kelas menandai semua siswa hadir lalu mengubah 1 siswa menjadi sakit dan mengirimkannya<br>**Then** 30 status kehadiran tersimpan untuk 6 Oktober |

#### 70. Learning Material

| Atribut | Nilai |
|---|---|
| **Portal** | Teacher Portal |
| **Peran** | Teacher |
| **Scope** | Add-on |
| **Phase** | Phase 8 |
| **Tujuan** | Membuat dan membagikan materi. |
| **Fitur** | Material management, Material distribution |
| **Sub-fitur** | Create, upload, link, subject/class, publish, draft, edit/delete, history |

| ID | Kebutuhan | Prio | Scope | Phase |
|---|---|---|---|---|
| FR-70-001 | Sistem harus memungkinkan guru membuat materi (file atau tautan) sebagai draft, mempublikasikannya ke kelas dan mata pelajaran tertentu, serta mengubahnya. | S | Add-on | Phase 8 |

| ID | Skenario | Given / When / Then |
|---|---|---|
| AC-70-01 | Draft belum terlihat | **Given** guru menyimpan materi sebagai draft<br>**When** siswa membuka materi<br>**Then** materi draft tidak tampil bagi siswa |

#### 71. Assignment

| Atribut | Nilai |
|---|---|
| **Portal** | Teacher Portal |
| **Peran** | Teacher |
| **Scope** | Add-on |
| **Phase** | Phase 8 |
| **Tujuan** | Membuat dan menilai tugas. |
| **Fitur** | Assignment management, Student submission, Assessment |
| **Sub-fitur** | Create, instruction, due date, attachment, assign, publish, submissions, late, review, feedback, score, resubmission |

| ID | Kebutuhan | Prio | Scope | Phase |
|---|---|---|---|---|
| FR-71-001 | Sistem harus memungkinkan guru membuat dan mempublikasikan tugas dengan instruksi, lampiran, dan tenggat. | S | Add-on | Phase 8 |
| FR-71-002 | Sistem harus memungkinkan guru meninjau pengumpulan, memberi nilai dan umpan balik, serta meminta pengumpulan ulang. | S | Add-on | Phase 8 |

| ID | Skenario | Given / When / Then |
|---|---|---|
| AC-71-01 | Penilaian tugas | **Given** siswa telah mengumpulkan tugas<br>**When** guru memberi nilai 85 dan umpan balik<br>**Then** nilai dan umpan balik tampil di portal siswa |

#### 72. Examination

| Atribut | Nilai |
|---|---|
| **Portal** | Teacher Portal |
| **Peran** | Teacher |
| **Scope** | Add-on |
| **Phase** | Phase 8 |
| **Tujuan** | Mengelola ujian dan hasilnya. |
| **Fitur** | Exam management, Exam schedule, Exam result |
| **Sub-fitur** | Create, type, subject/class, date/time/room, instruction, attachment, participation, score, publish result |

| ID | Kebutuhan | Prio | Scope | Phase |
|---|---|---|---|---|
| FR-72-001 | Sistem harus memungkinkan guru membuat ujian (jenis, mata pelajaran, kelas, jadwal, ruang, instruksi), mencatat kehadiran peserta dan nilai, lalu mempublikasikan hasil [OQ-06]. | S | Add-on | Phase 8 |

| ID | Skenario | Given / When / Then |
|---|---|---|
| AC-72-01 | Publikasi hasil | **Given** nilai ujian kelas 7A sudah diinput<br>**When** guru mempublikasikan hasil<br>**Then** hasil ujian tampil di portal siswa dan orang tua |

#### 73. Grade Management

| Atribut | Nilai |
|---|---|
| **Portal** | Teacher Portal |
| **Peran** | Teacher |
| **Scope** | Add-on |
| **Phase** | Phase 8 |
| **Tujuan** | Input dan perhitungan nilai. |
| **Fitur** | Grade input, Grade calculation, Grade history |
| **Sub-fitur** | Assignment/quiz/midterm/final/practical, weights, calculation, validation, submit, revision, history |

| ID | Kebutuhan | Prio | Scope | Phase |
|---|---|---|---|---|
| FR-73-001 | Sistem harus menghitung nilai akhir dari komponen tugas, kuis, tengah semester, akhir semester, dan praktik sesuai bobot yang dikonfigurasi [OQ-05]. | S | Add-on | Phase 8 |
| FR-73-002 | Sistem harus memvalidasi rentang nilai dan menyimpan riwayat revisi nilai setelah dikirim. | S | Add-on | Phase 8 |

| ID | Skenario | Given / When / Then |
|---|---|---|
| AC-73-01 | Nilai di luar rentang | **Given** rentang nilai 0–100<br>**When** guru menginput nilai 105<br>**Then** sistem menolak nilai dengan pesan rentang tidak valid |

#### 74. Student Performance

| Atribut | Nilai |
|---|---|
| **Portal** | Teacher Portal |
| **Peran** | Teacher |
| **Scope** | Add-on |
| **Phase** | Phase 8 |
| **Tujuan** | Memantau kinerja akademik siswa. |
| **Fitur** | Academic performance, Student monitoring |
| **Sub-fitur** | Score, attendance, completion, exam performance, trend, class comparison, detail, notes |

| ID | Kebutuhan | Prio | Scope | Phase |
|---|---|---|---|---|
| FR-74-001 | Sistem harus menampilkan nilai, kehadiran, penyelesaian tugas, dan tren kinerja per siswa serta perbandingan antar kelas yang diampu. | S | Add-on | Phase 8 |

| ID | Skenario | Given / When / Then |
|---|---|---|
| AC-74-01 | Tren menurun | **Given** rata-rata nilai siswa turun dari 85 ke 70 dalam dua bulan<br>**When** guru membuka kinerja siswa<br>**Then** tren penurunan ditampilkan |

#### 75. Announcement & Communication

| Atribut | Nilai |
|---|---|
| **Portal** | Teacher Portal |
| **Peran** | Teacher |
| **Scope** | Add-on |
| **Phase** | Phase 8 |
| **Tujuan** | Pengumuman kelas dan komunikasi dengan orang tua (memakai mesin notifikasi menu 07). |
| **Fitur** | Class announcement, Student communication, Parent communication |
| **Sub-fitur** | Create, audience, publish, attachment, history, read/unread, parent communication, notification |

| ID | Kebutuhan | Prio | Scope | Phase |
|---|---|---|---|---|
| FR-75-001 | Sistem harus memungkinkan guru mengirim pengumuman ke kelas yang diampu dan pesan ke orang tua siswa di kelas tersebut, dengan lampiran dan status baca. | S | Add-on | Phase 8 |

| ID | Skenario | Given / When / Then |
|---|---|---|
| AC-75-01 | Audiens dibatasi | **Given** guru mengampu kelas 7A saja<br>**When** guru membuat pengumuman<br>**Then** pilihan audiens hanya kelas 7A dan orang tua siswa 7A |

#### 76. Teacher Reports

| Atribut | Nilai |
|---|---|
| **Portal** | Teacher Portal |
| **Peran** | Teacher |
| **Scope** | Add-on |
| **Phase** | Phase 8 |
| **Tujuan** | Laporan untuk guru. |
| **Fitur** | Teaching report, Attendance report, Grade report |
| **Sub-fitur** | Class/student attendance, assignment completion, grade summary, performance, teaching activity, PDF/Excel |

| ID | Kebutuhan | Prio | Scope | Phase |
|---|---|---|---|---|
| FR-76-001 | Sistem harus menghasilkan laporan kehadiran, penyelesaian tugas, ringkasan nilai, dan aktivitas mengajar untuk kelas yang diampu, dalam PDF atau Excel. | S | Add-on | Phase 8 |

| ID | Skenario | Given / When / Then |
|---|---|---|
| AC-76-01 | Ekspor laporan nilai | **Given** guru mengampu kelas 7A<br>**When** guru mengekspor ringkasan nilai kelas 7A ke Excel<br>**Then** file berisi nilai seluruh siswa 7A |

#### 77. Teacher Settings

| Atribut | Nilai |
|---|---|
| **Portal** | Teacher Portal |
| **Peran** | Teacher |
| **Scope** | Add-on |
| **Phase** | Phase 8 |
| **Tujuan** | Pengaturan akun guru. |
| **Fitur** | Account settings, Notification, Security |
| **Sub-fitur** | Profile, password, notification preferences, session, login history, logout |

*Tidak ada kebutuhan spesifik; seluruh kebutuhan menu ini tercakup oleh CC-05 dan CC-08.*

### Grup Menu 11 — Core Extensions

*Menu 78–80 dikirim bersama phase modul yang membutuhkannya; menu 81 di Phase 5; menu 82 di Phase 7 (bergantung pada Student Portal).*

#### 78. Transfer & Withdrawal Siswa

| Atribut | Nilai |
|---|---|
| **Portal** | Admin / Academic / Finance |
| **Peran** | School Admin; Academic Admin; Finance Admin; Finance Manager |
| **Scope** | Core |
| **Phase** | Phase 2 |
| **Tujuan** | Memproses perpindahan siswa antar kelas/unit dan siswa keluar tanpa menghapus riwayat, termasuk penyelesaian keuangannya. |
| **Fitur** | Internal transfer, Withdrawal, Financial clearance |
| **Sub-fitur** | Pengajuan, alasan, tanggal efektif, dokumen pendukung, ringkasan keuangan, approval, perubahan status, refund deposit, penonaktifan akses |

| ID | Kebutuhan | Prio | Scope | Phase |
|---|---|---|---|---|
| FR-78-001 | Sistem harus mencatat pengajuan pindah (antar kelas atau antar unit) dan pengajuan keluar dengan alasan, tanggal efektif, dan dokumen pendukung. | M | Core | Phase 2 |
| FR-78-002 | Sistem harus menampilkan ringkasan keuangan siswa saat pengajuan dibuat: tagihan terbuka, tagihan yang belum jatuh tempo, deposit, dan pembayaran yang dapat dikembalikan. | M | Core | Phase 2 |
| FR-78-003 | Sistem harus menjalankan pengajuan melalui workflow approval (menu 79) yang mencakup persetujuan keuangan sebelum status siswa berubah (BR-STU-02). | M | Core | Phase 2 |
| FR-78-004 | Sistem harus menerapkan BR-STU-03 untuk perpindahan antar unit. | M | Core | Phase 2 |
| FR-78-005 | Sistem harus mengubah status siswa menjadi keluar per tanggal efektif, menghentikan penerbitan tagihan berikutnya, dan memperlakukan tagihan yang jatuh tempo setelah tanggal efektif sesuai kebijakan yang dikonfigurasi [OQ-07]. | M | Core | Phase 2 |
| FR-78-006 | Sistem harus membuat pengajuan refund (menu 25) untuk deposit atau kelebihan bayar yang disetujui dikembalikan. | S | Core | Phase 2 |
| FR-78-007 | Sistem harus menonaktifkan akses portal siswa sejak tanggal efektif dan menonaktifkan akun orang tua setelah masa tenggang [N] hari bila tidak ada anak aktif lain (BR-GEN-07). | M | Core | Phase 2 |

| ID | Skenario | Given / When / Then |
|---|---|---|
| AC-78-01 | Keluar dengan tunggakan | **Given** siswa "Rina" memiliki tunggakan SPP Rp800.000<br>**When** staf mengajukan withdrawal untuk Rina<br>**Then** pengajuan menampilkan tunggakan Rp800.000 dan menunggu persetujuan keuangan<br>**And** status Rina tidak berubah sebelum seluruh tahap approval selesai |
| AC-78-02 | Pindah antar unit tanpa kehilangan riwayat | **Given** siswa "Bayu" pindah dari unit "SMP" ke unit "SMK" mulai 1 Juli dan masih memiliki tunggakan Rp300.000 di SMP<br>**When** perpindahan disetujui<br>**Then** Bayu aktif di unit SMK, tagihan baru terbit di SMK<br>**And** riwayat dan tunggakan Rp300.000 tetap tercatat di unit SMP |
| AC-78-03 | Deposit dikembalikan | **Given** siswa yang keluar memiliki deposit Rp200.000<br>**When** keuangan menyetujui pengembalian<br>**Then** pengajuan refund Rp200.000 terbentuk di menu 25 |

#### 79. Approval Center

| Atribut | Nilai |
|---|---|
| **Portal** | Admin / Back Office |
| **Peran** | Super Admin (konfigurasi); semua peran penyetuju |
| **Scope** | Core |
| **Phase** | Phase 1 |
| **Tujuan** | Mesin approval terpusat untuk seluruh menu. Menu lain menentukan kapan approval dibutuhkan; menu ini menentukan siapa yang menyetujui dan bagaimana. |
| **Fitur** | Approval workflow, Approval inbox, Delegation, Escalation |
| **Sub-fitur** | Jenis approval, tahap, peran penyetuju, lingkup unit, batas nominal, maker-checker, kotak masuk, komentar, delegasi, batas waktu, eskalasi, riwayat |

| ID | Kebutuhan | Prio | Scope | Phase |
|---|---|---|---|---|
| FR-79-001 | Sistem harus memungkinkan admin mengatur alur approval per jenis permintaan (antara lain diskon khusus, perubahan tarif, penyesuaian/pembatalan tagihan, koreksi pembayaran, beasiswa, refund, jurnal, penyesuaian rekonsiliasi, hasil seleksi, transfer/withdrawal) dengan tahap, peran penyetuju, dan lingkup unit. | M | Core | Phase 1 |
| FR-79-002 | Sistem harus mendukung aturan berdasarkan batas nominal, misalnya permintaan di atas [Rp X] memerlukan tahap persetujuan tambahan. | M | Core | Phase 1 |
| FR-79-003 | Sistem harus menerapkan BR-GEN-09 (maker-checker) pada setiap tahap approval. | M | Core | Phase 1 |
| FR-79-004 | Sistem harus menyediakan kotak masuk approval per penyetuju dengan detail permintaan, serta aksi setujui atau tolak; komentar wajib saat menolak. | M | Core | Phase 1 |
| FR-79-005 | Sistem harus memungkinkan penyetuju mendelegasikan wewenangnya kepada pengguna lain dengan peran setara untuk periode tertentu. | S | Core | Phase 1 |
| FR-79-006 | Sistem harus mengeskalasi permintaan yang tidak ditindaklanjuti dalam [N] jam ke penyetuju berikutnya yang dikonfigurasi. | S | Core | Phase 1 |
| FR-79-007 | Sistem harus menampilkan riwayat approval (penyetuju, keputusan, waktu, komentar) pada transaksi asal. | M | Core | Phase 1 |

| ID | Skenario | Given / When / Then |
|---|---|---|
| AC-79-01 | Pengaju tidak dapat menyetujui sendiri | **Given** Finance Manager mengajukan refund Rp1.000.000<br>**When** Finance Manager yang sama membuka kotak masuk approval<br>**Then** refund tersebut tidak dapat disetujui olehnya dan menunggu penyetuju lain |
| AC-79-02 | Tahap tambahan berdasarkan nominal | **Given** aturan refund di atas Rp5.000.000 memerlukan persetujuan Kepala Unit setelah Finance Manager<br>**When** refund Rp6.000.000 disetujui Finance Manager<br>**Then** permintaan berpindah ke Kepala Unit dan belum diproses |
| AC-79-03 | Penolakan wajib komentar | **Given** penyetuju membuka permintaan perubahan tarif<br>**When** penyetuju menolak tanpa mengisi komentar<br>**Then** penolakan tidak dapat dikirim sampai komentar diisi |
| AC-79-04 | Eskalasi | **Given** batas waktu approval 24 jam<br>**When** permintaan tidak ditindaklanjuti selama 24 jam<br>**Then** permintaan dieskalasi ke penyetuju berikutnya dan kedua pihak menerima notifikasi |

#### 80. Integration Exception Queue

| Atribut | Nilai |
|---|---|
| **Portal** | Admin / Back Office |
| **Peran** | Super Admin; Finance Admin (pembayaran); Accounting (posting) |
| **Scope** | Core |
| **Phase** | Phase 1 |
| **Tujuan** | Satu tempat untuk melihat dan menyelesaikan kegagalan integrasi teknis. Selisih rekonsiliasi bisnis tetap ditangani di menu 31. |
| **Fitur** | Exception monitoring, Retry, Resolution |
| **Sub-fitur** | Webhook gagal, posting jurnal gagal, notifikasi gagal, event dead-letter, import gagal, correlation ID, retry, resolusi, penugasan |

| ID | Kebutuhan | Prio | Scope | Phase |
|---|---|---|---|---|
| FR-80-001 | Sistem harus menampilkan seluruh kegagalan integrasi: webhook pembayaran yang gagal diproses, posting jurnal gagal, notifikasi gagal terkirim, event yang masuk dead-letter, dan import yang gagal. | M | Core | Phase 1 |
| FR-80-002 | Sistem harus menampilkan detail setiap item: sumber, waktu, correlation ID, pesan galat, jumlah percobaan, dan ringkasan payload tanpa data sensitif. | M | Core | Phase 1 |
| FR-80-003 | Sistem harus menyediakan aksi retry yang aman tanpa membuat transaksi, alokasi, atau jurnal ganda (BR-INT-01). | M | Core | Phase 1 |
| FR-80-004 | Sistem harus memungkinkan item ditugaskan ke petugas dan ditandai selesai dengan alasan wajib; setiap aksi tercatat di audit trail. | M | Core | Phase 1 |
| FR-80-005 | Sistem harus membatasi item yang terlihat sesuai jenis dan peran (misalnya Finance Admin untuk pembayaran, Accounting untuk posting) serta lingkup unit. | M | Core | Phase 1 |
| FR-80-006 | Sistem harus mengirim notifikasi kepada penanggung jawab ketika item kritis baru muncul. | S | Core | Phase 1 |

| ID | Skenario | Given / When / Then |
|---|---|---|
| AC-80-01 | Retry posting berhasil | **Given** posting jurnal harian unit "SD" gagal karena Accounting Engine tidak tersedia<br>**When** akuntan menekan retry setelah layanan pulih<br>**Then** posting berhasil, item berstatus selesai<br>**And** tidak ada jurnal ganda di Accounting Engine |
| AC-80-02 | Selesai manual wajib alasan | **Given** item webhook gagal sudah ditangani di luar sistem<br>**When** petugas menandai selesai tanpa alasan<br>**Then** sistem menolak sampai alasan diisi |
| AC-80-03 | Data sensitif disembunyikan | **Given** item notifikasi gagal berisi nomor ponsel orang tua<br>**When** petugas membuka detail item<br>**Then** nomor ponsel ditampilkan dalam bentuk tersamar |

#### 81. Permintaan Layanan Orang Tua

| Atribut | Nilai |
|---|---|
| **Portal** | Parent Portal / Admin |
| **Peran** | Parent / Guardian; School Admin; peran penanggung jawab per kategori |
| **Scope** | Core |
| **Phase** | Phase 5 |
| **Tujuan** | Orang tua mengajukan permintaan layanan secara digital dan memantau penyelesaiannya. Pengaturan SLA dan eskalasi lanjutan ada di modul Service Desk (menu 137–138). |
| **Fitur** | Service request, Request handling |
| **Sub-fitur** | Kategori, formulir, lampiran, penugasan, status, tanggapan, dokumen hasil, notifikasi, riwayat |

| ID | Kebutuhan | Prio | Scope | Phase |
|---|---|---|---|---|
| FR-81-001 | Sistem harus memungkinkan orang tua mengajukan permintaan dengan kategori yang dikonfigurasi sekolah (contoh: surat keterangan, izin khusus, keluhan, pertanyaan tagihan), deskripsi, dan lampiran. | M | Core | Phase 5 |
| FR-81-002 | Sistem harus meneruskan permintaan ke unit dan peran penanggung jawab sesuai kategori. | M | Core | Phase 5 |
| FR-81-003 | Sistem harus memungkinkan petugas menanggapi, mengubah status (diterima, diproses, selesai, ditolak), dan melampirkan dokumen hasil. | M | Core | Phase 5 |
| FR-81-004 | Sistem harus menampilkan status dan riwayat permintaan kepada orang tua serta mengirim notifikasi setiap perubahan status. | M | Core | Phase 5 |

| ID | Skenario | Given / When / Then |
|---|---|---|
| AC-81-01 | Permintaan diteruskan sesuai kategori | **Given** kategori "Surat keterangan aktif" ditangani School Admin unit siswa<br>**When** orang tua mengajukan permintaan untuk anaknya di unit "SD"<br>**Then** permintaan masuk ke daftar kerja School Admin unit SD |
| AC-81-02 | Dokumen hasil diterima orang tua | **Given** School Admin melampirkan surat keterangan dan menandai selesai<br>**When** status disimpan<br>**Then** orang tua menerima notifikasi dan dapat mengunduh surat tersebut |

#### 82. Tagihan Saya

| Atribut | Nilai |
|---|---|
| **Portal** | Student Portal |
| **Peran** | Student; School Admin (pengaturan) |
| **Scope** | Add-on |
| **Phase** | Phase 7 |
| **Tujuan** | Siswa melihat tagihannya sendiri bila sekolah mengaktifkan fitur ini. Pembayaran tetap dilakukan orang tua. |
| **Fitur** | Student billing view |
| **Sub-fitur** | Pengaturan per unit/jenjang, daftar tagihan, status, sisa, jatuh tempo |

| ID | Kebutuhan | Prio | Scope | Phase |
|---|---|---|---|---|
| FR-82-001 | Sistem harus memungkinkan sekolah mengaktifkan atau menonaktifkan tampilan tagihan bagi siswa per unit dan jenjang. | S | Add-on | Phase 7 |
| FR-82-002 | Bila aktif, sistem harus menampilkan tagihan siswa yang bersangkutan beserta status, sisa, dan jatuh tempo tanpa aksi pembayaran (BR-GEN-04). | S | Add-on | Phase 7 |

| ID | Skenario | Given / When / Then |
|---|---|---|
| AC-82-01 | Fitur dinonaktifkan | **Given** tampilan tagihan siswa dinonaktifkan untuk jenjang SD<br>**When** siswa SD masuk ke portal siswa<br>**Then** menu Tagihan Saya tidak ditampilkan |
| AC-82-02 | Tampilan baca-saja | **Given** tampilan tagihan aktif untuk jenjang SMK<br>**When** siswa SMK membuka Tagihan Saya<br>**Then** tagihannya tampil tanpa tombol bayar |

### Grup Menu 12–33

Kebutuhan fungsional grup menu 12–33 (menu 83–159) dirinci di Master PRD Bagian 2, 3, dan 4. Daftar menunya tercantum di bab 6.

## 12. Alur Bisnis End-to-End

| Alur | Urutan (nomor menu) | Phase |
|---|---|---|
| PPDB ke siswa aktif | 09 Konfigurasi → 16 Pendaftaran & bayar formulir → 12 Verifikasi dokumen → 13 Seleksi → 14 Pengumuman → 15 Daftar ulang → 03 Master siswa → 19 Billing tahunan | Phase 1, billing di Phase 2 |
| Billing ke akuntansi | 18 Struktur biaya → 19 Billing → 27 Pembayaran → 21 Alokasi → 34 Jurnal otomatis → 35 Buku besar → 36 Neraca saldo | Phase 2–3 |
| Rekonsiliasi pembayaran | 27 Checkout → payment gateway → 29 Callback transaksi → 30 Settlement → 31 Rekonsiliasi → penyesuaian | Phase 1–2 |
| Pembayaran orang tua | Login → 45 Anak → 49 Keuangan → 27 Checkout → 28 Metode → konfirmasi → kuitansi (29) | Phase 1 (PPDB), Phase 2, Phase 5 |
| Absensi oleh guru | 68 Kelas saya → 69 Input absensi → kirim → 41 Penguncian → 43 Laporan → 48 Tampilan orang tua | Phase 4–5 |
| Penugasan guru (add-on) | 39 Kelas → 40 Penugasan → 67 Jadwal mengajar → 68 Kelas saya | Phase 4, Phase 8 |
| Pembelajaran siswa (add-on) | 70/71/72 Materi, tugas, ujian → 57/58/59 Portal siswa → 73 Nilai → 60/47 Tampilan siswa dan orang tua | Phase 7–8 |
| Konsolidasi keuangan | 34 Jurnal unit → 35 Buku besar → 37 Laporan unit → eliminasi/penyesuaian → 38 Konsolidasi → 53 Dashboard | Phase 3, Phase 6 |
| Izin | 48 Pengajuan orang tua → 42 Persetujuan → dampak ke absensi (BR-ATT-02) → notifikasi → riwayat dan audit | Phase 4–5 |
| Pindah / keluar siswa | 78 Pengajuan → ringkasan keuangan → 79 Approval (termasuk keuangan) → perubahan status → 25 Refund deposit → penonaktifan akses | Phase 2 |
| Approval | Transaksi di menu asal → 79 Kotak masuk penyetuju → setujui/tolak (maker-checker) → eskalasi bila lewat batas waktu → transaksi diproses | Phase 1 |
| Kegagalan integrasi | Kegagalan → retry otomatis → 80 Exception queue → retry aman / selesai dengan alasan → audit | Phase 1 |
| Permintaan layanan orang tua | 81 Pengajuan → penanggung jawab per kategori → tanggapan & dokumen → notifikasi → selesai | Phase 5 |

## 13. Integrasi & Antarmuka

| ID | Integrasi | Kebutuhan | Phase |
|---|---|---|---|
| IF-PG-01 | Payment gateway | Sistem harus membuat transaksi melalui API dengan ID tagihan, nominal, metode, dan masa berlaku. | Phase 1 |
| IF-PG-02 | Payment gateway | Sistem harus menerima notifikasi status melalui webhook dan memverifikasi tanda tangannya. | Phase 1 |
| IF-PG-03 | Payment gateway | Sistem harus mengambil laporan settlement harian untuk rekonsiliasi. | Phase 2 |
| IF-ACC-01 | Accounting Engine | Sistem harus mengirim posting jurnal melalui API berisi entitas unit, tanggal, akun, debit, kredit, dan referensi transaksi. | Phase 3 |
| IF-ACC-02 | Accounting Engine | Sistem harus menyimpan nomor jurnal dan status posting yang dikembalikan Accounting Engine. | Phase 3 |
| IF-ACC-03 | Accounting Engine | Sistem harus mengantrekan posting yang gagal dan mencobanya ulang otomatis hingga [5] kali sebelum memberi peringatan ke akuntan. | Phase 3 |
| IF-NT-01 | Email & WhatsApp | Sistem harus terhubung ke penyedia email (API/SMTP) dan WhatsApp Business API sebagai kanal pengiriman menu 07. | Phase 1 |
| IF-ST-01 | Object storage | Sistem harus menyimpan dokumen dan lampiran di object storage dengan akses terkontrol, tanpa URL publik. | Phase 1 |
| IF-SSO-01 | Identity provider | Sistem dapat mendukung SSO melalui OIDC/SAML jika disetujui (Could). | — |

**Kriteria penerimaan integrasi**

| ID | Skenario | Given / When / Then |
|---|---|---|
| AC-IF-01 | Webhook tidak valid | **Given** transaksi TX-001 berstatus pending<br>**When** sistem menerima webhook TX-001 dengan tanda tangan tidak valid<br>**Then** webhook ditolak, status TX-001 tetap pending, dan kejadian tercatat di log keamanan |
| AC-IF-02 | Webhook ganda (BR-PAY-01) | **Given** transaksi TX-002 Rp600.000 sudah sukses<br>**When** sistem menerima webhook sukses yang sama<br>**Then** tidak ada pembayaran kedua dan saldo terbayar tagihan tidak berubah |
| AC-IF-03 | Accounting Engine tidak merespons | **Given** posting harian unit SD siap dikirim dan Accounting Engine tidak merespons<br>**When** sistem mengirim posting<br>**Then** posting masuk antrean, dicoba ulang otomatis, dan akuntan mendapat peringatan setelah percobaan terakhir gagal |

**Ketentuan integrasi platform**

- Gunakan API terdokumentasi dan webhook bertanda tangan bila didukung; batch atau file terkontrol hanya bila API tidak tersedia.
- Setiap event yang dapat diulang memiliki idempotency key atau aturan deduplikasi yang deterministik.
- Kegagalan integrasi terlihat, dapat di-retry dengan aman, dan diselesaikan melalui menu 80.
- Secret dan kredensial disimpan di secret manager; tidak boleh masuk source code, log, atau ekspor.
- Setiap interface mendokumentasikan pemilik, pemetaan data, autentikasi, timeout, retry, rate limit, SLA, frekuensi rekonsiliasi, dan kontak support.

## 14. Migrasi & Kualitas Data

Migrasi data setiap klien mengikuti langkah berikut. Kebutuhan fungsionalnya ada di menu 04.

1. Inventaris sumber data: pemilik, volume, periode historis, format, kualitas, dan dependensi.
2. Pemetaan ID sumber ke ID baru, dengan ID sumber tetap dapat ditelusuri.
3. Profiling dan deteksi duplikasi siswa, orang tua, tagihan, pembayaran, alokasi, dan saldo.
4. Validasi field wajib, format, dan integritas referensi, dengan laporan galat yang dapat diunduh.
5. Migrasi percobaan dan rekonsiliasi bisnis sebelum cutover produksi.
6. Control totals untuk tagihan, pembayaran, alokasi, saldo piutang, dan jurnal.
7. Penetapan data yang dimigrasi, diarsipkan baca-saja, atau tetap di sistem sumber.
8. Rencana rollback, sign-off pemilik data, dan keputusan go/no-go.
9. Perpindahan dan keluarnya siswa diproses non-destruktif (BR-STU-01).

## 15. Kebutuhan Non-Fungsional

Nilai dalam [ ] adalah usulan awal dan dikonfirmasi dalam SLA sebelum go-live.

| ID | Area | Kebutuhan | Prio | Verif. |
|---|---|---|---|---|
| NFR-SEC-01 | Autentikasi | Semua portal memakai autentikasi aman; MFA wajib untuk Super Admin, Yayasan Admin, Finance Admin, Finance Manager, dan Accounting. | M | T |
| NFR-SEC-02 | Otorisasi | Pemeriksaan RBAC dan lingkup unit dilakukan di sisi server. | M | T |
| NFR-SEC-03 | Enkripsi | Lalu lintas data memakai TLS 1.2 atau lebih tinggi; data pribadi dan dokumen dienkripsi saat disimpan. | M | I |
| NFR-SEC-04 | Sesi | Sesi berakhir setelah [30] menit tidak aktif; akun terkunci sementara setelah [5] kali gagal login. | M | T |
| NFR-SEC-05 | Pembayaran | Sistem tidak menyimpan nomor kartu atau kredensial pembayaran. | M | I |
| NFR-SEC-06 | Data pribadi | Akses data siswa, orang tua, dan guru mengikuti prinsip least privilege; tersedia mekanisme permintaan akses, koreksi, dan penghapusan sesuai UU No. 27/2022; masa retensi [OQ-10]. | M | D |
| NFR-PERF-01 | Respons | Halaman daftar dan detail tampil ≤ [3] detik pada persentil ke-95 dengan koneksi 4G. | M | T |
| NFR-PERF-02 | Beban puncak | Melayani minimal [500] pengguna bersamaan saat pembukaan gelombang PPDB tanpa melanggar NFR-PERF-01. | M | T |
| NFR-PERF-03 | Pembayaran | Status tagihan diperbarui ≤ [60] detik sejak notifikasi gateway diterima. | M | T |
| NFR-PERF-04 | Billing massal | Billing tahunan untuk [1.800] siswa selesai ≤ [30] menit. | S | T |
| NFR-PERF-05 | Data besar | Daftar besar memakai paginasi dan filter di sisi server. | M | I |
| NFR-REL-01 | Ketersediaan | Ketersediaan minimal [99,5%] per bulan di luar pemeliharaan terjadwal. | M | A |
| NFR-REL-02 | Pemeliharaan | Pemeliharaan terjadwal di luar jam [06.00–18.00] WIB dan diumumkan minimal [3] hari sebelumnya. | M | I |
| NFR-REL-03 | Backup | Backup harian dengan RPO ≤ [24] jam dan RTO ≤ [8] jam. | M | T |
| NFR-SCAL-01 | Skalabilitas | Penambahan unit baru tidak memerlukan perubahan desain entitas inti. | M | D |
| NFR-OBS-01 | Observabilitas | Log aplikasi, integrasi, dan keamanan dapat dipantau terpusat. | S | I |
| NFR-UX-01 | Responsif | Portal orang tua, siswa, dan guru dapat dipakai di ponsel dengan lebar layar minimal 360 px. | M | D |
| NFR-UX-02 | Bahasa & format | Antarmuka berbahasa Indonesia; nominal ditampilkan dalam format Rupiah dengan pemisah ribuan titik. | M | I |
| NFR-UX-03 | Kegunaan | Orang tua baru dapat menyelesaikan pembayaran pertama tanpa bantuan ≤ [5] menit pada uji kegunaan minimal [10] peserta. | S | T |
| NFR-ACC-01 | Aksesibilitas | Antarmuka mengikuti WCAG 2.2 AA sejauh praktis, termasuk navigasi keyboard, kontras, dan label. | S | I |
| NFR-DATA-01 | Integritas keuangan | Total posting harian per unit sama dengan total transaksi sumbernya. | M | T |
| NFR-AUD-01 | Audit | Log audit tidak dapat diubah atau dihapus pengguna mana pun dan disimpan minimal [5] tahun. | M | I |
| NFR-MNT-01 | Konfigurasi | Perubahan konfigurasi (biaya, promo, jalur, aturan alokasi) tidak memerlukan deployment versi baru. | M | D |
| NFR-REL-04 | Idempotensi finansial | Retry tidak boleh menggandakan pembayaran, alokasi, payroll run, maupun jurnal; seluruh proses finansial dapat direkonsiliasi. | M | T |
| NFR-COMP-01 | Kompatibilitas | Daftar browser dan ukuran layar yang didukung dipublikasikan per rilis [OQ-11]. | M | I |
| NFR-PORT-01 | Portabilitas data | Data institusi dapat diekspor dalam format terdokumentasi, berotorisasi, dan tercatat. | M | D |
| NFR-LOC-01 | Lokalisasi | Bahasa Indonesia, format tanggal lokal, Rupiah, zona waktu, dan kalender sekolah dapat dikonfigurasi per klien. | M | I |
| NFR-QUAL-01 | Kualitas migrasi | Minimal [98%] record kritis lulus validasi konsistensi pada migrasi percobaan sebelum cutover. | M | A |
| NFR-QUAL-02 | Rekonsiliasi | Minimal [95%] transaksi pembayaran cocok otomatis dengan settlement; sisanya masuk laporan pengecualian. | S | A |
| NFR-TEN-01 | Isolasi klien | Setiap yayasan klien di-deploy terpisah (single-tenant): data, database, dan kredensial tidak dipakai bersama antar klien. | M | I |

## 16. Keamanan, Privasi & Kendala

### 15.1 Kontrol & segregation of duties

| Kontrol | Kebutuhan |
|---|---|
| Segregation of duties | Pembuat dan penyetuju dipisahkan untuk approval tarif, diskon khusus, pembatalan tagihan, koreksi pembayaran, refund, posting jurnal, settlement, dan penyesuaian rekonsiliasi [matriks approval, OQ-03]. |
| Imutabilitas keuangan | Catatan keuangan yang sudah diposting dikoreksi melalui pembalik/penyesuaian (BR-ACC-01). |
| Dokumen | Dokumen unggahan dikontrol aksesnya dan tidak terbuka untuk publik (IF-ST-01). |
| Audit | Aksi material tercatat (menu 06, NFR-AUD-01). |
| Retensi | Masa retensi data keuangan, siswa, absensi, dan audit ditetapkan bersama sekolah dan pihak hukum [OQ-10]. |

### 15.2 Kendala desain & kepatuhan

- Pembayaran digital hanya melalui penyedia jasa pembayaran berlisensi Bank Indonesia.
- Pengolahan data pribadi mematuhi UU No. 27 Tahun 2022 tentang Pelindungan Data Pribadi.
- Jurnal, buku besar, dan laporan keuangan dijalankan oleh Accounting Engine; SEKOCI tidak menyimpan buku besar sendiri.
- Format laporan keuangan mengikuti standar akuntansi entitas nonlaba yang dipakai yayasan [per klien].
- Data disimpan di pusat data yang berlokasi di Indonesia.

**Penguatan keamanan, privasi, dan audit**

- Segregation of duties dan maker-checker untuk refund, penyesuaian jurnal, perubahan tarif, dan aksi sensitif lainnya (BR-GEN-09).
- Perubahan relasi orang tua–anak memerlukan pemeriksaan otorisasi; orang tua hanya mengakses anak yang tertaut dan sah.
- Ekspor, perubahan massal, perubahan status/nilai sensitif, pembalikan pembayaran, finalisasi payroll, posting jurnal, dan aksi admin istimewa wajib tercatat.
- Retensi, arsip, penghapusan, dan legal hold ditetapkan per kategori data bersama pemilik bisnis dan legal; tidak ada satu masa retensi untuk semua data.
- Threat modeling dan pengujian keamanan sebelum produksi, termasuk isolasi klien, broken access control, upload file, manajemen sesi, injection, dan spoofing webhook.
- Kepatuhan pada regulasi Indonesia direview pihak yang kompeten sebelum go-live; dokumen ini tidak menyatakan sertifikasi atau kepatuhan otomatis.

## 17. Pelaporan, Analitik & Governance Metrik

- Laporan keuangan mencantumkan periode, unit, waktu pembuatan, dan basis laporan.
- Metrik dashboard memakai sumber data yang sama dengan laporan operasional yang setara.
- Nilai konsolidasi dapat dibedakan dari nilai per unit.
- Laporan menyediakan drilldown ke transaksi sumber yang diizinkan, sejauh memungkinkan.
- Definisi dan rumus metrik didokumentasikan di metric dictionary sebelum produksi.

**Metric dictionary (awal)**

| Metrik | Rumus | Dipakai di menu |
|---|---|---|
| Collection rate | Total pembayaran teralokasi ÷ total tagihan jatuh tempo pada periode yang sama | 17, 52, 53 |
| Outstanding | Total nilai tagihan jatuh tempo − total terbayar | 17, 22, 44, 49 |
| Konversi PPDB | Pendaftar selesai daftar ulang ÷ total pendaftar | 08 |
| Persentase kehadiran | (Hadir + terlambat) ÷ hari efektif [definisi terlambat sebagai hadir perlu dikonfirmasi, per klien] | 43, 48, 61 |
| Aging | Umur tagihan belum lunas sejak tanggal jatuh tempo, dikelompokkan sesuai menu 23 | 17, 23 |

**Governance metrik**

Setiap dashboard mendefinisikan rumus metrik, pemilik data, sumber otoritatif, frekuensi refresh, filter, cakupan unit, drilldown, freshness timestamp (CC-10), dan keterbatasan data. Angka keuangan harus dapat direkonsiliasi ke data keuangan dan akuntansi otoritatif.

## 18. Roadmap Phase & Release Gate

### 18.1 Roadmap phase

| Phase | Cakupan | Scope |
|---|---|---|
| Phase 1 | Grup 1 Platform Foundation, Grup 2 PPDB, pembayaran minimal tagihan PPDB, menu 79–80 | Core |
| Phase 2 | Grup 3 Finance, Grup 4 Payment penuh, menu 78 | Core |
| Phase 3 | Grup 5 Accounting, migrasi akuntansi dan parallel run | Core |
| Phase 4 | Grup 6 Operational | Core + Add-on |
| Phase 5 | Grup 7 Parent Portal, menu 81 | Core + Add-on |
| Phase 6 | Grup 8 Executive Dashboard | Core + Add-on |
| Phase 7 | Grup 9 Student Portal, menu 82 | Add-on |
| Phase 8 | Grup 10 Teacher Portal | Core + Add-on |
| Phase 9 | Grup 12 HRIS, Grup 14 Payroll | Add-on |
| Phase 10 | Grup 21 Kantin & Dompet Siswa, Grup 24 Event & Ekstrakurikuler, Grup 25 Service Desk | Add-on |
| Phase 11 | Grup 16 Inventory, Pengadaan & Aset, Grup 17 Ekstensi Akuntansi | Add-on |
| Phase 12 | Grup 18 Perpustakaan, Grup 19 Fasilitas, Grup 20 Transportasi | Add-on |
| Phase 13 | Grup 22 Kesehatan, Grup 23 Konseling & Kesiswaan, Grup 26 PKL, Grup 27 QA & Akreditasi, Grup 28 Pelaporan Pemerintah | Add-on |
| Phase 14 | Grup 13 Rekrutmen (ATS), Grup 15 Pelatihan Pegawai | Add-on |
| Phase 15 | Grup 29 Komunikasi Lanjutan, Grup 30 Ekstensi Pembayaran, Grup 31 Analitik & AI, Grup 33 Aplikasi White-label | Add-on |
| Phase 16 | Grup 32 3D Campus | Add-on |

Urutan Phase 9–16 adalah usulan awal dan dapat diubah Product Owner berdasarkan permintaan klien.

### 18.2 Release gate (berlaku untuk setiap phase)

| Gate | Bukti minimum |
|---|---|
| G0 — Discovery exit | Sign-off masalah, prioritas, inventaris sistem klien, fit-gap, pemilik data, risiko, dan scope phase |
| G1 — Design exit | Arsitektur, data model, kontrak integrasi, role matrix, UX flow, target NFR, dan pendekatan pengujian disetujui |
| G2 — Sprint acceptance | Kriteria penerimaan lulus, bukti QA tersedia, tidak ada blocker, dokumentasi dan telemetry diperbarui |
| G3 — Pilot readiness | Migrasi percobaan, control totals, training, support/runbook, rollback plan, dan UAT |
| G4 — Go-live approval | Sign-off bisnis, security review, rekonsiliasi selesai, tidak ada cacat kritis, monitoring dan recovery tervalidasi |
| G5 — Scale decision | Metrik pilot, insiden, adopsi, business case, dan kapasitas untuk phase berikutnya ditinjau |

Semua item backlog ditautkan ke tujuan (G1–G8) dan FR-ID. Perubahan setelah baseline scope disetujui harus mencantumkan nilai bisnis, risiko, dependensi, dampak phase, estimasi, dan approval.

## 19. Verifikasi, Traceability & Definition of Ready/Done

### 18.1 Metode verifikasi

| Metode | Kapan dipakai | Bukti |
|---|---|---|
| T — Test | Perilaku fungsional dan kinerja | Hasil test case, laporan uji beban |
| D — Demonstration | Fitur yang dinilai dengan diperagakan (dashboard, konfigurasi) | Berita acara demo |
| I — Inspection | Kepatuhan yang diperiksa dari konfigurasi, kode, atau dokumen | Checklist inspeksi |
| A — Analysis | Atribut yang diukur dalam periode waktu (ketersediaan) | Laporan monitoring bulanan |

### 18.2 Klasifikasi cacat

| Tingkat | Definisi | Syarat phase |
|---|---|---|
| Kritis | Data keuangan salah, pembayaran tidak tercatat, atau alur utama berhenti | Tidak boleh ada |
| Tinggi | Fungsi Must tidak berjalan tetapi ada cara kerja sementara | Disepakati per kasus |
| Sedang | Fungsi berjalan dengan gangguan yang tidak mempengaruhi data | Boleh, dengan rencana perbaikan |
| Rendah | Tampilan atau teks | Boleh |

### 18.3 Rantai traceability

Tujuan bisnis → Modul/menu → Fitur → FR-ID → User story → Kriteria penerimaan (AC) → Test case → UAT → Phase

| Artefak | Contoh | Tujuan |
|---|---|---|
| Aturan bisnis | BR-FIN-03 | Kendala bisnis |
| Kebutuhan fungsional | FR-21-001 | Perilaku sistem |
| User story | US-FIN-ALLOC-001 | Nilai bagi aktor |
| Kriteria penerimaan | AC-21-01 | Perilaku yang dapat diverifikasi |
| Test case | TC-FIN-ALLOC-001 | Bukti eksekusi |
| UAT | UAT-FIN-ALLOC-001 | Penerimaan bisnis |
| Cacat | BUG-FIN-ALLOC-001 | Pelacakan penyimpangan |

**Tujuan ke menu**

| Tujuan | Menu yang mewujudkan |
|---|---|
| G1 | 04 (migrasi), Grup Menu 5 (pengganti software akuntansi sebelumnya), 27–31 (pengganti payment aggregator sebelumnya), Grup Menu 1–2 dan 6 (pengganti sistem sekolah sebelumnya) |
| G2 | 08–16 |
| G3 | 17–31, 32–38 |
| G4 | 16, 44–50, 54–64 (add-on), 65–77 |
| G5 | 08, 17, 26, 32, 51–53 |
| G6 | 06, 34–36, CC-03, IF-ACC-01, IF-ACC-02, NFR-AUD-01 |
| G7 | 01, 05, 33, 38, 53 |

### 18.4 Definition of Ready

- Aktor, tujuan bisnis, dan scope (Core / Add-on) teridentifikasi.
- Aturan bisnis dan dependensi terdokumentasi.
- Input dan output data dipahami.
- Hak akses ditentukan.
- Kriteria penerimaan mencakup jalur positif dan negatif.
- Asumsi integrasi terdokumentasi.
- Alur UX tersedia untuk interaksi yang dibutuhkan.
- OQ yang mempengaruhi kebutuhan sudah dijawab.
### 18.5 Definition of Done

- Kebutuhan diimplementasikan dan direview.
- Kriteria penerimaan lulus.
- Unit, integrasi, dan API test selesai sesuai kebutuhan.
- Uji otorisasi dan lingkup unit lulus.
- Audit logging untuk aksi yang dapat diaudit terverifikasi.
- Status galat, memuat, dan kosong ditangani.
- Bukti UAT tercatat untuk alur kritis bisnis.
- Dokumentasi deployment dan operasional diperbarui.

## 20. Asumsi, Dependensi & Risiko

### 19.1 Asumsi

- Sekitar 1.800 siswa aktif di beberapa unit (SD, SMP, SMK); jumlah unit dikonfirmasi saat kick-off [per klien].
- Modul PPDB dibangun baru; akademik dasar, absensi, notifikasi, dan dashboard yayasan sudah tersedia di SEKOCI.
- Accounting Engine mendukung multi-entity dan menerima posting via API.
- Pembayaran diproses oleh satu payment gateway berlisensi.
### 19.2 Dependensi

- Pemilihan dan kontrak payment gateway, termasuk dukungan sub-akun per unit [OQ-04].
- Akses ekspor data sistem sekolah sebelumnya, software akuntansi sebelumnya, dan payment aggregator sebelumnya [OQ-09].
- Ketersediaan PIC sekolah untuk konfigurasi, UAT, dan training.
- Kebijakan akuntansi dari akuntan yayasan [per klien].
### 19.3 Risiko

| Risiko | Dampak | Mitigasi |
|---|---|---|
| PPDB dibuka sebelum Phase 1 siap | Tinggi | Tetapkan tanggal go-live Phase 1 di kick-off [per klien]; batasi Phase 1 ke kebutuhan M |
| Rentang phase membuat sistem lama klien dipakai lebih lama dan transaksi Phase 1–2 belum terjurnal otomatis | Tinggi | Putuskan penanganan transisi [OQ-16]; jadwalkan Phase 3 sedekat mungkin dengan Phase 2 |
| Data lama sulit diekspor atau tidak bersih | Sedang | Cek format ekspor di minggu pertama; template import dan validasi (menu 04) |
| Aturan promo dan alokasi lebih kompleks dari asumsi | Sedang | Kunci aturan di dokumen konfigurasi sebelum pengembangan [per klien] |
| Format laporan yayasan belum didukung penuh | Sedang | Validasi dengan akuntan yayasan [per klien] |
| Eliminasi antar-unit belum tersedia di Accounting Engine | Sedang | Konfirmasi dini [OQ-02]; jika belum ada, jadikan enhancement Phase 3 |
| Adopsi aplikasi orang tua rendah | Sedang | Sosialisasi per cabang, panduan in-app, pembayaran kasir tetap tersedia |
| Selisih saat parallel run dengan software akuntansi sebelumnya | Sedang | Rekonsiliasi berkala sebelum software akuntansi sebelumnya dihentikan |
| Cakupan produk SaaS (159 menu, 33 grup) melebihi kapasitas tim | Tinggi | Roadmap phase 1–16 dengan gate G5 di setiap phase; prioritas berdasarkan klien aktif |
| Biaya dan beban operasional model single-tenant per klien | Tinggi | Provisioning otomatis (IaC), satu pipeline untuk semua klien, ukuran minimum per klien; evaluasi alternatif di SAD [OQ-17] |
| Fitur SEKOCI existing diasumsikan siap produksi padahal belum terverifikasi | Sedang | Capability verification dan fit-gap sebelum G1 [OQ-01] |

## 21. Pertanyaan Terbuka

Pertanyaan tingkat produk. Pertanyaan khusus klien dicatat di PRD implementasi klien masing-masing.

| ID | Pertanyaan | Terdampak | Prioritas |
|---|---|---|---|
| OQ-01 | Kapabilitas SEKOCI mana yang benar-benar siap produksi, memiliki pengguna aktif, dan memiliki bukti QA? | Fit-gap, estimasi | Tinggi |
| OQ-02 | Spesifikasi API Accounting Engine, dukungan multi-entity, idempotensi, dan eliminasi antar-unit | Grup 5, menu 38 | Tinggi |
| OQ-03 | Template default alur approval per jenis permintaan dan batas nominalnya | Menu 79 | Tinggi |
| OQ-04 | Payment provider default produk dan provider alternatif yang didukung | Grup 4, Grup 30 | Tinggi |
| OQ-05 | Kebijakan akses rapor terkait tunggakan, pengecualian, banding, dan override berotorisasi (add-on) | Grup 9–10 | Sedang |
| OQ-06 | Apakah pelaksanaan ujian online (CBT) termasuk produk atau hanya jadwal dan hasil | Menu 59, 72 | Sedang |
| OQ-07 | Kebijakan default withdrawal: perlakuan tagihan setelah tanggal efektif, masa tenggang akun orang tua, dan dokumen wajib | Menu 78 | Tinggi |
| OQ-08 | Verifikasi hubungan orang tua/wali dan prosedur perubahan relasi | Menu 03, 45 | Tinggi |
| OQ-09 | Format standar import dan ekspor data untuk migrasi lintas klien | Menu 04 | Tinggi |
| OQ-10 | Masa retensi default per kategori data | NFR, keamanan | Sedang |
| OQ-11 | Daftar browser dan perangkat yang didukung | NFR-COMP-01 | Sedang |
| OQ-12 | Target NFR resmi (availability, performa, RPO/RTO) untuk SLA produk | Bab 15 | Tinggi |
| OQ-13 | Pemilik bisnis dan penyetuju untuk setiap domain dan release gate | Bab 18 | Tinggi |
| OQ-14 | Kategori default permintaan layanan orang tua | Menu 81 | Sedang |
| OQ-15 | Default tampilan tagihan siswa (aktif atau nonaktif) | Menu 82 | Rendah |
| OQ-16 | Penanganan transaksi sebelum modul akuntansi aktif (backfill otomatis atau pencatatan manual) | Phase 1–3 | Tinggi |
| OQ-17 | Model operasional single-tenant: lingkungan penuh per klien atau database terpisah dengan service bersama | NFR-TEN-01, SAD | Tinggi |
| OQ-18 | Sumber data indikator dashboard yang belum memiliki modul (kepuasan orang tua, indikator strategis, agenda) | Menu 51–53 | Sedang |

## 22. Persetujuan Dokumen

| Peran | Nama | Tanggal | Tanda tangan |
|---|---|---|---|
| Product Owner | David Pambudi | | |
| Project Manager | David Pambudi | | |
| Tech Lead | [Nama] | | |
| QA Lead | [Nama] | | |

## Lampiran — Batas Tanggung Jawab Antar-Menu

Setiap kebutuhan hanya dimiliki satu menu atau satu aturan; menu lain merujuk, bukan menulis ulang.

| Topik | Pemilik kebutuhan | Menu/aturan lain hanya … |
|---|---|---|
| Akses & lingkup unit | CC-01, BR-GEN-01; pemeriksaan server di NFR-SEC-02 | menerapkan, tanpa menulis ulang |
| Audit trail | Menu 06 (FR-06-001, 002) | menghasilkan kejadian yang dicatat |
| Pengiriman notifikasi | Menu 07 (kanal, template, approval, preferensi) | menetapkan pemicu kejadian (contoh: FR-19-005 tagihan terbit, FR-22-002 jatuh tempo, FR-41-004 alpa) |
| Preferensi & akun pengguna | CC-05, CC-08 | menu 50, 64, 77 tidak memiliki kebutuhan sendiri untuk ini |
| Promo PPDB | Menu 09 (definisi dan penerapan, BR-PPDB-01–03) | menu 15 dan 16 menerbitkan tagihan; promo diterapkan oleh FR-09-008 |
| Dokumen persyaratan | Menu 09 (konfigurasi), 12 (verifikasi pendaftaran) | menu 15 hanya menampilkan status kelengkapan daftar ulang |
| Penugasan wali kelas | Menu 39 | menu 40 hanya penugasan guru mata pelajaran (add-on); menu 68 menampilkan kelas guru |
| Absensi | Menu 41 (status, koreksi admin, kunci, notifikasi alpa); menu 69 (input guru) | menu 43, 48, 61 menampilkan; menu 42 menyetujui izin, menu 48 mengajukan izin |
| Pembayaran tunai/transfer | Menu 29 (FR-29-004) | menu 28 hanya metode pembayaran online |
| Status transaksi gateway | Menu 29 (FR-29-002); verifikasi webhook IF-PG-02; idempotensi BR-PAY-01 | — |
| Alokasi vs rekonsiliasi | Menu 21 (pembayaran ↔ tagihan); menu 31 (transaksi ↔ settlement/bank) | — |
| Migrasi data | Menu 04 (sistem sekolah sebelumnya, software akuntansi sebelumnya, payment aggregator sebelumnya) | bab 13 tidak lagi memuat kebutuhan migrasi terpisah |
| Jurnal & keterlacakan | Menu 34 (pembentukan), IF-ACC-01/02 (referensi dan nomor jurnal), menu 35 (drilldown) | — |
| Dashboard eksekutif | Menu 51 (snapshot total yayasan), 52 (per unit), 53 (perbandingan & tren antar unit) | metrik yang sama memakai satu definisi di metric dictionary (bab 16) |
| Tampilan per portal | Portal admin, orang tua, siswa, guru masing-masing memiliki kebutuhan tampilan untuk aktornya | data bersumber dari satu menu pemilik |
| Approval | Menu 79 (alur, penyetuju, maker-checker, delegasi, eskalasi) | menentukan kapan approval dibutuhkan dan menampilkan riwayatnya |
| Kegagalan integrasi teknis | Menu 80 | menu 31 hanya selisih rekonsiliasi bisnis; menu 32 menampilkan jumlah posting gagal |
| Pindah / keluar siswa | Menu 78 | menu 03 menyimpan status; menu 25 memproses refund |
| Permintaan layanan orang tua | Menu 81 (Core); SLA & eskalasi lanjutan di menu 137–138 | — |
| Absensi & cuti pegawai | Menu 86 | menu 41–43 hanya untuk siswa |
| Master ruang | Menu 116 | menu 03 dan 39 merujuk ke master ruang |

