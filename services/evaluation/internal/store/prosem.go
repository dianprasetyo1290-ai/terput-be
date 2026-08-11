package store

import (
	"context"
	"database/sql"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"skci/evaluation/internal/model"
)

// ProsemStore: Program Semester, parent (prosem) + line-items
// (prosem_items). Same import-only-create pattern as ProtaStore.
type ProsemStore struct{ db *pgxpool.Pool }

func NewProsemStore(db *pgxpool.Pool) *ProsemStore { return &ProsemStore{db: db} }

func nullDate(s string) sql.NullTime {
	if s == "" {
		return sql.NullTime{}
	}
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		return sql.NullTime{}
	}
	return sql.NullTime{Time: t, Valid: true}
}

func (s *ProsemStore) Import(ctx context.Context, p *model.Prosem) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	row := tx.QueryRow(ctx, `
		INSERT INTO prosem (school_id, academic_year_id, guru_user_id, mata_pelajaran, kelas)
		VALUES ($1,$2,$3,$4,$5)
		RETURNING id, created_at, updated_at`,
		p.SchoolID, p.AcademicYearID, p.GuruUserID, p.MataPelajaran, p.Kelas)
	if err := row.Scan(&p.ID, &p.CreatedAt, &p.UpdatedAt); err != nil {
		return err
	}
	for i := range p.Items {
		it := &p.Items[i]
		if err := tx.QueryRow(ctx, `
			INSERT INTO prosem_items (prosem_id, no, materi_pokok_aktivitas, alokasi_jp, bulan_target, minggu_ke, estimasi_tanggal, tipe_aktivitas_lms, status_input_guru)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9) RETURNING id`,
			p.ID, it.No, it.MateriPokokAktivitas, it.AlokasiJP, it.BulanTarget, it.MingguKe, nullDate(it.EstimasiTanggal), it.TipeAktivitasLMS, it.StatusInputGuru,
		).Scan(&it.ID); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

const prosemCols = `id, school_id, academic_year_id, guru_user_id, mata_pelajaran, kelas, created_at, updated_at`

func (s *ProsemStore) scanItems(ctx context.Context, prosemID string) ([]model.ProsemItem, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id, no, materi_pokok_aktivitas, alokasi_jp, bulan_target, minggu_ke, estimasi_tanggal, tipe_aktivitas_lms, status_input_guru
		FROM prosem_items WHERE prosem_id=$1 ORDER BY no`, prosemID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.ProsemItem
	for rows.Next() {
		var it model.ProsemItem
		var dt sql.NullTime
		if err := rows.Scan(&it.ID, &it.No, &it.MateriPokokAktivitas, &it.AlokasiJP, &it.BulanTarget, &it.MingguKe, &dt, &it.TipeAktivitasLMS, &it.StatusInputGuru); err != nil {
			return nil, err
		}
		if dt.Valid {
			it.EstimasiTanggal = dt.Time.Format("2006-01-02")
		}
		out = append(out, it)
	}
	return out, rows.Err()
}

func (s *ProsemStore) GetByID(ctx context.Context, id, schoolID string) (*model.Prosem, error) {
	var p model.Prosem
	row := s.db.QueryRow(ctx, `SELECT `+prosemCols+` FROM prosem WHERE id=$1 AND school_id=$2 AND deleted_at IS NULL`, id, schoolID)
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

func (s *ProsemStore) ListByGuru(ctx context.Context, guruUserID, schoolID string) ([]*model.Prosem, error) {
	rows, err := s.db.Query(ctx, `SELECT `+prosemCols+` FROM prosem WHERE guru_user_id=$1 AND school_id=$2 AND deleted_at IS NULL ORDER BY updated_at DESC`, guruUserID, schoolID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*model.Prosem
	for rows.Next() {
		var p model.Prosem
		if err := rows.Scan(&p.ID, &p.SchoolID, &p.AcademicYearID, &p.GuruUserID, &p.MataPelajaran, &p.Kelas, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, &p)
	}
	return out, rows.Err()
}

func (s *ProsemStore) ListAll(ctx context.Context, schoolID string, guruUserID *string) ([]*model.Prosem, error) {
	base := `FROM prosem WHERE school_id=$1 AND deleted_at IS NULL`
	args := []interface{}{schoolID}
	if guruUserID != nil {
		base += ` AND guru_user_id=$2`
		args = append(args, *guruUserID)
	}
	rows, err := s.db.Query(ctx, `SELECT `+prosemCols+` `+base+` ORDER BY updated_at DESC`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*model.Prosem
	for rows.Next() {
		var p model.Prosem
		if err := rows.Scan(&p.ID, &p.SchoolID, &p.AcademicYearID, &p.GuruUserID, &p.MataPelajaran, &p.Kelas, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, &p)
	}
	return out, rows.Err()
}

func (s *ProsemStore) Delete(ctx context.Context, id, guruUserID string) (bool, error) {
	tag, err := s.db.Exec(ctx, `UPDATE prosem SET deleted_at=now() WHERE id=$1 AND guru_user_id=$2 AND deleted_at IS NULL`, id, guruUserID)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}
