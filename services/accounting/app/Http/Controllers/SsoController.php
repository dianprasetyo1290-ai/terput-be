<?php

namespace App\Http\Controllers;

use App\Models\User;
use App\Support\Tenancy;
use Illuminate\Http\Request;
use Illuminate\Support\Facades\Auth;
use Illuminate\Support\Facades\Http;
use Illuminate\Support\Str;

class SsoController extends Controller
{
    /** Sekoci role -> accounting (spatie) role. Anything else is denied. */
    public const ROLE_MAP = [
        'superadmin' => 'SuperAdmin',
        'admin' => 'Admin',
        'kepala_sekolah' => 'Finance',
        'kepala_yayasan' => 'Finance',
    ];

    /**
     * Verify a Sekoci HS256 JWT. Returns the claims, or null if the token is
     * malformed, wrongly signed, not HS256, or expired.
     */
    public static function verify(string $token, string $secret): ?array
    {
        $parts = explode('.', $token);
        if (count($parts) !== 3 || $secret === '') {
            return null;
        }
        [$head, $body, $sig] = $parts;

        $header = json_decode(self::b64($head), true);
        if (! is_array($header) || ($header['alg'] ?? null) !== 'HS256') {
            return null;
        }

        $expected = self::b64url(hash_hmac('sha256', $head.'.'.$body, $secret, true));
        if (! hash_equals($expected, $sig)) {
            return null;
        }

        $claims = json_decode(self::b64($body), true);
        if (! is_array($claims) || ! isset($claims['exp']) || time() >= (int) $claims['exp']) {
            return null;
        }

        return $claims;
    }

    /**
     * Saklar fitur per sekolah ada di identity-service (ADDENDUM 2). Ini data
     * keuangan: fail CLOSED — identity tak terjangkau/lambat/bukan 200 = false.
     */
    public static function accountingEnabled(string $schoolId): bool
    {
        if ($schoolId === '') {
            return false;
        }

        try {
            $response = Http::withHeaders(['X-Internal-Key' => (string) config('app.sekoci_internal_key')])
                ->acceptJson()
                ->connectTimeout(3)
                ->timeout(5)
                ->get(rtrim((string) config('app.identity_service_url'), '/').'/schools/'.urlencode($schoolId).'/features');
        } catch (\Throwable $e) {
            return false;
        }

        if ($response->status() !== 200) {
            return false;
        }

        foreach ((array) $response->json('data') as $feature) {
            if (($feature['feature_key'] ?? null) === 'accounting') {
                return ($feature['enabled'] ?? false) === true;
            }
        }

        return false;
    }

    public function login(Request $request)
    {
        $token = (string) $request->input('token', '');
        $claims = self::verify($token, (string) config('app.sekoci_jwt_secret'));
        if ($claims === null) {
            return response()->json(['message' => 'Token tidak valid'], 401);
        }

        $userId = (string) ($claims['user_id'] ?? $claims['sub'] ?? '');
        $role = self::ROLE_MAP[(string) ($claims['active_role'] ?? '')] ?? null;
        if ($userId === '' || $role === null) {
            return response()->json(['message' => 'Role tidak diizinkan untuk Accounting'], 403);
        }

        $schoolId = (string) ($claims['school_id'] ?? '');
        // Superadmin's JWT carries no school (cross-school role); like the Go
        // services' ?school_id= override, it names the school it is browsing.
        if ($schoolId === '' && ($claims['active_role'] ?? '') === 'superadmin') {
            $schoolId = (string) $request->input('school_id', '');
        }
        if (! self::accountingEnabled($schoolId)) {
            return response()->json(['message' => 'Fitur Accounting tidak aktif untuk sekolah ini'], 403);
        }

        // Everything below writes the school's OWN books (mst_user and the
        // spatie role tables live inside the tenant schema), and on /sso the
        // session has no tenant yet, so TenantSchema let this request through
        // pointing at `public`. Activate before the first query or every
        // school's users pile up in the shared schema.
        try {
            $schema = Tenancy::schema($schoolId);
        } catch (\InvalidArgumentException) {
            return response()->json(['message' => 'school_id tidak valid'], 403);
        }
        if (! Tenancy::exists($schema)) {
            return response()->json(['message' => 'Akuntansi belum disiapkan untuk sekolah ini'], 403);
        }
        Tenancy::activate($schoolId);

        $user = User::withTrashed()->find($userId);
        if (! $user) {
            $user = new User();
            $user->setAttribute('user_id', $userId);
            $user->password = Str::random(40);
        }
        $user->deleted_at = null;
        $user->user_name = (string) ($claims['name'] ?? $user->user_name ?? $userId);
        $user->email = $claims['email'] ?? $user->email ?? $userId.'@sekoci.local';
        $user->email_verified_at ??= now();
        $user->actived = true;
        $user->save();

        if (! $user->hasRole($role)) {
            $user->syncRoles([$role]);
        }

        Auth::login($user);
        $request->session()->regenerate();
        $request->session()->put('sekoci_school_id', $schoolId);

        return response()->noContent();
    }

    private static function b64(string $v): string
    {
        return (string) base64_decode(strtr($v, '-_', '+/'), true);
    }

    private static function b64url(string $raw): string
    {
        return rtrim(strtr(base64_encode($raw), '+/', '-_'), '=');
    }
}
