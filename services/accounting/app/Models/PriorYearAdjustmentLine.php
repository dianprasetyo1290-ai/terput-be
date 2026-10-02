<?php

namespace App\Models;

use Illuminate\Database\Eloquent\Model;
use Illuminate\Database\Eloquent\Relations\BelongsTo;

class PriorYearAdjustmentLine extends Model
{
    protected $guarded = [];

    protected $casts = [
        'debet' => 'float',
        'credit' => 'float',
        'curr_rate' => 'float',
        'curr_debet' => 'float',
        'curr_credit' => 'float',
    ];

    public function session(): BelongsTo
    {
        return $this->belongsTo(PriorYearAdjustmentSession::class, 'session_id');
    }
}
