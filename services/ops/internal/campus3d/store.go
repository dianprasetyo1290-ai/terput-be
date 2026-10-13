package campus3d

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"skci/ops/internal/store"
)

var (
	ErrNotFound      = errors.New("data tidak ditemukan")
	ErrDuplicate     = errors.New("versi model sudah ada")
	ErrCannotDelete  = errors.New("hanya model berstatus draft yang dapat dihapus")
	ErrCannotPublish = errors.New("model tidak dapat dipublikasikan")
)

type Model struct {
	ID          string     `json:"id"`
	SchoolID    string     `json:"school_id"`
	BuildingID  string     `json:"building_id"`
	Version     int        `json:"version"`
	ObjectKey   string     `json:"object_key"`
	MIME        string     `json:"mime"`
	SizeBytes   int64      `json:"size_bytes"`
	SHA256      string     `json:"sha256"`
	NodeNames   []string   `json:"node_names"`
	Status      string     `json:"status"` // 'draft' | 'published' | 'archived'
	UploadedBy  string     `json:"uploaded_by"`
	PublishedBy *string    `json:"published_by,omitempty"`
	PublishedAt *time.Time `json:"published_at,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

type Store struct {
	DB *sql.DB
}

func NewStore(db *sql.DB) *Store {
	return &Store{DB: db}
}

func toTextArray(items []string) string {
	if len(items) == 0 {
		return "{}"
	}
	escaped := make([]string, len(items))
	for i, s := range items {
		escaped[i] = `"` + strings.ReplaceAll(strings.ReplaceAll(s, `\`, `\\`), `"`, `\"`) + `"`
	}
	return "{" + strings.Join(escaped, ",") + "}"
}

func (s *Store) inTx(ctx context.Context, fn func(tx *sql.Tx) error) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := fn(tx); err != nil {
		return err
	}
	return tx.Commit()
}

// NextVersion returns the next model version number for the given building.
func (s *Store) NextVersion(ctx context.Context, schoolID, buildingID string) (int, error) {
	var maxVer sql.NullInt64
	err := s.DB.QueryRowContext(ctx, `
		SELECT MAX(version) FROM campus3d_models
		WHERE school_id = $1 AND building_id = $2
	`, schoolID, buildingID).Scan(&maxVer)
	if err != nil {
		return 1, err
	}
	if !maxVer.Valid {
		return 1, nil
	}
	return int(maxVer.Int64) + 1, nil
}

// CreateModel inserts a new 3D model row.
func (s *Store) CreateModel(ctx context.Context, m Model, actorUserID, actorRole string) (string, error) {
	var id string
	nodeArr := toTextArray(m.NodeNames)
	err := s.inTx(ctx, func(tx *sql.Tx) error {
		err := tx.QueryRowContext(ctx, `
			INSERT INTO campus3d_models (
				school_id, building_id, version, object_key, mime, size_bytes,
				sha256, node_names, status, uploaded_by
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8::text[], $9, $10)
			RETURNING id
		`, m.SchoolID, m.BuildingID, m.Version, m.ObjectKey, m.MIME, m.SizeBytes,
			m.SHA256, nodeArr, m.Status, m.UploadedBy).Scan(&id)
		if err != nil {
			return err
		}

		return store.Audit(ctx, tx, store.AuditEntry{
			SchoolID:    m.SchoolID,
			Module:      "campus3d",
			EntityType:  "model",
			EntityID:    id,
			Action:      "upload",
			ActorUserID: &actorUserID,
			ActorRole:   &actorRole,
			After:       m,
		})
	})
	return id, err
}

// GetModel retrieves a model by ID.
func (s *Store) GetModel(ctx context.Context, schoolID, id string) (*Model, error) {
	var m Model
	var pubBy sql.NullString
	var pubAt sql.NullTime
	var nodesJSON string

	err := s.DB.QueryRowContext(ctx, `
		SELECT id, school_id, building_id, version, object_key, mime, size_bytes,
		       sha256, COALESCE(array_to_json(node_names)::text, '[]'), status, uploaded_by, published_by, published_at,
		       created_at, updated_at
		FROM campus3d_models
		WHERE id = $1 AND school_id = $2
	`, id, schoolID).Scan(
		&m.ID, &m.SchoolID, &m.BuildingID, &m.Version, &m.ObjectKey, &m.MIME, &m.SizeBytes,
		&m.SHA256, &nodesJSON, &m.Status, &m.UploadedBy, &pubBy, &pubAt,
		&m.CreatedAt, &m.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	_ = json.Unmarshal([]byte(nodesJSON), &m.NodeNames)
	if pubBy.Valid {
		m.PublishedBy = &pubBy.String
	}
	if pubAt.Valid {
		m.PublishedAt = &pubAt.Time
	}
	return &m, nil
}

// GetPublishedModel retrieves the active published model for a building.
func (s *Store) GetPublishedModel(ctx context.Context, schoolID, buildingID string) (*Model, error) {
	var m Model
	var pubBy sql.NullString
	var pubAt sql.NullTime
	var nodesJSON string

	err := s.DB.QueryRowContext(ctx, `
		SELECT id, school_id, building_id, version, object_key, mime, size_bytes,
		       sha256, COALESCE(array_to_json(node_names)::text, '[]'), status, uploaded_by, published_by, published_at,
		       created_at, updated_at
		FROM campus3d_models
		WHERE school_id = $1 AND building_id = $2 AND status = 'published'
	`, schoolID, buildingID).Scan(
		&m.ID, &m.SchoolID, &m.BuildingID, &m.Version, &m.ObjectKey, &m.MIME, &m.SizeBytes,
		&m.SHA256, &nodesJSON, &m.Status, &m.UploadedBy, &pubBy, &pubAt,
		&m.CreatedAt, &m.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	_ = json.Unmarshal([]byte(nodesJSON), &m.NodeNames)
	if pubBy.Valid {
		m.PublishedBy = &pubBy.String
	}
	if pubAt.Valid {
		m.PublishedAt = &pubAt.Time
	}
	return &m, nil
}

// ListModels retrieves all models for a building (or school).
func (s *Store) ListModels(ctx context.Context, schoolID, buildingID string) ([]Model, error) {
	query := `
		SELECT id, school_id, building_id, version, object_key, mime, size_bytes,
		       sha256, COALESCE(array_to_json(node_names)::text, '[]'), status, uploaded_by, published_by, published_at,
		       created_at, updated_at
		FROM campus3d_models
		WHERE school_id = $1
	`
	args := []any{schoolID}
	if buildingID != "" {
		query += " AND building_id = $2"
		args = append(args, buildingID)
	}
	query += " ORDER BY building_id, version DESC"

	rows, err := s.DB.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Model
	for rows.Next() {
		var m Model
		var pubBy sql.NullString
		var pubAt sql.NullTime
		var nodesJSON string

		if err := rows.Scan(
			&m.ID, &m.SchoolID, &m.BuildingID, &m.Version, &m.ObjectKey, &m.MIME, &m.SizeBytes,
			&m.SHA256, &nodesJSON, &m.Status, &m.UploadedBy, &pubBy, &pubAt,
			&m.CreatedAt, &m.UpdatedAt,
		); err != nil {
			return nil, err
		}
		_ = json.Unmarshal([]byte(nodesJSON), &m.NodeNames)
		if pubBy.Valid {
			m.PublishedBy = &pubBy.String
		}
		if pubAt.Valid {
			m.PublishedAt = &pubAt.Time
		}
		out = append(out, m)
	}
	return out, rows.Err()
}


// DeleteDraftModel removes a model if it is in draft status.
func (s *Store) DeleteDraftModel(ctx context.Context, schoolID, id, actorUserID, actorRole string) (string, error) {
	var objectKey string
	err := s.inTx(ctx, func(tx *sql.Tx) error {
		var status string
		err := tx.QueryRowContext(ctx, `
			SELECT status, object_key FROM campus3d_models
			WHERE id = $1 AND school_id = $2
			FOR UPDATE
		`, id, schoolID).Scan(&status, &objectKey)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		if status != "draft" {
			return ErrCannotDelete
		}

		_, err = tx.ExecContext(ctx, `DELETE FROM campus3d_models WHERE id = $1 AND school_id = $2`, id, schoolID)
		if err != nil {
			return err
		}

		return store.Audit(ctx, tx, store.AuditEntry{
			SchoolID:    schoolID,
			Module:      "campus3d",
			EntityType:  "model",
			EntityID:    id,
			Action:      "delete_draft",
			ActorUserID: &actorUserID,
			ActorRole:   &actorRole,
		})
	})
	return objectKey, err
}

// PublishModel archives previous published model and sets target model to published.
func (s *Store) PublishModel(ctx context.Context, schoolID, id, publishedBy, actorRole string) error {
	return s.inTx(ctx, func(tx *sql.Tx) error {
		var buildingID, status string
		err := tx.QueryRowContext(ctx, `
			SELECT building_id, status FROM campus3d_models
			WHERE id = $1 AND school_id = $2
			FOR UPDATE
		`, id, schoolID).Scan(&buildingID, &status)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		if status != "draft" {
			return ErrCannotPublish
		}

		// Archive any currently published model for this building
		_, err = tx.ExecContext(ctx, `
			UPDATE campus3d_models
			SET status = 'archived', updated_at = now()
			WHERE school_id = $1 AND building_id = $2 AND status = 'published'
		`, schoolID, buildingID)
		if err != nil {
			return err
		}

		// Mark target model published
		now := time.Now()
		_, err = tx.ExecContext(ctx, `
			UPDATE campus3d_models
			SET status = 'published', published_by = $3, published_at = $4, updated_at = now()
			WHERE id = $1 AND school_id = $2
		`, id, schoolID, publishedBy, now)
		if err != nil {
			return err
		}

		return store.Audit(ctx, tx, store.AuditEntry{
			SchoolID:    schoolID,
			Module:      "campus3d",
			EntityType:  "model",
			EntityID:    id,
			Action:      "publish",
			ActorUserID: &publishedBy,
			ActorRole:   &actorRole,
			After:       map[string]any{"status": "published", "published_at": now},
		})
	})
}

// GetModelMapping retrieves floor and room mappings for a model.
func (s *Store) GetModelMapping(ctx context.Context, schoolID, modelID string) (ModelMapping, error) {
	var buildingID string
	err := s.DB.QueryRowContext(ctx, `
		SELECT building_id FROM campus3d_models WHERE id = $1 AND school_id = $2
	`, modelID, schoolID).Scan(&buildingID)
	if errors.Is(err, sql.ErrNoRows) {
		return ModelMapping{}, ErrNotFound
	}
	if err != nil {
		return ModelMapping{}, err
	}

	mapping := ModelMapping{
		BuildingID: buildingID,
		Floors:     []FloorMapping{},
		Rooms:      []RoomMapping{},
	}

	// Fetch floors
	fRows, err := s.DB.QueryContext(ctx, `
		SELECT floor_id, node_name FROM campus3d_floor_nodes
		WHERE model_id = $1 AND school_id = $2
		ORDER BY node_name
	`, modelID, schoolID)
	if err != nil {
		return mapping, err
	}
	defer fRows.Close()
	for fRows.Next() {
		var fm FloorMapping
		if err := fRows.Scan(&fm.FloorID, &fm.NodeName); err != nil {
			return mapping, err
		}
		mapping.Floors = append(mapping.Floors, fm)
	}

	// Fetch rooms
	rRows, err := s.DB.QueryContext(ctx, `
		SELECT room_id, node_name FROM campus3d_room_nodes
		WHERE model_id = $1 AND school_id = $2
		ORDER BY node_name
	`, modelID, schoolID)
	if err != nil {
		return mapping, err
	}
	defer rRows.Close()
	for rRows.Next() {
		var rm RoomMapping
		if err := rRows.Scan(&rm.RoomID, &rm.NodeName); err != nil {
			return mapping, err
		}
		mapping.Rooms = append(mapping.Rooms, rm)
	}

	return mapping, nil
}

// SaveModelMapping replaces the floor and room mappings for a model.
func (s *Store) SaveModelMapping(ctx context.Context, schoolID, modelID string, floors []FloorMapping, rooms []RoomMapping, actorUserID, actorRole string) error {
	return s.inTx(ctx, func(tx *sql.Tx) error {
		// Verify model exists
		var status string
		err := tx.QueryRowContext(ctx, `
			SELECT status FROM campus3d_models WHERE id = $1 AND school_id = $2
		`, modelID, schoolID).Scan(&status)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}

		// Delete existing mappings
		if _, err := tx.ExecContext(ctx, `DELETE FROM campus3d_floor_nodes WHERE model_id = $1`, modelID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM campus3d_room_nodes WHERE model_id = $1`, modelID); err != nil {
			return err
		}

		// Insert floor mappings
		for _, fm := range floors {
			_, err := tx.ExecContext(ctx, `
				INSERT INTO campus3d_floor_nodes (school_id, model_id, floor_id, node_name)
				VALUES ($1, $2, $3, $4)
			`, schoolID, modelID, fm.FloorID, fm.NodeName)
			if err != nil {
				return err
			}
		}

		// Insert room mappings
		for _, rm := range rooms {
			_, err := tx.ExecContext(ctx, `
				INSERT INTO campus3d_room_nodes (school_id, model_id, room_id, node_name)
				VALUES ($1, $2, $3, $4)
			`, schoolID, modelID, rm.RoomID, rm.NodeName)
			if err != nil {
				return err
			}
		}

		return store.Audit(ctx, tx, store.AuditEntry{
			SchoolID:    schoolID,
			Module:      "campus3d",
			EntityType:  "mapping",
			EntityID:    modelID,
			Action:      "update_mapping",
			ActorUserID: &actorUserID,
			ActorRole:   &actorRole,
			After:       map[string]any{"floors_count": len(floors), "rooms_count": len(rooms)},
		})
	})
}

// ListTenantSpots returns all tenant-to-room spots for a school.
func (s *Store) ListTenantSpots(ctx context.Context, schoolID string) ([]TenantSpot, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT tenant_id, room_id FROM campus3d_tenant_spots
		WHERE school_id = $1
		ORDER BY created_at
	`, schoolID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []TenantSpot
	for rows.Next() {
		var ts TenantSpot
		if err := rows.Scan(&ts.TenantID, &ts.RoomID); err != nil {
			return nil, err
		}
		out = append(out, ts)
	}
	return out, rows.Err()
}

// SaveTenantSpots updates the tenant spots configuration for a school.
func (s *Store) SaveTenantSpots(ctx context.Context, schoolID string, spots []TenantSpot, actorUserID, actorRole string) error {
	return s.inTx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `DELETE FROM campus3d_tenant_spots WHERE school_id = $1`, schoolID); err != nil {
			return err
		}

		for _, sp := range spots {
			_, err := tx.ExecContext(ctx, `
				INSERT INTO campus3d_tenant_spots (school_id, tenant_id, room_id)
				VALUES ($1, $2, $3)
			`, schoolID, sp.TenantID, sp.RoomID)
			if err != nil {
				return err
			}
		}

		return store.Audit(ctx, tx, store.AuditEntry{
			SchoolID:    schoolID,
			Module:      "campus3d",
			EntityType:  "tenant_spots",
			EntityID:    schoolID,
			Action:      "update_tenant_spots",
			ActorUserID: &actorUserID,
			ActorRole:   &actorRole,
			After:       map[string]any{"spots_count": len(spots)},
		})
	})
}
