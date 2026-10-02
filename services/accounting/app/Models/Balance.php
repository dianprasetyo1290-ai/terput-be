<?php

namespace App\Models;

use App\Casts\FormattedAmount;
use Illuminate\Database\Eloquent\Factories\HasFactory;
use Illuminate\Database\Eloquent\Model;

class Balance extends Model
{
    use HasFactory;
    protected $table = 'mst_saldo_news';
    protected $guarded = [];
    // protected $casts = [
    //     'act_00' => FormattedAmount::class,
    //     'act_01' => FormattedAmount::class,
    //     'act_02' => FormattedAmount::class,
    //     'act_03' => FormattedAmount::class,
    //     'act_04' => FormattedAmount::class,
    //     'act_05' => FormattedAmount::class,
    //     'act_06' => FormattedAmount::class,
    //     'act_07' => FormattedAmount::class,
    //     'act_08' => FormattedAmount::class,
    //     'act_09' => FormattedAmount::class,
    //     'act_10' => FormattedAmount::class,
    //     'act_11' => FormattedAmount::class,
    //     'act_12' => FormattedAmount::class,
    //     'act_13' => FormattedAmount::class,
    //     'actk_00' => FormattedAmount::class,
    //     'actk_01' => FormattedAmount::class,
    //     'actk_02' => FormattedAmount::class,
    //     'actk_03' => FormattedAmount::class,
    //     'actk_04' => FormattedAmount::class,
    //     'actk_05' => FormattedAmount::class,
    //     'actk_06' => FormattedAmount::class,
    //     'actk_07' => FormattedAmount::class,
    //     'actk_08' => FormattedAmount::class,
    //     'actk_09' => FormattedAmount::class,
    //     'actk_10' => FormattedAmount::class,
    //     'actk_11' => FormattedAmount::class,
    //     'actk_12' => FormattedAmount::class,
    //     'actk_13' => FormattedAmount::class,
    //     'lst_00' => FormattedAmount::class,
    //     'lst_01' => FormattedAmount::class,
    //     'lst_02' => FormattedAmount::class,
    //     'lst_03' => FormattedAmount::class,
    //     'lst_04' => FormattedAmount::class,
    //     'lst_05' => FormattedAmount::class,
    //     'lst_06' => FormattedAmount::class,
    //     'lst_07' => FormattedAmount::class,
    //     'lst_08' => FormattedAmount::class,
    //     'lst_09' => FormattedAmount::class,
    //     'lstk_00' => FormattedAmount::class,
    //     'lstk_01' => FormattedAmount::class,
    //     'lstk_02' => FormattedAmount::class,
    //     'lstk_03' => FormattedAmount::class,
    //     'lstk_04' => FormattedAmount::class,
    //     'lstk_05' => FormattedAmount::class,
    //     'lstk_06' => FormattedAmount::class,
    //     'lstk_07' => FormattedAmount::class,
    //     'lstk_08' => FormattedAmount::class,
    //     'lstk_09' => FormattedAmount::class,
    //     'lstk_10' => FormattedAmount::class,
    //     'lstk_11' => FormattedAmount::class,
    //     'lstk_12' => FormattedAmount::class,
    //     'lstk_13' => FormattedAmount::class,
    //     'snd_00' => FormattedAmount::class,
    //     'snd_01' => FormattedAmount::class,
    //     'snd_02' => FormattedAmount::class,
    //     'snd_03' => FormattedAmount::class,
    //     'snd_04' => FormattedAmount::class,
    //     'snd_05' => FormattedAmount::class,
    //     'snd_06' => FormattedAmount::class,
    //     'snd_07' => FormattedAmount::class,
    //     'snd_08' => FormattedAmount::class,
    //     'snd_09' => FormattedAmount::class,
    //     'snd_10' => FormattedAmount::class,
    //     'snd_11' => FormattedAmount::class,
    //     'snd_12' => FormattedAmount::class,
    //     'snd_13' => FormattedAmount::class,
    //     'sndk_00' => FormattedAmount::class,
    //     'sndk_01' => FormattedAmount::class,
    //     'sndk_02' => FormattedAmount::class,
    //     'sndk_03' => FormattedAmount::class,
    //     'sndk_04' => FormattedAmount::class,
    //     'sndk_05' => FormattedAmount::class,
    //     'sndk_06' => FormattedAmount::class,
    //     'sndk_07' => FormattedAmount::class,
    //     'sndk_08' => FormattedAmount::class,
    //     'sndk_09' => FormattedAmount::class,
    //     'sndk_10' => FormattedAmount::class,
    //     'sndk_11' => FormattedAmount::class,
    //     'sndk_12' => FormattedAmount::class,
    //     'sndk_13' => FormattedAmount::class,
    // ];

    public function chartofaccount()
    {
        return $this->belongsTo(ChartOfAccount::class, 'acc_code', 'acc_code');
    }

    public function division()
    {
        return $this->belongsTo(Division::class, 'div_code', 'div_code');
    }
}
