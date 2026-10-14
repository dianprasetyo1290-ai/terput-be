<?php

namespace App\Livewire\Pages\JournalTemplate;

use App\Support\AuditLog;
use App\Support\JournalTemplates;
use App\Support\Ledger;
use App\Support\RecurringJournals;
use Illuminate\Support\Facades\Auth;
use Illuminate\Support\Facades\DB;
use Illuminate\Validation\ValidationException;
use Livewire\Component;

/**
 * Template jurnal & jurnal berulang terjadwal (QA BUG-144, FR-34-006).
 *  - viewjournal_template: see templates, schedules and run log;
 *  - createjournal_template: manage templates and schedules;
 *  - createjournal_transaction: turn a template into a draft journal.
 */
class JournalTemplate extends Component
{
    /** null = list, 'template' = template form, 'schedule' = schedule form */
    public $mode = null;

    public $editId = null;
    public $tpl = ['name' => '', 'trans_code' => '', 'description' => '', 'lines' => []];
    public $copyFrom = '';

    public $rec = ['template_id' => '', 'day_of_month' => 1, 'start_date' => '', 'end_date' => ''];

    /** Draft creation: template id => date */
    public $draftDate = '';
    public $draftFor = null;

    public $message = null;
    public $error = null;

    public function mount(): void
    {
        $this->draftDate = $this->defaultDate();
    }

    private function defaultDate(): string
    {
        $a = Ledger::active();
        $r = Ledger::range($a['year'], $a['period']);
        $today = now()->toDateString();

        return $r ? max($r[0], min($r[1], $today)) : $today;
    }

    private function actor(): string
    {
        return (string) Auth::id();
    }

    private function done(string $msg): void
    {
        $this->error = null;
        $this->message = $msg;
    }

    private function fail(ValidationException $e): void
    {
        $this->message = null;
        $this->error = implode(' ', collect($e->errors())->flatten()->all());
        foreach ($e->errors() as $k => $msgs) {
            foreach ($msgs as $m) {
                $this->addError($k, $m);
            }
        }
    }

    // ---- templates ------------------------------------------------------

    public function newTemplate(): void
    {
        $this->authorize('createjournal_template');
        $this->resetValidation();
        $this->editId = null;
        $this->tpl = ['name' => '', 'trans_code' => '', 'description' => '', 'lines' => [$this->blankLine(), $this->blankLine()]];
        $this->mode = 'template';
    }

    public function editTemplate(int $id): void
    {
        $this->authorize('createjournal_template');
        $t = DB::table('acc_journal_templates')->where('id', $id)->first();
        if (! $t) {
            $this->error = 'Template tidak ditemukan.';

            return;
        }
        $this->resetValidation();
        $this->editId = $id;
        $this->tpl = [
            'name' => $t->name, 'trans_code' => $t->trans_code, 'description' => (string) $t->description,
            'lines' => DB::table('acc_journal_template_lines')->where('template_id', $id)->orderBy('line_no')->get()
                ->map(fn ($l) => ['acc_code' => $l->acc_code, 'div_code' => $l->div_code, 'description' => $l->description,
                    'debit' => (float) $l->debit ?: '', 'credit' => (float) $l->credit ?: ''])->all(),
        ];
        $this->mode = 'template';
    }

    private function blankLine(): array
    {
        return ['acc_code' => '', 'div_code' => '', 'description' => '', 'debit' => '', 'credit' => ''];
    }

    public function addLine(): void
    {
        $this->tpl['lines'][] = $this->blankLine();
    }

    public function removeLine(int $i): void
    {
        unset($this->tpl['lines'][$i]);
        $this->tpl['lines'] = array_values($this->tpl['lines']);
    }

    public function copyFromJournal(): void
    {
        $this->authorize('createjournal_template');
        $no = trim((string) $this->copyFrom);
        $lines = $no !== '' ? JournalTemplates::linesFromJournal($no) : [];
        if (! $lines) {
            $this->addError('copyFrom', "Jurnal $no tidak ditemukan atau tidak memiliki baris.");

            return;
        }
        $this->tpl['lines'] = array_map(fn ($l) => array_merge($l, ['debit' => $l['debit'] ?: '', 'credit' => $l['credit'] ?: '']), $lines);
        if (! $this->tpl['trans_code']) {
            $this->tpl['trans_code'] = (string) (DB::table('trans_head')->where('trans_no', $no)->value('trans_code')
                ?? DB::table('this_head')->where('trans_no', $no)->value('trans_code'));
        }
    }

    public function saveTemplate(): void
    {
        $this->authorize('createjournal_template');
        $this->resetValidation();
        try {
            $id = JournalTemplates::save($this->editId, (string) $this->tpl['name'], (string) $this->tpl['trans_code'],
                $this->tpl['description'] ?? null, $this->tpl['lines'] ?? [], $this->actor());
        } catch (ValidationException $e) {
            $this->fail($e);

            return;
        }
        $this->mode = null;
        $this->done('Template "'.$this->tpl['name'].'" disimpan.');
        $this->editId = $id;
    }

    public function toggleTemplate(int $id): void
    {
        $this->authorize('createjournal_template');
        $t = DB::table('acc_journal_templates')->where('id', $id)->first();
        if (! $t) {
            return;
        }
        DB::table('acc_journal_templates')->where('id', $id)->update(['is_active' => ! $t->is_active, 'updated_at' => now()]);
        AuditLog::record($t->is_active ? 'journal_template.deactivate' : 'journal_template.activate', (string) $id);
        $this->done('Template "'.$t->name.'" '.($t->is_active ? 'dinonaktifkan' : 'diaktifkan').'.');
    }

    public function deleteTemplate(int $id): void
    {
        $this->authorize('createjournal_template');
        $n = DB::table('acc_recurring_journals')->where('template_id', $id)->count();
        if ($n > 0) {
            $this->message = null;
            $this->error = "Template dipakai $n jadwal berulang. Hapus jadwalnya dulu, atau nonaktifkan template.";

            return;
        }
        $name = DB::table('acc_journal_templates')->where('id', $id)->value('name');
        DB::table('acc_journal_templates')->where('id', $id)->delete();
        AuditLog::record('journal_template.delete', (string) $id, ['name' => $name]);
        $this->done("Template \"$name\" dihapus.");
    }

    public function askDraft(int $id): void
    {
        $this->authorize('createjournal_transaction');
        $this->draftFor = $id;
        $this->draftDate = $this->defaultDate();
    }

    public function createDraft(): void
    {
        $this->authorize('createjournal_transaction');
        try {
            $no = JournalTemplates::instantiate((int) $this->draftFor, (string) $this->draftDate, $this->actor());
        } catch (ValidationException $e) {
            $this->fail($e);

            return;
        }
        $this->draftFor = null;
        $this->done("Draft jurnal $no dibuat dari template. Buka Jurnal Transaksi untuk memeriksa lalu mengajukannya.");
    }

    // ---- schedules ------------------------------------------------------

    public function newSchedule(?int $templateId = null): void
    {
        $this->authorize('createjournal_template');
        $this->resetValidation();
        $this->rec = ['template_id' => $templateId ?? '', 'day_of_month' => 1, 'start_date' => $this->defaultDate(), 'end_date' => ''];
        $this->mode = 'schedule';
    }

    public function saveSchedule(): void
    {
        $this->authorize('createjournal_template');
        $this->resetValidation();
        try {
            RecurringJournals::create((int) $this->rec['template_id'], (int) $this->rec['day_of_month'],
                (string) $this->rec['start_date'], $this->rec['end_date'] ?: null, $this->actor());
        } catch (ValidationException $e) {
            $this->fail($e);

            return;
        }
        $this->mode = null;
        $this->done('Jadwal jurnal berulang disimpan. Draft dibuat otomatis setiap tanggal '.(int) $this->rec['day_of_month'].'.');
    }

    public function toggleSchedule(int $id): void
    {
        $this->authorize('createjournal_template');
        $s = DB::table('acc_recurring_journals')->where('id', $id)->first();
        if (! $s) {
            return;
        }
        DB::table('acc_recurring_journals')->where('id', $id)->update(['is_active' => ! $s->is_active, 'updated_at' => now()]);
        AuditLog::record($s->is_active ? 'recurring.pause' : 'recurring.resume', (string) $id);
        $this->done('Jadwal '.($s->is_active ? 'dihentikan sementara' : 'diaktifkan kembali').'.');
    }

    public function deleteSchedule(int $id): void
    {
        $this->authorize('createjournal_template');
        DB::table('acc_recurring_journals')->where('id', $id)->delete();
        AuditLog::record('recurring.delete', (string) $id);
        $this->done('Jadwal dihapus. Draft yang sudah dibuat tidak berubah.');
    }

    public function runNow(): void
    {
        $this->authorize('createjournal_template');
        $r = RecurringJournals::runDue();
        $this->done(sprintf('Jadwal diproses: %d draft dibuat, %d gagal, %d menunggu periode berikutnya dibuka.', $r['created'], $r['failed'], $r['waiting']));
    }

    public function retryRun(int $id, string $date): void
    {
        $this->authorize('createjournal_template');
        try {
            $no = RecurringJournals::retry($id, $date);
        } catch (ValidationException $e) {
            $this->fail($e);

            return;
        }
        $this->done("Draft $no dibuat untuk jadwal tanggal $date.");
    }

    public function skipRun(int $id, string $date): void
    {
        $this->authorize('createjournal_template');
        try {
            RecurringJournals::skip($id, $date, $this->actor());
        } catch (ValidationException $e) {
            $this->fail($e);

            return;
        }
        $this->done("Jadwal tanggal $date ditandai dilewati.");
    }

    public function cancel(): void
    {
        $this->mode = null;
        $this->draftFor = null;
        $this->resetValidation();
    }

    public function render()
    {
        $templates = DB::table('acc_journal_templates as t')
            ->leftJoin('acc_journal_template_lines as l', 'l.template_id', '=', 't.id')
            ->groupBy('t.id')->orderBy('t.name')
            ->selectRaw('t.*, count(l.id) as lines, coalesce(sum(l.debit),0) as total')->get();

        return view('livewire.pages.journal-template.journal-template', [
            'templates' => $templates,
            'schedules' => DB::table('acc_recurring_journals as r')->join('acc_journal_templates as t', 't.id', '=', 'r.template_id')
                ->orderBy('r.next_run')->get(['r.*', 't.name as template_name']),
            'runs' => DB::table('acc_recurring_runs as x')->join('acc_recurring_journals as r', 'r.id', '=', 'x.recurring_id')
                ->join('acc_journal_templates as t', 't.id', '=', 'r.template_id')
                ->orderByDesc('x.run_date')->limit(30)->get(['x.*', 't.name as template_name']),
            'types' => DB::table('mst_transaction')->orderBy('trans_code')->get(['trans_code', 'trans_name']),
            'accounts' => DB::table('mst_account')->where('is_active', true)->where('is_control', false)
                ->whereNotIn('acc_code', DB::table('mst_account')->whereNotNull('parent_code')->select('parent_code'))
                ->orderBy('acc_code')->get(['acc_code', 'acc_name', 'unit_scope']),
            'divisions' => DB::table('mst_division')->orderBy('div_code')->get(['div_code', 'div_name']),
        ]);
    }
}
