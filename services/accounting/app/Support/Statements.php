<?php

namespace App\Support;

use Illuminate\Support\Collection;
use Illuminate\Support\Facades\DB;

/**
 * Trial balance, the four financial statements and the consolidation
 * (PRD menu 36-38, QA BUG-063/064/065), all straight from the posted
 * ledger (Ledger::balances), so every figure ties back to the general
 * ledger and the trial balance.
 *
 * Amounts in the statements are presented with their natural sign: assets
 * and expenses debit-positive, liabilities, net assets and revenue
 * credit-positive.
 */
final class Statements
{
    private const EPS = 0.005;

    /** Trial balance rows (non-zero only unless $all) and totals. */
    public static function trialBalance(int $year, int $period, ?string $division = null, bool $all = false): array
    {
        $rows = Ledger::balances($year, $period, $division)
            ->filter(fn ($r) => $all || self::nz($r->opening) || self::nz($r->debit) || self::nz($r->credit) || self::nz($r->ending))
            ->map(function ($r) {
                $r->end_debit = $r->ending > 0 ? $r->ending : 0.0;
                $r->end_credit = $r->ending < 0 ? -$r->ending : 0.0;
                $r->open_debit = $r->opening > 0 ? $r->opening : 0.0;
                $r->open_credit = $r->opening < 0 ? -$r->opening : 0.0;

                return $r;
            })->values();

        $t = [];
        foreach (['open_debit', 'open_credit', 'debit', 'credit', 'end_debit', 'end_credit'] as $k) {
            $t[$k] = round((float) $rows->sum($k), 2);
        }
        $t['difference'] = round($t['end_debit'] - $t['end_credit'], 2);
        $t['movement_difference'] = round($t['debit'] - $t['credit'], 2);
        $t['balanced'] = abs($t['difference']) < 0.01 && abs($t['movement_difference']) < 0.01;

        return ['rows' => $rows, 'totals' => $t];
    }

    /**
     * All four statements for one period (and the previous one for
     * comparison).
     */
    public static function build(int $year, int $period, ?string $division = null): array
    {
        $a = Ledger::active();
        [$py, $pp] = Ledger::previous($year, $period);
        $cur = Ledger::balances($year, $period, $division);
        $prev = Ledger::balances($py, $pp, $division);

        return [
            'year' => $year,
            'period' => $period,
            'label' => Ledger::periodLabel($year, $period),
            'prev_label' => Ledger::periodLabel($py, $pp),
            'division' => $division,
            'provisional' => $year > $a['year'] || ($year === $a['year'] && $period >= $a['period']),
            'generated_at' => now(),
            'activity' => self::activity($cur, $prev),
            'position' => self::position($cur, $prev),
            'cash_flow' => self::cashFlow($cur, $year, $period, $division),
            'net_assets' => self::netAssetChanges($cur),
        ];
    }

    /** Laporan aktivitas (surplus/defisit): period movement and year to date. */
    public static function activity(Collection $cur, Collection $prev): array
    {
        $prevBy = $prev->keyBy('acc_code');
        $section = function (array $groups, int $sign) use ($cur, $prevBy) {
            return $cur->whereIn('acc_group', $groups)->map(fn ($r) => (object) [
                'acc_code' => $r->acc_code,
                'acc_name' => $r->acc_name,
                'period' => round($sign * ($r->debit - $r->credit), 2),
                'ytd' => round($sign * $r->ending, 2),
                'prev' => round($sign * (($prevBy[$r->acc_code]->debit ?? 0) - ($prevBy[$r->acc_code]->credit ?? 0)), 2),
            ])->filter(fn ($l) => self::nz($l->period) || self::nz($l->ytd) || self::nz($l->prev))->values();
        };
        $revenue = $section(Ledger::REVENUE, -1);
        $expense = $section(Ledger::EXPENSE, 1);
        $tot = fn (Collection $c, string $k) => round((float) $c->sum($k), 2);

        return [
            'revenue' => $revenue,
            'expense' => $expense,
            'total_revenue' => ['period' => $tot($revenue, 'period'), 'ytd' => $tot($revenue, 'ytd'), 'prev' => $tot($revenue, 'prev')],
            'total_expense' => ['period' => $tot($expense, 'period'), 'ytd' => $tot($expense, 'ytd'), 'prev' => $tot($expense, 'prev')],
            'surplus' => [
                'period' => round($tot($revenue, 'period') - $tot($expense, 'period'), 2),
                'ytd' => round($tot($revenue, 'ytd') - $tot($expense, 'ytd'), 2),
                'prev' => round($tot($revenue, 'prev') - $tot($expense, 'prev'), 2),
            ],
        ];
    }

    /** Laporan posisi keuangan at the end of the period vs the end of the previous one. */
    public static function position(Collection $cur, Collection $prev): array
    {
        $prevBy = $prev->keyBy('acc_code');
        $section = fn (array $groups, int $sign) => $cur->whereIn('acc_group', $groups)->map(fn ($r) => (object) [
            'acc_code' => $r->acc_code,
            'acc_name' => $r->acc_name,
            'amount' => round($sign * $r->ending, 2),
            'prev' => round($sign * ($prevBy[$r->acc_code]->ending ?? 0), 2),
        ])->filter(fn ($l) => self::nz($l->amount) || self::nz($l->prev))->values();

        $assets = $section(Ledger::ASSET, 1);
        $liabilities = $section(Ledger::LIABILITY, -1);
        $equity = $section(Ledger::EQUITY, -1);
        $nominal = array_merge(Ledger::REVENUE, Ledger::EXPENSE);
        $surplus = ['amount' => -Ledger::sumGroups($cur, $nominal), 'prev' => -Ledger::sumGroups($prev, $nominal)];

        $tA = ['amount' => round((float) $assets->sum('amount'), 2), 'prev' => round((float) $assets->sum('prev'), 2)];
        $tL = ['amount' => round((float) $liabilities->sum('amount'), 2), 'prev' => round((float) $liabilities->sum('prev'), 2)];
        $tN = ['amount' => round($equity->sum('amount') + $surplus['amount'], 2), 'prev' => round($equity->sum('prev') + $surplus['prev'], 2)];

        return [
            'assets' => $assets,
            'liabilities' => $liabilities,
            'equity' => $equity,
            'surplus' => $surplus,
            'total_assets' => $tA,
            'total_liabilities' => $tL,
            'total_net_assets' => $tN,
            'difference' => round($tA['amount'] - $tL['amount'] - $tN['amount'], 2),
        ];
    }

    /**
     * Laporan arus kas, direct method: each journal's net effect on kas/bank
     * accounts (is_cash) is attributed to its non-cash lines, so transfers
     * between kas and bank cancel out and every receipt/payment is shown
     * against the account it came from or went to.
     */
    public static function cashFlow(Collection $cur, int $year, int $period, ?string $division = null): array
    {
        $cash = $cur->where('is_cash', true)->pluck('acc_code')->flip();
        $info = $cur->keyBy('acc_code');
        $flows = [];
        $unattributed = 0.0;

        $lines = Ledger::periodLines($year, $period, $division);
        foreach ($lines->groupBy(fn ($l) => $l->trans_no.'|'.substr((string) $l->trans_date, 0, 10)) as $journal) {
            $net = 0.0;
            foreach ($journal as $l) {
                if (isset($cash[$l->acc_code])) {
                    $net += (float) $l->debet - (float) $l->credit;
                }
            }
            if (abs($net) < self::EPS) {
                continue;
            }
            $attributed = 0.0;
            foreach ($journal as $l) {
                if (! isset($cash[$l->acc_code])) {
                    $amt = (float) $l->credit - (float) $l->debet;
                    $flows[$l->acc_code] = ($flows[$l->acc_code] ?? 0) + $amt;
                    $attributed += $amt;
                }
            }
            // Only possible for a single unit's view of an inter-unit journal.
            $unattributed += $net - $attributed;
        }

        // PHP turns numeric-string keys ("1201") into ints; give the code back as a string.
        $rows = collect($flows)->map(fn ($amt, $code) => (object) [
            'acc_code' => (string) $code,
            'acc_name' => $info[$code]->acc_name ?? $code,
            'acc_group' => (int) ($info[$code]->acc_group ?? 0),
            'amount' => round($amt, 2),
        ])->filter(fn ($r) => self::nz($r->amount))->sortBy('acc_code')->values();

        $receipts = $rows->filter(fn ($r) => $r->amount > 0)->values();
        $payments = $rows->filter(fn ($r) => $r->amount < 0)->values();
        $opening = round((float) $cur->where('is_cash', true)->sum('opening'), 2);
        $ending = round((float) $cur->where('is_cash', true)->sum('ending'), 2);
        $net = round($rows->sum('amount') + $unattributed, 2);

        return [
            'opening' => $opening,
            'receipts' => $receipts,
            'payments' => $payments,
            'total_receipts' => round((float) $receipts->sum('amount'), 2),
            'total_payments' => round((float) $payments->sum('amount'), 2),
            'unattributed' => round($unattributed, 2),
            'net' => $net,
            'ending' => $ending,
            // Movement on cash accounts not explained by journal lines (e.g. a
            // balance corrected straight in mst_saldo_news) shows up here.
            'difference' => round($opening + $net - $ending, 2),
            'has_cash_accounts' => $cash->isNotEmpty(),
        ];
    }

    /** Laporan perubahan aset neto for the period. */
    public static function netAssetChanges(Collection $cur): array
    {
        $groups = array_merge(Ledger::EQUITY, Ledger::REVENUE, Ledger::EXPENSE);
        $opening = -Ledger::sumGroups($cur, $groups, 'opening');
        $surplus = round(-Ledger::movement($cur, array_merge(Ledger::REVENUE, Ledger::EXPENSE)), 2);
        $direct = round(-Ledger::movement($cur, Ledger::EQUITY), 2);
        $ending = -Ledger::sumGroups($cur, $groups);

        return [
            'opening' => round($opening, 2),
            'surplus' => $surplus,
            'direct' => $direct,
            'ending' => round($ending, 2),
            'difference' => round($opening + $surplus + $direct - $ending, 2),
        ];
    }

    /**
     * Consolidation per unit (mst_division) with elimination of inter-unit
     * accounts (is_interunit, BR-ACC-04): per account one column per unit,
     * the total, the elimination and the consolidated amount.
     */
    public static function consolidation(int $year, int $period): array
    {
        $units = DB::table('mst_division')->orderBy('div_code')->get(['div_code', 'div_name']);
        $total = Ledger::balances($year, $period)->keyBy('acc_code');
        $per = [];
        foreach ($units as $u) {
            $per[$u->div_code] = Ledger::balances($year, $period, $u->div_code)->keyBy('acc_code');
        }

        $rows = collect();
        foreach ($total as $code => $r) {
            $code = (string) $code;
            $cells = [];
            $sumUnits = 0.0;
            foreach ($units as $u) {
                $v = (float) ($per[$u->div_code][$code]->ending ?? 0);
                $cells[$u->div_code] = $v;
                $sumUnits += $v;
            }
            $other = round($r->ending - $sumUnits, 2);
            $elim = $r->is_interunit ? -$r->ending : 0.0;
            if (! self::nz($r->ending) && ! collect($cells)->contains(fn ($v) => self::nz($v))) {
                continue;
            }
            $rows->push((object) [
                'acc_code' => $code, 'acc_name' => $r->acc_name, 'acc_group' => $r->acc_group,
                'is_interunit' => (bool) $r->is_interunit,
                'units' => $cells, 'other' => $other,
                'total' => $r->ending, 'elimination' => round($elim, 2), 'consolidated' => round($r->ending + $elim, 2),
            ]);
        }

        $interunit = $rows->where('is_interunit', true);
        $groupSum = fn (array $groups, string $k) => round((float) $rows->whereIn('acc_group', $groups)->sum($k), 2);

        return [
            'units' => $units,
            'rows' => $rows,
            'has_other' => $rows->contains(fn ($r) => self::nz($r->other)),
            'interunit_residual' => round((float) $interunit->sum('total'), 2),
            'interunit_accounts' => $interunit->count(),
            'summary' => [
                'assets' => $groupSum(Ledger::ASSET, 'consolidated'),
                'liabilities' => -$groupSum(Ledger::LIABILITY, 'consolidated'),
                'revenue' => -$groupSum(Ledger::REVENUE, 'consolidated'),
                'expense' => $groupSum(Ledger::EXPENSE, 'consolidated'),
            ],
        ];
    }

    private static function nz($v): bool
    {
        return abs((float) $v) >= self::EPS;
    }
}
