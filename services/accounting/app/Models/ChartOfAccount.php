<?php

namespace App\Models;

use Illuminate\Database\Eloquent\Factories\HasFactory;
use Illuminate\Database\Eloquent\Model;

class ChartOfAccount extends Model
{
    use HasFactory;
    protected $table = 'mst_account';
    protected $guarded = [];
    // Set the primary key column
    protected $primaryKey = 'acc_code';

    // If the primary key is not an incrementing integer
    public $incrementing = false;

    // Set the data type of the primary key
    protected $keyType = 'string';

    public function group()
    {
        return $this->belongsTo(AccountGroupTable::class, 'acc_group');
    }
    public function sub_group()
    {
        return $this->belongsTo(AccountSubGroupTable::class, 'sub_acc_group');
    }
}
