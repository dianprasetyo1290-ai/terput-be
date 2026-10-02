<!DOCTYPE html>
<html lang="en">

<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Ledger Details</title>
    <link rel="stylesheet" href="https://stackpath.bootstrapcdn.com/bootstrap/4.5.2/css/bootstrap.min.css">
    <style>
        body,
        html {
            height: 100%;
            margin: 0;
            padding: 0;
            font-family: Arial, sans-serif;
        }

        .full-screen-table {
            height: 100%;
            display: flex;
            flex-direction: column;
        }

        .full-screen-table h2 {
            margin: 20px 0;
        }

        .table-container {
            flex: 1;
            overflow-y: auto;
        }

        .table-container table {
            width: 100%;
        }
    </style>
</head>

<body>
    <div class="container-fluid full-screen-table">
        <h2 class="text-center">Ledger Details</h2>
        <div class="table-container">
            @if($account)
            @foreach ($account as $val)
            @php
            $totaldebet = 0;
            $totalcredit = 0;
            @endphp
            <h3 class="text-left">Account: {{ '[ '.$val->acc_code.' ] '.$val->acc_name }}</h3>
            @php
            $bb = getBeginningBalance($tahun, $val->acc_code, $periodfrom, $codedivfrom, $codedivto);
            $bb_debet = $bb->total_debet ?? 0;
            $bb_credit = $bb->total_credit ?? 0;

            $totaldebet = 0;
            $totalcredit = 0;

            $details = getLedgerDetails($tahun, $val->acc_code, $periodfrom, $periodto, $codedivfrom, $codedivto,
            $this_period);
            @endphp

            <table class="table table-bordered table-striped">
                <thead class="thead-dark">
                    <tr>
                        @if(!$this_period) {{-- boolean, bukan string --}}
                        <th>Period</th>
                        @else
                        <th>Trans Date</th>
                        @endif
                        <th>Type</th>
                        <th>Voucher No</th>
                        <th>Div</th>
                        <th>Description</th>
                        <th>Debit</th>
                        <th>Credit</th>
                    </tr>
                </thead>
                <tbody>
                    <tr>
                        <td colspan="5" class="text-right"><strong>Beginning Balance:</strong></td>
                        <td>{{ cek_min($bb_debet) }}</td>
                        <td>{{ cek_min($bb_credit) }}</td>
                    </tr>

                    @foreach($details as $detail)
                    @php
                    $totaldebet += $detail->debet;
                    $totalcredit += $detail->credit;
                    @endphp
                    <tr>
                        @if(!$this_period)
                        <td>{{ $detail->period ?? '' }} - {{ $detail->trans_date }}</td>
                        @else
                        <td>{{ $detail->trans_date }}</td>
                        @endif
                        <td>{{ $detail->trans_code }}</td>
                        <td>{{ $detail->trans_no }}</td>
                        <td>{{ $detail->div_code }}</td>
                        <td>{{ $detail->description }}</td>
                        <td>{{ cek_min($detail->debet) }}</td>
                        <td>{{ cek_min($detail->credit) }}</td>
                    </tr>
                    @endforeach

                    <tr>
                        <td colspan="5"><strong>Total</strong></td>
                        <td><strong>{{ cek_min($totaldebet) }}</strong></td>
                        <td><strong>{{ cek_min($totalcredit) }}</strong></td>
                    </tr>

                    @php
                    $endingDebet  = $bb_debet + $totaldebet;
                    $endingCredit = $bb_credit + $totalcredit;
                    @endphp
                    <tr>
                        <td colspan="5" class="text-right"><strong>Ending Balance:</strong></td>
                        <td>{{ cek_min($endingDebet) }}</td>
                        <td>{{ cek_min($endingCredit) }}</td>
                    </tr>
                </tbody>
            </table>
            @endforeach
            @endif
        </div>
</body>

</html>
