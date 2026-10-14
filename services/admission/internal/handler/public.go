package handler

import (
	"database/sql"
	"fmt"
	"net/http"
	"strings"
	"time"

	"skci/admission/internal/common"
)

// resolveSchoolID picks the tenant for a public (unauthenticated) request. The
// public SPA does not know a school id, so this fork falls back to the
// configured default and, failing that, to the school that owns the most
// recently opened period.
func (a *App) resolveSchoolID(r *http.Request) (string, error) {
	if id := r.URL.Query().Get("school_id"); id != "" {
		return id, nil
	}
	if a.Config.DefaultSchoolID != "" {
		return a.Config.DefaultSchoolID, nil
	}
	var id string
	err := a.DB.QueryRowContext(r.Context(),
		`SELECT school_id FROM admission_periods WHERE status='open' AND deleted_at IS NULL ORDER BY created_at DESC LIMIT 1`).
		Scan(&id)
	if err == nil && id != "" {
		return id, nil
	}
	// Fallback to any period in the system
	err = a.DB.QueryRowContext(r.Context(),
		`SELECT school_id FROM admission_periods WHERE deleted_at IS NULL ORDER BY created_at DESC LIMIT 1`).
		Scan(&id)
	if err == nil && id != "" {
		return id, nil
	}
	// Fallback to default Teratai Putih school UUID
	return "00000000-0000-0000-0000-000000000001", nil
}

type discoveryPeriod struct {
	ID                  string     `json:"id"`
	Name                string     `json:"name"`
	AcademicYear        string     `json:"academic_year"`
	Status              string     `json:"status"`
	Timezone            string     `json:"timezone"`
	RegistrationOpenAt  *time.Time `json:"registration_open_at"`
	RegistrationCloseAt *time.Time `json:"registration_close_at"`
}

type discoveryWave struct {
	ID                string     `json:"id"`
	PeriodID          string     `json:"period_id"`
	Name              string     `json:"name"`
	RegistrationOpen  time.Time  `json:"registration_open_at"`
	RegistrationClose time.Time  `json:"registration_close_at"`
	TestAt            *time.Time `json:"test_at,omitempty"`
	AnnouncementAt    *time.Time `json:"announcement_at,omitempty"`
	Status            string     `json:"status"`
	EducationLevel    string     `json:"education_level,omitempty"`
	EntryGrade        *int       `json:"entry_grade,omitempty"`
}

type discoveryTrack struct {
	ID             string   `json:"id"`
	PeriodID       string   `json:"period_id"`
	Name           string   `json:"name"`
	Description    string   `json:"description"`
	Quota          int      `json:"quota"`
	ApplicantCount int      `json:"applicant_count"`
	FeeAmount      *int64   `json:"fee_amount,omitempty"`
	Requirements   []string `json:"requirements"`
	IsActive       bool     `json:"is_active"`
}

type discoveryRequirement struct {
	ID            string   `json:"id"`
	Name          string   `json:"name"`
	Description   string   `json:"description,omitempty"`
	AcceptedTypes []string `json:"accepted_types"`
	MaxSizeMB     int      `json:"max_size_mb"`
	Required      bool     `json:"required"`
}

type discoveryResponse struct {
	SchoolName           string                 `json:"school_name"`
	Headline             string                 `json:"headline"`
	Description          string                 `json:"description"`
	Contact              string                 `json:"contact"`
	Periods              []discoveryPeriod      `json:"periods"`
	Waves                []discoveryWave        `json:"waves"`
	Tracks               []discoveryTrack       `json:"tracks"`
	DocumentRequirements []discoveryRequirement `json:"document_requirements"`
	FormFields           []formFieldRow         `json:"form_fields"`
}

// Discovery is GET /public/discovery: the public registration catalogue.
func (a *App) Discovery(w http.ResponseWriter, r *http.Request) {
	schoolID, err := a.resolveSchoolID(r)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "sekolah tidak dikenali")
		return
	}
	if schoolID == "" {
		common.WriteError(w, http.StatusBadRequest, "sekolah wajib dipilih")
		return
	}

	out := discoveryResponse{
		SchoolName:           a.Config.SchoolName,
		Headline:             a.Config.SchoolHeadline,
		Description:          a.Config.SchoolDescription,
		Contact:              a.Config.SchoolContact,
		Periods:              []discoveryPeriod{},
		Waves:                []discoveryWave{},
		Tracks:               []discoveryTrack{},
		DocumentRequirements: []discoveryRequirement{},
	}

	periodRows, err := a.DB.QueryContext(r.Context(),
		`SELECT p.id,p.name,p.academic_year,p.status,p.timezone,
		        COALESCE(p.registration_open_at,(SELECT min(w.registration_start) FROM admission_waves w WHERE w.period_id=p.id AND w.deleted_at IS NULL)),
		        COALESCE(p.registration_close_at,(SELECT max(w.registration_end) FROM admission_waves w WHERE w.period_id=p.id AND w.deleted_at IS NULL))
		 FROM admission_periods p
		 WHERE p.school_id=$1 AND p.status='open' AND p.deleted_at IS NULL
		 ORDER BY p.created_at DESC`, schoolID)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "informasi PPDB gagal dimuat")
		return
	}
	defer periodRows.Close()
	for periodRows.Next() {
		var p discoveryPeriod
		var open, close sql.NullTime
		if err := periodRows.Scan(&p.ID, &p.Name, &p.AcademicYear, &p.Status, &p.Timezone, &open, &close); err != nil {
			common.WriteError(w, http.StatusInternalServerError, "informasi PPDB gagal dimuat")
			return
		}
		if open.Valid {
			p.RegistrationOpenAt = &open.Time
		}
		if close.Valid {
			p.RegistrationCloseAt = &close.Time
		}
		out.Periods = append(out.Periods, p)
	}

	waveRows, err := a.DB.QueryContext(r.Context(),
		`SELECT w.id,w.period_id,w.name,w.registration_start,w.registration_end,w.test_start,w.announcement_at,
		        COALESCE(w.education_level,''),w.entry_grade
		 FROM admission_waves w
		 JOIN admission_periods p ON p.id=w.period_id
		 WHERE w.school_id=$1 AND w.deleted_at IS NULL AND p.status='open'
		 ORDER BY w.registration_start`, schoolID)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "informasi PPDB gagal dimuat")
		return
	}
	defer waveRows.Close()
	for waveRows.Next() {
		var wave discoveryWave
		var testAt, announcement sql.NullTime
		var grade sql.NullInt64
		if err := waveRows.Scan(&wave.ID, &wave.PeriodID, &wave.Name, &wave.RegistrationOpen, &wave.RegistrationClose, &testAt, &announcement,
			&wave.EducationLevel, &grade); err != nil {
			common.WriteError(w, http.StatusInternalServerError, "informasi PPDB gagal dimuat")
			return
		}
		if testAt.Valid {
			wave.TestAt = &testAt.Time
		}
		if announcement.Valid {
			wave.AnnouncementAt = &announcement.Time
		}
		if grade.Valid {
			g := int(grade.Int64)
			wave.EntryGrade = &g
		}
		wave.Status = waveStatus(time.Now(), wave.RegistrationOpen, wave.RegistrationClose)
		out.Waves = append(out.Waves, wave)
	}

	trackRows, err := a.DB.QueryContext(r.Context(),
		`SELECT t.id,t.period_id,t.name,COALESCE(t.description,''),t.quota,t.is_active,
		        (SELECT count(*) FROM applications a WHERE a.track_id=t.id),
		        (SELECT fs.registration_fee_idr FROM fee_schemes fs
		           WHERE fs.period_id=t.period_id AND (fs.track_id=t.id OR fs.track_id IS NULL) AND fs.status IN('published','draft')
		           ORDER BY fs.revision DESC LIMIT 1)
		 FROM admission_tracks t
		 JOIN admission_periods p ON p.id=t.period_id
		 WHERE t.school_id=$1 AND t.deleted_at IS NULL AND t.is_active AND p.status='open'
		 ORDER BY t.name`, schoolID)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "informasi PPDB gagal dimuat")
		return
	}
	defer trackRows.Close()
	for trackRows.Next() {
		var t discoveryTrack
		var fee sql.NullInt64
		if err := trackRows.Scan(&t.ID, &t.PeriodID, &t.Name, &t.Description, &t.Quota, &t.IsActive, &t.ApplicantCount, &fee); err != nil {
			common.WriteError(w, http.StatusInternalServerError, "informasi PPDB gagal dimuat")
			return
		}
		if fee.Valid {
			amount := fee.Int64
			t.FeeAmount = &amount
		}
		t.Requirements = []string{}
		out.Tracks = append(out.Tracks, t)
	}

	reqRows, err := a.DB.QueryContext(r.Context(),
		`SELECT dr.id,dr.name,COALESCE(dr.description,''),dr.allowed_mimes,dr.max_bytes,dr.required
		 FROM document_requirements dr
		 JOIN admission_periods p ON p.id=dr.period_id
		 WHERE dr.school_id=$1 AND dr.deleted_at IS NULL AND p.status='open'
		 ORDER BY dr.name`, schoolID)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "informasi PPDB gagal dimuat")
		return
	}
	defer reqRows.Close()
	for reqRows.Next() {
		var req discoveryRequirement
		var maxBytes int64
		var mimes string
		if err := reqRows.Scan(&req.ID, &req.Name, &req.Description, &mimes, &maxBytes, &req.Required); err != nil {
			common.WriteError(w, http.StatusInternalServerError, "informasi PPDB gagal dimuat")
			return
		}
		req.AcceptedTypes = parseTextArray(mimes)
		req.MaxSizeMB = int(maxBytes / (1 << 20))
		out.DocumentRequirements = append(out.DocumentRequirements, req)
	}

	out.FormFields, err = formFields(r.Context(), a.DB, schoolID, "", "open", true)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "informasi PPDB gagal dimuat")
		return
	}

	common.WriteJSON(w, http.StatusOK, out, nil)
}

// parseTextArray converts a Postgres text[] literal (as returned by the
// database/sql pgx driver for a `text[]` column) into a Go slice. The driver
// hands arrays back as their literal form, e.g. {a,b,c} or {} when empty.
func parseTextArray(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "{}" {
		return []string{}
	}
	raw = strings.TrimPrefix(raw, "{")
	raw = strings.TrimSuffix(raw, "}")
	if raw == "" {
		return []string{}
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if v := strings.Trim(strings.TrimSpace(p), `"`); v != "" {
			out = append(out, v)
		}
	}
	return out
}

func waveStatus(now, open, close time.Time) string {
	switch {
	case now.Before(open):
		return "scheduled"
	case now.After(close):
		return "closed"
	default:
		return "open"
	}
}

// textArray scans a Postgres text[] (database/sql cannot scan it into []string).
type textArray []string

func (t *textArray) Scan(src any) error {
	switch v := src.(type) {
	case string:
		*t = parseTextArray(v)
	case []byte:
		*t = parseTextArray(string(v))
	case nil:
		*t = []string{}
	default:
		return fmt.Errorf("textArray: unsupported type %T", src)
	}
	return nil
}
