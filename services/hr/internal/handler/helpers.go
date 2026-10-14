package handler

import (
	"encoding/json"
	"net/http"
	"strconv"

	"skci/hr/internal/common"
	"skci/hr/internal/model"
)

// requireHrisFeature verifies that the school has the 'hris' module active.
func (a *App) requireHrisFeature(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		schoolID := a.effectiveSchoolID(r)
		if schoolID == "" {
			common.Error(w, http.StatusBadRequest, "ID sekolah tidak valid")
			return
		}

		enabled, err := a.Client.IsHrisEnabled(r.Context(), schoolID)
		if err != nil {
			common.Error(w, http.StatusInternalServerError, "Gagal memverifikasi modul sekolah")
			return
		}
		if !enabled {
			common.Error(w, http.StatusForbidden, "Modul HRIS dinonaktifkan untuk sekolah ini")
			return
		}

		next.ServeHTTP(w, r)
	})
}

// effectiveSchoolID returns the school ID from claims, or ?school_id= for
// superadmin; "" when missing or not a UUID.
func (a *App) effectiveSchoolID(r *http.Request) string {
	return common.EffectiveSchoolID(r)
}

// parsePagination extracts page and per_page from query params with safe defaults.
func parsePagination(r *http.Request) (int, int) {
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	perPage, _ := strconv.Atoi(r.URL.Query().Get("per_page"))
	if perPage < 1 || perPage > 100 {
		perPage = 20
	}
	return page, perPage
}

func decodeJSON(r *http.Request, v any) error {
	return json.NewDecoder(r.Body).Decode(v)
}

// maskEmployeeSensitives masks sensitive employee fields for unprivileged view.
func (a *App) maskEmployee(e *model.Employee, allowFull bool) map[string]any {
	// If allowFull is true (e.g. HR Admin viewing employee detail), decrypt and show.
	var (
		nikStr, npwpStr, bankAccStr, phoneStr, addrStr string
		salaryIDR                                      *int64
	)

	if len(e.NIKEnc) > 0 {
		if pt, err := a.Crypto.Decrypt(e.NIKEnc, e.SchoolID); err == nil {
			nikStr = pt
		}
	}
	if len(e.NPWPEnc) > 0 {
		if pt, err := a.Crypto.Decrypt(e.NPWPEnc, e.SchoolID); err == nil {
			npwpStr = pt
		}
	}
	if len(e.BankAccountEnc) > 0 {
		if pt, err := a.Crypto.Decrypt(e.BankAccountEnc, e.SchoolID); err == nil {
			bankAccStr = pt
		}
	}
	if len(e.PhoneEnc) > 0 {
		if pt, err := a.Crypto.Decrypt(e.PhoneEnc, e.SchoolID); err == nil {
			phoneStr = pt
		}
	}
	if len(e.AddressEnc) > 0 {
		if pt, err := a.Crypto.Decrypt(e.AddressEnc, e.SchoolID); err == nil {
			addrStr = pt
		}
	}
	if len(e.BaseSalaryEnc) > 0 {
		if amt, err := a.Crypto.DecryptAmount(e.BaseSalaryEnc, e.SchoolID); err == nil {
			salaryIDR = &amt
		}
	}

	if !allowFull {
		nikStr = model.MaskNIK(nikStr)
		npwpStr = model.MaskNPWP(npwpStr)
		bankAccStr = model.MaskBankAccount(bankAccStr)
		phoneStr = model.MaskPhone(phoneStr)
		salaryIDR = nil // never show salary to non-privileged
	}

	return map[string]any{
		"id":                      e.ID,
		"school_id":               e.SchoolID,
		"user_id":                 e.UserID,
		"employee_no":             e.EmployeeNo,
		"full_name":               e.FullName,
		"email":                   e.Email,
		"phone":                   phoneStr,
		"nik":                     nikStr,
		"npwp":                    npwpStr,
		"ptkp_status":             e.PTKPStatus,
		"bank_code":               e.BankCode,
		"bank_account":            bankAccStr,
		"bank_account_name":       e.BankAccountName,
		"birth_date":              e.BirthDate,
		"gender":                  e.Gender,
		"address":                 addrStr,
		"hire_date":               e.HireDate,
		"exit_date":               e.ExitDate,
		"exit_reason":             e.ExitReason,
		"status":                  e.Status,
		"current_position_id":     e.CurrentPositionID,
		"current_position_name":   e.CurrentPositionName,
		"current_department_id":   e.CurrentDepartmentID,
		"current_department_name": e.CurrentDepartmentName,
		"current_employment_type": e.CurrentEmploymentType,
		"current_contract_end":    e.CurrentContractEnd,
		"current_supervisor_id":   e.CurrentSupervisorID,
		"current_supervisor_name": e.CurrentSupervisorName,
		"base_salary_idr":         salaryIDR,
		"created_at":              e.CreatedAt,
		"updated_at":              e.UpdatedAt,
	}
}
