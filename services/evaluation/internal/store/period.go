package store

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"skci/evaluation/internal/model"
)

type PeriodStore struct{ db *pgxpool.Pool }

func NewPeriodStore(db *pgxpool.Pool) *PeriodStore { return &PeriodStore{db: db} }

func marshalStrs(ss []string) []byte {
	if ss == nil {
		ss = []string{}
	}
	b, _ := json.Marshal(ss)
	return b
}

const periodCols = `id, school_id, academic_year_id, title, COALESCE(semester,''), publish_start_date, publish_end_date,
	published, headmaster_user_id, curriculum_vp_user_id, student_affairs_vp_user_id,
	target_teacher_ids, peer_evaluatee_ids, peer_evaluator_ids, student_classroom_ids,
	COALESCE(lead_assessor_name,''), created_by, updated_by, created_at, updated_at`

func scanPeriod(row interface {
	Scan(dest ...interface{}) error
}) (*model.Period, error) {
	var p model.Period
	var startDate, endDate time.Time
	var jTarget, jPeerEvaluatee, jPeerEvaluator, jStudentClass []byte
	if err := row.Scan(&p.ID, &p.SchoolID, &p.AcademicYearID, &p.Title, &p.Semester, &startDate, &endDate,
		&p.Published, &p.HeadmasterUserID, &p.CurriculumVPUserID, &p.StudentAffairsVPUserID,
		&jTarget, &jPeerEvaluatee, &jPeerEvaluator, &jStudentClass,
		&p.LeadAssessorName, &p.CreatedBy, &p.UpdatedBy, &p.CreatedAt, &p.UpdatedAt); err != nil {
		return nil, err
	}
	p.PublishStartDate = startDate.Format("2006-01-02")
	p.PublishEndDate = endDate.Format("2006-01-02")
	_ = json.Unmarshal(jTarget, &p.TargetTeacherIDs)
	_ = json.Unmarshal(jPeerEvaluatee, &p.PeerEvaluateeIDs)
	_ = json.Unmarshal(jPeerEvaluator, &p.PeerEvaluatorIDs)
	_ = json.Unmarshal(jStudentClass, &p.StudentClassroomIDs)
	return &p, nil
}

func (s *PeriodStore) Create(ctx context.Context, p *model.Period) error {
	row := s.db.QueryRow(ctx, `
		INSERT INTO teacher_evaluation_periods
			(school_id, academic_year_id, title, semester, publish_start_date, publish_end_date,
			 published, headmaster_user_id, curriculum_vp_user_id, student_affairs_vp_user_id,
			 target_teacher_ids, peer_evaluatee_ids, peer_evaluator_ids, student_classroom_ids,
			 lead_assessor_name, created_by, updated_by)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$16)
		RETURNING `+periodCols,
		p.SchoolID, p.AcademicYearID, p.Title, p.Semester, p.PublishStartDate, p.PublishEndDate,
		p.Published, p.HeadmasterUserID, p.CurriculumVPUserID, p.StudentAffairsVPUserID,
		marshalStrs(p.TargetTeacherIDs), marshalStrs(p.PeerEvaluateeIDs), marshalStrs(p.PeerEvaluatorIDs), marshalStrs(p.StudentClassroomIDs),
		p.LeadAssessorName, p.CreatedBy)
	np, err := scanPeriod(row)
	if err != nil {
		return err
	}
	*p = *np
	return nil
}

func (s *PeriodStore) Update(ctx context.Context, id, schoolID string, p *model.Period) (*model.Period, error) {
	row := s.db.QueryRow(ctx, `
		UPDATE teacher_evaluation_periods SET
			title=$1, semester=$2, publish_start_date=$3, publish_end_date=$4,
			headmaster_user_id=$5, curriculum_vp_user_id=$6, student_affairs_vp_user_id=$7,
			target_teacher_ids=$8, peer_evaluatee_ids=$9, peer_evaluator_ids=$10, student_classroom_ids=$11,
			lead_assessor_name=$12, updated_by=$13, updated_at=now()
		WHERE id=$14 AND school_id=$15 AND deleted_at IS NULL
		RETURNING `+periodCols,
		p.Title, p.Semester, p.PublishStartDate, p.PublishEndDate,
		p.HeadmasterUserID, p.CurriculumVPUserID, p.StudentAffairsVPUserID,
		marshalStrs(p.TargetTeacherIDs), marshalStrs(p.PeerEvaluateeIDs), marshalStrs(p.PeerEvaluatorIDs), marshalStrs(p.StudentClassroomIDs),
		p.LeadAssessorName, p.UpdatedBy, id, schoolID)
	return scanPeriod(row)
}

func (s *PeriodStore) SetPublished(ctx context.Context, id, schoolID string, published bool) (*model.Period, error) {
	row := s.db.QueryRow(ctx, `UPDATE teacher_evaluation_periods SET published=$1, updated_at=now()
		WHERE id=$2 AND school_id=$3 AND deleted_at IS NULL RETURNING `+periodCols, published, id, schoolID)
	return scanPeriod(row)
}

func (s *PeriodStore) GetByID(ctx context.Context, id, schoolID string) (*model.Period, error) {
	row := s.db.QueryRow(ctx, `SELECT `+periodCols+` FROM teacher_evaluation_periods WHERE id=$1 AND school_id=$2 AND deleted_at IS NULL`, id, schoolID)
	return scanPeriod(row)
}

func (s *PeriodStore) List(ctx context.Context, schoolID string, academicYearID *string, page, perPage int) ([]*model.Period, int, error) {
	base := `FROM teacher_evaluation_periods WHERE school_id=$1 AND deleted_at IS NULL`
	args := []interface{}{schoolID}
	if academicYearID != nil {
		base += ` AND academic_year_id=$2`
		args = append(args, *academicYearID)
	}
	var total int
	if err := s.db.QueryRow(ctx, `SELECT COUNT(*) `+base, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	args = append(args, perPage, (page-1)*perPage)
	q := fmt.Sprintf(`SELECT `+periodCols+` %s ORDER BY created_at DESC LIMIT $%d OFFSET $%d`, base, len(args)-1, len(args))
	rows, err := s.db.Query(ctx, q, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []*model.Period
	for rows.Next() {
		p, err := scanPeriod(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, p)
	}
	return out, total, rows.Err()
}

func (s *PeriodStore) Delete(ctx context.Context, id, schoolID string) (bool, error) {
	tag, err := s.db.Exec(ctx, `UPDATE teacher_evaluation_periods SET deleted_at=now() WHERE id=$1 AND school_id=$2 AND deleted_at IS NULL`, id, schoolID)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}
