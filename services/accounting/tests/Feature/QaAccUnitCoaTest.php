<?php

namespace Tests\Feature;

use App\Livewire\Pages\ChartOfAccount\ChartOfAccount;
use App\Livewire\Pages\Division\Division;
use App\Support\Accounts;
use App\Support\CoaTemplates;
use Illuminate\Foundation\Testing\RefreshDatabase;
use Illuminate\Support\Facades\Crypt;
use Illuminate\Support\Facades\DB;
use Illuminate\Validation\ValidationException;
use Livewire\Livewire;
use Tests\Concerns\ActsAsAccountingRoles;
use Tests\Concerns\SeedsBooks;
use Tests\TestCase;

/** QA BUG-141: COA template copied per unit; unit-scoped accounts; unit guard. */
class QaAccUnitCoaTest extends TestCase
{
    use ActsAsAccountingRoles, RefreshDatabase, SeedsBooks;

    protected function setUp(): void
    {
        parent::setUp();
        $this->seedBooks();
        $this->installRoles();
        $this->withoutMiddleware(\App\Http\Middleware\TenantSchema::class);
    }

    public function test_template_snapshot_and_copy_to_unit(): void
    {
        DB::table('mst_account')->where('acc_code', '1102')->update(['parent_code' => '1101']);
        $id = CoaTemplates::saveFromCoa('COA Sekolah', null, 'U1');
        $this->assertSame(7, DB::table('acc_coa_template_lines')->where('template_id', $id)->count());

        $r = CoaTemplates::copyToUnit($id, 'SMP', 'U1');
        $this->assertSame(['created' => 7, 'skipped' => 0], $r);
        $acc = DB::table('mst_account')->where('acc_code', '1102.SMP')->first();
        $this->assertSame('SMP', $acc->unit_scope);
        $this->assertSame('1101.SMP', $acc->parent_code);
        $this->assertSame(['created' => 0, 'skipped' => 7], CoaTemplates::copyToUnit($id, 'SMP', 'U1'));

        // A snapshot holds shared accounts only.
        $id2 = CoaTemplates::saveFromCoa('COA 2', null, 'U1');
        $this->assertSame(0, DB::table('acc_coa_template_lines')->where('template_id', $id2)->where('acc_code', 'like', '%.SMP')->count());
    }

    public function test_unit_account_refuses_other_unit_lines(): void
    {
        $id = CoaTemplates::saveFromCoa('COA Sekolah', null, 'U1');
        CoaTemplates::copyToUnit($id, 'SMP', 'U1');
        $this->assertSame([], Accounts::postingProblems('6101.SMP', true, 'SMP'));
        $this->assertNotEmpty(Accounts::postingProblems('6101.SMP', true, 'SD'));
    }

    public function test_copy_refuses_codes_longer_than_15(): void
    {
        DB::table('mst_account')->insert(['acc_code' => '123456789012345', 'acc_name' => 'Panjang', 'acc_group' => 1, 'normal_balance' => 'D', 'is_active' => true]);
        $id = CoaTemplates::saveFromCoa('COA', null, 'U1');
        $this->expectException(ValidationException::class);
        CoaTemplates::copyToUnit($id, 'SMP', 'U1');
    }

    public function test_coa_page_template_actions(): void
    {
        $this->actingAs($this->userAs('ACC', 'Accounting'));
        Livewire::test(ChartOfAccount::class)
            ->set('coaTpl.name', 'COA Induk')->call('saveTemplate')->assertHasNoErrors()
            ->set('coaCopy.template_id', (string) DB::table('acc_coa_templates')->value('id'))->set('coaCopy.div_code', 'SD')
            ->call('copyTemplate')->assertHasNoErrors()
            ->set('unit', 'SD')->assertSee('4101.SD');
        $this->assertTrue(DB::table('mst_account')->where('acc_code', '4101.SD')->where('unit_scope', 'SD')->exists());
    }

    public function test_auditor_cannot_use_coa_template_actions(): void
    {
        $this->actingAs($this->userAs('AUD', 'Auditor'));
        Livewire::test(ChartOfAccount::class)->set('coaTpl.name', 'X')->call('saveTemplate')->assertForbidden();
    }

    public function test_new_division_with_template_and_delete_guard(): void
    {
        $id = CoaTemplates::saveFromCoa('COA Sekolah', null, 'U1');
        $this->actingAs($this->userAs('ADM', 'Admin'));
        Livewire::test(Division::class)->call('add')
            ->set('division.div_code', 'SMA')->set('division.div_name', 'Unit SMA')->set('coaTemplate', (string) $id)
            ->call('save')->assertSet('error', null);
        $this->assertSame(7, DB::table('mst_account')->where('unit_scope', 'SMA')->count());

        Livewire::test(Division::class)->call('confirmDelete', Crypt::encrypt('SMA'))
            ->assertSet('error', fn ($e) => str_contains((string) $e, 'tidak dapat dihapus'));
        $this->assertTrue(DB::table('mst_division')->where('div_code', 'SMA')->exists());
    }

    public function test_division_actions_need_permission(): void
    {
        $this->actingAs($this->userAs('AUD', 'Auditor'));
        Livewire::test(Division::class)->call('add')->assertForbidden();
    }
}
