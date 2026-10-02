<?php

namespace App\Livewire\Pages\Report\Generator;

use App\Models\ReportFinal;
use App\Models\ReportFinalData;
use App\Models\ReportRowDetail;
use App\Models\ReportRowHead;
use Livewire\Component;
use Livewire\WithPagination;

class ReportFinalDetail extends Component
{
    use WithPagination;

    public $idNya;
    public $form = false;
    public $updateMode = false;
    public $finalId;
    public $final;
    public $rowhead;
    public $rowHeadId;

    public $search = '';
    public $order = 'ASC';
    public $limit = 30;

    protected $rules = [
        'rowHeadId' => 'required',
    ];


    public function mount($id = '')
    {
        $this->finalId = $id;
        $this->final = ReportFinal::find($id);
        $this->rowhead = ReportRowHead::get();
    }

    public function render()
    {
        $data = ReportFinalData::where('report_final_id', $this->finalId)
            ->where(function ($query) {
                $query->orWhereHas('reportFinal', function ($a) {
                    $a->where('name', 'ilike', '%' . $this->search . '%');
                })->orWhereHas('reportRowHeadOne', function ($b) {
                    $b->where('name', 'ilike', '%' . $this->search . '%');
                });
            })
            ->paginate($this->limit);

        return view('livewire.pages.report.generator.report-final-detail', compact('data'));
    }

    public function create()
    {
        $this->resetInputFields();
        $this->form = true;
        $this->updateMode = false;
    }

    public function store()
    {
        $this->validate();

        if (!$this->idNya) {
            $alreadyLinked = ReportFinalData::where([
                'report_final_id' => $this->finalId,
                'report_row_head_id' => $this->rowHeadId,
            ])->exists();

            if ($alreadyLinked) {
                session()->flash('message', 'This report row is already linked to this workspace.');
                $this->form = false;
                $this->resetInputFields();

                return;
            }

            ReportFinalData::create(
                [
                    'report_final_id' => $this->finalId,
                    'report_row_head_id' => $this->rowHeadId,
                ]
            );
        } else {
            ReportFinalData::where('id', $this->idNya)->update(
                [
                    'report_final_id' => $this->finalId,
                    'report_row_head_id' => $this->rowHeadId,
                ]
            );
        }
        session()->flash('message', $this->updateMode ? 'Updated successfully.' : 'Created successfully.');

        $this->form = false;
        $this->resetInputFields();
    }

    public function edit($id)
    {
        $record = ReportFinalData::findOrFail($id);
        $this->idNya = $id;
        $this->rowHeadId = $record->report_row_head_id;
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
        ReportFinalData::find($id)->delete();
        session()->flash('message', 'Deleted successfully.');
    }

    private function resetInputFields()
    {
        $this->rowHeadId = '';
    }

    public function sort($order)
    {
        $this->order = $order;
    }

    public function limits($limit)
    {
        $this->limit = $limit;
    }
}
