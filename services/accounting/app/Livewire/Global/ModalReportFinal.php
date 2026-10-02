<?php

namespace App\Livewire\Global;

use App\Models\Division;
use App\Models\MstSaldoNew;
use App\Models\Period;
use App\Models\ReportFinal;
use Illuminate\Support\Facades\DB;
use Livewire\Component;

class ModalReportFinal extends Component
{
    public $isShow = false;
    public $idNya, $urlNya, $year, $tahun, $period, $periode, $div, $div_id;
    public $colType = null;
    public $periodOptions = [];

    protected $listeners = ['showModal'];

    public function updatedPeriode()
    {
        $this->updatedYear();
    }

    public function updatedDivId()
    {
        $this->updatedYear();
    }

    public function updatedYear()
    {
        $this->urlNya = url('report') . '/' . $this->idNya;
        if ($this->periode) {
            $this->urlNya .= (strpos($this->urlNya, '?') === false ? '?' : '&') . 'periode=' . $this->periode;
        }

        if ($this->year) {
            $this->urlNya .= (strpos($this->urlNya, '?') === false ? '?' : '&') . 'tahun=' . $this->year;
        }

        if ($this->div_id) {
            $this->urlNya .= (strpos($this->urlNya, '?') === false ? '?' : '&') . 'divisi=' . $this->div_id;
        }
    }


    public function showModal($id = '')
    {
        $this->idNya = $id;
        $this->isShow = !$this->isShow;
        $this->periode = null;
        $this->year = null;

        if (! $this->isShow || $id === '' || $id === null) {
            return;
        }

        // Determine col_type and set period options
        $report = ReportFinal::with('reportColHead')->find($id);
        $this->colType = $report?->reportColHead?->col_type;

        if (in_array($this->colType, ['bs_quarterly', 'is_quarterly'])) {
            $this->periodOptions = [
                ['p' => '03', 'label' => 'Q1 (Jan - Mar)'],
                ['p' => '06', 'label' => 'Q2 (Apr - Jun)'],
                ['p' => '09', 'label' => 'Q3 (Jul - Sep)'],
                ['p' => '12', 'label' => 'Q4 (Oct - Dec)'],
            ];
        } else {
            // Add month_name for JS rendering
            $this->periodOptions = collect($this->period)->map(function ($p) {
                $p['month_name'] = getMonthName($p['no']);
                return $p;
            })->toArray();
        }

        // Build period options JSON for JS
        $optionsJson = json_encode($this->periodOptions);
        $isQuarterly = in_array($this->colType, ['bs_quarterly', 'is_quarterly']) ? 'true' : 'false';

        $this->js(<<<JS
            $(document).ready(function() {
                $('.year').select2();
                \$(".year option[text='']").attr("selected","selected");
                $('.year').on('change', function (e) {
                    var data = $('.year').select2("val");
                    \$wire.set('year', data);
                });

                // Rebuild periode options based on report type
                var periodeEl = $('.periode');
                if (periodeEl.data('select2')) {
                    periodeEl.select2('destroy');
                }
                periodeEl.empty();
                periodeEl.append('<option value="">Pilih</option>');

                var options = {$optionsJson};
                var isQuarterly = {$isQuarterly};
                options.forEach(function(opt) {
                    if (isQuarterly) {
                        periodeEl.append('<option value="' + opt.p + '">' + opt.label + '</option>');
                    } else {
                        periodeEl.append('<option value="' + opt.p + '">' + opt.p + ' [' + (opt.month_name || '') + ']</option>');
                    }
                });

                periodeEl.select2();
                periodeEl.val('').trigger('change');
                periodeEl.on('change', function (e) {
                    var data = periodeEl.select2("val");
                    \$wire.set('periode', data);
                });

                $('.div_id').select2();
                \$(".div_id option[text='']").attr("selected","selected");
                $('.div_id').on('change', function (e) {
                    var data = $('.div_id').select2("val");
                    \$wire.set('div_id', data);
                });
            });
        JS);
    }

    public function showReport()
    {
        $this->validate([
            'year' => 'required',
            'periode' => 'required',
        ]);

        $url = $this->urlNya;
        $this->js(<<<JS
            window.open("$url", "_blank");
        JS);
    }


    public function mount($id = '')
    {
        $this->idNya = $id;
        $this->tahun = MstSaldoNew::selectRaw('tahun')
            ->groupBy(DB::raw('tahun'))
            ->get();

        $period = Period::first();
        if ($period) {
            $this->period = $period->periods;
        } else {
            // Fallback jika master periode belum ada (mis. data belum di-seed)
            $this->period = collect(range(1, 13))->map(function ($i) {
                $p = str_pad($i, 2, '0', STR_PAD_LEFT);
                return [
                    'no' => $i,
                    'p' => $p,
                    'period' => $p,
                    'from' => null,
                    'to' => null,
                ];
            })->toArray();
        }

        $this->div = Division::all();
        $this->periodOptions = $this->period;
    }

    public function render()
    {
        return view('livewire.global.modal-report-final');
    }
}
