<div>
    <section>
        <header>
            <h2 class="h5 font-weight-bold text-dark">
                {{ __('Update Password') }}
            </h2>

            <p class="text-muted small mt-2">
                {{ __('Ensure your account is using a long, random password to stay secure.') }}
            </p>
        </header>

        <form wire:submit.prevent="updatePassword" class="mt-4">
            <div class="form-group">
                <label for="update_password_current_password" class="form-label">{{ __('Current Password') }}</label>
                <input id="update_password_current_password" name="current_password" type="password" class="form-control"
                    autocomplete="current-password" wire:model="current_password">
                @error('current_password', 'updatePassword')
                    <div class="text-danger small mt-1">{{ $message }}</div>
                @enderror
            </div>

            <div class="form-group">
                <label for="update_password_password" class="form-label">{{ __('New Password') }}</label>
                <input id="update_password_password" name="password" type="password" class="form-control"
                    autocomplete="new-password" wire:model="password">
                @error('password', 'updatePassword')
                    <div class="text-danger small mt-1">{{ $message }}</div>
                @enderror
            </div>

            <div class="form-group">
                <label for="update_password_password_confirmation"
                    class="form-label">{{ __('Confirm Password') }}</label>
                <input id="update_password_password_confirmation" name="password_confirmation"
                    wire:model="password_confirmation" type="password" class="form-control" autocomplete="new-password">
                @error('password_confirmation', 'updatePassword')
                    <div class="text-danger small mt-1">{{ $message }}</div>
                @enderror
            </div>

            <div class="form-group d-flex justify-content-between align-items-center">
                <button type="submit" class="btn btn-primary">{{ __('Save') }}</button>

                @if (session('status') === 'password-updated')
                    <p x-data="{ show: true }" x-show="show" x-transition x-init="setTimeout(() => show = false, 2000)"
                        class="text-success small">{{ __('Saved.') }}</p>
                @endif
            </div>
        </form>
    </section>
</div>
