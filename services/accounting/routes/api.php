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

Route::middleware('internal.key')->group(function () {
    Route::post('/internal/journals', function (Request $request, \App\Services\AutoJournalService $service) {
        $result = $service->postJournal($request->all());
        return response()->json($result, $result['status']);
    });

    Route::put('/internal/vendors/{vendorId}', function (string $vendorId, Request $request, \App\Services\OpsExtensionService $service) {
        $result = $service->syncVendor($vendorId, $request->all());
        return response()->json($result, $result['status']);
    });

    Route::post('/internal/ledger/actuals', function (Request $request, \App\Services\OpsExtensionService $service) {
        $result = $service->fetchActuals($request->all());
        return response()->json($result, $result['status']);
    });

    Route::post('/internal/assets', function (Request $request, \App\Services\OpsExtensionService $service) {
        $result = $service->registerAsset($request->all());
        return response()->json($result, $result['status']);
    });

    Route::post('/internal/assets/{ref}/dispose', function (string $ref, Request $request, \App\Services\OpsExtensionService $service) {
        $result = $service->disposeAsset($ref, $request->all());
        return response()->json($result, $result['status']);
    });

    Route::post('/internal/assets/depreciate', function (Request $request, \App\Services\OpsExtensionService $service) {
        $result = $service->depreciateAssets($request->all());
        return response()->json($result, $result['status']);
    });
});

