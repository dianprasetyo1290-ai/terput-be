<div>
    <x-slot name="header">
        <livewire:global.page-header judul="Transaction" subjudul="Prior Year Adjustment" :breadcrumb="['Transaction', 'Prior Year Adjustment']" />
    </x-slot>

    <div class="card rounded-lg shadow-sm">
        <div class="card-header header-elements-inline">
            <h5 class="card-title"><i class="mr-2 icon-history"></i>Prior Year Adjustment</h5>
            <div class="header-elements">
                <span class="badge badge-light">Active Year {{ $activeYear }}</span>
            </div>
        </div>

        <div class="card-body">
            @if ($message)
                <div class="alert alert-success border-0 alert-dismissible">
                    {{ $message }}
                </div>
            @endif

            @if ($errors->any())
                <div class="alert alert-danger border-0">
                    {{ $errors->first() }}
                </div>
            @endif

            @if ($mode === 'list')
                <div class="d-flex justify-content-between align-items-center mb-3">
                    <div>
                        <h6 class="mb-0 font-weight-semibold">Adjustment List</h6>
                        <span class="text-muted small">Daftar draft dan adjustment tahun lalu yang sudah selesai.</span>
                    </div>
                    <button type="button" class="btn btn-primary" wire:click="openCreateForm">
                        <i class="icon-plus-circle2 mr-2"></i>New Adjustment
                    </button>
                </div>

                <div class="row mb-3">
                    <div class="col-md-3 mb-2 mb-md-0">
                        <label class="text-muted small mb-1">Tahun</label>
                        <select class="form-control pya-filter-control" wire:model.live="listFilterYear">
                            <option value="">All Years</option>
                            @foreach ($listYears as $year)
                                <option value="{{ $year }}">{{ $year }}</option>
                            @endforeach
                        </select>
                    </div>
                    <div class="col-md-5">
                        <label class="text-muted small mb-1">Search</label>
                        <div class="input-group">
                            <div class="input-group-prepend">
                                <span class="input-group-text"><i class="icon-search4"></i></span>
                            </div>
                            <input type="text" class="form-control pya-filter-control" wire:model.live.debounce.300ms="search" placeholder="Search description, no. transaksi, type, status">
                        </div>
                    </div>
                </div>

                <div class="table-responsive">
                    <table class="table table-hover">
                        <thead>
                            <tr>
                                <th>Tahun</th>
                                <th>Tanggal</th>
                                <th>Type</th>
                                <th>No. Transaksi</th>
                                <th>Keterangan</th>
                                <th class="text-center">Lines</th>
                                <th class="text-right">Total Debit</th>
                                <th class="text-right">Total Credit</th>
                                <th>Status</th>
                                <th class="text-right">Action</th>
                            </tr>
                        </thead>
                        <tbody>
                            @forelse ($sessions as $session)
                                <tr>
                                    <td>{{ $session->target_year }}</td>
                                    <td>{{ $session->adjustment_date?->format('d M Y') }}</td>
                                    <td>{{ $session->trans_code }}</td>
                                    <td>{{ $session->trans_no ?: '-' }}</td>
                                    <td>{{ $session->description ?: '-' }}</td>
                                    <td class="text-center">{{ $session->lines_count }}</td>
                                    <td class="text-right">{{ number_format((float) $session->lines_sum_debet, 2, ',', '.') }}</td>
                                    <td class="text-right">{{ number_format((float) $session->lines_sum_credit, 2, ',', '.') }}</td>
                                    <td>
                                        <span class="badge {{ $session->status === 'draft' ? 'badge-warning' : 'badge-success' }}">
                                            {{ $session->status }}
                                        </span>
                                    </td>
                                    <td class="text-right">
                                        <button type="button" class="btn btn-sm btn-light" wire:click="selectSession({{ $session->id }})">
                                            Open
                                        </button>
                                        @if ($session->status === 'draft')
                                            <button type="button" class="btn btn-sm btn-outline-danger ml-1" wire:click="confirmDeleteDraft({{ $session->id }})">
                                                <i class="icon-trash mr-1"></i>Delete
                                            </button>
                                        @endif
                                    </td>
                                </tr>
                            @empty
                                <tr>
                                    <td colspan="10" class="text-center text-muted py-4">Belum ada adjustment.</td>
                                </tr>
                            @endforelse
                        </tbody>
                        @if ($sessions->isNotEmpty())
                            <tfoot>
                                <tr>
                                    <th colspan="6" class="text-right">Total</th>
                                    <th class="text-right">{{ number_format($listTotalDebet, 2, ',', '.') }}</th>
                                    <th class="text-right">{{ number_format($listTotalCredit, 2, ',', '.') }}</th>
                                    <th colspan="2"></th>
                                </tr>
                            </tfoot>
                        @endif
                    </table>
                </div>
            @else
            <div class="row">
                <div class="col-xl-4 mb-3">
                    <div class="border rounded p-3 h-100" style="background:#f8fafc;">
                        <div class="d-flex align-items-center mb-3">
                            <i class="icon-calendar22 mr-2 text-primary"></i>
                            <h6 class="mb-0 font-weight-semibold">Draft Header</h6>
                        </div>

                        <button type="button" class="btn btn-light btn-block mb-3" wire:click="backToList">
                            <i class="icon-arrow-left16 mr-2"></i>Back to List
                        </button>

                        <div class="form-group">
                            <label>Transaction Type</label>
                            <select class="form-control" wire:model="transCode" @if($selectedSession && $selectedSession->status !== 'draft') disabled @endif>
                                @foreach ($transactionTypes as $type)
                                    <option value="{{ $type->trans_code }}">
                                        {{ $type->trans_code }} - {{ $type->trans_name }}
                                        @if ($type->trans_account)
                                            (Pasangan: {{ $type->trans_account }})
                                        @endif
                                    </option>
                                @endforeach
                            </select>
                        </div>

                        <div class="form-group">
                            <label>Tahun Adjustment</label>
                            <select class="form-control" wire:model.live="targetYear" @if($selectedSession && $selectedSession->status !== 'draft') disabled @endif>
                                @foreach ($years as $year)
                                    <option value="{{ $year }}">{{ $year }}</option>
                                @endforeach
                            </select>
                        </div>

                        <div class="form-group">
                            <label>Tanggal Adjustment</label>
                            <input type="date" class="form-control" wire:model="adjustmentDate" @if($selectedSession && $selectedSession->status !== 'draft') disabled @endif>
                        </div>

                        <div class="form-group">
                            <label>Keterangan</label>
                            <textarea class="form-control" rows="3" wire:model="description" @if($selectedSession && $selectedSession->status !== 'draft') disabled @endif></textarea>
                        </div>

                        <button type="button" class="btn btn-primary btn-block" wire:click="createDraft">
                            <i class="icon-plus-circle2 mr-2"></i>Create Draft
                        </button>

                        <div class="mt-4">
                            <div class="text-uppercase text-muted font-size-xs mb-2">Draft Terakhir</div>
                            <div class="list-group">
                                @forelse ($sessions as $session)
                                    <button type="button"
                                        class="list-group-item list-group-item-action d-flex justify-content-between align-items-center {{ $sessionId === $session->id ? 'active' : '' }}"
                                        wire:click="selectSession({{ $session->id }})">
                                        <span>
                                            {{ $session->target_year }}
                                            <small class="d-block">{{ $session->description ?: 'Adjustment' }}</small>
                                        </span>
                                        <span class="badge {{ $session->status === 'draft' ? 'badge-warning' : 'badge-success' }}">
                                            {{ $session->status }}
                                        </span>
                                    </button>
                                @empty
                                    <div class="text-muted small">Belum ada draft.</div>
                                @endforelse
                            </div>
                        </div>
                    </div>
                </div>

                <div class="col-xl-8">
                    @if ($selectedSession)
                        @php
                            $totalDebet = (float) $selectedSession->lines->sum('debet');
                            $totalCredit = (float) $selectedSession->lines->sum('credit');
                            $balanced = abs($totalDebet - $totalCredit) < 0.01;
                        @endphp

                        <div class="border rounded p-3 mb-3">
                            <div class="d-flex flex-wrap justify-content-between align-items-center mb-3">
                                <div>
                                    <div class="text-muted font-size-xs text-uppercase">Selected Draft</div>
                                    <h6 class="mb-0">{{ $selectedSession->trans_no ?: 'Draft #' . $selectedSession->id }}</h6>
                                </div>
                                <div class="text-right">
                                    <span class="badge {{ $balanced ? 'badge-success' : 'badge-danger' }}">
                                        {{ $balanced ? 'Balanced' : 'Not Balanced' }}
                                    </span>
                                    <span class="badge badge-light ml-1">Period 13 {{ $selectedSession->target_year }}</span>
                                </div>
                            </div>

                            @if ($selectedSession->status === 'draft')
                                <form wire:submit.prevent="addLine" class="mb-3">
                                    <div class="row">
                                        <div class="col-md-6 form-group" wire:ignore>
                                            <label>Account</label>
                                            <select class="form-control pya-select2 js-pya-account" data-placeholder="Search account">
                                                <option value="">Select account</option>
                                                @foreach ($accounts as $account)
                                                    <option value="{{ $account->acc_code }}">{{ $account->acc_code }} - {{ $account->acc_name }}</option>
                                                @endforeach
                                            </select>
                                        </div>
                                        <div class="col-md-6 form-group" wire:ignore>
                                            <label>Division</label>
                                            <select class="form-control pya-select2 js-pya-division" data-placeholder="Search division">
                                                <option value="">Select division</option>
                                                @foreach ($divisions as $division)
                                                    <option value="{{ $division->div_code }}">{{ $division->div_code }} - {{ $division->div_name }}</option>
                                                @endforeach
                                            </select>
                                        </div>
                                        <div class="col-md-6 form-group">
                                            <label>Description</label>
                                            <input type="text" class="form-control" wire:model="line.description">
                                        </div>
                                        <div class="col-md-2 form-group">
                                            <label>Debit</label>
                                            <input type="text" class="form-control text-right" wire:model.live="line.debet" placeholder="0">
                                            <small class="d-block text-muted mt-1">Format: {{ $this->formatAmount($line['debet'] ?? 0) }}</small>
                                        </div>
                                        <div class="col-md-2 form-group">
                                            <label>Credit</label>
                                            <input type="text" class="form-control text-right" wire:model.live="line.credit" placeholder="0">
                                            <small class="d-block text-muted mt-1">Format: {{ $this->formatAmount($line['credit'] ?? 0) }}</small>
                                        </div>
                                        <div class="col-md-2 form-group">
                                            <label>Currency</label>
                                            <input type="text" class="form-control text-center" value="Rp" readonly>
                                        </div>
                                    </div>

                                    <div class="rounded p-3 mb-3" style="background:#f8fafc; border:1px solid #e2e8f0;">
                                        <div class="text-uppercase text-muted font-size-xs mb-1">Terbilang</div>
                                        <div class="small">
                                            Debit: <strong>{{ $this->amountInWords($line['debet'] ?? 0) }}</strong>
                                        </div>
                                        <div class="small">
                                            Credit: <strong>{{ $this->amountInWords($line['credit'] ?? 0) }}</strong>
                                        </div>
                                    </div>

                                    <button type="submit" class="btn btn-outline-primary">
                                        <i class="icon-plus2 mr-2"></i>Add Line
                                    </button>
                                </form>
                            @endif

                            <div class="table-responsive">
                                <table class="table table-sm table-striped">
                                    <thead>
                                        <tr>
                                            <th>Account</th>
                                            <th>Division</th>
                                            <th>Description</th>
                                            <th class="text-right">Debit</th>
                                            <th class="text-right">Credit</th>
                                            <th style="width:60px;"></th>
                                        </tr>
                                    </thead>
                                    <tbody>
                                        @forelse ($selectedSession->lines as $item)
                                            <tr>
                                                <td>{{ $item->acc_code }}</td>
                                                <td>{{ $item->div_code }}</td>
                                                <td>{{ $item->description }}</td>
                                                <td class="text-right">{{ number_format((float) $item->debet, 2, ',', '.') }}</td>
                                                <td class="text-right">{{ number_format((float) $item->credit, 2, ',', '.') }}</td>
                                                <td class="text-right">
                                                    @if ($selectedSession->status === 'draft')
                                                        <button type="button" class="btn btn-sm btn-light" wire:click="confirmDeleteLine({{ $item->id }})">
                                                            <i class="icon-trash"></i>
                                                        </button>
                                                    @endif
                                                </td>
                                            </tr>
                                        @empty
                                            <tr>
                                                <td colspan="6" class="text-center text-muted">No adjustment lines.</td>
                                            </tr>
                                        @endforelse
                                    </tbody>
                                    <tfoot>
                                        <tr>
                                            <th colspan="3" class="text-right">Total</th>
                                            <th class="text-right">{{ number_format($totalDebet, 2, ',', '.') }}</th>
                                            <th class="text-right">{{ number_format($totalCredit, 2, ',', '.') }}</th>
                                            <th></th>
                                        </tr>
                                    </tfoot>
                                </table>
                            </div>

                            @if ($selectedSession->status === 'draft')
                                <div class="text-right mt-3">
                                    <button type="button" class="btn btn-success"
                                        wire:click="confirmFinalizeAdjustment"
                                        wire:loading.attr="disabled"
                                        wire:target="confirmFinalizeAdjustment,finalizeAdjustment">
                                        <span wire:loading.remove wire:target="confirmFinalizeAdjustment,finalizeAdjustment">
                                            <i class="icon-checkmark3 mr-2"></i>Adjustment Selesai
                                        </span>
                                        <span wire:loading wire:target="confirmFinalizeAdjustment,finalizeAdjustment">
                                            <i class="icon-spinner2 spinner mr-2"></i>Finalizing...
                                        </span>
                                    </button>
                                </div>
                            @endif
                        </div>
                    @else
                        <div class="border rounded p-5 text-center text-muted">
                            Pilih atau buat draft adjustment untuk mulai mengisi jurnal.
                        </div>
                    @endif
                </div>
            </div>
            @endif
        </div>
    </div>

    @if ($showFinalizeModal && $selectedSession)
        <div class="modal fade show d-block" tabindex="-1" style="background: rgba(15, 23, 42, .55);">
            <div class="modal-dialog modal-dialog-centered">
                <div class="modal-content">
                    <div class="modal-header">
                        <h5 class="modal-title">Finalize Prior Year Adjustment</h5>
                        <button type="button" class="close" wire:click="cancelFinalizeAdjustment">×</button>
                    </div>
                    <div class="modal-body">
                        <p class="mb-2">Adjustment tahun {{ $selectedSession->target_year }} akan diposting ke periode 13 dan opening balance tahun setelahnya akan dihitung ulang.</p>
                        <div class="rounded p-3" style="background:#fffbeb; border:1px solid #fde68a;">
                            Pastikan total debit dan credit sudah benar sebelum melanjutkan.
                        </div>
                    </div>
                    <div class="modal-footer">
                        <button type="button" class="btn btn-light" wire:click="cancelFinalizeAdjustment">Cancel</button>
                        <button type="button" class="btn btn-success" wire:click="finalizeAdjustment"
                            wire:loading.attr="disabled"
                            wire:target="finalizeAdjustment">
                            <span wire:loading.remove wire:target="finalizeAdjustment">
                                <i class="icon-checkmark3 mr-2"></i>Finalize
                            </span>
                            <span wire:loading wire:target="finalizeAdjustment">
                                <i class="icon-spinner2 spinner mr-2"></i>Finalizing...
                            </span>
                        </button>
                    </div>
                </div>
            </div>
        </div>
    @endif

    @if ($showDeleteLineModal && $selectedSession)
        <div class="modal fade show d-block" tabindex="-1" style="background: rgba(15, 23, 42, .55);">
            <div class="modal-dialog modal-dialog-centered">
                <div class="modal-content">
                    <div class="modal-header">
                        <h5 class="modal-title">Delete Adjustment Line</h5>
                        <button type="button" class="close" wire:click="cancelDeleteLine">×</button>
                    </div>
                    <div class="modal-body">
                        <p class="mb-2">Baris adjustment ini akan dihapus dari draft.</p>
                        <div class="rounded p-3" style="background:#fff7ed; border:1px solid #fed7aa;">
                            Pastikan baris yang dipilih sudah benar. Aksi ini tidak memposting jurnal.
                        </div>
                    </div>
                    <div class="modal-footer">
                        <button type="button" class="btn btn-light" wire:click="cancelDeleteLine">Cancel</button>
                        <button type="button" class="btn btn-danger" wire:click="deleteLine">
                            <i class="icon-trash mr-2"></i>Delete Line
                        </button>
                    </div>
                </div>
            </div>
        </div>
    @endif

    @if ($showDeleteDraftModal)
        <div class="modal fade show d-block" tabindex="-1" style="background: rgba(15, 23, 42, .55);">
            <div class="modal-dialog modal-dialog-centered">
                <div class="modal-content">
                    <div class="modal-header">
                        <h5 class="modal-title">Delete Draft Adjustment</h5>
                        <button type="button" class="close" wire:click="cancelDeleteDraft">×</button>
                    </div>
                    <div class="modal-body">
                        <p class="mb-2">Draft adjustment ini akan dihapus beserta semua barisnya.</p>
                        <div class="rounded p-3" style="background:#fff7ed; border:1px solid #fed7aa;">
                            Hanya draft yang dapat dihapus. Adjustment yang sudah selesai tetap tersimpan.
                        </div>
                    </div>
                    <div class="modal-footer">
                        <button type="button" class="btn btn-light" wire:click="cancelDeleteDraft">Cancel</button>
                        <button type="button" class="btn btn-danger" wire:click="deleteDraft">
                            <i class="icon-trash mr-2"></i>Delete Draft
                        </button>
                    </div>
                </div>
            </div>
        </div>
    @endif

    @push('scripts')
        <script>
            document.addEventListener('livewire:init', () => {
                const initPriorYearAdjustmentSelects = () => {
                    if (!window.jQuery || !jQuery.fn.select2) return;

                    jQuery('.pya-select2').each(function () {
                        const select = jQuery(this);
                        if (select.data('select2')) {
                            return;
                        }

                        select.select2({
                            width: '100%',
                            placeholder: select.data('placeholder') || 'Search',
                            allowClear: true
                        });
                    });

                    jQuery('.js-pya-account')
                        .off('change.prior-year-adjustment')
                        .on('change.prior-year-adjustment', function () {
                            @this.set('line.acc_code', jQuery(this).val() || '');
                        });

                    jQuery('.js-pya-division')
                        .off('change.prior-year-adjustment')
                        .on('change.prior-year-adjustment', function () {
                            @this.set('line.div_code', jQuery(this).val() || '');
                        });
                };

                initPriorYearAdjustmentSelects();

                Livewire.hook('morph.updated', () => {
                    initPriorYearAdjustmentSelects();
                });

                Livewire.on('prior-year-adjustment-line-reset', () => {
                    jQuery('.js-pya-account, .js-pya-division').val(null).trigger('change.select2');
                });
            });
        </script>
    @endpush

    <style>
        .pya-select2 + .select2-container .select2-selection--single {
            min-height: 38px;
            height: auto;
            padding: 5px 2.5rem 5px .75rem;
            border-color: #ddd;
            border-radius: .375rem;
        }

        .pya-select2 + .select2-container .select2-selection__rendered {
            white-space: normal;
            line-height: 1.35;
            padding-left: 0;
            padding-right: 0;
        }

        .pya-select2 + .select2-container .select2-selection__arrow {
            min-height: 38px;
            height: 100%;
            right: .5rem;
        }

        .pya-filter-control {
            min-height: 42px;
            line-height: 1.45;
            padding-top: .55rem;
            padding-bottom: .55rem;
        }
    </style>
</div>
