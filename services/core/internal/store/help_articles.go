package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"skci/core/internal/model"
)

const helpArticleCols = `id, school_id, audience, kind, title, body, sort_order, published, replaces_id, created_at, updated_at`

func scanHelpArticle(row rowScanner) (model.HelpArticle, error) {
	var a model.HelpArticle
	err := row.Scan(&a.ID, &a.SchoolID, &a.Audience, &a.Kind, &a.Title, &a.Body, &a.SortOrder, &a.Published,
		&a.ReplacesID, &a.CreatedAt, &a.UpdatedAt)
	return a, err
}

// ListHelpArticles returns the platform defaults plus the school's own rows
// for an audience (merge with model.MergeHelpArticles).
func (s *PGStore) ListHelpArticles(ctx context.Context, schoolID, audience string) ([]model.HelpArticle, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT `+helpArticleCols+` FROM help_articles
		WHERE (school_id IS NULL OR school_id = $1) AND audience = $2
		ORDER BY sort_order, title LIMIT 500`, schoolID, audience)
	if err != nil {
		return nil, fmt.Errorf("list help articles: %w", err)
	}
	defer rows.Close()
	var out []model.HelpArticle
	for rows.Next() {
		a, err := scanHelpArticle(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// GetHelpArticle loads a default or one of the school's own articles.
func (s *PGStore) GetHelpArticle(ctx context.Context, schoolID, id string) (*model.HelpArticle, error) {
	a, err := scanHelpArticle(s.DB.QueryRowContext(ctx, `SELECT `+helpArticleCols+` FROM help_articles
		WHERE id = $1 AND (school_id IS NULL OR school_id = $2)`, id, schoolID))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get help article: %w", err)
	}
	return &a, nil
}

// CreateHelpArticle inserts a school article. A second replacement for the
// same default is ErrConflict.
func (s *PGStore) CreateHelpArticle(ctx context.Context, a model.HelpArticle, actor string) (*model.HelpArticle, error) {
	out, err := scanHelpArticle(s.DB.QueryRowContext(ctx, `
		INSERT INTO help_articles (school_id, audience, kind, title, body, sort_order, published, replaces_id, created_by, updated_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, NULLIF($9, '')::uuid, NULLIF($9, '')::uuid)
		RETURNING `+helpArticleCols, a.SchoolID, a.Audience, a.Kind, a.Title, a.Body, a.SortOrder, a.Published, a.ReplacesID, actor))
	if isUniqueViolation(err) {
		return nil, fmt.Errorf("%w: artikel bawaan ini sudah memiliki pengganti", ErrConflict)
	}
	if err != nil {
		return nil, fmt.Errorf("insert help article: %w", err)
	}
	return &out, nil
}

// UpdateHelpArticle edits one of the school's own articles (defaults are
// read-only: ErrNotFound).
func (s *PGStore) UpdateHelpArticle(ctx context.Context, schoolID, id string, a model.HelpArticle, actor string) (*model.HelpArticle, error) {
	out, err := scanHelpArticle(s.DB.QueryRowContext(ctx, `
		UPDATE help_articles SET audience=$3, kind=$4, title=$5, body=$6, sort_order=$7, published=$8, updated_by=NULLIF($9, '')::uuid, updated_at=now()
		WHERE id = $1 AND school_id = $2
		RETURNING `+helpArticleCols, id, schoolID, a.Audience, a.Kind, a.Title, a.Body, a.SortOrder, a.Published, actor))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("update help article: %w", err)
	}
	return &out, nil
}

// DeleteHelpArticle removes one of the school's own articles; deleting a
// replacement brings the default back.
func (s *PGStore) DeleteHelpArticle(ctx context.Context, schoolID, id string) error {
	res, err := s.DB.ExecContext(ctx, `DELETE FROM help_articles WHERE id = $1 AND school_id = $2`, id, schoolID)
	if err != nil {
		return fmt.Errorf("delete help article: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}
