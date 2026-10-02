<div>
    <x-slot name="header">
        <livewire:global.page-header judul="Transaction" subjudul="Journal Validation"
            :breadcrumb="['Transaction', 'Journal Validation']" />
    </x-slot>

    <div class="card">
        <div class="card-header header-elements-inline">
            <h5 class="card-title">Journal Validation</h5>
            <div class="header-elements">
                <div class="list-icons">
                    <a class="list-icons-item" data-action="collapse"></a>
                    <a class="list-icons-item" data-action="reload"></a>
                    <a class="list-icons-item" data-action="remove"></a>
                </div>
            </div>
        </div>
        <div class="card-body">
            <form action="#" wire:submit="confirmTransfer" class="text-center">
                <h2>Check Journal Integrity</h2>
                <h2>Check Balance Integrity</h2>
                <h2>Check Foreign Currency Balance Integrity</h2>
                <div class="text-right">
                    <button type="submit" class="btn btn-primary">Process <i class="icon-paperplane ml-2"></i></button>
                </div>
            </form>
        </div>
    </div>
</div>