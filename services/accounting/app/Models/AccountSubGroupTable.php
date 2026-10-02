<?php

namespace App\Models;

use Illuminate\Database\Eloquent\Factories\HasFactory;
use Illuminate\Database\Eloquent\Model;

class AccountSubGroupTable extends Model
{
    use HasFactory;

    protected $fillable = [
        'account_group_table_id',
        'name',
    ];

    public function accountGroup()
    {
        return $this->belongsTo(AccountGroupTable::class, 'account_group_table_id');
    }
}
