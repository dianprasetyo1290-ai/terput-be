<?php

namespace App\Services;

use App\Models\ChartOfAccount;
use App\Models\MstSaldoNew;
use App\Models\Parameter;
use App\Models\PriorYearAdjustmentLine;
use App\Models\PriorYearAdjustmentSession;
use App\Models\TransactionType;
use Carbon\Carbon;
use Illuminate\Support\Facades\Auth;
use Illuminate\Support\Facades\DB;
use Illuminate\Support\Facades\Validator;
use Illuminate\Validation\ValidationException;

class PriorYearAdjustmentService
{
    public function createSession(array $data): PriorYearAdjustmentSession
    {
        $validated = Validator::make($data, [
            'target_year' => ['required', 'integer', 'min:1900'],
            'adjustment_date' => ['required', 'date'],
            'trans_code' => ['nullable', 'string', 'max:5', 'exists:mst_transaction,trans_code'],
            'description' => ['nullable', 'string', 'max:250'],
        ])->validate();

        $this->assertTargetYearIsClosed((int) $validated['target_year']);

        return PriorYearAdjustmentSession::create([
            'target_year' => (int) $validated['target_year'],
            'adjustment_date' => Carbon::parse($validated['adjustment_date'])->toDateString(),
            'description' => $validated['description'] ?? null,
            'status' => 'draft',
            'trans_code' => $validated['trans_code'] ?? 'PYA',
        ]);
    }

    public function addLine(PriorYearAdjustmentSession $session, array $data): PriorYearAdjustmentLine
    {
        $this->assertDraft($session);
        $data['debet'] = $this->normalizeAmount($data['debet'] ?? 0);
        $data['credit'] = $this->normalizeAmount($data['credit'] ?? 0);
        $data['curr_rate'] = $this->normalizeAmount($data['curr_rate'] ?? 0);
        $data['curr_debet'] = $this->normalizeAmount($data['curr_debet'] ?? 0);
        $data['curr_credit'] = $this->normalizeAmount($data['curr_credit'] ?? 0);

        $validated = Validator::make($data, [
            'acc_code' => ['required', 'string', 'max:15', 'exists:mst_account,acc_code'],
            'div_code' => ['required', 'string', 'max:5', 'exists:mst_division,div_code'],
            'description' => ['nullable', 'string', 'max:250'],
            'debet' => ['nullable', 'numeric', 'min:0'],
            'credit' => ['nullable', 'numeric', 'min:0'],
            'curr_code' => ['nullable', 'string', 'max:5', 'exists:mst_currency,curr_code'],
            'curr_rate' => ['nullable', 'numeric', 'min:0'],
            'curr_debet' => ['nullable', 'numeric', 'min:0'],
            'curr_credit' => ['nullable', 'numeric', 'min:0'],
        ])->validate();

        $debet = (float) ($validated['debet'] ?? 0);
        $credit = (float) ($validated['credit'] ?? 0);

        if ($debet <= 0 && $credit <= 0) {
            throw ValidationException::withMessages([
                'line' => 'Debit atau kredit harus diisi.',
            ]);
        }

        if ($debet > 0 && $credit > 0) {
            throw ValidationException::withMessages([
                'line' => 'Satu baris hanya boleh debit atau kredit.',
            ]);
        }

        $line = $session->lines()->create([
            'acc_code' => $validated['acc_code'],
            'div_code' => $validated['div_code'],
            'description' => $validated['description'] ?? null,
            'debet' => $debet,
            'credit' => $credit,
            'curr_code' => $validated['curr_code'] ?? null,
            'curr_rate' => (float) ($validated['curr_rate'] ?? 0),
            'curr_debet' => (float) ($validated['curr_debet'] ?? 0),
            'curr_credit' => (float) ($validated['curr_credit'] ?? 0),
        ]);

        $transactionType = TransactionType::with('akunLawan')->find($session->trans_code);
        if ($transactionType?->akunLawan) {
            $session->lines()->create([
                'acc_code' => $transactionType->trans_account,
                'div_code' => $validated['div_code'],
                'description' => $validated['description'] ?? null,
                'debet' => $credit,
                'credit' => $debet,
                'curr_code' => $validated['curr_code'] ?? null,
                'curr_rate' => (float) ($validated['curr_rate'] ?? 0),
                'curr_debet' => (float) ($validated['curr_credit'] ?? 0),
                'curr_credit' => (float) ($validated['curr_debet'] ?? 0),
            ]);
        }

        return $line;
    }

    public function finalize(PriorYearAdjustmentSession $session): PriorYearAdjustmentSession
    {
        return DB::transaction(function () use ($session) {
            $session = PriorYearAdjustmentSession::with('lines')
                ->lockForUpdate()
                ->findOrFail($session->id);

            $this->assertDraft($session);
            $this->assertTargetYearIsClosed((int) $session->target_year);
            $this->assertBalanced($session);
            $this->ensureTransactionType($session->trans_code);

            $transNo = $this->generateTransNo((int) $session->target_year);
            $period = '13';
            $date = Carbon::parse($session->adjustment_date)->toDateString();

            DB::table('this_head')->insert([
                'period' => $period,
                'trans_code' => $session->trans_code,
                'trans_no' => $transNo,
                'trans_date' => $date,
                'trans_update' => now()->toDateString(),
                'trans_user' => $this->resolveUserCode(),
                'debet' => $session->lines->sum('debet'),
                'credit' => $session->lines->sum('credit'),
                'created_at' => now(),
                'updated_at' => now(),
            ]);

            foreach ($session->lines as $index => $line) {
                DB::table('this_detail')->insert([
                    'period' => $period,
                    'trans_code' => $session->trans_code,
                    'trans_no' => $transNo,
                    'trans_count' => $index + 1,
                    'acc_code' => $line->acc_code,
                    'div_code' => $line->div_code,
                    'description' => $line->description,
                    'debet' => $line->debet,
                    'credit' => $line->credit,
                    'curr_code' => $line->curr_code,
                    'curr_rate' => $line->curr_rate,
                    'curr_debet' => $line->curr_debet,
                    'curr_credit' => $line->curr_credit,
                    'trans_date' => $date,
                    'curr_system' => null,
                    'curr_yes' => false,
                    'created_at' => now(),
                    'updated_at' => now(),
                ]);
            }

            $session->update([
                'status' => 'finalized',
                'trans_no' => $transNo,
                'finalized_at' => now(),
                'finalized_by' => $this->resolveUserCode(),
            ]);

            $activeYear = (int) Parameter::first()->year;
            $this->rebuildPeriodBalance((int) $session->target_year, 13);
            $this->cascadeOpeningBalances((int) $session->target_year, $activeYear);

            return $session->fresh('lines');
        });
    }

    public function rebuildPeriodBalance(int $year, int $period): void
    {
        $periodPadded = str_pad((string) $period, 2, '0', STR_PAD_LEFT);

        MstSaldoNew::where('tahun', $year)
            ->where('periode', $period)
            ->update(['debet' => 0, 'credit' => 0]);

        $aggregates = DB::table('this_detail')
            ->select('acc_code', 'div_code')
            ->selectRaw('SUM(COALESCE(debet, 0)) AS total_debet')
            ->selectRaw('SUM(COALESCE(credit, 0)) AS total_credit')
            ->where('period', $periodPadded)
            ->whereRaw('EXTRACT(YEAR FROM trans_date) = ?', [$year])
            ->groupBy('acc_code', 'div_code')
            ->get();

        foreach ($aggregates as $row) {
            MstSaldoNew::updateOrCreate(
                [
                    'tahun' => $year,
                    'periode' => $period,
                    'acc_code' => $row->acc_code,
                    'div_code' => $row->div_code,
                ],
                [
                    'debet' => (float) $row->total_debet,
                    'credit' => (float) $row->total_credit,
                ]
            );
        }
    }

    public function cascadeOpeningBalances(int $adjustedYear, int $activeYear): void
    {
        for ($closingYear = $adjustedYear; $closingYear < $activeYear; $closingYear++) {
            $this->rebuildOpeningBalance($closingYear, $closingYear + 1);
        }
    }

    public function rebuildOpeningBalance(int $closingYear, int $nextYear): void
    {
        $reAccCode = $this->createRetainedEarningsAccount($closingYear);
        $this->ensureSaldoTemplate($nextYear);

        MstSaldoNew::where('tahun', $nextYear)
            ->where('periode', 0)
            ->update(['debet' => 0, 'credit' => 0]);

        $realBalances = DB::table('mst_saldo_news')
            ->join('mst_account', 'mst_saldo_news.acc_code', '=', 'mst_account.acc_code')
            ->where('mst_saldo_news.tahun', $closingYear)
            ->whereIn('mst_account.acc_group', [1, 2, 3])
            ->groupBy('mst_saldo_news.acc_code', 'mst_saldo_news.div_code')
            ->select('mst_saldo_news.acc_code', 'mst_saldo_news.div_code')
            ->selectRaw('SUM(mst_saldo_news.debet) AS total_debet')
            ->selectRaw('SUM(mst_saldo_news.credit) AS total_credit')
            ->get();

        foreach ($realBalances as $row) {
            MstSaldoNew::updateOrCreate(
                [
                    'tahun' => $nextYear,
                    'periode' => 0,
                    'acc_code' => $row->acc_code,
                    'div_code' => $row->div_code,
                ],
                [
                    'debet' => (float) $row->total_debet,
                    'credit' => (float) $row->total_credit,
                ]
            );
        }

        $netIncome = (float) DB::table('mst_saldo_news')
            ->join('mst_account', 'mst_saldo_news.acc_code', '=', 'mst_account.acc_code')
            ->where('mst_saldo_news.tahun', $closingYear)
            ->whereIn('mst_account.acc_group', [4, 5, 6, 7, 8])
            ->selectRaw('COALESCE(SUM(mst_saldo_news.credit) - SUM(mst_saldo_news.debet), 0) AS net')
            ->value('net');

        MstSaldoNew::updateOrCreate(
            [
                'tahun' => $nextYear,
                'periode' => 0,
                'acc_code' => $reAccCode,
                'div_code' => '0000',
            ],
            [
                'debet' => $netIncome < 0 ? abs($netIncome) : 0,
                'credit' => $netIncome >= 0 ? $netIncome : 0,
            ]
        );
    }

    private function ensureSaldoTemplate(int $year): void
    {
        DB::statement(<<<'SQL'
            INSERT INTO mst_saldo_news (acc_code, div_code, periode, tahun, debet, credit, created_at, updated_at)
            SELECT a.acc_code, d.div_code, p.periode, ?, 0, 0, NOW(), NOW()
            FROM mst_account a
            CROSS JOIN mst_division d
            CROSS JOIN generate_series(0, 13) AS p(periode)
            ON CONFLICT (acc_code, div_code, periode, tahun) DO NOTHING
        SQL, [$year]);
    }

    private function createRetainedEarningsAccount(int $year): string
    {
        $existing = ChartOfAccount::where('acc_name', 'ilike', "%Retained Earning%{$year}%")->first();

        if ($existing) {
            return $existing->acc_code;
        }

        $lastRE = ChartOfAccount::where('acc_code', 'like', '38888888%')
            ->where('acc_code', '!=', '388888880000000')
            ->orderBy('acc_code', 'desc')
            ->first();

        $lastIndex = $lastRE ? (int) substr($lastRE->acc_code, -3) : 0;
        $newAccCode = '388888880000'.str_pad((string) ($lastIndex + 1), 3, '0', STR_PAD_LEFT);

        ChartOfAccount::create([
            'acc_code' => $newAccCode,
            'acc_name' => "Retained Earning of {$year}",
            'acc_group' => 3,
            'acc_currency' => 'Rp',
        ]);

        return $newAccCode;
    }

    private function generateTransNo(int $targetYear): string
    {
        $prefix = 'PYA'.substr((string) $targetYear, -2).'-';
        $last = PriorYearAdjustmentSession::where('target_year', $targetYear)
            ->whereNotNull('trans_no')
            ->where('trans_no', 'like', $prefix.'%')
            ->orderBy('trans_no', 'desc')
            ->value('trans_no');

        $next = $last ? ((int) substr($last, -6)) + 1 : 1;

        return $prefix.str_pad((string) $next, 6, '0', STR_PAD_LEFT);
    }

    private function ensureTransactionType(string $transCode): void
    {
        if ($transCode === 'PYA') {
            DB::table('mst_transaction')->updateOrInsert(
                ['trans_code' => $transCode],
                [
                    'trans_name' => 'Prior Year Adjustment',
                    'trans_status' => '1',
                    'updated_at' => now(),
                    'created_at' => now(),
                ]
            );
        }
    }

    private function normalizeAmount(mixed $value): float
    {
        if (is_numeric($value)) {
            return (float) $value;
        }

        $value = trim((string) $value);
        if ($value === '') {
            return 0.0;
        }

        $value = str_replace([' ', '.'], ['', ''], $value);
        $value = str_replace(',', '.', $value);

        return is_numeric($value) ? (float) $value : 0.0;
    }

    private function assertDraft(PriorYearAdjustmentSession $session): void
    {
        if ($session->status !== 'draft') {
            throw ValidationException::withMessages([
                'session' => 'Adjustment ini sudah difinalisasi.',
            ]);
        }
    }

    private function assertBalanced(PriorYearAdjustmentSession $session): void
    {
        if ($session->lines->isEmpty()) {
            throw ValidationException::withMessages([
                'lines' => 'Minimal satu baris adjustment harus diisi.',
            ]);
        }

        $debet = (float) $session->lines->sum('debet');
        $credit = (float) $session->lines->sum('credit');

        if (abs($debet - $credit) >= 0.01) {
            throw ValidationException::withMessages([
                'balance' => 'Total debit dan kredit harus balance.',
            ]);
        }
    }

    private function assertTargetYearIsClosed(int $targetYear): void
    {
        $activeYear = (int) optional(Parameter::first())->year ?: now()->year;

        if ($targetYear >= $activeYear) {
            throw ValidationException::withMessages([
                'target_year' => 'Tahun adjustment harus lebih kecil dari tahun aktif.',
            ]);
        }
    }

    private function resolveUserCode(): ?string
    {
        $user = Auth::user();

        if (! $user) {
            return null;
        }

        return substr((string) ($user->user_id ?? $user->id), 0, 20);
    }
}
