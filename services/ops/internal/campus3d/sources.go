package campus3d

import (
	"context"
	"time"

	"skci/ops/internal/asset"
	"skci/ops/internal/client"
	"skci/ops/internal/facility"
)

// FacilitySource provides read access to facility infrastructure (T9).
type FacilitySource interface {
	ListBuildings(ctx context.Context, schoolID string) ([]facility.Building, error)
	ListRooms(ctx context.Context, schoolID string, filter facility.RoomFilter) ([]facility.Room, error)
	GetRoom(ctx context.Context, schoolID, id string) (facility.Room, error)
	BusyIntervals(ctx context.Context, schoolID string, roomIDs []string, from, to time.Time) (map[string][]facility.BusySlot, error)
	ActiveWorkOrders(ctx context.Context, schoolID string, roomIDs []string) ([]facility.WorkOrder, error)
}

// AssetSource provides read access to room asset summaries (T16).
type AssetSource interface {
	ListByRooms(ctx context.Context, schoolID string, roomIDs []string) ([]AssetSummary, error)
}

// CanteenSource provides read access to tenant and menu data from billing (T12).
type CanteenSource interface {
	ListTenants(ctx context.Context, schoolID string) ([]CanteenTenant, error)
	GetMenu(ctx context.Context, schoolID, tenantID string) ([]client.CanteenMenuItem, error)
}

// facilitySourceAdapter wraps facility.Store to satisfy FacilitySource.
type facilitySourceAdapter struct {
	store *facility.Store
}

func NewFacilitySource(s *facility.Store) FacilitySource {
	return &facilitySourceAdapter{store: s}
}

func (a *facilitySourceAdapter) ListBuildings(ctx context.Context, schoolID string) ([]facility.Building, error) {
	return a.store.ListBuildings(ctx, schoolID)
}

func (a *facilitySourceAdapter) ListRooms(ctx context.Context, schoolID string, filter facility.RoomFilter) ([]facility.Room, error) {
	rooms, _, err := a.store.ListRooms(ctx, schoolID, filter)
	return rooms, err
}

func (a *facilitySourceAdapter) GetRoom(ctx context.Context, schoolID, id string) (facility.Room, error) {
	return a.store.GetRoom(ctx, schoolID, id)
}

func (a *facilitySourceAdapter) BusyIntervals(ctx context.Context, schoolID string, roomIDs []string, from, to time.Time) (map[string][]facility.BusySlot, error) {
	return a.store.BusyIntervals(ctx, a.store.DB, schoolID, roomIDs, from, to)
}

func (a *facilitySourceAdapter) ActiveWorkOrders(ctx context.Context, schoolID string, roomIDs []string) ([]facility.WorkOrder, error) {
	return a.store.ActiveWorkOrders(ctx, schoolID, roomIDs)
}

// assetSourceAdapter wraps asset.Store to satisfy AssetSource.
type assetSourceAdapter struct {
	store *asset.Store
}

func NewAssetSource(s *asset.Store) AssetSource {
	if s == nil {
		return nil
	}
	return &assetSourceAdapter{store: s}
}

func (a *assetSourceAdapter) ListByRooms(ctx context.Context, schoolID string, roomIDs []string) ([]AssetSummary, error) {
	if a.store == nil || len(roomIDs) == 0 {
		return []AssetSummary{}, nil
	}

	filter := asset.AssetFilter{
		SchoolID: schoolID,
		RoomIDs:  roomIDs,
		Status:   "active",
		PerPage:  200,
	}
	items, _, err := a.store.ListAssets(ctx, filter)
	if err != nil {
		return nil, err
	}

	out := make([]AssetSummary, 0, len(items))
	for _, it := range items {
		roomID := ""
		if it.RoomID != nil {
			roomID = *it.RoomID
		}
		category := ""
		if it.CategoryName != nil {
			category = *it.CategoryName
		}
		cond := ""
		if it.Condition != nil {
			cond = *it.Condition
		}
		out = append(out, AssetSummary{
			ID:        it.ID,
			Code:      it.Code,
			Name:      it.Name,
			Category:  category,
			Condition: cond,
			RoomID:    roomID,
		})
	}
	return out, nil
}

// canteenSourceAdapter wraps client.Client to satisfy CanteenSource.
type canteenSourceAdapter struct {
	client *client.Client
}

func NewCanteenSource(c *client.Client) CanteenSource {
	return &canteenSourceAdapter{client: c}
}

func (c *canteenSourceAdapter) ListTenants(ctx context.Context, schoolID string) ([]CanteenTenant, error) {
	raw, err := c.client.ListCanteenTenants(ctx, schoolID)
	if err != nil {
		return nil, err
	}
	out := make([]CanteenTenant, 0, len(raw))
	for _, t := range raw {
		out = append(out, CanteenTenant{
			ID:     t.TenantID,
			Name:   t.Name,
			Status: "active",
			IsOpen: t.IsOpen,
		})
	}
	return out, nil
}

func (c *canteenSourceAdapter) GetMenu(ctx context.Context, schoolID, tenantID string) ([]client.CanteenMenuItem, error) {
	return c.client.ListCanteenMenu(ctx, schoolID, tenantID)
}
