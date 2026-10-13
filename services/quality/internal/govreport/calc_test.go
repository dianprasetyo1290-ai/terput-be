package govreport

import (
	"errors"
	"testing"
	"time"
)

func TestNormalize(t *testing.T) {
	if got := Normalize("00 12-34", "digits"); got != "001234" {
		t.Errorf("Normalize digits failed: got %q, want %q", got, "001234")
	}
	if got := Normalize("31/07/2010", "date"); got != "2010-07-31" {
		t.Errorf("Normalize date failed: got %q, want %q", got, "2010-07-31")
	}
	if got := Normalize("2010-07-31", "date"); got != "2010-07-31" {
		t.Errorf("Normalize date ISO failed: got %q, want %q", got, "2010-07-31")
	}
	if got := Normalize(" budi santoso ", "trim_upper"); got != "BUDI SANTOSO" {
		t.Errorf("Normalize trim_upper failed: got %q, want %q", got, "BUDI SANTOSO")
	}
}

func TestMatchRecords_And_DiffFields_AC_145_01(t *testing.T) {
	mappings := []FieldMapping{
		{DapodikColumn: "Nama Siswa", SekociField: "name", IsMatchKey: true, MatchPriority: 1, Normalize: "trim_upper"},
		{DapodikColumn: "NISN", SekociField: "nisn", IsMatchKey: false, Normalize: "digits"},
		{DapodikColumn: "Tanggal Lahir", SekociField: "birth_date", IsMatchKey: false, Normalize: "date"},
	}

	sekociList := []Record{
		{"_id": "s-1", "name": "Ahmad Dani", "nisn": "0011223344", "birth_date": "2010-05-12"},
		{"_id": "s-2", "name": "Budi Santoso", "nisn": "0099887766", "birth_date": "2010-08-20"},
	}

	dapodikList := []Record{
		{"_row_id": "r-1", "Nama Siswa": "AHMAD DANI", "NISN": "0011223399", "Tanggal Lahir": "12/05/2010"}, // NISN mismatch!
		{"_row_id": "r-2", "Nama Siswa": "Citra Lestari", "NISN": "0055443322", "Tanggal Lahir": "2010-01-01"},
	}

	pairs, onlySekoci, onlyDapodik := MatchRecords(sekociList, dapodikList, mappings)
	if len(pairs) != 1 {
		t.Fatalf("expected 1 matched pair, got %d", len(pairs))
	}
	if len(onlySekoci) != 1 || len(onlyDapodik) != 1 {
		t.Fatalf("expected 1 onlySekoci and 1 onlyDapodik, got %d and %d", len(onlySekoci), len(onlyDapodik))
	}

	diffs := DiffFields(pairs[0], mappings)
	// AC-145-01: NISN beda, nama sama -> satu field_mismatch field nisn
	if len(diffs) != 1 {
		t.Fatalf("expected 1 field_mismatch, got %d: %+v", len(diffs), diffs)
	}
	if *diffs[0].Field != "nisn" {
		t.Errorf("expected diff field 'nisn', got %s", *diffs[0].Field)
	}
	if *diffs[0].SekociValue != "0011223344" || *diffs[0].DapodikValue != "0011223399" {
		t.Errorf("unexpected diff values: %+v", diffs[0])
	}
}

func TestHashSensitive_And_Last4(t *testing.T) {
	key := "test-hmac-key"
	h1 := HashSensitive(key, "3201012345670001")
	h2 := HashSensitive(key, "3201012345670001")
	h3 := HashSensitive(key, "3201012345670002")

	if h1 != h2 {
		t.Errorf("HMAC must be deterministic")
	}
	if h1 == h3 {
		t.Errorf("HMAC must differ for different values")
	}

	if Last4("3201012345670001") != "0001" {
		t.Errorf("Last4 failed: got %s, want 0001", Last4("3201012345670001"))
	}
}

func TestBuildRegulatorySnapshot_AC_146_01(t *testing.T) {
	now := time.Now()
	template := RegulatoryTemplate{
		Code: "LAP-BULANAN",
		Columns: []TemplateColumn{
			{Label: "Tingkat Kehadiran", MetricKey: "attendance_rate"},
			{Label: "Siswa Aktif", MetricKey: "active_students"},
			{Label: "Rasio Guru Siswa", MetricKey: "teacher_student_ratio"}, // will simulate 503/failure
		},
	}

	fetcher := func(metricKey string) MetricResult {
		switch metricKey {
		case "attendance_rate":
			val := int64(9500)
			return MetricResult{Value: &val, Unit: "percent_bp", AsOf: &now}
		case "active_students":
			val := int64(350)
			return MetricResult{Value: &val, Unit: "count", AsOf: &now}
		default:
			return MetricResult{Err: errors.New("503 service unavailable")}
		}
	}

	snap := BuildRegulatorySnapshot(template, fetcher)
	if len(snap.Columns) != 3 {
		t.Fatalf("expected 3 columns, got %d", len(snap.Columns))
	}

	if *snap.Columns[0].Value != 9500 || snap.Columns[0].Unavailable {
		t.Errorf("col 0 should have value 9500 and not unavailable")
	}

	// AC-146-01: metric that fails -> labeled "sumber tidak tersedia", NOT 0
	if !snap.Columns[2].Unavailable || snap.Columns[2].ErrorNotice != "sumber tidak tersedia" {
		t.Errorf("col 2 should be unavailable with label 'sumber tidak tersedia', got: %+v", snap.Columns[2])
	}
	if snap.Columns[2].Value != nil {
		t.Errorf("col 2 Value should be nil (not 0), got: %v", snap.Columns[2].Value)
	}
}
