<?php

namespace Database\Seeders;

use App\Models\ThisHead;
use Illuminate\Database\Console\Seeds\WithoutModelEvents;
use Illuminate\Database\Seeder;
use Illuminate\Support\Facades\DB;

class ThisHeadSeeder extends Seeder
{
    /**
     * Run the database seeds.
     */
    public function run(): void
    {
        // Ambil data dari database lama
        $oldData = DB::connection('dblama')->table('this_head')->get();

        $count = 0; // Inisialisasi counter
        $batchSize = 1000; // Jumlah data per batch

        // Masukkan data ke database baru
        foreach ($oldData as $data) {
            // Gabungkan data lama dengan data tambahan
            $dataArray = (array) $data;

            // Insert data ke database baru
            ThisHead::insert($dataArray);

            // Update counter
            $count++;

            // Log setiap 1000 baris
            if ($count % $batchSize === 0) {
                echo "Telah memasukkan $count baris data.\n";
            }
        }

        // Log sisa data jika kurang dari 1000
        if ($count % $batchSize !== 0) {
            echo "Telah memasukkan $count baris data.\n";
        }
    }
}
