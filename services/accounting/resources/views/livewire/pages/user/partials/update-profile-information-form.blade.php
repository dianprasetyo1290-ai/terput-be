<div>
    <section>
        <header>
            <h2 class="h5 font-weight-bold text-dark">
                {{ __('Profile Information') }}
            </h2>

            <p class="text-muted small mt-2">
                {{ __("Update your account's profile information and email address.") }}
            </p>
        </header>

        <form wire:submit='update' class="mt-4">
            {{-- <form method="post" action="{{ route('profile.update') }}" class="mt-4"> --}}
            @csrf

            <div class="form-group">
                <label for="name" class="form-label">{{ __('Name') }}</label>
                <input type="text" id="name" name="name" class="form-control"
                    value="{{ old('name', auth()->user()->user_name) }}" required autofocus autocomplete="user_name"
                    wire:model='user_name'>
                @error('user_name')
                    <div class="text-danger small mt-1">{{ $message }}</div>
                @enderror
            </div>

            <div class="form-group">
                <label for="email" class="form-label">{{ __('Email') }}</label>
                <input type="email" id="email" name="email" class="form-control"
                    value="{{ old('email', auth()->user()->email) }}" required autocomplete="email" wire:model='email'>
                @error('email')
                    <div class="text-danger small mt-1">{{ $message }}</div>
                @enderror

            </div>

            <div class="form-group d-flex justify-content-between align-items-center">
                <button type="submit" class="btn btn-primary">{{ __('Save') }}</button>

                @if (session('status') === 'profile-updated')
                    <h3 x-data="{ show: true }" x-show="show" x-transition x-init="setTimeout(() => show = false, 2000)"
                        class="text-success small">{{ __('Saved.') }}</h3>
                @endif
            </div>
        </form>
    </section>

</div>
