<?php

namespace App\Models;

use Illuminate\Database\Eloquent\Model;
use Illuminate\Database\Eloquent\Relations\HasMany;

class PriorYearAdjustmentSession extends Model
{
    protected $guarded = [];

    protected $casts = [
        'adjustment_date' => 'date',
        'finalized_at' => 'datetime',
    ];

    public function lines(): HasMany
    {
        return $this->hasMany(PriorYearAdjustmentLine::class, 'session_id');
    }

    public function getTotalDebetAttribute(): float
    {
        return (float) $this->lines->sum('debet');
    }

    public function getTotalCreditAttribute(): float
    {
        return (float) $this->lines->sum('credit');
    }

    public function getIsBalancedAttribute(): bool
    {
        return abs($this->total_debet - $this->total_credit) < 0.01;
    }
}
