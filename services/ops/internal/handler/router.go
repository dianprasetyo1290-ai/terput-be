package handler

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"skci/ops/internal/common"
	"skci/ops/internal/facility"
	"skci/ops/internal/inventory"
	"skci/ops/internal/procurement"
)

func (a *App) Router() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(common.SentryMiddleware())
	r.Use(middleware.Recoverer)
	r.Use(corsMiddleware(a.Config.CORSOrigins))

	r.Get("/healthz", a.Healthz)

	fac := a.Facility
	inv := a.Inventory
	proc := a.Procurement
	facilityOn := func(r *http.Request, schoolID string) bool { return a.featureOn(r, schoolID, "facility") }

	// Internal routes (X-Internal-Key), T9 §4.2.
	r.Group(func(in chi.Router) {
		in.Use(common.InternalAuth(a.Config.InternalKey))
		in.Post("/internal/approval-callbacks", a.ApprovalCallback)
		in.Get("/internal/facility/buildings", fac.InternalBuildings)
		in.Get("/internal/facility/rooms", fac.InternalRooms)
		in.Get("/internal/facility/rooms/{id}", fac.InternalRoom)
		in.Get("/internal/facility/availability", fac.InternalAvailability)
		in.Post("/internal/facility/bookings", fac.InternalCreateBooking(facilityOn))
		in.Post("/internal/facility/bookings/{id}/cancel", fac.InternalCancelBooking)
		in.Post("/internal/facility/work-orders", fac.InternalCreateWorkOrder(facilityOn))
		in.Get("/internal/facility/work-orders", fac.InternalWorkOrders)
	})

	// Facility (JWT + feature 'facility'), T9 §5.
	r.Route("/facility", func(api chi.Router) {
		api.Use(common.JWTMiddleware(a.Config.JWTSecret))
		api.Use(common.RequireAnyRole(common.StaffRoles...))
		api.Use(a.requireFeature("facility"))

		// All staff.
		api.Get("/buildings", fac.ListBuildings)
		api.Get("/rooms", fac.ListRooms)
		api.Get("/rooms/{id}", fac.GetRoom)
		api.Get("/rooms/{id}/calendar", fac.RoomCalendar)
		api.Post("/bookings", fac.CreateBooking)
		api.Get("/bookings/mine", fac.MyBookings)
		api.Post("/bookings/{id}/cancel", fac.CancelBooking) // own booking, or officer with reason
		api.Get("/capabilities", fac.Capabilities)
		api.Post("/work-orders", fac.CreateWorkOrder)
		api.Get("/work-orders/mine", fac.MyWorkOrders)
		api.Get("/work-orders", fac.ListWorkOrders)    // scoped by role inside
		api.Get("/work-orders/{id}", fac.GetWorkOrder) // officer/kepsek/assignee/reporter
		api.Get("/work-orders/{id}/photos/{photoId}", fac.WorkOrderPhoto)
		api.Post("/work-orders/{id}/status", fac.ChangeWorkOrderStatus) // assignee or officer
		api.Post("/work-orders/{id}/close", fac.CloseWorkOrder)         // officer or reporter
		api.Post("/work-orders/{id}/costs", fac.AddCost)                // assignee or officer
		api.Delete("/work-orders/{id}/costs/{costId}", fac.DeleteCost)

		// Readers: facility_officer, kepala_sekolah (read), admin, superadmin.
		api.Group(func(rd chi.Router) {
			rd.Use(common.RequireAnyRole(facility.Readers...))
			rd.Get("/bookings", fac.ListBookings)
		})

		// Officers: facility_officer, admin, superadmin.
		api.Group(func(of chi.Router) {
			of.Use(common.RequireAnyRole(facility.Officers...))
			of.Post("/buildings", fac.CreateBuilding)
			of.Put("/buildings/{id}", fac.UpdateBuilding)
			of.Delete("/buildings/{id}", fac.DeleteBuilding)
			of.Post("/floors", fac.CreateFloor)
			of.Put("/floors/{id}", fac.UpdateFloor)
			of.Delete("/floors/{id}", fac.DeleteFloor)
			of.Post("/rooms", fac.CreateRoom)
			of.Put("/rooms/{id}", fac.UpdateRoom)
			of.Delete("/rooms/{id}", fac.DeleteRoom)
			of.Put("/rooms/{id}/facilities", fac.SetFacilities)
			of.Get("/rooms/{id}/fixed-schedules", fac.ListFixedSchedules)
			of.Post("/rooms/{id}/fixed-schedules", fac.CreateFixedSchedule)
			of.Delete("/rooms/{id}/fixed-schedules/{scheduleId}", fac.DeleteFixedSchedule)
			of.Get("/technicians", fac.Technicians)
			of.Post("/work-orders/{id}/assign", fac.AssignWorkOrder)
			of.Post("/work-orders/{id}/cancel", fac.CancelWorkOrder)
			of.Post("/work-orders/{id}/purchase-request", fac.PurchaseRequest)
			of.Get("/maintenance-schedules", fac.ListSchedules)
			of.Post("/maintenance-schedules", fac.SaveSchedule)
			of.Put("/maintenance-schedules/{id}", fac.SaveSchedule)
			of.Delete("/maintenance-schedules/{id}", fac.DeleteSchedule)
		})
	})

	// Inventory (JWT + feature 'inventory'), T15.
	if inv != nil {
		r.Route("/inventory", func(api chi.Router) {
			api.Use(common.JWTMiddleware(a.Config.JWTSecret))
			api.Use(common.RequireAnyRole(common.StaffRoles...))
			api.Use(a.requireFeature("inventory"))

			// Read
			api.Get("/categories", inv.ListCategories)
			api.Get("/items", inv.ListItems)
			api.Get("/items/{id}", inv.GetItem)
			api.Get("/warehouses", inv.ListWarehouses)
			api.Get("/stock", inv.ListStock)
			api.Get("/stock/card", inv.GetStockCard)
			api.Get("/movements", inv.ListMovements)
			api.Get("/counts", inv.ListStockCounts)
			api.Get("/counts/{id}", inv.GetStockCount)
			api.Get("/counts/{id}/pdf", inv.GetStockCountReportPDF)

			// Officers write
			api.Group(func(of chi.Router) {
				of.Use(common.RequireAnyRole(inventory.Officers...))
				of.Post("/categories", inv.CreateCategory)
				of.Put("/categories/{id}", inv.UpdateCategory)
				of.Delete("/categories/{id}", inv.DeleteCategory)
				of.Post("/items", inv.CreateItem)
				of.Put("/items/{id}", inv.UpdateItem)
				of.Delete("/items/{id}", inv.DeleteItem)
				of.Post("/warehouses", inv.CreateWarehouse)
				of.Put("/warehouses/{id}", inv.UpdateWarehouse)
				of.Delete("/warehouses/{id}", inv.DeleteWarehouse)
				of.Post("/movements", inv.CreateMovement)
				of.Post("/transfers", inv.CreateTransfer)
				of.Post("/counts", inv.CreateStockCount)
				of.Put("/counts/{id}/lines", inv.UpdateCountLines)
				of.Post("/counts/{id}/submit", inv.SubmitStockCount)
				of.Post("/counts/{id}/adjust", inv.RequestAdjustment)
				of.Post("/counts/{id}/cancel", inv.CancelStockCount)
			})
		})
	}

	// Procurement (JWT + feature 'procurement'), T15.
	if proc != nil {
		r.Route("/procurement", func(api chi.Router) {
			api.Use(common.JWTMiddleware(a.Config.JWTSecret))
			api.Use(common.RequireAnyRole(common.StaffRoles...))
			api.Use(a.requireFeature("procurement"))

			// Staff PR
			api.Get("/purchase-requests", proc.ListPRs)
			api.Get("/purchase-requests/{id}", proc.GetPR)
			api.Post("/purchase-requests", proc.CreatePR)
			api.Post("/purchase-requests/{id}/submit", proc.SubmitPR)
			api.Post("/purchase-requests/{id}/cancel", proc.CancelPR)

			// Officers
			api.Group(func(of chi.Router) {
				of.Use(common.RequireAnyRole(procurement.Officers...))
				of.Get("/vendors", proc.ListVendors)
				of.Get("/vendors/{id}", proc.GetVendor)
				of.Post("/vendors", proc.CreateVendor)
				of.Post("/vendors/check-duplicate", proc.CheckDuplicate)
				of.Post("/vendors/{id}/bank-accounts", proc.AddBankAccount)
				of.Get("/vendors/{id}/evaluations", proc.ListEvaluations)
				of.Post("/vendors/{id}/evaluations", proc.CreateEvaluation)

				of.Get("/purchase-orders", proc.ListPOs)
				of.Get("/purchase-orders/{id}", proc.GetPO)
				of.Get("/purchase-orders/{id}/pdf", proc.GetPOReportPDF)
				of.Post("/purchase-orders", proc.CreatePO)
				of.Post("/purchase-orders/{id}/submit", proc.SubmitPO)
				of.Post("/purchase-orders/{id}/mark-sent", proc.MarkPOSent)
				of.Post("/purchase-orders/{id}/close", proc.ClosePO)
				of.Post("/purchase-orders/{id}/cancel", proc.CancelPO)

				of.Get("/receipts", proc.ListReceipts)
				of.Post("/receipts", proc.CreateReceipt)
				of.Post("/returns", proc.CreateReturn)
			})
		})
	}

	return r
}

func corsMiddleware(allowedOrigins []string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			if origin != "" {
				for _, o := range allowedOrigins {
					if o == "*" || o == origin {
						w.Header().Set("Access-Control-Allow-Origin", origin)
						w.Header().Set("Access-Control-Allow-Credentials", "true")
						w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
						w.Header().Set("Access-Control-Allow-Headers", "Accept, Authorization, Content-Type, X-Internal-Key, Idempotency-Key")
						break
					}
				}
			}
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
