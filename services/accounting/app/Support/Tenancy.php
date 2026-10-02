<?php

namespace App\Support;

use Illuminate\Support\Facades\DB;
use InvalidArgumentException;

/**
 * Schema-per-tenant (one Postgres schema per school, one database).
 *
 * `public` holds only the shared infrastructure tables listed in
 * PUBLIC_TABLES; every business table lives in a `tenant_<...>` schema.
 * Isolation comes from `search_path`, so nothing in the models/Livewire
 * layer has to know about tenants -- which also means a wrong schema name
 * here would be a silent cross-tenant leak. Hence schema() validates
 * instead of escaping: a school id that is not plain [a-z0-9_] after
 * normalisation is a bug or an attack, never something to quote and run.
 */
final class Tenancy
{
    /**
     * Tables that belong to `public` only (sessions must be readable before
     * the tenant is known -- it is where the tenant comes from). Used by
     * accounting:provision to remove the copies `migrate` creates inside a
     * tenant schema, so they cannot shadow the public ones.
     */
    public const PUBLIC_TABLES = [
        'sessions',
        'cache',
        'cache_locks',
        'jobs',
        'job_batches',
        'failed_jobs',
        'telescope_entries',
        'telescope_entries_tags',
        'telescope_monitoring',
    ];

    /**
     * Sekoci school_id (UUID) -> schema name, e.g.
     * 5006c011-b21e-42ad-9fb5-c07de8d99f74 -> tenant_5006c011b21e42ad9fb5c07de8d99f74
     *
     * @throws InvalidArgumentException when the result is not a safe identifier
     */
    public static function schema(string $schoolId): string
    {
        $schema = 'tenant_'.strtolower(str_replace('-', '', trim($schoolId)));

        // Postgres identifiers are max 63 chars; 'tenant_' + 32 hex = 39.
        if (! preg_match('/^tenant_[a-z0-9_]{1,56}$/', $schema)) {
            throw new InvalidArgumentException('school_id tidak valid untuk nama schema: '.$schoolId);
        }

        return $schema;
    }

    /**
     * Point the current connection at this school's schema for the rest of
     * the request. Returns the schema name.
     *
     * `public` stays last in the path on purpose: shared infra tables and
     * extension functions resolve there, tenant tables always win.
     */
    public static function activate(string $schoolId, ?string $connection = null): string
    {
        $schema = self::schema($schoolId);

        DB::connection($connection)->statement('SET search_path TO "'.$schema.'", public');

        return $schema;
    }

    public static function exists(string $schema, ?string $connection = null): bool
    {
        return DB::connection($connection)
            ->selectOne('select 1 from information_schema.schemata where schema_name = ?', [$schema]) !== null;
    }
}
