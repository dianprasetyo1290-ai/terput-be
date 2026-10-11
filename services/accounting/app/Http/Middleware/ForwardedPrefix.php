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
        if ($prefix === '') {
            $prefix = '/accounting';
        }

        // If an outer reverse proxy hosted Sekoci under a subpath like /teratai:
        if ($outer = $request->header('X-Forwarded-Prefix-Outer')) {
            $outer = rtrim(trim($outer), '/');
            if ($outer !== '' && !str_starts_with($prefix, $outer)) {
                $prefix = $outer . $prefix;
            }
        } elseif ($referer = (string) $request->header('Referer')) {
            $refPath = parse_url($referer, PHP_URL_PATH) ?? '';
            if (preg_match('#^(/[^/]+)/akuntansi#', $refPath, $m)) {
                $sub = $m[1];
                if (!str_starts_with($prefix, $sub)) {
                    $prefix = $sub . $prefix;
                }
            }
        }

        if ($prefix !== '' && str_starts_with($prefix, '/')) {
            URL::forceRootUrl($request->getSchemeAndHttpHost().$prefix);
        }

        $response = $next($request);

        // Livewire and limitless template assets emit root-relative paths in HTML.
        // Re-prefix them under a sub-path so they route through the reverse proxy correctly.
        if ($prefix !== '' && str_starts_with($prefix, '/')
            && str_contains((string) $response->headers->get('Content-Type'), 'text/html')) {
            $html = $response->getContent();
            if (is_string($html) && $html !== '') {
                $response->setContent(str_replace(
                    [
                        'data-update-uri="/livewire/',
                        'src="/livewire/',
                        'src="/limitless/',
                        'href="/limitless/',
                        'src="/costum/',
                        'href="/costum/',
                        'src="/build/',
                        'href="/build/',
                    ],
                    [
                        'data-update-uri="'.$prefix.'/livewire/',
                        'src="'.$prefix.'/livewire/',
                        'src="'.$prefix.'/limitless/',
                        'href="'.$prefix.'/limitless/',
                        'src="'.$prefix.'/costum/',
                        'href="'.$prefix.'/costum/',
                        'src="'.$prefix.'/build/',
                        'href="'.$prefix.'/build/',
                    ],
                    $html,
                ));
            }
        }

        return $response;
    }
}

