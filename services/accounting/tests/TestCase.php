<?php

namespace Tests;

use Illuminate\Foundation\Testing\TestCase as BaseTestCase;
use Illuminate\Support\Facades\URL;

abstract class TestCase extends BaseTestCase
{
    /**
     * ForwardedPrefix forces the URL root to ".../accounting" during a request,
     * and the URL generator outlives the request inside one test -- so a second
     * request in the same test would be sent to /accounting/... and 404.
     */
    protected function prepareUrlForRequest($uri)
    {
        URL::forceRootUrl(null);

        return parent::prepareUrlForRequest($uri);
    }
}
