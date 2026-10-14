<?php

namespace App\Livewire\Pages\JournalDetail;

use Illuminate\Support\Facades\DB;
use Livewire\Component;

/**
 * One journal, open or closed (GL drilldown, PRD FR-35-002, QA BUG-148):
 * header, status trail, source/reference, lines, the reversal link and --
 * for automatic journals -- the auto-posting log entry with the source ids
 * SEKOCI sent.
 */
class JournalDetail extends Component
{
    public string $transNo;
    public ?string $date = null;

    public function mount(string $transNo): void
    {
        $this->transNo = $transNo;
        $d = request()->query('date');
        $this->date = is_string($d) && preg_match('/^\d{4}-\d{2}-\d{2}$/', $d) ? $d : null;
    }

    public function render()
    {
        $head = DB::table('trans_head')->where('trans_no', $this->transNo)->first();
        $closed = false;
        if (! $head) {
            $q = DB::table('this_head')->where('trans_no', $this->transNo);
            if ($this->date) {
                $q->whereDate('trans_date', $this->date);
            }
            $head = $q->orderByDesc('trans_date')->first();
            $closed = (bool) $head;
        }
        abort_unless($head, 404, "Jurnal {$this->transNo} tidak ditemukan.");

        $lines = DB::table($closed ? 'this_detail as d' : 'trans_detail as d')
            ->leftJoin('mst_account as a', 'a.acc_code', '=', 'd.acc_code')
            ->leftJoin('mst_division as v', 'v.div_code', '=', 'd.div_code')
            ->where('d.trans_no', $this->transNo)
            ->when($closed, fn ($q) => $q->whereDate('d.trans_date', substr((string) $head->trans_date, 0, 10)))
            ->orderBy('d.trans_count')
            ->get(['d.trans_count', 'd.acc_code', 'a.acc_name', 'd.div_code', 'v.div_name', 'd.description', 'd.debet', 'd.credit', 'd.curr_code']);

        $reversedBy = collect(['trans_head', 'this_head'])->flatMap(fn ($t) => DB::table($t)->where('reversal_of', $this->transNo)
            ->get(['trans_no', 'trans_date']))->values();

        $posting = ! empty($head->reference) && ($head->source ?? 'manual') !== 'manual'
            ? DB::table('acc_auto_postings')->where('reference', $head->reference)->first()
            : null;

        return view('livewire.pages.journal-detail.journal-detail', [
            'head' => $head,
            'closed' => $closed,
            'lines' => $lines,
            'reversedBy' => $reversedBy,
            'posting' => $posting,
            'sourceIds' => $posting ? (json_decode((string) $posting->source_ids, true) ?: []) : [],
            'type' => DB::table('mst_transaction')->where('trans_code', $head->trans_code)->value('trans_name'),
        ]);
    }
}
