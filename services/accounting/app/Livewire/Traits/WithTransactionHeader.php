<?php

namespace App\Livewire\Traits;

use Carbon\Carbon;
use App\Models\JournalTransaction;
use App\Models\TransDetail;
use Illuminate\Validation\Rule;
use Illuminate\Support\Facades\Auth;
use Illuminate\Support\Facades\Crypt;

trait WithTransactionHeader
{
    public function add($id = "")
    {
        if ($id) {
            $this->form = true;
            $this->idNya = Crypt::decrypt($id);
            $jtModel = JournalTransaction::with('transDetail')->where('trans_no', $this->idNya)->first();
            $this->jt = $jtModel->toArray();
            $this->jt['debet'] = array_sum(array_column($this->jt['trans_detail'], 'debet'));
            $this->jt['credit'] = array_sum(array_column($this->jt['trans_detail'], 'credit'));
            $this->resetValidation();
        } else {
            $parameter = \App\Models\Parameter::first();
            if ($parameter) {
                $t = $parameter->year;
                $m = str_pad($parameter->period, 2, '0', STR_PAD_LEFT);
                $this->jt['trans_date'] = Carbon::createFromFormat('Y-m-d', "$t-$m-01")->format('Y-m-d');
            } else {
                $this->jt['trans_date'] = now()->format('Y-m-d');
            }
            $this->jt['trans_code'] = '';
            $this->jt['trans_no'] = '';

            $this->form = true;
        }

        $tanggal = isset($this->jt['trans_date']) ? $this->jt['trans_date'] : '';

        $fromJs = json_encode($this->dfrom);
        $toJs = json_encode($this->dto);
        $tanggalJs = json_encode($tanggal);

        $this->js("
            const from = {$fromJs};
            const to = {$toJs};
            let tanggal = {$tanggalJs};

            console.log('Tanggal Transaksi:', tanggal);

            $(document).ready(function() {
                $('.daterange-ranges').daterangepicker({
                    singleDatePicker: true,
                    locale: {
                        format: 'YYYY-MM-DD'
                    },
                    startDate: tanggal ? tanggal : from,
                    isInvalidDate: function(date) {
                        return !date.isBetween(from, to, null, '[]');
                    }
                });

                $('.daterange-ranges').on('apply.daterangepicker', function (ev, picker) {
                    var formattedDate = picker.startDate.format('YYYY-MM-DD');
                    console.log(formattedDate);
                    \$wire.set('jt.trans_date', formattedDate);
                });
            });
        ");

        $this->js(<<<'JS'
            $(document).ready(function() {
                $('.select-search').select2();

                $('.select-search').on('change', function (e) {
                    var data = $('.select-search').select2("val");
                    console.log(data);
                    $wire.set('jt.trans_code', data);
                });
            });
        JS);
    }

    public function save()
    {
        $valid = [
            // Bug 1 fix: was validating against mst_division, now validates against mst_transaction
            'jt.trans_code' => 'required|exists:mst_transaction,trans_code',
            'jt.trans_no' => [
                'required',
                'max:15',
                'regex:/^\S(.*\S)?$/',
                Rule::unique('trans_head', 'trans_no')
            ],
            'jt.trans_date' => 'required',
        ];
        // BR-ACC-01 + BR-ACC-07. The edit path below is a query-builder update,
        // which skips the model guards, so check explicitly.
        \App\Support\Journals::assertEditable($this->idNya);
        if (! empty($this->jt['trans_date'])) {
            \App\Support\Ledger::assertOpenDate($this->jt['trans_date'], 'jt.trans_date');
        }

        if ($this->idNya) {
            $valid['jt.trans_no'] = [
                'required',
                'max:15',
                'regex:/^\S(.*\S)?$/',
                Rule::unique('trans_head', 'trans_no')->ignore($this->idNya, 'trans_no')
            ];
            $this->validate($valid);

            $division = JournalTransaction::where('trans_no', $this->idNya);
            $dtNya = TransDetail::where('trans_no', $this->idNya);
            $division->update([
                'trans_code' => $this->jt['trans_code'],
                'trans_no' => $this->jt['trans_no'],
                'trans_date' => $this->jt['trans_date'],
                'trans_update' => Carbon::now(),
                'trans_user' => Auth::user()->user_id
            ]);
            $dtNya->update(['trans_no' => $this->jt['trans_no']]);
        } else {
            if (JournalTransaction::where('trans_no', $this->jt['trans_no'])->exists()) {
                $this->addError('jt.trans_no', 'Nomor Transaksi sudah ada. Silakan gunakan nomor lain.');
                return;
            }

            $this->validate($valid);
            JournalTransaction::create([
                'trans_code' => $this->jt['trans_code'],
                'trans_no' => $this->jt['trans_no'],
                'trans_date' => $this->jt['trans_date'],
                'trans_update' => Carbon::now(),
                'trans_user' => Auth::user()->user_id
            ]);
        }

        $this->detail(Crypt::encrypt($this->jt['trans_no']));

        $this->showToast('Data saved successfully');
    }

    public function confirmDelete($id)
    {
        $this->idNya = Crypt::decrypt($id);
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
        $datane = JournalTransaction::with('transDetail')->where('trans_no', $this->idNya)->first();

        if ($datane && $datane->transDetail->isNotEmpty()) {
            foreach ($datane->transDetail as $detail) {
                $detail->delete();
            }
            $datane->delete();
        } else {
            $datane->delete();
        }

        $this->clear();
        $this->showToast('Data deleted successfully');
    }

    public function clear()
    {
        $this->idNya = null;
        $this->form = false;
        $this->jt = [
            'trans_code' => '',
            'trans_no' => '',
            'trans_date' => '',
            'div_code' => '',
            'div_name' => '',
        ];
    }
}
