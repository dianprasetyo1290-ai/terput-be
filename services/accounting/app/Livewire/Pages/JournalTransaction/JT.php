<?php

namespace App\Livewire\Pages\JournalTransaction;

use Carbon\Carbon;
use App\Models\Period;
use Livewire\Component;
use App\Models\Currency;
use App\Models\Division;
use App\Models\Parameter;
use Livewire\WithPagination;
use App\Models\ChartOfAccount;
use App\Models\TransactionType;
use App\Models\JournalTransaction;
use App\Livewire\Traits\WithListFilters;
use App\Livewire\Traits\WithTransactionHeader;
use App\Livewire\Traits\WithTransactionDetail;
use App\Livewire\Traits\WithJournalWorkflow;

class JT extends Component
{
    use WithPagination, WithListFilters, WithTransactionHeader, WithTransactionDetail, WithJournalWorkflow;

    protected $paginationTheme = 'bootstrap';

    public $form, $idNya, $formDetail, $idDetail, $count, $countDetail, $currency, $rate_system, $idEdit, $dfrom, $dto;
    public $jt = [
        'trans_code' => '',
        'trans_no' => '',
        'trans_date' => '',
    ];

    public $jtdetail = [];
    public $jteditdetail = [];
    public $search, $order = "DESC", $limit = 10, $filter = 'a', $statusFilter = '';

    public function mount()
    {
        $parameter = Parameter::first();

        if (!$parameter) {
            $this->dfrom = now()->startOfMonth()->format('Y-m-d');
            $this->dto = now()->endOfMonth()->format('Y-m-d');
            return;
        }

        $periodField = 'p' . $parameter->period;
        $period = Period::where('period_name', 'ilike', '%' . $parameter->year . '%')->first();

        if ($period) {
            $this->dfrom = $period->{$periodField . '_from'} ?? now()->startOfMonth();
            $this->dto = $period->{$periodField . '_to'} ?? now()->endOfMonth();
        } else {
            $this->dfrom = now()->startOfMonth();
            $this->dto = now()->endOfMonth();
        }

        try {
            $this->dfrom = Carbon::parse($this->dfrom)->format('Y-m-d');
            $this->dto = Carbon::parse($this->dto)->format('Y-m-d');
        } catch (\Exception $e) {
            $this->dfrom = now()->startOfMonth()->format('Y-m-d');
            $this->dto = now()->endOfMonth()->format('Y-m-d');
        }
    }

    public function render()
    {
        // Bug 2 fix: always chain onto the same $query, never replace it
        $query = JournalTransaction::with('transDetail2');

        if ($this->filter === 'b') {
            $query->withEqualDebetAndCredit();
        } elseif ($this->filter === 'u') {
            $query->withNotEqualDebetAndCredit();
        }

        if ($this->search) {
            $query->where(function ($q) {
                $q->where('trans_head.trans_code', 'ilike', '%' . $this->search . '%')
                  ->orWhere('trans_head.trans_no', 'ilike', '%' . $this->search . '%');
            });
        }

        if ($this->order) {
            $query->orderBy('trans_head.trans_date', $this->order);
        }

        if (in_array($this->statusFilter, \App\Support\Journals::STATUSES, true)) {
            $query->where('trans_head.status', $this->statusFilter);
        }

        $data = $query->paginate($this->limit);

        return view('livewire.pages.journal-transaction.j-t', [
            'data' => $data,
            'acc' => ChartOfAccount::orderBy('acc_name', 'ASC')->get(),
            // Only accounts a manual line may hit (QA BUG-143/145/146).
            'postableAcc' => ChartOfAccount::where('is_active', true)->where('is_control', false)
                ->whereNotIn('acc_code', ChartOfAccount::whereNotNull('parent_code')->select('parent_code'))
                ->orderBy('acc_code')->get(),
            'div' => Division::all(),
            'cur' => Currency::all(),
            'transaction_code' => TransactionType::all(),
        ]);
    }
}
