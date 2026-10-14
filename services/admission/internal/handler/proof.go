package handler

import (
	"context"
	"database/sql"
	"net/http"
	"strconv"
	"time"

	"skci/admission/internal/common"
	"skci/admission/internal/simplepdf"
)

// Registration proof PDF (BUG-162): available once the form is submitted.

// GuardianRegistrationProof is GET /guardian/applications/{id}/proof.pdf.
func (a *App) GuardianRegistrationProof(w http.ResponseWriter, r *http.Request) {
	c := common.Guardian(r)
	a.writeProof(w, r, c.SchoolID, param(r, "id"), c.GuardianID)
}

// AdminRegistrationProof is GET /admin/applications/{id}/proof.pdf.
func (a *App) AdminRegistrationProof(w http.ResponseWriter, r *http.Request) {
	if !common.RequireAdmin(w, r) {
		return
	}
	a.writeProof(w, r, common.EffectiveSchoolID(r), param(r, "id"), "")
}

func (a *App) writeProof(w http.ResponseWriter, r *http.Request, schoolID, applicationID, guardianID string) {
	lines, number, err := a.proofLines(r.Context(), schoolID, applicationID, guardianID)
	if err != nil {
		writeErr(w, err)
		return
	}
	pdf := simplepdf.Render("Bukti Pendaftaran "+number, lines)
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", `attachment; filename="bukti-pendaftaran-`+number+`.pdf"`)
	w.Header().Set("Content-Length", strconv.Itoa(len(pdf)))
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(pdf)
}

func (a *App) proofLines(ctx context.Context, schoolID, applicationID, guardianID string) ([]simplepdf.Line, string, error) {
	var (
		number, status, name, birthPlace, period, year, wave, track, guardian string
		birth, submitted                                                      sql.NullTime
	)
	err := a.DB.QueryRowContext(ctx,
		`SELECT a.registration_number,a.status,ap.full_name,COALESCE(ap.birth_place,''),ap.birth_date,a.submitted_at,
		        p.name,p.academic_year,w.name,t.name,g.name
		 FROM applications a JOIN applicants ap ON ap.id=a.applicant_id JOIN guardians g ON g.id=ap.guardian_id
		 JOIN admission_periods p ON p.id=a.period_id JOIN admission_waves w ON w.id=a.wave_id JOIN admission_tracks t ON t.id=a.track_id
		 WHERE a.id=$1 AND a.school_id=$2 AND ($3='' OR ap.guardian_id::text=$3)`, applicationID, schoolID, guardianID).
		Scan(&number, &status, &name, &birthPlace, &birth, &submitted, &period, &year, &wave, &track, &guardian)
	if err == sql.ErrNoRows {
		return nil, "", &httpError{http.StatusNotFound, "pendaftaran tidak ditemukan"}
	}
	if err != nil {
		return nil, "", err
	}
	if !submitted.Valid {
		return nil, "", &httpError{http.StatusUnprocessableEntity, "bukti pendaftaran tersedia setelah formulir dikirim"}
	}
	birthText := "-"
	if birth.Valid {
		birthText = birth.Time.Format("02-01-2006")
		if birthPlace != "" {
			birthText = birthPlace + ", " + birthText
		}
	}
	lines := []simplepdf.Line{
		{Text: a.Config.SchoolName, Bold: true, Size: 14},
		{Text: "BUKTI PENDAFTARAN PESERTA DIDIK BARU", Bold: true, Size: 16},
		{Text: ""},
		{Text: "Nomor pendaftaran : " + number, Bold: true},
		{Text: "Nama calon siswa  : " + name},
		{Text: "Tempat, tgl lahir : " + birthText},
		{Text: "Orang tua/wali    : " + guardian},
		{Text: "Periode           : " + period + " (" + year + ")"},
		{Text: "Gelombang         : " + wave},
		{Text: "Jalur             : " + track},
		{Text: "Dikirim pada      : " + formatDateWIB(submitted.Time)},
		{Text: "Status saat ini   : " + statusTitle(status)},
	}
	charge, err := latestCharge(ctx, a.DB, schoolID, applicationID, stageRegistration)
	if err != nil {
		return nil, "", err
	}
	if charge != nil {
		paid, err := stagePaid(ctx, a.DB, schoolID, applicationID, stageRegistration)
		if err != nil {
			return nil, "", err
		}
		state := "belum lunas"
		if paid >= charge.Net {
			state = "lunas"
		}
		line := "Biaya formulir    : " + formatIDR(charge.Net) + " (" + state + ")"
		if charge.DueAt.Valid && paid < charge.Net {
			line += ", batas bayar " + formatDateWIB(charge.DueAt.Time)
		}
		lines = append(lines, simplepdf.Line{Text: line})
	}
	lines = append(lines,
		simplepdf.Line{Text: ""},
		simplepdf.Line{Text: "Simpan bukti ini dan tunjukkan nomor pendaftaran saat verifikasi atau tes.", Size: 10},
		simplepdf.Line{Text: "Dicetak " + formatDateWIB(time.Now()) + " dari portal PPDB.", Size: 9},
	)
	return lines, number, nil
}
