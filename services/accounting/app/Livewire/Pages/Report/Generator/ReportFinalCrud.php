<?php

namespace App\Livewire\Pages\Report\Generator;

use App\Models\ReportColHead;
use App\Models\ReportFinal;
use Livewire\Component;
use Livewire\WithPagination;

class ReportFinalCrud extends Component
{
    use WithPagination;

    public $name, $print_title, $sub_title, $reportFinalId, $report_col_head_id;
    public $updateMode = false;
    public $form = false;
    public $search;
    public $order = "asc";
    public $limit = 10;
    public $colHeads = [];

    protected $rules = [
        'name' => 'required|string|max:255',
        'print_title' => 'required|string|max:255',
    ];

    public function mount()
    {
        $this->colHeads = ReportColHead::all();
    }



    public function create()
    {
        $this->resetInputFields();
        $this->updateMode = false;
        $this->form = true;
    }

    private function resetInputFields()
    {
        $this->name = '';
        $this->print_title = '';
        $this->sub_title = '';
        $this->report_col_head_id = null;
        $this->reportFinalId = null;
    }

    public function store()
    {
        $this->validate();

        if ($this->updateMode) {
            ReportFinal::where('id', $this->reportFinalId)->update(
                [
                    'name' => $this->name,
                    'print_title' => $this->print_title,
                    'sub_title' => $this->sub_title,
                    'report_col_head_id' => $this->report_col_head_id ?: null,
                ]
            );
        } else {
            ReportFinal::create(
                [
                    'name' => $this->name,
                    'print_title' => $this->print_title,
                    'sub_title' => $this->sub_title,
                    'report_col_head_id' => $this->report_col_head_id ?: null,
                ]
            );
        }


        session()->flash('message', $this->reportFinalId ? 'Updated successfully.' : 'Created successfully.');

        $this->resetInputFields();
        $this->form = false;
    }

    public function edit($id)
    {

        $reportFinal = ReportFinal::findOrFail($id);
        $this->reportFinalId = $id;
        $this->name = $reportFinal->name;
        $this->print_title = $reportFinal->print_title;
        $this->sub_title = $reportFinal->sub_title;
        $this->report_col_head_id = $reportFinal->report_col_head_id;

        $this->updateMode = true;
        $this->form = true;
    }

    public function delete($id = "")
    {
        $id = $this->reportFinalId;
        if ($id) {
            ReportFinal::find($id)->delete();
            session()->flash('message', 'Deleted successfully.');
        }
    }

    public function confirmDelete($id)
    {
        $this->reportFinalId = $id;
        $this->js(<<< 'JS'
         Swal.fire({
                    title: 'Are you sure?',
                    text: '',
                    icon: 'warning',
                    showCancelButton: true,
                    confirmButtonColor: '#3085d6',
                    cancelButtonColor: '#d33',
                    confirmButtonText: 'Yes, delete it!'
                }).then((result) => {
                    if (result.isConfirmed) {
                        $wire.delete()
                    }
                });
        JS);
    }
    public function clear()
    {
        $this->resetForm();
    }

    private function resetForm()
    {
        $this->form = false;
        $this->name = null;
        $this->print_title = null;
        $this->sub_title = null;
        $this->report_col_head_id = null;
        $this->reportFinalId = null;
    }


    public function sort($order)
    {
        $this->order = $order;
    }

    public function limits($limit)
    {
        $this->limit = $limit;
    }

    public function render()
    {
        return view('livewire.pages.report.generator.report-final-crud', [
            'data' => ReportFinal::with('reportColHead')
                ->where('name', 'ilike', '%' . $this->search . '%')
                ->orderBy('id', $this->order)
                ->paginate($this->limit),
        ]);
    }
}
