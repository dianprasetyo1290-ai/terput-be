<?php

namespace Database\Seeders;

use App\Models\Balance;
use App\Models\Parameter;
use Illuminate\Database\Console\Seeds\WithoutModelEvents;
use Illuminate\Database\Seeder;
use Illuminate\Support\Facades\DB;

class SaldoSeeder extends Seeder
{
    /**
     * Run the database seeds.
     */
    public function run(): void
    {
        $oldData = DB::connection('dblama')->table('mst_saldo')->get();
        $count = 0;
        $batchSize = 1000;

        $tahun = Parameter::first()->year;
        $insertBatch = [];

        foreach ($oldData as $data) {
            $dataArray = (array) $data;

            $this->prepareInsertData($insertBatch, $dataArray, $tahun, 'act_', 'actk_');
            $this->prepareInsertData($insertBatch, $dataArray, $tahun - 1, 'lst_', 'lstk_');
            $this->prepareInsertData($insertBatch, $dataArray, $tahun - 2, 'snd_', 'sndk_');

            if (count($insertBatch) >= $batchSize) {
                Balance::insert($insertBatch);
                $count += count($insertBatch);
                echo "Telah memasukkan $count baris data.\n";
                $insertBatch = []; // Reset the batch
            }
        }

        if (!empty($insertBatch)) {
            Balance::insert($insertBatch);
            $count += count($insertBatch);
            echo "Telah memasukkan $count baris data.\n";
        }
    }

    private function prepareInsertData(&$insertBatch, $dataArray, $tahun, $debetPrefix, $creditPrefix)
    {
        for ($periode = 0; $periode <= 13; $periode++) {
            $periodeKey = str_pad($periode, 2, '0', STR_PAD_LEFT);

            $insertBatch[] = [
                'acc_code' => $dataArray['acc_code'],
                'div_code' => $dataArray['div_code'],
                'periode' => $periode,
                'tahun' => $tahun,
                //untuk kosongan data
                // 'debet' => 0,
                // 'credit' => 0,
                //jika isi
                'debet' => $dataArray["{$debetPrefix}{$periodeKey}"] ?? 0,
                'credit' => $dataArray["{$creditPrefix}{$periodeKey}"] ?? 0,
            ];
        }
    }
}
