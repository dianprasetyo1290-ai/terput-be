<?php

namespace Database\Seeders;

use App\Support\AuditLog;
use App\Support\CoaTemplates;
use App\Support\Ledger;
use App\Support\PeriodReopen;
use App\Support\RecurringJournals;
use Illuminate\Database\Seeder;
use Illuminate\Support\Carbon;
use Illuminate\Support\Facades\DB;

/**
 * Demo data for the accounting features of PRD Bagian 1 (menu 32-38) in the
 * CURRENT tenant schema (search_path set by `accounting:seed-demo`).
 *
 * Strictly ADDITIVE and idempotent:
 *  - never truncates/deletes; never calls ModulesSeeder/DatabaseSeeder;
 *  - master rows use insertOrIgnore on their unique key;
 *  - feature rows are looked up by a natural key (template name, journal
 *    number, reason text, ...) and only created when missing, together with
 *    their audit-log rows -- a second run changes nothing;
 *  - the only UPDATEs are the two flags the demo asks for (1201 = akun
 *    kontrol, 1301/2301 = antar unit), applied only when not yet set.
 *
 * Maker/checker are existing accounting users (mst_user; trans_head.trans_user
 * has a FK to it), so no user is created: maker prefers 0003 (Accounting),
 * checker 0004 (Finance). A Sekoci admin logging in via SSO is neither, so it
 * can approve/reject the pending demo journals itself.
 */
class DemoFeaturesSeeder extends Seeder
{
    /** @var array<string,int> what this run created (for the command output) */
    public array $created = [];

    private string $maker;

    private ?string $checker;

    private ?string $currency;

    /** @var array{0:string,1:string} active period [from, to] */
    private array $range;

    public function run(): void
    {
        $this->created = [];
        DB::transaction(function () {
            $this->masters();
            $this->accounts();
            if (! $this->users()) {
                $this->created['dilewati (belum ada user di mst_user)'] = 1;

                return;
            }
            $this->activeRange();
            $templates = $this->journalTemplates();
            $this->journals();
            $this->recurring($templates);
            $this->coaTemplates();
            $this->periodReopen();
        });
    }

    private function bump(string $what, int $n = 1): void
    {
        if ($n > 0) {
            $this->created[$what] = ($this->created[$what] ?? 0) + $n;
        }
    }

    // ---------------------------------------------------------------- masters

    private function masters(): void
    {
        $now = now();
        $cur = (string) (DB::table('parameter')->value('base_currency') ?: 'Rp');
        $this->bump('mst_currency', DB::table('mst_currency')->insertOrIgnore([
            'curr_code' => $cur, 'curr_name' => 'Rupiah', 'rate_system' => 'N', 'created_at' => $now, 'updated_at' => $now,
        ]));
        $this->currency = $cur;

        $this->bump('mst_division', DB::table('mst_division')->insertOrIgnore([
            ['div_code' => 'SD', 'div_name' => 'Unit SD', 'created_at' => $now, 'updated_at' => $now],
            ['div_code' => 'SMP', 'div_name' => 'Unit SMP', 'created_at' => $now, 'updated_at' => $now],
        ]));

        $this->bump('mst_transaction', DB::table('mst_transaction')->insertOrIgnore([
            ['trans_code' => 'JU', 'trans_name' => 'Jurnal Umum', 'trans_status' => '1', 'created_at' => $now, 'updated_at' => $now],
            ['trans_code' => 'BKM', 'trans_name' => 'Bukti Kas Masuk', 'trans_status' => '1', 'created_at' => $now, 'updated_at' => $now],
            ['trans_code' => 'BKK', 'trans_name' => 'Bukti Kas Keluar', 'trans_status' => '1', 'created_at' => $now, 'updated_at' => $now],
        ]));

        $groups = [1 => 'Asset', 2 => 'Liability', 3 => 'Equity', 4 => 'Income', 5 => 'Cost of Sales', 6 => 'Expense', 7 => 'Other Income', 8 => 'Other Expense'];
        foreach ($groups as $id => $name) {
            $this->bump('account_group_tables', DB::table('account_group_tables')->insertOrIgnore(['id' => $id, 'name' => $name, 'created_at' => $now, 'updated_at' => $now]));
        }
    }

    /** [code, name, group, is_cash] -- flat (no parents) detail accounts. */
    public const ACCOUNTS = [
        ['1101', 'Kas', 1, true],
        ['1102', 'Bank', 1, true],
        ['1104', 'Kliring Payment Gateway', 1, false],
        ['1201', 'Piutang Siswa', 1, false],
        ['1202', 'Piutang Tenant Kantin', 1, false],
        ['1301', 'Piutang Antar Unit', 1, false],
        ['1501', 'Gedung & Peralatan', 1, false],
        ['1502', 'Akumulasi Penyusutan', 1, false],
        ['2201', 'Titipan Uang Saku', 2, false],
        ['2202', 'Tabungan Siswa', 2, false],
        ['2203', 'Deposit Siswa', 2, false],
        ['2204', 'Utang Tenant Kantin', 2, false],
        ['2205', 'Utang Refund', 2, false],
        ['2301', 'Utang Antar Unit', 2, false],
        ['3101', 'Aset Neto', 3, false],
        ['4101', 'Pendapatan SPP', 4, false],
        ['4102', 'Pendapatan Uang Pangkal', 4, false],
        ['4103', 'Pendapatan Kegiatan', 4, false],
        ['4104', 'Pendapatan Seragam & Buku', 4, false],
        ['4105', 'Pendapatan Ekstrakurikuler', 4, false],
        ['4106', 'Pendapatan Sewa Kantin', 4, false],
        ['4107', 'Pendapatan Donasi', 4, false],
        ['4108', 'Pendapatan Jasa Giro', 7, false],
        ['4109', 'Pendapatan Lain-lain', 7, false],
        ['6101', 'Beban Gaji', 6, false],
        ['6102', 'Beban ATK & Perlengkapan', 6, false],
        ['6201', 'Beban Listrik & Air', 6, false],
        ['6301', 'Biaya Payment Gateway', 6, false],
        ['6401', 'Beban Beasiswa', 6, false],
        ['6402', 'Potongan Tagihan', 6, false],
        ['6501', 'Beban Penyusutan', 6, false],
    ];

    private static function normal(string $code, int $group): string
    {
        if ($code === '1502') {
            return 'K'; // contra asset
        }

        return in_array($group, [1, 5, 6, 8], true) ? 'D' : 'K';
    }

    private function accounts(): void
    {
        $now = now();
        foreach (self::ACCOUNTS as [$code, $name, $group, $cash]) {
            $this->bump('mst_account', DB::table('mst_account')->insertOrIgnore([
                'acc_code' => $code, 'acc_name' => $name, 'acc_group' => $group, 'acc_currency' => $this->currency,
                'normal_balance' => self::normal($code, $group), 'is_active' => true, 'is_cash' => $cash,
                'is_interunit' => in_array($code, ['1301', '2301'], true), 'is_control' => $code === '1201',
                'created_at' => $now, 'updated_at' => $now,
            ]));
        }

        // Requested demo flags; only flipped when not set yet (no-op on re-run).
        foreach (['1201' => 'is_control', '1301' => 'is_interunit', '2301' => 'is_interunit'] as $code => $flag) {
            $n = DB::table('mst_account')->where('acc_code', $code)
                ->where(fn ($q) => $q->whereNull($flag)->orWhere($flag, false))
                ->update([$flag => true, 'updated_at' => $now]);
            if ($n) {
                AuditLog::record('coa.update', $code, ['before' => [$flag => false], 'after' => [$flag => true], 'by' => 'seed-demo'], 'SYSTEM');
                $this->bump("mst_account.$flag diaktifkan");
            }
        }
    }

    private function users(): bool
    {
        $ids = DB::table('mst_user')->whereNull('deleted_at')->orderBy('user_id')->pluck('user_id')->map(fn ($v) => (string) $v)->all();
        if (! $ids) {
            return false;
        }
        $this->maker = in_array('0003', $ids, true) ? '0003' : $ids[0];
        $others = array_values(array_diff($ids, [$this->maker]));
        $this->checker = in_array('0004', $others, true) ? '0004' : ($others[0] ?? null);

        return true;
    }

    private function activeRange(): void
    {
        $a = Ledger::active();
        $this->range = Ledger::range($a['year'], $a['period']) ?? (function () use ($a) {
            $m = Carbon::create($a['year'], min(12, max(1, $a['period'])), 1);

            return [$m->toDateString(), $m->copy()->endOfMonth()->toDateString()];
        })();
    }

    /** A day inside the active period (clamped to its last day). */
    private function day(int $d): string
    {
        $date = Carbon::parse($this->range[0])->addDays($d - 1)->toDateString();

        return $date > $this->range[1] ? $this->range[1] : $date;
    }

    // ------------------------------------------------------- journal templates

    /** @return array<string,int> key => template id */
    private function journalTemplates(): array
    {
        $defs = [
            'gaji' => ['Gaji Guru & Karyawan Bulanan', 'JU', true, 'Pembayaran gaji bulanan guru dan karyawan unit SD dan SMP melalui transfer bank.', [
                ['6101', 'SD', 'Gaji guru & karyawan SD', 38500000, 0],
                ['6101', 'SMP', 'Gaji guru & karyawan SMP', 45000000, 0],
                ['1102', 'SD', 'Transfer gaji SD (Bank)', 0, 38500000],
                ['1102', 'SMP', 'Transfer gaji SMP (Bank)', 0, 45000000],
            ]],
            'listrik' => ['Listrik, Air & Internet', 'BKK', true, 'Tagihan PLN, PDAM dan internet sekolah per bulan.', [
                ['6201', 'SMP', 'Listrik PLN & air PDAM', 5250000, 0],
                ['6201', 'SMP', 'Internet sekolah 200 Mbps', 1500000, 0],
                ['1102', 'SMP', 'Pembayaran via bank', 0, 6750000],
            ]],
            'susut' => ['Penyusutan Gedung & Peralatan', 'JU', true, 'Penyusutan bulanan aset tetap (garis lurus).', [
                ['6501', 'SMP', 'Penyusutan gedung & peralatan', 4200000, 0],
                ['1502', 'SMP', 'Akumulasi penyusutan', 0, 4200000],
            ]],
            'giro' => ['Jasa Giro & Biaya Admin Bank', 'BKM', true, 'Bunga jasa giro dan potongan administrasi rekening operasional.', [
                ['1102', 'SMP', 'Jasa giro bersih', 287500, 0],
                ['6301', 'SMP', 'Biaya administrasi bank', 12500, 0],
                ['4108', 'SMP', 'Pendapatan jasa giro', 0, 300000],
            ]],
            'ekskul' => ['Honor Pelatih Ekstrakurikuler (TA 2025/2026)', 'BKK', false, 'Tidak dipakai lagi: honor pelatih kini lewat Payroll.', [
                ['6101', 'SMP', 'Honor pelatih ekstrakurikuler', 3600000, 0],
                ['1101', 'SMP', 'Pembayaran tunai', 0, 3600000],
            ]],
        ];

        $ids = [];
        foreach ($defs as $key => [$name, $code, $active, $desc, $lines]) {
            $id = DB::table('acc_journal_templates')->whereRaw('lower(name) = ?', [mb_strtolower($name)])->value('id');
            if (! $id) {
                $id = DB::table('acc_journal_templates')->insertGetId([
                    'name' => $name, 'trans_code' => $code, 'description' => $desc, 'is_active' => $active,
                    'created_by' => $this->maker, 'created_at' => now(), 'updated_at' => now(),
                ]);
                foreach ($lines as $i => [$acc, $div, $ld, $d, $c]) {
                    DB::table('acc_journal_template_lines')->insert([
                        'template_id' => $id, 'line_no' => $i + 1, 'acc_code' => $acc, 'div_code' => $div,
                        'description' => $ld, 'debit' => $d, 'credit' => $c,
                    ]);
                }
                AuditLog::record('journal_template.create', (string) $id, ['name' => $name, 'lines' => count($lines)], $this->maker);
                $this->bump('acc_journal_templates');
                $this->bump('acc_journal_template_lines', count($lines));
            }
            $ids[$key] = (int) $id;
        }

        return $ids;
    }

    // ---------------------------------------------------------------- journals

    /**
     * Book one demo journal unless its number exists. $trail = audit events
     * [event, user, from, to, note?] in order.
     */
    private function journal(string $no, string $code, int $day, string $status, ?string $statusBy, array $lines, array $trail, ?string $reference = null): bool
    {
        if (DB::table('trans_head')->where('trans_no', $no)->exists()) {
            return false;
        }
        $date = $this->day($day);
        $at = Carbon::parse($date)->setTime(9, 0);
        $debit = array_sum(array_column($lines, 2));
        $credit = array_sum(array_column($lines, 3));
        DB::table('trans_head')->insert([
            'trans_code' => $code, 'trans_no' => $no, 'trans_date' => $date, 'trans_update' => $date,
            'trans_user' => $this->maker, 'debet' => $debit, 'credit' => $credit, 'status' => $status,
            'source' => 'manual', 'reference' => $reference, 'status_by' => $statusBy,
            'status_at' => $statusBy ? $at->copy()->addHours(count($trail)) : null,
            'created_at' => $at, 'updated_at' => $at,
        ]);
        foreach ($lines as $i => [$acc, $div, $d, $c, $desc]) {
            DB::table('trans_detail')->insert([
                'trans_code' => $code, 'trans_no' => $no, 'trans_count' => $i + 1, 'acc_code' => $acc, 'div_code' => $div,
                'description' => $desc, 'debet' => $d, 'credit' => $c, 'curr_code' => $this->currency, 'curr_rate' => 1,
                'curr_debet' => $d, 'curr_credit' => $c, 'trans_date' => $date, 'created_at' => $at, 'updated_at' => $at,
            ]);
        }
        foreach ($trail as $i => [$event, $user, $from, $to]) {
            $detail = ['from' => $from, 'to' => $to];
            if (isset($trail[$i][4])) {
                $detail['note'] = $trail[$i][4];
            }
            DB::table('acc_audit_log')->insert([
                'event' => $event, 'subject' => $no, 'user_id' => $user,
                'detail' => json_encode($detail), 'created_at' => $at->copy()->addHours($i + 1),
            ]);
        }
        $this->bump('trans_head');
        $this->bump('trans_detail', count($lines));

        return true;
    }

    private function journals(): void
    {
        $m = $this->maker;
        $c = $this->checker;
        $submit = ['journal.submit', $m, 'draft', 'pending'];

        if ($c) {
            // Posted (maker -> checker)
            $this->journal('DEMO-JU-001', 'JU', 3, 'posted', $c, [
                ['1102', 'SMP', 12500000, 0, 'Setoran penerimaan SPP tunai ke rekening sekolah'],
                ['1101', 'SMP', 0, 12500000, 'Kas tunai disetor ke bank'],
            ], [$submit, ['journal.approve', $c, 'pending', 'posted']]);
            $this->journal('DEMO-BKK-001', 'BKK', 6, 'posted', $c, [
                ['6201', 'SMP', 3250000, 0, 'Tagihan listrik PLN gedung SMP'],
                ['6201', 'SD', 2100000, 0, 'Tagihan listrik PLN gedung SD'],
                ['1102', 'SMP', 0, 5350000, 'Pembayaran tagihan listrik via bank'],
            ], [$submit, ['journal.approve', $c, 'pending', 'posted']]);
            $this->journal('DEMO-BKM-001', 'BKM', 8, 'posted', $c, [
                ['1102', 'SMP', 7500000, 0, 'Donasi alumni angkatan 2015 untuk perpustakaan'],
                ['4107', 'SMP', 0, 7500000, 'Pendapatan donasi'],
            ], [$submit, ['journal.approve', $c, 'pending', 'posted']]);
        }

        // Pending approval (submitted by the maker)
        $this->journal('DEMO-JU-002', 'JU', 9, 'pending', $m, [
            ['6401', 'SMP', 1500000, 0, 'Beasiswa prestasi olimpiade sains (3 siswa)'],
            ['1102', 'SMP', 0, 1500000, 'Pencairan beasiswa via transfer'],
        ], [$submit]);
        $this->journal('DEMO-BKK-002', 'BKK', 10, 'pending', $m, [
            ['6102', 'SD', 875000, 0, 'Pembelian ATK & kertas ujian semester ganjil'],
            ['1101', 'SD', 0, 875000, 'Pembayaran tunai toko ATK'],
        ], [$submit]);

        // Draft: inter-unit reclass (antar unit 1301/2301)
        $this->journal('DEMO-JU-003', 'JU', 11, 'draft', null, [
            ['1301', 'SMP', 2000000, 0, 'Piutang ke unit SD atas listrik gedung bersama'],
            ['1102', 'SMP', 0, 2000000, 'Dibayar dari rekening SMP'],
            ['6201', 'SD', 2000000, 0, 'Bagian listrik gedung bersama unit SD'],
            ['2301', 'SD', 0, 2000000, 'Utang ke unit SMP'],
        ], []);

        // "Ditolak": the app returns a rejected journal to draft (Journals::transition);
        // the rejection lives in the audit trail. Unbalanced on purpose (selisih 50.000).
        if ($c) {
            $this->journal('DEMO-JU-004', 'JU', 12, 'draft', $c, [
                ['1102', 'SMP', 1200000, 0, 'Penerimaan sewa kantin KNT-01 Oktober'],
                ['4106', 'SMP', 0, 1250000, 'Pendapatan sewa kantin'],
            ], [$submit, ['journal.reject', $c, 'pending', 'draft', 'Nominal kredit tidak sama dengan bukti transfer (selisih Rp50.000). Mohon dicek ulang.']]);
        }
    }

    // ------------------------------------------------------ recurring journals

    private function recurring(array $tpl): void
    {
        $start = Carbon::parse($this->range[0]);
        $now = now();

        $schedule = function (int $templateId, int $day, Carbon $from, ?string $end, bool $active) use ($now): ?int {
            if (DB::table('acc_recurring_journals')->where('template_id', $templateId)->where('day_of_month', $day)->exists()) {
                return null;
            }
            $next = RecurringJournals::firstRun($from, $day)->toDateString();
            $id = (int) DB::table('acc_recurring_journals')->insertGetId([
                'template_id' => $templateId, 'day_of_month' => $day, 'start_date' => $from->toDateString(),
                'end_date' => $end, 'next_run' => $next, 'is_active' => $active, 'created_by' => $this->maker,
                'created_at' => $now, 'updated_at' => $now,
            ]);
            AuditLog::record('recurring.create', (string) $id, ['template' => $templateId, 'day' => $day, 'next_run' => $next], $this->maker);
            $this->bump('acc_recurring_journals');

            return $id;
        };
        $run = function (int $id, string $date, string $status, ?string $no, ?string $msg) use ($now) {
            $this->bump('acc_recurring_runs', DB::table('acc_recurring_runs')->insertOrIgnore([
                'recurring_id' => $id, 'run_date' => $date, 'status' => $status, 'trans_no' => $no, 'message' => $msg,
                'created_at' => $now, 'updated_at' => $now,
            ]));
        };

        // 1) Gaji: active, first run on the 25th of the active period (the daily job books it).
        $schedule($tpl['gaji'], 25, $start->copy(), null, true);

        // 2) Listrik: active since two periods ago, with run history; this
        //    period's draft is booked here, next run next month.
        $from = $start->copy()->subMonthsNoOverflow(2);
        if ($id = $schedule($tpl['listrik'], 5, $from, null, true)) {
            $r1 = $from->copy()->day(5)->toDateString();
            $r2 = $from->copy()->addMonthNoOverflow()->day(5)->toDateString();
            $r3 = $start->copy()->day(5);
            $r3 = ($r3->toDateString() > $this->range[1] ? Carbon::parse($this->range[1]) : $r3)->toDateString();
            $run($id, $r1, 'failed', null, 'Akun 6201 tidak ditemukan.');
            $run($id, $r2, 'skipped', null, 'Dilewati: tagihan bulan ini dibayar bersama bulan berikutnya.');
            $no = 'DEMO-RJ-001';
            $booked = $this->journal($no, 'BKK', (int) abs(Carbon::parse($this->range[0])->diffInDays(Carbon::parse($r3))) + 1, 'draft', null, [
                ['6201', 'SMP', 5250000, 0, 'Listrik PLN & air PDAM'],
                ['6201', 'SMP', 1500000, 0, 'Internet sekolah 200 Mbps'],
                ['1102', 'SMP', 0, 6750000, 'Pembayaran via bank'],
            ], [], "recurring:$id:$r3");
            $run($id, $r3, $booked ? 'created' : 'failed', $booked ? $no : null, $booked ? null : 'Nomor DEMO-RJ-001 sudah dipakai.');
            DB::table('acc_recurring_journals')->where('id', $id)->update([
                'next_run' => Carbon::parse($r3)->addMonthNoOverflow()->day(5)->toDateString(),
                'last_run_at' => Carbon::parse($r3)->setTime(1, 0), 'last_trans_no' => $booked ? $no : null, 'updated_at' => $now,
            ]);
            AuditLog::record('recurring.failed', (string) $id, ['date' => $r1, 'trans_no' => null, 'error' => 'Akun 6201 tidak ditemukan.'], 'SYSTEM');
            AuditLog::record('recurring.skip', (string) $id, ['date' => $r2], $this->maker);
            if ($booked) {
                AuditLog::record('recurring.run', (string) $id, ['date' => $r3, 'trans_no' => $no, 'error' => null], 'SYSTEM');
            }
        }

        // 3) Penyusutan: ended at the close of the previous period (inactive).
        $from = $start->copy()->subMonthsNoOverflow(6);
        $end = $start->copy()->subDay()->toDateString();
        if ($id = $schedule($tpl['susut'], 28, $from, $end, false)) {
            $last = $start->copy()->subMonthNoOverflow()->day(28)->toDateString();
            $run($id, $last, 'failed', null, "Periode untuk tanggal $last sudah ditutup; jurnal berulang tidak dibuat.");
            DB::table('acc_recurring_journals')->where('id', $id)->update([
                'next_run' => $start->copy()->day(28)->toDateString(), 'last_run_at' => Carbon::parse($last)->setTime(1, 0),
                'last_error' => "Periode untuk tanggal $last sudah ditutup; jurnal berulang tidak dibuat.", 'updated_at' => $now,
            ]);
            AuditLog::record('recurring.finished', (string) $id, ['end_date' => $end], 'SYSTEM');
        }

        // 4) Jasa giro: active, end of month.
        $schedule($tpl['giro'], 28, $start->copy(), null, true);
    }

    // ----------------------------------------------------------- COA templates

    private function coaTemplates(): void
    {
        $byCode = [];
        foreach (self::ACCOUNTS as [$code, $name, $group, $cash]) {
            $byCode[$code] = [$name, $group, $cash];
        }
        $defs = [
            ['COA Standar Sekolah', 'Bagan akun standar yayasan: kas/bank, piutang & titipan siswa, antar unit, pendapatan dan beban sekolah.', array_keys($byCode), null],
            ['COA Unit Ringkas (SD)', 'Akun operasional minimum untuk unit baru; sudah disalin ke unit SD (kode <akun>.SD).', ['1101', '1102', '3101', '4101', '4103', '6101', '6102', '6201'], 'SD'],
        ];
        foreach ($defs as [$name, $desc, $codes, $copyTo]) {
            if (DB::table('acc_coa_templates')->whereRaw('lower(name) = ?', [mb_strtolower($name)])->exists()) {
                continue;
            }
            $id = (int) DB::table('acc_coa_templates')->insertGetId([
                'name' => $name, 'description' => $desc, 'created_by' => $this->maker, 'created_at' => now(), 'updated_at' => now(),
            ]);
            foreach ($codes as $code) {
                [$n, $g, $cash] = $byCode[$code];
                DB::table('acc_coa_template_lines')->insert([
                    'template_id' => $id, 'acc_code' => $code, 'acc_name' => $n, 'acc_group' => $g, 'sub_acc_group' => null,
                    'parent_code' => null, 'normal_balance' => self::normal($code, $g), 'is_cash' => $cash,
                    'is_interunit' => in_array($code, ['1301', '2301'], true), 'is_control' => $code === '1201',
                ]);
            }
            AuditLog::record('coa_template.create', (string) $id, ['name' => $name, 'accounts' => count($codes)], $this->maker);
            $this->bump('acc_coa_templates');
            $this->bump('acc_coa_template_lines', count($codes));
            if ($copyTo) {
                // Same code path as the page: adds "<code>.SD" accounts, skips existing ones.
                $r = CoaTemplates::copyToUnit($id, $copyTo, $this->maker);
                $this->bump("mst_account (unit $copyTo)", (int) ($r['created'] ?? 0));
            }
        }
    }

    // ------------------------------------------------------------ period reopen

    private function periodReopen(): void
    {
        $t = PeriodReopen::target();
        if (! $t || ! $this->checker) {
            return;
        }
        $label = PeriodReopen::label($t);
        $base = Carbon::parse($this->range[0])->setTime(10, 0);
        $rows = [
            ['rejected', 'Faktur internet bulan lalu baru diterima; mohon buka periode untuk mencatat beban di bulan yang benar.', $this->checker,
                'Nilainya kecil; catat sebagai beban periode berjalan dengan keterangan bulan asal.', 1],
            ['cancelled', 'Salah input tanggal setoran SPP beberapa siswa, perlu dipindah ke tanggal transfer sebenarnya.', $this->maker, null, 2],
        ];
        if (! DB::table('acc_period_reopen_requests')->where('status', 'pending')->exists()) {
            $rows[] = ['pending', 'Ada pembayaran gaji honorer bulan lalu yang belum dijurnal; perlu buka periode untuk koreksi sebelum laporan yayasan.', null, null, 3];
        }
        foreach ($rows as [$status, $reason, $decider, $note, $h]) {
            if (DB::table('acc_period_reopen_requests')->where('reason', $reason)->exists()) {
                continue;
            }
            $at = $base->copy()->addHours($h);
            $id = (int) DB::table('acc_period_reopen_requests')->insertGetId([
                'year' => $t['year'], 'period' => $t['period'], 'reason' => $reason, 'status' => $status,
                'requested_by' => $this->maker, 'requested_at' => $at,
                'decided_by' => $decider, 'decided_at' => $decider ? $at->copy()->addMinutes(45) : null, 'decision_note' => $note,
            ]);
            $log = fn (string $event, array $detail, string $user, Carbon $when) => DB::table('acc_audit_log')->insert([
                'event' => $event, 'subject' => $label, 'user_id' => $user, 'detail' => json_encode($detail), 'created_at' => $when,
            ]);
            $log('period.reopen_request', ['request' => $id, 'reason' => $reason], $this->maker, $at);
            if ($status === 'rejected') {
                $log('period.reopen_reject', ['request' => $id, 'note' => $note], $this->checker, $at->copy()->addMinutes(45));
            } elseif ($status === 'cancelled') {
                $log('period.reopen_cancel', ['request' => $id], $this->maker, $at->copy()->addMinutes(45));
            }
            $this->bump('acc_period_reopen_requests');
        }
    }
}
