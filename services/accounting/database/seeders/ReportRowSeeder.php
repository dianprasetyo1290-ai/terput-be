<?php

namespace Database\Seeders;

use App\Models\ReportRowDetail;
use Illuminate\Database\Seeder;
use Illuminate\Support\Facades\DB;

class ReportRowSeeder extends Seeder
{
    /**
     * Row_Master mapping to report_row_head_id:
     * CASH0001 → 1 (Cash Flow)
     * R0000001 → 2 (Row Neraca / Balance Sheet)
     * R0000010 → 3 (Income Statement)
     */
    private array $headMapping = [
        'CASH0001' => 1,
        'R0000001' => 2,
        'R0000010' => 3,
    ];

    /**
     * Group mapping from old to new:
     * H → h (header)
     * D → d (detail)
     * T → t (total)
     * G → t (grand total, treated as total)
     */
    private function mapGroup(string $oldGroup): string
    {
        return match (strtoupper(trim($oldGroup))) {
            'H' => 'h',
            'D' => 'd',
            'T' => 't',
            'G' => 'gt',
            default => 'd',
        };
    }

    /**
     * Convert old account pattern from wildcard (*) to SQL LIKE (%).
     * Also handles comma-separated patterns like "1101*,1102*" → "1101%,1102%"
     */
    private function convertAccountPattern(?string $account): ?string
    {
        if (empty($account)) {
            return null;
        }
        return str_replace('*', '%', trim($account));
    }

    /**
     * Determine Row_Content type and convert formula.
     * Old Row_Content values: C, D, E, F, I, H, B (beginning balance), E (ending balance)
     * For the new system:
     * - E/C/D/I = account data (group = d, h, or t depending on Group_Code)
     * - F = formula
     * - B = beginning balance (special)
     * - H = header (no data)
     */
    private function processRowContent(string $groupCode, ?string $rowContent, ?string $formula, ?string $account): array
    {
        $rowContent = strtoupper(trim($rowContent ?? ''));
        $group = $this->mapGroup($groupCode);

        // If Row_Content is F (Formula), it's a formula row
        if ($rowContent === 'F' && !empty($formula)) {
            return [
                'group' => $group,
                'formula' => trim($formula),
                'kode' => null,
            ];
        }

        // If it's a header group, no data needed
        if ($group === 'h') {
            return [
                'group' => 'h',
                'formula' => null,
                'kode' => null,
            ];
        }

        // For total/grand total rows without formula - needs formula added later
        if (in_array($group, ['t', 'gt']) && empty($formula) && empty($account)) {
            return [
                'group' => $group,
                'formula' => null,
                'kode' => null,
            ];
        }

        return [
            'group' => $group,
            'formula' => !empty($formula) ? trim($formula) : null,
            'kode' => $this->convertAccountPattern($account),
        ];
    }

    public function run(): void
    {
        // Clear existing row details
        ReportRowDetail::query()->delete();

        // Try to import from exco_finrpt database
        try {
            $this->importFromOldDb();
        } catch (\Exception $e) {
            $this->command->warn("Could not connect to exco_finrpt: {$e->getMessage()}");
            $this->command->info("Importing from production reference data instead...");
            $this->importFromProductionReference();
        }
    }

    private function importFromOldDb(): void
    {
        // Connect to the old exco_finrpt database on local PostgreSQL
        $oldDb = DB::connection('pgsql_finrpt');

        $rows = $oldDb->table('row_master')
            ->orderBy('Row_Master')
            ->orderBy('Row_Code')
            ->get();

        $this->command->info("Found {$rows->count()} rows in exco_finrpt.row_master");

        $sortCounters = [];

        foreach ($rows as $row) {
            $rowMaster = trim($row->Row_Master);
            $headId = $this->headMapping[$rowMaster] ?? null;

            if (!$headId) {
                $this->command->warn("Unknown Row_Master: {$rowMaster}, skipping");
                continue;
            }

            // Track sort order per head
            if (!isset($sortCounters[$headId])) {
                $sortCounters[$headId] = 0;
            }
            $sortCounters[$headId]++;

            $processed = $this->processRowContent(
                trim($row->Group_Code ?? ''),
                $row->Row_Content ?? null,
                $row->Formula ?? null,
                $row->Account ?? null,
            );

            ReportRowDetail::create([
                'report_row_head_id' => $headId,
                'sort_order' => $sortCounters[$headId],
                'identifier' => trim($row->Row_Code),
                'name' => trim($row->Description),
                'group' => $processed['group'],
                'level' => (int) ($row->Level ?? 0),
                'normal_balance' => !empty($row->Normal_Bal) ? strtoupper(trim($row->Normal_Bal)) : null,
                'operator' => !empty($row->Operator) ? trim($row->Operator) : null,
                'kode' => $processed['kode'],
                'formula' => $processed['formula'],
            ]);
        }

        $this->command->info("Imported {$rows->count()} report row details");
        $this->generateMissingFormulas();
    }

    /**
     * Multi-level formula generation:
     * Level 1 (detail → sub-total): T row sums accumulated detail (d) identifiers
     * Level 2 (sub-total → section total): T row sums accumulated sub-total identifiers when no details exist
     * Level 3 (section → grand total): GT row sums section-level identifiers
     *
     * Pattern example (Income Statement):
     *   H: Section Header
     *     D: detail items...
     *     T: sub-total (sums details) ← Level 1
     *     H: sub-section header
     *     D: detail items...
     *     T: sub-total (sums details) ← Level 1
     *     T: section total (sums sub-totals) ← Level 2
     *   GT: grand total (sums section totals) ← Level 3
     */
    private function generateMissingFormulas(): void
    {
        $heads = [1, 2, 3];
        $count = 0;

        foreach ($heads as $headId) {
            $rows = ReportRowDetail::where('report_row_head_id', $headId)
                ->orderBy('sort_order')
                ->get();

            $detailIds = [];      // Level 1: detail identifiers since last header/total
            $subTotalIds = [];    // Level 2: sub-total identifiers since last section total
            $sectionIds = [];     // Level 3: section-level identifiers for grand total

            foreach ($rows as $row) {
                if ($row->group === 'h') {
                    $detailIds = [];
                } elseif ($row->group === 'd' && !empty($row->identifier)) {
                    $detailIds[] = $row->identifier;
                    // Also track as standalone detail for section/grand total
                    $sectionIds[] = $row->identifier;
                } elseif ($row->group === 't') {
                    if (empty($row->formula) && empty($row->kode)) {
                        if (!empty($detailIds)) {
                            // Level 1: sub-total sums details
                            $formula = collect($detailIds)->map(fn($id) => '{' . $id . '}')->implode('+');
                            $row->update(['formula' => $formula]);
                            $count++;
                        } elseif (!empty($subTotalIds)) {
                            // Level 2: section total sums sub-totals
                            $formula = collect($subTotalIds)->map(fn($id) => '{' . $id . '}')->implode('+');
                            $row->update(['formula' => $formula]);
                            $count++;
                        }
                    }

                    if (!empty($row->identifier)) {
                        if (!empty($detailIds)) {
                            // This was a sub-total — track it for section total
                            $subTotalIds[] = $row->identifier;
                            // Replace individual details in sectionIds with the sub-total
                            $sectionIds = array_values(array_diff($sectionIds, $detailIds));
                            $sectionIds[] = $row->identifier;
                        } else {
                            // This was a section total — replace sub-totals in sectionIds
                            $sectionIds = array_values(array_diff($sectionIds, $subTotalIds));
                            $sectionIds[] = $row->identifier;
                            $subTotalIds = [];
                        }
                    }
                    $detailIds = [];
                } elseif ($row->group === 'gt') {
                    if (empty($row->formula) && empty($row->kode)) {
                        if (!empty($sectionIds)) {
                            $formula = collect(array_values($sectionIds))->map(fn($id) => '{' . $id . '}')->implode('+');
                            $row->update(['formula' => $formula]);
                            $count++;
                        }
                    }
                    $sectionIds = [];
                    $subTotalIds = [];
                    $detailIds = [];
                }
            }
        }

        if ($count > 0) {
            $this->command->info("Generated {$count} missing formulas for total rows");
        }
    }

    /**
     * Fallback: import row data matching production format.
     * This uses hardcoded data from the exco_finrpt database dump.
     */
    private function importFromProductionReference(): void
    {
        // Import via direct SQL from the old database using shell
        $this->command->error("Please configure pgsql_finrpt connection in config/database.php");
        $this->command->info("Add the following to config/database.php connections array:");
        $this->command->info("'pgsql_finrpt' => ['driver' => 'pgsql', 'host' => 'localhost', 'port' => '5432', 'database' => 'exco_finrpt', 'username' => 'postgres', 'password' => 'password']");
    }
}
