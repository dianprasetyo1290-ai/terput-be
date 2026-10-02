<?php

namespace App\Livewire\Pages\Division;

use App\Models\Division as ModelsDivision;
use Illuminate\Support\Facades\Crypt;
use Livewire\Component;
use Livewire\WithPagination;

class Division extends Component
{
    use WithPagination;
    protected $paginationTheme = 'bootstrap';

    public $form, $idNya;
    public $division = [
        'div_code' => null,
        'div_name' => null,
    ];
    public $search, $order = "ASC", $limit = 10;

    public function add($id = "")
    {
        if ($id) {
            $this->form = true;
            $this->idNya = Crypt::decrypt($id);
            $this->division = ModelsDivision::find($this->idNya);
            $this->division = $this->division->toArray();
            $this->resetValidation();
        } else {
            $this->form = true;
        }
    }

    public function clear()
    {
        $this->idNya = null;
        $this->form = false;
        $this->division = [
            'div_code' => null,
            'div_name' => null,
        ];
    }

    public function sort($order)
    {
        $this->resetPage();
        $this->order = $order;
    }

    public function updatedSearch()
    {
        $this->resetPage();
    }

    public function limits($limit)
    {
        $this->resetPage();
        $this->limit = $limit;
    }

    public function save()
    {
        $valid = [
            'division.div_code' => 'required|max:5',
            'division.div_name' => 'required|max:50',
        ];
        if ($this->idNya) {
            $this->validate($valid);
            $division = ModelsDivision::find($this->idNya);
            $division->update($this->division);
        } else {
            $this->validate($valid);
            ModelsDivision::create($this->division);
        }

        $this->reset();

        $this->js(<<<'JS'
            const Toast = Swal.mixin({
                toast: true,
                position: "top-end",
                showConfirmButton: false,
                timer: 2000,
                timerProgressBar: true,
                didOpen: (toast) => {
                    toast.onmouseenter = Swal.stopTimer;
                    toast.onmouseleave = Swal.resumeTimer;
                }
            });
            Toast.fire({
                icon: "success",
                title: "Data saved successfully"
            });
        JS);
    }

    public function confirmDelete($id)
    {
        $this->idNya = Crypt::decrypt($id);
        $this->js(<<<'JS'
        Swal.fire({
                text: "Do you want to delete this data? This process is irreversible.",
                icon: 'warning',
                showDenyButton: true,
                confirmButtonColor: '#3085d6',
                confirmButtonText: 'Delete!',
                denyButtonText: 'Cancel'
          }).then((result) => {
            if (result.isConfirmed) {
                $wire.delete()
            } else if (result.isDenied) {
                $wire.clear()
            }
          })
        JS);
    }

    public function delete()
    {
        $user = ModelsDivision::find($this->idNya);
        $user->delete();
        $this->clear();
        $this->js(<<<'JS'
            const Toast = Swal.mixin({
            toast: true,
            position: "top-end",
            showConfirmButton: false,
            timer: 2000,
            timerProgressBar: true,
            didOpen: (toast) => {
                toast.onmouseenter = Swal.stopTimer;
                toast.onmouseleave = Swal.resumeTimer;
            }
            });
            Toast.fire({
            icon: "success",
            title: "Data deleted successfully"
            });
        JS);
    }

    public function render()
    {
        $data = ModelsDivision::query();
        if ($this->search) {
            $data->where('div_code', 'ilike', '%' .  $this->search . '%');
            $data->orWhere('div_name', 'ilike', '%' .  $this->search . '%');
        }
        if ($this->order) {
            $data->orderBy('div_code', $this->order);
        }
        $data = $data->paginate($this->limit);
        return view('livewire.pages.division.division', [
            'data' => $data,
        ]);
    }
}
