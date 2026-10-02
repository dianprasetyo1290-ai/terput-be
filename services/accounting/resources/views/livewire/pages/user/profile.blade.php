<div>
    <x-slot name="header">
        <livewire:global.page-header judul="User" subjudul="Manage your accounts" :breadcrumb="['User', 'Profile']" />
    </x-slot>
    <div class="content py-3">
        <div class="container-fluid">
            <div class="row">
                <!-- Update Profile Information -->
                <div class="col-md-6">
                    <div class="card">
                        <div class="card-body">
                            <livewire:pages.user.partials.update-profile-information-form />
                        </div>
                    </div>
                </div>

                <!-- Update Password -->
                <div class="col-md-6">
                    <div class="card">
                        <div class="card-body">
                            <livewire:pages.user.partials.update-password-form />
                        </div>
                    </div>
                </div>
            </div>

            <div class="row mt-3">
                <!-- Delete Account -->
                <div class="col-md-12">
                    <div class="card">
                        <div class="card-body">
                            <livewire:pages.user.partials.delete-user-form />
                        </div>
                    </div>
                </div>
            </div>
        </div>
    </div>
</div>
