<?php

namespace App\Models;

use Illuminate\Database\Eloquent\Factories\HasFactory;
use Illuminate\Database\Eloquent\Model;

class Division extends Model
{
    use HasFactory;
    protected $table = 'mst_division';
    protected $guarded = [];
    // Set the primary key column
    protected $primaryKey = 'div_code';

    // If the primary key is not an incrementing integer
    public $incrementing = false;

    // Set the data type of the primary key
    protected $keyType = 'string';
}
