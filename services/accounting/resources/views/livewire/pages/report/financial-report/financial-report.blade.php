<div>
    <x-slot name="header">
        <livewire:global.page-header judul="Report Final" subjudul="Report final" :breadcrumb="['Report Financial', 'Report Final']" />
    </x-slot>
    <div class="card">
        <div class="card-header header-elements-inline">
            <h5 class="card-title">Report Final Data</h5>
            <div class="header-elements">
                <div class="list-icons">
                    <a class="list-icons-item" data-action="collapse"></a>
                    <a class="list-icons-item" data-action="reload"></a>
                    <a class="list-icons-item" data-action="remove"></a>
                </div>
            </div>
        </div>
        <div class="card-body">
            <div class="card-header">
                <div class="col-12 row">
                    <div class="search-set">
                        <div class="search-input">
                            <div class="dataTables_filter"><label> <input type="search" class="form-control form-control-sm" placeholder="Search" wire:model.live='search'></label></div>
                        </div>
                    </div>
                    <label class="col-form-label col-md-1">Sort By</label>
                    <div class="col-md-2">
                        <div class="btn-group justify-content-center">
                            <a href="#" class="btn alpha-indigo dropdown-toggle" data-toggle="dropdown" aria-expanded="false">
                                @if ($order == 'ASC')
                                Latest added
                                @else
                                Oldest added
                                @endif
                            </a>

                            <div class="dropdown-menu">
                                <a class="dropdown-item" href="#" wire:click.prevent="sort('ASC')">Latest
                                    added</a>
                                <a class="dropdown-item" href="#" wire:click.prevent="sort('DESC')">Oldest
                                    added</a>
                            </div>
                        </div>
                    </div>
                    <label class="col-form-label col-md-1">Show</label>
                    <div class="col-md-2">
                        <div class="btn-group justify-content-center">
                            <a href="#" class="btn alpha-indigo dropdown-toggle" data-toggle="dropdown" aria-expanded="false">
                                {{ $limit }} items
                            </a>

                            <div class="dropdown-menu">
                                <a class="dropdown-item" href="#" wire:click.prevent="limits('30')">30 items</a>
                                <a class="dropdown-item" href="#" wire:click.prevent="limits('50')">50 items</a>
                                <a class="dropdown-item" href="#" wire:click.prevent="limits('100')">100 items</a>
                            </div>
                        </div>
                    </div>
                </div>
            </div>

            <div class="datatable-scroll">
                <table class="table datatable-basic">
                    <thead>
                        <tr role="row">
                            <th>#</th>
                            <th>Name</th>
                            <th>Actions</th>
                        </tr>
                    </thead>
                    <tbody>
                        @foreach ($data as $row)
                        <tr role="row" class="odd">
                            <td class="sorting_1">{{ $loop->iteration }}</td>
                            <td>{{ $row->name }}</td>
                            <td class="text-center">
                                <a class="btn btn-warning" href="#" wire:click="$dispatch('showModal', { id: {{ $row->id }} })">
                                    <i class="icon-printer4"></i>
                                </a>
                            </td>
                        </tr>
                        @endforeach
                    </tbody>
                </table>
                {{ $data->links() }}
            </div>
        </div>
    </div>
    <livewire:global.modal-report-final />
</div>
