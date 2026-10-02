<?php

namespace Database\Seeders;

use App\Models\Division;
use Illuminate\Database\Console\Seeds\WithoutModelEvents;
use Illuminate\Database\Seeder;
use Illuminate\Support\Facades\DB;

class DivisionSeeder extends Seeder
{
    /**
     * Run the database seeds.
     */
    public function run(): void
    {
        // Ambil data dari database lama
        $oldData = DB::connection('dblama')->table('mst_division')->get();
        // Masukkan data ke database baru
        foreach ($oldData as $data) {
            Division::insert(array_merge(
                (array) $data, // Data yang diambil dari database lama
            ));
        }
    }
}
