<!DOCTYPE html>
<html lang="en">

<head>
    <meta charset="utf-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0, user-scalable=0">
    <meta name="description" content="{{ get_setting_web()->meta_description }}">
    <meta name="keywords" content="{{ get_setting_web()->meta_keyword }}">
    <meta name="author" content="">
    <title>{{ title_web() }} | {{ get_setting_web()->title ?? 'Sekoci Accounting' }}</title>

    <!-- Favicon -->
    <link rel="shortcut icon" type="image/svg+xml"
        href="{{ get_setting_web()->favicon ? route('helper.show-picture', ['path' => get_setting_web()->favicon]) : asset('favicon.svg') }}">

    @include('layouts.partials.head')
    @vite(['resources/css/app.css', 'resources/js/app.js'])
    @vite(['resources/css/sekoci-theme.css'])
    @livewireStyles

    @if ($embed ?? false)
        {{-- Embedded di iframe shell Sekoci: chrome milik Sekoci, bukan milik kita --}}
        <style>
            html, body { background: transparent !important; }
            .page-content { padding: 0 !important; }
            .content-wrapper { margin-left: 0 !important; background: transparent !important; }
        </style>
    @endif
</head>

<body @class(['sekoci-embed' => $embed ?? false])>
@unless ($embed ?? false)
    @include('layouts.partials.header')
@endunless
<div class="page-content">
    @unless ($embed ?? false)
        @include('layouts.partials.sidebar')
    @endunless
    <div class="content-wrapper">
        {{ $header ?? '' }}
        @yield('content')
        <div class="content">
            {{ $slot ?? '' }}
        </div>
        <div class="navbar navbar-expand-lg navbar-light @if ($embed ?? false) d-none @endif">
            <div class="text-center d-lg-none w-100">
                <button type="button" class="navbar-toggler dropdown-toggle" data-toggle="collapse"
                    data-target="#navbar-footer">
                    <i class="mr-2 icon-unfold"></i>
                    Footer
                </button>
            </div>

            <div class="navbar-collapse collapse" id="navbar-footer">
                <span class="navbar-text">
                    &copy; {{ date('Y') }}. <a href="#" target="_blank">V.1.1.20</a>
                </span>

                <ul class="navbar-nav ml-lg-auto">

                </ul>
            </div>
        </div>
    </div>
</div>
@include('layouts.partials.footer-scripts')
<script src="https://cdn.jsdelivr.net/npm/cleave.js@1.6.0/dist/cleave.min.js"></script>
@stack('scripts')
@livewireScripts
</body>

</html>
