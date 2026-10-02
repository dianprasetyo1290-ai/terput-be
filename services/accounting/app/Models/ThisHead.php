<?php

namespace App\Models;

use Illuminate\Database\Eloquent\Factories\HasFactory;
use Illuminate\Database\Eloquent\Model;
use Illuminate\Support\Facades\DB;

class ThisHead extends Model
{
    use HasFactory;
    protected $table = 'this_head';
    protected $guarded = [];
    protected $primaryKey = 'trans_no';
    public $incrementing = false;
    protected $keyType = 'string';

    protected function casts(): array
    {
        return [
            'trans_date' => 'datetime',
        ];
    }

    public function transType()
    {
        return $this->belongsTo(TransactionType::class, 'trans_code');
    }

    public function transDetail()
    {
        return $this->hasMany(ThisDetail::class, 'trans_no', 'trans_no');
    }

    public function transDetail2()
    {
        return $this->hasMany(ThisDetail::class, 'trans_no', 'trans_no')
        ->where(function ($query) {
            $query->where('period', $this->period);
        });
    }

    public function getTransDateAttribute($value)
    {
        return \Carbon\Carbon::createFromFormat('Y-m-d', $value)->format('m/d/Y');
    }

    public function scopeWithEqualDebetAndCredit($query)
    {
        return $query->join('this_detail', 'this_head.trans_no', '=', 'this_detail.trans_no')
        ->select(
            'this_head.trans_no',
            'this_head.trans_code',
            'this_head.trans_date',
            'this_head.period',
            DB::raw('SUM(this_detail.debet) as total_debet'),
            DB::raw('SUM(this_detail.credit) as total_credit')
        )
            ->groupBy('this_head.trans_no', 'this_head.trans_code', 'this_head.trans_date', 'this_head.period')
            ->havingRaw('SUM(this_detail.debet) = SUM(this_detail.credit)');
    }

    public function scopeWithNotEqualDebetAndCredit($query)
    {
        return $query->join('this_detail', 'this_head.trans_no', '=', 'this_detail.trans_no')
        ->select(
            'this_head.trans_no',
            'this_head.trans_code',
            'this_head.trans_date',
            'this_head.period',
            DB::raw('SUM(this_detail.debet) as total_debet'),
            DB::raw('SUM(this_detail.credit) as total_credit')
        )
            ->groupBy('this_head.trans_no', 'this_head.trans_code', 'this_head.trans_date', 'this_head.period')
            ->havingRaw('SUM(this_detail.debet) != SUM(this_detail.credit)');
    }
}
