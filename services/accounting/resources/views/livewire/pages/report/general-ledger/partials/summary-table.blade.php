<div class="table-responsive">
    <table class="table table-bordered table-striped">
        <thead class="thead-dark">
            <tr>
                <th>Account Code</th>
                <th>Account Name</th>
                <th class="text-right">Beginning Balance</th>
                <th class="text-right">Debit</th>
                <th class="text-right">Credit</th>
                <th class="text-right">Ending Balance</th>
            </tr>
        </thead>
        <tbody>
            @forelse($accounts as $account)
            <tr>
                <td>{{ $account['acc_code'] ?? $account->acc_code }}</td>
                <td>{{ $account['acc_name'] ?? $account->acc_name }}</td>
                <td class="text-right">{{ number_format($account['beg_balance'] ?? 0, 2) }}</td>
                <td class="text-right">{{ number_format($account['debet'] ?? 0, 2) }}</td>
                <td class="text-right">{{ number_format($account['credit'] ?? 0, 2) }}</td>
                <td class="text-right">{{ number_format($account['end_balance'] ?? 0, 2) }}</td>
            </tr>
            @empty
            <tr>
                <td colspan="6" class="text-center text-muted">No data found</td>
            </tr>
            @endforelse
        </tbody>
    </table>
</div>
