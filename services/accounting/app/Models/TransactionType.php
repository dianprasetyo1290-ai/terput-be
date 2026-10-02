<?php

namespace App\Models;

use Illuminate\Database\Eloquent\Factories\HasFactory;
use Illuminate\Database\Eloquent\Model;

class TransactionType extends Model
{
    use HasFactory;
    protected $table = 'mst_transaction';
    protected $guarded = [];
    // Set the primary key column
    protected $primaryKey = 'trans_code';

    // If the primary key is not an incrementing integer
    public $incrementing = false;

    // Set the data type of the primary key
    protected $keyType = 'string';

    public function akunLawan()
    {
        return $this->belongsTo(ChartOfAccount::class, 'trans_account', 'acc_code');
    }
}
