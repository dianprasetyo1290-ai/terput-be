<?php

namespace Database\Seeders;

use App\Models\JournalTransaction;
use Illuminate\Database\Console\Seeds\WithoutModelEvents;
use Illuminate\Database\Seeder;
use Illuminate\Support\Facades\DB;

class TransHeadSeeder extends Seeder
{
    /**
     * Run the database seeds.
     */
    public function run(): void
    {
        // Ambil data dari database lama
        $oldData = DB::connection('dblama')->table('trans_head')->get();
        // Masukkan data ke database baru
        foreach ($oldData as $data) {
            JournalTransaction::insert(array_merge(
                (array) $data, // Data yang diambil dari database lama
            ));
        }
    }
}
