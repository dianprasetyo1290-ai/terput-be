<?php

namespace App\Models;

use Illuminate\Database\Eloquent\Factories\HasFactory;
use Illuminate\Database\Eloquent\Model;

class Parameter extends Model
{
    use HasFactory;
    protected $table = 'parameter';
    protected $guarded = [];
    // Set the primary key column
    protected $primaryKey = 'serial_no';

    // If the primary key is not an incrementing integer
    public $incrementing = false;

    // Set the data type of the primary key
    protected $keyType = 'string';

    public function getPeriodAttribute($value)
    {
        return str_pad($value, 2, '0', STR_PAD_LEFT);
    }

    // Mutator
    public function setPeriodAttribute($value)
    {
        $this->attributes['period'] = str_pad($value, 2, '0', STR_PAD_LEFT);
    }
}
