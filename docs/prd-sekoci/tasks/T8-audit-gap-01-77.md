# T8: Audit gap FR menu 01–77 (laporan saja, TANPA mengubah kode)

**Branch:** `feat/T8-audit` (hanya menambah file di `docs/prd-sekoci/`) · Boleh paralel dengan task mana pun.
**Tujuan:** README §1 menandai 01–77 "Ada/Sebagian" hanya dari nama menu. Task ini memeriksa **setiap FR dan AC** Bagian 1 menu 01–77 terhadap kode nyata, supaya gap bisa dijadikan task berikutnya (T9+).

## Cara kerja
1. Untuk setiap menu 01–77 di `source/Master_PRD_SaaS_SEKOCI_Bagian1_v1.0.md`, ambil semua baris `FR-xx-yyy` dan `AC-xx-yy`.
2. Untuk setiap FR, cari bukti di kode: migrasi (tabel/kolom), handler (endpoint), route + view frontend. Gunakan `grep -rn` dan baca handlernya. Nama menu saja **bukan** bukti.
3. Tentukan status:
   - **Ada**: alur end-to-end ada (DB + API + UI, dan aturan bisnisnya ditegakkan di server).
   - **Sebagian**: ada sebagian (mis. UI saja, atau aturan tidak ditegakkan di server). Tulis bagian yang kurang.
   - **Belum**: tidak ada.
   - **Di luar scope**: FR ber-scope `Add-on`/phase yang jelas tidak relevan. Tulis alasannya.
4. Untuk AC: tulis apakah ada test yang membuktikannya (`file_test.go:baris`). Kalau tidak ada test, tulis "tanpa test".
5. Catat juga pelanggaran aturan lintas modul (CC-*, BR-GEN-*, NFR-SEC-*, NFR-AUD-01) yang terlihat sambil lalu: data uang float, authz hanya di frontend, data sensitif di log, hardcode nilai bisnis. Ini sangat berharga.

## Output: `docs/prd-sekoci/audit/01-77.md`
```
## 21. Billing (contoh)
| FR | Status | Bukti (file:baris) | Kekurangan |
|---|---|---|---|
| FR-21-001 | Ada | services/billing/internal/handler/admin.go:123 CreateScheme; web/src/views/billing/SchemeListView.vue | — |
| FR-21-004 | Sebagian | … | mapping COA per komponen (BR-FIN-06) tidak divalidasi saat generate |
| AC | Test |
| AC-21-01 | services/billing/internal/model/domain_test.go:67 |
```
Di akhir file:
- **Ringkasan** per grup menu: jumlah FR Ada / Sebagian / Belum.
- **Usulan task lanjutan** dikelompokkan per service, diurutkan prioritas M → S → C, dengan perkiraan ukuran (S/M/L).
- **Temuan lintas modul** dengan file:baris.

## Aturan
- **Jangan mengubah kode apa pun.** Kalau menemukan bug, tulis saja.
- Jangan menyimpulkan dari nama file atau komentar: buka isinya.
- Kalau ragu, status "Sebagian" + alasan, bukan "Ada".
- Tidak perlu Playwright. Bukti = path:baris.
