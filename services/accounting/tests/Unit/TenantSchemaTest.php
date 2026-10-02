<?php

namespace Tests\Unit;

use App\Support\Tenancy;
use InvalidArgumentException;
use PHPUnit\Framework\TestCase;

class TenantSchemaTest extends TestCase
{
    public function test_uuid_becomes_schema_name(): void
    {
        $this->assertSame(
            'tenant_5006c011b21e42ad9fb5c07de8d99f74',
            Tenancy::schema('5006c011-b21e-42ad-9fb5-c07de8d99f74')
        );
        $this->assertSame(
            'tenant_5006c011b21e42ad9fb5c07de8d99f74',
            Tenancy::schema(' 5006C011-B21E-42AD-9FB5-C07DE8D99F74 ')
        );
    }

    /**
     * Hostile input must be REJECTED, not escaped -- the schema name is
     * interpolated into `SET search_path`, so there is no safe quoting story.
     */
    #[\PHPUnit\Framework\Attributes\DataProvider('hostile')]
    public function test_rejects_unsafe_input(string $input): void
    {
        $this->expectException(InvalidArgumentException::class);
        Tenancy::schema($input);
    }

    public static function hostile(): array
    {
        return [
            ['abc"; drop schema public cascade; --'],
            ['abc\'; drop schema public cascade; --'],
            ['tenant one'],                     // space
            ['tenant.one'],                     // schema-qualified
            [''],                               // empty -> "tenant_"
            [str_repeat('a', 60)],              // too long for Postgres
            ["5006c011\nb21e"],                 // newline
        ];
    }

}
