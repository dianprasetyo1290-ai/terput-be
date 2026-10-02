<?php

namespace App\Models;

use Illuminate\Database\Eloquent\Factories\HasFactory;
use Illuminate\Database\Eloquent\Model;

class Budgeting extends Model
{
    use HasFactory;
    protected $table = 'mst_budget';
    protected $guarded = [];

    public function chartofaccount()
    {
        return $this->belongsTo(ChartOfAccount::class, 'acc_code', 'acc_code');
    }

    public function division()
    {
        return $this->belongsTo(Division::class, 'div_code', 'div_code');
    }
}
