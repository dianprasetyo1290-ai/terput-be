package handler

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"skci/admission/internal/common"
	"skci/admission/internal/model"
)

// Waves carry every deadline the PPDB jobs use: form payment (BR-PPDB-07),
// re-registration (BR-PPDB-08) and the waitlist re-registration window
// (BR-PPDB-09) — BUG-098. Overlaps are reported as warnings (BUG-155) and a
// date change on a wave that already has applicants needs confirm_impact
// (BUG-156).

type waveInput struct {
	PeriodID                   string  `json:"period_id"`
	Name                       string  `json:"name"`
	RegistrationOpenAt         string  `json:"registration_open_at"`
	RegistrationCloseAt        string  `json:"registration_close_at"`
	PaymentEndAt               string  `json:"payment_end_at"`
	TestAt                     string  `json:"test_at"`
	AnnouncementAt             string  `json:"announcement_at"`
	ReregistrationEndAt        string  `json:"reregistration_end_at"`
	WaitlistReregistrationDays *int    `json:"waitlist_reregistration_days"`
	ConfirmImpact              bool    `json:"confirm_impact"`
	EducationLevel             *string `json:"education_level"`
	EntryGrade                 *int    `json:"entry_grade"`
}

// level resolves the jenjang/entry grade (BUG-093) on top of the stored
// values; an explicit empty jenjang clears both.
func (in waveInput) level(curLevel string, curGrade *int) (string, *int, error) {
	level, grade := curLevel, curGrade
	if in.EducationLevel != nil {
		level = strings.ToUpper(strings.TrimSpace(*in.EducationLevel))
		grade = in.EntryGrade
	} else if in.EntryGrade != nil {
		grade = in.EntryGrade
	}
	if err := model.ValidateWaveLevel(level, grade); err != nil {
		return "", nil, &httpError{http.StatusUnprocessableEntity, err.Error()}
	}
	return level, grade, nil
}

func intPtrOrNil(p *int) any {
	if p == nil {
		return nil
	}
	return *p
}

type waveRow struct {
	ID, PeriodID, Name    string
	Schedule              model.WaveSchedule
	WaitlistDays          int
	Level                 string
	Grade                 *int
	ApplicationCount      int
	ActiveApplicationsCnt int
}

func optionalTime(field, value string) (*time.Time, error) {
	t, err := parseTime(value)
	if err != nil {
		return nil, &httpError{http.StatusUnprocessableEntity, field + " tidak valid"}
	}
	if t.IsZero() {
		return nil, nil
	}
	return &t, nil
}

// schedule parses the input onto base (the stored wave on update).
func (in waveInput) schedule(base model.WaveSchedule) (model.WaveSchedule, error) {
	s := base
	for _, f := range []struct {
		name, value string
		dst         **time.Time
	}{
		{"tanggal tes", in.TestAt, &s.TestAt},
		{"tanggal pengumuman", in.AnnouncementAt, &s.AnnouncementAt},
		{"batas bayar formulir", in.PaymentEndAt, &s.PaymentEnd},
		{"batas daftar ulang", in.ReregistrationEndAt, &s.ReregistrationEnd},
	} {
		t, err := optionalTime(f.name, f.value)
		if err != nil {
			return s, err
		}
		if t != nil {
			*f.dst = t
		}
	}
	for _, f := range []struct {
		name, value string
		dst         *time.Time
	}{
		{"tanggal buka pendaftaran", in.RegistrationOpenAt, &s.RegistrationStart},
		{"tanggal tutup pendaftaran", in.RegistrationCloseAt, &s.RegistrationEnd},
	} {
		t, err := optionalTime(f.name, f.value)
		if err != nil {
			return s, err
		}
		if t != nil {
			*f.dst = *t
		}
	}
	if s.RegistrationStart.IsZero() || s.RegistrationEnd.IsZero() {
		return s, &httpError{http.StatusUnprocessableEntity, "tanggal buka dan tutup pendaftaran wajib diisi"}
	}
	return s, nil
}

func ptrTime(t *time.Time) any {
	if t == nil {
		return nil
	}
	return *t
}

func timeIfSet(item map[string]any, key string, t *time.Time) {
	if t != nil {
		item[key] = *t
	}
}

// ListWaves is GET /admin/config/waves.
func (a *App) ListWaves(w http.ResponseWriter, r *http.Request) {
	if !common.RequireAdmin(w, r) {
		return
	}
	schoolID := common.EffectiveSchoolID(r)
	rows, err := a.DB.QueryContext(r.Context(),
		`SELECT w.id,w.period_id,w.name,w.registration_start,w.registration_end,w.payment_end,w.test_start,w.announcement_at,
		        w.reregistration_end,w.waitlist_reregistration_days,(SELECT count(*) FROM applications a WHERE a.wave_id=w.id),
		        COALESCE(w.education_level,''),w.entry_grade
		 FROM admission_waves w WHERE w.school_id=$1 AND w.deleted_at IS NULL AND ($2='' OR w.period_id::text=$2)
		   AND ($3='' OR w.education_level=$3)
		 ORDER BY w.registration_start`, schoolID, r.URL.Query().Get("period_id"), r.URL.Query().Get("education_level"))
	if err != nil {
		writeErr(w, err)
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var (
			wr                                   waveRow
			payEnd, testAt, announceAt, reregEnd sql.NullTime
			grade                                sql.NullInt64
		)
		if err := rows.Scan(&wr.ID, &wr.PeriodID, &wr.Name, &wr.Schedule.RegistrationStart, &wr.Schedule.RegistrationEnd,
			&payEnd, &testAt, &announceAt, &reregEnd, &wr.WaitlistDays, &wr.ApplicationCount, &wr.Level, &grade); err != nil {
			writeErr(w, err)
			return
		}
		item := map[string]any{
			"id": wr.ID, "period_id": wr.PeriodID, "name": wr.Name,
			"registration_open_at":         wr.Schedule.RegistrationStart,
			"registration_close_at":        wr.Schedule.RegistrationEnd,
			"waitlist_reregistration_days": wr.WaitlistDays,
			"application_count":            wr.ApplicationCount,
			"status":                       waveStatus(time.Now(), wr.Schedule.RegistrationStart, wr.Schedule.RegistrationEnd),
			"education_level":              wr.Level,
		}
		if grade.Valid {
			item["entry_grade"] = grade.Int64
		}
		timeIfSet(item, "payment_end_at", nullTimePtr(payEnd))
		timeIfSet(item, "test_at", nullTimePtr(testAt))
		timeIfSet(item, "announcement_at", nullTimePtr(announceAt))
		timeIfSet(item, "reregistration_end_at", nullTimePtr(reregEnd))
		out = append(out, item)
	}
	common.WriteJSON(w, http.StatusOK, out, nil)
}

// waveWarnings lists other waves of the period whose registration window
// overlaps [start,end].
func waveWarnings(ctx context.Context, q queryer, schoolID, periodID, excludeID, level string, start, end time.Time) ([]string, error) {
	rows, err := q.QueryContext(ctx,
		`SELECT name,registration_start,registration_end,COALESCE(education_level,'') FROM admission_waves
		 WHERE school_id=$1 AND period_id=$2 AND deleted_at IS NULL AND id::text<>$3`, schoolID, periodID, excludeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var warnings []string
	for rows.Next() {
		var (
			name, otherLevel string
			bs, be           time.Time
		)
		if err := rows.Scan(&name, &bs, &be, &otherLevel); err != nil {
			return nil, err
		}
		if model.SameLevel(level, otherLevel) && model.RangesOverlap(start, end, bs, be) {
			warnings = append(warnings, "Periode pendaftaran tumpang tindih dengan "+name+" ("+
				formatDateWIB(bs)+" s.d. "+formatDateWIB(be)+").")
		}
	}
	return warnings, rows.Err()
}

func (a *App) paymentGraceDays(ctx context.Context, schoolID, periodID string) (int, error) {
	var days int
	err := a.DB.QueryRowContext(ctx,
		`SELECT max_payment_grace_days FROM admission_periods WHERE id=$1 AND school_id=$2`, periodID, schoolID).Scan(&days)
	if err == sql.ErrNoRows {
		return 0, &httpError{http.StatusNotFound, "periode tidak ditemukan"}
	}
	return days, err
}

// CreateWave is POST /admin/config/waves.
func (a *App) CreateWave(w http.ResponseWriter, r *http.Request) {
	if !common.RequireAdmin(w, r) {
		return
	}
	schoolID := common.EffectiveSchoolID(r)
	claims := common.Admin(r)
	var in waveInput
	if !common.Decode(w, r, &in) {
		return
	}
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" {
		common.WriteError(w, http.StatusUnprocessableEntity, "nama gelombang wajib diisi")
		return
	}
	periodID, err := a.resolvePeriodID(r, schoolID, in.PeriodID)
	if err != nil {
		writeErr(w, err)
		return
	}
	sched, err := in.schedule(model.WaveSchedule{})
	if err != nil {
		writeErr(w, err)
		return
	}
	grace, err := a.paymentGraceDays(r.Context(), schoolID, periodID)
	if err != nil {
		writeErr(w, err)
		return
	}
	if err := model.ValidateWaveSchedule(sched, grace); err != nil {
		common.WriteError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	waitlistDays := 7
	if in.WaitlistReregistrationDays != nil {
		waitlistDays = *in.WaitlistReregistrationDays
	}
	if waitlistDays < 0 {
		common.WriteError(w, http.StatusUnprocessableEntity, "batas hari daftar ulang cadangan tidak boleh negatif")
		return
	}
	level, grade, err := in.level("", nil)
	if err != nil {
		writeErr(w, err)
		return
	}
	warnings, err := waveWarnings(r.Context(), a.DB, schoolID, periodID, "", level, sched.RegistrationStart, sched.RegistrationEnd)
	if err != nil {
		writeErr(w, err)
		return
	}
	var id string
	err = a.DB.QueryRowContext(r.Context(),
		`INSERT INTO admission_waves(school_id,period_id,name,registration_start,registration_end,payment_end,test_start,
		                             announcement_at,reregistration_end,waitlist_reregistration_days,education_level,entry_grade)
		 VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,NULLIF($11,''),$12) RETURNING id`,
		schoolID, periodID, in.Name, sched.RegistrationStart, sched.RegistrationEnd, ptrTime(sched.PaymentEnd),
		ptrTime(sched.TestAt), ptrTime(sched.AnnouncementAt), ptrTime(sched.ReregistrationEnd), waitlistDays,
		level, intPtrOrNil(grade)).Scan(&id)
	if err != nil {
		if isUniqueViolation(err) {
			common.WriteError(w, http.StatusConflict, "nama gelombang sudah dipakai pada periode ini")
			return
		}
		writeErr(w, err)
		return
	}
	a.audit(r.Context(), schoolID, "wave", id, "wave_created", "admin", claims.UserID, claims.Name, "", nil, in)
	common.WriteJSON(w, http.StatusCreated, map[string]any{"id": id, "warnings": nonNil(warnings)}, nil)
}

// UpdateWave is PUT /admin/config/waves/{id}.
func (a *App) UpdateWave(w http.ResponseWriter, r *http.Request) {
	if !common.RequireAdmin(w, r) {
		return
	}
	schoolID := common.EffectiveSchoolID(r)
	claims := common.Admin(r)
	var in waveInput
	if !common.Decode(w, r, &in) {
		return
	}
	var (
		cur                                  waveRow
		payEnd, testAt, announceAt, reregEnd sql.NullTime
		curGrade                             sql.NullInt64
	)
	err := a.DB.QueryRowContext(r.Context(),
		`SELECT w.id,w.period_id,w.name,w.registration_start,w.registration_end,w.payment_end,w.test_start,w.announcement_at,
		        w.reregistration_end,w.waitlist_reregistration_days,COALESCE(w.education_level,''),w.entry_grade,
		        (SELECT count(*) FROM applications a WHERE a.wave_id=w.id),
		        (SELECT count(*) FROM applications a WHERE a.wave_id=w.id AND a.status NOT IN('withdrawn','rejected','dropped','converted'))
		 FROM admission_waves w WHERE w.id=$1 AND w.school_id=$2 AND w.deleted_at IS NULL`, param(r, "id"), schoolID).
		Scan(&cur.ID, &cur.PeriodID, &cur.Name, &cur.Schedule.RegistrationStart, &cur.Schedule.RegistrationEnd,
			&payEnd, &testAt, &announceAt, &reregEnd, &cur.WaitlistDays, &cur.Level, &curGrade, &cur.ApplicationCount, &cur.ActiveApplicationsCnt)
	if err == sql.ErrNoRows {
		common.WriteError(w, http.StatusNotFound, "gelombang tidak ditemukan")
		return
	}
	if err != nil {
		writeErr(w, err)
		return
	}
	cur.Schedule.PaymentEnd, cur.Schedule.TestAt = nullTimePtr(payEnd), nullTimePtr(testAt)
	cur.Schedule.AnnouncementAt, cur.Schedule.ReregistrationEnd = nullTimePtr(announceAt), nullTimePtr(reregEnd)
	if curGrade.Valid {
		g := int(curGrade.Int64)
		cur.Grade = &g
	}
	level, grade, err := in.level(cur.Level, cur.Grade)
	if err != nil {
		writeErr(w, err)
		return
	}
	sched, err := in.schedule(cur.Schedule)
	if err != nil {
		writeErr(w, err)
		return
	}
	grace, err := a.paymentGraceDays(r.Context(), schoolID, cur.PeriodID)
	if err != nil {
		writeErr(w, err)
		return
	}
	if err := model.ValidateWaveSchedule(sched, grace); err != nil {
		common.WriteError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	waitlistDays := cur.WaitlistDays
	if in.WaitlistReregistrationDays != nil {
		waitlistDays = *in.WaitlistReregistrationDays
	}
	if waitlistDays < 0 {
		common.WriteError(w, http.StatusUnprocessableEntity, "batas hari daftar ulang cadangan tidak boleh negatif")
		return
	}
	changed := model.WaveScheduleChanged(cur.Schedule, sched) || waitlistDays != cur.WaitlistDays
	if changed && cur.ActiveApplicationsCnt > 0 && !in.ConfirmImpact {
		common.WriteJSON(w, http.StatusConflict, map[string]any{
			"error":               "Perubahan jadwal berdampak pada pendaftar yang sudah ada. Kirim ulang dengan confirm_impact=true untuk melanjutkan.",
			"requires_confirm":    true,
			"active_applications": cur.ActiveApplicationsCnt,
			"total_applications":  cur.ApplicationCount,
		}, nil)
		return
	}
	warnings, err := waveWarnings(r.Context(), a.DB, schoolID, cur.PeriodID, cur.ID, level, sched.RegistrationStart, sched.RegistrationEnd)
	if err != nil {
		writeErr(w, err)
		return
	}
	name := strings.TrimSpace(in.Name)
	if name == "" {
		name = cur.Name
	}
	if _, err = a.DB.ExecContext(r.Context(),
		`UPDATE admission_waves SET name=$3,registration_start=$4,registration_end=$5,payment_end=$6,test_start=$7,
		        announcement_at=$8,reregistration_end=$9,waitlist_reregistration_days=$10,
		        education_level=NULLIF($11,''),entry_grade=$12,updated_at=now()
		 WHERE id=$1 AND school_id=$2`,
		cur.ID, schoolID, name, sched.RegistrationStart, sched.RegistrationEnd, ptrTime(sched.PaymentEnd), ptrTime(sched.TestAt),
		ptrTime(sched.AnnouncementAt), ptrTime(sched.ReregistrationEnd), waitlistDays, level, intPtrOrNil(grade)); err != nil {
		if isUniqueViolation(err) {
			common.WriteError(w, http.StatusConflict, "nama gelombang sudah dipakai pada periode ini")
			return
		}
		writeErr(w, err)
		return
	}
	a.audit(r.Context(), schoolID, "wave", cur.ID, "wave_updated", "admin", claims.UserID, claims.Name, "",
		map[string]any{"schedule": cur.Schedule, "waitlist_reregistration_days": cur.WaitlistDays},
		map[string]any{"schedule": sched, "waitlist_reregistration_days": waitlistDays, "education_level": level,
			"entry_grade": intPtrOrNil(grade), "affected_applications": cur.ActiveApplicationsCnt})
	common.WriteJSON(w, http.StatusOK, map[string]any{"id": cur.ID, "warnings": nonNil(warnings),
		"affected_applications": cur.ActiveApplicationsCnt}, nil)
}

func nonNil(list []string) []string {
	if list == nil {
		return []string{}
	}
	return list
}

// ---------------------------------------------------------------------------
// Tracks (BUG-094/099/110)
// ---------------------------------------------------------------------------

type trackInput struct {
	PeriodID          string          `json:"period_id"`
	Code              string          `json:"code"`
	Name              string          `json:"name"`
	Description       string          `json:"description"`
	Quota             *int            `json:"quota"`
	IsActive          *bool           `json:"is_active"`
	EligibilityConfig json.RawMessage `json:"eligibility_config"`
}

func (in *trackInput) validate() error {
	if in.Quota != nil && *in.Quota < 0 {
		return &httpError{http.StatusUnprocessableEntity, "kuota minimal 0 (0 berarti jalur tidak menerima pendaftar)"}
	}
	if len(in.EligibilityConfig) > 0 {
		if _, err := model.ParseEligibility(in.EligibilityConfig); err != nil {
			return &httpError{http.StatusUnprocessableEntity, err.Error()}
		}
	}
	return nil
}

// ListTracks is GET /admin/config/tracks.
func (a *App) ListTracks(w http.ResponseWriter, r *http.Request) {
	if !common.RequireAdmin(w, r) {
		return
	}
	schoolID := common.EffectiveSchoolID(r)
	rows, err := a.DB.QueryContext(r.Context(),
		`SELECT t.id,t.period_id,t.code,t.name,COALESCE(t.description,''),t.quota,t.is_active,t.eligibility_config,
		        (SELECT count(*) FROM applications a WHERE a.track_id=t.id),
		        (SELECT count(*) FROM applications a WHERE a.track_id=t.id AND a.status NOT IN('draft','withdrawn','rejected','dropped')),
		        COALESCE((SELECT array_agg(dr.name ORDER BY dr.name) FROM document_requirements dr
		                  WHERE dr.track_id=t.id AND dr.deleted_at IS NULL),'{}')
		 FROM admission_tracks t WHERE t.school_id=$1 AND t.deleted_at IS NULL AND ($2='' OR t.period_id::text=$2)
		 ORDER BY t.name`, schoolID, r.URL.Query().Get("period_id"))
	if err != nil {
		writeErr(w, err)
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var (
			id, period, code, name, description string
			quota, applicants, active           int
			isActive                            bool
			eligibility                         []byte
			requirements                        []string
		)
		if err := rows.Scan(&id, &period, &code, &name, &description, &quota, &isActive, &eligibility,
			&applicants, &active, &requirements); err != nil {
			writeErr(w, err)
			return
		}
		out = append(out, map[string]any{
			"id": id, "period_id": period, "code": code, "name": name, "description": description,
			"quota": quota, "applicant_count": applicants, "active_applicant_count": active,
			"remaining_quota": model.VacantSeats(quota, active), "is_active": isActive,
			"eligibility_config": json.RawMessage(eligibility), "requirements": nonNil(requirements),
		})
	}
	common.WriteJSON(w, http.StatusOK, out, nil)
}

// CreateTrack is POST /admin/config/tracks.
func (a *App) CreateTrack(w http.ResponseWriter, r *http.Request) {
	if !common.RequireAdmin(w, r) {
		return
	}
	schoolID := common.EffectiveSchoolID(r)
	claims := common.Admin(r)
	var in trackInput
	if !common.Decode(w, r, &in) {
		return
	}
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" {
		common.WriteError(w, http.StatusUnprocessableEntity, "nama jalur wajib diisi")
		return
	}
	if in.Quota == nil {
		common.WriteError(w, http.StatusUnprocessableEntity, "kuota jalur wajib diisi")
		return
	}
	if err := in.validate(); err != nil {
		writeErr(w, err)
		return
	}
	periodID, err := a.resolvePeriodID(r, schoolID, in.PeriodID)
	if err != nil {
		writeErr(w, err)
		return
	}
	code := strings.ToUpper(strings.TrimSpace(in.Code))
	if code == "" {
		code = strings.ToUpper(truncateSlug(in.Name))
	}
	if code == "" {
		code = "TRACK"
	}
	eligibility := []byte("{}")
	if len(in.EligibilityConfig) > 0 && string(in.EligibilityConfig) != "null" {
		eligibility = in.EligibilityConfig
	}
	isActive := in.IsActive == nil || *in.IsActive
	var id string
	err = a.DB.QueryRowContext(r.Context(),
		`INSERT INTO admission_tracks(school_id,period_id,code,name,description,quota,is_active,eligibility_config)
		 SELECT $1,p.id,$2,$3,NULLIF($4,''),$5,$6,$8::jsonb FROM admission_periods p WHERE p.id=$7 AND p.school_id=$1 RETURNING id`,
		schoolID, code, in.Name, in.Description, *in.Quota, isActive, periodID, string(eligibility)).Scan(&id)
	if err != nil {
		if isUniqueViolation(err) {
			common.WriteError(w, http.StatusConflict, "kode jalur sudah dipakai pada periode ini")
			return
		}
		writeErr(w, err)
		return
	}
	a.audit(r.Context(), schoolID, "track", id, "track_created", "admin", claims.UserID, claims.Name, "", nil, in)
	common.WriteJSON(w, http.StatusCreated, map[string]any{"id": id}, nil)
}

// UpdateTrack is PUT /admin/config/tracks/{id}. Omitted fields keep their value.
func (a *App) UpdateTrack(w http.ResponseWriter, r *http.Request) {
	if !common.RequireAdmin(w, r) {
		return
	}
	schoolID := common.EffectiveSchoolID(r)
	claims := common.Admin(r)
	var in trackInput
	if !common.Decode(w, r, &in) {
		return
	}
	if err := in.validate(); err != nil {
		writeErr(w, err)
		return
	}
	var eligibility any
	if len(in.EligibilityConfig) > 0 && string(in.EligibilityConfig) != "null" {
		eligibility = string(in.EligibilityConfig)
	}
	var quota any
	if in.Quota != nil {
		quota = *in.Quota
	}
	var active any
	if in.IsActive != nil {
		active = *in.IsActive
	}
	res, err := a.DB.ExecContext(r.Context(),
		`UPDATE admission_tracks SET name=COALESCE(NULLIF($1,''),name),description=NULLIF($2,''),
		        quota=COALESCE($3::int,quota),is_active=COALESCE($4::boolean,is_active),
		        eligibility_config=COALESCE($5::jsonb,eligibility_config),updated_at=now()
		 WHERE id=$6 AND school_id=$7`,
		strings.TrimSpace(in.Name), in.Description, quota, active, eligibility, param(r, "id"), schoolID)
	if err != nil {
		writeErr(w, err)
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		common.WriteError(w, http.StatusNotFound, "jalur tidak ditemukan")
		return
	}
	a.audit(r.Context(), schoolID, "track", param(r, "id"), "track_updated", "admin", claims.UserID, claims.Name, "", nil, in)
	common.WriteJSON(w, http.StatusOK, map[string]any{"id": param(r, "id")}, nil)
}
