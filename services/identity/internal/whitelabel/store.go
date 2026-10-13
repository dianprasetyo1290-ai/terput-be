package whitelabel

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
)

var ErrNotFound = errors.New("tidak ditemukan")
var ErrConflict = errors.New("konflik data")

type Store struct {
	db *sql.DB
}

func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}

func (s *Store) IsFeatureActive(ctx context.Context, schoolID, featureKey string) (bool, error) {
	var enabled bool
	err := s.db.QueryRowContext(ctx, `
		SELECT enabled FROM school_features
		WHERE school_id = $1 AND feature_key = $2
	`, schoolID, featureKey).Scan(&enabled)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return enabled, err
}

func (s *Store) GetSchoolFoundationID(ctx context.Context, schoolID string) (string, error) {
	var foundationID sql.NullString
	err := s.db.QueryRowContext(ctx, `
		SELECT foundation_id FROM schools WHERE id = $1
	`, schoolID).Scan(&foundationID)
	if err != nil {
		return "", err
	}
	if !foundationID.Valid {
		return "", nil
	}
	return foundationID.String, nil
}

func (s *Store) GetDraft(ctx context.Context, schoolID string) (*BrandingDraft, error) {
	var b BrandingDraft
	var portalsJSON string
	err := s.db.QueryRowContext(ctx, `
		SELECT id, school_id, app_name, short_name, theme_color, background_color,
		       icon_192, icon_512, icon_maskable, splash, domain,
		       COALESCE(array_to_json(included_portals)::text, '[]'),
		       privacy_policy_url, support_email, updated_by, created_at, updated_at
		FROM app_brandings
		WHERE school_id = $1
	`, schoolID).Scan(
		&b.ID, &b.SchoolID, &b.AppName, &b.ShortName, &b.ThemeColor, &b.BackgroundColor,
		&b.Icon192, &b.Icon512, &b.IconMaskable, &b.Splash, &b.Domain,
		&portalsJSON,
		&b.PrivacyPolicyURL, &b.SupportEmail, &b.UpdatedBy, &b.CreatedAt, &b.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	_ = json.Unmarshal([]byte(portalsJSON), &b.IncludedPortals)
	return &b, nil
}

func (s *Store) UpsertDraft(ctx context.Context, draft BrandingDraft, updatedBy string) (*BrandingDraft, error) {
	portals := "{parent,student,teacher}"
	if len(draft.IncludedPortals) > 0 {
		portals = "{" + strings.Join(draft.IncludedPortals, ",") + "}"
	}

	var b BrandingDraft
	var portalsJSON string
	err := s.db.QueryRowContext(ctx, `
		INSERT INTO app_brandings (
			school_id, app_name, short_name, theme_color, background_color,
			icon_192, icon_512, icon_maskable, splash,
			included_portals, privacy_policy_url, support_email, updated_by, updated_at
		) VALUES (
			$1, $2, $3, $4, $5,
			$6, $7, $8, $9,
			$10::text[], $11, $12, $13, now()
		)
		ON CONFLICT (school_id) DO UPDATE SET
			app_name = EXCLUDED.app_name,
			short_name = EXCLUDED.short_name,
			theme_color = EXCLUDED.theme_color,
			background_color = EXCLUDED.background_color,
			icon_192 = COALESCE(EXCLUDED.icon_192, app_brandings.icon_192),
			icon_512 = COALESCE(EXCLUDED.icon_512, app_brandings.icon_512),
			icon_maskable = COALESCE(EXCLUDED.icon_maskable, app_brandings.icon_maskable),
			splash = COALESCE(EXCLUDED.splash, app_brandings.splash),
			included_portals = EXCLUDED.included_portals,
			privacy_policy_url = EXCLUDED.privacy_policy_url,
			support_email = EXCLUDED.support_email,
			updated_by = EXCLUDED.updated_by,
			updated_at = now()
		RETURNING id, school_id, app_name, short_name, theme_color, background_color,
		          icon_192, icon_512, icon_maskable, splash, domain,
		          COALESCE(array_to_json(included_portals)::text, '[]'),
		          privacy_policy_url, support_email, updated_by, created_at, updated_at
	`,
		draft.SchoolID, draft.AppName, draft.ShortName, draft.ThemeColor, draft.BackgroundColor,
		draft.Icon192, draft.Icon512, draft.IconMaskable, draft.Splash,
		portals, draft.PrivacyPolicyURL, draft.SupportEmail, updatedBy,
	).Scan(
		&b.ID, &b.SchoolID, &b.AppName, &b.ShortName, &b.ThemeColor, &b.BackgroundColor,
		&b.Icon192, &b.Icon512, &b.IconMaskable, &b.Splash, &b.Domain,
		&portalsJSON,
		&b.PrivacyPolicyURL, &b.SupportEmail, &b.UpdatedBy, &b.CreatedAt, &b.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	_ = json.Unmarshal([]byte(portalsJSON), &b.IncludedPortals)
	return &b, nil
}

func (s *Store) UpdateDomain(ctx context.Context, schoolID, domain string) error {
	var domArg *string
	clean := strings.ToLower(strings.TrimSpace(domain))
	if clean != "" {
		domArg = &clean
	}

	_, err := s.db.ExecContext(ctx, `
		UPDATE app_brandings
		SET domain = $1, updated_at = now()
		WHERE school_id = $2
	`, domArg, schoolID)
	if err != nil {
		if strings.Contains(err.Error(), "unique") || strings.Contains(err.Error(), "duplicate") {
			return ErrConflict
		}
		return err
	}
	return nil
}

func (s *Store) Publish(ctx context.Context, schoolID, publishedBy string) (*BrandingVersion, []FieldError, error) {
	draft, err := s.GetDraft(ctx, schoolID)
	if err != nil {
		return nil, nil, err
	}

	valErrors := ValidateBranding(*draft, true)
	if len(valErrors) > 0 {
		return nil, valErrors, nil
	}

	// Prepare snapshot (excluding large data URLs)
	snapshotMap := map[string]any{
		"app_name":           draft.AppName,
		"short_name":         draft.ShortName,
		"theme_color":        draft.ThemeColor,
		"background_color":   draft.BackgroundColor,
		"domain":             draft.Domain,
		"included_portals":   draft.IncludedPortals,
		"privacy_policy_url": draft.PrivacyPolicyURL,
		"support_email":      draft.SupportEmail,
	}
	snapshotBytes, _ := json.Marshal(snapshotMap)

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, nil, err
	}
	defer tx.Rollback()

	var nextVersion int
	err = tx.QueryRowContext(ctx, `
		SELECT COALESCE(MAX(version), 0) + 1
		FROM app_branding_versions
		WHERE school_id = $1
	`, schoolID).Scan(&nextVersion)
	if err != nil {
		return nil, nil, err
	}

	var bv BrandingVersion
	err = tx.QueryRowContext(ctx, `
		INSERT INTO app_branding_versions (
			school_id, version, snapshot,
			icon_192, icon_512, icon_maskable, splash,
			published_by, published_at
		) VALUES (
			$1, $2, $3,
			$4, $5, $6, $7,
			$8, now()
		)
		RETURNING id, school_id, version, snapshot, icon_192, icon_512, icon_maskable, splash, published_by, published_at
	`,
		schoolID, nextVersion, snapshotBytes,
		draft.Icon192, draft.Icon512, draft.IconMaskable, draft.Splash,
		publishedBy,
	).Scan(
		&bv.ID, &bv.SchoolID, &bv.Version, &bv.Snapshot,
		&bv.Icon192, &bv.Icon512, &bv.IconMaskable, &bv.Splash,
		&bv.PublishedBy, &bv.PublishedAt,
	)
	if err != nil {
		return nil, nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, nil, err
	}

	return &bv, nil, nil
}

func (s *Store) ListVersions(ctx context.Context, schoolID string) ([]BrandingVersion, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, school_id, version, snapshot,
		       icon_192 IS NOT NULL, icon_512 IS NOT NULL,
		       published_by, published_at
		FROM app_branding_versions
		WHERE school_id = $1
		ORDER BY version DESC
	`, schoolID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []BrandingVersion
	for rows.Next() {
		var v BrandingVersion
		var has192, has512 bool
		if err := rows.Scan(
			&v.ID, &v.SchoolID, &v.Version, &v.Snapshot,
			&has192, &has512,
			&v.PublishedBy, &v.PublishedAt,
		); err != nil {
			return nil, err
		}
		list = append(list, v)
	}
	return list, rows.Err()
}

func (s *Store) GetPublishedBySchool(ctx context.Context, schoolID string) (*BrandingDraft, int, error) {
	var version int
	var snapshotBytes []byte
	var icon192, icon512, iconMaskable, splash *string
	err := s.db.QueryRowContext(ctx, `
		SELECT version, snapshot, icon_192, icon_512, icon_maskable, splash
		FROM app_branding_versions
		WHERE school_id = $1
		ORDER BY version DESC
		LIMIT 1
	`, schoolID).Scan(&version, &snapshotBytes, &icon192, &icon512, &iconMaskable, &splash)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, 0, ErrNotFound
	}
	if err != nil {
		return nil, 0, err
	}

	var d BrandingDraft
	if err := json.Unmarshal(snapshotBytes, &d); err != nil {
		return nil, 0, err
	}
	d.SchoolID = schoolID
	d.Icon192 = icon192
	d.Icon512 = icon512
	d.IconMaskable = iconMaskable
	d.Splash = splash

	return &d, version, nil
}

func (s *Store) GetPublishedByHost(ctx context.Context, host string) (*BrandingDraft, int, error) {
	norm := NormalizeHost(host)
	if norm == "" {
		return nil, 0, ErrNotFound
	}

	var schoolID string
	err := s.db.QueryRowContext(ctx, `
		SELECT b.school_id
		FROM app_brandings b
		JOIN school_features f ON f.school_id = b.school_id AND f.feature_key = 'white_label' AND f.enabled = true
		WHERE b.domain = $1
	`, norm).Scan(&schoolID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, 0, ErrNotFound
	}
	if err != nil {
		return nil, 0, err
	}

	return s.GetPublishedBySchool(ctx, schoolID)
}

func (s *Store) GetStoreListing(ctx context.Context, schoolID, store string) (*StoreListing, error) {
	var sl StoreListing
	var fprintsJSON string
	err := s.db.QueryRowContext(ctx, `
		SELECT id, school_id, store, account_owner, package_id,
		       COALESCE(array_to_json(sha256_fingerprints)::text, '[]'),
		       status, listing_url, note, updated_by, created_at, updated_at
		FROM app_store_listings
		WHERE school_id = $1 AND store = $2
	`, schoolID, store).Scan(
		&sl.ID, &sl.SchoolID, &sl.Store, &sl.AccountOwner, &sl.PackageID,
		&fprintsJSON,
		&sl.Status, &sl.ListingURL, &sl.Note, &sl.UpdatedBy, &sl.CreatedAt, &sl.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	_ = json.Unmarshal([]byte(fprintsJSON), &sl.SHA256Fingerprints)
	return &sl, nil
}

func (s *Store) UpsertStoreListing(ctx context.Context, sl StoreListing, updatedBy string) (*StoreListing, error) {
	fprints := "{}"
	if len(sl.SHA256Fingerprints) > 0 {
		fprints = "{" + strings.Join(sl.SHA256Fingerprints, ",") + "}"
	}

	var out StoreListing
	var fprintsJSON string
	err := s.db.QueryRowContext(ctx, `
		INSERT INTO app_store_listings (
			school_id, store, account_owner, package_id, sha256_fingerprints,
			status, listing_url, note, updated_by, updated_at
		) VALUES (
			$1, $2, $3, $4, $5::text[],
			$6, $7, $8, $9, now()
		)
		ON CONFLICT (school_id, store) DO UPDATE SET
			account_owner = EXCLUDED.account_owner,
			package_id = EXCLUDED.package_id,
			sha256_fingerprints = EXCLUDED.sha256_fingerprints,
			status = EXCLUDED.status,
			listing_url = EXCLUDED.listing_url,
			note = EXCLUDED.note,
			updated_by = EXCLUDED.updated_by,
			updated_at = now()
		RETURNING id, school_id, store, account_owner, package_id,
		          COALESCE(array_to_json(sha256_fingerprints)::text, '[]'),
		          status, listing_url, note, updated_by, created_at, updated_at
	`,
		sl.SchoolID, sl.Store, sl.AccountOwner, sl.PackageID, fprints,
		sl.Status, sl.ListingURL, sl.Note, updatedBy,
	).Scan(
		&out.ID, &out.SchoolID, &out.Store, &out.AccountOwner, &out.PackageID,
		&fprintsJSON,
		&out.Status, &out.ListingURL, &out.Note, &out.UpdatedBy, &out.CreatedAt, &out.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	_ = json.Unmarshal([]byte(fprintsJSON), &out.SHA256Fingerprints)
	return &out, nil
}
