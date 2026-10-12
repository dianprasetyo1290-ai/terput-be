# Master PRD — SEKOCI Integrated School Operating Platform (SaaS)

**Bagian 4 dari 4 · Mutu, pemerintah, komunikasi, pembayaran lanjutan, analitik & AI, 3D Campus, white-label (menu 142–159)**

Kebutuhan fungsional, aturan bisnis, dan kriteria penerimaan untuk domain lanjutan produk SEKOCI.

| Atribut | Nilai |
|---|---|
| Document ID | SEKOCI-MPRD-001 |
| Versi | 1.0 — Bagian 4 (draft) |
| Tanggal | 10/10/2026 |
| Status | Draft, menunggu review |
| Produk | SEKOCI Integrated School Operating Platform (SaaS) |
| Product Owner | David Pambudi |
| Project Manager | David Pambudi |
| Klasifikasi | Rahasia — internal tim |

## Daftar Isi

- [Kontrol Dokumen](#kontrol-dokumen)
  - [Riwayat versi](#riwayat-versi)
  - [Hubungan dengan Bagian 1–3](#hubungan-dengan-bagian-13)
- [1. Ringkasan Bagian 4](#1-ringkasan-bagian-4)
- [2. Peran Tambahan](#2-peran-tambahan)
- [3. Entitas Data & System of Record](#3-entitas-data--system-of-record)
- [4. Aturan Bisnis Domain Lanjutan](#4-aturan-bisnis-domain-lanjutan)
- [5. Kebutuhan Fungsional per Grup Menu & Menu](#5-kebutuhan-fungsional-per-grup-menu--menu)
  - [Grup Menu 27 — QA & Akreditasi](#grup-menu-27--qa--akreditasi)
  - [Grup Menu 28 — Pelaporan Pemerintah](#grup-menu-28--pelaporan-pemerintah)
  - [Grup Menu 29 — Komunikasi Lanjutan](#grup-menu-29--komunikasi-lanjutan)
  - [Grup Menu 30 — Ekstensi Pembayaran](#grup-menu-30--ekstensi-pembayaran)
  - [Grup Menu 31 — Analitik & AI](#grup-menu-31--analitik--ai)
  - [Grup Menu 32 — 3D Campus](#grup-menu-32--3d-campus)
  - [Grup Menu 33 — Aplikasi White-label](#grup-menu-33--aplikasi-white-label)
- [6. Alur Bisnis End-to-End](#6-alur-bisnis-end-to-end)
- [7. Integrasi](#7-integrasi)
- [8. Kebutuhan Non-Fungsional Khusus](#8-kebutuhan-non-fungsional-khusus)
- [9. Pertanyaan Terbuka Bagian 4](#9-pertanyaan-terbuka-bagian-4)
- [10. Persetujuan Dokumen](#10-persetujuan-dokumen)
- [Lampiran — Batas Tanggung Jawab Antar-Menu (Domain Lanjutan)](#lampiran--batas-tanggung-jawab-antar-menu-domain-lanjutan)

## Kontrol Dokumen

### Riwayat versi

| Versi | Tanggal | Pemilik | Perubahan |
|---|---|---|---|
| 1.0 — Bagian 4 | 10/10/2026 | PT FullCircle Solusi Indonesia | Kebutuhan grup menu 27–33 (menu 142–159); penyesuaian batas menu 28 dan 75 di Bagian 1 |

### Hubungan dengan Bagian 1–3

Kerangka umum di Bagian 1 berlaku penuh. Bagian ini melengkapi katalog 159 menu. Dua kebutuhan di Bagian 1 disesuaikan agar tidak tumpang tindih: pembayaran gerai retail dipindah dari menu 28 ke menu 151, dan menu 75 dibatasi pada pengumuman satu arah karena percakapan dua arah ada di menu 148.

## 1. Ringkasan Bagian 4

| Grup | Menu | Phase |
|---|---|---|
| 27. QA & Akreditasi | 142–144 | Phase 13 |
| 28. Pelaporan Pemerintah | 145–146 | Phase 13 |
| 29. Komunikasi Lanjutan | 147–149 | Phase 15 |
| 30. Ekstensi Pembayaran | 150–151 | Phase 15 |
| 31. Analitik & AI | 152–154 | Phase 15 |
| 32. 3D Campus | 155–158 | Phase 16 |
| 33. Aplikasi White-label | 159 | Phase 15 |

Seluruh menu berlabel Add-on. Bagian ini memuat 66 kebutuhan fungsional dan 24 skenario Given/When/Then.

## 2. Peran Tambahan

| Peran | Tujuan | Lingkup data |
|---|---|---|
| QA Coordinator / Tim Penjaminan Mutu | Standar, bukti, evaluasi diri | Unit atau yayasan |
| Operator Dapodik | Sinkronisasi data pemerintah | Unit yang ditugaskan |
| Humas / Marketing | Kampanye, landing page, referral | Unit atau yayasan |
| BI Analyst | Dashboard dan laporan kustom | Sesuai lingkup yang diberikan |
| Platform Admin (FullCircle) | Konfigurasi provider pembayaran dan build white-label per klien | Instalasi klien |

## 3. Entitas Data & System of Record

| Entitas | Sumber data utama | Catatan |
|---|---|---|
| Standard, Indicator, Target, Evidence, Self-evaluation, Action Plan | SEKOCI Quality | Nilai indikator otomatis dari metric dictionary |
| Dapodik Mapping, Sync Log, Regulatory Report Snapshot | SEKOCI Government Reporting | SEKOCI tidak menimpa data Dapodik secara otomatis |
| Segment, Campaign, Consent (opt-in/opt-out) | SEKOCI Communication | Pengiriman melalui notification-service |
| Conversation, Message | SEKOCI Communication | Disimpan untuk audit |
| Landing Page, Referral Code, Attribution | SEKOCI Marketing | Reward melalui promo menu 09 |
| Payment Provider, Routing Rule, Autodebit Mandate | SEKOCI Payment | Kredensial di secret manager |
| Custom Dashboard, Scheduled Report | SEKOCI Analytics | Memakai read model dan metric dictionary |
| AI Conversation Log, Prediction, Model Metadata | SEKOCI AI | Retensi mengikuti kebijakan klien |
| 3D Model, Room Mapping | SEKOCI 3D Campus | Data ruang, aset, tenant, dan pemesanan dibaca dari menu pemiliknya |
| App Branding, App Build | SEKOCI Platform | Per klien |

## 4. Aturan Bisnis Domain Lanjutan

| ID | Aturan |
|---|---|
| BR-QA-01 | Setelah evaluasi diri difinalisasi, bukti dan nilai indikator periode tersebut dikunci; perubahan memerlukan pembukaan kembali melalui approval. |
| BR-GOV-01 | SEKOCI tidak menimpa data Dapodik maupun data SEKOCI secara otomatis; setiap perbedaan diputuskan operator dan keputusan tercatat. |
| BR-COM-01 | Kampanye promosi hanya dikirim kepada penerima yang memberikan persetujuan dan tidak memilih berhenti; notifikasi transaksional dan pengumuman wajib (menu 07) tidak terpengaruh. |
| BR-COM-02 | Percakapan hanya dapat dilakukan antara guru dan orang tua siswa yang diampunya atau yang diwalikannya; seluruh pesan disimpan untuk audit. |
| BR-REF-01 | Reward referral hanya diterbitkan setelah pendaftar yang memakai kode referral menyelesaikan daftar ulang, dan diterbitkan melalui mesin promo (menu 09). |
| BR-PAY-03 | Setiap transaksi diproses oleh tepat satu provider; status, settlement, dan rekonsiliasinya dicatat per provider. |
| BR-PAY-04 | Autodebit hanya berjalan dengan mandat aktif dari orang tua untuk jenis tagihan yang disetujui; orang tua dapat membatalkan mandat kapan saja. |
| BR-AI-01 | AI Assistant hanya membaca data dalam lingkup akses pengguna yang bertanya, tidak menjalankan aksi tanpa konfirmasi, dan data klien tidak dipakai untuk melatih model tanpa persetujuan tertulis klien. |
| BR-AI-02 | Hasil prediksi bersifat rekomendasi; tidak ada keputusan otomatis yang berdampak pada siswa, orang tua, atau pegawai (misalnya sanksi, penolakan, atau penagihan khusus) tanpa keputusan manusia. |
| BR-3D-01 | 3D Campus tidak menyimpan salinan data operasional; data ruang, aset, tenant, pemesanan, dan perawatan dibaca dari menu pemiliknya. |

## 5. Kebutuhan Fungsional per Grup Menu & Menu

Kebutuhan lintas modul (Bagian 1, bab 10) dan aturan bisnis (Bagian 1, bab 9 dan bab 4 di atas) berlaku tanpa ditulis ulang.

### Grup Menu 27 — QA & Akreditasi

*Phase 13 · Add-on. Penjaminan mutu internal dan persiapan akreditasi.*

#### 142. Standar & Indikator

| Atribut | Nilai |
|---|---|
| **Portal** | Admin / Penjaminan Mutu |
| **Peran** | QA Coordinator; Kepala Unit |
| **Scope** | Add-on |
| **Phase** | Phase 13 |
| **Tujuan** | Kerangka standar mutu dan indikatornya yang dipakai untuk evaluasi diri dan akreditasi. |
| **Fitur** | Quality framework, Indicators, Targets |
| **Sub-fitur** | Standar, butir/instrumen, indikator, versi instrumen, target per unit, sumber data otomatis, penanggung jawab |

| ID | Kebutuhan | Prio | Scope | Phase |
|---|---|---|---|---|
| FR-142-001 | Sistem harus mengelola kerangka standar mutu beserta butir, indikator, dan versinya, sehingga instrumen akreditasi yang berlaku dapat dimuat tanpa perubahan kode [OQ-37]. | M | Add-on | Phase 13 |
| FR-142-002 | Sistem harus menetapkan target indikator per unit dan tahun ajaran beserta penanggung jawabnya. | M | Add-on | Phase 13 |
| FR-142-003 | Sistem harus menautkan indikator ke metrik yang sudah ada di SEKOCI (misalnya persentase kehadiran, collection rate, rasio pendaftar) sehingga nilainya terisi otomatis dari metric dictionary. | S | Add-on | Phase 13 |

| ID | Skenario | Given / When / Then |
|---|---|---|
| AC-142-01 | Indikator terisi otomatis | **Given** indikator "rata-rata kehadiran siswa" ditautkan ke metrik kehadiran<br>**When** QA Coordinator membuka indikator untuk unit SMP semester ganjil<br>**Then** nilai indikator terisi dari data kehadiran beserta waktu pembaruan terakhir |

#### 143. Repositori Bukti

| Atribut | Nilai |
|---|---|
| **Portal** | Admin / Penjaminan Mutu |
| **Peran** | QA Coordinator; penanggung jawab indikator |
| **Scope** | Add-on |
| **Phase** | Phase 13 |
| **Tujuan** | Kumpulan bukti dokumen untuk setiap indikator. |
| **Fitur** | Evidence repository |
| **Sub-fitur** | Unggah bukti, tautan ke indikator, tautan ke laporan SEKOCI, versi, status verifikasi, kelengkapan |

| ID | Kebutuhan | Prio | Scope | Phase |
|---|---|---|---|---|
| FR-143-001 | Sistem harus memungkinkan penanggung jawab mengunggah bukti dan menautkannya ke satu atau lebih indikator. | M | Add-on | Phase 13 |
| FR-143-002 | Sistem harus memungkinkan bukti berupa tautan ke laporan SEKOCI yang dapat dibuka ulang sesuai hak akses. | S | Add-on | Phase 13 |
| FR-143-003 | Sistem harus memverifikasi bukti oleh QA Coordinator dan menampilkan kelengkapan bukti per standar. | M | Add-on | Phase 13 |
| FR-143-004 | Sistem harus menerapkan BR-QA-01 setelah evaluasi diri difinalisasi. | M | Add-on | Phase 13 |

| ID | Skenario | Given / When / Then |
|---|---|---|
| AC-143-01 | Kelengkapan bukti | **Given** standar sarana memiliki 10 indikator dan 7 sudah memiliki bukti terverifikasi<br>**When** QA Coordinator membuka ringkasan<br>**Then** kelengkapan standar sarana tampil 70% |

#### 144. Evaluasi Diri & Laporan Akreditasi

| Atribut | Nilai |
|---|---|
| **Portal** | Admin / Penjaminan Mutu / Yayasan |
| **Peran** | QA Coordinator; Kepala Unit; Pengurus Yayasan |
| **Scope** | Add-on |
| **Phase** | Phase 13 |
| **Tujuan** | Evaluasi diri berkala, rencana tindak lanjut, dan laporan untuk akreditasi. |
| **Fitur** | Self-evaluation, Action plan, Accreditation report |
| **Sub-fitur** | Penilaian per indikator, skor standar, rencana perbaikan, penanggung jawab dan tenggat, finalisasi, laporan per instrumen, perbandingan antar unit |

| ID | Kebutuhan | Prio | Scope | Phase |
|---|---|---|---|---|
| FR-144-001 | Sistem harus memungkinkan tim menilai capaian setiap indikator dan menghitung skor per standar sesuai bobot instrumen. | M | Add-on | Phase 13 |
| FR-144-002 | Sistem harus mengelola rencana tindak lanjut untuk indikator di bawah target, dengan penanggung jawab dan tenggat. | M | Add-on | Phase 13 |
| FR-144-003 | Sistem harus memproses finalisasi evaluasi diri melalui workflow approval (menu 79). | M | Add-on | Phase 13 |
| FR-144-004 | Sistem harus menghasilkan laporan evaluasi diri dan laporan persiapan akreditasi per unit, serta perbandingan antar unit untuk yayasan. | M | Add-on | Phase 13 |

| ID | Skenario | Given / When / Then |
|---|---|---|
| AC-144-01 | Rencana tindak lanjut | **Given** target indikator rasio buku per siswa 2 dan capaian 1,4<br>**When** evaluasi diri disimpan<br>**Then** indikator ditandai di bawah target dan meminta rencana tindak lanjut sebelum finalisasi |

### Grup Menu 28 — Pelaporan Pemerintah

*Phase 13 · Add-on. Sinkronisasi data dan laporan wajib ke instansi pemerintah.*

#### 145. Sinkronisasi Dapodik

| Atribut | Nilai |
|---|---|
| **Portal** | Admin / Operator |
| **Peran** | Operator Dapodik; School Admin |
| **Scope** | Add-on |
| **Phase** | Phase 13 |
| **Tujuan** | Menyelaraskan data siswa, rombel, dan PTK antara SEKOCI dan Dapodik. |
| **Fitur** | Data mapping, Comparison, Sync log |
| **Sub-fitur** | Pemetaan field, perbandingan data, daftar perbedaan, validasi, ekspor/impor sesuai mekanisme yang diizinkan, log sinkronisasi |

| ID | Kebutuhan | Prio | Scope | Phase |
|---|---|---|---|---|
| FR-145-001 | Sistem harus memetakan data siswa, rombongan belajar, dan PTK di SEKOCI ke struktur data Dapodik. | M | Add-on | Phase 13 |
| FR-145-002 | Sistem harus membandingkan data SEKOCI dengan data Dapodik yang diambil melalui mekanisme yang diizinkan [OQ-36] dan menampilkan daftar perbedaan per field. | M | Add-on | Phase 13 |
| FR-145-003 | Sistem harus menerapkan BR-GOV-01 dalam menyelesaikan perbedaan. | M | Add-on | Phase 13 |
| FR-145-004 | Sistem harus menyiapkan data pembaruan dalam format yang dapat dipakai operator untuk memperbarui Dapodik, dan mencatat log setiap sinkronisasi. | M | Add-on | Phase 13 |

| ID | Skenario | Given / When / Then |
|---|---|---|
| AC-145-01 | Perbedaan data | **Given** NISN siswa di SEKOCI berbeda dengan di Dapodik<br>**When** perbandingan dijalankan<br>**Then** perbedaan ditampilkan untuk diputuskan operator dan data tidak ditimpa otomatis |

#### 146. Laporan Regulasi

| Atribut | Nilai |
|---|---|
| **Portal** | Admin / Operator / Yayasan |
| **Peran** | School Admin; Kepala Unit; Pengurus Yayasan |
| **Scope** | Add-on |
| **Phase** | Phase 13 |
| **Tujuan** | Laporan berkala yang diwajibkan instansi pemerintah atau yayasan. |
| **Fitur** | Regulatory report templates |
| **Sub-fitur** | Template laporan, sumber data, periode, pratinjau, approval, ekspor, riwayat pengiriman |

| ID | Kebutuhan | Prio | Scope | Phase |
|---|---|---|---|---|
| FR-146-001 | Sistem harus mengelola template laporan regulasi (misalnya laporan bulanan sekolah) yang mengambil data dari modul SEKOCI. | M | Add-on | Phase 13 |
| FR-146-002 | Sistem harus menampilkan pratinjau, memproses persetujuan laporan melalui workflow approval (menu 79), dan mengekspornya dalam format yang ditentukan. | M | Add-on | Phase 13 |
| FR-146-003 | Sistem harus menyimpan riwayat laporan yang sudah diserahkan beserta datanya pada saat itu. | M | Add-on | Phase 13 |

| ID | Skenario | Given / When / Then |
|---|---|---|
| AC-146-01 | Laporan tersimpan apa adanya | **Given** laporan bulan September sudah diserahkan<br>**When** data siswa September kemudian dikoreksi<br>**Then** laporan September yang tersimpan tetap menampilkan data saat diserahkan |

### Grup Menu 29 — Komunikasi Lanjutan

*Phase 15 · Add-on. Kampanye pesan massal, percakapan guru–orang tua, dan pemasaran PPDB.*

#### 147. WhatsApp Broadcast & Kampanye

| Atribut | Nilai |
|---|---|
| **Portal** | Admin / Humas |
| **Peran** | Humas / Marketing; School Admin |
| **Scope** | Add-on |
| **Phase** | Phase 15 |
| **Tujuan** | Pengiriman pesan massal tersegmentasi. Notifikasi transaksional dan pengumuman tetap di menu 07. |
| **Fitur** | Segmentation, Campaign, Scheduling, Analytics |
| **Sub-fitur** | Segmen penerima, template WhatsApp yang disetujui, penjadwalan, opt-in/opt-out, kuota, laporan terkirim/dibaca/dibalas |

| ID | Kebutuhan | Prio | Scope | Phase |
|---|---|---|---|---|
| FR-147-001 | Sistem harus membuat segmen penerima dari data SEKOCI (misalnya orang tua per unit, pendaftar PPDB per tahap, alumni) dengan filter yang dapat disimpan. | M | Add-on | Phase 15 |
| FR-147-002 | Sistem harus mengirim kampanye memakai template WhatsApp yang sudah disetujui penyedia, dengan penjadwalan. | M | Add-on | Phase 15 |
| FR-147-003 | Sistem harus menerapkan BR-COM-01. | M | Add-on | Phase 15 |
| FR-147-004 | Sistem harus melaporkan jumlah terkirim, terbaca, dibalas, dan gagal per kampanye. | M | Add-on | Phase 15 |

| ID | Skenario | Given / When / Then |
|---|---|---|
| AC-147-01 | Penerima opt-out dikecualikan | **Given** segmen berisi 500 orang tua dan 20 sudah memilih berhenti menerima promosi<br>**When** kampanye dikirim<br>**Then** pesan terkirim ke 480 orang tua |

#### 148. Chat Guru–Orang Tua

| Atribut | Nilai |
|---|---|
| **Portal** | Web-portal (guru dan orang tua) |
| **Peran** | Guru; Orang tua; Kepala Unit (moderasi) |
| **Scope** | Add-on |
| **Phase** | Phase 15 |
| **Tujuan** | Percakapan dua arah antara guru dan orang tua. Pengumuman satu arah tetap di menu 75. |
| **Fitur** | Messaging, Moderation |
| **Sub-fitur** | Percakapan per siswa, lampiran, jam layanan, status baca, pelaporan pesan, arsip |

| ID | Kebutuhan | Prio | Scope | Phase |
|---|---|---|---|---|
| FR-148-001 | Sistem harus memungkinkan percakapan antara guru dan orang tua siswa yang diampunya (BR-COM-02). | M | Add-on | Phase 15 |
| FR-148-002 | Sistem harus mendukung lampiran, status terkirim dan dibaca, serta notifikasi pesan baru. | M | Add-on | Phase 15 |
| FR-148-003 | Sistem harus menerapkan jam layanan yang dikonfigurasi; pesan di luar jam layanan tetap terkirim dengan informasi bahwa balasan akan diberikan pada jam layanan. | S | Add-on | Phase 15 |
| FR-148-004 | Sistem harus memungkinkan pengguna melaporkan pesan yang tidak pantas kepada Kepala Unit, dan menyimpan seluruh pesan untuk keperluan audit. | M | Add-on | Phase 15 |

| ID | Skenario | Given / When / Then |
|---|---|---|
| AC-148-01 | Hanya guru yang mengampu | **Given** guru Matematika tidak mengajar di kelas 9B<br>**When** guru tersebut mencoba memulai percakapan dengan orang tua siswa 9B<br>**Then** percakapan tidak dapat dimulai |
| AC-148-02 | Di luar jam layanan | **Given** jam layanan 07.00–17.00<br>**When** orang tua mengirim pesan pukul 21.00<br>**Then** pesan terkirim dan orang tua melihat informasi jam layanan |

#### 149. Landing Page & Referral PPDB

| Atribut | Nilai |
|---|---|
| **Portal** | Admin / Humas / Publik |
| **Peran** | Humas / Marketing; PPDB Admin; Orang tua |
| **Scope** | Add-on |
| **Phase** | Phase 15 |
| **Tujuan** | Halaman pemasaran PPDB per unit atau gelombang dan program referral. |
| **Fitur** | Landing page builder, Referral program, Campaign tracking |
| **Sub-fitur** | Template halaman, konten, CTA ke formulir PPDB, kode referral, pelacakan sumber, reward melalui promo, laporan konversi |

| ID | Kebutuhan | Prio | Scope | Phase |
|---|---|---|---|---|
| FR-149-001 | Sistem harus menyediakan pembuat landing page PPDB per unit atau gelombang dari template, dengan tombol yang mengarah ke formulir pendaftaran (menu 16). | M | Add-on | Phase 15 |
| FR-149-002 | Sistem harus mencatat sumber kunjungan dan pendaftaran (misalnya parameter kampanye dan kode referral). | M | Add-on | Phase 15 |
| FR-149-003 | Sistem harus membuat kode referral untuk orang tua siswa aktif dan alumni, dan menerapkan BR-REF-01. | M | Add-on | Phase 15 |
| FR-149-004 | Sistem harus melaporkan kunjungan, pendaftaran, dan konversi per sumber dan per kode referral. | M | Add-on | Phase 15 |

| ID | Skenario | Given / When / Then |
|---|---|---|
| AC-149-01 | Reward referral | **Given** orang tua A membagikan kode referral dan pendaftar B memakai kode tersebut<br>**When** pendaftar B menyelesaikan daftar ulang<br>**Then** reward untuk orang tua A diterbitkan melalui promo (menu 09) |
| AC-149-02 | Reward belum berlaku | **Given** pendaftar B baru membayar formulir<br>**When** laporan referral dibuka<br>**Then** referral orang tua A berstatus menunggu daftar ulang |

### Grup Menu 30 — Ekstensi Pembayaran

*Phase 15 · Add-on. Lebih dari satu penyedia pembayaran, autodebit, dan gerai retail.*

#### 150. Multi Payment Provider

| Atribut | Nilai |
|---|---|
| **Portal** | Admin / Platform |
| **Peran** | Platform Admin; Finance Manager |
| **Scope** | Add-on |
| **Phase** | Phase 15 |
| **Tujuan** | Pemakaian beberapa payment gateway dalam satu instalasi klien. |
| **Fitur** | Provider configuration, Routing, Failover |
| **Sub-fitur** | Daftar provider, kredensial, metode per provider, aturan routing per metode/unit, failover, rekonsiliasi per provider |

| ID | Kebutuhan | Prio | Scope | Phase |
|---|---|---|---|---|
| FR-150-001 | Sistem harus mengelola lebih dari satu payment gateway beserta kredensial (disimpan di secret manager) dan metode yang didukung masing-masing. | M | Add-on | Phase 15 |
| FR-150-002 | Sistem harus menentukan provider untuk setiap transaksi berdasarkan aturan routing per metode dan unit (BR-PAY-03). | M | Add-on | Phase 15 |
| FR-150-003 | Sistem harus mengalihkan pembuatan transaksi baru ke provider cadangan bila provider utama tidak tersedia, sesuai pengaturan. | S | Add-on | Phase 15 |
| FR-150-004 | Sistem harus menjalankan rekonsiliasi settlement (menu 31) per provider. | M | Add-on | Phase 15 |

| ID | Skenario | Given / When / Then |
|---|---|---|
| AC-150-01 | Routing per metode | **Given** QRIS dirutekan ke provider A dan VA ke provider B<br>**When** orang tua memilih VA<br>**Then** transaksi dibuat di provider B |
| AC-150-02 | Failover | **Given** provider A tidak merespons dan failover aktif<br>**When** orang tua memilih QRIS<br>**Then** transaksi baru dibuat di provider cadangan dan kejadian tercatat di menu 80 |

#### 151. Autodebit & Gerai Retail

| Atribut | Nilai |
|---|---|
| **Portal** | Portal Orang Tua / Admin |
| **Peran** | Orang tua; Finance Admin |
| **Scope** | Add-on |
| **Phase** | Phase 15 |
| **Tujuan** | Pembayaran tagihan rutin secara autodebit dan pembayaran tunai di gerai retail. |
| **Fitur** | Recurring debit, Mandate, Retail payment |
| **Sub-fitur** | Pendaftaran mandat autodebit, tagihan yang dicakup, jadwal debit, gagal debit, pembatalan, kode bayar gerai retail |

| ID | Kebutuhan | Prio | Scope | Phase |
|---|---|---|---|---|
| FR-151-001 | Sistem harus memungkinkan orang tua mendaftarkan mandat autodebit untuk jenis tagihan tertentu melalui provider yang mendukung (BR-PAY-04). | M | Add-on | Phase 15 |
| FR-151-002 | Sistem harus mendebit tagihan yang jatuh tempo sesuai jadwal dan mencoba ulang debit yang gagal maksimal [2] kali. | M | Add-on | Phase 15 |
| FR-151-003 | Sistem harus memungkinkan orang tua membatalkan mandat kapan saja, berlaku untuk debit berikutnya. | M | Add-on | Phase 15 |
| FR-151-004 | Sistem harus menyediakan pembayaran tunai di gerai retail dengan kode bayar dan masa berlaku. | M | Add-on | Phase 15 |

| ID | Skenario | Given / When / Then |
|---|---|---|
| AC-151-01 | Debit gagal | **Given** saldo rekening orang tua tidak mencukupi pada tanggal debit<br>**When** debit dijalankan<br>**Then** debit gagal, dicoba ulang sesuai jadwal, dan orang tua menerima notifikasi |
| AC-151-02 | Pembatalan mandat | **Given** orang tua membatalkan mandat pada 20 Oktober<br>**When** tagihan November jatuh tempo<br>**Then** tagihan November tidak didebit otomatis |

### Grup Menu 31 — Analitik & AI

*Phase 15 · Add-on. Analitik mandiri, asisten AI, dan analitik prediktif.*

#### 152. BI & Dashboard Lanjutan

| Atribut | Nilai |
|---|---|
| **Portal** | Admin / Yayasan |
| **Peran** | BI Analyst; Pengurus Yayasan; Kepala Unit |
| **Scope** | Add-on |
| **Phase** | Phase 15 |
| **Tujuan** | Analitik mandiri di atas data SEKOCI. Dashboard standar tetap di menu 08, 17, 26, 32, dan 51–53. |
| **Fitur** | Self-service dashboard, Report builder, Scheduled reports, Data export |
| **Sub-fitur** | Dashboard kustom, pembuat laporan, metrik dari metric dictionary, filter, jadwal kirim laporan, ekspor untuk alat BI eksternal |

| ID | Kebutuhan | Prio | Scope | Phase |
|---|---|---|---|---|
| FR-152-001 | Sistem harus memungkinkan pengguna berwenang menyusun dashboard dan laporan kustom dari metrik dan dimensi yang terdaftar di metric dictionary. | M | Add-on | Phase 15 |
| FR-152-002 | Sistem harus menerapkan lingkup akses pengguna pada setiap dashboard dan laporan kustom (BR-GEN-01). | M | Add-on | Phase 15 |
| FR-152-003 | Sistem harus mengirim laporan terjadwal melalui email kepada penerima yang berwenang. | S | Add-on | Phase 15 |
| FR-152-004 | Sistem harus menyediakan ekspor data agregat terkontrol untuk alat BI eksternal, dengan pencatatan audit. | S | Add-on | Phase 15 |

| ID | Skenario | Given / When / Then |
|---|---|---|
| AC-152-01 | Lingkup akses dihormati | **Given** Kepala Unit SD membuka dashboard kustom yang dibuat oleh Yayasan Admin<br>**When** dashboard ditampilkan<br>**Then** seluruh angka hanya mencakup unit SD |

#### 153. AI Assistant

| Atribut | Nilai |
|---|---|
| **Portal** | Seluruh portal |
| **Peran** | Seluruh peran (sesuai hak akses) |
| **Scope** | Add-on |
| **Phase** | Phase 15 |
| **Tujuan** | Asisten berbasis AI untuk menjawab pertanyaan, mencari data, dan membantu menyusun konten. |
| **Fitur** | Q&A, Data lookup, Drafting, Help |
| **Sub-fitur** | Tanya jawab panduan produk, pencarian data sesuai hak akses, ringkasan, draf pengumuman atau pesan, konfirmasi sebelum aksi, log |

| ID | Kebutuhan | Prio | Scope | Phase |
|---|---|---|---|---|
| FR-153-001 | Sistem harus menjawab pertanyaan tentang cara memakai SEKOCI berdasarkan panduan produk. | M | Add-on | Phase 15 |
| FR-153-002 | Sistem harus menjawab pertanyaan tentang data hanya dari data yang boleh diakses pengguna tersebut (BR-AI-01). | M | Add-on | Phase 15 |
| FR-153-003 | Sistem harus membantu menyusun draf pengumuman, pesan, atau ringkasan laporan yang dapat diedit pengguna sebelum dipakai. | S | Add-on | Phase 15 |
| FR-153-004 | Sistem harus meminta konfirmasi pengguna sebelum menjalankan aksi apa pun yang mengubah data, dan menjalankannya dengan hak akses pengguna tersebut. | M | Add-on | Phase 15 |
| FR-153-005 | Sistem harus menandai jawaban yang dihasilkan AI, menampilkan sumber data yang dipakai, dan mencatat percakapan untuk audit sesuai kebijakan retensi. | M | Add-on | Phase 15 |

| ID | Skenario | Given / When / Then |
|---|---|---|
| AC-153-01 | Pertanyaan di luar hak akses | **Given** orang tua bertanya tentang tunggakan siswa lain<br>**When** asisten memproses pertanyaan<br>**Then** asisten menolak menampilkan data siswa lain |
| AC-153-02 | Aksi memerlukan konfirmasi | **Given** Finance Admin meminta asisten mengirim pengingat ke orang tua yang menunggak<br>**When** asisten menyiapkan daftar penerima dan isi pesan<br>**Then** pesan belum dikirim sampai Finance Admin mengonfirmasi |

#### 154. Analitik Prediktif

| Atribut | Nilai |
|---|---|
| **Portal** | Admin / Yayasan |
| **Peran** | Kepala Unit; Finance Manager; Pengurus Yayasan; BI Analyst |
| **Scope** | Add-on |
| **Phase** | Phase 15 |
| **Tujuan** | Prediksi sebagai bahan pertimbangan pimpinan. |
| **Fitur** | Risk scoring, Forecasting, Explainability |
| **Sub-fitur** | Risiko tunggakan, risiko ketidakhadiran berulang, proyeksi pendaftar PPDB, proyeksi arus kas, faktor penjelas, akurasi model |

| ID | Kebutuhan | Prio | Scope | Phase |
|---|---|---|---|---|
| FR-154-001 | Sistem harus menghasilkan skor risiko tunggakan per siswa dan risiko ketidakhadiran berulang per siswa, beserta faktor yang paling memengaruhi skor. | M | Add-on | Phase 15 |
| FR-154-002 | Sistem harus memproyeksikan jumlah pendaftar PPDB per gelombang dan arus kas penerimaan per bulan. | S | Add-on | Phase 15 |
| FR-154-003 | Sistem harus menerapkan BR-AI-02 pada seluruh hasil prediksi. | M | Add-on | Phase 15 |
| FR-154-004 | Sistem harus menampilkan akurasi model dari periode sebelumnya dan tanggal pelatihan terakhir. | S | Add-on | Phase 15 |

| ID | Skenario | Given / When / Then |
|---|---|---|
| AC-154-01 | Prediksi sebagai rekomendasi | **Given** siswa memiliki skor risiko tunggakan tinggi<br>**When** Finance Admin membuka daftar risiko<br>**Then** daftar menampilkan skor dan faktor penjelas<br>**And** tidak ada tindakan otomatis terhadap siswa tersebut |

### Grup Menu 32 — 3D Campus

*Phase 16 · Add-on. Tampilan kampus tiga dimensi yang terhubung dengan data operasional.*

#### 155. Peta Kampus 3D

| Atribut | Nilai |
|---|---|
| **Portal** | Web-portal / Admin |
| **Peran** | Seluruh peran; Facility Officer (pengelola model) |
| **Scope** | Add-on |
| **Phase** | Phase 16 |
| **Tujuan** | Model kampus 3D untuk navigasi dan informasi ruang. |
| **Fitur** | 3D campus model, Navigation, Room info |
| **Sub-fitur** | Model gedung dan lantai, navigasi, pencarian ruang, info ruang dari menu 116, pengelolaan model, tampilan di perangkat menengah |

| ID | Kebutuhan | Prio | Scope | Phase |
|---|---|---|---|---|
| FR-155-001 | Sistem harus menampilkan model 3D kampus per unit dengan navigasi antar gedung dan lantai. | M | Add-on | Phase 16 |
| FR-155-002 | Sistem harus menautkan setiap ruang di model ke data ruang menu 116 dan menampilkan informasinya saat dipilih (BR-3D-01). | M | Add-on | Phase 16 |
| FR-155-003 | Sistem harus menyediakan pencarian ruang yang menyorot lokasinya di model. | M | Add-on | Phase 16 |
| FR-155-004 | Sistem harus memungkinkan Facility Officer mengunggah dan memperbarui model 3D beserta pemetaan ruangnya [OQ-39]. | M | Add-on | Phase 16 |

| ID | Skenario | Given / When / Then |
|---|---|---|
| AC-155-01 | Pencarian ruang | **Given** pengunjung mencari "Lab Kimia"<br>**When** hasil dipilih<br>**Then** model berpindah ke lantai dan gedung lab kimia dan ruang tersebut disorot |

#### 156. Peta Kantin & Tenant

| Atribut | Nilai |
|---|---|
| **Portal** | Web-portal |
| **Peran** | Orang tua; Siswa; Pegawai; Canteen Admin |
| **Scope** | Add-on |
| **Phase** | Phase 16 |
| **Tujuan** | Lokasi tenant kantin di model 3D beserta menunya. |
| **Fitur** | Canteen map |
| **Sub-fitur** | Lokasi tenant, status buka, menu dan harga dari menu 124 |

| ID | Kebutuhan | Prio | Scope | Phase |
|---|---|---|---|---|
| FR-156-001 | Sistem harus menampilkan lokasi tenant kantin di model 3D dengan status buka atau tutup. | M | Add-on | Phase 16 |
| FR-156-002 | Sistem harus menampilkan menu dan harga tenant dari menu 124 saat tenant dipilih. | M | Add-on | Phase 16 |

| ID | Skenario | Given / When / Then |
|---|---|---|
| AC-156-01 | Menu dari POS | **Given** harga nasi goreng tenant A diubah di menu 124<br>**When** pengguna memilih tenant A di peta<br>**Then** harga terbaru yang tampil |

#### 157. Peta Lab, Perpustakaan & Ruang Serbaguna

| Atribut | Nilai |
|---|---|
| **Portal** | Web-portal |
| **Peran** | Guru; Siswa; Pegawai |
| **Scope** | Add-on |
| **Phase** | Phase 16 |
| **Tujuan** | Status pemakaian ruang khusus di model 3D. |
| **Fitur** | Room availability map |
| **Sub-fitur** | Status terpakai/kosong dari menu 117, jadwal hari ini, tautan pemesanan |

| ID | Kebutuhan | Prio | Scope | Phase |
|---|---|---|---|---|
| FR-157-001 | Sistem harus menampilkan status terpakai atau kosong untuk lab, perpustakaan, dan ruang serbaguna berdasarkan pemesanan dan jadwal di menu 117. | M | Add-on | Phase 16 |
| FR-157-002 | Sistem harus menampilkan jadwal pemakaian hari ini dan tautan ke pemesanan ruang (menu 117). | S | Add-on | Phase 16 |

| ID | Skenario | Given / When / Then |
|---|---|---|
| AC-157-01 | Status dari pemesanan | **Given** lab komputer dipesan 10.00–12.00<br>**When** pengguna membuka peta pukul 10.30<br>**Then** lab komputer tampil terpakai beserta pemesannya |

#### 158. Facility Digital Twin

| Atribut | Nilai |
|---|---|
| **Portal** | Admin / Fasilitas |
| **Peran** | Facility Officer; Asset Officer; Kepala Unit |
| **Scope** | Add-on |
| **Phase** | Phase 16 |
| **Tujuan** | Tampilan aset dan perawatan di model 3D untuk pengelolaan fasilitas. |
| **Fitur** | Asset overlay, Maintenance overlay, Sensor data |
| **Sub-fitur** | Lokasi aset dari menu 107, perintah kerja aktif dari menu 118, kondisi aset, data sensor bila tersedia |

| ID | Kebutuhan | Prio | Scope | Phase |
|---|---|---|---|---|
| FR-158-001 | Sistem harus menampilkan aset per ruang di model 3D berdasarkan lokasi aset di menu 107. | M | Add-on | Phase 16 |
| FR-158-002 | Sistem harus menyorot ruang yang memiliki perintah kerja aktif di menu 118 beserta statusnya. | M | Add-on | Phase 16 |
| FR-158-003 | Sistem harus menampilkan data sensor ruang (misalnya suhu atau pemakaian listrik) bila sumber datanya tersedia [OQ-42]. | C | Add-on | Phase 16 |

| ID | Skenario | Given / When / Then |
|---|---|---|
| AC-158-01 | Perintah kerja terlihat di model | **Given** ruang 8A memiliki perintah kerja perbaikan AC berstatus diproses<br>**When** Facility Officer membuka digital twin<br>**Then** ruang 8A disorot dengan status perbaikan AC |

### Grup Menu 33 — Aplikasi White-label

*Phase 15 · Add-on. Aplikasi mobile dengan merek yayasan klien.*

#### 159. Aplikasi White-label

| Atribut | Nilai |
|---|---|
| **Portal** | Admin / Platform |
| **Peran** | Platform Admin; Yayasan Admin |
| **Scope** | Add-on |
| **Phase** | Phase 15 |
| **Tujuan** | Aplikasi mobile bermerek yayasan untuk portal orang tua, siswa, dan guru. |
| **Fitur** | Branding, App build, Store publishing, Push notification |
| **Sub-fitur** | Nama aplikasi, ikon, warna, splash screen, domain, portal yang disertakan, build per klien, publikasi ke toko aplikasi, notifikasi push |

| ID | Kebutuhan | Prio | Scope | Phase |
|---|---|---|---|---|
| FR-159-001 | Sistem harus memungkinkan pengaturan merek per klien: nama aplikasi, ikon, warna, splash screen, dan domain. | M | Add-on | Phase 15 |
| FR-159-002 | Sistem harus menghasilkan build aplikasi mobile per klien yang memuat portal orang tua, siswa, dan guru sesuai modul yang aktif [OQ-40]. | M | Add-on | Phase 15 |
| FR-159-003 | Sistem harus mendukung publikasi ke Google Play dan App Store atas nama akun klien atau akun FullCircle sesuai perjanjian. | M | Add-on | Phase 15 |
| FR-159-004 | Sistem harus mengirim notifikasi push melalui aplikasi sebagai kanal tambahan menu 07, dengan menghormati preferensi pengguna (CC-08). | M | Add-on | Phase 15 |

| ID | Skenario | Given / When / Then |
|---|---|---|
| AC-159-01 | Merek klien | **Given** klien mengatur nama aplikasi "Teratai App" dan warna hijau<br>**When** build baru dibuat<br>**Then** aplikasi tampil dengan nama dan warna tersebut tanpa merek SEKOCI di layar utama |
| AC-159-02 | Notifikasi push | **Given** orang tua memasang aplikasi klien dan mengizinkan notifikasi<br>**When** tagihan baru terbit<br>**Then** orang tua menerima notifikasi push |

## 6. Alur Bisnis End-to-End

| Alur | Urutan (nomor menu) | Phase |
|---|---|---|
| Siklus mutu | 142 Standar & target → 143 Bukti → 144 Evaluasi diri → 79 Finalisasi → 144 Rencana tindak lanjut → laporan akreditasi | Phase 13 |
| Sinkronisasi Dapodik | 03/39/83 Data SEKOCI → 145 Pemetaan → perbandingan → keputusan operator → data pembaruan → log | Phase 13 |
| Pemasaran PPDB | 149 Landing page → 16 Pendaftaran (dengan kode referral) → 15 Daftar ulang → 09 Reward referral → 147 Kampanye lanjutan | Phase 15 |
| Autodebit | 151 Mandat → 19 Tagihan jatuh tempo → 151 Debit → 29 Transaksi → 21 Alokasi | Phase 15 |
| Asisten AI | Pertanyaan → pemeriksaan hak akses → jawaban dengan sumber → konfirmasi bila ada aksi → log | Phase 15 |

## 7. Integrasi

| ID | Integrasi | Kebutuhan | Phase |
|---|---|---|---|
| IF-ADV-01 | Dapodik | Sistem harus mengambil dan menyiapkan data melalui mekanisme yang diizinkan [OQ-36]. | Phase 13 |
| IF-ADV-02 | WhatsApp Business API | Sistem harus mengirim kampanye dengan template yang disetujui melalui penyedia resmi [OQ-38]. | Phase 15 |
| IF-ADV-03 | Payment gateway (beberapa) | Sistem harus memakai adapter per provider untuk transaksi, webhook, settlement, dan mandat autodebit. | Phase 15 |
| IF-ADV-04 | Penyedia model AI | Sistem harus memanggil model AI melalui API dengan data minimum yang diperlukan, sesuai kebijakan pemrosesan data [OQ-35]. | Phase 15 |
| IF-ADV-05 | Toko aplikasi & layanan notifikasi push | Sistem harus mendukung build dan publikasi aplikasi serta pengiriman notifikasi push [OQ-40]. | Phase 15 |
| IF-ADV-06 | Alat BI eksternal | Sistem harus menyediakan ekspor data agregat terkontrol. | Phase 15 |

## 8. Kebutuhan Non-Fungsional Khusus

| ID | Area | Kebutuhan | Prio | Verif. |
|---|---|---|---|---|
| NFR-ADV-01 | Privasi AI | Data pribadi yang dikirim ke penyedia model AI diminimalkan dan pemrosesannya mengikuti ketentuan transfer data UU PDP. | M | I |
| NFR-ADV-02 | Kinerja AI | Jawaban AI Assistant mulai tampil ≤ [5] detik untuk pertanyaan umum. | S | T |
| NFR-ADV-03 | Kinerja 3D | Model 3D kampus dapat dibuka di ponsel kelas menengah dalam ≤ [8] detik melalui koneksi 4G. | S | T |
| NFR-ADV-04 | Kinerja BI | Dashboard kustom dengan data satu tahun ajaran tampil ≤ [5] detik. | S | T |
| NFR-ADV-05 | Kepatuhan toko aplikasi | Aplikasi white-label memenuhi kebijakan Google Play dan App Store yang berlaku, termasuk kebijakan privasi per klien. | M | I |
| NFR-ADV-06 | Audit komunikasi | Pesan percakapan dan kampanye disimpan beserta pengirim, penerima, dan waktu sesuai kebijakan retensi. | M | I |

## 9. Pertanyaan Terbuka Bagian 4

Melanjutkan penomoran Bagian 3.

| ID | Pertanyaan | Terdampak | Prioritas |
|---|---|---|---|
| OQ-35 | Penyedia model AI, lokasi pemrosesan data, dan kesesuaiannya dengan ketentuan transfer data UU PDP | 153, 154 | Tinggi |
| OQ-36 | Mekanisme integrasi Dapodik yang diizinkan (web service lokal, file, atau lainnya) | 145 | Tinggi |
| OQ-37 | Instrumen akreditasi dan kerangka mutu yang dimuat sebagai default | 142 | Sedang |
| OQ-38 | Penyedia WhatsApp Business API dan model biaya pesan untuk kampanye | 147 | Sedang |
| OQ-39 | Sumber model 3D (gambar teknik, foto, atau pemodelan manual) dan proses pembaruannya | 155 | Sedang |
| OQ-40 | Teknologi aplikasi white-label (pembungkus PWA atau native) dan kepemilikan akun toko aplikasi | 159 | Tinggi |
| OQ-41 | Daftar payment provider dan bank yang didukung untuk autodebit | 150, 151 | Sedang |
| OQ-42 | Kebutuhan dan sumber data sensor untuk digital twin | 158 | Rendah |
| OQ-43 | Mekanisme persetujuan (opt-in) default untuk kampanye promosi | 147 | Sedang |
| OQ-44 | Proses tinjauan etika dan keadilan untuk model prediktif sebelum dipakai | 154 | Sedang |

## 10. Persetujuan Dokumen

| Peran | Nama | Tanggal | Tanda tangan |
|---|---|---|---|
| Product Owner | David Pambudi | | |
| Project Manager | David Pambudi | | |
| Tech Lead | [Nama] | | |
| Legal / Data Protection | [Nama] | | |

## Lampiran — Batas Tanggung Jawab Antar-Menu (Domain Lanjutan)

| Topik | Pemilik kebutuhan | Menu/aturan lain hanya … |
|---|---|---|
| Notifikasi transaksional dan pengumuman | Menu 07 | menu 147 untuk kampanye promosi tersegmentasi |
| Komunikasi guru | Menu 75 (pengumuman satu arah) | menu 148 untuk percakapan dua arah |
| Formulir pendaftaran PPDB | Menu 16 | menu 149 membuat halaman pemasaran dan mengarahkan ke menu 16 |
| Reward referral | Menu 09 (mesin promo) | menu 149 menentukan kapan reward berhak diterbitkan |
| Metode pembayaran inti | Menu 28 | menu 150 mengatur provider; menu 151 autodebit dan gerai retail |
| Dashboard standar | Menu 08, 17, 26, 32, 51–53 | menu 152 untuk dashboard dan laporan kustom |
| Data ruang, aset, tenant, pemesanan, perawatan | Menu 116, 107, 123–124, 117, 118 | menu 155–158 hanya menampilkan dalam model 3D |
| Indikator mutu | Metric dictionary (Bagian 1 bab 17) | menu 142 menautkan indikator ke metrik |
| Data siswa, rombel, PTK | Menu 03, 39, 83 | menu 145 hanya membandingkan dan menyiapkan data Dapodik |
| Kanal notifikasi push | Menu 07 | menu 159 menyediakan aplikasinya |
