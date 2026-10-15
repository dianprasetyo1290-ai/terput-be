<?php

namespace App\Support;

use Illuminate\Support\Facades\DB;
use Illuminate\Support\Facades\Schema;
use Illuminate\Validation\ValidationException;

/**
 * Reopening the last closed period (QA BUG-178, TC-34-032 "hanya melalui
 * approval yang ditetapkan; kejadian tercatat di audit trail"). Conservative
 * by design:
 *  - only the most recently closed period of the active year (the one End of
 *    Month closed last) -- never an older one, never across End of Year;
 *  - only while the active period is still empty (no journal of any status),
 *    so nothing booked after the close gets mixed into the reopened month;
 *  - maker-checker: a request with a mandatory reason, approved by a
 *    DIFFERENT user holding editperiod_reopen (FinanceManager / SuperAdmin);
 *  - request, approval, rejection and the reopen itself go to acc_audit_log.
 *
 * Reopening moves the period's journals from history (this_*) back to the
 * open tables (trans_*) and sets the active period back. Its closed balance
 * row in mst_saldo_news is left alone: the ledger ignores the open period's
 * row, and End of Month rebuilds it from scratch when the period is closed
 * again.
 */
final class PeriodReopen
{
    public const MIN_REASON = 10;

    /** @return array{year:int, period:int}|null the period a reopen would target */
    public static function target(): ?array
    {
        $a = Ledger::active();
        $last = $a['period'] - 1;

        return $last >= 1 ? ['year' => $a['year'], 'period' => $last] : null;
    }

    /** Why the last closed period cannot be reopened now; empty = it can. */
    public static function blockers(): array
    {
        $t = self::target();
        if (! $t) {
            return ['Belum ada periode yang ditutup pada tahun buku berjalan. Periode tahun lalu tidak dapat dibuka kembali (koreksi melalui Penyesuaian Tahun Lalu).'];
        }
        $b = [];
        $n = DB::table('trans_head')->count();
        if ($n > 0) {
            $a = Ledger::active();
            $b[] = "Periode aktif ({$a['period']}/{$a['year']}) sudah berisi $n jurnal. Periode sebelumnya hanya dapat dibuka kembali selama periode aktif masih kosong.";
        }
        if (self::closedJournals($t)->count() === 0 && ! DB::table('mst_saldo_news')->where('tahun', $t['year'])->where('periode', $t['period'])->exists()) {
            $b[] = "Periode {$t['period']}/{$t['year']} tidak memiliki data tutup buku.";
        }

        return $b;
    }

    /** @throws ValidationException */
    public static function request(string $reason, string $userId): int
    {
        $reason = trim($reason);
        if (mb_strlen($reason) < self::MIN_REASON) {
            throw ValidationException::withMessages(['reason' => 'Alasan wajib diisi (minimal '.self::MIN_REASON.' karakter).']);
        }
        if ($b = self::blockers()) {
            throw ValidationException::withMessages(['reason' => $b]);
        }
        if (DB::table('acc_period_reopen_requests')->where('status', 'pending')->exists()) {
            throw ValidationException::withMessages(['reason' => 'Masih ada permintaan buka periode yang menunggu keputusan.']);
        }
        $t = self::target();
        $id = (int) DB::table('acc_period_reopen_requests')->insertGetId([
            'year' => $t['year'], 'period' => $t['period'], 'reason' => mb_substr($reason, 0, 2000),
            'status' => 'pending', 'requested_by' => $userId, 'requested_at' => now(),
        ]);
        AuditLog::record('period.reopen_request', self::label($t), ['request' => $id, 'reason' => $reason], $userId);

        return $id;
    }

    /** @throws ValidationException */
    public static function cancel(int $id, string $userId): void
    {
        $r = self::pending($id);
        if ((string) $r->requested_by !== $userId) {
            throw ValidationException::withMessages(['request' => 'Hanya pengaju yang dapat membatalkan permintaannya.']);
        }
        DB::table('acc_period_reopen_requests')->where('id', $id)->update(['status' => 'cancelled', 'decided_by' => $userId, 'decided_at' => now()]);
        AuditLog::record('period.reopen_cancel', self::label((array) $r), ['request' => $id], $userId);
    }

    /** @throws ValidationException */
    public static function reject(int $id, string $userId, string $note): void
    {
        $r = self::pending($id);
        self::assertChecker($r, $userId);
        if (mb_strlen(trim($note)) < 5) {
            throw ValidationException::withMessages(['note' => 'Alasan penolakan wajib diisi.']);
        }
        DB::table('acc_period_reopen_requests')->where('id', $id)
            ->update(['status' => 'rejected', 'decided_by' => $userId, 'decided_at' => now(), 'decision_note' => mb_substr(trim($note), 0, 2000)]);
        AuditLog::record('period.reopen_reject', self::label((array) $r), ['request' => $id, 'note' => $note], $userId);
    }

    /**
     * Approve and reopen. Returns the number of journals moved back.
     *
     * @throws ValidationException
     */
    public static function approve(int $id, string $userId, ?string $note = null): int
    {
        return DB::transaction(function () use ($id, $userId, $note) {
            // Same lock End of Month takes: no close and reopen at once.
            $param = DB::table('parameter')->lockForUpdate()->first();
            $r = DB::table('acc_period_reopen_requests')->where('id', $id)->lockForUpdate()->first();
            if (! $r || $r->status !== 'pending') {
                throw ValidationException::withMessages(['request' => 'Permintaan tidak ditemukan atau sudah diputuskan.']);
            }
            self::assertChecker($r, $userId);
            $t = self::target();
            if (! $param || ! $t || (int) $r->year !== $t['year'] || (int) $r->period !== $t['period']) {
                throw ValidationException::withMessages(['request' => 'Permintaan sudah tidak berlaku: periode terakhir yang ditutup telah berubah. Ajukan permintaan baru.']);
            }
            if ($b = self::blockers()) {
                throw ValidationException::withMessages(['request' => $b]);
            }

            $heads = self::closedJournals($t)->get();
            $headCols = array_flip(Schema::getColumnListing('trans_head'));
            $detailCols = array_flip(array_diff(Schema::getColumnListing('trans_detail'), ['id']));
            foreach ($heads as $h) {
                DB::table('trans_head')->insert(array_intersect_key((array) $h, $headCols));
                $lines = DB::table('this_detail')->where('trans_no', $h->trans_no)
                    ->whereDate('trans_date', $h->trans_date)->orderBy('trans_count')->get();
                foreach ($lines as $l) {
                    DB::table('trans_detail')->insert(array_intersect_key((array) $l, $detailCols));
                }
                DB::table('this_detail')->where('trans_no', $h->trans_no)->whereDate('trans_date', $h->trans_date)->delete();
                DB::table('this_head')->where('trans_no', $h->trans_no)->whereDate('trans_date', $h->trans_date)->delete();
            }

            DB::table('parameter')->update(['period' => $t['period'], 'updated_at' => now()]);
            DB::table('acc_period_reopen_requests')->where('id', $id)->update([
                'status' => 'approved', 'decided_by' => $userId, 'decided_at' => now(),
                'decision_note' => $note ? mb_substr(trim($note), 0, 2000) : null, 'moved_journals' => $heads->count(),
            ]);
            AuditLog::record('period.reopen', self::label($t), ['request' => $id, 'requested_by' => $r->requested_by,
                'reason' => $r->reason, 'journals' => $heads->count()], $userId);

            return $heads->count();
        });
    }

    private static function closedJournals(array $t)
    {
        return DB::table('this_head')
            ->whereRaw("nullif(trim(period), '')::int = ?", [$t['period']])
            ->whereRaw('extract(year from trans_date) = ?', [$t['year']])
            ->orderBy('trans_date')->orderBy('trans_no');
    }

    private static function pending(int $id): object
    {
        $r = DB::table('acc_period_reopen_requests')->where('id', $id)->first();
        if (! $r || $r->status !== 'pending') {
            throw ValidationException::withMessages(['request' => 'Permintaan tidak ditemukan atau sudah diputuskan.']);
        }

        return $r;
    }

    private static function assertChecker(object $r, string $userId): void
    {
        if ((string) $r->requested_by === $userId) {
            throw ValidationException::withMessages(['request' => 'Permintaan harus diputuskan oleh pengguna lain (maker-checker).']);
        }
    }

    public static function label(array $t): string
    {
        return $t['year'].'-'.str_pad((string) $t['period'], 2, '0', STR_PAD_LEFT);
    }
}
