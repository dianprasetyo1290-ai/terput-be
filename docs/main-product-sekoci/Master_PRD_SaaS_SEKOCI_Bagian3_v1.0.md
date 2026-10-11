# Master PRD — SEKOCI Integrated School Operating Platform (SaaS)

**Bagian 3 dari 4 · Operasional sekolah: Inventory & Aset sampai PKL (menu 102–141)**

Kebutuhan fungsional, aturan bisnis, dan kriteria penerimaan untuk operasional sekolah dan layanan siswa.

| Atribut | Nilai |
|---|---|
| Document ID | SEKOCI-MPRD-001 |
| Versi | 1.0 — Bagian 3 (draft) |
| Tanggal | 10/10/2026 |
| Status | Draft, menunggu review |
| Produk | SEKOCI Integrated School Operating Platform (SaaS) |
| Product Owner | David Pambudi |
| Project Manager | David Pambudi |
| Klasifikasi | Rahasia — internal tim |

## Daftar Isi

- [Kontrol Dokumen](#kontrol-dokumen)
  - [Riwayat versi](#riwayat-versi)
  - [Hubungan dengan Bagian 1 dan 2](#hubungan-dengan-bagian-1-dan-2)
- [1. Ringkasan Bagian 3](#1-ringkasan-bagian-3)
- [2. Peran Tambahan](#2-peran-tambahan)
- [3. Entitas Data & System of Record](#3-entitas-data--system-of-record)
- [4. Aturan Bisnis Operasional](#4-aturan-bisnis-operasional)
- [5. Kebutuhan Fungsional per Grup Menu & Menu](#5-kebutuhan-fungsional-per-grup-menu--menu)
  - [Grup Menu 16 — Inventory, Pengadaan & Aset](#grup-menu-16--inventory-pengadaan--aset)
  - [Grup Menu 17 — Ekstensi Akuntansi](#grup-menu-17--ekstensi-akuntansi)
  - [Grup Menu 18 — Perpustakaan](#grup-menu-18--perpustakaan)
  - [Grup Menu 19 — Fasilitas](#grup-menu-19--fasilitas)
  - [Grup Menu 20 — Transportasi](#grup-menu-20--transportasi)
  - [Grup Menu 21 — Kantin & Dompet Siswa](#grup-menu-21--kantin--dompet-siswa)
  - [Grup Menu 22 — Kesehatan Sekolah](#grup-menu-22--kesehatan-sekolah)
  - [Grup Menu 23 — Konseling & Kesiswaan](#grup-menu-23--konseling--kesiswaan)
  - [Grup Menu 24 — Event & Ekstrakurikuler](#grup-menu-24--event--ekstrakurikuler)
  - [Grup Menu 25 — Service Desk](#grup-menu-25--service-desk)
  - [Grup Menu 26 — PKL](#grup-menu-26--pkl)
- [6. Alur Bisnis End-to-End](#6-alur-bisnis-end-to-end)
- [7. Integrasi](#7-integrasi)
- [8. Kebutuhan Non-Fungsional Khusus](#8-kebutuhan-non-fungsional-khusus)
- [9. Pertanyaan Terbuka Bagian 3](#9-pertanyaan-terbuka-bagian-3)
- [10. Persetujuan Dokumen](#10-persetujuan-dokumen)
- [Lampiran — Batas Tanggung Jawab Antar-Menu (Operasional)](#lampiran--batas-tanggung-jawab-antar-menu-operasional)

## Kontrol Dokumen

### Riwayat versi

| Versi | Tanggal | Pemilik | Perubahan |
|---|---|---|---|
| 1.0 — Bagian 3 | 10/10/2026 | PT FullCircle Solusi Indonesia | Kebutuhan grup menu 16–26 (menu 102–141) |

### Hubungan dengan Bagian 1 dan 2

Kerangka umum di Bagian 1 berlaku penuh, termasuk kebutuhan lintas modul, aturan bisnis umum, Approval Center (menu 79), Integration Exception Queue (menu 80), NFR, roadmap, dan release gate. Data pegawai mengacu ke Master Pegawai di Bagian 2 (menu 83).

## 1. Ringkasan Bagian 3

| Grup | Menu | Phase |
|---|---|---|
| 16. Inventory, Pengadaan & Aset | 102–108 | Phase 11 |
| 17. Ekstensi Akuntansi | 109–111 | Phase 11 |
| 18. Perpustakaan | 112–115 | Phase 12 |
| 19. Fasilitas | 116–118 | Phase 12 |
| 20. Transportasi | 119–122 | Phase 12 |
| 21. Kantin & Dompet Siswa | 123–126 | Phase 10 |
| 22. Kesehatan Sekolah | 127–129 | Phase 13 |
| 23. Konseling & Kesiswaan | 130–132 | Phase 13 |
| 24. Event & Ekstrakurikuler | 133–136 | Phase 10 |
| 25. Service Desk | 137–138 | Phase 10 |
| 26. PKL | 139–141 | Phase 13 |

Seluruh menu berlabel Add-on. Bagian ini memuat 145 kebutuhan fungsional dan 47 skenario Given/When/Then.

## 2. Peran Tambahan

| Peran | Tujuan | Lingkup data |
|---|---|---|
| Inventory Officer | Stok dan gudang | Unit yang ditugaskan |
| Procurement Officer | Purchase order dan vendor | Unit atau yayasan |
| Asset Officer | Register, mutasi, dan pelepasan aset | Unit atau yayasan |
| Pustakawan | Koleksi, anggota, sirkulasi | Unit yang ditugaskan |
| Facility Officer / Teknisi | Ruang, pemesanan, perawatan | Unit yang ditugaskan |
| Transport Officer / Pendamping / Pengemudi | Rute, kendaraan, perjalanan | Rute yang ditugaskan |
| Canteen Admin / Operator tenant | Tenant, POS, penjualan | Tenant sendiri (operator) |
| Petugas UKS | Data kesehatan dan kunjungan | Unit yang ditugaskan |
| Guru BK / Konselor | Konseling dan rujukan | Siswa yang didampingi |
| Kesiswaan Officer | Disiplin dan prestasi | Unit yang ditugaskan |
| Event Coordinator / Pembina ekskul | Event dan ekstrakurikuler | Kegiatan yang dikelola |
| Service Desk Agent / Manager | Tiket dan SLA | Kategori yang ditugaskan |
| Koordinator PKL / Pembimbing sekolah | Mitra dan penempatan PKL | Unit SMK |
| Pembimbing industri (eksternal) | Absensi dan penilaian PKL | Siswa yang dibimbing |

## 3. Entitas Data & System of Record

| Entitas | Sumber data utama | Catatan |
|---|---|---|
| Item, Warehouse, Stock Movement, Stock Count | SEKOCI Inventory | Mutasi stok tidak dihapus |
| Purchase Request, Purchase Order, Goods Receipt, Vendor | SEKOCI Procurement | Vendor disinkronkan ke Accounting Engine |
| Asset (register fisik), Asset Movement | SEKOCI Asset | Nilai buku dan penyusutan dari Accounting Engine |
| Budget, Budget Line, Commitment | SEKOCI Budget | Realisasi dari jurnal posted di Accounting Engine |
| Vendor Invoice, Withholding Tax | SEKOCI Finance (ekstensi) | Buku besar utang di Accounting Engine |
| Catalog, Copy, Member, Loan, Fine | SEKOCI Library | Denda dapat ditagih melalui menu 19 |
| Room, Booking, Work Order | SEKOCI Facility | Master ruang dipakai semua modul |
| Route, Stop, Vehicle, Allocation, Trip Log | SEKOCI Transport | Tagihan melalui menu 19 |
| Tenant, Menu Item, POS Sale, Wallet, Wallet Transaction | SEKOCI Canteen & Wallet | Saldo dompet adalah dana titipan |
| Health Profile, Clinic Visit, Immunization | SEKOCI Health | Data sensitif |
| Counseling Record, Discipline Record, Referral | SEKOCI Student Affairs | Data rahasia |
| Event, Registration, Consent, Extracurricular, Achievement | SEKOCI Activities | Prestasi terverifikasi menjadi sumber dashboard |
| Ticket, SLA Policy | SEKOCI Service Desk | Menu 81 memakai mesin tiket ini bila aktif |
| Partner, Placement, PKL Journal, PKL Assessment | SEKOCI PKL | — |

## 4. Aturan Bisnis Operasional

| ID | Aturan |
|---|---|
| BR-INV-01 | Stok tidak boleh negatif; setiap perubahan stok tercatat sebagai mutasi dengan referensi dokumen dan tidak dapat dihapus. |
| BR-PRC-01 | Permintaan pembelian dan purchase order diproses melalui workflow approval dengan tahap berdasarkan nilai dan unit. |
| BR-PRC-02 | Tagihan vendor hanya dapat dibayar bila cocok dengan purchase order dan penerimaan dalam toleransi [2%]; selisih di luar toleransi memerlukan penyelesaian dan approval. |
| BR-AST-01 | Penyusutan dan nilai buku aset dihitung oleh Accounting Engine; SEKOCI menyimpan data fisik aset dan mengirim perolehan, mutasi, dan pelepasan. |
| BR-BUD-01 | Realisasi anggaran dihitung dari jurnal yang sudah diposting; komitmen dari PR/PO yang disetujui ditampilkan terpisah. |
| BR-BUD-02 | Permintaan yang melebihi sisa anggaran pos memerlukan tahap approval tambahan atau ditolak, sesuai pengaturan klien. |
| BR-LIB-01 | Peminjaman dibatasi jumlah dan lama pinjam per kategori anggota; anggota dengan pinjaman terlambat tidak dapat meminjam lagi sampai diselesaikan. |
| BR-LIB-02 | Denda dapat dibayar di perpustakaan atau ditagihkan melalui menu 19, sesuai pengaturan klien. |
| BR-FAC-01 | Satu ruang tidak dapat dipesan untuk waktu yang tumpang tindih atau untuk peserta di atas kapasitasnya. |
| BR-FAC-02 | Master ruang (menu 116) menjadi satu-satunya sumber data ruang untuk semua modul. |
| BR-TRS-01 | Jumlah siswa yang dialokasikan ke satu perjalanan tidak boleh melebihi kapasitas kendaraan. |
| BR-WAL-01 | Transaksi uang saku hanya disetujui bila saldo mencukupi dan tidak melebihi batas harian yang diatur orang tua. |
| BR-WAL-02 | Saldo uang saku dan tabungan dicatat sebagai dana titipan (kewajiban), bukan pendapatan sekolah; top-up hanya melalui payment layer atau kasir. |
| BR-HLT-01 | Data kesehatan lengkap hanya dapat dilihat petugas UKS dan orang tua; wali kelas hanya melihat peringatan medis (misalnya alergi) dan kontak darurat. |
| BR-CNS-01 | Catatan konseling dan rujukan hanya dapat dilihat konselor yang menangani dan peran lain yang ditetapkan secara eksplisit oleh kebijakan klien. |
| BR-DIS-01 | Ketika akumulasi poin pelanggaran mencapai ambang, sistem membuat tindakan pembinaan yang ditetapkan dan memberi tahu orang tua. |
| BR-EVT-01 | Event yang memerlukan persetujuan orang tua tidak menerbitkan tagihan maupun menerima kehadiran siswa sebelum persetujuan diberikan. |
| BR-SD-01 | Bila Service Desk aktif, permintaan layanan orang tua (menu 81) diproses sebagai tiket di menu 137 dan mengikuti SLA menu 138. |
| BR-PKL-01 | Penempatan PKL tidak boleh melebihi kuota mitra per periode dan jurusan, dan hanya pada mitra dengan MoU yang berlaku. |

## 5. Kebutuhan Fungsional per Grup Menu & Menu

Kebutuhan lintas modul (Bagian 1, bab 10) dan aturan bisnis (Bagian 1, bab 9 dan bab 4 di atas) berlaku tanpa ditulis ulang.

### Grup Menu 16 — Inventory, Pengadaan & Aset

*Phase 11 · Add-on. Stok barang, pengadaan dari permintaan sampai penerimaan, vendor, dan aset tetap.*

#### 102. Master Barang & Stok

| Atribut | Nilai |
|---|---|
| **Portal** | Admin / Inventory |
| **Peran** | Inventory Officer; Kepala Unit |
| **Scope** | Add-on |
| **Phase** | Phase 11 |
| **Tujuan** | Data barang habis pakai dan persediaan beserta stok per gudang dan unit. |
| **Fitur** | Item master, Warehouse, Stock movement |
| **Sub-fitur** | Kode, nama, kategori, satuan, gudang per unit, stok minimum, barang masuk/keluar/transfer, kartu stok |

| ID | Kebutuhan | Prio | Scope | Phase |
|---|---|---|---|---|
| FR-102-001 | Sistem harus mengelola data barang dengan kode, nama, kategori, satuan, dan stok minimum. | M | Add-on | Phase 11 |
| FR-102-002 | Sistem harus mengelola gudang atau lokasi penyimpanan per unit. | M | Add-on | Phase 11 |
| FR-102-003 | Sistem harus mencatat barang masuk, barang keluar, dan transfer antar gudang dengan referensi dokumen, serta menerapkan BR-INV-01. | M | Add-on | Phase 11 |
| FR-102-004 | Sistem harus menampilkan kartu stok per barang dan gudang. | M | Add-on | Phase 11 |
| FR-102-005 | Sistem harus memberi tahu Inventory Officer ketika stok berada di bawah stok minimum. | S | Add-on | Phase 11 |

| ID | Skenario | Given / When / Then |
|---|---|---|
| AC-102-01 | Stok tidak boleh negatif | **Given** stok kertas A4 di gudang SD 5 rim<br>**When** petugas mencatat pengeluaran 8 rim<br>**Then** transaksi ditolak dengan pesan stok tidak mencukupi |
| AC-102-02 | Peringatan stok minimum | **Given** stok minimum spidol 20 dan stok tersisa 25<br>**When** pengeluaran 10 spidol dicatat<br>**Then** Inventory Officer menerima peringatan stok di bawah minimum |

#### 103. Stock Opname

| Atribut | Nilai |
|---|---|
| **Portal** | Admin / Inventory |
| **Peran** | Inventory Officer; Finance Manager |
| **Scope** | Add-on |
| **Phase** | Phase 11 |
| **Tujuan** | Penghitungan fisik stok dan penyesuaian selisih. |
| **Fitur** | Physical count, Adjustment |
| **Sub-fitur** | Jadwal opname, lembar hitung, input hasil, selisih, penyesuaian dengan approval, berita acara |

| ID | Kebutuhan | Prio | Scope | Phase |
|---|---|---|---|---|
| FR-103-001 | Sistem harus membuat sesi opname per gudang yang membekukan stok sistem sebagai acuan pada waktu mulai. | M | Add-on | Phase 11 |
| FR-103-002 | Sistem harus menerima hasil hitung fisik dan menghitung selisih per barang. | M | Add-on | Phase 11 |
| FR-103-003 | Sistem harus memproses penyesuaian selisih melalui workflow approval (menu 79) sebelum stok diperbarui. | M | Add-on | Phase 11 |
| FR-103-004 | Sistem harus menghasilkan berita acara opname dalam PDF. | S | Add-on | Phase 11 |

| ID | Skenario | Given / When / Then |
|---|---|---|
| AC-103-01 | Selisih opname | **Given** stok sistem tinta printer 12 dan hasil hitung 10<br>**When** hasil hitung disimpan<br>**Then** selisih -2 ditampilkan dan stok belum berubah sebelum penyesuaian disetujui |

#### 104. Purchase Request

| Atribut | Nilai |
|---|---|
| **Portal** | Admin / Pengadaan |
| **Peran** | Pemohon (staf/guru); Kepala Unit; Procurement Officer |
| **Scope** | Add-on |
| **Phase** | Phase 11 |
| **Tujuan** | Permintaan pembelian dari unit sebelum pengadaan. |
| **Fitur** | Purchase request, Budget check |
| **Sub-fitur** | Barang/jasa, jumlah, perkiraan harga, alasan, unit, pos anggaran, cek anggaran, approval, status |

| ID | Kebutuhan | Prio | Scope | Phase |
|---|---|---|---|---|
| FR-104-001 | Sistem harus memungkinkan pengguna mengajukan permintaan pembelian barang atau jasa dengan jumlah, perkiraan harga, alasan, unit, dan pos anggaran. | M | Add-on | Phase 11 |
| FR-104-002 | Sistem harus membandingkan nilai permintaan dengan sisa anggaran pos terkait (menu 109) dan menerapkan BR-BUD-02. | M | Add-on | Phase 11 |
| FR-104-003 | Sistem harus memproses permintaan melalui workflow approval (menu 79) sesuai nilai dan unit (BR-PRC-01). | M | Add-on | Phase 11 |
| FR-104-004 | Sistem harus menampilkan status permintaan sampai menjadi purchase order atau dibatalkan. | S | Add-on | Phase 11 |

| ID | Skenario | Given / When / Then |
|---|---|---|
| AC-104-01 | Melebihi sisa anggaran | **Given** sisa anggaran pos ATK unit SMP Rp2.000.000<br>**When** guru mengajukan pembelian Rp2.500.000<br>**Then** permintaan ditandai melebihi anggaran dan memerlukan approval tambahan sesuai BR-BUD-02 |

#### 105. Purchase Order & Penerimaan

| Atribut | Nilai |
|---|---|
| **Portal** | Admin / Pengadaan |
| **Peran** | Procurement Officer; Inventory Officer; Finance Manager |
| **Scope** | Add-on |
| **Phase** | Phase 11 |
| **Tujuan** | Pemesanan ke vendor dan penerimaan barang. |
| **Fitur** | Purchase order, Goods receipt |
| **Sub-fitur** | PO dari PR, vendor, harga, termin, approval, pengiriman PO, penerimaan sebagian/penuh, retur, stok/aset masuk |

| ID | Kebutuhan | Prio | Scope | Phase |
|---|---|---|---|---|
| FR-105-001 | Sistem harus membuat purchase order dari satu atau lebih permintaan yang disetujui, dengan vendor (menu 106), harga, dan termin pembayaran. | M | Add-on | Phase 11 |
| FR-105-002 | Sistem harus memproses purchase order melalui workflow approval (menu 79) dan mengirim PO kepada vendor dalam PDF. | M | Add-on | Phase 11 |
| FR-105-003 | Sistem harus mencatat penerimaan barang sebagian atau penuh terhadap PO, termasuk barang yang ditolak atau diretur. | M | Add-on | Phase 11 |
| FR-105-004 | Sistem harus menambah stok (menu 102) untuk barang persediaan atau membuat calon aset (menu 107) untuk barang yang dikategorikan aset saat penerimaan disimpan. | M | Add-on | Phase 11 |

| ID | Skenario | Given / When / Then |
|---|---|---|
| AC-105-01 | Penerimaan sebagian | **Given** PO berisi 10 unit kursi<br>**When** gudang menerima 6 unit<br>**Then** status PO menjadi diterima sebagian dengan sisa 4 unit |
| AC-105-02 | Barang aset | **Given** laptop dikategorikan aset<br>**When** penerimaan 2 laptop disimpan<br>**Then** 2 calon aset terbentuk di menu 107 |

#### 106. Vendor

| Atribut | Nilai |
|---|---|
| **Portal** | Admin / Pengadaan |
| **Peran** | Procurement Officer; Finance Admin |
| **Scope** | Add-on |
| **Phase** | Phase 11 |
| **Tujuan** | Data induk vendor dan evaluasinya. |
| **Fitur** | Vendor master, Vendor evaluation |
| **Sub-fitur** | Identitas, NPWP, rekening, kontak, kategori, dokumen legal, status, evaluasi kinerja, duplikasi |

| ID | Kebutuhan | Prio | Scope | Phase |
|---|---|---|---|---|
| FR-106-001 | Sistem harus mengelola data vendor: nama, NPWP, rekening, kontak, kategori barang/jasa, dan dokumen legal. | M | Add-on | Phase 11 |
| FR-106-002 | Sistem harus mendeteksi kemungkinan duplikat vendor berdasarkan NPWP dan rekening. | S | Add-on | Phase 11 |
| FR-106-003 | Sistem harus memproses perubahan rekening vendor melalui workflow approval (menu 79). | M | Add-on | Phase 11 |
| FR-106-004 | Sistem harus mencatat evaluasi kinerja vendor (ketepatan waktu, kualitas, harga) setelah penerimaan. | S | Add-on | Phase 11 |
| FR-106-005 | Sistem harus menyinkronkan data vendor ke Accounting Engine untuk pencatatan utang. | M | Add-on | Phase 11 |

| ID | Skenario | Given / When / Then |
|---|---|---|
| AC-106-01 | Perubahan rekening vendor | **Given** rekening vendor "CV Maju" akan diubah<br>**When** Procurement Officer menyimpan perubahan<br>**Then** rekening baru belum dipakai untuk pembayaran sebelum approval selesai |

#### 107. Register Aset

| Atribut | Nilai |
|---|---|
| **Portal** | Admin / Aset |
| **Peran** | Asset Officer; Kepala Unit |
| **Scope** | Add-on |
| **Phase** | Phase 11 |
| **Tujuan** | Daftar aset tetap secara fisik: lokasi, kondisi, dan penanggung jawab. |
| **Fitur** | Asset register, Tagging, Custody |
| **Sub-fitur** | Kode aset, kategori, nilai perolehan, tanggal perolehan, unit, lokasi/ruang, kondisi, penanggung jawab, label QR, foto |

| ID | Kebutuhan | Prio | Scope | Phase |
|---|---|---|---|---|
| FR-107-001 | Sistem harus mengelola aset dengan kode, kategori, nilai dan tanggal perolehan, unit, lokasi (ruang dari menu 116), kondisi, dan penanggung jawab. | M | Add-on | Phase 11 |
| FR-107-002 | Sistem harus mengaktifkan calon aset dari penerimaan barang (menu 105) setelah data aset dilengkapi. | M | Add-on | Phase 11 |
| FR-107-003 | Sistem harus membuat label QR per aset; memindai label menampilkan detail aset sesuai hak akses. | S | Add-on | Phase 11 |
| FR-107-004 | Sistem harus mengirim data perolehan aset ke Accounting Engine untuk pencatatan dan penyusutan (BR-AST-01). | M | Add-on | Phase 11 |

| ID | Skenario | Given / When / Then |
|---|---|---|
| AC-107-01 | Aktivasi dari penerimaan | **Given** calon aset laptop dari penerimaan PO-012<br>**When** Asset Officer melengkapi lokasi dan penanggung jawab<br>**Then** aset aktif dan data perolehannya terkirim ke Accounting Engine |
| AC-107-02 | Pindai label | **Given** label QR proyektor ditempel di ruang 7A<br>**When** petugas memindai label<br>**Then** detail proyektor, lokasi, dan kondisinya ditampilkan |

#### 108. Penyusutan & Mutasi Aset

| Atribut | Nilai |
|---|---|
| **Portal** | Admin / Aset / Accounting |
| **Peran** | Asset Officer; Accounting; Finance Manager |
| **Scope** | Add-on |
| **Phase** | Phase 11 |
| **Tujuan** | Perpindahan, pemeliharaan status, dan pelepasan aset. Perhitungan penyusutan dilakukan Accounting Engine. |
| **Fitur** | Asset transfer, Disposal, Depreciation view |
| **Sub-fitur** | Mutasi lokasi/unit/penanggung jawab, perubahan kondisi, pelepasan (jual, hibah, rusak), approval, tampilan penyusutan |

| ID | Kebutuhan | Prio | Scope | Phase |
|---|---|---|---|---|
| FR-108-001 | Sistem harus mencatat mutasi aset antar lokasi, unit, atau penanggung jawab dengan tanggal efektif. | M | Add-on | Phase 11 |
| FR-108-002 | Sistem harus memproses pelepasan aset (dijual, dihibahkan, rusak, hilang) melalui workflow approval (menu 79) dan mengirimnya ke Accounting Engine. | M | Add-on | Phase 11 |
| FR-108-003 | Sistem harus menampilkan nilai buku dan akumulasi penyusutan per aset dari Accounting Engine (BR-AST-01). | M | Add-on | Phase 11 |
| FR-108-004 | Sistem harus mengirim mutasi antar unit ke Accounting Engine sebagai perpindahan antar entitas. | S | Add-on | Phase 11 |

| ID | Skenario | Given / When / Then |
|---|---|---|
| AC-108-01 | Pelepasan aset rusak | **Given** laptop berstatus rusak berat<br>**When** Asset Officer mengajukan pelepasan<br>**Then** aset tetap aktif sampai approval selesai, lalu berstatus dilepas dan transaksi pelepasan terkirim ke Accounting Engine |

### Grup Menu 17 — Ekstensi Akuntansi

*Phase 11 · Add-on. Anggaran, pajak non-gaji, dan utang usaha lanjutan di atas Accounting Engine.*

#### 109. Anggaran (RAPBS) & Realisasi

| Atribut | Nilai |
|---|---|
| **Portal** | Admin / Finance / Yayasan |
| **Peran** | Finance Manager; Kepala Unit; Pengurus Yayasan |
| **Scope** | Add-on |
| **Phase** | Phase 11 |
| **Tujuan** | Penyusunan anggaran per unit dan pemantauan realisasinya. |
| **Fitur** | Budget planning, Budget approval, Budget vs actual |
| **Sub-fitur** | Tahun anggaran, pos per akun dan unit, versi, revisi, approval, realisasi dari Accounting Engine, komitmen dari PR/PO, peringatan |

| ID | Kebutuhan | Prio | Scope | Phase |
|---|---|---|---|---|
| FR-109-001 | Sistem harus mengelola anggaran per tahun anggaran, unit, dan pos (dipetakan ke akun COA), dengan versi dan revisi. | M | Add-on | Phase 11 |
| FR-109-002 | Sistem harus memproses penetapan dan revisi anggaran melalui workflow approval (menu 79). | M | Add-on | Phase 11 |
| FR-109-003 | Sistem harus menampilkan realisasi dari jurnal yang sudah diposting di Accounting Engine dan komitmen dari PR/PO yang disetujui (BR-BUD-01). | M | Add-on | Phase 11 |
| FR-109-004 | Sistem harus menampilkan persentase serapan per pos dan memberi peringatan ketika serapan melewati [80%]. | S | Add-on | Phase 11 |

| ID | Skenario | Given / When / Then |
|---|---|---|
| AC-109-01 | Realisasi dari jurnal posted | **Given** anggaran pos listrik unit SD Rp24.000.000 dan jurnal posted Januari–Juni Rp11.000.000<br>**When** Kepala Unit membuka laporan anggaran<br>**Then** realisasi Rp11.000.000 dan serapan 45,8% ditampilkan |
| AC-109-02 | Komitmen dihitung terpisah | **Given** PO perbaikan AC Rp3.000.000 sudah disetujui tetapi belum ditagih vendor<br>**When** laporan dibuka<br>**Then** Rp3.000.000 tampil sebagai komitmen, bukan realisasi |

#### 110. Pajak & e-Faktur

| Atribut | Nilai |
|---|---|
| **Portal** | Admin / Finance |
| **Peran** | Finance Admin; Finance Manager |
| **Scope** | Add-on |
| **Phase** | Phase 11 |
| **Tujuan** | Pemotongan dan pelaporan pajak atas transaksi non-gaji. PPh 21 pegawai ada di menu 97. |
| **Fitur** | Withholding tax, Tax invoice, Tax reporting |
| **Sub-fitur** | PPh 23 dan PPh 4(2) atas jasa/sewa vendor, bukti potong, PPN masukan, e-Faktur bila klien PKP, rekap masa |

| ID | Kebutuhan | Prio | Scope | Phase |
|---|---|---|---|---|
| FR-110-001 | Sistem harus menghitung pemotongan PPh atas transaksi vendor sesuai jenis transaksi yang dikonfigurasi (misalnya PPh 23 untuk jasa, PPh 4(2) untuk sewa) [OQ-28]. | M | Add-on | Phase 11 |
| FR-110-002 | Sistem harus menerbitkan bukti potong dan menyiapkan data pelaporan dalam format DJP yang berlaku. | M | Add-on | Phase 11 |
| FR-110-003 | Sistem harus mencatat PPN masukan atas pembelian dan, bila klien berstatus PKP, menyiapkan data faktur pajak keluaran. | S | Add-on | Phase 11 |
| FR-110-004 | Sistem harus merekap pajak per masa dan per unit untuk pelaporan. | M | Add-on | Phase 11 |

| ID | Skenario | Given / When / Then |
|---|---|---|
| AC-110-01 | PPh atas jasa vendor | **Given** tarif PPh jasa yang dikonfigurasi 2% dan tagihan jasa perbaikan Rp5.000.000<br>**When** tagihan vendor dicatat<br>**Then** PPh dipotong Rp100.000 dan yang dibayarkan ke vendor Rp4.900.000 |

#### 111. Utang Usaha Lanjutan

| Atribut | Nilai |
|---|---|
| **Portal** | Admin / Finance |
| **Peran** | Finance Admin; Finance Manager |
| **Scope** | Add-on |
| **Phase** | Phase 11 |
| **Tujuan** | Pencatatan tagihan vendor, pencocokan dengan PO dan penerimaan, serta penjadwalan pembayaran. Buku besar utang ada di Accounting Engine. |
| **Fitur** | Vendor invoice, 3-way matching, Payment scheduling, AP aging |
| **Sub-fitur** | Tagihan vendor, pencocokan PO–penerimaan–tagihan, toleransi, jadwal bayar, approval, aging, posting |

| ID | Kebutuhan | Prio | Scope | Phase |
|---|---|---|---|---|
| FR-111-001 | Sistem harus mencatat tagihan vendor dan mencocokkannya dengan PO dan penerimaan barang (BR-PRC-02). | M | Add-on | Phase 11 |
| FR-111-002 | Sistem harus menjadwalkan pembayaran sesuai termin dan memprosesnya melalui workflow approval (menu 79). | M | Add-on | Phase 11 |
| FR-111-003 | Sistem harus mengirim utang dan pembayarannya ke Accounting Engine secara idempoten (BR-INT-01). | M | Add-on | Phase 11 |
| FR-111-004 | Sistem harus menampilkan aging utang per vendor dan unit. | S | Add-on | Phase 11 |

| ID | Skenario | Given / When / Then |
|---|---|---|
| AC-111-01 | Tagihan melebihi toleransi | **Given** PO dan penerimaan senilai Rp10.000.000, toleransi 2%<br>**When** vendor menagih Rp10.500.000<br>**Then** tagihan ditandai tidak cocok dan tidak dapat dijadwalkan bayar sebelum diselesaikan |

### Grup Menu 18 — Perpustakaan

*Phase 12 · Add-on. Koleksi, anggota, sirkulasi, dan laporan perpustakaan sekolah.*

#### 112. Katalog Perpustakaan

| Atribut | Nilai |
|---|---|
| **Portal** | Admin / Perpustakaan / Portal |
| **Peran** | Pustakawan; Siswa; Guru |
| **Scope** | Add-on |
| **Phase** | Phase 12 |
| **Tujuan** | Koleksi perpustakaan dan pencariannya. |
| **Fitur** | Catalog, Copies, OPAC |
| **Sub-fitur** | Judul, pengarang, penerbit, ISBN, kategori, eksemplar, barcode, lokasi rak, status, pencarian online |

| ID | Kebutuhan | Prio | Scope | Phase |
|---|---|---|---|---|
| FR-112-001 | Sistem harus mengelola judul koleksi (judul, pengarang, penerbit, ISBN, kategori) dan eksemplarnya dengan barcode, lokasi rak, dan status. | M | Add-on | Phase 12 |
| FR-112-002 | Sistem harus mengimport data koleksi dari file Excel. | S | Add-on | Phase 12 |
| FR-112-003 | Sistem harus menyediakan pencarian katalog bagi siswa dan guru di portal, termasuk ketersediaan eksemplar. | M | Add-on | Phase 12 |

| ID | Skenario | Given / When / Then |
|---|---|---|
| AC-112-01 | Ketersediaan di katalog | **Given** buku "Fisika Dasar" memiliki 3 eksemplar dan 3 sedang dipinjam<br>**When** siswa mencari buku tersebut<br>**Then** katalog menampilkan 0 eksemplar tersedia |

#### 113. Keanggotaan Perpustakaan

| Atribut | Nilai |
|---|---|
| **Portal** | Admin / Perpustakaan |
| **Peran** | Pustakawan |
| **Scope** | Add-on |
| **Phase** | Phase 12 |
| **Tujuan** | Anggota perpustakaan dari siswa dan pegawai. |
| **Fitur** | Membership |
| **Sub-fitur** | Anggota otomatis dari data siswa dan pegawai, kartu anggota, kategori anggota, batas pinjam, status |

| ID | Kebutuhan | Prio | Scope | Phase |
|---|---|---|---|---|
| FR-113-001 | Sistem harus menjadikan siswa aktif dan pegawai aktif sebagai anggota secara otomatis dari menu 03 dan 83. | M | Add-on | Phase 12 |
| FR-113-002 | Sistem harus mengelola kategori anggota dengan batas jumlah pinjam dan lama pinjam (BR-LIB-01). | M | Add-on | Phase 12 |
| FR-113-003 | Sistem harus menonaktifkan keanggotaan siswa yang keluar atau lulus, setelah seluruh pinjamannya dikembalikan atau diselesaikan. | M | Add-on | Phase 12 |

| ID | Skenario | Given / When / Then |
|---|---|---|
| AC-113-01 | Siswa keluar masih meminjam | **Given** siswa keluar per 1 Juli masih meminjam 1 buku<br>**When** status keanggotaan diproses<br>**Then** keanggotaan ditandai belum dapat ditutup sampai buku dikembalikan atau diselesaikan |

#### 114. Sirkulasi & Denda

| Atribut | Nilai |
|---|---|
| **Portal** | Admin / Perpustakaan / Portal |
| **Peran** | Pustakawan; Anggota |
| **Scope** | Add-on |
| **Phase** | Phase 12 |
| **Tujuan** | Peminjaman, pengembalian, perpanjangan, reservasi, dan denda. |
| **Fitur** | Loan, Return, Renewal, Reservation, Fine |
| **Sub-fitur** | Pindai barcode, batas pinjam, tanggal kembali, pengingat, perpanjangan, reservasi, denda keterlambatan dan kehilangan |

| ID | Kebutuhan | Prio | Scope | Phase |
|---|---|---|---|---|
| FR-114-001 | Sistem harus mencatat peminjaman dan pengembalian dengan pemindaian barcode anggota dan eksemplar, serta menerapkan BR-LIB-01. | M | Add-on | Phase 12 |
| FR-114-002 | Sistem harus mengingatkan anggota [2] hari sebelum tanggal kembali dan saat terlambat. | S | Add-on | Phase 12 |
| FR-114-003 | Sistem harus mendukung perpanjangan dan reservasi eksemplar yang sedang dipinjam. | S | Add-on | Phase 12 |
| FR-114-004 | Sistem harus menghitung denda keterlambatan dan kehilangan sesuai tarif yang dikonfigurasi, dan menerapkan BR-LIB-02. | M | Add-on | Phase 12 |

| ID | Skenario | Given / When / Then |
|---|---|---|
| AC-114-01 | Batas pinjam | **Given** batas pinjam siswa 3 buku dan siswa sedang meminjam 3 buku<br>**When** pustakawan memindai buku keempat<br>**Then** peminjaman ditolak |
| AC-114-02 | Denda keterlambatan | **Given** tarif denda Rp1.000 per hari<br>**When** buku dikembalikan terlambat 4 hari<br>**Then** denda Rp4.000 tercatat |

#### 115. Laporan Perpustakaan

| Atribut | Nilai |
|---|---|
| **Portal** | Admin / Perpustakaan |
| **Peran** | Pustakawan; Kepala Unit |
| **Scope** | Add-on |
| **Phase** | Phase 12 |
| **Tujuan** | Laporan koleksi dan sirkulasi. |
| **Fitur** | Library reports |
| **Sub-fitur** | Statistik peminjaman, koleksi terpopuler, keterlambatan, denda, kunjungan, ekspor |

| ID | Kebutuhan | Prio | Scope | Phase |
|---|---|---|---|---|
| FR-115-001 | Sistem harus menghasilkan laporan peminjaman per periode, kelas, dan kategori koleksi. | M | Add-on | Phase 12 |
| FR-115-002 | Sistem harus menampilkan koleksi paling sering dipinjam dan daftar keterlambatan. | S | Add-on | Phase 12 |
| FR-115-003 | Sistem harus merekap denda yang terbit dan yang sudah dibayar. | M | Add-on | Phase 12 |

| ID | Skenario | Given / When / Then |
|---|---|---|
| AC-115-01 | Daftar keterlambatan | **Given** 5 anggota terlambat mengembalikan buku<br>**When** pustakawan membuka laporan keterlambatan<br>**Then** kelima anggota tampil dengan judul dan lama keterlambatan |

### Grup Menu 19 — Fasilitas

*Phase 12 · Add-on. Ruang dan fasilitas, pemesanan, dan perawatan.*

#### 116. Master Ruang & Fasilitas

| Atribut | Nilai |
|---|---|
| **Portal** | Admin / Fasilitas |
| **Peran** | Facility Officer |
| **Scope** | Add-on |
| **Phase** | Phase 12 |
| **Tujuan** | Data induk ruang dan fasilitas yang dipakai seluruh modul (kelas, jadwal, ujian, event, aset). |
| **Fitur** | Room & facility master |
| **Sub-fitur** | Gedung, lantai, ruang, jenis (kelas, lab, aula, lapangan), kapasitas, fasilitas pendukung, status, unit pemilik |

| ID | Kebutuhan | Prio | Scope | Phase |
|---|---|---|---|---|
| FR-116-001 | Sistem harus mengelola gedung, lantai, dan ruang dengan jenis, kapasitas, fasilitas pendukung, unit pemilik, dan status. | M | Add-on | Phase 12 |
| FR-116-002 | Sistem harus menandai ruang yang dapat dipesan dan ruang yang hanya untuk penggunaan tetap (misalnya kelas). | M | Add-on | Phase 12 |
| FR-116-003 | Sistem harus menyediakan data ruang untuk menu 39, 107, 117, dan modul pembelajaran (BR-FAC-02). | M | Add-on | Phase 12 |

| ID | Skenario | Given / When / Then |
|---|---|---|
| AC-116-01 | Ruang dalam perbaikan | **Given** aula berstatus dalam perbaikan<br>**When** pengguna mencari ruang untuk dipesan<br>**Then** aula tidak tampil sebagai pilihan |

#### 117. Booking & Cek Bentrok

| Atribut | Nilai |
|---|---|
| **Portal** | Admin / Fasilitas / Portal |
| **Peran** | Facility Officer; Guru; Event Coordinator |
| **Scope** | Add-on |
| **Phase** | Phase 12 |
| **Tujuan** | Pemesanan ruang dan fasilitas tanpa bentrok. |
| **Fitur** | Room booking, Conflict check, Calendar |
| **Sub-fitur** | Pemesanan, cek ketersediaan, bentrok dengan jadwal tetap, approval, kalender, pembatalan |

| ID | Kebutuhan | Prio | Scope | Phase |
|---|---|---|---|---|
| FR-117-001 | Sistem harus memungkinkan pengguna memesan ruang atau fasilitas untuk tanggal dan jam tertentu dengan tujuan dan jumlah peserta. | M | Add-on | Phase 12 |
| FR-117-002 | Sistem harus menolak pemesanan yang bentrok dengan pemesanan lain atau jadwal tetap, dan peserta yang melebihi kapasitas (BR-FAC-01). | M | Add-on | Phase 12 |
| FR-117-003 | Sistem harus memproses pemesanan ruang tertentu melalui workflow approval (menu 79). | S | Add-on | Phase 12 |
| FR-117-004 | Sistem harus menampilkan kalender pemakaian per ruang. | M | Add-on | Phase 12 |

| ID | Skenario | Given / When / Then |
|---|---|---|
| AC-117-01 | Bentrok ditolak | **Given** lab komputer dipesan 10.00–12.00<br>**When** guru memesan lab yang sama 11.00–13.00<br>**Then** pemesanan ditolak dan slot kosong terdekat ditampilkan |

#### 118. Permintaan Perawatan

| Atribut | Nilai |
|---|---|
| **Portal** | Admin / Fasilitas |
| **Peran** | Pemohon; Facility Officer; Teknisi |
| **Scope** | Add-on |
| **Phase** | Phase 12 |
| **Tujuan** | Perintah kerja perbaikan dan perawatan ruang serta aset. |
| **Fitur** | Work order, Preventive maintenance |
| **Sub-fitur** | Laporan kerusakan, ruang/aset terkait, prioritas, penugasan teknisi, status, biaya, jadwal perawatan berkala |

| ID | Kebutuhan | Prio | Scope | Phase |
|---|---|---|---|---|
| FR-118-001 | Sistem harus menerima laporan kerusakan dengan ruang atau aset terkait, foto, dan prioritas. | M | Add-on | Phase 12 |
| FR-118-002 | Sistem harus mengubah laporan menjadi perintah kerja yang ditugaskan kepada teknisi, dengan status dan catatan penyelesaian. | M | Add-on | Phase 12 |
| FR-118-003 | Sistem harus menjadwalkan perawatan berkala per aset atau ruang dan membuat perintah kerja otomatis. | S | Add-on | Phase 12 |
| FR-118-004 | Sistem harus mencatat biaya perawatan dan, bila memerlukan pembelian, membuat permintaan pembelian (menu 104). | S | Add-on | Phase 12 |

| ID | Skenario | Given / When / Then |
|---|---|---|
| AC-118-01 | Perawatan berkala | **Given** AC ruang guru dijadwalkan servis setiap 3 bulan<br>**When** jadwal berikutnya tiba<br>**Then** perintah kerja servis AC terbentuk dan ditugaskan ke teknisi |

### Grup Menu 20 — Transportasi

*Phase 12 · Add-on. Layanan antar-jemput siswa.*

#### 119. Rute & Halte

| Atribut | Nilai |
|---|---|
| **Portal** | Admin / Transport |
| **Peran** | Transport Officer |
| **Scope** | Add-on |
| **Phase** | Phase 12 |
| **Tujuan** | Rute antar-jemput dan titik jemput. |
| **Fitur** | Route, Stop |
| **Sub-fitur** | Rute, urutan halte, jam jemput/antar, zona tarif, status |

| ID | Kebutuhan | Prio | Scope | Phase |
|---|---|---|---|---|
| FR-119-001 | Sistem harus mengelola rute dengan urutan halte, perkiraan jam jemput dan antar, serta zona tarif. | M | Add-on | Phase 12 |
| FR-119-002 | Sistem harus menampilkan daftar siswa per halte dan rute. | M | Add-on | Phase 12 |

| ID | Skenario | Given / When / Then |
|---|---|---|
| AC-119-01 | Daftar siswa per halte | **Given** halte "Perumahan A" memiliki 6 siswa<br>**When** petugas membuka rute pagi<br>**Then** halte tersebut tampil dengan 6 siswa |

#### 120. Kendaraan & Pengemudi

| Atribut | Nilai |
|---|---|
| **Portal** | Admin / Transport |
| **Peran** | Transport Officer |
| **Scope** | Add-on |
| **Phase** | Phase 12 |
| **Tujuan** | Kendaraan, pengemudi, dan pendamping. |
| **Fitur** | Vehicle, Driver |
| **Sub-fitur** | Kendaraan, kapasitas, dokumen (STNK, uji), masa berlaku, pengemudi dan pendamping, penugasan ke rute |

| ID | Kebutuhan | Prio | Scope | Phase |
|---|---|---|---|---|
| FR-120-001 | Sistem harus mengelola kendaraan dengan kapasitas, nomor polisi, dan dokumen beserta masa berlakunya. | M | Add-on | Phase 12 |
| FR-120-002 | Sistem harus mengelola pengemudi dan pendamping (pegawai dari menu 83 atau pihak luar) dan menugaskannya ke rute. | M | Add-on | Phase 12 |
| FR-120-003 | Sistem harus mengingatkan [30] hari sebelum dokumen kendaraan atau SIM pengemudi kedaluwarsa. | S | Add-on | Phase 12 |

| ID | Skenario | Given / When / Then |
|---|---|---|
| AC-120-01 | Dokumen kendaraan kedaluwarsa | **Given** uji kendaraan bus 02 berlaku sampai 30 November<br>**When** tanggal menunjukkan 31 Oktober<br>**Then** Transport Officer menerima pengingat |

#### 121. Alokasi & Absensi Perjalanan

| Atribut | Nilai |
|---|---|
| **Portal** | Admin / Transport / Portal |
| **Peran** | Transport Officer; Pendamping; Orang tua |
| **Scope** | Add-on |
| **Phase** | Phase 12 |
| **Tujuan** | Penempatan siswa ke rute dan pencatatan naik-turun kendaraan. |
| **Fitur** | Student allocation, Trip attendance, Parent notification |
| **Sub-fitur** | Alokasi siswa ke rute dan halte, kapasitas, absensi naik/turun, notifikasi orang tua, siswa tidak hadir |

| ID | Kebutuhan | Prio | Scope | Phase |
|---|---|---|---|---|
| FR-121-001 | Sistem harus mengalokasikan siswa ke rute dan halte dengan menerapkan BR-TRS-01. | M | Add-on | Phase 12 |
| FR-121-002 | Sistem harus memungkinkan pendamping mencatat siswa naik dan turun per perjalanan melalui aplikasi. | M | Add-on | Phase 12 |
| FR-121-003 | Sistem harus memberi tahu orang tua saat anaknya naik dan turun kendaraan. | S | Add-on | Phase 12 |
| FR-121-004 | Sistem harus memungkinkan orang tua memberi tahu bahwa anak tidak ikut perjalanan pada tanggal tertentu. | S | Add-on | Phase 12 |

| ID | Skenario | Given / When / Then |
|---|---|---|
| AC-121-01 | Kapasitas penuh | **Given** kapasitas bus 01 adalah 15 dan sudah terisi 15<br>**When** petugas menambahkan siswa ke rute bus 01<br>**Then** alokasi ditolak dengan pesan kapasitas penuh |
| AC-121-02 | Notifikasi turun | **Given** pendamping mencatat siswa "Nara" turun di halte rumah<br>**When** catatan disimpan<br>**Then** orang tua Nara menerima notifikasi |

#### 122. Tagihan Transport

| Atribut | Nilai |
|---|---|
| **Portal** | Admin / Transport / Finance |
| **Peran** | Transport Officer; Finance Admin |
| **Scope** | Add-on |
| **Phase** | Phase 12 |
| **Tujuan** | Tarif layanan transport dan penerbitan tagihannya melalui billing. |
| **Fitur** | Transport fee, Billing trigger |
| **Sub-fitur** | Tarif per zona/rute, periode, penerbitan melalui menu 19, perubahan alokasi, prorata |

| ID | Kebutuhan | Prio | Scope | Phase |
|---|---|---|---|---|
| FR-122-001 | Sistem harus mengelola tarif transport per zona atau rute dan periode sebagai komponen biaya (menu 18). | M | Add-on | Phase 12 |
| FR-122-002 | Sistem harus menerbitkan tagihan transport bagi siswa yang dialokasikan melalui menu 19. | M | Add-on | Phase 12 |
| FR-122-003 | Sistem harus menyesuaikan tagihan secara prorata saat siswa mulai, pindah rute, atau berhenti di tengah periode sesuai kebijakan yang dikonfigurasi. | S | Add-on | Phase 12 |

| ID | Skenario | Given / When / Then |
|---|---|---|
| AC-122-01 | Prorata berhenti di tengah bulan | **Given** tarif transport Rp600.000 per bulan dan kebijakan prorata harian aktif<br>**When** siswa berhenti setelah 15 dari 30 hari<br>**Then** tagihan bulan tersebut disesuaikan menjadi Rp300.000 |

### Grup Menu 21 — Kantin & Dompet Siswa

*Phase 10 · Add-on. Kantin cashless, tabungan, dan uang saku digital.*

#### 123. Tenant Kantin

| Atribut | Nilai |
|---|---|
| **Portal** | Admin / Kantin |
| **Peran** | Canteen Admin |
| **Scope** | Add-on |
| **Phase** | Phase 10 |
| **Tujuan** | Data penjual kantin dan perjanjian bagi hasil. |
| **Fitur** | Tenant management |
| **Sub-fitur** | Tenant, unit/lokasi, pemilik, rekening, skema bagi hasil, status, akun operator |

| ID | Kebutuhan | Prio | Scope | Phase |
|---|---|---|---|---|
| FR-123-001 | Sistem harus mengelola tenant kantin dengan lokasi, pemilik, rekening, dan status. | M | Add-on | Phase 10 |
| FR-123-002 | Sistem harus menyimpan skema bagi hasil atau sewa per tenant. | M | Add-on | Phase 10 |
| FR-123-003 | Sistem harus membuat akun operator POS untuk setiap tenant dengan akses terbatas pada tenant tersebut. | M | Add-on | Phase 10 |

| ID | Skenario | Given / When / Then |
|---|---|---|
| AC-123-01 | Akses operator dibatasi | **Given** operator tenant A login ke POS<br>**When** operator membuka laporan<br>**Then** hanya transaksi tenant A yang tampil |

#### 124. Menu & POS

| Atribut | Nilai |
|---|---|
| **Portal** | POS Kantin |
| **Peran** | Operator tenant |
| **Scope** | Add-on |
| **Phase** | Phase 10 |
| **Tujuan** | Daftar menu dan transaksi penjualan kantin. |
| **Fitur** | Menu catalog, Point of sale |
| **Sub-fitur** | Item menu, harga, stok harian, transaksi dengan kartu/QR siswa, tunai, pembatalan, struk, mode luring |

| ID | Kebutuhan | Prio | Scope | Phase |
|---|---|---|---|---|
| FR-124-001 | Sistem harus mengelola item menu dan harga per tenant. | M | Add-on | Phase 10 |
| FR-124-002 | Sistem harus mencatat penjualan dengan pembayaran dari uang saku digital siswa (pindai kartu atau QR) atau tunai. | M | Add-on | Phase 10 |
| FR-124-003 | Sistem harus menerapkan BR-WAL-01 sebelum transaksi uang saku disetujui. | M | Add-on | Phase 10 |
| FR-124-004 | Sistem harus memproses pembatalan transaksi dengan alasan dan mengembalikan saldo uang saku. | M | Add-on | Phase 10 |
| FR-124-005 | Sistem harus tetap dapat mencatat transaksi tunai saat koneksi terputus dan menyinkronkannya tanpa duplikasi setelah tersambung kembali. | S | Add-on | Phase 10 |

| ID | Skenario | Given / When / Then |
|---|---|---|
| AC-124-01 | Melebihi batas harian | **Given** batas jajan harian siswa Rp25.000 dan hari ini sudah terpakai Rp20.000<br>**When** siswa membeli makanan Rp10.000 dengan uang saku<br>**Then** transaksi ditolak dengan pesan melebihi batas harian |

#### 125. Tabungan & Uang Saku Digital

| Atribut | Nilai |
|---|---|
| **Portal** | Portal Orang Tua / Admin |
| **Peran** | Orang tua; Siswa; Finance Admin |
| **Scope** | Add-on |
| **Phase** | Phase 10 |
| **Tujuan** | Dompet siswa: uang saku untuk belanja di sekolah dan tabungan. |
| **Fitur** | Student wallet, Savings, Top-up, Limits |
| **Sub-fitur** | Saldo uang saku, saldo tabungan, top-up melalui payment layer atau kasir, batas harian, riwayat, penarikan tabungan, penutupan |

| ID | Kebutuhan | Prio | Scope | Phase |
|---|---|---|---|---|
| FR-125-001 | Sistem harus menyediakan dua jenis saldo per siswa: uang saku dan tabungan. | M | Add-on | Phase 10 |
| FR-125-002 | Sistem harus menerima top-up melalui pembayaran online (menu 27–28) atau kasir sekolah (menu 29). | M | Add-on | Phase 10 |
| FR-125-003 | Sistem harus memungkinkan orang tua mengatur batas belanja harian dan melihat riwayat transaksi anak. | M | Add-on | Phase 10 |
| FR-125-004 | Sistem harus memproses setoran dan penarikan tabungan dengan bukti, dan penarikan memerlukan persetujuan orang tua. | M | Add-on | Phase 10 |
| FR-125-005 | Sistem harus mengembalikan atau memindahkan sisa saldo saat siswa keluar atau lulus melalui menu 78 dan 25. | M | Add-on | Phase 10 |
| FR-125-006 | Sistem harus mencatat saldo dompet sebagai dana titipan (BR-WAL-02) dan mengirim jurnalnya ke Accounting Engine. | M | Add-on | Phase 10 |

| ID | Skenario | Given / When / Then |
|---|---|---|
| AC-125-01 | Top-up online | **Given** orang tua melakukan top-up Rp200.000 melalui QRIS<br>**When** pembayaran sukses<br>**Then** saldo uang saku anak bertambah Rp200.000 dan orang tua menerima notifikasi |
| AC-125-02 | Saldo siswa keluar | **Given** siswa yang keluar memiliki saldo tabungan Rp350.000<br>**When** withdrawal disetujui<br>**Then** pengajuan refund Rp350.000 terbentuk di menu 25 |

#### 126. Laporan Transaksi Kantin

| Atribut | Nilai |
|---|---|
| **Portal** | Admin / Kantin / Finance |
| **Peran** | Canteen Admin; Finance Admin; Operator tenant |
| **Scope** | Add-on |
| **Phase** | Phase 10 |
| **Tujuan** | Rekap penjualan dan penyelesaian bagi hasil dengan tenant. |
| **Fitur** | Sales report, Tenant settlement |
| **Sub-fitur** | Penjualan per tenant dan item, metode bayar, bagi hasil, penyelesaian dana ke tenant, jurnal |

| ID | Kebutuhan | Prio | Scope | Phase |
|---|---|---|---|---|
| FR-126-001 | Sistem harus merekap penjualan per tenant, item, periode, dan metode pembayaran. | M | Add-on | Phase 10 |
| FR-126-002 | Sistem harus menghitung hak tenant dan bagian sekolah sesuai skema bagi hasil, lalu memproses penyelesaian dana melalui workflow approval (menu 79). | M | Add-on | Phase 10 |
| FR-126-003 | Sistem harus mengirim jurnal penjualan, bagi hasil, dan penyelesaian ke Accounting Engine. | M | Add-on | Phase 10 |

| ID | Skenario | Given / When / Then |
|---|---|---|
| AC-126-01 | Bagi hasil | **Given** penjualan uang saku tenant A bulan Oktober Rp10.000.000 dan bagi hasil sekolah 10%<br>**When** penyelesaian dihitung<br>**Then** hak tenant Rp9.000.000 dan bagian sekolah Rp1.000.000 |

### Grup Menu 22 — Kesehatan Sekolah

*Phase 13 · Add-on. Catatan kesehatan siswa dan layanan UKS.*

#### 127. Profil Kesehatan

| Atribut | Nilai |
|---|---|
| **Portal** | Admin / UKS / Portal Orang Tua |
| **Peran** | Petugas UKS; Orang tua; Wali kelas (terbatas) |
| **Scope** | Add-on |
| **Phase** | Phase 13 |
| **Tujuan** | Data kesehatan siswa yang diperlukan sekolah. |
| **Fitur** | Health profile |
| **Sub-fitur** | Golongan darah, alergi, kondisi khusus, obat rutin, kontak darurat, pembaruan oleh orang tua, akses terbatas |

| ID | Kebutuhan | Prio | Scope | Phase |
|---|---|---|---|---|
| FR-127-001 | Sistem harus menyimpan profil kesehatan siswa: golongan darah, alergi, kondisi khusus, obat rutin, dan kontak darurat. | M | Add-on | Phase 13 |
| FR-127-002 | Sistem harus memungkinkan orang tua memperbarui profil kesehatan anak, dengan perubahan diverifikasi petugas UKS. | M | Add-on | Phase 13 |
| FR-127-003 | Sistem harus menerapkan BR-HLT-01 pada akses data kesehatan. | M | Add-on | Phase 13 |

| ID | Skenario | Given / When / Then |
|---|---|---|
| AC-127-01 | Wali kelas hanya melihat peringatan | **Given** siswa memiliki alergi kacang dan riwayat penyakit lain<br>**When** wali kelas membuka data siswa<br>**Then** hanya peringatan alergi dan kontak darurat yang tampil |

#### 128. Kunjungan UKS/Klinik

| Atribut | Nilai |
|---|---|
| **Portal** | Admin / UKS |
| **Peran** | Petugas UKS |
| **Scope** | Add-on |
| **Phase** | Phase 13 |
| **Tujuan** | Pencatatan kunjungan siswa ke UKS. |
| **Fitur** | Clinic visit |
| **Sub-fitur** | Keluhan, pemeriksaan, tindakan, obat diberikan, istirahat/pulang/rujukan, notifikasi orang tua, stok obat UKS |

| ID | Kebutuhan | Prio | Scope | Phase |
|---|---|---|---|---|
| FR-128-001 | Sistem harus mencatat kunjungan dengan keluhan, pemeriksaan, tindakan, obat yang diberikan, dan hasil (kembali ke kelas, pulang, dirujuk). | M | Add-on | Phase 13 |
| FR-128-002 | Sistem harus memberi tahu orang tua ketika siswa dipulangkan atau dirujuk. | M | Add-on | Phase 13 |
| FR-128-003 | Sistem harus mencatat penggunaan obat UKS sebagai pengeluaran stok (menu 102). | S | Add-on | Phase 13 |

| ID | Skenario | Given / When / Then |
|---|---|---|
| AC-128-01 | Siswa dipulangkan | **Given** petugas UKS memutuskan siswa dipulangkan karena demam<br>**When** kunjungan disimpan<br>**Then** orang tua menerima notifikasi dan status kehadiran siswa diperbarui menjadi sakit sejak jam tersebut |

#### 129. Imunisasi & Skrining

| Atribut | Nilai |
|---|---|
| **Portal** | Admin / UKS / Portal Orang Tua |
| **Peran** | Petugas UKS; Orang tua |
| **Scope** | Add-on |
| **Phase** | Phase 13 |
| **Tujuan** | Program imunisasi dan skrining kesehatan sekolah. |
| **Fitur** | Immunization, Screening, Consent |
| **Sub-fitur** | Program, jadwal, peserta, persetujuan orang tua, hasil, tindak lanjut, rekap |

| ID | Kebutuhan | Prio | Scope | Phase |
|---|---|---|---|---|
| FR-129-001 | Sistem harus mengelola program imunisasi atau skrining dengan jadwal dan sasaran peserta. | M | Add-on | Phase 13 |
| FR-129-002 | Sistem harus meminta persetujuan orang tua secara digital sebelum program yang memerlukan persetujuan. | M | Add-on | Phase 13 |
| FR-129-003 | Sistem harus mencatat hasil per siswa dan menandai siswa yang memerlukan tindak lanjut. | M | Add-on | Phase 13 |

| ID | Skenario | Given / When / Then |
|---|---|---|
| AC-129-01 | Tanpa persetujuan | **Given** orang tua belum memberi persetujuan imunisasi<br>**When** petugas mencatat imunisasi untuk siswa tersebut<br>**Then** pencatatan ditolak sampai persetujuan tersedia |

### Grup Menu 23 — Konseling & Kesiswaan

*Phase 13 · Add-on. Bimbingan konseling, pembinaan disiplin, dan rujukan.*

#### 130. Catatan Konseling

| Atribut | Nilai |
|---|---|
| **Portal** | Admin / BK |
| **Peran** | Guru BK / Konselor |
| **Scope** | Add-on |
| **Phase** | Phase 13 |
| **Tujuan** | Catatan sesi bimbingan konseling yang bersifat rahasia. |
| **Fitur** | Counseling records |
| **Sub-fitur** | Sesi individu/kelompok, kategori, catatan, rencana tindak lanjut, kerahasiaan |

| ID | Kebutuhan | Prio | Scope | Phase |
|---|---|---|---|---|
| FR-130-001 | Sistem harus mencatat sesi konseling individu atau kelompok dengan kategori, catatan, dan rencana tindak lanjut. | M | Add-on | Phase 13 |
| FR-130-002 | Sistem harus menerapkan BR-CNS-01 pada akses catatan konseling. | M | Add-on | Phase 13 |
| FR-130-003 | Sistem harus menjadwalkan sesi lanjutan dan mengingatkan konselor. | S | Add-on | Phase 13 |

| ID | Skenario | Given / When / Then |
|---|---|---|
| AC-130-01 | Catatan rahasia | **Given** wali kelas tidak memiliki peran konselor<br>**When** wali kelas mencari catatan konseling siswanya<br>**Then** catatan tidak ditampilkan; hanya keterangan bahwa siswa sedang dalam pendampingan BK bila konselor mengizinkan |

#### 131. Poin & Pembinaan Disiplin

| Atribut | Nilai |
|---|---|
| **Portal** | Admin / Kesiswaan / Portal Orang Tua |
| **Peran** | Kesiswaan Officer; Wali kelas; Orang tua |
| **Scope** | Add-on |
| **Phase** | Phase 13 |
| **Tujuan** | Pencatatan pelanggaran dan prestasi perilaku dengan sistem poin. |
| **Fitur** | Discipline points, Intervention |
| **Sub-fitur** | Jenis pelanggaran dan poin, pencatatan, bukti, akumulasi, ambang tindakan, pembinaan, notifikasi orang tua, banding |

| ID | Kebutuhan | Prio | Scope | Phase |
|---|---|---|---|---|
| FR-131-001 | Sistem harus mengelola jenis pelanggaran beserta poinnya, serta ambang tindakan pembinaan. | M | Add-on | Phase 13 |
| FR-131-002 | Sistem harus mencatat pelanggaran dengan tanggal, bukti, dan pelapor, lalu memberi tahu orang tua. | M | Add-on | Phase 13 |
| FR-131-003 | Sistem harus menerapkan BR-DIS-01 ketika akumulasi poin mencapai ambang. | M | Add-on | Phase 13 |
| FR-131-004 | Sistem harus memungkinkan orang tua mengajukan keberatan atas catatan pelanggaran, yang diproses melalui workflow approval (menu 79). | S | Add-on | Phase 13 |

| ID | Skenario | Given / When / Then |
|---|---|---|
| AC-131-01 | Ambang pembinaan | **Given** ambang pemanggilan orang tua 50 poin dan siswa memiliki 45 poin<br>**When** pelanggaran 10 poin dicatat<br>**Then** tindakan pemanggilan orang tua terbentuk dan orang tua menerima notifikasi |

#### 132. Rujukan & Tindak Lanjut

| Atribut | Nilai |
|---|---|
| **Portal** | Admin / BK |
| **Peran** | Guru BK / Konselor; Kesiswaan Officer |
| **Scope** | Add-on |
| **Phase** | Phase 13 |
| **Tujuan** | Rujukan siswa ke pihak lain dan pemantauan tindak lanjutnya. |
| **Fitur** | Referral, Case follow-up |
| **Sub-fitur** | Rujukan internal/eksternal (psikolog, puskesmas), persetujuan orang tua, tugas tindak lanjut, status kasus |

| ID | Kebutuhan | Prio | Scope | Phase |
|---|---|---|---|---|
| FR-132-001 | Sistem harus mencatat rujukan siswa ke pihak internal atau eksternal dengan alasan dan persetujuan orang tua. | M | Add-on | Phase 13 |
| FR-132-002 | Sistem harus mengelola tugas tindak lanjut kasus dengan penanggung jawab dan tenggat. | M | Add-on | Phase 13 |
| FR-132-003 | Sistem harus menampilkan status kasus sampai ditutup, dengan akses mengikuti BR-CNS-01. | M | Add-on | Phase 13 |

| ID | Skenario | Given / When / Then |
|---|---|---|
| AC-132-01 | Rujukan memerlukan persetujuan | **Given** konselor merujuk siswa ke psikolog<br>**When** persetujuan orang tua belum ada<br>**Then** rujukan berstatus menunggu persetujuan orang tua |

### Grup Menu 24 — Event & Ekstrakurikuler

*Phase 10 · Add-on. Kegiatan sekolah, ekstrakurikuler, dan prestasi siswa.*

#### 133. Manajemen Event

| Atribut | Nilai |
|---|---|
| **Portal** | Admin / Kegiatan |
| **Peran** | Event Coordinator; Kepala Unit |
| **Scope** | Add-on |
| **Phase** | Phase 10 |
| **Tujuan** | Perencanaan kegiatan sekolah. |
| **Fitur** | Event planning |
| **Sub-fitur** | Nama, jenis, tanggal, lokasi (booking menu 117), sasaran peserta, biaya, kuota, panitia, publikasi, approval |

| ID | Kebutuhan | Prio | Scope | Phase |
|---|---|---|---|---|
| FR-133-001 | Sistem harus mengelola event dengan jenis, tanggal, lokasi, sasaran peserta, kuota, biaya, dan panitia. | M | Add-on | Phase 10 |
| FR-133-002 | Sistem harus memesan lokasi melalui menu 117 saat event dibuat. | M | Add-on | Phase 10 |
| FR-133-003 | Sistem harus memproses event yang memerlukan biaya atau anggaran melalui workflow approval (menu 79). | M | Add-on | Phase 10 |
| FR-133-004 | Sistem harus mempublikasikan event ke kalender dan portal sasaran. | S | Add-on | Phase 10 |

| ID | Skenario | Given / When / Then |
|---|---|---|
| AC-133-01 | Lokasi bentrok | **Given** aula sudah dipesan pada tanggal event<br>**When** coordinator membuat event di aula pada tanggal yang sama<br>**Then** event tidak dapat disimpan dengan lokasi tersebut |

#### 134. Pendaftaran & Persetujuan Orang Tua

| Atribut | Nilai |
|---|---|
| **Portal** | Portal Orang Tua / Admin |
| **Peran** | Orang tua; Siswa; Event Coordinator |
| **Scope** | Add-on |
| **Phase** | Phase 10 |
| **Tujuan** | Pendaftaran peserta event dan persetujuan orang tua secara digital. |
| **Fitur** | Registration, Digital consent |
| **Sub-fitur** | Pendaftaran, kuota, persetujuan orang tua, tagihan biaya, daftar peserta, kehadiran event |

| ID | Kebutuhan | Prio | Scope | Phase |
|---|---|---|---|---|
| FR-134-001 | Sistem harus memungkinkan siswa atau orang tua mendaftar event sesuai kuota dan sasaran. | M | Add-on | Phase 10 |
| FR-134-002 | Sistem harus meminta persetujuan orang tua secara digital untuk event yang memerlukannya (BR-EVT-01). | M | Add-on | Phase 10 |
| FR-134-003 | Sistem harus menerbitkan tagihan biaya event melalui menu 19 setelah persetujuan diberikan. | M | Add-on | Phase 10 |
| FR-134-004 | Sistem harus mencatat kehadiran peserta pada hari event. | S | Add-on | Phase 10 |

| ID | Skenario | Given / When / Then |
|---|---|---|
| AC-134-01 | Tagihan setelah persetujuan | **Given** study tour berbiaya Rp750.000 memerlukan persetujuan orang tua<br>**When** orang tua menyetujui<br>**Then** tagihan Rp750.000 terbit untuk anaknya |

#### 135. Ekstrakurikuler

| Atribut | Nilai |
|---|---|
| **Portal** | Admin / Kegiatan / Portal |
| **Peran** | Pembina ekskul; Siswa; Orang tua |
| **Scope** | Add-on |
| **Phase** | Phase 10 |
| **Tujuan** | Kelompok ekstrakurikuler, anggota, jadwal, dan kehadiran. |
| **Fitur** | Extracurricular group, Membership, Attendance |
| **Sub-fitur** | Kelompok, pembina, jadwal, lokasi, pendaftaran anggota, kuota, kehadiran, penilaian, biaya |

| ID | Kebutuhan | Prio | Scope | Phase |
|---|---|---|---|---|
| FR-135-001 | Sistem harus mengelola kelompok ekstrakurikuler dengan pembina, jadwal, lokasi, kuota, dan biaya. | M | Add-on | Phase 10 |
| FR-135-002 | Sistem harus memungkinkan siswa mendaftar ekstrakurikuler pada periode pendaftaran dan menerbitkan biaya melalui menu 19 bila ada. | M | Add-on | Phase 10 |
| FR-135-003 | Sistem harus memungkinkan pembina mencatat kehadiran dan penilaian anggota. | M | Add-on | Phase 10 |

| ID | Skenario | Given / When / Then |
|---|---|---|
| AC-135-01 | Kuota ekskul penuh | **Given** kuota ekskul robotik 20 dan sudah terisi 20<br>**When** siswa mendaftar<br>**Then** pendaftaran ditolak atau masuk daftar tunggu sesuai pengaturan |

#### 136. Prestasi Siswa

| Atribut | Nilai |
|---|---|
| **Portal** | Admin / Kesiswaan / Portal |
| **Peran** | Kesiswaan Officer; Pembina; Orang tua |
| **Scope** | Add-on |
| **Phase** | Phase 10 |
| **Tujuan** | Pencatatan prestasi siswa sebagai sumber data portofolio dan dashboard. |
| **Fitur** | Achievement record |
| **Sub-fitur** | Nama lomba/kegiatan, tingkat, peringkat, tanggal, bukti, verifikasi, publikasi |

| ID | Kebutuhan | Prio | Scope | Phase |
|---|---|---|---|---|
| FR-136-001 | Sistem harus mencatat prestasi siswa dengan nama kegiatan, tingkat (sekolah sampai internasional), peringkat, tanggal, dan bukti. | M | Add-on | Phase 10 |
| FR-136-002 | Sistem harus memverifikasi prestasi sebelum tampil di portal dan dashboard. | M | Add-on | Phase 10 |
| FR-136-003 | Sistem harus menyediakan data prestasi terverifikasi untuk dashboard menu 51–53. | S | Add-on | Phase 10 |

| ID | Skenario | Given / When / Then |
|---|---|---|
| AC-136-01 | Prestasi menunggu verifikasi | **Given** orang tua mengunggah sertifikat juara 2 lomba sains tingkat kota<br>**When** Kesiswaan Officer belum memverifikasi<br>**Then** prestasi belum tampil di dashboard |

### Grup Menu 25 — Service Desk

*Phase 10 · Add-on. Tiket layanan untuk orang tua, siswa, dan pegawai.*

#### 137. Tiket & Kategori

| Atribut | Nilai |
|---|---|
| **Portal** | Admin / Portal |
| **Peran** | Service Desk Agent; Orang tua; Pegawai |
| **Scope** | Add-on |
| **Phase** | Phase 10 |
| **Tujuan** | Pencatatan dan penanganan tiket layanan lintas unit. |
| **Fitur** | Ticketing, Categories, Assignment |
| **Sub-fitur** | Kategori, formulir per kategori, tim penanggung jawab, penugasan, status, komentar, lampiran, konversi ke perintah kerja, kepuasan |

| ID | Kebutuhan | Prio | Scope | Phase |
|---|---|---|---|---|
| FR-137-001 | Sistem harus mengelola kategori tiket beserta formulir dan tim penanggung jawabnya. | M | Add-on | Phase 10 |
| FR-137-002 | Sistem harus menerima tiket dari portal orang tua, siswa, dan pegawai, lalu meneruskannya ke tim sesuai kategori (BR-SD-01). | M | Add-on | Phase 10 |
| FR-137-003 | Sistem harus mencatat status, komentar, dan lampiran tiket, serta memberi tahu pelapor setiap perubahan status. | M | Add-on | Phase 10 |
| FR-137-004 | Sistem harus mengubah tiket kategori fasilitas menjadi perintah kerja di menu 118. | S | Add-on | Phase 10 |
| FR-137-005 | Sistem harus meminta penilaian kepuasan pelapor setelah tiket selesai. | S | Add-on | Phase 10 |

| ID | Skenario | Given / When / Then |
|---|---|---|
| AC-137-01 | Tiket menjadi perintah kerja | **Given** tiket "AC kelas 8A mati" berkategori fasilitas<br>**When** agent mengonversi tiket<br>**Then** perintah kerja terbentuk di menu 118 dan tautannya tersimpan di tiket |

#### 138. SLA & Eskalasi

| Atribut | Nilai |
|---|---|
| **Portal** | Admin / Service Desk |
| **Peran** | Service Desk Manager |
| **Scope** | Add-on |
| **Phase** | Phase 10 |
| **Tujuan** | Target waktu layanan, eskalasi, dan laporan kinerja service desk. |
| **Fitur** | SLA policy, Escalation, Reporting |
| **Sub-fitur** | Target respons dan penyelesaian per kategori/prioritas, jam layanan, eskalasi, pelanggaran SLA, laporan |

| ID | Kebutuhan | Prio | Scope | Phase |
|---|---|---|---|---|
| FR-138-001 | Sistem harus mengelola target waktu respons dan penyelesaian per kategori dan prioritas, dengan jam layanan yang dikonfigurasi. | M | Add-on | Phase 10 |
| FR-138-002 | Sistem harus mengeskalasi tiket yang mendekati atau melewati target ke penanggung jawab berikutnya. | M | Add-on | Phase 10 |
| FR-138-003 | Sistem harus melaporkan kepatuhan SLA, jumlah tiket, dan kepuasan per kategori dan tim. | M | Add-on | Phase 10 |

| ID | Skenario | Given / When / Then |
|---|---|---|
| AC-138-01 | Pelanggaran SLA | **Given** target penyelesaian kategori tagihan 2 hari kerja<br>**When** tiket belum selesai setelah 2 hari kerja<br>**Then** tiket ditandai melanggar SLA dan dieskalasi ke supervisor |

### Grup Menu 26 — PKL

*Phase 13 · Add-on. Praktik Kerja Lapangan untuk unit SMK.*

#### 139. Mitra & Lokasi PKL

| Atribut | Nilai |
|---|---|
| **Portal** | Admin / PKL |
| **Peran** | Koordinator PKL |
| **Scope** | Add-on |
| **Phase** | Phase 13 |
| **Tujuan** | Data mitra industri dan kapasitas penempatan. |
| **Fitur** | Industry partner, Placement slot |
| **Sub-fitur** | Mitra, bidang, alamat, kontak, MoU dan masa berlaku, kuota per periode dan jurusan, pembimbing industri |

| ID | Kebutuhan | Prio | Scope | Phase |
|---|---|---|---|---|
| FR-139-001 | Sistem harus mengelola mitra industri dengan bidang, alamat, kontak, dan dokumen MoU beserta masa berlakunya. | M | Add-on | Phase 13 |
| FR-139-002 | Sistem harus mengelola kuota penempatan per mitra, periode, dan jurusan. | M | Add-on | Phase 13 |
| FR-139-003 | Sistem harus mengelola akun pembimbing industri dengan akses terbatas pada siswa yang dibimbingnya. | M | Add-on | Phase 13 |

| ID | Skenario | Given / When / Then |
|---|---|---|
| AC-139-01 | MoU kedaluwarsa | **Given** MoU mitra berakhir 31 Desember<br>**When** koordinator membuat penempatan periode Januari pada mitra tersebut<br>**Then** sistem memperingatkan bahwa MoU sudah tidak berlaku |

#### 140. Penempatan & Absensi PKL

| Atribut | Nilai |
|---|---|
| **Portal** | Admin / PKL / Portal |
| **Peran** | Koordinator PKL; Siswa; Pembimbing sekolah; Pembimbing industri |
| **Scope** | Add-on |
| **Phase** | Phase 13 |
| **Tujuan** | Penempatan siswa ke mitra dan kehadirannya selama PKL. |
| **Fitur** | Placement, PKL attendance |
| **Sub-fitur** | Penempatan sesuai kuota, pembimbing, periode, absensi harian, izin, rekap |

| ID | Kebutuhan | Prio | Scope | Phase |
|---|---|---|---|---|
| FR-140-001 | Sistem harus menempatkan siswa ke mitra dengan menerapkan BR-PKL-01, beserta pembimbing sekolah dan pembimbing industri. | M | Add-on | Phase 13 |
| FR-140-002 | Sistem harus memungkinkan siswa mencatat kehadiran harian di lokasi PKL dan pembimbing industri mengonfirmasinya. | M | Add-on | Phase 13 |
| FR-140-003 | Sistem harus merekap kehadiran PKL per siswa dan mitra. | M | Add-on | Phase 13 |

| ID | Skenario | Given / When / Then |
|---|---|---|
| AC-140-01 | Kuota mitra penuh | **Given** kuota mitra A untuk jurusan TKJ adalah 4 dan sudah terisi 4<br>**When** koordinator menempatkan siswa TKJ kelima<br>**Then** penempatan ditolak |

#### 141. Dokumentasi & Penilaian PKL

| Atribut | Nilai |
|---|---|
| **Portal** | Admin / PKL / Portal |
| **Peran** | Siswa; Pembimbing sekolah; Pembimbing industri |
| **Scope** | Add-on |
| **Phase** | Phase 13 |
| **Tujuan** | Jurnal kegiatan, dokumentasi, dan penilaian PKL. |
| **Fitur** | Activity journal, Assessment |
| **Sub-fitur** | Jurnal harian, foto kegiatan, laporan akhir, penilaian pembimbing industri dan sekolah, nilai akhir, sertifikat |

| ID | Kebutuhan | Prio | Scope | Phase |
|---|---|---|---|---|
| FR-141-001 | Sistem harus memungkinkan siswa mengisi jurnal kegiatan harian dengan foto dan mengunggah laporan akhir. | M | Add-on | Phase 13 |
| FR-141-002 | Sistem harus memungkinkan pembimbing industri dan pembimbing sekolah memberi penilaian sesuai komponen dan bobot yang dikonfigurasi. | M | Add-on | Phase 13 |
| FR-141-003 | Sistem harus menghitung nilai akhir PKL dan menerbitkan sertifikat PKL dalam PDF. | S | Add-on | Phase 13 |

| ID | Skenario | Given / When / Then |
|---|---|---|
| AC-141-01 | Nilai akhir PKL | **Given** bobot penilaian industri 70% dan sekolah 30%, dengan nilai 85 dan 80<br>**When** nilai akhir dihitung<br>**Then** nilai akhir PKL siswa 83,5 |

## 6. Alur Bisnis End-to-End

| Alur | Urutan (nomor menu) | Phase |
|---|---|---|
| Pengadaan sampai pembayaran vendor | 104 PR → 109 Cek anggaran → 79 Approval → 105 PO → 105 Penerimaan → 102 Stok / 107 Aset → 111 Tagihan vendor (3-way match) → 110 Pemotongan pajak → 79 Approval → Accounting Engine | Phase 11 |
| Siklus aset | 105 Penerimaan → 107 Aktivasi → Accounting Engine (penyusutan) → 108 Mutasi / pelepasan → 118 Perawatan | Phase 11–12 |
| Anggaran | 109 Penyusunan → 79 Approval → 104 Komitmen → jurnal posted → 109 Realisasi | Phase 11 |
| Dompet siswa | 27 Top-up → 125 Saldo → 124 Transaksi POS → 126 Bagi hasil → Accounting Engine | Phase 10 |
| Event berbayar | 133 Event + 117 Lokasi → 134 Pendaftaran → persetujuan orang tua → 19 Tagihan → 134 Kehadiran | Phase 10 |
| Layanan dan perbaikan | 81/137 Tiket → 138 SLA → 118 Perintah kerja → 104 Pembelian bila perlu | Phase 10–12 |
| Antar-jemput | 119 Rute → 121 Alokasi → 122 Tagihan → 121 Absensi perjalanan + notifikasi | Phase 12 |
| PKL | 139 Mitra → 140 Penempatan → 140 Absensi → 141 Jurnal & penilaian → sertifikat | Phase 13 |

## 7. Integrasi

| ID | Integrasi | Kebutuhan | Phase |
|---|---|---|---|
| IF-OPS-01 | Accounting Engine | Sistem harus mengirim transaksi utang, aset (perolehan, mutasi, pelepasan), dompet, penjualan kantin, dan bagi hasil secara idempoten. | Phase 10–11 |
| IF-OPS-02 | Accounting Engine | Sistem harus membaca realisasi jurnal posted dan nilai buku aset untuk menu 108 dan 109. | Phase 11 |
| IF-OPS-03 | Payment layer | Top-up dompet memakai alur checkout menu 27–28. | Phase 10 |
| IF-OPS-04 | Pemindai | Sistem harus menerima input dari pemindai barcode/QR standar (keyboard wedge) dan kamera ponsel untuk perpustakaan, aset, POS, dan absensi perjalanan. | Phase 10–12 |
| IF-OPS-05 | DJP | Sistem harus menghasilkan data pelaporan PPh non-gaji dan faktur pajak dalam format yang berlaku. | Phase 11 |

## 8. Kebutuhan Non-Fungsional Khusus

| ID | Area | Kebutuhan | Prio | Verif. |
|---|---|---|---|---|
| NFR-OPS-01 | Kinerja POS | Transaksi POS uang saku selesai ≤ [2] detik pada jam istirahat dengan [300] transaksi per 15 menit per unit. | M | T |
| NFR-OPS-02 | Ketahanan POS | Transaksi tunai tetap dapat dicatat saat koneksi terputus hingga [30] menit dan disinkronkan tanpa duplikasi. | S | T |
| NFR-OPS-03 | Data sensitif | Data kesehatan dan konseling dienkripsi di tingkat kolom, aksesnya dicatat, dan tidak muncul di ekspor umum. | M | I |
| NFR-OPS-04 | Rekonsiliasi dompet | Total saldo dompet seluruh siswa sama dengan saldo akun dana titipan di Accounting Engine setiap hari. | M | A |

## 9. Pertanyaan Terbuka Bagian 3

Melanjutkan penomoran Bagian 2.

| ID | Pertanyaan | Terdampak | Prioritas |
|---|---|---|---|
| OQ-28 | Jenis transaksi dan tarif pemotongan PPh non-gaji default, serta apakah klien berstatus PKP (validasi konsultan pajak) | 110 | Tinggi |
| OQ-29 | Kajian regulasi dompet uang saku dan tabungan siswa (dana tertutup di lingkungan sekolah) terhadap ketentuan uang elektronik Bank Indonesia | 125 | Tinggi |
| OQ-30 | Bentuk kartu siswa untuk POS (kartu QR, RFID, atau QR di ponsel) | 124 | Sedang |
| OQ-31 | Kebijakan default akses data kesehatan dan konseling oleh kepala sekolah | 127, 130 | Tinggi |
| OQ-32 | Apakah pelacakan lokasi kendaraan secara langsung (GPS) diperlukan | 121 | Rendah |
| OQ-33 | Kebijakan default denda perpustakaan (bayar langsung atau ditagihkan) | 114 | Rendah |
| OQ-34 | Kebutuhan katalog perpustakaan digital (e-book) | 112 | Rendah |

## 10. Persetujuan Dokumen

| Peran | Nama | Tanggal | Tanda tangan |
|---|---|---|---|
| Product Owner | David Pambudi | | |
| Project Manager | David Pambudi | | |
| Tech Lead | [Nama] | | |
| Konsultan Pajak / Legal | [Nama] | | |

## Lampiran — Batas Tanggung Jawab Antar-Menu (Operasional)

| Topik | Pemilik kebutuhan | Menu/aturan lain hanya … |
|---|---|---|
| Ruang | Menu 116 | menu 39, 107, 117, dan modul pembelajaran memakai data ruang |
| Penyusutan aset | Accounting Engine | menu 108 menampilkan nilai buku; menu 107 menyimpan data fisik |
| Utang vendor | Menu 111 (pencocokan dan jadwal bayar) | Accounting Engine memegang buku besar utang; menu 106 memegang data vendor |
| Anggaran | Menu 109 | menu 104 dan 133 memakai pengecekan anggaran |
| Penerbitan tagihan non-SPP | Menu 19 | menu 114, 122, 134, 135 menentukan nilai dan pemicu tagihan |
| Top-up dompet | Menu 27–29 | menu 125 mengelola saldo dan batas |
| Pajak | Menu 110 (non-gaji) | menu 97 untuk PPh 21 pegawai |
| Tiket layanan | Menu 137–138 | menu 81 meneruskan permintaan orang tua ke Service Desk bila aktif |
| Perbaikan fasilitas | Menu 118 | menu 137 dapat mengonversi tiket menjadi perintah kerja |
| Prestasi siswa | Menu 136 | menu 51–53 menampilkan prestasi terverifikasi |
| Kehadiran siswa di sekolah | Menu 41 / 69 | menu 128 memperbarui status sakit; menu 121 dan 140 mencatat kehadiran perjalanan dan PKL secara terpisah |
