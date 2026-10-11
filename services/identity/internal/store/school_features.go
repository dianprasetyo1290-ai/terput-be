package store

import "database/sql"

// Saklar fitur per sekolah (school_features, migrasi 0012). Tidak ada baris
// = fitur MATI, jadi tak ada state default yang perlu di-backfill.

// FeatureKeys adalah daftar fitur opsional yang dikenal. Dipakai GET
// /schools/{id}/features supaya UI tetap bisa merender saklar walau belum
// ada satu baris pun, dan sebagai allowlist pada PUT.
var FeatureKeys = []string{"accounting", "hris"}

// SchoolFeature satu baris tabel (dipakai daftar lintas-sekolah).
type SchoolFeature struct {
	SchoolID   string `json:"school_id"`
	FeatureKey string `json:"feature_key"`
	Enabled    bool   `json:"enabled"`
}

// FeatureStatus status satu fitur untuk satu sekolah.
type FeatureStatus struct {
	FeatureKey string `json:"feature_key"`
	Enabled    bool   `json:"enabled"`
}

// SchoolFeatureMap membaca baris yang ada untuk satu sekolah.
func SchoolFeatureMap(db *sql.DB, schoolID string) (map[string]bool, error) {
	rows, err := db.Query(`SELECT feature_key, enabled FROM school_features WHERE school_id = $1`, schoolID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]bool{}
	for rows.Next() {
		var k string
		var enabled bool
		if err := rows.Scan(&k, &enabled); err != nil {
			return nil, err
		}
		out[k] = enabled
	}
	return out, rows.Err()
}

// FeatureStatuses memetakan baris yang ada ke status SEMUA fitur yang
// dikenal — kunci yang tidak punya baris jatuh ke false (default mati).
// Fungsi murni supaya aturan default-mati bisa diuji tanpa DB.
func FeatureStatuses(existing map[string]bool) []FeatureStatus {
	out := make([]FeatureStatus, 0, len(FeatureKeys))
	for _, k := range FeatureKeys {
		out = append(out, FeatureStatus{FeatureKey: k, Enabled: existing[k]})
	}
	return out
}

// EnabledFeatureKeys mengembalikan hanya fitur yang menyala (untuk SPA).
func EnabledFeatureKeys(existing map[string]bool) []string {
	out := []string{}
	for _, k := range FeatureKeys {
		if existing[k] {
			out = append(out, k)
		}
	}
	return out
}

// IsKnownFeature dipakai sebagai allowlist PUT — feature_key sembarang
// tidak boleh masuk tabel.
func IsKnownFeature(key string) bool {
	for _, k := range FeatureKeys {
		if k == key {
			return true
		}
	}
	return false
}

// SetSchoolFeature upsert satu saklar. updatedBy boleh kosong (nil di DB).
func SetSchoolFeature(db *sql.DB, schoolID, key string, enabled bool, updatedBy string) error {
	var by interface{}
	if updatedBy != "" {
		by = updatedBy
	}
	_, err := db.Exec(`INSERT INTO school_features (school_id, feature_key, enabled, updated_by)
		VALUES ($1,$2,$3,$4)
		ON CONFLICT (school_id, feature_key)
		DO UPDATE SET enabled = EXCLUDED.enabled, updated_at = now(), updated_by = EXCLUDED.updated_by`,
		schoolID, key, enabled, by)
	return err
}

// AllSchoolFeatures semua baris (hanya superadmin) — supaya halaman Data
// Sekolah tidak perlu satu request per sekolah.
func AllSchoolFeatures(db *sql.DB) ([]SchoolFeature, error) {
	rows, err := db.Query(`SELECT school_id, feature_key, enabled FROM school_features`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []SchoolFeature{}
	for rows.Next() {
		var f SchoolFeature
		if err := rows.Scan(&f.SchoolID, &f.FeatureKey, &f.Enabled); err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}
