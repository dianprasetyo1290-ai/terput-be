<?php

namespace App\Http\Middleware;

use Closure;
use Illuminate\Http\Request;
use Symfony\Component\HttpFoundation\Response;

class InternalKey
{
    public function handle(Request $request, Closure $next): Response
    {
        $expectedKey = (string) config('app.sekoci_internal_key');
        $providedKey = (string) $request->header('X-Internal-Key', '');

        if ($expectedKey === '' || $providedKey === '' || ! hash_equals($expectedKey, $providedKey)) {
            return response()->json(['message' => 'Unauthorized: Invalid or missing X-Internal-Key'], 401);
        }

        return $next($request);
    }
}
