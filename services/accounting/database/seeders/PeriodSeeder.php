<?php

namespace Database\Seeders;

use App\Models\Period;
use Carbon\Carbon;
use Illuminate\Database\Seeder;
use Illuminate\Support\Collection;
use Illuminate\Support\Facades\DB;
use Throwable;

class PeriodSeeder extends Seeder
{
    /**
     * Run the database seeds.
     */
    public function run(): void
    {
        $periodRows = $this->getLegacyPeriods();

        Period::query()->truncate();

        foreach ($periodRows as $data) {
            $payload = (array) $data;

            $periodName = $data->{'Period_Name'} ?? $data->period_name ?? $payload['period_name'] ?? null;

            Period::insert(array_merge(
                $payload,
                [
                    'period_name' => $this->normalizePeriodName($periodName),
                ]
            ));
        }
    }

    private function getLegacyPeriods(): Collection
    {
        try {
            $rows = DB::connection('dblama')->table('mst_period')->get();
            if ($rows->count() > 0) {
                return $rows;
            }
        } catch (Throwable $e) {
            // fallback below
        }

        return collect($this->buildFallbackPeriods());
    }

    private function buildFallbackPeriods(): array
    {
        $currentYear = (int) Carbon::now()->format('Y');
        $rows = [];

        // Match app convention: code 1=current year, 2=last year, 3=2 years ago
        for ($code = 1; $code <= 3; $code++) {
            $year = $currentYear - ($code - 1);
            $rows[] = (object) $this->buildPeriodPayload($code, $year);
        }

        return $rows;
    }

    private function buildPeriodPayload(int $periodCode, int $year): array
    {
        $payload = [
            'period_code' => (string) $periodCode,
            'period_name' => (string) $year,
        ];

        for ($month = 1; $month <= 13; $month++) {
            $key = str_pad((string) $month, 2, '0', STR_PAD_LEFT);
            if ($month === 13) {
                $payload["p$key"] = 'Adj  ' . $year;
                $from = Carbon::create($year, 12, 31);
                $to = Carbon::create($year, 12, 31);
            } else {
                $payload["p$key"] = Carbon::create($year, $month, 1)->format('M  Y');
                $from = Carbon::create($year, $month, 1);
                $to = Carbon::create($year, $month, 1)->endOfMonth();
            }

            $payload["p{$key}_from"] = $from->format('Y-m-d');
            $payload["p{$key}_to"] = $to->format('Y-m-d');
        }

        return $payload;
    }

    private function normalizePeriodName(?string $value): string
    {
        return trim((string) $value);
    }
}
