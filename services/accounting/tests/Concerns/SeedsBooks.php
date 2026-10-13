<?php

namespace Tests\Concerns;

use App\Models\User;
use Illuminate\Support\Carbon;
use Illuminate\Support\Facades\DB;

/**
 * Minimal tenant books for accounting feature tests: active period, a
 * calendar mst_period row, account groups, a small school COA and helpers to
 * book journals straight into trans_* (open period) or this_* (history).
 */
trait SeedsBooks
{
    protected function seedBooks(int $year = 2026, int $period = 10): void
    {
        DB::table('parameter')->insert([
            'serial_no' => 'SKCI', 'company' => 'Sekolah Uji', 'period' => $period, 'year' => $year,
            'base_currency' => 'Rp', 'created_at' => now(), 'updated_at' => now(),
        ]);
        DB::table('mst_currency')->insertOrIgnore([
            'curr_code' => 'Rp', 'curr_name' => 'Rupiah', 'rate_system' => 'N', 'created_at' => now(), 'updated_at' => now(),
        ]);
        DB::table('mst_division')->insertOrIgnore([
            ['div_code' => 'SD', 'div_name' => 'Unit SD', 'created_at' => now(), 'updated_at' => now()],
            ['div_code' => 'SMP', 'div_name' => 'Unit SMP', 'created_at' => now(), 'updated_at' => now()],
        ]);

        $row = ['period_code' => '1', 'period_name' => "Periode $year", 'created_at' => now(), 'updated_at' => now()];
        for ($m = 1; $m <= 12; $m++) {
            $k = 'p'.str_pad((string) $m, 2, '0', STR_PAD_LEFT);
            $start = Carbon::create($year, $m, 1);
            $row[$k] = $start->format('M Y');
            $row[$k.'_from'] = $start->toDateString();
            $row[$k.'_to'] = $start->copy()->endOfMonth()->toDateString();
        }
        DB::table('mst_period')->insert($row);

        foreach ([1 => 'Asset', 2 => 'Liability', 3 => 'Equity', 4 => 'Income', 5 => 'Cost of Sales', 6 => 'Expense', 7 => 'Other Income', 8 => 'Other Expense'] as $id => $name) {
            DB::table('account_group_tables')->insertOrIgnore(['id' => $id, 'name' => $name, 'created_at' => now(), 'updated_at' => now()]);
        }

        foreach ([
            ['1101', 'Kas', 1, true], ['1102', 'Bank', 1, true], ['1201', 'Piutang SPP', 1, false],
            ['2101', 'Utang Gaji', 2, false], ['3101', 'Aset Neto', 3, false],
            ['4101', 'Pendapatan SPP', 4, false], ['6101', 'Beban Gaji', 6, false],
        ] as [$code, $name, $group, $cash]) {
            DB::table('mst_account')->insertOrIgnore([
                'acc_code' => $code, 'acc_name' => $name, 'acc_group' => $group, 'acc_currency' => 'Rp',
                'is_cash' => $cash, 'is_active' => true, 'normal_balance' => in_array($group, [1, 5, 6, 8]) ? 'D' : 'K',
                'created_at' => now(), 'updated_at' => now(),
            ]);
        }

        foreach (['JU' => 'Jurnal Umum', 'BKK' => 'Bukti Kas Keluar', 'BIL' => 'Billing'] as $code => $name) {
            DB::table('mst_transaction')->insertOrIgnore(['trans_code' => $code, 'trans_name' => $name, 'trans_status' => '1', 'created_at' => now(), 'updated_at' => now()]);
        }
    }

    /**
     * Book a journal directly. $lines = [[acc, debit, credit, div?], ...].
     */
    protected function book(string $transNo, string $date, array $lines, string $status = 'posted', bool $history = false, array $head = []): void
    {
        $period = Carbon::parse($date)->format('m');
        $d = array_sum(array_column($lines, 1));
        $c = array_sum(array_column($lines, 2));
        $h = array_merge([
            'trans_code' => 'JU', 'trans_no' => $transNo, 'trans_date' => $date, 'trans_update' => $date,
            'trans_user' => 'U1', 'debet' => $d, 'credit' => $c, 'status' => $status,
            'created_at' => now(), 'updated_at' => now(),
        ], $head);
        if ($history) {
            $h['period'] = $period;
        }
        DB::table($history ? 'this_head' : 'trans_head')->insert($h);

        foreach (array_values($lines) as $i => $l) {
            $row = [
                'trans_code' => $h['trans_code'], 'trans_no' => $transNo, 'trans_count' => $i + 1,
                'acc_code' => $l[0], 'div_code' => $l[3] ?? 'SD', 'description' => "Baris $transNo",
                'debet' => $l[1], 'credit' => $l[2], 'trans_date' => $date, 'curr_code' => 'Rp',
                'created_at' => now(), 'updated_at' => now(),
            ];
            if ($history) {
                $row['period'] = $period;
            }
            DB::table($history ? 'this_detail' : 'trans_detail')->insert($row);
        }
    }

    protected function accountant(string $role = 'SuperAdmin'): User
    {
        $user = User::query()->forceCreate([
            'user_id' => 'U1', 'user_name' => 'Akuntan Uji', 'email' => 'akuntan@uji.test', 'password' => bcrypt('secret'),
        ]);
        \Spatie\Permission\Models\Role::findOrCreate($role, 'web');
        $user->assignRole($role);

        return $user;
    }
}
