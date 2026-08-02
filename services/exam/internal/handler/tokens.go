package handler

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"skci/exam/internal/common"
)

// Voucher management (client spec 2026-07-24: admin-only, issued per
// student from Data Siswa — not per exam, and not guru-accessible). Dates
// are plain YYYY-MM-DD (no time-of-day component — a voucher is valid for
// whole days).
type issueTokenReq struct {
	ValidFrom  string `json:"valid_from"`
	ValidUntil string `json:"valid_until"`
}

func parseVoucherDates(req issueTokenReq) (time.Time, time.Time, string, bool) {
	if req.ValidFrom == "" || req.ValidUntil == "" {
		return time.Time{}, time.Time{}, "valid_from and valid_until are required (YYYY-MM-DD)", false
	}
	from, err := time.Parse("2006-01-02", req.ValidFrom)
	if err != nil {
		return time.Time{}, time.Time{}, "valid_from must be YYYY-MM-DD", false
	}
	until, err := time.Parse("2006-01-02", req.ValidUntil)
	if err != nil {
		return time.Time{}, time.Time{}, "valid_until must be YYYY-MM-DD", false
	}
	if until.Before(from) {
		return time.Time{}, time.Time{}, "valid_until cannot be before valid_from", false
	}
	return from, until, "", true
}

// IssueToken: POST /students/{userId}/voucher — issues or reissues (same
// code, new date range) the student's voucher.
func (h *Handler) IssueToken(w http.ResponseWriter, r *http.Request) {
	c := common.FromCtx(r)
	if !requireRole(w, c, "admin", "superadmin") {
		return
	}
	userID := chi.URLParam(r, "userId")
	var req issueTokenReq
	if !common.DecodeJSON(w, r, &req) {
		return
	}
	from, until, msg, ok := parseVoucherDates(req)
	if !ok {
		common.WriteError(w, http.StatusBadRequest, msg)
		return
	}
	schoolID := common.EffectiveSchoolID(r, c)
	tok, err := h.Store.IssueToken(r.Context(), schoolID, r.URL.Query().Get("academic_year_id"), userID, c.UserID, from, until)
	if err != nil {
		writeStoreErr(w, err)
		return
	}
	common.WriteJSON(w, http.StatusCreated, tok, nil)
}

type bulkIssueTokenReq struct {
	StudentIDs []string `json:"student_ids"`
	ValidFrom  string   `json:"valid_from"`
	ValidUntil string   `json:"valid_until"`
}

// BulkIssueTokens: POST /vouchers/bulk — issues the same date-range voucher
// for a list of students at once (e.g. a whole classroom picked in Data
// Siswa).
func (h *Handler) BulkIssueTokens(w http.ResponseWriter, r *http.Request) {
	c := common.FromCtx(r)
	if !requireRole(w, c, "admin", "superadmin") {
		return
	}
	var req bulkIssueTokenReq
	if !common.DecodeJSON(w, r, &req) {
		return
	}
	if len(req.StudentIDs) == 0 {
		common.WriteError(w, http.StatusBadRequest, "student_ids must be non-empty")
		return
	}
	from, until, msg, ok := parseVoucherDates(issueTokenReq{ValidFrom: req.ValidFrom, ValidUntil: req.ValidUntil})
	if !ok {
		common.WriteError(w, http.StatusBadRequest, msg)
		return
	}
	schoolID := common.EffectiveSchoolID(r, c)
	academicYearID := r.URL.Query().Get("academic_year_id")
	var issued []interface{}
	for _, sid := range req.StudentIDs {
		tok, err := h.Store.IssueToken(r.Context(), schoolID, academicYearID, sid, c.UserID, from, until)
		if err != nil {
			writeStoreErr(w, err)
			return
		}
		issued = append(issued, tok)
	}
	common.WriteJSON(w, http.StatusCreated, issued, nil)
}

// ListTokens: GET /vouchers — every student voucher issued in the school
// (Data Siswa's voucher column reads this).
func (h *Handler) ListTokens(w http.ResponseWriter, r *http.Request) {
	c := common.FromCtx(r)
	if !requireRole(w, c, "admin", "superadmin") {
		return
	}
	list, err := h.Store.ListTokens(r.Context(), common.EffectiveSchoolID(r, c))
	if err != nil {
		writeStoreErr(w, err)
		return
	}
	common.WriteJSON(w, http.StatusOK, list, nil)
}

type regenTokenReq struct {
	ValidFrom  string `json:"valid_from"`
	ValidUntil string `json:"valid_until"`
}

// RegenerateToken: POST /vouchers/{tokenId}/regenerate — new code, optional
// new date range (omit both to just rotate the code).
func (h *Handler) RegenerateToken(w http.ResponseWriter, r *http.Request) {
	c := common.FromCtx(r)
	if !requireRole(w, c, "admin", "superadmin") {
		return
	}
	tokenID := chi.URLParam(r, "tokenId")
	var req regenTokenReq
	if r.ContentLength > 0 {
		if !common.DecodeJSON(w, r, &req) {
			return
		}
	}
	var from, until *time.Time
	if req.ValidFrom != "" && req.ValidUntil != "" {
		f, u, msg, ok := parseVoucherDates(issueTokenReq{ValidFrom: req.ValidFrom, ValidUntil: req.ValidUntil})
		if !ok {
			common.WriteError(w, http.StatusBadRequest, msg)
			return
		}
		from, until = &f, &u
	}
	tok, err := h.Store.RegenerateToken(r.Context(), tokenID, common.EffectiveSchoolID(r, c), from, until)
	if err != nil {
		writeStoreErr(w, err)
		return
	}
	if tok == nil {
		common.WriteError(w, http.StatusNotFound, "token not found")
		return
	}
	common.WriteJSON(w, http.StatusOK, tok, nil)
}
