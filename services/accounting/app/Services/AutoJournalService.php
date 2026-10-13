<?php

namespace App\Services;

use App\Http\Controllers\SsoController;
use App\Support\Tenancy;
use Carbon\Carbon;
use Illuminate\Support\Facades\DB;
use Illuminate\Support\Facades\Validator;
use Illuminate\Validation\ValidationException;

class AutoJournalService
{
    /**
     * Post auto journal from external Sekoci services (payroll, payment gateway).
     *
     * @param array $payload
     * @return array{status: int, trans_no: string, message?: string}
     */
    public function postJournal(array $payload): array
    {
        $schoolId = (string) ($payload['school_id'] ?? '');
        if ($schoolId === '') {
            throw ValidationException::withMessages(['school_id' => 'school_id is required']);
        }

        // Activate tenant schema
        Tenancy::activate($schoolId);

        // Feature switch check
        if (! SsoController::accountingEnabled($schoolId)) {
            return [
                'status' => 409,
                'trans_no' => '',
                'message' => 'modul akuntansi tidak aktif',
            ];
        }

        $source = (string) ($payload['source'] ?? '');
        $reference = (string) ($payload['reference'] ?? '');
        $transDate = (string) ($payload['trans_date'] ?? Carbon::now()->toDateString());
        $description = (string) ($payload['description'] ?? '');
        $lines = (array) ($payload['lines'] ?? []);

        // 1. Validation: minimum 2 lines
        if (count($lines) < 2) {
            return [
                'status' => 422,
                'trans_no' => '',
                'message' => 'Jurnal harus memiliki minimal 2 baris',
            ];
        }

        // 2. Validation: balance in integer rupiah
        $totalDebit = 0;
        $totalCredit = 0;
        foreach ($lines as $line) {
            $totalDebit += (int) ($line['debit_idr'] ?? 0);
            $totalCredit += (int) ($line['credit_idr'] ?? 0);
        }

        if ($totalDebit !== $totalCredit) {
            return [
                'status' => 422,
                'trans_no' => '',
                'message' => 'Total debit dan kredit tidak seimbang',
            ];
        }

        if ($totalDebit <= 0) {
            return [
                'status' => 422,
                'trans_no' => '',
                'message' => 'Nominal jurnal harus lebih besar dari 0',
            ];
        }

        // 3. Validation: every acc_code exists in mst_account and is active
        $invalidAccounts = [];
        foreach ($lines as $line) {
            $accCode = (string) ($line['acc_code'] ?? '');
            if ($accCode === '') {
                $invalidAccounts[] = '(kosong)';
                continue;
            }
            $exists = DB::table('mst_account')
                ->where('acc_code', $accCode)
                ->where('is_active', true)
                ->exists();
            if (! $exists) {
                $invalidAccounts[] = $accCode;
            }
        }

        if (! empty($invalidAccounts)) {
            return [
                'status' => 422,
                'trans_no' => '',
                'message' => 'Akun tidak ditemukan atau tidak aktif: ' . implode(', ', array_unique($invalidAccounts)),
                'invalid_accounts' => array_values(array_unique($invalidAccounts)),
            ];
        }

        // 3b. Validation: vendor_id must exist in acc_vendors if supplied
        foreach ($lines as $line) {
            $vendorId = ! empty($line['vendor_id']) ? (string) $line['vendor_id'] : null;
            if ($vendorId !== null) {
                $vExists = DB::table('acc_vendors')->where('vendor_id', $vendorId)->exists();
                if (! $vExists) {
                    return [
                        'status' => 422,
                        'trans_no' => '',
                        'message' => "Vendor $vendorId tidak ditemukan di acc_vendors",
                    ];
                }
            }
        }

        // 4. Idempotency (BR-INT-01, AC-98-02): check existing reference
        if ($reference !== '') {
            $existing = DB::table('trans_head')
                ->where('source', $source)
                ->where('reference', $reference)
                ->first();

            if ($existing) {
                return [
                    'status' => 200,
                    'trans_no' => (string) $existing->trans_no,
                    'message' => 'Jurnal sudah pernah dibuat sebelumnya',
                ];
            }
        }

        // 5. Execute within DB::transaction
        return DB::transaction(function () use ($source, $reference, $transDate, $description, $lines, $totalDebit, $totalCredit) {
            // Re-check inside transaction lock for concurrency idempotency
            if ($reference !== '') {
                $existing = DB::table('trans_head')
                    ->where('source', $source)
                    ->where('reference', $reference)
                    ->lockForUpdate()
                    ->first();

                if ($existing) {
                    return [
                        'status' => 200,
                        'trans_no' => (string) $existing->trans_no,
                        'message' => 'Jurnal sudah pernah dibuat sebelumnya',
                    ];
                }
            }

            $prefix = match ($source) {
                'payroll' => 'PRL',
                'payment_gateway' => 'PGW',
                'payable' => 'APV',
                'asset' => 'AST',
                default => strtoupper(substr($source, 0, 3)),
            };

            $this->ensureTransactionType($prefix, match ($source) {
                'payroll' => 'Payroll',
                'payment_gateway' => 'Payment Gateway',
                'payable' => 'Hutang Usaha (AP)',
                'asset' => 'Aset Tetap (FA)',
                default => 'Auto Journal ' . ucfirst($source),
            });

            $year = Carbon::parse($transDate)->year;
            $transNo = $this->generateTransNo($prefix, $year);
            $userCode = $this->resolveUserCode();

            DB::table('trans_head')->insert([
                'trans_code' => $prefix,
                'trans_no' => $transNo,
                'trans_date' => $transDate,
                'trans_update' => Carbon::now()->toDateString(),
                'trans_user' => $userCode,
                'debet' => (float) $totalDebit,
                'credit' => (float) $totalCredit,
                'status' => 'posted',
                'source' => $source,
                'reference' => $reference !== '' ? $reference : null,
                'created_at' => Carbon::now(),
                'updated_at' => Carbon::now(),
            ]);

            foreach ($lines as $index => $line) {
                $divCode = ! empty($line['div_code']) ? (string) $line['div_code'] : null;
                $vendorId = ! empty($line['vendor_id']) ? (string) $line['vendor_id'] : null;
                DB::table('trans_detail')->insert([
                    'trans_code' => $prefix,
                    'trans_no' => $transNo,
                    'trans_count' => $index + 1,
                    'acc_code' => (string) $line['acc_code'],
                    'div_code' => $divCode,
                    'vendor_id' => $vendorId,
                    'description' => (string) ($line['description'] ?? $description),
                    'debet' => (float) ($line['debit_idr'] ?? 0),
                    'credit' => (float) ($line['credit_idr'] ?? 0),
                    'trans_date' => $transDate,
                    'created_at' => Carbon::now(),
                    'updated_at' => Carbon::now(),
                ]);
            }

            return [
                'status' => 201,
                'trans_no' => $transNo,
                'message' => 'Jurnal berhasil dicatat',
            ];
        });
    }

    private function ensureTransactionType(string $transCode, string $transName): void
    {
        DB::table('mst_transaction')->updateOrInsert(
            ['trans_code' => $transCode],
            [
                'trans_name' => $transName,
                'trans_status' => '1',
                'updated_at' => Carbon::now(),
                'created_at' => Carbon::now(),
            ]
        );
    }

    private function generateTransNo(string $prefix, int $year): string
    {
        $code = $prefix . substr((string) $year, -2) . '-';
        $last = DB::table('trans_head')
            ->where('trans_no', 'like', $code . '%')
            ->orderBy('trans_no', 'desc')
            ->value('trans_no');

        $next = $last ? ((int) substr($last, -6)) + 1 : 1;

        return $code . str_pad((string) $next, 6, '0', STR_PAD_LEFT);
    }

    private function resolveUserCode(): string
    {
        $existing = DB::table('mst_user')->value('user_id');
        if ($existing) {
            return (string) $existing;
        }

        $systemUserId = 'SYSTEM';
        DB::table('mst_user')->insertOrIgnore([
            'user_id' => $systemUserId,
            'name' => 'System Auto Journal',
            'email' => 'system@accounting.local',
            'password' => bcrypt('system-secret'),
            'created_at' => Carbon::now(),
            'updated_at' => Carbon::now(),
        ]);

        return $systemUserId;
    }
}
