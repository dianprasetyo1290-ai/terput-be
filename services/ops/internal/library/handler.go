package library

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"skci/ops/internal/client"
	"skci/ops/internal/common"
)

var (
	Officers      = []string{common.RolePustakawan, common.RoleAdmin, common.RoleSuperadmin}
	ReportReaders = []string{common.RolePustakawan, common.RoleKepalaSekolah, common.RoleAdmin, common.RoleSuperadmin}
)

func claimsUserID(r *http.Request) string {
	if c := common.FromCtx(r); c != nil {
		return c.UserID
	}
	return ""
}

type Handler struct {
	Store     *Store
	Client    *client.Client
	JobRunner *JobRunner
}

func NewHandler(s *Store, c *client.Client, j *JobRunner) *Handler {
	return &Handler{
		Store:     s,
		Client:    c,
		JobRunner: j,
	}
}

// --- Settings ---

func (h *Handler) GetSettings(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	st, err := h.Store.GetSettings(r.Context(), schoolID)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, st, nil)
}

func (h *Handler) UpdateSettings(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	var req struct {
		FinePolicy         string `json:"fine_policy"`
		ReminderDaysBefore int    `json:"reminder_days_before"`
	}
	if !common.Decode(w, r, &req) {
		return
	}
	if req.FinePolicy != "pay_at_library" && req.FinePolicy != "bill" {
		common.WriteError(w, http.StatusBadRequest, "Kebijakan denda ('pay_at_library'/'bill') wajib valid")
		return
	}
	if req.ReminderDaysBefore < 0 {
		common.WriteError(w, http.StatusBadRequest, "Hari pengingat harus >= 0")
		return
	}

	st := Settings{
		SchoolID:           schoolID,
		FinePolicy:         req.FinePolicy,
		ReminderDaysBefore: req.ReminderDaysBefore,
	}
	if err := h.Store.UpdateSettings(r.Context(), st); err != nil {
		common.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, st, nil)
}

// --- Member Categories ---

func (h *Handler) ListMemberCategories(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	list, err := h.Store.ListMemberCategories(r.Context(), schoolID)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, list, nil)
}

func (h *Handler) CreateMemberCategory(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	var c MemberCategory
	if !common.Decode(w, r, &c) {
		return
	}
	if c.Name == "" || (c.AppliesTo != "student" && c.AppliesTo != "employee") || c.MaxLoans <= 0 || c.LoanDays <= 0 {
		common.WriteError(w, http.StatusBadRequest, "Nama, sasaran ('student'/'employee'), batas pinjam (>0) dan durasi hari (>0) wajib diisi")
		return
	}
	c.SchoolID = schoolID
	if c.ReservationHoldDays <= 0 {
		c.ReservationHoldDays = 2
	}
	created, err := h.Store.CreateMemberCategory(r.Context(), c)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	common.WriteJSON(w, http.StatusCreated, created, nil)
}

func (h *Handler) UpdateMemberCategory(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	id := chi.URLParam(r, "id")
	var c MemberCategory
	if !common.Decode(w, r, &c) {
		return
	}
	c.SchoolID = schoolID
	c.ID = id
	if err := h.Store.UpdateMemberCategory(r.Context(), c); err != nil {
		if err == ErrNotFound {
			common.WriteError(w, http.StatusNotFound, "Kategori anggota tidak ditemukan")
			return
		}
		common.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"}, nil)
}

func (h *Handler) DeleteMemberCategory(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	id := chi.URLParam(r, "id")
	if err := h.Store.DeleteMemberCategory(r.Context(), schoolID, id); err != nil {
		if err == ErrNotFound {
			common.WriteError(w, http.StatusNotFound, "Kategori anggota tidak ditemukan")
			return
		}
		common.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, map[string]string{"status": "deleted"}, nil)
}

// --- Fine Rates ---

func (h *Handler) ListFineRates(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	list, err := h.Store.ListFineRates(r.Context(), schoolID)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, list, nil)
}

func (h *Handler) CreateFineRate(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	var req struct {
		MemberCategoryID *string `json:"member_category_id"`
		LateFeePerDayIDR int64   `json:"late_fee_per_day_idr"`
		LostFeeIDR       *int64  `json:"lost_fee_idr"`
		EffectiveFrom    string  `json:"effective_from"`
		EffectiveTo      *string `json:"effective_to"`
	}
	if !common.Decode(w, r, &req) {
		return
	}
	from, err := time.Parse("2006-01-02", req.EffectiveFrom)
	if err != nil {
		common.WriteError(w, http.StatusBadRequest, "Format effective_from wajib YYYY-MM-DD")
		return
	}
	var pTo *time.Time
	if req.EffectiveTo != nil && *req.EffectiveTo != "" {
		to, err := time.Parse("2006-01-02", *req.EffectiveTo)
		if err != nil {
			common.WriteError(w, http.StatusBadRequest, "Format effective_to wajib YYYY-MM-DD")
			return
		}
		pTo = &to
	}

	rate := FineRate{
		SchoolID:         schoolID,
		MemberCategoryID: req.MemberCategoryID,
		LateFeePerDayIDR: req.LateFeePerDayIDR,
		LostFeeIDR:       req.LostFeeIDR,
		EffectiveFrom:    from,
		EffectiveTo:      pTo,
	}
	created, err := h.Store.CreateFineRate(r.Context(), rate)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	common.WriteJSON(w, http.StatusCreated, created, nil)
}

func (h *Handler) DeleteFineRate(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	id := chi.URLParam(r, "id")
	if err := h.Store.DeleteFineRate(r.Context(), schoolID, id); err != nil {
		if err == ErrNotFound {
			common.WriteError(w, http.StatusNotFound, "Tarif denda tidak ditemukan")
			return
		}
		common.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, map[string]string{"status": "deleted"}, nil)
}

// --- Collection Categories ---

func (h *Handler) ListCollectionCategories(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	list, err := h.Store.ListCollectionCategories(r.Context(), schoolID)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, list, nil)
}

func (h *Handler) CreateCollectionCategory(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	var c CollectionCategory
	if !common.Decode(w, r, &c) {
		return
	}
	if c.Code == "" || c.Name == "" {
		common.WriteError(w, http.StatusBadRequest, "Kode dan nama kategori koleksi wajib diisi")
		return
	}
	c.SchoolID = schoolID
	created, err := h.Store.CreateCollectionCategory(r.Context(), c)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	common.WriteJSON(w, http.StatusCreated, created, nil)
}

func (h *Handler) UpdateCollectionCategory(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	id := chi.URLParam(r, "id")
	var c CollectionCategory
	if !common.Decode(w, r, &c) {
		return
	}
	c.SchoolID = schoolID
	c.ID = id
	if err := h.Store.UpdateCollectionCategory(r.Context(), c); err != nil {
		if err == ErrNotFound {
			common.WriteError(w, http.StatusNotFound, "Kategori koleksi tidak ditemukan")
			return
		}
		common.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"}, nil)
}

func (h *Handler) DeleteCollectionCategory(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	id := chi.URLParam(r, "id")
	if err := h.Store.DeleteCollectionCategory(r.Context(), schoolID, id); err != nil {
		if err == ErrNotFound {
			common.WriteError(w, http.StatusNotFound, "Kategori koleksi tidak ditemukan")
			return
		}
		common.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, map[string]string{"status": "deleted"}, nil)
}

// --- Titles & Copies ---

func (h *Handler) ListTitles(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	q := r.URL.Query().Get("q")
	categoryID := r.URL.Query().Get("category_id")
	list, err := h.Store.ListTitles(r.Context(), schoolID, q, categoryID)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, list, nil)
}

func (h *Handler) GetTitle(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	id := chi.URLParam(r, "id")
	t, err := h.Store.GetTitle(r.Context(), schoolID, id)
	if err != nil {
		if err == ErrNotFound {
			common.WriteError(w, http.StatusNotFound, "Judul buku tidak ditemukan")
			return
		}
		common.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, t, nil)
}

func (h *Handler) CreateTitle(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	var req struct {
		Title               string   `json:"title"`
		Author              string   `json:"author"`
		Publisher           string   `json:"publisher"`
		ISBN                *string  `json:"isbn"`
		PublishYear         *int     `json:"publish_year"`
		CategoryID          string   `json:"category_id"`
		ReplacementPriceIDR *int64   `json:"replacement_price_idr"`
		CopiesBarcodes      []string `json:"copies_barcodes"`
		ShelfLocation       string   `json:"shelf_location"`
	}
	if !common.Decode(w, r, &req) {
		return
	}
	if req.Title == "" || req.Author == "" || req.CategoryID == "" {
		common.WriteError(w, http.StatusBadRequest, "Judul, penulis, dan kategori koleksi wajib diisi")
		return
	}

	t := Title{
		SchoolID:            schoolID,
		Title:               req.Title,
		Author:              req.Author,
		Publisher:           req.Publisher,
		ISBN:                req.ISBN,
		PublishYear:         req.PublishYear,
		CategoryID:          req.CategoryID,
		ReplacementPriceIDR: req.ReplacementPriceIDR,
	}
	created, err := h.Store.CreateTitle(r.Context(), t)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Create initial copies if provided
	location := req.ShelfLocation
	if location == "" {
		location = "Rak Utama"
	}
	for _, barcode := range req.CopiesBarcodes {
		if barcode != "" {
			_, _ = h.Store.CreateCopy(r.Context(), Copy{
				SchoolID:      schoolID,
				TitleID:       created.ID,
				Barcode:       barcode,
				ShelfLocation: location,
				Status:        "available",
			})
		}
	}

	common.WriteJSON(w, http.StatusCreated, created, nil)
}

func (h *Handler) UpdateTitle(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	id := chi.URLParam(r, "id")
	var t Title
	if !common.Decode(w, r, &t) {
		return
	}
	t.SchoolID = schoolID
	t.ID = id
	if err := h.Store.UpdateTitle(r.Context(), t); err != nil {
		if err == ErrNotFound {
			common.WriteError(w, http.StatusNotFound, "Judul buku tidak ditemukan")
			return
		}
		common.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"}, nil)
}

func (h *Handler) DeleteTitle(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	id := chi.URLParam(r, "id")
	if err := h.Store.DeleteTitle(r.Context(), schoolID, id); err != nil {
		if err == ErrNotFound {
			common.WriteError(w, http.StatusNotFound, "Judul buku tidak ditemukan")
			return
		}
		common.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, map[string]string{"status": "deleted"}, nil)
}

// --- Copies ---

func (h *Handler) AddCopy(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	titleID := chi.URLParam(r, "titleId")
	var cp Copy
	if !common.Decode(w, r, &cp) {
		return
	}
	if cp.Barcode == "" || cp.ShelfLocation == "" {
		common.WriteError(w, http.StatusBadRequest, "Barcode dan lokasi rak wajib diisi")
		return
	}
	cp.SchoolID = schoolID
	cp.TitleID = titleID
	cp.Status = "available"
	created, err := h.Store.CreateCopy(r.Context(), cp)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	common.WriteJSON(w, http.StatusCreated, created, nil)
}

func (h *Handler) UpdateCopy(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	id := chi.URLParam(r, "id")
	var cp Copy
	if !common.Decode(w, r, &cp) {
		return
	}
	cp.SchoolID = schoolID
	cp.ID = id
	if err := h.Store.UpdateCopy(r.Context(), cp); err != nil {
		if err == ErrNotFound {
			common.WriteError(w, http.StatusNotFound, "Eksemplar tidak ditemukan")
			return
		}
		common.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"}, nil)
}

func (h *Handler) DeleteCopy(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	id := chi.URLParam(r, "id")
	if err := h.Store.DeleteCopy(r.Context(), schoolID, id); err != nil {
		if err == ErrNotFound {
			common.WriteError(w, http.StatusNotFound, "Eksemplar tidak ditemukan")
			return
		}
		common.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, map[string]string{"status": "deleted"}, nil)
}

func (h *Handler) GetCopyLabel(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	barcode := chi.URLParam(r, "barcode")
	cp, err := h.Store.GetCopyByBarcode(r.Context(), schoolID, barcode)
	if err != nil {
		common.WriteError(w, http.StatusNotFound, err.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, cp, nil)
}

// --- Members ---

func (h *Handler) ListMembers(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	status := r.URL.Query().Get("status")
	q := r.URL.Query().Get("q")
	list, err := h.Store.ListMembers(r.Context(), schoolID, status, q)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, list, nil)
}

func (h *Handler) GetMember(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	id := chi.URLParam(r, "id")
	m, err := h.Store.GetMember(r.Context(), schoolID, id)
	if err != nil {
		if err == ErrNotFound {
			common.WriteError(w, http.StatusNotFound, "Anggota perpustakaan tidak ditemukan")
			return
		}
		common.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Also fetch active loans and open fines
	fines, _ := h.Store.ListFines(r.Context(), schoolID, "open", m.ID)
	reservations, _ := h.Store.ListReservations(r.Context(), schoolID, m.ID)

	resp := map[string]any{
		"member":       m,
		"fines":        fines,
		"reservations": reservations,
	}
	common.WriteJSON(w, http.StatusOK, resp, nil)
}

func (h *Handler) UpdateMember(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	id := chi.URLParam(r, "id")
	var req struct {
		CategoryID string `json:"category_id"`
	}
	if !common.Decode(w, r, &req) {
		return
	}
	if err := h.Store.UpdateMemberCategoryOnly(r.Context(), schoolID, id, req.CategoryID); err != nil {
		common.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"}, nil)
}

func (h *Handler) SyncMembers(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	count, err := h.JobRunner.RunMemberSync(r.Context(), schoolID)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, map[string]any{"synced_count": count}, nil)
}

func (h *Handler) GetMemberCard(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	id := chi.URLParam(r, "id")
	m, err := h.Store.GetMember(r.Context(), schoolID, id)
	if err != nil {
		common.WriteError(w, http.StatusNotFound, "Anggota tidak ditemukan")
		return
	}
	common.WriteJSON(w, http.StatusOK, m, nil)
}

// --- Circulation (Loans, Returns, Renew, Lost, Visits) ---

func (h *Handler) CreateLoan(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	loanedBy := claimsUserID(r)
	var req struct {
		MemberCode    string `json:"member_code"`
		CopyBarcode   string `json:"copy_barcode"`
		ClassroomID   string `json:"classroom_id"`
		ClassroomName string `json:"classroom_name"`
	}
	if !common.Decode(w, r, &req) {
		return
	}
	if req.MemberCode == "" || req.CopyBarcode == "" {
		common.WriteError(w, http.StatusBadRequest, "Nomor kartu anggota dan barcode buku wajib diisi")
		return
	}

	idempKey := r.Header.Get("Idempotency-Key")
	if idempKey == "" {
		idempKey = fmt.Sprintf("loan:%s:%s:%d", req.MemberCode, req.CopyBarcode, time.Now().UnixNano())
	}

	loan, err := h.Store.BorrowCopy(r.Context(), schoolID, req.MemberCode, req.CopyBarcode, req.ClassroomID, req.ClassroomName, loanedBy, idempKey)
	if err != nil {
		common.WriteError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	common.WriteJSON(w, http.StatusCreated, loan, nil)
}

func (h *Handler) ReturnCopy(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	returnedBy := claimsUserID(r)
	var req struct {
		CopyBarcode string `json:"copy_barcode"`
		ReturnedOn  string `json:"returned_on"`
	}
	if !common.Decode(w, r, &req) {
		return
	}
	if req.CopyBarcode == "" {
		common.WriteError(w, http.StatusBadRequest, "Barcode buku wajib diisi")
		return
	}

	returnedOn := time.Now()
	if req.ReturnedOn != "" {
		if parsed, err := time.Parse("2006-01-02", req.ReturnedOn); err == nil {
			returnedOn = parsed
		}
	}

	res, err := h.Store.ReturnCopy(r.Context(), schoolID, req.CopyBarcode, returnedBy, returnedOn)
	if err != nil {
		common.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	// If fine generated and settlement is 'bill', trigger billing adhoc
	if res.Fine != nil && res.Fine.Settlement == "bill" {
		go func(f Fine) {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			// Fetch student user_id and title
			m, _ := h.Store.GetMember(ctx, schoolID, f.MemberID)
			t, _ := h.Store.GetTitle(ctx, schoolID, res.Loan.TitleID)
			_ = h.JobRunner.BillFineToAccounting(ctx, f, m.UserID, t.Title)
		}(*res.Fine)
	}

	common.WriteJSON(w, http.StatusOK, res, nil)
}

func (h *Handler) RenewLoan(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	loanID := chi.URLParam(r, "id")
	now := time.Now()
	loan, err := h.Store.RenewLoan(r.Context(), schoolID, loanID, now)
	if err != nil {
		common.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, loan, nil)
}

func (h *Handler) ReportLost(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	loanID := chi.URLParam(r, "id")
	reportedBy := claimsUserID(r)
	fine, err := h.Store.ReportLost(r.Context(), schoolID, loanID, reportedBy)
	if err != nil {
		common.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, fine, nil)
}

func (h *Handler) RecordVisit(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	var req struct {
		MemberCode string `json:"member_code"`
	}
	if !common.Decode(w, r, &req) {
		return
	}
	v, err := h.Store.RecordVisit(r.Context(), schoolID, req.MemberCode, time.Now())
	if err != nil {
		common.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, v, nil)
}

// --- Fines ---

func (h *Handler) ListFines(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	status := r.URL.Query().Get("status")
	memberID := r.URL.Query().Get("member_id")
	list, err := h.Store.ListFines(r.Context(), schoolID, status, memberID)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, list, nil)
}

func (h *Handler) PayFine(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	receivedBy := claimsUserID(r)
	id := chi.URLParam(r, "id")
	if err := h.Store.PayFine(r.Context(), schoolID, id, receivedBy); err != nil {
		if err == ErrNotFound {
			common.WriteError(w, http.StatusNotFound, "Denda tidak ditemukan")
			return
		}
		common.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, map[string]string{"status": "paid"}, nil)
}

func (h *Handler) WaiveFine(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	waivedBy := claimsUserID(r)
	id := chi.URLParam(r, "id")
	var req struct {
		Reason string `json:"reason"`
	}
	if !common.Decode(w, r, &req) {
		return
	}
	if req.Reason == "" {
		common.WriteError(w, http.StatusBadRequest, "Alasan keringanan / penghapusan denda wajib diisi")
		return
	}
	if err := h.Store.WaiveFine(r.Context(), schoolID, id, waivedBy, req.Reason); err != nil {
		if err == ErrNotFound {
			common.WriteError(w, http.StatusNotFound, "Denda tidak ditemukan")
			return
		}
		common.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, map[string]string{"status": "waived"}, nil)
}

// --- Reports ---

func (h *Handler) ReportLoans(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	fromStr := r.URL.Query().Get("from")
	toStr := r.URL.Query().Get("to")
	groupBy := r.URL.Query().Get("group_by")

	from, err := time.Parse("2006-01-02", fromStr)
	if err != nil {
		from = time.Now().AddDate(0, -1, 0)
	}
	to, err := time.Parse("2006-01-02", toStr)
	if err != nil {
		to = time.Now().AddDate(0, 0, 1)
	}

	report, err := h.Store.LoansReport(r.Context(), schoolID, from, to, groupBy)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, report, nil)
}

func (h *Handler) ReportPopular(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	fromStr := r.URL.Query().Get("from")
	toStr := r.URL.Query().Get("to")
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))

	from, err := time.Parse("2006-01-02", fromStr)
	if err != nil {
		from = time.Now().AddDate(0, -3, 0)
	}
	to, err := time.Parse("2006-01-02", toStr)
	if err != nil {
		to = time.Now().AddDate(0, 0, 1)
	}

	report, err := h.Store.PopularTitles(r.Context(), schoolID, from, to, limit)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, report, nil)
}

// ReportOverdue implements AC-115-01: list of overdue loans.
func (h *Handler) ReportOverdue(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	list, err := h.Store.OverdueLoans(r.Context(), schoolID)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, list, nil)
}

func (h *Handler) ReportVisits(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	fromStr := r.URL.Query().Get("from")
	toStr := r.URL.Query().Get("to")

	from, err := time.Parse("2006-01-02", fromStr)
	if err != nil {
		from = time.Now().AddDate(0, -1, 0)
	}
	to, err := time.Parse("2006-01-02", toStr)
	if err != nil {
		to = time.Now()
	}

	visits, err := h.Store.ListVisits(r.Context(), schoolID, from, to)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, visits, nil)
}

// --- OPAC ---

func (h *Handler) OpacSearch(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	q := r.URL.Query().Get("q")
	categoryID := r.URL.Query().Get("category_id")

	titles, err := h.Store.ListTitles(r.Context(), schoolID, q, categoryID)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Security requirement: OPAC does not expose individual copies or borrower identities
	type OpacItem struct {
		ID              string `json:"id"`
		Title           string `json:"title"`
		Author          string `json:"author"`
		Publisher       string `json:"publisher"`
		PublishYear     *int   `json:"publish_year,omitempty"`
		CategoryName    string `json:"category_name"`
		TotalCopies     int    `json:"total_copies"`
		AvailableCopies int    `json:"available_copies"`
	}

	res := make([]OpacItem, len(titles))
	for i, t := range titles {
		res[i] = OpacItem{
			ID:              t.ID,
			Title:           t.Title,
			Author:          t.Author,
			Publisher:       t.Publisher,
			PublishYear:     t.PublishYear,
			CategoryName:    t.CategoryName,
			TotalCopies:     t.TotalCopies,
			AvailableCopies: t.AvailableCopies,
		}
	}
	common.WriteJSON(w, http.StatusOK, res, nil)
}

func (h *Handler) OpacDetail(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	id := chi.URLParam(r, "id")
	t, err := h.Store.GetTitle(r.Context(), schoolID, id)
	if err != nil {
		common.WriteError(w, http.StatusNotFound, "Buku tidak ditemukan")
		return
	}

	// Hide copy barcodes/borrower details in public OPAC detail
	type OpacCopyLocation struct {
		ShelfLocation string `json:"shelf_location"`
		Status        string `json:"status"`
	}
	var locations []OpacCopyLocation
	for _, cp := range t.Copies {
		locations = append(locations, OpacCopyLocation{
			ShelfLocation: cp.ShelfLocation,
			Status:        cp.Status,
		})
	}

	res := map[string]any{
		"id":               t.ID,
		"title":            t.Title,
		"author":           t.Author,
		"publisher":        t.Publisher,
		"isbn":             t.ISBN,
		"publish_year":     t.PublishYear,
		"category_name":    t.CategoryName,
		"total_copies":     t.TotalCopies,
		"available_copies": t.AvailableCopies,
		"locations":        locations,
	}
	common.WriteJSON(w, http.StatusOK, res, nil)
}

// --- My Library (/me/library) ---

func (h *Handler) MyLibrary(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	userID := claimsUserID(r)

	member, err := h.Store.GetMemberByUserID(r.Context(), schoolID, userID)
	if err != nil {
		if err == ErrNotFound {
			// Not (yet) a library member: an empty state, not an error — the
			// "Pustaka Saya" menu is shown to every member-eligible role.
			common.WriteJSON(w, http.StatusOK, map[string]any{
				"member":       nil,
				"active_loans": []Loan{},
				"loans":        []Loan{},
				"fines":        []any{},
				"reservations": []any{},
			}, nil)
			return
		}
		common.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Active loans
	rows, err := h.Store.db.QueryContext(r.Context(), `
		SELECT l.id, l.copy_id, l.loaned_at, l.due_date, l.renew_count, l.status,
		       t.id, t.title, cp.barcode
		FROM lib_loans l
		JOIN lib_copies cp ON cp.id = l.copy_id
		JOIN lib_titles t ON t.id = cp.title_id
		WHERE l.school_id = $1 AND l.member_id = $2 AND l.status = 'active'
		ORDER BY l.due_date ASC
	`, schoolID, member.ID)

	loans := []Loan{}
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var l Loan
			if err := rows.Scan(&l.ID, &l.CopyID, &l.LoanedAt, &l.DueDate, &l.RenewCount, &l.Status, &l.TitleID, &l.TitleName, &l.CopyBarcode); err == nil {
				loans = append(loans, l)
			}
		}
	}

	fines, _ := h.Store.ListFines(r.Context(), schoolID, "open", member.ID)
	reservations, _ := h.Store.ListReservations(r.Context(), schoolID, member.ID)

	if fines == nil {
		fines = []Fine{}
	}
	if reservations == nil {
		reservations = []Reservation{}
	}
	resp := map[string]any{
		"member":       member,
		"active_loans": loans, // key the SPA (MyLibraryView) reads
		"loans":        loans,
		"fines":        fines,
		"reservations": reservations,
	}
	common.WriteJSON(w, http.StatusOK, resp, nil)
}

func (h *Handler) MyRenewLoan(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	userID := claimsUserID(r)
	loanID := chi.URLParam(r, "id")

	// Ensure loan belongs to member
	member, err := h.Store.GetMemberByUserID(r.Context(), schoolID, userID)
	if err != nil {
		common.WriteError(w, http.StatusForbidden, "Tidak berhak mengakses")
		return
	}

	var ownerMemberID string
	err = h.Store.db.QueryRowContext(r.Context(), `SELECT member_id FROM lib_loans WHERE school_id = $1 AND id = $2`, schoolID, loanID).Scan(&ownerMemberID)
	if err != nil || ownerMemberID != member.ID {
		common.WriteError(w, http.StatusForbidden, "Pinjaman ini bukan milik Anda")
		return
	}

	loan, err := h.Store.RenewLoan(r.Context(), schoolID, loanID, time.Now())
	if err != nil {
		common.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, loan, nil)
}

func (h *Handler) MyReserveTitle(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	userID := claimsUserID(r)
	var req struct {
		TitleID string `json:"title_id"`
	}
	if !common.Decode(w, r, &req) {
		return
	}

	member, err := h.Store.GetMemberByUserID(r.Context(), schoolID, userID)
	if err != nil {
		common.WriteError(w, http.StatusForbidden, "Tidak berhak mengakses")
		return
	}

	res, err := h.Store.CreateReservation(r.Context(), schoolID, req.TitleID, member.ID)
	if err != nil {
		common.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	common.WriteJSON(w, http.StatusCreated, res, nil)
}

func (h *Handler) MyCancelReservation(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	userID := claimsUserID(r)
	resID := chi.URLParam(r, "id")

	member, err := h.Store.GetMemberByUserID(r.Context(), schoolID, userID)
	if err != nil {
		common.WriteError(w, http.StatusForbidden, "Tidak berhak mengakses")
		return
	}

	if err := h.Store.CancelReservation(r.Context(), schoolID, resID, member.ID); err != nil {
		common.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, map[string]string{"status": "cancelled"}, nil)
}

// InternalBillFine allows retrying billing of a fine
func (h *Handler) InternalBillFine(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	fineID := chi.URLParam(r, "id")

	f, err := h.Store.GetFine(r.Context(), schoolID, fineID)
	if err != nil {
		common.WriteError(w, http.StatusNotFound, "Denda tidak ditemukan")
		return
	}
	m, _ := h.Store.GetMember(r.Context(), schoolID, f.MemberID)

	if err := h.JobRunner.BillFineToAccounting(r.Context(), f, m.UserID, f.TitleName); err != nil {
		common.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, map[string]string{"status": "billed"}, nil)
}
