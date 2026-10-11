<?php

use Illuminate\Database\Migrations\Migration;
use Illuminate\Database\Schema\Blueprint;
use Illuminate\Support\Facades\Schema;

// trans_user was string(5) from the standalone Exco app, but users now arrive
// via Sekoci SSO with a 36-char UUID user_id, so every journal save 500'd.
return new class extends Migration
{
    public function up(): void
    {
        foreach (['trans_head', 'this_head'] as $table) {
            Schema::table($table, fn (Blueprint $t) => $t->string('trans_user', 255)->nullable($table === 'this_head')->change());
        }
    }

    public function down(): void
    {
        // Not reversible once UUIDs are stored.
    }
};
