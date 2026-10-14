{{-- Period / unit / generated-at line and the provisional banner (open period). --}}
<div class="d-flex flex-wrap justify-content-between align-items-center mb-3">
    <div>
        <strong>Periode:</strong> {{ $label }}
        &middot; <strong>Unit:</strong> {{ $unitLabel ?? 'Semua unit' }}
    </div>
    <div class="text-muted small">Dibuat {{ $generatedAt->format('d M Y H:i') }}</div>
</div>
@if ($provisional)
    <div class="alert alert-warning py-2">
        <strong>SEMENTARA</strong> &mdash; periode ini belum ditutup (End of Month); angka dapat berubah.
        Hanya jurnal berstatus <em>posted</em> yang dihitung.
    </div>
@endif
