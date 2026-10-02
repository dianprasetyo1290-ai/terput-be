<?php

namespace App\Console\Commands;

use Illuminate\Console\Command;
use Illuminate\Support\Facades\DB;

class ListTenants extends Command
{
    protected $signature = 'accounting:tenants';

    protected $description = 'List tenant schemas with table count + migration status';

    public function handle(): int
    {
        $rows = DB::select(<<<'SQL'
            select s.schema_name as schema,
                   (select count(*) from information_schema.tables t
                     where t.table_schema = s.schema_name and t.table_type = 'BASE TABLE') as tables,
                   exists (select 1 from information_schema.tables t
                     where t.table_schema = s.schema_name and t.table_name = 'migrations') as has_migrations
            from information_schema.schemata s
            where s.schema_name like 'tenant\_%'
            order by s.schema_name
        SQL);

        if ($rows === []) {
            $this->warn('Belum ada schema tenant. Jalankan: php artisan accounting:provision <school_id>');

            return self::SUCCESS;
        }

        $this->table(['schema', 'tables', 'migrated'], array_map(fn ($r) => [
            $r->schema,
            $r->tables,
            $r->has_migrations
                ? DB::scalar('select count(*) from "'.$r->schema.'".migrations').' migrasi'
                : 'BELUM',
        ], $rows));

        return self::SUCCESS;
    }
}
