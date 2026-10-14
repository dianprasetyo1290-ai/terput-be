package handler

import (
	"context"
	"database/sql"
	"net/http"
	"strings"

	"skci/admission/internal/model"
)

// documentIssues lists why an application's registration-stage documents do
// not yet allow the test stage: every required requirement (period-wide or
// for the application's track) needs an uploaded file whose latest review is
// "accepted" (BUG-033).
func documentIssues(ctx context.Context, q queryer, schoolID, applicationID string) ([]string, error) {
	rows, err := q.QueryContext(ctx,
		`SELECT dr.name,d.id IS NOT NULL,COALESCE(rev.status,'')
		 FROM applications a
		 JOIN document_requirements dr ON dr.period_id=a.period_id AND dr.school_id=a.school_id
		      AND (dr.track_id IS NULL OR dr.track_id=a.track_id)
		 LEFT JOIN application_documents d ON d.application_id=a.id AND d.requirement_id=dr.id
		 LEFT JOIN LATERAL (SELECT status FROM document_reviews rv WHERE rv.document_id=d.id
		                    ORDER BY rv.created_at DESC LIMIT 1) rev ON true
		 WHERE a.id=$1 AND a.school_id=$2 AND dr.required AND dr.deleted_at IS NULL
		   AND dr.stage='registration'
		 ORDER BY dr.name`, applicationID, schoolID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var issues []string
	for rows.Next() {
		var (
			name     string
			uploaded bool
			review   string
		)
		if err := rows.Scan(&name, &uploaded, &review); err != nil {
			return nil, err
		}
		if msg := model.DocumentIssue(name, uploaded, review); msg != "" {
			issues = append(issues, msg)
		}
	}
	return issues, rows.Err()
}

// eligibilityIssues is everything that blocks mark_eligible: the form fee
// must be settled (BR-PPDB-06, BUG-107) and the required documents accepted.
func eligibilityIssues(ctx context.Context, q queryer, schoolID, applicationID string) ([]string, error) {
	var issues []string
	settled, charge, _, err := stageSettled(ctx, q, schoolID, applicationID, stageRegistration)
	if err != nil {
		return nil, err
	}
	if charge == nil {
		issues = append(issues, "Tagihan formulir belum terbit.")
	} else if !settled {
		issues = append(issues, "Biaya formulir belum lunas.")
	}
	docs, err := documentIssues(ctx, q, schoolID, applicationID)
	if err != nil {
		return nil, err
	}
	return append(issues, docs...), nil
}

func issuesError(prefix string, issues []string) error {
	return &httpError{http.StatusUnprocessableEntity, prefix + " " + strings.Join(issues, " ")}
}

// afterPaymentVerified advances the application when a verified payment
// settles a stage. Runs inside the payment review transaction.
//   - Form fee settled while "submitted": the file enters document review
//     (BR-PPDB-06: verification only starts once the form is paid).
//   - Re-registration minimum met: the guardian is told the seat is secured.
func (a *App) afterPaymentVerified(ctx context.Context, tx *sql.Tx, schoolID, applicationID, stage string) error {
	settled, _, _, err := stageSettled(ctx, tx, schoolID, applicationID, stage)
	if err != nil || !settled {
		return err
	}
	var status string
	if err := tx.QueryRowContext(ctx,
		`SELECT status FROM applications WHERE id=$1 AND school_id=$2`, applicationID, schoolID).Scan(&status); err != nil {
		return err
	}
	switch stage {
	case stageRegistration:
		if model.ApplicationStatus(status) != model.StatusSubmitted {
			return nil
		}
		if _, err := a.transitionTx(ctx, tx, schoolID, applicationID, "system", "admission", "Sistem PPDB",
			model.StatusDocumentReview, "", 0, ""); err != nil {
			return err
		}
		return a.notify(ctx, tx, schoolID, applicationID, "form_paid", "Biaya formulir lunas",
			"Pembayaran formulir sudah lunas. Berkas pendaftaran sedang diverifikasi panitia.",
			"form_paid:"+applicationID, nil)
	case stageReregistration:
		return a.notify(ctx, tx, schoolID, applicationID, "reregistration_min_paid", "Pembayaran daftar ulang diterima",
			"Pembayaran minimum daftar ulang sudah terpenuhi. Kursi ananda telah diamankan.",
			"rereg_min:"+applicationID, nil)
	}
	return nil
}
