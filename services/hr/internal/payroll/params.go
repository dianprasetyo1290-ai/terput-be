package payroll

import (
	"context"
	"database/sql"
	"time"

	"skci/hr/internal/payroll/calc"
)

type querier interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

// StatutoryRow is one parameter row as the Parameter PPh & BPJS page shows it.
type StatutoryRow struct {
	Key            string  `json:"key"`
	Category       string  `json:"category,omitempty"`
	LowerIDR       *int64  `json:"lower_idr,omitempty"`
	UpperIDR       *int64  `json:"upper_idr,omitempty"`
	RateBP         *int    `json:"rate_bp,omitempty"`
	EmployerBP     *int    `json:"employer_bp,omitempty"`
	EmployeeBP     *int    `json:"employee_bp,omitempty"`
	WageCapIDR     *int64  `json:"wage_cap_idr,omitempty"`
	ValueIDR       *int64  `json:"value,omitempty"`
	EffectiveFrom  string  `json:"effective_from"`
	EffectiveTo    *string `json:"effective_to,omitempty"`
	SourceNote     string  `json:"source_note,omitempty"`
	SchoolSpecific bool    `json:"school_specific,omitempty"`
}

// Statutory is GET /payroll/statutory: every table effective on one date.
type Statutory struct {
	Date        string         `json:"date"`
	BPJS        []StatutoryRow `json:"bpjs"`
	PTKP        []StatutoryRow `json:"ptkp"`
	TERCategory []StatutoryRow `json:"ter_categories"`
	TER         []StatutoryRow `json:"ter_brackets"`
	Progressive []StatutoryRow `json:"progressive"`
	Misc        []StatutoryRow `json:"misc"`
	Missing     []string       `json:"missing"`
}

const effective = `effective_from <= $1 AND (effective_to IS NULL OR effective_to >= $1)`

// LoadStatutory reads the parameter tables as of date. A school-specific
// BPJS row overrides the national (school_id NULL) one.
func LoadStatutory(ctx context.Context, q querier, schoolID string, date time.Time) (*Statutory, calc.Params, error) {
	d := date.Format("2006-01-02")
	st := &Statutory{Date: d, BPJS: []StatutoryRow{}, PTKP: []StatutoryRow{}, TERCategory: []StatutoryRow{}, TER: []StatutoryRow{}, Progressive: []StatutoryRow{}, Misc: []StatutoryRow{}, Missing: []string{}}
	p := calc.Params{BPJS: map[string]calc.BPJSRule{}, TER: map[string][]calc.TERBracket{}, TERCat: map[string]string{}, PTKP: map[string]int64{}}

	rows, err := q.QueryContext(ctx, `
		SELECT DISTINCT ON (program) program, employer_bp, employee_bp, wage_cap_idr,
		       effective_from::text, effective_to::text, source_note, school_id IS NOT NULL
		FROM statutory_bpjs
		WHERE `+effective+` AND (school_id IS NULL OR school_id = $2::uuid)
		ORDER BY program, (school_id IS NOT NULL) DESC, effective_from DESC`, d, schoolID)
	if err != nil {
		return nil, p, err
	}
	for rows.Next() {
		var r StatutoryRow
		var er, ee int
		var capIDR sql.NullInt64
		if err := rows.Scan(&r.Key, &er, &ee, &capIDR, &r.EffectiveFrom, &r.EffectiveTo, &r.SourceNote, &r.SchoolSpecific); err != nil {
			rows.Close()
			return nil, p, err
		}
		r.EmployerBP, r.EmployeeBP = &er, &ee
		rule := calc.BPJSRule{EmployerBP: er, EmployeeBP: ee}
		if capIDR.Valid {
			r.WageCapIDR = &capIDR.Int64
			rule.WageCapIDR = capIDR.Int64
		}
		p.BPJS[r.Key] = rule
		st.BPJS = append(st.BPJS, r)
	}
	rows.Close()

	rows, err = q.QueryContext(ctx, `
		SELECT DISTINCT ON (status_code) status_code, annual_idr, effective_from::text, effective_to::text, source_note
		FROM statutory_ptkp WHERE `+effective+`
		ORDER BY status_code, effective_from DESC`, d)
	if err != nil {
		return nil, p, err
	}
	for rows.Next() {
		var r StatutoryRow
		var v int64
		if err := rows.Scan(&r.Key, &v, &r.EffectiveFrom, &r.EffectiveTo, &r.SourceNote); err != nil {
			rows.Close()
			return nil, p, err
		}
		r.ValueIDR = &v
		p.PTKP[r.Key] = v
		st.PTKP = append(st.PTKP, r)
	}
	rows.Close()

	rows, err = q.QueryContext(ctx, `
		SELECT DISTINCT ON (ptkp_status_code) ptkp_status_code, category, effective_from::text, effective_to::text
		FROM statutory_ter_categories WHERE `+effective+`
		ORDER BY ptkp_status_code, effective_from DESC`, d)
	if err != nil {
		return nil, p, err
	}
	for rows.Next() {
		var r StatutoryRow
		if err := rows.Scan(&r.Key, &r.Category, &r.EffectiveFrom, &r.EffectiveTo); err != nil {
			rows.Close()
			return nil, p, err
		}
		p.TERCat[r.Key] = r.Category
		st.TERCategory = append(st.TERCategory, r)
	}
	rows.Close()

	// Brackets of the latest table version per category.
	rows, err = q.QueryContext(ctx, `
		SELECT category, lower_idr, upper_idr, rate_bp, effective_from::text, effective_to::text
		FROM statutory_ter_brackets b
		WHERE `+effective+` AND effective_from = (
			SELECT max(effective_from) FROM statutory_ter_brackets x
			WHERE x.category = b.category AND x.effective_from <= $1 AND (x.effective_to IS NULL OR x.effective_to >= $1))
		ORDER BY category, lower_idr`, d)
	if err != nil {
		return nil, p, err
	}
	for rows.Next() {
		var r StatutoryRow
		var lo int64
		var up sql.NullInt64
		var rate int
		if err := rows.Scan(&r.Category, &lo, &up, &rate, &r.EffectiveFrom, &r.EffectiveTo); err != nil {
			rows.Close()
			return nil, p, err
		}
		r.Key = "ter_" + r.Category
		r.LowerIDR, r.RateBP = &lo, &rate
		b := calc.TERBracket{LowerIDR: lo, RateBP: rate}
		if up.Valid {
			r.UpperIDR = &up.Int64
			b.UpperIDR = up.Int64
		}
		p.TER[r.Category] = append(p.TER[r.Category], b)
		st.TER = append(st.TER, r)
	}
	rows.Close()

	rows, err = q.QueryContext(ctx, `
		SELECT lower_idr, upper_idr, rate_bp, effective_from::text, effective_to::text
		FROM statutory_progressive p
		WHERE `+effective+` AND effective_from = (
			SELECT max(effective_from) FROM statutory_progressive x WHERE x.effective_from <= $1 AND (x.effective_to IS NULL OR x.effective_to >= $1))
		ORDER BY lower_idr`, d)
	if err != nil {
		return nil, p, err
	}
	for rows.Next() {
		var r StatutoryRow
		var lo int64
		var up sql.NullInt64
		var rate int
		if err := rows.Scan(&lo, &up, &rate, &r.EffectiveFrom, &r.EffectiveTo); err != nil {
			rows.Close()
			return nil, p, err
		}
		r.Key = "progressive"
		r.LowerIDR, r.RateBP = &lo, &rate
		b := calc.TERBracket{LowerIDR: lo, RateBP: rate}
		if up.Valid {
			r.UpperIDR = &up.Int64
			b.UpperIDR = up.Int64
		}
		p.Progressive = append(p.Progressive, b)
		st.Progressive = append(st.Progressive, r)
	}
	rows.Close()

	rows, err = q.QueryContext(ctx, `
		SELECT DISTINCT ON (key) key, value, effective_from::text, effective_to::text
		FROM statutory_misc WHERE `+effective+`
		ORDER BY key, effective_from DESC`, d)
	if err != nil {
		return nil, p, err
	}
	for rows.Next() {
		var r StatutoryRow
		var v int64
		if err := rows.Scan(&r.Key, &v, &r.EffectiveFrom, &r.EffectiveTo); err != nil {
			rows.Close()
			return nil, p, err
		}
		r.ValueIDR = &v
		switch r.Key {
		case "biaya_jabatan_bp":
			p.BiayaJabatanBP = int(v)
		case "biaya_jabatan_max_annual_idr":
			p.BiayaJabatanMaxAnnual = v
		}
		st.Misc = append(st.Misc, r)
	}
	rows.Close()

	for _, prog := range calc.BPJSPrograms {
		if _, ok := p.BPJS[prog]; !ok {
			st.Missing = append(st.Missing, "BPJS "+prog)
		}
	}
	for _, cat := range []string{"A", "B", "C"} {
		b := p.TER[cat]
		if len(b) == 0 {
			st.Missing = append(st.Missing, "tabel TER kategori "+cat)
		} else if b[len(b)-1].UpperIDR != 0 {
			st.Missing = append(st.Missing, "tabel TER kategori "+cat+" belum sampai bracket teratas")
		}
	}
	if len(p.Progressive) == 0 {
		st.Missing = append(st.Missing, "tarif progresif Pasal 17")
	}
	if p.BiayaJabatanBP == 0 || p.BiayaJabatanMaxAnnual == 0 {
		st.Missing = append(st.Missing, "biaya jabatan")
	}
	return st, p, nil
}
