<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <title>Summary General Ledger</title>
    <link rel="stylesheet" href="https://stackpath.bootstrapcdn.com/bootstrap/4.5.2/css/bootstrap.min.css">
    <style>
        html, body { height: 100%; margin: 0; padding: 0; font-family: Arial, sans-serif; }
        .full-screen-table { height: 100%; display: flex; flex-direction: column; }
        .table-container { flex: 1; overflow-y: auto; }
        .table-container table { width: 100%; }
    </style>
</head>
<body>
<div class="container-fluid full-screen-table">
    <h2 class="text-center my-3">Summary General Ledger</h2>

    <div class="table-container">
        <table class="table table-bordered table-striped">
            <thead class="thead-dark">
            <tr>
                <th>Account Code</th>
                <th>Account Name</th>
                <th>Beginning Balance</th>
                <th>Debit</th>
                <th>Credit</th>
                <th>Ending Balance</th>
            </tr>
            </thead>
            <tbody>
            @forelse ($results as $val)
                <tr>
                    <td>{{ $val->acc_code }}</td>
                    <td>{{ $val->acc_name }}</td>
                    <td>{{ cek_min($val->beg_balance ?? 0) }}</td>
                    <td>{{ cek_min($val->debet ?? 0) }}</td>
                    <td>{{ cek_min($val->credit ?? 0) }}</td>
                    <td>{{ cek_min($val->end_balance ?? 0) }}</td>
                </tr>
            @empty
                <tr><td colspan="6" class="text-center">Tidak ada data.</td></tr>
            @endforelse
            </tbody>
        </table>
    </div>
</div>
</body>
</html>
