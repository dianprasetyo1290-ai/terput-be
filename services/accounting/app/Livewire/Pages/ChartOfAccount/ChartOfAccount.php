<?php

namespace App\Livewire\Pages\ChartOfAccount;

use App\Models\AccountGroupTable;
use App\Models\AccountSubGroupTable;
use App\Models\ChartOfAccount as ModelsChartOfAccount;
use App\Models\Currency;
use App\Support\Accounts;
use App\Support\AuditLog;
use Illuminate\Support\Facades\Crypt;
use Illuminate\Validation\Rule;
use Livewire\Component;
use Livewire\WithPagination;

/**
 * COA (PRD menu 33, FR-33-001): code, name, type (group), category (sub group),
 * parent, normal balance, active status and the kas/bank, inter-unit and
 * control flags. Accounts are deactivated, never deleted (QA BUG-060).
 */
class ChartOfAccount extends Component
{
    use WithPagination;
    protected $paginationTheme = 'bootstrap';

    private const EMPTY = [
        'acc_code' => null,
        'acc_name' => null,
        'acc_group' => null,
        'sub_acc_group' => null,
        'acc_currency' => null,
        'parent_code' => null,
        'normal_balance' => null,
        'is_active' => true,
        'is_cash' => false,
        'is_interunit' => false,
        'is_control' => false,
    ];

    public $listSubGroup = [];
    public $form, $idNya;
    public $coa = self::EMPTY;
    /** Reasons the last deactivation was refused (shown as a list). */
    public $blockers = [];
    public $blockedCode;
    public $search, $order = "ASC", $limit = 10, $status = 'all';

    public function updatedCoaAccGroup($value = '')
    {
        // Ambil data sub-group berdasarkan grup yang dipilih
        $this->listSubGroup = AccountSubGroupTable::where('account_group_table_id', $value)->get();
        if ($value !== '' && $value !== null && empty($this->idNya)) {
            $this->coa['normal_balance'] = Accounts::defaultNormalBalance((int) $value);
        }
        $data = json_encode($this->listSubGroup);

        $this->js(<<<JS
                $('#OutletId').select2({ allowClear: true });
                var selectElement = document.getElementById('OutletId');
                // Remove all options
                selectElement.innerHTML = '';

                // Add new options
                var newOptions = $data;

                var newOption2 = document.createElement('option');
                newOption2.value = '';
                newOption2.text = 'Pilih';
                selectElement.add(newOption2);
                newOptions.forEach(function(option) {
                    var newOption = document.createElement('option');
                    newOption.value = option.id;
                    newOption.text = option.name;
                    selectElement.add(newOption);
                });
        JS);
    }

    public function updatedSearch()
    {
        $this->resetPage();
    }

    public function updatedStatus()
    {
        $this->resetPage();
    }

    public function add($id = "")
    {
        $this->authorize($id ? 'editchart_of_account' : 'createchart_of_account');
        $this->js(<<<'JS'
            $(document).ready(function() {
                $('.select-search').select2();
                $('.select-search2').select2();
                $('.select-search3').select2();

                $('.select-search').on('change', function (e) {
                    $wire.set('coa.acc_group', $('.select-search').select2("val"));
                });
                $('.select-search2').on('change', function (e) {
                    $wire.set('coa.sub_acc_group', $('.select-search2').select2("val"));
                });
                $('.select-search3').on('change', function (e) {
                    $wire.set('coa.acc_currency', $('.select-search3').select2("val"));
                });
            });
        JS);

        $this->blockers = [];
        $this->form = true;
        if ($id) {
            $this->idNya = Crypt::decrypt($id);
            $acc = ModelsChartOfAccount::findOrFail($this->idNya);
            $this->listSubGroup = AccountSubGroupTable::where('account_group_table_id', $acc->acc_group)->get();
            $this->coa = array_merge(self::EMPTY, array_intersect_key($acc->toArray(), self::EMPTY));
            $this->coa['is_active'] = (bool) $this->coa['is_active'];
            $this->coa['is_cash'] = (bool) $this->coa['is_cash'];
            $this->coa['is_interunit'] = (bool) $this->coa['is_interunit'];
            $this->coa['is_control'] = (bool) $this->coa['is_control'];
            $this->resetValidation();
        } else {
            $this->idNya = null;
            $this->coa = self::EMPTY;
        }
    }

    public function clear()
    {
        $this->idNya = null;
        $this->form = false;
        $this->coa = self::EMPTY;
        $this->resetValidation();
    }

    protected function rules(): array
    {
        return [
            'coa.acc_code' => array_filter([
                'required', 'max:15', 'regex:/^\S+$/',
                $this->idNya ? null : Rule::unique('mst_account', 'acc_code'),
            ]),
            'coa.acc_name' => 'required|max:50',
            'coa.acc_group' => 'required|exists:account_group_tables,id',
            'coa.sub_acc_group' => 'nullable|exists:account_sub_group_tables,id',
            'coa.acc_currency' => 'nullable|exists:mst_currency,curr_code',
            'coa.parent_code' => 'nullable|max:15',
            'coa.normal_balance' => 'required|in:D,K',
            'coa.is_active' => 'boolean',
            'coa.is_cash' => 'boolean',
            'coa.is_interunit' => 'boolean',
            'coa.is_control' => 'boolean',
        ];
    }

    protected function messages(): array
    {
        return [
            'coa.acc_code.regex' => 'Kode akun tidak boleh mengandung spasi.',
            'coa.normal_balance.required' => 'Saldo normal wajib dipilih (Debit/Kredit).',
        ];
    }

    public function save()
    {
        $this->authorize($this->idNya ? 'editchart_of_account' : 'createchart_of_account');
        foreach (['sub_acc_group', 'acc_currency', 'parent_code'] as $k) {
            if (($this->coa[$k] ?? null) === '') {
                $this->coa[$k] = null;
            }
        }
        if ($this->idNya) {
            // The code is the key every journal and balance row points at.
            $this->coa['acc_code'] = $this->idNya;
        }
        $this->validate();

        $code = (string) $this->coa['acc_code'];
        $group = (int) $this->coa['acc_group'];
        $errors = [];
        foreach (Accounts::parentProblems($this->idNya, $this->coa['parent_code'], $group) as $p) {
            $errors['coa.parent_code'][] = $p;
        }

        $current = $this->idNya ? ModelsChartOfAccount::findOrFail($this->idNya) : null;
        if ($current && Accounts::hasActivity($code)) {
            if (($current->normal_balance ?? null) !== $this->coa['normal_balance']) {
                $errors['coa.normal_balance'][] = 'Saldo normal terkunci: akun sudah memiliki transaksi/saldo.';
            }
            if ((int) $current->acc_group !== $group) {
                $errors['coa.acc_group'][] = 'Kelompok akun terkunci: akun sudah memiliki transaksi/saldo.';
            }
        }
        if ($current && Accounts::isParent($code) && $this->coa['is_control']) {
            $errors['coa.is_control'][] = 'Akun induk tidak dapat menjadi akun kontrol.';
        }
        if ($current && $current->is_active && ! $this->coa['is_active']) {
            foreach (Accounts::deactivationBlockers($code) as $b) {
                $errors['coa.is_active'][] = $b;
            }
        }
        if ($errors) {
            foreach ($errors as $field => $msgs) {
                foreach ($msgs as $m) {
                    $this->addError($field, $m);
                }
            }

            return;
        }

        $values = array_intersect_key($this->coa, self::EMPTY);
        if ($current) {
            $before = array_intersect_key($current->toArray(), self::EMPTY);
            $current->update($values);
            AuditLog::record('coa.update', $code, ['before' => $before, 'after' => $values]);
        } else {
            ModelsChartOfAccount::create($values);
            AuditLog::record('coa.create', $code, $values);
        }

        $this->clear();
        $this->toast('Data saved successfully');
    }

    /**
     * Soft delete (QA BUG-060): deactivate, refused with the list of reasons
     * when the account still carries a balance, mapping, open journals or
     * active children.
     */
    public function deactivate($id)
    {
        $this->authorize('deletechart_of_account');
        $code = Crypt::decrypt($id);
        $acc = ModelsChartOfAccount::findOrFail($code);
        $this->blockers = Accounts::deactivationBlockers($code);
        $this->blockedCode = $code;
        if ($this->blockers) {
            return;
        }
        $acc->update(['is_active' => false]);
        AuditLog::record('coa.deactivate', $code);
        $this->toast("Akun $code dinonaktifkan");
    }

    public function activate($id)
    {
        $this->authorize('editchart_of_account');
        $code = Crypt::decrypt($id);
        ModelsChartOfAccount::findOrFail($code)->update(['is_active' => true]);
        AuditLog::record('coa.activate', $code);
        $this->blockers = [];
        $this->toast("Akun $code diaktifkan kembali");
    }

    public function sort($order)
    {
        $this->resetPage();
        $this->order = $order === 'DESC' ? 'DESC' : 'ASC';
    }

    public function limits($limit)
    {
        $this->resetPage();
        $this->limit = max(1, min(200, (int) $limit));
    }

    private function toast(string $title): void
    {
        $title = json_encode($title);
        $this->js("Swal.mixin({toast: true, position: 'top-end', showConfirmButton: false, timer: 2000, timerProgressBar: true}).fire({icon: 'success', title: $title});");
    }

    public function render()
    {
        $data = ModelsChartOfAccount::with('group', 'sub_group');
        if ($this->search) {
            $s = '%'.$this->search.'%';
            $data->where(fn ($q) => $q->where('acc_code', 'ilike', $s)
                ->orWhere('acc_name', 'ilike', $s)
                ->orWhere('acc_currency', 'ilike', $s));
        }
        if ($this->status === 'active') {
            $data->where('is_active', true);
        } elseif ($this->status === 'inactive') {
            $data->where('is_active', false);
        }
        $data->orderBy('acc_code', $this->order === 'DESC' ? 'DESC' : 'ASC');

        return view('livewire.pages.chart-of-account.chart-of-account', [
            'data' => $data->paginate($this->limit),
            'listGroup' => AccountGroupTable::all(),
            'listSubGroup' => $this->listSubGroup,
            'currency' => Currency::orderBy('curr_code', 'ASC')->get(),
            'parents' => ModelsChartOfAccount::orderBy('acc_code')
                ->when($this->coa['acc_group'] ?? null, fn ($q, $g) => $q->where('acc_group', $g))
                ->when($this->idNya, fn ($q, $id) => $q->where('acc_code', '!=', $id))
                ->get(['acc_code', 'acc_name']),
            'parentCodes' => ModelsChartOfAccount::whereNotNull('parent_code')->distinct()->pluck('parent_code')->flip(),
        ]);
    }
}
