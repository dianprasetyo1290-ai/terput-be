<?php

namespace App\Support;

use Illuminate\Support\Facades\DB;
use Illuminate\Validation\ValidationException;

/**
 * COA templates per unit (QA BUG-141, FR-33-002 / AC-33-01: "template COA
 * sekolah dapat disalin dan disesuaikan per unit").
 *
 *  - saveFromCoa(): snapshot the active SHARED accounts (unit_scope NULL) as a
 *    named template;
 *  - copyToUnit(): create the template's accounts for one unit. A unit copy of
 *    account X gets the code "X.<UNIT>" and unit_scope = <UNIT>, so journal
 *    lines of another unit can not post to it (Accounts::postingProblems).
 *    Parents inside the template map to the unit copy; existing codes are
 *    skipped, so copying again only adds what is missing. The copy is then
 *    adjusted like any account on the COA page.
 */
final class CoaTemplates
{
    /** @throws ValidationException */
    public static function saveFromCoa(string $name, ?string $description, string $userId): int
    {
        $name = trim($name);
        if ($name === '' || mb_strlen($name) > 100) {
            throw ValidationException::withMessages(['coaTpl.name' => 'Nama template wajib diisi (maks. 100 karakter).']);
        }
        if (DB::table('acc_coa_templates')->whereRaw('lower(name) = ?', [mb_strtolower($name)])->exists()) {
            throw ValidationException::withMessages(['coaTpl.name' => "Nama template \"$name\" sudah dipakai."]);
        }
        $accounts = DB::table('mst_account')->where('is_active', true)->whereNull('unit_scope')->orderBy('acc_code')->get();
        if ($accounts->isEmpty()) {
            throw ValidationException::withMessages(['coaTpl.name' => 'Belum ada akun bersama yang aktif untuk dijadikan template.']);
        }

        return DB::transaction(function () use ($name, $description, $userId, $accounts) {
            $id = (int) DB::table('acc_coa_templates')->insertGetId([
                'name' => $name, 'description' => $description ? mb_substr(trim($description), 0, 250) : null,
                'created_by' => $userId, 'created_at' => now(), 'updated_at' => now(),
            ]);
            foreach ($accounts as $a) {
                DB::table('acc_coa_template_lines')->insert([
                    'template_id' => $id, 'acc_code' => $a->acc_code, 'acc_name' => (string) $a->acc_name,
                    'acc_group' => (int) $a->acc_group, 'sub_acc_group' => $a->sub_acc_group, 'parent_code' => $a->parent_code,
                    'normal_balance' => $a->normal_balance, 'is_cash' => (bool) $a->is_cash,
                    'is_interunit' => (bool) $a->is_interunit, 'is_control' => (bool) $a->is_control,
                ]);
            }
            AuditLog::record('coa_template.create', (string) $id, ['name' => $name, 'accounts' => $accounts->count()], $userId);

            return $id;
        });
    }

    public static function unitCode(string $code, string $div): string
    {
        return $code.'.'.$div;
    }

    /**
     * Copy template $id to unit $div. Returns [created, skipped].
     *
     * @return array{created:int, skipped:int}
     *
     * @throws ValidationException
     */
    public static function copyToUnit(int $id, string $div, string $userId): array
    {
        $tpl = DB::table('acc_coa_templates')->where('id', $id)->first();
        if (! $tpl) {
            throw ValidationException::withMessages(['coaCopy.template_id' => 'Template COA wajib dipilih.']);
        }
        if (! DB::table('mst_division')->where('div_code', $div)->exists()) {
            throw ValidationException::withMessages(['coaCopy.div_code' => 'Unit wajib dipilih.']);
        }
        $lines = DB::table('acc_coa_template_lines')->where('template_id', $id)->orderBy('acc_code')->get();
        $tooLong = $lines->filter(fn ($l) => strlen(self::unitCode($l->acc_code, $div)) > 15)->pluck('acc_code')->all();
        if ($tooLong) {
            throw ValidationException::withMessages(['coaCopy.template_id' => 'Kode akun unit melebihi 15 karakter untuk: '
                .implode(', ', array_slice($tooLong, 0, 10)).(count($tooLong) > 10 ? ' …' : '')
                .'. Gunakan kode unit yang lebih pendek atau template dengan kode akun lebih pendek.']);
        }
        $inTemplate = $lines->pluck('acc_code')->flip();
        $currency = DB::table('parameter')->value('base_currency');
        if ($currency !== null && ! DB::table('mst_currency')->where('curr_code', $currency)->exists()) {
            $currency = null;
        }

        return DB::transaction(function () use ($tpl, $lines, $inTemplate, $div, $currency, $userId) {
            $created = 0;
            $skipped = 0;
            foreach ($lines as $l) {
                $code = self::unitCode($l->acc_code, $div);
                if (DB::table('mst_account')->where('acc_code', $code)->exists()) {
                    $skipped++;

                    continue;
                }
                $parent = $l->parent_code;
                if ($parent !== null && isset($inTemplate[$parent])) {
                    $parent = self::unitCode($parent, $div);
                }
                DB::table('mst_account')->insert([
                    'acc_code' => $code, 'acc_name' => mb_substr($l->acc_name, 0, 50), 'acc_group' => $l->acc_group,
                    'sub_acc_group' => $l->sub_acc_group, 'acc_currency' => $currency, 'parent_code' => $parent,
                    'normal_balance' => $l->normal_balance, 'is_active' => true, 'is_cash' => $l->is_cash,
                    'is_interunit' => $l->is_interunit, 'is_control' => $l->is_control, 'unit_scope' => $div,
                    'created_at' => now(), 'updated_at' => now(),
                ]);
                $created++;
            }
            AuditLog::record('coa_template.copy', (string) $tpl->id, ['name' => $tpl->name, 'unit' => $div, 'created' => $created, 'skipped' => $skipped], $userId);

            return ['created' => $created, 'skipped' => $skipped];
        });
    }

    public static function delete(int $id, string $userId): void
    {
        $name = DB::table('acc_coa_templates')->where('id', $id)->value('name');
        if ($name === null) {
            throw ValidationException::withMessages(['coaTpl.name' => 'Template COA tidak ditemukan.']);
        }
        DB::table('acc_coa_templates')->where('id', $id)->delete();
        AuditLog::record('coa_template.delete', (string) $id, ['name' => $name], $userId);
    }
}
