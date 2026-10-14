<?php

namespace App\Livewire\Pages\Report\FinancialReport;

use App\Models\ReportFinal;
use Livewire\Component;
use Livewire\WithPagination;

class FinancialReport extends Component
{
    use WithPagination;

    public $name, $reportFinalId;
    public $updateMode = false;
    public $form = false; // Pastikan variabel ini ada
    public $search;
    public $order = "asc";
    public $limit = 10;

    protected $rules = [
        'name' => 'required|string|max:255',
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
        $this->reportFinalId = null;
    }

    public function store()
    {
        $this->validate();

        if ($this->updateMode) {
            ReportFinal::where('id', $this->reportFinalId)->update(
                ['name' => $this->name]
            );
        } else {
            ReportFinal::create(
                ['name' => $this->name]
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
        $this->name = null; // Reset to default
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
        return view('livewire.pages.report.financial-report.financial-report', [
            'data' => ReportFinal::where('name', 'ilike', '%'.$this->search.'%')
            ->orderBy('id', strtoupper((string) $this->order) === 'DESC' ? 'desc' : 'asc')
            ->paginate(in_array((int) $this->limit, [10, 30, 50, 100], true) ? (int) $this->limit : 10),
        ]);
    }
}
