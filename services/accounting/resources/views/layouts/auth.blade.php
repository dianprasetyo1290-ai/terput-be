<!DOCTYPE html>
<html lang="en">

<head>
    <meta charset="utf-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <meta name="description" content="{{ get_setting_web()->meta_description ?? '' }}">
    <meta name="keywords" content="{{ get_setting_web()->meta_keyword ?? '' }}">
    <title>@yield('title', 'Login') | {{ get_setting_web()->title ?? 'Sekoci Accounting' }}</title>

    <link rel="shortcut icon" type="image/svg+xml"
        href="{{ get_setting_web()->favicon ? route('helper.show-picture', ['path' => get_setting_web()->favicon]) : asset('favicon.svg') }}">

    @vite(['resources/css/app.css', 'resources/js/app.js'])
    @vite(['resources/css/sekoci-theme.css'])
    @livewireStyles
</head>

<body class="min-h-screen bg-white">
    @yield('content')
    @livewireScripts
</body>

</html>
