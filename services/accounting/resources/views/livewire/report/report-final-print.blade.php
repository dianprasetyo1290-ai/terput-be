<div>
    @php
    $isianThisMonth = [];
    $isianLastMonth = [];
    $isianLastYear = [];
    $totalThisMonth = [];
    $totalLastMonth = [];
    $totalLastYear = [];
    @endphp
    @foreach ($dataRow->reportRowDetail ?? [] as $item)
    @if ($item->group == 'h')
    <tr>
        <td style="padding-left: {{ 10 * $item->level }}px;" colspan="4">{{ $item->name }}</td>
    </tr>
    @elseif ($item->group == 'd')
    <tr>
        <td style="padding-left: {{ 10 * $item->level }}px;">{{ $item->name }}</td>
        @php
        $nilai = getTotalValue($item->kode, $periode, $tahun, $item->normal_balance);
        array_push($isianThisMonth, $nilai);
        @endphp
        <td class="rata-kanan">{{ number_format($nilai, 2, ',', '.') }}</td>
        @php
        $nilai = getTotalLastMonth($item->kode, $periode, $tahun, $item->normal_balance);
        array_push($isianLastMonth, $nilai);
        @endphp
        <td class="rata-kanan">{{ number_format($nilai, 2, ',', '.') }}</td>
        @php
        $nilai = getTotalLastYear($item->kode, $tahun, $item->normal_balance);
        array_push($isianLastYear, $nilai);
        @endphp
        <td class="rata-kanan">{{ number_format($nilai, 2, ',', '.') }}</td>
    </tr>
    @elseif ($item->group == 't')
    <tr class="total-row">
        <td style="padding-left: {{ 10 * $item->level }}px;">{{ $item->name }}</td>
        @php
        $nilai = array_sum($isianThisMonth);
        $isianThisMonth = [];
        array_push($totalThisMonth, $nilai);
        @endphp
        <td class="rata-kanan">{{ number_format($nilai, 2, ',', '.') }}</td>
        @php
        $nilai = array_sum($isianLastMonth);
        $isianLastMonth = [];
        array_push($totalLastMonth, $nilai);
        @endphp
        <td class="rata-kanan">{{ number_format($nilai, 2, ',', '.') }}</td>
        @php
        $nilai = array_sum($isianLastYear);
        $isianLastYear = [];
        array_push($totalLastYear, $nilai);
        @endphp
        <td class="rata-kanan">{{ number_format($nilai, 2, ',', '.') }}</td>
    </tr>
    @elseif ($item->group == 'gt')
    <tr class="grand-total-row">
        <td style="padding-left: {{ 10 * $item->level }}px;">{{ $item->name }}</td>
        @php
        $nilai = array_sum($totalThisMonth);
        @endphp
        <td class="rata-kanan">{{ number_format($nilai, 2, ',', '.') }}</td>
        @php
        $nilai = array_sum($totalLastMonth);
        @endphp
        <td class="rata-kanan">{{ number_format($nilai, 2, ',', '.') }}</td>
        @php
        $nilai = array_sum($totalLastYear);
        @endphp
        <td class="rata-kanan">{{ number_format($nilai, 2, ',', '.') }}</td>
    </tr>
    @endif
    @endforeach
</div>
