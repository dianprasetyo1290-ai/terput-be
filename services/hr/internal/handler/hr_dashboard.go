package handler

import (
	"net/http"
	"time"

	"skci/hr/internal/common"
	"skci/hr/internal/store"
)

// HRDashboard: GET /dashboard (hr_admin, hr_manager, superadmin) — live
// counts for the SDM landing page, in the school's configured timezone.
func (a *App) HRDashboard(w http.ResponseWriter, r *http.Request) {
	schoolID := a.effectiveSchoolID(r)
	loc := time.FixedZone("WIB", 7*3600)
	if settings, err := store.GetHRSettings(r.Context(), a.DB, schoolID); err == nil && settings != nil && settings.Timezone != "" {
		if l, err := time.LoadLocation(settings.Timezone); err == nil {
			loc = l
		}
	}
	d, err := store.GetHRDashboard(r.Context(), a.DB, schoolID, time.Now().In(loc))
	if err != nil {
		common.InternalError(w, "Gagal memuat dashboard SDM", err)
		return
	}
	common.JSON(w, http.StatusOK, d)
}
