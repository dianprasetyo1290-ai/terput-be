<?php

namespace App\Http\Controllers;

use Illuminate\Http\Request;
use Illuminate\Support\Facades\Cache;
use Illuminate\Support\Facades\Log;
use Illuminate\Support\Facades\RateLimiter;
use Illuminate\Support\Facades\Storage;

class HelperController extends Controller
{
    public function showPicture(Request $request, $path = null)
    {
        // Rate limiting to prevent DDOS - 100 requests per minute per IP
        $key = 'show-picture:' . $request->ip();
        if (RateLimiter::tooManyAttempts($key, 100)) {
            abort(429, 'Too many requests');
        }
        RateLimiter::hit($key, 60);

        // Validate and sanitize path input
        if (!$path || strpos($path, '..') !== false || trim($path) === '') {
            return $this->getDefaultImage();
        }

        // Try to get cached response first
        $cacheKey = 'image_exists:' . md5($path);
        $exists = Cache::remember($cacheKey, 300, function () use ($path) {
            return Storage::disk('public')->exists($path);
        });

        if ($exists) {
            try {
                // Set proper cache headers for browser caching
                return Storage::disk('public')->response($path, null, [
                    'Cache-Control' => 'public, max-age=86400', // 24 hours
                    'Expires' => gmdate('D, d M Y H:i:s', time() + 86400) . ' GMT',
                ]);
            } catch (\Exception $e) {
                // Log error and return default image
                Log::warning('Failed to serve image: ' . $path . ' Error: ' . $e->getMessage());

                return $this->getDefaultImage();
            }
        }

        return $this->getDefaultImage();
    }

    private function getDefaultImage()
    {
        try {
            // Try to serve the default image from public disk first
            if (Storage::disk('public')->exists('no-image.png')) {
                return Storage::disk('public')->response('no-image.png', null, [
                    'Cache-Control' => 'public, max-age=86400',
                    'Expires' => gmdate('D, d M Y H:i:s', time() + 86400) . ' GMT',
                ]);
            }

            // Try default disk as fallback
            if (Storage::exists('no-image.png')) {
                return Storage::response('no-image.png', null, [
                    'Cache-Control' => 'public, max-age=86400',
                    'Expires' => gmdate('D, d M Y H:i:s', time() + 86400) . ' GMT',
                ]);
            }

            // If no default image exists, return a simple 1x1 transparent PNG
            return $this->generateTransparentPixel();
        } catch (\Exception) {
            // Last resort: return transparent pixel
            return $this->generateTransparentPixel();
        }
    }

    private function generateTransparentPixel()
    {
        // Generate a 1x1 transparent PNG
        $transparentPixel = base64_decode('iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAAC0lEQVQIHWNgAAIAAAUAAY27m/MAAAAASUVORK5CYII=');

        return response($transparentPixel)
            ->header('Content-Type', 'image/png')
            ->header('Cache-Control', 'public, max-age=86400')
            ->header('Expires', gmdate('D, d M Y H:i:s', time() + 86400) . ' GMT')
            ->header('Content-Length', strlen($transparentPixel));
    }
}
