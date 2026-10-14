<div class="sidebar sidebar-dark sidebar-main sidebar-expand-md">

    <!-- Sidebar mobile toggler -->
    <div class="sidebar-mobile-toggler text-center">
        <a href="#" class="sidebar-mobile-main-toggle">
            <i class="icon-arrow-left8"></i>
        </a>
        Navigation
        <a href="#" class="sidebar-mobile-expand">
            <i class="icon-screen-full"></i>
            <i class="icon-screen-normal"></i>
        </a>
    </div>
    <!-- /sidebar mobile toggler -->


    <!-- Sidebar content -->
    <div class="sidebar-content">

        <!-- User menu -->
        <div class="sidebar-user" style="padding: 0; margin: 0;">
            <div class="card-body" style="padding: 1rem 1.25rem;">
                <div class="media" style="align-items: center;">
                    <div class="mr-3">
                        <a href="{{ route('profile') }}">
                            <img src="https://ui-avatars.com/api/?name={{ auth()->user()->user_name ?? 'null' }}&background=3b82f6&color=fff&size=38"
                                width="38" height="38" class="rounded-circle" alt="">
                        </a>
                    </div>

                    <div class="media-body" style="min-width: 0;">
                        <div class="media-title font-weight-semibold" style="font-size: 0.875rem; margin-bottom: 2px;">{{ Auth::user()->user_name }}</div>
                        <div style="font-size: 0.7rem;">
                            {{ Auth::user()->getRoleNames()->first() }}
                        </div>
                    </div>

                    <div class="ml-3 align-self-center">
                        <a href="{{ route('profile') }}" class="text-white" style="opacity: 0.6; transition: opacity 0.2s;">
                            <i class="icon-cog3"></i>
                        </a>
                    </div>
                </div>
            </div>
        </div>
        <!-- /user menu -->


        <!-- Main navigation -->
        <div class="card card-sidebar-mobile">
            <ul class="nav nav-sidebar" data-nav-type="accordion">

                <!-- Main -->
                <li class="nav-item-header">
                    <div class="text-uppercase font-size-xs line-height-xs">Main</div> <i class="icon-menu"
                        title="Main"></i>
                </li>
                <li class="nav-item">
                    <a href="{{ url('/home') }}" class="nav-link {{ Request::is('home') ? 'active' : '' }}">
                        <i class="icon-home4"></i>
                        <span>
                            Dashboard
                        </span>
                    </a>
                </li>
                <!-- Transaction -->
                @canany(['viewjournal_transaction', 'viewjournal_validation', 'viewend_of_month', 'viewhistory_journal',
                    'viewhistory_journal_validation', 'viewend_of_year', 'viewprior_year_adjustment', 'viewauto_posting', 'viewjournal_template', 'viewperiod_reopen'])
                    <li class="nav-item-header">
                        <div class="text-uppercase font-size-xs line-height-xs">Transaction</div> <i class="icon-menu"
                            title="Forms"></i>
                    </li>
                    <li
                        class="nav-item nav-item-submenu {{ Request::is(['historyjournalvalidation', 'endofyear', 'prior-year-adjustment', 'journalvalidation', 'endofmonth', 'journaltransaction', 'historyjournaltransaction', 'autoposting', 'journaltemplate', 'period-reopen']) ? 'nav-item-expanded nav-item-open' : '' }}">
                        <a href="#" class="nav-link"><i class="icon-book"></i> <span>Transaction</span></a>
                        <ul class="nav nav-group-sub" data-submenu-title="User pages">
                            @canany('viewjournal_transaction')
                                <li class="nav-item"><a href="{{ url('journaltransaction') }}"
                                        class="nav-link {{ Request::is(['journaltransaction']) ? 'active' : '' }}">Journal
                                        Transaction</a>
                                </li>
                            @endcanany
                            @canany('viewjournal_validation')
                                <li class="nav-item"><a href="{{ url('journalvalidation') }}"
                                        class="nav-link {{ Request::is(['journalvalidation']) ? 'active' : '' }}">Approval Jurnal</a>
                                </li>
                            @endcanany
                            @canany('viewjournal_template')
                                <li class="nav-item"><a href="{{ url('journaltemplate') }}"
                                        class="nav-link {{ Request::is(['journaltemplate']) ? 'active' : '' }}">Template &amp; Jurnal Berulang</a>
                                </li>
                            @endcanany
                            @canany('viewauto_posting')
                                <li class="nav-item"><a href="{{ url('autoposting') }}"
                                        class="nav-link {{ Request::is(['autoposting']) ? 'active' : '' }}">Posting Otomatis</a>
                                </li>
                            @endcanany
                            @canany('viewend_of_month')
                                <li class="nav-item"><a href="{{ url('endofmonth') }}"
                                        class="nav-link {{ Request::is(['endofmonth']) ? 'active' : '' }}">End of Month</a>
                                </li>
                            @endcanany
                            @canany('viewperiod_reopen')
                                <li class="nav-item"><a href="{{ url('period-reopen') }}"
                                        class="nav-link {{ Request::is(['period-reopen']) ? 'active' : '' }}">Buka Kembali Periode</a>
                                </li>
                            @endcanany
                            @canany('viewhistory_journal')
                                <li class="nav-item"><a href="{{ url('historyjournaltransaction') }}"
                                        class="nav-link {{ Request::is(['historyjournaltransaction']) ? 'active' : '' }}">History
                                        Journal</a></li>
                            @endcanany
                            @canany('viewhistory_journal_validation')
                                <!-- <li class="nav-item"><a href="{{ url('historyjournalvalidation') }}"
                                                                                                                class="nav-link {{ Request::is(['historyjournalvalidation']) ? 'active' : '' }}">History
                                                                                                                Journal
                                                                                                                Validation</a></li> -->
                            @endcanany
                            @canany('viewend_of_year')
                                <li class="nav-item"><a href="{{ url('endofyear') }}"
                                        class="nav-link {{ Request::is(['endofyear']) ? 'active' : '' }}">End of Year</a>
                                </li>
                            @endcanany
                            @canany('viewprior_year_adjustment')
                                <li class="nav-item"><a href="{{ url('prior-year-adjustment') }}"
                                        class="nav-link {{ Request::is(['prior-year-adjustment']) ? 'active' : '' }}">Prior Year Adjustment</a>
                                </li>
                            @endcanany
                        </ul>
                    </li>
                @endcanany
                <!-- /Transaction -->

                <!-- Report -->
                @canany(['viewgeneral_ledger', 'viewfinancial_report', 'viewtrial_balance', 'viewconsolidation'])
                    <li class="nav-item-header">
                        <div class="text-uppercase font-size-xs line-height-xs">Report</div> <i class="icon-menu"
                            title="Components"></i>
                    </li>
                    <li
                        class="nav-item nav-item-submenu {{ Request::is(['reportgeneralledger', 'report/financial', 'report/trial-balance', 'report/statements', 'report/consolidation']) ? 'nav-item-expanded nav-item-open' : '' }}">
                        <a href="#" class="nav-link"><i class="icon-file-presentation"></i> <span>Report</span></a>
                        <ul class="nav nav-group-sub" data-submenu-title="User pages">
                            @canany('viewgeneral_ledger')
                                <li class="nav-item"><a href="{{ url('reportgeneralledger') }}"
                                        class="nav-link {{ Request::is('reportgeneralledger') ? 'active' : '' }}">General
                                        Ledger</a>
                                </li>
                            @endcanany
                            @canany('viewtrial_balance')
                                <li class="nav-item"><a href="{{ route('report.trial-balance') }}"
                                        class="nav-link {{ Request::is('report/trial-balance') ? 'active' : '' }}">Neraca Saldo</a>
                                </li>
                            @endcanany
                            @canany('viewfinancial_report')
                                <li class="nav-item"><a href="{{ route('report.statements') }}"
                                        class="nav-link {{ Request::is('report/statements') ? 'active' : '' }}">Laporan Keuangan</a>
                                </li>
                            @endcanany
                            @canany('viewconsolidation')
                                <li class="nav-item"><a href="{{ route('report.consolidation') }}"
                                        class="nav-link {{ Request::is('report/consolidation') ? 'active' : '' }}">Konsolidasi</a>
                                </li>
                            @endcanany
                            @canany('viewfinancial_report')
                                <li class="nav-item"><a href="{{ route('report.financial') }}"
                                        class="nav-link {{ Request::is('report/financial') ? 'active' : '' }}">Financial
                                        Report</a>
                                </li>
                            @endcanany
                        </ul>
                    </li>
                @endcanany
                <!-- /Report -->

                <!-- Report Generator -->
                @canany(['viewreport_row_design', 'viewfinal_design'])
                    <li class="nav-item-header">
                        <div class="text-uppercase font-size-xs line-height-xs">Report Generator</div> <i class="icon-menu"
                            title="Layout options"></i>
                    </li>
                    <li
                        class="nav-item nav-item-submenu {{ Request::is(['generator/report-final', 'generator/report-final-detail/*', 'generator/row-generator', 'generator/row-generator/detail/*', 'generator/col-generator', 'generator/col-generator/detail/*']) ? 'nav-item-expanded nav-item-open' : '' }}">
                        <a href="#" class="nav-link"><i class="icon-file-presentation2"></i> <span>Report
                                Generator</span></a>
                        <ul class="nav nav-group-sub" data-submenu-title="User pages">
                            @canany('viewreport_row_design')
                                <li class="nav-item"><a href="{{ route('generator.row') }}"
                                        class="nav-link {{ Request::is(['generator/row-generator', 'generator/row-generator/detail/*']) ? 'active' : '' }}">Report
                                        Row Design</a>
                                </li>
                            @endcanany
                            @canany('viewfinal_design')
                                <li class="nav-item"><a href="{{ route('generator.col') }}"
                                        class="nav-link {{ Request::is(['generator/col-generator', 'generator/col-generator/detail/*']) ? 'active' : '' }}">Column
                                        Template Design</a>
                                </li>
                            @endcanany
                            @canany('viewfinal_design')
                                <li class="nav-item"><a href="{{ route('generator.viewfinal_design') }}"
                                        class="nav-link  {{ Request::is(['generator/report-final', 'generator/report-final-detail/*']) ? 'active' : '' }}">Final
                                        Design</a>
                                </li>
                            @endcanany
                        </ul>
                    </li>
                @endcanany
                <!-- /Report Generator -->

                @canany(['viewbalance', 'viewbalance_currency', 'viewbudgeting', 'viewchart_of_account',
                    'viewcompany_parameter', 'viewcurrency_table', 'viewdivision', 'viewperiode', 'viewtransaction_type',
                    'viewusers', 'viewrole', 'viewpermission', 'viewsetting_website', 'viewaccount_group_table',
                    'viewsub_account_group_table'])
                    <!-- Master File -->
                    <li class="nav-item-header">
                        <div class="text-uppercase font-size-xs line-height-xs">Master File</div>
                        <i class="icon-menu" title="Page kits"></i>
                    </li>
                    <li
                        class="nav-item nav-item-submenu {{ Request::is([
                            'users',
                            'role',
                            'permissions',
                            'settingwebsite',
                            'companyparameter',
                            'division',
                            'chartofaccount',
                            'currency',
                            'transaction',
                            'budgeting',
                            'periode',
                            'balance',
                            'balance-currency',
                            'agt',
                            'subagt',
                        ])
                            ? 'nav-item-expanded nav-item-open'
                            : '' }}">
                        <a href="#" class="nav-link"><i class="icon-stack"></i> <span>Master File</span></a>
                        <ul class="nav nav-group-sub" data-submenu-title="Starter kit">

                            @canany('viewbalance')
                                <li class="nav-item">
                                    <a href="{{ url('balance') }}"
                                        class="nav-link {{ Request::is('balance') ? 'active' : '' }}">Balance</a>
                                </li>
                            @endcanany

                            @canany('viewbalance_currency')
                                <!-- <li class="nav-item">
                                                                                                            <a href="{{ url('balance-currency') }}"
                                                                                                                class="nav-link {{ Request::is('balance-currency') ? 'active' : '' }}">Balance (Foreign
                                                                                                                Currency)</a>
                                                                                                        </li> -->
                            @endcanany

                            @canany('viewbudgeting')
                                <!-- <li class="nav-item">
                                                                                            <a href="{{ route('budgeting') }}" class="nav-link {{ Request::is('budgeting') ? 'active' : '' }}">Budgeting</a>
                                                                                        </li> -->
                            @endcanany

                            @canany('viewchart_of_account')
                                <li class="nav-item">
                                    <a href="{{ url('chartofaccount') }}"
                                        class="nav-link {{ Request::is('chartofaccount') ? 'active' : '' }}">Chart Of
                                        Account</a>
                                </li>
                            @endcanany

                            @canany('viewimport_data')
                                <li class="nav-item">
                                    <a href="{{ url('import') }}"
                                        class="nav-link {{ Request::is('import') ? 'active' : '' }}">Import COA / Saldo Awal</a>
                                </li>
                            @endcanany

                            @canany('viewcompany_parameter')
                                <li class="nav-item">
                                    <a href="{{ url('companyparameter') }}"
                                        class="nav-link {{ Request::is('companyparameter') ? 'active' : '' }}">Company
                                        Parameter</a>
                                </li>
                            @endcanany

                            @canany('viewcurrency_table')
                                <li class="nav-item">
                                    <a href="{{ url('currency') }}"
                                        class="nav-link {{ Request::is('currency') ? 'active' : '' }}">Currency
                                        Table</a>
                                </li>
                            @endcanany

                            @canany('viewdivision')
                                <li class="nav-item">
                                    <a href="{{ url('division') }}"
                                        class="nav-link {{ Request::is('division') ? 'active' : '' }}">Division</a>
                                </li>
                            @endcanany

                            @canany('viewperiode')
                                <li class="nav-item">
                                    <a href="{{ url('periode') }}"
                                        class="nav-link {{ Request::is('periode') ? 'active' : '' }}">Periode</a>
                                </li>
                            @endcanany

                            @canany('viewtransaction_type')
                                <li class="nav-item">
                                    <a href="{{ url('transaction') }}"
                                        class="nav-link {{ Request::is('transaction') ? 'active' : '' }}">Transaction Type</a>
                                </li>
                            @endcanany

                            @canany(['viewusers', 'viewrole', 'viewpermission'])
                                <li
                                    class="nav-item nav-item-submenu {{ Request::is(['users', 'role', 'permissions']) ? 'nav-item-expanded nav-item-open' : '' }}">
                                    <a href="#" class="nav-link">User Management</a>
                                    <ul class="nav nav-group-sub">
                                        @canany('viewusers')
                                            <li class="nav-item">
                                                <a href="{{ url('users') }}"
                                                    class="nav-link {{ Request::is('users') ? 'active' : '' }}">User</a>
                                            </li>
                                        @endcanany

                                        @canany(['viewrole', 'viewpermission'])
                                            <li class="nav-item">
                                                <a href="{{ url('role') }}"
                                                    class="nav-link {{ Request::is(['role', 'permissions']) ? 'active' : '' }}">Role
                                                    Permission</a>
                                            </li>
                                        @endcanany
                                    </ul>
                                </li>
                            @endcanany

                            @canany('viewsetting_website')
                                <li class="nav-item">
                                    <a href="{{ url('settingwebsite') }}"
                                        class="nav-link {{ Request::is('settingwebsite') ? 'active' : '' }}">Website
                                        Setting</a>
                                </li>
                            @endcanany
                            @canany('viewaccount_group_table')
                                <!-- Extra Links -->
                                <li class="nav-item">
                                    <a href="{{ url('agt') }}"
                                        class="nav-link {{ Request::is('agt') ? 'active' : '' }}">Account
                                        Group Table</a>
                                </li>
                            @endcanany
                            @canany('viewsub_account_group_table')
                                <li class="nav-item">
                                    <a href="{{ url('subagt') }}"
                                        class="nav-link {{ Request::is('subagt') ? 'active' : '' }}">Sub Account Group
                                        Table</a>
                                </li>
                            @endcanany
                        </ul>
                    </li>
                @endcanany
                <!-- /Master File -->

            </ul>
        </div>
        <!-- /main navigation -->

    </div>
    <!-- /sidebar content -->

</div>
