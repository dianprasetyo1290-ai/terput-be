<?php

namespace App\Support;

use Illuminate\Support\Facades\DB;

/** Append-only trail of accounting events (acc_audit_log, tenant schema). */
final class AuditLog
{
    public static function record(string $event, ?string $subject, array $detail = [], ?string $userId = null): void
    {
        DB::table('acc_audit_log')->insert([
            'event' => mb_substr($event, 0, 40),
            'subject' => $subject !== null ? mb_substr($subject, 0, 100) : null,
            'user_id' => $userId ?? auth()->id(),
            'detail' => json_encode($detail),
            'created_at' => now(),
        ]);
    }
}
