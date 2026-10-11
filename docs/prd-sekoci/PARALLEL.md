# Menjalankan Gemini secara paralel

## Gelombang
| Gelombang | Task (jalan bersamaan) | Syarat mulai |
|---|---|---|
| **1** | T1 core · T2 HRIS · T6 payment · T7a pindah/keluar + Tagihan Saya · T8 audit | baseline sudah di-commit |
| **2** | T3 payroll · T4 ATS · T5 pelatihan · T7b layanan ortu | gelombang 1 selesai & otomatis digabung ke `integrasi/prd-v1` (T1 & T2 wajib SELESAI) |

Kenapa tidak semua sekaligus? T3/T4/T5 butuh tabel pegawai & jabatan dari T2 (service `hr` yang sama), dan T7b butuh service `core` dari T1.

## Nomor migrasi yang dicadangkan (jangan dipakai task lain)
| Service | Nomor → task |
|---|---|
| core | 0001 T1 · 0002 T7b |
| hr | 0001–0009 T2 · 0010–0019 T3 · 0020–0029 T4 · 0030–0039 T5 |
| identity | 0014 T1 · 0015 T2 · 0016 T7a |
| billing | 0003 T6 · 0004 T7a |
| academic | 0005 T7a |
| accounting (Laravel) | migrasi bertanggal, milik T1 |

> Karena nomor `hr` diisi bergelombang, DB `hr` lokal di-reset (`migrate down -all && up`) setelah gelombang 2 di-merge. Belum ada produksi, jadi aman.

## Prompt tunggal (disarankan): satu kali tempel, semua jalan
Sekali saja, di terminal:
```bash
cd ~/Documents/LMS/skci-teratai
git add -A && git commit -m "baseline sebelum PRD v1.0"
gemini --yolo
```
Lalu tempel prompt ini ke Gemini:
```text
Kamu adalah ORKESTRATOR untuk repo skci-teratai. Tugasmu menjalankan seluruh PRD v1.0 (task T1–T8) memakai agen Gemini paralel yang sudah disiapkan skrip, lalu memastikan semuanya selesai. Kamu sendiri tidak menulis fitur.

1. Baca GEMINI.md (§7b) dan docs/prd-sekoci/PARALLEL.md supaya paham alurnya: gelombang 1 (T1,T2,T6,T7a,T8) paralel → gabung otomatis ke branch integrasi/prd-v1 → gelombang 2 (T3,T4,T5,T7b) paralel → gabung → uji integrasi akhir docker + Playwright.
2. Pastikan `git status --porcelain` kosong. Kalau tidak kosong, BERHENTI dan tulis "BLOCKED ORKESTRATOR: commit baseline dulu". Jangan meng-commit sendiri.
3. Jalankan: scripts/gemini-parallel.sh all
4. Pantau dengan `scripts/gemini-parallel.sh status` setiap 10 menit (pakai `sleep 600` di shell, satu perintah per cek). Jangan mengganggu worktree yang masih "berjalan".
5. Kalau ada task BLOCKED atau BERHENTI (lihat log di ../skci-teratai-wt/logs/<TASK>.log dan docs/prd-sekoci/handoff/<TASK>.md di worktree-nya):
   a. Baca penyebabnya. Kalau bisa diperbaiki tanpa keputusan pemilik (bug build/test, migrasi gagal, salah path), perbaiki di worktree ../skci-teratai-wt/<TASK> mengikuti GEMINI.md, verifikasi, commit, tulis "SELESAI <TASK>" sebagai baris terakhir di log-nya: echo "SELESAI <TASK>" >> ../skci-teratai-wt/logs/<TASK>.log
   b. Kalau tidak bisa, biarkan BLOCKED dan catat alasannya.
   c. Kalau fase berhenti ("BERHENTI: ..."), jalankan lagi `scripts/gemini-parallel.sh all` setelah perbaikan. Skrip melanjutkan: task yang sudah SELESAI tidak diulang. Untuk mengulang satu task dari nol, hapus log-nya dulu.
6. Ulangi sampai fase status berbunyi "SELESAI: semua di branch integrasi/prd-v1".
7. Tulis docs/prd-sekoci/handoff/ORKESTRATOR.md di worktree ../skci-teratai-wt/INT: tabel task → status → branch → catatan, daftar task yang tidak tergabung (file SKIPPED), ringkasan FINAL.md. Commit di branch integrasi/prd-v1.

Larangan: jangan push, jangan deploy, jangan mengubah .env produksi, jangan merge ke branch dasar (itu setelah Claude me-review), jangan bertanya ke pengguna. Baris terakhir output: "SELESAI ORKESTRATOR" atau "BLOCKED ORKESTRATOR".
```
Setelah Gemini menulis `SELESAI ORKESTRATOR`, bilang ke Claude: **"review semua"**. Claude me-review sekali di akhir (branch `integrasi/prd-v1`, checklist GEMINI.md §8).

Tanpa Gemini orkestrator juga bisa: `scripts/gemini-parallel.sh all` (jalan di background), pantau `scripts/gemini-parallel.sh status`. Log: `../skci-teratai-wt/logs/`. Model: `GEMINI_MODEL=gemini-2.5-pro scripts/gemini-parallel.sh all`.

## Cara pakai (manual, copy-paste ke beberapa jendela Gemini)
Buat worktree dulu untuk setiap task, lalu buka satu terminal per task:
```bash
cd ~/Documents/LMS/skci-teratai
git worktree add -b feat/T1-core ../skci-teratai-wt/T1 HEAD
cd ../skci-teratai-wt/T1 && gemini --yolo
```
Lalu tempel prompt di bawah (ganti `<TASK>` dan `<FILE>` sesuai tabel).

| TASK | Branch | FILE | Bagian |
|---|---|---|---|
| T1 | feat/T1-core | tasks/T1-core-approval-exception-audit.md | semua |
| T2 | feat/T2-hris | tasks/T2-hris.md | semua |
| T6 | feat/T6-payment | tasks/T6-payment-gateway.md | semua |
| T7a | feat/T7a-transfer | tasks/T7-transfer-layanan-tagihan-saya.md | §1 dan §3 saja |
| T8 | feat/T8-audit | tasks/T8-audit-gap-01-77.md | semua |
| T3 | feat/T3-payroll | tasks/T3-payroll.md | semua |
| T4 | feat/T4-ats | tasks/T4-ats.md | semua |
| T5 | feat/T5-training | tasks/T5-pelatihan-onboarding.md | semua |
| T7b | feat/T7b-layanan | tasks/T7-transfer-layanan-tagihan-saya.md | §2 saja |

### Prompt (sama untuk semua task)
```text
Kamu adalah engineer implementasi untuk repo skci-teratai. Kamu bekerja dalam MODE PARALEL: ada beberapa agen Gemini lain yang mengerjakan task berbeda di worktree lain pada saat yang sama.

Task kamu: <TASK>. File spesifikasi: docs/prd-sekoci/<FILE> (bagian: <BAGIAN>).

Langkah wajib:
1. Baca seluruh GEMINI.md (terutama §7b Mode paralel). Aturannya mengikat dan mengalahkan kebiasaanmu sendiri.
2. Baca docs/prd-sekoci/README.md, docs/prd-sekoci/PARALLEL.md (nomor migrasimu), lalu file spesifikasi task-mu sampai habis, lalu bagian PRD di docs/prd-sekoci/source/ yang dirujuk.
3. Sebelum menulis kode, baca kode acuan yang disebut di GEMINI.md §1 dan file donor yang disebut di task. Ikuti pola yang ada, jangan menciptakan pola baru.
4. Kerjakan backend dulu (migrasi → fungsi murni + test tabel → store → handler + test → integrasi), lalu frontend (types → service → route → view), sesuai urutan di file task.
5. Verifikasi sesuai GEMINI.md §7b: go vet + go test setiap service yang disentuh, migrasi up/down/up di DB Postgres lokal khusus task-mu, npx vue-tsc -b && npm run build. Semua harus hijau. Jangan pakai docker compose, jangan jalankan server.
6. Tulis docs/prd-sekoci/handoff/<TASK>.md (format GEMINI.md §7 langkah 4) dengan tabel FR/AC → bukti dan output asli perintah verifikasi.
7. Commit semua perubahan di branch saat ini (lokal, jangan push, jangan deploy). Baris terakhir output: "SELESAI <TASK>" atau "BLOCKED <TASK>".

Larangan: jangan bertanya ke pengguna (tidak ada yang menjawab, putuskan dengan default README §5 lalu catat). Jangan hardcode nilai bisnis. Jangan menyentuh file di luar cakupan task kecuali penambahan baris di file bersama. Jangan menyalin kredensial dari repo donor. Jangan menghapus atau melemahkan test yang ada.
```

## Uji integrasi (sudah otomatis di mode `all`, prompt ini hanya untuk mode manual)
```text
MODE INTEGRASI gelombang <N>. Semua branch gelombang ini sudah di-merge ke branch dasar. Jalankan di repo utama (bukan worktree):
1. docker compose -f docker-compose.local.yml up -d --build (semua service, termasuk *-migrate).
2. Pastikan semua *-migrate exit 0 dan semua service healthy.
3. Sambungkan semua catatan "setelah rebase" dari handoff gelombang ini (mis. ApprovalTimeline).
4. Jalankan uji Playwright end-to-end yang disebut di bagian "Bukti handoff" setiap task gelombang ini, headed, simpan screenshot ke docs/prd-sekoci/handoff/screens/.
5. Tulis docs/prd-sekoci/handoff/W<N>-integrasi.md: hasil per skenario + bug yang ditemukan. Perbaiki bug kecil, catat bug besar. Commit. Baris terakhir: "SELESAI W<N>".
```
