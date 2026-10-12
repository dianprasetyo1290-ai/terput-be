package store

import (
	"context"
	"database/sql"
	"errors"
	"strconv"
	"strings"

	"skci/notification/internal/model"
)

// ---- T13 teacher–parent chat extension ---------------------------------------

func kindOrGeneral(kind string) string {
	if kind == "" {
		return model.ChatKindGeneral
	}
	return kind
}

// attachmentCols scans the nullable attachment metadata.
type attachmentCols struct {
	name, mime sql.NullString
	size       sql.NullInt64
}

func (a attachmentCols) apply(m *model.ChatMessage) {
	if a.name.Valid {
		m.AttachmentName = &a.name.String
	}
	if a.mime.Valid {
		m.AttachmentMime = &a.mime.String
	}
	if a.size.Valid {
		m.AttachmentSize = &a.size.Int64
	}
}

// FindActiveTeacherParentThread returns the open thread for a trio, or
// ErrNotFound.
func (s *Store) FindActiveTeacherParentThread(ctx context.Context, schoolID, teacherID, guardianID, studentID string) (*model.ChatThread, error) {
	t, err := scanThread(s.DB.QueryRowContext(ctx, `SELECT `+threadColumns+` FROM chat_threads
		WHERE school_id=$1 AND kind='teacher_parent' AND closed_at IS NULL
		  AND teacher_user_id=$2 AND guardian_user_id=$3 AND student_user_id=$4`, schoolID, teacherID, guardianID, studentID))
	if err != nil {
		return nil, err
	}
	t.Participants, err = s.ListChatParticipants(ctx, t.ID)
	return t, err
}

// ArchiveChatThread sets archived_at; messages are never removed.
func (s *Store) ArchiveChatThread(ctx context.Context, threadID string) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE chat_threads SET archived_at=COALESCE(archived_at, now()) WHERE id=$1`, threadID)
	return err
}

// GetChatMessage loads one message of the school (with its thread id).
func (s *Store) GetChatMessage(ctx context.Context, id, schoolID string) (*model.ChatMessage, error) {
	var (
		m   model.ChatMessage
		key sql.NullString
		att attachmentCols
	)
	err := s.DB.QueryRowContext(ctx, `SELECT m.id, m.thread_id, m.sender_user_id, m.body, m.attachment_key, m.created_at,
		m.attachment_name, m.attachment_mime, m.attachment_size, m.outside_service_hours
		FROM chat_messages m JOIN chat_threads t ON t.id=m.thread_id
		WHERE m.id=$1 AND t.school_id=$2`, id, schoolID).Scan(
		&m.ID, &m.ThreadID, &m.SenderUserID, &m.Body, &key, &m.CreatedAt, &att.name, &att.mime, &att.size, &m.OutsideServiceHours)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if key.Valid {
		m.AttachmentKey = &key.String
	}
	att.apply(&m)
	return &m, nil
}

// GetChatSettings returns a school's chat settings, or defaults when unset.
func (s *Store) GetChatSettings(ctx context.Context, schoolID string) (*model.ChatSettings, error) {
	var (
		start, end sql.NullString
		days       sql.NullString
		max        int64
	)
	err := s.DB.QueryRowContext(ctx, `SELECT to_char(service_start,'HH24:MI'), to_char(service_end,'HH24:MI'),
		array_to_string(service_days, ','), max_attachment_bytes FROM chat_settings WHERE school_id=$1`, schoolID).Scan(&start, &end, &days, &max)
	if errors.Is(err, sql.ErrNoRows) {
		return &model.ChatSettings{SchoolID: schoolID, ServiceDays: []int{}, MaxAttachmentBytes: model.DefaultMaxAttachmentBytes}, nil
	}
	if err != nil {
		return nil, err
	}
	out := &model.ChatSettings{SchoolID: schoolID, ServiceDays: []int{}, MaxAttachmentBytes: max}
	if start.Valid {
		out.ServiceStart = &start.String
	}
	if end.Valid {
		out.ServiceEnd = &end.String
	}
	if days.Valid && days.String != "" {
		for _, d := range strings.Split(days.String, ",") {
			if n, err := strconv.Atoi(d); err == nil {
				out.ServiceDays = append(out.ServiceDays, n)
			}
		}
	}
	return out, nil
}

// UpsertChatSettings stores a school's chat settings.
func (s *Store) UpsertChatSettings(ctx context.Context, in model.ChatSettings) error {
	days := make([]string, 0, len(in.ServiceDays))
	for _, d := range in.ServiceDays {
		days = append(days, strconv.Itoa(d))
	}
	var daysArg any
	if len(days) > 0 {
		daysArg = "{" + strings.Join(days, ",") + "}"
	}
	_, err := s.DB.ExecContext(ctx, `INSERT INTO chat_settings (school_id, service_start, service_end, service_days, max_attachment_bytes)
		VALUES ($1,$2::time,$3::time,$4::int[],$5)
		ON CONFLICT (school_id) DO UPDATE SET service_start=EXCLUDED.service_start, service_end=EXCLUDED.service_end,
		service_days=EXCLUDED.service_days, max_attachment_bytes=EXCLUDED.max_attachment_bytes, updated_at=now()`,
		in.SchoolID, in.ServiceStart, in.ServiceEnd, daysArg, in.MaxAttachmentBytes)
	return err
}

const reportCols = `r.id, r.school_id, r.message_id, m.thread_id, r.reporter_user_id, r.reason, r.status, r.reviewed_by,
	r.review_note, r.reviewed_at, r.created_at, m.body, m.sender_user_id`

func scanReport(row rowScanner) (*model.ChatMessageReport, error) {
	var (
		r                model.ChatMessageReport
		reviewedBy, note sql.NullString
		reviewedAt       sql.NullTime
	)
	if err := row.Scan(&r.ID, &r.SchoolID, &r.MessageID, &r.ThreadID, &r.ReporterUserID, &r.Reason, &r.Status, &reviewedBy,
		&note, &reviewedAt, &r.CreatedAt, &r.MessageBody, &r.MessageSender); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	if reviewedBy.Valid {
		r.ReviewedBy = &reviewedBy.String
	}
	if note.Valid {
		r.ReviewNote = &note.String
	}
	if reviewedAt.Valid {
		t := reviewedAt.Time
		r.ReviewedAt = &t
	}
	return &r, nil
}

// CreateChatReport records a report; a second report of the same message by
// the same user returns ErrConflict.
func (s *Store) CreateChatReport(ctx context.Context, schoolID, messageID, reporterID, reason string) (*model.ChatMessageReport, error) {
	var id string
	err := s.DB.QueryRowContext(ctx, `INSERT INTO chat_message_reports (school_id, message_id, reporter_user_id, reason)
		VALUES ($1,$2,$3,$4) ON CONFLICT (message_id, reporter_user_id) DO NOTHING RETURNING id`,
		schoolID, messageID, reporterID, reason).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrConflict
	}
	if err != nil {
		return nil, err
	}
	return s.GetChatReport(ctx, id, schoolID)
}

// GetChatReport loads one report of a school.
func (s *Store) GetChatReport(ctx context.Context, id, schoolID string) (*model.ChatMessageReport, error) {
	return scanReport(s.DB.QueryRowContext(ctx, `SELECT `+reportCols+` FROM chat_message_reports r
		JOIN chat_messages m ON m.id=r.message_id WHERE r.id=$1 AND r.school_id=$2`, id, schoolID))
}

// ListChatReports pages a school's reports, optionally by status.
func (s *Store) ListChatReports(ctx context.Context, schoolID, status string, page, perPage int) ([]model.ChatMessageReport, int, error) {
	var total int
	if err := s.DB.QueryRowContext(ctx, `SELECT count(*) FROM chat_message_reports WHERE school_id=$1 AND ($2='' OR status=$2)`,
		schoolID, status).Scan(&total); err != nil {
		return nil, 0, err
	}
	perPage = clampPerPage(perPage)
	if page < 1 {
		page = 1
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT `+reportCols+` FROM chat_message_reports r
		JOIN chat_messages m ON m.id=r.message_id WHERE r.school_id=$1 AND ($2='' OR r.status=$2)
		ORDER BY r.created_at DESC LIMIT $3 OFFSET $4`, schoolID, status, perPage, (page-1)*perPage)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := []model.ChatMessageReport{}
	for rows.Next() {
		r, err := scanReport(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, *r)
	}
	return out, total, rows.Err()
}

// ReviewChatReport closes an open report.
func (s *Store) ReviewChatReport(ctx context.Context, id, schoolID, status, note, reviewerID string) (*model.ChatMessageReport, error) {
	res, err := s.DB.ExecContext(ctx, `UPDATE chat_message_reports SET status=$3, review_note=NULLIF($4,''), reviewed_by=$5, reviewed_at=now()
		WHERE id=$1 AND school_id=$2 AND status='open'`, id, schoolID, status, note, reviewerID)
	if err != nil {
		return nil, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return nil, ErrConflict
	}
	return s.GetChatReport(ctx, id, schoolID)
}
