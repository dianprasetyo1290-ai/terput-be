<div>
    <x-slot name="header">
        <livewire:global.page-header judul="Transaction" subjudul="Manage your Journal Transaction" :breadcrumb="['Transaction', 'Journal Transaction']" />
    </x-slot>

    <div class="rounded-lg shadow-lg card">
        <div class="card-header header-elements-inline">
            <h5 class="card-title"><i class="mr-2 icon-book"></i>Data Jurnal Transaksi</h5>
            <div class="header-elements">
                <div class="list-icons">
                    <a class="list-icons-item" data-action="collapse"></a>
                    <a class="list-icons-item" data-action="reload"></a>
                    <a class="list-icons-item" data-action="remove"></a>
                </div>
            </div>
        </div>

        <div class="card-body">
            @if ($formDetail)
                @include('livewire.pages.journal-transaction.partials.detail-form')
            @elseif ($form)
                @include('livewire.pages.journal-transaction.partials.header-form')
            @else
                @include('livewire.pages.journal-transaction.partials.transaction-list')
            @endif
        </div>
    </div>
</div>
