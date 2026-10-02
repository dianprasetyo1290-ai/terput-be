<?php

namespace App\Models;

use Illuminate\Database\Eloquent\Factories\HasFactory;
use Illuminate\Database\Eloquent\Model;

class ReportFinal extends Model
{
    use HasFactory;
    protected $guarded = [];

    public function reportData()
    {
        return $this->hasMany(ReportFinalData::class, 'report_final_id')->orderBy('sort_order')->orderBy('id');
    }

    public function reportColHead()
    {
        return $this->belongsTo(ReportColHead::class);
    }
}
