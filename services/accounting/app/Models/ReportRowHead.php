<?php

namespace App\Models;

use Illuminate\Database\Eloquent\Factories\HasFactory;
use Illuminate\Database\Eloquent\Model;

class ReportRowHead extends Model
{
    use HasFactory;
    protected $guarded = [];

    public function reportData()
    {
        return $this->hasMany(ReportFinalData::class, 'report_row_head_id');
    }

    public function reportRowDetail()    {
        return $this->hasMany(ReportRowDetail::class,'report_row_head_id')->orderBy('sort_order', 'asc')->orderBy('id', 'asc');
    }
}
