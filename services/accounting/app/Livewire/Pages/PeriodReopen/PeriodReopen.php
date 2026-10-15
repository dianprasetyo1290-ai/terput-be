<?php

namespace App\Livewire\Pages\PeriodReopen;

use App\Support\Ledger;
use App\Support\PeriodReopen as Reopen;
use Illuminate\Support\Facades\Auth;
use Illuminate\Support\Facades\DB;
use Illuminate\Validation\ValidationException;
use Livewire\Component;

/**
 * Buka kembali periode (QA BUG-178): request (createperiod_reopen) ->
 * approve/reject by another user (editperiod_reopen, FinanceManager /
 * SuperAdmin). See App\Support\PeriodReopen for the rules.
 */
class PeriodReopen extends Component
{
    public $reason = '';
    public $note = '';
    public $message = null;
    public $error = null;

    private function actor(): string
    {
        return (string) Auth::id();
    }

    private function run(callable $fn, string $ok): void
    {
        $this->resetValidation();
        try {
            $fn();
        } catch (ValidationException $e) {
            $this->message = null;
            $this->error = implode(' ', collect($e->errors())->flatten()->all());

            return;
        }
        $this->error = null;
        $this->message = $ok;
        $this->reason = '';
        $this->note = '';
    }

    public function requestReopen(): void
    {
        $this->authorize('createperiod_reopen');
        $this->run(fn () => Reopen::request((string) $this->reason, $this->actor()),
            'Permintaan buka kembali periode dikirim. Menunggu persetujuan Finance Manager.');
    }

    public function cancelRequest(int $id): void
    {
        $this->authorize('createperiod_reopen');
        $this->run(fn () => Reopen::cancel($id, $this->actor()), 'Permintaan dibatalkan.');
    }

    public function approve(int $id): void
    {
        $this->authorize('editperiod_reopen');
        $moved = 0;
        $this->run(function () use ($id, &$moved) {
            $moved = Reopen::approve($id, $this->actor(), (string) $this->note ?: null);
        }, 'Periode dibuka kembali.');
        if (! $this->error) {
            $this->message = "Periode dibuka kembali; $moved jurnal dikembalikan ke periode aktif.";
        }
    }

    public function reject(int $id): void
    {
        $this->authorize('editperiod_reopen');
        $this->run(fn () => Reopen::reject($id, $this->actor(), (string) $this->note), 'Permintaan ditolak.');
    }

    public function render()
    {
        $a = Ledger::active();
        $t = Reopen::target();

        return view('livewire.pages.period-reopen.period-reopen', [
            'active' => $a,
            'activeLabel' => Ledger::periodLabel($a['year'], $a['period']),
            'target' => $t,
            'targetLabel' => $t ? Ledger::periodLabel($t['year'], $t['period']) : null,
            'blockers' => Reopen::blockers(),
            'pending' => DB::table('acc_period_reopen_requests')->where('status', 'pending')->first(),
            'history' => DB::table('acc_period_reopen_requests')->orderByDesc('id')->limit(20)->get(),
            'me' => $this->actor(),
        ]);
    }
}
