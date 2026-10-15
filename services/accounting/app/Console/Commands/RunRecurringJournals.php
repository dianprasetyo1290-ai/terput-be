<?php

namespace App\Console\Commands;

use App\Support\RecurringJournals;
use App\Support\Tenancy;
use Illuminate\Console\Command;
use Illuminate\Support\Facades\DB;
use Illuminate\Support\Facades\Schema;

/**
 * Daily job for scheduled recurring journals (QA BUG-144). Scheduled in
 * routes/console.php and run by `php artisan schedule:work` (supervisord in
 * the image). Walks every tenant schema; one school's failure never stops the
 * others. Idempotent, so running it twice a day is harmless.
 */
class RunRecurringJournals extends Command
{
    protected $signature = 'accounting:recurring-journals
        {--schema= : Only this tenant schema (tenant_<hex>)}
        {--date= : Treat this date (Y-m-d) as today}';

    protected $description = 'Create the draft journals of recurring schedules that are due, in every tenant schema';

    public function handle(): int
    {
        $schemas = $this->option('schema') ? [(string) $this->option('schema')] : Tenancy::schemas();
        $failed = false;
        foreach ($schemas as $schema) {
            try {
                Tenancy::activateSchema($schema);
                if (! Schema::hasTable('acc_recurring_journals') || ! DB::table('parameter')->exists()) {
                    $this->line("$schema: dilewati (belum dimigrasi / belum ada parameter)");

                    continue;
                }
                $r = RecurringJournals::runDue($this->option('date') ?: null);
                $this->line(sprintf('%s: %d draft dibuat, %d gagal, %d menunggu periode dibuka', $schema, $r['created'], $r['failed'], $r['waiting']));
            } catch (\Throwable $e) {
                $failed = true;
                report($e);
                $this->error("$schema: ".$e->getMessage());
            }
        }
        DB::statement('SET search_path TO public');

        return $failed ? self::FAILURE : self::SUCCESS;
    }
}
