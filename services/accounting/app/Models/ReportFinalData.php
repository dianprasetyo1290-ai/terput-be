<?php

namespace App\Models;

use Illuminate\Database\Eloquent\Factories\HasFactory;
use Illuminate\Database\Eloquent\Model;

class ReportFinalData extends Model
{
    use HasFactory;
    protected $guarded = [];

    public function reportFinal()
    {
        return $this->belongsTo(ReportFinal::class, 'report_final_id');
    }

    public function reportRowHead()
    {
        return $this->belongsTo(ReportRowHead::class, 'report_row_head_id');
    }

    public function reportRowHeadOne()
    {
        return $this->belongsTo(ReportRowHead::class,'report_row_head_id');
    }
}
