<?php

namespace App\Models;

use Illuminate\Support\Facades\Log;
use Illuminate\Database\Eloquent\Model;
use Illuminate\Database\Eloquent\Factories\HasFactory;

class TransDetail extends Model
{
    use HasFactory;

    protected $table = 'trans_detail';
    protected $guarded = [];
    public $incrementing = true;

    public function head()
    {
        return $this->belongsTo(JournalTransaction::class, 'trans_no', 'trans_no');
    }

    protected static function booted()
    {
        // BR-ACC-01: lines of a pending/posted/reversed journal are frozen.
        // Guarded here, in the model, so every caller (Livewire, API) hits it.
        $guard = fn ($d) => \App\Support\Journals::assertEditable($d->getOriginal('trans_no') ?? $d->trans_no, 'jtdetail.debetnew');
        static::creating($guard);
        static::updating($guard);
        static::deleting($guard);

        static::created(function ($transDetail) {
            $period = (int) Parameter::first()->period;
            $year = (int) Parameter::first()->year;

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
            $period = (int) Parameter::first()->period;
            $year = (int) Parameter::first()->year;

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

    public static function updateBalance($transDetail)
    {
        $period = (int)Parameter::first()->period;
        $year = (int)Parameter::first()->year;

        $balance = Balance::where('acc_code', $transDetail->acc_code)
            ->where('div_code', $transDetail->div_code)
            ->where('periode', $period)
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
        $transHead = JournalTransaction::where('trans_no', $transNo)->first();
        if ($transHead) {
            $totalDebet = TransDetail::where('trans_no', $transNo)->sum('debet');
            $totalCredit = TransDetail::where('trans_no', $transNo)->sum('credit');

            $transHead->debet = $totalDebet;
            $transHead->credit = $totalCredit;
            $transHead->save();

            Log::info("Updated Head: {$transNo}, Debet: {$totalDebet}, Credit: {$totalCredit}");
        } else {
            Log::warning("Trans Head not found for: {$transNo}");
        }
    }
}
