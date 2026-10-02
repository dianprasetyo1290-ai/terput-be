@extends('layouts.auth')

@section('title', 'Sign In')

@section('content')
@if ($embed ?? false)
    {{-- Di dalam iframe Sekoci: sesi habis. Jangan tampilkan form login kosong. --}}
    <div class="min-h-screen flex items-center justify-center p-8 bg-transparent">
        <div class="max-w-sm text-center">
            <h2 class="text-lg font-semibold text-gray-900">Sesi Accounting berakhir</h2>
            <p class="mt-2 text-sm text-gray-500">Menyambungkan ulang dengan akun Sekoci Anda...</p>
            <button type="button" id="sekoci-reauth"
                    class="mt-5 rounded-lg bg-blue-600 px-4 py-2.5 text-sm font-semibold text-white hover:bg-blue-700">
                Sambungkan ulang
            </button>
            <p id="sekoci-reauth-error" class="mt-3 text-sm text-red-600 hidden">
                Gagal menyambung. Muat ulang halaman Sekoci lalu coba lagi.
            </p>
        </div>
    </div>
    <script>
        (function () {
            var btn = document.getElementById('sekoci-reauth');
            var err = document.getElementById('sekoci-reauth-error');
            function reauth() {
                err.classList.add('hidden');
                // Shell Sekoci satu origin dengan kita, jadi tokennya bisa dibaca langsung.
                var token = null;
                try { token = window.parent.localStorage.getItem('sekoci_token'); } catch (e) {}
                try { window.parent.postMessage({ type: 'sekoci-accounting-reauth' }, window.location.origin); } catch (e) {}
                if (!token) { err.classList.remove('hidden'); return; }
                fetch(@json(route('sso')), {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json', 'Accept': 'application/json' },
                    credentials: 'same-origin',
                    body: JSON.stringify({ token: token }),
                }).then(function (r) {
                    if (r.ok) { window.location.reload(); } else { err.classList.remove('hidden'); }
                }).catch(function () { err.classList.remove('hidden'); });
            }
            btn.addEventListener('click', reauth);
            reauth();
        })();
    </script>
@else
<div class="min-h-screen flex">
    {{-- Left Panel - Branding --}}
    <div class="hidden lg:flex lg:w-3/5 bg-gradient-to-br from-blue-900 via-blue-700 to-blue-500 relative overflow-hidden items-center justify-center">
        {{-- Decorative circles --}}
        <div class="absolute -top-20 -left-20 w-80 h-80 bg-white/5 rounded-full"></div>
        <div class="absolute top-1/3 -right-16 w-64 h-64 bg-white/5 rounded-full"></div>
        <div class="absolute -bottom-10 left-1/4 w-48 h-48 bg-white/5 rounded-full"></div>
        <div class="absolute top-10 right-1/4 w-32 h-32 bg-white/10 rounded-full"></div>

        <div class="relative z-10 text-center px-12">
            @if(get_setting_web()?->logo)
                <img src="{{ route('helper.show-picture', ['path' => get_setting_web()->logo]) }}"
                     alt="Logo" class="h-24 mx-auto mb-8">
            @else
                <img src="{{ asset('favicon.svg') }}"
                     alt="Logo" class="h-20 w-20 mx-auto mb-8">
            @endif
            <h1 class="text-white text-5xl font-bold tracking-tight mb-4">
                {{ get_setting_web()->title ?? 'Sekoci Accounting' }}
            </h1>
            <p class="text-blue-100 text-lg max-w-md mx-auto leading-relaxed">
                Accounting & Financial Management System — Sekoci
            </p>
        </div>
    </div>

    {{-- Right Panel - Login Form --}}
    <div class="w-full lg:w-2/5 flex items-center justify-center p-8 sm:p-12">
        <div class="w-full max-w-md">
            {{-- Mobile logo --}}
            <div class="lg:hidden text-center mb-8">
                @if(get_setting_web()?->logo)
                    <img src="{{ route('helper.show-picture', ['path' => get_setting_web()->logo]) }}"
                         alt="Logo" class="h-16 mx-auto mb-4">
                @else
                    <img src="{{ asset('favicon.svg') }}"
                         alt="Logo" class="h-14 w-14 mx-auto mb-4">
                @endif
                <h2 class="text-2xl font-bold text-gray-900">{{ get_setting_web()->title ?? 'Sekoci Accounting' }}</h2>
            </div>

            <div class="mb-8">
                <h2 class="text-3xl font-bold text-gray-900">Sign in</h2>
                <p class="mt-2 text-gray-500">Enter your credentials to access your account</p>
            </div>

            {{-- Error Messages --}}
            @if ($errors->any())
                <div class="mb-6 rounded-lg bg-red-50 border border-red-200 p-4">
                    <div class="flex">
                        <svg class="h-5 w-5 text-red-400 shrink-0 mt-0.5" viewBox="0 0 20 20" fill="currentColor">
                            <path fill-rule="evenodd" d="M10 18a8 8 0 100-16 8 8 0 000 16zM8.28 7.22a.75.75 0 00-1.06 1.06L8.94 10l-1.72 1.72a.75.75 0 101.06 1.06L10 11.06l1.72 1.72a.75.75 0 101.06-1.06L11.06 10l1.72-1.72a.75.75 0 00-1.06-1.06L10 8.94 8.28 7.22z" clip-rule="evenodd" />
                        </svg>
                        <div class="ml-3">
                            @foreach ($errors->all() as $error)
                                <p class="text-sm text-red-700">{{ $error }}</p>
                            @endforeach
                        </div>
                    </div>
                </div>
            @endif

            <form method="POST" action="{{ route('login') }}" class="space-y-5">
                @csrf

                {{-- Email --}}
                <div>
                    <label for="email" class="block text-sm font-medium text-gray-700 mb-1">Email address</label>
                    <input id="email" type="email" name="email" value="{{ old('email') }}" required autofocus
                           autocomplete="username"
                           class="block w-full rounded-lg border border-gray-300 px-4 py-3 text-gray-900 placeholder-gray-400 focus:border-blue-500 focus:ring-2 focus:ring-blue-500/20 focus:outline-none transition">
                </div>

                {{-- Password --}}
                <div>
                    <label for="password" class="block text-sm font-medium text-gray-700 mb-1">Password</label>
                    <input id="password" type="password" name="password" required
                           autocomplete="current-password"
                           class="block w-full rounded-lg border border-gray-300 px-4 py-3 text-gray-900 placeholder-gray-400 focus:border-blue-500 focus:ring-2 focus:ring-blue-500/20 focus:outline-none transition">
                </div>

                {{-- Remember Me --}}
                <div class="flex items-center justify-between">
                    <label class="flex items-center gap-2 cursor-pointer">
                        <input type="checkbox" name="remember"
                               class="h-4 w-4 rounded border-gray-300 text-blue-600 focus:ring-blue-500">
                        <span class="text-sm text-gray-600">Remember me</span>
                    </label>
                    @if (Route::has('password.request'))
                        <a href="{{ route('password.request') }}" class="text-sm text-blue-600 hover:text-blue-700 font-medium">
                            Forgot password?
                        </a>
                    @endif
                </div>

                {{-- Submit --}}
                <button type="submit"
                        class="w-full rounded-lg bg-blue-600 px-4 py-3 text-sm font-semibold text-white shadow-sm hover:bg-blue-700 focus:outline-none focus:ring-2 focus:ring-blue-500 focus:ring-offset-2 transition">
                    Sign in
                </button>
            </form>

        </div>
    </div>
</div>
@endif
@endsection
