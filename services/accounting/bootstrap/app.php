<?php

use Illuminate\Foundation\Application;
use Illuminate\Foundation\Configuration\Exceptions;
use Illuminate\Foundation\Configuration\Middleware;

return Application::configure(basePath: dirname(__DIR__))
    ->withRouting(
        web: __DIR__.'/../routes/web.php',
        api: __DIR__.'/../routes/api.php',
        commands: __DIR__.'/../routes/console.php',
        health: '/up',
    )
    ->withMiddleware(function (Middleware $middleware) {
        $middleware->trustProxies(at: '*');
        $middleware->prepend(\App\Http\Middleware\ForwardedPrefix::class);
        // TenantSchema before EmbedMode: nothing after it may touch the DB
        // before search_path points at the school's schema.
        $middleware->web(append: [
            \App\Http\Middleware\TenantSchema::class,
            \App\Http\Middleware\EmbedMode::class,
        ]);
        $middleware->validateCsrfTokens(except: ['sso']);

        // Laravel SORTS route middleware by its priority list, and
        // Authenticate sits above SubstituteBindings there -- so appending
        // TenantSchema to the `web` group is not enough: the guard would run
        // first and look the user up in `public.mst_user`, which is empty,
        // redirecting every authenticated request to /login. Pin TenantSchema
        // ahead of Authenticate (still after StartSession, which the priority
        // list keeps above it) so search_path is set before the first query.
        $middleware->prependToPriorityList(
            before: \Illuminate\Contracts\Auth\Middleware\AuthenticatesRequests::class,
            prepend: \App\Http\Middleware\TenantSchema::class,
        );
    })
    ->withExceptions(function (Exceptions $exceptions) {
        //
    })->create();
