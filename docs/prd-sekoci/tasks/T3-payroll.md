# T3: Payroll di service `hr`: komponen, input, run & approval, slip, PPh 21 & BPJS, jurnal

**Menu PRD:** 93–98 (Bagian 2 §5 Grup 14, BR-PRL-01..04, IF-HR-01..04, NFR-HR-01) · **Branch:** `feat/T3-payroll` · **Service:** `hr` (paket `internal/payroll`) · **Migrasi:** `services/hr/migrations/0010_payroll.*` (nomor 0010–0019 milik T3)
**Prasyarat:** T1 (approval, exception, endpoint jurnal accounting §7b) dan T2 (employees, assignments terenkripsi, attendance_periods) sudah di-merge.
**Feature key:** `payroll`. Semua uang dalam `bigint` rupiah, persentase dalam basis poin (`_bp`, 100 bp = 1%). **Tidak ada float** di jalur uang.

## 1. Donor
| Ambil | Dari | Catatan |
|---|---|---|
| PPh 21 TER (PMK 168/2023) + progresif Pasal 17 + rekonsiliasi masa terakhir | `hris-mulnco/backend/internal/usecase/payroll/pph21.go`, `pph21_test.go`, `calculation_test.go` | ganti `shopspring/decimal` → int64 rupiah + bp. **Angka tarif TIDAK disalin ke kode**: struktur bracket diisi dari tabel `statutory_*` (§3). Test donor dipindah dan disesuaikan (input dari fixture tabel) |
| BPJS | `virelis/backend/internal/utils/payroll.go` (`CalcBPJS`, `DefaultPayrollRules`) | `DefaultPayrollRules` **dibuang** (itu hardcode). Logika batas upah dipakai |
| State run, slip PDF, file bank, rekap pajak | `virelis/backend/internal/handlers/payroll.go` (`GeneratePayrollRun`, `buildPayslip`, `PayrollTransition`, `buildPayslipPDF` gofpdf, `BankTransferExport`, `TaxSummary`) | transisi lewat core approval; format bank jadi template (OQ-22) |

## 2. Migrasi `0010_payroll.up.sql`
```
pay_components(id, school_id, code, name, kind CHECK IN('earning','deduction'),
  nature CHECK IN('fixed','variable'), taxable bool, bpjs_base bool,
  calc CHECK IN('fixed_amount','per_hour','from_structure','from_input','overtime'),
  default_amount_idr bigint NULL, rate_per_unit_idr bigint NULL,   -- per_hour: tarif per jam (AC-94-01)
  max_input_idr bigint NULL,                                       -- batas validasi FR-94-003
  sort int, status CHECK IN('draft','pending_approval','active','retired'), approval_id NULL, deleted_at,
  UNIQUE(school_id, code))
pay_component_accounts(id, school_id, component_id, acc_code text NOT NULL, liability_acc_code text NULL,
  UNIQUE(school_id, component_id))                                 -- BR-PRL-04 (unit = school_id)
payroll_accounts(school_id PK, salary_payable_acc text, pph21_payable_acc text, bpjs_payable_acc text,
  bpjs_employer_expense_acc text, bank_acc text)                   -- akun utang & kas untuk jurnal (FR-98-001/002)
salary_structures(id, school_id, position_id NULL, grade text NULL, employment_type NULL,
  base_min_idr bigint, base_max_idr bigint, effective_from date, effective_to NULL, status, approval_id)
employee_components(id, school_id, employee_id, component_id, amount_idr bigint, effective_from, effective_to NULL)  -- tunjangan tetap per pegawai

-- Parameter statutori, effective-dated, diubah lewat approval (FR-93-003, FR-93-006)
statutory_bpjs(id, school_id NULL /* NULL = default nasional, sekolah boleh override */, program CHECK IN('kesehatan','jht','jp','jkk','jkm'),
  employer_bp int, employee_bp int, wage_cap_idr bigint NULL, effective_from date, effective_to NULL, source_note text NOT NULL)
statutory_ptkp(id, status_code text, annual_idr bigint, effective_from, effective_to NULL, source_note)
statutory_ter_categories(id, category CHECK IN('A','B','C'), ptkp_status_code text, effective_from, effective_to NULL)
statutory_ter_brackets(id, category, lower_idr bigint, upper_idr bigint NULL, rate_bp int, effective_from, effective_to NULL)
statutory_progressive(id, lower_idr, upper_idr NULL, rate_bp, effective_from, effective_to NULL)
statutory_misc(id, key CHECK IN('biaya_jabatan_bp','biaya_jabatan_max_annual_idr'), value bigint, effective_from, effective_to NULL)

payroll_calendars(id, school_id, period char(7), cutoff_date date, pay_date date, UNIQUE(school_id, period))
payroll_inputs(id, school_id, period, employee_id, component_id, quantity numeric(10,2) NULL, amount_idr bigint,
  source CHECK IN('manual','import','attendance'), locked bool default false, created_by,
  UNIQUE(school_id, period, employee_id, component_id))
payroll_input_changes(id, school_id, input_id, before jsonb, after jsonb, reason, status, approval_id)   -- FR-94-004

payroll_runs(id, school_id, period, kind CHECK IN('regular','supplementary'),
  status CHECK IN('draft','calculated','pending_approval','approved','final','rejected','cancelled'),
  approval_id NULL, calculated_at, finalized_at, finalized_by, params_snapshot jsonb NOT NULL,
  totals jsonb, idempotency_key, UNIQUE(school_id, idempotency_key))
CREATE UNIQUE INDEX one_regular_run ON payroll_runs(school_id, period) WHERE kind='regular' AND status NOT IN ('rejected','cancelled')   -- BR-PRL-03, AC-95-04
payroll_lines(id, school_id, run_id, employee_id, gross_idr bigint, taxable_idr bigint, bpjs_base_idr bigint,
  bpjs_employee_idr bigint, bpjs_employer_idr bigint, pph21_idr bigint, other_deductions_idr bigint, net_idr bigint,
  prev_net_idr bigint NULL, variance_bp int NULL, flagged bool,
  details jsonb NOT NULL,        -- per komponen: {code,name,kind,amount_idr}
  bank_snapshot_enc bytea,       -- rekening saat run (AC-87-01: rekening baru belum disetujui tidak ikut)
  payment_status CHECK IN('unpaid','paid','failed') default 'unpaid', UNIQUE(run_id, employee_id))
payslips(id, school_id, line_id UNIQUE, object_key, sha256, published_at NULL, reissued_count int default 0)
payslip_templates(school_id PK, header_text, footer_text, logo_object_key NULL, show_employer_contrib bool)
bank_file_templates(id, school_id, name, delimiter text, header_row bool, columns jsonb NOT NULL, encoding text default 'utf-8')
  -- columns: [{"header":"No Rek","field":"account_number"},{"header":"Nominal","field":"net_idr","format":"plain"}]
tax_remittances(id, school_id, period, kind CHECK IN('pph21','bpjs_kesehatan','bpjs_tk'), amount_idr, paid_on, ref)
payroll_journals(id, school_id, run_id, kind CHECK IN('accrual','payment'), reference text UNIQUE,
  status CHECK IN('pending','posted','failed'), trans_no NULL, lines jsonb, error NULL, attempts int default 0)
```
Seed parameter statutori (nasional, `school_id NULL`) **di migrasi terpisah** `0011_statutory_seed.up.sql` dengan nilai dari regulasi resmi, beserta `source_note` (mis. "PMK 168/2023 Lampiran A", "Perpres 59/2024", "PP 44/2015"). Komentar SQL di atasnya: `-- PERLU VALIDASI KONSULTAN PAJAK (OQ-19/20)`. Kalau ragu soal satu angka, **jangan isi**: tulis di handoff sebagai item terbuka. Run akan gagal 422 "parameter X belum dikonfigurasi", dan itu benar.

## 3. Fungsi murni (`internal/payroll/calc`), inti task ini
```go
type Params struct { BPJS []BPJSRule; TER map[Category][]Bracket; TERCat map[string]Category; Progressive []Bracket; PTKP map[string]int64; BiayaJabatanBP int; BiayaJabatanMaxAnnual int64 }
func LoadParams(ctx, db, schoolID, onDate) (Params, error)   // ambil baris yang berlaku pada tanggal akhir periode (BR-PRL-01). Override sekolah menang atas default
func CalcBPJS(baseIDR int64, r BPJSRule) (employee, employer int64)          // base dibatasi wage_cap; floor ke rupiah
func CalcPPh21Monthly(grossIDR int64, ptkp string, p Params) int64           // TER: bracket kategori × gross
func CalcPPh21December(annual AnnualInput, p Params) (taxDec int64)          // progresif setahun − PPh Jan–Nov
func CalcLine(emp EmployeeSnapshot, inputs []Input, p Params) Line
func Variance(prev, cur int64) (bp int, flagged bool)  // threshold dari setting, default 1000 bp (10%) di tabel payroll_settings, bukan konstanta
```
Pembulatan: **floor ke rupiah** di setiap komponen (catat di handoff, OQ-19). Vektor uji wajib:
| ID | Input | Hasil |
|---|---|---|
| AC-95-01 | base 5.000.000, kesehatan employer 400bp, employee 100bp, cap 12.000.000 | employee 50.000, employer 200.000 |
| cap | base 15.000.000, sama | employee 120.000, employer 480.000 |
| AC-95-02 | gross 10.000.000, bracket 200bp untuk kategori pegawai | 200.000 |
| AC-94-01 | per_hour tarif 50.000 × 40 jam | 2.000.000 |
| AC-95-03 | prev 6.000.000, cur 7.200.000 | variance 2000bp, flagged |
| AC-93-01 | cap berubah 1 Jan, hitung periode Des | memakai cap lama |
| Desember | port test donor `pph21_test.go` (kasus TK/0, K/1, pegawai masuk tengah tahun) | sama seperti donor |
| AC-98-01 | beban 150.000.000, PPh 5.000.000, BPJS 3.000.000 | utang gaji 142.000.000, debit=credit |

## 4. Alur & endpoint
| Method & path | Peran | Perilaku |
|---|---|---|
| `CRUD /payroll/components` (+ `/accounts`) | payroll_officer (approve lewat core: hr_manager, finance_manager) | perubahan komponen aktif → `pending_approval` (`payroll_component_change`). Versi lama tetap dipakai sampai disetujui |
| `CRUD /payroll/structures`, `/payroll/employee-components` | sama | effective-dated |
| `GET/PUT /payroll/statutory/*` | payroll_officer (tulis), finance_manager | tulis = baris baru dengan `effective_from`, tidak update di tempat. Lewat approval |
| `CRUD /payroll/calendars`, `/payroll/settings`, `/payroll/bank-templates`, `/payroll/payslip-template`, `/payroll/accounts` | payroll_officer | |
| `POST /payroll/inputs/pull-attendance?period=` | payroll_officer, hr_admin | FR-94-001: tarik dari `attendance_periods` status `sent`/`locked` (status lain → 409 "rekap absensi belum dikirim HR"): lembur → komponen `overtime`, alpa → potongan jika komponen dikonfigurasi |
| `GET/PUT /payroll/inputs?period=` · `POST /payroll/inputs/import` | sama | validasi FR-94-003: pegawai aktif pada periode (AC-94-02 "Pegawai tidak aktif pada periode"), komponen `from_input`/`per_hour`, nilai ≤ `max_input_idr`. Setelah `cutoff_date` → `payroll_input_changes` + approval |
| `POST /payroll/runs` `{period, kind}` | payroll_officer | cek BR-PRL-04: komponen tanpa akun → 422 berisi daftar komponen (AC-93-02). Regular ganda → 409 "Payroll reguler periode ini sudah ada, gunakan run susulan" (AC-95-04) |
| `POST /payroll/runs/{id}/calculate` | payroll_officer | hitung semua pegawai aktif dalam satu transaksi, simpan `params_snapshot`. Bisa diulang selama `draft/calculated` |
| `GET /payroll/runs/{id}` · `/lines?flagged=` | payroll_officer, hr_manager, finance_manager | ringkasan + perbandingan periode lalu |
| `POST /payroll/runs/{id}/submit` | payroll_officer | → core approval `payroll_run`, `amount_idr` = total net. Callback approved → `approved`; payroll_officer lalu `POST /finalize` |
| `POST /payroll/runs/{id}/finalize` | payroll_officer | `final` (immutable: trigger DB menolak UPDATE pada `payroll_lines` jika run final, kecuali `payment_status`), buat slip, jurnal accrual |
| `GET /payroll/runs/{id}/bank-file?template_id=` | payroll_officer, finance_manager | file sesuai template. Diaudit karena berisi rekening |
| `POST /payroll/runs/{id}/payments` `{employee_ids, status}` | finance_manager | status bayar. Semua `paid` → jurnal payment (FR-98-002) |
| `POST /payroll/payslips/publish?run_id=` · `POST /payroll/payslips/{id}/reissue` | payroll_officer | publish → notifikasi pegawai. Reissue = render ulang dari `payroll_lines` yang sama, sha256 harus identik (FR-96-004) |
| `GET /me/payslips` · `GET /me/payslips/{id}/pdf` | pemilik | hanya `published_at IS NOT NULL` (AC-96-01). Selain pemilik dan peran payroll → 403 (AC-96-02) |
| `GET /payroll/reports/pph21?period=` · `/pph21/export?format=` · `/pph21/annual?year=` · `/bpjs?period=` · `/reconciliation?period=` | payroll_officer, finance_manager | AC-97-01 per unit & NPWP pemotong (NPWP pemotong dari `hr_settings.employer_npwp_enc`, tambahkan kolom). Rekonsiliasi = total potongan vs `tax_remittances` (AC-97-02). Format e-Bupot: generator CSV per definisi kolom di tabel `tax_export_templates` (sama seperti bank, OQ-19) |
| `GET /payroll/journals?period=` · `POST /payroll/journals/{id}/retry` (juga `retry_url` untuk exception) | payroll_officer, finance_manager | FR-98-004 |

**Jurnal:** `BuildAccrualJournal(run, mapping) []JournalLine`. Debit beban per komponen earning (+ beban BPJS employer), kredit utang gaji (net), utang PPh 21, utang BPJS (employee + employer), dan potongan lain ke `liability_acc_code` komponen. Total wajib seimbang (test). Kirim ke `POST {ACCOUNTING_URL}/api/internal/journals` dengan reference `payroll:<run_id>:accrual`. Gagal → `failed`, lalu `POST {CORE_URL}/internal/exceptions` (kind `journal_posting`, retry_url ke endpoint retry). Job retry otomatis dengan backoff.

## 5. Frontend
`services/payroll/*.ts`, `router/domains/payroll.routes.ts`, `meta.feature: 'payroll'`, navGroup **Payroll**.
| Path | View (`views/payroll/…`) | Peran | Label |
|---|---|---|---|
| `payroll/komponen` | `ComponentsView.vue` (tab Komponen / Mapping Akun / Struktur Gaji / Tunjangan Pegawai) | payroll_officer, hr_manager, finance_manager | Komponen & Pengaturan |
| `payroll/parameter` | `StatutoryView.vue` (tab BPJS / PTKP / TER / Progresif, timeline per tanggal berlaku, banner "perlu validasi konsultan pajak" jika `source_note` kosong) | payroll_officer, finance_manager | Parameter PPh 21 & BPJS |
| `payroll/input` | `InputsView.vue` (pilih periode; grid pegawai × komponen yang bisa diedit (inline, Enter/Tab); tombol Tarik Absensi, Import, unduh template; banner "Terkunci setelah cut-off" + alur ajukan perubahan) | payroll_officer, hr_admin | Input Payroll |
| `payroll/run` · `payroll/run/:id` | `RunsView.vue`, `RunDetailView.vue` (stepper Draft → Dihitung → Approval → Final → Dibayar; StatCard total gross/BPJS/PPh/net; tabel baris dengan filter "Selisih > 10%"; drawer rincian per pegawai; `ApprovalTimeline`; tombol sesuai status, setiap aksi dengan dialog konfirmasi) | payroll_officer, hr_manager, finance_manager | Payroll Run |
| `payroll/slip` | `PayslipsView.vue` (per run: publikasikan, terbitkan ulang, pratinjau PDF) | payroll_officer | Slip Gaji |
| `payroll/laporan` | `TaxReportsView.vue` (tab PPh 21 bulanan / Tahunan & bukti potong / BPJS / Rekonsiliasi setoran) | payroll_officer, finance_manager | PPh 21 & BPJS |
| `payroll/jurnal` | `JournalsView.vue` (status posting per periode, tombol coba lagi) | payroll_officer, finance_manager | Jurnal Payroll |
| `pegawai/slip` | `ess/EssPayslipsView.vue` (daftar slip milik sendiri + unduh) | pegawai, guru (ber-employee) | Pegawai / Slip Gaji |
Isi juga kartu "Slip terbaru" di `EssDashboardView` (T2) memakai endpoint `/me/payslips?limit=1`.
Angka uang memakai formatter rupiah bersama, rata kanan, `tabular-nums`.

## 6. Bukti handoff
Semua vektor §3 hijau. Test immutability run final, run ganda, komponen tanpa akun, slip milik orang lain → 403, reissue sha256 sama, jurnal idempoten (kirim 2× → 1 trans_no). Playwright: input → run → approval 2 tahap → final → slip tampil di ESS. Daftar parameter statutori yang di-seed beserta sumbernya + item yang dikosongkan.
