package govreport

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"time"
	"unicode"
)

type FieldMapping struct {
	ID            string `json:"id"`
	SchoolID      string `json:"school_id"`
	Entity        string `json:"entity"` // student, rombel, ptk
	DapodikColumn string `json:"dapodik_column"`
	SekociField   string `json:"sekoci_field"`
	IsMatchKey    bool   `json:"is_match_key"`
	MatchPriority int    `json:"match_priority"`
	Normalize     string `json:"normalize"` // none, trim_upper, digits, date
}

type DapodikImport struct {
	ID           string    `json:"id"`
	SchoolID     string    `json:"school_id"`
	Entity       string    `json:"entity"`
	FileName     string    `json:"file_name"`
	FileChecksum string    `json:"file_checksum"`
	RowCount     int       `json:"row_count"`
	ImportedBy   string    `json:"imported_by"`
	Status       string    `json:"status"` // parsed, compared, closed
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type DapodikDiff struct {
	ID             string     `json:"id"`
	SchoolID       string     `json:"school_id"`
	ImportID       string     `json:"import_id"`
	Entity         string     `json:"entity"`
	SekociRef      *string    `json:"sekoci_ref,omitempty"`
	ImportRowID    *string    `json:"import_row_id,omitempty"`
	Kind           string     `json:"kind"` // field_mismatch, missing_in_dapodik, missing_in_sekoci
	Field          *string    `json:"field,omitempty"`
	SekociValue    *string    `json:"sekoci_value,omitempty"`
	DapodikValue   *string    `json:"dapodik_value,omitempty"`
	Status         string     `json:"status"` // open, decided
	Decision       *string    `json:"decision,omitempty"` // update_dapodik, update_sekoci, ignore
	DecisionReason *string    `json:"decision_reason,omitempty"`
	DecidedBy      *string    `json:"decided_by,omitempty"`
	DecidedAt      *time.Time `json:"decided_at,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

type RegulatoryTemplate struct {
	ID           string             `json:"id"`
	SchoolID     string             `json:"school_id"`
	Code         string             `json:"code"`
	Name         string             `json:"name"`
	Recipient    string             `json:"recipient"`
	Frequency    string             `json:"frequency"` // monthly, semester, annual, adhoc
	Columns      []TemplateColumn   `json:"columns"`
	ExportFormat string             `json:"export_format"` // xlsx, csv
	Status       string             `json:"status"` // active, inactive
	CreatedAt    time.Time          `json:"created_at"`
	UpdatedAt    time.Time          `json:"updated_at"`
}

type TemplateColumn struct {
	Label     string  `json:"label"`
	MetricKey string  `json:"metric_key"`
	GroupBy   *string `json:"group_by,omitempty"`
}

type RegulatoryReport struct {
	ID               string                 `json:"id"`
	SchoolID         string                 `json:"school_id"`
	TemplateID       string                 `json:"template_id"`
	PeriodFrom       string                 `json:"period_from"`
	PeriodTo         string                 `json:"period_to"`
	Status           string                 `json:"status"` // draft, pending_approval, approved, rejected, submitted
	ApprovalID       *string                `json:"approval_id,omitempty"`
	TemplateSnapshot *RegulatoryTemplate    `json:"template_snapshot,omitempty"`
	DataSnapshot     *RegulatoryDataSnapshot `json:"data_snapshot,omitempty"`
	SnapshotAt       *time.Time             `json:"snapshot_at,omitempty"`
	SubmittedAt      *time.Time             `json:"submitted_at,omitempty"`
	SubmittedBy      *string                `json:"submitted_by,omitempty"`
	SubmissionNote   *string                `json:"submission_note,omitempty"`
	ExportObjectKey  *string                `json:"export_object_key,omitempty"`
	IdempotencyKey   string                 `json:"idempotency_key"`
	CreatedAt        time.Time              `json:"created_at"`
	UpdatedAt        time.Time              `json:"updated_at"`
}

type SnapshotColumnValue struct {
	Label       string     `json:"label"`
	MetricKey   string     `json:"metric_key"`
	Value       *int64     `json:"value,omitempty"`
	Unit        string     `json:"unit"`
	AsOf        *time.Time `json:"as_of,omitempty"`
	Unavailable bool       `json:"unavailable"`
	ErrorNotice string     `json:"error_notice,omitempty"`
}

type RegulatoryDataSnapshot struct {
	Columns []SnapshotColumnValue `json:"columns"`
}

// Normalize applies normalization rules to strings.
func Normalize(value, rule string) string {
	switch rule {
	case "digits":
		var b strings.Builder
		for _, r := range value {
			if unicode.IsDigit(r) {
				b.WriteRune(r)
			}
		}
		return b.String()
	case "trim_upper":
		return strings.ToUpper(strings.TrimSpace(value))
	case "date":
		v := strings.TrimSpace(value)
		// Try DD/MM/YYYY
		if t, err := time.Parse("02/01/2006", v); err == nil {
			return t.Format("2006-01-02")
		}
		// Try YYYY-MM-DD
		if t, err := time.Parse("2006-01-02", v); err == nil {
			return t.Format("2006-01-02")
		}
		return v
	default:
		return strings.TrimSpace(value)
	}
}

type Record map[string]string

type Pair struct {
	Sekoci  Record
	Dapodik Record
}

type DiffItem struct {
	Kind         string  // field_mismatch, missing_in_dapodik, missing_in_sekoci
	Field        *string
	SekociValue  *string
	DapodikValue *string
	SekociRef    string
	ImportRowID  string
}

// MatchRecords pairs records based on field mappings with is_match_key = true, ordered by match_priority.
func MatchRecords(sekoci []Record, dapodik []Record, mappings []FieldMapping) (pairs []Pair, onlySekoci []Record, onlyDapodik []Record) {
	// Filter and sort match keys
	var matchKeys []FieldMapping
	for _, m := range mappings {
		if m.IsMatchKey {
			matchKeys = append(matchKeys, m)
		}
	}

	pairedDapodik := make(map[int]bool)
	pairedSekoci := make(map[int]bool)

	// Try matching by keys
	for _, k := range matchKeys {
		for sIdx, sRec := range sekoci {
			if pairedSekoci[sIdx] {
				continue
			}
			sVal := Normalize(sRec[k.SekociField], k.Normalize)
			if sVal == "" {
				continue
			}

			for dIdx, dRec := range dapodik {
				if pairedDapodik[dIdx] {
					continue
				}
				dVal := Normalize(dRec[k.DapodikColumn], k.Normalize)
				if dVal == "" {
					continue
				}

				if sVal == dVal {
					pairs = append(pairs, Pair{Sekoci: sRec, Dapodik: dRec})
					pairedSekoci[sIdx] = true
					pairedDapodik[dIdx] = true
					break
				}
			}
		}
	}

	for sIdx, sRec := range sekoci {
		if !pairedSekoci[sIdx] {
			onlySekoci = append(onlySekoci, sRec)
		}
	}
	for dIdx, dRec := range dapodik {
		if !pairedDapodik[dIdx] {
			onlyDapodik = append(onlyDapodik, dRec)
		}
	}

	return pairs, onlySekoci, onlyDapodik
}

// DiffFields compares mapped fields between matched pair.
func DiffFields(pair Pair, mappings []FieldMapping) []DiffItem {
	var diffs []DiffItem
	sRef := pair.Sekoci["_id"]
	rowID := pair.Dapodik["_row_id"]

	for _, m := range mappings {
		sRaw := pair.Sekoci[m.SekociField]
		dRaw := pair.Dapodik[m.DapodikColumn]

		sNorm := Normalize(sRaw, m.Normalize)
		dNorm := Normalize(dRaw, m.Normalize)

		if sNorm != dNorm {
			fName := m.SekociField
			sVal := sRaw
			dVal := dRaw
			diffs = append(diffs, DiffItem{
				Kind:         "field_mismatch",
				Field:        &fName,
				SekociValue:  &sVal,
				DapodikValue: &dVal,
				SekociRef:    sRef,
				ImportRowID:  rowID,
			})
		}
	}
	return diffs
}

// HashSensitive computes HMAC-SHA256 hex string of sensitive data.
func HashSensitive(key, value string) string {
	mac := hmac.New(sha256.New, []byte(key))
	mac.Write([]byte(strings.TrimSpace(value)))
	return hex.EncodeToString(mac.Sum(nil))
}

// Last4 extracts the last 4 characters of a string.
func Last4(val string) string {
	s := strings.TrimSpace(val)
	if len(s) <= 4 {
		return s
	}
	return s[len(s)-4:]
}

type MetricResult struct {
	Value *int64
	Unit  string
	AsOf  *time.Time
	Err   error
}

// BuildRegulatorySnapshot builds a frozen snapshot of regulatory report columns.
// If metric fetcher returns error, labels the column "sumber tidak tersedia" instead of 0.
func BuildRegulatorySnapshot(template RegulatoryTemplate, fetchMetric func(metricKey string) MetricResult) RegulatoryDataSnapshot {
	var cols []SnapshotColumnValue

	for _, col := range template.Columns {
		res := fetchMetric(col.MetricKey)
		if res.Err != nil || res.Value == nil {
			cols = append(cols, SnapshotColumnValue{
				Label:       col.Label,
				MetricKey:   col.MetricKey,
				Unavailable: true,
				ErrorNotice: "sumber tidak tersedia",
			})
		} else {
			cols = append(cols, SnapshotColumnValue{
				Label:       col.Label,
				MetricKey:   col.MetricKey,
				Value:       res.Value,
				Unit:        res.Unit,
				AsOf:        res.AsOf,
				Unavailable: false,
			})
		}
	}
	return RegulatoryDataSnapshot{Columns: cols}
}
