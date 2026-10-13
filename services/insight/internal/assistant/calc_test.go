package assistant

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestClassifyIntent(t *testing.T) {
	intents := []AssistantIntent{
		{
			Key:     "help_search",
			ToolKey: "help_search",
			Phrases: []string{"panduan", "bantuan", "cara pakai", "tutorial"},
			Roles:   []string{"admin", "guru", "siswa", "orang_tua"},
		},
		{
			Key:     "child_bills",
			ToolKey: "child_bills",
			Phrases: []string{"tagihan anak", "spp anak saya", "tunggakan anak"},
			Roles:   []string{"orang_tua"},
		},
		{
			Key:     "send_payment_reminders",
			ToolKey: "send_payment_reminders",
			Phrases: []string{"ingatkan tunggakan", "kirim pengingat spp"},
			Roles:   []string{"admin", "finance_manager"},
		},
	}

	// 1. Help search query
	tool, conf := ClassifyIntent("Bagaimana cara pakai aplikasi ini?", intents, "guru")
	if tool != "help_search" || conf <= 0 {
		t.Errorf("expected tool help_search, got %s (conf: %f)", tool, conf)
	}

	// 2. Child bills by orang_tua -> matches
	tool, conf = ClassifyIntent("Berapa spp anak saya bulan ini?", intents, "orang_tua")
	if tool != "child_bills" || conf <= 0 {
		t.Errorf("expected tool child_bills, got %s (conf: %f)", tool, conf)
	}

	// 3. Child bills by siswa -> not authorized for intent -> fails / returns empty
	tool, _ = ClassifyIntent("Berapa spp anak saya?", intents, "siswa")
	if tool == "child_bills" {
		t.Errorf("siswa should not match child_bills intent")
	}
}

func TestRetrieveHelp(t *testing.T) {
	articles := []HelpArticle{
		{
			ID:     "art-1",
			Title:  "Panduan Pembayaran SPP Melalui Virtual Account",
			BodyMd: "Langkah-langkah melakukan transfer VA bank",
			Tags:   []string{"spp", "keuangan", "va"},
			Status: "published",
		},
		{
			ID:     "art-2",
			Title:  "Draf Belum Terbit",
			BodyMd: "Konten rahasia internal",
			Tags:   []string{"spp"},
			Status: "draft",
		},
	}

	// Search for VA transfer -> art-1 should be returned, art-2 skipped because draft
	results := RetrieveHelp("Bagaimana transfer VA?", articles, 5)
	if len(results) != 1 || results[0].ID != "art-1" {
		t.Fatalf("expected art-1, got %v", results)
	}

	// Search for unrelated term -> empty
	unrelated := RetrieveHelp("astronomi dan roket luar angkasa", articles, 5)
	if len(unrelated) != 0 {
		t.Errorf("expected empty results for unrelated query, got %v", unrelated)
	}
}

func TestMinimize_NFR_ADV_01(t *testing.T) {
	rawPayload := map[string]interface{}{
		"school_name": "SD Teratai 01",
		"total_students": 120,
		"students": []interface{}{
			map[string]interface{}{
				"student_user_id": "usr-12345",
				"name":            "Ahmad Dahlan",
				"nik":             "3201123456789012",
				"nisn":            "0012345678",
				"email":           "ahmad@example.com",
				"phone":           "081234567890",
				"address":         "Jl. Mawar No. 12, Jakarta",
				"overdue_count":   3,
				"total_due_idr":   1500000,
			},
			map[string]interface{}{
				"student_user_id": "usr-67890",
				"name":            "Siti Nurhaliza",
				"nik":             "3201987654321098",
				"nisn":            "0098765432",
				"email":           "siti@example.com",
				"phone":           "081987654321",
				"address":         "Jl. Melati No. 45, Jakarta",
				"overdue_count":   1,
				"total_due_idr":   500000,
			},
		},
	}

	minimized := Minimize(rawPayload)

	minBytes, err := json.Marshal(minimized)
	if err != nil {
		t.Fatalf("failed to marshal minimized payload: %v", err)
	}
	minStr := string(minBytes)

	// Verify absence of sensitive PII substrings
	piiSubstrings := []string{
		"Ahmad Dahlan",
		"Siti Nurhaliza",
		"3201123456789012",
		"3201987654321098",
		"0012345678",
		"0098765432",
		"ahmad@example.com",
		"siti@example.com",
		"081234567890",
		"081987654321",
		"Jl. Mawar No. 12",
		"Jl. Melati No. 45",
		"usr-12345",
		"usr-67890",
	}

	for _, pii := range piiSubstrings {
		if strings.Contains(minStr, pii) {
			t.Errorf("NFR-ADV-01 violation: minimized payload contains PII string %q: %s", pii, minStr)
		}
	}

	// Verify that aggregate counts and numbers are retained
	if !strings.Contains(minStr, "1500000") || !strings.Contains(minStr, "500000") {
		t.Errorf("minimized payload lost aggregate numerical values: %s", minStr)
	}
}
