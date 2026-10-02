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
