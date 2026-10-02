<?php

namespace App\Livewire\Pages\PriorYearAdjustment;

use App\Models\ChartOfAccount;
use App\Models\Division;
use App\Models\Parameter;
use App\Models\PriorYearAdjustmentLine;
use App\Models\PriorYearAdjustmentSession;
use App\Models\TransactionType;
use App\Services\PriorYearAdjustmentService;
use Carbon\Carbon;
use Illuminate\Support\Facades\DB;
use Illuminate\Validation\ValidationException;
use Livewire\Component;
use Throwable;

class PriorYearAdjustment extends Component
{
    public int $activeYear;

    public int $targetYear;

    public string $adjustmentDate;

    public ?string $description = null;

    public ?int $sessionId = null;

    public ?string $message = null;

    public string $mode = 'list';

    public string $search = '';

    public string $listFilterYear = '';

    public string $transCode = 'PYA';

    public bool $showFinalizeModal = false;

    public bool $showDeleteLineModal = false;

    public ?int $deleteLineId = null;

    public bool $showDeleteDraftModal = false;

    public ?int $deleteDraftId = null;

    public array $line = [
        'acc_code' => '',
        'div_code' => '',
        'description' => '',
        'debet' => '',
        'credit' => '',
        'curr_code' => 'Rp',
    ];

    public function mount(): void
    {
        TransactionType::firstOrCreate(
            ['trans_code' => 'PYA'],
            ['trans_name' => 'Prior Year Adjustment', 'trans_status' => '1']
        );

        $parameter = Parameter::first();
        $this->activeYear = (int) ($parameter?->year ?: now()->year);
        $this->targetYear = $this->availableYears()[0] ?? ($this->activeYear - 1);
        $this->adjustmentDate = Carbon::create($this->targetYear, 12, 31)->toDateString();
    }

    public function updatedTargetYear($value): void
    {
        $this->targetYear = (int) $value;
        $this->adjustmentDate = Carbon::create($this->targetYear, 12, 31)->toDateString();
    }

    public function openCreateForm(): void
    {
        $this->mode = 'create';
        $this->sessionId = null;
        $this->description = null;
        $this->transCode = 'PYA';
        $this->adjustmentDate = Carbon::create($this->targetYear, 12, 31)->toDateString();
    }

    public function backToList(): void
    {
        $this->mode = 'list';
        $this->sessionId = null;
        $this->showFinalizeModal = false;
        $this->showDeleteLineModal = false;
        $this->deleteLineId = null;
        $this->showDeleteDraftModal = false;
        $this->deleteDraftId = null;
    }

    public function createDraft(): void
    {
        $session = app(PriorYearAdjustmentService::class)->createSession([
            'target_year' => $this->targetYear,
            'adjustment_date' => $this->adjustmentDate,
            'trans_code' => $this->transCode,
            'description' => $this->description,
        ]);

        $this->sessionId = $session->id;
        $this->mode = 'detail';
        $this->message = 'Draft adjustment dibuat.';
    }

    public function selectSession(int $sessionId): void
    {
        $this->sessionId = $sessionId;
        $session = $this->selectedSession();

        if ($session) {
            $this->targetYear = (int) $session->target_year;
            $this->adjustmentDate = $session->adjustment_date->toDateString();
            $this->description = $session->description;
            $this->transCode = $session->trans_code;
            $this->mode = 'detail';
        }
    }

    public function addLine(): void
    {
        $session = $this->selectedSession();

        if (! $session) {
            $this->addError('sessionId', 'Buat atau pilih draft adjustment dulu.');

            return;
        }

        $this->line['curr_code'] = 'Rp';
        app(PriorYearAdjustmentService::class)->addLine($session, $this->line);

        $this->line = [
            'acc_code' => '',
            'div_code' => '',
            'description' => '',
            'debet' => '',
            'credit' => '',
            'curr_code' => 'Rp',
        ];
        $this->message = 'Baris adjustment ditambahkan.';
        $this->dispatch('prior-year-adjustment-line-reset');
    }

    public function confirmDeleteLine(int $lineId): void
    {
        $session = $this->selectedSession();

        if (! $session || $session->status !== 'draft') {
            return;
        }

        $lineExists = PriorYearAdjustmentLine::where('session_id', $session->id)
            ->where('id', $lineId)
            ->exists();

        if (! $lineExists) {
            return;
        }

        $this->deleteLineId = $lineId;
        $this->showDeleteLineModal = true;
    }

    public function cancelDeleteLine(): void
    {
        $this->deleteLineId = null;
        $this->showDeleteLineModal = false;
    }

    public function deleteLine(): void
    {
        $session = $this->selectedSession();

        if (! $session || $session->status !== 'draft' || ! $this->deleteLineId) {
            return;
        }

        PriorYearAdjustmentLine::where('session_id', $session->id)
            ->where('id', $this->deleteLineId)
            ->delete();

        $this->deleteLineId = null;
        $this->showDeleteLineModal = false;
        $this->message = 'Baris adjustment dihapus.';
    }

    public function confirmDeleteDraft(int $sessionId): void
    {
        $session = PriorYearAdjustmentSession::find($sessionId);

        if (! $session || $session->status !== 'draft') {
            return;
        }

        $this->deleteDraftId = $session->id;
        $this->showDeleteDraftModal = true;
    }

    public function cancelDeleteDraft(): void
    {
        $this->deleteDraftId = null;
        $this->showDeleteDraftModal = false;
    }

    public function deleteDraft(): void
    {
        if (! $this->deleteDraftId) {
            return;
        }

        $session = PriorYearAdjustmentSession::find($this->deleteDraftId);

        if (! $session || $session->status !== 'draft') {
            $this->cancelDeleteDraft();

            return;
        }

        $session->delete();

        if ($this->sessionId === $session->id) {
            $this->sessionId = null;
            $this->mode = 'list';
        }

        $this->deleteDraftId = null;
        $this->showDeleteDraftModal = false;
        $this->message = 'Draft adjustment dihapus.';
    }

    public function finalizeAdjustment(): void
    {
        $session = $this->selectedSession();

        if (! $session) {
            $this->addError('sessionId', 'Pilih draft adjustment yang akan difinalisasi.');

            return;
        }

        try {
            $finalized = app(PriorYearAdjustmentService::class)->finalize($session);
            $this->sessionId = $finalized->id;
            $this->showFinalizeModal = false;
            $this->message = "Adjustment selesai. Nomor transaksi {$finalized->trans_no} diposting ke periode 13 {$finalized->target_year}.";
        } catch (ValidationException $exception) {
            $this->showFinalizeModal = false;
            $this->applyValidationErrors($exception);
        } catch (Throwable $exception) {
            report($exception);
            $this->showFinalizeModal = false;
            $this->addError('finalize', 'Adjustment gagal difinalisasi. '.$exception->getMessage());
        }
    }

    public function confirmFinalizeAdjustment(): void
    {
        $session = $this->selectedSession();

        if (! $session) {
            $this->addError('sessionId', 'Pilih draft adjustment yang akan difinalisasi.');

            return;
        }

        if ($session->status !== 'draft') {
            $this->addError('session', 'Adjustment ini sudah difinalisasi.');

            return;
        }

        if (! $this->validateFinalizableDraft($session)) {
            return;
        }

        $this->showFinalizeModal = true;
    }

    public function cancelFinalizeAdjustment(): void
    {
        $this->showFinalizeModal = false;
    }

    public function formatAmount(mixed $value): string
    {
        $amount = $this->parseAmount($value);

        return number_format($amount, 2, ',', '.');
    }

    public function amountInWords(mixed $value): string
    {
        $amount = $this->parseAmount($value);
        if ($amount <= 0) {
            return 'Nol rupiah';
        }

        $whole = (int) floor($amount);
        $cents = (int) round(($amount - $whole) * 100);
        $words = trim($this->spellNumber($whole)).' rupiah';

        if ($cents > 0) {
            $words .= ' '.trim($this->spellNumber($cents)).' sen';
        }

        return ucfirst($words);
    }

    public function render()
    {
        $session = $this->selectedSession();
        $sessions = $this->filteredSessions();

        return view('livewire.pages.prior-year-adjustment.prior-year-adjustment', [
            'sessions' => $sessions,
            'listTotalDebet' => (float) $sessions->sum('lines_sum_debet'),
            'listTotalCredit' => (float) $sessions->sum('lines_sum_credit'),
            'selectedSession' => $session,
            'accounts' => ChartOfAccount::orderBy('acc_name')->get(),
            'divisions' => Division::orderBy('div_code')->get(),
            'transactionTypes' => TransactionType::orderBy('trans_name')->get(),
            'years' => $this->availableYears(),
            'listYears' => $this->sessionYears(),
        ]);
    }

    private function filteredSessions()
    {
        return PriorYearAdjustmentSession::query()
            ->withCount('lines')
            ->withSum('lines', 'debet')
            ->withSum('lines', 'credit')
            ->when($this->listFilterYear !== '', function ($query) {
                $query->where('target_year', (int) $this->listFilterYear);
            })
            ->when(trim($this->search) !== '', function ($query) {
                $search = '%'.trim($this->search).'%';

                $query->where(function ($query) use ($search) {
                    $query->where('description', 'ilike', $search)
                        ->orWhere('trans_no', 'ilike', $search)
                        ->orWhere('trans_code', 'ilike', $search)
                        ->orWhere('status', 'ilike', $search);
                });
            })
            ->orderByDesc('created_at')
            ->limit(20)
            ->get();
    }

    private function selectedSession(): ?PriorYearAdjustmentSession
    {
        if (! $this->sessionId) {
            return null;
        }

        return PriorYearAdjustmentSession::with('lines')
            ->find($this->sessionId);
    }

    private function validateFinalizableDraft(PriorYearAdjustmentSession $session): bool
    {
        if ($session->lines->isEmpty()) {
            $this->addError('lines', 'Minimal satu baris adjustment harus diisi.');

            return false;
        }

        $debet = (float) $session->lines->sum('debet');
        $credit = (float) $session->lines->sum('credit');

        if (abs($debet - $credit) >= 0.01) {
            $this->addError('balance', 'Total debit dan kredit harus balance.');

            return false;
        }

        return true;
    }

    private function applyValidationErrors(ValidationException $exception): void
    {
        foreach ($exception->errors() as $field => $messages) {
            foreach ($messages as $message) {
                $this->addError($field, $message);
            }
        }
    }

    private function availableYears(): array
    {
        $years = DB::table('mst_saldo_news')
            ->where('tahun', '<', $this->activeYear ?? now()->year)
            ->select('tahun')
            ->distinct()
            ->orderByDesc('tahun')
            ->pluck('tahun')
            ->map(fn ($year) => (int) $year)
            ->values()
            ->all();

        if ($years) {
            return $years;
        }

        return range($this->activeYear - 1, max($this->activeYear - 3, 1900));
    }

    private function sessionYears(): array
    {
        return PriorYearAdjustmentSession::query()
            ->select('target_year')
            ->distinct()
            ->orderByDesc('target_year')
            ->pluck('target_year')
            ->map(fn ($year) => (int) $year)
            ->values()
            ->all();
    }

    private function parseAmount(mixed $value): float
    {
        if (is_numeric($value)) {
            return (float) $value;
        }

        $value = str_replace([' ', '.'], ['', ''], (string) $value);
        $value = str_replace(',', '.', $value);

        return is_numeric($value) ? (float) $value : 0.0;
    }

    private function spellNumber(int $number): string
    {
        $words = ['', 'satu', 'dua', 'tiga', 'empat', 'lima', 'enam', 'tujuh', 'delapan', 'sembilan', 'sepuluh', 'sebelas'];

        if ($number < 12) {
            return $words[$number] ?? '';
        }
        if ($number < 20) {
            return $this->spellNumber($number - 10).' belas';
        }
        if ($number < 100) {
            return $this->spellNumber(intdiv($number, 10)).' puluh '.$this->spellNumber($number % 10);
        }
        if ($number < 200) {
            return 'seratus '.$this->spellNumber($number - 100);
        }
        if ($number < 1000) {
            return $this->spellNumber(intdiv($number, 100)).' ratus '.$this->spellNumber($number % 100);
        }
        if ($number < 2000) {
            return 'seribu '.$this->spellNumber($number - 1000);
        }
        if ($number < 1000000) {
            return $this->spellNumber(intdiv($number, 1000)).' ribu '.$this->spellNumber($number % 1000);
        }
        if ($number < 1000000000) {
            return $this->spellNumber(intdiv($number, 1000000)).' juta '.$this->spellNumber($number % 1000000);
        }

        return $this->spellNumber(intdiv($number, 1000000000)).' miliar '.$this->spellNumber($number % 1000000000);
    }
}
