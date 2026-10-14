package handler

import (
	"context"
	"database/sql"
	"net/http"
	"strings"
	"time"

	"skci/admission/internal/client"
	"skci/admission/internal/common"
	"skci/admission/internal/model"
)

// Maker-checker for PPDB goes through core's Approval Center: admission only
// opens the request and applies the outcome when core calls back
// (POST /internal/approval-callbacks, X-Internal-Key, Idempotency-Key
// approval:<id>). Nothing is applied before the callback says "approved".

func (a *App) callbackURL() string {
	return a.Config.InternalURL + "/internal/approval-callbacks"
}

// openApproval creates the core approval and maps a failure to the HTTP
// answer of the source endpoint.
func (a *App) openApproval(ctx context.Context, req client.ApprovalRequest) (string, error) {
	if a.Core == nil || a.Config.CoreURL == "" {
		return "", &httpError{http.StatusServiceUnavailable, "Approval Center belum dikonfigurasi (CORE_URL)"}
	}
	req.CallbackURL = a.callbackURL()
	id, err := a.Core.CreateApproval(ctx, req)
	if err != nil {
		status, msg := client.ApprovalErrorMessage(err)
		return "", &httpError{status, msg}
	}
	return id, nil
}

// ---------------------------------------------------------------------------
// Special (manual) discounts — BUG-032, BR-PPDB-03
// ---------------------------------------------------------------------------

type specialDiscountInput struct {
	Type   string `json:"type"`
	Value  int64  `json:"value"`
	Cap    int64  `json:"cap"`
	Reason string `json:"reason"`
	Stage  string `json:"stage"`
}

// CreateSpecialDiscount is POST /admin/applications/{id}/special-discounts.
// The discount is stored as pending and only reaches the bill after the
// Approval Center approves it.
func (a *App) CreateSpecialDiscount(w http.ResponseWriter, r *http.Request) {
	if !common.RequireAdmin(w, r) {
		return
	}
	schoolID := common.EffectiveSchoolID(r)
	claims := common.Admin(r)
	applicationID := param(r, "id")
	var in specialDiscountInput
	if !common.Decode(w, r, &in) {
		return
	}
	in.Reason = strings.TrimSpace(in.Reason)
	if in.Reason == "" {
		common.WriteError(w, http.StatusUnprocessableEntity, "alasan diskon khusus wajib diisi")
		return
	}
	if in.Stage == "" {
		in.Stage = stageReregistration
	}
	if in.Stage != stageRegistration && in.Stage != stageReregistration {
		common.WriteError(w, http.StatusUnprocessableEntity, "tahap tagihan tidak valid")
		return
	}
	kind, value, err := ruleKindFromAPI(in.Type, in.Value)
	if err != nil {
		writeErr(w, err)
		return
	}
	if value <= 0 || in.Cap < 0 {
		common.WriteError(w, http.StatusUnprocessableEntity, "nilai diskon harus lebih dari 0")
		return
	}

	tx, err := a.DB.BeginTx(r.Context(), nil)
	if err != nil {
		writeErr(w, err)
		return
	}
	defer func() { _ = tx.Rollback() }()

	var (
		status, registrationNumber, applicantName string
	)
	err = tx.QueryRowContext(r.Context(),
		`SELECT a.status,a.registration_number,ap.full_name FROM applications a JOIN applicants ap ON ap.id=a.applicant_id
		 WHERE a.id=$1 AND a.school_id=$2 FOR UPDATE OF a`, applicationID, schoolID).Scan(&status, &registrationNumber, &applicantName)
	if err == sql.ErrNoRows {
		common.WriteError(w, http.StatusNotFound, "pendaftaran tidak ditemukan")
		return
	}
	if err != nil {
		writeErr(w, err)
		return
	}
	switch model.ApplicationStatus(status) {
	case model.StatusWithdrawn, model.StatusRejected, model.StatusDropped, model.StatusDraft:
		common.WriteError(w, http.StatusUnprocessableEntity, "diskon khusus hanya untuk pendaftaran yang aktif")
		return
	}
	var id string
	if err = tx.QueryRowContext(r.Context(),
		`INSERT INTO manual_discounts(school_id,application_id,kind,value,cap_idr,priority,reason,created_by,created_by_name,status,stage)
		 VALUES($1,$2,$3,$4,NULLIF($5,0),100,$6,$7,$8,'pending',$9) RETURNING id`,
		schoolID, applicationID, kind, value, in.Cap, in.Reason, claims.UserID, claims.Name, in.Stage).Scan(&id); err != nil {
		writeErr(w, err)
		return
	}
	summary := map[string]any{
		"application_id": applicationID, "registration_number": registrationNumber, "applicant": applicantName,
		"type": ruleKindToAPI(string(kind)), "value": in.Value, "cap": in.Cap, "stage": stageLabel(in.Stage), "reason": in.Reason,
	}
	var amount *int64
	if kind == model.DiscountFixed {
		amount = &value
	}
	approvalID, err := a.openApproval(r.Context(), client.ApprovalRequest{
		SchoolID: schoolID, RequestType: client.ReqTypeDiscountSpecial, SourceID: id,
		Title:   "Diskon khusus PPDB " + registrationNumber + " (" + applicantName + ")",
		Summary: summary, AmountIDR: amount, MakerUserID: claims.UserID,
	})
	if err != nil {
		writeErr(w, err)
		return
	}
	if _, err = tx.ExecContext(r.Context(), `UPDATE manual_discounts SET approval_id=$2 WHERE id=$1`, id, approvalID); err != nil {
		writeErr(w, err)
		return
	}
	if _, err = tx.ExecContext(r.Context(),
		`INSERT INTO audit_events(school_id,aggregate_type,aggregate_id,action,actor_type,actor_id,actor_name,after_data,reason)
		 VALUES($1,'application',$2,'special_discount_requested','admin',$3,$4,$5,$6)`,
		schoolID, applicationID, claims.UserID, claims.Name, jsonRaw(map[string]any{"discount_id": id, "approval_id": approvalID, "summary": summary}), in.Reason); err != nil {
		writeErr(w, err)
		return
	}
	if err = tx.Commit(); err != nil {
		writeErr(w, err)
		return
	}
	common.WriteJSON(w, http.StatusCreated, map[string]any{"id": id, "status": "pending", "approval_id": approvalID}, nil)
}

// ListSpecialDiscounts is GET /admin/applications/{id}/special-discounts.
func (a *App) ListSpecialDiscounts(w http.ResponseWriter, r *http.Request) {
	if !common.RequireAdmin(w, r) {
		return
	}
	out, err := a.specialDiscounts(r.Context(), common.EffectiveSchoolID(r), param(r, "id"))
	if err != nil {
		writeErr(w, err)
		return
	}
	common.WriteJSON(w, http.StatusOK, out, nil)
}

func (a *App) specialDiscounts(ctx context.Context, schoolID, applicationID string) ([]map[string]any, error) {
	rows, err := a.DB.QueryContext(ctx,
		`SELECT id,kind,value,COALESCE(cap_idr,0),reason,stage,status,COALESCE(created_by_name,''),created_at,
		        COALESCE(decided_by_name,decided_by,''),decided_at,COALESCE(decided_note,''),COALESCE(approval_id::text,'')
		 FROM manual_discounts WHERE school_id=$1 AND application_id=$2 ORDER BY created_at DESC`, schoolID, applicationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var (
			id, kind, reason, stage, status, createdBy, decidedBy, note, approvalID string
			value, capIDR                                                           int64
			createdAt                                                               time.Time
			decidedAt                                                               sql.NullTime
		)
		if err := rows.Scan(&id, &kind, &value, &capIDR, &reason, &stage, &status, &createdBy, &createdAt,
			&decidedBy, &decidedAt, &note, &approvalID); err != nil {
			return nil, err
		}
		apiValue := value
		if kind == string(model.DiscountPercent) {
			apiValue = value / 100
		}
		item := map[string]any{"id": id, "type": ruleKindToAPI(kind), "value": apiValue, "cap": capIDR, "reason": reason,
			"stage": stage, "stage_label": stageLabel(stage), "status": status, "created_by_name": createdBy, "created_at": createdAt,
			"approval_id": approvalID}
		if decidedAt.Valid {
			item["decided_at"] = decidedAt.Time
			item["decided_by"] = decidedBy
			item["decided_note"] = note
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

// ---------------------------------------------------------------------------
// Selection results — BUG-034/036
// ---------------------------------------------------------------------------

// SubmitSelectionApproval is POST /admin/selection/approvals {wave_id}. Every
// ranked application in the wave needs a draft decision; the whole set goes
// to the Selection Committee as one selection_result approval.
func (a *App) SubmitSelectionApproval(w http.ResponseWriter, r *http.Request) {
	if !common.RequireAdmin(w, r) {
		return
	}
	schoolID := common.EffectiveSchoolID(r)
	claims := common.Admin(r)
	var in struct {
		WaveID string `json:"wave_id"`
	}
	if !common.Decode(w, r, &in) {
		return
	}
	if in.WaveID == "" {
		common.WriteError(w, http.StatusUnprocessableEntity, "wave_id wajib diisi")
		return
	}
	tx, err := a.DB.BeginTx(r.Context(), nil)
	if err != nil {
		writeErr(w, err)
		return
	}
	defer func() { _ = tx.Rollback() }()

	var periodID, waveName string
	err = tx.QueryRowContext(r.Context(),
		`SELECT period_id,name FROM admission_waves WHERE id=$1 AND school_id=$2 FOR UPDATE`, in.WaveID, schoolID).Scan(&periodID, &waveName)
	if err == sql.ErrNoRows {
		common.WriteError(w, http.StatusNotFound, "gelombang tidak ditemukan")
		return
	}
	if err != nil {
		writeErr(w, err)
		return
	}
	var ranked, undecided int
	counts := map[string]int{}
	rows, err := tx.QueryContext(r.Context(),
		`SELECT a.id,COALESCE(d.decision,'')
		 FROM applications a
		 LEFT JOIN LATERAL (SELECT decision FROM decisions d WHERE d.application_id=a.id AND d.status='draft'
		                    ORDER BY version DESC LIMIT 1) d ON true
		 WHERE a.school_id=$1 AND a.wave_id=$2 AND a.status='ranked'`, schoolID, in.WaveID)
	if err != nil {
		writeErr(w, err)
		return
	}
	for rows.Next() {
		var id, decision string
		if err := rows.Scan(&id, &decision); err != nil {
			rows.Close()
			writeErr(w, err)
			return
		}
		ranked++
		if decision == "" {
			undecided++
		} else {
			counts[decision]++
		}
	}
	rows.Close()
	if ranked == 0 {
		common.WriteError(w, http.StatusUnprocessableEntity, "belum ada hasil seleksi yang siap diajukan pada gelombang ini")
		return
	}
	if undecided > 0 {
		common.WriteError(w, http.StatusUnprocessableEntity, "masih ada "+itoa(undecided)+" pendaftar tanpa keputusan seleksi")
		return
	}
	var id string
	err = tx.QueryRowContext(r.Context(),
		`INSERT INTO selection_approvals(school_id,period_id,wave_id,decision_count,created_by,created_by_name)
		 VALUES($1,$2,$3,$4,$5,$6) RETURNING id`,
		schoolID, periodID, in.WaveID, ranked, claims.UserID, claims.Name).Scan(&id)
	if isUniqueViolation(err) {
		common.WriteError(w, http.StatusConflict, "hasil seleksi gelombang ini sedang menunggu approval")
		return
	}
	if err != nil {
		writeErr(w, err)
		return
	}
	if _, err = tx.ExecContext(r.Context(),
		`UPDATE decisions d SET status='pending_approval',selection_approval_id=$1
		 FROM applications a WHERE a.id=d.application_id AND a.school_id=$2 AND a.wave_id=$3 AND a.status='ranked' AND d.status='draft'`,
		id, schoolID, in.WaveID); err != nil {
		writeErr(w, err)
		return
	}
	summary := map[string]any{"wave": waveName, "total": ranked, "offered": counts["offered"],
		"waitlisted": counts["waitlisted"], "rejected": counts["rejected"]}
	approvalID, err := a.openApproval(r.Context(), client.ApprovalRequest{
		SchoolID: schoolID, RequestType: client.ReqTypeSelectionResult, SourceID: id,
		Title: "Hasil seleksi PPDB " + waveName, Summary: summary, MakerUserID: claims.UserID,
	})
	if err != nil {
		writeErr(w, err)
		return
	}
	if _, err = tx.ExecContext(r.Context(), `UPDATE selection_approvals SET approval_id=$2 WHERE id=$1`, id, approvalID); err != nil {
		writeErr(w, err)
		return
	}
	if _, err = tx.ExecContext(r.Context(),
		`INSERT INTO audit_events(school_id,aggregate_type,aggregate_id,action,actor_type,actor_id,actor_name,after_data)
		 VALUES($1,'selection_approval',$2,'selection_result_submitted','admin',$3,$4,$5)`,
		schoolID, id, claims.UserID, claims.Name, jsonRaw(summary)); err != nil {
		writeErr(w, err)
		return
	}
	if err = tx.Commit(); err != nil {
		writeErr(w, err)
		return
	}
	common.WriteJSON(w, http.StatusCreated, map[string]any{"id": id, "status": "pending", "approval_id": approvalID, "summary": summary}, nil)
}

// ListSelectionApprovals is GET /admin/selection/approvals?wave_id=.
func (a *App) ListSelectionApprovals(w http.ResponseWriter, r *http.Request) {
	if !common.RequireAdmin(w, r) {
		return
	}
	schoolID := common.EffectiveSchoolID(r)
	rows, err := a.DB.QueryContext(r.Context(),
		`SELECT s.id,s.wave_id,w.name,s.status,s.decision_count,s.created_by_name,s.created_at,COALESCE(s.decided_comment,''),s.decided_at,
		        EXISTS(SELECT 1 FROM ppdb_announcements pa WHERE pa.selection_approval_id=s.id AND pa.status<>'cancelled')
		 FROM selection_approvals s JOIN admission_waves w ON w.id=s.wave_id
		 WHERE s.school_id=$1 AND ($2='' OR s.wave_id::text=$2) ORDER BY s.created_at DESC LIMIT 100`,
		schoolID, r.URL.Query().Get("wave_id"))
	if err != nil {
		writeErr(w, err)
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var (
			id, waveID, waveName, status, createdBy, comment string
			count                                            int
			createdAt                                        time.Time
			decidedAt                                        sql.NullTime
			announced                                        bool
		)
		if err := rows.Scan(&id, &waveID, &waveName, &status, &count, &createdBy, &createdAt, &comment, &decidedAt, &announced); err != nil {
			writeErr(w, err)
			return
		}
		item := map[string]any{"id": id, "wave_id": waveID, "wave_name": waveName, "status": status, "decision_count": count,
			"created_by_name": createdBy, "created_at": createdAt, "announced": announced}
		if decidedAt.Valid {
			item["decided_at"] = decidedAt.Time
			item["decided_comment"] = comment
		}
		out = append(out, item)
	}
	common.WriteJSON(w, http.StatusOK, out, nil)
}

// ---------------------------------------------------------------------------
// Waitlist promotion — BUG-035, BR-PPDB-09
// ---------------------------------------------------------------------------

// vacantSeats counts the free seats of a track: quota minus applicants that
// hold or are converting an offer.
func vacantSeats(ctx context.Context, q queryer, schoolID, trackID string) (int, error) {
	var quota, holding int
	err := q.QueryRowContext(ctx,
		`SELECT t.quota,(SELECT count(*) FROM applications a WHERE a.track_id=t.id AND a.status IN('offered','reregistration','converted'))
		 FROM admission_tracks t WHERE t.id=$1 AND t.school_id=$2`, trackID, schoolID).Scan(&quota, &holding)
	if err != nil {
		return 0, err
	}
	return model.VacantSeats(quota, holding), nil
}

// waitlistCandidates returns the waitlisted applicants of a track in rank
// order (latest ranking run).
func waitlistCandidates(ctx context.Context, q queryer, schoolID, trackID string) ([]model.WaitlistCandidate, error) {
	rows, err := q.QueryContext(ctx,
		`SELECT a.id,COALESCE(re.rank,2147483647),COALESCE(re.total_score,0)
		 FROM applications a
		 LEFT JOIN LATERAL (SELECT e.rank,e.total_score FROM ranking_entries e JOIN ranking_runs rr ON rr.id=e.ranking_run_id
		                    WHERE e.application_id=a.id ORDER BY rr.version DESC LIMIT 1) re ON true
		 WHERE a.school_id=$1 AND a.track_id=$2 AND a.status='waitlisted'
		   AND NOT EXISTS (SELECT 1 FROM waitlist_promotions wp WHERE wp.application_id=a.id AND wp.status='pending')`,
		schoolID, trackID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.WaitlistCandidate
	for rows.Next() {
		var c model.WaitlistCandidate
		if err := rows.Scan(&c.ApplicationID, &c.Rank, &c.Total); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// ListWaitlist is GET /admin/selection/waitlist?track_id=.
func (a *App) ListWaitlist(w http.ResponseWriter, r *http.Request) {
	if !common.RequireAdmin(w, r) {
		return
	}
	schoolID := common.EffectiveSchoolID(r)
	trackID := r.URL.Query().Get("track_id")
	if trackID == "" {
		common.WriteError(w, http.StatusUnprocessableEntity, "track_id wajib diisi")
		return
	}
	seats, err := vacantSeats(r.Context(), a.DB, schoolID, trackID)
	if err == sql.ErrNoRows {
		common.WriteError(w, http.StatusNotFound, "jalur tidak ditemukan")
		return
	}
	if err != nil {
		writeErr(w, err)
		return
	}
	cands, err := waitlistCandidates(r.Context(), a.DB, schoolID, trackID)
	if err != nil {
		writeErr(w, err)
		return
	}
	ordered := model.PickWaitlist(cands, nil, len(cands))
	items := []map[string]any{}
	for _, c := range ordered {
		var name, number string
		_ = a.DB.QueryRowContext(r.Context(),
			`SELECT ap.full_name,a.registration_number FROM applications a JOIN applicants ap ON ap.id=a.applicant_id WHERE a.id=$1`,
			c.ApplicationID).Scan(&name, &number)
		items = append(items, map[string]any{"application_id": c.ApplicationID, "rank": c.Rank, "total_score": c.Total,
			"applicant_name": name, "registration_number": number})
	}
	promotions := []map[string]any{}
	rows, err := a.DB.QueryContext(r.Context(),
		`SELECT wp.id,wp.application_id,ap.full_name,wp.status,wp.created_by_name,wp.created_at,COALESCE(wp.decided_comment,'')
		 FROM waitlist_promotions wp JOIN applications a ON a.id=wp.application_id JOIN applicants ap ON ap.id=a.applicant_id
		 WHERE wp.school_id=$1 AND wp.track_id=$2 ORDER BY wp.created_at DESC LIMIT 50`, schoolID, trackID)
	if err != nil {
		writeErr(w, err)
		return
	}
	defer rows.Close()
	for rows.Next() {
		var id, appID, name, status, by, comment string
		var at time.Time
		if err := rows.Scan(&id, &appID, &name, &status, &by, &at, &comment); err != nil {
			writeErr(w, err)
			return
		}
		promotions = append(promotions, map[string]any{"id": id, "application_id": appID, "applicant_name": name,
			"status": status, "created_by_name": by, "created_at": at, "decided_comment": comment})
	}
	common.WriteJSON(w, http.StatusOK, map[string]any{"vacant_seats": seats, "candidates": items, "promotions": promotions}, nil)
}

// PromoteWaitlist is POST /admin/selection/waitlist/promote {track_id,
// application_id?}. Without application_id the best-ranked candidate is
// proposed. Requires a vacant seat; the promotion itself is approved by the
// Selection Committee in the Approval Center.
func (a *App) PromoteWaitlist(w http.ResponseWriter, r *http.Request) {
	if !common.RequireAdmin(w, r) {
		return
	}
	schoolID := common.EffectiveSchoolID(r)
	claims := common.Admin(r)
	var in struct {
		TrackID       string `json:"track_id"`
		ApplicationID string `json:"application_id"`
	}
	if !common.Decode(w, r, &in) {
		return
	}
	if in.TrackID == "" {
		common.WriteError(w, http.StatusUnprocessableEntity, "track_id wajib diisi")
		return
	}
	tx, err := a.DB.BeginTx(r.Context(), nil)
	if err != nil {
		writeErr(w, err)
		return
	}
	defer func() { _ = tx.Rollback() }()
	if _, err = tx.ExecContext(r.Context(), `SELECT 1 FROM admission_tracks WHERE id=$1 AND school_id=$2 FOR UPDATE`, in.TrackID, schoolID); err != nil {
		writeErr(w, err)
		return
	}
	seats, err := vacantSeats(r.Context(), tx, schoolID, in.TrackID)
	if err == sql.ErrNoRows {
		common.WriteError(w, http.StatusNotFound, "jalur tidak ditemukan")
		return
	}
	if err != nil {
		writeErr(w, err)
		return
	}
	var pending int
	if err = tx.QueryRowContext(r.Context(),
		`SELECT count(*) FROM waitlist_promotions WHERE track_id=$1 AND status='pending'`, in.TrackID).Scan(&pending); err != nil {
		writeErr(w, err)
		return
	}
	if seats-pending <= 0 {
		common.WriteError(w, http.StatusUnprocessableEntity, "tidak ada kursi kosong pada jalur ini")
		return
	}
	cands, err := waitlistCandidates(r.Context(), tx, schoolID, in.TrackID)
	if err != nil {
		writeErr(w, err)
		return
	}
	var pick *model.WaitlistCandidate
	if in.ApplicationID == "" {
		if best := model.PickWaitlist(cands, nil, 1); len(best) == 1 {
			pick = &best[0]
		}
	} else {
		for i := range cands {
			if cands[i].ApplicationID == in.ApplicationID {
				pick = &cands[i]
			}
		}
	}
	if pick == nil {
		common.WriteError(w, http.StatusUnprocessableEntity, "tidak ada pendaftar cadangan yang dapat dinaikkan")
		return
	}
	var periodID, waveID, name, number string
	if err = tx.QueryRowContext(r.Context(),
		`SELECT a.period_id,a.wave_id,ap.full_name,a.registration_number FROM applications a JOIN applicants ap ON ap.id=a.applicant_id
		 WHERE a.id=$1`, pick.ApplicationID).Scan(&periodID, &waveID, &name, &number); err != nil {
		writeErr(w, err)
		return
	}
	var id string
	if err = tx.QueryRowContext(r.Context(),
		`INSERT INTO waitlist_promotions(school_id,application_id,period_id,wave_id,track_id,total_score,created_by,created_by_name)
		 VALUES($1,$2,$3,$4,$5,$6,$7,$8) RETURNING id`,
		schoolID, pick.ApplicationID, periodID, waveID, in.TrackID, pick.Total, claims.UserID, claims.Name).Scan(&id); err != nil {
		writeErr(w, err)
		return
	}
	approvalID, err := a.openApproval(r.Context(), client.ApprovalRequest{
		SchoolID: schoolID, RequestType: client.ReqTypeSelectionResult, SourceID: id,
		Title:       "Naik cadangan PPDB " + number + " (" + name + ")",
		Summary:     map[string]any{"application_id": pick.ApplicationID, "rank": pick.Rank, "total_score": pick.Total, "vacant_seats": seats, "kind": "waitlist_promotion"},
		MakerUserID: claims.UserID,
	})
	if err != nil {
		writeErr(w, err)
		return
	}
	if _, err = tx.ExecContext(r.Context(), `UPDATE waitlist_promotions SET approval_id=$2 WHERE id=$1`, id, approvalID); err != nil {
		writeErr(w, err)
		return
	}
	if _, err = tx.ExecContext(r.Context(),
		`INSERT INTO audit_events(school_id,aggregate_type,aggregate_id,action,actor_type,actor_id,actor_name,after_data)
		 VALUES($1,'application',$2,'waitlist_promotion_requested','admin',$3,$4,$5)`,
		schoolID, pick.ApplicationID, claims.UserID, claims.Name, jsonRaw(map[string]any{"promotion_id": id, "approval_id": approvalID})); err != nil {
		writeErr(w, err)
		return
	}
	if err = tx.Commit(); err != nil {
		writeErr(w, err)
		return
	}
	common.WriteJSON(w, http.StatusCreated, map[string]any{"id": id, "application_id": pick.ApplicationID, "status": "pending", "approval_id": approvalID}, nil)
}

// ---------------------------------------------------------------------------
// Core callback
// ---------------------------------------------------------------------------

type approvalCallback struct {
	ApprovalID  string    `json:"approval_id"`
	SourceID    string    `json:"source_id"`
	RequestType string    `json:"request_type"`
	Decision    string    `json:"decision"`
	DecidedBy   string    `json:"decided_by"`
	Comment     string    `json:"comment"`
	DecidedAt   time.Time `json:"decided_at"`
}

// ApprovalCallback is POST /internal/approval-callbacks (X-Internal-Key).
// Processing and the dedupe row commit together, so a failed apply is
// retried by core and a repeated delivery is a no-op.
func (a *App) ApprovalCallback(w http.ResponseWriter, r *http.Request) {
	var in approvalCallback
	if !common.Decode(w, r, &in) {
		return
	}
	switch in.Decision {
	case "approved", "rejected", "cancelled":
	default:
		common.WriteError(w, http.StatusUnprocessableEntity, "decision tidak dikenal")
		return
	}
	if in.ApprovalID == "" || in.SourceID == "" {
		common.WriteError(w, http.StatusUnprocessableEntity, "approval_id dan source_id wajib diisi")
		return
	}
	result, err := a.applyApprovalCallback(r.Context(), in, time.Now())
	if err != nil {
		writeErr(w, err)
		return
	}
	common.WriteJSON(w, http.StatusOK, result, nil)
}

func (a *App) applyApprovalCallback(ctx context.Context, in approvalCallback, now time.Time) (map[string]any, error) {
	tx, err := a.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	var (
		schoolID string
		kind     string
	)
	err = tx.QueryRowContext(ctx,
		`SELECT school_id::text,'discount' FROM manual_discounts WHERE id::text=$1 AND approval_id::text=$2
		 UNION ALL SELECT school_id::text,'selection' FROM selection_approvals WHERE id::text=$1 AND approval_id::text=$2
		 UNION ALL SELECT school_id::text,'waitlist' FROM waitlist_promotions WHERE id::text=$1 AND approval_id::text=$2`,
		in.SourceID, in.ApprovalID).Scan(&schoolID, &kind)
	if err == sql.ErrNoRows {
		return nil, &httpError{http.StatusNotFound, "sumber approval tidak ditemukan"}
	}
	if err != nil {
		return nil, err
	}
	res, err := tx.ExecContext(ctx,
		`INSERT INTO approval_callback_events(approval_id,school_id,request_type,decision) VALUES($1,$2,$3,$4)
		 ON CONFLICT (approval_id) DO NOTHING`, in.ApprovalID, schoolID, in.RequestType, in.Decision)
	if err != nil {
		return nil, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return map[string]any{"approval_id": in.ApprovalID, "duplicate": true}, nil
	}
	switch kind {
	case "discount":
		err = a.applyDiscountDecision(ctx, tx, schoolID, in)
	case "selection":
		err = a.applySelectionDecision(ctx, tx, schoolID, in)
	case "waitlist":
		err = a.applyWaitlistDecision(ctx, tx, schoolID, in, now)
	}
	if err != nil {
		return nil, err
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return map[string]any{"approval_id": in.ApprovalID, "applied": kind, "decision": in.Decision}, nil
}

func (a *App) applyDiscountDecision(ctx context.Context, tx *sql.Tx, schoolID string, in approvalCallback) error {
	var applicationID, stage, status string
	if err := tx.QueryRowContext(ctx,
		`SELECT application_id,stage,status FROM manual_discounts WHERE id=$1 FOR UPDATE`, in.SourceID).
		Scan(&applicationID, &stage, &status); err != nil {
		return err
	}
	if status != "pending" {
		return nil
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE manual_discounts SET status=$2,decided_by=$3,decided_by_name=$3,decided_at=$4,decided_note=NULLIF($5,'') WHERE id=$1`,
		in.SourceID, in.Decision, in.DecidedBy, in.DecidedAt, in.Comment); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO audit_events(school_id,aggregate_type,aggregate_id,action,actor_type,actor_id,actor_name,after_data,reason)
		 VALUES($1,'application',$2,'special_discount_'||$3,'approval',$4,'Approval Center',$5,NULLIF($6,''))`,
		schoolID, applicationID, in.Decision, in.DecidedBy, jsonRaw(map[string]any{"discount_id": in.SourceID, "approval_id": in.ApprovalID}), in.Comment); err != nil {
		return err
	}
	if in.Decision != "approved" {
		return nil
	}
	// An already issued, still active stage bill is rebuilt with the
	// approved discount; otherwise it applies when the bill is issued.
	charge, err := latestCharge(ctx, tx, schoolID, applicationID, stage)
	if err != nil || charge == nil || charge.Cancelled.Valid {
		return err
	}
	if _, err := a.issueChargeTx(ctx, tx, schoolID, applicationID, stage, issueOptions{Rebuild: true}); err != nil {
		return err
	}
	return a.notify(ctx, tx, schoolID, applicationID, "special_discount_approved", "Tagihan diperbarui",
		"Diskon khusus untuk tagihan "+stageLabel(stage)+" telah disetujui. Silakan cek tagihan terbaru di portal PPDB.",
		"special_discount:"+in.SourceID, nil)
}

func (a *App) applySelectionDecision(ctx context.Context, tx *sql.Tx, schoolID string, in approvalCallback) error {
	var status string
	if err := tx.QueryRowContext(ctx, `SELECT status FROM selection_approvals WHERE id=$1 FOR UPDATE`, in.SourceID).Scan(&status); err != nil {
		return err
	}
	if status != "pending" {
		return nil
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE selection_approvals SET status=$2,decided_by=$3,decided_comment=NULLIF($4,''),decided_at=$5 WHERE id=$1`,
		in.SourceID, in.Decision, in.DecidedBy, in.Comment, in.DecidedAt); err != nil {
		return err
	}
	if in.Decision == "approved" {
		if _, err := tx.ExecContext(ctx,
			`UPDATE decisions SET status='approved' WHERE selection_approval_id=$1 AND status='pending_approval'`, in.SourceID); err != nil {
			return err
		}
	} else {
		// Rejected/cancelled: decisions return to draft for revision.
		if _, err := tx.ExecContext(ctx,
			`UPDATE decisions SET status='draft',selection_approval_id=NULL WHERE selection_approval_id=$1 AND status='pending_approval'`, in.SourceID); err != nil {
			return err
		}
	}
	_, err := tx.ExecContext(ctx,
		`INSERT INTO audit_events(school_id,aggregate_type,aggregate_id,action,actor_type,actor_id,actor_name,reason)
		 VALUES($1,'selection_approval',$2,'selection_result_'||$3,'approval',$4,'Approval Center',NULLIF($5,''))`,
		schoolID, in.SourceID, in.Decision, in.DecidedBy, in.Comment)
	return err
}

func (a *App) applyWaitlistDecision(ctx context.Context, tx *sql.Tx, schoolID string, in approvalCallback, now time.Time) error {
	var applicationID, trackID, status string
	if err := tx.QueryRowContext(ctx,
		`SELECT application_id,track_id,status FROM waitlist_promotions WHERE id=$1 FOR UPDATE`, in.SourceID).
		Scan(&applicationID, &trackID, &status); err != nil {
		return err
	}
	if status != "pending" {
		return nil
	}
	final := in.Decision
	if final == "cancelled" {
		final = "rejected"
	}
	if in.Decision == "approved" {
		if _, err := tx.ExecContext(ctx, `SELECT 1 FROM admission_tracks WHERE id=$1 FOR UPDATE`, trackID); err != nil {
			return err
		}
		seats, err := vacantSeats(ctx, tx, schoolID, trackID)
		if err != nil {
			return err
		}
		var current string
		if err := tx.QueryRowContext(ctx, `SELECT status FROM applications WHERE id=$1`, applicationID).Scan(&current); err != nil {
			return err
		}
		if seats <= 0 || current != string(model.StatusWaitlisted) {
			final = "failed"
		} else if err := a.offerSeat(ctx, tx, schoolID, applicationID, "waitlist", now, "Naik dari daftar cadangan"); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE waitlist_promotions SET status=$2,decided_by=$3,decided_comment=NULLIF($4,''),decided_at=$5 WHERE id=$1`,
		in.SourceID, final, in.DecidedBy, in.Comment, in.DecidedAt); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx,
		`INSERT INTO audit_events(school_id,aggregate_type,aggregate_id,action,actor_type,actor_id,actor_name,after_data,reason)
		 VALUES($1,'application',$2,'waitlist_promotion_'||$3,'approval',$4,'Approval Center',$5,NULLIF($6,''))`,
		schoolID, applicationID, final, in.DecidedBy, jsonRaw(map[string]any{"promotion_id": in.SourceID}), in.Comment)
	return err
}

// offerSeat turns an application into an offer: status offered, a published
// decision, the re-registration deadline, the re-registration bill and the
// guardian notification. Used by announcement publication and waitlist
// promotion (BUG-035/038/039).
func (a *App) offerSeat(ctx context.Context, tx *sql.Tx, schoolID, applicationID, source string, now time.Time, reason string) error {
	if _, err := a.transitionTx(ctx, tx, schoolID, applicationID, "system", "admission", "Sistem PPDB",
		model.StatusOffered, reason, 0, ""); err != nil {
		return err
	}
	var (
		reregEnd     sql.NullTime
		waitlistDays int
	)
	if err := tx.QueryRowContext(ctx,
		`SELECT w.reregistration_end,w.waitlist_reregistration_days FROM applications a JOIN admission_waves w ON w.id=a.wave_id WHERE a.id=$1`,
		applicationID).Scan(&reregEnd, &waitlistDays); err != nil {
		return err
	}
	due := model.ReregistrationDeadline(now, nullTimePtr(reregEnd), waitlistDays, source == "waitlist")
	if _, err := tx.ExecContext(ctx, `UPDATE applications SET reregistration_due_at=$2 WHERE id=$1`, applicationID, due); err != nil {
		return err
	}
	if source == "waitlist" {
		var version int
		if err := tx.QueryRowContext(ctx, `SELECT COALESCE(max(version),0)+1 FROM decisions WHERE application_id=$1`, applicationID).Scan(&version); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO decisions(school_id,application_id,decision,override,reason,actor_id,version,status,published_at)
			 VALUES($1,$2,'offered',true,$3,'approval',$4,'published',$5)`,
			schoolID, applicationID, reason, version, now); err != nil {
			return err
		}
	}
	chargeID, err := a.issueChargeTx(ctx, tx, schoolID, applicationID, stageReregistration, issueOptions{})
	if err != nil {
		return err
	}
	var net, minimum int64
	if err := tx.QueryRowContext(ctx, `SELECT net_idr,min_payment_idr FROM charges WHERE id=$1`, chargeID).Scan(&net, &minimum); err != nil {
		return err
	}
	return a.notify(ctx, tx, schoolID, applicationID, "offered", "Selamat, ananda diterima",
		"Ananda dinyatakan diterima. Tagihan daftar ulang sebesar "+formatIDR(net)+" telah terbit; pembayaran minimum "+
			formatIDR(minimum)+" paling lambat "+formatDateWIB(due)+". Kursi akan dilepas bila minimum tidak dibayar sampai batas waktu.",
		"offered:"+applicationID, map[string]any{"charge_id": chargeID})
}
