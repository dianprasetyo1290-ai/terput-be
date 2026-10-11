package handler

import (
	"database/sql"
	"net/http"
	"time"

	"golang.org/x/crypto/bcrypt"

	"skci/admission/internal/common"
	"skci/admission/internal/otp"
)

type otpRequestBody struct {
	Contact string `json:"contact"`
	Channel string `json:"channel"`
	Name    string `json:"name"`
}

// RequestOTP is POST /public/otp/request: start a guardian login challenge.
func (a *App) RequestOTP(w http.ResponseWriter, r *http.Request) {
	var req otpRequestBody
	if !common.Decode(w, r, &req) {
		return
	}
	a.issueOTP(w, r, req)
}

// ResendOTP is POST /public/otp/resend: identical semantics to request, but the
// cooldown check makes it a real resend rather than an unbounded send.
func (a *App) ResendOTP(w http.ResponseWriter, r *http.Request) {
	var req otpRequestBody
	if !common.Decode(w, r, &req) {
		return
	}
	a.issueOTP(w, r, req)
}

func (a *App) issueOTP(w http.ResponseWriter, r *http.Request, req otpRequestBody) {
	schoolID, err := a.resolveSchoolID(r)
	if err != nil || schoolID == "" {
		common.WriteError(w, http.StatusBadRequest, "school_id is required")
		return
	}
	kind := channelKind(req.Channel, req.Contact)
	contact, ok := normalizeContact(kind, req.Contact)
	if !ok {
		common.WriteError(w, http.StatusBadRequest, "a valid email or phone contact is required")
		return
	}
	const purpose = "guardian_login"

	var resendAfter time.Time
	err = a.DB.QueryRowContext(r.Context(),
		`SELECT resend_after FROM otp_challenges
		 WHERE school_id=$1 AND contact_kind=$2 AND contact_value=$3 AND purpose=$4 AND used_at IS NULL
		 ORDER BY created_at DESC LIMIT 1`, schoolID, kind, contact, purpose).Scan(&resendAfter)
	if err != nil && err != sql.ErrNoRows {
		common.WriteError(w, http.StatusInternalServerError, "failed to request OTP")
		return
	}
	if err == nil && !time.Now().After(resendAfter) {
		common.WriteError(w, http.StatusTooManyRequests, "OTP resend cooldown is active")
		return
	}

	code, err := a.OTP.Generate()
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "failed to generate OTP")
		return
	}
	expires := time.Now().Add(a.Config.OTPTTL)
	resend := time.Now().Add(a.Config.OTPCooldown)

	var challengeID string
	err = a.DB.QueryRowContext(r.Context(),
		`INSERT INTO otp_challenges(school_id,contact_kind,contact_value,purpose,code_hash,max_attempts,expires_at,resend_after,request_ip_hash)
		 VALUES($1,$2,$3,$4,'pending',$5,$6,$7,$8) RETURNING id`,
		schoolID, kind, contact, purpose, a.Config.OTPMaxAttempts, expires, resend, ipHash(r)).Scan(&challengeID)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "failed to request OTP")
		return
	}
	if _, err = a.DB.ExecContext(r.Context(), `UPDATE otp_challenges SET code_hash=$1 WHERE id=$2`, a.OTP.Hash(challengeID, code), challengeID); err != nil {
		common.WriteError(w, http.StatusInternalServerError, "failed to request OTP")
		return
	}
	if err := a.Provider.Send(r.Context(), contact, code); err != nil {
		common.WriteError(w, http.StatusServiceUnavailable, "OTP delivery unavailable")
		return
	}
	resp := map[string]any{
		"challenge_id": challengeID,
		"expires_at":   expires,
		"resend_after": resend,
	}
	if a.Config.OTPProvider == "dev" {
		resp["dev_code"] = code
	}
	common.WriteJSON(w, http.StatusAccepted, resp, nil)
}

type verifyOTPBody struct {
	Contact string `json:"contact"`
	Code    string `json:"code"`
	Name    string `json:"name"`
}

// VerifyOTP is POST /public/otp/verify: validate the code, upsert the guardian,
// and issue a guardian session token.
func (a *App) VerifyOTP(w http.ResponseWriter, r *http.Request) {
	var req verifyOTPBody
	if !common.Decode(w, r, &req) {
		return
	}
	schoolID, err := a.resolveSchoolID(r)
	if err != nil || schoolID == "" {
		common.WriteError(w, http.StatusBadRequest, "school_id is required")
		return
	}
	kind := channelKind("", req.Contact)
	contact, ok := normalizeContact(kind, req.Contact)
	if !ok {
		common.WriteError(w, http.StatusBadRequest, "a valid email or phone contact is required")
		return
	}

	tx, err := a.DB.BeginTx(r.Context(), nil)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "verification failed")
		return
	}
	defer func() { _ = tx.Rollback() }()

	var (
		challengeID, hash string
		expires, resend   time.Time
		attempts, max     int
		used              sql.NullTime
	)
	err = tx.QueryRowContext(r.Context(),
		`SELECT id,code_hash,expires_at,resend_after,attempts,max_attempts,used_at
		 FROM otp_challenges
		 WHERE school_id=$1 AND contact_kind=$2 AND contact_value=$3 AND purpose='guardian_login' AND used_at IS NULL
		 ORDER BY created_at DESC LIMIT 1 FOR UPDATE`, schoolID, kind, contact).Scan(
		&challengeID, &hash, &expires, &resend, &attempts, &max, &used)
	if err != nil {
		common.WriteError(w, http.StatusUnauthorized, "no active OTP challenge for this contact")
		return
	}

	challenge := &otp.Challenge{Hash: hash, ExpiresAt: expires, ResendAfter: resend, Attempts: attempts, MaxAttempts: max}
	if used.Valid {
		challenge.UsedAt = &used.Time
	}
	verifyErr := a.OTP.Verify(challenge, challengeID, req.Code)
	if _, err = tx.ExecContext(r.Context(),
		`UPDATE otp_challenges SET attempts=$1,used_at=$2 WHERE id=$3`,
		challenge.Attempts, challenge.UsedAt, challengeID); err != nil {
		common.WriteError(w, http.StatusInternalServerError, "verification failed")
		return
	}
	if verifyErr != nil {
		if commitErr := tx.Commit(); commitErr != nil {
			common.WriteError(w, http.StatusInternalServerError, "verification failed")
			return
		}
		common.WriteError(w, http.StatusUnauthorized, verifyErr.Error())
		return
	}

	var guardianID, guardianName string
	err = tx.QueryRowContext(r.Context(),
		`SELECT g.id,g.name FROM guardian_contacts c JOIN guardians g ON g.id=c.guardian_id
		 WHERE c.school_id=$1 AND c.kind=$2 AND c.normalized_value=$3`, schoolID, kind, contact).
		Scan(&guardianID, &guardianName)
	if err == sql.ErrNoRows {
		guardianName = req.Name
		if guardianName == "" {
			guardianName = "Wali Calon Siswa"
		}
		if err = tx.QueryRowContext(r.Context(),
			`INSERT INTO guardians(school_id,name) VALUES($1,$2) RETURNING id`, schoolID, guardianName).Scan(&guardianID); err == nil {
			_, err = tx.ExecContext(r.Context(),
				`INSERT INTO guardian_contacts(school_id,guardian_id,kind,normalized_value,verified_at) VALUES($1,$2,$3,$4,now())`,
				schoolID, guardianID, kind, contact)
		}
	} else if req.Name != "" && (guardianName == "" || guardianName == "Wali" || guardianName == "Wali Calon Siswa") {
		guardianName = req.Name
		_, _ = tx.ExecContext(r.Context(), `UPDATE guardians SET name=$1, updated_at=now() WHERE id=$2`, guardianName, guardianID)
	}
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "verification failed")
		return
	}
	if err = tx.Commit(); err != nil {
		common.WriteError(w, http.StatusInternalServerError, "verification failed")
		return
	}

	// Auto-provision & persist guardian account into Identity Service per PRD v2.1 FR-16-001 & FR-16-006
	if a.Identity != nil && guardianID != "" {
		email := contact
		phone := ""
		if kind == "phone" {
			phone = contact
			email = "wali." + guardianID[:8] + "@ppdb.terataiputih.sch.id"
		}
		var identityOut struct {
			ID string `json:"id"`
		}
		_ = a.Identity.Post(r.Context(), "/internal/guardians/upsert", guardianID+":otp-account", map[string]any{
			"school_id":    schoolID,
			"source":       "admission",
			"external_ref": guardianID,
			"name":         guardianName,
			"email":        email,
			"phone":        phone,
		}, &identityOut)
		if identityOut.ID != "" {
			_, _ = a.DB.ExecContext(r.Context(), `UPDATE guardians SET identity_user_id=$1 WHERE id=$2`, identityOut.ID, guardianID)
		}
	}

	token, expiresAt, err := a.guardianToken(guardianID, schoolID)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "session issue failed")
		return
	}
	common.WriteJSON(w, http.StatusOK, map[string]any{
		"token":      token,
		"expires_at": expiresAt,
		"expires_in": int64(a.Config.GuardianTTL.Seconds()),
		"guardian": map[string]any{
			"id":           guardianID,
			"name":         guardianName,
			"contact":      contact,
			"contact_type": kind,
			"verified_at":  time.Now(),
		},
	}, nil)
}

// hashVoucher hashes a voucher code for storage; the plaintext is never kept.
func hashVoucher(code string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(code), bcrypt.DefaultCost)
	return string(b), err
}
