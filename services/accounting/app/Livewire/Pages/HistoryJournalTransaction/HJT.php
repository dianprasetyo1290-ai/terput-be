<?php

namespace App\Livewire\Pages\HistoryJournalTransaction;

use Carbon\Carbon;
use Livewire\Component;
use App\Models\Currency;
use App\Models\Division;
use App\Models\ThisHead;
use App\Models\Parameter;
use App\Models\ThisDetail;
use Livewire\WithPagination;
use App\Models\ChartOfAccount;
use App\Models\TransactionType;
use Illuminate\Validation\Rule;
use Illuminate\Support\Facades\Auth;
use Illuminate\Support\Facades\Crypt;

class HJT extends Component
{
    use WithPagination;
    protected $paginationTheme = 'bootstrap';

    public $form, $idNya, $formDetail, $idDetail, $count, $countDetail, $currency, $rate_system, $idEdit, $bulan;
    public $jt = [
        'trans_code' => null,
        'trans_no' => null,
        'trans_date' => null,
    ];

    public $jtdetail = [];
    public $jteditdetail = [];
    public $search, $order = "DESC", $limit = 10, $filter = 'a';

    public function add($id = "")
    {
        $this->jt['trans_date'] = Carbon::now()->format('m-d-Y');

        if ($id) {
            $this->form = true;
            $this->idNya = Crypt::decrypt($id);
            $this->jt = ThisHead::with('transDetail')->where('trans_no', $this->idNya)->first();
            $this->jt = $this->jt->toArray();
            $this->jt['debet'] = array_sum(array_column($this->jt['trans_detail'], 'debet'));
            $this->jt['credit'] = array_sum(array_column($this->jt['trans_detail'], 'credit'));
            $this->resetValidation();
        } else {
            $this->form = true;
        }

        $this->js(<<<'JS'
            $(document).ready(function() {
                $('.select-search').select2();

                $('.daterange-single').daterangepicker({
                    singleDatePicker: true,
                });

                $('.select-search').on('change', function (e) {
                    var data = $('.select-search').select2("val");
                    console.log(data);
                    $wire.set('jt.trans_code', data);
                });

                $('.daterange-single').on('change', function (e) {
                    var data = $('.daterange-single').val();
                    console.log(data);
                    $wire.set('jt.trans_date', data);
                });
            });
        JS);
    }

    public function updatedCurrency()
    {
        $this->rate_system = Currency::find($this->currency);
        $this->rate_system = $this->rate_system->rate_system ?? '';
    }

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
            $this->jteditdetail = ThisHead::with(['transType', 'transDetail' => function ($a) use ($c) {
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
            $this->jtdetail = ThisHead::with('transType', 'transDetail')
                ->where('trans_no', $this->idDetail)->first();
            $this->jtdetail = $this->jtdetail->toArray();
            $this->count = (ThisDetail::where('trans_no', $this->jtdetail['trans_no'])->max('trans_count') ?? 0) + 1;
            $this->jtdetail['tdebet'] = array_sum(array_column($this->jtdetail['trans_detail'], 'debet'));
            $this->jtdetail['tcredit'] = array_sum(array_column($this->jtdetail['trans_detail'], 'credit'));
            $this->resetValidation();
        }
    }

    public function batal()
    {
        $this->idNya = null;
        $this->formDetail = false;
        $this->form = false;
        $this->jt = [
            'div_code' => null,
            'div_name' => null,
        ];
        $this->currency = '';
        $this->rate_system = '';
    }

    public function clear()
    {
        $this->idNya = null;
        $this->form = false;
        $this->jt = [
            'div_code' => null,
            'div_name' => null,
        ];
    }

    public function save()
    {
        $valid = [
            'jt.trans_code' => 'required|exists:mst_transaction,trans_code',
            'jt.trans_no' => ['required', 'max:15', 'regex:/^\S(.*\S)?$/', Rule::unique('this_head', 'trans_no')],
            'jt.trans_date' => 'required',
        ];
        if ($this->idNya) {
            $valid['jt.trans_no'] = ['required', 'max:15', 'regex:/^\S(.*\S)?$/',
                Rule::unique('this_head', 'trans_no')->ignore($this->idNya, 'trans_no')];
            $this->validate($valid);
            $division = ThisHead::where('trans_no', $this->idNya);
            $dtNya = ThisDetail::where('trans_no', $this->idNya);
            $division->update([
                'trans_code' => $this->jt['trans_code'],
                'trans_no' => $this->jt['trans_no'],
                'trans_date' => $this->jt['trans_date'],
                'trans_update' => Carbon::now(),
                'trans_user' => Auth::user()->user_id
            ]);
            $dtNya->update(['trans_no' => $this->jt['trans_no']]);
        } else {
            $this->validate($valid);
            ThisHead::create([
                'period' => Parameter::first()->period,
                'trans_code' => $this->jt['trans_code'],
                'trans_no' => $this->jt['trans_no'],
                'trans_date' => $this->jt['trans_date'],
                'trans_update' => Carbon::now(),
                'trans_user' => Auth::user()->user_id
            ]);
        }

        $this->detail(Crypt::encrypt($this->jt['trans_no']));
        // $this->reset();

        $this->js(<<<'JS'
            const Toast = Swal.mixin({
                toast: true,
                position: "top-end",
                showConfirmButton: false,
                timer: 2000,
                timerProgressBar: true,
                didOpen: (toast) => {
                    toast.onmouseenter = Swal.stopTimer;
                    toast.onmouseleave = Swal.resumeTimer;
                }
            });
            Toast.fire({
                icon: "success",
                title: "Data saved successfully"
            });
        JS);
    }

    public function saveDetail()
    {
        $valid = [
            'jtdetail.acc_code' => 'required',
            'jtdetail.div_code' => 'required',
            'currency' => 'required',
        ];
        if (!empty($this->jteditdetail)) {
            $td = ThisDetail::where('id', $this->idEdit)->first();
            // JANGAN ubah 'period' — baris histori harus tetap di periode
            // aslinya, kalau tidak event ThisDetail::updated akan mengoreksi
            // saldo di periode yang salah.
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
                'curr_system' => $this->rate_system,
                'curr_yes' => $this->jtdetail['curr_yes'] ?? 0,
            ]);
            $this->jteditdetail = [];
        } else {
            $this->validate($valid);
            // Baris baru harus mengikuti periode jurnal-nya, bukan periode aktif
            // (this_head.period nullable secara skema, fallback ke periode aktif).
            $journalPeriod = $this->jtdetail['period'] ?? Parameter::first()->period;
            $al = TransactionType::with('akunLawan')->where('trans_code', $this->jtdetail['trans_code'])->first();
            if ($al->akunLawan) {
                ThisDetail::create([
                    'period' => $journalPeriod,
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
                    'curr_system' => $this->rate_system,
                    'curr_yes' => $this->jtdetail['curr_yes'] ?? 0,
                ]);

                $lawan = TransactionType::where('trans_code', $this->jtdetail['trans_code'])->first();

                ThisDetail::create([
                    'period' => $journalPeriod,
                    'trans_code' => $this->jtdetail['trans_code'],
                    'trans_no' => $this->jtdetail['trans_no'],
                    'trans_count' => $this->count + 1,
                    'acc_code' => $lawan->trans_account,
                    'div_code' => $this->jtdetail['div_code'],
                    'description' => $this->jtdetail['description'],
                    'debet' => $this->jtdetail['creditnew'] ?? 0,
                    'credit' => $this->jtdetail['debetnew'] ?? 0,
                    'curr_code' => $this->currency,
                    'curr_rate' => $this->jtdetail['curr_rate'] ?? 0,
                    'curr_debet' => $this->jtdetail['curr_debet'] ?? 0,
                    'curr_credit' => $this->jtdetail['curr_credit'] ?? 0,
                    'trans_date' => $this->jtdetail['trans_date'],
                    'curr_system' => $this->rate_system,
                    'curr_yes' => $this->jtdetail['curr_yes'] ?? 0,
                ]);
            } else {
                ThisDetail::create([
                    'period' => $journalPeriod,
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
                    'curr_system' => $this->rate_system,
                    'curr_yes' => $this->jtdetail['curr_yes'] ?? 0,
                ]);
            }
        }
        $this->detail(Crypt::encrypt($this->jtdetail['trans_no']));
        $this->currency = '';
        $this->rate_system = '';
        $this->js(<<<'JS'
            const Toast = Swal.mixin({
                toast: true,
                position: "top-end",
                showConfirmButton: false,
                timer: 2000,
                timerProgressBar: true,
                didOpen: (toast) => {
                    toast.onmouseenter = Swal.stopTimer;
                    toast.onmouseleave = Swal.resumeTimer;
                }
            });
            Toast.fire({
                icon: "success",
                title: "Data saved successfully"
            });
        JS);
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

    public function delete()
    {
        $datane = ThisHead::with('transDetail')->where('trans_no', $this->idNya)->first();

        if ($datane && $datane->transDetail->isNotEmpty()) {
            // Hapus tiap baris detail via Eloquent. Event ThisDetail::deleted
            // mengoreksi mst_saldo_news per-baris, per-periode, per-akun —
            // tidak boleh dikurangi manual di sini (akan double-counting &
            // salah periode/akun).
            $datane->transDetail->each(function ($transDetail) {
                $transDetail->delete();
            });

            $datane->delete();
        }
        $this->clear();
        $this->js(<<<'JS'
            const Toast = Swal.mixin({
            toast: true,
            position: "top-end",
            showConfirmButton: false,
            timer: 2000,
            timerProgressBar: true,
            didOpen: (toast) => {
                toast.onmouseenter = Swal.stopTimer;
                toast.onmouseleave = Swal.resumeTimer;
            }
            });
            Toast.fire({
            icon: "success",
            title: "Data deleted successfully"
            });
        JS);
    }

    public function deleteD()
    {
        $detail = ThisDetail::where('id', $this->countDetail)->first();
        if ($detail) {
            $detail->delete();
        }
        $this->jtdetail = [];

        $this->js(<<<'JS'
            const Toast = Swal.mixin({
                toast: true,
                position: "top-end",
                showConfirmButton: false,
                timer: 2000,
                timerProgressBar: true,
                didOpen: (toast) => {
                    toast.onmouseenter = Swal.stopTimer;
                    toast.onmouseleave = Swal.resumeTimer;
                }
            });
            Toast.fire({
                icon: "success",
                title: "Data deleted successfully"
            });
            JS);
        $this->detail(Crypt::encrypt($this->idDetail));
    }

    public function sort($order)
    {
        $this->order = $order;
    }

    public function filt($filt)
    {
        $this->filter = $filt;
    }

    public function limits($limit)
    {
        $this->limit = $limit;
    }

    public function month($bln)
    {
        $this->bulan = $bln;
    }

    public function render()
    {
        $currentYear = (int) (Parameter::first()?->year ?? Carbon::now()->year);

        $startDate = Carbon::create($currentYear, 1, 1);

        $numberOfMonths = 12;

        $monthsAndYears = [];
        for ($i = 0; $i < $numberOfMonths; $i++) {
            $monthYear = $startDate->copy()->addMonths($i);
            $monthsAndYears[] = [
                'month' => $monthYear->format('F'),
                'year' => $monthYear->format('Y')
            ];
        }

        $monthsAndYears[] = [
            'month' => 'Adj',
            'year' =>  $startDate->format('Y')
        ];

        $query = ThisHead::with('transDetail')
            ->whereYear('this_head.trans_date', $currentYear);

        if ($this->filter === 'b') {
            $query->withEqualDebetAndCredit();
        } elseif ($this->filter === 'u') {
            $query->withNotEqualDebetAndCredit();
        }

        if ($this->bulan) {
            $query->where('this_head.period', $this->bulan);
        }

        if ($this->search) {
            $query->where(function ($q) {
                $q->where('this_head.trans_code', 'ilike', '%' . $this->search . '%')
                    ->orWhere('this_head.trans_no', 'ilike', '%' . $this->search . '%');
            });
        }

        if ($this->order) {
            $query->orderBy('this_head.trans_date', $this->order);
        }

        $data = $query->paginate($this->limit);
        $transaction_code = TransactionType::all();
        $acc = ChartOfAccount::orderBy('acc_name', 'ASC')->get();
        $div = Division::all();
        $cur = Currency::all();

        return view('livewire.pages.history-journal-transaction.h-j-t', [
            'data' => $data,
            'acc' => $acc,
            'div' => $div,
            'cur' => $cur,
            'monthsAndYears' => $monthsAndYears,
            'transaction_code' => $transaction_code,
        ]);
    }
}
