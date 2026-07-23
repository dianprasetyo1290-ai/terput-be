package store

import (
	"context"
	"database/sql"
	"strconv"

	"skci/exam/internal/model"
)

// CreateQuestion inserts a question plus its nested answer_options /
// essay_options in one transaction.
func (s *Store) CreateQuestion(ctx context.Context, q *model.Question) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	err = tx.QueryRowContext(ctx, `
		INSERT INTO questions (school_id, academic_year_id, folder_id, created_by, question_type, content, image_url,
			no_kd, konten_kd, indikator_pencapaian, level_kognitif, poin)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)
		RETURNING id, created_at, updated_at`,
		q.SchoolID, q.AcademicYearID, q.FolderID, q.CreatedBy, q.QuestionType, q.Content, q.ImageURL,
		q.NoKD, q.KontenKD, q.IndikatorPencapaian, q.LevelKognitif, q.Poin,
	).Scan(&q.ID, &q.CreatedAt, &q.UpdatedAt)
	if err != nil {
		return err
	}

	for i := range q.AnswerOptions {
		opt := &q.AnswerOptions[i]
		opt.QuestionID = q.ID
		opt.OrderIndex = i
		err = tx.QueryRowContext(ctx, `
			INSERT INTO answer_options (question_id, content, image_url, is_correct, order_index)
			VALUES ($1,$2,$3,$4,$5) RETURNING id`,
			opt.QuestionID, opt.Content, opt.ImageURL, opt.IsCorrect, opt.OrderIndex).Scan(&opt.ID)
		if err != nil {
			return err
		}
	}

	for i := range q.EssayOptions {
		eo := &q.EssayOptions[i]
		eo.QuestionID = q.ID
		err = tx.QueryRowContext(ctx, `
			INSERT INTO essay_answer_options (question_id, answer_text) VALUES ($1,$2) RETURNING id`,
			eo.QuestionID, eo.AnswerText).Scan(&eo.ID)
		if err != nil {
			return err
		}
	}

	return tx.Commit()
}

// ListQuestions scopes to folder + (if ownerFilter != "") to created_by =
// ownerFilter, implementing the "guru only ever sees their own soal" rule.
func (s *Store) ListQuestions(ctx context.Context, schoolID, folderID, ownerFilter string, page, perPage int) ([]model.Question, int64, error) {
	args := []interface{}{schoolID, folderID}
	where := `school_id=$1 AND folder_id=$2 AND deleted_at IS NULL`
	if ownerFilter != "" {
		args = append(args, ownerFilter)
		where += " AND created_by=$3"
	}

	var total int64
	countQ := `SELECT COUNT(*) FROM questions WHERE ` + where
	if err := s.DB.QueryRowContext(ctx, countQ, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	args = append(args, perPage, (page-1)*perPage)
	listQ := `
		SELECT id, school_id, academic_year_id, folder_id, created_by, question_type, content, image_url,
			no_kd, konten_kd, indikator_pencapaian, level_kognitif, poin, is_published, created_at, updated_at
		FROM questions WHERE ` + where + ` ORDER BY created_at DESC LIMIT $` +
		strconv.Itoa(len(args)-1) + ` OFFSET $` + strconv.Itoa(len(args))

	rows, err := s.DB.QueryContext(ctx, listQ, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []model.Question
	for rows.Next() {
		var q model.Question
		if err := rows.Scan(&q.ID, &q.SchoolID, &q.AcademicYearID, &q.FolderID, &q.CreatedBy, &q.QuestionType, &q.Content, &q.ImageURL,
			&q.NoKD, &q.KontenKD, &q.IndikatorPencapaian, &q.LevelKognitif, &q.Poin, &q.IsPublished, &q.CreatedAt, &q.UpdatedAt); err != nil {
			return nil, 0, err
		}
		out = append(out, q)
	}
	return out, total, rows.Err()
}

func (s *Store) GetQuestion(ctx context.Context, id, schoolID string) (*model.Question, error) {
	var q model.Question
	err := s.DB.QueryRowContext(ctx, `
		SELECT id, school_id, academic_year_id, folder_id, created_by, question_type, content, image_url,
			no_kd, konten_kd, indikator_pencapaian, level_kognitif, poin, is_published, created_at, updated_at
		FROM questions WHERE id=$1 AND school_id=$2 AND deleted_at IS NULL`, id, schoolID,
	).Scan(&q.ID, &q.SchoolID, &q.AcademicYearID, &q.FolderID, &q.CreatedBy, &q.QuestionType, &q.Content, &q.ImageURL,
		&q.NoKD, &q.KontenKD, &q.IndikatorPencapaian, &q.LevelKognitif, &q.Poin, &q.IsPublished, &q.CreatedAt, &q.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	rows, err := s.DB.QueryContext(ctx, `
		SELECT id, question_id, content, image_url, is_correct, order_index
		FROM answer_options WHERE question_id=$1 ORDER BY order_index`, q.ID)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var o model.AnswerOption
		if err := rows.Scan(&o.ID, &o.QuestionID, &o.Content, &o.ImageURL, &o.IsCorrect, &o.OrderIndex); err != nil {
			rows.Close()
			return nil, err
		}
		q.AnswerOptions = append(q.AnswerOptions, o)
	}
	rows.Close()

	rows2, err := s.DB.QueryContext(ctx, `
		SELECT id, question_id, answer_text FROM essay_answer_options WHERE question_id=$1`, q.ID)
	if err != nil {
		return nil, err
	}
	for rows2.Next() {
		var e model.EssayAnswerOption
		if err := rows2.Scan(&e.ID, &e.QuestionID, &e.AnswerText); err != nil {
			rows2.Close()
			return nil, err
		}
		q.EssayOptions = append(q.EssayOptions, e)
	}
	rows2.Close()

	return &q, nil
}

// UpdateQuestion replaces the question's scalar fields and (if provided)
// fully replaces its answer_options / essay_options.
func (s *Store) UpdateQuestion(ctx context.Context, q *model.Question, replaceOptions bool) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	_, err = tx.ExecContext(ctx, `
		UPDATE questions SET content=$1, image_url=$2, no_kd=$3, konten_kd=$4, indikator_pencapaian=$5,
			level_kognitif=$6, poin=$7, updated_at=now()
		WHERE id=$8 AND school_id=$9`,
		q.Content, q.ImageURL, q.NoKD, q.KontenKD, q.IndikatorPencapaian, q.LevelKognitif, q.Poin, q.ID, q.SchoolID)
	if err != nil {
		return err
	}

	if replaceOptions {
		if _, err = tx.ExecContext(ctx, `DELETE FROM answer_options WHERE question_id=$1`, q.ID); err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, `DELETE FROM essay_answer_options WHERE question_id=$1`, q.ID); err != nil {
			return err
		}
		for i := range q.AnswerOptions {
			opt := &q.AnswerOptions[i]
			opt.QuestionID = q.ID
			opt.OrderIndex = i
			if err = tx.QueryRowContext(ctx, `
				INSERT INTO answer_options (question_id, content, image_url, is_correct, order_index)
				VALUES ($1,$2,$3,$4,$5) RETURNING id`,
				opt.QuestionID, opt.Content, opt.ImageURL, opt.IsCorrect, opt.OrderIndex).Scan(&opt.ID); err != nil {
				return err
			}
		}
		for i := range q.EssayOptions {
			eo := &q.EssayOptions[i]
			eo.QuestionID = q.ID
			if err = tx.QueryRowContext(ctx, `
				INSERT INTO essay_answer_options (question_id, answer_text) VALUES ($1,$2) RETURNING id`,
				eo.QuestionID, eo.AnswerText).Scan(&eo.ID); err != nil {
				return err
			}
		}
	}

	return tx.Commit()
}

func (s *Store) DeleteQuestion(ctx context.Context, id, schoolID string) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE questions SET deleted_at=now() WHERE id=$1 AND school_id=$2`, id, schoolID)
	return err
}

func (s *Store) PublishQuestion(ctx context.Context, id, schoolID string) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE questions SET is_published=true, updated_at=now() WHERE id=$1 AND school_id=$2`, id, schoolID)
	return err
}
