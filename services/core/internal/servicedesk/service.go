package servicedesk

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"sync"
	"time"

	"skci/core/internal/client"
	"skci/core/internal/config"
	"skci/core/internal/model"
	"skci/core/internal/storage"
	"skci/core/internal/store"
)

// featureTTL bounds how long a school's feature switch is cached.
const featureTTL = 60 * time.Second

type featureEntry struct {
	enabled   bool
	expiresAt time.Time
}

// Service wires the service desk store to core's audit/exception store,
// the downstream clients and private file storage.
type Service struct {
	Store   *Store
	Core    store.Store
	Client  *client.Client
	Files   storage.Storage
	Config  config.Config
	Loc     *time.Location
	Now     func() time.Time
	featMu  sync.Mutex
	featMap map[string]featureEntry
}

func New(db *sql.DB, core store.Store, cl *client.Client, files storage.Storage, cfg config.Config) (*Service, error) {
	loc, err := time.LoadLocation(cfg.SchoolTimezone)
	if err != nil {
		return nil, fmt.Errorf("SCHOOL_TIMEZONE: %w", err)
	}
	return &Service{Store: &Store{DB: db}, Core: core, Client: cl, Files: files, Config: cfg, Loc: loc,
		Now: time.Now, featMap: map[string]featureEntry{}}, nil
}

// FeatureEnabled reports the school's service_desk switch, cached 60s. It
// fails closed: any lookup error counts as "off".
func (s *Service) FeatureEnabled(ctx context.Context, schoolID string) bool {
	s.featMu.Lock()
	e, ok := s.featMap[schoolID]
	s.featMu.Unlock()
	if ok && s.Now().Before(e.expiresAt) {
		return e.enabled
	}
	feats, err := s.Client.SchoolFeatures(ctx, schoolID)
	if err != nil {
		log.Printf("service desk: feature lookup %s: %v", schoolID, err)
		return false
	}
	s.featMu.Lock()
	s.featMap[schoolID] = featureEntry{enabled: feats[FeatureKey], expiresAt: s.Now().Add(featureTTL)}
	s.featMu.Unlock()
	return feats[FeatureKey]
}

func (s *Service) audit(ctx context.Context, schoolID, entityID, action string, actor *string, before, after any, reason string) {
	ev := model.AuditEvent{SchoolID: &schoolID, Service: "core", Module: AuditModule, EntityType: "sd_ticket",
		EntityID: entityID, Action: action, ActorUserID: actor, BeforeData: before, AfterData: after}
	if reason != "" {
		ev.Reason = &reason
	}
	if err := s.Core.InsertAudit(ctx, ev); err != nil {
		log.Printf("service desk audit %s/%s: %v", action, entityID, err)
	}
}

func (s *Service) notify(ctx context.Context, schoolID, keyBase, title, body, ticketID string, users []string) {
	if len(users) == 0 {
		return
	}
	err := s.Client.NotifyUsers(ctx, client.Notice{SchoolID: schoolID, KeyBase: keyBase, Type: NoticeType,
		Title: title, Body: body, Data: map[string]any{"ticket_id": ticketID}}, users)
	if err != nil && !errors.Is(err, client.ErrNotConfigured) {
		log.Printf("service desk notify %s: %v", keyBase, err)
	}
}

// statusLabels are the Indonesian status names used in notifications.
var statusLabels = map[string]string{
	StatusNew: "Baru", StatusAssigned: "Ditugaskan", StatusInProgress: "Diproses",
	StatusResolved: "Selesai", StatusClosed: "Ditutup", StatusRejected: "Ditolak",
}

// NotifyReporter tells the reporter about one public event (FR-137-003).
func (s *Service) NotifyReporter(ctx context.Context, t *Ticket, eventID, what string) {
	s.notify(ctx, t.SchoolID, fmt.Sprintf("sd:%s:%s", t.ID, eventID), "Tiket "+t.Number+" diperbarui",
		fmt.Sprintf("%s (status: %s)", what, statusLabels[t.Status]), t.ID, []string{t.ReporterUserID})
}

// NotifyTeam tells the handling team (members + manager) about a ticket.
func (s *Service) NotifyTeam(ctx context.Context, t *Ticket, keySuffix, title, body string) {
	members, manager, err := s.Store.TeamRecipients(ctx, t.SchoolID, t.TeamID)
	if err != nil {
		log.Printf("service desk team recipients %s: %v", t.TeamID, err)
		return
	}
	if manager != nil {
		members = append(members, *manager)
	}
	s.notify(ctx, t.SchoolID, fmt.Sprintf("sd:%s:%s", t.ID, keySuffix), title, body, t.ID, members)
}

// computeSLA resolves the policy and due times for a new ticket.
func (s *Service) computeSLA(ctx context.Context, schoolID, categoryID, priority string, at time.Time) (*Policy, *time.Time, *time.Time) {
	policies, err := s.Store.ListPolicies(ctx, schoolID)
	if err != nil {
		log.Printf("service desk policies %s: %v", schoolID, err)
		return nil, nil, nil
	}
	p := ResolvePolicy(policies, categoryID, priority, at.In(s.Loc).Format("2006-01-02"))
	if p == nil {
		return nil, nil, nil
	}
	cal, err := s.Store.LoadCalendar(ctx, schoolID, s.Loc)
	if err != nil {
		log.Printf("service desk calendar %s: %v", schoolID, err)
		return nil, nil, nil
	}
	resp, err1 := DueAt(at, p.ResponseValue, p.ResponseUnit, cal)
	res, err2 := DueAt(at, p.ResolutionValue, p.ResolutionUnit, cal)
	if err1 != nil || err2 != nil {
		log.Printf("service desk SLA %s: jam layanan belum diatur", schoolID)
		return nil, nil, nil
	}
	return p, &resp, &res
}

// CreateTicket files a ticket (SLA + snapshot), notifies the team and audits.
func (s *Service) CreateTicket(ctx context.Context, cat *Category, n NewTicket) (*Ticket, error) {
	n.CreatedAt = s.Now()
	n.CategoryID, n.TeamID = cat.ID, cat.TeamID
	n.SLA, n.ResponseDueAt, n.ResolutionDueAt = s.computeSLA(ctx, n.SchoolID, cat.ID, n.Priority, n.CreatedAt)
	t, created, err := s.Store.CreateTicket(ctx, n)
	if err != nil {
		return nil, err
	}
	if created {
		actor := n.ActorUserID
		s.audit(ctx, t.SchoolID, t.ID, "ticket_created", &actor, nil,
			map[string]any{"number": t.Number, "category_id": t.CategoryID, "priority": t.Priority, "channel": t.Channel}, "")
		s.NotifyTeam(ctx, t, "new", "Tiket baru "+t.Number, t.Subject)
	}
	return t, nil
}

// FromServiceRequest turns a menu 81 request into a ticket when the school
// runs the service desk (BR-SD-01). It never fails the menu 81 flow: no
// feature / no category → nothing happens; an error lands in the exception queue.
// Returns true when the request is now handled by a ticket.
func (s *Service) FromServiceRequest(ctx context.Context, sr model.ServiceRequest) bool {
	if !s.FeatureEnabled(ctx, sr.SchoolID) {
		return false
	}
	_, err := s.TicketFromServiceRequest(ctx, sr)
	if err != nil && !errors.Is(err, ErrNotFound) {
		log.Printf("service desk: ticket from service request %s: %v", sr.ID, err)
		retry := s.Config.SelfInternalURL + "/internal/service-desk/from-service-request/" + sr.ID + "?school_id=" + sr.SchoolID
		_, _, xerr := s.Core.UpsertException(ctx, model.IntegrationException{
			SchoolID: sr.SchoolID, SourceService: "core", Kind: model.ExceptionKindDeadLetter, Severity: model.SeverityNormal,
			CorrelationID: "sr:" + sr.ID, ErrorMessage: "Gagal membuat tiket Service Desk: " + err.Error(), MaxAttempts: 3,
			PayloadSummary: map[string]any{"service_request_id": sr.ID}, RetryURL: &retry,
			IdempotencyKey: "service_desk:sr:" + sr.ID,
		})
		if xerr != nil {
			log.Printf("service desk: exception for %s: %v", sr.ID, xerr)
		}
	}
	return err == nil
}

// TicketFromServiceRequest returns ErrNotFound when no category maps the request.
func (s *Service) TicketFromServiceRequest(ctx context.Context, sr model.ServiceRequest) (*Ticket, error) {
	cat, err := s.Store.CategoryForServiceRequest(ctx, sr.SchoolID, sr.CategoryID)
	if errors.Is(err, ErrNotFound) {
		log.Printf("service desk: no category for service request category %s (school %s); menu 81 flow kept", sr.CategoryID, sr.SchoolID)
		return nil, err
	}
	if err != nil {
		return nil, err
	}
	var atts []Attachment
	_ = jsonUnmarshal(sr.Attachments, &atts)
	srID := sr.ID
	return s.CreateTicket(ctx, cat, NewTicket{
		SchoolID: sr.SchoolID, Priority: "normal", Subject: sr.Subject, Description: sr.Description,
		Channel: ChannelParent, ReporterUserID: sr.RequesterUserID, StudentUserID: sr.StudentUserID,
		ServiceRequestID: &srID, Attachments: atts, ActorUserID: sr.RequesterUserID, IdempotencyKey: "sr:" + sr.ID,
	})
}

// HasTicketForServiceRequest is the menu 81 guard: a request with a ticket
// is handled in the service desk (independent of the feature switch).
func (s *Service) HasTicketForServiceRequest(ctx context.Context, schoolID, srID string) (string, bool) {
	t, err := s.Store.TicketByServiceRequest(ctx, schoolID, srID)
	if err != nil {
		if !errors.Is(err, ErrNotFound) {
			log.Printf("service desk guard %s: %v", srID, err)
		}
		return "", false
	}
	return t.Number, true
}

// ---------------------------------------------------------------------------
// SLA & escalation job (FR-138-002, AC-138-01)
// ---------------------------------------------------------------------------

// RunSLAJob stamps breaches and fires escalation rules for open tickets.
// Safe to run repeatedly: breach stamps are set once and escalation events
// are deduplicated per "<trigger>:<level>".
func (s *Service) RunSLAJob(ctx context.Context) error {
	now := s.Now()
	tickets, err := s.Store.OpenTicketsWithSLA(ctx)
	if err != nil {
		return err
	}
	cals := map[string]*Calendar{}
	for i := range tickets {
		t := &tickets[i]
		if !s.FeatureEnabled(ctx, t.SchoolID) {
			continue
		}
		cal, ok := cals[t.SchoolID]
		if !ok {
			c, err := s.Store.LoadCalendar(ctx, t.SchoolID, s.Loc)
			if err != nil {
				log.Printf("sla job calendar %s: %v", t.SchoolID, err)
				continue
			}
			cal = &c
			cals[t.SchoolID] = cal
		}
		s.evaluateTicket(ctx, t, *cal, now)
	}
	return nil
}

func (s *Service) evaluateTicket(ctx context.Context, t *Ticket, cal Calendar, now time.Time) {
	state := SLAOk
	if t.FirstRespondedAt == nil && t.ResponseDueAt != nil {
		rs := SLAState(*t.ResponseDueAt, t.CreatedAt, now, t.SLA.WarnBp, cal)
		if rs == SLABreached {
			if _, err := s.Store.MarkBreach(ctx, *t, "response", now); err != nil {
				log.Printf("sla job breach %s: %v", t.ID, err)
			}
		}
		state = WorstState(state, rs)
	}
	if t.ResolutionDueAt != nil {
		rs := SLAState(*t.ResolutionDueAt, t.CreatedAt, now, t.SLA.WarnBp, cal)
		if rs == SLABreached {
			if marked, err := s.Store.MarkBreach(ctx, *t, "resolution", now); err != nil {
				log.Printf("sla job breach %s: %v", t.ID, err)
			} else if marked {
				s.audit(ctx, t.SchoolID, t.ID, "sla_breached", nil, nil, map[string]any{"resolution_due_at": t.ResolutionDueAt}, "")
			}
		}
		state = WorstState(state, rs)
	}
	for _, rule := range NextEscalations(t.EscalationLevel, state, t.SLA.Rules) {
		body := fmt.Sprintf("Eskalasi level %d (%s)", rule.Level, map[string]string{TriggerWarning: "mendekati SLA", TriggerBreach: "melanggar SLA"}[rule.Trigger])
		fired, err := s.Store.RecordEscalation(ctx, *t, rule, body, now)
		if err != nil {
			log.Printf("sla job escalation %s: %v", t.ID, err)
			continue
		}
		if !fired {
			continue
		}
		t.EscalationLevel = max(t.EscalationLevel, rule.Level)
		s.audit(ctx, t.SchoolID, t.ID, "escalated", nil, nil, map[string]any{"trigger": rule.Trigger, "level": rule.Level}, "")
		targets := s.escalationTargets(ctx, t, rule)
		title := "Eskalasi tiket " + t.Number
		if rule.Trigger == TriggerBreach {
			title = "Tiket " + t.Number + " melanggar SLA"
		}
		s.notify(ctx, t.SchoolID, fmt.Sprintf("sd:%s:esc:%s:%d", t.ID, rule.Trigger, rule.Level), title, t.Subject, t.ID, targets)
	}
}

func (s *Service) escalationTargets(ctx context.Context, t *Ticket, r EscalationRule) []string {
	switch r.TargetKind {
	case TargetTeamManager:
		_, manager, err := s.Store.TeamRecipients(ctx, t.SchoolID, t.TeamID)
		if err == nil && manager != nil {
			return []string{*manager}
		}
	case TargetUser:
		if r.TargetUserID != nil {
			return []string{*r.TargetUserID}
		}
	case TargetRole:
		if r.TargetRole != nil {
			ids, err := s.Client.UsersByRole(ctx, t.SchoolID, *r.TargetRole)
			if err != nil {
				log.Printf("sla job role %s: %v", *r.TargetRole, err)
			}
			return ids
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// Work order conversion (FR-137-004, AC-137-01; contract T9 §4.2)
// ---------------------------------------------------------------------------

var (
	ErrNotFacility   = errors.New("hanya tiket kategori fasilitas yang dapat dijadikan perintah kerja")
	ErrTicketClosed  = errors.New("tiket yang sudah ditutup/ditolak tidak dapat dijadikan perintah kerja")
	ErrOpsCallFailed = errors.New("gagal membuat perintah kerja di modul fasilitas")
)

// ConvertToWorkOrder creates (or replays) the ops work order for a ticket.
func (s *Service) ConvertToWorkOrder(ctx context.Context, t *Ticket, roomID *string, actor string) (*Ticket, error) {
	if !t.IsFacility {
		return nil, ErrNotFacility
	}
	if t.Status == StatusClosed || t.Status == StatusRejected {
		return nil, ErrTicketClosed
	}
	if t.WorkOrderID != nil {
		return t, nil
	}
	key := "sd_ticket:" + t.ID + ":work_order"
	wo, err := s.Client.CreateWorkOrder(ctx, client.WorkOrderInput{
		SchoolID: t.SchoolID, RoomID: roomID, Title: t.Subject, Description: t.Description + "\n\nTiket: " + t.Number,
		Priority: WorkOrderPriority(t.Priority), ReporterUserID: t.ReporterUserID, TicketRef: t.Number, IdempotencyKey: key,
	})
	if err != nil {
		_, _, xerr := s.Core.UpsertException(ctx, model.IntegrationException{
			SchoolID: t.SchoolID, SourceService: "core", Kind: model.ExceptionKindCallback, Severity: model.SeverityNormal,
			CorrelationID: "sd_ticket:" + t.ID, ErrorMessage: "Gagal membuat perintah kerja: " + err.Error(), MaxAttempts: 3,
			PayloadSummary: map[string]any{"ticket_id": t.ID, "ticket_number": t.Number}, IdempotencyKey: key,
		})
		if xerr != nil {
			log.Printf("work order exception %s: %v", t.ID, xerr)
		}
		return nil, fmt.Errorf("%w: %v", ErrOpsCallFailed, err)
	}
	if err := s.Store.SetWorkOrder(ctx, t.SchoolID, t.ID, wo.ID, wo.Code, actor, s.Now()); err != nil {
		return nil, err
	}
	s.audit(ctx, t.SchoolID, t.ID, "work_order_created", &actor, nil, map[string]any{"work_order_id": wo.ID, "work_order_number": wo.Code}, "")
	return s.Store.GetTicket(ctx, t.SchoolID, t.ID)
}
