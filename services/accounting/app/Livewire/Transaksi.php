<?php

namespace App\Livewire;

use Livewire\Component;
use Illuminate\Support\Facades\DB;
use Illuminate\Support\Facades\Auth;
use Carbon\Carbon;
use Illuminate\Support\Collection;

class Transaksi extends Component
{
    // Properties for TransHead
    public $transCode;
    public $transNo; // Ini akan diisi manual
    public $transDate;
    public $headerDescription;
    public $debetAmountHeader = 0;
    public $creditAmountHeader = 0;

    // Properties for TransDetail (array of arrays)
    public $details = [];

    // Properties for master data (dropdowns/search)
    public $masterTransactionTypes = [];
    public $masterAccounts; // Akan menjadi Collection
    public $masterDivisions; // Akan menjadi Collection

    // Validation Rules
    protected $rules = [
        'transCode' => 'required|string|max:5',
        'transNo' => 'required|string|max:15|unique:trans_head,trans_no', // Tetap pastikan unique
        'transDate' => 'required|date',
        'headerDescription' => 'nullable|string|max:500',
        'debetAmountHeader' => 'required|numeric|min:0',
        'creditAmountHeader' => 'required|numeric|min:0',

        'details.*.acc_code' => 'required|string|max:15',
        'details.*.div_code' => 'nullable|string|max:5',
        'details.*.description' => 'nullable|string|max:250',
        'details.*.debet' => 'required|numeric|min:0',
        'details.*.credit' => 'required|numeric|min:0',
    ];

    // Custom Validation Messages
    protected $messages = [
        'transNo.unique' => 'Nomor Transaksi ini sudah ada. Mohon gunakan nomor lain.',
        'debetAmountHeader.required' => 'Total Debet header wajib diisi.',
        'creditAmountHeader.required' => 'Total Kredit header wajib diisi.',
        'details.*.acc_code.required' => 'Akun Detail wajib diisi.',
        'details.*.debet.required' => 'Debet Detail wajib diisi.',
        'details.*.credit.required' => 'Kredit Detail wajib diisi.',
        'details.*.debet.min' => 'Debet Detail minimal 0.',
        'details.*.credit.min' => 'Kredit Detail minimal 0.',
    ];

    public function mount()
    {
        $this->transDate = Carbon::now()->toDateString();
        $this->transNo = ''; // Inisialisasi transNo sebagai string kosong untuk input manual

        $this->masterTransactionTypes = DB::table('mst_transaction')->select('trans_code', 'trans_name')->get();
        $this->masterAccounts = DB::table('mst_account')->select('acc_code', 'acc_name')->get();
        $this->masterDivisions = DB::table('mst_division')->select('div_code', 'div_name')->get();

        $this->addDetailRow();
    }

    // Hapus fungsi generateTransactionNumber() karena tidak lagi dibutuhkan

    public function addDetailRow()
    {
        $newIndex = count($this->details);
        $this->details[] = [
            'acc_code' => '',
            'div_code' => '',
            'description' => '',
            'debet' => 0,
            'credit' => 0,
            'curr_code' => null,
            'curr_rate' => 0,
            'curr_debet' => 0,
            'curr_credit' => 0,
            'trans_date' => $this->transDate,
            'trans_system' => 'N',
            'curr_system' => 'N',
            'curr_yes' => false,
            'trans_yes' => true,
        ];
    }

    public function removeDetailRow($index)
    {
        unset($this->details[$index]);
        $this->details = array_values($this->details);

        $this->calculateTotals();
    }

    public function updated($propertyName)
    {
        if (str_starts_with($propertyName, 'details.') && (str_ends_with($propertyName, '.debet') || str_ends_with($propertyName, '.credit'))) {
            $this->calculateTotals();
        }

        if ($propertyName === 'transDate') {
            foreach ($this->details as $index => $detail) {
                $this->details[$index]['trans_date'] = $this->transDate;
            }
        }
    }

    public function getAccountDisplayName($accCode)
    {
        $account = $this->masterAccounts->firstWhere('acc_code', $accCode);
        return $account ? $account->acc_code . ' - ' . $account->acc_name : '';
    }

    public function getDivisionDisplayName($divCode)
    {
        $division = $this->masterDivisions->firstWhere('div_code', $divCode);
        return $division ? $division->div_code . ' - ' . $division->div_name : '';
    }

    public function calculateTotals()
    {
        $totalDebet = 0;
        $totalCredit = 0;

        foreach ($this->details as $detail) {
            $totalDebet += (float) $detail['debet'];
            $totalCredit += (float) $detail['credit'];
        }

        $this->debetAmountHeader = $totalDebet;
        $this->creditAmountHeader = $totalCredit;
    }

    public function saveTransaction()
    {
        $this->validate();

        $totalDetailDebet = array_sum(array_column($this->details, 'debet'));
        $totalDetailCredit = array_sum(array_column($this->details, 'credit'));

        if ($totalDetailDebet != $totalDetailCredit) {
            session()->flash('error', 'Total Debet dan Kredit pada detail transaksi harus seimbang.');
            return;
        }

        if ($this->debetAmountHeader != $totalDetailDebet || $this->creditAmountHeader != $totalDetailCredit) {
            session()->flash('error', 'Total Debet/Kredit Header tidak sesuai dengan total detail. Silakan periksa kembali.');
            $this->calculateTotals();
            return;
        }

        DB::beginTransaction();
        try {
            DB::table('trans_head')->insert([
                'trans_code' => $this->transCode,
                'trans_no' => $this->transNo,
                'trans_date' => $this->transDate,
                'trans_update' => Carbon::now(),
                'trans_user' => Auth::user()->user_id ?? 'default_user',
                'debet' => $this->debetAmountHeader,
                'credit' => $this->creditAmountHeader,
                'created_at' => Carbon::now(),
                'updated_at' => Carbon::now(),
            ]);

            foreach ($this->details as $index => $detail) {
                DB::table('trans_detail')->insert([
                    'trans_code' => $this->transCode,
                    'trans_no' => $this->transNo,
                    'trans_count' => $index + 1,
                    'acc_code' => $detail['acc_code'],
                    'div_code' => $detail['div_code'],
                    'description' => $detail['description'],
                    'debet' => $detail['debet'],
                    'credit' => $detail['credit'],
                    'curr_code' => null,
                    'curr_rate' => 0,
                    'curr_debet' => 0,
                    'curr_credit' => 0,
                    'trans_date' => $this->transDate,
                    'trans_system' => $detail['trans_system'],
                    'curr_system' => $detail['curr_system'],
                    'curr_yes' => $detail['curr_yes'],
                    'trans_yes' => $detail['trans_yes'],
                    'created_at' => Carbon::now(),
                    'updated_at' => Carbon::now(),
                ]);
            }

            DB::commit();

            session()->flash('message', 'Transaksi berhasil disimpan!');

            $this->reset([
                'transCode',
                'headerDescription',
                'debetAmountHeader',
                'creditAmountHeader',
                'details'
            ]);
            $this->transDate = Carbon::now()->toDateString();
            $this->transNo = ''; // Reset transNo ke kosong agar bisa diisi lagi
            $this->addDetailRow();
        } catch (\Exception $e) {
            DB::rollBack();
            session()->flash('error', 'Gagal menyimpan transaksi: ' . $e->getMessage());
            \Log::error('Error saving transaction: ' . $e->getMessage());
        }
    }

    public function render()
    {
        return view('livewire.transaksi');
    }
}
