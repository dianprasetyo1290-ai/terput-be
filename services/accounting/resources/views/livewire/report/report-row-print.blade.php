<div>
    @if (session()->has('error'))
        <tr>
            <td colspan="{{ count($columnDefinitions ?? []) + 1 }}"
                style="background-color: #f8d7da; color: #721c24; padding: 10px; border: 1px solid #f5c6cb; border-radius: 5px;">
                {{ session('error') }}
            </td>
        </tr>
    @endif

    @forelse ($processedRows ?? [] as $item)
        @php
            $paddingLeft = 20 + (int) $item->level * 24;

            $rowClass = '';
            if ($item->group == 'h') {
                $rowClass = 'header-row';
            } elseif ($item->group == 't') {
                $rowClass = 'total-row';
            } elseif ($item->group == 'gt') {
                $rowClass = 'grand-total-row';
            }

            // Detail rows get extra indent under their header
            if ($item->group == 'd') {
                $paddingLeft += 20;
            }
        @endphp
        <tr class="{{ $rowClass }}">
            <td style="padding-left: {{ $paddingLeft }}px;">
                {{ $item->name }}
            </td>

            @foreach ($columnDefinitions as $col)
                <td class="rata-kanan">
                    @if (isset($item->columns[$col['sort_order']]) && $item->columns[$col['sort_order']] !== null)
                        @if (($col['content'] ?? '') === 'Base%')
                            {{ number_format($item->columns[$col['sort_order']], 2, ',', '.') }}%
                        @else
                            {{ function_exists('cek_min') ? cek_min($item->columns[$col['sort_order']]) : number_format($item->columns[$col['sort_order']], 2, ',', '.') }}
                        @endif
                    @endif
                </td>
            @endforeach
        </tr>
    @empty
        <tr>
            <td colspan="{{ count($columnDefinitions ?? []) + 1 }}" style="text-align: center;">No report data found.</td>
        </tr>
    @endforelse
</div>
