<?php

namespace App\Services;

use App\Http\Controllers\SsoController;
use App\Support\Ledger;
use App\Support\Tenancy;
use Carbon\Carbon;
use Illuminate\Support\Facades\DB;
use Illuminate\Support\Facades\Log;
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
        // QA BUG-148: the billing/payment ids behind this journal, so the GL
        // drilldown can list the source transactions. Optional; bad shapes are
        // dropped rather than failing the journal.
        $this->sourceRefs = self::cleanSourceRefs($payload['source_refs'] ?? null);

        // Idempotency (BR-INT-01, AC-98-02) comes first: a retry of a journal
        // that was already booked must return the original number even when
        // its period has since been closed by End of Month (the row then
        // lives in this_head) or an account was deactivated afterwards.
        if ($reference !== '' && ($existing = $this->findExisting($source, $reference))) {
            $this->logPosting($source, $reference, $transDate, 0, 'posted', (string) $existing, null);

            return [
                'status' => 200,
                'trans_no' => (string) $existing,
                'message' => 'Jurnal sudah pernah dibuat sebelumnya',
            ];
        }

        if ($source === '' || strlen($source) > 20) {
            return $this->fail($source, $reference, $transDate, 0, [
                'status' => 422,
                'trans_no' => '',
                'message' => 'source wajib diisi (maksimal 20 karakter)',
            ]);
        }

        try {
            $transDate = Carbon::parse($transDate)->toDateString();
        } catch (\Throwable $e) {
            return $this->fail($source, $reference, '', 0, [
                'status' => 422,
                'trans_no' => '',
                'message' => 'trans_date tidak valid',
            ]);
        }

        // 1. Validation: minimum 2 lines
        if (count($lines) < 2) {
            return $this->fail($source, $reference, $transDate, 0, [
                'status' => 422,
                'trans_no' => '',
                'message' => 'Jurnal harus memiliki minimal 2 baris',
            ]);
        }

        // 2. Validation: balance in integer rupiah
        $totalDebit = 0;
        $totalCredit = 0;
        foreach ($lines as $line) {
            $d = (int) ($line['debit_idr'] ?? 0);
            $c = (int) ($line['credit_idr'] ?? 0);
            if ($d < 0 || $c < 0) {
                return $this->fail($source, $reference, $transDate, 0, [
                    'status' => 422,
                    'trans_no' => '',
                    'message' => 'Nominal debit/kredit tidak boleh negatif',
                ]);
            }
            $totalDebit += $d;
            $totalCredit += $c;
        }

        if ($totalDebit !== $totalCredit) {
            return $this->fail($source, $reference, $transDate, max($totalDebit, $totalCredit), [
                'status' => 422,
                'trans_no' => '',
                'message' => 'Total debit dan kredit tidak seimbang',
            ]);
        }

        if ($totalDebit <= 0) {
            return $this->fail($source, $reference, $transDate, 0, [
                'status' => 422,
                'trans_no' => '',
                'message' => 'Nominal jurnal harus lebih besar dari 0',
            ]);
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

        // QA BUG-143: header (induk) accounts only sum their children.
        $parentAccounts = [];
        foreach ($lines as $line) {
            $accCode = (string) ($line['acc_code'] ?? '');
            if ($accCode !== '' && ! in_array($accCode, $invalidAccounts, true) && \App\Support\Accounts::isParent($accCode)) {
                $parentAccounts[] = $accCode;
            }
        }
        if (! empty($parentAccounts)) {
            return $this->fail($source, $reference, $transDate, $totalDebit, [
                'status' => 422,
                'trans_no' => '',
                'message' => 'Akun induk tidak dapat diposting (posting hanya ke akun detail): '.implode(', ', array_unique($parentAccounts)),
                'invalid_accounts' => array_values(array_unique($parentAccounts)),
            ]);
        }

        // QA BUG-141: a unit's own account only takes that unit's lines.
        $scopeProblems = [];
        foreach ($lines as $line) {
            $accCode = (string) ($line['acc_code'] ?? '');
            $div = ! empty($line['div_code']) ? (string) $line['div_code'] : null;
            if ($accCode !== '' && ! in_array($accCode, $invalidAccounts, true) && $div !== null) {
                $scope = DB::table('mst_account')->where('acc_code', $accCode)->value('unit_scope');
                if ($scope !== null && $scope !== '' && $scope !== $div) {
                    $scopeProblems[] = "$accCode (unit $scope) untuk baris unit $div";
                }
            }
        }
        if (! empty($scopeProblems)) {
            return $this->fail($source, $reference, $transDate, $totalDebit, [
                'status' => 422,
                'trans_no' => '',
                'message' => 'Akun khusus unit dipakai unit lain: '.implode(', ', array_unique($scopeProblems)),
            ]);
        }

        if (! empty($invalidAccounts)) {
            return $this->fail($source, $reference, $transDate, $totalDebit, [
                'status' => 422,
                'trans_no' => '',
                'message' => 'Akun tidak ditemukan atau tidak aktif: ' . implode(', ', array_unique($invalidAccounts)),
                'invalid_accounts' => array_values(array_unique($invalidAccounts)),
            ]);
        }

        // 3b. Validation: vendor_id must exist in acc_vendors if supplied
        foreach ($lines as $line) {
            $vendorId = ! empty($line['vendor_id']) ? (string) $line['vendor_id'] : null;
            if ($vendorId !== null) {
                $vExists = DB::table('acc_vendors')->where('vendor_id', $vendorId)->exists();
                if (! $vExists) {
                    return $this->fail($source, $reference, $transDate, $totalDebit, [
                        'status' => 422,
                        'trans_no' => '',
                        'message' => "Vendor $vendorId tidak ditemukan di acc_vendors",
                    ]);
                }
            }
        }

        // 4. BR-ACC-07: no new journal in a closed (or not yet opened) period.
        try {
            Ledger::assertOpenDate($transDate);
        } catch (ValidationException $e) {
            return $this->fail($source, $reference, $transDate, $totalDebit, [
                'status' => 422,
                'trans_no' => '',
                'code' => 'period_closed',
                'message' => collect($e->errors())->flatten()->first() ?? 'Periode sudah ditutup',
            ]);
        }

        // 5. Execute within DB::transaction
        $result = DB::transaction(function () use ($source, $reference, $transDate, $description, $lines, $totalDebit, $totalCredit) {
            // Re-check inside the transaction: serialise concurrent posts of the
            // same reference on an advisory lock (the partial unique index on
            // trans_head is the last line of defence).
            if ($reference !== '') {
                DB::statement('SELECT pg_advisory_xact_lock(hashtext(?))', [$source.'|'.$reference]);
                if ($existing = $this->findExisting($source, $reference)) {
                    return [
                        'status' => 200,
                        'trans_no' => (string) $existing,
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

        $this->logPosting($source, $reference, $transDate, $totalDebit, 'posted', $result['trans_no'], null);

        return $result;
    }

    /** @var array<string, array<string>> source_refs of the journal being posted */
    private array $sourceRefs = [];

    /**
     * {"<type>": ["<id>", ...]} with type [a-z0-9_]{1,40}, ids up to 100
     * chars, at most 5000 ids in total; anything else is ignored.
     *
     * @return array<string, array<string>>
     */
    public static function cleanSourceRefs(mixed $refs): array
    {
        if (! is_array($refs)) {
            return [];
        }
        $out = [];
        $n = 0;
        foreach ($refs as $type => $ids) {
            if (! is_string($type) || ! preg_match('/^[a-z0-9_]{1,40}$/', $type) || ! is_array($ids)) {
                continue;
            }
            foreach ($ids as $id) {
                if ((is_string($id) || is_int($id)) && ($id = trim((string) $id)) !== '' && strlen($id) <= 100 && $n < 5000) {
                    $out[$type][] = $id;
                    $n++;
                }
            }
        }

        return array_map(fn ($ids) => array_values(array_unique($ids)), $out);
    }

    /** trans_no of a journal already booked for (source, reference), open or closed period. */
    private function findExisting(string $source, string $reference): ?string
    {
        foreach (['trans_head', 'this_head'] as $table) {
            $no = DB::table($table)->where('source', $source)->where('reference', $reference)->value('trans_no');
            if ($no) {
                return (string) $no;
            }
        }

        return null;
    }

    /** Record a rejected post in acc_auto_postings (FR-32-002 "posting gagal") and pass the result through. */
    private function fail(string $source, string $reference, string $transDate, int $amount, array $result): array
    {
        $this->logPosting($source, $reference, $transDate, $amount, 'failed', null, (string) ($result['message'] ?? ''));

        return $result;
    }

    /**
     * Auto-posting log, one row per (source, reference). A later successful
     * retry flips a failed row to posted, so the dashboard's failed count only
     * shows what still needs handling. Never lets a logging problem break the
     * API response.
     */
    private function logPosting(string $source, string $reference, string $transDate, int $amount, string $status, ?string $transNo, ?string $message): void
    {
        if ($reference === '') {
            return;
        }
        try {
            // Own savepoint: a failed log statement must not abort an
            // enclosing transaction.
            DB::transaction(fn () => $this->writeLog($source, $reference, $transDate, $amount, $status, $transNo, $message));
        } catch (\Throwable $e) {
            Log::warning('acc_auto_postings log gagal: '.$e->getMessage());
        }
    }

    private function writeLog(string $source, string $reference, string $transDate, int $amount, string $status, ?string $transNo, ?string $message): void
    {
        $now = Carbon::now();
        $values = [
            'posting_date' => $transDate !== '' ? $transDate : $now->toDateString(),
            'status' => $status,
            'trans_no' => $transNo,
            'message' => $message !== null ? mb_substr($message, 0, 2000) : null,
            'updated_at' => $now,
        ];
        if ($amount > 0) {
            $values['amount'] = $amount;
        }
        if ($this->sourceRefs !== []) {
            $values['source_ids'] = json_encode($this->sourceRefs);
        }
        $kind = mb_substr($source !== '' ? $source : 'unknown', 0, 20);
        $q = DB::table('acc_auto_postings')->where('kind', $kind)->where('reference', $reference);
        if ($q->exists()) {
            $q->update($values);
        } else {
            DB::table('acc_auto_postings')->insert($values + [
                'kind' => $kind,
                'reference' => mb_substr($reference, 0, 100),
                'amount' => max($amount, 0),
                'created_at' => $now,
            ]);
        }
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
        // Both tables: after End of Month trans_head is empty again, and
        // restarting at 000001 would collide with numbers already in history.
        $last = collect(['trans_head', 'this_head'])
            ->map(fn ($t) => DB::table($t)->where('trans_no', 'like', $code.'%')->max('trans_no'))
            ->filter()
            ->max();

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
            'user_name' => 'System Auto Journal',
            'email' => 'system@accounting.local',
            'password' => bcrypt('system-secret'),
            'created_at' => Carbon::now(),
            'updated_at' => Carbon::now(),
        ]);

        return $systemUserId;
    }
}
