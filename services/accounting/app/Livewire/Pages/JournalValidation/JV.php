<?php

namespace App\Livewire\Pages\JournalValidation;

use App\Livewire\Traits\WithJournalWorkflow;
use Illuminate\Support\Facades\Auth;
use Illuminate\Support\Facades\DB;
use Livewire\Component;

/**
 * Journal approval queue (PRD FR-34-005, QA BUG-062): every journal waiting
 * for approval with its lines and totals. Approve/reject is for holders of
 * editjournal_validation who did not make or submit the journal.
 */
class JV extends Component
{
    use WithJournalWorkflow;

    public $open = null;

    public function toggle(string $transNo): void
    {
        $this->open = $this->open === $transNo ? null : $transNo;
    }

    public function render()
    {
        $pending = DB::table('trans_head as h')
            ->leftJoin('trans_detail as d', 'd.trans_no', '=', 'h.trans_no')
            ->where('h.status', 'pending')
            ->groupBy('h.trans_no', 'h.trans_code', 'h.trans_date', 'h.trans_user', 'h.status_by', 'h.status_at', 'h.reference')
            ->orderBy('h.status_at')
            ->get(['h.trans_no', 'h.trans_code', 'h.trans_date', 'h.trans_user', 'h.status_by', 'h.status_at', 'h.reference',
                DB::raw('coalesce(sum(d.debet),0) as debit'), DB::raw('coalesce(sum(d.credit),0) as credit'), DB::raw('count(d.id) as lines')]);

        $lines = $this->open
            ? DB::table('trans_detail as d')->leftJoin('mst_account as a', 'a.acc_code', '=', 'd.acc_code')
                ->where('d.trans_no', $this->open)->orderBy('d.trans_count')
                ->get(['d.trans_count', 'd.acc_code', 'a.acc_name', 'd.div_code', 'd.description', 'd.debet', 'd.credit'])
            : collect();

        return view('livewire.pages.journal-validation.j-v', [
            'pending' => $pending,
            'lines' => $lines,
            'me' => (string) Auth::id(),
        ]);
    }
}
