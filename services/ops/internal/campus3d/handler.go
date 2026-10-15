package campus3d

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"skci/ops/internal/client"
	"skci/ops/internal/common"
	"skci/ops/internal/facility"
	"skci/ops/internal/storage"
)

type Handler struct {
	Store        *Store
	Storage      storage.Storage
	Facility     FacilitySource
	Asset        AssetSource
	Canteen      CanteenSource
	Client       *client.Client
	MaxGLBBytes  int64
	TimeLocation *time.Location
}

func NewHandler(
	store *Store,
	stor storage.Storage,
	facilitySrc FacilitySource,
	assetSrc AssetSource,
	canteenSrc CanteenSource,
	c *client.Client,
	maxGLBBytes int64,
	timeLoc *time.Location,
) *Handler {
	if maxGLBBytes <= 0 {
		maxGLBBytes = DefaultMaxGLB
	}
	if timeLoc == nil {
		var err error
		timeLoc, err = time.LoadLocation("Asia/Jakarta")
		if err != nil {
			timeLoc = time.FixedZone("WIB", 7*3600)
		}
	}
	return &Handler{
		Store:        store,
		Storage:      stor,
		Facility:     facilitySrc,
		Asset:        assetSrc,
		Canteen:      canteenSrc,
		Client:       c,
		MaxGLBBytes:  maxGLBBytes,
		TimeLocation: timeLoc,
	}
}

// requireFeature checks if "campus3d" is enabled for the school.
func (h *Handler) requireFeature(w http.ResponseWriter, r *http.Request, schoolID string) bool {
	if h.Client == nil {
		return true // testing/standalone
	}
	ok, err := h.Client.FeatureEnabled(r.Context(), schoolID, "campus3d")
	if err != nil || !ok {
		common.WriteError(w, http.StatusForbidden, "Fitur kampus 3D belum diaktifkan")
		return false
	}
	return true
}

func isManager(c *common.Claims) bool {
	return common.HasActiveRole(c, common.RoleFacilityOfficer, common.RoleAdmin, common.RoleSuperadmin)
}

// -----------------------------------------------------------------------------
// Model Management (FR-155-004)
// -----------------------------------------------------------------------------

// UploadModel handles POST /campus3d/models (multipart building_id + file GLB).
func (h *Handler) UploadModel(w http.ResponseWriter, r *http.Request) {
	c := common.FromCtx(r)
	if !isManager(c) {
		common.WriteError(w, http.StatusForbidden, "Akses ditolak")
		return
	}
	schoolID := common.EffectiveSchoolID(r)
	if !h.requireFeature(w, r, schoolID) {
		return
	}

	if err := r.ParseMultipartForm(h.MaxGLBBytes + (1 << 20)); err != nil {
		common.WriteError(w, http.StatusBadRequest, "Ukuran file melebihi batas atau request tidak valid")
		return
	}

	buildingID := strings.TrimSpace(r.FormValue("building_id"))
	if buildingID == "" {
		common.WriteError(w, http.StatusUnprocessableEntity, "building_id wajib diisi")
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		common.WriteError(w, http.StatusBadRequest, "File model GLB wajib diunggah")
		return
	}
	defer file.Close()

	if header.Size > h.MaxGLBBytes {
		common.WriteError(w, http.StatusUnprocessableEntity, "Ukuran file GLB melebihi batas maksimum")
		return
	}

	// 1. Parse node names and validate GLB v2
	nodeNames, err := ParseGLBNodeNames(file, h.MaxGLBBytes)
	if err != nil {
		common.WriteError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}

	// Rewind file for storage
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal memproses file")
		return
	}

	// 2. Compute next version
	nextVer, err := h.Store.NextVersion(r.Context(), schoolID, buildingID)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal mendapatkan versi model")
		return
	}

	objectKey := fmt.Sprintf("campus3d/%s/%s/v%d.glb", schoolID, buildingID, nextVer)

	// Compute sha256 checksum and store file
	hasher := sha256.New()
	pipeReader, pipeWriter := io.Pipe()
	go func() {
		defer pipeWriter.Close()
		_, _ = io.Copy(io.MultiWriter(pipeWriter, hasher), file)
	}()

	obj, err := h.Storage.Put(r.Context(), objectKey, "model/gltf-binary", pipeReader, header.Size)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal menyimpan file model ke storage")
		return
	}
	checksum := hex.EncodeToString(hasher.Sum(nil))
	if obj.Checksum != "" {
		checksum = obj.Checksum
	}

	model := Model{
		SchoolID:   schoolID,
		BuildingID: buildingID,
		Version:    nextVer,
		ObjectKey:  objectKey,
		MIME:       "model/gltf-binary",
		SizeBytes:  header.Size,
		SHA256:     checksum,
		NodeNames:  nodeNames,
		Status:     "draft",
		UploadedBy: c.UserID,
	}

	id, err := h.Store.CreateModel(r.Context(), model, c.UserID, c.ActiveRole)
	if err != nil {
		_ = h.Storage.Delete(r.Context(), objectKey)
		common.WriteError(w, http.StatusInternalServerError, "Gagal menyimpan data model")
		return
	}

	model.ID = id
	common.WriteJSON(w, http.StatusCreated, model, nil)
}

// ListModels handles GET /campus3d/models?building_id=.
func (h *Handler) ListModels(w http.ResponseWriter, r *http.Request) {
	c := common.FromCtx(r)
	if !isManager(c) {
		common.WriteError(w, http.StatusForbidden, "Akses ditolak")
		return
	}
	schoolID := common.EffectiveSchoolID(r)
	if !h.requireFeature(w, r, schoolID) {
		return
	}

	buildingID := strings.TrimSpace(r.URL.Query().Get("building_id"))
	models, err := h.Store.ListModels(r.Context(), schoolID, buildingID)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal memuat daftar model")
		return
	}
	common.WriteJSON(w, http.StatusOK, models, nil)
}

// DeleteModel handles DELETE /campus3d/models/{id} (only draft models).
func (h *Handler) DeleteModel(w http.ResponseWriter, r *http.Request) {
	c := common.FromCtx(r)
	if !isManager(c) {
		common.WriteError(w, http.StatusForbidden, "Akses ditolak")
		return
	}
	schoolID := common.EffectiveSchoolID(r)
	if !h.requireFeature(w, r, schoolID) {
		return
	}

	id := chi.URLParam(r, "id")
	objKey, err := h.Store.DeleteDraftModel(r.Context(), schoolID, id, c.UserID, c.ActiveRole)
	if errors.Is(err, ErrNotFound) {
		common.WriteError(w, http.StatusNotFound, "Model tidak ditemukan")
		return
	}
	if errors.Is(err, ErrCannotDelete) {
		common.WriteError(w, http.StatusUnprocessableEntity, "Hanya model berstatus draft yang dapat dihapus")
		return
	}
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal menghapus model")
		return
	}

	if objKey != "" {
		_ = h.Storage.Delete(r.Context(), objKey)
	}

	common.WriteJSON(w, http.StatusOK, map[string]string{"message": "Model draft berhasil dihapus"}, nil)
}

// GetModelFile handles GET /campus3d/models/{id}/file.
func (h *Handler) GetModelFile(w http.ResponseWriter, r *http.Request) {
	c := common.FromCtx(r)
	schoolID := common.EffectiveSchoolID(r)
	if !h.requireFeature(w, r, schoolID) {
		return
	}

	id := chi.URLParam(r, "id")
	model, err := h.Store.GetModel(r.Context(), schoolID, id)
	if errors.Is(err, ErrNotFound) || model == nil {
		common.WriteError(w, http.StatusNotFound, "Model tidak ditemukan")
		return
	}
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal memuat informasi model")
		return
	}

	// Non-managers may only download published models
	if !isManager(c) && model.Status != "published" {
		common.WriteError(w, http.StatusForbidden, "Akses ke model ini dibatasi")
		return
	}

	// Check client If-None-Match header
	if etag := r.Header.Get("If-None-Match"); etag != "" && (etag == model.SHA256 || etag == `"`+model.SHA256+`"`) {
		w.WriteHeader(http.StatusNotModified)
		return
	}

	rc, err := h.Storage.Open(r.Context(), model.ObjectKey)
	if err != nil {
		common.WriteError(w, http.StatusNotFound, "File model tidak ditemukan di penyimpanan")
		return
	}
	defer rc.Close()

	w.Header().Set("Content-Type", "model/gltf-binary")
	w.Header().Set("Content-Length", strconv.FormatInt(model.SizeBytes, 10))
	w.Header().Set("ETag", `"`+model.SHA256+`"`)
	w.Header().Set("Cache-Control", "private, max-age=86400")

	_, _ = io.Copy(w, rc)
}

// GetModelMapping handles GET /campus3d/models/{id}/mapping.
func (h *Handler) GetModelMapping(w http.ResponseWriter, r *http.Request) {
	c := common.FromCtx(r)
	if !isManager(c) {
		common.WriteError(w, http.StatusForbidden, "Akses ditolak")
		return
	}
	schoolID := common.EffectiveSchoolID(r)
	if !h.requireFeature(w, r, schoolID) {
		return
	}

	id := chi.URLParam(r, "id")
	mapping, err := h.Store.GetModelMapping(r.Context(), schoolID, id)
	if errors.Is(err, ErrNotFound) {
		common.WriteError(w, http.StatusNotFound, "Model tidak ditemukan")
		return
	}
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal memuat pemetaan model")
		return
	}

	common.WriteJSON(w, http.StatusOK, mapping, nil)
}

// UpdateModelMapping handles PUT /campus3d/models/{id}/mapping.
func (h *Handler) UpdateModelMapping(w http.ResponseWriter, r *http.Request) {
	c := common.FromCtx(r)
	if !isManager(c) {
		common.WriteError(w, http.StatusForbidden, "Akses ditolak")
		return
	}
	schoolID := common.EffectiveSchoolID(r)
	if !h.requireFeature(w, r, schoolID) {
		return
	}

	id := chi.URLParam(r, "id")
	model, err := h.Store.GetModel(r.Context(), schoolID, id)
	if errors.Is(err, ErrNotFound) || model == nil {
		common.WriteError(w, http.StatusNotFound, "Model tidak ditemukan")
		return
	}

	var req struct {
		Floors []FloorMapping `json:"floors"`
		Rooms  []RoomMapping  `json:"rooms"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		common.WriteError(w, http.StatusBadRequest, "Format JSON tidak valid")
		return
	}

	// Fetch current floors and rooms for validation
	bFloors, err := h.Facility.ListBuildings(r.Context(), schoolID)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal memverifikasi data gedung")
		return
	}
	var floors []FloorRef
	for _, b := range bFloors {
		if b.ID == model.BuildingID {
			for _, f := range b.Floors {
				floors = append(floors, FloorRef{
					ID:         f.ID,
					BuildingID: b.ID,
					Level:      f.Level,
					Name:       f.Name,
				})
			}
		}
	}

	rRooms, err := h.Facility.ListRooms(r.Context(), schoolID, facility.RoomFilter{Limit: 1000})
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal memverifikasi data ruang")
		return
	}
	var rooms []RoomRef
	for _, rm := range rRooms {
		if rm.Building.ID == model.BuildingID {
			rooms = append(rooms, RoomRef{
				ID:         rm.ID,
				BuildingID: rm.Building.ID,
				FloorID:    rm.FloorID,
				Code:       rm.Code,
				Name:       rm.Name,
				Kind:       rm.Kind,
				Capacity:   rm.Capacity,
				Status:     rm.Status,
			})
		}
	}

	mappingPayload := ModelMapping{
		BuildingID: model.BuildingID,
		Floors:     req.Floors,
		Rooms:      req.Rooms,
	}
	issues := ValidateMappings(model.NodeNames, floors, rooms, mappingPayload)

	if err := h.Store.SaveModelMapping(r.Context(), schoolID, id, req.Floors, req.Rooms, c.UserID, c.ActiveRole); err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal menyimpan pemetaan")
		return
	}

	common.WriteJSON(w, http.StatusOK, map[string]any{
		"mapping": mappingPayload,
		"issues":  issues,
	}, nil)
}

// PublishModel handles POST /campus3d/models/{id}/publish.
func (h *Handler) PublishModel(w http.ResponseWriter, r *http.Request) {
	c := common.FromCtx(r)
	if !isManager(c) {
		common.WriteError(w, http.StatusForbidden, "Akses ditolak")
		return
	}
	schoolID := common.EffectiveSchoolID(r)
	if !h.requireFeature(w, r, schoolID) {
		return
	}

	id := chi.URLParam(r, "id")
	model, err := h.Store.GetModel(r.Context(), schoolID, id)
	if errors.Is(err, ErrNotFound) || model == nil {
		common.WriteError(w, http.StatusNotFound, "Model tidak ditemukan")
		return
	}

	mapping, err := h.Store.GetModelMapping(r.Context(), schoolID, id)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal memuat pemetaan")
		return
	}

	// Validate mappings before publishing
	bFloors, _ := h.Facility.ListBuildings(r.Context(), schoolID)
	var floors []FloorRef
	for _, b := range bFloors {
		if b.ID == model.BuildingID {
			for _, f := range b.Floors {
				floors = append(floors, FloorRef{ID: f.ID, BuildingID: b.ID, Level: f.Level, Name: f.Name})
			}
		}
	}
	rRooms, _ := h.Facility.ListRooms(r.Context(), schoolID, facility.RoomFilter{Limit: 1000})
	var rooms []RoomRef
	for _, rm := range rRooms {
		if rm.Building.ID == model.BuildingID {
			rooms = append(rooms, RoomRef{ID: rm.ID, BuildingID: rm.Building.ID, FloorID: rm.FloorID, Code: rm.Code, Name: rm.Name, Status: rm.Status})
		}
	}

	issues := ValidateMappings(model.NodeNames, floors, rooms, mapping)
	for _, iss := range issues {
		if iss.Kind == "error" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnprocessableEntity)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"message": "Pemetaan memiliki error yang harus diperbaiki sebelum dipublikasikan",
				"issues":  issues,
			})
			return
		}
	}

	if err := h.Store.PublishModel(r.Context(), schoolID, id, c.UserID, c.ActiveRole); err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal mempublikasikan model")
		return
	}

	common.WriteJSON(w, http.StatusOK, map[string]string{"message": "Model 3D berhasil dipublikasikan"}, nil)
}

// -----------------------------------------------------------------------------
// Tenant Spots Management
// -----------------------------------------------------------------------------

// ListTenantSpots handles GET /campus3d/tenant-spots.
func (h *Handler) ListTenantSpots(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	if !h.requireFeature(w, r, schoolID) {
		return
	}

	spots, err := h.Store.ListTenantSpots(r.Context(), schoolID)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal memuat data spot tenant")
		return
	}
	common.WriteJSON(w, http.StatusOK, spots, nil)
}

// UpdateTenantSpots handles PUT /campus3d/tenant-spots.
func (h *Handler) UpdateTenantSpots(w http.ResponseWriter, r *http.Request) {
	c := common.FromCtx(r)
	if !common.HasActiveRole(c, common.RoleCanteenAdmin, common.RoleFacilityOfficer, common.RoleAdmin, common.RoleSuperadmin) {
		common.WriteError(w, http.StatusForbidden, "Akses ditolak")
		return
	}
	schoolID := common.EffectiveSchoolID(r)
	if !h.requireFeature(w, r, schoolID) {
		return
	}

	var spots []TenantSpot
	if err := json.NewDecoder(r.Body).Decode(&spots); err != nil {
		common.WriteError(w, http.StatusBadRequest, "Format JSON tidak valid")
		return
	}

	// Verify all assigned rooms have kind='kantin'
	if len(spots) > 0 {
		var roomIDs []string
		for _, s := range spots {
			roomIDs = append(roomIDs, s.RoomID)
		}
		rooms, err := h.Facility.ListRooms(r.Context(), schoolID, facility.RoomFilter{IDs: roomIDs})
		if err != nil {
			common.WriteError(w, http.StatusInternalServerError, "Gagal memvalidasi ruang")
			return
		}
		for _, r := range rooms {
			if r.Kind != "kantin" {
				common.WriteError(w, http.StatusUnprocessableEntity, fmt.Sprintf("Ruang '%s' bukan jenis kantin", r.Name))
				return
			}
		}
	}

	if err := h.Store.SaveTenantSpots(r.Context(), schoolID, spots, c.UserID, c.ActiveRole); err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal menyimpan spot tenant")
		return
	}

	common.WriteJSON(w, http.StatusOK, map[string]string{"message": "Spot tenant berhasil diperbarui"}, nil)
}

// -----------------------------------------------------------------------------
// Operational 3D Views (FR-155-001, FR-155-002, FR-155-003, FR-156, FR-157, FR-158)
// -----------------------------------------------------------------------------

// GetScene handles GET /campus3d/scene (live buildings and active models).
func (h *Handler) GetScene(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	if !h.requireFeature(w, r, schoolID) {
		return
	}

	buildings, err := h.Facility.ListBuildings(r.Context(), schoolID)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal memuat data gedung kampus")
		return
	}

	type FloorScene struct {
		ID       string  `json:"id"`
		Level    int     `json:"level"`
		Name     string  `json:"name"`
		NodeName *string `json:"node_name,omitempty"`
	}

	type ModelSummary struct {
		ID      string `json:"id"`
		Version int    `json:"version"`
		FileURL string `json:"file_url"`
		SHA256  string `json:"sha256"`
	}

	type BuildingScene struct {
		ID     string        `json:"id"`
		Code   string        `json:"code"`
		Name   string        `json:"name"`
		Floors []FloorScene  `json:"floors"`
		Model  *ModelSummary `json:"model,omitempty"`
	}

	var sceneBuildings []BuildingScene
	for _, b := range buildings {
		bs := BuildingScene{
			ID:     b.ID,
			Code:   b.Code,
			Name:   b.Name,
			Floors: make([]FloorScene, 0, len(b.Floors)),
		}

		pubModel, _ := h.Store.GetPublishedModel(r.Context(), schoolID, b.ID)
		var floorNodeMap map[string]string
		if pubModel != nil {
			bs.Model = &ModelSummary{
				ID:      pubModel.ID,
				Version: pubModel.Version,
				FileURL: fmt.Sprintf("/api/ops/campus3d/models/%s/file", pubModel.ID),
				SHA256:  pubModel.SHA256,
			}
			mapping, _ := h.Store.GetModelMapping(r.Context(), schoolID, pubModel.ID)
			floorNodeMap = make(map[string]string, len(mapping.Floors))
			for _, fm := range mapping.Floors {
				floorNodeMap[fm.FloorID] = fm.NodeName
			}
		}

		for _, fl := range b.Floors {
			fs := FloorScene{
				ID:    fl.ID,
				Level: fl.Level,
				Name:  fl.Name,
			}
			if node, ok := floorNodeMap[fl.ID]; ok && node != "" {
				fs.NodeName = &node
			}
			bs.Floors = append(bs.Floors, fs)
		}

		sceneBuildings = append(sceneBuildings, bs)
	}

	w.Header().Set("Cache-Control", "no-store")
	common.WriteJSON(w, http.StatusOK, map[string]any{"buildings": sceneBuildings}, nil)
}

// SearchRooms handles GET /campus3d/rooms/search?q=.
func (h *Handler) SearchRooms(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	if !h.requireFeature(w, r, schoolID) {
		return
	}

	q := r.URL.Query().Get("q")
	rooms, err := h.Facility.ListRooms(r.Context(), schoolID, facility.RoomFilter{Limit: 1000})
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal mencari ruang")
		return
	}

	// Build combined room references and mappings across buildings
	var roomRefs []RoomRef
	var allMappings ModelMapping
	for _, rm := range rooms {
		roomRefs = append(roomRefs, RoomRef{
			ID:         rm.ID,
			BuildingID: rm.Building.ID,
			FloorID:    rm.FloorID,
			Code:       rm.Code,
			Name:       rm.Name,
			Kind:       rm.Kind,
			Capacity:   rm.Capacity,
			Status:     rm.Status,
		})
	}

	// Load mappings from all published models
	buildings, _ := h.Facility.ListBuildings(r.Context(), schoolID)
	for _, b := range buildings {
		if pubModel, _ := h.Store.GetPublishedModel(r.Context(), schoolID, b.ID); pubModel != nil {
			if m, err := h.Store.GetModelMapping(r.Context(), schoolID, pubModel.ID); err == nil {
				allMappings.Rooms = append(allMappings.Rooms, m.Rooms...)
			}
		}
	}

	hits := SearchRooms(roomRefs, allMappings, q)
	w.Header().Set("Cache-Control", "no-store")
	common.WriteJSON(w, http.StatusOK, hits, nil)
}

// GetRoomDetail handles GET /campus3d/rooms/{roomId}.
func (h *Handler) GetRoomDetail(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	if !h.requireFeature(w, r, schoolID) {
		return
	}

	roomID := chi.URLParam(r, "roomId")
	rm, err := h.Facility.GetRoom(r.Context(), schoolID, roomID)
	if err != nil {
		common.WriteError(w, http.StatusNotFound, "Ruang tidak ditemukan")
		return
	}

	roomRef := RoomRef{
		ID:         rm.ID,
		BuildingID: rm.Building.ID,
		FloorID:    rm.FloorID,
		Code:       rm.Code,
		Name:       rm.Name,
		Kind:       rm.Kind,
		Capacity:   rm.Capacity,
		Status:     rm.Status,
	}

	var focusTarget *Target
	if pubModel, _ := h.Store.GetPublishedModel(r.Context(), schoolID, rm.Building.ID); pubModel != nil {
		if mapping, err := h.Store.GetModelMapping(r.Context(), schoolID, pubModel.ID); err == nil {
			if target, ok := FocusTarget(mapping, roomID, []RoomRef{roomRef}); ok {
				focusTarget = &target
			}
		}
	}

	w.Header().Set("Cache-Control", "no-store")
	common.WriteJSON(w, http.StatusOK, map[string]any{
		"room":         rm,
		"focus_target": focusTarget,
	}, nil)
}

// GetCanteenView handles GET /campus3d/canteen.
func (h *Handler) GetCanteenView(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	if !h.requireFeature(w, r, schoolID) {
		return
	}

	spots, err := h.Store.ListTenantSpots(r.Context(), schoolID)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal memuat spot kantin")
		return
	}

	tenants, err := h.Canteen.ListTenants(r.Context(), schoolID)
	if err != nil {
		// As per T25 §4: if billing down/timeout -> 502 with clear message
		common.WriteError(w, http.StatusBadGateway, "Data kantin tidak tersedia")
		return
	}

	rooms, _ := h.Facility.ListRooms(r.Context(), schoolID, facility.RoomFilter{Kind: "kantin", Limit: 100})
	var roomRefs []RoomRef
	for _, rm := range rooms {
		roomRefs = append(roomRefs, RoomRef{
			ID:         rm.ID,
			BuildingID: rm.Building.ID,
			FloorID:    rm.FloorID,
			Name:       rm.Name,
		})
	}

	// Mappings for canteen rooms
	var mappings ModelMapping
	buildings, _ := h.Facility.ListBuildings(r.Context(), schoolID)
	for _, b := range buildings {
		if pubModel, _ := h.Store.GetPublishedModel(r.Context(), schoolID, b.ID); pubModel != nil {
			if m, err := h.Store.GetModelMapping(r.Context(), schoolID, pubModel.ID); err == nil {
				mappings.Rooms = append(mappings.Rooms, m.Rooms...)
			}
		}
	}

	view := CanteenView(spots, tenants, roomRefs, mappings)
	w.Header().Set("Cache-Control", "no-store")
	common.WriteJSON(w, http.StatusOK, view, nil)
}

// GetCanteenMenu handles GET /campus3d/canteen/{tenantId}/menu.
func (h *Handler) GetCanteenMenu(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	if !h.requireFeature(w, r, schoolID) {
		return
	}

	tenantID := chi.URLParam(r, "tenantId")
	menu, err := h.Canteen.GetMenu(r.Context(), schoolID, tenantID)
	if err != nil {
		common.WriteError(w, http.StatusBadGateway, "Data kantin tidak tersedia")
		return
	}

	w.Header().Set("Cache-Control", "no-store")
	common.WriteJSON(w, http.StatusOK, menu, nil)
}

// GetAvailability handles GET /campus3d/availability?at=.
func (h *Handler) GetAvailability(w http.ResponseWriter, r *http.Request) {
	c := common.FromCtx(r)
	schoolID := common.EffectiveSchoolID(r)
	if !h.requireFeature(w, r, schoolID) {
		return
	}

	at := time.Now()
	if atStr := r.URL.Query().Get("at"); atStr != "" {
		if parsed, err := time.Parse(time.RFC3339, atStr); err == nil {
			at = parsed
		}
	}

	// Filter special rooms (lab, perpustakaan, ruang_serbaguna, aula)
	allSpecialRooms, err := h.Facility.ListRooms(r.Context(), schoolID, facility.RoomFilter{Limit: 1000})
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal memuat daftar ruang")
		return
	}

	var targetRooms []facility.Room
	var roomIDs []string
	for _, rm := range allSpecialRooms {
		if rm.Kind == "lab" || rm.Kind == "perpustakaan" || rm.Kind == "ruang_serbaguna" || rm.Kind == "aula" {
			targetRooms = append(targetRooms, rm)
			roomIDs = append(roomIDs, rm.ID)
		}
	}

	busyMap, err := h.Facility.BusyIntervals(r.Context(), schoolID, roomIDs, at.Add(-24*time.Hour), at.Add(24*time.Hour))
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal memeriksa jadwal penggunaan ruang")
		return
	}

	isStaff := slices.Contains(common.StaffRoles, c.ActiveRole)

	type AvailabilityItem struct {
		RoomID     string   `json:"room_id"`
		Name       string   `json:"name"`
		Code       string   `json:"code"`
		Kind       string   `json:"kind"`
		Status     string   `json:"status"` // "terpakai" | "kosong"
		Current    *Busy    `json:"current,omitempty"`
		Today      []Busy   `json:"today"`
		BookingURL string   `json:"booking_url"`
	}

	var results []AvailabilityItem
	for _, rm := range targetRooms {
		rawSlots := busyMap[rm.ID]
		var busySlots []Busy
		for _, s := range rawSlots {
			b := Busy{
				Start: s.Start,
				End:   s.End,
				Label: s.Label,
			}
			if isStaff && s.BookedByName != nil {
				b.BookedByName = s.BookedByName
			}
			busySlots = append(busySlots, b)
		}

		status, cur, _ := RoomStatusAt(busySlots, at)
		todayList := TodaySchedule(busySlots, at, h.TimeLocation)
		if todayList == nil {
			todayList = []Busy{} // JSON [] bukan null — klien membaca .length
		}

		item := AvailabilityItem{
			RoomID:     rm.ID,
			Name:       rm.Name,
			Code:       rm.Code,
			Kind:       rm.Kind,
			Status:     status,
			Current:    cur,
			Today:      todayList,
			BookingURL: fmt.Sprintf("/fasilitas/booking?room_id=%s", rm.ID),
		}
		results = append(results, item)
	}

	w.Header().Set("Cache-Control", "no-store")
	common.WriteJSON(w, http.StatusOK, results, nil)
}

// GetDigitalTwin handles GET /campus3d/twin.
func (h *Handler) GetDigitalTwin(w http.ResponseWriter, r *http.Request) {
	c := common.FromCtx(r)
	if !common.HasActiveRole(c, common.RoleFacilityOfficer, common.RoleAssetOfficer, common.RoleKepalaSekolah, common.RoleAdmin, common.RoleSuperadmin) {
		common.WriteError(w, http.StatusForbidden, "Akses ke digital twin dibatasi untuk pengelola sarana dan aset")
		return
	}
	schoolID := common.EffectiveSchoolID(r)
	if !h.requireFeature(w, r, schoolID) {
		return
	}

	rooms, err := h.Facility.ListRooms(r.Context(), schoolID, facility.RoomFilter{Limit: 1000})
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal memuat ruang")
		return
	}

	var roomIDs []string
	var roomRefs []RoomRef
	for _, rm := range rooms {
		roomIDs = append(roomIDs, rm.ID)
		roomRefs = append(roomRefs, RoomRef{
			ID:         rm.ID,
			BuildingID: rm.Building.ID,
			FloorID:    rm.FloorID,
			Code:       rm.Code,
			Name:       rm.Name,
		})
	}

	activeWOs, _ := h.Facility.ActiveWorkOrders(r.Context(), schoolID, roomIDs)
	var woRefs []WorkOrderRef
	for _, wo := range activeWOs {
		roomID := ""
		if wo.RoomID != nil {
			roomID = *wo.RoomID
		}
		woRefs = append(woRefs, WorkOrderRef{
			ID:       wo.ID,
			RoomID:   roomID,
			Code:     wo.Code,
			Title:    wo.Title,
			Priority: wo.Priority,
			Status:   wo.Status,
		})
	}

	var assets []AssetSummary
	assetActive := false
	if h.Asset != nil {
		if rawAssets, err := h.Asset.ListByRooms(r.Context(), schoolID, roomIDs); err == nil {
			assets = rawAssets
			assetActive = true
		}
	}

	overlays := TwinOverlay(roomRefs, woRefs, assets)

	w.Header().Set("Cache-Control", "no-store")
	common.WriteJSON(w, http.StatusOK, map[string]any{
		"rooms":         overlays,
		"asset_active":  assetActive,
		"sensor_status": "Data sensor belum tersedia", // per OQ-42
	}, nil)
}
