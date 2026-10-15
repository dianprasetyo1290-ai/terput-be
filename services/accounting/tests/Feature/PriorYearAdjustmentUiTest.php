<?php

namespace Tests\Feature;

use App\Livewire\Pages\PriorYearAdjustment\PriorYearAdjustment;
use App\Services\PriorYearAdjustmentService;
use Illuminate\Foundation\Testing\RefreshDatabase;
use Illuminate\Support\Facades\DB;
use Livewire\Livewire;
use Tests\TestCase;

class PriorYearAdjustmentUiTest extends TestCase
{
    use RefreshDatabase;

    public function test_prior_year_adjustment_route_and_sidebar_are_registered(): void
    {
        $routes = file_get_contents(base_path('routes/web.php'));
        $sidebar = file_get_contents(resource_path('views/layouts/partials/sidebar.blade.php'));

        $this->assertStringContainsString('/prior-year-adjustment', $routes);
        $this->assertStringContainsString('Prior Year Adjustment', $sidebar);
        $this->assertStringContainsString('viewprior_year_adjustment', $sidebar);
    }

    public function test_prior_year_adjustment_view_has_draft_line_and_finalize_actions(): void
    {
        $view = file_get_contents(resource_path('views/livewire/pages/prior-year-adjustment/prior-year-adjustment.blade.php'));

        $this->assertStringContainsString('wire:click="createDraft"', $view);
        $this->assertStringContainsString('wire:submit.prevent="addLine"', $view);
        $this->assertStringContainsString('wire:click="finalizeAdjustment"', $view);
        $this->assertStringContainsString('wire:click="openCreateForm"', $view);
        $this->assertStringContainsString('wire:click="confirmFinalizeAdjustment"', $view);
        $this->assertStringContainsString('Finalize Prior Year Adjustment', $view);
        $this->assertStringContainsString('wire:click="confirmDeleteLine(', $view);
        $this->assertStringContainsString('Delete Adjustment Line', $view);
        $this->assertStringContainsString('wire:click="confirmDeleteDraft(', $view);
        $this->assertStringContainsString('Delete Draft Adjustment', $view);
        $this->assertStringContainsString('wire:model.live="listFilterYear"', $view);
        $this->assertStringContainsString('wire:model.live.debounce.300ms="search"', $view);
        $this->assertStringContainsString('Total Debit', $view);
        $this->assertStringContainsString('Total Credit', $view);
        $this->assertStringNotContainsString('wire:confirm=', $view);
        $this->assertStringContainsString('Adjustment List', $view);
        $this->assertStringContainsString('Terbilang', $view);
        $this->assertStringContainsString('wire:loading.attr="disabled"', $view);
        $this->assertStringContainsString('wire:target="confirmFinalizeAdjustment,finalizeAdjustment"', $view);
        $this->assertStringContainsString('Finalizing...', $view);
    }

    public function test_finalize_confirmation_modal_opens_for_selected_draft(): void
    {
        $this->seedPriorYearAdjustmentUiReferenceData();

        $service = app(PriorYearAdjustmentService::class);
        $session = $service->createSession([
            'target_year' => 2025,
            'adjustment_date' => '2025-12-31',
            'description' => 'Late audit adjustment',
        ]);
        $service->addLine($session, [
            'acc_code' => '1000',
            'div_code' => '0000',
            'description' => 'Debit line',
            'debet' => 100,
            'credit' => 0,
            'curr_code' => 'Rp',
        ]);
        $service->addLine($session, [
            'acc_code' => '4000',
            'div_code' => '0000',
            'description' => 'Credit line',
            'debet' => 0,
            'credit' => 100,
            'curr_code' => 'Rp',
        ]);

        $this->actingAsSuperAdmin();
        Livewire::test(PriorYearAdjustment::class)
            ->call('selectSession', $session->id)
            ->call('confirmFinalizeAdjustment')
            ->assertSet('showFinalizeModal', true)
            ->assertSee('Finalize Prior Year Adjustment');
    }

    public function test_finalize_confirmation_shows_error_when_draft_has_no_lines(): void
    {
        $this->seedPriorYearAdjustmentUiReferenceData();

        $session = app(PriorYearAdjustmentService::class)->createSession([
            'target_year' => 2025,
            'adjustment_date' => '2025-12-31',
            'description' => 'Late audit adjustment',
        ]);

        $this->actingAsSuperAdmin();
        Livewire::test(PriorYearAdjustment::class)
            ->call('selectSession', $session->id)
            ->call('confirmFinalizeAdjustment')
            ->assertSet('showFinalizeModal', false)
            ->assertHasErrors('lines');
    }

    public function test_draft_adjustment_can_be_deleted_with_confirmation(): void
    {
        $this->seedPriorYearAdjustmentUiReferenceData();

        $session = app(PriorYearAdjustmentService::class)->createSession([
            'target_year' => 2025,
            'adjustment_date' => '2025-12-31',
            'description' => 'Draft to delete',
        ]);

        app(PriorYearAdjustmentService::class)->addLine($session, [
            'acc_code' => '1000',
            'div_code' => '0000',
            'description' => 'Temporary line',
            'debet' => 100,
            'credit' => 0,
            'curr_code' => 'Rp',
        ]);

        $this->actingAsSuperAdmin();
        Livewire::test(PriorYearAdjustment::class)
            ->call('selectSession', $session->id)
            ->call('confirmDeleteDraft', $session->id)
            ->assertSet('showDeleteDraftModal', true)
            ->call('deleteDraft')
            ->assertSet('showDeleteDraftModal', false)
            ->assertSet('sessionId', null)
            ->assertSet('mode', 'list')
            ->assertSee('Draft adjustment dihapus.');

        $this->assertDatabaseMissing('prior_year_adjustment_sessions', ['id' => $session->id]);
        $this->assertDatabaseMissing('prior_year_adjustment_lines', ['session_id' => $session->id]);
    }

    public function test_adjustment_list_can_be_filtered_by_year_and_search_and_shows_totals(): void
    {
        $this->seedPriorYearAdjustmentUiReferenceData();

        DB::table('mst_saldo_news')->insert([
            'acc_code' => '1000',
            'div_code' => '0000',
            'periode' => 0,
            'tahun' => 2024,
            'debet' => 0,
            'credit' => 0,
            'created_at' => now(),
            'updated_at' => now(),
        ]);

        $service = app(PriorYearAdjustmentService::class);
        $auditSession = $service->createSession([
            'target_year' => 2025,
            'adjustment_date' => '2025-12-31',
            'description' => 'Audit revenue correction',
        ]);
        $service->addLine($auditSession, [
            'acc_code' => '1000',
            'div_code' => '0000',
            'description' => 'Debit line',
            'debet' => 100,
            'credit' => 0,
            'curr_code' => 'Rp',
        ]);
        $service->addLine($auditSession, [
            'acc_code' => '4000',
            'div_code' => '0000',
            'description' => 'Credit line',
            'debet' => 0,
            'credit' => 100,
            'curr_code' => 'Rp',
        ]);

        $otherSession = $service->createSession([
            'target_year' => 2024,
            'adjustment_date' => '2024-12-31',
            'description' => 'Other correction',
        ]);
        $service->addLine($otherSession, [
            'acc_code' => '1000',
            'div_code' => '0000',
            'description' => 'Other debit',
            'debet' => 50,
            'credit' => 0,
            'curr_code' => 'Rp',
        ]);

        $this->actingAsSuperAdmin();
        Livewire::test(PriorYearAdjustment::class)
            ->set('listFilterYear', '2025')
            ->set('search', 'audit')
            ->assertSee('Audit revenue correction')
            ->assertDontSee('Other correction')
            ->assertSee('100,00');
    }

    private function seedPriorYearAdjustmentUiReferenceData(): void
    {
        DB::table('parameter')->insert([
            'serial_no' => 'EXCO',
            'company' => 'EXCO',
            'period' => 5,
            'year' => 2026,
            'base_currency' => 'Rp',
            'created_at' => now(),
            'updated_at' => now(),
        ]);

        DB::table('mst_currency')->insert([
            'curr_code' => 'Rp',
            'curr_name' => 'Rupiah',
            'rate_system' => 'N',
            'created_at' => now(),
            'updated_at' => now(),
        ]);

        DB::table('mst_division')->insert([
            'div_code' => '0000',
            'div_name' => 'Head Office',
            'created_at' => now(),
            'updated_at' => now(),
        ]);

        DB::table('mst_transaction')->insert([
            'trans_code' => 'PYA',
            'trans_name' => 'Prior Year Adjustment',
            'trans_status' => '1',
            'created_at' => now(),
            'updated_at' => now(),
        ]);

        DB::table('mst_account')->insert([
            [
                'acc_code' => '1000',
                'acc_name' => 'Cash',
                'acc_group' => 1,
                'acc_currency' => 'Rp',
                'created_at' => now(),
                'updated_at' => now(),
            ],
            [
                'acc_code' => '4000',
                'acc_name' => 'Revenue',
                'acc_group' => 4,
                'acc_currency' => 'Rp',
                'created_at' => now(),
                'updated_at' => now(),
            ],
        ]);

        DB::table('mst_saldo_news')->insert([
            'acc_code' => '1000',
            'div_code' => '0000',
            'periode' => 0,
            'tahun' => 2025,
            'debet' => 0,
            'credit' => 0,
            'created_at' => now(),
            'updated_at' => now(),
        ]);
    }

    /** The central Livewire guard (QA round 2) needs a signed-in user. */
    private function actingAsSuperAdmin(): void
    {
        \Spatie\Permission\Models\Role::findOrCreate('SuperAdmin', 'web');
        $user = \App\Models\User::query()->forceCreate([
            'user_id' => 'SA'.substr(uniqid(), -6), 'user_name' => 'Super Admin', 'email' => uniqid().'@uji.test', 'password' => bcrypt('secret'),
        ]);
        $user->assignRole('SuperAdmin');
        $this->actingAs($user);
    }
}
