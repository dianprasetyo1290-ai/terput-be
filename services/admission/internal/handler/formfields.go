package handler

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"strings"

	"skci/admission/internal/common"
	"skci/admission/internal/model"
)

// Registration form builder (BUG-095). Admins add fields per period and mark
// them mandatory; a field keyed like a built-in biodata column (nisn, nik,
// address, ...) makes that column mandatory instead of adding a new input.
// Values of custom fields live in applicants.custom_fields and are checked
// when the application is submitted.

type formFieldInput struct {
	PeriodID  string   `json:"period_id"`
	Key       string   `json:"key"`
	Label     string   `json:"label"`
	Type      string   `json:"type"`
	Options   []string `json:"options"`
	Required  *bool    `json:"required"`
	HelpText  string   `json:"help_text"`
	SortOrder *int     `json:"sort_order"`
	Active    *bool    `json:"active"`
}

type formFieldRow struct {
	model.FormField
	ID        string `json:"id"`
	PeriodID  string `json:"period_id"`
	HelpText  string `json:"help_text"`
	SortOrder int    `json:"sort_order"`
	Active    bool   `json:"active"`
	Builtin   bool   `json:"builtin"`
}

// formFields loads the fields of a school, optionally limited to one period
// and/or to periods with the given status, active ones only when activeOnly.
func formFields(ctx context.Context, q queryer, schoolID, periodID, periodStatus string, activeOnly bool) ([]formFieldRow, error) {
	rows, err := q.QueryContext(ctx,
		`SELECT f.id,f.period_id,f.field_key,f.label,f.field_type,f.options,f.required,f.help_text,f.sort_order,f.active
		 FROM admission_form_fields f JOIN admission_periods p ON p.id=f.period_id
		 WHERE f.school_id=$1 AND ($2='' OR f.period_id::text=$2) AND ($3='' OR p.status=$3) AND (NOT $4 OR f.active)
		 ORDER BY f.sort_order,f.label`, schoolID, periodID, periodStatus, activeOnly)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []formFieldRow{}
	for rows.Next() {
		var (
			f       formFieldRow
			options []byte
		)
		if err := rows.Scan(&f.ID, &f.PeriodID, &f.Key, &f.Label, &f.Type, &options, &f.Required, &f.HelpText, &f.SortOrder, &f.Active); err != nil {
			return nil, err
		}
		f.Options = []string{}
		if len(options) > 0 {
			if err := json.Unmarshal(options, &f.Options); err != nil {
				return nil, err
			}
		}
		_, f.Builtin = model.BuiltinApplicantFields[f.Key]
		out = append(out, f)
	}
	return out, rows.Err()
}

func plainFields(rows []formFieldRow) []model.FormField {
	out := make([]model.FormField, 0, len(rows))
	for _, r := range rows {
		out = append(out, r.FormField)
	}
	return out
}

// ListFormFields is GET /admin/config/form-fields?period_id=.
func (a *App) ListFormFields(w http.ResponseWriter, r *http.Request) {
	if !common.RequireAdmin(w, r) {
		return
	}
	out, err := formFields(r.Context(), a.DB, common.EffectiveSchoolID(r), r.URL.Query().Get("period_id"), "", false)
	if err != nil {
		writeErr(w, err)
		return
	}
	builtin := []map[string]string{}
	for key, label := range model.BuiltinApplicantFields {
		builtin = append(builtin, map[string]string{"key": key, "label": label})
	}
	common.WriteJSON(w, http.StatusOK, map[string]any{"items": out, "builtin_fields": builtin}, nil)
}

func (in *formFieldInput) field() model.FormField {
	opts := []string{}
	for _, o := range in.Options {
		if o = strings.TrimSpace(o); o != "" {
			opts = append(opts, o)
		}
	}
	return model.FormField{Key: strings.TrimSpace(in.Key), Label: strings.TrimSpace(in.Label), Type: in.Type,
		Options: opts, Required: in.Required != nil && *in.Required}
}

// CreateFormField is POST /admin/config/form-fields.
func (a *App) CreateFormField(w http.ResponseWriter, r *http.Request) {
	if !common.RequireAdmin(w, r) {
		return
	}
	schoolID := common.EffectiveSchoolID(r)
	claims := common.Admin(r)
	var in formFieldInput
	if !common.Decode(w, r, &in) {
		return
	}
	f := in.field()
	if err := model.ValidateFormField(f); err != nil {
		common.WriteError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	var periodOK bool
	if err := a.DB.QueryRowContext(r.Context(),
		`SELECT EXISTS(SELECT 1 FROM admission_periods WHERE id::text=$1 AND school_id=$2 AND deleted_at IS NULL)`,
		in.PeriodID, schoolID).Scan(&periodOK); err != nil {
		writeErr(w, err)
		return
	}
	if !periodOK {
		common.WriteError(w, http.StatusUnprocessableEntity, "periode tidak ditemukan")
		return
	}
	order := 0
	if in.SortOrder != nil {
		order = *in.SortOrder
	}
	active := in.Active == nil || *in.Active
	var id string
	err := a.DB.QueryRowContext(r.Context(),
		`INSERT INTO admission_form_fields(school_id,period_id,field_key,label,field_type,options,required,help_text,sort_order,active)
		 VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
		 ON CONFLICT (school_id,period_id,field_key) DO NOTHING RETURNING id`,
		schoolID, in.PeriodID, f.Key, f.Label, f.Type, jsonRaw(f.Options), f.Required, strings.TrimSpace(in.HelpText), order, active).Scan(&id)
	if err == sql.ErrNoRows {
		common.WriteError(w, http.StatusConflict, "kode field sudah dipakai pada periode ini")
		return
	}
	if err != nil {
		writeErr(w, err)
		return
	}
	a.audit(r.Context(), schoolID, "form_field", id, "form_field_created", "admin", claims.UserID, claims.Name, "", nil, f)
	common.WriteJSON(w, http.StatusCreated, map[string]any{"id": id}, nil)
}

// UpdateFormField is PUT /admin/config/form-fields/{id}. The key and period
// are fixed once created so stored answers keep their meaning.
func (a *App) UpdateFormField(w http.ResponseWriter, r *http.Request) {
	if !common.RequireAdmin(w, r) {
		return
	}
	schoolID := common.EffectiveSchoolID(r)
	claims := common.Admin(r)
	var in formFieldInput
	if !common.Decode(w, r, &in) {
		return
	}
	var key string
	err := a.DB.QueryRowContext(r.Context(),
		`SELECT field_key FROM admission_form_fields WHERE id::text=$1 AND school_id=$2`, param(r, "id"), schoolID).Scan(&key)
	if err == sql.ErrNoRows {
		common.WriteError(w, http.StatusNotFound, "field tidak ditemukan")
		return
	}
	if err != nil {
		writeErr(w, err)
		return
	}
	in.Key = key
	f := in.field()
	if err := model.ValidateFormField(f); err != nil {
		common.WriteError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	order := 0
	if in.SortOrder != nil {
		order = *in.SortOrder
	}
	active := in.Active == nil || *in.Active
	if _, err := a.DB.ExecContext(r.Context(),
		`UPDATE admission_form_fields SET label=$3,field_type=$4,options=$5,required=$6,help_text=$7,sort_order=$8,active=$9,updated_at=now()
		 WHERE id::text=$1 AND school_id=$2`,
		param(r, "id"), schoolID, f.Label, f.Type, jsonRaw(f.Options), f.Required, strings.TrimSpace(in.HelpText), order, active); err != nil {
		writeErr(w, err)
		return
	}
	a.audit(r.Context(), schoolID, "form_field", param(r, "id"), "form_field_updated", "admin", claims.UserID, claims.Name, "", nil, f)
	common.WriteJSON(w, http.StatusOK, map[string]any{"id": param(r, "id")}, nil)
}

// normalizeApplicantCustom validates custom answers against the active
// fields of the school's open periods.
func normalizeApplicantCustom(ctx context.Context, q queryer, schoolID string, values map[string]any) (map[string]any, error) {
	if len(values) == 0 {
		return map[string]any{}, nil
	}
	rows, err := formFields(ctx, q, schoolID, "", "open", true)
	if err != nil {
		return nil, err
	}
	out, err := model.NormalizeCustomValues(plainFields(rows), values)
	if err != nil {
		return nil, &httpError{http.StatusUnprocessableEntity, err.Error()}
	}
	return out, nil
}

// missingFormFields checks the application's period form against the
// applicant's answers at submission.
func missingFormFields(ctx context.Context, q queryer, schoolID, applicationID string) ([]string, error) {
	var (
		periodID                                                               string
		custom                                                                 []byte
		builtin                                                                = map[string]string{}
		nickname, nisn, nik, birthPlace, previousSchool, address, relationship string
	)
	if err := q.QueryRowContext(ctx,
		`SELECT a.period_id,ap.custom_fields,COALESCE(ap.nickname,''),COALESCE(ap.nisn,''),COALESCE(ap.nik,''),COALESCE(ap.birth_place,''),
		        COALESCE(ap.previous_school,''),COALESCE(ap.address,''),COALESCE(ap.guardian_relationship,'')
		 FROM applications a JOIN applicants ap ON ap.id=a.applicant_id WHERE a.id=$1 AND a.school_id=$2`,
		applicationID, schoolID).Scan(&periodID, &custom, &nickname, &nisn, &nik, &birthPlace, &previousSchool, &address, &relationship); err != nil {
		return nil, err
	}
	builtin["nickname"], builtin["nisn"], builtin["nik"], builtin["birth_place"] = nickname, nisn, nik, birthPlace
	builtin["previous_school"], builtin["address"], builtin["guardian_relationship"] = previousSchool, address, relationship
	values := map[string]any{}
	if len(custom) > 0 {
		if err := json.Unmarshal(custom, &values); err != nil {
			return nil, err
		}
	}
	rows, err := formFields(ctx, q, schoolID, periodID, "", true)
	if err != nil {
		return nil, err
	}
	return model.MissingFormFields(plainFields(rows), builtin, values), nil
}
