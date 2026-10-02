<?php

namespace Tests\Feature;

use Illuminate\Support\Facades\Artisan;
use Illuminate\Support\Facades\DB;
use Tests\TestCase;

class ImportLegacyIasYearTest extends TestCase
{
    public function test_it_imports_legacy_periods_from_ias_file(): void
    {
        $file = base_path('storage/exco_mdb_special/EXCO Database/IAS.2025');

        if (!is_readable($file) || trim((string) shell_exec('command -v mdb-export')) === '') {
            $this->markTestSkipped('Legacy IAS sample or mdb-export is not available.');
        }

        DB::beginTransaction();

        try {
            DB::table('mst_period')->truncate();

            $exitCode = Artisan::call('legacy:import-ias-year', [
                'file' => $file,
                '--year' => 2025,
                '--skip-budget' => true,
                '--fresh-year' => true,
            ]);

            $this->assertSame(0, $exitCode);

            $periods = DB::table('mst_period')
                ->orderBy('period_code')
                ->get(['period_code', 'period_name', 'p01', 'p01_from', 'p13', 'p13_to']);

            $this->assertCount(3, $periods);
            $this->assertSame('1', $periods[0]->period_code);
            $this->assertSame('2025', trim($periods[0]->period_name));
            $this->assertSame('Jan  2025', $periods[0]->p01);
            $this->assertSame('2025-01-01', $periods[0]->p01_from);
            $this->assertSame('Adj  2025', $periods[0]->p13);
            $this->assertSame('2025-12-31', $periods[0]->p13_to);
        } finally {
            DB::rollBack();
        }
    }
}
