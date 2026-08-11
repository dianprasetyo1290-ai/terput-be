package store

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
	"skci/evaluation/internal/model"
)

// ProtaStore: Program Tahunan, parent (prota) + line-items (prota_items).
// Same shape/pattern as LessonPlanStore (guru-owned CRUD-ish, admin
// read-only), except creation is import-only (a whole parent+items set is
// written in one transaction from the Excel import, not built field-by-field
// via a form) -- see internal/handler/prota.go.
type ProtaStore struct{ db *pgxpool.Pool }

func NewProtaStore(db *pgxpool.Pool) *ProtaStore { return &ProtaStore{db: db} }

// Import creates a new Prota + its items in one transaction (the Excel
// import always creates a fresh record; editing an existing Prota is
// "delete and re-import", there's no partial-update requirement in spec).
func (s *ProtaStore) Import(ctx context.Context, p *model.Prota) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	row := tx.QueryRow(ctx, `
		INSERT INTO prota (school_id, academic_year_id, guru_user_id, mata_pelajaran, kelas)
		VALUES ($1,$2,$3,$4,$5)
		RETURNING id, created_at, updated_at`,
		p.SchoolID, p.AcademicYearID, p.GuruUserID, p.MataPelajaran, p.Kelas)
	if err := row.Scan(&p.ID, &p.CreatedAt, &p.UpdatedAt); err != nil {
		return err
	}
	for i := range p.Items {
		it := &p.Items[i]
		if err := tx.QueryRow(ctx, `
			INSERT INTO prota_items (prota_id, no, semester, elemen_kode_bab, kompetensi_dasar_tp, alokasi_waktu_jp, keterangan_target_asesmen)
			VALUES ($1,$2,$3,$4,$5,$6,$7) RETURNING id`,
			p.ID, it.No, it.Semester, it.ElemenKodeBab, it.KompetensiDasarTP, it.AlokasiWaktuJP, it.KeteranganTargetAsesmen,
		).Scan(&it.ID); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

const protaCols = `id, school_id, academic_year_id, guru_user_id, mata_pelajaran, kelas, created_at, updated_at`

func (s *ProtaStore) scanItems(ctx context.Context, protaID string) ([]model.ProtaItem, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id, no, semester, elemen_kode_bab, kompetensi_dasar_tp, alokasi_waktu_jp, keterangan_target_asesmen
		FROM prota_items WHERE prota_id=$1 ORDER BY no`, protaID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.ProtaItem
	for rows.Next() {
		var it model.ProtaItem
		if err := rows.Scan(&it.ID, &it.No, &it.Semester, &it.ElemenKodeBab, &it.KompetensiDasarTP, &it.AlokasiWaktuJP, &it.KeteranganTargetAsesmen); err != nil {
			return nil, err
		}
		out = append(out, it)
	}
	return out, rows.Err()
}

func (s *ProtaStore) GetByID(ctx context.Context, id, schoolID string) (*model.Prota, error) {
	var p model.Prota
	row := s.db.QueryRow(ctx, `SELECT `+protaCols+` FROM prota WHERE id=$1 AND school_id=$2 AND deleted_at IS NULL`, id, schoolID)
	if err := row.Scan(&p.ID, &p.SchoolID, &p.AcademicYearID, &p.GuruUserID, &p.MataPelajaran, &p.Kelas, &p.CreatedAt, &p.UpdatedAt); err != nil {
		return nil, err
	}
	items, err := s.scanItems(ctx, p.ID)
	if err != nil {
		return nil, err
	}
	p.Items = items
	return &p, nil
}

func (s *ProtaStore) ListByGuru(ctx context.Context, guruUserID, schoolID string) ([]*model.Prota, error) {
	rows, err := s.db.Query(ctx, `SELECT `+protaCols+` FROM prota WHERE guru_user_id=$1 AND school_id=$2 AND deleted_at IS NULL ORDER BY updated_at DESC`, guruUserID, schoolID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*model.Prota
	for rows.Next() {
		var p model.Prota
		if err := rows.Scan(&p.ID, &p.SchoolID, &p.AcademicYearID, &p.GuruUserID, &p.MataPelajaran, &p.Kelas, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, &p)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	// list views don't need items (avoid N+1 for a table row); GetByID loads them for print/detail.
	return out, nil
}

// ListAll is the admin read-only view: all guru's Prota in the school (optionally filter by guru_id).
func (s *ProtaStore) ListAll(ctx context.Context, schoolID string, guruUserID *string) ([]*model.Prota, error) {
	base := `FROM prota WHERE school_id=$1 AND deleted_at IS NULL`
	args := []interface{}{schoolID}
	if guruUserID != nil {
		base += ` AND guru_user_id=$2`
		args = append(args, *guruUserID)
	}
	rows, err := s.db.Query(ctx, `SELECT `+protaCols+` `+base+` ORDER BY updated_at DESC`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*model.Prota
	for rows.Next() {
		var p model.Prota
		if err := rows.Scan(&p.ID, &p.SchoolID, &p.AcademicYearID, &p.GuruUserID, &p.MataPelajaran, &p.Kelas, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, &p)
	}
	return out, rows.Err()
}

func (s *ProtaStore) Delete(ctx context.Context, id, guruUserID string) (bool, error) {
	tag, err := s.db.Exec(ctx, `UPDATE prota SET deleted_at=now() WHERE id=$1 AND guru_user_id=$2 AND deleted_at IS NULL`, id, guruUserID)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}
