package campus3d

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"skci/ops/internal/client"
	"skci/ops/internal/common"
	"skci/ops/internal/facility"
)

// mockFacilitySource implements FacilitySource for testing.
type mockFacilitySource struct {
	buildings  []facility.Building
	rooms      []facility.Room
	busySlots  map[string][]facility.BusySlot
	workOrders []facility.WorkOrder
}

func (m *mockFacilitySource) ListBuildings(ctx context.Context, schoolID string) ([]facility.Building, error) {
	return m.buildings, nil
}

func (m *mockFacilitySource) ListRooms(ctx context.Context, schoolID string, filter facility.RoomFilter) ([]facility.Room, error) {
	return m.rooms, nil
}

func (m *mockFacilitySource) GetRoom(ctx context.Context, schoolID, id string) (facility.Room, error) {
	for _, r := range m.rooms {
		if r.ID == id {
			return r, nil
		}
	}
	return facility.Room{}, facility.ErrNotFound
}

func (m *mockFacilitySource) BusyIntervals(ctx context.Context, schoolID string, roomIDs []string, from, to time.Time) (map[string][]facility.BusySlot, error) {
	return m.busySlots, nil
}

func (m *mockFacilitySource) ActiveWorkOrders(ctx context.Context, schoolID string, roomIDs []string) ([]facility.WorkOrder, error) {
	return m.workOrders, nil
}

// mockCanteenSource implements CanteenSource with dynamic live updates.
type mockCanteenSource struct {
	tenants []CanteenTenant
	menus   map[string][]client.CanteenMenuItem
}

func (m *mockCanteenSource) ListTenants(ctx context.Context, schoolID string) ([]CanteenTenant, error) {
	return m.tenants, nil
}

func (m *mockCanteenSource) GetMenu(ctx context.Context, schoolID, tenantID string) ([]client.CanteenMenuItem, error) {
	return m.menus[tenantID], nil
}

// mockAssetSource implements AssetSource.
type mockAssetSource struct {
	assets []AssetSummary
}

func (m *mockAssetSource) ListByRooms(ctx context.Context, schoolID string, roomIDs []string) ([]AssetSummary, error) {
	return m.assets, nil
}

func TestHandler_AC156_LiveCanteenNoReplica(t *testing.T) {
	canteenSrc := &mockCanteenSource{
		tenants: []CanteenTenant{
			{ID: "tenant-1", Name: "Kantin Bu Siti", Status: "active", IsOpen: true},
		},
		menus: map[string][]client.CanteenMenuItem{
			"tenant-1": {
				{ItemID: "m1", Name: "Nasi Goreng", PriceIDR: 15000, Available: true},
			},
		},
	}

	h := &Handler{
		Canteen: canteenSrc,
	}

	r := chi.NewRouter()
	r.Get("/campus3d/canteen/{tenantId}/menu", h.GetCanteenMenu)

	// First request: price is 15000
	req1 := httptest.NewRequest("GET", "/campus3d/canteen/tenant-1/menu", nil)
	req1 = req1.WithContext(common.WithClaims(req1.Context(), &common.Claims{SchoolID: "sch-1", ActiveRole: "siswa"}))
	w1 := httptest.NewRecorder()
	r.ServeHTTP(w1, req1)

	if w1.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w1.Code)
	}
	var res1 struct {
		Data []client.CanteenMenuItem `json:"data"`
	}
	_ = json.Unmarshal(w1.Body.Bytes(), &res1)
	if len(res1.Data) != 1 || res1.Data[0].PriceIDR != 15000 {
		t.Fatalf("expected price 15000, got %+v", res1.Data)
	}

	// Update live source price to 18000 without any local replication (BR-3D-01)
	canteenSrc.menus["tenant-1"][0].PriceIDR = 18000

	req2 := httptest.NewRequest("GET", "/campus3d/canteen/tenant-1/menu", nil)
	req2 = req2.WithContext(common.WithClaims(req2.Context(), &common.Claims{SchoolID: "sch-1", ActiveRole: "siswa"}))
	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, req2)

	if w2.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w2.Code)
	}
	var res2 struct {
		Data []client.CanteenMenuItem `json:"data"`
	}
	_ = json.Unmarshal(w2.Body.Bytes(), &res2)
	if len(res2.Data) != 1 || res2.Data[0].PriceIDR != 18000 {
		t.Fatalf("expected price 18000 on immediate second request, got %+v", res2.Data)
	}
}

func TestHandler_AC157_AvailabilityStaffVsStudent(t *testing.T) {
	loc := time.FixedZone("WIB", 7*3600)
	t10_00 := time.Date(2026, 10, 15, 10, 0, 0, 0, loc)
	t12_00 := time.Date(2026, 10, 15, 12, 0, 0, 0, loc)
	booker := "Budi Santoso (Guru)"

	facSrc := &mockFacilitySource{
		rooms: []facility.Room{
			{ID: "r-lab", Name: "Lab Kimia", Code: "LK", Kind: "lab"},
		},
		busySlots: map[string][]facility.BusySlot{
			"r-lab": {
				{
					Interval:     facility.Interval{Start: t10_00, End: t12_00},
					Label:        "Praktikum Kimia",
					BookedByName: &booker,
				},
			},
		},
	}

	h := &Handler{
		Facility:     facSrc,
		TimeLocation: loc,
	}

	// 1. Staff request at 10:30 sees status "terpakai" AND booked_by_name
	reqStaff := httptest.NewRequest("GET", "/campus3d/availability?at=2026-10-15T10:30:00%2B07:00", nil)
	reqStaff = reqStaff.WithContext(common.WithClaims(reqStaff.Context(), &common.Claims{SchoolID: "sch-1", ActiveRole: "guru"}))
	wStaff := httptest.NewRecorder()
	h.GetAvailability(wStaff, reqStaff)

	if wStaff.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", wStaff.Code, wStaff.Body.String())
	}
	var resStaff struct {
		Data []struct {
			RoomID  string `json:"room_id"`
			Status  string `json:"status"`
			Current *struct {
				Label        string  `json:"label"`
				BookedByName *string `json:"booked_by_name"`
			} `json:"current"`
		} `json:"data"`
	}
	_ = json.Unmarshal(wStaff.Body.Bytes(), &resStaff)
	if len(resStaff.Data) != 1 || resStaff.Data[0].Status != "terpakai" {
		t.Fatalf("expected terpakai for staff, got %+v", resStaff.Data)
	}
	if resStaff.Data[0].Current == nil || resStaff.Data[0].Current.BookedByName == nil || *resStaff.Data[0].Current.BookedByName != booker {
		t.Fatalf("expected staff to see booker name, got %+v", resStaff.Data[0].Current)
	}

	// 2. Student request at 10:30 sees status "terpakai" WITHOUT booked_by_name (redacted per FR-157-002)
	reqStudent := httptest.NewRequest("GET", "/campus3d/availability?at=2026-10-15T10:30:00%2B07:00", nil)
	reqStudent = reqStudent.WithContext(common.WithClaims(reqStudent.Context(), &common.Claims{SchoolID: "sch-1", ActiveRole: "siswa"}))
	wStudent := httptest.NewRecorder()
	h.GetAvailability(wStudent, reqStudent)

	if wStudent.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", wStudent.Code)
	}
	var resStudent struct {
		Data []struct {
			Status  string `json:"status"`
			Current *struct {
				Label        string  `json:"label"`
				BookedByName *string `json:"booked_by_name"`
			} `json:"current"`
		} `json:"data"`
	}
	_ = json.Unmarshal(wStudent.Body.Bytes(), &resStudent)
	if resStudent.Data[0].Status != "terpakai" {
		t.Errorf("expected student to see terpakai")
	}
	if resStudent.Data[0].Current != nil && resStudent.Data[0].Current.BookedByName != nil {
		t.Errorf("expected student NOT to see booked_by_name, but got %s", *resStudent.Data[0].Current.BookedByName)
	}
}

func TestHandler_AC158_DigitalTwinPermissionsAndAssets(t *testing.T) {
	roomID := "r-8a"
	woRoomID := "r-8a"
	facSrc := &mockFacilitySource{
		rooms: []facility.Room{
			{ID: roomID, Name: "Ruang 8A", Building: facility.RefBuilding{ID: "b1"}},
		},
		workOrders: []facility.WorkOrder{
			{ID: "wo-1", RoomID: &woRoomID, Code: "WO-01", Title: "Perbaikan AC", Priority: "tinggi", Status: "diproses"},
		},
	}
	astSrc := &mockAssetSource{
		assets: []AssetSummary{
			{ID: "a1", RoomID: roomID, Name: "AC 2PK"},
		},
	}

	h := &Handler{
		Facility: facSrc,
		Asset:    astSrc,
	}

	// 1. Siswa is forbidden (403)
	reqSiswa := httptest.NewRequest("GET", "/campus3d/twin", nil)
	reqSiswa = reqSiswa.WithContext(common.WithClaims(reqSiswa.Context(), &common.Claims{SchoolID: "sch-1", ActiveRole: "siswa"}))
	wSiswa := httptest.NewRecorder()
	h.GetDigitalTwin(wSiswa, reqSiswa)
	if wSiswa.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for siswa, got %d", wSiswa.Code)
	}

	// 2. Facility officer gets 200 with overlays and asset_active=true
	reqFO := httptest.NewRequest("GET", "/campus3d/twin", nil)
	reqFO = reqFO.WithContext(common.WithClaims(reqFO.Context(), &common.Claims{SchoolID: "sch-1", ActiveRole: "facility_officer"}))
	wFO := httptest.NewRecorder()
	h.GetDigitalTwin(wFO, reqFO)
	if wFO.Code != http.StatusOK {
		t.Fatalf("expected 200 for facility officer, got %d", wFO.Code)
	}

	var resFO struct {
		Data struct {
			Rooms        []RoomOverlay `json:"rooms"`
			AssetActive  bool          `json:"asset_active"`
			SensorStatus string        `json:"sensor_status"`
		} `json:"data"`
	}
	_ = json.Unmarshal(wFO.Body.Bytes(), &resFO)
	if !resFO.Data.AssetActive {
		t.Errorf("expected asset_active=true")
	}
	if resFO.Data.SensorStatus != "Data sensor belum tersedia" {
		t.Errorf("expected 'Data sensor belum tersedia', got %s", resFO.Data.SensorStatus)
	}
	if len(resFO.Data.Rooms) != 1 || !resFO.Data.Rooms[0].Highlighted {
		t.Errorf("expected 8A to be highlighted, got %+v", resFO.Data.Rooms)
	}

	// 3. AssetSource nil -> twin still succeeds with asset_active=false
	hNoAsset := &Handler{
		Facility: facSrc,
		Asset:    nil,
	}
	wNoAsset := httptest.NewRecorder()
	hNoAsset.GetDigitalTwin(wNoAsset, reqFO)
	if wNoAsset.Code != http.StatusOK {
		t.Fatalf("expected 200 when AssetSource is nil, got %d", wNoAsset.Code)
	}
	var resNoAsset struct {
		Data struct {
			AssetActive bool `json:"asset_active"`
		} `json:"data"`
	}
	_ = json.Unmarshal(wNoAsset.Body.Bytes(), &resNoAsset)
	if resNoAsset.Data.AssetActive {
		t.Errorf("expected asset_active=false when asset source is nil")
	}
}

func TestHandler_FeatureDisabled(t *testing.T) {
	// Mock identity service that says campus3d is disabled
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]map[string]any{
			{"feature_key": "campus3d", "enabled": false},
		})
	}))
	defer srv.Close()

	cli := client.New(srv.URL, "", "", "test-key", time.Minute)

	h := &Handler{
		Client: cli,
	}

	req := httptest.NewRequest("GET", "/campus3d/scene", nil)
	req = req.WithContext(common.WithClaims(req.Context(), &common.Claims{SchoolID: "sch-1", ActiveRole: "guru"}))
	w := httptest.NewRecorder()
	h.GetScene(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403 when feature is disabled, got %d", w.Code)
	}
}
