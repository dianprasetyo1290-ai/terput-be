<?php

namespace App\Http\Middleware;

use Closure;
use Illuminate\Http\Request;
use Illuminate\Support\Facades\View;

/**
 * ?embed=1 means we are inside the Sekoci shell's iframe: hide our own chrome.
 * Livewire POSTs carry no query string, so the flag is remembered in the session.
 */
class EmbedMode
{
    public function handle(Request $request, Closure $next)
    {
        if ($request->has('embed')) {
            $request->session()->put('embed', $request->boolean('embed'));
        }

        View::share('embed', (bool) $request->session()->get('embed', false));

        return $next($request);
    }
}
