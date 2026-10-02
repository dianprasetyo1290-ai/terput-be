<?php

namespace App\Models;

use Illuminate\Database\Eloquent\Factories\HasFactory;
use Illuminate\Database\Eloquent\Model;

class MstBudget extends Model
{
    use HasFactory;

    protected $table = 'mst_budget';
    protected $guarded = [];

    /**
     * Get budget value for specific periods.
     * mst_budget has pivoted columns: act_01..act_13
     *
     * @param string $accCodePattern Account code pattern (with %)
     * @param array $periods Array of period numbers [1,2,3]
     * @param string|null $divisi Division code or null for all
     * @return float
     */
    public static function getBudgetForPeriods(string $accCodePattern, array $periods, ?string $divisi = null): float
    {
        $columns = collect($periods)->map(function ($p) {
            $padded = str_pad((int) $p, 2, '0', STR_PAD_LEFT);
            return "COALESCE(act_{$padded}, 0)";
        })->implode(' + ');

        $query = self::selectRaw("acc_code, ({$columns}) as total_budget")
            ->where('acc_code', 'LIKE', $accCodePattern);

        if ($divisi !== null) {
            $query->where('div_code', $divisi);
        }

        return (float) $query->get()->sum('total_budget');
    }
}
