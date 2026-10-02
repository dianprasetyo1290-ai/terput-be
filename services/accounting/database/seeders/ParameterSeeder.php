<?php

namespace Database\Seeders;

use App\Models\Parameter;
use Carbon\Carbon;
use Illuminate\Database\Seeder;
use Illuminate\Support\Facades\DB;
use Throwable;

class ParameterSeeder extends Seeder
{
    /**
     * Run the database seeds.
     */
    public function run(): void
    {
        $payload = $this->getLegacyPayload();

        if (! $payload) {
            $payload = $this->getFallbackPayload();
        }

        Parameter::query()->truncate();
        Parameter::create($payload);
    }

    private function getLegacyPayload(): ?array
    {
        try {
            $legacy = DB::connection('dblama')->table('parameter')->first();
        } catch (Throwable $e) {
            return null;
        }

        if (! $legacy) {
            return null;
        }

        $period = (int) preg_replace('/\\D/', '', (string) ($legacy->{'Period'} ?? 1));
        if ($period < 1) {
            $period = 1;
        }

        return [
            'company' => trim((string) ($legacy->Company ?? 'Sekoci Accounting')),
            'serial_no' => trim((string) ($legacy->{'Serial No'} ?? uniqid('SEKOCI-'))),
            'address' => trim((string) ($legacy->Address ?? null)),
            'contact' => trim((string) ($legacy->Contact ?? null)),
            'tax_number' => trim((string) ($legacy->{'Tax Number'} ?? null)),
            'period' => $period,
            'year' => $this->resolveYearFromLegacyParameterPeriod(),
            'mask_account' => trim((string) ($legacy->{'Mask Account'} ?? null)),
            'base_currency' => trim((string) ($legacy->{'Base Currency'} ?? 'Rp')),
            'control_date' => (bool) ($legacy->{'Control Date'} ?? false),
        ];
    }

    private function resolveYearFromLegacyParameterPeriod(): int
    {
        try {
            $periods = DB::connection('dblama')->table('mst_period')->get();

            $latestYear = 0;
            foreach ($periods as $item) {
                if (empty($item->Period_Name)) {
                    continue;
                }

                $year = (int) trim((string) $item->Period_Name);
                if ($year > $latestYear) {
                    $latestYear = $year;
                }
            }

            if ($latestYear > 0) {
                return $latestYear;
            }
        } catch (Throwable $e) {
            // fallback below
        }

        return (int) Carbon::now()->format('Y');
    }

    private function getFallbackPayload(): array
    {
        $year = (int) Carbon::now()->format('Y');
        $period = (int) date('n');

        return [
            'company' => 'Sekoci Accounting',
            'serial_no' => 'LOCAL001',
            'address' => 'Local Database',
            'contact' => '-',
            'tax_number' => null,
            'period' => $period,
            'year' => $year,
            'mask_account' => 'AAAAAAAAAAAAAAA',
            'base_currency' => 'Rp',
            'control_date' => false,
        ];
    }
}
