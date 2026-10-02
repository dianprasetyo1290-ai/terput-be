<?php

namespace App\Models;

use Illuminate\Database\Eloquent\Factories\HasFactory;
use Illuminate\Database\Eloquent\Model;

class BalanceCurrency extends Model
{
    use HasFactory;

    protected $table = 'mst_saldocurr';
    protected $guarded = [];

    public function chartofaccount()
    {
        return $this->belongsTo(ChartOfAccount::class, 'acc_code', 'acc_code');
    }

    public function division()
    {
        return $this->belongsTo(Division::class, 'div_code', 'div_code');
    }

    public function currency()
    {
        return $this->belongsTo(Currency::class, 'curr_code', 'curr_code');
    }
}
