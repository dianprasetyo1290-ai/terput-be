<?php

namespace App\Livewire\Pages\Report\Generator;


use Livewire\Component;
use Livewire\WithPagination;
use App\Models\ReportRowDetail;

class ReportRowDetails extends Component
{
    use WithPagination;

    public $idNya;
    public $form = false;
    public $updateMode = false;
    public $name;
    public $group;
    public $level;
    public $normal_balance;
    public $operator;
    public $kode;
    public $report_row_head_id;
    public $identifier;
    public $formula;

    public $groupList = [
        'h' => 'Header',
        'd' => 'Detail',
        't' => 'Total',
        'gt' => 'Grand Total',
    ];
    public $levelList = [
        '0' => 'Indent 0',
        '1' => 'Indent 1',
        '2' => 'Indent 2',

    ];
    public $normal_balanceList = [
        'd' => 'Debit',
        'c' => 'Credit',
    ];
    public $operatorList = [
        '+' => 'Increment',
        '-' => 'Decrement',
    ];

    public $search = '';
    public $limit = 30;

    protected function rules() {
       return [
        'identifier' => 'required|unique:report_row_details,identifier,'.$this->idNya,
        'name' => 'required|string|max:255',
        'group' => 'required|string|max:255',
        'level' => 'required|integer',
        'normal_balance' => 'nullable|string|max:1',
        'operator' => 'nullable|string|max:1',
        'kode' => 'nullable|string|max:255',
        ];
    }

    public function mount($id = null) {
        // Opened without (or with an unknown) row design: back to the list.
        if (! ctype_digit((string) $id) || ! \App\Models\ReportRowHead::whereKey($id)->exists()) {
            abort(new \Illuminate\Http\RedirectResponse(route('generator.row')));
        }
        $this->report_row_head_id = $id;
    }

    public function render()
    {
        $data = ReportRowDetail::where('name', 'ilike', '%'.$this->search.'%')
                               ->where('report_row_head_id' , $this->report_row_head_id)
                               ->orderBy('sort_order', 'asc')
                               ->paginate($this->limit);

        return view('livewire.pages.report.generator.report-row-details', compact('data'));
    }

    public function create()
    {
        $this->resetInputFields();
        $this->form = true;
        $this->updateMode = false;
    }

    public function store()
    {

        if(!$this->idNya){
            $this->validate();

            // Get max sort_order untuk report_row_head_id ini
            $maxOrder = ReportRowDetail::where('report_row_head_id', $this->report_row_head_id)
                                        ->max('sort_order');

            ReportRowDetail::create(
                [
                    'identifier' => $this->identifier,
                    'formula' => $this->formula,
                    'name' => $this->name,
                    'group' => $this->group,
                    'level' => $this->level,
                    'normal_balance' => $this->normal_balance,
                    'operator' => $this->operator,
                    'kode' => $this->kode,
                    'report_row_head_id' => $this->report_row_head_id,
                    'sort_order' => ($maxOrder ?? 0) + 1,
                ]
            );
        }
        else {
            ReportRowDetail::where('id', $this->idNya)->update(
                [
                    'name' => $this->name,
                    'formula' => $this->formula,
                    'identifier' => $this->identifier,
                    'group' => $this->group,
                    'level' => $this->level,
                    'normal_balance' => $this->normal_balance,
                    'operator' => $this->operator,
                    'kode' => $this->kode,
                    // sort_order tidak diupdate saat edit, tetap di posisi semula
                ]
            );
        }
        session()->flash('message', $this->updateMode ? 'Updated successfully.' : 'Created successfully.');

        $this->form = false;
        $this->resetInputFields();
    }

    public function edit($id)
    {
        $record = ReportRowDetail::findOrFail($id);
        $this->idNya = $id;
        $this->name = $record->name;
        $this->group = $record->group;
        $this->level = $record->level;
        $this->normal_balance = $record->normal_balance;
        $this->operator = $record->operator;
        $this->kode = $record->kode;
        $this->identifier = $record->identifier;
        $this->formula = $record->formula;
        $this->form = true;
        $this->updateMode = true;
    }

    public function clear()
    {
        $this->resetInputFields();
        $this->form = false;
    }

    public function confirmDelete($id)
    {
        $this->idNya = $id;
        $this->js(<<<'JS'
        Swal.fire({
                text: "Do you want to delete this data? This process is irreversible.",
                icon: 'warning',
                showDenyButton: true,
                confirmButtonColor: '#3085d6',
                confirmButtonText: 'Delete!',
                denyButtonText: 'Cancel'
          }).then((result) => {
            if (result.isConfirmed) {
                $wire.delete()
            }
          })
        JS);
    }

    public function delete()
    {
        $id = $this->idNya;
        ReportRowDetail::find($id)->delete();
        session()->flash('message', 'Deleted successfully.');
    }

    private function resetInputFields()
    {
        $this->identifier = '';
        $this->formula = '';
        $this->name = '';
        $this->group = '';
        $this->level = '';
        $this->normal_balance = '';
        $this->operator = '';
        $this->kode = '';

    }

    public function limits($limit)
    {
        $this->limit = $limit;
    }

    public function moveUp($id)
    {
        $current = ReportRowDetail::find($id);
        if (!$current) return;

        // Cari row sebelumnya (sort_order lebih kecil)
        $previous = ReportRowDetail::where('report_row_head_id', $current->report_row_head_id)
                                    ->where('sort_order', '<', $current->sort_order)
                                    ->orderBy('sort_order', 'desc')
                                    ->first();

        if ($previous) {
            // Tukar sort_order
            $tempOrder = $current->sort_order;
            $current->sort_order = $previous->sort_order;
            $previous->sort_order = $tempOrder;

            $current->save();
            $previous->save();

            session()->flash('message', 'Order updated successfully.');
        }
    }

    public function moveDown($id)
    {
        $current = ReportRowDetail::find($id);
        if (!$current) return;

        // Cari row sesudahnya (sort_order lebih besar)
        $next = ReportRowDetail::where('report_row_head_id', $current->report_row_head_id)
                                ->where('sort_order', '>', $current->sort_order)
                                ->orderBy('sort_order', 'asc')
                                ->first();

        if ($next) {
            // Tukar sort_order
            $tempOrder = $current->sort_order;
            $current->sort_order = $next->sort_order;
            $next->sort_order = $tempOrder;

            $current->save();
            $next->save();

            session()->flash('message', 'Order updated successfully.');
        }
    }

}
