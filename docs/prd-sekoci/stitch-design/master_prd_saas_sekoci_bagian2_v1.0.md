# Master PRD — SEKOCI Integrated School Operating Platform (SaaS)

**Bagian 2 dari 4 · Domain SDM: HRIS, Rekrutmen, Payroll, Pelatihan Pegawai (menu 83–101)**

Kebutuhan fungsional, aturan bisnis, dan kriteria penerimaan untuk pengelolaan guru dan staf.

| Atribut | Nilai |
|---|---|
| Document ID | SEKOCI-MPRD-001 |
| Versi | 1.0 — Bagian 2 (draft) |
| Tanggal | 10/10/2026 |
| Status | Draft, menunggu review |
| Produk | SEKOCI Integrated School Operating Platform (SaaS) |
| Product Owner | David Pambudi |
| Project Manager | David Pambudi |
| Klasifikasi | Rahasia — internal tim |

## Daftar Isi

- [Kontrol Dokumen](#kontrol-dokumen)
  - [Riwayat versi](#riwayat-versi)
  - [Hubungan dengan Bagian 1](#hubungan-dengan-bagian-1)
- [1. Ringkasan Bagian 2](#1-ringkasan-bagian-2)
- [2. Peran Tambahan](#2-peran-tambahan)
- [3. Entitas Data & System of Record](#3-entitas-data--system-of-record)
- [4. Aturan Bisnis Domain SDM](#4-aturan-bisnis-domain-sdm)
- [5. Kebutuhan Fungsional per Grup Menu & Menu](#5-kebutuhan-fungsional-per-grup-menu--menu)
  - [Grup Menu 12 — HRIS](#grup-menu-12--hris)
  - [Grup Menu 13 — Rekrutmen (ATS)](#grup-menu-13--rekrutmen-ats)
  - [Grup Menu 14 — Payroll](#grup-menu-14--payroll)
  - [Grup Menu 15 — Pelatihan Pegawai](#grup-menu-15--pelatihan-pegawai)
- [6. Alur Bisnis End-to-End](#6-alur-bisnis-end-to-end)
- [7. Integrasi](#7-integrasi)
- [8. Kebutuhan Non-Fungsional & Keamanan Khusus SDM](#8-kebutuhan-non-fungsional--keamanan-khusus-sdm)
- [9. Pertanyaan Terbuka Bagian 2](#9-pertanyaan-terbuka-bagian-2)
- [10. Persetujuan Dokumen](#10-persetujuan-dokumen)
- [Lampiran — Batas Tanggung Jawab Antar-Menu (Domain SDM)](#lampiran--batas-tanggung-jawab-antar-menu-domain-sdm)

## Kontrol Dokumen

### Riwayat versi

| Versi | Tanggal | Pemilik | Perubahan |
|---|---|---|---|
| 1.0 — Bagian 2 | 10/10/2026 | PT FullCircle Solusi Indonesia | Kebutuhan grup menu 12–15 (menu 83–101) |

### Hubungan dengan Bagian 1

Kerangka umum di Bagian 1 berlaku penuh untuk bagian ini, termasuk prinsip produk, konvensi, kebutuhan lintas modul (CC-01 sampai CC-10), aturan bisnis umum (BR-GEN, BR-INT), Approval Center (menu 79), Integration Exception Queue (menu 80), kebutuhan non-fungsional, roadmap phase, dan release gate G0–G5. Bagian ini hanya menambahkan hal yang khusus untuk domain SDM.

## 1. Ringkasan Bagian 2

Domain SDM mencakup 19 menu dalam 4 grup:

| Grup | Menu | Phase | Scope |
|---|---|---|---|
| 12. HRIS | 83–87 | Phase 9 | Add-on |
| 13. Rekrutmen (ATS) | 88–92 | Phase 14 | Add-on |
| 14. Payroll | 93–98 | Phase 9 | Add-on |
| 15. Pelatihan Pegawai | 99–101 | Phase 14 | Add-on |

Alurnya membentuk satu rantai: kebutuhan posisi → lowongan → kandidat → offer → pegawai → onboarding dan pelatihan → kehadiran dan cuti → payroll → slip, pajak, BPJS → jurnal di Accounting Engine. Bagian ini memuat 93 kebutuhan fungsional dan 43 skenario Given/When/Then.

## 2. Peran Tambahan

| Peran | Tujuan | Lingkup data |
|---|---|---|
| HR Admin | Administrasi data pegawai, dokumen, kehadiran, cuti, onboarding | Unit yang ditugaskan |
| HR Manager | Pengawasan SDM dan approval | Unit atau yayasan |
| Payroll Officer | Input dan perhitungan payroll, slip, pelaporan pajak dan BPJS | Unit atau yayasan |
| Recruiter | Pengelolaan lowongan dan kandidat | Unit atau yayasan |
| Pewawancara | Penilaian kandidat yang ditugaskan | Kandidat yang ditugaskan |
| Training Admin | Katalog dan penugasan pelatihan | Unit atau yayasan |
| Pegawai | Layanan mandiri pegawai | Data sendiri |
| Kandidat / Pelamar | Melamar dan merespons offer | Data lamaran sendiri |

Peran dari Bagian 1 yang terlibat: Kepala Unit (hiring manager, atasan langsung, penyetuju), Finance Manager (approval payroll), dan Accounting (jurnal payroll). Hak akses rinci tercantum di Role & Permission Matrix.

## 3. Entitas Data & System of Record

| Entitas | Atribut / relasi utama | Sumber data utama |
|---|---|---|
| Employee | Identitas, NIK, NPWP, PTKP, rekening, unit, jabatan, status kepegawaian | SEKOCI HRIS |
| Employment History | Jabatan, unit, kontrak, gaji pokok, status, tanggal efektif | SEKOCI HRIS |
| Department / Position | Bagian per unit, jabatan, level, atasan langsung, formasi | SEKOCI HRIS |
| Employee Document | Jenis, versi, masa berlaku | SEKOCI HRIS (file di object storage) |
| Work Schedule / Employee Attendance | Jadwal, shift, kehadiran harian | SEKOCI HRIS |
| Leave Type / Balance / Request, Overtime Request | Kuota, saldo, pengajuan, approval | SEKOCI HRIS |
| Requisition / Job Posting | Kebutuhan posisi, lowongan | SEKOCI ATS |
| Candidate / Application / Interview / Scorecard / Offer | Kandidat, lamaran, tahap, penilaian, penawaran | SEKOCI ATS |
| Pay Component / Salary Structure / Statutory Parameter | Komponen, skala gaji, parameter BPJS dan PPh 21 dengan tanggal berlaku | SEKOCI Payroll |
| Payroll Period / Run / Line / Payslip | Periode, perhitungan, hasil per pegawai, slip | SEKOCI Payroll |
| Tax & BPJS Report | Rekap PPh 21, bukti potong, laporan iuran | SEKOCI Payroll |
| Payroll Journal | Jurnal beban, utang, pembayaran gaji | Dibentuk SEKOCI Payroll, dicatat Accounting Engine |
| Course / Enrollment / Training Assignment / Onboarding Task | Pelatihan, pendaftaran, penugasan, progres | SEKOCI Learning (pegawai) |

Bila modul HRIS aktif, data guru dan staf di menu 03 bersumber dari Master Pegawai (BR-HR-01).

## 4. Aturan Bisnis Domain SDM

| ID | Aturan |
|---|---|
| BR-HR-01 | Bila modul HRIS aktif, Master Pegawai (menu 83) menjadi sumber data guru dan staf; data di menu 03 disinkronkan dari menu 83 dan tidak dapat diubah langsung. |
| BR-HR-02 | Perubahan jabatan, unit, gaji, dan status kepegawaian berlaku per tanggal efektif; riwayat tidak dihapus. |
| BR-HR-03 | NIK, NPWP, rekening, gaji, slip, dan dokumen pribadi pegawai hanya dapat dilihat HR dan Payroll yang berwenang serta pegawai yang bersangkutan; peran lain melihat data tersamar. |
| BR-HR-04 | Cuti yang memiliki kuota hanya dapat disetujui bila saldo mencukupi; saldo berkurang saat cuti disetujui dan kembali bila cuti dibatalkan. |
| BR-HR-05 | Atasan langsung (menu 84) menjadi penyetuju default untuk cuti dan lembur, kecuali alur approval di menu 79 menentukan lain. |
| BR-REC-01 | Lowongan hanya dapat dibuat dari permintaan rekrutmen yang disetujui. |
| BR-REC-02 | Data kandidat diolah sesuai persetujuannya; data kandidat yang tidak diterima dan tidak menyetujui talent pool dihapus atau dianonimkan setelah [6] bulan [OQ-25]. |
| BR-REC-03 | Offer memerlukan approval; gaji pokok di luar rentang struktur gaji jabatan memerlukan tahap approval tambahan. |
| BR-PRL-01 | Payroll dihitung dengan komponen dan parameter yang berlaku pada periode tersebut, bukan parameter terbaru. |
| BR-PRL-02 | Payroll yang sudah final tidak dapat diubah; koreksi dilakukan melalui penyesuaian di periode berikutnya atau run susulan dengan approval. |
| BR-PRL-03 | Satu pegawai hanya memiliki satu payroll run reguler per periode. |
| BR-PRL-04 | Setiap komponen gaji wajib dipetakan ke akun unit sebelum dipakai dalam payroll run. |
| BR-TRN-01 | Pelatihan wajib suatu jabatan otomatis ditugaskan kepada pegawai baru dan pegawai yang pindah ke jabatan tersebut. |

## 5. Kebutuhan Fungsional per Grup Menu & Menu

Kebutuhan lintas modul (Bagian 1, bab 10) dan aturan bisnis (Bagian 1, bab 9 dan bab 4 di atas) berlaku tanpa ditulis ulang.

### Grup Menu 12 — HRIS

*Phase 9 · Add-on. Data induk dan operasional kepegawaian guru dan staf.*

#### 83. Master Pegawai

| Atribut | Nilai |
|---|---|
| **Portal** | Admin / HR |
| **Peran** | HR Admin; HR Manager |
| **Scope** | Add-on |
| **Phase** | Phase 9 |
| **Tujuan** | Data induk pegawai (guru dan staf) sebagai sumber tunggal ketika modul HRIS aktif. |
| **Fitur** | Employee master, Employment history, Offboarding |
| **Sub-fitur** | Identitas, NIK, NPWP, status PTKP, kontak, rekening bank, unit, jabatan, status kepegawaian, tanggal masuk/keluar, riwayat jabatan dan kontrak, deteksi duplikat |

| ID | Kebutuhan | Prio | Scope | Phase |
|---|---|---|---|---|
| FR-83-001 | Sistem harus menyimpan data pegawai: identitas (NIK, NPWP), status PTKP, kontak, rekening bank, unit, jabatan, status kepegawaian (tetap, kontrak, honorer, bukan pegawai), dan tanggal mulai kerja. | M | Add-on | Phase 9 |
| FR-83-002 | Sistem harus menyimpan riwayat penugasan, jabatan, kontrak, dan status kepegawaian dengan tanggal efektif (BR-HR-02). | M | Add-on | Phase 9 |
| FR-83-003 | Sistem harus menerapkan BR-HR-01 terhadap data guru dan staf di menu 03. | M | Add-on | Phase 9 |
| FR-83-004 | Sistem harus mendeteksi kemungkinan duplikat pegawai berdasarkan NIK dan email sebelum data disimpan. | S | Add-on | Phase 9 |
| FR-83-005 | Sistem harus mengingatkan HR [30] hari sebelum kontrak pegawai berakhir. | S | Add-on | Phase 9 |
| FR-83-006 | Sistem harus memproses pegawai keluar dengan tanggal efektif dan alasan, menonaktifkan akun penggunanya (menu 05) per tanggal tersebut, dan tetap menyimpan seluruh datanya. | M | Add-on | Phase 9 |
| FR-83-007 | Sistem harus menerapkan BR-HR-03 pada tampilan dan ekspor data pegawai. | M | Add-on | Phase 9 |

| ID | Skenario | Given / When / Then |
|---|---|---|
| AC-83-01 | Pengingat kontrak berakhir | **Given** kontrak pegawai "Sinta" berakhir 30 November<br>**When** tanggal menunjukkan 31 Oktober<br>**Then** HR menerima pengingat kontrak Sinta |
| AC-83-02 | Data guru tersinkron ke menu 03 | **Given** modul HRIS aktif dan HR mengubah unit tugas guru "Pak Arif" ke unit "SMP"<br>**When** admin akademik membuka data guru di menu 03<br>**Then** unit tugas Pak Arif tampil "SMP" dan tidak dapat diubah dari menu 03 |
| AC-83-03 | Pegawai keluar | **Given** Sinta keluar per 1 Desember<br>**When** HR menyimpan proses offboarding<br>**Then** akun Sinta nonaktif mulai 1 Desember<br>**And** riwayat kepegawaian Sinta tetap tersimpan |
| AC-83-04 | Data sensitif tersamar | **Given** Kepala Unit membuka data pegawai di unitnya<br>**When** halaman detail ditampilkan<br>**Then** NIK, NPWP, rekening, dan gaji ditampilkan tersamar |

#### 84. Organisasi & Jabatan

| Atribut | Nilai |
|---|---|
| **Portal** | Admin / HR |
| **Peran** | HR Admin; HR Manager |
| **Scope** | Add-on |
| **Phase** | Phase 9 |
| **Tujuan** | Struktur bagian, jabatan, atasan langsung, dan formasi per unit. Struktur yayasan dan unit tetap dikelola di menu 01. |
| **Fitur** | Department, Position, Reporting line, Headcount plan |
| **Sub-fitur** | Bagian per unit, kode dan level jabatan, deskripsi, atasan langsung, formasi, bagan organisasi, riwayat perubahan |

| ID | Kebutuhan | Prio | Scope | Phase |
|---|---|---|---|---|
| FR-84-001 | Sistem harus mengelola bagian (departemen) per unit. | M | Add-on | Phase 9 |
| FR-84-002 | Sistem harus mengelola jabatan dengan kode, nama, level, dan deskripsi tugas. | M | Add-on | Phase 9 |
| FR-84-003 | Sistem harus menyimpan atasan langsung setiap pegawai dan menyediakannya sebagai penyetuju default (BR-HR-05). | M | Add-on | Phase 9 |
| FR-84-004 | Sistem harus mengelola formasi (jumlah posisi yang direncanakan) per jabatan dan unit, beserta jumlah posisi terisi dan kosong. | S | Add-on | Phase 9 |
| FR-84-005 | Sistem harus menampilkan bagan organisasi per unit berdasarkan atasan langsung. | S | Add-on | Phase 9 |
| FR-84-006 | Sistem harus menyimpan riwayat perubahan struktur dengan tanggal efektif. | S | Add-on | Phase 9 |

| ID | Skenario | Given / When / Then |
|---|---|---|
| AC-84-01 | Atasan langsung sebagai penyetuju | **Given** atasan langsung "Bu Rina" adalah Kepala SD<br>**When** Bu Rina mengajukan cuti<br>**Then** pengajuan diarahkan ke Kepala SD sebagai penyetuju default |
| AC-84-02 | Formasi terisi penuh | **Given** formasi guru Matematika SMP adalah 3 dan terisi 3<br>**When** bagan formasi ditampilkan<br>**Then** posisi tersebut ditandai penuh dengan 0 posisi kosong |

#### 85. Dokumen Pegawai

| Atribut | Nilai |
|---|---|
| **Portal** | Admin / HR / ESS |
| **Peran** | HR Admin; Pegawai |
| **Scope** | Add-on |
| **Phase** | Phase 9 |
| **Tujuan** | Penyimpanan dokumen kepegawaian beserta masa berlaku dan kelengkapannya. |
| **Fitur** | Employee documents, Expiry tracking, Completeness |
| **Sub-fitur** | Jenis dokumen, unggah, versi, masa berlaku, pengingat, dokumen wajib, kontrol akses |

| ID | Kebutuhan | Prio | Scope | Phase |
|---|---|---|---|---|
| FR-85-001 | Sistem harus menyimpan dokumen pegawai per jenis (antara lain KTP, ijazah, sertifikat pendidik, SK, kontrak) dengan versi. | M | Add-on | Phase 9 |
| FR-85-002 | Sistem harus menyimpan masa berlaku dokumen dan mengingatkan HR serta pegawai [30] hari sebelum kedaluwarsa. | S | Add-on | Phase 9 |
| FR-85-003 | Sistem harus mengelola daftar dokumen wajib per status kepegawaian dan menandai kelengkapan dokumen setiap pegawai. | M | Add-on | Phase 9 |
| FR-85-004 | Sistem harus membatasi akses dokumen kepada HR yang berwenang dan pegawai yang bersangkutan (BR-HR-03). | M | Add-on | Phase 9 |

| ID | Skenario | Given / When / Then |
|---|---|---|
| AC-85-01 | Sertifikat akan kedaluwarsa | **Given** sertifikat pegawai berlaku sampai 15 Desember<br>**When** tanggal menunjukkan 15 November<br>**Then** HR dan pegawai menerima pengingat |
| AC-85-02 | Dokumen wajib belum lengkap | **Given** pegawai kontrak wajib memiliki KTP, ijazah, dan kontrak, tetapi kontrak belum diunggah<br>**When** HR membuka data pegawai tersebut<br>**Then** status kelengkapan menunjukkan "belum lengkap: kontrak" |

#### 86. Absensi & Cuti Pegawai

| Atribut | Nilai |
|---|---|
| **Portal** | Admin / HR |
| **Peran** | HR Admin; HR Manager; atasan langsung |
| **Scope** | Add-on |
| **Phase** | Phase 9 |
| **Tujuan** | Pengelolaan kehadiran, cuti, dan lembur pegawai di back office. Pengajuan oleh pegawai ada di menu 87. |
| **Fitur** | Work schedule, Employee attendance, Leave management, Overtime |
| **Sub-fitur** | Jadwal dan shift, status kehadiran, input manual, import file mesin absensi, jenis cuti, kuota, akrual, carry-over, persetujuan, rekap, kirim ke payroll, kunci periode |

| ID | Kebutuhan | Prio | Scope | Phase |
|---|---|---|---|---|
| FR-86-001 | Sistem harus mengelola jadwal kerja dan shift per unit dan jabatan. | M | Add-on | Phase 9 |
| FR-86-002 | Sistem harus mencatat kehadiran pegawai dari check-in di menu 87, input manual HR, dan import file ekspor mesin absensi, dengan status hadir, terlambat, izin, sakit, cuti, dinas luar, atau alpa. | M | Add-on | Phase 9 |
| FR-86-003 | Sistem harus mengelola jenis cuti beserta kuota tahunan, aturan akrual, dan carry-over yang dikonfigurasi. | M | Add-on | Phase 9 |
| FR-86-004 | Sistem harus memproses pengajuan cuti dan lembur melalui workflow approval (menu 79) dengan atasan langsung sebagai penyetuju default, dan menerapkan BR-HR-04. | M | Add-on | Phase 9 |
| FR-86-005 | Sistem harus menghasilkan rekap kehadiran, cuti, dan lembur pegawai per unit dan periode, serta mengekspornya. | M | Add-on | Phase 9 |
| FR-86-006 | Sistem harus mengirim rekap periode ke Input Payroll (menu 94) dan mengunci data periode tersebut; koreksi setelahnya memerlukan approval. | M | Add-on | Phase 9 |

| ID | Skenario | Given / When / Then |
|---|---|---|
| AC-86-01 | Cuti melebihi saldo ditolak | **Given** saldo cuti tahunan pegawai 2 hari<br>**When** pegawai mengajukan cuti tahunan 3 hari<br>**Then** pengajuan ditolak dengan pesan saldo tidak mencukupi |
| AC-86-02 | Saldo berkurang setelah disetujui | **Given** saldo cuti tahunan 12 hari dan pengajuan 2 hari disetujui atasan<br>**When** approval selesai<br>**Then** saldo cuti menjadi 10 hari |
| AC-86-03 | Import file mesin absensi | **Given** file ekspor mesin absensi berisi 300 baris untuk 1 Oktober<br>**When** HR mengimport file<br>**Then** kehadiran 1 Oktober tercatat dan baris yang tidak cocok dengan pegawai aktif ditampilkan sebagai galat |
| AC-86-04 | Periode terkunci setelah dikirim ke payroll | **Given** rekap Oktober sudah dikirim ke menu 94<br>**When** HR mengubah status kehadiran 5 Oktober<br>**Then** perubahan menunggu approval dan tercatat di audit trail |

#### 87. Employee Self-Service

| Atribut | Nilai |
|---|---|
| **Portal** | Portal Pegawai (web-portal) |
| **Peran** | Pegawai |
| **Scope** | Add-on |
| **Phase** | Phase 9 |
| **Tujuan** | Portal mandiri pegawai: kehadiran, pengajuan cuti dan lembur, profil, dokumen, dan slip gaji. |
| **Fitur** | Employee dashboard, Self check-in, Leave & overtime request, Profile change request, My documents, My payslips |
| **Sub-fitur** | Ringkasan sisa cuti, slip terbaru, pelatihan wajib, check-in/out, pengajuan, status, permintaan perubahan data, unggah dokumen yang diminta, slip gaji |

| ID | Kebutuhan | Prio | Scope | Phase |
|---|---|---|---|---|
| FR-87-001 | Sistem harus menampilkan dashboard pegawai: sisa cuti, status pengajuan, slip gaji terbaru, dan pelatihan wajib yang belum selesai. | M | Add-on | Phase 9 |
| FR-87-002 | Sistem harus memungkinkan pegawai melakukan check-in dan check-out beserta waktunya; pencatatan lokasi mengikuti kebijakan klien [OQ-23]. | M | Add-on | Phase 9 |
| FR-87-003 | Sistem harus memungkinkan pegawai mengajukan cuti dan lembur serta memantau status persetujuannya. | M | Add-on | Phase 9 |
| FR-87-004 | Sistem harus memungkinkan pegawai mengajukan perubahan data pribadi (alamat, kontak, rekening, status PTKP) yang berlaku setelah disetujui HR. | M | Add-on | Phase 9 |
| FR-87-005 | Sistem harus memungkinkan pegawai melihat dokumennya dan mengunggah dokumen yang diminta HR (disimpan di menu 85). | S | Add-on | Phase 9 |
| FR-87-006 | Sistem harus memungkinkan pegawai melihat dan mengunduh slip gajinya sendiri yang sudah dipublikasikan (menu 96). | M | Add-on | Phase 9 |

| ID | Skenario | Given / When / Then |
|---|---|---|
| AC-87-01 | Perubahan rekening menunggu persetujuan | **Given** pegawai mengajukan perubahan nomor rekening<br>**When** HR belum menyetujui<br>**Then** payroll berikutnya tetap memakai rekening lama |
| AC-87-02 | Slip hanya milik sendiri | **Given** slip Oktober sudah dipublikasikan<br>**When** pegawai membuka menu slip gaji<br>**Then** hanya slip miliknya yang tampil dan dapat diunduh |

### Grup Menu 13 — Rekrutmen (ATS)

*Phase 14 · Add-on. Dari permintaan posisi sampai kandidat diterima.*

#### 88. Permintaan Rekrutmen

| Atribut | Nilai |
|---|---|
| **Portal** | Admin / HR |
| **Peran** | Kepala Unit (hiring manager); HR Manager |
| **Scope** | Add-on |
| **Phase** | Phase 14 |
| **Tujuan** | Pengajuan kebutuhan posisi oleh unit sebelum rekrutmen dimulai. |
| **Fitur** | Job requisition |
| **Sub-fitur** | Jabatan, unit, jumlah, alasan (pengganti/penambahan), kualifikasi, perkiraan anggaran, validasi formasi, approval |

| ID | Kebutuhan | Prio | Scope | Phase |
|---|---|---|---|---|
| FR-88-001 | Sistem harus memungkinkan Kepala Unit mengajukan kebutuhan posisi dengan jabatan (menu 84), unit, jumlah, alasan, kualifikasi, dan perkiraan anggaran. | M | Add-on | Phase 14 |
| FR-88-002 | Sistem harus membandingkan pengajuan dengan formasi kosong (menu 84) dan menandai pengajuan yang melebihi formasi. | M | Add-on | Phase 14 |
| FR-88-003 | Sistem harus memproses pengajuan melalui workflow approval (menu 79). | M | Add-on | Phase 14 |
| FR-88-004 | Sistem harus menampilkan status setiap permintaan sampai posisi terisi atau dibatalkan. | S | Add-on | Phase 14 |

| ID | Skenario | Given / When / Then |
|---|---|---|
| AC-88-01 | Pengajuan melebihi formasi | **Given** formasi guru Fisika SMK kosong 1<br>**When** Kepala Unit mengajukan 2 posisi<br>**Then** pengajuan ditandai melebihi formasi dan memerlukan tahap approval tambahan |

#### 89. Lowongan & Publikasi

| Atribut | Nilai |
|---|---|
| **Portal** | Admin / HR / Halaman karier publik |
| **Peran** | HR Admin; Pelamar |
| **Scope** | Add-on |
| **Phase** | Phase 14 |
| **Tujuan** | Publikasi lowongan dan penerimaan lamaran. |
| **Fitur** | Job posting, Career page, Application form |
| **Sub-fitur** | Lowongan dari requisition, deskripsi, kualifikasi, unit, tenggat, halaman karier, formulir lamaran, unggah CV, tautan berbagi, penutupan otomatis |

| ID | Kebutuhan | Prio | Scope | Phase |
|---|---|---|---|---|
| FR-89-001 | Sistem harus membuat lowongan hanya dari permintaan rekrutmen yang disetujui (BR-REC-01). | M | Add-on | Phase 14 |
| FR-89-002 | Sistem harus menyediakan halaman karier publik per yayasan yang menampilkan lowongan aktif. | M | Add-on | Phase 14 |
| FR-89-003 | Sistem harus menyediakan formulir lamaran dengan unggah CV dan dokumen, serta persetujuan kandidat atas pengolahan datanya (BR-REC-02). | M | Add-on | Phase 14 |
| FR-89-004 | Sistem harus menyediakan tautan lowongan yang dapat dibagikan ke kanal eksternal. | S | Add-on | Phase 14 |
| FR-89-005 | Sistem harus menutup lowongan otomatis setelah tenggat atau ketika posisi sudah terisi. | M | Add-on | Phase 14 |

| ID | Skenario | Given / When / Then |
|---|---|---|
| AC-89-01 | Lowongan tanpa requisition | **Given** belum ada permintaan rekrutmen yang disetujui untuk jabatan Staf TU<br>**When** HR membuat lowongan Staf TU<br>**Then** lowongan tidak dapat dibuat |
| AC-89-02 | Lowongan tutup otomatis | **Given** tenggat lowongan 31 Oktober<br>**When** tanggal menunjukkan 1 November<br>**Then** lowongan tidak lagi tampil di halaman karier dan formulir lamaran ditutup |

#### 90. Pipeline Kandidat

| Atribut | Nilai |
|---|---|
| **Portal** | Admin / HR |
| **Peran** | HR Admin; Recruiter; Kepala Unit |
| **Scope** | Add-on |
| **Phase** | Phase 14 |
| **Tujuan** | Pengelolaan pelamar dari seleksi administrasi sampai keputusan. |
| **Fitur** | Candidate pipeline, Communication, Talent pool |
| **Sub-fitur** | Daftar pelamar, tahap yang dikonfigurasi, perpindahan tahap, catatan, lamaran ganda, template email, penolakan, talent pool |

| ID | Kebutuhan | Prio | Scope | Phase |
|---|---|---|---|---|
| FR-90-001 | Sistem harus menampilkan pelamar per lowongan dengan tahap yang dikonfigurasi (misalnya seleksi administrasi, tes, wawancara, offer, diterima, ditolak). | M | Add-on | Phase 14 |
| FR-90-002 | Sistem harus memindahkan pelamar antar tahap dengan catatan dan mencatat riwayatnya. | M | Add-on | Phase 14 |
| FR-90-003 | Sistem harus mendeteksi lamaran ganda dari kandidat yang sama pada lowongan yang sama. | S | Add-on | Phase 14 |
| FR-90-004 | Sistem harus mengirim pemberitahuan kepada kandidat pada setiap perubahan tahap memakai template yang dapat diubah. | M | Add-on | Phase 14 |
| FR-90-005 | Sistem harus menyimpan kandidat yang menyetujui ke talent pool untuk lowongan lain, dan menerapkan BR-REC-02 untuk kandidat lainnya. | S | Add-on | Phase 14 |

| ID | Skenario | Given / When / Then |
|---|---|---|
| AC-90-01 | Pemberitahuan kandidat | **Given** kandidat "Dimas" lolos seleksi administrasi<br>**When** recruiter memindahkan Dimas ke tahap tes<br>**Then** Dimas menerima pemberitahuan berisi jadwal dan instruksi tes |
| AC-90-02 | Data kandidat tanpa persetujuan talent pool | **Given** kandidat ditolak dan tidak menyetujui talent pool<br>**When** masa simpan [6] bulan terlewati<br>**Then** data pribadi kandidat dihapus atau dianonimkan |

#### 91. Wawancara & Asesmen

| Atribut | Nilai |
|---|---|
| **Portal** | Admin / HR |
| **Peran** | Recruiter; Pewawancara; Kepala Unit |
| **Scope** | Add-on |
| **Phase** | Phase 14 |
| **Tujuan** | Penjadwalan dan penilaian wawancara atau microteaching. |
| **Fitur** | Interview scheduling, Scorecard |
| **Sub-fitur** | Jadwal, panel, undangan, kriteria dan bobot, penilaian per pewawancara, rekap skor, rekomendasi |

| ID | Kebutuhan | Prio | Scope | Phase |
|---|---|---|---|---|
| FR-91-001 | Sistem harus menjadwalkan wawancara atau microteaching dengan panel pewawancara dan mengirim undangan kepada kandidat dan panel. | M | Add-on | Phase 14 |
| FR-91-002 | Sistem harus menyediakan scorecard dengan kriteria dan bobot yang dikonfigurasi per jabatan. | M | Add-on | Phase 14 |
| FR-91-003 | Sistem harus memungkinkan setiap pewawancara mengisi penilaian sendiri; nilai pewawancara lain baru terlihat setelah ia mengirim penilaiannya. | S | Add-on | Phase 14 |
| FR-91-004 | Sistem harus merekap skor akhir dan rekomendasi panel per kandidat. | M | Add-on | Phase 14 |

| ID | Skenario | Given / When / Then |
|---|---|---|
| AC-91-01 | Skor akhir tertimbang | **Given** bobot kompetensi 60% dan sikap 40%, dan rata-rata panel 80 dan 90<br>**When** rekap dihitung<br>**Then** skor akhir kandidat 84 |
| AC-91-02 | Nilai panel tersembunyi | **Given** pewawancara A belum mengirim penilaian<br>**When** pewawancara A membuka scorecard kandidat<br>**Then** nilai pewawancara lain belum ditampilkan |

#### 92. Offer & Hiring

| Atribut | Nilai |
|---|---|
| **Portal** | Admin / HR |
| **Peran** | HR Admin; HR Manager; Kandidat |
| **Scope** | Add-on |
| **Phase** | Phase 14 |
| **Tujuan** | Penawaran kerja sampai kandidat menjadi calon pegawai. |
| **Fitur** | Offer, Acceptance, Hire |
| **Sub-fitur** | Jabatan, unit, gaji pokok, tanggal mulai, status kepegawaian, approval, dokumen offer, respons kandidat, pembuatan data pegawai, pemicu onboarding |

| ID | Kebutuhan | Prio | Scope | Phase |
|---|---|---|---|---|
| FR-92-001 | Sistem harus membuat offer berisi jabatan, unit, gaji pokok, tanggal mulai, dan status kepegawaian untuk kandidat terpilih. | M | Add-on | Phase 14 |
| FR-92-002 | Sistem harus memproses offer melalui workflow approval (menu 79) dan menerapkan BR-REC-03. | M | Add-on | Phase 14 |
| FR-92-003 | Sistem harus mengirim dokumen offer kepada kandidat dan menerima respons terima atau tolak secara online. | M | Add-on | Phase 14 |
| FR-92-004 | Setelah offer diterima, sistem harus membuat data calon pegawai di menu 83, memicu onboarding (menu 100), dan menutup permintaan rekrutmen bila kuotanya terpenuhi. | M | Add-on | Phase 14 |

| ID | Skenario | Given / When / Then |
|---|---|---|
| AC-92-01 | Gaji di luar rentang | **Given** rentang gaji pokok jabatan Guru SMP Rp4.000.000–Rp5.000.000<br>**When** HR membuat offer Rp5.500.000<br>**Then** offer memerlukan tahap approval tambahan sebelum dikirim |
| AC-92-02 | Offer diterima | **Given** kandidat Dimas menerima offer dengan tanggal mulai 1 Januari<br>**When** respons disimpan<br>**Then** data calon pegawai Dimas terbentuk di menu 83 dan checklist onboarding dibuat |

### Grup Menu 14 — Payroll

*Phase 9 · Add-on. Penggajian guru dan staf, termasuk PPh 21, BPJS, dan jurnal.*

#### 93. Komponen & Pengaturan Gaji

| Atribut | Nilai |
|---|---|
| **Portal** | Admin / Payroll |
| **Peran** | Payroll Officer; HR Manager; Finance Manager |
| **Scope** | Add-on |
| **Phase** | Phase 9 |
| **Tujuan** | Komponen gaji, struktur gaji, dan parameter perhitungan. |
| **Fitur** | Pay components, Salary structure, Statutory parameters, Payroll calendar |
| **Sub-fitur** | Pendapatan dan potongan, sifat komponen, struktur/skala gaji, honor per jam, parameter BPJS, tabel PPh 21, tanggal berlaku, mapping akun, periode dan cut-off |

| ID | Kebutuhan | Prio | Scope | Phase |
|---|---|---|---|---|
| FR-93-001 | Sistem harus mengelola komponen pendapatan dan potongan (antara lain gaji pokok, tunjangan tetap dan tidak tetap, honor mengajar per jam, lembur, potongan koperasi, pinjaman) beserta sifatnya: tetap atau variabel, objek pajak atau bukan, dan termasuk dasar iuran BPJS atau bukan. | M | Add-on | Phase 9 |
| FR-93-002 | Sistem harus mengelola struktur atau skala gaji per jabatan, golongan, atau status kepegawaian. | M | Add-on | Phase 9 |
| FR-93-003 | Sistem harus menyimpan parameter BPJS (persentase pemberi kerja dan pegawai, batas upah) dan tabel PPh 21 (tarif efektif bulanan dan tarif tahunan) dengan tanggal berlaku, sehingga perubahan regulasi cukup diatur tanpa perubahan kode [OQ-19, OQ-20]. | M | Add-on | Phase 9 |
| FR-93-004 | Sistem harus mewajibkan setiap komponen dipetakan ke akun COA per unit sebelum dipakai dalam payroll run (BR-PRL-04). | M | Add-on | Phase 9 |
| FR-93-005 | Sistem harus mengelola periode payroll dan tanggal cut-off per unit. | M | Add-on | Phase 9 |
| FR-93-006 | Sistem harus memproses perubahan komponen, struktur gaji, dan parameter melalui workflow approval (menu 79). | M | Add-on | Phase 9 |

| ID | Skenario | Given / When / Then |
|---|---|---|
| AC-93-01 | Parameter berlaku per tanggal | **Given** batas upah BPJS diubah dengan tanggal berlaku 1 Januari<br>**When** payroll Desember dihitung<br>**Then** payroll Desember memakai batas upah lama (BR-PRL-01) |
| AC-93-02 | Komponen tanpa akun | **Given** komponen "Tunjangan Wali Kelas" belum dipetakan ke akun unit "SD"<br>**When** payroll unit SD dijalankan<br>**Then** payroll run ditolak dengan pesan mapping akun belum lengkap |

#### 94. Input Payroll

| Atribut | Nilai |
|---|---|
| **Portal** | Admin / Payroll |
| **Peran** | Payroll Officer; HR Admin |
| **Scope** | Add-on |
| **Phase** | Phase 9 |
| **Tujuan** | Pengumpulan data variabel per periode sebelum perhitungan gaji. |
| **Fitur** | Attendance import, Variable input, Validation |
| **Sub-fitur** | Tarik rekap dari menu 86, jam mengajar tambahan, insentif, potongan, import, validasi, kunci setelah cut-off |

| ID | Kebutuhan | Prio | Scope | Phase |
|---|---|---|---|---|
| FR-94-001 | Sistem harus menarik rekap kehadiran, cuti, dan lembur periode berjalan dari menu 86. | M | Add-on | Phase 9 |
| FR-94-002 | Sistem harus menerima input variabel (jam mengajar tambahan, insentif, potongan) secara manual atau melalui import. | M | Add-on | Phase 9 |
| FR-94-003 | Sistem harus memvalidasi input: pegawai aktif pada periode, komponen valid, dan nilai dalam batas yang dikonfigurasi. | M | Add-on | Phase 9 |
| FR-94-004 | Sistem harus mengunci input setelah cut-off; perubahan setelahnya memerlukan approval. | M | Add-on | Phase 9 |

| ID | Skenario | Given / When / Then |
|---|---|---|
| AC-94-01 | Honor mengajar per jam | **Given** tarif honor mengajar Rp50.000 per jam dan guru honorer mengajar 40 jam pada Oktober<br>**When** input periode Oktober disimpan<br>**Then** honor mengajar Oktober guru tersebut Rp2.000.000 |
| AC-94-02 | Pegawai tidak aktif | **Given** pegawai keluar per 1 Oktober<br>**When** Payroll Officer menginput insentif Oktober untuknya<br>**Then** input ditolak dengan pesan pegawai tidak aktif pada periode |

#### 95. Payroll Run & Approval

| Atribut | Nilai |
|---|---|
| **Portal** | Admin / Payroll |
| **Peran** | Payroll Officer; HR Manager; Finance Manager |
| **Scope** | Add-on |
| **Phase** | Phase 9 |
| **Tujuan** | Perhitungan, persetujuan, dan finalisasi gaji per periode. |
| **Fitur** | Payroll calculation, Variance review, Approval, Finalization, Bank file |
| **Sub-fitur** | Draft perhitungan, BPJS, PPh 21, perbandingan periode lalu, approval berjenjang, finalisasi, run susulan, file transfer bank, status pembayaran |

| ID | Kebutuhan | Prio | Scope | Phase |
|---|---|---|---|---|
| FR-95-001 | Sistem harus menghitung gaji setiap pegawai aktif per periode: komponen pendapatan dan potongan, iuran BPJS, dan PPh 21 sesuai parameter yang berlaku (BR-PRL-01), sebagai draft. | M | Add-on | Phase 9 |
| FR-95-002 | Sistem harus menampilkan perbandingan dengan periode sebelumnya dan menandai pegawai dengan selisih take-home pay di atas [10%]. | S | Add-on | Phase 9 |
| FR-95-003 | Sistem harus memproses payroll run melalui workflow approval (menu 79), misalnya HR Manager lalu Finance Manager, sebelum finalisasi. | M | Add-on | Phase 9 |
| FR-95-004 | Sistem harus mengunci payroll yang sudah final dan menerapkan BR-PRL-02 serta BR-PRL-03. | M | Add-on | Phase 9 |
| FR-95-005 | Sistem harus menghasilkan file transfer gaji sesuai format bank yang dikonfigurasi [OQ-22] dan mencatat status pembayaran gaji per pegawai. | M | Add-on | Phase 9 |

| ID | Skenario | Given / When / Then |
|---|---|---|
| AC-95-01 | Iuran BPJS Kesehatan | **Given** iuran BPJS Kesehatan dikonfigurasi 4% pemberi kerja dan 1% pegawai dengan batas upah Rp12.000.000, dan upah pegawai Rp5.000.000<br>**When** payroll dihitung<br>**Then** potongan pegawai Rp50.000 dan iuran pemberi kerja Rp200.000 |
| AC-95-02 | PPh 21 bulanan dari tabel | **Given** tabel tarif efektif yang dikonfigurasi menetapkan 2% untuk penghasilan bruto Rp10.000.000 pada kategori pegawai tersebut<br>**When** payroll Oktober dihitung<br>**Then** PPh 21 Oktober pegawai tersebut Rp200.000 |
| AC-95-03 | Selisih besar ditandai | **Given** take-home pay pegawai September Rp6.000.000 dan Oktober Rp7.200.000<br>**When** draft Oktober ditinjau<br>**Then** pegawai tersebut ditandai selisih 20% |
| AC-95-04 | Run ganda dicegah | **Given** payroll reguler Oktober unit SD sudah final<br>**When** Payroll Officer menjalankan payroll reguler Oktober unit SD lagi<br>**Then** sistem menolak dan menyarankan run susulan |

#### 96. Slip Gaji

| Atribut | Nilai |
|---|---|
| **Portal** | Admin / Payroll |
| **Peran** | Payroll Officer; Pegawai |
| **Scope** | Add-on |
| **Phase** | Phase 9 |
| **Tujuan** | Pembuatan dan distribusi slip gaji setelah payroll final. Pegawai melihat slip di menu 87. |
| **Fitur** | Payslip generation, Publication, Archive |
| **Sub-fitur** | Template slip per yayasan, PDF per pegawai, publikasi, notifikasi, perlindungan akses, arsip per periode |

| ID | Kebutuhan | Prio | Scope | Phase |
|---|---|---|---|---|
| FR-96-001 | Sistem harus membuat slip gaji PDF per pegawai setelah payroll final, memakai template slip per yayasan. | M | Add-on | Phase 9 |
| FR-96-002 | Sistem harus mempublikasikan slip ke menu 87 dan memberi tahu pegawai. | M | Add-on | Phase 9 |
| FR-96-003 | Sistem harus membatasi akses slip kepada pegawai yang bersangkutan dan peran yang berwenang (BR-HR-03). | M | Add-on | Phase 9 |
| FR-96-004 | Sistem harus mengarsipkan slip per periode dan memungkinkan Payroll Officer menerbitkan ulang slip yang sama tanpa mengubah isinya. | S | Add-on | Phase 9 |

| ID | Skenario | Given / When / Then |
|---|---|---|
| AC-96-01 | Slip terbit setelah final | **Given** payroll Oktober masih draft<br>**When** pegawai membuka menu slip gaji<br>**Then** slip Oktober belum tersedia |
| AC-96-02 | Akses slip dibatasi | **Given** Kepala Unit tidak memiliki peran payroll<br>**When** Kepala Unit membuka slip gaji guru di unitnya<br>**Then** akses ditolak |

#### 97. PPh 21 & BPJS

| Atribut | Nilai |
|---|---|
| **Portal** | Admin / Payroll |
| **Peran** | Payroll Officer; Finance Manager |
| **Scope** | Add-on |
| **Phase** | Phase 9 |
| **Tujuan** | Rekap dan pelaporan pajak penghasilan serta iuran jaminan sosial pegawai. |
| **Fitur** | Tax reporting, Social security reporting |
| **Sub-fitur** | Rekap PPh 21 bulanan, data unggah e-Bupot, perhitungan tahunan, bukti potong, laporan iuran BPJS, rekonsiliasi potongan dan setoran |

| ID | Kebutuhan | Prio | Scope | Phase |
|---|---|---|---|---|
| FR-97-001 | Sistem harus merekap PPh 21 bulanan per unit dan per NPWP pemotong dari payroll yang sudah final. | M | Add-on | Phase 9 |
| FR-97-002 | Sistem harus menghasilkan data siap unggah untuk pelaporan PPh 21 sesuai format DJP yang berlaku [OQ-19]. | M | Add-on | Phase 9 |
| FR-97-003 | Sistem harus menghitung PPh 21 setahun pada masa pajak terakhir dan menerbitkan bukti potong tahunan per pegawai. | M | Add-on | Phase 9 |
| FR-97-004 | Sistem harus menghasilkan laporan iuran BPJS Kesehatan dan BPJS Ketenagakerjaan per program dan periode. | M | Add-on | Phase 9 |
| FR-97-005 | Sistem harus merekonsiliasi total potongan pajak dan iuran dengan nilai setoran yang dicatat. | S | Add-on | Phase 9 |

| ID | Skenario | Given / When / Then |
|---|---|---|
| AC-97-01 | Rekap PPh 21 bulanan | **Given** payroll Oktober unit SD dan SMP sudah final<br>**When** Payroll Officer membuka rekap PPh 21 Oktober<br>**Then** rekap menampilkan total PPh 21 per unit dan per NPWP pemotong |
| AC-97-02 | Selisih setoran | **Given** total potongan BPJS Oktober Rp12.500.000 dan setoran tercatat Rp12.000.000<br>**When** rekonsiliasi dijalankan<br>**Then** selisih Rp500.000 ditampilkan untuk ditindaklanjuti |

#### 98. Jurnal Payroll

| Atribut | Nilai |
|---|---|
| **Portal** | Admin / Payroll / Accounting |
| **Peran** | Payroll Officer; Accounting |
| **Scope** | Add-on |
| **Phase** | Phase 9 |
| **Tujuan** | Pembentukan jurnal penggajian per unit ke Accounting Engine. |
| **Fitur** | Payroll journal, Payment journal |
| **Sub-fitur** | Jurnal beban dan utang per komponen, jurnal pembayaran gaji, status posting, kegagalan posting |

| ID | Kebutuhan | Prio | Scope | Phase |
|---|---|---|---|---|
| FR-98-001 | Setelah payroll final, sistem harus membentuk jurnal per unit: beban gaji per komponen, utang gaji, utang PPh 21, utang BPJS, dan potongan lain sesuai mapping di menu 93. | M | Add-on | Phase 9 |
| FR-98-002 | Sistem harus membentuk jurnal pembayaran gaji ketika status transfer dikonfirmasi. | M | Add-on | Phase 9 |
| FR-98-003 | Sistem harus mengirim jurnal ke Accounting Engine secara idempoten dan menerapkan BR-INT-01; kegagalan masuk menu 80. | M | Add-on | Phase 9 |
| FR-98-004 | Sistem harus menampilkan status posting jurnal payroll per periode dan unit. | S | Add-on | Phase 9 |

| ID | Skenario | Given / When / Then |
|---|---|---|
| AC-98-01 | Jurnal payroll seimbang | **Given** total beban gaji unit SD Oktober Rp150.000.000 dengan potongan PPh 21 Rp5.000.000 dan BPJS pegawai Rp3.000.000<br>**When** jurnal dibentuk<br>**Then** utang gaji Rp142.000.000, utang PPh 21 Rp5.000.000, dan utang BPJS Rp3.000.000 dicatat seimbang dengan beban |
| AC-98-02 | Pengiriman ulang tidak ganda | **Given** jurnal payroll Oktober unit SD sudah diposting<br>**When** sistem mengirim ulang jurnal yang sama<br>**Then** Accounting Engine tidak membuat jurnal kedua |

### Grup Menu 15 — Pelatihan Pegawai

*Phase 14 · Add-on. Pembelajaran dan pengembangan guru serta staf.*

#### 99. Katalog Pelatihan

| Atribut | Nilai |
|---|---|
| **Portal** | Admin / HR / ESS |
| **Peran** | Training Admin; Pegawai |
| **Scope** | Add-on |
| **Phase** | Phase 14 |
| **Tujuan** | Katalog pelatihan internal dan eksternal yang dapat diikuti pegawai. |
| **Fitur** | Course catalog, Enrollment |
| **Sub-fitur** | Judul, tujuan, durasi, penyelenggara, materi (file, tautan, video), kuis, kategori, kompetensi, pelatihan wajib per jabatan, jadwal tatap muka, kuota, pendaftaran |

| ID | Kebutuhan | Prio | Scope | Phase |
|---|---|---|---|---|
| FR-99-001 | Sistem harus mengelola pelatihan dengan judul, tujuan, durasi, penyelenggara, materi (file, tautan, atau video), dan kuis opsional. | M | Add-on | Phase 14 |
| FR-99-002 | Sistem harus mengelompokkan pelatihan per kategori dan kompetensi, serta menandai pelatihan wajib per jabatan. | M | Add-on | Phase 14 |
| FR-99-003 | Sistem harus mengelola jadwal dan kuota untuk pelatihan tatap muka. | S | Add-on | Phase 14 |
| FR-99-004 | Sistem harus memungkinkan pegawai menelusuri katalog dan mendaftar; pendaftaran pelatihan berbayar diproses melalui workflow approval (menu 79). | M | Add-on | Phase 14 |

| ID | Skenario | Given / When / Then |
|---|---|---|
| AC-99-01 | Kuota penuh | **Given** kuota pelatihan tatap muka 20 dan sudah terisi 20<br>**When** pegawai mendaftar<br>**Then** pendaftaran ditolak dengan pesan kuota penuh |
| AC-99-02 | Pelatihan berbayar | **Given** pelatihan eksternal berbiaya Rp1.500.000<br>**When** pegawai mendaftar<br>**Then** pendaftaran menunggu persetujuan atasan |

#### 100. Onboarding Pegawai

| Atribut | Nilai |
|---|---|
| **Portal** | Admin / HR / ESS |
| **Peran** | HR Admin; atasan langsung; IT; Pegawai baru |
| **Scope** | Add-on |
| **Phase** | Phase 14 |
| **Tujuan** | Checklist kesiapan pegawai baru dari offer diterima sampai hari pertama dan masa awal kerja. |
| **Fitur** | Onboarding plan, Task assignment |
| **Sub-fitur** | Template per jabatan/unit, pemicu otomatis, tugas dan penanggung jawab, tenggat, pembuatan akun, pelatihan wajib, progres |

| ID | Kebutuhan | Prio | Scope | Phase |
|---|---|---|---|---|
| FR-100-001 | Sistem harus mengelola template checklist onboarding per jabatan dan unit (dokumen, akun, perangkat, orientasi, pelatihan wajib). | M | Add-on | Phase 14 |
| FR-100-002 | Sistem harus membuat checklist otomatis saat offer diterima (menu 92) atau pegawai baru ditambahkan di menu 83. | M | Add-on | Phase 14 |
| FR-100-003 | Sistem harus menugaskan setiap tugas kepada penanggung jawab dengan tenggat dan mengingatkan tugas yang terlambat. | M | Add-on | Phase 14 |
| FR-100-004 | Sistem harus membuat akun pengguna pegawai (menu 05) pada tanggal mulai kerja dan menerapkan BR-TRN-01. | M | Add-on | Phase 14 |
| FR-100-005 | Sistem harus menampilkan checklist kepada pegawai baru di menu 87 dan progresnya kepada HR. | S | Add-on | Phase 14 |

| ID | Skenario | Given / When / Then |
|---|---|---|
| AC-100-01 | Checklist otomatis | **Given** template onboarding Guru berisi 8 tugas<br>**When** offer guru baru diterima<br>**Then** checklist 8 tugas terbentuk dengan penanggung jawab dan tenggatnya |
| AC-100-02 | Akun dibuat pada tanggal mulai | **Given** tanggal mulai kerja pegawai baru 1 Januari<br>**When** tanggal menunjukkan 1 Januari<br>**Then** akun pengguna pegawai aktif dan pelatihan wajib jabatannya tertugaskan |

#### 101. Penugasan & Penyelesaian Pelatihan

| Atribut | Nilai |
|---|---|
| **Portal** | Admin / HR / ESS |
| **Peran** | Training Admin; Pegawai; atasan langsung |
| **Scope** | Add-on |
| **Phase** | Phase 14 |
| **Tujuan** | Penugasan pelatihan, pelaksanaan oleh pegawai, dan pemantauan kepatuhan. |
| **Fitur** | Training assignment, Learning progress, Compliance |
| **Sub-fitur** | Penugasan per pegawai/jabatan/unit, tenggat, materi, kuis, unggah sertifikat, status, pengingat, rekap kepatuhan, riwayat |

| ID | Kebutuhan | Prio | Scope | Phase |
|---|---|---|---|---|
| FR-101-001 | Sistem harus menugaskan pelatihan kepada pegawai, jabatan, atau unit dengan tenggat. | M | Add-on | Phase 14 |
| FR-101-002 | Sistem harus memungkinkan pegawai mempelajari materi, mengerjakan kuis, dan mengunggah bukti atau sertifikat. | M | Add-on | Phase 14 |
| FR-101-003 | Sistem harus menghitung status pelatihan (belum mulai, berjalan, selesai, terlambat) dan mengingatkan tenggat. | M | Add-on | Phase 14 |
| FR-101-004 | Sistem harus merekap kepatuhan pelatihan wajib per unit dan jabatan. | M | Add-on | Phase 14 |
| FR-101-005 | Sistem harus menyimpan sertifikat pelatihan yang selesai ke dokumen pegawai (menu 85). | S | Add-on | Phase 14 |

| ID | Skenario | Given / When / Then |
|---|---|---|
| AC-101-01 | Kuis lulus | **Given** nilai lulus kuis 70<br>**When** pegawai mendapat nilai 80<br>**Then** status pelatihan menjadi selesai |
| AC-101-02 | Rekap kepatuhan | **Given** pelatihan wajib K3 ditugaskan ke 40 guru SMP dan 30 sudah selesai<br>**When** Kepala Unit membuka rekap kepatuhan<br>**Then** kepatuhan pelatihan K3 SMP tampil 75% |

## 6. Alur Bisnis End-to-End

| Alur | Urutan (nomor menu) | Phase |
|---|---|---|
| Rekrutmen sampai pegawai aktif | 88 Permintaan → 79 Approval → 89 Lowongan → 90 Pipeline → 91 Wawancara → 92 Offer → 79 Approval → 83 Calon pegawai → 100 Onboarding → 05 Akun aktif | Phase 14 (HRIS Phase 9) |
| Cuti dan lembur | 87 Pengajuan → 79 Approval atasan → 86 Saldo dan rekap | Phase 9 |
| Penggajian bulanan | 86 Rekap periode → 94 Input → 95 Run draft → 79 Approval → 95 Final + file bank → 96 Slip → 87 Pegawai → 97 PPh 21 & BPJS → 98 Jurnal → Accounting Engine | Phase 9 |
| Pegawai keluar | 83 Offboarding → 05 Akun nonaktif → 95 Payroll terakhir → 97 Bukti potong | Phase 9 |
| Pelatihan wajib | 84 Jabatan → BR-TRN-01 → 101 Penugasan → 101 Penyelesaian → 85 Sertifikat | Phase 14 |

## 7. Integrasi

| ID | Integrasi | Kebutuhan | Phase |
|---|---|---|---|
| IF-HR-01 | Accounting Engine | Sistem harus mengirim jurnal payroll melalui accounting-adapter dengan idempotency key per unit dan periode. | Phase 9 |
| IF-HR-02 | Bank | Sistem harus menghasilkan file transfer gaji sesuai format bank yang dikonfigurasi [OQ-22]. | Phase 9 |
| IF-HR-03 | DJP | Sistem harus menghasilkan data pelaporan PPh 21 dalam format unggah yang berlaku [OQ-19]. | Phase 9 |
| IF-HR-04 | BPJS | Sistem harus menghasilkan data iuran dalam format yang dapat dipakai untuk pelaporan ke BPJS [OQ-20]. | Phase 9 |
| IF-HR-05 | Mesin absensi | Sistem harus menerima file ekspor mesin absensi (CSV/Excel) dengan pemetaan kolom; integrasi langsung perangkat belum termasuk [OQ-24]. | Phase 9 |
| IF-HR-06 | Email | Sistem harus mengirim undangan dan pemberitahuan kepada kandidat yang belum memiliki akun melalui email. | Phase 14 |

## 8. Kebutuhan Non-Fungsional & Keamanan Khusus SDM

| ID | Area | Kebutuhan | Prio | Verif. |
|---|---|---|---|---|
| NFR-HR-01 | Kerahasiaan | Gaji, rekening, NIK, dan NPWP dienkripsi di tingkat kolom dan tidak muncul di log. | M | I |
| NFR-HR-02 | Akses | Peran Payroll Officer, HR Manager, dan penyetuju payroll wajib MFA. | M | T |
| NFR-HR-03 | Audit | Finalisasi payroll, perubahan komponen dan parameter, perubahan gaji pegawai, serta penerbitan ulang slip tercatat di audit trail. | M | T |
| NFR-HR-04 | Kinerja | Payroll run untuk [500] pegawai selesai ≤ [5] menit. | S | T |
| NFR-HR-05 | Retensi | Data payroll dan pajak disimpan sesuai kewajiban dokumentasi perpajakan yang berlaku; data kandidat mengikuti BR-REC-02. | M | I |
| NFR-HR-06 | Akurasi | Hasil perhitungan PPh 21 dan BPJS diverifikasi terhadap contoh kasus yang disetujui konsultan pajak sebelum go-live. | M | T |

## 9. Pertanyaan Terbuka Bagian 2

Melanjutkan penomoran Bagian 1.

| ID | Pertanyaan | Terdampak | Prioritas |
|---|---|---|---|
| OQ-19 | Validasi aturan PPh 21 terkini (tarif efektif bulanan, perhitungan tahunan) dan format pelaporan DJP yang berlaku, oleh konsultan pajak | 93, 95, 97 | Tinggi |
| OQ-20 | Parameter default BPJS (persentase dan batas upah tahun berjalan) dan format laporan iuran | 93, 95, 97 | Tinggi |
| OQ-21 | Skema default honor guru (per jam, per kegiatan, per wali kelas) | 93, 94 | Sedang |
| OQ-22 | Format file transfer gaji untuk bank yang didukung | 95 | Sedang |
| OQ-23 | Kebijakan pencatatan lokasi saat check-in pegawai | 87 | Sedang |
| OQ-24 | Kebutuhan integrasi langsung perangkat absensi (fingerprint, RFID, face) sebagai menu tersendiri | 86 | Sedang |
| OQ-25 | Masa simpan data kandidat yang tidak diterima | 90, BR-REC-02 | Sedang |
| OQ-26 | Kebutuhan integrasi dengan portal lowongan eksternal | 89 | Rendah |
| OQ-27 | Apakah pegawai berstatus bukan pegawai (honorer lepas) diproses dalam payroll yang sama | 93, 95, 97 | Sedang |

## 10. Persetujuan Dokumen

| Peran | Nama | Tanggal | Tanda tangan |
|---|---|---|---|
| Product Owner | David Pambudi | | |
| Project Manager | David Pambudi | | |
| HR / Payroll Subject Matter Expert | [Nama] | | |
| Konsultan Pajak | [Nama] | | |

## Lampiran — Batas Tanggung Jawab Antar-Menu (Domain SDM)

| Topik | Pemilik kebutuhan | Menu/aturan lain hanya … |
|---|---|---|
| Data guru dan staf | Menu 83 (bila HRIS aktif) | menu 03 menampilkan data hasil sinkronisasi (BR-HR-01) |
| Struktur yayasan dan unit | Menu 01 | menu 84 menambahkan bagian, jabatan, dan atasan langsung di dalam unit |
| Kehadiran dan cuti pegawai | Menu 86 (back office) | menu 87 untuk pengajuan dan check-in pegawai; menu 41–43 hanya untuk siswa |
| Slip gaji | Menu 96 (pembuatan dan publikasi) | menu 87 menampilkan slip milik pegawai |
| Dokumen pegawai | Menu 85 | menu 87 untuk unggah dokumen yang diminta; menu 101 menyimpan sertifikat ke menu 85 |
| Perhitungan PPh 21 dan BPJS | Menu 95 | menu 93 menyimpan parameter; menu 97 merekap dan melaporkan |
| Jurnal payroll | Menu 98 | menu 34 menerima posting dari sumber payroll |
| Pegawai baru | Menu 92 (dari ATS) atau menu 83 | menu 100 menjalankan onboarding |
| Approval SDM | Menu 79 | menu 86, 88, 92, 93, 95, 99 menentukan kapan approval dibutuhkan |
