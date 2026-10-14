<?php

namespace App\Support;

use Illuminate\Support\Facades\DB;

/**
 * CSV import of the chart of accounts and opening balances (PRD FR-33-002,
 * QA BUG-059/141). Both run in one transaction and are all-or-nothing: every
 * row is validated, any error rolls the whole file back, and a dry run
 * (preview) always rolls back.
 *
 * COA columns:     acc_code, acc_name, acc_group, parent_code, normal_balance,
 *                  is_cash, is_interunit, is_control   (header row required;
 *                  only acc_code, acc_name, acc_group are mandatory)
 * Opening columns: acc_code, div_code, debit, credit
 */
final class Importer
{
    public const COA_COLUMNS = ['acc_code', 'acc_name', 'acc_group', 'parent_code', 'normal_balance', 'is_cash', 'is_interunit', 'is_control'];
    public const OPENING_COLUMNS = ['acc_code', 'div_code', 'debit', 'credit'];
    public const MAX_ROWS = 5000;

    /**
     * Parse CSV text (comma or semicolon, UTF-8, optional BOM) into rows keyed
     * by the lower-cased header. Returns [rows, errors].
     */
    public static function parse(string $csv, array $required): array
    {
        $csv = preg_replace('/^\xEF\xBB\xBF/', '', $csv);
        $lines = preg_split('/\r\n|\n|\r/', trim($csv));
        if (! $lines || trim($lines[0]) === '') {
            return [[], ['File kosong.']];
        }
        $delim = substr_count($lines[0], ';') > substr_count($lines[0], ',') ? ';' : ',';
        $header = array_map(fn ($h) => strtolower(trim($h)), str_getcsv($lines[0], $delim, '"', ''));
        if ($missing = array_diff($required, $header)) {
            return [[], ['Kolom wajib tidak ada di baris judul: '.implode(', ', $missing).'.']];
        }
        if (count($lines) - 1 > self::MAX_ROWS) {
            return [[], ['Maksimal '.self::MAX_ROWS.' baris per file.']];
        }
        $rows = [];
        foreach (array_slice($lines, 1) as $i => $line) {
            if (trim($line) === '' || trim(str_replace($delim, '', $line)) === '') {
                continue;
            }
            $vals = str_getcsv($line, $delim, '"', '');
            $row = ['_line' => $i + 2];
            foreach ($header as $k => $h) {
                $row[$h] = trim((string) ($vals[$k] ?? ''));
            }
            $rows[] = $row;
        }

        return [$rows, $rows ? [] : ['Tidak ada baris data.']];
    }

    /** Import / update accounts. Existing accounts keep group and normal balance once used. */
    public static function coa(array $rows, bool $dryRun): array
    {
        $errors = [];
        $created = $updated = 0;
        $groups = DB::table('account_group_tables')->pluck('id')->map(fn ($g) => (int) $g)->all();
        $seen = [];

        DB::beginTransaction();
        try {
            // Pass 1: accounts without parents, so a parent may appear anywhere in the file.
            foreach ($rows as $r) {
                $l = "Baris {$r['_line']}";
                $code = $r['acc_code'];
                if ($code === '' || preg_match('/\s/', $code) || mb_strlen($code) > 15) {
                    $errors[] = "$l: kode akun wajib, tanpa spasi, maks 15 karakter.";
                    continue;
                }
                if (isset($seen[$code])) {
                    $errors[] = "$l: kode $code muncul lebih dari sekali.";
                    continue;
                }
                $seen[$code] = true;
                if ($r['acc_name'] === '' || mb_strlen($r['acc_name']) > 50) {
                    $errors[] = "$l: nama akun wajib, maks 50 karakter.";
                }
                $group = (int) $r['acc_group'];
                if (! in_array($group, $groups, true)) {
                    $errors[] = "$l: kelompok akun {$r['acc_group']} tidak dikenal (".implode(', ', $groups).').';
                    continue;
                }
                $nb = strtoupper($r['normal_balance'] ?? '') ?: Accounts::defaultNormalBalance($group);
                if (! in_array($nb, ['D', 'K'], true)) {
                    $errors[] = "$l: saldo normal harus D atau K.";
                    continue;
                }
                $values = [
                    'acc_name' => $r['acc_name'],
                    'acc_group' => $group,
                    'normal_balance' => $nb,
                    'is_cash' => self::bool($r['is_cash'] ?? ''),
                    'is_interunit' => self::bool($r['is_interunit'] ?? ''),
                    'is_control' => self::bool($r['is_control'] ?? ''),
                    'updated_at' => now(),
                ];
                $existing = Accounts::find($code);
                if ($existing) {
                    if (Accounts::hasActivity($code) && ((int) $existing->acc_group !== $group || ($existing->normal_balance ?? $nb) !== $nb)) {
                        $errors[] = "$l: akun $code sudah memiliki transaksi/saldo; kelompok dan saldo normal tidak dapat diubah.";
                        continue;
                    }
                    DB::table('mst_account')->where('acc_code', $code)->update($values);
                    $updated++;
                } else {
                    DB::table('mst_account')->insert($values + [
                        'acc_code' => $code, 'acc_currency' => DB::table('parameter')->value('base_currency'),
                        'is_active' => true, 'created_at' => now(),
                    ]);
                    $created++;
                }
            }

            // Pass 2: parents, validated against the file and the existing COA together.
            foreach ($rows as $r) {
                $parent = $r['parent_code'] ?? '';
                if ($r['acc_code'] === '' || ! isset($seen[$r['acc_code']])) {
                    continue;
                }
                $p = $parent === '' ? [] : Accounts::parentProblems($r['acc_code'], $parent, (int) $r['acc_group']);
                foreach ($p as $m) {
                    $errors[] = "Baris {$r['_line']}: $m";
                }
                if (! $p) {
                    DB::table('mst_account')->where('acc_code', $r['acc_code'])->update(['parent_code' => $parent ?: null]);
                }
            }

            if ($errors || $dryRun) {
                DB::rollBack();
            } else {
                AuditLog::record('import.coa', 'mst_account', ['created' => $created, 'updated' => $updated]);
                DB::commit();
            }
        } catch (\Throwable $e) {
            DB::rollBack();
            throw $e;
        }

        return ['errors' => $errors, 'created' => $created, 'updated' => $updated, 'committed' => ! $errors && ! $dryRun];
    }

    /**
     * Opening balances (mst_saldo_news periode 0) of $year. The file is the
     * complete opening trial balance: every existing periode-0 row of the
     * year not in the file is set to zero, so the result is exactly the file
     * and stays balanced.
     */
    public static function opening(array $rows, int $year, bool $dryRun): array
    {
        $errors = [];
        $sum = [];
        $td = $tc = 0.0;
        $divisions = DB::table('mst_division')->pluck('div_code')->flip();

        foreach ($rows as $r) {
            $l = "Baris {$r['_line']}";
            $code = $r['acc_code'];
            $div = $r['div_code'];
            $d = self::amount($r['debit']);
            $c = self::amount($r['credit']);
            if ($d === null || $c === null) {
                $errors[] = "$l: debit/kredit harus angka.";
                continue;
            }
            if ($d < 0 || $c < 0) {
                $errors[] = "$l: nilai tidak boleh negatif.";
                continue;
            }
            if ($d > 0 && $c > 0) {
                $errors[] = "$l: isi debit ATAU kredit, tidak keduanya.";
                continue;
            }
            if (! isset($divisions[$div])) {
                $errors[] = "$l: unit/divisi '$div' tidak ada.";
                continue;
            }
            // Control accounts (e.g. Piutang Siswa) may carry an opening balance;
            // only manual journals are kept off them.
            foreach (Accounts::postingProblems($code, manual: false) as $m) {
                $errors[] = "$l: $m";
            }
            $key = "$code|$div";
            $sum[$key] = [($sum[$key][0] ?? 0) + $d, ($sum[$key][1] ?? 0) + $c];
            $td += $d;
            $tc += $c;
        }
        $td = round($td, 2);
        $tc = round($tc, 2);
        if (! $errors && abs($td - $tc) >= 0.01) {
            $errors[] = sprintf('Saldo awal tidak seimbang: total debit %s, total kredit %s.', number_format($td, 2, ',', '.'), number_format($tc, 2, ',', '.'));
        }

        $committed = false;
        if (! $errors && ! $dryRun) {
            DB::transaction(function () use ($sum, $year, $td) {
                DB::table('mst_saldo_news')->where('tahun', $year)->where('periode', 0)
                    ->update(['debet' => 0, 'credit' => 0, 'updated_at' => now()]);
                $rows = [];
                foreach ($sum as $key => [$d, $c]) {
                    [$acc, $div] = explode('|', $key, 2);
                    // One side only: net the account/unit.
                    $net = round($d - $c, 2);
                    $rows[] = ['acc_code' => $acc, 'div_code' => $div, 'periode' => 0, 'tahun' => $year,
                        'debet' => max($net, 0), 'credit' => max(-$net, 0), 'created_at' => now(), 'updated_at' => now()];
                }
                foreach (array_chunk($rows, 500) as $chunk) {
                    DB::table('mst_saldo_news')->upsert($chunk, ['acc_code', 'div_code', 'periode', 'tahun'], ['debet', 'credit', 'updated_at']);
                }
                AuditLog::record('import.opening', (string) $year, ['rows' => count($rows), 'total' => $td]);
            });
            $committed = true;
        }

        return ['errors' => $errors, 'rows' => count($sum), 'debit' => $td, 'credit' => $tc, 'committed' => $committed];
    }

    private static function bool(string $v): bool
    {
        return in_array(strtolower(trim($v)), ['1', 'y', 'ya', 'yes', 'true', 'x'], true);
    }

    /** "1.234.567,50", "1234567.50", "1,234,567.50", "" -> float; garbage -> null. */
    private static function amount(string $v): ?float
    {
        $v = str_replace([' ', 'Rp', 'rp'], '', trim($v));
        if ($v === '' || $v === '-') {
            return 0.0;
        }
        if (preg_match('/^-?\d{1,3}(\.\d{3})+(,\d+)?$/', $v) || preg_match('/^-?\d+,\d+$/', $v)) {
            $v = str_replace(['.', ','], ['', '.'], $v);
        } else {
            $v = str_replace(',', '', $v);
        }

        return is_numeric($v) ? (float) $v : null;
    }
}
