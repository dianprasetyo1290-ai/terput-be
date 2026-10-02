<?php

namespace App\Http\Middleware;

use Closure;
use Illuminate\Http\Request;
use Illuminate\Support\Facades\URL;

/**
 * Sekoci's nginx proxies /accounting/ -> this app with the prefix stripped, so
 * Laravel never sees it. It sends it back as X-Forwarded-Prefix; we re-apply it
 * to the URL generator so route()/asset()/url() keep pointing at /accounting/...
 */
class ForwardedPrefix
{
    public function handle(Request $request, Closure $next)
    {
        if ($request->header('X-Forwarded-Proto') === 'https') {
            URL::forceScheme('https');
        }

        $prefix = rtrim(trim((string) $request->header('X-Forwarded-Prefix')), '/');
        if ($prefix !== '' && str_starts_with($prefix, '/')) {
            URL::forceRootUrl($request->getSchemeAndHttpHost().$prefix);
        }

        return $next($request);
    }
}
