<?php

namespace App\Console\Commands;

use App\Support\Tenancy;
use Illuminate\Console\Command;
use Illuminate\Support\Facades\DB;
use Illuminate\Support\Facades\Process;

/**
 * Provision one school: create its schema, migrate it, and seed the baseline
 * ONLY while that schema is still empty. Run on a school's first SSO, or by
 * hand. Safe to run twice -- that is the point: container restarts and deploys
 * must never reseed, because the baseline seeders truncate.
 */
class ProvisionTenant extends Command
{
    protected $signature = 'accounting:provision
        {school_id : Sekoci school_id (UUID)}
        {--seed : Also seed a schema that ALREADY has tables. The baseline seeders TRUNCATE, so only pass this when you really mean to reset the baseline.}';

    protected $description = 'Create + migrate (+ baseline-seed when empty) the accounting schema for one school';

    public function handle(): int
    {
        $schema = Tenancy::schema($this->argument('school_id'));

        DB::statement('CREATE SCHEMA IF NOT EXISTS "'.$schema.'"');
        $wasEmpty = $this->tableCount($schema) === 0;
        $this->info("[$schema] ".($wasEmpty ? 'schema baru / masih kosong' : 'schema sudah ada isinya'));

        if (! $this->artisan($schema, ['migrate', '--force'])) {
            return self::FAILURE;
        }

        if ($wasEmpty && $this->tableCount($schema) === 0) {
            $this->error("[$schema] migrate tidak membuat tabel apa pun di schema ini — DB_SEARCH_PATH tidak terpakai. JANGAN lanjut.");

            return self::FAILURE;
        }

        // migrate also recreated the shared infra tables inside this schema;
        // drop them so they cannot shadow public's. A tenant-local `sessions`
        // in particular would break login: the session is read before the
        // tenant -- and therefore the search_path -- is known.
        foreach (Tenancy::PUBLIC_TABLES as $table) {
            DB::statement('DROP TABLE IF EXISTS "'.$schema.'"."'.$table.'" CASCADE');
        }

        if (! ($wasEmpty || $this->option('seed'))) {
            $this->warn("[$schema] seeder dilewati: schema sudah punya data (pakai --seed hanya kalau memang mau reset baseline).");

            return self::SUCCESS;
        }

        if (! $this->artisan($schema, ['db:seed', '--force'])) {
            return self::FAILURE;
        }

        $this->info("[$schema] selesai.");

        return self::SUCCESS;
    }

    /**
     * Run artisan in a SUBPROCESS pinned to this tenant's schema.
     *
     * Not `$this->call(...)` with a mutated config + DB::purge(): purging the
     * connection leaves other already-resolved holders of it (the database
     * cache store, resolved during bootstrap) with a dead PDO. The first thing
     * they do is reconnect -- and DatabaseManager::reconnect() disconnects the
     * live connection first, which silently ROLLS BACK the transaction the
     * migration is running in. The result is the worst kind of failure: the
     * migration is recorded as run in `migrations`, but its tables are gone,
     * and `migrate` exits 0. (Hit on 2026-10-02 with the spatie permission
     * migration, whose up() ends with a cache forget.) A fresh process boots
     * with the right search_path from the start and has nothing stale.
     */
    private function artisan(string $schema, array $args): bool
    {
        $result = Process::forever()
            ->env(['DB_SEARCH_PATH' => $schema.',public'])
            ->run(array_merge([PHP_BINARY, base_path('artisan')], $args),
                fn ($type, $out) => $this->output->write($out));

        if (! $result->successful()) {
            $this->error("[$schema] gagal: artisan ".implode(' ', $args));
        }

        return $result->successful();
    }

    private function tableCount(string $schema): int
    {
        return (int) DB::scalar(
            "select count(*) from information_schema.tables where table_schema = ? and table_type = 'BASE TABLE'",
            [$schema]
        );
    }
}
