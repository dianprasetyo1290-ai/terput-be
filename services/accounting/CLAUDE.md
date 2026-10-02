# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

This is **Sekoci Accounting**: the accounting/financial module of Sekoci (Laravel 11 + Livewire 3), handling journal transactions, general ledger, financial reporting, chart of accounts, budgeting and multi-currency support.

It is no longer a standalone app. It runs as the `accounting` service inside the Sekoci compose stack, is reached at `/accounting/` behind the Sekoci frontend nginx, and users never log in here directly: the Sekoci SPA calls `POST /accounting/sso` with a Sekoci JWT (`app/Http/Controllers/SsoController.php`).

**Multi-tenant, schema-per-tenant.** One Postgres database (`skci_accounting`), one schema per school (`tenant_<school_id hex>`), `public` holding only session/cache/queue/telescope tables. `app/Http/Middleware/TenantSchema.php` sets `search_path` per request from the session, and `app/Support/Tenancy.php` is the only place a schema name is built. **No model or Livewire component filters by school**, so isolation rests entirely on `search_path` -- a query that runs with the wrong or unset schema is a cross-school data leak, not a cosmetic bug.

## Technology Stack

- **Backend**: Laravel 11 (PHP 8.2+)
- **Frontend**: Livewire 3 with Tailwind CSS
- **Authentication**: Laravel Jetstream with Sanctum
- **Authorization**: Spatie Laravel Permission (roles & permissions)
- **Database**: SQLite (default), supports MySQL/PostgreSQL
- **Dev Tools**: Laravel Telescope, Laravel Pint, PHPUnit
- **Build Tools**: Vite

## Development Commands

### Setup & Installation
```bash
# Install dependencies
composer install
npm install

# Setup environment
cp .env.example .env
php artisan key:generate

# Database setup: public (shared infra) only -- NEVER a global db:seed,
# several seeders TRUNCATE and would wipe every school's books.
php artisan migrate --force

# Then provision each school's schema (idempotent):
php artisan accounting:provision <school_id> --seed
php artisan accounting:tenants

# Build frontend assets
npm run dev          # Development with hot reload
npm run build        # Production build
```

### Running the Application
```bash
# Start development server
php artisan serve

# Run Vite dev server (in separate terminal)
npm run dev
```

### Database & Seeding

**FORBIDDEN on any shared or production database: `migrate:fresh`, `migrate --seed`, and a bare `db:seed`.** ParameterSeeder, PeriodSeeder, ModulesSeeder, ComCodesSeeder, RoleAndPermissionSeeder and RowDetailsSeeder all `truncate()`, so running them against a provisioned schema destroys that school's data. `docker/entrypoint.sh` deliberately migrates `public` only and seeds nothing.

```bash
# Migrate one school's schema
DB_SEARCH_PATH=tenant_<hex>,public php artisan migrate --force

# Seed the baseline into a NEW, empty school schema only
php artisan accounting:provision <school_id> --seed
```

### Code Quality
```bash
# Format code with Laravel Pint
./vendor/bin/pint

# Run tests
php artisan test

# Run specific test
php artisan test --filter TestName
```

### Accounting Cycle Test
```bash
# Run end-to-end accounting cycle test (uses exco_test database via .env.testing)
# Tests: 12 months EOM, quarterly reports, EOY, new year — 255 checks
APP_ENV=testing php artisan test:accounting-cycle --fresh --force

# First time setup: create exco_test database and run migrations
PGPASSWORD=change-me psql -h localhost -U postgres -c "CREATE DATABASE exco_test;"
APP_ENV=testing php artisan migrate --env=testing
```

### Debugging
```bash
# Clear all caches
php artisan optimize:clear

# Access Telescope (dev only)
# Available at /telescope when APP_DEBUG=true
```

## Architecture & Structure

### Database Architecture

The application uses a dual transaction table structure:
- **Current Period**: `trans_head` and `trans_detail` tables store transactions for the active period
- **History**: `this_head` and `this_detail` tables archive validated/closed period transactions
- **Balance Tracking**: `mst_saldo_news` table maintains running balances by account, division, period, and year

Key models: `JournalTransaction`, `TransDetail`, `ThisHead`, `ThisDetail`, `MstSaldoNew`

### Helper Functions

Three helper files are auto-loaded (see composer.json):
- `app/Helpers/Helper.php`: General utilities (image_url, title_web, get_setting_web, cek_min, formatTanggalIndonesia, gen_mst_saldo)
- `app/Helpers/Query.php`: Financial queries (getTotalValue, getTotalLastMonth)
- `app/Helpers/QueryGeneral.php`: Ledger queries (getBeginningBalance, getLedgerDetails)

These are globally available without imports.

### Livewire Component Pattern

UI is built entirely with Livewire components following this structure:
- `app/Livewire/Pages/`: Full page components (e.g., JournalTransaction/JT.php)
- `app/Livewire/Component/`: Reusable components (e.g., FormBalance.php)
- `app/Livewire/Global/`: Shared components (PageHeader, ModalReportFinal)
- `app/Livewire/Report/`: Report generation components

All pages use Livewire routing defined in `routes/web.php`. No traditional Laravel controllers for views.

### Permission System

Routes are protected with Spatie Permission middleware:
```php
Route::group(["middleware" => ['can:viewjournal_transaction']], function () {
    Route::get('/journaltransaction', JT::class);
});
```

Permissions are seeded via `RoleAndPermissionSeeder` and managed through the UI.

### Financial Period Management

The `Parameter` model stores the active year and period. Most financial operations respect this active period:
- Journal transactions default to the active period date range
- Period closing moves data from `trans_*` to `this_*` tables
- End-of-month (EOM) and end-of-year (EOY) processes update balances

### Report Generator System

Custom report builder with three components:
- **Row Generator**: `ReportRowHead` and `ReportRowDetail` define report structure
- **Final Generator**: `ReportFinal` and `ReportFinalData` combine multiple row reports
- Dynamic formula evaluation for calculated fields
- Export to Excel via Maatwebsite Excel package

### Multi-Currency Support

- `Currency` model manages exchange rates
- `BalanceCurrency` tracks foreign currency balances
- Transactions can specify currency and conversion rate

## Key Workflows

### Journal Entry Flow
1. Create in Journal Transaction (JT) page
2. Stored in `trans_head` and `trans_detail`
3. Validated through Journal Validation (JV)
4. After period close, moves to `this_head` and `this_detail`
5. Updates `mst_saldo_news` balances

### Period Closing
1. Validate all transactions for the period
2. Run End-of-Month (EOM) process
3. Transactions archived to history tables
4. Balances updated in `mst_saldo_news`
5. Period advanced in `Parameter` table

### Balance Calculation
Use helper functions from `app/Helpers/Query.php`:
- `getTotalValue($account, $periode, $tahun, $normalBalance, $divisi)` - Current balance
- `getBeginningBalance($tahun, $acc_code, $period)` - Opening balance
- All calculations aggregate from `mst_saldo_news` table

## Database Conventions

- **Account Codes**: Pattern matching with ILIKE for flexible querying
- **Period Format**: Numeric month (1-12), year stored separately
- **Division Codes**: Prefix-based system for organizational units
- **Transaction Numbers**: Auto-generated per transaction type

## Image Handling

Images are served through a custom route:
```php
Route::get('show-picture/{path}', [HelperController::class, 'showPicture'])
```

Use the `image_url($path, $fallback)` helper for safe image URLs with placeholder fallback.

## Seeding Strategy

Core seeders (always run):
- ModulesSeeder: Application modules
- RoleAndPermissionSeeder: Initial roles/permissions
- UserSeeder: Default users
- SettingWebSeeder: Website settings
- ComCodesSeeder: Common codes/lookups
- ParameterSeeder: System parameters

Optional seeders (commented out by default) for testing data.

## Testing Notes

- PHPUnit configured in `phpunit.xml`
- Tests use array-based session and cache
- Database connection can be set to SQLite in-memory for testing
- Pest plugin available but not actively used

## Docker & Deployment

- `Dockerfile` present for containerization
- GitLab CI/CD configured in `.gitlab-ci.yml`
- Kubernetes deployment manifest: `kube-deployment-dev.yaml`
- Deployment targets Harbor registry and GCP Kubernetes cluster

### Production

Deployment is Sekoci's, not this repo's: the stack lives at `/opt/apps/sekoci-src` on the Sekoci GCP VM (project `sekoci-trial`, SSH only via `--tunnel-through-iap`) and is served at https://app.sekoci.id. Build images on the Mac for `linux/amd64` and `docker save`/`load` them -- the VMs have 2 GB RAM and build OOM on-box.

The old standalone Exco deployment (its own server, `/var/opt/exco`, Harbor registry, `kube-deployment-dev.yaml`, `.gitlab-ci.yml`) is **not** how this service ships any more. Two plaintext passwords for those legacy hosts shipped in the original zip and have been removed from this repo; they should be rotated if those hosts still run.
