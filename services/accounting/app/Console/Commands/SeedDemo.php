<?php

namespace App\Console\Commands;

use App\Support\Tenancy;
use Database\Seeders\DemoFeaturesSeeder;
use Illuminate\Console\Command;
use Illuminate\Support\Facades\DB;
use Illuminate\Support\Facades\Schema;

/**
 * Additive, idempotent demo data for the accounting features (journal
 * templates, recurring journals, sample journals in every status, COA
 * templates, period-reopen requests, control/inter-unit accounts) in every
 * provisioned tenant schema. Never truncates or deletes -- safe to re-run;
 * unlike `db:seed` it does not touch the baseline seeders.
 */
class SeedDemo extends Command
{
    protected $signature = 'accounting:seed-demo
        {--tenant= : Only this school (school_id UUID or tenant_<hex> schema)}';

    protected $description = 'Add demo data for the accounting features to every tenant schema (additive, idempotent)';

    public function handle(): int
    {
        $tenant = trim((string) $this->option('tenant'));
        if ($tenant !== '') {
            $schema = str_starts_with($tenant, 'tenant_') ? $tenant : Tenancy::schema($tenant);
            if (! Tenancy::exists($schema)) {
                $this->error("$schema: schema tidak ditemukan (jalankan accounting:provision dulu).");

                return self::FAILURE;
            }
            $schemas = [$schema];
        } else {
            $schemas = Tenancy::schemas();
        }

        $failed = false;
        foreach ($schemas as $schema) {
            try {
                Tenancy::activateSchema($schema);
                if (! Schema::hasTable('acc_coa_templates') || ! Schema::hasTable('acc_period_reopen_requests') || ! DB::table('parameter')->exists()) {
                    $this->line("$schema: dilewati (belum dimigrasi / belum ada parameter)");

                    continue;
                }
                $seeder = new DemoFeaturesSeeder;
                $seeder->run();
                $made = $seeder->created;
                $this->line("$schema: ".($made
                    ? implode(', ', array_map(fn ($k, $v) => "$k +$v", array_keys($made), $made))
                    : 'tidak ada perubahan (data demo sudah ada)'));
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
