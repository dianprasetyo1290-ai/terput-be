<?php

namespace App\Exports;

use App\Models\Parameter; // <— tambahkan
use Illuminate\Contracts\View\View;
use Illuminate\Support\Facades\DB;
use Maatwebsite\Excel\Concerns\FromView;
use Maatwebsite\Excel\Concerns\ShouldAutoSize;
use Maatwebsite\Excel\Concerns\WithColumnFormatting;

class LedgerExport implements FromView, ShouldAutoSize, WithColumnFormatting
{
    protected $acc_code;

    protected $acc_code2;

    protected $periodeFrom;

    protected $periodeTo;

    protected $codedivfrom;

    protected $codedivto;

    protected $year;

    protected $this_period;

    public function __construct($acc_code, $acc_code2, $periodeFrom, $periodeTo, $codedivfrom, $codedivto, $year, $this_period)
    {
        $this->acc_code = $acc_code;
        $this->acc_code2 = $acc_code2;
        $this->periodeFrom = $periodeFrom;
        $this->periodeTo = $periodeTo;
        $this->codedivfrom = $codedivfrom;
        $this->codedivto = $codedivto;
        $this->year = $year;
        $this->this_period = filter_var($this_period, FILTER_VALIDATE_BOOLEAN); // jadikan boolean
    }

    public function view(): View
    {
        $account = DB::table('mst_account')
            ->whereRaw('acc_code BETWEEN ? AND ?', [$this->acc_code, $this->acc_code2])
            ->get();

        $periodfrom = $this->periodeFrom;
        $periodto = $this->periodeTo ?: optional(Parameter::first())->period; // fallback ke parameter.period
        $codedivfrom = $this->codedivfrom;
        $codedivto = $this->codedivto;
        $tahun = $this->year;
        $this_period = $this->this_period; // boolean

        return view('exports.ledger', compact(
            'account',
            'periodfrom',
            'periodto',
            'codedivfrom',
            'codedivto',
            'tahun',
            'this_period'
        ));
    }

    public function columnFormats(): array
    {
        return []; // bisa diisi nanti kalau mau format angka/uang
    }
}
