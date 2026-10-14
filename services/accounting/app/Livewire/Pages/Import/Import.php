<?php

namespace App\Livewire\Pages\Import;

use App\Support\Importer;
use App\Support\Ledger;
use App\Support\ReceivableReconciliation;
use Illuminate\Support\Facades\Auth;
use Illuminate\Validation\ValidationException;
use Illuminate\Support\Facades\DB;
use Livewire\Component;
use Livewire\WithFileUploads;

/** Import COA / saldo awal dari CSV (PRD FR-33-002, QA BUG-059/141). */
class Import extends Component
{
    use WithFileUploads;

    public $kind = 'coa';
    public $file;
    public $result = null;
    public $confirmReplace = false;

    /** Control-account vs billing check (QA BUG-059); null = not run yet. */
    public $recon = null;
    public $signOffNote = '';
    public $signOffMessage = null;

    protected function rules(): array
    {
        return [
            'kind' => 'required|in:coa,opening',
            'file' => 'required|file|max:2048|mimes:csv,txt',
        ];
    }

    public function updatedKind(): void
    {
        $this->reset('result', 'confirmReplace');
    }

    public function preview(): void
    {
        $this->run(true);
    }

    public function import(): void
    {
        $this->run(false);
    }

    private function run(bool $dryRun): void
    {
        $this->authorize('createimport_data');
        $this->validate();
        $required = $this->kind === 'coa' ? ['acc_code', 'acc_name', 'acc_group'] : Importer::OPENING_COLUMNS;
        [$rows, $errors] = Importer::parse((string) file_get_contents($this->file->getRealPath()), $required);
        if ($errors) {
            $this->result = ['errors' => $errors, 'dry' => $dryRun];

            return;
        }

        if ($this->kind === 'coa') {
            $this->result = Importer::coa($rows, $dryRun) + ['dry' => $dryRun];
        } else {
            $year = Ledger::active()['year'];
            if (! $dryRun && $this->existingOpening($year) && ! $this->confirmReplace) {
                $this->result = ['errors' => ["Saldo awal tahun $year sudah ada. Centang konfirmasi penggantian untuk menimpanya."], 'dry' => false];

                return;
            }
            $this->result = Importer::opening($rows, $year, $dryRun) + ['dry' => $dryRun, 'year' => $year];
        }
    }

    public function reconcile(): void
    {
        $this->authorize('viewimport_data');
        $this->signOffMessage = null;
        $this->recon = ReceivableReconciliation::check();
    }

    public function signOff(): void
    {
        $this->authorize('createimport_data');
        $this->resetValidation();
        $this->recon = ReceivableReconciliation::check(); // never trust the numbers held in the browser
        try {
            ReceivableReconciliation::signOff($this->recon, $this->signOffNote, (string) Auth::id());
        } catch (ValidationException $e) {
            foreach ($e->errors() as $k => $msgs) {
                foreach ($msgs as $m) {
                    $this->addError($k, $m);
                }
            }

            return;
        }
        $this->signOffNote = '';
        $this->signOffMessage = 'Sign-off migrasi piutang tercatat di audit trail.';
    }

    private function existingOpening(int $year): bool
    {
        return DB::table('mst_saldo_news')->where('tahun', $year)->where('periode', 0)
            ->where(fn ($q) => $q->where('debet', '!=', 0)->orWhere('credit', '!=', 0))->exists();
    }

    public function render()
    {
        $year = Ledger::active()['year'];

        return view('livewire.pages.import.import', [
            'year' => $year,
            'hasOpening' => $this->existingOpening($year),
            'lastSignOff' => ReceivableReconciliation::lastSignOff(),
        ]);
    }
}
