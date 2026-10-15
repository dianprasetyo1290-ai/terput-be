<?php

namespace App\Support;

use Illuminate\Support\Facades\Http;
use Illuminate\Support\Facades\Log;

/**
 * Read-only client for SEKOCI billing's internal API (X-Internal-Key), used
 * where accounting has to show what lives in billing's sub-ledger
 * (BR-ACC-06): the source payments behind an auto journal (QA BUG-148) and
 * the per-student receivable total that must equal the control account
 * (QA BUG-059, AC-04-02/03).
 *
 * Contract (billing side, see docs/prd-sekoci/handoff/QA-ACC.md):
 *   GET /internal/journal-sources?school_id=&reference=
 *       -> {data: {reference, items: [{type, id, number, student_name, nis,
 *                                      unit, amount_idr, date, status}]}}
 *   GET /internal/receivables/summary?school_id=&as_of=YYYY-MM-DD
 *       -> {data: {outstanding_idr, students, as_of}}
 *
 * Every call returns ['ok' => bool, 'data' => ..., 'error' => string|null];
 * nothing here throws, so a billing outage is shown as a readable message and
 * never as a 500.
 */
final class Billing
{
    public static function configured(): bool
    {
        return self::baseUrl() !== '' && (string) config('app.sekoci_internal_key') !== '';
    }

    public static function schoolId(): string
    {
        return (string) session('sekoci_school_id', '');
    }

    /** @return array{ok: bool, data: array, error: ?string} */
    public static function journalSources(string $reference, ?string $schoolId = null): array
    {
        $r = self::get('/internal/journal-sources', ['school_id' => $schoolId ?? self::schoolId(), 'reference' => $reference]);
        if ($r['ok']) {
            $items = $r['data']['items'] ?? null;
            if (! is_array($items)) {
                return ['ok' => false, 'data' => [], 'error' => 'Jawaban billing tidak sesuai kontrak (items tidak ada).'];
            }
            $r['data'] = array_values(array_filter($items, 'is_array'));
        }

        return $r;
    }

    /** @return array{ok: bool, data: array, error: ?string} */
    public static function receivablesSummary(?string $asOf = null, ?string $schoolId = null): array
    {
        $q = ['school_id' => $schoolId ?? self::schoolId()];
        if ($asOf) {
            $q['as_of'] = $asOf;
        }
        $r = self::get('/internal/receivables/summary', $q);
        if ($r['ok'] && ! is_numeric($r['data']['outstanding_idr'] ?? null)) {
            return ['ok' => false, 'data' => [], 'error' => 'Jawaban billing tidak sesuai kontrak (outstanding_idr tidak ada).'];
        }

        return $r;
    }

    private static function baseUrl(): string
    {
        return rtrim((string) config('app.billing_internal_url'), '/');
    }

    private static function get(string $path, array $query): array
    {
        if (! self::configured()) {
            return ['ok' => false, 'data' => [], 'error' => 'Integrasi billing belum dikonfigurasi (BILLING_INTERNAL_URL / SEKOCI_INTERNAL_KEY).'];
        }
        if (($query['school_id'] ?? '') === '') {
            return ['ok' => false, 'data' => [], 'error' => 'Sekolah aktif tidak diketahui; buka ulang Akuntansi dari SEKOCI.'];
        }
        try {
            $res = Http::withHeaders(['X-Internal-Key' => (string) config('app.sekoci_internal_key')])
                ->acceptJson()->timeout(5)->get(self::baseUrl().$path, $query);
        } catch (\Throwable $e) {
            Log::warning("billing $path: ".$e->getMessage());

            return ['ok' => false, 'data' => [], 'error' => 'Billing tidak dapat dihubungi.'];
        }
        if ($res->status() === 404) {
            return ['ok' => false, 'data' => [], 'error' => 'Data tidak ditemukan di billing (atau endpoint belum tersedia).'];
        }
        if (! $res->successful()) {
            $msg = (string) ($res->json('message') ?: 'HTTP '.$res->status());

            return ['ok' => false, 'data' => [], 'error' => "Billing menolak permintaan: $msg"];
        }
        $data = $res->json('data');

        return is_array($data)
            ? ['ok' => true, 'data' => $data, 'error' => null]
            : ['ok' => false, 'data' => [], 'error' => 'Jawaban billing tidak sesuai kontrak.'];
    }
}
