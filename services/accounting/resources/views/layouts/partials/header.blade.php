<div class="navbar navbar-expand-md navbar-dark" style="min-height: 60px;">
    <div class="navbar-brand" style="padding: 0.5rem 1.25rem;">
        <a href="{{ url('/home') }}" class="d-inline-flex align-items-center" style="gap: 10px; text-decoration: none;">
            <img src="{{ get_setting_web()?->logo_sidebar
                ? route('helper.show-picture', ['path' => get_setting_web()?->logo_sidebar])
                : asset('favicon.svg') }}"
                alt="logo" style="height: 34px; max-width: 34px; object-fit: contain;">
            <span style="font-weight: 700; font-size: 1.05rem; letter-spacing: 0.3px;">{{ get_setting_web()->title ?? 'Sekoci Accounting' }}</span>
        </a>
    </div>

    <div class="d-md-none">
        <button class="navbar-toggler" type="button" data-toggle="collapse" data-target="#navbar-mobile">
            <i class="icon-tree5"></i>
        </button>
        <button class="navbar-toggler sidebar-mobile-main-toggle" type="button">
            <i class="icon-paragraph-justify3"></i>
        </button>
    </div>

    <div class="collapse navbar-collapse" id="navbar-mobile">
        <ul class="navbar-nav">
            <li class="nav-item">
                <a href="#" class="navbar-nav-link sidebar-control sidebar-main-toggle d-none d-md-block">
                    <i class="icon-paragraph-justify3"></i>
                </a>
            </li>
        </ul>

        <span class="navbar-text ml-md-3 mr-md-auto">
            <span class="badge bg-success" style="font-size: 0.7rem; padding: 4px 10px; border-radius: 50px;">
                <i class="icon-checkmark3 mr-1" style="font-size: 0.6rem;"></i>Online
            </span>
        </span>

        <ul class="navbar-nav">
            <li class="nav-item dropdown dropdown-user">
                <a href="#" class="navbar-nav-link dropdown-toggle d-flex align-items-center" data-toggle="dropdown"
                   style="gap: 8px;">
                    <img src="https://ui-avatars.com/api/?name={{ auth()->user()->user_name ?? 'null' }}&background=3b82f6&color=fff&size=32"
                        class="rounded-circle" width="32" height="32" alt="">
                    <span style="font-weight: 500;">{{ Auth::user()->user_name }}</span>
                </a>

                <div class="dropdown-menu dropdown-menu-right" style="border: none; box-shadow: 0 4px 20px rgba(0,0,0,0.15); border-radius: 8px; margin-top: 8px;">
                    <a href="{{ route('profile') }}" class="dropdown-item" style="padding: 8px 20px;">
                        <i class="icon-user-plus mr-2"></i> My profile
                    </a>
                    <div class="dropdown-divider"></div>
                    <form id="logout-form" action="{{ route('logout') }}" method="POST" style="display: none;">
                        @csrf
                    </form>
                    <a onclick="event.preventDefault(); document.getElementById('logout-form').submit();"
                        href="{{ route('logout') }}" class="dropdown-item" style="padding: 8px 20px; color: #ef4444;">
                        <i class="icon-switch2 mr-2"></i> Logout
                    </a>
                </div>
            </li>
        </ul>
    </div>
</div>
