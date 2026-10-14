<?php

namespace App\Livewire\Pages\AutoPosting;

use Illuminate\Support\Facades\DB;
use Livewire\Component;
use Livewire\WithPagination;

/**
 * Log posting otomatis billing/pembayaran (PRD FR-32-002, QA BUG-140): what
 * SEKOCI sent, which journal it became, and why a post failed. A failed post
 * is fixed at the source (mapping / period / account) and re-sent by SEKOCI;
 * a later success flips the row to posted.
 */
class AutoPosting extends Component
{
    use WithPagination;

    protected $paginationTheme = 'bootstrap';

    public $status = 'failed';
    public $search = '';

    public function updatedStatus(): void
    {
        $this->resetPage();
    }

    public function updatedSearch(): void
    {
        $this->resetPage();
    }

    public function render()
    {
        $q = DB::table('acc_auto_postings')
            ->when(in_array($this->status, ['failed', 'posted'], true), fn ($q) => $q->where('status', $this->status))
            ->when($this->search !== '', fn ($q) => $q->where(fn ($w) => $w->where('reference', 'ilike', '%'.$this->search.'%')
                ->orWhere('trans_no', 'ilike', '%'.$this->search.'%')->orWhere('message', 'ilike', '%'.$this->search.'%')))
            ->orderByDesc('updated_at');

        return view('livewire.pages.auto-posting.auto-posting', [
            'rows' => $q->paginate(25),
            'counts' => DB::table('acc_auto_postings')->selectRaw('status, count(*) as n')->groupBy('status')->pluck('n', 'status'),
        ]);
    }
}
