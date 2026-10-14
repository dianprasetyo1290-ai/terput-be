<?php

namespace Tests\Feature;

// use Illuminate\Foundation\Testing\RefreshDatabase;
use Tests\TestCase;

class ExampleTest extends TestCase
{
    protected function setUp(): void
    {
        parent::setUp();
        // Unmodified Jetstream scaffold: assumes a `users` table with `name`,
        // a password login and a `dashboard` route. Sekoci Accounting uses
        // mst_user, SSO-only login and refuses authenticated requests without
        // a tenant (TenantSchema), so these cases do not apply.
        $this->markTestSkipped('Jetstream scaffold test; not applicable to SSO-only, tenant-scoped Sekoci Accounting.');
    }

    /**
     * A basic test example.
     */
    public function test_the_application_returns_a_successful_response(): void
    {
        $response = $this->get('/');

        $response->assertStatus(200);
    }
}
