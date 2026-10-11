package handler

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"

	"skci/core/internal/common"
	"skci/core/internal/config"
)

// NewRouter sets up all HTTP routes for core-service.
func NewRouter(h *Handler, cfg config.Config) http.Handler {
	r := chi.NewRouter()

	r.Use(middleware.RequestID, middleware.RealIP, middleware.Logger, common.SentryMiddleware(), middleware.Recoverer)
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   cfg.CORSOrigins,
		AllowedMethods:   []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-Internal-Key", "Idempotency-Key"},
		AllowCredentials: true,
	}))

	// Health check (no auth, no envelope)
	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok","service":"core"}`))
	})

	// ---------------------------------------------------------------------
	// Service-to-service internal calls (authenticated by X-Internal-Key)
	// ---------------------------------------------------------------------
	r.Group(func(ir chi.Router) {
		ir.Use(common.InternalAuth(cfg.InternalKey))

		ir.Post("/internal/approvals", h.InternalCreateApproval)
		ir.Get("/internal/approvals", h.InternalGetApprovalBySource)
		ir.Post("/internal/approvals/{id}/cancel", h.InternalCancelApproval)
		ir.Post("/internal/exceptions", h.InternalCreateException)
		ir.Post("/internal/audit", h.InternalCreateAudit)
	})

	// ---------------------------------------------------------------------
	// Authenticated JWT routes (admin, superadmin, approvers, auditors)
	// ---------------------------------------------------------------------
	r.Group(func(pr chi.Router) {
		pr.Use(common.JWTMiddleware(cfg.JWTSecret))

		// Approval Flows CRUD (superadmin, admin)
		pr.Get("/approval-flows", h.ListFlows)
		pr.Post("/approval-flows", h.CreateFlow)
		pr.Get("/approval-flows/{id}", h.GetFlow)
		pr.Put("/approval-flows/{id}", h.UpdateFlow)
		pr.Delete("/approval-flows/{id}", h.DeleteFlow)
		pr.Get("/approval-request-types", h.GetRequestTypes)

		// Approvals Inbox, Mine, Detail, Decisions
		pr.Get("/approvals/inbox", h.ListInbox)
		pr.Get("/approvals/mine", h.ListMine)
		pr.Get("/approvals/{id}", h.GetApproval)
		pr.Post("/approvals/{id}/approve", h.Approve)
		pr.Post("/approvals/{id}/reject", h.Reject)

		// Approval Delegations
		pr.Get("/approval-delegations", h.ListDelegations)
		pr.Post("/approval-delegations", h.CreateDelegation)
		pr.Delete("/approval-delegations/{id}", h.RevokeDelegation)

		// Integration Exception Queue
		pr.Get("/exceptions", h.ListExceptions)
		pr.Post("/exceptions/{id}/retry", h.RetryException)
		pr.Post("/exceptions/{id}/assign", h.AssignException)
		pr.Post("/exceptions/{id}/resolve", h.ResolveException)

		// Central Audit Trail
		pr.Get("/audit", h.ListAudit)

		// Parent Service Requests (Menu 81)
		pr.Get("/service-request-categories", h.ListCategories)
		pr.Post("/service-request-categories", h.CreateCategory)
		pr.Get("/service-request-categories/{id}", h.GetCategory)
		pr.Put("/service-request-categories/{id}", h.UpdateCategory)
		pr.Delete("/service-request-categories/{id}", h.DeleteCategory)

		pr.Post("/parent/service-requests", h.CreateParentServiceRequest)
		pr.Get("/parent/service-requests", h.ListParentServiceRequests)
		pr.Get("/parent/service-requests/{id}", h.GetServiceRequestDetail)

		pr.Get("/service-requests", h.ListHandlerServiceRequests)
		pr.Get("/service-requests/can-handle", h.CanUserHandle)
		pr.Get("/service-requests/{id}", h.GetServiceRequestDetail)
		pr.Post("/service-requests/{id}/status", h.UpdateServiceRequestStatus)
		pr.Post("/service-requests/{id}/result", h.UpdateServiceRequestResult)
	})

	return r
}
