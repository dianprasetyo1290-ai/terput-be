<?php

namespace App\Livewire\Pages\User\Partials;

use App\Models\User;
use Livewire\Component;
use Illuminate\Validation\ValidationException;

class UpdateProfileInformationForm extends Component
{
    public $user_name, $email;
    public function update()
    {
        $this->validate([
            'user_name' => 'required|string|max:255',
            'email' => 'required|email|max:255',
        ]);

        $existingUser = \App\Models\User::where('email', $this->email)
            ->where('user_id', '!=', auth()->user()->user_id)
            ->first();

        if ($existingUser) {
            session()->flash('status', 'email-taken');
            throw ValidationException::withMessages([
                'email' => 'The email has already been taken.',
            ]);
        }

        $user = auth()->user();
        $user->user_name = $this->user_name;
        $user->email = $this->email;
        $user->save();

        redirect()->route('profile')->with('status', 'profile-updated');
    }
    public function mount()
    {
        $this->user_name = auth()->user()->user_name;
        $this->email = auth()->user()->email;
    }
    public function render()
    {
        return view('livewire.pages.user.partials.update-profile-information-form');
    }
}
