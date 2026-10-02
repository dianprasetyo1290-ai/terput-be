<?php

namespace App\Models;

use Carbon\Carbon;
use Illuminate\Database\Eloquent\Model;
use Illuminate\Database\Eloquent\Factories\HasFactory;

class ThisDetail extends Model
{
    use HasFactory;

    protected $guarded = [];
    protected $table = 'this_detail';

    private static $params;

    public function head()
    {
        return $this->belongsTo(ThisHead::class, 'trans_no', 'trans_no');
    }

    protected static function booted()
    {
        static::created(function ($transDetail) {
            $year = Carbon::parse($transDetail->trans_date)->year;
            $period = (int) $transDetail->period;

            $balance = Balance::where('acc_code', $transDetail->acc_code)
                ->where('div_code', $transDetail->div_code)
                ->where('periode', $period)
                ->where('tahun', $year)
                ->first();

            $debitField = 'debet';
            $creditField = 'credit';

            if ($balance) {
                $balance->{$debitField} = (float) $balance->{$debitField} ?? 0;
                $balance->{$creditField} = (float) $balance->{$creditField} ?? 0;

                $balance->{$debitField} += $transDetail->debet;
                $balance->{$creditField} += $transDetail->credit;

                $balance->save();
            } else {
                Balance::create([
                    'periode' => $period,
                    'tahun' => $year,
                    'acc_code' => $transDetail->acc_code,
                    'div_code' => $transDetail->div_code,
                    $debitField => (float) $transDetail->debet,
                    $creditField => (float) $transDetail->credit,
                ]);
            }

            static::updateHeadTotals($transDetail->trans_no);
        });

        static::updated(function ($transDetail) {
            $year = Carbon::parse($transDetail->trans_date)->year;
            $period = (int) $transDetail->period;

            $debitField = 'debet';
            $creditField = 'credit';

            $originalBalance = Balance::where('acc_code', $transDetail->getOriginal('acc_code'))
                ->where('div_code', $transDetail->getOriginal('div_code'))
                ->where('periode', $period)
                ->where('tahun', $year)
                ->first();

            if ($originalBalance) {
                $originalBalance->{$debitField} = (float) $originalBalance->{$debitField} ?? 0;
                $originalBalance->{$creditField} = (float) $originalBalance->{$creditField} ?? 0;

                // Kurangi nilai debet dan kredit pada akun asal
                $originalBalance->{$debitField} -= $transDetail->getOriginal('debet');
                $originalBalance->{$creditField} -= $transDetail->getOriginal('credit');

                $originalBalance->save();
            }

            $newBalance = Balance::where('acc_code', $transDetail->acc_code)
                ->where('div_code', $transDetail->div_code)
                ->where('periode', $period)
                ->where('tahun', $year)
                ->first();

            if ($newBalance) {
                $newBalance->{$debitField} = (float) $newBalance->{$debitField} ?? 0;
                $newBalance->{$creditField} = (float) $newBalance->{$creditField} ?? 0;

                // Tambahkan nilai debet dan kredit pada akun tujuan
                $newBalance->{$debitField} += $transDetail->debet;
                $newBalance->{$creditField} += $transDetail->credit;

                $newBalance->save();
            } else {
                // Jika akun tujuan belum ada, buat entri baru
                Balance::create([
                    'periode' => $period,
                    'tahun' => $year,
                    'acc_code' => $transDetail->acc_code,
                    'div_code' => $transDetail->div_code,
                    $debitField => (float) $transDetail->debet,
                    $creditField => (float) $transDetail->credit,
                ]);
            }

            static::updateHeadTotals($transDetail->trans_no);
        });

        static::deleted(function ($transDetail) {
            // Update balance FIRST (subtract deleted values from mst_saldo_news)
            static::updateBalance($transDetail);

            // Then update head totals
            static::updateHeadTotals($transDetail->trans_no);
        });
    }


    // private static function initializeParams()
    // {
    //     if (!Cokrams) {
    //         $params = Parameter::first();
    //         Cokrams = [
    //             'period' => (int) $params->period,
    //             'year' => (int) $params->year,
    //         ];
    //     }
    // }

    public static function updateBalance($transDetail)
    {
        // self::initializeParams();
        $year = Carbon::parse($transDetail->trans_date)->year;

        $balance = Balance::where('acc_code', $transDetail->acc_code)
            ->where('div_code', $transDetail->div_code)
            ->where('periode', $transDetail->period)
            ->where('tahun', $year)
            ->first();

        if ($balance) {
            $debitField = 'debet';
            $creditField = 'credit';
            // dd($balance->{$debitField} . ' | ' . $transDetail->debet . ' | ' . $transDetail);
            $balance->{$debitField} = (float) $balance->{$debitField} ?? 0;
            $balance->{$creditField} = (float) $balance->{$creditField} ?? 0;

            $balance->{$debitField} -= $transDetail->debet;
            $balance->{$creditField} -= $transDetail->credit;
            $balance->save();
        }
    }

    public static function updateHeadTotals($transNo)
    {
        $transHead = ThisHead::where('trans_no', $transNo)->first();
        if ($transHead) {
            $totalDebet = ThisDetail::where('trans_no', $transNo)->sum('debet');
            $totalCredit = ThisDetail::where('trans_no', $transNo)->sum('credit');

            $transHead->debet = $totalDebet;
            $transHead->credit = $totalCredit;
            $transHead->save();
        }
    }
}
