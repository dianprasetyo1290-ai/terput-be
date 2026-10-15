<?php

namespace Tests\Feature;

use App\Models\User;
use App\Support\LivewireGuard;
use Illuminate\Foundation\Testing\RefreshDatabase;
use Illuminate\Routing\Route;
use Illuminate\Support\Facades\Gate;
use Illuminate\Support\Facades\URL;
use Tests\Concerns\ActsAsAccountingRoles;
use Tests\Concerns\SeedsBooks;
use Tests\TestCase;

/**
 * QA round 2: for every accounting role, every page opens (no 403/404/500
 * from a menu) and every button rendered on it is one the role may press
 * (no button that ends in a 403).
 */
class QaAccRoleSmokeTest extends TestCase
{
    use ActsAsAccountingRoles, RefreshDatabase, SeedsBooks;

    private const ROLES = ['SuperAdmin', 'Admin', 'Accounting', 'FinanceManager', 'Auditor', 'Executive'];

    protected function setUp(): void
    {
        parent::setUp();
        $this->seedBooks();
        $this->installRoles();
        $this->withoutMiddleware(\App\Http\Middleware\TenantSchema::class);
        \Spatie\Permission\Models\Role::findOrCreate('SuperAdmin', 'web');
        $this->userAs('U1', 'Accounting');
        $this->book('JU-0001', '2026-10-05', [['6101', 100, 0], ['1101', 0, 100]], 'draft');
    }

    /** GET pages of the app: [uri, permission|null, component|null]. */
    private function pages(): array
    {
        $out = [];
        foreach (app('router')->getRoutes() as $r) {
            /** @var Route $r */
            if (! in_array('GET', $r->methods(), true) || ! in_array('auth:sanctum', $r->gatherMiddleware(), true)) {
                continue;
            }
            $uri = $r->uri();
            if (preg_match('/\{[a-zA-Z]+\}/', $uri) || str_starts_with($uri, 'api/') || in_array($uri, ['z', 'summarygeneralledger', 'user/profile', 'user/api-tokens', 'logout'], true)) {
                continue; // needs a parameter, JSON/print endpoint, or Jetstream page
            }
            $uri = preg_replace('/\/?\{[a-zA-Z]+\?\}/', '', $uri);
            $perm = null;
            foreach ($r->gatherMiddleware() as $m) {
                if (is_string($m) && str_starts_with($m, 'can:')) {
                    $perm = substr($m, 4);
                }
            }
            $action = $r->getAction('uses');
            $action = is_string($action) ? explode('@', $action)[0] : $r->getControllerClass();
            $component = $action && is_subclass_of($action, \Livewire\Component::class) ? $action : null;
            $out[] = [$uri === '' ? '/' : '/'.ltrim($uri, '/'), $perm, $component];
        }

        return $out;
    }

    private static function where(?\Throwable $e): string
    {
        if (! $e) {
            return '';
        }
        $at = '';
        foreach (array_merge([['file' => $e->getFile(), 'line' => $e->getLine()]], $e->getTrace()) as $f) {
            if (isset($f['file']) && (str_contains($f['file'], '/app/app/') || str_contains($f['file'], '/storage/framework/views/'))) {
                $at = basename($f['file']).':'.$f['line'];
                break;
            }
        }

        return mb_substr(strip_tags($e->getMessage()), 0, 160).' @'.$at;
    }

    private function user(string $role): User
    {
        return $this->userAs('R'.substr(md5($role), 0, 6), $role);
    }

    private function check(string $role): void
    {
        $user = $this->user($role);
        $this->actingAs($user);
        $problems = [];
        $buttons = 0;
        $pages = 0;
        foreach ($this->pages() as [$uri, $perm, $component]) {
            if (in_array($uri, ['/', '/user/profile'], true)) {
                continue;
            }
            URL::forceRootUrl(null);
            $allowed = $perm === null || $user->can($perm);
            // A savepoint per page: one failing query must not poison the rest.
            \Illuminate\Support\Facades\DB::beginTransaction();
            try {
                $res = $this->get($uri);
            } finally {
                \Illuminate\Support\Facades\DB::rollBack();
            }
            $code = $res->getStatusCode();
            if ($allowed && $code === 302) {
                continue; // opened without its record: sent back to its list
            }
            if ($allowed && $code !== 200) {
                $problems[] = "$role GET $uri -> $code (expected 200)".($code >= 500 ? ': '.self::where($res->exception) : '');

                continue;
            }
            if (! $allowed && $code !== 403) {
                $problems[] = "$role GET $uri -> $code (expected 403)";

                continue;
            }
            if (! $allowed || ! $component) {
                continue;
            }
            // Every button on the page must be allowed for this role.
            $html = (string) $res->getContent();
            preg_match_all('/wire:(?:click(?:\.prevent)?|submit(?:\.prevent)?)\s*=\s*["\']([A-Za-z_]+)/', $html, $m);
            // Classes of every Livewire component rendered on the page.
            $classes = [];
            preg_match_all('/wire:snapshot="([^"]+)"/', $html, $snaps);
            foreach ($snaps[1] as $snap) {
                $name = json_decode(html_entity_decode($snap), true)['memo']['name'] ?? null;
                if ($name) {
                    try {
                        $classes[] = app(\Livewire\Mechanisms\ComponentRegistry::class)->getClass($name);
                    } catch (\Throwable) {
                    }
                }
            }
            $pages++;
            foreach (array_unique($m[1]) as $method) {
                $buttons++;
                $exists = false;
                foreach ($classes as $cls) {
                    if (method_exists($cls, $method) && (new \ReflectionMethod($cls, $method))->isPublic()) {
                        $exists = true;
                    }
                }
                if (! $exists) {
                    $problems[] = "$role on $uri: button $method() has no handler";
                }
                $need = LivewireGuard::permissionFor($component, $method);
                if ($need !== null && ! Gate::forUser($user)->any($need)) {
                    $problems[] = "$role on $uri sees button $method() needing ".implode('|', $need);
                }
            }
        }
        // Every menu entry the role sees opens.
        URL::forceRootUrl(null);
        $home = $this->get('/home');
        if ($home->getStatusCode() !== 200) {
            $problems[] = "$role GET /home -> ".$home->getStatusCode();
        } else {
            $html = (string) $home->getContent();
            $nav = substr($html, (int) strpos($html, 'nav-sidebar'));
            preg_match_all('#href="https?://[^/"]+(/[^"\#]*)"#', $nav, $mm);
            foreach (array_unique($mm[1]) as $path) {
                $path = preg_replace('#^/accounting#', '', $path);
                if ($path === '' || $path === '/' || str_contains($path, 'logout') || str_contains($path, 'profile')) {
                    continue;
                }
                URL::forceRootUrl(null);
                \Illuminate\Support\Facades\DB::beginTransaction();
                try {
                    $code = $this->get($path)->getStatusCode();
                } finally {
                    \Illuminate\Support\Facades\DB::rollBack();
                }
                if ($code !== 200) {
                    $problems[] = "$role menu $path -> $code";
                }
            }
        }
        $this->assertSame([], $problems, implode("\n", $problems));
        fwrite(STDERR, "[$role] pages=$pages buttons=$buttons\n");
        $this->assertGreaterThan(0, $buttons);
    }

    public function test_superadmin(): void
    {
        $this->check('SuperAdmin');
    }

    public function test_admin(): void
    {
        $this->check('Admin');
    }

    public function test_accounting(): void
    {
        $this->check('Accounting');
    }

    public function test_finance_manager(): void
    {
        $this->check('FinanceManager');
    }

    public function test_auditor(): void
    {
        $this->check('Auditor');
    }

    public function test_executive(): void
    {
        $this->check('Executive');
    }
}
