<?php

namespace App\Livewire\Pages\Division;

use App\Models\Division as ModelsDivision;
use App\Support\AuditLog;
use App\Support\CoaTemplates;
use Illuminate\Support\Facades\Auth;
use Illuminate\Support\Facades\Crypt;
use Illuminate\Support\Facades\DB;
use Illuminate\Validation\Rule;
use Illuminate\Validation\ValidationException;
use Livewire\Component;
use Livewire\WithPagination;

/**
 * Units / divisions. QA round 2: every action checks its permission; the code
 * is the key journal lines and balances point at, so it is fixed after
 * creation and a unit in use can not be deleted (the FKs on trans_detail /
 * this_detail cascade, so a delete would have wiped its journal lines). A new
 * unit can get its own accounts from a COA template (QA BUG-141).
 */
class Division extends Component
{
    use WithPagination;
    protected $paginationTheme = 'bootstrap';

    public $form, $idNya;
    public $division = [
        'div_code' => null,
        'div_name' => null,
    ];
    /** COA template to copy to a NEW unit ('' = none). */
    public $coaTemplate = '';
    public $error = null;
    public $search, $order = "ASC", $limit = 10;

    public function add($id = "")
    {
        $this->authorize($id ? 'editdivision' : 'createdivision');
        $this->error = null;
        $this->resetValidation();
        if ($id) {
            $this->form = true;
            $this->idNya = Crypt::decrypt($id);
            $row = ModelsDivision::findOrFail($this->idNya);
            $this->division = ['div_code' => $row->div_code, 'div_name' => $row->div_name];
        } else {
            $this->idNya = null;
            $this->division = ['div_code' => null, 'div_name' => null];
            $this->coaTemplate = '';
            $this->form = true;
        }
    }

    public function clear()
    {
        $this->idNya = null;
        $this->form = false;
        $this->coaTemplate = '';
        $this->division = [
            'div_code' => null,
            'div_name' => null,
        ];
        $this->resetValidation();
    }

    public function sort($order)
    {
        $this->resetPage();
        $this->order = $order === 'DESC' ? 'DESC' : 'ASC';
    }

    public function updatedSearch()
    {
        $this->resetPage();
    }

    public function limits($limit)
    {
        $this->resetPage();
        $this->limit = max(1, min(200, (int) $limit));
    }

    public function save()
    {
        $this->authorize($this->idNya ? 'editdivision' : 'createdivision');
        $this->error = null;
        if ($this->idNya) {
            $this->division['div_code'] = $this->idNya; // the code is fixed once created
        }
        $this->validate([
            'division.div_code' => array_filter(['required', 'max:5', 'regex:/^[A-Za-z0-9_-]+$/',
                $this->idNya ? null : Rule::unique('mst_division', 'div_code')]),
            'division.div_name' => 'required|max:50',
        ], ['division.div_code.regex' => 'Kode unit hanya huruf, angka, - dan _.']);

        $copied = null;
        if ($this->idNya) {
            ModelsDivision::findOrFail($this->idNya)->update(['div_name' => $this->division['div_name']]);
            AuditLog::record('division.update', (string) $this->idNya, ['div_name' => $this->division['div_name']]);
        } else {
            try {
                DB::transaction(function () use (&$copied) {
                    ModelsDivision::create(['div_code' => $this->division['div_code'], 'div_name' => $this->division['div_name']]);
                    AuditLog::record('division.create', (string) $this->division['div_code'], ['div_name' => $this->division['div_name']]);
                    if ($this->coaTemplate !== '' && $this->coaTemplate !== null) {
                        $this->authorize('createchart_of_account');
                        $copied = CoaTemplates::copyToUnit((int) $this->coaTemplate, (string) $this->division['div_code'], (string) Auth::id());
                    }
                });
            } catch (ValidationException $e) {
                $this->error = implode(' ', collect($e->errors())->flatten()->all());

                return;
            }
        }

        $this->clear();
        $this->toast($copied ? "Unit disimpan; {$copied['created']} akun COA unit dibuat dari template" : 'Data saved successfully');
    }

    /** Why the unit can not be deleted; empty = it can. */
    private function usage(string $code): array
    {
        $b = [];
        foreach ([
            'trans_detail' => 'baris jurnal periode berjalan', 'this_detail' => 'baris jurnal histori',
            'mst_saldo_news' => 'baris saldo', 'mst_budget' => 'anggaran',
        ] as $table => $label) {
            if (DB::getSchemaBuilder()->hasTable($table) && DB::table($table)->where('div_code', $code)->exists()) {
                $b[] = "Unit $code masih dipakai $label.";
            }
        }
        if (DB::table('mst_account')->where('unit_scope', $code)->exists()) {
            $b[] = "Unit $code memiliki akun COA khusus unit.";
        }

        return $b;
    }

    public function confirmDelete($id)
    {
        $this->authorize('deletedivision');
        $this->idNya = Crypt::decrypt($id);
        if ($b = $this->usage((string) $this->idNya)) {
            $this->error = implode(' ', $b).' Unit yang sudah dipakai tidak dapat dihapus.';
            $this->idNya = null;

            return;
        }
        $this->error = null;
        $this->js(<<<'JS'
        Swal.fire({
                text: "Do you want to delete this data? This process is irreversible.",
                icon: 'warning',
                showDenyButton: true,
                confirmButtonColor: '#3085d6',
                confirmButtonText: 'Delete!',
                denyButtonText: 'Cancel'
          }).then((result) => {
            if (result.isConfirmed) {
                $wire.delete()
            } else if (result.isDenied) {
                $wire.clear()
            }
          })
        JS);
    }

    public function delete()
    {
        $this->authorize('deletedivision');
        $code = (string) $this->idNya;
        if ($code === '' || ($b = $this->usage($code))) {
            $this->error = isset($b) && $b ? implode(' ', $b).' Unit yang sudah dipakai tidak dapat dihapus.' : null;
            $this->clear();

            return;
        }
        ModelsDivision::find($code)?->delete();
        AuditLog::record('division.delete', $code);
        $this->clear();
        $this->toast('Data deleted successfully');
    }

    private function toast(string $title): void
    {
        $title = json_encode($title);
        $this->js("Swal.mixin({toast: true, position: 'top-end', showConfirmButton: false, timer: 2000, timerProgressBar: true}).fire({icon: 'success', title: $title});");
    }

    public function render()
    {
        $data = ModelsDivision::query();
        if ($this->search) {
            $s = '%'.$this->search.'%';
            $data->where(fn ($q) => $q->where('div_code', 'ilike', $s)->orWhere('div_name', 'ilike', $s));
        }
        $data->orderBy('div_code', $this->order === 'DESC' ? 'DESC' : 'ASC');

        return view('livewire.pages.division.division', [
            'data' => $data->paginate($this->limit),
            'coaTemplates' => DB::table('acc_coa_templates')->orderBy('name')->get(['id', 'name']),
        ]);
    }
}
