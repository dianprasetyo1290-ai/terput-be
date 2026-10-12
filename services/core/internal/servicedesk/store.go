package servicedesk

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
)

var (
	ErrNotFound = errors.New("data tidak ditemukan")
	ErrConflict = errors.New("konflik data")
)

// Store is the SQL layer of the service desk (same *sql.DB as core's store).
type Store struct{ DB *sql.DB }

func isUnique(err error) bool {
	var pg *pgconn.PgError
	return errors.As(err, &pg) && pg.Code == "23505"
}

func conflict(msg string) error { return fmt.Errorf("%w: %s", ErrConflict, msg) }

func jsonb(v any) []byte {
	b, _ := json.Marshal(v)
	return b
}

// ---------------------------------------------------------------------------
// Teams
// ---------------------------------------------------------------------------

func (s *Store) ListTeams(ctx context.Context, schoolID string) ([]Team, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT t.id, t.school_id, t.name, t.manager_user_id, t.created_at,
		       coalesce(array_agg(m.user_id::text) FILTER (WHERE m.user_id IS NOT NULL), '{}')
		FROM sd_teams t LEFT JOIN sd_team_members m ON m.team_id = t.id
		WHERE t.school_id = $1 AND t.deleted_at IS NULL
		GROUP BY t.id ORDER BY t.name`, schoolID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Team{}
	for rows.Next() {
		var t Team
		var members []string
		if err := rows.Scan(&t.ID, &t.SchoolID, &t.Name, &t.ManagerUserID, &t.CreatedAt, &members); err != nil {
			return nil, err
		}
		t.MemberIDs = members
		out = append(out, t)
	}
	return out, rows.Err()
}

// SaveTeam creates (id "") or updates a team and replaces its member list.
func (s *Store) SaveTeam(ctx context.Context, schoolID, id string, t Team) (string, error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer tx.Rollback()
	if id == "" {
		err = tx.QueryRowContext(ctx, `INSERT INTO sd_teams (school_id, name, manager_user_id) VALUES ($1,$2,$3) RETURNING id`,
			schoolID, t.Name, t.ManagerUserID).Scan(&id)
	} else {
		var res sql.Result
		res, err = tx.ExecContext(ctx, `UPDATE sd_teams SET name=$1, manager_user_id=$2, updated_at=now()
			WHERE id=$3 AND school_id=$4 AND deleted_at IS NULL`, t.Name, t.ManagerUserID, id, schoolID)
		if err == nil {
			if n, _ := res.RowsAffected(); n == 0 {
				return "", ErrNotFound
			}
		}
	}
	if err != nil {
		return "", err
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM sd_team_members WHERE team_id=$1`, id); err != nil {
		return "", err
	}
	for _, uid := range t.MemberIDs {
		if _, err = tx.ExecContext(ctx, `INSERT INTO sd_team_members (team_id, user_id, school_id) VALUES ($1,$2,$3)
			ON CONFLICT DO NOTHING`, id, uid, schoolID); err != nil {
			return "", err
		}
	}
	return id, tx.Commit()
}

func (s *Store) DeleteTeam(ctx context.Context, schoolID, id string) error {
	var used bool
	if err := s.DB.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM sd_categories WHERE team_id=$1 AND deleted_at IS NULL)`, id).Scan(&used); err != nil {
		return err
	}
	if used {
		return conflict("tim masih dipakai kategori aktif")
	}
	res, err := s.DB.ExecContext(ctx, `UPDATE sd_teams SET deleted_at=now() WHERE id=$1 AND school_id=$2 AND deleted_at IS NULL`, id, schoolID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// UserTeamIDs lists the teams a user belongs to or supervises.
func (s *Store) UserTeamIDs(ctx context.Context, schoolID, userID string) ([]string, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT m.team_id::text FROM sd_team_members m JOIN sd_teams t ON t.id = m.team_id AND t.deleted_at IS NULL
		WHERE m.school_id=$1 AND m.user_id=$2
		UNION
		SELECT id::text FROM sd_teams WHERE school_id=$1 AND manager_user_id=$2 AND deleted_at IS NULL`, schoolID, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ids := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// TeamRecipients returns the members and the manager of a team.
func (s *Store) TeamRecipients(ctx context.Context, schoolID, teamID string) (members []string, manager *string, err error) {
	err = s.DB.QueryRowContext(ctx, `
		SELECT t.manager_user_id, coalesce(array_agg(m.user_id::text) FILTER (WHERE m.user_id IS NOT NULL), '{}')
		FROM sd_teams t LEFT JOIN sd_team_members m ON m.team_id = t.id
		WHERE t.id=$1 AND t.school_id=$2 GROUP BY t.id`, teamID, schoolID).Scan(&manager, &members)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil, ErrNotFound
	}
	return members, manager, err
}

// ---------------------------------------------------------------------------
// Categories
// ---------------------------------------------------------------------------

const categoryCols = `c.id, c.school_id, c.name, c.description, c.team_id, t.name, c.is_facility, c.form_schema,
	c.reporter_kinds, c.service_request_category_id, c.is_fallback, c.active`

func scanCategory(row interface{ Scan(...any) error }) (Category, error) {
	var c Category
	var schema []byte
	err := row.Scan(&c.ID, &c.SchoolID, &c.Name, &c.Description, &c.TeamID, &c.TeamName, &c.IsFacility, &schema,
		&c.ReporterKinds, &c.ServiceRequestCategoryID, &c.IsFallback, &c.Active)
	if err != nil {
		return c, err
	}
	c.FormSchema = []FormField{}
	_ = json.Unmarshal(schema, &c.FormSchema)
	return c, nil
}

func (s *Store) ListCategories(ctx context.Context, schoolID string, activeOnly bool) ([]Category, error) {
	q := `SELECT ` + categoryCols + ` FROM sd_categories c JOIN sd_teams t ON t.id = c.team_id
		WHERE c.school_id=$1 AND c.deleted_at IS NULL`
	if activeOnly {
		q += ` AND c.active`
	}
	rows, err := s.DB.QueryContext(ctx, q+` ORDER BY c.name`, schoolID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Category{}
	for rows.Next() {
		c, err := scanCategory(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (s *Store) GetCategory(ctx context.Context, schoolID, id string) (*Category, error) {
	c, err := scanCategory(s.DB.QueryRowContext(ctx, `SELECT `+categoryCols+` FROM sd_categories c JOIN sd_teams t ON t.id = c.team_id
		WHERE c.school_id=$1 AND c.id=$2 AND c.deleted_at IS NULL`, schoolID, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &c, nil
}

// CategoryForServiceRequest resolves the ticket category of a menu 81
// request: the active mapped category, else the active fallback (BR-SD-01).
func (s *Store) CategoryForServiceRequest(ctx context.Context, schoolID, srCategoryID string) (*Category, error) {
	c, err := scanCategory(s.DB.QueryRowContext(ctx, `SELECT `+categoryCols+` FROM sd_categories c JOIN sd_teams t ON t.id = c.team_id
		WHERE c.school_id=$1 AND c.deleted_at IS NULL AND c.active
		  AND (c.service_request_category_id=$2 OR c.is_fallback)
		ORDER BY (c.service_request_category_id IS NOT DISTINCT FROM $2::uuid) DESC LIMIT 1`, schoolID, srCategoryID))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &c, nil
}

func (s *Store) SaveCategory(ctx context.Context, schoolID, id string, c Category) (string, error) {
	var teamOK bool
	if err := s.DB.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM sd_teams WHERE id=$1 AND school_id=$2 AND deleted_at IS NULL)`,
		c.TeamID, schoolID).Scan(&teamOK); err != nil {
		return "", err
	}
	if !teamOK {
		return "", conflict("tim tidak ditemukan di sekolah ini")
	}
	if c.ServiceRequestCategoryID != nil {
		var srOK bool
		if err := s.DB.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM service_request_categories WHERE id=$1 AND school_id=$2 AND deleted_at IS NULL)`,
			*c.ServiceRequestCategoryID, schoolID).Scan(&srOK); err != nil {
			return "", err
		}
		if !srOK {
			return "", conflict("kategori layanan orang tua tidak ditemukan")
		}
	}
	var err error
	if id == "" {
		err = s.DB.QueryRowContext(ctx, `INSERT INTO sd_categories (school_id, name, description, team_id, is_facility, form_schema,
			reporter_kinds, service_request_category_id, is_fallback, active) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10) RETURNING id`,
			schoolID, c.Name, c.Description, c.TeamID, c.IsFacility, jsonb(c.FormSchema), c.ReporterKinds,
			c.ServiceRequestCategoryID, c.IsFallback, c.Active).Scan(&id)
	} else {
		var res sql.Result
		res, err = s.DB.ExecContext(ctx, `UPDATE sd_categories SET name=$1, description=$2, team_id=$3, is_facility=$4, form_schema=$5,
			reporter_kinds=$6, service_request_category_id=$7, is_fallback=$8, active=$9, updated_at=now()
			WHERE id=$10 AND school_id=$11 AND deleted_at IS NULL`,
			c.Name, c.Description, c.TeamID, c.IsFacility, jsonb(c.FormSchema), c.ReporterKinds,
			c.ServiceRequestCategoryID, c.IsFallback, c.Active, id, schoolID)
		if err == nil {
			if n, _ := res.RowsAffected(); n == 0 {
				return "", ErrNotFound
			}
		}
	}
	if isUnique(err) {
		return "", conflict("hanya boleh satu kategori cadangan, dan satu kategori per kategori layanan orang tua")
	}
	return id, err
}

func (s *Store) DeleteCategory(ctx context.Context, schoolID, id string) error {
	// Soft delete also releases the unique mapping/fallback slots.
	res, err := s.DB.ExecContext(ctx, `UPDATE sd_categories SET deleted_at=now(), active=false, is_fallback=false,
		service_request_category_id=NULL, updated_at=now() WHERE id=$1 AND school_id=$2 AND deleted_at IS NULL`, id, schoolID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// ---------------------------------------------------------------------------
// Business hours, holidays, calendar
// ---------------------------------------------------------------------------

func (s *Store) BusinessHours(ctx context.Context, schoolID string) ([]BusinessHour, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT weekday, to_char(opens_at,'HH24:MI'), to_char(closes_at,'HH24:MI')
		FROM sd_business_hours WHERE school_id=$1 ORDER BY weekday`, schoolID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []BusinessHour{}
	for rows.Next() {
		var b BusinessHour
		if err := rows.Scan(&b.Weekday, &b.OpensAt, &b.ClosesAt); err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

func (s *Store) ReplaceBusinessHours(ctx context.Context, schoolID string, hours []BusinessHour) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `DELETE FROM sd_business_hours WHERE school_id=$1`, schoolID); err != nil {
		return err
	}
	for _, h := range hours {
		if _, err := tx.ExecContext(ctx, `INSERT INTO sd_business_hours (school_id, weekday, opens_at, closes_at) VALUES ($1,$2,$3,$4)`,
			schoolID, h.Weekday, h.OpensAt, h.ClosesAt); err != nil {
			if isUnique(err) {
				return conflict("hari ganda pada jam layanan")
			}
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) ListHolidays(ctx context.Context, schoolID string) ([]Holiday, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT id, to_char(date,'YYYY-MM-DD'), name FROM sd_holidays WHERE school_id=$1 ORDER BY date`, schoolID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Holiday{}
	for rows.Next() {
		var h Holiday
		if err := rows.Scan(&h.ID, &h.Date, &h.Name); err != nil {
			return nil, err
		}
		out = append(out, h)
	}
	return out, rows.Err()
}

func (s *Store) CreateHoliday(ctx context.Context, schoolID string, h Holiday) (string, error) {
	var id string
	err := s.DB.QueryRowContext(ctx, `INSERT INTO sd_holidays (school_id, date, name) VALUES ($1,$2,$3) RETURNING id`,
		schoolID, h.Date, h.Name).Scan(&id)
	if isUnique(err) {
		return "", conflict("tanggal libur sudah ada")
	}
	return id, err
}

func (s *Store) DeleteHoliday(ctx context.Context, schoolID, id string) error {
	res, err := s.DB.ExecContext(ctx, `DELETE FROM sd_holidays WHERE id=$1 AND school_id=$2`, id, schoolID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// LoadCalendar builds a school's business calendar in loc.
func (s *Store) LoadCalendar(ctx context.Context, schoolID string, loc *time.Location) (Calendar, error) {
	cal := Calendar{Loc: loc, Hours: map[time.Weekday]Window{}, Holidays: map[string]bool{}}
	hours, err := s.BusinessHours(ctx, schoolID)
	if err != nil {
		return cal, err
	}
	for _, h := range hours {
		o, err1 := ParseClock(h.OpensAt)
		c, err2 := ParseClock(h.ClosesAt)
		if err1 != nil || err2 != nil {
			return cal, fmt.Errorf("jam layanan rusak untuk hari %d", h.Weekday)
		}
		cal.Hours[ISOWeekday(h.Weekday)] = Window{Open: o, Close: c}
	}
	hols, err := s.ListHolidays(ctx, schoolID)
	if err != nil {
		return cal, err
	}
	for _, h := range hols {
		cal.Holidays[h.Date] = true
	}
	return cal, nil
}

// ---------------------------------------------------------------------------
// SLA policies + escalation rules
// ---------------------------------------------------------------------------

func (s *Store) ListPolicies(ctx context.Context, schoolID string) ([]Policy, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT id, school_id, category_id, priority, response_value, response_unit,
		resolution_value, resolution_unit, warn_bp, to_char(effective_from,'YYYY-MM-DD'), to_char(effective_to,'YYYY-MM-DD')
		FROM sd_sla_policies WHERE school_id=$1 AND deleted_at IS NULL ORDER BY effective_from DESC, priority`, schoolID)
	if err != nil {
		return nil, err
	}
	out := []Policy{}
	idx := map[string]int{}
	for rows.Next() {
		var p Policy
		if err := rows.Scan(&p.ID, &p.SchoolID, &p.CategoryID, &p.Priority, &p.ResponseValue, &p.ResponseUnit,
			&p.ResolutionValue, &p.ResolutionUnit, &p.WarnBp, &p.EffectiveFrom, &p.EffectiveTo); err != nil {
			rows.Close()
			return nil, err
		}
		p.Rules = []EscalationRule{}
		idx[p.ID] = len(out)
		out = append(out, p)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	rrows, err := s.DB.QueryContext(ctx, `SELECT id, policy_id, trigger, level, target_kind, target_role, target_user_id
		FROM sd_escalation_rules WHERE school_id=$1 ORDER BY level`, schoolID)
	if err != nil {
		return nil, err
	}
	defer rrows.Close()
	for rrows.Next() {
		var r EscalationRule
		var pid string
		if err := rrows.Scan(&r.ID, &pid, &r.Trigger, &r.Level, &r.TargetKind, &r.TargetRole, &r.TargetUserID); err != nil {
			return nil, err
		}
		if i, ok := idx[pid]; ok {
			out[i].Rules = append(out[i].Rules, r)
		}
	}
	return out, rrows.Err()
}

func (s *Store) SavePolicy(ctx context.Context, schoolID, id string, p Policy) (string, error) {
	if p.CategoryID != nil {
		if _, err := s.GetCategory(ctx, schoolID, *p.CategoryID); err != nil {
			if errors.Is(err, ErrNotFound) {
				return "", conflict("kategori tidak ditemukan")
			}
			return "", err
		}
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer tx.Rollback()
	if id == "" {
		err = tx.QueryRowContext(ctx, `INSERT INTO sd_sla_policies (school_id, category_id, priority, response_value, response_unit,
			resolution_value, resolution_unit, warn_bp, effective_from, effective_to) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10) RETURNING id`,
			schoolID, p.CategoryID, p.Priority, p.ResponseValue, p.ResponseUnit, p.ResolutionValue, p.ResolutionUnit,
			p.WarnBp, p.EffectiveFrom, p.EffectiveTo).Scan(&id)
	} else {
		var res sql.Result
		res, err = tx.ExecContext(ctx, `UPDATE sd_sla_policies SET category_id=$1, priority=$2, response_value=$3, response_unit=$4,
			resolution_value=$5, resolution_unit=$6, warn_bp=$7, effective_from=$8, effective_to=$9, updated_at=now()
			WHERE id=$10 AND school_id=$11 AND deleted_at IS NULL`,
			p.CategoryID, p.Priority, p.ResponseValue, p.ResponseUnit, p.ResolutionValue, p.ResolutionUnit,
			p.WarnBp, p.EffectiveFrom, p.EffectiveTo, id, schoolID)
		if err == nil {
			if n, _ := res.RowsAffected(); n == 0 {
				return "", ErrNotFound
			}
		}
	}
	if err != nil {
		return "", err
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM sd_escalation_rules WHERE policy_id=$1`, id); err != nil {
		return "", err
	}
	for _, r := range p.Rules {
		_, err = tx.ExecContext(ctx, `INSERT INTO sd_escalation_rules (school_id, policy_id, trigger, level, target_kind, target_role, target_user_id)
			VALUES ($1,$2,$3,$4,$5,$6,$7)`, schoolID, id, r.Trigger, r.Level, r.TargetKind, r.TargetRole, r.TargetUserID)
		if isUnique(err) {
			return "", conflict("level eskalasi ganda untuk pemicu yang sama")
		}
		if err != nil {
			return "", err
		}
	}
	return id, tx.Commit()
}

func (s *Store) DeletePolicy(ctx context.Context, schoolID, id string) error {
	res, err := s.DB.ExecContext(ctx, `UPDATE sd_sla_policies SET deleted_at=now() WHERE id=$1 AND school_id=$2 AND deleted_at IS NULL`, id, schoolID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// ---------------------------------------------------------------------------
// Tickets
// ---------------------------------------------------------------------------

const ticketCols = `t.id, t.school_id, t.number, t.category_id, c.name, c.is_facility, t.team_id, tm.name, t.priority, t.subject,
	t.description, t.form_answers, t.channel, t.reporter_user_id, t.student_user_id, t.service_request_id, t.status,
	t.assignee_user_id, t.sla_snapshot, t.response_due_at, t.resolution_due_at, t.first_responded_at, t.resolved_at,
	t.closed_at, t.response_breached_at, t.resolution_breached_at, t.escalation_level, t.work_order_id, t.work_order_number,
	t.satisfaction_score, t.satisfaction_comment, t.rated_at, t.created_at, t.updated_at`

const ticketFrom = ` FROM sd_tickets t JOIN sd_categories c ON c.id = t.category_id JOIN sd_teams tm ON tm.id = t.team_id`

func scanTicket(row interface{ Scan(...any) error }) (Ticket, error) {
	var t Ticket
	var answers, sla []byte
	err := row.Scan(&t.ID, &t.SchoolID, &t.Number, &t.CategoryID, &t.CategoryName, &t.IsFacility, &t.TeamID, &t.TeamName,
		&t.Priority, &t.Subject, &t.Description, &answers, &t.Channel, &t.ReporterUserID, &t.StudentUserID,
		&t.ServiceRequestID, &t.Status, &t.AssigneeUserID, &sla, &t.ResponseDueAt, &t.ResolutionDueAt,
		&t.FirstRespondedAt, &t.ResolvedAt, &t.ClosedAt, &t.ResponseBreachedAt, &t.ResolutionBreachedAt,
		&t.EscalationLevel, &t.WorkOrderID, &t.WorkOrderNumber, &t.SatisfactionScore, &t.SatisfactionComment,
		&t.RatedAt, &t.CreatedAt, &t.UpdatedAt)
	if err != nil {
		return t, err
	}
	t.FormAnswers = json.RawMessage(answers)
	if len(sla) > 0 {
		var p Policy
		if json.Unmarshal(sla, &p) == nil {
			t.SLA = &p
		}
	}
	return t, nil
}

// NewTicket is the insert payload; SLA fields are computed by the service.
type NewTicket struct {
	SchoolID         string
	CategoryID       string
	TeamID           string
	Priority         string
	Subject          string
	Description      string
	FormAnswers      map[string]any
	Channel          string
	ReporterUserID   string
	StudentUserID    *string
	ServiceRequestID *string
	SLA              *Policy
	ResponseDueAt    *time.Time
	ResolutionDueAt  *time.Time
	Attachments      []Attachment
	ActorUserID      string
	IdempotencyKey   string
	CreatedAt        time.Time
}

// CreateTicket inserts a ticket with the next yearly number and its opening
// event. A repeated idempotency key returns the existing ticket (created=false).
func (s *Store) CreateTicket(ctx context.Context, n NewTicket) (*Ticket, bool, error) {
	if t, err := s.ticketWhere(ctx, `t.school_id=$1 AND t.idempotency_key=$2`, n.SchoolID, n.IdempotencyKey); err == nil {
		return t, false, nil
	} else if !errors.Is(err, ErrNotFound) {
		return nil, false, err
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, false, err
	}
	defer tx.Rollback()
	year := n.CreatedAt.Year()
	var seq int
	if err := tx.QueryRowContext(ctx, `INSERT INTO sd_ticket_sequences (school_id, year, last) VALUES ($1,$2,1)
		ON CONFLICT (school_id, year) DO UPDATE SET last = sd_ticket_sequences.last + 1 RETURNING last`,
		n.SchoolID, year).Scan(&seq); err != nil {
		return nil, false, err
	}
	answers := n.FormAnswers
	if answers == nil {
		answers = map[string]any{}
	}
	var sla []byte
	if n.SLA != nil {
		sla = jsonb(n.SLA)
	}
	var id string
	err = tx.QueryRowContext(ctx, `INSERT INTO sd_tickets (school_id, number, category_id, team_id, priority, subject, description,
		form_answers, channel, reporter_user_id, student_user_id, service_request_id, status, sla_snapshot, response_due_at,
		resolution_due_at, idempotency_key, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,'new',$13,$14,$15,$16,$17,$17) RETURNING id`,
		n.SchoolID, TicketNumber(year, seq), n.CategoryID, n.TeamID, n.Priority, n.Subject, n.Description, jsonb(answers),
		n.Channel, n.ReporterUserID, n.StudentUserID, n.ServiceRequestID, sla, n.ResponseDueAt, n.ResolutionDueAt,
		n.IdempotencyKey, n.CreatedAt).Scan(&id)
	if isUnique(err) {
		_ = tx.Rollback()
		t, gerr := s.ticketWhere(ctx, `t.school_id=$1 AND (t.idempotency_key=$2 OR t.service_request_id IS NOT DISTINCT FROM $3::uuid)`,
			n.SchoolID, n.IdempotencyKey, n.ServiceRequestID)
		if gerr != nil {
			return nil, false, err
		}
		return t, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	atts := n.Attachments
	if atts == nil {
		atts = []Attachment{}
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO sd_ticket_events (school_id, ticket_id, kind, to_status, attachments, actor_user_id, created_at)
		VALUES ($1,$2,'status','new',$3,$4,$5)`, n.SchoolID, id, jsonb(atts), n.ActorUserID, n.CreatedAt); err != nil {
		return nil, false, err
	}
	if err := tx.Commit(); err != nil {
		return nil, false, err
	}
	t, err := s.GetTicket(ctx, n.SchoolID, id)
	return t, true, err
}

func (s *Store) ticketWhere(ctx context.Context, where string, args ...any) (*Ticket, error) {
	t, err := scanTicket(s.DB.QueryRowContext(ctx, `SELECT `+ticketCols+ticketFrom+` WHERE `+where, args...))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &t, nil
}

func (s *Store) GetTicket(ctx context.Context, schoolID, id string) (*Ticket, error) {
	return s.ticketWhere(ctx, `t.school_id=$1 AND t.id=$2`, schoolID, id)
}

func (s *Store) TicketByServiceRequest(ctx context.Context, schoolID, srID string) (*Ticket, error) {
	return s.ticketWhere(ctx, `t.school_id=$1 AND t.service_request_id=$2`, schoolID, srID)
}

// TicketFilter narrows ListTickets. TeamIDs nil = no team restriction.
type TicketFilter struct {
	SchoolID   string
	Status     string
	CategoryID string
	TeamID     string
	AssigneeID string
	ReporterID string
	SLA        string // warning | breached
	TeamIDs    []string
	Now        time.Time
}

func (s *Store) ListTickets(ctx context.Context, f TicketFilter, page, perPage int) ([]Ticket, int, error) {
	where := []string{"t.school_id=$1"}
	args := []any{f.SchoolID}
	add := func(cond string, v any) {
		args = append(args, v)
		where = append(where, strings.ReplaceAll(cond, "?", fmt.Sprintf("$%d", len(args))))
	}
	if f.Status != "" {
		add("t.status=?", f.Status)
	}
	if f.CategoryID != "" {
		add("t.category_id=?", f.CategoryID)
	}
	if f.TeamID != "" {
		add("t.team_id=?", f.TeamID)
	}
	if f.AssigneeID != "" {
		add("t.assignee_user_id=?", f.AssigneeID)
	}
	if f.ReporterID != "" {
		add("t.reporter_user_id=?", f.ReporterID)
	}
	if f.TeamIDs != nil {
		add("t.team_id::text = ANY(?)", f.TeamIDs)
	}
	switch f.SLA {
	case SLABreached:
		add("(t.response_breached_at IS NOT NULL OR t.resolution_breached_at IS NOT NULL OR (t.status IN ('new','assigned','in_progress') AND t.resolution_due_at < ?))", f.Now)
	case SLAWarning:
		// Approximation for the filter: escalated by a warning rule but not breached.
		where = append(where, "t.status IN ('new','assigned','in_progress') AND t.escalation_level > 0 AND t.response_breached_at IS NULL AND t.resolution_breached_at IS NULL")
	}
	w := strings.Join(where, " AND ")
	var total int
	if err := s.DB.QueryRowContext(ctx, `SELECT count(*) FROM sd_tickets t WHERE `+w, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	args = append(args, perPage, (page-1)*perPage)
	rows, err := s.DB.QueryContext(ctx, `SELECT `+ticketCols+ticketFrom+` WHERE `+w+
		fmt.Sprintf(` ORDER BY t.created_at DESC LIMIT $%d OFFSET $%d`, len(args)-1, len(args)), args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := []Ticket{}
	for rows.Next() {
		t, err := scanTicket(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, t)
	}
	return out, total, rows.Err()
}

func (s *Store) ListEvents(ctx context.Context, schoolID, ticketID string, includeInternal bool) ([]Event, error) {
	q := `SELECT id, kind, from_status, to_status, body, internal, attachments, actor_user_id, created_at
		FROM sd_ticket_events WHERE school_id=$1 AND ticket_id=$2`
	if !includeInternal {
		q += ` AND NOT internal AND kind NOT IN ('escalation','sla_breach')`
	}
	rows, err := s.DB.QueryContext(ctx, q+` ORDER BY created_at, id`, schoolID, ticketID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Event{}
	for rows.Next() {
		var e Event
		var atts []byte
		if err := rows.Scan(&e.ID, &e.Kind, &e.FromStatus, &e.ToStatus, &e.Body, &e.Internal, &atts, &e.ActorUserID, &e.CreatedAt); err != nil {
			return nil, err
		}
		e.Attachments = []Attachment{}
		_ = json.Unmarshal(atts, &e.Attachments)
		out = append(out, e)
	}
	return out, rows.Err()
}

// Change is one mutation applied to a locked ticket by Mutate.
type Change struct {
	ActorUserID string
	ByAgent     bool   // actor handles the ticket (counts as first response)
	To          string // new status ("" = none)
	Note        string
	AssigneeID  string // non-empty = (re)assign
	Comment     *string
	Internal    bool
	Attachments []Attachment
	Now         time.Time
}

// MutateResult reports what Mutate changed.
type MutateResult struct {
	Ticket     *Ticket
	EventID    string // the public event the reporter is told about ("" = none)
	StatusFrom string
}

// Mutate applies a status change / assignment / comment inside one
// transaction and mirrors the status to the linked menu 81 request (BR-SD-01).
func (s *Store) Mutate(ctx context.Context, schoolID, id string, c Change) (*MutateResult, error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	var status, number, reporter string
	var assignee, srID *string
	var firstResp *time.Time
	err = tx.QueryRowContext(ctx, `SELECT status, number, reporter_user_id, assignee_user_id, service_request_id, first_responded_at
		FROM sd_tickets WHERE id=$1 AND school_id=$2 FOR UPDATE`, id, schoolID).
		Scan(&status, &number, &reporter, &assignee, &srID, &firstResp)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	res := &MutateResult{StatusFrom: status}
	newStatus := status
	insertEvent := func(kind string, from, to, body *string, internal bool, atts []Attachment) (string, error) {
		if atts == nil {
			atts = []Attachment{}
		}
		var eid string
		err := tx.QueryRowContext(ctx, `INSERT INTO sd_ticket_events (school_id, ticket_id, kind, from_status, to_status, body, internal,
			attachments, actor_user_id, created_at) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10) RETURNING id`,
			schoolID, id, kind, from, to, body, internal, jsonb(atts), c.ActorUserID, c.Now).Scan(&eid)
		return eid, err
	}

	if c.AssigneeID != "" {
		if !IsOpen(status) {
			return nil, fmt.Errorf("%w: tiket sudah selesai", ErrInvalidTransition)
		}
		if _, err := tx.ExecContext(ctx, `UPDATE sd_tickets SET assignee_user_id=$1, updated_at=$2 WHERE id=$3`, c.AssigneeID, c.Now, id); err != nil {
			return nil, err
		}
		if _, err := insertEvent("assign", nil, nil, &c.AssigneeID, true, nil); err != nil {
			return nil, err
		}
		assignee = &c.AssigneeID
		if status == StatusNew && c.To == "" {
			c.To = StatusAssigned
		}
	}
	if c.To != "" {
		if err := ValidTransition(status, c.To, c.Note); err != nil {
			return nil, err
		}
		if c.To == StatusAssigned && assignee == nil {
			return nil, fmt.Errorf("%w: tetapkan petugas terlebih dahulu", ErrInvalidTransition)
		}
		set := `status=$1, updated_at=$2`
		switch c.To {
		case StatusResolved:
			set += `, resolved_at=$2`
		case StatusClosed:
			set += `, closed_at=$2`
		case StatusRejected:
			set += `, closed_at=$2`
		case StatusInProgress:
			if status == StatusResolved {
				set += `, resolved_at=NULL`
			}
		}
		if _, err := tx.ExecContext(ctx, `UPDATE sd_tickets SET `+set+` WHERE id=$3`, c.To, c.Now, id); err != nil {
			return nil, err
		}
		var note *string
		if strings.TrimSpace(c.Note) != "" {
			note = &c.Note
		}
		from, to := status, c.To
		eid, err := insertEvent("status", &from, &to, note, false, nil)
		if err != nil {
			return nil, err
		}
		res.EventID = eid
		newStatus = c.To
	}
	if c.Comment != nil {
		eid, err := insertEvent("comment", nil, nil, c.Comment, c.Internal, c.Attachments)
		if err != nil {
			return nil, err
		}
		if !c.Internal && res.EventID == "" && c.ActorUserID != reporter {
			res.EventID = eid
		}
	}
	publicTouch := c.To != "" || (c.Comment != nil && !c.Internal)
	if c.ByAgent && firstResp == nil && publicTouch {
		if _, err := tx.ExecContext(ctx, `UPDATE sd_tickets SET first_responded_at=$1 WHERE id=$2`, c.Now, id); err != nil {
			return nil, err
		}
	}
	if srID != nil && newStatus != status {
		if err := syncServiceRequest(ctx, tx, schoolID, id, *srID, number, reporter, newStatus, c, assignee); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	t, err := s.GetTicket(ctx, schoolID, id)
	if err != nil {
		return nil, err
	}
	res.Ticket = t
	return res, nil
}

// syncServiceRequest mirrors a ticket status onto service_requests (one-way)
// and, on resolution, copies the agents' public attachments as result files.
func syncServiceRequest(ctx context.Context, tx *sql.Tx, schoolID, ticketID, srID, number, reporter, ticketStatus string, c Change, assignee *string) error {
	var cur string
	if err := tx.QueryRowContext(ctx, `SELECT status FROM service_requests WHERE id=$1 AND school_id=$2 FOR UPDATE`, srID, schoolID).Scan(&cur); err != nil {
		return fmt.Errorf("lock service request: %w", err)
	}
	next := ToServiceRequestStatus(ticketStatus)
	if next == cur {
		return nil
	}
	if ticketStatus == StatusResolved {
		if _, err := tx.ExecContext(ctx, `UPDATE service_requests SET result_attachments = coalesce((
				SELECT jsonb_agg(a) FROM sd_ticket_events e, jsonb_array_elements(e.attachments) a
				WHERE e.ticket_id=$1 AND NOT e.internal AND e.actor_user_id IS DISTINCT FROM $2::uuid), '[]'::jsonb)
			WHERE id=$3`, ticketID, reporter, srID); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, `UPDATE service_requests SET status=$1, assignee_user_id=coalesce($2, assignee_user_id), updated_at=$3 WHERE id=$4`,
		next, assignee, c.Now, srID); err != nil {
		return err
	}
	note := "Sinkron dari tiket " + number
	if strings.TrimSpace(c.Note) != "" {
		note += ": " + c.Note
	}
	_, err := tx.ExecContext(ctx, `INSERT INTO service_request_events (school_id, request_id, from_status, to_status, note, actor_user_id, created_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7)`, schoolID, srID, cur, next, note, c.ActorUserID, c.Now)
	return err
}

// SetWorkOrder links a work order once (AC-137-01).
func (s *Store) SetWorkOrder(ctx context.Context, schoolID, id, woID, woNumber, actor string, now time.Time) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	res, err := tx.ExecContext(ctx, `UPDATE sd_tickets SET work_order_id=$1, work_order_number=$2, updated_at=$3
		WHERE id=$4 AND school_id=$5 AND work_order_id IS NULL`, woID, woNumber, now, id, schoolID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return nil // already linked by a concurrent call; the replay returned the same order
	}
	body := "Perintah kerja " + woNumber
	if _, err := tx.ExecContext(ctx, `INSERT INTO sd_ticket_events (school_id, ticket_id, kind, body, actor_user_id, dedupe_key, created_at)
		VALUES ($1,$2,'work_order',$3,$4,'work_order',$5) ON CONFLICT DO NOTHING`, schoolID, id, body, actor, now); err != nil {
		return err
	}
	return tx.Commit()
}

// Rate stores the reporter's satisfaction score (FR-137-005).
func (s *Store) Rate(ctx context.Context, schoolID, id, userID string, score int, comment *string, now time.Time) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var t Ticket
	err = tx.QueryRowContext(ctx, `SELECT reporter_user_id, status, rated_at FROM sd_tickets WHERE id=$1 AND school_id=$2 FOR UPDATE`,
		id, schoolID).Scan(&t.ReporterUserID, &t.Status, &t.RatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if err := CanRate(t, userID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE sd_tickets SET satisfaction_score=$1, satisfaction_comment=$2, rated_at=$3, updated_at=$3 WHERE id=$4`,
		score, comment, now, id); err != nil {
		return err
	}
	body := fmt.Sprintf("Penilaian %d/5", score)
	if _, err := tx.ExecContext(ctx, `INSERT INTO sd_ticket_events (school_id, ticket_id, kind, body, actor_user_id, created_at)
		VALUES ($1,$2,'satisfaction',$3,$4,$5)`, schoolID, id, body, userID, now); err != nil {
		return err
	}
	return tx.Commit()
}

// ---------------------------------------------------------------------------
// SLA job
// ---------------------------------------------------------------------------

// OpenTicketsWithSLA lists every open ticket that carries an SLA snapshot.
func (s *Store) OpenTicketsWithSLA(ctx context.Context) ([]Ticket, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT `+ticketCols+ticketFrom+`
		WHERE t.status IN ('new','assigned','in_progress') AND t.sla_snapshot IS NOT NULL ORDER BY t.school_id, t.created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Ticket{}
	for rows.Next() {
		t, err := scanTicket(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// MarkBreach stamps response/resolution breach once; true when this call did it.
func (s *Store) MarkBreach(ctx context.Context, t Ticket, which string, now time.Time) (bool, error) {
	col := "resolution_breached_at"
	body := "Target penyelesaian terlewati"
	if which == "response" {
		col = "response_breached_at"
		body = "Target respons terlewati"
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	res, err := tx.ExecContext(ctx, `UPDATE sd_tickets SET `+col+`=$1 WHERE id=$2 AND `+col+` IS NULL`, now, t.ID)
	if err != nil {
		return false, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return false, nil
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO sd_ticket_events (school_id, ticket_id, kind, body, internal, dedupe_key, created_at)
		VALUES ($1,$2,'sla_breach',$3,true,$4,$5) ON CONFLICT DO NOTHING`, t.SchoolID, t.ID, body, which+"_breach", now); err != nil {
		return false, err
	}
	return true, tx.Commit()
}

// RecordEscalation writes the escalation event (dedupe "<trigger>:<level>")
// and raises escalation_level; false when the rule had already fired.
func (s *Store) RecordEscalation(ctx context.Context, t Ticket, r EscalationRule, body string, now time.Time) (bool, error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	res, err := tx.ExecContext(ctx, `INSERT INTO sd_ticket_events (school_id, ticket_id, kind, body, internal, dedupe_key, created_at)
		VALUES ($1,$2,'escalation',$3,true,$4,$5) ON CONFLICT (ticket_id, dedupe_key) DO NOTHING`,
		t.SchoolID, t.ID, body, fmt.Sprintf("%s:%d", r.Trigger, r.Level), now)
	if err != nil {
		return false, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return false, nil
	}
	if _, err := tx.ExecContext(ctx, `UPDATE sd_tickets SET escalation_level=GREATEST(escalation_level,$1) WHERE id=$2`, r.Level, t.ID); err != nil {
		return false, err
	}
	return true, tx.Commit()
}

// ---------------------------------------------------------------------------
// Report (FR-138-003)
// ---------------------------------------------------------------------------

type ReportRow struct {
	GroupID              string `json:"group_id"`
	GroupName            string `json:"group_name"`
	Total                int    `json:"total"`
	Open                 int    `json:"open"`
	Met                  int    `json:"met"`
	Breached             int    `json:"breached"`
	ComplianceBp         *int   `json:"compliance_bp"`
	AvgSatisfactionX100  *int   `json:"avg_satisfaction_x100"`
	Rated                int    `json:"rated"`
	AvgResolutionMinutes *int   `json:"avg_resolution_minutes"`
}

// Report aggregates tickets created in [from, to) per category or team.
// met = resolved on/before the resolution due time; breached = resolution
// breach stamped or resolved late. Tickets without SLA count only in total.
func (s *Store) Report(ctx context.Context, schoolID string, from, to time.Time, groupBy string) ([]ReportRow, error) {
	key, name, join := "t.category_id", "c.name", "JOIN sd_categories c ON c.id = t.category_id"
	if groupBy == "team" {
		key, name, join = "t.team_id", "tm.name", "JOIN sd_teams tm ON tm.id = t.team_id"
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT `+key+`::text, `+name+`,
		count(*),
		count(*) FILTER (WHERE t.status IN ('new','assigned','in_progress')),
		count(*) FILTER (WHERE t.resolution_due_at IS NOT NULL AND t.resolved_at IS NOT NULL AND t.resolved_at <= t.resolution_due_at),
		count(*) FILTER (WHERE t.resolution_breached_at IS NOT NULL OR (t.resolved_at IS NOT NULL AND t.resolved_at > t.resolution_due_at)),
		(avg(t.satisfaction_score) * 100)::int,
		count(t.satisfaction_score),
		(avg(EXTRACT(EPOCH FROM (t.resolved_at - t.created_at)) / 60) FILTER (WHERE t.resolved_at IS NOT NULL))::int
		FROM sd_tickets t `+join+`
		WHERE t.school_id=$1 AND t.created_at >= $2 AND t.created_at < $3
		GROUP BY 1, 2 ORDER BY 2`, schoolID, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ReportRow{}
	for rows.Next() {
		var r ReportRow
		if err := rows.Scan(&r.GroupID, &r.GroupName, &r.Total, &r.Open, &r.Met, &r.Breached,
			&r.AvgSatisfactionX100, &r.Rated, &r.AvgResolutionMinutes); err != nil {
			return nil, err
		}
		r.ComplianceBp = ComplianceBp(r.Met, r.Met+r.Breached)
		out = append(out, r)
	}
	return out, rows.Err()
}

// jsonUnmarshal decodes optional JSON (empty input is not an error).
func jsonUnmarshal(raw []byte, dst any) error {
	if len(raw) == 0 {
		return nil
	}
	return json.Unmarshal(raw, dst)
}
