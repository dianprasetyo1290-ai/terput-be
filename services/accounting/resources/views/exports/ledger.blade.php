@foreach ($account as $val)
@php
    $totaldebet  = 0;
    $totalcredit = 0;

    // Saldo awal: pakai periodfrom (sebelum awal range), bukan periodto
    $bb = getBeginningBalance($tahun, $val->acc_code, $periodfrom, $codedivfrom, $codedivto);

    // Ambil detail
    $details = getLedgerDetails($tahun, $val->acc_code, $periodfrom, $periodto, $codedivfrom, $codedivto, $this_period);
@endphp

<table>
    <tr>
        <td colspan="7"><strong>Account: {{ '[ '.$val->acc_code.' ] '.$val->acc_name }}</strong></td>
    </tr>
    <thead>
        <tr>
            @if(!$this_period)
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
            <td colspan="5"><strong>Beginning Balance:</strong></td>
            <td>{{ $bb->total_debet ?? 0 }}</td>
            <td>{{ $bb->total_credit ?? 0 }}</td>
        </tr>

        @foreach($details as $detail)
            @php
                $totaldebet  += $detail->debet;
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
                <td>{{ $detail->debet }}</td>
                <td>{{ $detail->credit }}</td>
            </tr>
        @endforeach

        <tr>
            <td colspan="5"><strong>Total</strong></td>
            <td><strong>{{ $totaldebet }}</strong></td>
            <td><strong>{{ $totalcredit }}</strong></td>
        </tr>

        @php
            $endingDebet  = $bb->total_debet + $totaldebet;
            $endingCredit = $bb->total_credit + $totalcredit;
        @endphp
        <tr>
            <td colspan="5"><strong>Ending Balance:</strong></td>
            <td>{{ $endingDebet }}</td>
            <td>{{ $endingCredit }}</td>
        </tr>
    </tbody>
</table>

{{-- spacer antar akun --}}
<tr><td colspan="7"></td></tr>
@endforeach
