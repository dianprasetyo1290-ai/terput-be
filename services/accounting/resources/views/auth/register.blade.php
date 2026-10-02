@extends('layouts.auth')

@section('title', 'Create Account')

@section('content')
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

    {{-- Right Panel - Register Form --}}
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
                <h2 class="text-3xl font-bold text-gray-900">Create account</h2>
                <p class="mt-2 text-gray-500">Fill in the details below to get started</p>
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

            <form method="POST" action="{{ route('register') }}" class="space-y-5">
                @csrf

                {{-- Name --}}
                <div>
                    <label for="name" class="block text-sm font-medium text-gray-700 mb-1">Full name</label>
                    <input id="name" type="text" name="name" value="{{ old('name') }}" required autofocus
                           class="block w-full rounded-lg border border-gray-300 px-4 py-3 text-gray-900 placeholder-gray-400 focus:border-blue-500 focus:ring-2 focus:ring-blue-500/20 focus:outline-none transition">
                </div>

                {{-- Email --}}
                <div>
                    <label for="email" class="block text-sm font-medium text-gray-700 mb-1">Email address</label>
                    <input id="email" type="email" name="email" value="{{ old('email') }}" required
                           class="block w-full rounded-lg border border-gray-300 px-4 py-3 text-gray-900 placeholder-gray-400 focus:border-blue-500 focus:ring-2 focus:ring-blue-500/20 focus:outline-none transition">
                </div>

                {{-- Password --}}
                <div>
                    <label for="password" class="block text-sm font-medium text-gray-700 mb-1">Password</label>
                    <input id="password" type="password" name="password" required
                           class="block w-full rounded-lg border border-gray-300 px-4 py-3 text-gray-900 placeholder-gray-400 focus:border-blue-500 focus:ring-2 focus:ring-blue-500/20 focus:outline-none transition">
                </div>

                {{-- Confirm Password --}}
                <div>
                    <label for="password_confirmation" class="block text-sm font-medium text-gray-700 mb-1">Confirm password</label>
                    <input id="password_confirmation" type="password" name="password_confirmation" required
                           class="block w-full rounded-lg border border-gray-300 px-4 py-3 text-gray-900 placeholder-gray-400 focus:border-blue-500 focus:ring-2 focus:ring-blue-500/20 focus:outline-none transition">
                </div>

                {{-- Submit --}}
                <button type="submit"
                        class="w-full rounded-lg bg-blue-600 px-4 py-3 text-sm font-semibold text-white shadow-sm hover:bg-blue-700 focus:outline-none focus:ring-2 focus:ring-blue-500 focus:ring-offset-2 transition">
                    Create account
                </button>
            </form>

            {{-- Login Link --}}
            <p class="mt-8 text-center text-sm text-gray-500">
                Already have an account?
                <a href="{{ route('login') }}" class="font-semibold text-blue-600 hover:text-blue-700">
                    Sign in
                </a>
            </p>
        </div>
    </div>
</div>
@endsection
