<?php

namespace App\Http\Middleware;

use App\Support\Tenancy;
use Closure;
use Illuminate\Http\Request;
use Illuminate\Support\Facades\Auth;
use Symfony\Component\HttpFoundation\Response;

/**
 * Points every request at its school's Postgres schema (ADDENDUM 1).
 *
 * This is the ONLY thing standing between two schools' books: the models and
 * Livewire components carry no school_id filter at all, so if search_path is
 * left at `public` an authenticated request happily reads and writes whatever
 * it finds there. That is why an authenticated request without a usable
 * tenant is logged out and refused rather than served -- "fall back to
 * public" is exactly the failure that loses one school's ledger inside
 * another's.
 *
 * Appended to the `web` group so the session (where /sso puts
 * sekoci_school_id) is already started, and so it runs before the `auth`
 * middleware resolves the user out of the tenant's own mst_user table.
 * Guests are let through untouched: public holds no business data, and the
 * login/SSO routes must stay reachable before a tenant is known.
 */
class TenantSchema
{
    public function handle(Request $request, Closure $next): Response
    {
        $schoolId = (string) $request->session()->get('sekoci_school_id', '');

        if ($schoolId === '') {
            return Auth::check() ? $this->reject($request) : $next($request);
        }

        try {
            $schema = Tenancy::schema($schoolId);
        } catch (\InvalidArgumentException) {
            // A school_id that cannot become an identifier reached the session
            // from a JWT claim -- never interpolate it, never guess.
            return $this->reject($request);
        }

        // Without this the tables would silently resolve against `public`,
        // where an unprovisioned school's writes would pile up.
        if (! Tenancy::exists($schema)) {
            return $this->reject($request, 'Akuntansi belum disiapkan untuk sekolah ini.');
        }

        Tenancy::activate($schoolId);

        return $next($request);
    }

    private function reject(Request $request, string $message = 'Sesi Akuntansi tidak valid. Silakan buka ulang dari Sekoci.'): Response
    {
        Auth::guard('web')->logout();
        $request->session()->forget('sekoci_school_id');

        abort(403, $message);
    }
}
