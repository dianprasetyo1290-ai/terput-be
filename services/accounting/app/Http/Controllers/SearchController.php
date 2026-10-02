<?php

namespace App\Http\Controllers;

use App\Http\Controllers\Controller;
use Illuminate\Http\Request;
use Illuminate\Support\Facades\DB; // Make sure to use DB facade

class SearchController extends Controller
{
    public function searchAccounts(Request $request)
    {
        $search = $request->input('search');
        $accounts = DB::table('mst_account')
            ->where('acc_code', 'ilike', '%' . $search . '%')
            ->orWhere('acc_name', 'ilike', '%' . $search . '%')
            ->limit(10) // Limit results
            ->get();

        return response()->json([
            'data' => $accounts,
            'current_page' => 1, // Simple pagination, adjust if you need true pagination
            'last_page' => 1,
        ]);
    }

    public function searchDivisions(Request $request)
    {
        $search = $request->input('search');
        $divisions = DB::table('mst_division')
            ->where('div_code', 'ilike', '%' . $search . '%')
            ->orWhere('div_name', 'ilike', '%' . $search . '%')
            ->limit(10) // Limit results
            ->get();

        return response()->json([
            'data' => $divisions,
            'current_page' => 1, // Simple pagination
            'last_page' => 1,
        ]);
    }
}
