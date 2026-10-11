<?php

namespace App\Models;

use Illuminate\Support\Facades\DB;
use Illuminate\Database\Eloquent\Model;
use Illuminate\Database\Eloquent\Factories\HasFactory;

class JournalTransaction extends Model
{
    use HasFactory;
    protected $table = 'trans_head';
    protected $guarded = [];
    // Set the primary key column
    protected $primaryKey = 'trans_no';

    // If the primary key is not an incrementing integer
    public $incrementing = false;

    // Set the data type of the primary key
    protected $keyType = 'string';

    protected function casts(): array
    {
        return [
            'trans_date' => 'datetime',
        ];
    }

    protected static function booted()
    {
        // BR-ACC-01: header fields of a non-draft journal are frozen; workflow
        // columns (status, status_by, status_at) and recomputed totals may change.
        static::updating(function ($h) {
            if ($h->isDirty(['trans_code', 'trans_no', 'trans_date'])) {
                \App\Support\Journals::assertEditable($h->getOriginal('trans_no'));
            }
        });
        static::deleting(fn ($h) => \App\Support\Journals::assertEditable($h->trans_no));
    }

    public function transType()
    {
        return $this->belongsTo(TransactionType::class, 'trans_code');
    }

    public function transDetail()
    {
        return $this->hasMany(TransDetail::class, 'trans_no', 'trans_no');
    }

    public function transDetail2()
    {
        return $this->hasMany(TransDetail::class, 'trans_no', 'trans_no');
        // ->where(function ($query) {
        //     $query->where('period', $this->period);
        // });
    }

    // public function getTransDateAttribute($value)
    // {
    //     return \Carbon\Carbon::createFromFormat('Y-m-d', $value)->format('m/d/Y');
    // }

    public function scopeWithEqualDebetAndCredit($query)
    {
        return $query->select('trans_head.trans_no', 'trans_head.trans_code', 'trans_head.trans_date', 'trans_head.status')
            ->join('trans_detail', 'trans_head.trans_no', '=', 'trans_detail.trans_no')
            ->select(
                'trans_head.trans_no',
                'trans_head.trans_code',
                'trans_head.trans_date',
                'trans_head.status',
                DB::raw('SUM(trans_detail.debet) as total_debet'),
                DB::raw('SUM(trans_detail.credit) as total_credit')
            )
            ->groupBy('trans_head.trans_no', 'trans_head.trans_code', 'trans_head.trans_date', 'trans_head.status')
            ->havingRaw('SUM(trans_detail.debet) = SUM(trans_detail.credit)');
    }

    public function scopeWithNotEqualDebetAndCredit($query)
    {
        return $query->select('trans_head.trans_no', 'trans_head.trans_code', 'trans_head.trans_date', 'trans_head.status')
            ->join('trans_detail', 'trans_head.trans_no', '=', 'trans_detail.trans_no')
            ->select(
                'trans_head.trans_no',
                'trans_head.trans_code',
                'trans_head.trans_date',
                'trans_head.status',
                DB::raw('SUM(trans_detail.debet) as total_debet'),
                DB::raw('SUM(trans_detail.credit) as total_credit')
            )
            ->groupBy('trans_head.trans_no', 'trans_head.trans_code', 'trans_head.trans_date', 'trans_head.status')
            ->havingRaw('SUM(trans_detail.debet) != SUM(trans_detail.credit)');
    }
}
