<div>
    <x-slot name="header">
        <livewire:global.page-header judul="Period" subjudul="Manage your Period" :breadcrumb="['Master', 'Period']" />
    </x-slot>
    <div class="card">
        <div class="card-header header-elements-inline">
            <h5 class="card-title">Period</h5>
        </div>
        <div class="card-body">
            @if ($errors->any())
                <div class="alert alert-danger">
                    <ul>
                        @foreach ($errors->all() as $error)
                            <li>{{ $error }}</li>
                        @endforeach
                    </ul>
                </div>
            @endif
        </div>
    </div>
    <div class="card">
        <div class="card-header header-elements-inline">
            <h6 class="card-title">Highlighted justified</h6>
        </div>

        <div class="card-body">
            <form action="#" wire:submit="save">
                @csrf
                <ul class="nav nav-tabs nav-tabs-highlight nav-justified">
                    <li class="nav-item">
                        <a href="#highlighted-justified-tab1" wire:click="setActiveTab('tab1')"
                            class="nav-link {{ $activeTab === 'tab1' ? 'active' : '' }}" data-toggle="tab">Curent
                            Period</a>
                    </li>
                    <li class="nav-item">
                        <a href="#highlighted-justified-tab2" wire:click="setActiveTab('tab2')"
                            class="nav-link {{ $activeTab === 'tab2' ? 'active' : '' }}" data-toggle="tab">Last
                            Period</a>
                    </li>
                    <li class="nav-item">
                        <a href="#highlighted-justified-tab3" wire:click="setActiveTab('tab3')"
                            class="nav-link {{ $activeTab === 'tab3' ? 'active' : '' }}" data-toggle="tab">Last
                            Two Period</a>
                    </li>
                </ul>

                <div class="tab-content">
                    @if ($activeTab === 'tab1')
                        <div class="tab-pane fade {{ $activeTab === 'tab1' ? 'show active' : '' }}"
                            id="highlighted-justified-tab1">
                            <fieldset class="mb-3">
                                <div class="form-group row text-center">
                                    <label class="col-form-label col">
                                        <h1>{{ __('Tahun') }} {{ App\Models\Parameter::first()->year }}</h1>
                                    </label>
                                </div>

                                <div class="form-group row">
                                    <table class="table table-bordered">
                                        <tr class="text-center">
                                            <td>Periode</td>
                                            <td>
                                                Period Name
                                            </td>
                                            <td>
                                                From Date
                                            </td>
                                            <td>
                                                To Date
                                            </td>
                                        </tr>
                                        @foreach (range(1, 13) as $month)
                                            <tr class="text-center">
                                                <td>{{ sprintf('%02d', $month) }}</td>
                                                <td>
                                                    <input @if ($lihat) readonly @endif
                                                        type="text" class="form-control"
                                                        wire:model.live="Curent.p{{ sprintf('%02d', $month) }}">
                                                </td>
                                                <td>
                                                    <input @if ($lihat) readonly @endif
                                                        type="date" class="form-control"
                                                        wire:model.live="Curent.p{{ sprintf('%02d', $month) }}_from">
                                                </td>
                                                <td>
                                                    <input @if ($lihat) readonly @endif
                                                        type="date" class="form-control"
                                                        wire:model.live="Curent.p{{ sprintf('%02d', $month) }}_to">
                                                </td>
                                            </tr>
                                        @endforeach
                                    </table>
                                </div>
                            </fieldset>
                        </div>
                    @elseif ($activeTab === 'tab2')
                        <div class="tab-pane fade {{ $activeTab === 'tab2' ? 'show active' : '' }}"
                            id="highlighted-justified-tab2">
                            <fieldset class="mb-3">
                                <div class="form-group row text-center">
                                    <label class="col-form-label col">
                                        <h1>{{ __('Tahun') }} {{ App\Models\Parameter::first()->year - 1 }}</h1>
                                    </label>
                                </div>

                                <div class="form-group row">
                                    <table class="table table-bordered">
                                        <tr class="text-center">
                                            <td>Periode</td>
                                            <td>
                                                Period Name
                                            </td>
                                            <td>
                                                From Date
                                            </td>
                                            <td>
                                                To Date
                                            </td>
                                        </tr>
                                        @foreach (range(1, 13) as $month)
                                            <tr class="text-center">
                                                <td>{{ sprintf('%02d', $month) }}</td>
                                                <td>
                                                    <input @if ($lihat) readonly @endif
                                                        type="text" class="form-control"
                                                        wire:model="Last.p{{ sprintf('%02d', $month) }}">
                                                </td>
                                                <td>
                                                    <input @if ($lihat) readonly @endif
                                                        type="date" class="form-control"
                                                        wire:model="Last.p{{ sprintf('%02d', $month) }}_from">
                                                </td>
                                                <td>
                                                    <input @if ($lihat) readonly @endif
                                                        type="date" class="form-control"
                                                        wire:model="Last.p{{ sprintf('%02d', $month) }}_to">
                                                </td>
                                            </tr>
                                        @endforeach
                                    </table>
                                </div>
                            </fieldset>

                        </div>
                    @elseif ($activeTab === 'tab3')
                        <div class="tab-pane fade {{ $activeTab === 'tab3' ? 'show active' : '' }}"
                            id="highlighted-justified-tab3">
                            <fieldset class="mb-3">
                                <div class="form-group row text-center">
                                    <label class="col-form-label col">
                                        <h1>{{ __('Tahun') }} {{ App\Models\Parameter::first()->year - 2 }}</h1>
                                    </label>
                                </div>

                                <div class="form-group row">
                                    <table class="table table-bordered">
                                        <tr class="text-center">
                                            <td>Periode</td>
                                            <td>
                                                Period Name
                                            </td>
                                            <td>
                                                From Date
                                            </td>
                                            <td>
                                                To Date
                                            </td>
                                        </tr>
                                        @foreach (range(1, 13) as $month)
                                            <tr class="text-center">
                                                <td>{{ sprintf('%02d', $month) }}</td>
                                                <td>
                                                    <input @if ($lihat) readonly @endif
                                                        type="text" class="form-control"
                                                        wire:model="LastTwo.p{{ sprintf('%02d', $month) }}">
                                                </td>
                                                <td>
                                                    <input @if ($lihat) readonly @endif
                                                        type="date" class="form-control"
                                                        wire:model="LastTwo.p{{ sprintf('%02d', $month) }}_from">
                                                </td>
                                                <td>
                                                    <input @if ($lihat) readonly @endif
                                                        type="date" class="form-control"
                                                        wire:model="LastTwo.p{{ sprintf('%02d', $month) }}_to">
                                                </td>
                                            </tr>
                                        @endforeach
                                    </table>
                                </div>
                            </fieldset>
                        </div>
                    @endif
                </div>


                <div class="card-footer bg-white d-flex justify-content-between align-items-center py-2">
                    <ul class="list-inline mb-0">
                        <li class="list-inline-item">
                            <div wire:loading.remove wire:target='clear'>
                                <button type="button" class="btn btn-danger" wire:click="clear">Cancel</button>
                            </div>
                            <div wire:loading wire:target="clear">
                                <button type="button" class="btn bg-slate-600" disabled>Canceling...<i
                                        class="icon-spinner2 spinner"></i></button>
                            </div>
                        </li>
                    </ul>

                    @if (!$lihat)
                        @canany(['createperiode', 'editperiode'])
                            <ul class="list-inline mb-0">
                                <li class="list-inline-item">
                                    <div wire:loading.remove wire:target='save'>
                                        <button type="submit" class="btn btn-primary text-right">Save changes</button>
                                    </div>
                                    <div wire:loading wire:target="save">
                                        <button type="button" class="btn bg-slate-600" disabled>saving...<i
                                                class="icon-spinner2 spinner"></i></button>
                                    </div>
                                </li>
                            </ul>
                        @endcanany
                    @endif
                </div>
            </form>
        </div>
    </div>
</div>
