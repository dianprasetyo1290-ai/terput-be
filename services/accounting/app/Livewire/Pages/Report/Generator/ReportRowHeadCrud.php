<?php

namespace App\Livewire\Pages\Report\Generator;

use Livewire\Component;
use Livewire\WithPagination;
use App\Models\ReportRowHead;
use SebastianBergmann\Type\TrueType;

class ReportRowHeadCrud extends Component
{
    use WithPagination;

    public $name, $reportRowHeadId;
    public $updateMode = false;
    public $form = false; // Pastikan variabel ini ada
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
        $this->reportRowHeadId = null;
    }

    public function store()
    {
        $this->validate();

        if($this->updateMode){
            ReportRowHead::where('id', $this->reportRowHeadId)->update(
                ['name' => $this->name]
            );
        } else {
            ReportRowHead::create(
                ['name' => $this->name]
            );
        }


        session()->flash('message', $this->reportRowHeadId ? 'Updated successfully.' : 'Created successfully.');

        $this->resetInputFields();
        $this->form = false;
    }



    public function edit($id)
    {

        $reportRowHead = ReportRowHead::findOrFail($id);
        $this->reportRowHeadId = $id;
        $this->name = $reportRowHead->name;

        $this->updateMode = true;
        $this->form = true;
    }

    public function delete($id = "")
    {
        $id = $this->reportRowHeadId;
        if ($id) {
            ReportRowHead::find($id)->delete();
            session()->flash('message', 'Deleted successfully.');
        }
    }

    public function confirmDelete($id)
    {
        $this->reportRowHeadId = $id;
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
        $this->reportRowHeadId = null;
    }

    public function sort($order)
    {
        $this->order = strtoupper((string) $order) === 'DESC' ? 'desc' : 'asc';
        $this->resetPage();
    }

    public function limits($limit)
    {
        $this->limit = in_array((int) $limit, [10, 30, 50, 100], true) ? (int) $limit : 10;
        $this->resetPage();
    }

    public function render()
    {
        return view('livewire.pages.report.generator.report-row-head-crud',[
            'data' => ReportRowHead::orderBy('id', $this->order)->paginate($this->limit),
        ]);
    }
}
