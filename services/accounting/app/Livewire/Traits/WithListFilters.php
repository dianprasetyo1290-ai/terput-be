<?php

namespace App\Livewire\Traits;

trait WithListFilters
{
    public function updatedSearch()
    {
        $this->resetPage();
    }

    public function sort($order)
    {
        $this->resetPage();
        $this->order = $order;
    }

    public function filt($filt)
    {
        $this->resetPage();
        $this->filter = $filt;
    }

    public function limits($limit)
    {
        $this->resetPage();
        $this->limit = $limit;
    }
}
