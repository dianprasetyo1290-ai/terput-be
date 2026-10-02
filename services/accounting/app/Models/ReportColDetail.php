<?php

namespace App\Models;

use Illuminate\Database\Eloquent\Factories\HasFactory;
use Illuminate\Database\Eloquent\Model;

class ReportColDetail extends Model
{
    use HasFactory;

    protected $guarded = [];

    public function reportColHead()
    {
        return $this->belongsTo(ReportColHead::class);
    }
}
