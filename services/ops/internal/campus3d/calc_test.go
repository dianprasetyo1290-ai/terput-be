package campus3d

import (
	"bytes"
	"encoding/binary"
	"strings"
	"testing"
	"time"
)

func makeTestGLB(nodeNames []string) []byte {
	var nodes []string
	for _, n := range nodeNames {
		nodes = append(nodes, `{"name":"`+n+`"}`)
	}
	jsonStr := `{"asset":{"version":"2.0"},"nodes":[` + strings.Join(nodes, ",") + `]}`
	jsonBytes := []byte(jsonStr)

	// Pad JSON to 4-byte boundary with spaces
	padLen := (4 - (len(jsonBytes) % 4)) % 4
	for i := 0; i < padLen; i++ {
		jsonBytes = append(jsonBytes, 0x20)
	}

	jsonChunkLen := uint32(len(jsonBytes))
	totalLen := uint32(12 + 8 + jsonChunkLen)

	buf := new(bytes.Buffer)
	// 12-byte GLB header
	_ = binary.Write(buf, binary.LittleEndian, uint32(glbMagic))
	_ = binary.Write(buf, binary.LittleEndian, uint32(glbVersion2))
	_ = binary.Write(buf, binary.LittleEndian, totalLen)

	// Chunk 0 header
	_ = binary.Write(buf, binary.LittleEndian, jsonChunkLen)
	_ = binary.Write(buf, binary.LittleEndian, uint32(chunkTypeJSON))
	buf.Write(jsonBytes)

	return buf.Bytes()
}

func TestParseGLBNodeNames(t *testing.T) {
	// Vector: valid GLB with 3 nodes
	glbData := makeTestGLB([]string{"Floor_1", "Lab_Kimia", "Perpustakaan"})
	names, err := ParseGLBNodeNames(bytes.NewReader(glbData), DefaultMaxGLB)
	if err != nil {
		t.Fatalf("unexpected error parsing valid GLB: %v", err)
	}
	if len(names) != 3 {
		t.Fatalf("expected 3 nodes, got %d", len(names))
	}
	if names[0] != "Floor_1" || names[1] != "Lab_Kimia" || names[2] != "Perpustakaan" {
		t.Errorf("mismatched names: %v", names)
	}

	// Vector: invalid file (non-GLB)
	invalidData := []byte("NOT A GLB FILE HEADER AT ALL")
	_, err = ParseGLBNodeNames(bytes.NewReader(invalidData), DefaultMaxGLB)
	if err == nil || err.Error() != "File harus GLB (glTF 2.0)" {
		t.Errorf("expected 'File harus GLB (glTF 2.0)', got %v", err)
	}

	// Vector: size exceeding max
	_, err = ParseGLBNodeNames(bytes.NewReader(glbData), 10)
	if err == nil || err.Error() != "File harus GLB (glTF 2.0)" {
		t.Errorf("expected max limit rejection, got %v", err)
	}
}

func TestValidateMappings(t *testing.T) {
	nodeNames := []string{"Floor_1", "Lab_Kimia", "Perpustakaan"}
	floors := []FloorRef{
		{ID: "f1", BuildingID: "b1", Level: 1, Name: "Lantai 1"},
	}
	rooms := []RoomRef{
		{ID: "r1", BuildingID: "b1", FloorID: "f1", Code: "LK01", Name: "Lab Kimia", Status: "active"},
		{ID: "r2", BuildingID: "b1", FloorID: "f1", Code: "PERP", Name: "Perpustakaan", Status: "active"},
	}

	// 1. Valid mapping with warning for unmapped room
	mapping := ModelMapping{
		BuildingID: "b1",
		Floors: []FloorMapping{
			{FloorID: "f1", NodeName: "Floor_1"},
		},
		Rooms: []RoomMapping{
			{RoomID: "r1", NodeName: "Lab_Kimia"},
		},
	}
	issues := ValidateMappings(nodeNames, floors, rooms, mapping)
	var hasError bool
	var hasWarning bool
	for _, iss := range issues {
		if iss.Kind == "error" {
			hasError = true
		}
		if iss.Kind == "warning" {
			hasWarning = true
		}
	}
	if hasError {
		t.Errorf("expected no errors for valid partial mapping, got issues: %v", issues)
	}
	if !hasWarning {
		t.Errorf("expected warning for unmapped active room 'r2'")
	}

	// 2. Invalid mapping: non-existent node and duplicate node mapping
	invalidMapping := ModelMapping{
		BuildingID: "b1",
		Floors: []FloorMapping{
			{FloorID: "f1", NodeName: "NonExistentNode"},
		},
		Rooms: []RoomMapping{
			{RoomID: "r1", NodeName: "Floor_1"},
			{RoomID: "r2", NodeName: "Floor_1"}, // duplicate!
		},
	}
	errIssues := ValidateMappings(nodeNames, floors, rooms, invalidMapping)
	var errCount int
	for _, iss := range errIssues {
		if iss.Kind == "error" {
			errCount++
		}
	}
	if errCount < 2 {
		t.Errorf("expected at least 2 errors for invalid mapping, got %d", errCount)
	}
}

func TestFocusTarget_AC155_01(t *testing.T) {
	mapping := ModelMapping{
		BuildingID: "b1",
		Rooms: []RoomMapping{
			{RoomID: "r_lab", NodeName: "Mesh_LabKimia"},
		},
	}
	rooms := []RoomRef{
		{ID: "r_lab", BuildingID: "b1", FloorID: "f1", Name: "Lab Kimia"},
		{ID: "r_unmapped", BuildingID: "b1", FloorID: "f1", Name: "Ruang Musik"},
	}

	// Mapped room
	target, ok := FocusTarget(mapping, "r_lab", rooms)
	if !ok {
		t.Fatalf("expected FocusTarget to succeed for mapped room")
	}
	if target.BuildingID != "b1" || target.FloorID != "f1" || target.NodeName != "Mesh_LabKimia" {
		t.Errorf("unexpected target: %+v", target)
	}

	// Unmapped room
	_, ok = FocusTarget(mapping, "r_unmapped", rooms)
	if ok {
		t.Errorf("expected ok=false for unmapped room")
	}
}

func TestSearchRooms_AC155_01(t *testing.T) {
	mapping := ModelMapping{
		BuildingID: "b1",
		Rooms: []RoomMapping{
			{RoomID: "r1", NodeName: "Node_LabKimia"},
		},
	}
	rooms := []RoomRef{
		{ID: "r1", BuildingID: "b1", FloorID: "f1", Code: "LAB-KIM", Name: "Lab Kimia"},
		{ID: "r2", BuildingID: "b1", FloorID: "f1", Code: "R-KIM-2", Name: "Kimia Ruang Tutor"},
		{ID: "r3", BuildingID: "b1", FloorID: "f1", Code: "LAB-BIO", Name: "Lab Biologi"},
	}

	// Search "lab kimia"
	hits := SearchRooms(rooms, mapping, "lab kimia")
	if len(hits) == 0 {
		t.Fatalf("expected hits for 'lab kimia'")
	}
	if hits[0].RoomID != "r1" {
		t.Errorf("expected Lab Kimia as top hit, got %s", hits[0].Name)
	}
	if !hits[0].Mapped || hits[0].NodeName == nil || *hits[0].NodeName != "Node_LabKimia" {
		t.Errorf("expected mapped=true with node name for Lab Kimia")
	}

	// Search "kim" matches both chemistry rooms
	kimHits := SearchRooms(rooms, mapping, "kim")
	if len(kimHits) != 2 {
		t.Errorf("expected 2 hits for 'kim', got %d", len(kimHits))
	}
}

func TestRoomStatusAt_AC157_01(t *testing.T) {
	loc, _ := time.LoadLocation("Asia/Jakarta")
	if loc == nil {
		loc = time.FixedZone("WIB", 7*3600)
	}

	t10_00 := time.Date(2026, 10, 15, 10, 0, 0, 0, loc)
	t12_00 := time.Date(2026, 10, 15, 12, 0, 0, 0, loc)
	t14_00 := time.Date(2026, 10, 15, 14, 0, 0, 0, loc)
	t16_00 := time.Date(2026, 10, 15, 16, 0, 0, 0, loc)

	reqName := "Pak Budi (Guru Kimia)"
	busy := []Busy{
		{Start: t10_00, End: t12_00, Label: "Praktikum Kimia Organik", BookedByName: &reqName},
		{Start: t14_00, End: t16_00, Label: "Praktikum Kelas 12", BookedByName: &reqName},
	}

	// AC-157-01: checked at 10.30 -> "terpakai" + pemesan + next is 14.00
	t10_30 := time.Date(2026, 10, 15, 10, 30, 0, 0, loc)
	status, cur, next := RoomStatusAt(busy, t10_30)
	if status != "terpakai" {
		t.Errorf("expected 'terpakai' at 10:30, got %s", status)
	}
	if cur == nil || cur.Label != "Praktikum Kimia Organik" || *cur.BookedByName != reqName {
		t.Errorf("unexpected current slot: %+v", cur)
	}
	if next == nil || next.Start != t14_00 {
		t.Errorf("expected next slot at 14:00, got %+v", next)
	}

	// AC-157-01: checked at 12.00 -> "kosong" + current nil + next is 14.00
	status, cur, next = RoomStatusAt(busy, t12_00)
	if status != "kosong" {
		t.Errorf("expected 'kosong' at 12:00, got %s", status)
	}
	if cur != nil {
		t.Errorf("expected cur=nil at 12:00")
	}
	if next == nil || next.Start != t14_00 {
		t.Errorf("expected next slot at 14:00, got %+v", next)
	}
}

func TestTwinOverlay_AC158_01(t *testing.T) {
	rooms := []RoomRef{
		{ID: "r_8a", Name: "Ruang 8A"},
		{ID: "r_8b", Name: "Ruang 8B"},
	}

	workOrders := []WorkOrderRef{
		{
			ID:       "wo_1",
			RoomID:   "r_8a",
			Code:     "WO-2026-001",
			Title:    "Perbaikan AC",
			Priority: "tinggi",
			Status:   "diproses",
		},
	}

	assets := []AssetSummary{
		{ID: "a1", RoomID: "r_8a", Name: "AC Daikin 2PK"},
		{ID: "a2", RoomID: "r_8a", Name: "Proyektor Epson"},
		{ID: "a3", RoomID: "r_8a", Name: "Whiteboard Magnetik"},
		{ID: "a4", RoomID: "r_8b", Name: "Whiteboard Magnetik"},
	}

	// AC-158-01: Room 8A highlighted with "Perbaikan AC – Diproses", asset_count=3
	// Room 8B not highlighted, asset_count=1
	overlays := TwinOverlay(rooms, workOrders, assets)
	if len(overlays) != 2 {
		t.Fatalf("expected 2 overlays, got %d", len(overlays))
	}

	var ov8A, ov8B RoomOverlay
	for _, o := range overlays {
		if o.RoomID == "r_8a" {
			ov8A = o
		} else if o.RoomID == "r_8b" {
			ov8B = o
		}
	}

	if !ov8A.Highlighted || ov8A.StatusLabel != "Perbaikan AC – Diproses" {
		t.Errorf("expected 8A highlighted with 'Perbaikan AC – Diproses', got %+v", ov8A)
	}
	if ov8A.AssetCount != 3 {
		t.Errorf("expected 8A asset_count=3, got %d", ov8A.AssetCount)
	}

	if ov8B.Highlighted {
		t.Errorf("expected 8B not highlighted")
	}
	if ov8B.AssetCount != 1 {
		t.Errorf("expected 8B asset_count=1, got %d", ov8B.AssetCount)
	}
}

func TestCanteenView(t *testing.T) {
	spots := []TenantSpot{
		{TenantID: "t1", RoomID: "r_k1"},
		{TenantID: "t2", RoomID: "r_k2"},
	}
	tenants := []CanteenTenant{
		{ID: "t1", Name: "Kantin Bu Siti", IsOpen: true},
		{ID: "t2", Name: "Kantin Pak Joko", IsOpen: false},
		{ID: "t3", Name: "Kantin Baru Belum Ada Tempat", IsOpen: true},
	}
	rooms := []RoomRef{
		{ID: "r_k1", BuildingID: "b1", FloorID: "f1"},
		{ID: "r_k2", BuildingID: "b1", FloorID: "f1"},
	}
	mapping := ModelMapping{
		BuildingID: "b1",
		Rooms: []RoomMapping{
			{RoomID: "r_k1", NodeName: "Kantin_Spot_1"},
		},
	}

	views := CanteenView(spots, tenants, rooms, mapping)
	if len(views) != 2 {
		t.Fatalf("expected 2 mapped spots, got %d", len(views))
	}

	if views[0].Name != "Kantin Bu Siti" || views[0].StatusText != "Buka" || views[0].NodeName == nil || *views[0].NodeName != "Kantin_Spot_1" {
		t.Errorf("unexpected view for t1: %+v", views[0])
	}
	if views[1].Name != "Kantin Pak Joko" || views[1].StatusText != "Tutup" {
		t.Errorf("unexpected view for t2: %+v", views[1])
	}
}
