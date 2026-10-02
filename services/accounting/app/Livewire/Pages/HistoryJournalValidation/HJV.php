<?php

namespace App\Livewire\Pages\HistoryJournalValidation;

use App\Models\Period;
use Livewire\Component;
use App\Models\Parameter;

class HJV extends Component
{
    public function render()
    {
        // Ambil nilai yang dicari
        $searchValue = Parameter::first()->period - 1;
        // Format nilai dengan menambahkan nol di depan jika kurang dari 10
        $formattedSearchValue = str_pad($searchValue, 2, '0', STR_PAD_LEFT);

        // Daftar kolom yang ingin dicari
        $columns = [];
        for ($i = 1; $i <= $formattedSearchValue; $i++) {
            $columns[] = 'p' . str_pad($i, 2, '0', STR_PAD_LEFT);
        }

        // Query dengan Eloquent untuk memilih kolom-kolom
        $results = Period::select($columns)
            ->where('period_name', 'ilike', '%' . Parameter::first()->year . '%')->get();

        // Ambil data dari hasil query
        $data = $results->map(function ($item) use ($columns) {
            return $item->only($columns);
        });

        return view('livewire.pages.history-journal-validation.h-j-v', [
            'data' => $data,
        ]);
    }
}
