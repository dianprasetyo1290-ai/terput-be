package store

import (
	"context"
	"database/sql"
)

type NoteTemplate struct {
	MidTemplate   string `json:"mid_template"`
	FinalTemplate string `json:"final_template"`
}

// GetNoteTemplate returns the school's Deskripsi Rapor templates ("Atur
// Template Deskripsi" in the old app), or empty strings if none set yet.
func (s *Store) GetNoteTemplate(ctx context.Context, schoolID string) (NoteTemplate, error) {
	var t NoteTemplate
	err := s.DB.QueryRowContext(ctx,
		`SELECT mid_template, final_template FROM report_note_templates WHERE school_id = $1`, schoolID,
	).Scan(&t.MidTemplate, &t.FinalTemplate)
	if err == sql.ErrNoRows {
		return NoteTemplate{}, nil
	}
	return t, err
}

func (s *Store) SetNoteTemplate(ctx context.Context, schoolID string, t NoteTemplate, updatedBy string) error {
	_, err := s.DB.ExecContext(ctx, `
		INSERT INTO report_note_templates (school_id, mid_template, final_template, updated_by)
		VALUES ($1,$2,$3,$4)
		ON CONFLICT (school_id) DO UPDATE SET
			mid_template = EXCLUDED.mid_template, final_template = EXCLUDED.final_template,
			updated_by = EXCLUDED.updated_by, updated_at = now()`,
		schoolID, t.MidTemplate, t.FinalTemplate, updatedBy)
	return err
}
