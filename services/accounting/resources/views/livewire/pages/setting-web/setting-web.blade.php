<div>
    <x-slot name="header">
        <livewire:global.page-header judul="Website Setting" subjudul="Manage Website Setting" :breadcrumb="['Website Setting']" />
    </x-slot>
    <div class="card" style="">

        <div class="card-body border-top">
            <form action="#" wire:submit="save">
                @csrf
                <div class="row mb-3">
                    <label class="col-lg-3 col-form-label">Title:</label>
                    <div class="col-lg-9">
                        <input type="text" class="form-control" wire:model="setting.title">
                    </div>
                </div>
                <div class="row mb-3">
                    <label class="col-lg-3 col-form-label">Slogan:</label>
                    <div class="col-lg-9">
                        <input type="text" class="form-control" wire:model="setting.slogan">
                    </div>
                </div>
                <div class="row mb-3">
                    <label class="col-lg-3 col-form-label">Site Description:</label>
                    <div class="col-lg-9">
                        <textarea rows="2" cols="2" class="form-control" placeholder="Enter text here"
                            wire:model="setting.site_description"></textarea>
                    </div>
                </div>
                <div class="row mb-3">
                    <label class="col-lg-3 col-form-label">Meta Description:</label>
                    <div class="col-lg-9">
                        <textarea rows="2" cols="2" class="form-control" placeholder="Enter text here"
                            wire:model="setting.meta_description"></textarea>
                    </div>
                </div>
                <div class="row mb-3">
                    <label class="col-lg-3 col-form-label">Meta Keyword:</label>
                    <div class="col-lg-9">
                        <textarea rows="2" cols="2" class="form-control" placeholder="Enter text here"
                            wire:model="setting.meta_keyword"></textarea>
                    </div>
                </div>
                <div class="row mb-3">
                    <label class="col-lg-3 col-form-label">Email Website:</label>
                    <div class="col-lg-9">
                        <input type="email" class="form-control" wire:model="setting.email">
                    </div>
                </div>
                <div class="row mb-3">
                    <label class="col-lg-3 col-form-label">No. Whatsapp:</label>
                    <div class="col-lg-9">
                        <input type="number" class="form-control" wire:model="setting.phone">
                    </div>
                </div>
                <div class="row mb-3">
                    <label class="col-lg-3 col-form-label">Address:</label>
                    <div class="col-lg-9">
                        <textarea rows="2" cols="2" class="form-control" placeholder="Enter text here" wire:model="setting.address"></textarea>
                    </div>
                </div>
                <div class="row mb-3">
                    <label class="col-lg-3 col-form-label">Logo Login Web:</label>
                    <div class="col-lg-9">
                        <div class="new-employee-field">
                            <div class="profile-pic-upload edit-pic">
                                <div class="profile-pic mb-2">
                                    <div wire:loading wire:target="setting.logo">
                                        <div class="text-center">
                                            <div class="spinner-border" role="status">
                                                <span class="visually-hidden">Loading...</span>
                                            </div>
                                        </div>
                                    </div>

                                    <span wire:loading.remove wire:target="setting.logo">
                                        <img src="{{ $this->getImageUrlLogo() }}" alt="User Image" width="150"
                                            height="150" class="rounded-circle">
                                    </span>
                                </div>
                                <div class="input-blocks mb-2">
                                    <input type="file" wire:model="setting.logo"
                                        accept="image/png, image/gif, image/jpeg">
                                </div>
                                @error('setting.logo')
                                    <div class="form-check form-check-inline form-check-md">
                                        <div class="error">
                                            <p class="text-danger">Gambar Wajib Di Isi <br>
                                                Format(*.jpg,*.png,*.jpeg)
                                                Max
                                                5Mb.</p>
                                        </div>
                                    </div>
                                @enderror
                            </div>
                        </div>
                    </div>
                </div>
                <div class="row mb-3">
                    <label class="col-lg-3 col-form-label">Logo Side Bar Web:</label>
                    <div class="col-lg-9">
                        <div class="new-employee-field">
                            <div class="profile-pic-upload edit-pic">
                                <div class="profile-pic mb-2">
                                    <div wire:loading wire:target="setting.logo_sidebar">
                                        <div class="text-center">
                                            <div class="spinner-border" role="status">
                                                <span class="visually-hidden">Loading...</span>
                                            </div>
                                        </div>
                                    </div>

                                    <span wire:loading.remove wire:target="setting.logo_sidebar">
                                        <img src="{{ $this->getImageUrlLogoSideBar() }}" alt="User Image"
                                            width="150" height="150" class="rounded-circle">
                                    </span>
                                </div>
                                <div class="input-blocks mb-2">
                                    <input type="file" wire:model="setting.logo_sidebar"
                                        accept="image/png, image/gif, image/jpeg">
                                </div>
                                @error('setting.logo_sidebar')
                                    <div class="form-check form-check-inline form-check-md">
                                        <div class="error">
                                            <p class="text-danger">Gambar Wajib Di Isi <br>
                                                Format(*.jpg,*.png,*.jpeg)
                                                Max
                                                5Mb.</p>
                                        </div>
                                    </div>
                                @enderror
                            </div>
                        </div>
                    </div>
                </div>
                <div class="row mb-3">
                    <label class="col-lg-3 col-form-label">Favicon Name:</label>
                    <div class="col-lg-9">
                        <input type="text" class="form-control" wire:model="setting.favicon_name">
                    </div>
                </div>
                <div class="row mb-3">
                    <label class="col-lg-3 col-form-label">Favicon File:</label>
                    <div class="col-lg-9">
                        <div class="new-employee-field">
                            <div class="profile-pic-upload edit-pic">
                                <div class="profile-pic mb-2">
                                    <div wire:loading wire:target="setting.favicon">
                                        <div class="text-center">
                                            <div class="spinner-border" role="status">
                                                <span class="visually-hidden">Loading...</span>
                                            </div>
                                        </div>
                                    </div>

                                    <span wire:loading.remove wire:target="setting.favicon">
                                        <img src="{{ $this->getImageUrlFavicon() }}" alt="User Image" width="150"
                                            height="150" class="rounded-circle">
                                    </span>
                                </div>
                                <div class="input-blocks mb-2">
                                    <input type="file" wire:model="setting.favicon"
                                        accept="image/png, image/gif, image/jpeg">
                                </div>
                                @error('setting.favicon')
                                    <div class="form-check form-check-inline form-check-md">
                                        <div class="error">
                                            <p class="text-danger">Gambar Wajib Di Isi <br>
                                                Format(*.jpg,*.png,*.jpeg)
                                                Max
                                                5Mb.</p>
                                        </div>
                                    </div>
                                @enderror
                            </div>
                        </div>
                    </div>
                </div>
                <div class="card-footer d-flex justify-content-between align-items-center py-2">

                    <ul class="list-inline mb-0">
                        <li class="list-inline-item">

                        </li>
                    </ul>

                    <ul class="list-inline mb-0">
                        <li class="list-inline-item">
                            <button type="submit" class="btn btn-primary text-right" wire:loading.remove
                                wire:target='save'>Save</button>
                            <button wire:loading wire:target='save' type="button"
                                class="btn btn-outline-light rounded-pill p-1" disabled>
                                <i class="ph-spinner spinner m-1"></i><b>Saving</b>
                            </button>
                        </li>
                    </ul>
                </div>
            </form>
        </div>
    </div>
</div>
