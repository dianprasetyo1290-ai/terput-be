<?php

namespace App\Models;

use Illuminate\Database\Eloquent\Factories\HasFactory;
use Illuminate\Database\Eloquent\Model;

class Currency extends Model
{
    use HasFactory;
    protected $table = 'mst_currency';
    protected $guarded = [];
    // Set the primary key column
    protected $primaryKey = 'curr_code';

    // If the primary key is not an incrementing integer
    public $incrementing = false;

    // Set the data type of the primary key
    protected $keyType = 'string';
    
    // public function getRateSystemAttribute($value)
    // {
    //     if ($value === 'B') {
    //         return '1 Base = n Foreign';
    //     } elseif ($value === 'F') {
    //         return '1 Foreign = n Base';
    //     }

    //     return null;
    // }
}
