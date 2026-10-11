<?php

namespace App\Support;

use App\Models\Parameter;
use App\Models\Period;
use Illuminate\Support\Carbon;
use Illuminate\Support\Collection;
use Illuminate\Support\Facades\DB;
use Illuminate\Validation\ValidationException;

/**
 * Posted-ledger math for the accounting dashboard, trial balance, financial
 * reports and consolidation (PRD menu 32-38). Runs against whatever tenant
 * schema search_path points at.
 *
 * Sources (BR-ACC-03: only POSTED amounts):
 *  - closed periods  -> mst_saldo_news (periode 0 = opening of the year; End of
 *    Month rebuilds a closed period's row from this_detail, which only ever
 *    receives posted journals);
 *  - the open period -> trans_detail of trans_head rows with status posted or
 *    reversed (a reversed journal still counts; its reversal cancels it).
 *    mst_saldo_news' row for the open period is ignored on purpose: Exco's
 *    TransDetail events add drafts to it as they are typed.
 *
 * Amounts are debit-positive (debit - credit).
 */
final class Ledger
{
    public const POSTED = ['posted', 'reversed'];

    /** Account groups (account_group_tables ids, seeded 1..8). */
    public const ASSET = [1];
    public const LIABILITY = [2];
    public const EQUITY = [3];
    public const REVENUE = [4, 7];
    public const EXPENSE = [5, 6, 8];

    /** @return array{year:int, period:int} the open (active) period. */
    public static function active(): array
    {
        $p = Parameter::first();

        return ['year' => (int) ($p?->year ?? now()->year), 'period' => (int) ($p?->period ?? 1)];
    }

    /** Date range [from, to] (Y-m-d) of one period, null when mst_period has none. */
    public static function range(int $year, int $period): ?array
    {
        $row = Period::where('period_name', 'ilike', '%'.$year.'%')->first();
        $key = 'p'.str_pad((string) $period, 2, '0', STR_PAD_LEFT);
        if (! $row || ! $row->{$key.'_from'} || ! $row->{$key.'_to'}) {
            return null;
        }

        return [Carbon::parse($row->{$key.'_from'})->toDateString(), Carbon::parse($row->{$key.'_to'})->toDateString()];
    }

    public static function periodLabel(int $year, int $period): string
    {
        $row = Period::where('period_name', 'ilike', '%'.$year.'%')->first();
        $name = $row?->{'p'.str_pad((string) $period, 2, '0', STR_PAD_LEFT)};

        return $name ?: ($period <= 12 ? Carbon::create($year, $period, 1)->locale('id')->translatedFormat('F Y') : "Penyesuaian $year");
    }

    /**
     * BR-ACC-07: a journal date must fall inside the open period -- earlier
     * periods are closed (End of Month moved them to history), later ones are
     * not open yet.
     *
     * @throws ValidationException
     */
    public static function assertOpenDate(string $date, string $field = 'trans_date'): void
    {
        $a = self::active();
        $r = self::range($a['year'], $a['period']);
        $d = Carbon::parse($date)->toDateString();
        if ($r && ($d < $r[0] || $d > $r[1])) {
            $msg = $d < $r[0]
                ? "Periode untuk tanggal $d sudah ditutup (dikunci). Periode aktif: {$r[0]} s/d {$r[1]}."
                : "Tanggal $d berada di luar periode aktif ({$r[0]} s/d {$r[1]}).";
            throw ValidationException::withMessages([$field => $msg]);
        }
    }

    /**
     * One row per account: opening (before $period), debit/credit movement in
     * $period, ending -- all posted only.
     */
    public static function balances(int $year, int $period): Collection
    {
        $a = self::active();
        $closedBefore = $year < $a['year'] ? 99 : ($year === $a['year'] ? $a['period'] : 0);

        $agg = [];
        $add = function (string $acc, float $open, float $d, float $c) use (&$agg) {
            $agg[$acc] ??= [0.0, 0.0, 0.0];
            $agg[$acc][0] += $open;
            $agg[$acc][1] += $d;
            $agg[$acc][2] += $c;
        };

        if ($closedBefore > 0) {
            $rows = DB::select(
                'select acc_code,
                        sum(case when periode < ? then coalesce(debet,0) - coalesce(credit,0) else 0 end) as open,
                        sum(case when periode = ? then coalesce(debet,0) else 0 end) as d,
                        sum(case when periode = ? then coalesce(credit,0) else 0 end) as c
                   from mst_saldo_news
                  where tahun = ? and periode <= ? and periode < ?
                  group by acc_code',
                [$period, $period, $period, $year, $period, $closedBefore]
            );
            foreach ($rows as $r) {
                $add($r->acc_code, (float) $r->open, (float) $r->d, (float) $r->c);
            }
        }

        if ($year === $a['year'] && $a['period'] <= $period) {
            foreach (self::openLines()->groupBy('acc_code') as $acc => $lines) {
                $d = (float) $lines->sum('debet');
                $c = (float) $lines->sum('credit');
                $a['period'] < $period ? $add($acc, $d - $c, 0, 0) : $add($acc, 0, $d, $c);
            }
        }

        return DB::table('mst_account as a')
            ->leftJoin('account_group_tables as g', 'g.id', '=', 'a.acc_group')
            ->leftJoin('account_sub_group_tables as s', 's.id', '=', 'a.sub_acc_group')
            ->orderBy('a.acc_code')
            ->get(['a.acc_code', 'a.acc_name', 'a.acc_group', 'g.name as group_name', 'a.sub_acc_group', 's.name as sub_group_name',
                'a.parent_code', 'a.normal_balance', 'a.is_active', 'a.is_interunit', 'a.is_cash'])
            ->map(function ($r) use ($agg) {
                [$open, $d, $c] = $agg[$r->acc_code] ?? [0.0, 0.0, 0.0];
                $r->acc_group = (int) $r->acc_group;
                $r->opening = round($open, 2);
                $r->debit = round($d, 2);
                $r->credit = round($c, 2);
                $r->ending = round($open + $d - $c, 2);

                return $r;
            });
    }

    /** Lines of posted journals in the open period (trans_detail). */
    public static function openLines(): Collection
    {
        return DB::table('trans_detail as d')
            ->join('trans_head as h', 'h.trans_no', '=', 'd.trans_no')
            ->whereIn('h.status', self::POSTED)
            ->get(['d.trans_no', 'd.trans_date', 'd.acc_code', 'd.description', 'd.debet', 'd.credit']);
    }

    /** Posted journal lines of one period, closed or open, with their journal key. */
    public static function periodLines(int $year, int $period): Collection
    {
        $a = self::active();
        if ($year === $a['year'] && $period === $a['period']) {
            return self::openLines();
        }
        if ($year > $a['year'] || ($year === $a['year'] && $period > $a['period'])) {
            return collect();
        }

        return DB::table('this_detail')
            ->where('period', str_pad((string) $period, 2, '0', STR_PAD_LEFT))
            ->whereRaw('extract(year from trans_date) = ?', [$year])
            ->get(['trans_no', 'trans_date', 'acc_code', 'description', 'debet', 'credit']);
    }

    /** Sum of `ending` (debit-positive) over accounts in the given groups. */
    public static function sumGroups(Collection $rows, array $groups, string $field = 'ending'): float
    {
        return round((float) $rows->whereIn('acc_group', $groups)->sum($field), 2);
    }

    /** Period movement (debit - credit) over accounts in the given groups. */
    public static function movement(Collection $rows, array $groups): float
    {
        return round((float) $rows->whereIn('acc_group', $groups)->sum(fn ($r) => $r->debit - $r->credit), 2);
    }

    /** The headline figures every report starts from. Credit-normal groups are flipped to positive. */
    public static function summary(Collection $rows): array
    {
        $revenue = -self::movement($rows, self::REVENUE);
        $expense = self::movement($rows, self::EXPENSE);
        $ytdSurplus = -self::sumGroups($rows, array_merge(self::REVENUE, self::EXPENSE));

        return [
            'revenue' => $revenue,
            'expense' => $expense,
            'surplus' => round($revenue - $expense, 2),
            'assets' => self::sumGroups($rows, self::ASSET),
            'liabilities' => -self::sumGroups($rows, self::LIABILITY),
            'net_assets' => round(-self::sumGroups($rows, self::EQUITY) + $ytdSurplus, 2),
            'ytd_surplus' => $ytdSurplus,
            'cash' => round((float) $rows->where('is_cash', true)->sum('ending'), 2),
        ];
    }

    /** [year, period] of the period before ($period 1 -> December of last year). */
    public static function previous(int $year, int $period): array
    {
        return $period > 1 ? [$year, min($period - 1, 12)] : [$year - 1, 12];
    }
}
