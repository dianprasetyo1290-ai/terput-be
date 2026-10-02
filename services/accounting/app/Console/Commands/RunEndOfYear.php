<?php

namespace App\Console\Commands;

use App\Livewire\Pages\EndOfYear\EOY;
use Illuminate\Console\Command;

class RunEndOfYear extends Command
{
    protected $signature = 'legacy:run-eoy {--force : Run even if validation checks fail}';

    protected $description = 'Run the application End of Year process from CLI for local legacy validation.';

    public function handle(): int
    {
        $eoy = new EOY();
        $eoy->mount();

        foreach ($eoy->checks as $check) {
            $this->line($check['label'] . ':' . ($check['passed'] ? 'pass' : 'fail') . ':' . $check['issue']);
        }

        if (!$eoy->allPassed && !$this->option('force')) {
            $this->error('EOY checks failed.');
            return self::FAILURE;
        }

        $eoy->generate();
        $this->line($eoy->answer);
        $this->info("Next year: {$eoy->nextYear}");

        return self::SUCCESS;
    }
}
