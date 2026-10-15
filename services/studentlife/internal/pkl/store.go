package pkl

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"skci/studentlife/internal/store"
)

var (
	ErrNotFound       = store.ErrNotFound
	ErrAlreadyExists  = store.ErrAlreadyExists
	ErrSchemeLocked   = errors.New("skema penilaian sudah dikunci karena ada penilaian yang tersimpan")
	ErrAlreadyScored  = errors.New("absensi untuk hari ini sudah dicatat")
)

type Store struct {
	DB *sql.DB
}

func NewStore(db *sql.DB) *Store {
	return &Store{DB: db}
}

// ---------------------------------------------------------------------------
// 1. Majors & Classroom-Majors
// ---------------------------------------------------------------------------

func (s *Store) ListMajors(ctx context.Context, schoolID string) ([]Major, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT id, school_id, code, name, active, created_at, deleted_at
		FROM majors
		WHERE school_id = $1 AND deleted_at IS NULL
		ORDER BY code ASC`, schoolID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []Major
	for rows.Next() {
		var m Major
		if err := rows.Scan(&m.ID, &m.SchoolID, &m.Code, &m.Name, &m.Active, &m.CreatedAt, &m.DeletedAt); err != nil {
			return nil, err
		}
		list = append(list, m)
	}
	return list, nil
}

func (s *Store) GetMajor(ctx context.Context, schoolID, id string) (*Major, error) {
	var m Major
	err := s.DB.QueryRowContext(ctx, `
		SELECT id, school_id, code, name, active, created_at, deleted_at
		FROM majors
		WHERE school_id = $1 AND id = $2 AND deleted_at IS NULL`, schoolID, id).
		Scan(&m.ID, &m.SchoolID, &m.Code, &m.Name, &m.Active, &m.CreatedAt, &m.DeletedAt)
	if err != nil {
		return nil, store.One(err)
	}
	return &m, nil
}

func (s *Store) CreateMajor(ctx context.Context, m Major) (*Major, error) {
	if m.ID == "" {
		m.ID = store.NewID()
	}
	m.CreatedAt = time.Now()
	_, err := s.DB.ExecContext(ctx, `
		INSERT INTO majors (id, school_id, code, name, active, created_at)
		VALUES ($1, $2, $3, $4, $5, $6)`,
		m.ID, m.SchoolID, m.Code, m.Name, m.Active, m.CreatedAt)
	if err != nil {
		if strings.Contains(err.Error(), "uq_majors_school_code") || strings.Contains(err.Error(), "unique") {
			return nil, ErrAlreadyExists
		}
		return nil, err
	}
	return &m, nil
}

func (s *Store) UpdateMajor(ctx context.Context, m Major) (*Major, error) {
	res, err := s.DB.ExecContext(ctx, `
		UPDATE majors
		SET code = $3, name = $4, active = $5
		WHERE school_id = $1 AND id = $2 AND deleted_at IS NULL`,
		m.SchoolID, m.ID, m.Code, m.Name, m.Active)
	if err != nil {
		if strings.Contains(err.Error(), "uq_majors_school_code") || strings.Contains(err.Error(), "unique") {
			return nil, ErrAlreadyExists
		}
		return nil, err
	}
	rows, _ := res.RowsAffected()
	if rows == 0 {
		return nil, ErrNotFound
	}
	return s.GetMajor(ctx, m.SchoolID, m.ID)
}

func (s *Store) DeleteMajor(ctx context.Context, schoolID, id string) error {
	res, err := s.DB.ExecContext(ctx, `
		UPDATE majors
		SET deleted_at = now()
		WHERE school_id = $1 AND id = $2 AND deleted_at IS NULL`, schoolID, id)
	if err != nil {
		return err
	}
	rows, _ := res.RowsAffected()
	if rows == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) ListClassroomMajors(ctx context.Context, schoolID string) ([]ClassroomMajor, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT cm.classroom_id, cm.school_id, cm.major_id, m.name, m.code
		FROM classroom_majors cm
		JOIN majors m ON cm.major_id = m.id
		WHERE cm.school_id = $1 AND m.deleted_at IS NULL
		ORDER BY m.name ASC`, schoolID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []ClassroomMajor
	for rows.Next() {
		var cm ClassroomMajor
		if err := rows.Scan(&cm.ClassroomID, &cm.SchoolID, &cm.MajorID, &cm.MajorName, &cm.MajorCode); err != nil {
			return nil, err
		}
		list = append(list, cm)
	}
	return list, nil
}

func (s *Store) PutClassroomMajors(ctx context.Context, schoolID string, items []ClassroomMajor) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	for _, item := range items {
		if item.MajorID == "" {
			// Remove mapping
			_, err = tx.ExecContext(ctx, `DELETE FROM classroom_majors WHERE school_id = $1 AND classroom_id = $2`, schoolID, item.ClassroomID)
		} else {
			_, err = tx.ExecContext(ctx, `
				INSERT INTO classroom_majors (classroom_id, school_id, major_id)
				VALUES ($1, $2, $3)
				ON CONFLICT (classroom_id) DO UPDATE SET major_id = EXCLUDED.major_id`,
				item.ClassroomID, schoolID, item.MajorID)
		}
		if err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) GetMajorByClassroom(ctx context.Context, schoolID, classroomID string) (*Major, error) {
	var m Major
	err := s.DB.QueryRowContext(ctx, `
		SELECT m.id, m.school_id, m.code, m.name, m.active, m.created_at, m.deleted_at
		FROM classroom_majors cm
		JOIN majors m ON cm.major_id = m.id
		WHERE cm.school_id = $1 AND cm.classroom_id = $2 AND m.deleted_at IS NULL`, schoolID, classroomID).
		Scan(&m.ID, &m.SchoolID, &m.Code, &m.Name, &m.Active, &m.CreatedAt, &m.DeletedAt)
	if err != nil {
		return nil, store.One(err)
	}
	return &m, nil
}

// ---------------------------------------------------------------------------
// 2. Partners & MoUs
// ---------------------------------------------------------------------------

func (s *Store) ListPartners(ctx context.Context, schoolID string) ([]Partner, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT p.id, p.school_id, p.name, p.field, p.address, p.contact_name, p.contact_phone, p.contact_email,
		       p.active, p.created_at, p.deleted_at
		FROM pkl_partners p
		WHERE p.school_id = $1 AND p.deleted_at IS NULL
		ORDER BY p.name ASC`, schoolID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []Partner
	for rows.Next() {
		var p Partner
		if err := rows.Scan(&p.ID, &p.SchoolID, &p.Name, &p.Field, &p.Address, &p.ContactName, &p.ContactPhone,
			&p.ContactEmail, &p.Active, &p.CreatedAt, &p.DeletedAt); err != nil {
			return nil, err
		}
		list = append(list, p)
	}

	// Compute MoU status for each partner
	today := time.Now()
	for i := range list {
		mous, err := s.ListPartnerMous(ctx, schoolID, list[i].ID)
		if err != nil || len(mous) == 0 {
			list[i].MouStatus = "belum_ada"
			continue
		}
		hasValid := false
		for _, mou := range mous {
			if !mou.ValidFrom.After(today) && !mou.ValidTo.Before(today) {
				hasValid = true
				break
			}
		}
		if hasValid {
			list[i].MouStatus = "berlaku"
		} else {
			list[i].MouStatus = "berakhir"
		}
	}

	return list, nil
}

func (s *Store) GetPartner(ctx context.Context, schoolID, id string) (*Partner, error) {
	var p Partner
	err := s.DB.QueryRowContext(ctx, `
		SELECT id, school_id, name, field, address, contact_name, contact_phone, contact_email,
		       active, created_at, deleted_at
		FROM pkl_partners
		WHERE school_id = $1 AND id = $2 AND deleted_at IS NULL`, schoolID, id).
		Scan(&p.ID, &p.SchoolID, &p.Name, &p.Field, &p.Address, &p.ContactName, &p.ContactPhone,
			&p.ContactEmail, &p.Active, &p.CreatedAt, &p.DeletedAt)
	if err != nil {
		return nil, store.One(err)
	}

	mous, err := s.ListPartnerMous(ctx, schoolID, p.ID)
	if err == nil && len(mous) > 0 {
		today := time.Now()
		hasValid := false
		for _, mou := range mous {
			if !mou.ValidFrom.After(today) && !mou.ValidTo.Before(today) {
				hasValid = true
				break
			}
		}
		if hasValid {
			p.MouStatus = "berlaku"
		} else {
			p.MouStatus = "berakhir"
		}
	} else {
		p.MouStatus = "belum_ada"
	}

	return &p, nil
}

func (s *Store) CreatePartner(ctx context.Context, p Partner) (*Partner, error) {
	if p.ID == "" {
		p.ID = store.NewID()
	}
	p.CreatedAt = time.Now()
	_, err := s.DB.ExecContext(ctx, `
		INSERT INTO pkl_partners (id, school_id, name, field, address, contact_name, contact_phone, contact_email, active, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`,
		p.ID, p.SchoolID, p.Name, p.Field, p.Address, p.ContactName, p.ContactPhone, p.ContactEmail, p.Active, p.CreatedAt)
	if err != nil {
		return nil, err
	}
	p.MouStatus = "belum_ada"
	return &p, nil
}

func (s *Store) UpdatePartner(ctx context.Context, p Partner) (*Partner, error) {
	res, err := s.DB.ExecContext(ctx, `
		UPDATE pkl_partners
		SET name = $3, field = $4, address = $5, contact_name = $6, contact_phone = $7, contact_email = $8, active = $9
		WHERE school_id = $1 AND id = $2 AND deleted_at IS NULL`,
		p.SchoolID, p.ID, p.Name, p.Field, p.Address, p.ContactName, p.ContactPhone, p.ContactEmail, p.Active)
	if err != nil {
		return nil, err
	}
	rows, _ := res.RowsAffected()
	if rows == 0 {
		return nil, ErrNotFound
	}
	return s.GetPartner(ctx, p.SchoolID, p.ID)
}

func (s *Store) DeletePartner(ctx context.Context, schoolID, id string) error {
	res, err := s.DB.ExecContext(ctx, `
		UPDATE pkl_partners
		SET deleted_at = now()
		WHERE school_id = $1 AND id = $2 AND deleted_at IS NULL`, schoolID, id)
	if err != nil {
		return err
	}
	rows, _ := res.RowsAffected()
	if rows == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) ListPartnerMous(ctx context.Context, schoolID, partnerID string) ([]PartnerMou, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT id, school_id, partner_id, mou_number, valid_from, valid_to, object_key, file_name, mime, uploaded_by, created_at
		FROM pkl_partner_mous
		WHERE school_id = $1 AND partner_id = $2
		ORDER BY valid_to DESC, created_at DESC`, schoolID, partnerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []PartnerMou
	for rows.Next() {
		var m PartnerMou
		if err := rows.Scan(&m.ID, &m.SchoolID, &m.PartnerID, &m.MouNumber, &m.ValidFrom, &m.ValidTo,
			&m.ObjectKey, &m.FileName, &m.Mime, &m.UploadedBy, &m.CreatedAt); err != nil {
			return nil, err
		}
		list = append(list, m)
	}
	return list, nil
}

func (s *Store) GetPartnerMou(ctx context.Context, schoolID, id string) (*PartnerMou, error) {
	var m PartnerMou
	err := s.DB.QueryRowContext(ctx, `
		SELECT id, school_id, partner_id, mou_number, valid_from, valid_to, object_key, file_name, mime, uploaded_by, created_at
		FROM pkl_partner_mous
		WHERE school_id = $1 AND id = $2`, schoolID, id).
		Scan(&m.ID, &m.SchoolID, &m.PartnerID, &m.MouNumber, &m.ValidFrom, &m.ValidTo,
			&m.ObjectKey, &m.FileName, &m.Mime, &m.UploadedBy, &m.CreatedAt)
	if err != nil {
		return nil, store.One(err)
	}
	return &m, nil
}

func (s *Store) CreatePartnerMou(ctx context.Context, mou PartnerMou) (*PartnerMou, error) {
	if mou.ID == "" {
		mou.ID = store.NewID()
	}
	mou.CreatedAt = time.Now()
	_, err := s.DB.ExecContext(ctx, `
		INSERT INTO pkl_partner_mous (id, school_id, partner_id, mou_number, valid_from, valid_to, object_key, file_name, mime, uploaded_by, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`,
		mou.ID, mou.SchoolID, mou.PartnerID, mou.MouNumber, mou.ValidFrom, mou.ValidTo,
		mou.ObjectKey, mou.FileName, mou.Mime, mou.UploadedBy, mou.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &mou, nil
}

// ---------------------------------------------------------------------------
// 3. Periods & Quotas
// ---------------------------------------------------------------------------

func (s *Store) ListPeriods(ctx context.Context, schoolID string) ([]Period, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT id, school_id, academic_year_id, name, starts_on, ends_on, status, created_at
		FROM pkl_periods
		WHERE school_id = $1
		ORDER BY starts_on DESC`, schoolID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []Period
	for rows.Next() {
		var p Period
		if err := rows.Scan(&p.ID, &p.SchoolID, &p.AcademicYearID, &p.Name, &p.StartsOn, &p.EndsOn, &p.Status, &p.CreatedAt); err != nil {
			return nil, err
		}
		list = append(list, p)
	}
	return list, nil
}

func (s *Store) GetPeriod(ctx context.Context, schoolID, id string) (*Period, error) {
	var p Period
	err := s.DB.QueryRowContext(ctx, `
		SELECT id, school_id, academic_year_id, name, starts_on, ends_on, status, created_at
		FROM pkl_periods
		WHERE school_id = $1 AND id = $2`, schoolID, id).
		Scan(&p.ID, &p.SchoolID, &p.AcademicYearID, &p.Name, &p.StartsOn, &p.EndsOn, &p.Status, &p.CreatedAt)
	if err != nil {
		return nil, store.One(err)
	}
	return &p, nil
}

func (s *Store) CreatePeriod(ctx context.Context, p Period) (*Period, error) {
	if p.ID == "" {
		p.ID = store.NewID()
	}
	if p.Status == "" {
		p.Status = PeriodStatusPlanning
	}
	p.CreatedAt = time.Now()
	_, err := s.DB.ExecContext(ctx, `
		INSERT INTO pkl_periods (id, school_id, academic_year_id, name, starts_on, ends_on, status, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		p.ID, p.SchoolID, p.AcademicYearID, p.Name, p.StartsOn, p.EndsOn, p.Status, p.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

func (s *Store) UpdatePeriod(ctx context.Context, p Period) (*Period, error) {
	res, err := s.DB.ExecContext(ctx, `
		UPDATE pkl_periods
		SET academic_year_id = $3, name = $4, starts_on = $5, ends_on = $6, status = $7
		WHERE school_id = $1 AND id = $2`,
		p.SchoolID, p.ID, p.AcademicYearID, p.Name, p.StartsOn, p.EndsOn, p.Status)
	if err != nil {
		return nil, err
	}
	rows, _ := res.RowsAffected()
	if rows == 0 {
		return nil, ErrNotFound
	}
	return s.GetPeriod(ctx, p.SchoolID, p.ID)
}

func (s *Store) ClosePeriod(ctx context.Context, schoolID, id string) error {
	res, err := s.DB.ExecContext(ctx, `
		UPDATE pkl_periods
		SET status = 'closed'
		WHERE school_id = $1 AND id = $2`, schoolID, id)
	if err != nil {
		return err
	}
	rows, _ := res.RowsAffected()
	if rows == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) ListQuotas(ctx context.Context, schoolID, periodID string) ([]Quota, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT q.id, q.school_id, q.partner_id, q.period_id, q.major_id, q.quota, q.created_at,
		       p.name AS partner_name, m.name AS major_name,
		       COALESCE((
		           SELECT COUNT(*)
		           FROM pkl_placements pl
		           WHERE pl.school_id = q.school_id
		             AND pl.partner_id = q.partner_id
		             AND pl.period_id = q.period_id
		             AND pl.major_id = q.major_id
		             AND pl.status <> 'cancelled'
		       ), 0) AS filled_count
		FROM pkl_quotas q
		JOIN pkl_partners p ON q.partner_id = p.id
		JOIN majors m ON q.major_id = m.id
		WHERE q.school_id = $1 AND q.period_id = $2
		ORDER BY p.name ASC, m.name ASC`, schoolID, periodID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []Quota
	for rows.Next() {
		var q Quota
		if err := rows.Scan(&q.ID, &q.SchoolID, &q.PartnerID, &q.PeriodID, &q.MajorID, &q.Quota, &q.CreatedAt,
			&q.PartnerName, &q.MajorName, &q.FilledCount); err != nil {
			return nil, err
		}
		list = append(list, q)
	}
	return list, nil
}

func (s *Store) PutQuotas(ctx context.Context, schoolID, periodID string, quotas []Quota) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	now := time.Now()
	for _, q := range quotas {
		id := q.ID
		if id == "" {
			id = store.NewID()
		}
		_, err := tx.ExecContext(ctx, `
			INSERT INTO pkl_quotas (id, school_id, partner_id, period_id, major_id, quota, created_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7)
			ON CONFLICT (partner_id, period_id, major_id)
			DO UPDATE SET quota = EXCLUDED.quota`,
			id, schoolID, q.PartnerID, periodID, q.MajorID, q.Quota, now)
		if err != nil {
			return err
		}
	}
	return tx.Commit()
}

// ---------------------------------------------------------------------------
// 4. Industry Mentors
// ---------------------------------------------------------------------------

func (s *Store) ListIndustryMentors(ctx context.Context, schoolID, partnerID string) ([]IndustryMentor, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT id, school_id, partner_id, user_id, full_name, email, phone, job_title, active, created_at, deactivated_at
		FROM industry_mentors
		WHERE school_id = $1 AND partner_id = $2
		ORDER BY full_name ASC`, schoolID, partnerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []IndustryMentor
	for rows.Next() {
		var m IndustryMentor
		if err := rows.Scan(&m.ID, &m.SchoolID, &m.PartnerID, &m.UserID, &m.FullName, &m.Email,
			&m.Phone, &m.JobTitle, &m.Active, &m.CreatedAt, &m.DeactivatedAt); err != nil {
			return nil, err
		}
		list = append(list, m)
	}
	return list, nil
}

func (s *Store) GetIndustryMentor(ctx context.Context, schoolID, id string) (*IndustryMentor, error) {
	var m IndustryMentor
	err := s.DB.QueryRowContext(ctx, `
		SELECT id, school_id, partner_id, user_id, full_name, email, phone, job_title, active, created_at, deactivated_at
		FROM industry_mentors
		WHERE school_id = $1 AND id = $2`, schoolID, id).
		Scan(&m.ID, &m.SchoolID, &m.PartnerID, &m.UserID, &m.FullName, &m.Email,
			&m.Phone, &m.JobTitle, &m.Active, &m.CreatedAt, &m.DeactivatedAt)
	if err != nil {
		return nil, store.One(err)
	}
	return &m, nil
}

func (s *Store) GetIndustryMentorByUserID(ctx context.Context, schoolID, userID string) (*IndustryMentor, error) {
	var m IndustryMentor
	err := s.DB.QueryRowContext(ctx, `
		SELECT id, school_id, partner_id, user_id, full_name, email, phone, job_title, active, created_at, deactivated_at
		FROM industry_mentors
		WHERE school_id = $1 AND user_id = $2`, schoolID, userID).
		Scan(&m.ID, &m.SchoolID, &m.PartnerID, &m.UserID, &m.FullName, &m.Email,
			&m.Phone, &m.JobTitle, &m.Active, &m.CreatedAt, &m.DeactivatedAt)
	if err != nil {
		return nil, store.One(err)
	}
	return &m, nil
}

func (s *Store) CreateIndustryMentor(ctx context.Context, m IndustryMentor) (*IndustryMentor, error) {
	if m.ID == "" {
		m.ID = store.NewID()
	}
	m.CreatedAt = time.Now()
	m.Active = true
	_, err := s.DB.ExecContext(ctx, `
		INSERT INTO industry_mentors (id, school_id, partner_id, user_id, full_name, email, phone, job_title, active, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		ON CONFLICT (school_id, partner_id, user_id)
		DO UPDATE SET full_name = EXCLUDED.full_name, email = EXCLUDED.email, phone = EXCLUDED.phone,
		              job_title = EXCLUDED.job_title, active = true, deactivated_at = NULL`,
		m.ID, m.SchoolID, m.PartnerID, m.UserID, m.FullName, m.Email, m.Phone, m.JobTitle, m.Active, m.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &m, nil
}

func (s *Store) DeactivateIndustryMentor(ctx context.Context, schoolID, id string) error {
	res, err := s.DB.ExecContext(ctx, `
		UPDATE industry_mentors
		SET active = false, deactivated_at = now()
		WHERE school_id = $1 AND id = $2`, schoolID, id)
	if err != nil {
		return err
	}
	rows, _ := res.RowsAffected()
	if rows == 0 {
		return ErrNotFound
	}
	return nil
}

// FindMentorsEligibleForClosure returns mentors in this period that have NO other active/planned placements
// and no pending assessments in other ongoing periods (§4.1 b).
func (s *Store) FindMentorsEligibleForClosure(ctx context.Context, schoolID, closedPeriodID string) ([]IndustryMentor, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT DISTINCT im.id, im.school_id, im.partner_id, im.user_id, im.full_name, im.email, im.phone, im.job_title,
		       im.active, im.created_at, im.deactivated_at
		FROM industry_mentors im
		JOIN pkl_placements p ON p.industry_mentor_id = im.id
		WHERE im.school_id = $1 AND p.period_id = $2 AND im.active = true
		  AND NOT EXISTS (
		      SELECT 1 FROM pkl_placements other_p
		      JOIN pkl_periods other_per ON other_p.period_id = other_per.id
		      WHERE other_p.industry_mentor_id = im.id
		        AND other_p.period_id <> $2
		        AND other_per.status IN ('planning', 'ongoing')
		        AND other_p.status IN ('planned', 'active')
		  )`, schoolID, closedPeriodID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []IndustryMentor
	for rows.Next() {
		var m IndustryMentor
		if err := rows.Scan(&m.ID, &m.SchoolID, &m.PartnerID, &m.UserID, &m.FullName, &m.Email,
			&m.Phone, &m.JobTitle, &m.Active, &m.CreatedAt, &m.DeactivatedAt); err != nil {
			return nil, err
		}
		list = append(list, m)
	}
	return list, nil
}

// ---------------------------------------------------------------------------
// 5. Settings
// ---------------------------------------------------------------------------

func (s *Store) GetSettings(ctx context.Context, schoolID string) (*Settings, error) {
	var st Settings
	err := s.DB.QueryRowContext(ctx, `
		SELECT school_id, certificate_prefix, updated_at
		FROM pkl_settings
		WHERE school_id = $1`, schoolID).
		Scan(&st.SchoolID, &st.CertificatePrefix, &st.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return &Settings{SchoolID: schoolID}, nil
	}
	if err != nil {
		return nil, err
	}
	return &st, nil
}

func (s *Store) PutSettings(ctx context.Context, schoolID string, prefix *string) error {
	_, err := s.DB.ExecContext(ctx, `
		INSERT INTO pkl_settings (school_id, certificate_prefix, updated_at)
		VALUES ($1, $2, now())
		ON CONFLICT (school_id)
		DO UPDATE SET certificate_prefix = EXCLUDED.certificate_prefix, updated_at = now()`,
		schoolID, prefix)
	return err
}

// ---------------------------------------------------------------------------
// 6. Placements
// ---------------------------------------------------------------------------

func (s *Store) CheckPlacement(ctx context.Context, schoolID, periodID, partnerID, majorID string) (*CheckPlacementResult, error) {
	res := &CheckPlacementResult{
		Valid:   true,
		MajorID: majorID,
	}

	period, err := s.GetPeriod(ctx, schoolID, periodID)
	if err != nil {
		return nil, fmt.Errorf("periode tidak ditemukan: %w", err)
	}

	mous, err := s.ListPartnerMous(ctx, schoolID, partnerID)
	if err != nil {
		return nil, err
	}

	coveringMou, mouErr := MouCovering(mous, *period)
	if mouErr != nil {
		res.Valid = false
		res.Warning = mouErr.Error()
	} else if coveringMou != nil {
		res.MouNumber = coveringMou.MouNumber
		res.MouExpiresOn = coveringMou.ValidTo.Format("2006-01-02")
	}

	var quota Quota
	err = s.DB.QueryRowContext(ctx, `
		SELECT q.quota, p.name, m.name
		FROM pkl_quotas q
		JOIN pkl_partners p ON q.partner_id = p.id
		JOIN majors m ON q.major_id = m.id
		WHERE q.school_id = $1 AND q.partner_id = $2 AND q.period_id = $3 AND q.major_id = $4`,
		schoolID, partnerID, periodID, majorID).Scan(&quota.Quota, &quota.PartnerName, &quota.MajorName)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	if errors.Is(err, sql.ErrNoRows) {
		res.Valid = false
		if res.Warning == "" {
			res.Warning = "Kuota mitra untuk jurusan ini belum diatur"
		}
		return res, nil
	}

	res.Quota = quota.Quota
	res.MajorName = quota.MajorName

	// Count filled
	var filled int
	err = s.DB.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM pkl_placements
		WHERE school_id = $1 AND partner_id = $2 AND period_id = $3 AND major_id = $4
		  AND status <> 'cancelled'`,
		schoolID, partnerID, periodID, majorID).Scan(&filled)
	if err != nil {
		return nil, err
	}
	res.Filled = filled

	if err := CanPlace(&quota, filled, mouErr); err != nil {
		res.Valid = false
		res.Warning = err.Error()
	}

	return res, nil
}

func (s *Store) ListPlacements(ctx context.Context, schoolID string, f PlacementFilter) ([]Placement, error) {
	query := `
		SELECT pl.id, pl.school_id, pl.period_id, pl.partner_id, pl.major_id, pl.student_id,
		       pl.school_mentor_user_id, pl.industry_mentor_id, pl.starts_on, pl.ends_on,
		       pl.status, pl.cancel_reason, pl.created_by, pl.idempotency_key, pl.created_at, pl.updated_at,
		       p.name AS partner_name, m.name AS major_name,
		       COALESCE(im.full_name, '') AS industry_mentor_name
		FROM pkl_placements pl
		JOIN pkl_partners p ON pl.partner_id = p.id
		JOIN majors m ON pl.major_id = m.id
		LEFT JOIN industry_mentors im ON pl.industry_mentor_id = im.id
		WHERE pl.school_id = $1`
	args := []any{schoolID}
	idx := 2

	if f.PeriodID != "" {
		query += fmt.Sprintf(" AND pl.period_id = $%d", idx)
		args = append(args, f.PeriodID)
		idx++
	}
	if f.PartnerID != "" {
		query += fmt.Sprintf(" AND pl.partner_id = $%d", idx)
		args = append(args, f.PartnerID)
		idx++
	}
	if f.MajorID != "" {
		query += fmt.Sprintf(" AND pl.major_id = $%d", idx)
		args = append(args, f.MajorID)
		idx++
	}
	if f.StudentID != "" {
		query += fmt.Sprintf(" AND pl.student_id = $%d", idx)
		args = append(args, f.StudentID)
		idx++
	}
	if f.SchoolMentorID != "" {
		query += fmt.Sprintf(" AND pl.school_mentor_user_id = $%d", idx)
		args = append(args, f.SchoolMentorID)
		idx++
	}
	if f.IndustryMentorID != "" {
		query += fmt.Sprintf(" AND (pl.industry_mentor_id = $%d OR im.user_id = $%d)", idx, idx)
		args = append(args, f.IndustryMentorID)
		idx++
	}
	if f.Status != "" {
		query += fmt.Sprintf(" AND pl.status = $%d", idx)
		args = append(args, f.Status)
		idx++
	}

	query += " ORDER BY pl.created_at DESC"

	rows, err := s.DB.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []Placement
	for rows.Next() {
		var pl Placement
		if err := rows.Scan(&pl.ID, &pl.SchoolID, &pl.PeriodID, &pl.PartnerID, &pl.MajorID, &pl.StudentID,
			&pl.SchoolMentorUserID, &pl.IndustryMentorID, &pl.StartsOn, &pl.EndsOn,
			&pl.Status, &pl.CancelReason, &pl.CreatedBy, &pl.IdempotencyKey, &pl.CreatedAt, &pl.UpdatedAt,
			&pl.PartnerName, &pl.MajorName, &pl.IndustryMentorName); err != nil {
			return nil, err
		}
		list = append(list, pl)
	}
	return list, nil
}

func (s *Store) GetPlacement(ctx context.Context, schoolID, id string) (*Placement, error) {
	var pl Placement
	err := s.DB.QueryRowContext(ctx, `
		SELECT pl.id, pl.school_id, pl.period_id, pl.partner_id, pl.major_id, pl.student_id,
		       pl.school_mentor_user_id, pl.industry_mentor_id, pl.starts_on, pl.ends_on,
		       pl.status, pl.cancel_reason, pl.created_by, pl.idempotency_key, pl.created_at, pl.updated_at,
		       p.name AS partner_name, m.name AS major_name,
		       COALESCE(im.full_name, '') AS industry_mentor_name
		FROM pkl_placements pl
		JOIN pkl_partners p ON pl.partner_id = p.id
		JOIN majors m ON pl.major_id = m.id
		LEFT JOIN industry_mentors im ON pl.industry_mentor_id = im.id
		WHERE pl.school_id = $1 AND pl.id = $2`, schoolID, id).
		Scan(&pl.ID, &pl.SchoolID, &pl.PeriodID, &pl.PartnerID, &pl.MajorID, &pl.StudentID,
			&pl.SchoolMentorUserID, &pl.IndustryMentorID, &pl.StartsOn, &pl.EndsOn,
			&pl.Status, &pl.CancelReason, &pl.CreatedBy, &pl.IdempotencyKey, &pl.CreatedAt, &pl.UpdatedAt,
			&pl.PartnerName, &pl.MajorName, &pl.IndustryMentorName)
	if err != nil {
		return nil, store.One(err)
	}
	return &pl, nil
}

func (s *Store) GetPlacementByStudent(ctx context.Context, schoolID, studentID string) (*Placement, error) {
	var pl Placement
	err := s.DB.QueryRowContext(ctx, `
		SELECT pl.id, pl.school_id, pl.period_id, pl.partner_id, pl.major_id, pl.student_id,
		       pl.school_mentor_user_id, pl.industry_mentor_id, pl.starts_on, pl.ends_on,
		       pl.status, pl.cancel_reason, pl.created_by, pl.idempotency_key, pl.created_at, pl.updated_at,
		       p.name AS partner_name, m.name AS major_name,
		       COALESCE(im.full_name, '') AS industry_mentor_name
		FROM pkl_placements pl
		JOIN pkl_partners p ON pl.partner_id = p.id
		JOIN majors m ON pl.major_id = m.id
		LEFT JOIN industry_mentors im ON pl.industry_mentor_id = im.id
		WHERE pl.school_id = $1 AND pl.student_id = $2 AND pl.status IN ('planned', 'active', 'completed')
		ORDER BY pl.starts_on DESC
		LIMIT 1`, schoolID, studentID).
		Scan(&pl.ID, &pl.SchoolID, &pl.PeriodID, &pl.PartnerID, &pl.MajorID, &pl.StudentID,
			&pl.SchoolMentorUserID, &pl.IndustryMentorID, &pl.StartsOn, &pl.EndsOn,
			&pl.Status, &pl.CancelReason, &pl.CreatedBy, &pl.IdempotencyKey, &pl.CreatedAt, &pl.UpdatedAt,
			&pl.PartnerName, &pl.MajorName, &pl.IndustryMentorName)
	if err != nil {
		return nil, store.One(err)
	}
	return &pl, nil
}

// CreatePlacement performs transactional placement creation with SELECT ... FOR UPDATE (BR-PKL-01, AC-140-01).
func (s *Store) CreatePlacement(ctx context.Context, p Placement) (*Placement, error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	// Check idempotency first if key given
	if p.IdempotencyKey != nil && *p.IdempotencyKey != "" {
		var existingID string
		err := tx.QueryRowContext(ctx, `
			SELECT id FROM pkl_placements WHERE school_id = $1 AND idempotency_key = $2`,
			p.SchoolID, *p.IdempotencyKey).Scan(&existingID)
		if err == nil {
			_ = tx.Rollback()
			return s.GetPlacement(ctx, p.SchoolID, existingID)
		}
	}

	// 1. Lock quota row for update
	var quota Quota
	err = tx.QueryRowContext(ctx, `
		SELECT q.id, q.quota, p.name, m.name
		FROM pkl_quotas q
		JOIN pkl_partners p ON q.partner_id = p.id
		JOIN majors m ON q.major_id = m.id
		WHERE q.school_id = $1 AND q.partner_id = $2 AND q.period_id = $3 AND q.major_id = $4
		FOR UPDATE`,
		p.SchoolID, p.PartnerID, p.PeriodID, p.MajorID).
		Scan(&quota.ID, &quota.Quota, &quota.PartnerName, &quota.MajorName)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, errors.New("Kuota mitra untuk jurusan ini belum diatur")
	}
	if err != nil {
		return nil, err
	}

	// 2. Count current filled placements
	var filled int
	err = tx.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM pkl_placements
		WHERE school_id = $1 AND partner_id = $2 AND period_id = $3 AND major_id = $4
		  AND status <> 'cancelled'`,
		p.SchoolID, p.PartnerID, p.PeriodID, p.MajorID).Scan(&filled)
	if err != nil {
		return nil, err
	}

	// 3. Fetch period & MoUs to check covering
	var period Period
	err = tx.QueryRowContext(ctx, `
		SELECT id, starts_on, ends_on FROM pkl_periods WHERE school_id = $1 AND id = $2`,
		p.SchoolID, p.PeriodID).Scan(&period.ID, &period.StartsOn, &period.EndsOn)
	if err != nil {
		return nil, fmt.Errorf("periode tidak ditemukan: %w", err)
	}

	mouRows, err := tx.QueryContext(ctx, `
		SELECT id, valid_from, valid_to FROM pkl_partner_mous WHERE school_id = $1 AND partner_id = $2`,
		p.SchoolID, p.PartnerID)
	if err != nil {
		return nil, err
	}
	defer mouRows.Close()

	var mous []PartnerMou
	for mouRows.Next() {
		var mou PartnerMou
		if err := mouRows.Scan(&mou.ID, &mou.ValidFrom, &mou.ValidTo); err != nil {
			return nil, err
		}
		mous = append(mous, mou)
	}
	mouRows.Close()

	_, mouErr := MouCovering(mous, period)
	if err := CanPlace(&quota, filled, mouErr); err != nil {
		return nil, err
	}

	// Check duplicate active placement for this student in this period
	var dupCount int
	err = tx.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM pkl_placements
		WHERE school_id = $1 AND period_id = $2 AND student_id = $3 AND status <> 'cancelled'`,
		p.SchoolID, p.PeriodID, p.StudentID).Scan(&dupCount)
	if err != nil {
		return nil, err
	}
	if dupCount > 0 {
		return nil, errors.New("Siswa sudah memiliki penempatan aktif di periode ini")
	}

	// Insert placement
	if p.ID == "" {
		p.ID = store.NewID()
	}
	now := time.Now()
	p.CreatedAt = now
	p.UpdatedAt = now
	if p.Status == "" {
		p.Status = PlacementStatusPlanned
	}

	_, err = tx.ExecContext(ctx, `
		INSERT INTO pkl_placements (id, school_id, period_id, partner_id, major_id, student_id,
		                            school_mentor_user_id, industry_mentor_id, starts_on, ends_on,
		                            status, created_by, idempotency_key, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)`,
		p.ID, p.SchoolID, p.PeriodID, p.PartnerID, p.MajorID, p.StudentID,
		p.SchoolMentorUserID, p.IndustryMentorID, p.StartsOn, p.EndsOn,
		p.Status, p.CreatedBy, p.IdempotencyKey, p.CreatedAt, p.UpdatedAt)
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	return s.GetPlacement(ctx, p.SchoolID, p.ID)
}

func (s *Store) UpdatePlacement(ctx context.Context, p Placement) (*Placement, error) {
	res, err := s.DB.ExecContext(ctx, `
		UPDATE pkl_placements
		SET school_mentor_user_id = $3, industry_mentor_id = $4, starts_on = $5, ends_on = $6, status = $7, updated_at = now()
		WHERE school_id = $1 AND id = $2`,
		p.SchoolID, p.ID, p.SchoolMentorUserID, p.IndustryMentorID, p.StartsOn, p.EndsOn, p.Status)
	if err != nil {
		return nil, err
	}
	rows, _ := res.RowsAffected()
	if rows == 0 {
		return nil, ErrNotFound
	}
	return s.GetPlacement(ctx, p.SchoolID, p.ID)
}

func (s *Store) CancelPlacement(ctx context.Context, schoolID, id, reason string) error {
	res, err := s.DB.ExecContext(ctx, `
		UPDATE pkl_placements
		SET status = 'cancelled', cancel_reason = $3, updated_at = now()
		WHERE school_id = $1 AND id = $2 AND status <> 'cancelled'`,
		schoolID, id, reason)
	if err != nil {
		return err
	}
	rows, _ := res.RowsAffected()
	if rows == 0 {
		return ErrNotFound
	}
	return nil
}

// ---------------------------------------------------------------------------
// 7. Attendance
// ---------------------------------------------------------------------------

func (s *Store) ListAttendance(ctx context.Context, schoolID, placementID string) ([]Attendance, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT id, school_id, placement_id, work_date, status, check_in_at, check_out_at,
		       note, attachment_key, confirmation, confirmed_by, confirmed_at, reject_reason, created_at
		FROM pkl_attendance
		WHERE school_id = $1 AND placement_id = $2
		ORDER BY work_date DESC`, schoolID, placementID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []Attendance
	for rows.Next() {
		var a Attendance
		if err := rows.Scan(&a.ID, &a.SchoolID, &a.PlacementID, &a.WorkDate, &a.Status, &a.CheckInAt, &a.CheckOutAt,
			&a.Note, &a.AttachmentKey, &a.Confirmation, &a.ConfirmedBy, &a.ConfirmedAt, &a.RejectReason, &a.CreatedAt); err != nil {
			return nil, err
		}
		list = append(list, a)
	}
	return list, nil
}

func (s *Store) GetAttendance(ctx context.Context, schoolID, id string) (*Attendance, error) {
	var a Attendance
	err := s.DB.QueryRowContext(ctx, `
		SELECT id, school_id, placement_id, work_date, status, check_in_at, check_out_at,
		       note, attachment_key, confirmation, confirmed_by, confirmed_at, reject_reason, created_at
		FROM pkl_attendance
		WHERE school_id = $1 AND id = $2`, schoolID, id).
		Scan(&a.ID, &a.SchoolID, &a.PlacementID, &a.WorkDate, &a.Status, &a.CheckInAt, &a.CheckOutAt,
			&a.Note, &a.AttachmentKey, &a.Confirmation, &a.ConfirmedBy, &a.ConfirmedAt, &a.RejectReason, &a.CreatedAt)
	if err != nil {
		return nil, store.One(err)
	}
	return &a, nil
}

func (s *Store) RecordCheckIn(ctx context.Context, schoolID, placementID string, now time.Time) (*Attendance, error) {
	pl, err := s.GetPlacement(ctx, schoolID, placementID)
	if err != nil {
		return nil, err
	}
	day := WorkDay(now)
	if err := CanRecordAttendance(*pl, day); err != nil {
		return nil, err
	}

	// work_date is the Asia/Jakarta calendar day (not the UTC day), sent as
	// a plain date string so the DB session timezone cannot shift it.
	today := day.Format("2006-01-02")
	id := store.NewID()
	var created Attendance
	err = s.DB.QueryRowContext(ctx, `
		INSERT INTO pkl_attendance (id, school_id, placement_id, work_date, status, check_in_at, confirmation, created_at)
		VALUES ($1, $2, $3, $4, 'hadir', $5, 'pending', $5)
		RETURNING id, school_id, placement_id, work_date, status, check_in_at, check_out_at,
		          note, attachment_key, confirmation, confirmed_by, confirmed_at, reject_reason, created_at`,
		id, schoolID, placementID, today, now).
		Scan(&created.ID, &created.SchoolID, &created.PlacementID, &created.WorkDate, &created.Status, &created.CheckInAt, &created.CheckOutAt,
			&created.Note, &created.AttachmentKey, &created.Confirmation, &created.ConfirmedBy, &created.ConfirmedAt, &created.RejectReason, &created.CreatedAt)
	if err != nil {
		if strings.Contains(err.Error(), "unique") || strings.Contains(err.Error(), "uq_") {
			return nil, ErrAlreadyScored
		}
		return nil, err
	}
	return &created, nil
}

func (s *Store) RecordCheckOut(ctx context.Context, schoolID, placementID string, now time.Time) (*Attendance, error) {
	today := WorkDay(now).Format("2006-01-02")
	var updated Attendance
	err := s.DB.QueryRowContext(ctx, `
		UPDATE pkl_attendance
		SET check_out_at = $4
		WHERE school_id = $1 AND placement_id = $2 AND work_date = $3
		RETURNING id, school_id, placement_id, work_date, status, check_in_at, check_out_at,
		          note, attachment_key, confirmation, confirmed_by, confirmed_at, reject_reason, created_at`,
		schoolID, placementID, today, now).
		Scan(&updated.ID, &updated.SchoolID, &updated.PlacementID, &updated.WorkDate, &updated.Status, &updated.CheckInAt, &updated.CheckOutAt,
			&updated.Note, &updated.AttachmentKey, &updated.Confirmation, &updated.ConfirmedBy, &updated.ConfirmedAt, &updated.RejectReason, &updated.CreatedAt)
	if err != nil {
		return nil, store.One(err)
	}
	return &updated, nil
}

func (s *Store) SubmitLeave(ctx context.Context, schoolID, placementID string, workDate time.Time, status, note string, attachmentKey *string) (*Attendance, error) {
	pl, err := s.GetPlacement(ctx, schoolID, placementID)
	if err != nil {
		return nil, err
	}
	if err := CanRecordAttendance(*pl, workDate); err != nil {
		return nil, err
	}

	today := workDate.Truncate(24 * time.Hour)
	id := store.NewID()
	now := time.Now()
	var created Attendance
	err = s.DB.QueryRowContext(ctx, `
		INSERT INTO pkl_attendance (id, school_id, placement_id, work_date, status, note, attachment_key, confirmation, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, 'pending', $8)
		RETURNING id, school_id, placement_id, work_date, status, check_in_at, check_out_at,
		          note, attachment_key, confirmation, confirmed_by, confirmed_at, reject_reason, created_at`,
		id, schoolID, placementID, today, status, note, attachmentKey, now).
		Scan(&created.ID, &created.SchoolID, &created.PlacementID, &created.WorkDate, &created.Status, &created.CheckInAt, &created.CheckOutAt,
			&created.Note, &created.AttachmentKey, &created.Confirmation, &created.ConfirmedBy, &created.ConfirmedAt, &created.RejectReason, &created.CreatedAt)
	if err != nil {
		if strings.Contains(err.Error(), "unique") || strings.Contains(err.Error(), "uq_") {
			return nil, ErrAlreadyScored
		}
		return nil, err
	}
	return &created, nil
}

func (s *Store) ConfirmAttendance(ctx context.Context, schoolID, id, confirmedBy string) error {
	res, err := s.DB.ExecContext(ctx, `
		UPDATE pkl_attendance
		SET confirmation = 'confirmed', confirmed_by = $3, confirmed_at = now(), reject_reason = NULL
		WHERE school_id = $1 AND id = $2`,
		schoolID, id, confirmedBy)
	if err != nil {
		return err
	}
	rows, _ := res.RowsAffected()
	if rows == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) RejectAttendance(ctx context.Context, schoolID, id, reason, confirmedBy string) error {
	res, err := s.DB.ExecContext(ctx, `
		UPDATE pkl_attendance
		SET confirmation = 'rejected', confirmed_by = $3, confirmed_at = now(), reject_reason = $4
		WHERE school_id = $1 AND id = $2`,
		schoolID, id, confirmedBy, reason)
	if err != nil {
		return err
	}
	rows, _ := res.RowsAffected()
	if rows == 0 {
		return ErrNotFound
	}
	return nil
}

type RecapRow struct {
	PlacementID string `json:"placement_id"`
	StudentID   string `json:"student_id"`
	StudentName string `json:"student_name"`
	PartnerName string `json:"partner_name"`
	MajorName   string `json:"major_name"`
	AttendanceRecapResult
}

func (s *Store) RecapAttendance(ctx context.Context, schoolID, periodID, partnerID string) ([]RecapRow, error) {
	query := `
		SELECT pl.id, pl.student_id, p.name AS partner_name, m.name AS major_name
		FROM pkl_placements pl
		JOIN pkl_partners p ON pl.partner_id = p.id
		JOIN majors m ON pl.major_id = m.id
		WHERE pl.school_id = $1 AND pl.status <> 'cancelled'`
	args := []any{schoolID}
	idx := 2

	if periodID != "" {
		query += fmt.Sprintf(" AND pl.period_id = $%d", idx)
		args = append(args, periodID)
		idx++
	}
	if partnerID != "" {
		query += fmt.Sprintf(" AND pl.partner_id = $%d", idx)
		args = append(args, partnerID)
		idx++
	}
	query += " ORDER BY p.name ASC, pl.created_at ASC"

	rows, err := s.DB.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	type placementMeta struct {
		id, studentID, partnerName, majorName string
	}
	var placements []placementMeta
	for rows.Next() {
		var pm placementMeta
		if err := rows.Scan(&pm.id, &pm.studentID, &pm.partnerName, &pm.majorName); err != nil {
			return nil, err
		}
		placements = append(placements, pm)
	}
	rows.Close()

	var result []RecapRow
	for _, pm := range placements {
		attList, err := s.ListAttendance(ctx, schoolID, pm.id)
		if err != nil {
			return nil, err
		}
		recap := AttendanceRecap(attList)
		result = append(result, RecapRow{
			PlacementID:     pm.id,
			StudentID:       pm.studentID,
			PartnerName:     pm.partnerName,
			MajorName:       pm.majorName,
			AttendanceRecapResult: recap,
		})
	}
	return result, nil
}

// ---------------------------------------------------------------------------
// 8. Journals & Final Reports
// ---------------------------------------------------------------------------

func (s *Store) ListJournals(ctx context.Context, schoolID, placementID string) ([]Journal, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT id, school_id, placement_id, entry_date, activity, photos, mentor_note, mentor_note_by, created_at
		FROM pkl_journals
		WHERE school_id = $1 AND placement_id = $2
		ORDER BY entry_date DESC`, schoolID, placementID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []Journal
	for rows.Next() {
		var j Journal
		var photosRaw []byte
		if err := rows.Scan(&j.ID, &j.SchoolID, &j.PlacementID, &j.EntryDate, &j.Activity, &photosRaw,
			&j.MentorNote, &j.MentorNoteBy, &j.CreatedAt); err != nil {
			return nil, err
		}
		if len(photosRaw) > 0 {
			_ = json.Unmarshal(photosRaw, &j.Photos)
		}
		if j.Photos == nil {
			j.Photos = []string{}
		}
		list = append(list, j)
	}
	return list, nil
}

func (s *Store) GetJournal(ctx context.Context, schoolID, id string) (*Journal, error) {
	var j Journal
	var photosRaw []byte
	err := s.DB.QueryRowContext(ctx, `
		SELECT id, school_id, placement_id, entry_date, activity, photos, mentor_note, mentor_note_by, created_at
		FROM pkl_journals
		WHERE school_id = $1 AND id = $2`, schoolID, id).
		Scan(&j.ID, &j.SchoolID, &j.PlacementID, &j.EntryDate, &j.Activity, &photosRaw,
			&j.MentorNote, &j.MentorNoteBy, &j.CreatedAt)
	if err != nil {
		return nil, store.One(err)
	}
	if len(photosRaw) > 0 {
		_ = json.Unmarshal(photosRaw, &j.Photos)
	}
	if j.Photos == nil {
		j.Photos = []string{}
	}
	return &j, nil
}

func (s *Store) CreateJournal(ctx context.Context, j Journal) (*Journal, error) {
	if j.ID == "" {
		j.ID = store.NewID()
	}
	j.CreatedAt = time.Now()
	if j.Photos == nil {
		j.Photos = []string{}
	}
	photosRaw, _ := json.Marshal(j.Photos)

	_, err := s.DB.ExecContext(ctx, `
		INSERT INTO pkl_journals (id, school_id, placement_id, entry_date, activity, photos, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		j.ID, j.SchoolID, j.PlacementID, j.EntryDate, j.Activity, photosRaw, j.CreatedAt)
	if err != nil {
		if strings.Contains(err.Error(), "unique") || strings.Contains(err.Error(), "uq_") {
			return nil, ErrAlreadyExists
		}
		return nil, err
	}
	return &j, nil
}

func (s *Store) UpdateJournal(ctx context.Context, j Journal) (*Journal, error) {
	photosRaw, _ := json.Marshal(j.Photos)
	res, err := s.DB.ExecContext(ctx, `
		UPDATE pkl_journals
		SET activity = $3, photos = $4
		WHERE school_id = $1 AND id = $2`,
		j.SchoolID, j.ID, j.Activity, photosRaw)
	if err != nil {
		return nil, err
	}
	rows, _ := res.RowsAffected()
	if rows == 0 {
		return nil, ErrNotFound
	}
	return s.GetJournal(ctx, j.SchoolID, j.ID)
}

func (s *Store) AddJournalMentorNote(ctx context.Context, schoolID, journalID, note, noteBy string) error {
	res, err := s.DB.ExecContext(ctx, `
		UPDATE pkl_journals
		SET mentor_note = $3, mentor_note_by = $4
		WHERE school_id = $1 AND id = $2`,
		schoolID, journalID, note, noteBy)
	if err != nil {
		return err
	}
	rows, _ := res.RowsAffected()
	if rows == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) ListFinalReports(ctx context.Context, schoolID, placementID string) ([]FinalReport, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT id, school_id, placement_id, version, object_key, file_name, mime, uploaded_at
		FROM pkl_final_reports
		WHERE school_id = $1 AND placement_id = $2
		ORDER BY version DESC`, schoolID, placementID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []FinalReport
	for rows.Next() {
		var f FinalReport
		if err := rows.Scan(&f.ID, &f.SchoolID, &f.PlacementID, &f.Version, &f.ObjectKey, &f.FileName, &f.Mime, &f.UploadedAt); err != nil {
			return nil, err
		}
		list = append(list, f)
	}
	return list, nil
}

func (s *Store) CreateFinalReport(ctx context.Context, rep FinalReport) (*FinalReport, error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	var maxVer sql.NullInt32
	err = tx.QueryRowContext(ctx, `
		SELECT MAX(version) FROM pkl_final_reports WHERE school_id = $1 AND placement_id = $2`,
		rep.SchoolID, rep.PlacementID).Scan(&maxVer)
	if err != nil {
		return nil, err
	}
	version := 1
	if maxVer.Valid {
		version = int(maxVer.Int32) + 1
	}

	if rep.ID == "" {
		rep.ID = store.NewID()
	}
	rep.Version = version
	rep.UploadedAt = time.Now()

	_, err = tx.ExecContext(ctx, `
		INSERT INTO pkl_final_reports (id, school_id, placement_id, version, object_key, file_name, mime, uploaded_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		rep.ID, rep.SchoolID, rep.PlacementID, rep.Version, rep.ObjectKey, rep.FileName, rep.Mime, rep.UploadedAt)
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return &rep, nil
}

// ---------------------------------------------------------------------------
// 9. Schemes & Scores
// ---------------------------------------------------------------------------

func (s *Store) GetScheme(ctx context.Context, schoolID, periodID string) (*Scheme, error) {
	var sc Scheme
	err := s.DB.QueryRowContext(ctx, `
		SELECT id, school_id, period_id, industry_weight_bp, school_weight_bp, created_at
		FROM pkl_schemes
		WHERE school_id = $1 AND period_id = $2`, schoolID, periodID).
		Scan(&sc.ID, &sc.SchoolID, &sc.PeriodID, &sc.IndustryWeightBP, &sc.SchoolWeightBP, &sc.CreatedAt)
	if err != nil {
		return nil, store.One(err)
	}

	rows, err := s.DB.QueryContext(ctx, `
		SELECT id, school_id, scheme_id, assessor, name, weight_bp, sort
		FROM pkl_components
		WHERE school_id = $1 AND scheme_id = $2
		ORDER BY assessor ASC, sort ASC, id ASC`, schoolID, sc.ID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var c Component
		if err := rows.Scan(&c.ID, &c.SchoolID, &c.SchemeID, &c.Assessor, &c.Name, &c.WeightBP, &c.Sort); err != nil {
			return nil, err
		}
		sc.Components = append(sc.Components, c)
	}
	return &sc, nil
}

func (s *Store) PutScheme(ctx context.Context, schoolID, periodID string, sc Scheme) (*Scheme, error) {
	if err := ValidateScheme(sc, sc.Components); err != nil {
		return nil, err
	}

	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	// Check if any scores exist for this period's placements
	var scoreCount int
	err = tx.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM pkl_scores sc
		JOIN pkl_placements pl ON sc.placement_id = pl.id
		WHERE pl.school_id = $1 AND pl.period_id = $2`, schoolID, periodID).Scan(&scoreCount)
	if err != nil {
		return nil, err
	}
	if scoreCount > 0 {
		return nil, ErrSchemeLocked
	}

	var schemeID string
	err = tx.QueryRowContext(ctx, `
		INSERT INTO pkl_schemes (id, school_id, period_id, industry_weight_bp, school_weight_bp)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (period_id)
		DO UPDATE SET industry_weight_bp = EXCLUDED.industry_weight_bp,
		              school_weight_bp = EXCLUDED.school_weight_bp
		RETURNING id`,
		store.NewID(), schoolID, periodID, sc.IndustryWeightBP, sc.SchoolWeightBP).Scan(&schemeID)
	if err != nil {
		return nil, err
	}

	// Delete old components and insert new
	_, err = tx.ExecContext(ctx, `DELETE FROM pkl_components WHERE school_id = $1 AND scheme_id = $2`, schoolID, schemeID)
	if err != nil {
		return nil, err
	}

	for i, c := range sc.Components {
		cid := c.ID
		if cid == "" {
			cid = store.NewID()
		}
		sort := c.Sort
		if sort == 0 {
			sort = i + 1
		}
		_, err = tx.ExecContext(ctx, `
			INSERT INTO pkl_components (id, school_id, scheme_id, assessor, name, weight_bp, sort)
			VALUES ($1, $2, $3, $4, $5, $6, $7)`,
			cid, schoolID, schemeID, c.Assessor, c.Name, c.WeightBP, sort)
		if err != nil {
			return nil, err
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return s.GetScheme(ctx, schoolID, periodID)
}

func (s *Store) ListScores(ctx context.Context, schoolID, placementID string) ([]Score, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT id, school_id, placement_id, component_id, score_x100, scored_by, scored_at
		FROM pkl_scores
		WHERE school_id = $1 AND placement_id = $2`, schoolID, placementID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []Score
	for rows.Next() {
		var sc Score
		if err := rows.Scan(&sc.ID, &sc.SchoolID, &sc.PlacementID, &sc.ComponentID, &sc.ScoreX100, &sc.ScoredBy, &sc.ScoredAt); err != nil {
			return nil, err
		}
		list = append(list, sc)
	}
	return list, nil
}

func (s *Store) PutScores(ctx context.Context, schoolID, placementID, assessor string, scores []Score, scoredBy string) (*Result, error) {
	pl, err := s.GetPlacement(ctx, schoolID, placementID)
	if err != nil {
		return nil, err
	}

	scheme, err := s.GetScheme(ctx, schoolID, pl.PeriodID)
	if err != nil {
		return nil, fmt.Errorf("skema penilaian periode belum diatur: %w", err)
	}

	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	now := time.Now()
	for _, sc := range scores {
		id := sc.ID
		if id == "" {
			id = store.NewID()
		}
		_, err := tx.ExecContext(ctx, `
			INSERT INTO pkl_scores (id, school_id, placement_id, component_id, score_x100, scored_by, scored_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7)
			ON CONFLICT (placement_id, component_id)
			DO UPDATE SET score_x100 = EXCLUDED.score_x100, scored_by = EXCLUDED.scored_by, scored_at = EXCLUDED.scored_at`,
			id, schoolID, placementID, sc.ComponentID, sc.ScoreX100, scoredBy, now)
		if err != nil {
			return nil, err
		}
	}

	// Fetch all scores for this placement
	allScoresRows, err := tx.QueryContext(ctx, `
		SELECT id, school_id, placement_id, component_id, score_x100, scored_by, scored_at
		FROM pkl_scores WHERE school_id = $1 AND placement_id = $2`, schoolID, placementID)
	if err != nil {
		return nil, err
	}
	var allScores []Score
	for allScoresRows.Next() {
		var sc Score
		if err := allScoresRows.Scan(&sc.ID, &sc.SchoolID, &sc.PlacementID, &sc.ComponentID, &sc.ScoreX100, &sc.ScoredBy, &sc.ScoredAt); err != nil {
			allScoresRows.Close()
			return nil, err
		}
		allScores = append(allScores, sc)
	}
	allScoresRows.Close()

	// Calculate industry and school scores if components are complete
	indScore, indErr := AssessorScore(scheme.Components, allScores, AssessorIndustry)
	schScore, schErr := AssessorScore(scheme.Components, allScores, AssessorSchool)

	var indPtr *int
	if indErr == nil {
		indPtr = &indScore
	}
	var schPtr *int
	if schErr == nil {
		schPtr = &schScore
	}

	var finalPtr *int
	if indPtr != nil && schPtr != nil {
		f := FinalScore(*indPtr, *schPtr, *scheme)
		finalPtr = &f
	}

	// Upsert pkl_results
	_, err = tx.ExecContext(ctx, `
		INSERT INTO pkl_results (placement_id, school_id, industry_x100, school_x100, final_x100, computed_at)
		VALUES ($1, $2, $3, $4, $5, now())
		ON CONFLICT (placement_id)
		DO UPDATE SET industry_x100 = EXCLUDED.industry_x100,
		              school_x100 = EXCLUDED.school_x100,
		              final_x100 = EXCLUDED.final_x100,
		              computed_at = now()`,
		placementID, schoolID, indPtr, schPtr, finalPtr)
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	return s.GetResult(ctx, schoolID, placementID)
}

func (s *Store) GetResult(ctx context.Context, schoolID, placementID string) (*Result, error) {
	var res Result
	err := s.DB.QueryRowContext(ctx, `
		SELECT placement_id, school_id, industry_x100, school_x100, final_x100, computed_at, finalized_by, finalized_at
		FROM pkl_results
		WHERE school_id = $1 AND placement_id = $2`, schoolID, placementID).
		Scan(&res.PlacementID, &res.SchoolID, &res.IndustryX100, &res.SchoolX100, &res.FinalX100,
			&res.ComputedAt, &res.FinalizedBy, &res.FinalizedAt)
	if err != nil {
		return nil, store.One(err)
	}
	return &res, nil
}

func (s *Store) FinalizeResult(ctx context.Context, schoolID, placementID, finalizedBy string) (*Result, error) {
	res, err := s.GetResult(ctx, schoolID, placementID)
	if err != nil {
		return nil, err
	}
	if res.FinalX100 == nil {
		return nil, errors.New("Hasil nilai akhir belum lengkap (belum ada skor industri atau sekolah)")
	}

	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	now := time.Now()
	_, err = tx.ExecContext(ctx, `
		UPDATE pkl_results
		SET finalized_by = $3, finalized_at = $4
		WHERE school_id = $1 AND placement_id = $2`,
		schoolID, placementID, finalizedBy, now)
	if err != nil {
		return nil, err
	}

	// Mark placement as completed if active/planned
	_, err = tx.ExecContext(ctx, `
		UPDATE pkl_placements
		SET status = 'completed', updated_at = now()
		WHERE school_id = $1 AND id = $2 AND status <> 'cancelled'`,
		schoolID, placementID)
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return s.GetResult(ctx, schoolID, placementID)
}

// ---------------------------------------------------------------------------
// 10. Certificates
// ---------------------------------------------------------------------------

func (s *Store) IssueCertificate(ctx context.Context, schoolID, placementID, issuedBy, objectKey string) (*Certificate, error) {
	pl, err := s.GetPlacement(ctx, schoolID, placementID)
	if err != nil {
		return nil, err
	}
	res, err := s.GetResult(ctx, schoolID, placementID)
	if err != nil {
		return nil, err
	}
	if res == nil || !CanIssueCertificate(*pl, *res) {
		return nil, errors.New("Sertifikat hanya dapat diterbitkan untuk penempatan yang sudah selesai (completed) dan nilai telah difinalisasi")
	}

	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	// Check if certificate already exists
	var existingCert Certificate
	err = tx.QueryRowContext(ctx, `
		SELECT id, school_id, placement_id, certificate_no, object_key, issued_by, issued_at
		FROM pkl_certificates
		WHERE school_id = $1 AND placement_id = $2`, schoolID, placementID).
		Scan(&existingCert.ID, &existingCert.SchoolID, &existingCert.PlacementID, &existingCert.CertificateNo,
			&existingCert.ObjectKey, &existingCert.IssuedBy, &existingCert.IssuedAt)
	if err == nil {
		_ = tx.Rollback()
		return &existingCert, nil
	}

	// Get prefix
	var prefix *string
	_ = tx.QueryRowContext(ctx, `SELECT certificate_prefix FROM pkl_settings WHERE school_id = $1`, schoolID).Scan(&prefix)
	prefixStr := ""
	if prefix != nil {
		prefixStr = *prefix
	}

	year := time.Now().Year()
	var seq int
	err = tx.QueryRowContext(ctx, `
		SELECT COUNT(*) + 1
		FROM pkl_certificates
		WHERE school_id = $1 AND EXTRACT(YEAR FROM issued_at) = $2
		FOR UPDATE`, schoolID, year).Scan(&seq)
	if err != nil {
		return nil, err
	}

	certNo := CertificateNo(prefixStr, year, seq)
	certID := store.NewID()
	now := time.Now()

	_, err = tx.ExecContext(ctx, `
		INSERT INTO pkl_certificates (id, school_id, placement_id, certificate_no, object_key, issued_by, issued_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		certID, schoolID, placementID, certNo, objectKey, issuedBy, now)
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	return s.GetCertificate(ctx, schoolID, certID)
}

func (s *Store) GetCertificate(ctx context.Context, schoolID, id string) (*Certificate, error) {
	var c Certificate
	err := s.DB.QueryRowContext(ctx, `
		SELECT id, school_id, placement_id, certificate_no, object_key, issued_by, issued_at
		FROM pkl_certificates
		WHERE school_id = $1 AND id = $2`, schoolID, id).
		Scan(&c.ID, &c.SchoolID, &c.PlacementID, &c.CertificateNo, &c.ObjectKey, &c.IssuedBy, &c.IssuedAt)
	if err != nil {
		return nil, store.One(err)
	}
	return &c, nil
}

func (s *Store) GetCertificateByPlacement(ctx context.Context, schoolID, placementID string) (*Certificate, error) {
	var c Certificate
	err := s.DB.QueryRowContext(ctx, `
		SELECT id, school_id, placement_id, certificate_no, object_key, issued_by, issued_at
		FROM pkl_certificates
		WHERE school_id = $1 AND placement_id = $2`, schoolID, placementID).
		Scan(&c.ID, &c.SchoolID, &c.PlacementID, &c.CertificateNo, &c.ObjectKey, &c.IssuedBy, &c.IssuedAt)
	if err != nil {
		return nil, store.One(err)
	}
	return &c, nil
}

// ---------------------------------------------------------------------------
// 11. Supervision
// ---------------------------------------------------------------------------

func (s *Store) ListSupervision(ctx context.Context, schoolID, viewerUserID, viewerRole string) ([]SupervisionItem, error) {
	query := `
		SELECT pl.id, pl.school_id, pl.period_id, pl.partner_id, pl.major_id, pl.student_id,
		       pl.school_mentor_user_id, pl.industry_mentor_id, pl.starts_on, pl.ends_on,
		       pl.status, pl.cancel_reason, pl.created_by, pl.idempotency_key, pl.created_at, pl.updated_at,
		       p.name AS partner_name, m.name AS major_name,
		       COALESCE(im.full_name, '') AS industry_mentor_name,
		       (
		           SELECT COUNT(*)
		           FROM pkl_attendance att
		           WHERE att.placement_id = pl.id AND att.confirmation = 'pending'
		       ) AS pending_attendance_count,
		       (
		           SELECT COUNT(*)
		           FROM pkl_journals j
		           WHERE j.placement_id = pl.id
		       ) AS journal_count
		FROM pkl_placements pl
		JOIN pkl_partners p ON pl.partner_id = p.id
		JOIN majors m ON pl.major_id = m.id
		LEFT JOIN industry_mentors im ON pl.industry_mentor_id = im.id
		WHERE pl.school_id = $1 AND pl.status <> 'cancelled'`
	args := []any{schoolID}

	if viewerRole == "pembimbing_industri" {
		query += ` AND im.user_id = $2`
		args = append(args, viewerUserID)
	} else if viewerRole == "guru" || viewerRole == "walikelas" {
		query += ` AND pl.school_mentor_user_id = $2`
		args = append(args, viewerUserID)
	} else if viewerRole == "koordinator_pkl" || viewerRole == "admin" || viewerRole == "superadmin" {
		// All placements
	} else {
		// Unknown role -> no supervision
		return []SupervisionItem{}, nil
	}

	query += " ORDER BY pl.starts_on DESC, pl.created_at DESC"

	rows, err := s.DB.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []SupervisionItem
	for rows.Next() {
		var item SupervisionItem
		if err := rows.Scan(&item.ID, &item.SchoolID, &item.PeriodID, &item.PartnerID, &item.MajorID, &item.StudentID,
			&item.SchoolMentorUserID, &item.IndustryMentorID, &item.StartsOn, &item.EndsOn,
			&item.Status, &item.CancelReason, &item.CreatedBy, &item.IdempotencyKey, &item.CreatedAt, &item.UpdatedAt,
			&item.PartnerName, &item.MajorName, &item.IndustryMentorName,
			&item.PendingAttendanceCount, &item.JournalCount); err != nil {
			return nil, err
		}
		list = append(list, item)
	}
	return list, nil
}
