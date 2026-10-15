package handler

import (
	"encoding/json"
	"net/http"

	"skci/quality/internal/client"
	"skci/quality/internal/common"
	"skci/quality/internal/config"
	"skci/quality/internal/govreport"
	"skci/quality/internal/qa"
	"skci/quality/internal/storage"
)

type App struct {
	Config     *config.Config
	QAStore    *qa.Store
	GovStore   *govreport.Store
	QAHandler  *qa.Handler
	GovHandler *govreport.Handler
	Clients    *client.Clients
	Storage    *storage.LocalStorage
}

func NewApp(cfg *config.Config, qaStore *qa.Store, govStore *govreport.Store, qaHandler *qa.Handler, govHandler *govreport.Handler, clients *client.Clients, st *storage.LocalStorage) *App {
	return &App{
		Config:     cfg,
		QAStore:    qaStore,
		GovStore:   govStore,
		QAHandler:  qaHandler,
		GovHandler: govHandler,
		Clients:    clients,
		Storage:    st,
	}
}

// InternalApprovalCallback handles callbacks from Core's Approval Center (T1 §3).
func (a *App) InternalApprovalCallback(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		ApprovalID  string `json:"approval_id"`
		SourceID    string `json:"source_id"`
		RequestType string `json:"request_type"`
		Decision    string `json:"decision"`
		DecidedBy   string `json:"decided_by"`
		Comment     string `json:"comment"`
		DecidedAt   string `json:"decided_at"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		common.WriteError(w, http.StatusBadRequest, "invalid callback payload")
		return
	}

	if payload.RequestType == "qa_self_eval_finalize" || payload.RequestType == "qa_self_eval_reopen" {
		if err := a.QAStore.HandleApprovalCallback(r.Context(), payload.ApprovalID, payload.RequestType, payload.Decision, payload.DecidedBy, payload.Comment); err != nil {
			common.WriteError(w, http.StatusInternalServerError, "failed to process approval callback: "+err.Error())
			return
		}
	} else if payload.RequestType == "regulatory_report" {
		id, schoolID, err := a.GovStore.ApplyApprovalDecision(r.Context(), payload.ApprovalID, payload.Decision)
		if err != nil {
			common.WriteError(w, http.StatusInternalServerError, "failed to process approval callback")
			return
		}
		if id != "" {
			_ = a.QAStore.RecordAudit(r.Context(), schoolID, "regulatory_report", id, "approval_"+payload.Decision, payload.DecidedBy, "", nil, map[string]string{"decision": payload.Decision}, payload.Comment)
		}
	}

	common.WriteJSON(w, http.StatusOK, map[string]string{"message": "callback processed successfully"})
}
