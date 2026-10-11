<?php

use Illuminate\Http\Request;
use Illuminate\Support\Facades\Route;
use App\Http\Controllers\SearchController;

Route::get('/user', function (Request $request) {
    return $request->user();
})->middleware('auth:sanctum');
Route::get('/healthz', function () {
    return response()->json(['status' => 'ok', 'service' => 'accounting']);
});
Route::get('/search-accounts', [SearchController::class, 'searchAccounts'])->name('api.search-accounts');
Route::get('/search-divisions', [SearchController::class, 'searchDivisions'])->name('api.search-divisions');

Route::middleware('internal.key')->post('/internal/journals', function (Request $request, \App\Services\AutoJournalService $service) {
    $result = $service->postJournal($request->all());
    return response()->json($result, $result['status']);
});

