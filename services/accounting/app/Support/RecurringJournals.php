<?php

namespace App\Support;

use Illuminate\Support\Carbon;
use Illuminate\Support\Facades\DB;
use Illuminate\Validation\ValidationException;

/**
 * Scheduled recurring journals (QA BUG-144, FR-34-006). A schedule books a
 * DRAFT from its template on day D of every month; drafts then go through the
 * normal submit/approve workflow.
 *
 * runDue() is what the daily job (accounting:recurring-journals, Laravel
 * scheduler) and the "Proses jadwal sekarang" button call. It is idempotent:
 * acc_recurring_runs has one row per (schedule, run date), and the draft's
 * reference "recurring:<id>:<date>" is unique on trans_head.
 *
 *  - run date inside the open period  -> draft created (or failure recorded);
 *  - run date in a closed period       -> recorded as failed ("periode sudah
 *    ditutup"), the schedule moves on;
 *  - run date in a period not open yet -> nothing happens; it is picked up
 *    once End of Month opens that period.
 */
final class RecurringJournals
{
    /** @throws ValidationException */
    public static function create(int $templateId, int $day, string $start, ?string $end, string $userId): int
    {
        $errors = [];
        $tpl = DB::table('acc_journal_templates')->where('id', $templateId)->first();
        if (! $tpl) {
            $errors['rec.template_id'][] = 'Template wajib dipilih.';
        } elseif (! $tpl->is_active) {
            $errors['rec.template_id'][] = 'Template tidak aktif.';
        }
        if ($day < 1 || $day > 28) {
            $errors['rec.day_of_month'][] = 'Tanggal jurnal harus 1–28 (berlaku untuk semua bulan).';
        }
        try {
            $startD = Carbon::parse($start)->startOfDay();
        } catch (\Throwable) {
            $startD = null;
            $errors['rec.start_date'][] = 'Tanggal mulai tidak valid.';
        }
        $endD = null;
        if ($end !== null && $end !== '') {
            try {
                $endD = Carbon::parse($end)->startOfDay();
            } catch (\Throwable) {
                $errors['rec.end_date'][] = 'Tanggal selesai tidak valid.';
            }
        }
        if ($errors) {
            throw ValidationException::withMessages($errors);
        }
        $next = self::firstRun($startD, $day);
        if ($endD && $next->gt($endD)) {
            throw ValidationException::withMessages(['rec.end_date' => 'Tanggal selesai sebelum jadwal pertama ('.$next->toDateString().').']);
        }

        $id = (int) DB::table('acc_recurring_journals')->insertGetId([
            'template_id' => $templateId, 'day_of_month' => $day, 'start_date' => $startD->toDateString(),
            'end_date' => $endD?->toDateString(), 'next_run' => $next->toDateString(), 'is_active' => true,
            'created_by' => $userId, 'created_at' => now(), 'updated_at' => now(),
        ]);
        AuditLog::record('recurring.create', (string) $id, ['template' => $templateId, 'day' => $day, 'next_run' => $next->toDateString()]);

        return $id;
    }

    /** First date on/after $start that falls on $day. */
    public static function firstRun(Carbon $start, int $day): Carbon
    {
        $d = $start->copy()->day($day);

        return $d->lt($start) ? $d->addMonthNoOverflow()->day($day) : $d;
    }

    /**
     * Book every due run up to $today. Returns counts.
     *
     * @return array{created: int, failed: int, waiting: int}
     */
    public static function runDue(?string $today = null): array
    {
        $today = Carbon::parse($today ?? now()->toDateString())->toDateString();
        $out = ['created' => 0, 'failed' => 0, 'waiting' => 0];
        $active = Ledger::active();
        $range = Ledger::range($active['year'], $active['period']);

        $schedules = DB::table('acc_recurring_journals')->where('is_active', true)
            ->whereDate('next_run', '<=', $today)->orderBy('id')->get();
        foreach ($schedules as $s) {
            // Several months may be due (job was down): catch up one by one.
            for ($guard = 0; $guard < 24; $guard++) {
                $s = DB::table('acc_recurring_journals')->where('id', $s->id)->first();
                $run = Carbon::parse($s->next_run)->toDateString();
                if (! $s->is_active || $run > $today) {
                    break;
                }
                if ($s->end_date && $run > Carbon::parse($s->end_date)->toDateString()) {
                    DB::table('acc_recurring_journals')->where('id', $s->id)->update(['is_active' => false, 'updated_at' => now()]);
                    AuditLog::record('recurring.finished', (string) $s->id, ['end_date' => $s->end_date], 'SYSTEM');
                    break;
                }
                if ($range && $run > $range[1]) {
                    $out['waiting']++;
                    break; // that period is not open yet
                }
                $result = self::runOne($s, $run, $range);
                $out[$result]++;
            }
        }

        return $out;
    }

    /** @return 'created'|'failed' */
    private static function runOne(object $s, string $run, ?array $range): string
    {
        $inserted = DB::table('acc_recurring_runs')->insertOrIgnore([
            'recurring_id' => $s->id, 'run_date' => $run, 'status' => 'running', 'created_at' => now(), 'updated_at' => now(),
        ]);
        $status = 'failed';
        if ($inserted === 0) {
            // Already handled (a second job run, or a retry from the page).
            $status = DB::table('acc_recurring_runs')->where('recurring_id', $s->id)->where('run_date', $run)->value('status') === 'created' ? 'created' : 'failed';
        } else {
            $status = self::attempt($s, $run, $range);
        }
        DB::table('acc_recurring_journals')->where('id', $s->id)->update([
            'next_run' => Carbon::parse($run)->addMonthNoOverflow()->day((int) $s->day_of_month)->toDateString(),
            'updated_at' => now(),
        ]);

        return $status;
    }

    /** Try to book the draft for ($s, $run); updates the run row. */
    private static function attempt(object $s, string $run, ?array $range): string
    {
        $no = null;
        $error = null;
        if ($range && $run < $range[0]) {
            $error = "Periode untuk tanggal $run sudah ditutup; jurnal berulang tidak dibuat.";
        } else {
            try {
                $no = JournalTemplates::instantiate((int) $s->template_id, $run, (string) $s->created_by, "recurring:{$s->id}:$run");
            } catch (ValidationException $e) {
                $error = implode(' ', collect($e->errors())->flatten()->all());
            } catch (\Throwable $e) {
                report($e);
                $error = 'Gagal membuat draft: '.$e->getMessage();
            }
        }
        DB::table('acc_recurring_runs')->where('recurring_id', $s->id)->where('run_date', $run)->update([
            'status' => $no ? 'created' : 'failed', 'trans_no' => $no, 'message' => $error, 'updated_at' => now(),
        ]);
        DB::table('acc_recurring_journals')->where('id', $s->id)->update([
            'last_run_at' => now(), 'last_trans_no' => $no ?? DB::raw('last_trans_no'), 'last_error' => $error, 'updated_at' => now(),
        ]);
        AuditLog::record($no ? 'recurring.run' : 'recurring.failed', (string) $s->id, ['date' => $run, 'trans_no' => $no, 'error' => $error], 'SYSTEM');

        return $no ? 'created' : 'failed';
    }

    /**
     * Retry one failed run from the page (after fixing the template or the
     * account). Returns the draft number.
     *
     * @throws ValidationException
     */
    public static function retry(int $recurringId, string $runDate): string
    {
        $s = DB::table('acc_recurring_journals')->where('id', $recurringId)->first();
        $r = DB::table('acc_recurring_runs')->where('recurring_id', $recurringId)->whereDate('run_date', $runDate)->first();
        if (! $s || ! $r) {
            throw ValidationException::withMessages(['run' => 'Jadwal tidak ditemukan.']);
        }
        if ($r->status !== 'failed') {
            throw ValidationException::withMessages(['run' => 'Hanya jadwal yang gagal yang dapat dicoba lagi.']);
        }
        $a = Ledger::active();
        $status = self::attempt($s, Carbon::parse($r->run_date)->toDateString(), Ledger::range($a['year'], $a['period']));
        $row = DB::table('acc_recurring_runs')->where('recurring_id', $recurringId)->whereDate('run_date', $runDate)->first();
        if ($status !== 'created') {
            throw ValidationException::withMessages(['run' => $row->message ?: 'Gagal membuat draft.']);
        }

        return (string) $row->trans_no;
    }

    /** Mark a failed run as handled by hand (e.g. booked manually). */
    public static function skip(int $recurringId, string $runDate, string $userId): void
    {
        $q = DB::table('acc_recurring_runs')->where('recurring_id', $recurringId)->whereDate('run_date', $runDate)->where('status', 'failed');
        $row = $q->first();
        $n = $row ? $q->update(['status' => 'skipped', 'message' => trim(($row->message ?? '')." (dilewati oleh $userId)"), 'updated_at' => now()]) : 0;
        if ($n === 0) {
            throw ValidationException::withMessages(['run' => 'Hanya jadwal yang gagal yang dapat dilewati.']);
        }
        AuditLog::record('recurring.skip', (string) $recurringId, ['date' => $runDate]);
    }
}
