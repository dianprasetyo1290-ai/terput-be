<?php

namespace Tests\Feature;

use App\Models\User;
use App\Support\Journals;
use Database\Seeders\DemoFeaturesSeeder;
use Illuminate\Foundation\Testing\RefreshDatabase;
use Illuminate\Support\Facades\DB;
use Tests\Concerns\SeedsBooks;
use Tests\TestCase;

/** accounting:seed-demo is additive and idempotent, and its data works with the app's workflows. */
class DemoFeaturesSeederTest extends TestCase
{
    use RefreshDatabase, SeedsBooks;

    private const TABLES = [
        'mst_account', 'trans_head', 'trans_detail', 'acc_journal_templates', 'acc_journal_template_lines',
        'acc_recurring_journals', 'acc_recurring_runs', 'acc_coa_templates', 'acc_coa_template_lines',
        'acc_period_reopen_requests', 'acc_audit_log',
    ];

    protected function setUp(): void
    {
        parent::setUp();
        $this->seedBooks(2026, 10);
        foreach (['0003' => 'Akuntan', '0004' => 'Manajer Keuangan'] as $id => $name) {
            User::query()->forceCreate(['user_id' => $id, 'user_name' => $name, 'email' => "u$id@uji.test", 'password' => bcrypt('x'), 'actived' => true]);
        }
    }

    private function counts(): array
    {
        return collect(self::TABLES)->mapWithKeys(fn ($t) => [$t => DB::table($t)->count()])->all();
    }

    public function test_seeding_twice_adds_nothing_the_second_time(): void
    {
        DB::table('mst_account')->where('acc_code', '1201')->update(['is_control' => false]);

        (new DemoFeaturesSeeder)->run();
        $first = $this->counts();
        $second = new DemoFeaturesSeeder;
        $second->run();

        $this->assertSame($first, $this->counts());
        $this->assertSame([], $second->created);
        $this->assertSame(1, DB::table('mst_account')->where('acc_code', '1201')->value('is_control') ? 1 : 0);
        $this->assertTrue((bool) DB::table('mst_account')->where('acc_code', '2301')->value('is_interunit'));
        // pre-existing account kept its name (insertOrIgnore)
        $this->assertSame('Piutang SPP', DB::table('mst_account')->where('acc_code', '1201')->value('acc_name'));
        $this->assertSame(1, DB::table('acc_journal_templates')->where('is_active', false)->count());
        $this->assertSame(1, DB::table('acc_period_reopen_requests')->where('status', 'pending')->count());
        $this->assertEqualsCanonicalizing(['posted', 'pending', 'draft'],
            DB::table('trans_head')->distinct()->pluck('status')->all());
        $this->assertTrue(DB::table('acc_audit_log')->where('event', 'journal.reject')->where('subject', 'DEMO-JU-004')->exists());
        $this->assertSame(0, DB::table('trans_head')->whereNotBetween('trans_date', ['2026-10-01', '2026-10-31'])->count());
    }

    public function test_pending_demo_journal_can_be_approved_by_another_user(): void
    {
        (new DemoFeaturesSeeder)->run();

        Journals::transition('DEMO-JU-002', 'approve', 'sekoci-admin');
        Journals::transition('DEMO-JU-003', 'submit', '0003');

        $this->assertSame('posted', DB::table('trans_head')->where('trans_no', 'DEMO-JU-002')->value('status'));
        $this->assertSame('pending', DB::table('trans_head')->where('trans_no', 'DEMO-JU-003')->value('status'));
    }
}
