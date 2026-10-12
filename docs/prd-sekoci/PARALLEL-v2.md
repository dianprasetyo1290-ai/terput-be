# PRD v1.0 Bagian 3–4 (menu 102–159): rencana paralel

Lanjutan dari `PARALLEL.md` (T1–T8, Bagian 1–2). Sumber: `source/Master_PRD_SaaS_SEKOCI_Bagian3_v1.0.md` (menu 102–141) dan `source/Master_PRD_SaaS_SEKOCI_Bagian4_v1.0.md` (menu 142–159). Semua menu berlabel **Add-on**, jadi setiap route wajib punya `meta.feature`.

Aturan kerja tetap `GEMINI.md` (§7b mode paralel). Agen pelaksana: **subagent Claude**, masing-masing di git worktree sendiri. Branch dasar: `integrasi/prd-v2`.

## Pembagian task

| Task | Menu | Judul | Service (paket Go) | Gelombang |
|---|---|---|---|---|
| T9 | 116–118 | Fasilitas: master ruang, booking, perawatan + **kerangka service `ops`** + **semua peran & feature key Bagian 3–4** | `ops` (`internal/facility`), identity | 3 |
| T10 | 127–129 | Kesehatan sekolah + **kerangka service `studentlife`** | `studentlife` (`internal/health`) | 3 |
| T11 | 137–138 | Service Desk: tiket, kategori, SLA & eskalasi | `core` (`internal/servicedesk`) | 3 |
| T12 | 123–126 | Kantin, POS & dompet siswa | `billing` (`internal/canteen`, `internal/wallet`) | 3 |
| T13 | 147–149 | Komunikasi lanjutan: kampanye WA, chat guru–ortu, landing page & referral PPDB | `notification`, `admission` | 3 |
| T14 | 150–151 | Ekstensi pembayaran: multi provider, autodebit & gerai retail | `billing` (`internal/gateway`) | 3 |
| T15 | 102–106 | Inventory & pengadaan: barang, stok, opname, PR, PO, penerimaan, vendor | `ops` (`internal/inventory`, `internal/procurement`) | 4 |
| T18 | 112–115 | Perpustakaan | `ops` (`internal/library`) | 4 |
| T19 | 119–122 | Transportasi | `ops` (`internal/transport`) | 4 |
| T20 | 130–132 | Konseling & kesiswaan | `studentlife` (`internal/counseling`, `internal/discipline`) | 4 |
| T21 | 133–136 | Event, ekstrakurikuler & prestasi | `studentlife` (`internal/activity`) | 4 |
| T22 | 139–141 | PKL | `studentlife` (`internal/pkl`) | 4 |
| T16 | 107–108 | Register aset, penyusutan & mutasi | `ops` (`internal/asset`) | 5 |
| T17 | 109–111 | Ekstensi akuntansi: anggaran, pajak non-gaji & e-Faktur, utang usaha | `ops` (`internal/budget`, `internal/payable`) | 5 |
| T23 | 142–146 | QA & akreditasi + pelaporan pemerintah (Dapodik) | `quality` (baru) | 5 |
| T24 | 152–154 | BI, AI Assistant & analitik prediktif | `insight` (baru) | 5 |
| T25 | 155–158 | 3D Campus | `ops` (`internal/campus3d`) + web | 5 |
| T26 | 159 | Aplikasi white-label | `identity` + web (PWA) | 5 |

## Gelombang dan alasannya
| Gelombang | Task | Syarat mulai |
|---|---|---|
| **3** | T9, T10, T11, T12, T13, T14 | `integrasi/prd-v2` di-commit, toolchain lengkap (lihat bawah) |
| **4** | T15, T18, T19, T20, T21, T22 | gelombang 3 di-merge. T9 wajib (kerangka `ops`, master ruang, peran). T10 wajib (kerangka `studentlife`) |
| **5** | T16, T17, T23, T24, T25, T26 | gelombang 4 di-merge. T16/T17 butuh tabel PO & penerimaan dari T15. T23/T24 membaca data banyak modul |

Kontrak antar-task di gelombang yang sama ditulis di file task masing-masing dan diuji dengan fake (GEMINI.md §7b). Contoh: PR (T15) memanggil pengecekan anggaran lewat hook nil-safe `budget.Check` yang baru didaftarkan T17 di gelombang 5. 3D Campus (T25) membaca aset lewat endpoint T16 yang kontraknya ditulis di T16 §5.

## Port service baru
`ops` 8093 · `studentlife` 8094 · `quality` 8095 · `insight` 8096. Daftarkan sesuai GEMINI.md §1 (env, compose, `client.ts`, `web/.env*`, gateway nginx).

## Nomor migrasi yang dicadangkan (jangan dipakai task lain)
| Service | Nomor → task |
|---|---|
| ops | 0001–0009 T9 · 0010–0019 T15 · 0020–0029 T16 · 0030–0039 T17 · 0040–0049 T18 · 0050–0059 T19 · 0060–0069 T25 |
| studentlife | 0001–0009 T10 · 0010–0019 T20 · 0020–0029 T21 · 0030–0039 T22 |
| core | 0003 T11 |
| attendance | 0009 T10 (absen sakit dari UKS, `source_ref` idempoten) |
| billing | 0005 T12 · 0006 T14 · 0007 T12 (BILL-ADHOC: tagihan ad-hoc `source_module`/`source_ref`, dipakai T18, T19, T21; juga `ScanInput.vue` + `/internal/student-cards/resolve`) |
| notification | 0003 T13 · 0004 T26 (kanal push) |
| admission | 0004 T13 |
| identity | 0017 T9 (**semua** peran baru + feature key Bagian 3–4, sekaligus) · 0018 T26 |
| quality | 0001–0009 T23 |
| insight | 0001–0009 T24 |
| accounting (Laravel) | migrasi bertanggal, milik T17 (dan T12 bila perlu akun dana titipan) |

Hanya T9 yang menyentuh `CHECK (role IN (...))` dan daftar feature di identity. Task lain memakai nama peran/feature persis dari tabel di T9 §2.

## Keputusan terbuka (OQ-28 s.d. OQ-44): default konservatif, dicatat di handoff
| OQ | Default yang dipakai |
|---|---|
| OQ-28 tarif PPh non-gaji, status PKP | tabel tarif per sekolah dengan `effective_from`, di-seed dari regulasi + tanda "perlu validasi konsultan pajak". PKP = saklar per sekolah, default mati |
| OQ-29 regulasi dompet | dompet tertutup di lingkungan sekolah, saldo dicatat sebagai dana titipan (BR-WAL-02), batas saldo maksimum per sekolah dapat diatur |
| OQ-30 kartu POS | QR (statis per siswa, bisa dicabut). Input lewat keyboard wedge / kamera (IF-OPS-04). RFID tidak dikerjakan |
| OQ-31 akses kepala sekolah ke data kesehatan/konseling | default **tidak** punya akses. Bisa diberikan eksplisit lewat kebijakan per sekolah |
| OQ-32 GPS kendaraan | tidak dikerjakan |
| OQ-33 denda perpustakaan | parameter per sekolah (`pay_at_library` / `bill`), default `pay_at_library` |
| OQ-34 e-book | tidak dikerjakan |
| OQ-35 penyedia AI | adapter penyedia via env (default nonaktif). Kirim data minimum, log percakapan, tanpa pelatihan model |
| OQ-36 Dapodik | impor/ekspor file (CSV/XLSX) + perbandingan, tanpa web service langsung |
| OQ-37 instrumen akreditasi | master standar kosong + importer. Tidak ada instrumen bawaan yang dikarang |
| OQ-38 penyedia WA | adapter via env, default nonaktif (kampanye tetap bisa via email/in-app) |
| OQ-39 sumber model 3D | unggah file glTF/GLB per gedung + pemetaan ruang manual |
| OQ-40 teknologi white-label | PWA (manifest + ikon + warna per sekolah). Build native tidak dikerjakan |
| OQ-41 provider autodebit | kerangka adapter + NICEPay yang sudah ada. Provider lain ditambah lewat adapter |
| OQ-42 sensor digital twin | tidak dikerjakan. Menu 158 hanya overlay status ruang/perawatan dari data yang ada |
| OQ-43 opt-in kampanye | default opt-out (belum setuju = tidak dikirim, BR-COM-01) |
| OQ-44 etika model prediktif | prediksi hanya rekomendasi + label "perlu tinjauan", saklar per sekolah default mati |

## Siklus per gelombang (dijalankan Claude sebagai orkestrator)
1. Buat worktree per task dari `integrasi/prd-v2`: `git worktree add -b feat/<TASK>-<slug> ../skci-teratai-wt/<TASK> integrasi/prd-v2`.
2. Satu subagent per task mengerjakan file task-nya sampai `SELESAI <TASK>` / `BLOCKED <TASK>` (GEMINI.md §7b).
3. Merge berurutan ke `integrasi/prd-v2`, selesaikan konflik file bersama.
4. Cek gabungan: build + test semua service yang tersentuh, `handoff/W<N>-gabung.md`.
5. Setelah gelombang 5: uji integrasi docker + Playwright, `handoff/FINAL-v2.md`.

## Toolchain wajib di mesin
Go (sesuai `go.mod`), Node + npm, Docker Desktop/OrbStack, Postgres lokal (`createdb`), `golang-migrate` (`migrate`). Tanpa ini, tidak ada task yang bisa dinyatakan SELESAI.

## Catatan merge (dikerjakan orkestrator saat cek gabungan)
- **W3, billing (T12 + T14):** tambahkan wiring di `services/billing/cmd/server/main.go`: `app.PurposeCharger = app` dan `app.PurposeHandlers["wallet_topup"] = app.WalletTopupHook()` (T12 §4.6, T14 §4.6). `internal/jobs/runner.go` disalin identik oleh keduanya, jadi simpan satu.
- **W3, core:** T9 (`room_booking`), T10 (`health_profile_update`), T11, dan T12 (`canteen_settlement`, `wallet_transfer`) sama-sama menambah baris ke `KnownRequestTypes` di `services/core/internal/model/domain.go`. Gabungkan semua barisnya.
- **Pemilik kontrak bersama:** daftar wali `GET /internal/students/{id}/guardians` (identity) = T9. Akun peran eksternal `POST /internal/role-accounts/create-or-link` (identity) = T12. BILL-ADHOC + `ScanInput.vue` + resolve kartu siswa = T12. Metric dictionary = T24. Task lain hanya memakai kontrak ini.
- **Sebelum W3:** perbaiki temuan di `handoff/W2-temuan-awal.md` saat CEK-W2.
