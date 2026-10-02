<?php

namespace App\Models;

use Illuminate\Database\Eloquent\Factories\HasFactory;
use Illuminate\Database\Eloquent\Model;

class ReportColHead extends Model
{
    use HasFactory;

    protected $guarded = [];

    public function reportColDetails()
    {
        return $this->hasMany(ReportColDetail::class)->orderBy('sort_order', 'asc');
    }

    public function reportFinals()
    {
        return $this->hasMany(ReportFinal::class);
    }

    /**
     * Get the column details applicable for a given period.
     * For 'fixed' templates, returns all columns.
     * For 'ytd', returns columns with period_key <= given period.
     * For 'bs_quarterly'/'is_quarterly', returns quarters whose period_key <= given period's quarter end.
     */
    public function getColumnsForPeriod(int $periode): \Illuminate\Database\Eloquent\Collection
    {
        $query = $this->reportColDetails()->orderBy('sort_order', 'asc');

        if ($this->col_type === 'ytd') {
            $query->where(function ($q) use ($periode) {
                $q->whereNotNull('period_key')->where('period_key', '<=', $periode);
            });
        } elseif (in_array($this->col_type, ['bs_quarterly', 'is_quarterly'])) {
            // Quarter end months: 3, 6, 9, 12
            $currentQuarterEnd = (int) (ceil($periode / 3) * 3);
            $quarterCount = (int) ceil($periode / 3);

            $query->where(function ($q) use ($currentQuarterEnd, $quarterCount) {
                // Current quarter: ALL columns (individual months + quarter total)
                $q->where('period_key', $currentQuarterEnd)
                    // Previous quarters: ONLY summary columns (period contains comma = multi-month)
                    ->orWhere(function ($q2) use ($currentQuarterEnd) {
                        $q2->where('period_key', '<', $currentQuarterEnd)
                            ->where('period', 'LIKE', '%,%');
                    });
                // For IS quarterly: include YTD column only when 2+ quarters
                if ($this->col_type === 'is_quarterly' && $quarterCount >= 2) {
                    $q->orWhereNull('period_key');
                }
            });
        }

        return $query->get();
    }
}
