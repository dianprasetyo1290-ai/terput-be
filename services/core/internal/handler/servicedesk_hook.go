package handler

import (
	"net/http"

	"skci/core/internal/common"
)

// serviceDeskGuard blocks menu 81 status/result changes on a request that is
// handled as a service desk ticket (BR-SD-01). It keys on the ticket's
// existence, not the feature switch, so disabling the feature later never
// leaves an orphaned ticket. Returns true when the response was written.
func (h *Handler) serviceDeskGuard(w http.ResponseWriter, r *http.Request, schoolID, srID string) bool {
	if h.ServiceDesk == nil {
		return false
	}
	number, ok := h.ServiceDesk.HasTicketForServiceRequest(r.Context(), schoolID, srID)
	if !ok {
		return false
	}
	common.WriteError(w, http.StatusConflict, "Permintaan ini dikelola di Service Desk ("+number+")")
	return true
}
