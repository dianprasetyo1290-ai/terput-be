<div>
    <x-slot name="header">
        <livewire:global.page-header judul="Master" subjudul="Manage your Company Parameter" :breadcrumb="['Master', 'Company Parameter']" />
    </x-slot>

    <div class="card">
        <div class="card-header header-elements-inline">
            <h5 class="card-title">Company Parameter Data</h5>
            <div class="header-elements">
                <div class="list-icons">
                    <a class="list-icons-item" data-action="collapse"></a>
                    <a class="list-icons-item" data-action="reload"></a>
                    <a class="list-icons-item" data-action="remove"></a>
                </div>
            </div>

        </div>
        <div class="card-body">
            <form action="#" wire:submit="save">
                @csrf
                <fieldset class="mb-3">
                    <div class="form-group row">
                        <label class="col-form-label col-lg-2">{{ __('Company Name') }}</label>
                        <div class="col-lg-10">
                            <input @if (!$edit) readonly @endif type="text"
                                class="form-control @error('cp.company') is-invalid  @enderror" wire:model="cp.company">
                            @error('cp.company')
                                <div class="invalid-feedback">
                                    {{ $errors->first('cp.company') }}
                                </div>
                            @enderror
                        </div>
                    </div>

                    <div class="form-group row">
                        <label class="col-form-label col-lg-2">{{ __('Serial Number') }}</label>
                        <div class="col-lg-10">
                            <input @if (!$edit) readonly @endif type="text"
                                class="form-control @error('cp.serial_no') is-invalid  @enderror"
                                wire:model="cp.serial_no">
                            @error('cp.serial_no')
                                <div class="invalid-feedback">
                                    {{ $errors->first('cp.serial_no') }}
                                </div>
                            @enderror
                        </div>
                    </div>

                    <div class="form-group row">
                        <label class="col-form-label col-lg-2">{{ __('Address') }}</label>
                        <div class="col-lg-10">
                            <input @if (!$edit) readonly @endif type="text"
                                class="form-control @error('cp.address') is-invalid  @enderror"
                                wire:model="cp.address">
                            @error('cp.address')
                                <div class="invalid-feedback">
                                    {{ $errors->first('cp.address') }}
                                </div>
                            @enderror
                        </div>
                    </div>

                    <div class="form-group row">
                        <label class="col-form-label col-lg-2">{{ __('Contact') }}</label>
                        <div class="col-lg-10">
                            <input @if (!$edit) readonly @endif type="text"
                                class="form-control @error('cp.contact') is-invalid  @enderror"
                                wire:model="cp.contact">
                            @error('cp.contact')
                                <div class="invalid-feedback">
                                    {{ $errors->first('cp.contact') }}
                                </div>
                            @enderror
                        </div>
                    </div>

                    <div class="form-group row">
                        <label class="col-form-label col-lg-2">{{ __('Tax Number') }}</label>
                        <div class="col-lg-10">
                            <input @if (!$edit) readonly @endif type="text"
                                class="form-control @error('cp.tax_number') is-invalid  @enderror"
                                wire:model="cp.tax_number">
                            @error('cp.tax_number')
                                <div class="invalid-feedback">
                                    {{ $errors->first('cp.tax_number') }}
                                </div>
                            @enderror
                        </div>
                    </div>

                    <div class="form-group row">
                        <label class="col-form-label col-lg-2">{{ __('Current Period') }}</label>
                        <div class="col-lg-10">
                            <input @if (!$edit) readonly @endif type="text"
                                class="form-control @error('cp.period') is-invalid  @enderror" wire:model="cp.period">
                            @error('cp.period')
                                <div class="invalid-feedback">
                                    {{ $errors->first('cp.period') }}
                                </div>
                            @enderror
                        </div>
                    </div>

                    <div class="form-group row">
                        <label class="col-form-label col-lg-2">{{ __('Current Year') }}</label>
                        <div class="col-lg-10">
                            <input @if (!$edit) readonly @endif type="text"
                                class="form-control @error('cp.year') is-invalid  @enderror" wire:model="cp.year">
                            @error('cp.year')
                                <div class="invalid-feedback">
                                    {{ $errors->first('cp.year') }}
                                </div>
                            @enderror
                        </div>
                    </div>

                    <div class="form-group row">
                        <label class="col-form-label col-lg-2">{{ __('Mask Account') }}</label>
                        <div class="col-lg-10">
                            <input @if (!$edit) readonly @endif type="text"
                                class="form-control @error('cp.mask_account') is-invalid  @enderror"
                                wire:model="cp.mask_account">
                            @error('cp.mask_account')
                                <div class="invalid-feedback">
                                    {{ $errors->first('cp.mask_account') }}
                                </div>
                            @enderror
                        </div>
                    </div>

                    <div class="form-group row">
                        <label class="col-form-label col-lg-2">{{ __('Base Currency') }}</label>
                        <div class="col-lg-10">
                            <input @if (!$edit) readonly @endif type="text"
                                class="form-control @error('cp.base_currency') is-invalid  @enderror"
                                wire:model="cp.base_currency">
                            @error('cp.base_currency')
                                <div class="invalid-feedback">
                                    {{ $errors->first('cp.base_currency') }}
                                </div>
                            @enderror
                        </div>
                    </div>

                    <div class="form-group row" wire:ignore>
                        <label class="col-form-label col-lg-2">{{ __('Control Period Date') }}</label>
                        <div class="col-lg-10 d-flex align-items-center">
                            <input id="check" type="checkbox" class="form-check-input-styled"
                                wire:model="cp.control_date" data-fouc>
                        </div>
                    </div>
                </fieldset>

                @if ($edit)
                    <div class="card-footer bg-white d-flex justify-content-between align-items-center py-2">
                        <ul class="list-inline mb-0">
                            <li class="list-inline-item">
                                <div wire:loading.remove wire:target='btnBatal'>
                                    <button type="button" class="btn btn-danger" wire:click="btnBatal">Cancel</button>
                                </div>
                                <div wire:loading wire:target="btnBatal">
                                    <button type="button" class="btn bg-slate-600" disabled>Canceling...<i
                                            class="icon-spinner2 spinner"></i></button>
                                </div>
                            </li>
                        </ul>
                        @can('createcompany_parameter')
                            <ul class="list-inline mb-0">
                                <li class="list-inline-item">
                                    <div wire:loading.remove wire:target='save'>
                                        <button type="submit" class="btn btn-primary text-right">Save changes</button>
                                    </div>
                                    <div wire:loading wire:target="save">
                                        <button type="button" class="btn bg-slate-600" disabled>Saving...<i
                                                class="icon-spinner2 spinner"></i></button>
                                    </div>
                                </li>
                            </ul>
                        @endcan
                    </div>
                @else
                    @can('editcompany_paramater')
                        <div class="card-footer bg-white d-flex justify-content-end align-items-center py-2">
                            <ul class="list-inline mb-0">
                                <li class="list-inline-item">
                                    <div wire:loading.remove wire:target='btnedit'>
                                        <button type="button" class="btn btn-secondary"
                                            wire:click="btnedit">Edit</button>
                                    </div>
                                    <div wire:loading wire:target="btnedit">
                                        <button type="button" class="btn bg-slate-600" disabled>Open...<i
                                                class="icon-spinner2 spinner"></i></button>
                                    </div>
                                </li>
                            </ul>
                        </div>
                    @endcan
                @endif

            </form>
        </div>
    </div>
</div>
