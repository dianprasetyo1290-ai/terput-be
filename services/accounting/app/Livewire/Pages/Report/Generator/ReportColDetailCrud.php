<?php

namespace App\Livewire\Pages\Report\Generator;

use App\Models\ReportColDetail;
use App\Models\ReportColHead;
use Livewire\Component;
use Livewire\WithPagination;

class ReportColDetailCrud extends Component
{
    use WithPagination;

    public $colHeadId;
    public $colHead;
    public $idNya;
    public $form = false;
    public $updateMode = false;

    public $col_title, $content, $period, $division, $col_format, $period_key;

    public $search = '';
    public $limit = 30;

    public $contentList = [
        'Current' => 'Current (Actual Balance)',
        'Budget' => 'Budget',
        'Formula' => 'Formula (Column Calculation)',
        'Base%' => 'Base % (Percentage)',
    ];

    protected $rules = [
        'col_title' => 'required|string|max:100',
        'content' => 'required|string|in:Current,Budget,Formula,Base%',
        'period' => 'nullable|string|max:50',
        'division' => 'nullable|string|max:10',
        'col_format' => 'nullable|string|max:30',
        'period_key' => 'nullable|integer',
    ];

    public function mount($id = null)
    {
        if (! ctype_digit((string) $id) || ! ReportColHead::whereKey($id)->exists()) {
            abort(new \Illuminate\Http\RedirectResponse(route('generator.col')));
        }
        $this->colHeadId = $id;
        $this->colHead = ReportColHead::findOrFail($id);
    }

    public function render()
    {
        $data = ReportColDetail::where('report_col_head_id', $this->colHeadId)
            ->where('col_title', 'ilike', '%' . $this->search . '%')
            ->orderBy('sort_order', 'asc')
            ->paginate($this->limit);

        return view('livewire.pages.report.generator.report-col-detail-crud', compact('data'));
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
            $maxOrder = ReportColDetail::where('report_col_head_id', $this->colHeadId)
                ->max('sort_order');

            ReportColDetail::create([
                'report_col_head_id' => $this->colHeadId,
                'sort_order' => ($maxOrder ?? 0) + 1,
                'col_title' => $this->col_title,
                'content' => $this->content,
                'period' => $this->period,
                'division' => $this->division ?: '*',
                'col_format' => $this->col_format ?: '#,##0.00',
                'period_key' => $this->period_key,
            ]);
        } else {
            ReportColDetail::where('id', $this->idNya)->update([
                'col_title' => $this->col_title,
                'content' => $this->content,
                'period' => $this->period,
                'division' => $this->division ?: '*',
                'col_format' => $this->col_format ?: '#,##0.00',
                'period_key' => $this->period_key,
            ]);
        }

        session()->flash('message', $this->updateMode ? 'Updated successfully.' : 'Created successfully.');

        $this->form = false;
        $this->resetInputFields();
    }

    public function edit($id)
    {
        $record = ReportColDetail::findOrFail($id);
        $this->idNya = $id;
        $this->col_title = $record->col_title;
        $this->content = $record->content;
        $this->period = $record->period;
        $this->division = $record->division;
        $this->col_format = $record->col_format;
        $this->period_key = $record->period_key;
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
                text: "Do you want to delete this column? This process is irreversible.",
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
        ReportColDetail::find($id)->delete();
        session()->flash('message', 'Deleted successfully.');
    }

    private function resetInputFields()
    {
        $this->idNya = null;
        $this->col_title = '';
        $this->content = '';
        $this->period = '';
        $this->division = '*';
        $this->col_format = '#,##0.00';
        $this->period_key = null;
    }

    public function limits($limit)
    {
        $this->limit = $limit;
    }

    public function moveUp($id)
    {
        $current = ReportColDetail::find($id);
        if (!$current) return;

        $previous = ReportColDetail::where('report_col_head_id', $current->report_col_head_id)
            ->where('sort_order', '<', $current->sort_order)
            ->orderBy('sort_order', 'desc')
            ->first();

        if ($previous) {
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
        $current = ReportColDetail::find($id);
        if (!$current) return;

        $next = ReportColDetail::where('report_col_head_id', $current->report_col_head_id)
            ->where('sort_order', '>', $current->sort_order)
            ->orderBy('sort_order', 'asc')
            ->first();

        if ($next) {
            $tempOrder = $current->sort_order;
            $current->sort_order = $next->sort_order;
            $next->sort_order = $tempOrder;
            $current->save();
            $next->save();
            session()->flash('message', 'Order updated successfully.');
        }
    }
}
