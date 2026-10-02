@forelse($accounts as $account)
<div class="mt-4">
    <h3 class="text-left">
        <button
            type="button"
            wire:click="toggleAccount('{{ $account['acc_code'] }}')"
            style="background: none; border: none; padding: 0; cursor: pointer; color: inherit; text-decoration: none;"
        >
            <i class="icon-chevron-{{ isset($expandedAccounts[$account['acc_code']]) && $expandedAccounts[$account['acc_code']] ? 'down' : 'right' }}"></i>
            Account: [ {{ $account['acc_code'] }} ] {{ $account['acc_name'] }}
        </button>
    </h3>

    @if(isset($expandedAccounts[$account['acc_code']]) && $expandedAccounts[$account['acc_code']])
    @if(isset($loadedDetails[$account['acc_code']]))
    <div class="table-responsive">
        <table class="table table-bordered table-striped">
            <thead class="thead-dark">
                <tr>
                    <th>{{ $this_period ? 'Trans Date' : 'Period' }}</th>
                    <th>Type</th>
                    <th>Voucher No</th>
                    <th>Div</th>
                    <th>Description</th>
                    <th class="text-right">Debit</th>
                    <th class="text-right">Credit</th>
                </tr>
            </thead>
            <tbody>
                <tr>
                    <td colspan="5" class="text-right"><strong>Beginning Balance:</strong></td>
                    <td class="text-right"><strong>{{ number_format($account['beg_balance_debit'], 2) }}</strong></td>
                    <td class="text-right"><strong>{{ number_format($account['beg_balance_credit'], 2) }}</strong></td>
                </tr>

                @forelse($loadedDetails[$account['acc_code']] as $detail)
                <tr>
                    <td>{{ $this_period ? $detail['trans_date'] : ($detail['period'] ?? '') }}</td>
                    <td>{{ $detail['trans_code'] }}</td>
                    <td>{{ $detail['trans_no'] }}</td>
                    <td>{{ $detail['div_code'] }}</td>
                    <td>{{ $detail['description'] }}</td>
                    <td class="text-right">{{ number_format($detail['debet'], 2) }}</td>
                    <td class="text-right">{{ number_format($detail['credit'], 2) }}</td>
                </tr>
                @empty
                <tr>
                    <td colspan="7" class="text-center text-muted">No transactions found</td>
                </tr>
                @endforelse

                <tr>
                    <td colspan="5" class="text-right"><strong>Total</strong></td>
                    <td class="text-right"><strong>{{ number_format($account['total_debit'], 2) }}</strong></td>
                    <td class="text-right"><strong>{{ number_format($account['total_credit'], 2) }}</strong></td>
                </tr>

                <tr>
                    <td colspan="5" class="text-right"><strong>Ending Balance:</strong></td>
                    <td colspan="2" class="text-right"><strong>{{ number_format($account['end_balance'], 2) }}</strong></td>
                </tr>
            </tbody>
        </table>
    </div>
    @else
    <div class="text-center py-3" wire:loading wire:target="toggleAccount">
        <div class="spinner-border spinner-border-sm" role="status">
            <span class="sr-only">Loading transactions...</span>
        </div>
        <p class="mt-2 text-muted">Loading transactions...</p>
    </div>
    @endif
    @endif
</div>
@empty
<div class="alert alert-info">
    No accounts found in the specified range.
</div>
@endforelse
