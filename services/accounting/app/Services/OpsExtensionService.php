<?php

namespace App\Services;

use App\Http\Controllers\SsoController;
use App\Support\Tenancy;
use Carbon\Carbon;
use Illuminate\Support\Facades\DB;

class OpsExtensionService
{
    protected AutoJournalService $autoJournalService;

    public function __construct(AutoJournalService $autoJournalService)
    {
        $this->autoJournalService = $autoJournalService;
    }

    /**
     * FR-106-005: Upsert master vendor di accounting engine
     */
    public function syncVendor(string $vendorId, array $payload): array
    {
        $schoolId = (string) ($payload['school_id'] ?? '');
        if ($schoolId !== '') {
            Tenancy::activate($schoolId);
        }

        DB::table('acc_vendors')->updateOrInsert(
            ['vendor_id' => $vendorId],
            [
                'code' => (string) ($payload['code'] ?? ''),
                'name' => (string) ($payload['name'] ?? ''),
                'npwp_masked' => $payload['npwp_masked'] ?? null,
                'status' => (string) ($payload['status'] ?? 'active'),
                'updated_at' => Carbon::now(),
            ]
        );

        return [
            'status' => 200,
            'message' => 'Vendor berhasil disinkronkan',
        ];
    }

    /**
     * IF-OPS-02: Ambil realisasi buku besar aktual untuk RAPBS
     */
    public function fetchActuals(array $payload): array
    {
        $schoolId = (string) ($payload['school_id'] ?? '');
        if ($schoolId === '') {
            return ['status' => 400, 'message' => 'school_id required'];
        }

        Tenancy::activate($schoolId);

        if (! SsoController::accountingEnabled($schoolId)) {
            return ['status' => 409, 'message' => 'modul akuntansi tidak aktif'];
        }

        $from = (string) ($payload['from'] ?? Carbon::now()->startOfYear()->toDateString());
        $to = (string) ($payload['to'] ?? Carbon::now()->toDateString());
        $items = (array) ($payload['items'] ?? []);

        $results = [];
        foreach ($items as $item) {
            $key = (string) ($item['key'] ?? '');
            $accCode = (string) ($item['acc_code'] ?? '');
            $divCode = ! empty($item['div_code']) ? (string) $item['div_code'] : null;

            // Cari normal balance akun
            $acc = DB::table('mst_account')->where('acc_code', $accCode)->first();
            $normalBalance = $acc ? (string) $acc->normal_balance : 'D';

            // Ambil akun dan turunannya
            $accountCodes = DB::table('mst_account')
                ->where('acc_code', $accCode)
                ->orWhere('parent_code', $accCode)
                ->pluck('acc_code')
                ->all();

            if (empty($accountCodes)) {
                $accountCodes = [$accCode];
            }

            // Agregasi dari trans_detail (posted journals)
            $queryTrans = DB::table('trans_detail as d')
                ->join('trans_head as h', function ($join) {
                    $join->on('h.trans_code', '=', 'd.trans_code')
                         ->on('h.trans_no', '=', 'd.trans_no');
                })
                ->where('h.status', 'posted')
                ->whereIn('d.acc_code', $accountCodes)
                ->whereBetween('h.trans_date', [$from, $to]);

            if ($divCode !== null) {
                $queryTrans->where('d.div_code', $divCode);
            }

            $sumTrans = $queryTrans->selectRaw('COALESCE(SUM(d.debet), 0) as total_debet, COALESCE(SUM(d.credit), 0) as total_credit')->first();

            // Agregasi dari this_detail (historical posted journals)
            $queryThis = DB::table('this_detail as d')
                ->join('this_head as h', function ($join) {
                    $join->on('h.trans_code', '=', 'd.trans_code')
                         ->on('h.trans_no', '=', 'd.trans_no');
                })
                ->where('h.status', 'posted')
                ->whereIn('d.acc_code', $accountCodes)
                ->whereBetween('h.trans_date', [$from, $to]);

            if ($divCode !== null) {
                $queryThis->where('d.div_code', $divCode);
            }

            $sumThis = $queryThis->selectRaw('COALESCE(SUM(d.debet), 0) as total_debet, COALESCE(SUM(d.credit), 0) as total_credit')->first();

            $totalDebet = (float) ($sumTrans->total_debet ?? 0) + (float) ($sumThis->total_debet ?? 0);
            $totalCredit = (float) ($sumTrans->total_credit ?? 0) + (float) ($sumThis->total_credit ?? 0);

            // Sesuai normal_balance: D = debet - credit, K = credit - debet
            $actual = $normalBalance === 'K' ? ($totalCredit - $totalDebet) : ($totalDebet - $totalCredit);

            $results[] = [
                'key' => $key,
                'actual_idr' => max(0, (int) round($actual)),
            ];
        }

        return [
            'status' => 200,
            'as_of' => Carbon::now()->toIso8601String(),
            'items' => $results,
        ];
    }

    /**
     * T16 §4.2 / T17: Pendaftaran aset tetap baru
     */
    public function registerAsset(array $payload): array
    {
        $schoolId = (string) ($payload['school_id'] ?? '');
        if ($schoolId === '') return ['status' => 400, 'message' => 'school_id required'];
        Tenancy::activate($schoolId);

        if (! SsoController::accountingEnabled($schoolId)) {
            return ['status' => 409, 'message' => 'modul akuntansi tidak aktif'];
        }

        $ref = (string) ($payload['asset_ref'] ?? '');
        $cost = (int) ($payload['cost_idr'] ?? 0);
        $residual = (int) ($payload['residual_idr'] ?? 0);
        $acquiredOn = (string) ($payload['acquired_on'] ?? Carbon::now()->toDateString());

        DB::table('fa_assets')->updateOrInsert(
            ['asset_ref' => $ref],
            [
                'code' => (string) ($payload['code'] ?? ''),
                'name' => (string) ($payload['name'] ?? ''),
                'acquired_on' => $acquiredOn,
                'cost' => $cost,
                'residual' => $residual,
                'opening_accum' => (int) ($payload['opening_accum_idr'] ?? 0),
                'useful_life_months' => (int) ($payload['useful_life_months'] ?? 48),
                'dep_start' => (string) ($payload['dep_start'] ?? 'acquisition_month'),
                'asset_acc_code' => (string) ($payload['asset_acc_code'] ?? ''),
                'accum_dep_acc_code' => (string) ($payload['accum_dep_acc_code'] ?? ''),
                'dep_expense_acc_code' => (string) ($payload['dep_expense_acc_code'] ?? ''),
                'status' => 'active',
                'updated_at' => Carbon::now(),
            ]
        );

        $postJournal = (bool) ($payload['post_journal'] ?? false);
        $transNo = null;

        if ($postJournal) {
            $offsetAcc = (string) ($payload['offset_acc_code'] ?? '');
            $assetAcc = (string) ($payload['asset_acc_code'] ?? '');
            $journalRes = $this->autoJournalService->postJournal([
                'school_id' => $schoolId,
                'source' => 'asset',
                'reference' => 'asset:acq:' . $ref,
                'trans_date' => $acquiredOn,
                'description' => 'Perolehan Aset Tetap: ' . ($payload['name'] ?? $ref),
                'lines' => [
                    ['acc_code' => $assetAcc, 'debit_idr' => $cost, 'credit_idr' => 0],
                    ['acc_code' => $offsetAcc, 'debit_idr' => 0, 'credit_idr' => $cost],
                ],
            ]);
            if ($journalRes['status'] === 201 || $journalRes['status'] === 200) {
                $transNo = $journalRes['trans_no'];
            }
        }

        return [
            'status' => 200,
            'asset_ref' => $ref,
            'trans_no' => $transNo,
        ];
    }

    /**
     * T16 §4.2: Pelepasan aset tetap
     */
    public function disposeAsset(string $ref, array $payload): array
    {
        $schoolId = (string) ($payload['school_id'] ?? '');
        Tenancy::activate($schoolId);

        if (! SsoController::accountingEnabled($schoolId)) {
            return ['status' => 409, 'message' => 'modul akuntansi tidak aktif'];
        }

        $asset = DB::table('fa_assets')->where('asset_ref', $ref)->first();
        if (! $asset) {
            return ['status' => 404, 'message' => 'Aset tidak ditemukan'];
        }

        $cost = (int) $asset->cost;
        // Total akumulasi yang sudah diposting
        $accumDep = (int) DB::table('fa_depreciation')->where('asset_ref', $ref)->sum('amount');
        $accum = $accumDep + (int) $asset->opening_accum;

        $proceeds = (int) ($payload['proceeds_idr'] ?? 0);
        $gainAcc = (string) ($payload['gain_acc_code'] ?? '');
        $lossAcc = (string) ($payload['loss_acc_code'] ?? '');
        $cashAcc = (string) ($payload['proceeds_acc_code'] ?? '');
        $disposedOn = (string) ($payload['disposed_on'] ?? Carbon::now()->toDateString());

        $lines = DepreciationCalculator::disposalLines(
            $cost,
            $accum,
            $proceeds,
            (string) $asset->asset_acc_code,
            (string) $asset->accum_dep_acc_code,
            $gainAcc,
            $lossAcc,
            $cashAcc
        );

        $jRes = $this->autoJournalService->postJournal([
            'school_id' => $schoolId,
            'source' => 'asset',
            'reference' => 'asset:disp:' . $ref,
            'trans_date' => $disposedOn,
            'description' => 'Pelepasan Aset Tetap: ' . $asset->name,
            'lines' => $lines,
        ]);

        DB::table('fa_assets')->where('asset_ref', $ref)->update([
            'status' => 'disposed',
            'closed_on' => $disposedOn,
            'updated_at' => Carbon::now(),
        ]);

        return [
            'status' => 200,
            'asset_ref' => $ref,
            'trans_no' => $jRes['trans_no'] ?? null,
        ];
    }

    /**
     * T16 §4.2: Penyusutan bulanan untuk semua aset aktif
     */
    public function depreciateAssets(array $payload): array
    {
        $schoolId = (string) ($payload['school_id'] ?? '');
        Tenancy::activate($schoolId);

        if (! SsoController::accountingEnabled($schoolId)) {
            return ['status' => 409, 'message' => 'modul akuntansi tidak aktif'];
        }

        $period = (string) ($payload['period'] ?? Carbon::now()->format('Y-m'));

        // Idempotensi: cek apakah penyusutan periode ini sudah pernah dibuat
        $existingDep = DB::table('fa_depreciation')->where('period', $period)->first();
        if ($existingDep && ! empty($existingDep->trans_no)) {
            return [
                'status' => 200,
                'period' => $period,
                'trans_no' => $existingDep->trans_no,
                'message' => 'Penyusutan periode ini sudah diposting sebelumnya',
            ];
        }

        $assets = DB::table('fa_assets')->where('status', 'active')->get();
        if ($assets->isEmpty()) {
            return ['status' => 200, 'period' => $period, 'trans_no' => null, 'message' => 'Tidak ada aset aktif'];
        }

        $accountPairs = []; // key: expAcc:accumAcc -> amount
        $assetAmounts = []; // asset_ref -> amount

        foreach ($assets as $ast) {
            $firstP = DepreciationCalculator::firstPeriod((string) $ast->acquired_on, (string) $ast->dep_start);
            if ($period < $firstP) {
                continue; // Belum mulai disusutkan
            }

            // Hitung monthIndex
            $dtStart = Carbon::parse($firstP . '-01');
            $dtCur = Carbon::parse($period . '-01');
            $monthIndex = $dtStart->diffInMonths($dtCur);

            $lifeMonths = (int) $ast->useful_life_months;
            if ($monthIndex >= $lifeMonths) {
                continue; // Sudah habis masa manfaat
            }

            $cost = (int) $ast->cost;
            $residual = (int) $ast->residual;
            $amt = DepreciationCalculator::monthly($cost, $residual, $lifeMonths, $monthIndex);

            if ($amt > 0) {
                $pairKey = $ast->dep_expense_acc_code . '|' . $ast->accum_dep_acc_code;
                $accountPairs[$pairKey] = ($accountPairs[$pairKey] ?? 0) + $amt;
                $assetAmounts[$ast->asset_ref] = $amt;
            }
        }

        if (empty($accountPairs)) {
            return ['status' => 200, 'period' => $period, 'trans_no' => null, 'message' => 'Total beban penyusutan 0'];
        }

        $journalLines = [];
        foreach ($accountPairs as $pairKey => $amt) {
            [$expAcc, $accumAcc] = explode('|', $pairKey);
            $journalLines[] = ['acc_code' => $expAcc, 'debit_idr' => $amt, 'credit_idr' => 0];
            $journalLines[] = ['acc_code' => $accumAcc, 'debit_idr' => 0, 'credit_idr' => $amt];
        }

        $jRes = $this->autoJournalService->postJournal([
            'school_id' => $schoolId,
            'source' => 'asset',
            'reference' => 'asset:dep:' . $period,
            'trans_date' => $period . '-28',
            'description' => 'Penyusutan Aset Tetap Periode ' . $period,
            'lines' => $journalLines,
        ]);

        $transNo = $jRes['trans_no'] ?? null;

        foreach ($assetAmounts as $ref => $amt) {
            DB::table('fa_depreciation')->updateOrInsert(
                ['asset_ref' => $ref, 'period' => $period],
                ['amount' => $amt, 'trans_no' => $transNo]
            );
        }

        return [
            'status' => 200,
            'period' => $period,
            'trans_no' => $transNo,
        ];
    }
}
