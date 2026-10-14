<?php

namespace App\Livewire\Traits;

use App\Support\Journals;
use Illuminate\Support\Facades\Auth;
use Illuminate\Validation\ValidationException;

/**
 * Journal workflow actions for Livewire pages (PRD FR-34-005, QA BUG-062):
 * draft -> (submit) pending -> (approve) posted | (reject) draft, and
 * posted -> reversal journal. Rules live in App\Support\Journals; this only
 * authorises and reports.
 */
trait WithJournalWorkflow
{
    /** Last workflow error, shown above the list. */
    public $workflowError = null;

    public function submitJournal(string $transNo): void
    {
        $this->authorize('editjournal_transaction');
        $this->runWorkflow(fn () => Journals::transition($transNo, 'submit', $this->actorId()),
            "Jurnal $transNo diajukan untuk approval");
    }

    public function approveJournal(string $transNo): void
    {
        $this->authorize('editjournal_validation');
        $this->runWorkflow(fn () => Journals::transition($transNo, 'approve', $this->actorId()),
            "Jurnal $transNo disetujui dan diposting");
    }

    public function rejectJournal(string $transNo): void
    {
        $this->authorize('editjournal_validation');
        $this->runWorkflow(fn () => Journals::transition($transNo, 'reject', $this->actorId()),
            "Jurnal $transNo dikembalikan ke draft");
    }

    public function reverseJournal(string $transNo, ?string $date = null): void
    {
        $this->authorize('editjournal_validation');
        $this->runWorkflow(function () use ($transNo, $date) {
            $no = Journals::reverse($transNo, $date, $this->actorId());
            $this->lastReversal = $no;
        }, "Jurnal pembalik untuk $transNo dibuat");
    }

    /** Number of the reversal journal booked by the last reverseJournal(). */
    public $lastReversal = null;

    private function actorId(): string
    {
        return (string) Auth::id();
    }

    private function runWorkflow(callable $fn, string $ok): void
    {
        $this->workflowError = null;
        try {
            $fn();
        } catch (ValidationException $e) {
            $this->workflowError = implode(' ', collect($e->errors())->flatten()->all());

            return;
        }
        $title = json_encode($ok);
        $this->js("Swal.mixin({toast: true, position: 'top-end', showConfirmButton: false, timer: 2500, timerProgressBar: true}).fire({icon: 'success', title: $title});");
    }
}
