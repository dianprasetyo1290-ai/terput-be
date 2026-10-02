<?php

namespace App\Models;

use Illuminate\Database\Eloquent\Factories\HasFactory;
use Illuminate\Database\Eloquent\Model;

class Period extends Model
{
    use HasFactory;
    protected $table = 'mst_period';
    protected $guarded = [];

    // Accessor to get the period fields as an array
    public function getPeriodsAttribute()
    {
        $periods = [];
        for ($i = 1; $i <= 13; $i++) {
            // Menambahkan 0 di depan jika kurang dari 10
            $p = str_pad($i, 2, '0', STR_PAD_LEFT);

            $index = str_pad($i, 2, '0', STR_PAD_LEFT);
            $periods[] = [
                'no' => $i, // Sekarang ini akan menjadi 01, 02, dst. jika kurang dari 10
                'p' => $p, // Sekarang ini akan menjadi 01, 02, dst. jika kurang dari 10
                'period' => $this->attributes["p{$index}"],
                'from' => $this->attributes["p{$index}_from"],
                'to' => $this->attributes["p{$index}_to"],
            ];
        }

        return $periods;
    }
}
