<?php

namespace App\Support;

use App\Models\JournalTransaction;
use App\Models\TransDetail;
use Illuminate\Support\Facades\DB;
use Illuminate\Validation\ValidationException;

/**
 * Journal workflow (PRD FR-34-005): draft -> pending (menunggu approval) ->
 * posted -> reversed.
 *  - BR-ACC-01: only a draft may be edited or deleted; a posted journal is
 *    corrected by a reversal journal.
 *  - BR-ACC-02: posting requires lines and debit == credit.
 *  - BR-ACC-07: the journal date must be inside the open period.
 */
final class Journals
{
    public const STATUSES = ['draft', 'pending', 'posted', 'reversed'];

    /** Throws unless the open journal $transNo is (still) a draft. Unknown numbers pass: nothing to protect. */
    public static function assertEditable(?string $transNo, string $field = 'jt.trans_no'): void
    {
        if (! $transNo) {
            return;
        }
        $status = DB::table('trans_head')->where('trans_no', $transNo)->value('status');
        if ($status !== null && $status !== 'draft') {
            throw ValidationException::withMessages([$field => match ($status) {
                'pending' => "Jurnal $transNo sedang menunggu approval dan tidak dapat diubah. Kembalikan ke draft terlebih dahulu.",
                default => "Jurnal $transNo sudah berstatus $status dan tidak dapat diubah langsung. Gunakan jurnal pembalik.",
            }]);
        }
    }

    /** @return array{debit: float, credit: float, lines: int} */
    public static function totals(string $transNo): array
    {
        $t = DB::table('trans_detail')->where('trans_no', $transNo)
            ->selectRaw('coalesce(sum(debet),0) as d, coalesce(sum(credit),0) as c, count(*) as n')->first();

        return ['debit' => round((float) $t->d, 2), 'credit' => round((float) $t->c, 2), 'lines' => (int) $t->n];
    }

    /**
     * Apply a workflow action to an OPEN journal (trans_head).
     *
     * @throws ValidationException
     */
    public static function transition(string $transNo, string $action, string $userId): void
    {
        $head = DB::table('trans_head')->where('trans_no', $transNo)->first();
        if (! $head) {
            throw ValidationException::withMessages(['trans_no' => "Jurnal $transNo tidak ditemukan di periode aktif."]);
        }

        [$from, $to] = match ($action) {
            'submit' => ['draft', 'pending'],
            'reject' => ['pending', 'draft'],
            'approve' => ['pending', 'posted'],
            default => throw ValidationException::withMessages(['action' => 'Aksi tidak dikenal.']),
        };
        if ($head->status !== $from) {
            throw ValidationException::withMessages(['status' => "Aksi ini hanya untuk jurnal berstatus $from (status sekarang: {$head->status})."]);
        }

        // Segregation of duties (PRD §NFR "pembuat dan penyetuju dipisahkan"):
        // whoever made or submitted the journal cannot approve or reject it.
        if (in_array($action, ['approve', 'reject'], true)
            && in_array($userId, array_filter([(string) $head->trans_user, (string) $head->status_by]), true)) {
            throw ValidationException::withMessages(['status' => 'Penyetuju harus berbeda dengan pembuat/pengaju jurnal (maker-checker).']);
        }

        if ($action === 'submit' || $action === 'approve') {
            $problems = [];
            foreach (DB::table('trans_detail')->where('trans_no', $transNo)->orderBy('trans_count')->get() as $l) {
                if ((float) $l->debet < 0 || (float) $l->credit < 0) {
                    $problems[] = "Baris {$l->trans_count}: nilai tidak boleh negatif.";
                }
                if (trim((string) $l->description) === '') {
                    $problems[] = "Baris {$l->trans_count}: keterangan wajib diisi.";
                }
                foreach (Accounts::postingProblems((string) $l->acc_code, ($head->source ?? 'manual') === 'manual', $l->div_code !== null ? (string) $l->div_code : null) as $m) {
                    $problems[] = "Baris {$l->trans_count}: $m";
                }
            }
            if ($problems) {
                throw ValidationException::withMessages(['status' => array_values(array_unique($problems))]);
            }
        }

        if ($to !== 'draft') {
            $t = self::totals($transNo);
            if ($t['lines'] === 0) {
                throw ValidationException::withMessages(['status' => 'Jurnal belum memiliki baris debit/kredit.']);
            }
            if (abs($t['debit'] - $t['credit']) >= 0.01) {
                throw ValidationException::withMessages(['status' => sprintf(
                    'Debit dan kredit tidak seimbang (debit %s, kredit %s). Jurnal tidak dapat diposting.',
                    number_format($t['debit'], 2, ',', '.'), number_format($t['credit'], 2, ',', '.'))]);
            }
            Ledger::assertOpenDate((string) $head->trans_date, 'status');
        }

        DB::table('trans_head')->where('trans_no', $transNo)
            ->update(['status' => $to, 'status_by' => $userId, 'status_at' => now()]);
        AuditLog::record("journal.$action", $transNo, ['from' => $from, 'to' => $to]);
    }

    /**
     * BR-ACC-01: correct a posted journal by booking its mirror image (debit
     * <-> credit) in the open period. Works for open journals (trans_head) and
     * closed ones (this_head, identified by number + date since recurring
     * journals reuse numbers across months). Returns the reversal's number.
     *
     * @throws ValidationException
     */
    public static function reverse(string $transNo, ?string $date, string $userId): string
    {
        $open = DB::table('trans_head')->where('trans_no', $transNo)->first();
        if ($open) {
            $head = $open;
            $lines = DB::table('trans_detail')->where('trans_no', $transNo)->orderBy('trans_count')->get();
        } else {
            $q = DB::table('this_head')->where('trans_no', $transNo);
            if ($date) {
                $q->whereDate('trans_date', $date);
            }
            $matches = $q->get();
            if ($matches->count() > 1) {
                throw ValidationException::withMessages(['trans_no' => "Nomor $transNo dipakai di beberapa tanggal; sertakan tanggal jurnal."]);
            }
            $head = $matches->first();
            $lines = $head ? DB::table('this_detail')->where('trans_no', $transNo)
                ->whereDate('trans_date', $head->trans_date)->orderBy('trans_count')->get() : collect();
        }

        if (! $head) {
            throw ValidationException::withMessages(['trans_no' => "Jurnal $transNo tidak ditemukan."]);
        }
        if ($head->status !== 'posted') {
            throw ValidationException::withMessages(['status' => "Hanya jurnal berstatus posted yang dapat dibalik (status sekarang: {$head->status})."]);
        }
        if (! empty($head->reversal_of)) {
            throw ValidationException::withMessages(['status' => "Jurnal $transNo adalah jurnal pembalik; koreksi lewat jurnal baru."]);
        }
        if ((string) $head->trans_user === $userId) {
            throw ValidationException::withMessages(['status' => 'Pembalik jurnal harus dibuat oleh pengguna yang berbeda dengan pembuat jurnal asal (maker-checker).']);
        }

        $a = Ledger::active();
        $r = Ledger::range($a['year'], $a['period']);
        $today = now()->toDateString();
        $revDate = $r ? max($r[0], min($r[1], $today)) : $today;

        return DB::transaction(function () use ($head, $lines, $open, $transNo, $revDate, $userId) {
            $no = self::newNumber('RV');
            JournalTransaction::create([
                'trans_code' => $head->trans_code,
                'trans_no' => $no,
                'trans_date' => $revDate,
                'trans_update' => now(),
                'trans_user' => $userId,
                'source' => 'reversal',
                'reference' => "Pembalik $transNo",
                'reversal_of' => $transNo,
            ]);
            foreach ($lines->values() as $i => $l) {
                TransDetail::create([
                    'trans_code' => $head->trans_code,
                    'trans_no' => $no,
                    'trans_count' => $i + 1,
                    'acc_code' => $l->acc_code,
                    'div_code' => $l->div_code,
                    'description' => mb_substr("Pembalik $transNo: ".($l->description ?? ''), 0, 250),
                    'debet' => (float) $l->credit,
                    'credit' => (float) $l->debet,
                    'curr_code' => $l->curr_code,
                    'curr_rate' => $l->curr_rate,
                    'curr_debet' => $l->curr_credit,
                    'curr_credit' => $l->curr_debet,
                    'trans_date' => $revDate,
                ]);
            }
            DB::table('trans_head')->where('trans_no', $no)
                ->update(['status' => 'posted', 'status_by' => $userId, 'status_at' => now()]);

            $table = $open ? 'trans_head' : 'this_head';
            $q = DB::table($table)->where('trans_no', $transNo);
            if (! $open) {
                $q->whereDate('trans_date', $head->trans_date);
            }
            $q->update(['status' => 'reversed', 'status_by' => $userId, 'status_at' => now()]);
            AuditLog::record('journal.reverse', $transNo, ['reversal' => $no, 'date' => $revDate]);

            return $no;
        });
    }

    /** Next free journal number "<prefix><yymmddHHMMSS>" (max 15 chars, trans_no limit). */
    public static function newNumber(string $prefix): string
    {
        $base = $prefix.now()->format('ymdHis');
        $no = $base;
        for ($i = 1; DB::table('trans_head')->where('trans_no', $no)->exists(); $i++) {
            $no = substr($base, 0, 14 - strlen((string) $i)).'-'.$i;
        }

        return $no;
    }
}
