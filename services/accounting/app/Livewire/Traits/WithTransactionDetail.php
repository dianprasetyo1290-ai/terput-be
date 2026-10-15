<?php

namespace App\Livewire\Traits;

use App\Models\Currency;
use App\Models\TransDetail;
use App\Models\TransactionType;
use App\Models\JournalTransaction;
use Livewire\Attributes\Computed;
use Illuminate\Support\Facades\Crypt;

trait WithTransactionDetail
{
    public function detail($id, $c = "")
    {
        $this->js(<<<'JS'
            $(document).ready(function() {
                $('.select-search2').select2();
                $('.select-search2').on('change', function (e) {
                    var data = $('.select-search2').select2("val");
                    console.log(data);
                    $wire.set('jtdetail.acc_code', data);
                });
                $('.select-search3').select2();
                $('.select-search3').on('change', function (e) {
                    var data = $('.select-search3').select2("val");
                    console.log(data);
                    $wire.set('jtdetail.div_code', data);
                });
                $wire.set('currency', 'Rp');
            });
        JS);
        if ($id && $c) {
            $this->jteditdetail = JournalTransaction::with(['transType', 'transDetail' => function ($a) use ($c) {
                $a->where('id', $c)->first();
            }])->where('trans_no', $this->idDetail)->first();
            $this->jteditdetail = $this->jteditdetail->transDetail[0]->toArray();
            $this->idEdit = $this->jteditdetail['id'];
            $this->count = $this->jteditdetail['trans_count'];
            $this->jtdetail['debetnew'] = $this->jteditdetail['debet'];
            $this->jtdetail['creditnew'] = $this->jteditdetail['credit'];
            $this->jtdetail['description'] = $this->jteditdetail['description'];
            $this->jtdetail['acc_code'] = $this->jteditdetail['acc_code'];
            $this->jtdetail['div_code'] = $this->jteditdetail['div_code'];
            $this->jtdetail['curr_rate'] = $this->jteditdetail['curr_rate'];
            $this->jtdetail['curr_debet'] = $this->jteditdetail['curr_debet'];
            $this->jtdetail['curr_credit'] = $this->jteditdetail['curr_credit'];
            $this->currency = $this->jteditdetail['curr_code'];
            $this->updatedCurrency();
        } else {
            $this->formDetail = true;
            $this->idDetail = Crypt::decrypt($id);
            $this->jtdetail = JournalTransaction::with(['transType', 'transDetail' => function ($query) {
                $query->orderBy('created_at', 'asc');
            }])->where('trans_no', $this->idDetail)->first();

            $transDetails = $this->jtdetail->transDetail;
            $lastTransDetail = $transDetails->last();

            $this->jtdetail['description'] = $lastTransDetail->description ?? '';
            $this->jtdetail = $this->jtdetail->toArray();
            // Bug 4 fix: use max() instead of count() to avoid duplicate trans_count
            $this->count = (TransDetail::where('trans_no', $this->jtdetail['trans_no'])->max('trans_count') ?? 0) + 1;
            $this->jtdetail['tdebet'] = array_sum(array_column($this->jtdetail['trans_detail'], 'debet'));
            $this->jtdetail['tcredit'] = array_sum(array_column($this->jtdetail['trans_detail'], 'credit'));
            $this->resetValidation();
        }
    }

    public function saveDetail()
    {
        $this->authorize('editjournal_transaction');
        // QA BUG-145/146/143: every line needs an account that can take a
        // manual posting (active, detail, not a control account), a unit, a
        // description and exactly one positive amount.
        foreach (['debetnew', 'creditnew'] as $k) {
            if (($this->jtdetail[$k] ?? '') === '' || ($this->jtdetail[$k] ?? null) === null) {
                $this->jtdetail[$k] = 0;
            }
        }
        $valid = [
            'jtdetail.acc_code' => 'required',
            'jtdetail.div_code' => 'required|exists:mst_division,div_code',
            'jtdetail.description' => 'required|string|max:250',
            'jtdetail.debetnew' => 'numeric|min:0',
            'jtdetail.creditnew' => 'numeric|min:0',
        ];
        $this->validate($valid, [
            'jtdetail.description.required' => 'Keterangan baris jurnal wajib diisi.',
            'jtdetail.debetnew.min' => 'Nilai debit tidak boleh negatif.',
            'jtdetail.creditnew.min' => 'Nilai kredit tidak boleh negatif.',
            'jtdetail.debetnew.numeric' => 'Nilai debit harus berupa angka.',
            'jtdetail.creditnew.numeric' => 'Nilai kredit harus berupa angka.',
        ]);

        $debet = (float) $this->jtdetail['debetnew'];
        $credit = (float) $this->jtdetail['creditnew'];

        if ($debet == 0 && $credit == 0) {
            $this->addError('jtdetail.debetnew', 'Debet atau Credit harus diisi');
            return;
        }

        if ($debet > 0 && $credit > 0) {
            $this->addError('jtdetail.debetnew', 'Pilih salah satu: Debet ATAU Credit, tidak boleh keduanya');
            return;
        }

        if ($problems = \App\Support\Accounts::postingProblems((string) $this->jtdetail['acc_code'], true, (string) $this->jtdetail['div_code'])) {
            foreach ($problems as $m) {
                $this->addError('jtdetail.acc_code', $m);
            }
            return;
        }

        if (!empty($this->jteditdetail)) {
            $td = TransDetail::where('id', $this->idEdit)->first();
            $td->update([
                'acc_code' => $this->jtdetail['acc_code'],
                'div_code' => $this->jtdetail['div_code'],
                'description' => $this->jtdetail['description'],
                'debet' => $this->jtdetail['debetnew'] ?? 0,
                'credit' => $this->jtdetail['creditnew'] ?? 0,
                'curr_code' => $this->currency,
                'curr_rate' => $this->jtdetail['curr_rate'] ?? 0,
                'curr_debet' => $this->jtdetail['curr_debet'] ?? 0,
                'curr_credit' => $this->jtdetail['curr_credit'] ?? 0,
                'trans_date' => $this->jtdetail['trans_date'],
                'trans_system' => $this->jtdetail['trans_system'] ?? '',
                'trans_yes' => $this->jtdetail['trans_yes'] ?? 0,
                'curr_system' => $this->rate_system,
                'curr_yes' => $this->jtdetail['curr_yes'] ?? 0,
            ]);
            $this->jteditdetail = [];
        } else {
            // Bug 7 fix: single query with eager load, reuse $al->trans_account
            $al = TransactionType::with('akunLawan')->where('trans_code', $this->jtdetail['trans_code'])->first();
            if ($al->akunLawan) {
                if ($problems = \App\Support\Accounts::postingProblems((string) $al->trans_account, true, (string) $this->jtdetail['div_code'])) {
                    foreach ($problems as $m) {
                        $this->addError('jtdetail.acc_code', "Akun lawan tipe transaksi: $m");
                    }
                    return;
                }
                // Bug 6 fix: use extracted helper to avoid DRY violation
                $this->createTransDetail();

                $this->createTransDetail([
                    'trans_count' => $this->count + 1,
                    'acc_code' => $al->trans_account,
                    'debet' => $this->jtdetail['creditnew'] ?? 0,
                    'credit' => $this->jtdetail['debetnew'] ?? 0,
                ]);
            } else {
                $this->createTransDetail();
            }
        }
        $this->detail(Crypt::encrypt($this->jtdetail['trans_no']));
        $this->currency = '';
        $this->rate_system = '';
        $this->showToast('Data saved successfully');
    }

    public function confirmDeleteDetail($id)
    {
        $this->countDetail = $id;
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
                $wire.deleteD()
            } else if (result.isDenied) {
                $wire.clear()
            }
          })
        JS);
    }

    public function deleteD()
    {
        $this->authorize('editjournal_transaction');
        $user = TransDetail::where('id', $this->countDetail)->first();
        $user->delete();
        $this->jtdetail = [];

        $this->showToast('Data deleted successfully');
        $this->detail(Crypt::encrypt($this->idDetail));
    }

    public function updatedCurrency()
    {
        $this->rate_system = Currency::find($this->currency);
        $this->rate_system = $this->rate_system->rate_system ?? '';
    }

    public function batal()
    {
        $this->idNya = null;
        $this->formDetail = false;
        $this->form = false;
        $this->jt = [
            'trans_code' => '',
            'trans_no' => '',
            'trans_date' => '',
            'div_code' => '',
            'div_name' => '',
        ];
        $this->currency = '';
        $this->rate_system = '';
    }

    #[Computed]
    public function sisasaldodebet()
    {
        return TransDetail::where('trans_no', $this->idDetail)->sum('credit') - TransDetail::where('trans_no', $this->idDetail)->sum('debet') - (int)($this->jtdetail['debetnew'] ?? 0);
    }

    #[Computed]
    public function sisasaldokredit()
    {
        return TransDetail::where('trans_no', $this->idDetail)->sum('debet') - TransDetail::where('trans_no', $this->idDetail)->sum('credit') - (int)($this->jtdetail['creditnew'] ?? 0);
    }

    // Bug 6 fix: extracted DRY helper for creating TransDetail
    private function createTransDetail(array $overrides = []): TransDetail
    {
        $defaults = [
            'trans_code' => $this->jtdetail['trans_code'],
            'trans_no' => $this->jtdetail['trans_no'],
            'trans_count' => $this->count,
            'acc_code' => $this->jtdetail['acc_code'],
            'div_code' => $this->jtdetail['div_code'],
            'description' => $this->jtdetail['description'],
            'debet' => $this->jtdetail['debetnew'] ?? 0,
            'credit' => $this->jtdetail['creditnew'] ?? 0,
            'curr_code' => $this->currency,
            'curr_rate' => $this->jtdetail['curr_rate'] ?? 0,
            'curr_debet' => $this->jtdetail['curr_debet'] ?? 0,
            'curr_credit' => $this->jtdetail['curr_credit'] ?? 0,
            'trans_date' => $this->jtdetail['trans_date'],
            'trans_system' => $this->jtdetail['trans_system'] ?? '',
            'trans_yes' => $this->jtdetail['trans_yes'] ?? 0,
            'curr_system' => $this->rate_system,
            'curr_yes' => $this->jtdetail['curr_yes'] ?? 0,
        ];

        return TransDetail::create(array_merge($defaults, $overrides));
    }

    // Bug 8 fix: extracted reusable toast JS
    private function showToast(string $title = 'Data saved successfully', string $icon = 'success'): void
    {
        $this->js("
            Swal.mixin({
                toast: true,
                position: 'top-end',
                showConfirmButton: false,
                timer: 2000,
                timerProgressBar: true,
                didOpen: (toast) => {
                    toast.onmouseenter = Swal.stopTimer;
                    toast.onmouseleave = Swal.resumeTimer;
                }
            }).fire({
                icon: '{$icon}',
                title: '{$title}'
            });
        ");
    }
}
