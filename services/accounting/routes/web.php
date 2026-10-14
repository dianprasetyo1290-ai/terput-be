<?php

use App\Http\Controllers\HelperController;
use App\Http\Controllers\SsoController;
use App\Http\Controllers\Report\GeneralLedgerController;
use App\Livewire\Component\FormBalance;
use App\Livewire\Pages\AccountGroup\AccountGroupPages;
use App\Livewire\Pages\AccountGroup\AccountSubGroupPages;
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
use App\Livewire\Pages\HistoryJournalValidation\HJV;
use App\Livewire\Pages\Home;
use App\Livewire\Pages\JournalTransaction\JT;
use App\Livewire\Pages\JournalValidation\JV;
use App\Livewire\Pages\Period\Period;
use App\Livewire\Pages\PriorYearAdjustment\PriorYearAdjustment;
use App\Livewire\Pages\Report\FinancialReport\FinancialReport;
use App\Livewire\Pages\Report\GeneralLedger\ReportGeneralLedger;
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
use App\Livewire\Pages\User\Profile;
use App\Livewire\Pages\User\User;
use App\Livewire\Report\ReportPrint;
use App\Livewire\Report\ReportRowPrint;
use Illuminate\Support\Facades\Route;

Route::get('/healthz', function () {
    return response()->json(['status' => 'ok', 'service' => 'accounting']);
});

Route::get('/', function () {
    return redirect('login');
});

// SSO dari Sekoci SPA: body {token} = JWT HS256. CSRF dikecualikan di bootstrap/app.php.
Route::post('/sso', [SsoController::class, 'login'])->name('sso');

Route::get('show-picture/{path}', [HelperController::class, 'showPicture'])->name('helper.show-picture')->where('path', '.*');
Route::middleware([
    'auth:sanctum',
    config('jetstream.auth_session'),
    'verified',
])->group(function () {
    Route::get('/home', Home::class)->name('home');
    Route::group(['middleware' => ['can:viewcompany_parameter']], function () {
        Route::get('/companyparameter', CompanyParameter::class);
    });
    Route::group(['middleware' => ['can:viewjournal_transaction']], function () {
        Route::get('/journaltransaction', JT::class);
    });
    Route::group(['middleware' => ['can:viewhistory_journal']], function () {
        Route::get('/historyjournaltransaction', HJT::class);
    });
    Route::group(['middleware' => ['can:viewdivision']], function () {
        Route::get('/division', Division::class);
    });
    Route::group(['middleware' => ['can:viewchart_of_account']], function () {
        Route::get('/chartofaccount', ChartOfAccount::class);
    });
    Route::group(['middleware' => ['can:viewcurrency_table']], function () {
        Route::get('/currency', CurrencyTable::class);
    });
    Route::group(['middleware' => ['can:viewtransaction_type']], function () {
        Route::get('/transaction', Transaction::class);
    });
    Route::group(['middleware' => ['can:viewperiode']], function () {
        Route::get('/periode', Period::class);
    });
    Route::group(['middleware' => ['can:viewend_of_month']], function () {
        Route::get('/endofmonth', EOM::class);
    });
    Route::group(['middleware' => ['can:viewend_of_year']], function () {
        Route::get('/endofyear', EOY::class);
    });
    Route::group(['middleware' => ['can:viewprior_year_adjustment']], function () {
        Route::get('/prior-year-adjustment', PriorYearAdjustment::class)->name('prior-year-adjustment');
    });
    Route::group(['middleware' => ['can:viewjournal_validation']], function () {
        Route::get('/journalvalidation', JV::class);
    });
    Route::group(['middleware' => ['can:viewaccount_group_table']], function () {
        Route::get('/agt', AccountGroupPages::class);
    });
    Route::group(['middleware' => ['can:viewsub_account_group_table']], function () {
        Route::get('/subagt', AccountSubGroupPages::class);
    });
    Route::group(['middleware' => ['can:viewhistory_journal_validation']], function () {
        Route::get('/historyjournalvalidation', HJV::class);
    });
    Route::group(['middleware' => ['can:viewbalance']], function () {
        Route::get('/balance', Balance::class)->name('balance');
    });
    Route::group(['middleware' => ['can:viewbalance_currency']], function () {
        Route::get('/balance-currency', BalanceCurrency::class)->name('balance-currency');
    });
    Route::group(['middleware' => ['can:viewbudgeting']], function () {
        Route::get('/budgeting', Budgeting::class)->name('budgeting');
    });
    Route::group(['middleware' => ['can:viewusers']], function () {
        Route::get('/users', User::class)->name('users');
    });
    Route::group(['middleware' => ['can:viewrole']], function () {
        Route::get('/role', Role::class)->name('role');
    });
    Route::group(['middleware' => ['can:viewpermission']], function () {
        Route::get('/permissions', Permissions::class)->name('permissions');
    });
    Route::group(['middleware' => ['can:viewsetting_website']], function () {
        Route::get('/settingwebsite', SettingWeb::class)->name('settingwebsite');
    });
    Route::get('/profile', Profile::class)->name('profile');
    Route::group(['middleware' => ['can:viewgeneral_ledger']], function () {
        Route::get('/reportgeneralledger', ReportGeneralLedger::class)->name('reportgeneralledger');
    });
    // Every page below used to be reachable by any logged-in role (QA BUG-061).
    Route::group(['middleware' => ['can:viewgeneral_ledger']], function () {
        Route::get('/z', [GeneralLedgerController::class, 'report']);
        Route::get('/summarygeneralledger', [GeneralLedgerController::class, 'reportsummary']);
    });

    Route::group(['prefix' => 'generator', 'as' => 'generator.'], function () {
        Route::group(['middleware' => ['can:viewreport_row_design']], function () {
            Route::get('row-generator', ReportRowHeadCrud::class)->name('row');
            Route::get('row-generator/detail/{id?}', ReportRowDetails::class)->name('row-detail');
            Route::get('report/row', ReportRowPrint::class)->name('report-preview');
        });
        Route::group(['middleware' => ['can:viewfinal_design']], function () {
            // Column Generator
            Route::get('col-generator', ReportColHeadCrud::class)->name('col');
            Route::get('col-generator/detail/{id?}', ReportColDetailCrud::class)->name('col-detail');
            // Final Generator
            Route::get('report-final', ReportFinalCrud::class)->name('viewfinal_design');
            Route::get('report-final-detail/{id?}', ReportFinalDetail::class)->name('viewfinal_design_detail');
        });
    });

    Route::group(['prefix' => 'report', 'as' => 'report.'], function () {
        Route::group(['middleware' => ['can:viewfinancial_report']], function () {
            Route::get('financial', FinancialReport::class)->name('financial');
        });
    });

    Route::group(['middleware' => ['can:viewfinancial_report']], function () {
        Route::get('report/{id?}', ReportPrint::class)->name('report');
    });
    Route::group(['middleware' => ['can:viewbalance']], function () {
        Route::get('/form-balance', FormBalance::class)->name('form.balance');
    });
});
