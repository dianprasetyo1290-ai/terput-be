<?php

namespace Tests\Unit;

use App\Http\Controllers\SsoController;
use PHPUnit\Framework\TestCase;

class SsoJwtTest extends TestCase
{
    private const SECRET = 'rahasia-sekoci';

    private function jwt(array $claims, string $secret = self::SECRET, array $header = ['alg' => 'HS256', 'typ' => 'JWT']): string
    {
        $b64 = fn ($v) => rtrim(strtr(base64_encode($v), '+/', '-_'), '=');
        $payload = $b64(json_encode($header)).'.'.$b64(json_encode($claims));

        return $payload.'.'.$b64(hash_hmac('sha256', $payload, $secret, true));
    }

    public function test_valid_token_returns_claims(): void
    {
        $claims = ['user_id' => 'abc-123', 'active_role' => 'admin', 'school_id' => 7, 'exp' => time() + 600];

        $this->assertSame($claims, SsoController::verify($this->jwt($claims), self::SECRET));
    }

    public function test_wrong_signature_is_rejected(): void
    {
        $token = $this->jwt(['user_id' => 'abc', 'exp' => time() + 600], 'secret-lain');

        $this->assertNull(SsoController::verify($token, self::SECRET));
    }

    public function test_tampered_payload_is_rejected(): void
    {
        [$h, $p, $s] = explode('.', $this->jwt(['user_id' => 'abc', 'active_role' => 'admin', 'exp' => time() + 600]));
        $evil = rtrim(strtr(base64_encode(json_encode(['user_id' => 'abc', 'active_role' => 'superadmin', 'exp' => time() + 600])), '+/', '-_'), '=');

        $this->assertNull(SsoController::verify($h.'.'.$evil.'.'.$s, self::SECRET));
    }

    public function test_expired_or_missing_exp_is_rejected(): void
    {
        $this->assertNull(SsoController::verify($this->jwt(['user_id' => 'abc', 'exp' => time() - 1]), self::SECRET));
        $this->assertNull(SsoController::verify($this->jwt(['user_id' => 'abc']), self::SECRET));
    }

    public function test_malformed_token_or_empty_secret_is_rejected(): void
    {
        $this->assertNull(SsoController::verify('bukan.jwt', self::SECRET));
        $this->assertNull(SsoController::verify($this->jwt(['user_id' => 'abc', 'exp' => time() + 600]), ''));
    }

    public function test_alg_none_is_rejected(): void
    {
        $token = $this->jwt(['user_id' => 'abc', 'exp' => time() + 600], self::SECRET, ['alg' => 'none', 'typ' => 'JWT']);

        $this->assertNull(SsoController::verify($token, self::SECRET));
    }

    public function test_only_mapped_sekoci_roles_are_allowed(): void
    {
        $this->assertSame('SuperAdmin', SsoController::ROLE_MAP['superadmin']);
        $this->assertSame('Executive', SsoController::ROLE_MAP['kepala_sekolah']);
        $this->assertSame('Executive', SsoController::ROLE_MAP['kepala_yayasan']);
        $this->assertSame('FinanceManager', SsoController::ROLE_MAP['finance_manager']);
        $this->assertSame('Auditor', SsoController::ROLE_MAP['auditor']);
        $this->assertSame('Accounting', SsoController::ROLE_MAP['akuntan']);
        $this->assertArrayNotHasKey('guru', SsoController::ROLE_MAP);
        $this->assertArrayNotHasKey('siswa', SsoController::ROLE_MAP);
    }
}
