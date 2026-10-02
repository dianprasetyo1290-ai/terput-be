<?php

namespace App\Livewire\Pages\User\Partials;

use Illuminate\Support\Facades\Auth;
use Livewire\Component;

class DeleteUserForm extends Component
{
    public $password;
    public function destroy()
    {
        $this->validate([
            'password' => ['required', 'current_password'],
        ]);

        $user = Auth::user();

        Auth::guard('web')->logout();

        $user->delete();

        session()->invalidate();
        session()->regenerateToken();

        return redirect('/');
    }
    public function render()
    {
        return view('livewire.pages.user.partials.delete-user-form');
    }
}
