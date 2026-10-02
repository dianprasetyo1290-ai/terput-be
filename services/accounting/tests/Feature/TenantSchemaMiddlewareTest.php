<?php

namespace Tests\Feature;

use App\Http\Middleware\TenantSchema;
use App\Models\User;
use Illuminate\Http\Request;
use Illuminate\Http\Response;
use Illuminate\Support\Facades\Auth;
use Symfony\Component\HttpKernel\Exception\HttpException;
use Tests\TestCase;

/**
 * The branches that decide whether a request may touch the database at all.
 * None of them reaches Postgres: an authenticated request without a usable
 * tenant must be refused BEFORE any query, because `public` is shared and a
 * silent fallback there is how one school's ledger ends up in another's.
 */
class TenantSchemaMiddlewareTest extends TestCase
{
    private function pass(Request $request): mixed
    {
        $request->setLaravelSession(app('session.store'));

        return (new TenantSchema())->handle($request, fn () => new Response('passed'));
    }

    public function test_guest_without_tenant_passes_through(): void
    {
        // The login/SSO routes must stay reachable before a tenant is known,
        // and public holds no business data.
        $this->assertSame('passed', $this->pass(Request::create('/login'))->getContent());
    }

    public function test_authenticated_request_without_tenant_is_refused(): void
    {
        Auth::guard('web')->setUser(new User());
        $this->assertTrue(Auth::check(), 'sanity: the guard must consider us logged in');

        $this->expectException(HttpException::class);

        try {
            $this->pass(Request::create('/journaltransaction'));
        } catch (HttpException $e) {
            $this->assertSame(403, $e->getStatusCode(), 'must refuse, never serve on public');
            throw $e;
        }
    }

    public function test_unusable_school_id_is_refused_without_reaching_sql(): void
    {
        $request = Request::create('/journaltransaction');
        $request->setLaravelSession(app('session.store'));
        // Anything that cannot become a plain identifier is a bug or an
        // injection attempt; it must never be quoted and run.
        $request->session()->put('sekoci_school_id', 'abc"; drop schema public cascade; --');

        $this->expectException(HttpException::class);
        (new TenantSchema())->handle($request, fn () => new Response('passed'));
    }
}
