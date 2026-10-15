<?php

namespace App\Support;

use App\Models\JournalTransaction;
use App\Models\TransDetail;
use Illuminate\Support\Carbon;
use Illuminate\Support\Facades\DB;
use Illuminate\Validation\ValidationException;

/**
 * Journal templates (QA BUG-144, FR-34-006): a named, balanced set of lines a
 * user turns into a DRAFT journal with one click, or that a recurring
 * schedule turns into a draft every month (see RecurringJournals). A draft
 * made from a template is an ordinary manual journal: it is submitted and
 * approved by the normal maker-checker workflow (BR-ACC-01/02).
 */
final class JournalTemplates
{
    /**
     * Validate and save a template. $lines = [[acc_code, div_code,
     * description, debit, credit], ...] (assoc arrays). Returns the id.
     *
     * @throws ValidationException
     */
    public static function save(?int $id, string $name, string $transCode, ?string $description, array $lines, string $userId): int
    {
        $name = trim($name);
        $errors = [];
        if ($name === '' || mb_strlen($name) > 100) {
            $errors['tpl.name'][] = 'Nama template wajib diisi (maks. 100 karakter).';
        } elseif (DB::table('acc_journal_templates')->whereRaw('lower(name) = ?', [mb_strtolower($name)])
            ->when($id, fn ($q) => $q->where('id', '!=', $id))->exists()) {
            $errors['tpl.name'][] = "Nama template \"$name\" sudah dipakai.";
        }
        if (! DB::table('mst_transaction')->where('trans_code', $transCode)->exists()) {
            $errors['tpl.trans_code'][] = 'Tipe transaksi wajib dipilih.';
        }
        $clean = [];
        foreach (array_values($lines) as $i => $l) {
            $n = $i + 1;
            $acc = trim((string) ($l['acc_code'] ?? ''));
            $div = trim((string) ($l['div_code'] ?? ''));
            $desc = trim((string) ($l['description'] ?? ''));
            $d = self::amount($l['debit'] ?? 0);
            $c = self::amount($l['credit'] ?? 0);
            if ($acc === '' && $div === '' && $desc === '' && ! $d && ! $c) {
                continue; // empty row left in the form
            }
            $errors = array_merge_recursive($errors, self::lineProblems($n, $acc, $div, $desc, $d, $c));
            $clean[] = ['acc_code' => $acc, 'div_code' => $div, 'description' => mb_substr($desc, 0, 250), 'debit' => (float) $d, 'credit' => (float) $c];
        }
        if (count($clean) < 2) {
            $errors['tpl.lines'][] = 'Template minimal memiliki 2 baris.';
        }
        $td = round(array_sum(array_column($clean, 'debit')), 2);
        $tc = round(array_sum(array_column($clean, 'credit')), 2);
        if (abs($td - $tc) >= 0.01) {
            $errors['tpl.lines'][] = sprintf('Total debit (%s) dan kredit (%s) tidak seimbang.', self::fmt($td), self::fmt($tc));
        }
        if ($errors) {
            throw ValidationException::withMessages($errors);
        }

        return DB::transaction(function () use ($id, $name, $transCode, $description, $clean, $userId) {
            $isNew = ! $id;
            $row = ['name' => $name, 'trans_code' => $transCode, 'description' => $description ? mb_substr(trim($description), 0, 250) : null, 'updated_at' => now()];
            if ($id) {
                DB::table('acc_journal_templates')->where('id', $id)->update($row);
                DB::table('acc_journal_template_lines')->where('template_id', $id)->delete();
            } else {
                $id = (int) DB::table('acc_journal_templates')->insertGetId($row + ['created_by' => $userId, 'created_at' => now()]);
            }
            foreach ($clean as $i => $l) {
                DB::table('acc_journal_template_lines')->insert($l + ['template_id' => $id, 'line_no' => $i + 1]);
            }
            AuditLog::record($isNew ? 'journal_template.create' : 'journal_template.save', (string) $id, ['name' => $name, 'lines' => count($clean)]);

            return $id;
        });
    }

    /** Lines of an existing journal (open or history) as template lines. */
    public static function linesFromJournal(string $transNo): array
    {
        $lines = DB::table('trans_detail')->where('trans_no', $transNo)->orderBy('trans_count')->get();
        if ($lines->isEmpty()) {
            $date = DB::table('this_head')->where('trans_no', $transNo)->orderByDesc('trans_date')->value('trans_date');
            $lines = $date ? DB::table('this_detail')->where('trans_no', $transNo)->whereDate('trans_date', $date)->orderBy('trans_count')->get() : collect();
        }

        return $lines->map(fn ($l) => [
            'acc_code' => (string) $l->acc_code, 'div_code' => (string) $l->div_code,
            'description' => (string) $l->description, 'debit' => (float) $l->debet, 'credit' => (float) $l->credit,
        ])->all();
    }

    /**
     * Create a DRAFT journal from template $id dated $date (must be inside the
     * open period). $reference makes the draft idempotent (unique per
     * source+reference). Returns the new journal number.
     *
     * @throws ValidationException
     */
    public static function instantiate(int $id, string $date, string $userId, ?string $reference = null): string
    {
        $tpl = DB::table('acc_journal_templates')->where('id', $id)->first();
        if (! $tpl) {
            throw ValidationException::withMessages(['template' => 'Template tidak ditemukan.']);
        }
        if (! $tpl->is_active) {
            throw ValidationException::withMessages(['template' => "Template \"{$tpl->name}\" tidak aktif."]);
        }
        $lines = DB::table('acc_journal_template_lines')->where('template_id', $id)->orderBy('line_no')->get();
        $problems = [];
        foreach ($lines as $l) {
            foreach (self::lineProblems((int) $l->line_no, (string) $l->acc_code, (string) $l->div_code, (string) $l->description, (float) $l->debit, (float) $l->credit) as $msgs) {
                array_push($problems, ...$msgs);
            }
        }
        if ($lines->count() < 2) {
            $problems[] = 'Template belum memiliki minimal 2 baris.';
        }
        if ($problems) {
            throw ValidationException::withMessages(['template' => array_values(array_unique($problems))]);
        }
        try {
            $date = Carbon::parse($date)->toDateString();
        } catch (\Throwable) {
            throw ValidationException::withMessages(['date' => 'Tanggal jurnal tidak valid.']);
        }
        Ledger::assertOpenDate($date, 'date');

        return DB::transaction(function () use ($tpl, $lines, $date, $userId, $reference) {
            if ($reference !== null) {
                DB::statement('SELECT pg_advisory_xact_lock(hashtext(?))', ['manual|'.$reference]);
                if ($no = DB::table('trans_head')->where('source', 'manual')->where('reference', $reference)->value('trans_no')) {
                    return (string) $no;
                }
            }
            $no = Journals::newNumber($reference !== null ? 'RJ' : 'TP');
            JournalTransaction::create([
                'trans_code' => $tpl->trans_code,
                'trans_no' => $no,
                'trans_date' => $date,
                'trans_update' => now(),
                'trans_user' => $userId,
                'reference' => $reference ?? mb_substr('Template: '.$tpl->name, 0, 100),
            ]);
            $currency = DB::table('parameter')->value('base_currency');
            foreach ($lines->values() as $i => $l) {
                TransDetail::create([
                    'trans_code' => $tpl->trans_code,
                    'trans_no' => $no,
                    'trans_count' => $i + 1,
                    'acc_code' => $l->acc_code,
                    'div_code' => $l->div_code,
                    'description' => $l->description,
                    'debet' => (float) $l->debit,
                    'credit' => (float) $l->credit,
                    'curr_code' => $currency,
                    'curr_rate' => 1,
                    'curr_debet' => (float) $l->debit,
                    'curr_credit' => (float) $l->credit,
                    'trans_date' => $date,
                ]);
            }
            AuditLog::record('journal_template.use', $no, ['template' => $tpl->id, 'date' => $date, 'reference' => $reference], $userId);

            return $no;
        });
    }

    /** @return array<string, array<string>> */
    private static function lineProblems(int $n, string $acc, string $div, string $desc, float $d, float $c): array
    {
        $e = [];
        if ($acc === '') {
            $e["tpl.lines.$n"][] = "Baris $n: akun wajib dipilih.";
        } else {
            foreach (Accounts::postingProblems($acc, true, $div !== '' ? $div : null) as $m) {
                $e["tpl.lines.$n"][] = "Baris $n: $m";
            }
        }
        if ($div === '' || ! DB::table('mst_division')->where('div_code', $div)->exists()) {
            $e["tpl.lines.$n"][] = "Baris $n: unit (divisi) wajib dipilih.";
        }
        if ($desc === '') {
            $e["tpl.lines.$n"][] = "Baris $n: keterangan wajib diisi.";
        }
        if ($d < 0 || $c < 0) {
            $e["tpl.lines.$n"][] = "Baris $n: nilai tidak boleh negatif.";
        } elseif (($d > 0) === ($c > 0)) {
            $e["tpl.lines.$n"][] = "Baris $n: isi salah satu, debit ATAU kredit.";
        }

        return $e;
    }

    private static function amount(mixed $v): float
    {
        if (is_string($v)) {
            $v = str_replace([' ', ','], ['', '.'], trim($v));
        }

        return is_numeric($v) ? round((float) $v, 2) : 0.0;
    }

    private static function fmt(float $v): string
    {
        return number_format($v, 2, ',', '.');
    }
}
