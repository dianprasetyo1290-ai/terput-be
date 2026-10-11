# Handoff Cek Gabungan Gelombang 1

**Branch:** `integrasi/prd-v1`  
**Tanggal Cek:** 2026-10-11  
**Operator:** Gemini (mode paralel GEMINI.md §7b)  
**Status Akhir:** ✅ SELESAI — semua perintah hijau

---

## 1. Kondisi Awal

Branch `integrasi/prd-v1` memuat hasil merge berurutan:

| Urutan | Branch | Merged Commit |
|---|---|---|
| 1 | `feat/T1-core` | `66a4bd1` |
| 2 | `feat/T2-hris` | `0e1ad4b` |
| 3 | `feat/T6-payment` | `27399f3` |
| 4 | `feat/T7a-transfer` | `90d52ea` |
| 5 | `feat/T8-audit` | `2194ec1` |

Working tree bersih saat cek dimulai.

---

## 2. Temuan Kerusakan Pasca Merge

### 2.1 ApprovalTimeline Placeholder Belum Disambungkan

Sesuai catatan handoff T2 §5 dan T7a §4, dua view menyimpan placeholder karena saat pengerjaan paralel komponen T1 belum tersedia:

| File | Placeholder | Catatan Handoff |
|---|---|---|
| `web/src/views/ess/EssLeaveView.vue` (baris 290) | `<!-- Sambungkan ApprovalTimeline setelah rebase T1 -->` + badge status cuti | T2 §5 poin 1 |
| `web/src/views/ess/EssLeaveView.vue` (baris 346) | `<!-- Sambungkan ApprovalTimeline setelah rebase T1 -->` + badge status lembur | T2 §5 poin 1 |
| `web/src/views/academic/StudentStatusRequestDetailView.vue` (baris 286) | `<span>[ApprovalTimeline: T1]</span>` | T7a §4 poin 1 |

**Tidak ditemukan kerusakan lain**: tidak ada import ganda, route ganda, tipe bentrok, atau konflik pada file shared (`client.ts`, `router/index.ts`, `AppLayout.vue`, `format.ts`).

---

## 3. Perbaikan yang Dilakukan

### 3.1 EssLeaveView.vue — Tab Cuti (Pengajuan Cuti)

- Dihapus: badge `<Badge :variant="...">` sementara + 2 komentar placeholder
- Ditambahkan: `<ApprovalTimeline source-service="hr" :source-id="req.id" />`
- Import `ApprovalTimeline` dari `../../components/core/ApprovalTimeline.vue` ditambahkan
- Import `Badge` (tidak lagi dipakai) dihapus dari destructured import `components/ui`

### 3.2 EssLeaveView.vue — Tab Lembur (Overtime)

- Dihapus: badge `<Badge class="capitalize" ...>` sementara + komentar placeholder
- Ditambahkan: `<ApprovalTimeline source-service="hr" :source-id="ot.id" />`

### 3.3 StudentStatusRequestDetailView.vue — Detail Pengajuan Transfer/Mutasi

- Dihapus: `<span class="...font-mono">[ApprovalTimeline: T1]</span>`
- Ditambahkan: `<ApprovalTimeline source-service="academic" :source-id="request.approval_id || String(request.id)" />`
- Import `ApprovalTimeline` dari `../../components/core/ApprovalTimeline.vue` ditambahkan

**Perubahan ini minimal**: hanya menyambungkan komponen yang sudah ada (T1), tidak menulis ulang logika manapun.

---

## 4. Output Verifikasi (Asli)

### 4.1 `go vet ./...` — Semua Service

```
=== academic ===
--- (EXIT 0)
=== admission ===
--- (EXIT 0)
=== attendance ===
--- (EXIT 0)
=== billing ===
--- (EXIT 0)
=== core ===
--- (EXIT 0)
=== evaluation ===
--- (EXIT 0)
=== exam ===
--- (EXIT 0)
=== hr ===
--- (EXIT 0)
=== identity ===
--- (EXIT 0)
=== notification ===
--- (EXIT 0)
=== report ===
--- (EXIT 0)
```
Semua **PASS** tanpa warning.

### 4.2 `go test ./...` — Semua Service

```
academic:     ok skci/academic/internal/common (cached)
              ok skci/academic/internal/handler (cached)
              ok skci/academic/internal/model (cached)
              ok skci/academic/internal/store (cached)
admission:    ok skci/admission/internal/client (cached)
              ok skci/admission/internal/conversion (cached)
              ok skci/admission/internal/model (cached)
              ok skci/admission/internal/otp (cached)
attendance:   ok skci/attendance/internal/client (cached)
              ok skci/attendance/internal/handler (cached)
billing:      ok skci/billing/internal/client (cached)
              ok skci/billing/internal/common (cached)
              ok skci/billing/internal/handler (cached)
              ok skci/billing/internal/model (cached)
              ok skci/billing/internal/nicepay (cached)
core:         ok skci/core/internal/common (cached)
              ok skci/core/internal/handler (cached)
              ok skci/core/internal/model (cached)
              ok skci/core/internal/store (cached)
evaluation:   ok skci/evaluation/internal/handler (cached)
exam:         ok skci/exam/internal/cache (cached)
              ok skci/exam/internal/common (cached)
              ok skci/exam/internal/config (cached)
              ok skci/exam/internal/handler (cached)
              ok skci/exam/internal/model (cached)
              ok skci/exam/internal/store (cached)
hr:           ok skci/hr/internal/crypto (cached)
              ok skci/hr/internal/handler (cached)
              ok skci/hr/internal/jobs (cached)
              ok skci/hr/internal/model (cached)
identity:     ok skci/identity/internal/common (cached)
              ok skci/identity/internal/handler (cached)
              ok skci/identity/internal/store (cached)
notification: ok skci/notification/internal/config (cached)
              ok skci/notification/internal/handler (cached)
              ok skci/notification/internal/provider (cached)
              ok skci/notification/internal/store (cached)
report:       ok skci/report/internal/cache (cached)
              ok skci/report/internal/docx (cached)
              ok skci/report/internal/handler (cached)
              ok skci/report/internal/model (cached)
              ok skci/report/internal/upstream (cached)
```
Semua **PASS** (EXIT 0 untuk semua 11 service).

### 4.3 Migrasi DB — `up → down 1 → up` (DB `skci_int_w1_*`)

DB lama di-drop dan dibuat ulang dari nol.

```
academic:     1/u→5/u  ✓  5/d ✓  5/u ✓
admission:    1/u→3/u  ✓  3/d ✓  3/u ✓
attendance:   1/u→8/u  ✓  8/d ✓  8/u ✓
billing:      1/u→4/u  ✓  4/d ✓  4/u ✓
core:         1/u      ✓  1/d ✓  1/u ✓
evaluation:   1/u→6/u  ✓  6/d ✓  6/u ✓
exam:         1/u→8/u  ✓  8/d ✓  8/u ✓
hr:           1/u      ✓  1/d ✓  1/u ✓
identity:     1/u→16/u ✓  16/d✓  16/u✓
notification: 1/u→2/u  ✓  2/d ✓  2/u ✓
report:       1/u→13/u ✓  13/d✓  13/u✓
```
Semua **PASS** — migrasi up/down bolak-balik bersih.

### 4.4 `npx vue-tsc -b` (Sebelum Perbaikan)

```
EXIT:0
```

### 4.5 `npm run build` (Sebelum Perbaikan)

```
✓ built in 2.10s
BUILD_EXIT:0
```

### 4.6 `npx vue-tsc -b` (Setelah Perbaikan ApprovalTimeline)

```
VUETSC_EXIT:0
```

### 4.7 `npm run build` (Setelah Perbaikan ApprovalTimeline)

```
✓ built in 1.83s
FINALBUILD_EXIT:0
```

---

## 5. File yang Diubah dalam Cek Gabungan Ini

| File | Perubahan |
|---|---|
| `web/src/views/ess/EssLeaveView.vue` | Sambungkan `ApprovalTimeline` (cuti + lembur), hapus `Badge` sementara, tambah import |
| `web/src/views/academic/StudentStatusRequestDetailView.vue` | Ganti span placeholder dengan `<ApprovalTimeline>`, tambah import |

---

## 6. Tidak Ada Kerusakan Tambahan

- **Shared files tidak berkonflik**: `web/src/api/client.ts`, `web/src/router/index.ts`, `web/src/layouts/AppLayout.vue`, `web/src/utils/format.ts`, `.env.example`, `docker-compose.yml` — semua bersih, tidak ada kunci/route ganda.
- **Tipe TS konsisten**: tidak ada `any`, tidak ada tipe bentrok antar-task.
- **Tidak ada kode mati**: semua import terpakai setelah cleanup `Badge`.
- **Migrasi identity**: T1 menambahkan `0014_hr_roles`, T2 menambahkan `0015_hris_users` — urutan dan konten keduanya tidak berkonflik, berjalan bersih hingga migrasi ke-16 (`deactivation_timestamp`).

---

## 7. Pertanyaan Terbuka / Asumsi

- `ApprovalTimeline` di `EssLeaveView` menggunakan `req.id` (UUID leave request) sebagai `source-id`. Backend `core` service akan menerima query `GET /approvals?source_service=hr&source_id=<uuid>` sesuai kontrak T1 §3. Jika di prod ternyata yang disimpan adalah `approval_request_id` terpisah (kolom di tabel HR), perlu update `source-id` ke kolom tersebut.
- Tidak ada T3/T4/T5 di gelombang ini (hanya T1, T2, T6, T7a, T8) — tidak ada handoff yang merujuk ApprovalTimeline selain yang sudah disambungkan.
