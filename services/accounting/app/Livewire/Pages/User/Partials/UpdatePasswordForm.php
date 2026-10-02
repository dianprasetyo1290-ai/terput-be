<?php

namespace App\Livewire\Pages\User\Partials;

use Illuminate\Support\Facades\Hash;
use Illuminate\Validation\Rules\Password;
use Livewire\Component;

class UpdatePasswordForm extends Component
{
    public $current_password;
    public $password;
    public $password_confirmation;

    public function updatePassword()
    {
        $validatedData = $this->validate([
            'current_password' => ['required', 'current_password'],
            'password' => ['required', Password::defaults(), 'confirmed'],
        ]);

        auth()->user()->update([
            'password' => Hash::make($validatedData['password']),
        ]);
        redirect()->route('profile')->with('status', 'password-updated');
    }


    public function render()
    {
        return view('livewire.pages.user.partials.update-password-form');
    }
}
