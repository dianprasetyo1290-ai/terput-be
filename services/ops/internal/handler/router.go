package handler

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"skci/ops/internal/asset"
	"skci/ops/internal/budget"
	"skci/ops/internal/common"
	"skci/ops/internal/facility"
	"skci/ops/internal/inventory"
	"skci/ops/internal/library"
	"skci/ops/internal/payable"
	"skci/ops/internal/procurement"
	"skci/ops/internal/tax"
	"skci/ops/internal/transport"
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
	lib := a.Library
	tr := a.Transport
	ast := a.Asset
	bgt := a.Budget
	tx := a.Tax
	pay := a.Payable
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
		if lib != nil {
			in.Post("/internal/library/fines/{id}/bill", lib.InternalBillFine)
		}
		if tr != nil {
			in.Post("/internal/transport/charges/{id}/retry", tr.InternalRetryCharge)
		}
		if ast != nil {
			in.Post("/internal/assets/events/{id}/retry", ast.InternalRetryEvent)
		}
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

	// Library (JWT + feature 'library'), T18.
	if lib != nil {
		// Self-service for any authenticated member (siswa, guru, staff, etc.)
		r.Route("/me/library", func(api chi.Router) {
			api.Use(common.JWTMiddleware(a.Config.JWTSecret))
			api.Use(a.requireFeature("library"))

			api.Get("/", lib.MyLibrary)
			api.Post("/loans/{id}/renew", lib.MyRenewLoan)
			api.Post("/reservations", lib.MyReserveTitle)
			api.Post("/reservations/{id}/cancel", lib.MyCancelReservation)
		})

		r.Route("/library", func(api chi.Router) {
			api.Use(common.JWTMiddleware(a.Config.JWTSecret))
			api.Use(a.requireFeature("library"))

			// OPAC: accessible to any authenticated user
			api.Get("/opac", lib.OpacSearch)
			api.Get("/opac/{id}", lib.OpacDetail)

			// Reports: readers (pustakawan, kepsek, admin, superadmin)
			api.Group(func(rd chi.Router) {
				rd.Use(common.RequireAnyRole(library.ReportReaders...))
				rd.Get("/reports/loans", lib.ReportLoans)
				rd.Get("/reports/popular", lib.ReportPopular)
				rd.Get("/reports/overdue", lib.ReportOverdue)
				rd.Get("/reports/visits", lib.ReportVisits)
			})

			// Officers: pustakawan, admin, superadmin
			api.Group(func(of chi.Router) {
				of.Use(common.RequireAnyRole(library.Officers...))

				// Settings & Master Data
				of.Get("/settings", lib.GetSettings)
				of.Put("/settings", lib.UpdateSettings)
				of.Get("/member-categories", lib.ListMemberCategories)
				of.Post("/member-categories", lib.CreateMemberCategory)
				of.Put("/member-categories/{id}", lib.UpdateMemberCategory)
				of.Delete("/member-categories/{id}", lib.DeleteMemberCategory)
				of.Get("/fine-rates", lib.ListFineRates)
				of.Post("/fine-rates", lib.CreateFineRate)
				of.Delete("/fine-rates/{id}", lib.DeleteFineRate)
				of.Get("/collection-categories", lib.ListCollectionCategories)
				of.Post("/collection-categories", lib.CreateCollectionCategory)
				of.Put("/collection-categories/{id}", lib.UpdateCollectionCategory)
				of.Delete("/collection-categories/{id}", lib.DeleteCollectionCategory)

				// Catalog & Copies
				of.Get("/titles", lib.ListTitles)
				of.Get("/titles/{id}", lib.GetTitle)
				of.Post("/titles", lib.CreateTitle)
				of.Put("/titles/{id}", lib.UpdateTitle)
				of.Delete("/titles/{id}", lib.DeleteTitle)
				of.Post("/titles/{id}/copies", lib.AddCopy)
				of.Put("/copies/{id}", lib.UpdateCopy)
				of.Delete("/copies/{id}", lib.DeleteCopy)
				of.Get("/copies/{id}/label", lib.GetCopyLabel)

				// Members
				of.Get("/members", lib.ListMembers)
				of.Get("/members/{id}", lib.GetMember)
				of.Put("/members/{id}", lib.UpdateMember)
				of.Post("/members/sync", lib.SyncMembers)
				of.Get("/members/{id}/card", lib.GetMemberCard)

				// Circulation
				of.Post("/loans", lib.CreateLoan)
				of.Post("/loans/return", lib.ReturnCopy)
				of.Post("/loans/{id}/renew", lib.RenewLoan)
				of.Post("/loans/{id}/lost", lib.ReportLost)
				of.Post("/visits", lib.RecordVisit)

				// Fines
				of.Get("/fines", lib.ListFines)
				of.Post("/fines/{id}/pay", lib.PayFine)
				of.Post("/fines/{id}/waive", lib.WaiveFine)
			})
		})
	}

	// Transport (JWT + feature 'transport'), T19.
	if tr != nil {
		// Parent Portal: /me/transport (orang_tua)
		r.Route("/me/transport", func(api chi.Router) {
			api.Use(common.JWTMiddleware(a.Config.JWTSecret))
			api.Use(common.RequireAnyRole(transport.Parents...))
			api.Use(a.requireFeature("transport"))

			api.Get("/children", tr.MyChildren)
			api.Get("/children/{studentId}/events", tr.MyChildEvents)
			api.Post("/absences", tr.CreateMyAbsence)
			api.Delete("/absences/{id}", tr.CancelMyAbsence)
		})

		r.Route("/transport", func(api chi.Router) {
			api.Use(common.JWTMiddleware(a.Config.JWTSecret))
			api.Use(a.requireFeature("transport"))

			// Crew (mobile-first trip execution)
			api.Group(func(cr chi.Router) {
				cr.Use(common.RequireAnyRole(transport.CrewRoles...))
				cr.Get("/my-trips", tr.GetMyTrips)
				cr.Get("/trips/{id}/manifest", tr.GetTripManifest)
				cr.Post("/trips/{id}/events", tr.RecordTripEvent)
				cr.Post("/trips/{id}/start", tr.StartTrip)
				cr.Post("/trips/{id}/finish", tr.FinishTrip)
			})

			// Officers: transport_officer, admin, superadmin
			api.Group(func(of chi.Router) {
				of.Use(common.RequireAnyRole(transport.Officers...))

				// Settings & Master Data
				of.Get("/settings", tr.GetSettings)
				of.Put("/settings", tr.UpdateSettings)

				of.Get("/zones", tr.ListZones)
				of.Post("/zones", tr.CreateZone)
				of.Put("/zones/{id}", tr.UpdateZone)
				of.Delete("/zones/{id}", tr.DeleteZone)

				of.Get("/routes", tr.ListRoutes)
				of.Get("/routes/{id}", tr.GetRoute)
				of.Post("/routes", tr.CreateRoute)
				of.Put("/routes/{id}", tr.UpdateRoute)
				of.Delete("/routes/{id}", tr.DeleteRoute)
				of.Put("/routes/{id}/stops", tr.SetStops)
				of.Get("/routes/{id}/roster", tr.GetRouteRoster)

				of.Get("/vehicles", tr.ListVehicles)
				of.Post("/vehicles", tr.CreateVehicle)
				of.Put("/vehicles/{id}", tr.UpdateVehicle)
				of.Delete("/vehicles/{id}", tr.DeleteVehicle)
				of.Get("/vehicles/{id}/documents", tr.ListDocuments)
				of.Post("/vehicles/{id}/documents", tr.AddDocument)
				of.Delete("/vehicles/{id}/documents/{docId}", tr.DeleteDocument)
				of.Get("/expiring", tr.ListExpiring)

				of.Get("/crew", tr.ListCrew)
				of.Post("/crew", tr.CreateCrew)
				of.Put("/crew/{id}", tr.UpdateCrew)
				of.Delete("/crew/{id}", tr.DeleteCrew)
				of.Post("/routes/{id}/assignments", tr.AssignRoute)
				of.Get("/employees", tr.ListEmployees)

				of.Get("/allocations", tr.ListAllocations)
				of.Post("/allocations", tr.CreateAllocations)
				of.Post("/allocations/{id}/end", tr.EndAllocation)
				of.Post("/allocations/{id}/move", tr.MoveAllocation)

				of.Get("/trips", tr.ListTrips)
				of.Get("/fares", tr.ListFares)
				of.Post("/fares", tr.CreateFare)
				of.Delete("/fares/{id}", tr.DeleteFare)
			})

			// Finance: admin, finance_manager
			api.Group(func(fn chi.Router) {
				fn.Use(common.RequireAnyRole(transport.FinanceOfficers...))
				fn.Get("/billing/preview", tr.PreviewBilling)
				fn.Post("/billing/issue", tr.IssueBilling)
				fn.Get("/billing", tr.ListCharges)
			})
		})
	}

	// Assets (JWT + feature 'asset'), T16 §5.
	if ast != nil {
		r.Route("/assets", func(api chi.Router) {
			api.Use(common.JWTMiddleware(a.Config.JWTSecret))
			api.Use(common.RequireAnyRole(common.StaffRoles...))
			api.Use(a.requireFeature("asset"))

			// QR Scan: all logged-in staff can scan (role-aware view)
			api.Get("/scan/{qr_token}", ast.ScanAsset)

			// Photo download
			api.Get("/{id}/photo", ast.GetPhoto)

			// Summary by room for 3D Campus (T25)
			api.Get("/summary-by-room", ast.SummaryByRoom)

			// Readers: AssetReaders
			api.Group(func(rd chi.Router) {
				rd.Use(common.RequireAnyRole(asset.AssetReaders...))
				rd.Get("/", ast.ListAssets)
				rd.Get("/{id}", ast.GetAsset)
				rd.Get("/categories", ast.ListCategories)
				rd.Get("/categories/{id}", ast.GetCategory)
				rd.Get("/settings", ast.GetSettings)
				rd.Get("/{id}/movements", ast.ListMovements)
				rd.Get("/disposals", ast.ListDisposals)
				rd.Get("/{id}/depreciation", ast.GetDepreciation)
			})

			// Officers & Writers: AssetWriters
			api.Group(func(wr chi.Router) {
				wr.Use(common.RequireAnyRole(asset.AssetWriters...))
				wr.Post("/", ast.CreateAsset)
				wr.Put("/{id}", ast.UpdateAsset)
				wr.Post("/{id}/activate", ast.ActivateAsset)
				wr.Post("/{id}/photo", ast.UploadPhoto)
				wr.Get("/{id}/label", ast.GetLabel)
				wr.Get("/labels", ast.GetLabels)
				wr.Post("/categories", ast.CreateCategory)
				wr.Put("/categories/{id}", ast.UpdateCategory)
				wr.Delete("/categories/{id}", ast.DeleteCategory)
				wr.Put("/settings", ast.UpdateSettings)
				wr.Post("/{id}/movements", ast.CreateMovement)
				wr.Post("/{id}/disposals", ast.CreateDisposal)
				wr.Post("/disposals/{id}/cancel", ast.CancelDisposal)
			})
		})
	}

	// Budget / RAPBS & Realisasi (Menu 109, feature 'budget' + requires 'accounting')
	if bgt != nil {
		r.Route("/budget", func(api chi.Router) {
			api.Use(common.JWTMiddleware(a.Config.JWTSecret))
			api.Use(a.requireAccountingAndFeature("budget"))

			// Readers: FinanceRoles
			api.Group(func(rd chi.Router) {
				rd.Use(common.RequireAnyRole(budget.FinanceRoles...))
				rd.Get("/settings", bgt.GetSettings)
				rd.Get("/", bgt.ListBudgets)
				rd.Get("/{id}", bgt.GetBudget)
				rd.Get("/{id}/lines", bgt.ListBudgetLines)
				rd.Get("/{id}/report", bgt.GetBudgetReport)
				rd.Get("/alerts", bgt.ListAlerts)
			})

			// Writers: BudgetWriters
			api.Group(func(wr chi.Router) {
				wr.Use(common.RequireAnyRole(budget.BudgetWriters...))
				wr.Put("/settings", bgt.UpdateSettings)
				wr.Post("/", bgt.CreateBudget)
				wr.Put("/{id}", bgt.UpdateBudget)
				wr.Post("/{id}/submit", bgt.SubmitBudget)
				wr.Post("/{id}/lines", bgt.CreateBudgetLine)
				wr.Put("/{id}/lines/{lineId}", bgt.UpdateBudgetLine)
				wr.Delete("/{id}/lines/{lineId}", bgt.DeleteBudgetLine)
				wr.Post("/{id}/sync-actuals", bgt.SyncActuals)
				wr.Post("/alerts/{id}/ack", bgt.AcknowledgeAlert)
			})
		})
	}

	// Tax Non-Payroll & e-Faktur (Menu 110, feature 'tax_nonpayroll' + requires 'accounting')
	if tx != nil {
		r.Route("/tax", func(api chi.Router) {
			api.Use(common.JWTMiddleware(a.Config.JWTSecret))
			api.Use(a.requireAccountingAndFeature("tax_nonpayroll"))

			// Readers: TaxReaders
			api.Group(func(rd chi.Router) {
				rd.Use(common.RequireAnyRole(tax.TaxReaders...))
				rd.Get("/settings", tx.GetSettings)
				rd.Get("/types", tx.ListTaxTypes)
				rd.Get("/types/{id}", tx.GetTaxType)
				rd.Get("/slips", tx.ListWithholdingSlips)
				rd.Get("/slips/{id}", tx.GetWithholdingSlip)
				rd.Get("/input-vat", tx.ListInputVAT)
				rd.Get("/output-invoices", tx.ListOutputTaxInvoices)
			})

			// Writers: TaxWriters
			api.Group(func(wr chi.Router) {
				wr.Use(common.RequireAnyRole(tax.TaxWriters...))
				wr.Put("/settings", tx.UpdateSettings)
				wr.Post("/types", tx.CreateTaxType)
				wr.Put("/types/{id}", tx.UpdateTaxType)
				wr.Post("/slips", tx.CreateWithholdingSlip)
				wr.Post("/slips/{id}/issue", tx.IssueWithholdingSlip)
				wr.Post("/input-vat", tx.CreateInputVAT)
				wr.Put("/input-vat/{id}/status", tx.UpdateInputVATStatus)
				wr.Post("/output-invoices", tx.CreateOutputTaxInvoice)
				wr.Post("/export", tx.ExportTax)
			})
		})
	}

	// Accounts Payable & 3-Way Match (Menu 111, feature 'payable' + requires 'accounting')
	if pay != nil {
		r.Route("/payable", func(api chi.Router) {
			api.Use(common.JWTMiddleware(a.Config.JWTSecret))
			api.Use(a.requireAccountingAndFeature("payable"))

			// Readers: PayableReaders
			api.Group(func(rd chi.Router) {
				rd.Use(common.RequireAnyRole(payable.PayableReaders...))
				rd.Get("/settings", pay.GetSettings)
				rd.Get("/invoices", pay.ListInvoices)
				rd.Get("/invoices/{id}", pay.GetInvoice)
				rd.Get("/schedules", pay.ListSchedules)
				rd.Get("/aging", pay.GetAgingReport)
				rd.Get("/journals", pay.ListJournals)
			})

			// Writers: PayableWriters
			api.Group(func(wr chi.Router) {
				wr.Use(common.RequireAnyRole(payable.PayableWriters...))
				wr.Put("/settings", pay.UpdateSettings)
				wr.Post("/invoices", pay.CreateInvoice)
				wr.Post("/invoices/{id}/match", pay.RunMatch)
				wr.Post("/invoices/{id}/request-override", pay.RequestOverride)
				wr.Post("/schedules", pay.CreateSchedule)
				wr.Post("/schedules/{id}/submit", pay.SubmitSchedule)
				wr.Post("/schedules/{id}/execute", pay.ExecuteSchedule)
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
