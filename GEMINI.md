# GEMINI.md: Aturan Kerja skci-teratai

Repo ini adalah SEKOCI untuk Teratai Putih Global School (Go microservices, Vue 3, satu modul Laravel Accounting).
Kamu (Gemini) yang **menulis kode**. Claude yang **me-review**. Pekerjaan dianggap belum selesai sebelum Claude menyetujuinya.

Baca berurutan sebelum menulis kode:
1. File ini.
2. `docs/prd-sekoci/README.md`: peta gap PRD dan urutan task.
3. File task yang sedang dikerjakan: `docs/prd-sekoci/tasks/T0X-*.md`.
4. Bagian PRD yang dirujuk task itu: `docs/prd-sekoci/source/*.md` (FR-xx-xxx, AC-xx-xx, BR-xxx).

Jika file ini bertentangan dengan `docs/CONVENTIONS.md` atau `docs/FRONTEND_CONVENTIONS.md`, **file ini yang menang**. Dua dokumen itu ditulis saat rewrite awal dan sebagian sudah usang (contoh: sekarang frontend memakai Tailwind v4 dan `src/components/ui`; daftar port juga sudah berubah).

---

## 1. Standar = kode Sekoci yang sudah ada

Jangan mengarang pola baru. Tiru implementasi rujukan berikut:

| Kebutuhan | Tiru dari |
|---|---|
| Layout service Go | `services/billing/` (`cmd/server/main.go`, `internal/{common,config,handler,store,model,client,storage}`) |
| Auth JWT, envelope, helper HTTP | `services/billing/internal/common/{auth,response,http,internal}.go` (salin ke service baru, jangan impor lintas service) |
| Migrasi | `services/admission/migrations/` (golang-migrate, pasangan `NNNN_nama.up.sql`/`.down.sql`, nomor lanjut dari yang terakhir) |
| Uang, idempotensi, audit | `services/admission/migrations/0001_admission.up.sql`: `amount_idr bigint`, `idempotency_key` + `UNIQUE(school_id, idempotency_key)`, tabel `audit_events` (before/after jsonb + `correlation_id`) |
| Panggilan antar-service | `services/billing/internal/client/client.go` + header `X-Internal-Key` |
| Saklar modul per sekolah | `services/identity/migrations/0012_school_features.up.sql` + `meta.feature` di route (`web/src/router/index.ts`) |
| Peran baru | migrasi identity yang memperlebar `CHECK (role IN (...))` (lihat `0013_parent_portal.up.sql`) |
| Route + sidebar | `web/src/router/domains/billing.routes.ts` (`meta.role`, `meta.navGroup`, `meta.navLabel`, `meta.feature`) |
| Service frontend | `web/src/services/<domain>/` memanggil `api('<service>')` dari `web/src/api/client.ts` |
| Komponen UI | `web/src/components/ui/*` (button, card, dialog, table, tabs, input, label, alert, badge), `components/dashboard/{DashboardHero,StatCard}.vue` |

### Port (sumber kebenaran: `.env.example`)
identity 8081 · academic 8082 · attendance 8083 · exam 8084 · evaluation 8085 · report 8086 · accounting 8087 · admission 8088 · notification 8089 · billing 8090 · **core 8091 (baru, T1)** · **hr 8092 (baru, T2–T5)**

Service baru wajib didaftarkan di: `.env.example`, `docker-compose.yml` (+ service `-migrate`), `docker-compose.local.yml`, `web/src/api/client.ts` (`BASE_URLS`), `web/.env*` (`VITE_<SERVICE>_URL`), dan `db-init/` jika database dibuat di sana.

---

## 2. Kode bersih, mudah dirawat, bisa dipakai ulang

1. **Cari dulu, baru tulis.** Sebelum membuat helper, komponen, atau tabel, cari di repo (`grep -rn`). Kalau sudah ada, pakai. Kalau ada dua salinan yang mirip (contoh: `ConfirmDialog.vue` di `components/exam` dan `components/evaluation`), jangan buat salinan ketiga. Pakai `components/ui/dialog`.
2. **Satu fungsi, satu tugas.** Handler HTTP hanya parse, validasi, panggil store/logic, lalu tulis respons. Perhitungan (PPh 21, BPJS, alokasi pembayaran, saldo cuti) ditaruh di fungsi murni tanpa I/O supaya bisa dites tabel.
3. **Tanpa abstraksi spekulatif.** Jangan buat interface yang implementasinya cuma satu, factory, atau "framework" generik. Tiga baris yang mirip masih lebih baik daripada abstraksi yang prematur.
4. **Nama jelas, bahasa konsisten.** Tabel, kolom, dan kode Go memakai bahasa Inggris snake_case/camelCase mengikuti service yang ada. Label UI dan pesan error untuk pengguna memakai Bahasa Indonesia.
5. **Tidak ada kode mati.** Tidak boleh ada TODO kosong, fungsi yang tidak dipanggil, atau blok yang di-comment-out.
6. **Diff kecil dan terfokus.** Jangan memformat ulang file yang tidak kamu ubah. Jangan me-refactor modul di luar scope task.
7. **Dependensi baru** hanya boleh kalau stdlib atau dependensi yang sudah ada tidak cukup. Tulis alasannya di handoff.

## 3. Dilarang hardcode

| Jenis nilai | Tempatnya |
|---|---|
| URL, port, secret, kredensial, nama bucket, limit teknis | env melalui `internal/config/config.go`, default aman untuk dev saja |
| Aturan bisnis yang berbeda per sekolah (BR-GEN-08): kuota cuti, tarif BPJS, PTKP, tabel TER, batas retry, SLA, minimum daftar ulang, format file bank | **tabel DB** per `school_id` dengan `effective_from`/`effective_to` jika nilainya berubah dari waktu ke waktu. Isi awal di-seed lewat migrasi, lalu bisa diubah dari UI |
| Daftar pilihan (status, jenis dokumen, kategori) | tabel master atau `CHECK` constraint + satu konstanta Go/TS per domain. Jangan menyalin string literal yang sama ke banyak file |
| Warna, spacing | token di `web/src/style.css` / kelas Tailwind. Tidak boleh ada hex di komponen |
| ID (school, user, role) | selalu dari JWT/claims/DB. Tidak boleh menulis UUID literal di kode |

Selain itu juga **dilarang**: data demo/fallback di `web/src/services` (error harus tampil apa adanya), bypass OTP/kode rahasia, dan menu yang belum punya backend. Menu tanpa backend dihapus, bukan diberi data palsu.

## 4. Aturan backend Go

- chi v5 + pgx v5. Semua query diberi scope `school_id` dari claims. Superadmin boleh memakai `?school_id=`. Ikuti `EffectiveSchoolID` yang sudah ada.
- Otorisasi dicek di server per endpoint (peran dan kepemilikan data). Menyembunyikan menu di frontend tidak dihitung sebagai otorisasi (AC-CC-03).
- Uang: `bigint` rupiah, suffix kolom `_idr`. **Tidak boleh `float`.** Persentase disimpan dalam basis poin (`_bp`, 1 bp = 0,01%). Aturan pembulatan ditulis eksplisit dalam satu fungsi.
- Waktu: kolom `timestamptz`. Logika "hari ini" dan tanggal efektif memakai zona `Asia/Jakarta` dari config sekolah, bukan zona server.
- Data yang punya riwayat: soft delete dengan `deleted_at`, atau status nonaktif (BR-GEN-07). Perubahan yang berlaku per tanggal efektif disimpan sebagai baris riwayat baru, bukan UPDATE yang menimpa (BR-HR-02).
- Setiap mutasi menulis `audit_events` (actor, before/after, reason, correlation_id). Field sensitif (NIK, NPWP, rekening, gaji) **tidak boleh** muncul di log maupun di before/after dalam bentuk mentah (NFR-HR-01).
- Operasi yang bisa diulang (pembayaran, posting jurnal, konversi, payroll final) wajib idempoten lewat `idempotency_key` + UNIQUE constraint.
- Approval apa pun dijalankan lewat Approval Center (service `core`, T1). Jangan membuat tabel approval sendiri di service lain. Maker-checker: pembuat permintaan tidak boleh menyetujui permintaannya sendiri (BR-GEN-09).
- List endpoint memakai `?page=&per_page=` dan mengembalikan `meta`.

## 5. Aturan frontend Vue

- `<script setup lang="ts">`, tanpa `any`. Tipe respons ditaruh di `web/src/services/<domain>/types.ts`. Payload diambil dari `res.data.data`.
- Pakai `components/ui/*` dan Tailwind v4. Gotcha: CSS global input/select yang tidak di-layer mengalahkan utility class, jadi pakai `pl-10!` atau sejenisnya bila perlu.
- Setiap halaman wajib punya tiga state: **memuat** (skeleton/spinner), **kosong** (pesan + aksi berikutnya), **galat** (pesan dari API + tombol coba lagi) (CC-06).
- Aksesibilitas dan interaksi: setiap input punya `<label for>`. Fokus terlihat. Tombol ikon diberi `aria-label`. Target sentuh minimal 44px. Tombol dinonaktifkan selama request berjalan. Aksi destruktif memakai dialog konfirmasi. Warna tidak boleh jadi satu-satunya penanda status (badge harus berteks). Ikon memakai PrimeIcons/SVG, **bukan emoji**.
- Responsif di lebar 375/768/1024/1440 tanpa scroll horizontal di halaman. Tabel lebar di-scroll di dalam wrapper-nya sendiri.
- Format uang `Intl.NumberFormat('id-ID', { style: 'currency', currency: 'IDR', maximumFractionDigits: 0 })`, tanggal `id-ID` WIB. Buat formatter-nya sekali di `src/utils/format.ts` lalu pakai ulang.
- Dashboard dan laporan agregat menampilkan "Diperbarui: <waktu>" (CC-10).
- Setiap peran punya landing dan sidebar yang benar-benar berbeda. Menu modul add-on diberi `meta.feature`.

## 6. Mengambil kode dari proyek lain di `~/Documents/LMS`

Beberapa fitur sudah pernah dibuat di proyek lain (daftarnya per task ada di `docs/prd-sekoci/README.md`). Aturannya:
- **Ambil logika dan skema, jangan salin arsitekturnya.** Hasil akhir wajib mengikuti layout Sekoci (§1). Proyek gin+GORM (virelis, hris-mulnco, senb-v1, inquira) ditulis ulang ke chi+pgx.
- ATS-TIA memakai chi+pgx yang sama. Paket domainnya boleh dipindah lebih utuh, tetapi konsep `company_id`/tenant dan RLS diganti dengan `school_id` ala Sekoci, dan izin `authz` granular diganti dengan cek peran Sekoci.
- `float64` untuk uang (virelis) wajib diubah ke `int64` rupiah. Test dari proyek asal **ikut dipindahkan** dan harus tetap hijau setelah diporting. Itu bukti logikanya tidak berubah.
- Hapus semua yang khusus klien lain: nama BRI/Mul&Co/Virelis, branding, seed, email, domain, kredensial.
- Cantumkan file sumber di handoff (`asal: ../ATS-TIA/backend/internal/pipeline/move.go`) supaya reviewer bisa membandingkan.

## 7. Alur kerja dan serah terima ke Claude

0. Pastikan working tree bersih (`git status`). Jika belum bersih, **berhenti** dan minta pemilik repo meng-commit baseline. Jangan meng-commit perubahan orang lain.
1. Satu task = satu branch `feat/T0X-<slug>`, dibuat dari branch dasar yang di-commit pemilik repo (bukan selalu `main`). Commit kecil per langkah logis, **lokal saja**.
2. Kerjakan hanya file yang menjadi milik task. Kalau perlu mengubah file bersama (`client.ts`, `router/index.ts`, `docker-compose*.yml`, `AppLayout.vue`), batasi pada penambahan baris dan catat di handoff.
3. Sebelum serah terima, semua perintah berikut wajib hijau:
   ```bash
   cd services/<svc> && go vet ./... && go test ./...      # setiap service yang disentuh
   cd web && npx vue-tsc -b && npm run build
   docker compose -f docker-compose.local.yml up -d --build <svc> <svc>-migrate   # migrasi up
   # lalu jalankan migrasi down 1 langkah dan up lagi untuk membuktikan file .down.sql benar
   ```
   Uji klik di UI lewat Playwright (pola `web/e2e-teratai-*.mjs`) untuk setiap peran yang tersentuh.
4. Tulis `docs/prd-sekoci/handoff/T0X.md` berisi:
   - ringkasan + daftar file (baru/ubah) + file sumber porting
   - migrasi dan endpoint baru (method, path, peran)
   - **tabel cakupan**: setiap FR/AC di task → bukti (nama test atau langkah UI + hasil)
   - output perintah verifikasi (ringkas, tetapi asli)
   - penyimpangan dari spec, asumsi, pertanyaan terbuka
5. **Jangan deploy**, jangan menyentuh server, jangan mengubah `.env` produksi, dan jangan push. Deploy dilakukan setelah review Claude disetujui pemilik repo.

### 7b. Mode paralel (beberapa Gemini sekaligus, lihat `docs/prd-sekoci/PARALLEL.md`)
Jika prompt menyebut **mode paralel**, aturan ini menggantikan langkah 3 di atas:
- Kamu bekerja di **git worktree sendiri** (`../skci-teratai-wt/<TASK>`). Jangan pernah `cd` ke worktree lain atau repo utama.
- **Dilarang** `docker compose up`, `docker build`, atau menjalankan server di port tetap: agen lain sedang berjalan bersamaan dan port/nama container akan bentrok.
- Database uji: Postgres Homebrew lokal. Buat DB khusus task: `createdb skci_<task>_<svc>` (huruf kecil, mis. `skci_t2_hr`), lalu buktikan migrasi:
  ```bash
  DB="postgres://localhost/skci_t2_hr?sslmode=disable"
  migrate -path services/<svc>/migrations -database "$DB" up
  migrate -path services/<svc>/migrations -database "$DB" down 1
  migrate -path services/<svc>/migrations -database "$DB" up
  ```
  Untuk service yang sudah ada (identity, billing, academic, core), buat DB baru juga lalu jalankan **semua** migrasinya dari nol. Test Go yang butuh DB membaca DSN dari env `TEST_PG_DSN`. Jangan memakai testcontainers.
- Frontend: `cd web && npm ci` (sekali), lalu `npx vue-tsc -b && npm run build`. **Tanpa** dev server dan tanpa Playwright: uji UI end-to-end dijalankan sekali per gelombang setelah merge (prompt integrasi di PARALLEL.md).
- Ketergantungan ke task lain di gelombang yang sama: koding sesuai **kontrak tertulis** di file task (mis. T1 §3), uji dengan `httptest` fake. Komponen frontend milik task lain yang belum ada (mis. `ApprovalTimeline` dari T1): tampilkan badge status sementara dan tulis di handoff "sambungkan ApprovalTimeline setelah rebase". **Jangan** membuat salinan komponen milik task lain.
- File bersama (`docker-compose*.yml`, `.env.example`, `web/src/api/client.ts`, `web/src/router/index.ts`, `web/.env*`, `go.work` bila ada): hanya **tambah baris**, jangan menata ulang atau memformat ulang. Konflik merge diselesaikan Claude.
- Nomor migrasi sudah dicadangkan per task (tabel di PARALLEL.md). Pakai **hanya** nomormu.
- Jangan bertanya ke pengguna: tidak ada yang menjawab. Keputusan terbuka → pakai default di `docs/prd-sekoci/README.md` §5 atau pilihan paling konservatif, lalu catat di handoff.
- Selesai = semua perintah hijau, `handoff/<TASK>.md` ditulis, semua di-commit di branch-mu, lalu cetak baris terakhir `SELESAI <TASK>`. Jika buntu setelah 3 percobaan pada masalah yang sama, tulis handoff dengan status `BLOCKED` + detail, commit, lalu cetak `BLOCKED <TASK>`.

## 8. Yang diperiksa Claude saat review (lolos semua = disetujui)

- [ ] Semua FR/AC di task tercakup dan ada buktinya. Tidak ada fitur yang diklaim tanpa test atau langkah UI.
- [ ] Scope `school_id` + cek peran ada di setiap endpoint. Akses lintas sekolah/peran lewat API langsung ditolak.
- [ ] Tidak ada hardcode (§3), tidak ada float untuk uang, tidak ada data demo.
- [ ] Logika bisnis ada di fungsi murni dan dites tabel, termasuk kasus batas.
- [ ] Idempotensi, audit, soft delete, dan approval lewat `core` diterapkan di tempat yang diwajibkan.
- [ ] Migrasi up/down bisa dijalankan bolak-balik. Tidak ada perubahan destruktif pada data lama.
- [ ] Reuse: tidak ada helper/komponen duplikat. Pola sesuai §1.
- [ ] UI: tiga state, aksesibilitas §5, responsif, peran yang benar melihat menu yang benar.
- [ ] Build + test hijau dan bisa diulang dari handoff.
