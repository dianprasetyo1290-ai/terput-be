<?php

namespace App\Support;

use App\Livewire\Component\FormBalance;
use App\Livewire\Pages\AccountGroup\AccountGroupPages;
use App\Livewire\Pages\AccountGroup\AccountSubGroupPages;
use App\Livewire\Pages\AutoPosting\AutoPosting;
use App\Livewire\Pages\Balance\Balance;
use App\Livewire\Pages\BalanceCurrency\BalanceCurrency;
use App\Livewire\Pages\Budgeting\Budgeting;
use App\Livewire\Pages\ChartOfAccount\ChartOfAccount;
use App\Livewire\Pages\CompanyParameter\CompanyParameter;
use App\Livewire\Pages\Currency\CurrencyTable;
use App\Livewire\Pages\Division\Division;
use App\Livewire\Pages\EndOfMonth\EOM;
use App\Livewire\Pages\EndOfYear\EOY;
use App\Livewire\Pages\HistoryJournalTransaction\HJT;
use App\Livewire\Pages\Import\Import;
use App\Livewire\Pages\JournalTemplate\JournalTemplate;
use App\Livewire\Pages\JournalTransaction\JT;
use App\Livewire\Pages\JournalValidation\JV;
use App\Livewire\Pages\Period\Period;
use App\Livewire\Pages\PeriodReopen\PeriodReopen;
use App\Livewire\Pages\PriorYearAdjustment\PriorYearAdjustment;
use App\Livewire\Pages\Report\FinancialReport\FinancialReport;
use App\Livewire\Pages\Report\Generator\ReportColDetailCrud;
use App\Livewire\Pages\Report\Generator\ReportColHeadCrud;
use App\Livewire\Pages\Report\Generator\ReportFinalCrud;
use App\Livewire\Pages\Report\Generator\ReportFinalDetail;
use App\Livewire\Pages\Report\Generator\ReportRowDetails;
use App\Livewire\Pages\Report\Generator\ReportRowHeadCrud;
use App\Livewire\Pages\RoleAndPermission\Permissions;
use App\Livewire\Pages\RoleAndPermission\Role;
use App\Livewire\Pages\SettingWeb\SettingWeb;
use App\Livewire\Pages\TransactionType\Transaction;
use App\Livewire\Pages\User\User;
use App\Livewire\Transaksi;
use Illuminate\Auth\Access\AuthorizationException;
use Illuminate\Support\Facades\Gate;

/**
 * One permission check for every Livewire action (QA round 2: "tidak boleh
 * ada tombol tanpa izin"). Hooked on Livewire's "call" event in
 * AppServiceProvider, so it runs before ANY public method a browser asks for
 * -- including methods a page forgot to authorize() itself.
 *
 * For a component listed in MODULES, a method is
 *   - a write  (WRITE)  -> needs create<module> or edit<module>;
 *   - a delete (DELETE) -> needs delete<module>;
 *   - anything else     -> needs view<module> (paging, sorting, opening).
 * Components with finer rules (journal workflow, reopen approval, …) still
 * call authorize() themselves; this is the floor, not the only check.
 */
final class LivewireGuard
{
    /** component => permission module */
    public const MODULES = [
        AccountGroupPages::class => 'account_group_table',
        AccountSubGroupPages::class => 'sub_account_group_table',
        AutoPosting::class => 'auto_posting',
        Balance::class => 'balance',
        FormBalance::class => 'balance',
        BalanceCurrency::class => 'balance_currency',
        Budgeting::class => 'budgeting',
        ChartOfAccount::class => 'chart_of_account',
        CompanyParameter::class => 'company_parameter',
        CurrencyTable::class => 'currency_table',
        Division::class => 'division',
        EOM::class => 'end_of_month',
        EOY::class => 'end_of_year',
        HJT::class => 'history_journal',
        Import::class => 'import_data',
        JournalTemplate::class => 'journal_template',
        JT::class => 'journal_transaction',
        JV::class => 'journal_validation',
        Period::class => 'periode',
        PeriodReopen::class => 'period_reopen',
        PriorYearAdjustment::class => 'prior_year_adjustment',
        FinancialReport::class => 'financial_report',
        ReportColDetailCrud::class => 'final_design',
        ReportColHeadCrud::class => 'final_design',
        ReportFinalCrud::class => 'final_design',
        ReportFinalDetail::class => 'final_design',
        ReportRowDetails::class => 'report_row_design',
        ReportRowHeadCrud::class => 'report_row_design',
        Permissions::class => 'permission',
        Role::class => 'role',
        SettingWeb::class => 'setting_website',
        Transaction::class => 'transaction_type',
        User::class => 'users',
        Transaksi::class => 'journal_transaction',
    ];

    /**
     * Methods that change data (or open a form to change it). Pages whose
     * workflow actions use another module's permission (JV approve, JT
     * submit, journal templates -> draft, reopen) authorize() those
     * themselves and are left out here by name.
     */
    public const WRITE = [
        'add', 'create', 'edit', 'save', 'store', 'simpan', 'btnedit', 'abc', 'moveUp', 'moveDown',
        'openCreateForm', 'createDraft', 'addLine', 'finalizeAdjustment', 'confirmFinalizeAdjustment',
        'suspend', 'confirm', 'transfer', 'confirmTransfer', 'updateSaldoFromTransactions', 'incrementPeriod',
        'generate', 'confirmGenerate', 'saveDetail', 'detail', 'saveTemplate', 'copyTemplate', 'import', 'preview',
        'addDetailRow', 'removeDetailRow', 'saveTransaction', 'newTemplate', 'editTemplate', 'copyFromJournal',
        'toggleTemplate', 'newSchedule', 'saveSchedule', 'toggleSchedule', 'runNow', 'retryRun', 'skipRun',
        'activate', 'requestReopen', 'cancelRequest', 'signOff',
    ];

    public const DELETE = [
        'delete', 'deleteD', 'confirmDelete', 'confirmDeleteDetail', 'deleteLine', 'confirmDeleteLine',
        'deleteDraft', 'confirmDeleteDraft', 'deactivate', 'deleteTemplate', 'deleteSchedule',
    ];

    /**
     * Per component, methods whose own authorize() is the rule (they need a
     * permission of ANOTHER module, e.g. JT's draft submit is
     * editjournal_transaction, JV's approve editjournal_validation, a draft
     * from a template createjournal_transaction, the reopen approval
     * editperiod_reopen) -- the guard only requires view<module> for them.
     */
    public const OWN_RULE = [
        JournalTemplate::class => ['askDraft', 'createDraft'],
        PeriodReopen::class => ['approve', 'reject'],
        // Journal pages: create/edit/delete/workflow authorize per action
        // (WithTransactionHeader/Detail, WithJournalWorkflow).
        JT::class => ['*'],
        // Opening a closed journal's lines is reading (viewhistory_journal_detail).
        HJT::class => ['detail'],
        JV::class => ['*'],
    ];

    /** The permission a call needs, or null when the component is not guarded. */
    public static function permissionFor(string $class, string $method): ?array
    {
        $module = self::MODULES[$class] ?? null;
        if ($module === null) {
            return null;
        }
        if (str_starts_with($method, '_')) {
            // Livewire internals (_startUpload, _finishUpload, __dispatch …):
            // they change no books; the action that uses them is checked.
            return null;
        }
        $own = self::OWN_RULE[$class] ?? [];
        if (in_array('*', $own, true) || in_array($method, $own, true)) {
            return ["view$module"];
        }
        if (in_array($method, self::DELETE, true)) {
            return ["delete$module"];
        }
        if (in_array($method, self::WRITE, true)) {
            return ["create$module", "edit$module"];
        }

        return ["view$module"];
    }

    /** @throws AuthorizationException */
    public static function check(object $component, string $method): void
    {
        $need = self::permissionFor($component::class, $method);
        if ($need === null) {
            return;
        }
        if (! Gate::any($need)) {
            throw new AuthorizationException('Anda tidak memiliki izin untuk aksi ini.');
        }
    }
}
