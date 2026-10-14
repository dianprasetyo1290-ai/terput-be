<?php

namespace App\Support;

use Illuminate\Support\Facades\DB;
use Illuminate\Validation\ValidationException;

/**
 * Control-account check for the migrated receivables (QA BUG-059, AC-04-02/03,
 * BR-ACC-06): the per-student receivables live in billing's sub-ledger; their
 * total must equal the receivable control accounts (asset accounts flagged
 * is_control) in accounting. check() puts both side by side; signOff() records
 * the migration sign-off in the audit trail, allowed when the difference is
 * zero or an explanation is given.
 */
final class ReceivableReconciliation
{
    public const MIN_NOTE = 10;

    /**
     * @return array{year:int, period:int, accounts: array, opening: float, ledger: float,
     *               billing: ?float, students: ?int, difference: ?float, error: ?string}
     */
    public static function check(): array
    {
        $a = Ledger::active();
        $rows = Ledger::balances($a['year'], $a['period'])->filter(fn ($r) => $r->acc_group === 1 && $r->is_control)->values();
        $codes = $rows->pluck('acc_code')->all();
        $opening = $codes ? (float) DB::table('mst_saldo_news')->where('tahun', $a['year'])->where('periode', 0)
            ->whereIn('acc_code', $codes)->sum(DB::raw('coalesce(debet,0) - coalesce(credit,0)')) : 0.0;
        $ledger = round((float) $rows->sum('ending'), 2);

        $out = [
            'year' => $a['year'], 'period' => $a['period'],
            'accounts' => $rows->map(fn ($r) => ['acc_code' => $r->acc_code, 'acc_name' => $r->acc_name, 'ending' => $r->ending])->all(),
            'opening' => round($opening, 2), 'ledger' => $ledger,
            'billing' => null, 'students' => null, 'difference' => null, 'error' => null,
        ];
        if (! $codes) {
            $out['error'] = 'Belum ada akun piutang yang ditandai sebagai akun kontrol (Chart of Account → "Akun kontrol").';

            return $out;
        }
        $r = Billing::receivablesSummary(now()->toDateString());
        if (! $r['ok']) {
            $out['error'] = $r['error'];

            return $out;
        }
        $out['billing'] = round((float) $r['data']['outstanding_idr'], 2);
        $out['students'] = isset($r['data']['students']) ? (int) $r['data']['students'] : null;
        $out['difference'] = round($ledger - $out['billing'], 2);

        return $out;
    }

    /** @throws ValidationException */
    public static function signOff(array $check, ?string $note, string $userId): void
    {
        if ($check['billing'] === null) {
            throw ValidationException::withMessages(['signOffNote' => 'Total piutang billing belum didapat; sign-off belum dapat dilakukan.'
                .($check['error'] ? ' '.$check['error'] : '')]);
        }
        $note = trim((string) $note);
        if (abs((float) $check['difference']) >= 0.01 && mb_strlen($note) < self::MIN_NOTE) {
            throw ValidationException::withMessages(['signOffNote' => 'Ada selisih; tuliskan penjelasan selisih (minimal '.self::MIN_NOTE.' karakter) sebelum sign-off.']);
        }
        AuditLog::record('migration.signoff', $check['year'].'-'.str_pad((string) $check['period'], 2, '0', STR_PAD_LEFT), [
            'ledger' => $check['ledger'], 'opening' => $check['opening'], 'billing' => $check['billing'],
            'students' => $check['students'], 'difference' => $check['difference'], 'note' => $note ?: null,
            'accounts' => array_column($check['accounts'], 'acc_code'),
        ], $userId);
    }

    public static function lastSignOff(): ?object
    {
        $r = DB::table('acc_audit_log')->where('event', 'migration.signoff')->orderByDesc('id')->first();
        if ($r) {
            $r->detail = json_decode((string) $r->detail, true) ?: [];
        }

        return $r;
    }
}
