// Package handler wires the ops HTTP router: shared middleware (CORS,
// module switch), the approval callback dispatcher and the domain packages
// (facility; later inventory, procurement, asset, … per T15–T25).
package handler

import (
	"context"
	"database/sql"
	"log"
	"net/http"

	"skci/ops/internal/asset"
	"skci/ops/internal/budget"
	"skci/ops/internal/client"
	"skci/ops/internal/common"
	"skci/ops/internal/config"
	"skci/ops/internal/facility"
	"skci/ops/internal/inventory"
	"skci/ops/internal/library"
	"skci/ops/internal/campus3d"
	"skci/ops/internal/payable"
	"skci/ops/internal/procurement"
	"skci/ops/internal/store"
	"skci/ops/internal/tax"
	"skci/ops/internal/transport"
)

type App struct {
	DB          *sql.DB
	Config      *config.Config
	Client      *client.Client
	Facility    *facility.Handler
	Inventory   *inventory.Handler
	Procurement *procurement.Handler
	Library     *library.Handler
	Transport   *transport.Handler
	Asset       *asset.Handler
	Budget      *budget.Handler
	Tax         *tax.Handler
	Payable     *payable.Handler
	Campus3D    *campus3d.Handler
}

func (a *App) Healthz(w http.ResponseWriter, r *http.Request) {
	common.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"}, nil)
}

// featureLabels names each ops module in the "dinonaktifkan" message.
var featureLabels = map[string]string{
	"facility":       "Fasilitas",
	"inventory":      "Inventaris",
	"procurement":    "Pengadaan",
	"library":        "Perpustakaan",
	"transport":      "Transportasi",
	"asset":          "Aset",
	"budget":         "Anggaran & Realisasi (RAPBS)",
	"tax_nonpayroll": "Pajak Non-Gaji & e-Faktur",
	"payable":        "Utang Usaha & Pembayaran",
	"campus3d":       "Kampus 3D",
}

// featureOn reports whether key is on for the school (fails closed).
func (a *App) featureOn(r *http.Request, schoolID, key string) bool {
	on, err := a.Client.FeatureEnabled(r.Context(), schoolID, key)
	if err != nil {
		log.Printf("feature lookup %s/%s: %v", schoolID, key, err)
		return false
	}
	return on
}

// requireFeature rejects JWT requests when the school's module is off
// (403 "Modul <label> dinonaktifkan untuk sekolah ini").
func (a *App) requireFeature(key string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			schoolID := common.EffectiveSchoolID(r)
			if schoolID == "" {
				common.WriteError(w, http.StatusBadRequest, "Pilih sekolah terlebih dahulu")
				return
			}
			if !a.featureOn(r, schoolID, key) {
				label := featureLabels[key]
				if label == "" {
					label = key
				}
				common.WriteError(w, http.StatusForbidden, "Modul "+label+" dinonaktifkan untuk sekolah ini")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// requireAccountingAndFeature enforces that feature 'accounting' is active (returns 409 if off)
// and that the specific domain feature key is active (returns 403 if off).
func (a *App) requireAccountingAndFeature(key string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			schoolID := common.EffectiveSchoolID(r)
			if schoolID == "" {
				common.WriteError(w, http.StatusBadRequest, "Pilih sekolah terlebih dahulu")
				return
			}
			if !a.featureOn(r, schoolID, "accounting") {
				common.WriteError(w, http.StatusConflict, "Modul akuntansi tidak aktif")
				return
			}
			if !a.featureOn(r, schoolID, key) {
				label := featureLabels[key]
				if label == "" {
					label = key
				}
				common.WriteError(w, http.StatusForbidden, "Modul "+label+" dinonaktifkan untuk sekolah ini")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// ApprovalCallback receives core's decision (T1 §3) and dispatches it by
// request_type. Idempotent: replays answer 200 without re-applying.
func (a *App) ApprovalCallback(w http.ResponseWriter, r *http.Request) {
	var d facility.ApprovalDecision
	if !common.Decode(w, r, &d) {
		return
	}
	if d.SourceID == "" || (d.Decision != "approved" && d.Decision != "rejected") {
		common.WriteError(w, http.StatusBadRequest, "source_id dan decision wajib valid")
		return
	}
	schoolID := common.EffectiveSchoolID(r)

	switch d.RequestType {
	case "room_booking":
		b, err := a.Facility.ApplyApprovalDecision(r.Context(), d)
		if err == facility.ErrNotFound {
			common.WriteError(w, http.StatusNotFound, "Pemesanan tidak ditemukan")
			return
		}
		if err != nil {
			log.Printf("approval callback %s: %v", d.SourceID, err)
			common.WriteError(w, http.StatusInternalServerError, "Gagal memproses keputusan approval")
			return
		}
		common.WriteJSON(w, http.StatusOK, map[string]string{"id": b.ID, "status": b.Status}, nil)

	case "stock_adjustment":
		if a.Inventory == nil {
			common.WriteError(w, http.StatusInternalServerError, "Inventory handler tidak aktif")
			return
		}
		invDec := inventory.ApprovalDecision{
			ApprovalID:  d.ApprovalID,
			SourceID:    d.SourceID,
			RequestType: d.RequestType,
			Decision:    d.Decision,
			DecidedBy:   d.DecidedBy,
			Comment:     d.Comment,
		}
		if err := a.Inventory.ApplyApprovalDecision(r.Context(), invDec); err != nil {
			log.Printf("stock adjustment approval callback %s: %v", d.SourceID, err)
			common.WriteError(w, http.StatusInternalServerError, "Gagal memproses penyesuaian stok: "+err.Error())
			return
		}
		common.WriteJSON(w, http.StatusOK, map[string]string{"id": d.SourceID, "decision": d.Decision}, nil)

	case "purchase_request", "purchase_order", "vendor_bank_change":
		if a.Procurement == nil {
			common.WriteError(w, http.StatusInternalServerError, "Procurement handler tidak aktif")
			return
		}
		prcDec := procurement.ApprovalDecision{
			ApprovalID:  d.ApprovalID,
			SourceID:    d.SourceID,
			RequestType: d.RequestType,
			Decision:    d.Decision,
			DecidedBy:   d.DecidedBy,
			Comment:     d.Comment,
		}
		if err := a.Procurement.ApplyApprovalDecision(r.Context(), prcDec); err != nil {
			log.Printf("procurement approval callback %s: %v", d.SourceID, err)
			common.WriteError(w, http.StatusInternalServerError, "Gagal memproses approval pengadaan: "+err.Error())
			return
		}
		common.WriteJSON(w, http.StatusOK, map[string]string{"id": d.SourceID, "decision": d.Decision}, nil)

	case "asset_disposal", "asset_transfer":
		if a.Asset == nil {
			common.WriteError(w, http.StatusInternalServerError, "Asset handler tidak aktif")
			return
		}
		if err := a.Asset.ApplyApprovalDecision(r.Context(), d.RequestType, d.SourceID, d.Decision); err != nil {
			log.Printf("asset approval callback %s: %v", d.SourceID, err)
			common.WriteError(w, http.StatusInternalServerError, "Gagal memproses approval aset: "+err.Error())
			return
		}
		common.WriteJSON(w, http.StatusOK, map[string]string{"id": d.SourceID, "decision": d.Decision}, nil)

	case "budget":
		if a.Budget == nil {
			common.WriteError(w, http.StatusInternalServerError, "Budget handler tidak aktif")
			return
		}
		if d.Decision == "approved" {
			_ = a.Budget.Store.ApproveBudget(r.Context(), schoolID, d.SourceID)
		}
		common.WriteJSON(w, http.StatusOK, map[string]string{"id": d.SourceID, "decision": d.Decision}, nil)

	case "vendor_invoice_override":
		if a.Payable == nil {
			common.WriteError(w, http.StatusInternalServerError, "Payable handler tidak aktif")
			return
		}
		if err := a.Payable.Store.ApplyOverrideDecision(r.Context(), schoolID, d.SourceID, d.Decision); err != nil {
			log.Printf("vendor invoice override callback %s: %v", d.SourceID, err)
			common.WriteError(w, http.StatusInternalServerError, "Gagal memproses dispensasi faktur: "+err.Error())
			return
		}
		common.WriteJSON(w, http.StatusOK, map[string]string{"id": d.SourceID, "decision": d.Decision}, nil)

	case "vendor_payment":
		if a.Payable == nil {
			common.WriteError(w, http.StatusInternalServerError, "Payable handler tidak aktif")
			return
		}
		if d.Decision == "approved" {
			_ = a.Payable.Store.ApprovePaymentSchedule(r.Context(), schoolID, d.SourceID)
		}
		common.WriteJSON(w, http.StatusOK, map[string]string{"id": d.SourceID, "decision": d.Decision}, nil)

	default:
		common.WriteError(w, http.StatusBadRequest, "request_type tidak dikenal: "+d.RequestType)
	}
}

// ForwardAudit pushes pending outbox rows to core POST /internal/audit.
func (a *App) ForwardAudit(ctx context.Context, db *sql.DB) error {
	rows, err := store.PendingAudit(ctx, db, 200)
	if err != nil {
		return err
	}
	for _, e := range rows {
		ev := client.AuditEvent{SchoolID: e.SchoolID, Service: "ops", Module: e.Module, EntityType: e.EntityType,
			EntityID: e.EntityID, Action: e.Action, ActorUserID: e.ActorUserID, ActorRole: e.ActorRole,
			Before: e.BeforeRaw, After: e.AfterRaw, Reason: e.Reason, CorrelationID: e.CorrelationID}
		if err := a.Client.PostAudit(ctx, ev); err != nil {
			return err
		}
		if err := store.MarkAuditForwarded(ctx, db, e.ID); err != nil {
			return err
		}
	}
	return nil
}
