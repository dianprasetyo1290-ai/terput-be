<?php

namespace App\Livewire\Pages\Report\Generator;

use App\Models\ReportColHead;
use Livewire\Component;
use Livewire\WithPagination;

class ReportColHeadCrud extends Component
{
    use WithPagination;

    public $name, $col_type, $colHeadId;
    public $updateMode = false;
    public $form = false;
    public $search = '';
    public $order = 'asc';
    public $limit = 10;

    public $colTypeList = [
        'fixed' => 'Fixed',
        'ytd' => 'Year To Date',
        'bs_quarterly' => 'BS Quarterly',
        'is_quarterly' => 'IS Quarterly',
    ];

    protected $rules = [
        'name' => 'required|string|max:255',
        'col_type' => 'required|string|in:fixed,ytd,bs_quarterly,is_quarterly',
    ];

    public function create()
    {
        $this->resetInputFields();
        $this->updateMode = false;
        $this->form = true;
    }

    private function resetInputFields()
    {
        $this->name = '';
        $this->col_type = '';
        $this->colHeadId = null;
    }

    public function store()
    {
        $this->validate();

        if ($this->updateMode) {
            ReportColHead::where('id', $this->colHeadId)->update([
                'name' => $this->name,
                'col_type' => $this->col_type,
            ]);
        } else {
            ReportColHead::create([
                'name' => $this->name,
                'col_type' => $this->col_type,
            ]);
        }

        session()->flash('message', $this->colHeadId ? 'Updated successfully.' : 'Created successfully.');

        $this->resetInputFields();
        $this->form = false;
    }

    public function edit($id)
    {
        $colHead = ReportColHead::findOrFail($id);
        $this->colHeadId = $id;
        $this->name = $colHead->name;
        $this->col_type = $colHead->col_type;

        $this->updateMode = true;
        $this->form = true;
    }

    public function delete()
    {
        $id = $this->colHeadId;
        if ($id) {
            ReportColHead::find($id)->delete();
            session()->flash('message', 'Deleted successfully.');
        }
    }

    public function confirmDelete($id)
    {
        $this->colHeadId = $id;
        $this->js(<<<'JS'
         Swal.fire({
                    title: 'Are you sure?',
                    text: 'All column details under this template will also be deleted.',
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
        $this->form = false;
        $this->resetInputFields();
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
        return view('livewire.pages.report.generator.report-col-head-crud', [
            'data' => ReportColHead::withCount('reportColDetails')
                ->where('name', 'ilike', '%' . $this->search . '%')
                ->orderBy('id', $this->order)
                ->paginate($this->limit),
        ]);
    }
}
