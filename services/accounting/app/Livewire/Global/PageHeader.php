<?php

namespace App\Livewire\Global;

use Livewire\Component;

class PageHeader extends Component
{
    public $judul = "";
    public $subjudul = "";
    public $breadcrumb = [];
    public function render()
    {
        return view('livewire.global.page-header');
    }
}
