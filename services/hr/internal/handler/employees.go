package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/xuri/excelize/v2"

	"skci/hr/internal/common"
	"skci/hr/internal/model"
	"skci/hr/internal/store"
)

type CreateEmployeeRequest struct {
	FullName          string  `json:"full_name"`
	Email             string  `json:"email"`
	Phone             string  `json:"phone"`
	NIK               string  `json:"nik"`
	NPWP              string  `json:"npwp"`
	PTKPStatus        string  `json:"ptkp_status"`
	BankCode          string  `json:"bank_code"`
	BankAccount       string  `json:"bank_account"`
	BankAccountName   string  `json:"bank_account_name"`
	BirthDate         string  `json:"birth_date"`
	Gender            string  `json:"gender"`
	Address           string  `json:"address"`
	HireDate          string  `json:"hire_date"`
	CreateUserAccount bool    `json:"create_user_account"`
	UserRole          string  `json:"user_role"`

	// Initial Assignment
	PositionID           string  `json:"position_id"`
	DepartmentID         *string `json:"department_id"`
	SupervisorEmployeeID *string `json:"supervisor_employee_id"`
	EmploymentType       string  `json:"employment_type"`
	Grade                *string `json:"grade"`
	BaseSalaryIDR        *int64  `json:"base_salary_idr"`
	ContractEnd          *string `json:"contract_end"`

	Force       bool   `json:"force"`
	ForceReason string `json:"force_reason"`
}

func (a *App) ListEmployees(w http.ResponseWriter, r *http.Request) {
	schoolID := a.effectiveSchoolID(r)
	page, perPage := parsePagination(r)

	filter := store.EmployeeFilter{
		Q:              r.URL.Query().Get("q"),
		DepartmentID:   r.URL.Query().Get("department_id"),
		PositionID:     r.URL.Query().Get("position_id"),
		Status:         r.URL.Query().Get("status"),
		EmploymentType: r.URL.Query().Get("employment_type"),
		Page:           page,
		PerPage:        perPage,
	}

	employees, total, err := store.ListEmployees(r.Context(), a.DB, schoolID, filter)
	if err != nil {
		common.Error(w, http.StatusInternalServerError, "Gagal mengambil daftar pegawai")
		return
	}

	claims := common.GetClaims(r.Context())
	allowFull := claims != nil && (claims.Role == common.RoleSuperadmin || claims.Role == common.RoleHRAdmin)

	var list []map[string]any
	for i := range employees {
		list = append(list, a.maskEmployee(&employees[i], allowFull))
	}

	common.JSONWithMeta(w, http.StatusOK, list, common.Meta{
		Page:       page,
		PerPage:    perPage,
		Total:      total,
		TotalPages: (total + perPage - 1) / perPage,
	})
}

func (a *App) CreateEmployee(w http.ResponseWriter, r *http.Request) {
	schoolID := a.effectiveSchoolID(r)
	claims := common.GetClaims(r.Context())

	var req CreateEmployeeRequest
	if err := decodeJSON(r, &req); err != nil {
		common.Error(w, http.StatusBadRequest, "Format JSON tidak valid")
		return
	}

	if strings.TrimSpace(req.FullName) == "" {
		common.Error(w, http.StatusBadRequest, "Nama lengkap wajib diisi")
		return
	}
	if strings.TrimSpace(req.HireDate) == "" {
		common.Error(w, http.StatusBadRequest, "Tanggal masuk (hire_date) wajib diisi")
		return
	}
	if strings.TrimSpace(req.PositionID) == "" {
		common.Error(w, http.StatusBadRequest, "Jabatan (position_id) wajib diisi")
		return
	}
	if req.EmploymentType == "" {
		req.EmploymentType = "kontrak"
	}

	// Compute blind indices for duplicate checks
	var nikBidx, npwpBidx string
	if req.NIK != "" {
		nikBidx = a.Crypto.BlindIndex(req.NIK, schoolID)
	}
	if req.NPWP != "" {
		npwpBidx = a.Crypto.BlindIndex(req.NPWP, schoolID)
	}

	// Check duplicates
	if (nikBidx != "" || req.Email != "") && !req.Force {
		dups, err := store.FindDuplicates(r.Context(), a.DB, schoolID, nikBidx, req.Email)
		if err != nil {
			common.Error(w, http.StatusInternalServerError, "Gagal memeriksa duplikasi")
			return
		}
		if len(dups) > 0 {
			common.JSON(w, http.StatusConflict, map[string]any{
				"error":      "Terdeteksi calon duplikat data pegawai",
				"duplicates": dups,
			})
			return
		}
	}

	// Encrypt sensitive fields
	var (
		nikEnc, npwpEnc, bankAccEnc, phoneEnc, addrEnc, salaryEnc []byte
		err                                                       error
	)
	if req.NIK != "" {
		nikEnc, err = a.Crypto.Encrypt(req.NIK, schoolID)
		if err != nil {
			common.Error(w, http.StatusInternalServerError, "Gagal mengenkripsi data")
			return
		}
	}
	if req.NPWP != "" {
		npwpEnc, err = a.Crypto.Encrypt(req.NPWP, schoolID)
		if err != nil {
			common.Error(w, http.StatusInternalServerError, "Gagal mengenkripsi data")
			return
		}
	}
	if req.BankAccount != "" {
		bankAccEnc, err = a.Crypto.Encrypt(req.BankAccount, schoolID)
		if err != nil {
			common.Error(w, http.StatusInternalServerError, "Gagal mengenkripsi data")
			return
		}
	}
	if req.Phone != "" {
		phoneEnc, err = a.Crypto.Encrypt(req.Phone, schoolID)
		if err != nil {
			common.Error(w, http.StatusInternalServerError, "Gagal mengenkripsi data")
			return
		}
	}
	if req.Address != "" {
		addrEnc, err = a.Crypto.Encrypt(req.Address, schoolID)
		if err != nil {
			common.Error(w, http.StatusInternalServerError, "Gagal mengenkripsi data")
			return
		}
	}
	if req.BaseSalaryIDR != nil {
		salaryEnc, err = a.Crypto.EncryptAmount(*req.BaseSalaryIDR, schoolID)
		if err != nil {
			common.Error(w, http.StatusInternalServerError, "Gagal mengenkripsi data gaji")
			return
		}
	}

	emp := model.Employee{
		SchoolID:       schoolID,
		FullName:       req.FullName,
		PTKPStatus:     req.PTKPStatus,
		HireDate:       req.HireDate,
		Status:         "active",
		PhoneEnc:       phoneEnc,
		NIKEnc:         nikEnc,
		NPWPEnc:        npwpEnc,
		BankAccountEnc: bankAccEnc,
		AddressEnc:     addrEnc,
	}
	if req.Email != "" {
		emp.Email = &req.Email
	}
	if nikBidx != "" {
		emp.NIKBidx = &nikBidx
	}
	if npwpBidx != "" {
		emp.NPWPBidx = &npwpBidx
	}
	if req.BankCode != "" {
		emp.BankCode = &req.BankCode
	}
	if req.BankAccountName != "" {
		emp.BankAccountName = &req.BankAccountName
	}
	if req.BirthDate != "" {
		emp.BirthDate = &req.BirthDate
	}
	if req.Gender != "" {
		emp.Gender = &req.Gender
	}

	initialAssignment := model.EmploymentAssignment{
		SchoolID:             schoolID,
		PositionID:           req.PositionID,
		DepartmentID:         req.DepartmentID,
		SupervisorEmployeeID: req.SupervisorEmployeeID,
		EmploymentType:       req.EmploymentType,
		Grade:                req.Grade,
		BaseSalaryEnc:        salaryEnc,
		ContractEnd:          req.ContractEnd,
		EffectiveFrom:        req.HireDate,
	}

	if err := store.CreateEmployee(r.Context(), a.DB, &emp, &initialAssignment); err != nil {
		common.Error(w, http.StatusInternalServerError, "Gagal menyimpan data pegawai: "+err.Error())
		return
	}

	// Link with Identity service if user account requested
	if req.CreateUserAccount && req.Email != "" {
		roleToAssign := common.RolePegawai
		if req.UserRole != "" {
			roleToAssign = req.UserRole
		}
		userID, err := a.Client.CreateOrLinkUser(r.Context(), schoolID, req.Email, req.FullName, roleToAssign)
		if err == nil && userID != "" {
			emp.UserID = &userID
			_ = a.DB.QueryRowContext(r.Context(), `UPDATE employees SET user_id = $1, updated_at = now() WHERE id = $2`, userID, emp.ID)
		}
	}

	// Audit event (sanitized before/after per NFR-HR-01)
	afterState := map[string]any{
		"id":          emp.ID,
		"employee_no": emp.EmployeeNo,
		"full_name":   emp.FullName,
		"email":       req.Email,
		"status":      emp.Status,
		"position_id": req.PositionID,
	}
	afterBytes, _ := json.Marshal(afterState)
	var actorID *string
	if claims != nil && claims.Subject != "" {
		actorID = &claims.Subject
	}
	_ = store.WriteAuditEvent(r.Context(), a.DB, schoolID, actorID, "employees", emp.ID, "create", nil, afterBytes, "Pembuatan data pegawai baru", nil)

	created, _ := store.GetEmployeeByID(r.Context(), a.DB, schoolID, emp.ID)
	common.JSON(w, http.StatusCreated, a.maskEmployee(created, true))
}

func (a *App) GetEmployee(w http.ResponseWriter, r *http.Request) {
	schoolID := a.effectiveSchoolID(r)
	id := chi.URLParam(r, "id")

	emp, err := store.GetEmployeeByID(r.Context(), a.DB, schoolID, id)
	if err != nil {
		common.Error(w, http.StatusNotFound, "Pegawai tidak ditemukan")
		return
	}

	claims := common.GetClaims(r.Context())
	allowFull := claims != nil && (claims.Role == common.RoleSuperadmin || claims.Role == common.RoleHRAdmin)

	common.JSON(w, http.StatusOK, a.maskEmployee(emp, allowFull))
}

func (a *App) UpdateEmployee(w http.ResponseWriter, r *http.Request) {
	schoolID := a.effectiveSchoolID(r)
	id := chi.URLParam(r, "id")
	claims := common.GetClaims(r.Context())

	existing, err := store.GetEmployeeByID(r.Context(), a.DB, schoolID, id)
	if err != nil {
		common.Error(w, http.StatusNotFound, "Pegawai tidak ditemukan")
		return
	}

	var req CreateEmployeeRequest
	if err := decodeJSON(r, &req); err != nil {
		common.Error(w, http.StatusBadRequest, "Format JSON tidak valid")
		return
	}

	if strings.TrimSpace(req.FullName) != "" {
		existing.FullName = req.FullName
	}
	if req.Email != "" {
		existing.Email = &req.Email
	}
	if req.PTKPStatus != "" {
		existing.PTKPStatus = req.PTKPStatus
	}
	if req.BankCode != "" {
		existing.BankCode = &req.BankCode
	}
	if req.BankAccountName != "" {
		existing.BankAccountName = &req.BankAccountName
	}
	if req.BirthDate != "" {
		existing.BirthDate = &req.BirthDate
	}
	if req.Gender != "" {
		existing.Gender = &req.Gender
	}

	if req.NIK != "" {
		nikEnc, _ := a.Crypto.Encrypt(req.NIK, schoolID)
		existing.NIKEnc = nikEnc
		bidx := a.Crypto.BlindIndex(req.NIK, schoolID)
		existing.NIKBidx = &bidx
	}
	if req.NPWP != "" {
		npwpEnc, _ := a.Crypto.Encrypt(req.NPWP, schoolID)
		existing.NPWPEnc = npwpEnc
		bidx := a.Crypto.BlindIndex(req.NPWP, schoolID)
		existing.NPWPBidx = &bidx
	}
	if req.BankAccount != "" {
		bankEnc, _ := a.Crypto.Encrypt(req.BankAccount, schoolID)
		existing.BankAccountEnc = bankEnc
	}
	if req.Phone != "" {
		phoneEnc, _ := a.Crypto.Encrypt(req.Phone, schoolID)
		existing.PhoneEnc = phoneEnc
	}
	if req.Address != "" {
		addrEnc, _ := a.Crypto.Encrypt(req.Address, schoolID)
		existing.AddressEnc = addrEnc
	}

	if err := store.UpdateEmployee(r.Context(), a.DB, existing); err != nil {
		common.Error(w, http.StatusInternalServerError, "Gagal memperbarui data pegawai")
		return
	}

	var actorID *string
	if claims != nil && claims.Subject != "" {
		actorID = &claims.Subject
	}
	_ = store.WriteAuditEvent(r.Context(), a.DB, schoolID, actorID, "employees", existing.ID, "update", nil, nil, "Pembaruan data master pegawai", nil)

	updated, _ := store.GetEmployeeByID(r.Context(), a.DB, schoolID, existing.ID)
	common.JSON(w, http.StatusOK, a.maskEmployee(updated, true))
}

type CreateAssignmentRequest struct {
	PositionID           string  `json:"position_id"`
	DepartmentID         *string `json:"department_id"`
	SupervisorEmployeeID *string `json:"supervisor_employee_id"`
	EmploymentType       string  `json:"employment_type"`
	Grade                *string `json:"grade"`
	BaseSalaryIDR        *int64  `json:"base_salary_idr"`
	ContractEnd          *string `json:"contract_end"`
	EffectiveFrom        string  `json:"effective_from"`
	EffectiveTo          *string `json:"effective_to"`
}

func (a *App) CreateAssignment(w http.ResponseWriter, r *http.Request) {
	schoolID := a.effectiveSchoolID(r)
	empID := chi.URLParam(r, "id")
	claims := common.GetClaims(r.Context())

	var req CreateAssignmentRequest
	if err := decodeJSON(r, &req); err != nil {
		common.Error(w, http.StatusBadRequest, "Format JSON tidak valid")
		return
	}

	if strings.TrimSpace(req.PositionID) == "" {
		common.Error(w, http.StatusBadRequest, "Jabatan (position_id) wajib diisi")
		return
	}
	if strings.TrimSpace(req.EffectiveFrom) == "" {
		common.Error(w, http.StatusBadRequest, "Tanggal efektif (effective_from) wajib diisi")
		return
	}
	if req.EmploymentType == "" {
		req.EmploymentType = "kontrak"
	}

	var salaryEnc []byte
	if req.BaseSalaryIDR != nil {
		salaryEnc, _ = a.Crypto.EncryptAmount(*req.BaseSalaryIDR, schoolID)
	}

	assign := model.EmploymentAssignment{
		SchoolID:             schoolID,
		EmployeeID:           empID,
		PositionID:           req.PositionID,
		DepartmentID:         req.DepartmentID,
		SupervisorEmployeeID: req.SupervisorEmployeeID,
		EmploymentType:       req.EmploymentType,
		Grade:                req.Grade,
		BaseSalaryEnc:        salaryEnc,
		ContractEnd:          req.ContractEnd,
		EffectiveFrom:        req.EffectiveFrom,
		EffectiveTo:          req.EffectiveTo,
	}

	if err := store.CreateAssignment(r.Context(), a.DB, &assign); err != nil {
		common.Error(w, http.StatusInternalServerError, "Gagal membuat penugasan/jabatan baru: "+err.Error())
		return
	}

	var actorID *string
	if claims != nil && claims.Subject != "" {
		actorID = &claims.Subject
	}
	_ = store.WriteAuditEvent(r.Context(), a.DB, schoolID, actorID, "employment_assignments", assign.ID, "create", nil, nil, "Penugasan/promosi pegawai", nil)

	common.JSON(w, http.StatusCreated, assign)
}

func (a *App) GetEmployeeTimeline(w http.ResponseWriter, r *http.Request) {
	schoolID := a.effectiveSchoolID(r)
	empID := chi.URLParam(r, "id")

	timeline, err := store.GetEmployeeTimeline(r.Context(), a.DB, schoolID, empID)
	if err != nil {
		common.Error(w, http.StatusInternalServerError, "Gagal mengambil riwayat pegawai")
		return
	}

	common.JSON(w, http.StatusOK, timeline)
}

type OffboardRequest struct {
	ExitDate string `json:"exit_date"`
	Reason   string `json:"reason"`
}

func (a *App) OffboardEmployee(w http.ResponseWriter, r *http.Request) {
	schoolID := a.effectiveSchoolID(r)
	empID := chi.URLParam(r, "id")
	claims := common.GetClaims(r.Context())

	var req OffboardRequest
	if err := decodeJSON(r, &req); err != nil {
		common.Error(w, http.StatusBadRequest, "Format JSON tidak valid")
		return
	}
	if strings.TrimSpace(req.ExitDate) == "" {
		common.Error(w, http.StatusBadRequest, "Tanggal keluar wajib diisi")
		return
	}

	emp, err := store.GetEmployeeByID(r.Context(), a.DB, schoolID, empID)
	if err != nil {
		common.Error(w, http.StatusNotFound, "Pegawai tidak ditemukan")
		return
	}

	if err := store.OffboardEmployee(r.Context(), a.DB, schoolID, empID, req.ExitDate, req.Reason); err != nil {
		common.Error(w, http.StatusInternalServerError, "Gagal melakukan offboarding pegawai")
		return
	}

	// Deactivate user in identity service if exit_date is today or in past
	today := time.Now().Format("2006-01-02")
	if req.ExitDate <= today && emp.UserID != nil {
		exitTime, _ := time.Parse("2006-01-02", req.ExitDate)
		_ = a.Client.DeactivateUser(r.Context(), *emp.UserID, schoolID, exitTime)
	}

	var actorID *string
	if claims != nil && claims.Subject != "" {
		actorID = &claims.Subject
	}
	_ = store.WriteAuditEvent(r.Context(), a.DB, schoolID, actorID, "employees", empID, "offboard", nil, nil, req.Reason, nil)

	common.JSON(w, http.StatusOK, map[string]string{
		"message":   "Proses offboarding berhasil dijadwalkan/dieksekusi",
		"exit_date": req.ExitDate,
	})
}

func (a *App) ExportEmployees(w http.ResponseWriter, r *http.Request) {
	schoolID := a.effectiveSchoolID(r)

	filter := store.EmployeeFilter{
		Q:              r.URL.Query().Get("q"),
		DepartmentID:   r.URL.Query().Get("department_id"),
		PositionID:     r.URL.Query().Get("position_id"),
		Status:         r.URL.Query().Get("status"),
		EmploymentType: r.URL.Query().Get("employment_type"),
		Page:           1,
		PerPage:        10000,
	}

	employees, _, err := store.ListEmployees(r.Context(), a.DB, schoolID, filter)
	if err != nil {
		common.Error(w, http.StatusInternalServerError, "Gagal mengambil data pegawai untuk ekspor")
		return
	}

	f := excelize.NewFile()
	sheet := "Pegawai"
	f.SetSheetName("Sheet1", sheet)

	headers := []string{"No. Pegawai", "Nama Lengkap", "Email", "Status", "Departemen", "Jabatan", "Tipe Pegawai", "Tanggal Masuk"}
	for colIdx, h := range headers {
		cell, _ := excelize.CoordinatesToCellName(colIdx+1, 1)
		f.SetCellValue(sheet, cell, h)
	}

	for rowIdx, e := range employees {
		rNum := rowIdx + 2
		dept := ""
		if e.CurrentDepartmentName != nil {
			dept = *e.CurrentDepartmentName
		}
		pos := ""
		if e.CurrentPositionName != nil {
			pos = *e.CurrentPositionName
		}
		empType := ""
		if e.CurrentEmploymentType != nil {
			empType = *e.CurrentEmploymentType
		}
		email := ""
		if e.Email != nil {
			email = *e.Email
		}

		f.SetCellValue(sheet, fmt.Sprintf("A%d", rNum), e.EmployeeNo)
		f.SetCellValue(sheet, fmt.Sprintf("B%d", rNum), e.FullName)
		f.SetCellValue(sheet, fmt.Sprintf("C%d", rNum), email)
		f.SetCellValue(sheet, fmt.Sprintf("D%d", rNum), e.Status)
		f.SetCellValue(sheet, fmt.Sprintf("E%d", rNum), dept)
		f.SetCellValue(sheet, fmt.Sprintf("F%d", rNum), pos)
		f.SetCellValue(sheet, fmt.Sprintf("G%d", rNum), empType)
		f.SetCellValue(sheet, fmt.Sprintf("H%d", rNum), e.HireDate)
	}

	w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	w.Header().Set("Content-Disposition", "attachment; filename=data-pegawai.xlsx")
	if err := f.Write(w); err != nil {
		common.Error(w, http.StatusInternalServerError, "Gagal menulis file ekspor")
	}
}

// Internal: GET /internal/employees/by-user/{userId}/supervisor
func (a *App) InternalGetSupervisorByUserID(w http.ResponseWriter, r *http.Request) {
	userID := chi.URLParam(r, "userId")
	schoolID := r.URL.Query().Get("school_id")

	emp, err := store.GetEmployeeByUserID(r.Context(), a.DB, schoolID, userID)
	if err != nil {
		common.Error(w, http.StatusNotFound, "Pegawai tidak ditemukan")
		return
	}

	if emp.CurrentSupervisorID == nil {
		common.JSON(w, http.StatusOK, map[string]any{
			"supervisor_user_id": nil,
			"supervisor_name":    nil,
		})
		return
	}

	sup, err := store.GetEmployeeByID(r.Context(), a.DB, schoolID, *emp.CurrentSupervisorID)
	if err != nil {
		common.JSON(w, http.StatusOK, map[string]any{
			"supervisor_user_id": nil,
			"supervisor_name":    nil,
		})
		return
	}

	common.JSON(w, http.StatusOK, map[string]any{
		"supervisor_employee_id": sup.ID,
		"supervisor_user_id":     sup.UserID,
		"supervisor_name":        sup.FullName,
	})
}
