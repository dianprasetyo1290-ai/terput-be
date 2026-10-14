<?php

namespace App\Support;

use Illuminate\Support\Facades\DB;
use Illuminate\Validation\ValidationException;

/**
 * Chart-of-accounts rules (PRD FR-33-001, QA BUG-060/142/143/145/146).
 *
 *  - An account with children is a header (induk) account: it only sums its
 *    children and never receives a journal line.
 *  - An inactive account receives no new journal line.
 *  - A control account (is_control, e.g. Piutang Siswa) is fed by automatic
 *    postings only; its sub-ledger lives in SEKOCI billing (BR-ACC-06).
 *  - Accounts are never hard-deleted: they are deactivated, and only when
 *    nothing still depends on them.
 */
final class Accounts
{
    public static function find(string $code): ?object
    {
        return DB::table('mst_account')->where('acc_code', $code)->first();
    }

    public static function isParent(string $code): bool
    {
        return DB::table('mst_account')->where('parent_code', $code)->exists();
    }

    /** Debit-normal groups: 1 Asset, 5 Cost of Sales, 6 Expense, 8 Other Expense. */
    public static function defaultNormalBalance(int $group): string
    {
        return in_array($group, [1, 5, 6, 8], true) ? 'D' : 'K';
    }

    /** True when any journal line or balance row references the account. */
    public static function hasActivity(string $code): bool
    {
        return DB::table('trans_detail')->where('acc_code', $code)->exists()
            || DB::table('this_detail')->where('acc_code', $code)->exists()
            || DB::table('mst_saldo_news')->where('acc_code', $code)
                ->where(fn ($q) => $q->where('debet', '!=', 0)->orWhere('credit', '!=', 0))->exists();
    }

    /**
     * Reasons a journal line may not post to $code; empty = postable.
     *
     * @param  bool  $manual  manual journal (control accounts are refused)
     */
    public static function postingProblems(string $code, bool $manual = true): array
    {
        $acc = self::find($code);
        if (! $acc) {
            return ["Akun $code tidak ditemukan."];
        }
        $problems = [];
        if (! ($acc->is_active ?? true)) {
            $problems[] = "Akun $code tidak aktif.";
        }
        if (self::isParent($code)) {
            $problems[] = "Akun $code adalah akun induk; posting hanya ke akun detail.";
        }
        if ($manual && ($acc->is_control ?? false)) {
            $problems[] = "Akun $code adalah akun kontrol (sub-ledger di SEKOCI) dan hanya diisi posting otomatis. Koreksi melalui transaksi sumber (tagihan/pembayaran).";
        }

        return $problems;
    }

    /** @throws ValidationException */
    public static function assertPostable(string $code, string $field, bool $manual = true): void
    {
        if ($problems = self::postingProblems($code, $manual)) {
            throw ValidationException::withMessages([$field => $problems]);
        }
    }

    /**
     * Validate a parent choice for $code (null code = new account).
     *
     * @return array<string> problems
     */
    public static function parentProblems(?string $code, ?string $parent, ?int $group): array
    {
        if ($parent === null || $parent === '') {
            return [];
        }
        if ($code !== null && $parent === $code) {
            return ['Akun tidak boleh menjadi induk dirinya sendiri.'];
        }
        $p = self::find($parent);
        if (! $p) {
            return ["Akun induk $parent tidak ditemukan."];
        }
        $problems = [];
        if ($group !== null && (int) $p->acc_group !== $group) {
            $problems[] = "Akun induk $parent berada di kelompok berbeda; induk harus satu kelompok (tipe) dengan akunnya.";
        }
        if (! ($p->is_active ?? true)) {
            $problems[] = "Akun induk $parent tidak aktif.";
        }
        // A header account must not carry its own postings.
        if (! self::isParent($parent) && self::hasActivity($parent)) {
            $problems[] = "Akun $parent sudah memiliki transaksi/saldo sehingga tidak dapat dijadikan akun induk.";
        }
        // No cycles: walk up from the proposed parent.
        for ($cur = $p, $guard = 0; $cur && $cur->parent_code && $guard < 50; $guard++) {
            if ($code !== null && $cur->parent_code === $code) {
                $problems[] = 'Pilihan induk membentuk hierarki melingkar.';
                break;
            }
            $cur = self::find($cur->parent_code);
        }

        return $problems;
    }

    /** Ending balance (debit-positive) incl. opening, closed periods and posted open-period journals. */
    public static function balance(string $code): float
    {
        $a = Ledger::active();
        $row = Ledger::balances($a['year'], $a['period'])->firstWhere('acc_code', $code);

        return (float) ($row->ending ?? 0);
    }

    /**
     * Why $code cannot be deactivated (BUG-060 "ditolak dengan daftar
     * penyebab"); empty = it can.
     */
    public static function deactivationBlockers(string $code): array
    {
        $reasons = [];
        $bal = self::balance($code);
        if (abs($bal) >= 0.01) {
            $reasons[] = 'Saldo akun belum nol ('.number_format($bal, 2, ',', '.').').';
        }
        $open = DB::table('trans_detail as d')->join('trans_head as h', 'h.trans_no', '=', 'd.trans_no')
            ->where('d.acc_code', $code)->whereIn('h.status', ['draft', 'pending'])->distinct()->count('d.trans_no');
        if ($open > 0) {
            $reasons[] = "Dipakai di $open jurnal draft/menunggu approval.";
        }
        $maps = DB::table('acc_posting_maps')->where('acc_code', $code)->pluck('map_key')->all();
        if ($maps) {
            $reasons[] = 'Masih dipetakan untuk posting otomatis: '.implode(', ', $maps).'.';
        }
        $types = DB::table('mst_transaction')->where('trans_account', $code)->pluck('trans_code')->all();
        if ($types) {
            $reasons[] = 'Menjadi akun lawan tipe transaksi: '.implode(', ', $types).'.';
        }
        $children = DB::table('mst_account')->where('parent_code', $code)->where('is_active', true)->pluck('acc_code')->all();
        if ($children) {
            $reasons[] = 'Masih memiliki akun anak aktif: '.implode(', ', $children).'.';
        }

        return $reasons;
    }
}
