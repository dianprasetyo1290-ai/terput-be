package govreport

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/xuri/excelize/v2"

	"skci/quality/internal/client"
	"skci/quality/internal/common"
	"skci/quality/internal/storage"
)

type Handler struct {
	store       *Store
	clients     *client.Clients
	storage     *storage.LocalStorage
	hashKey     string
	maxRows     int
}

func NewHandler(s *Store, c *client.Clients, st *storage.LocalStorage, hashKey string, maxRows int) *Handler {
	return &Handler{
		store:   s,
		clients: c,
		storage: st,
		hashKey: hashKey,
		maxRows: maxRows,
	}
}

func (h *Handler) effectiveSchoolID(r *http.Request) string {
	claims := common.FromContext(r.Context())
	if claims == nil {
		return r.URL.Query().Get("school_id")
	}
	if claims.ActiveRole == common.RoleSuperadmin {
		if q := r.URL.Query().Get("school_id"); q != "" {
			return q
		}
	}
	return claims.SchoolID
}

// ------------------- Dapodik Mappings -------------------

func (h *Handler) ListMappings(w http.ResponseWriter, r *http.Request) {
	schoolID := h.effectiveSchoolID(r)
	entity := r.URL.Query().Get("entity")

	list, err := h.store.ListMappings(r.Context(), schoolID, entity)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "failed to list mappings: "+err.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, list)
}

func (h *Handler) UpsertMapping(w http.ResponseWriter, r *http.Request) {
	claims := common.FromContext(r.Context())
	schoolID := h.effectiveSchoolID(r)

	var req FieldMapping
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		common.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	req.SchoolID = schoolID
	if req.Entity != "student" && req.Entity != "rombel" && req.Entity != "ptk" {
		common.WriteError(w, http.StatusBadRequest, "entity must be student, rombel, or ptk")
		return
	}

	saved, err := h.store.UpsertMapping(r.Context(), req)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "failed to save mapping: "+err.Error())
		return
	}
	_ = h.store.RecordAudit(r.Context(), schoolID, "dapodik_mapping", saved.ID, "upsert", claims.UserID, claims.ActiveRole, nil, saved, "Pemetaan Dapodik disimpan")
	common.WriteJSON(w, http.StatusOK, saved)
}

func (h *Handler) DeleteMapping(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	claims := common.FromContext(r.Context())
	schoolID := h.effectiveSchoolID(r)

	if err := h.store.DeleteMapping(r.Context(), schoolID, id); err != nil {
		common.WriteError(w, http.StatusInternalServerError, "failed to delete mapping: "+err.Error())
		return
	}
	_ = h.store.RecordAudit(r.Context(), schoolID, "dapodik_mapping", id, "delete", claims.UserID, claims.ActiveRole, nil, nil, "Pemetaan dihapus")
	common.WriteJSON(w, http.StatusOK, map[string]string{"message": "Pemetaan berhasil dihapus"})
}

// ------------------- Dapodik Imports & Comparison -------------------

func (h *Handler) ListImports(w http.ResponseWriter, r *http.Request) {
	schoolID := h.effectiveSchoolID(r)
	entity := r.URL.Query().Get("entity")

	list, err := h.store.ListImports(r.Context(), schoolID, entity)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "failed to list imports: "+err.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, list)
}

func (h *Handler) ImportDapodikFile(w http.ResponseWriter, r *http.Request) {
	claims := common.FromContext(r.Context())
	schoolID := h.effectiveSchoolID(r)

	if err := r.ParseMultipartForm(25 << 20); err != nil {
		common.WriteError(w, http.StatusBadRequest, "failed to parse multipart form")
		return
	}

	entity := r.FormValue("entity")
	if entity != "student" && entity != "rombel" && entity != "ptk" {
		common.WriteError(w, http.StatusBadRequest, "entity must be student, rombel, or ptk")
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		common.WriteError(w, http.StatusBadRequest, "missing file in form")
		return
	}
	defer file.Close()

	// Parse file into rows
	var rows []Record
	ext := strings.ToLower(header.Filename)

	if strings.HasSuffix(ext, ".csv") {
		reader := csv.NewReader(file)
		allRecords, err := reader.ReadAll()
		if err != nil || len(allRecords) < 2 {
			common.WriteError(w, http.StatusBadRequest, "failed to read CSV or file is empty")
			return
		}
		headers := allRecords[0]
		for i, line := range allRecords[1:] {
			if len(rows) >= h.maxRows {
				break
			}
			rec := Record{"_row_id": fmt.Sprintf("row-%d", i+1)}
			for colIdx, colName := range headers {
				if colIdx < len(line) {
					rec[strings.TrimSpace(colName)] = line[colIdx]
				}
			}
			rows = append(rows, rec)
		}
	} else {
		// XLSX
		f, err := excelize.OpenReader(file)
		if err != nil {
			common.WriteError(w, http.StatusBadRequest, "failed to read XLSX file: "+err.Error())
			return
		}
		defer f.Close()

		sheetName := f.GetSheetName(0)
		excelRows, err := f.GetRows(sheetName)
		if err != nil || len(excelRows) < 2 {
			common.WriteError(w, http.StatusBadRequest, "failed to read excel rows or sheet empty")
			return
		}
		headers := excelRows[0]
		for i, line := range excelRows[1:] {
			if len(rows) >= h.maxRows {
				break
			}
			rec := Record{"_row_id": fmt.Sprintf("row-%d", i+1)}
			for colIdx, colName := range headers {
				if colIdx < len(line) {
					rec[strings.TrimSpace(colName)] = line[colIdx]
				}
			}
			rows = append(rows, rec)
		}
	}

	if len(rows) == 0 {
		common.WriteError(w, http.StatusBadRequest, "no records found in file")
		return
	}

	// Fetch field mappings
	mappings, err := h.store.ListMappings(r.Context(), schoolID, entity)
	if err != nil || len(mappings) == 0 {
		// Provide default mappings if none defined yet
		if entity == "student" {
			mappings = []FieldMapping{
				{SchoolID: schoolID, Entity: "student", DapodikColumn: "Nama Siswa", SekociField: "name", IsMatchKey: true, MatchPriority: 1, Normalize: "trim_upper"},
				{SchoolID: schoolID, Entity: "student", DapodikColumn: "NISN", SekociField: "nisn", IsMatchKey: false, Normalize: "digits"},
				{SchoolID: schoolID, Entity: "student", DapodikColumn: "Tanggal Lahir", SekociField: "birth_date", IsMatchKey: false, Normalize: "date"},
			}
		} else if entity == "rombel" {
			mappings = []FieldMapping{
				{SchoolID: schoolID, Entity: "rombel", DapodikColumn: "Nama Rombel", SekociField: "name", IsMatchKey: true, MatchPriority: 1, Normalize: "trim_upper"},
				{SchoolID: schoolID, Entity: "rombel", DapodikColumn: "Tingkat", SekociField: "grade_level", IsMatchKey: false, Normalize: "digits"},
			}
		} else {
			mappings = []FieldMapping{
				{SchoolID: schoolID, Entity: "ptk", DapodikColumn: "Nama", SekociField: "full_name", IsMatchKey: true, MatchPriority: 1, Normalize: "trim_upper"},
				{SchoolID: schoolID, Entity: "ptk", DapodikColumn: "Jabatan", SekociField: "position", IsMatchKey: false, Normalize: "trim_upper"},
			}
		}
	}

	// Fetch Sekoci records for matching
	var sekociRecords []Record
	switch entity {
	case "student":
		students, err := h.clients.GetDapodikStudents(r.Context(), schoolID)
		if err == nil {
			for _, s := range students {
				rec := Record{
					"_id":  s.UserID,
					"name": s.Name,
				}
				if s.NISN != nil {
					rec["nisn"] = *s.NISN
				}
				if s.BirthDate != nil {
					rec["birth_date"] = *s.BirthDate
				}
				if s.BirthPlace != nil {
					rec["birth_place"] = *s.BirthPlace
				}
				if s.Religion != nil {
					rec["religion"] = *s.Religion
				}
				sekociRecords = append(sekociRecords, rec)
			}
		}
	case "rombel":
		rData, err := h.clients.GetDapodikRombel(r.Context(), schoolID, "")
		if err == nil && rData != nil {
			for _, rItem := range rData.Rombel {
				rec := Record{
					"_id":         rItem.ClassroomID,
					"name":        rItem.Name,
					"grade_level": fmt.Sprintf("%d", rItem.GradeLevel),
				}
				sekociRecords = append(sekociRecords, rec)
			}
		}
	case "ptk":
		ptkList, err := h.clients.GetDapodikPTK(r.Context(), schoolID)
		if err == nil {
			for _, p := range ptkList {
				rec := Record{
					"_id":               p.EmployeeID,
					"full_name":         p.FullName,
					"position":          p.Position,
					"employment_status": p.EmploymentStatus,
				}
				if p.NIKLast4 != nil {
					rec["nik_last4"] = *p.NIKLast4
				}
				sekociRecords = append(sekociRecords, rec)
			}
		}
	}

	// Match records and compute diffs
	pairs, onlySekoci, onlyDapodik := MatchRecords(sekociRecords, rows, mappings)

	var allDiffs []DiffItem
	for _, p := range pairs {
		diffs := DiffFields(p, mappings)
		allDiffs = append(allDiffs, diffs...)
	}

	// Add missing in dapodik
	for _, sRec := range onlySekoci {
		ref := sRec["_id"]
		allDiffs = append(allDiffs, DiffItem{
			Kind:      "missing_in_dapodik",
			SekociRef: ref,
		})
	}

	// Add missing in sekoci
	for _, dRec := range onlyDapodik {
		rowID := dRec["_row_id"]
		allDiffs = append(allDiffs, DiffItem{
			Kind:        "missing_in_sekoci",
			ImportRowID: rowID,
		})
	}

	// Create import record
	imp := DapodikImport{
		SchoolID:     schoolID,
		Entity:       entity,
		FileName:     header.Filename,
		FileChecksum: fmt.Sprintf("chk-%d", time.Now().UnixNano()),
		RowCount:     len(rows),
		ImportedBy:   claims.UserID,
		Status:       "compared",
	}
	savedImp, err := h.store.CreateImport(r.Context(), imp)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "failed to record import: "+err.Error())
		return
	}

	// Hash sensitive NIKs before storing in DB
	for _, r := range rows {
		if nik, ok := r["NIK"]; ok {
			r["NIK"] = HashSensitive(h.hashKey, nik)
			r["nik_last4"] = Last4(nik)
		}
	}
	_ = h.store.InsertImportRows(r.Context(), savedImp.ID, schoolID, rows)
	_ = h.store.InsertDiffs(r.Context(), schoolID, savedImp.ID, entity, allDiffs)

	summary := map[string]interface{}{
		"rows_imported": len(rows),
		"pairs_matched": len(pairs),
		"diff_count":    len(allDiffs),
	}
	_ = h.store.LogDapodikSync(r.Context(), schoolID, entity, "compare", claims.UserID, savedImp.ID, summary)
	_ = h.store.RecordAudit(r.Context(), schoolID, "dapodik_import", savedImp.ID, "import", claims.UserID, claims.ActiveRole, nil, summary, "Impor & perbandingan Dapodik")

	common.WriteJSON(w, http.StatusCreated, map[string]interface{}{
		"import_id":   savedImp.ID,
		"row_count":   len(rows),
		"diff_count":  len(allDiffs),
		"matched":     len(pairs),
	})
}

func (h *Handler) ListDiffs(w http.ResponseWriter, r *http.Request) {
	importID := chi.URLParam(r, "id")
	schoolID := h.effectiveSchoolID(r)
	kind := r.URL.Query().Get("kind")
	status := r.URL.Query().Get("status")

	diffs, err := h.store.ListDiffs(r.Context(), schoolID, importID, kind, status)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "failed to list diffs: "+err.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, diffs)
}

func (h *Handler) DecideDiff(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	claims := common.FromContext(r.Context())
	schoolID := h.effectiveSchoolID(r)

	var req struct {
		Decision string `json:"decision"` // update_dapodik, update_sekoci, ignore
		Reason   string `json:"reason"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		common.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Decision != "update_dapodik" && req.Decision != "update_sekoci" && req.Decision != "ignore" {
		common.WriteError(w, http.StatusBadRequest, "keputusan harus salah satu dari: update_dapodik, update_sekoci, ignore")
		return
	}
	if strings.TrimSpace(req.Reason) == "" {
		common.WriteError(w, http.StatusUnprocessableEntity, "alasan keputusan wajib diisi")
		return
	}

	if err := h.store.DecideDiff(r.Context(), schoolID, id, req.Decision, req.Reason, claims.UserID); err != nil {
		common.WriteError(w, http.StatusInternalServerError, "failed to record decision: "+err.Error())
		return
	}

	_ = h.store.RecordAudit(r.Context(), schoolID, "dapodik_diff", id, "decide", claims.UserID, claims.ActiveRole, nil, req, "Keputusan perbedaan Dapodik")
	common.WriteJSON(w, http.StatusOK, map[string]string{"message": "Keputusan berhasil disimpan"})
}

func (h *Handler) ExportDapodikUpdates(w http.ResponseWriter, r *http.Request) {
	importID := chi.URLParam(r, "id")
	schoolID := h.effectiveSchoolID(r)
	format := r.URL.Query().Get("format")

	diffs, err := h.store.ListDiffs(r.Context(), schoolID, importID, "", "decided")
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "failed to get diffs: "+err.Error())
		return
	}

	// Filter update_dapodik
	var updates []DapodikDiff
	for _, d := range diffs {
		if d.Decision != nil && *d.Decision == "update_dapodik" {
			updates = append(updates, d)
		}
	}

	if format == "csv" {
		w.Header().Set("Content-Type", "text/csv")
		w.Header().Set("Content-Disposition", "attachment; filename=dapodik_update.csv")
		writer := csv.NewWriter(w)
		_ = writer.Write([]string{"Entity", "ID / Ref", "Field", "Nilai Terkoreksi (SEKOCI)", "Nilai Lama (Dapodik)", "Alasan"})
		for _, u := range updates {
			fName := ""
			if u.Field != nil {
				fName = *u.Field
			}
			sVal := ""
			if u.SekociValue != nil {
				sVal = *u.SekociValue
			}
			dVal := ""
			if u.DapodikValue != nil {
				dVal = *u.DapodikValue
			}
			ref := ""
			if u.SekociRef != nil {
				ref = *u.SekociRef
			}
			reason := ""
			if u.DecisionReason != nil {
				reason = *u.DecisionReason
			}
			_ = writer.Write([]string{u.Entity, ref, fName, sVal, dVal, reason})
		}
		writer.Flush()
		return
	}

	// Default XLSX
	f := excelize.NewFile()
	defer f.Close()
	sheet := "Pembaruan Dapodik"
	f.SetSheetName("Sheet1", sheet)
	headers := []string{"Entity", "ID / Ref", "Field", "Nilai Terkoreksi (SEKOCI)", "Nilai Lama (Dapodik)", "Alasan"}
	for i, hdr := range headers {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		f.SetCellValue(sheet, cell, hdr)
	}
	for rowIdx, u := range updates {
		fName := ""
		if u.Field != nil {
			fName = *u.Field
		}
		sVal := ""
		if u.SekociValue != nil {
			sVal = *u.SekociValue
		}
		dVal := ""
		if u.DapodikValue != nil {
			dVal = *u.DapodikValue
		}
		ref := ""
		if u.SekociRef != nil {
			ref = *u.SekociRef
		}
		reason := ""
		if u.DecisionReason != nil {
			reason = *u.DecisionReason
		}
		vals := []interface{}{u.Entity, ref, fName, sVal, dVal, reason}
		for colIdx, v := range vals {
			cell, _ := excelize.CoordinatesToCellName(colIdx+1, rowIdx+2)
			f.SetCellValue(sheet, cell, v)
		}
	}

	w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	w.Header().Set("Content-Disposition", "attachment; filename=dapodik_update.xlsx")
	_ = f.Write(w)
}

func (h *Handler) ListLogs(w http.ResponseWriter, r *http.Request) {
	schoolID := h.effectiveSchoolID(r)
	logs, err := h.store.ListSyncLogs(r.Context(), schoolID)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "failed to list logs: "+err.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, logs)
}

// ------------------- Regulatory Templates -------------------

func (h *Handler) ListTemplates(w http.ResponseWriter, r *http.Request) {
	schoolID := h.effectiveSchoolID(r)
	templates, err := h.store.ListTemplates(r.Context(), schoolID)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "failed to list templates: "+err.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, templates)
}

func (h *Handler) CreateTemplate(w http.ResponseWriter, r *http.Request) {
	claims := common.FromContext(r.Context())
	schoolID := h.effectiveSchoolID(r)

	var req RegulatoryTemplate
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		common.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	req.SchoolID = schoolID

	tpl, err := h.store.CreateTemplate(r.Context(), req)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "failed to create template: "+err.Error())
		return
	}

	_ = h.store.RecordAudit(r.Context(), schoolID, "regulatory_template", tpl.ID, "create", claims.UserID, claims.ActiveRole, nil, tpl, "Template laporan dibuat")
	common.WriteJSON(w, http.StatusCreated, tpl)
}

// ------------------- Regulatory Reports -------------------

func (h *Handler) ListReports(w http.ResponseWriter, r *http.Request) {
	schoolID := h.effectiveSchoolID(r)
	status := r.URL.Query().Get("status")

	reports, err := h.store.ListReports(r.Context(), schoolID, status)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "failed to list reports: "+err.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, reports)
}

func (h *Handler) GetReport(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	schoolID := h.effectiveSchoolID(r)

	rep, err := h.store.GetReport(r.Context(), schoolID, id)
	if err != nil || rep == nil {
		common.WriteError(w, http.StatusNotFound, "report not found")
		return
	}
	common.WriteJSON(w, http.StatusOK, rep)
}

func (h *Handler) CreateReport(w http.ResponseWriter, r *http.Request) {
	claims := common.FromContext(r.Context())
	schoolID := h.effectiveSchoolID(r)

	var req struct {
		TemplateID string `json:"template_id"`
		PeriodFrom string `json:"period_from"`
		PeriodTo   string `json:"period_to"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		common.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	idempotencyKey := r.Header.Get("Idempotency-Key")
	if idempotencyKey == "" {
		idempotencyKey = fmt.Sprintf("rep:%s:%s:%s", req.TemplateID, req.PeriodFrom, req.PeriodTo)
	}

	rep := RegulatoryReport{
		SchoolID:       schoolID,
		TemplateID:     req.TemplateID,
		PeriodFrom:     req.PeriodFrom,
		PeriodTo:       req.PeriodTo,
		IdempotencyKey: idempotencyKey,
	}

	saved, err := h.store.CreateReport(r.Context(), rep)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "failed to create report: "+err.Error())
		return
	}

	_ = h.store.RecordAudit(r.Context(), schoolID, "regulatory_report", saved.ID, "create", claims.UserID, claims.ActiveRole, nil, saved, "Laporan regulasi dibuat")
	common.WriteJSON(w, http.StatusCreated, saved)
}

func (h *Handler) PreviewReport(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	schoolID := h.effectiveSchoolID(r)

	rep, err := h.store.GetReport(r.Context(), schoolID, id)
	if err != nil || rep == nil {
		common.WriteError(w, http.StatusNotFound, "report not found")
		return
	}

	tpl, err := h.store.GetTemplate(r.Context(), schoolID, rep.TemplateID)
	if err != nil || tpl == nil {
		common.WriteError(w, http.StatusNotFound, "template not found")
		return
	}

	snap := BuildRegulatorySnapshot(*tpl, func(metricKey string) MetricResult {
		mv, err := h.clients.GetMetricValue(r.Context(), schoolID, metricKey, rep.PeriodFrom, rep.PeriodTo, "")
		if err != nil || mv == nil {
			return MetricResult{Err: err}
		}
		return MetricResult{Value: &mv.Value, Unit: mv.Unit, AsOf: &mv.AsOf}
	})

	common.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"report_id": rep.ID,
		"template":  tpl,
		"snapshot":  snap,
	})
}

func (h *Handler) SubmitReportForApproval(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	claims := common.FromContext(r.Context())
	schoolID := h.effectiveSchoolID(r)

	rep, err := h.store.GetReport(r.Context(), schoolID, id)
	if err != nil || rep == nil {
		common.WriteError(w, http.StatusNotFound, "report not found")
		return
	}
	if rep.Status != "draft" {
		common.WriteError(w, http.StatusConflict, "laporan sudah pernah diajukan")
		return
	}

	tpl, err := h.store.GetTemplate(r.Context(), schoolID, rep.TemplateID)
	if err != nil || tpl == nil {
		common.WriteError(w, http.StatusNotFound, "template not found")
		return
	}

	// Freeze live snapshot now (AC-146-01: snapshot will never change after submission)
	snap := BuildRegulatorySnapshot(*tpl, func(metricKey string) MetricResult {
		mv, err := h.clients.GetMetricValue(r.Context(), schoolID, metricKey, rep.PeriodFrom, rep.PeriodTo, "")
		if err != nil || mv == nil {
			return MetricResult{Err: err}
		}
		return MetricResult{Value: &mv.Value, Unit: mv.Unit, AsOf: &mv.AsOf}
	})

	// Submit approval to Core
	appReq := client.ApprovalRequest{
		SchoolID:      schoolID,
		RequestType:   "regulatory_report",
		RequesterID:   claims.UserID,
		SourceService: "quality",
		SourceID:      rep.ID,
		Summary:       fmt.Sprintf("Persetujuan Laporan %s (%s s/d %s)", tpl.Name, rep.PeriodFrom, rep.PeriodTo),
		CallbackURL:   "http://quality:8095/internal/approval-callbacks",
		IdempotencyKey: fmt.Sprintf("rep-approve:%s", rep.ID),
	}

	appResp, err := h.clients.CreateApproval(r.Context(), appReq)
	if err != nil {
		common.WriteError(w, http.StatusUnprocessableEntity, "Gagal mengajukan persetujuan ke Approval Center: "+err.Error())
		return
	}

	if err := h.store.SubmitReportForApproval(r.Context(), rep.ID, schoolID, appResp.ID, *tpl, snap); err != nil {
		common.WriteError(w, http.StatusInternalServerError, "failed to freeze report snapshot: "+err.Error())
		return
	}

	_ = h.store.RecordAudit(r.Context(), schoolID, "regulatory_report", rep.ID, "submit", claims.UserID, claims.ActiveRole, nil, snap, "Laporan diajukan untuk approval")
	common.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"message":     "Laporan berhasil diajukan untuk persetujuan",
		"approval_id": appResp.ID,
	})
}

func (h *Handler) MarkReportSubmitted(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	claims := common.FromContext(r.Context())
	schoolID := h.effectiveSchoolID(r)

	var req struct {
		Note            string `json:"note"`
		ExportObjectKey string `json:"export_object_key"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)

	if err := h.store.MarkReportSubmitted(r.Context(), id, schoolID, claims.UserID, req.Note, req.ExportObjectKey); err != nil {
		common.WriteError(w, http.StatusInternalServerError, "failed to mark report as submitted: "+err.Error())
		return
	}

	_ = h.store.RecordAudit(r.Context(), schoolID, "regulatory_report", id, "mark_submitted", claims.UserID, claims.ActiveRole, nil, req, "Laporan ditandai telah diserahkan")
	common.WriteJSON(w, http.StatusOK, map[string]string{"message": "Laporan berhasil ditandai telah diserahkan"})
}

func (h *Handler) ExportRegulatoryReport(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	schoolID := h.effectiveSchoolID(r)
	format := r.URL.Query().Get("format")

	rep, err := h.store.GetReport(r.Context(), schoolID, id)
	if err != nil || rep == nil {
		common.WriteError(w, http.StatusNotFound, "report not found")
		return
	}

	// Use data snapshot if present, otherwise live
	var snapCols []SnapshotColumnValue
	if rep.DataSnapshot != nil {
		snapCols = rep.DataSnapshot.Columns
	} else {
		tpl, _ := h.store.GetTemplate(r.Context(), schoolID, rep.TemplateID)
		if tpl != nil {
			s := BuildRegulatorySnapshot(*tpl, func(metricKey string) MetricResult {
				mv, err := h.clients.GetMetricValue(r.Context(), schoolID, metricKey, rep.PeriodFrom, rep.PeriodTo, "")
				if err != nil || mv == nil {
					return MetricResult{Err: err}
				}
				return MetricResult{Value: &mv.Value, Unit: mv.Unit, AsOf: &mv.AsOf}
			})
			snapCols = s.Columns
		}
	}

	if format == "csv" {
		w.Header().Set("Content-Type", "text/csv")
		w.Header().Set("Content-Disposition", "attachment; filename=laporan_regulasi.csv")
		writer := csv.NewWriter(w)
		_ = writer.Write([]string{"Indikator / Kolom", "Kunci Metrik", "Nilai", "Satuan", "Keterangan"})
		for _, col := range snapCols {
			valStr := ""
			if col.Value != nil {
				valStr = fmt.Sprintf("%d", *col.Value)
			}
			notice := ""
			if col.Unavailable {
				notice = col.ErrorNotice
			}
			_ = writer.Write([]string{col.Label, col.MetricKey, valStr, col.Unit, notice})
		}
		writer.Flush()
		return
	}

	// XLSX
	f := excelize.NewFile()
	defer f.Close()
	sheet := "Laporan"
	f.SetSheetName("Sheet1", sheet)
	headers := []string{"Indikator / Kolom", "Kunci Metrik", "Nilai", "Satuan", "Keterangan"}
	for i, hdr := range headers {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		f.SetCellValue(sheet, cell, hdr)
	}
	for rowIdx, col := range snapCols {
		var val interface{}
		if col.Value != nil {
			val = *col.Value
		} else {
			val = ""
		}
		notice := ""
		if col.Unavailable {
			notice = col.ErrorNotice
		}
		rowVals := []interface{}{col.Label, col.MetricKey, val, col.Unit, notice}
		for colIdx, v := range rowVals {
			cell, _ := excelize.CoordinatesToCellName(colIdx+1, rowIdx+2)
			f.SetCellValue(sheet, cell, v)
		}
	}

	w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	w.Header().Set("Content-Disposition", "attachment; filename=laporan_regulasi.xlsx")
	_ = f.Write(w)
}
