package campus3d

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// TestSchema_BR3D01 tests that campus3d tables adhere to BR-3D-01:
// NO operational data is stored in campus3d tables.
func TestSchema_BR3D01(t *testing.T) {
	// Look for 0060_campus3d.up.sql in relative paths
	candidates := []string{
		"../../migrations/0060_campus3d.up.sql",
		"services/ops/migrations/0060_campus3d.up.sql",
	}

	var content []byte
	var err error
	for _, c := range candidates {
		content, err = os.ReadFile(c)
		if err == nil {
			break
		}
	}
	if err != nil {
		t.Fatalf("could not read 0060_campus3d.up.sql: %v", err)
	}

	sql := string(content)

	// Allowed columns per table according to T25 §2
	allowedColumns := map[string]map[string]bool{
		"campus3d_models": {
			"id": true, "school_id": true, "building_id": true, "version": true,
			"object_key": true, "mime": true, "size_bytes": true, "sha256": true,
			"node_names": true, "status": true, "uploaded_by": true,
			"published_by": true, "published_at": true, "created_at": true, "updated_at": true,
		},
		"campus3d_floor_nodes": {
			"id": true, "school_id": true, "model_id": true, "floor_id": true,
			"node_name": true, "created_at": true, "updated_at": true,
		},
		"campus3d_room_nodes": {
			"id": true, "school_id": true, "model_id": true, "room_id": true,
			"node_name": true, "created_at": true, "updated_at": true,
		},
		"campus3d_tenant_spots": {
			"id": true, "school_id": true, "tenant_id": true, "room_id": true,
			"created_at": true, "updated_at": true,
		},
	}

	// Strictly forbidden operational columns
	forbidden := []string{
		"room_name", "building_name", "capacity", "price", "price_idr",
		"menu", "is_open", "work_order", "asset", "schedule", "booking",
	}

	tableRegex := regexp.MustCompile(`(?i)CREATE\s+TABLE\s+([a-zA-Z0-9_]+)\s*\(([\s\S]*?)\);`)
	matches := tableRegex.FindAllStringSubmatch(sql, -1)

	if len(matches) < 4 {
		t.Fatalf("expected at least 4 CREATE TABLE statements in migration, found %d", len(matches))
	}

	for _, m := range matches {
		tableName := strings.ToLower(m[1])
		tableBody := m[2]

		allowed, ok := allowedColumns[tableName]
		if !ok {
			t.Fatalf("unexpected table %s in campus3d migration", tableName)
		}

		// Split table lines
		lines := strings.Split(tableBody, "\n")
		for _, line := range lines {
			trimmed := strings.TrimSpace(line)
			if trimmed == "" || strings.HasPrefix(trimmed, "--") {
				continue
			}
			upper := strings.ToUpper(trimmed)
			if strings.HasPrefix(upper, "PRIMARY") || strings.HasPrefix(upper, "UNIQUE") || strings.HasPrefix(upper, "CONSTRAINT") || strings.HasPrefix(upper, "CHECK") {
				continue
			}

			// First token is column name
			parts := strings.Fields(trimmed)
			if len(parts) == 0 {
				continue
			}
			colName := strings.ToLower(parts[0])
			if !allowed[colName] {
				t.Errorf("BR-3D-01 VIOLATION: table %s contains non-permitted column '%s' (only allowed: %v)",
					tableName, colName, allowed)
			}

			for _, forb := range forbidden {
				if strings.Contains(colName, forb) {
					t.Errorf("BR-3D-01 VIOLATION: table %s contains operational column '%s'", tableName, colName)
				}
			}
		}
	}
}
