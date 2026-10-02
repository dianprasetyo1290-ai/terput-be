<?php

namespace Tests\Feature;

use App\Http\Controllers\SsoController;
use Illuminate\Http\Client\ConnectionException;
use Illuminate\Support\Facades\Http;
use Tests\TestCase;

/**
 * Saklar fitur Accounting per sekolah (DESIGN ADDENDUM 2). HTTP client difake,
 * jadi test ini tidak butuh jaringan maupun identity-service hidup.
 */
class SsoSchoolFeatureTest extends TestCase
{
    private const SCHOOL = '5006c011-b21e-42ad-9fb5-c07de8d99f74';

    protected function setUp(): void
    {
        parent::setUp();
        config(['app.identity_service_url' => 'http://identity:8081', 'app.sekoci_internal_key' => 'kunci-internal']);
    }

    private function fakeFeatures(array $features, int $status = 200): void
    {
        Http::fake(['identity:8081/*' => Http::response(['data' => $features, 'message' => '', 'errors' => null], $status)]);
    }

    public function test_enabled_feature_passes(): void
    {
        $this->fakeFeatures([['feature_key' => 'accounting', 'enabled' => true]]);

        $this->assertTrue(SsoController::accountingEnabled(self::SCHOOL));

        Http::assertSent(fn ($request) => $request->url() === 'http://identity:8081/schools/'.urlencode(self::SCHOOL).'/features'
            && $request->hasHeader('X-Internal-Key', 'kunci-internal')
            && ! $request->hasHeader('Authorization'));
    }

    public function test_disabled_feature_is_rejected(): void
    {
        $this->fakeFeatures([['feature_key' => 'accounting', 'enabled' => false]]);

        $this->assertFalse(SsoController::accountingEnabled(self::SCHOOL));
    }

    public function test_feature_absent_from_list_is_rejected(): void
    {
        $this->fakeFeatures([['feature_key' => 'ppdb', 'enabled' => true]]);

        $this->assertFalse(SsoController::accountingEnabled(self::SCHOOL));
        $this->assertFalse(SsoController::accountingEnabled('')); // school_id tak ada di JWT
    }

    public function test_non_200_response_fails_closed(): void
    {
        foreach ([401, 403, 404, 500, 503] as $status) {
            $this->fakeFeatures([['feature_key' => 'accounting', 'enabled' => true]], $status);
            $this->assertFalse(SsoController::accountingEnabled(self::SCHOOL), "status $status harus ditolak");
        }
    }

    public function test_unreachable_identity_fails_closed(): void
    {
        Http::fake(fn () => throw new ConnectionException('Connection timed out'));

        $this->assertFalse(SsoController::accountingEnabled(self::SCHOOL));
    }
}
