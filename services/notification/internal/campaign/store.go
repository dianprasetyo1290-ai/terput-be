package campaign

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"
)

// ErrNotFound is returned when a row does not exist in the caller's school.
var ErrNotFound = errors.New("not found")

// SegmentFilter is the saved filter of a segment.
type SegmentFilter struct {
	GradeLevels  []string `json:"grade_levels,omitempty"`
	ClassroomIDs []string `json:"classroom_ids,omitempty"`
	PeriodID     string   `json:"period_id,omitempty"`
	Statuses     []string `json:"statuses,omitempty"`
}

// Segment is one saved recipient segment.
type Segment struct {
	ID        string        `json:"id"`
	SchoolID  string        `json:"school_id"`
	Name      string        `json:"name"`
	Kind      string        `json:"kind"`
	Filter    SegmentFilter `json:"filter"`
	CreatedBy string        `json:"created_by"`
	CreatedAt time.Time     `json:"created_at"`
	UpdatedAt time.Time     `json:"updated_at"`
}

// WATemplate is a provider WhatsApp template mirrored by humas.
type WATemplate struct {
	ID                 string    `json:"id"`
	SchoolID           string    `json:"school_id"`
	Name               string    `json:"name"`
	ProviderTemplateID string    `json:"provider_template_id"`
	Language           string    `json:"language"`
	BodyPreview        string    `json:"body_preview"`
	Variables          []string  `json:"variables"`
	ApprovalStatus     string    `json:"approval_status"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
}

// Campaign is one campaign.
type Campaign struct {
	ID           string            `json:"id"`
	SchoolID     string            `json:"school_id"`
	Name         string            `json:"name"`
	SegmentID    string            `json:"segment_id"`
	Channel      string            `json:"channel"`
	TemplateID   *string           `json:"template_id"`
	TemplateVars map[string]string `json:"template_vars"`
	Subject      *string           `json:"subject"`
	Body         *string           `json:"body"`
	ScheduledAt  *time.Time        `json:"scheduled_at"`
	Status       string            `json:"status"`
	LastError    *string           `json:"last_error,omitempty"`
	CreatedBy    string            `json:"created_by"`
	CreatedAt    time.Time         `json:"created_at"`
	UpdatedAt    time.Time         `json:"updated_at"`
	SentAt       *time.Time        `json:"sent_at"`
}

// Recipient is one campaign_recipients row (contact masked only).
type Recipient struct {
	ID                string     `json:"id"`
	SubjectKey        string     `json:"-"`
	UserID            *string    `json:"user_id"`
	DisplayName       string     `json:"display_name"`
	ContactMasked     string     `json:"contact_masked"`
	Status            string     `json:"status"`
	ExcludeReason     *string    `json:"exclude_reason"`
	ProviderMessageID *string    `json:"-"`
	LastError         *string    `json:"last_error"`
	SentAt            *time.Time `json:"sent_at"`
	DeliveredAt       *time.Time `json:"delivered_at"`
	ReadAt            *time.Time `json:"read_at"`
	RepliedAt         *time.Time `json:"replied_at"`
}

// Consent is one marketing_consents row.
type Consent struct {
	Channel   string    `json:"channel"`
	Status    string    `json:"status"`
	Source    string    `json:"source"`
	ChangedAt time.Time `json:"changed_at"`
}

// Store is the campaign persistence layer (database/sql over pgx).
type Store struct{ DB *sql.DB }

// NewStore wraps a pool.
func NewStore(db *sql.DB) *Store { return &Store{DB: db} }

type scanner interface{ Scan(dest ...any) error }

func noRows(err error) error {
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	return err
}

// ---- segments ----------------------------------------------------------------

const segmentCols = `id, school_id, name, kind, filter, created_by, created_at, updated_at`

func scanSegment(row scanner) (*Segment, error) {
	var (
		s   Segment
		raw []byte
	)
	if err := row.Scan(&s.ID, &s.SchoolID, &s.Name, &s.Kind, &raw, &s.CreatedBy, &s.CreatedAt, &s.UpdatedAt); err != nil {
		return nil, noRows(err)
	}
	_ = json.Unmarshal(raw, &s.Filter)
	return &s, nil
}

// ListSegments returns a school's active segments.
func (s *Store) ListSegments(ctx context.Context, schoolID string) ([]Segment, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT `+segmentCols+` FROM campaign_segments
		WHERE school_id=$1 AND deleted_at IS NULL ORDER BY created_at DESC`, schoolID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Segment{}
	for rows.Next() {
		seg, err := scanSegment(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *seg)
	}
	return out, rows.Err()
}

// GetSegment loads one active segment.
func (s *Store) GetSegment(ctx context.Context, schoolID, id string) (*Segment, error) {
	return scanSegment(s.DB.QueryRowContext(ctx, `SELECT `+segmentCols+` FROM campaign_segments
		WHERE id=$1 AND school_id=$2 AND deleted_at IS NULL`, id, schoolID))
}

// CreateSegment inserts a segment.
func (s *Store) CreateSegment(ctx context.Context, seg Segment) (*Segment, error) {
	raw, _ := json.Marshal(seg.Filter)
	return scanSegment(s.DB.QueryRowContext(ctx, `INSERT INTO campaign_segments (school_id, name, kind, filter, created_by)
		VALUES ($1,$2,$3,$4,$5) RETURNING `+segmentCols, seg.SchoolID, seg.Name, seg.Kind, raw, seg.CreatedBy))
}

// UpdateSegment changes name/kind/filter.
func (s *Store) UpdateSegment(ctx context.Context, seg Segment) (*Segment, error) {
	raw, _ := json.Marshal(seg.Filter)
	return scanSegment(s.DB.QueryRowContext(ctx, `UPDATE campaign_segments SET name=$3, kind=$4, filter=$5, updated_at=now()
		WHERE id=$1 AND school_id=$2 AND deleted_at IS NULL RETURNING `+segmentCols, seg.ID, seg.SchoolID, seg.Name, seg.Kind, raw))
}

// DeleteSegment soft-deletes a segment.
func (s *Store) DeleteSegment(ctx context.Context, schoolID, id string) error {
	res, err := s.DB.ExecContext(ctx, `UPDATE campaign_segments SET deleted_at=now() WHERE id=$1 AND school_id=$2 AND deleted_at IS NULL`, id, schoolID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// ---- WhatsApp templates -------------------------------------------------------

const templateCols = `id, school_id, name, provider_template_id, language, body_preview, variables, approval_status, created_at, updated_at`

func scanTemplate(row scanner) (*WATemplate, error) {
	var (
		t   WATemplate
		raw []byte
	)
	if err := row.Scan(&t.ID, &t.SchoolID, &t.Name, &t.ProviderTemplateID, &t.Language, &t.BodyPreview, &raw, &t.ApprovalStatus, &t.CreatedAt, &t.UpdatedAt); err != nil {
		return nil, noRows(err)
	}
	_ = json.Unmarshal(raw, &t.Variables)
	if t.Variables == nil {
		t.Variables = []string{}
	}
	return &t, nil
}

// ListTemplates returns a school's active templates.
func (s *Store) ListTemplates(ctx context.Context, schoolID string) ([]WATemplate, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT `+templateCols+` FROM wa_templates
		WHERE school_id=$1 AND deleted_at IS NULL ORDER BY name`, schoolID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []WATemplate{}
	for rows.Next() {
		t, err := scanTemplate(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *t)
	}
	return out, rows.Err()
}

// GetTemplate loads one active template.
func (s *Store) GetTemplate(ctx context.Context, schoolID, id string) (*WATemplate, error) {
	return scanTemplate(s.DB.QueryRowContext(ctx, `SELECT `+templateCols+` FROM wa_templates
		WHERE id=$1 AND school_id=$2 AND deleted_at IS NULL`, id, schoolID))
}

// CreateTemplate inserts a template.
func (s *Store) CreateTemplate(ctx context.Context, t WATemplate) (*WATemplate, error) {
	raw, _ := json.Marshal(t.Variables)
	return scanTemplate(s.DB.QueryRowContext(ctx, `INSERT INTO wa_templates
		(school_id, name, provider_template_id, language, body_preview, variables, approval_status)
		VALUES ($1,$2,$3,$4,$5,$6,$7) RETURNING `+templateCols,
		t.SchoolID, t.Name, t.ProviderTemplateID, t.Language, t.BodyPreview, raw, t.ApprovalStatus))
}

// UpdateTemplate replaces a template's fields.
func (s *Store) UpdateTemplate(ctx context.Context, t WATemplate) (*WATemplate, error) {
	raw, _ := json.Marshal(t.Variables)
	return scanTemplate(s.DB.QueryRowContext(ctx, `UPDATE wa_templates SET name=$3, provider_template_id=$4, language=$5,
		body_preview=$6, variables=$7, approval_status=$8, updated_at=now()
		WHERE id=$1 AND school_id=$2 AND deleted_at IS NULL RETURNING `+templateCols,
		t.ID, t.SchoolID, t.Name, t.ProviderTemplateID, t.Language, t.BodyPreview, raw, t.ApprovalStatus))
}

// DeleteTemplate soft-deletes a template.
func (s *Store) DeleteTemplate(ctx context.Context, schoolID, id string) error {
	res, err := s.DB.ExecContext(ctx, `UPDATE wa_templates SET deleted_at=now() WHERE id=$1 AND school_id=$2 AND deleted_at IS NULL`, id, schoolID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// ---- campaigns -----------------------------------------------------------------

const campaignCols = `id, school_id, name, segment_id, channel, template_id, template_vars, subject, body, scheduled_at,
	status, last_error, created_by, created_at, updated_at, sent_at`

func scanCampaign(row scanner) (*Campaign, error) {
	var (
		c                  Campaign
		templateID         sql.NullString
		subject, body, err sql.NullString
		scheduledAt, sent  sql.NullTime
		vars               []byte
	)
	if e := row.Scan(&c.ID, &c.SchoolID, &c.Name, &c.SegmentID, &c.Channel, &templateID, &vars, &subject, &body, &scheduledAt,
		&c.Status, &err, &c.CreatedBy, &c.CreatedAt, &c.UpdatedAt, &sent); e != nil {
		return nil, noRows(e)
	}
	c.TemplateID = nullStr(templateID)
	c.Subject = nullStr(subject)
	c.Body = nullStr(body)
	c.LastError = nullStr(err)
	c.ScheduledAt = nullTime(scheduledAt)
	c.SentAt = nullTime(sent)
	c.TemplateVars = map[string]string{}
	_ = json.Unmarshal(vars, &c.TemplateVars)
	return &c, nil
}

func nullStr(v sql.NullString) *string {
	if !v.Valid {
		return nil
	}
	return &v.String
}

func nullTime(v sql.NullTime) *time.Time {
	if !v.Valid {
		return nil
	}
	t := v.Time
	return &t
}

// ListCampaigns returns a page of a school's campaigns.
func (s *Store) ListCampaigns(ctx context.Context, schoolID, status string, page, perPage int) ([]Campaign, int, error) {
	var total int
	if err := s.DB.QueryRowContext(ctx, `SELECT count(*) FROM campaigns WHERE school_id=$1 AND ($2='' OR status=$2)`, schoolID, status).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT `+campaignCols+` FROM campaigns WHERE school_id=$1 AND ($2='' OR status=$2)
		ORDER BY created_at DESC LIMIT $3 OFFSET $4`, schoolID, status, perPage, (page-1)*perPage)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := []Campaign{}
	for rows.Next() {
		c, err := scanCampaign(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, *c)
	}
	return out, total, rows.Err()
}

// GetCampaign loads one campaign.
func (s *Store) GetCampaign(ctx context.Context, schoolID, id string) (*Campaign, error) {
	return scanCampaign(s.DB.QueryRowContext(ctx, `SELECT `+campaignCols+` FROM campaigns WHERE id=$1 AND school_id=$2`, id, schoolID))
}

// CreateCampaign inserts a draft campaign.
func (s *Store) CreateCampaign(ctx context.Context, c Campaign) (*Campaign, error) {
	vars, _ := json.Marshal(c.TemplateVars)
	return scanCampaign(s.DB.QueryRowContext(ctx, `INSERT INTO campaigns
		(school_id, name, segment_id, channel, template_id, template_vars, subject, body, status, created_by)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,'draft',$9) RETURNING `+campaignCols,
		c.SchoolID, c.Name, c.SegmentID, c.Channel, c.TemplateID, vars, c.Subject, c.Body, c.CreatedBy))
}

// UpdateCampaign edits a draft campaign.
func (s *Store) UpdateCampaign(ctx context.Context, c Campaign) (*Campaign, error) {
	vars, _ := json.Marshal(c.TemplateVars)
	return scanCampaign(s.DB.QueryRowContext(ctx, `UPDATE campaigns SET name=$3, segment_id=$4, channel=$5, template_id=$6,
		template_vars=$7, subject=$8, body=$9, updated_at=now()
		WHERE id=$1 AND school_id=$2 AND status='draft' RETURNING `+campaignCols,
		c.ID, c.SchoolID, c.Name, c.SegmentID, c.Channel, c.TemplateID, vars, c.Subject, c.Body))
}

// DeleteDraftCampaign removes a draft that never had recipients.
func (s *Store) DeleteDraftCampaign(ctx context.Context, schoolID, id string) error {
	res, err := s.DB.ExecContext(ctx, `DELETE FROM campaigns WHERE id=$1 AND school_id=$2 AND status='draft'
		AND NOT EXISTS (SELECT 1 FROM campaign_recipients r WHERE r.campaign_id=campaigns.id)`, id, schoolID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// ScheduleCampaign moves a draft/scheduled campaign to scheduled at t.
func (s *Store) ScheduleCampaign(ctx context.Context, schoolID, id string, at time.Time) (*Campaign, error) {
	return scanCampaign(s.DB.QueryRowContext(ctx, `UPDATE campaigns SET status='scheduled', scheduled_at=$3, updated_at=now()
		WHERE id=$1 AND school_id=$2 AND status IN ('draft','scheduled') RETURNING `+campaignCols, id, schoolID, at))
}

// CancelCampaign cancels a draft/scheduled campaign.
func (s *Store) CancelCampaign(ctx context.Context, schoolID, id string) (*Campaign, error) {
	return scanCampaign(s.DB.QueryRowContext(ctx, `UPDATE campaigns SET status='cancelled', updated_at=now()
		WHERE id=$1 AND school_id=$2 AND status IN ('draft','scheduled') RETURNING `+campaignCols, id, schoolID))
}

// ClaimCampaign atomically moves one scheduled campaign to sending.
func (s *Store) ClaimCampaign(ctx context.Context, id string) (*Campaign, error) {
	return scanCampaign(s.DB.QueryRowContext(ctx, `UPDATE campaigns SET status='sending', updated_at=now()
		WHERE id=$1 AND status='scheduled' RETURNING `+campaignCols, id))
}

// DueCampaignIDs lists scheduled campaigns that are due, plus campaigns stuck
// in sending (worker crash) for longer than staleAfter.
func (s *Store) DueCampaignIDs(ctx context.Context, now time.Time, staleAfter time.Duration) ([]string, error) {
	if _, err := s.DB.ExecContext(ctx, `UPDATE campaigns SET status='scheduled', updated_at=now()
		WHERE status='sending' AND updated_at < $1`, now.Add(-staleAfter)); err != nil {
		return nil, err
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT id FROM campaigns WHERE status='scheduled' AND scheduled_at <= $1
		ORDER BY scheduled_at LIMIT 20`, now)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// FinishCampaign marks a campaign sent or failed.
func (s *Store) FinishCampaign(ctx context.Context, id, status, lastError string) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE campaigns SET status=$2, last_error=NULLIF($3,''), updated_at=now(),
		sent_at=CASE WHEN $2='sent' THEN now() ELSE sent_at END WHERE id=$1`, id, status, lastError)
	return err
}

// ---- recipients ----------------------------------------------------------------

// RecipientInput is one ledger row to insert.
type RecipientInput struct {
	SubjectKey    string
	UserID        string
	DisplayName   string
	ContactMasked string
	ContactHash   string
	Status        string
	ExcludeReason string
}

// InsertRecipients inserts ledger rows; existing (campaign, subject) rows are
// left untouched so a re-run never duplicates or re-sends.
func (s *Store) InsertRecipients(ctx context.Context, schoolID, campaignID string, rows []RecipientInput) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	stmt, err := tx.PrepareContext(ctx, `INSERT INTO campaign_recipients
		(school_id, campaign_id, subject_key, user_id, display_name, contact_masked, contact_hash, status, exclude_reason)
		VALUES ($1,$2,$3,NULLIF($4,'')::uuid,$5,$6,$7,$8,NULLIF($9,''))
		ON CONFLICT (campaign_id, subject_key) DO NOTHING`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for _, r := range rows {
		if _, err := stmt.ExecContext(ctx, schoolID, campaignID, r.SubjectKey, r.UserID, r.DisplayName, r.ContactMasked, r.ContactHash, r.Status, r.ExcludeReason); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// QueuedRecipients returns subject keys still waiting to be sent.
func (s *Store) QueuedRecipients(ctx context.Context, campaignID string) ([]Recipient, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT id, subject_key, user_id FROM campaign_recipients
		WHERE campaign_id=$1 AND status='queued' ORDER BY created_at, id`, campaignID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Recipient
	for rows.Next() {
		var (
			r   Recipient
			uid sql.NullString
		)
		if err := rows.Scan(&r.ID, &r.SubjectKey, &uid); err != nil {
			return nil, err
		}
		r.UserID = nullStr(uid)
		out = append(out, r)
	}
	return out, rows.Err()
}

// MarkRecipientSent records a successful send.
func (s *Store) MarkRecipientSent(ctx context.Context, id, providerMessageID, notificationID string) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE campaign_recipients SET status='sent', sent_at=now(),
		provider_message_id=NULLIF($2,''), notification_id=NULLIF($3,'')::uuid WHERE id=$1 AND status='queued'`, id, providerMessageID, notificationID)
	return err
}

// MarkRecipientFailed records a failed send.
func (s *Store) MarkRecipientFailed(ctx context.Context, id, lastError string) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE campaign_recipients SET status='failed', last_error=$2 WHERE id=$1 AND status='queued'`, id, lastError)
	return err
}

// SyncInAppReads copies notifications.read_at onto in-app recipients.
func (s *Store) SyncInAppReads(ctx context.Context, campaignID string) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE campaign_recipients cr SET status='read', read_at=n.read_at
		FROM notifications n WHERE cr.campaign_id=$1 AND cr.notification_id=n.id AND n.read_at IS NOT NULL
		AND cr.status IN ('sent','delivered')`, campaignID)
	return err
}

// RecipientStates loads every state of a campaign for ComputeStats.
func (s *Store) RecipientStates(ctx context.Context, campaignID string) ([]RecipientState, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT status, COALESCE(exclude_reason,'') FROM campaign_recipients WHERE campaign_id=$1`, campaignID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []RecipientState
	for rows.Next() {
		var r RecipientState
		if err := rows.Scan(&r.Status, &r.ExcludeReason); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// ListRecipients pages a campaign's ledger, optionally by status.
func (s *Store) ListRecipients(ctx context.Context, campaignID, status string, page, perPage int) ([]Recipient, int, error) {
	var total int
	if err := s.DB.QueryRowContext(ctx, `SELECT count(*) FROM campaign_recipients WHERE campaign_id=$1 AND ($2='' OR status=$2)`, campaignID, status).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT id, subject_key, user_id, display_name, contact_masked, status, exclude_reason,
		provider_message_id, last_error, sent_at, delivered_at, read_at, replied_at
		FROM campaign_recipients WHERE campaign_id=$1 AND ($2='' OR status=$2)
		ORDER BY display_name, id LIMIT $3 OFFSET $4`, campaignID, status, perPage, (page-1)*perPage)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := []Recipient{}
	for rows.Next() {
		var (
			r                                    Recipient
			uid, reason, msgID, lastErr          sql.NullString
			sentAt, deliveredAt, readAt, replied sql.NullTime
		)
		if err := rows.Scan(&r.ID, &r.SubjectKey, &uid, &r.DisplayName, &r.ContactMasked, &r.Status, &reason, &msgID, &lastErr,
			&sentAt, &deliveredAt, &readAt, &replied); err != nil {
			return nil, 0, err
		}
		r.UserID, r.ExcludeReason, r.ProviderMessageID, r.LastError = nullStr(uid), nullStr(reason), nullStr(msgID), nullStr(lastErr)
		r.SentAt, r.DeliveredAt, r.ReadAt, r.RepliedAt = nullTime(sentAt), nullTime(deliveredAt), nullTime(readAt), nullTime(replied)
		out = append(out, r)
	}
	return out, total, rows.Err()
}

// RecipientByProviderMessage finds the ledger row for a provider message id.
func (s *Store) RecipientByProviderMessage(ctx context.Context, providerMessageID string) (id, schoolID, subjectKey string, err error) {
	err = s.DB.QueryRowContext(ctx, `SELECT id, school_id, subject_key FROM campaign_recipients WHERE provider_message_id=$1 LIMIT 1`,
		providerMessageID).Scan(&id, &schoolID, &subjectKey)
	return id, schoolID, subjectKey, noRows(err)
}

// ApplyWAStatus advances a recipient's WhatsApp status; a state never moves
// backwards (read does not regress to delivered).
func (s *Store) ApplyWAStatus(ctx context.Context, id, status string, at time.Time, lastError string) error {
	var q string
	switch status {
	case RecipientDelivered:
		q = `UPDATE campaign_recipients SET status='delivered', delivered_at=$2 WHERE id=$1 AND status='sent'`
	case RecipientRead:
		q = `UPDATE campaign_recipients SET status='read', read_at=$2, delivered_at=COALESCE(delivered_at,$2) WHERE id=$1 AND status IN ('sent','delivered')`
	case RecipientReplied:
		q = `UPDATE campaign_recipients SET status='replied', replied_at=$2, read_at=COALESCE(read_at,$2), delivered_at=COALESCE(delivered_at,$2)
			WHERE id=$1 AND status IN ('sent','delivered','read')`
	case RecipientFailed:
		_, err := s.DB.ExecContext(ctx, `UPDATE campaign_recipients SET status='failed', last_error=$2 WHERE id=$1 AND status IN ('queued','sent')`, id, lastError)
		return err
	default:
		return nil
	}
	_, err := s.DB.ExecContext(ctx, q, id, at)
	return err
}

// WAUsedThisMonth counts WhatsApp messages sent by a school this month (in loc).
func (s *Store) WAUsedThisMonth(ctx context.Context, schoolID string, now time.Time, loc *time.Location) (int, error) {
	local := now.In(loc)
	start := time.Date(local.Year(), local.Month(), 1, 0, 0, 0, 0, loc)
	var n int
	err := s.DB.QueryRowContext(ctx, `SELECT count(*) FROM campaign_recipients r JOIN campaigns c ON c.id=r.campaign_id
		WHERE r.school_id=$1 AND c.channel='whatsapp' AND r.status IN ('sent','delivered','read','replied') AND r.sent_at >= $2`,
		schoolID, start).Scan(&n)
	return n, err
}

// ---- settings --------------------------------------------------------------------

// MonthlyWAQuota returns a school's quota (nil = no quota).
func (s *Store) MonthlyWAQuota(ctx context.Context, schoolID string) (*int, error) {
	var q sql.NullInt64
	err := s.DB.QueryRowContext(ctx, `SELECT monthly_wa_quota FROM campaign_settings WHERE school_id=$1`, schoolID).Scan(&q)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && !q.Valid) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	v := int(q.Int64)
	return &v, nil
}

// SetMonthlyWAQuota upserts a school's quota.
func (s *Store) SetMonthlyWAQuota(ctx context.Context, schoolID string, quota *int) error {
	_, err := s.DB.ExecContext(ctx, `INSERT INTO campaign_settings (school_id, monthly_wa_quota) VALUES ($1,$2)
		ON CONFLICT (school_id) DO UPDATE SET monthly_wa_quota=EXCLUDED.monthly_wa_quota, updated_at=now()`, schoolID, quota)
	return err
}

// ---- consents ----------------------------------------------------------------------

// ConsentsFor returns subject_key -> status for one channel.
func (s *Store) ConsentsFor(ctx context.Context, schoolID, channel string, subjectKeys []string) (map[string]string, error) {
	out := map[string]string{}
	if len(subjectKeys) == 0 {
		return out, nil
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT subject_key, status FROM marketing_consents
		WHERE school_id=$1 AND channel=$2 AND subject_key = ANY($3)`, schoolID, channel, subjectKeys)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var k, st string
		if err := rows.Scan(&k, &st); err != nil {
			return nil, err
		}
		out[k] = st
	}
	return out, rows.Err()
}

// ListConsents returns every stored consent of one subject.
func (s *Store) ListConsents(ctx context.Context, schoolID, subjectKey string) ([]Consent, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT channel, status, source, changed_at FROM marketing_consents
		WHERE school_id=$1 AND subject_key=$2`, schoolID, subjectKey)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Consent{}
	for rows.Next() {
		var c Consent
		if err := rows.Scan(&c.Channel, &c.Status, &c.Source, &c.ChangedAt); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// SetConsent upserts the current consent and appends the history event.
func (s *Store) SetConsent(ctx context.Context, schoolID, subjectKey, userID, channel, status, source, actorUserID string) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `INSERT INTO marketing_consents (school_id, subject_key, user_id, channel, status, source)
		VALUES ($1,$2,NULLIF($3,'')::uuid,$4,$5,$6)
		ON CONFLICT (school_id, subject_key, channel) DO UPDATE SET status=EXCLUDED.status, source=EXCLUDED.source,
		user_id=COALESCE(EXCLUDED.user_id, marketing_consents.user_id), changed_at=now()`,
		schoolID, subjectKey, userID, channel, status, source); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO marketing_consent_events (school_id, subject_key, channel, status, source, actor_user_id)
		VALUES ($1,$2,$3,$4,$5,NULLIF($6,'')::uuid)`, schoolID, subjectKey, channel, status, source, actorUserID); err != nil {
		return err
	}
	return tx.Commit()
}
