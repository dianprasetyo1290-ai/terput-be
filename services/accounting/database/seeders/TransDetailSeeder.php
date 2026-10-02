<?php

namespace Database\Seeders;

use App\Models\TransDetail;
use Illuminate\Database\Console\Seeds\WithoutModelEvents;
use Illuminate\Database\Seeder;
use Illuminate\Support\Facades\DB;

class TransDetailSeeder extends Seeder
{
    /**
     * Run the database seeds.
     */
    public function run(): void
    {
        // Ambil data dari database lama
        $oldData = DB::connection('dblama')->table('trans_detail')->get();
        // Masukkan data ke database baru
        foreach ($oldData as $data) {
            TransDetail::insert(array_merge(
                (array) $data, // Data yang diambil dari database lama
            ));
        }
    }
}
