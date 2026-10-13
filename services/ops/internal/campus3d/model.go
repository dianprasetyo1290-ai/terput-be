package campus3d

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"
)

// Standard GLB constants (glTF 2.0 binary).
const (
	glbMagic       = 0x46546C67 // "glTF"
	glbVersion2    = 2
	chunkTypeJSON  = 0x4E4F534A // "JSON"
	DefaultMaxGLB  = 50 * 1024 * 1024 // 50MB technical limit
)

// Common domain references for pure functions.
type FloorRef struct {
	ID         string `json:"id"`
	BuildingID string `json:"building_id"`
	Level      int    `json:"level"`
	Name       string `json:"name"`
}

type RoomRef struct {
	ID         string `json:"id"`
	BuildingID string `json:"building_id"`
	FloorID    string `json:"floor_id"`
	Code       string `json:"code"`
	Name       string `json:"name"`
	Kind       string `json:"kind"`
	Capacity   int    `json:"capacity"`
	Status     string `json:"status"`
}

type FloorMapping struct {
	FloorID  string `json:"floor_id"`
	NodeName string `json:"node_name"`
}

type RoomMapping struct {
	RoomID   string `json:"room_id"`
	NodeName string `json:"node_name"`
}

type ModelMapping struct {
	BuildingID string         `json:"building_id"`
	Floors     []FloorMapping `json:"floors"`
	Rooms      []RoomMapping  `json:"rooms"`
}

type Issue struct {
	Kind    string `json:"kind"` // "error" | "warning"
	Field   string `json:"field"`
	Message string `json:"message"`
}

type Target struct {
	BuildingID string `json:"building_id"`
	FloorID    string `json:"floor_id"`
	NodeName   string `json:"node_name"`
}

type Hit struct {
	RoomID     string  `json:"room_id"`
	Name       string  `json:"name"`
	Code       string  `json:"code"`
	Kind       string  `json:"kind"`
	BuildingID string  `json:"building_id"`
	FloorID    string  `json:"floor_id"`
	NodeName   *string `json:"node_name,omitempty"`
	Mapped     bool    `json:"mapped"`
}

type Busy struct {
	Start        time.Time `json:"start"`
	End          time.Time `json:"end"`
	Label        string    `json:"label"`
	BookedByName *string   `json:"booked_by_name,omitempty"`
}

type WorkOrderRef struct {
	ID       string `json:"id"`
	RoomID   string `json:"room_id"`
	Code     string `json:"code"`
	Title    string `json:"title"`
	Priority string `json:"priority"` // "darurat" | "tinggi" | "sedang" | "rendah"
	Status   string `json:"status"`   // "menunggu" | "diproses" | "selesai" | "dibatalkan"
}

type AssetSummary struct {
	ID        string `json:"id"`
	Code      string `json:"code"`
	Name      string `json:"name"`
	Category  string `json:"category"`
	Condition string `json:"condition"`
	RoomID    string `json:"room_id"`
}

type RoomOverlay struct {
	RoomID      string `json:"room_id"`
	Highlighted bool   `json:"highlighted"`
	StatusLabel string `json:"status_label"`
	Priority    string `json:"priority,omitempty"`
	AssetCount  int    `json:"asset_count"`
}

type TenantSpot struct {
	TenantID string `json:"tenant_id"`
	RoomID   string `json:"room_id"`
}

type CanteenTenant struct {
	ID        string  `json:"id"`
	Name      string  `json:"name"`
	Status    string  `json:"status"` // "active" | "inactive"
	IsOpen    bool    `json:"is_open"`
	OpenHours *string `json:"open_hours,omitempty"`
}

type CanteenSpotView struct {
	TenantID   string  `json:"tenant_id"`
	Name       string  `json:"name"`
	IsOpen     bool    `json:"is_open"`
	RoomID     string  `json:"room_id"`
	NodeName   *string `json:"node_name,omitempty"`
	BuildingID string  `json:"building_id,omitempty"`
	FloorID    string  `json:"floor_id,omitempty"`
	StatusText string  `json:"status_text"` // "Buka" | "Tutup"
}

// ParseGLBNodeNames reads a glTF 2.0 Binary (GLB) file, validates its header,
// parses the JSON chunk, and returns all node names defined in the model.
func ParseGLBNodeNames(r io.Reader, max int64) ([]string, error) {
	if max <= 0 {
		max = DefaultMaxGLB
	}

	lr := io.LimitReader(r, max+1)

	// 1. Read 12-byte GLB header
	var header struct {
		Magic   uint32
		Version uint32
		Length  uint32
	}
	if err := binary.Read(lr, binary.LittleEndian, &header); err != nil {
		return nil, errors.New("File harus GLB (glTF 2.0)")
	}

	if header.Magic != glbMagic || header.Version != glbVersion2 {
		return nil, errors.New("File harus GLB (glTF 2.0)")
	}
	if int64(header.Length) > max {
		return nil, errors.New("File harus GLB (glTF 2.0)")
	}

	// 2. Read first chunk header (must be JSON chunk)
	var chunkHeader struct {
		Length uint32
		Type   uint32
	}
	if err := binary.Read(lr, binary.LittleEndian, &chunkHeader); err != nil {
		return nil, errors.New("File harus GLB (glTF 2.0)")
	}
	if chunkHeader.Type != chunkTypeJSON {
		return nil, errors.New("File harus GLB (glTF 2.0)")
	}
	if int64(chunkHeader.Length) > max {
		return nil, errors.New("File harus GLB (glTF 2.0)")
	}

	// 3. Read JSON data
	jsonBuf := make([]byte, chunkHeader.Length)
	if _, err := io.ReadFull(lr, jsonBuf); err != nil {
		return nil, errors.New("File harus GLB (glTF 2.0)")
	}

	// 4. Parse glTF JSON
	var gltf struct {
		Nodes []struct {
			Name string `json:"name"`
		} `json:"nodes"`
	}
	if err := json.Unmarshal(jsonBuf, &gltf); err != nil {
		return nil, errors.New("File harus GLB (glTF 2.0)")
	}

	var names []string
	seen := make(map[string]bool)
	for _, n := range gltf.Nodes {
		name := strings.TrimSpace(n.Name)
		if name != "" && !seen[name] {
			seen[name] = true
			names = append(names, name)
		}
	}

	return names, nil
}

// ValidateMappings checks whether mapped nodes exist in the model, whether referenced
// floors and rooms exist, and checks for duplicate node mappings.
func ValidateMappings(nodeNames []string, floors []FloorRef, rooms []RoomRef, mappings ModelMapping) []Issue {
	var issues []Issue

	validNodes := make(map[string]bool, len(nodeNames))
	for _, n := range nodeNames {
		validNodes[n] = true
	}

	validFloors := make(map[string]FloorRef, len(floors))
	for _, f := range floors {
		validFloors[f.ID] = f
	}

	validRooms := make(map[string]RoomRef, len(rooms))
	for _, r := range rooms {
		validRooms[r.ID] = r
	}

	usedNodes := make(map[string]string) // node_name -> target_id

	// 1. Validate Floor mappings
	for _, fm := range mappings.Floors {
		if !validNodes[fm.NodeName] {
			issues = append(issues, Issue{
				Kind:    "error",
				Field:   "floors",
				Message: fmt.Sprintf("Node '%s' tidak ditemukan dalam model 3D", fm.NodeName),
			})
		}
		if _, ok := validFloors[fm.FloorID]; !ok {
			issues = append(issues, Issue{
				Kind:    "error",
				Field:   "floors",
				Message: fmt.Sprintf("Lantai ID '%s' tidak valid atau telah dihapus", fm.FloorID),
			})
		}
		if existing, ok := usedNodes[fm.NodeName]; ok {
			issues = append(issues, Issue{
				Kind:    "error",
				Field:   "floors",
				Message: fmt.Sprintf("Node '%s' sudah dipetakan ke objek '%s'", fm.NodeName, existing),
			})
		} else {
			usedNodes[fm.NodeName] = "floor:" + fm.FloorID
		}
	}

	// 2. Validate Room mappings
	mappedRoomIDs := make(map[string]bool)
	for _, rm := range mappings.Rooms {
		if !validNodes[rm.NodeName] {
			issues = append(issues, Issue{
				Kind:    "error",
				Field:   "rooms",
				Message: fmt.Sprintf("Node '%s' tidak ditemukan dalam model 3D", rm.NodeName),
			})
		}
		if _, ok := validRooms[rm.RoomID]; !ok {
			issues = append(issues, Issue{
				Kind:    "error",
				Field:   "rooms",
				Message: fmt.Sprintf("Ruang ID '%s' tidak valid atau telah dihapus", rm.RoomID),
			})
		}
		if existing, ok := usedNodes[rm.NodeName]; ok {
			issues = append(issues, Issue{
				Kind:    "error",
				Field:   "rooms",
				Message: fmt.Sprintf("Node '%s' sudah dipetakan ke objek '%s'", rm.NodeName, existing),
			})
		} else {
			usedNodes[rm.NodeName] = "room:" + rm.RoomID
		}
		mappedRoomIDs[rm.RoomID] = true
	}

	// 3. Warnings for active rooms not yet mapped
	for _, r := range rooms {
		if !mappedRoomIDs[r.ID] && r.Status != "inactive" {
			issues = append(issues, Issue{
				Kind:    "warning",
				Field:   "rooms",
				Message: fmt.Sprintf("Ruang aktif '%s' (%s) belum dipetakan ke node 3D", r.Name, r.Code),
			})
		}
	}

	return issues
}

// FocusTarget resolves a room ID to its 3D focus target (building, floor, node_name).
func FocusTarget(mappings ModelMapping, roomID string, rooms []RoomRef) (Target, bool) {
	var targetRoom *RoomRef
	for i := range rooms {
		if rooms[i].ID == roomID {
			targetRoom = &rooms[i]
			break
		}
	}
	if targetRoom == nil {
		return Target{}, false
	}

	var mappedNode string
	for _, rm := range mappings.Rooms {
		if rm.RoomID == roomID {
			mappedNode = rm.NodeName
			break
		}
	}
	if mappedNode == "" {
		return Target{}, false
	}

	return Target{
		BuildingID: targetRoom.BuildingID,
		FloorID:    targetRoom.FloorID,
		NodeName:   mappedNode,
	}, true
}

// SearchRooms searches rooms by normalized code or name, prioritizing prefix matches over
// substring matches, and indicates whether each room is mapped in the 3D model.
func SearchRooms(rooms []RoomRef, mappings ModelMapping, q string) []Hit {
	query := strings.ToLower(strings.TrimSpace(q))
	if query == "" {
		return nil
	}

	// Map room ID to node name and floor ID
	type mappingInfo struct {
		NodeName   string
		BuildingID string
		FloorID    string
	}
	roomMap := make(map[string]mappingInfo)
	for _, rm := range mappings.Rooms {
		roomMap[rm.RoomID] = mappingInfo{
			NodeName:   rm.NodeName,
			BuildingID: mappings.BuildingID,
		}
	}

	var prefixHits []Hit
	var containsHits []Hit

	for _, r := range rooms {
		nameLower := strings.ToLower(r.Name)
		codeLower := strings.ToLower(r.Code)

		matched := false
		isPrefix := false

		if strings.HasPrefix(nameLower, query) || strings.HasPrefix(codeLower, query) {
			matched = true
			isPrefix = true
		} else if strings.Contains(nameLower, query) || strings.Contains(codeLower, query) {
			matched = true
		}

		if matched {
			hit := Hit{
				RoomID:     r.ID,
				Name:       r.Name,
				Code:       r.Code,
				Kind:       r.Kind,
				BuildingID: r.BuildingID,
				FloorID:    r.FloorID,
			}
			if info, ok := roomMap[r.ID]; ok && info.NodeName != "" {
				node := info.NodeName
				hit.NodeName = &node
				hit.Mapped = true
			}
			if isPrefix {
				prefixHits = append(prefixHits, hit)
			} else {
				containsHits = append(containsHits, hit)
			}
		}
	}

	return append(prefixHits, containsHits...)
}

// RoomStatusAt evaluates busy intervals at a given point in time.
// Returns status ("terpakai" or "kosong"), the current occupant slot (if any),
// and the next upcoming occupant slot (if any).
func RoomStatusAt(busy []Busy, at time.Time) (string, *Busy, *Busy) {
	var current *Busy
	var next *Busy

	for i := range busy {
		b := &busy[i]
		// Check if occupied at `at`: start <= at < end
		if (b.Start.Before(at) || b.Start.Equal(at)) && b.End.After(at) {
			current = b
			break
		}
	}

	refTime := at
	if current != nil {
		refTime = current.End
	}

	for i := range busy {
		b := &busy[i]
		if b.Start.After(refTime) || b.Start.Equal(refTime) {
			if next == nil || b.Start.Before(next.Start) {
				next = b
			}
		}
	}

	if current != nil {
		return "terpakai", current, next
	}
	return "kosong", nil, next
}

// TodaySchedule filters busy slots to those occurring within the given calendar day in loc.
func TodaySchedule(busy []Busy, day time.Time, loc *time.Location) []Busy {
	if loc == nil {
		var err error
		loc, err = time.LoadLocation("Asia/Jakarta")
		if err != nil {
			loc = time.FixedZone("WIB", 7*3600)
		}
	}

	dayInLoc := day.In(loc)
	startOfDay := time.Date(dayInLoc.Year(), dayInLoc.Month(), dayInLoc.Day(), 0, 0, 0, 0, loc)
	endOfDay := startOfDay.Add(24 * time.Hour)

	var res []Busy
	for _, b := range busy {
		bStart := b.Start.In(loc)
		bEnd := b.End.In(loc)

		// Overlaps day
		if bStart.Before(endOfDay) && bEnd.After(startOfDay) {
			res = append(res, b)
		}
	}

	return res
}

// TwinOverlay produces room overlays highlighting active work orders and asset counts.
func TwinOverlay(rooms []RoomRef, workOrders []WorkOrderRef, assets []AssetSummary) []RoomOverlay {
	// Group assets by room
	assetCounts := make(map[string]int)
	for _, a := range assets {
		assetCounts[a.RoomID]++
	}

	// Group active work orders by room (only status 'diproses' or 'menunggu')
	activeWOsByRoom := make(map[string][]WorkOrderRef)
	for _, wo := range workOrders {
		if wo.Status == "diproses" || wo.Status == "menunggu" {
			activeWOsByRoom[wo.RoomID] = append(activeWOsByRoom[wo.RoomID], wo)
		}
	}

	out := make([]RoomOverlay, 0, len(rooms))
	for _, r := range rooms {
		overlay := RoomOverlay{
			RoomID:     r.ID,
			AssetCount: assetCounts[r.ID],
		}

		wos := activeWOsByRoom[r.ID]
		if len(wos) > 0 {
			// Find highest priority / active status: 'diproses' takes precedence over 'menunggu'
			var bestWO WorkOrderRef
			for _, wo := range wos {
				if bestWO.ID == "" {
					bestWO = wo
					continue
				}
				if wo.Status == "diproses" && bestWO.Status != "diproses" {
					bestWO = wo
				}
			}

			statusText := "Diproses"
			if bestWO.Status == "menunggu" {
				statusText = "Menunggu"
			}

			overlay.Highlighted = true
			overlay.StatusLabel = fmt.Sprintf("%s – %s", bestWO.Title, statusText)
			overlay.Priority = bestWO.Priority
		}

		out = append(out, overlay)
	}

	return out
}

// CanteenView joins tenant spot locations with live tenant operational data.
func CanteenView(spots []TenantSpot, tenants []CanteenTenant, rooms []RoomRef, mappings ModelMapping) []CanteenSpotView {
	tenantMap := make(map[string]CanteenTenant)
	for _, t := range tenants {
		tenantMap[t.ID] = t
	}

	roomMap := make(map[string]RoomRef)
	for _, r := range rooms {
		roomMap[r.ID] = r
	}

	nodeMap := make(map[string]string)
	for _, rm := range mappings.Rooms {
		nodeMap[rm.RoomID] = rm.NodeName
	}

	var res []CanteenSpotView
	for _, s := range spots {
		tenant, exists := tenantMap[s.TenantID]
		if !exists {
			continue
		}

		statusText := "Tutup"
		if tenant.IsOpen {
			statusText = "Buka"
		}

		view := CanteenSpotView{
			TenantID:   tenant.ID,
			Name:       tenant.Name,
			IsOpen:     tenant.IsOpen,
			RoomID:     s.RoomID,
			StatusText: statusText,
		}

		if node, ok := nodeMap[s.RoomID]; ok && node != "" {
			view.NodeName = &node
		}
		if r, ok := roomMap[s.RoomID]; ok {
			view.BuildingID = r.BuildingID
			view.FloorID = r.FloorID
		}

		res = append(res, view)
	}

	return res
}
