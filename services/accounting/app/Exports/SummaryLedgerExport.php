<?php

namespace App\Exports;

use App\Models\Parameter;
use Carbon\Carbon;
use Illuminate\Support\Collection;
use Illuminate\Support\Facades\DB;
use Maatwebsite\Excel\Concerns\FromCollection;
use Maatwebsite\Excel\Concerns\ShouldAutoSize;
use Maatwebsite\Excel\Concerns\WithColumnFormatting;
use Maatwebsite\Excel\Concerns\WithEvents;
use Maatwebsite\Excel\Concerns\WithHeadings;
use Maatwebsite\Excel\Events\AfterSheet;
use PhpOffice\PhpSpreadsheet\Style\NumberFormat;

class SummaryLedgerExport implements FromCollection, ShouldAutoSize, WithColumnFormatting, WithEvents, WithHeadings
{
    protected $acc_code;

    protected $acc_code2;

    protected $periodeFrom;   // "yyyymm"

    protected $periodeTo;     // "yyyymm"

    protected $codedivfrom;

    protected $codedivto;

    protected $year;          // int (YYYY)

    protected $this_period;   // bool: true=trans_*, false=this_*

    public function __construct($acc_code, $acc_code2, $periodeFrom, $periodeTo, $codedivfrom, $codedivto, $year, $this_period)
    {
        $this->acc_code = $acc_code;
        $this->acc_code2 = $acc_code2;
        $this->periodeFrom = $periodeFrom;
        $this->periodeTo = $periodeTo;
        $this->codedivfrom = $codedivfrom;
        $this->codedivto = $codedivto;
        $this->year = (int) $year;
        $this->this_period = filter_var($this_period, FILTER_VALIDATE_BOOLEAN);
    }

    public function collection(): Collection
    {
        $detailTable = $this->this_period ? 'trans_detail' : 'this_detail';
        $headTable = $this->this_period ? 'trans_head' : 'this_head';

        // Period cutoff untuk beginning balance
        // mst_saldo_news.periode = integer 1-13, bukan yyyymm
        $paramPeriod = optional(Parameter::first())->period; // bisa null
        $periodCutoff = $this->periodeFrom ?: ($paramPeriod ?: 1);

        // Rentang tanggal (untuk trans_*)
        [$dateFrom, $dateTo] = $this->dateRangeFromPeriods($this->periodeFrom, $this->periodeTo);

        $q = DB::table('mst_account as a')
            ->leftJoin("{$detailTable} as d", 'd.acc_code', '=', 'a.acc_code')
            ->leftJoin("{$headTable} as h", function ($join) use ($detailTable) {
                $join->on('d.trans_code', '=', 'h.trans_code')
                    ->on('d.trans_no', '=', 'h.trans_no');
                if ($detailTable === 'this_detail') {
                    $join->on('d.period', '=', 'h.period')
                        ->whereRaw('d.trans_date::date = h.trans_date::date');
                }
            })
            ->whereBetween('a.acc_code', [$this->acc_code, $this->acc_code2]);

        // Filter divisi (opsional)
        if (! empty($this->codedivfrom) && ! empty($this->codedivto)) {
            $q->whereBetween('d.div_code', [$this->codedivfrom, $this->codedivto]);
        } elseif (! empty($this->codedivfrom)) {
            $q->where('d.div_code', $this->codedivfrom);
        }

        // Filter periode/tanggal
        if ($detailTable === 'this_detail') {
            if (! empty($this->periodeFrom) && ! empty($this->periodeTo)) {
                $q->whereBetween(DB::raw('CAST(d.period AS INTEGER)'), [(int) $this->periodeFrom, (int) $this->periodeTo]);
            }
            $q->whereYear('d.trans_date', $this->year);
        } else {
            if ($dateFrom && $dateTo) {
                $q->whereBetween('h.trans_date', [$dateFrom->toDateTimeString(), $dateTo->toDateTimeString()]);
            } else {
                // Fallback: filter tahun (portable Postgres)
                $q->whereRaw('EXTRACT(YEAR FROM h.trans_date) = ?', [$this->year]);
            }
        }

        // SELECT dengan subquery saldo awal + binding
        $q->select([
            // Kolom kode akun sebagai TEXT; kita format kolom A ke text via AfterSheet
            'a.acc_code',
            'a.acc_name',
        ])
            ->selectRaw(
                '(SELECT COALESCE(SUM(s.debet - s.credit),0)
               FROM mst_saldo_news s
              WHERE s.acc_code = a.acc_code
                AND s.tahun    = ?
                AND s.periode  < ?) AS beg_balance',
                [$this->year, (int) $periodCutoff]
            )
            ->selectRaw('COALESCE(SUM(d.debet),0)  AS debet')
            ->selectRaw('COALESCE(SUM(d.credit),0) AS credit')
            ->selectRaw(
                '( (SELECT COALESCE(SUM(s2.debet - s2.credit),0)
                  FROM mst_saldo_news s2
                 WHERE s2.acc_code = a.acc_code
                   AND s2.tahun    = ?
                   AND s2.periode  < ?)
               + COALESCE(SUM(d.debet),0)
               - COALESCE(SUM(d.credit),0)
             ) AS end_balance',
                [$this->year, (int) $periodCutoff]
            )
            ->groupBy('a.acc_code', 'a.acc_name')
            ->orderBy('a.acc_code');

        $rows = $q->get();

        // Pastikan urutan kolom sesuai headings()
        return collect($rows)->map(function ($r) {
            return [
                $r->acc_code,
                $r->acc_name,
                (float) ($r->beg_balance ?? 0),
                (float) ($r->debet ?? 0),
                (float) ($r->credit ?? 0),
                (float) ($r->end_balance ?? 0),
            ];
        });
    }

    public function headings(): array
    {
        return ['acc_code', 'name', 'beg_balance', 'debet', 'credit', 'end_balance'];
    }

    public function registerEvents(): array
    {
        return [
            AfterSheet::class => function (AfterSheet $event) {
                // Header bold
                $event->sheet->getDelegate()->getStyle('A1:F1')->getFont()->setBold(true)->setSize(12);

                // Kolom A dipaksa TEXT (hindari trimming nol/kode akun jadi angka)
                $event->sheet->getDelegate()->getStyle('A:A')
                    ->getNumberFormat()->setFormatCode(NumberFormat::FORMAT_TEXT);
            },
        ];
    }

    public function columnFormats(): array
    {
        // Format angka ribuan dengan 2 desimal
        return [
            'C' => NumberFormat::FORMAT_NUMBER_COMMA_SEPARATED1, // beg_balance
            'D' => NumberFormat::FORMAT_NUMBER_COMMA_SEPARATED1, // debet
            'E' => NumberFormat::FORMAT_NUMBER_COMMA_SEPARATED1, // credit
            'F' => NumberFormat::FORMAT_NUMBER_COMMA_SEPARATED1, // end_balance
        ];
    }

    /** Helper: konversi yyyymm -> [awal bulan, akhir bulan] */
    private function dateRangeFromPeriods(?string $periodFrom, ?string $periodTo, string $tz = 'Asia/Jakarta'): array
    {
        if (empty($periodFrom) || empty($periodTo)) {
            return [null, null];
        }
        if (! preg_match('/^\d{6}$/', $periodFrom) || ! preg_match('/^\d{6}$/', $periodTo)) {
            return [null, null];
        }

        $y1 = (int) substr($periodFrom, 0, 4);
        $m1 = (int) substr($periodFrom, 4, 2);
        $y2 = (int) substr($periodTo, 0, 4);
        $m2 = (int) substr($periodTo, 4, 2);

        $start = Carbon::create($y1, $m1, 1, 0, 0, 0, $tz)->startOfDay();
        $end = Carbon::create($y2, $m2, 1, 0, 0, 0, $tz)->endOfMonth()->endOfDay();
        if ($start->gt($end)) {
            [$start, $end] = [
                $end->copy()->startOfMonth()->startOfDay(),
                $start->copy()->endOfMonth()->endOfDay(),
            ];
        }

        return [$start, $end];
    }
}
