<?php

use Illuminate\Foundation\Inspiring;
use Illuminate\Support\Facades\Artisan;
use Illuminate\Support\Facades\Schedule;

Artisan::command('inspire', function () {
    $this->comment(Inspiring::quote());
})->purpose('Display an inspiring quote')->hourly();

// QA BUG-144: scheduled recurring journals (drafts), every tenant. Hourly so a
// container that was down at night still catches up the same day; the job is
// idempotent per (schedule, date).
Schedule::command('accounting:recurring-journals')->hourlyAt(5)->withoutOverlapping()->onOneServer();
