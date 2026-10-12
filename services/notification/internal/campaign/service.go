package campaign

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/url"
	"strings"
	"time"

	"skci/notification/internal/client"
	"skci/notification/internal/model"
	"skci/notification/internal/provider"
	"skci/notification/internal/store"
)

// SegmentSource resolves a saved segment into reachable candidates for one
// channel. Production reads the owning services (ClientSource); tests use a
// fake so AC-147-01 runs without siblings.
type SegmentSource interface {
	Resolve(ctx context.Context, schoolID string, seg Segment, channel string) ([]Candidate, error)
}

// InAppCreator creates an in-app notification (notification store).
type InAppCreator interface {
	CreateNotification(ctx context.Context, in store.CreateNotificationInput) (*model.Notification, bool, error)
}

// Service holds campaign dispatch logic shared by the HTTP handler
// (send-now) and the periodic job.
type Service struct {
	Store             *Store
	InApp             InAppCreator
	Source            SegmentSource
	Email             provider.Provider
	WhatsApp          provider.TemplateSender // nil = WhatsApp disabled
	Audit             func(ctx context.Context, e client.AuditEvent)
	UnsubscribeSecret string
	PublicURL         string
	Loc               *time.Location
	Now               func() time.Time
}

func (s *Service) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

func (s *Service) audit(ctx context.Context, e client.AuditEvent) {
	if s.Audit != nil {
		s.Audit(ctx, e)
	}
}

// Preview is the segment preview / dispatch summary.
type Preview struct {
	Total    int            `json:"total"`
	Eligible int            `json:"eligible"`
	Excluded int            `json:"excluded"`
	ByReason map[string]int `json:"excluded_by_reason"`
}

// plan resolves the segment and splits it into send / excluded.
func (s *Service) plan(ctx context.Context, schoolID string, seg Segment, channel string) ([]Candidate, []Exclusion, error) {
	cands, err := s.Source.Resolve(ctx, schoolID, seg, channel)
	if err != nil {
		return nil, nil, err
	}
	cands = dedupe(cands)
	keys := make([]string, 0, len(cands))
	for _, c := range cands {
		keys = append(keys, c.SubjectKey)
	}
	consents, err := s.Store.ConsentsFor(ctx, schoolID, channel, keys)
	if err != nil {
		return nil, nil, err
	}
	send, excluded := Eligible(cands, consents, channel)
	if channel == ChannelWhatsApp {
		quota, err := s.Store.MonthlyWAQuota(ctx, schoolID)
		if err != nil {
			return nil, nil, err
		}
		used, err := s.Store.WAUsedThisMonth(ctx, schoolID, s.now(), s.Loc)
		if err != nil {
			return nil, nil, err
		}
		var over []Exclusion
		send, over = ApplyQuota(send, used, quota)
		excluded = append(excluded, over...)
	}
	return send, excluded, nil
}

// PreviewSegment counts total / eligible / excluded-by-reason for a channel.
func (s *Service) PreviewSegment(ctx context.Context, schoolID string, seg Segment, channel string) (Preview, error) {
	send, excluded, err := s.plan(ctx, schoolID, seg, channel)
	if err != nil {
		return Preview{}, err
	}
	p := Preview{Total: len(send) + len(excluded), Eligible: len(send), Excluded: len(excluded), ByReason: map[string]int{}}
	for _, e := range excluded {
		p.ByReason[e.Reason]++
	}
	return p, nil
}

// ErrNotDispatchable is returned when a campaign cannot be claimed (not
// scheduled, or already taken by another worker).
var ErrNotDispatchable = errors.New("campaign is not scheduled for sending")

// Dispatch claims a scheduled campaign and sends it. It is idempotent: the
// recipient ledger's UNIQUE(campaign_id, subject_key) and the queued→sent
// transition make a re-run after a crash resume without re-sending.
func (s *Service) Dispatch(ctx context.Context, campaignID string) error {
	c, err := s.Store.ClaimCampaign(ctx, campaignID)
	if errors.Is(err, ErrNotFound) {
		return ErrNotDispatchable
	}
	if err != nil {
		return err
	}
	if err := s.send(ctx, c); err != nil {
		_ = s.Store.FinishCampaign(ctx, c.ID, StatusFailed, err.Error())
		return err
	}
	return s.Store.FinishCampaign(ctx, c.ID, StatusSent, "")
}

func (s *Service) send(ctx context.Context, c *Campaign) error {
	seg, err := s.Store.GetSegment(ctx, c.SchoolID, c.SegmentID)
	if err != nil {
		return fmt.Errorf("segment: %w", err)
	}
	var tpl *WATemplate
	if c.Channel == ChannelWhatsApp {
		if s.WhatsApp == nil {
			return provider.ErrWhatsAppDisabled
		}
		if c.TemplateID == nil {
			return errors.New("template is required for whatsapp")
		}
		if tpl, err = s.Store.GetTemplate(ctx, c.SchoolID, *c.TemplateID); err != nil {
			return fmt.Errorf("template: %w", err)
		}
		if _, err := RenderTemplate(Template{ApprovalStatus: tpl.ApprovalStatus, Variables: tpl.Variables, Body: tpl.BodyPreview}, c.TemplateVars); err != nil {
			return err
		}
	}

	send, excluded, err := s.plan(ctx, c.SchoolID, *seg, c.Channel)
	if err != nil {
		return err
	}
	kind := ContactKind(c.Channel)
	rows := make([]RecipientInput, 0, len(send)+len(excluded))
	bySubject := make(map[string]Candidate, len(send))
	for _, cand := range send {
		bySubject[cand.SubjectKey] = cand
		rows = append(rows, recipientRow(cand, kind, RecipientQueued, ""))
	}
	for _, e := range excluded {
		rows = append(rows, recipientRow(e.Candidate, kind, RecipientExcluded, e.Reason))
	}
	if err := s.Store.InsertRecipients(ctx, c.SchoolID, c.ID, rows); err != nil {
		return err
	}

	queued, err := s.Store.QueuedRecipients(ctx, c.ID)
	if err != nil {
		return err
	}
	for _, r := range queued {
		cand, ok := bySubject[r.SubjectKey]
		if !ok {
			// Re-run after the segment changed: the member left or lost
			// consent since the row was queued. Never send without a fresh check.
			_ = s.Store.MarkRecipientFailed(ctx, r.ID, "penerima tidak lagi memenuhi syarat segmen/persetujuan")
			continue
		}
		msgID, notifID, err := s.deliver(ctx, c, tpl, cand)
		if err != nil {
			_ = s.Store.MarkRecipientFailed(ctx, r.ID, err.Error())
			continue
		}
		if err := s.Store.MarkRecipientSent(ctx, r.ID, msgID, notifID); err != nil {
			return err
		}
	}
	return nil
}

func recipientRow(c Candidate, kind, status, reason string) RecipientInput {
	r := RecipientInput{SubjectKey: c.SubjectKey, UserID: c.UserID, DisplayName: c.Name, Status: status, ExcludeReason: reason}
	if c.Contact != "" {
		r.ContactMasked = MaskContact(kind, c.Contact)
		r.ContactHash = HashContact(kind, c.Contact)
	}
	return r
}

func (s *Service) deliver(ctx context.Context, c *Campaign, tpl *WATemplate, cand Candidate) (msgID, notificationID string, err error) {
	switch c.Channel {
	case ChannelWhatsApp:
		msgID, err = s.WhatsApp.SendTemplate(ctx, cand.Contact, tpl.ProviderTemplateID, tpl.Language, c.TemplateVars)
		return msgID, "", err
	case ChannelEmail:
		if s.Email == nil {
			return "", "", errors.New("no email provider configured")
		}
		body := deref(c.Body) + "\n\n---\nBerhenti menerima info promosi: " + s.UnsubscribeURL(c.SchoolID, cand.SubjectKey, ChannelEmail)
		msgID, err = s.Email.Send(ctx, provider.Message{Channel: model.ChannelEmail, Recipient: cand.Contact, Subject: deref(c.Subject), Body: body})
		return msgID, "", err
	default:
		title := deref(c.Subject)
		if title == "" {
			title = c.Name
		}
		data, _ := json.Marshal(map[string]string{"campaign_id": c.ID})
		key := "campaign:" + c.ID + ":" + cand.UserID
		n, _, err := s.InApp.CreateNotification(ctx, store.CreateNotificationInput{
			SchoolID: c.SchoolID, UserID: cand.UserID, Type: "campaign", Title: title, Body: deref(c.Body), Data: data, IdempotencyKey: &key,
		})
		if err != nil {
			return "", "", err
		}
		return "in_app", n.ID, nil
	}
}

// UnsubscribeURL builds the signed public unsubscribe link.
func (s *Service) UnsubscribeURL(schoolID, subjectKey, channel string) string {
	return s.PublicURL + "/public/unsubscribe?token=" + url.QueryEscape(SignUnsubscribe(s.UnsubscribeSecret, schoolID, subjectKey, channel))
}

// RunDue is the campaign_dispatch job body: dispatch every due campaign.
func (s *Service) RunDue(ctx context.Context) error {
	ids, err := s.Store.DueCampaignIDs(ctx, s.now(), 15*time.Minute)
	if err != nil {
		return err
	}
	for _, id := range ids {
		if err := s.Dispatch(ctx, id); err != nil && !errors.Is(err, ErrNotDispatchable) {
			log.Printf("[campaign_dispatch] campaign %s: %v", id, err)
		}
	}
	return nil
}

func deref(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func dedupe(in []Candidate) []Candidate {
	seen := make(map[string]bool, len(in))
	out := make([]Candidate, 0, len(in))
	for _, c := range in {
		if c.SubjectKey == "" || seen[c.SubjectKey] {
			continue
		}
		seen[c.SubjectKey] = true
		out = append(out, c)
	}
	return out
}

// ---- production segment source -------------------------------------------------

// ClientSource resolves segments from the services that own the data
// (academic classrooms/enrollments, identity guardians/users, admission
// applicant guardians). Nothing is copied into notification.
type ClientSource struct {
	Identity  *client.Client
	Academic  *client.Client
	Admission *client.Client
}

// Resolve implements SegmentSource.
func (cs ClientSource) Resolve(ctx context.Context, schoolID string, seg Segment, channel string) ([]Candidate, error) {
	switch seg.Kind {
	case KindParents:
		students, err := cs.classroomStudents(ctx, schoolID, seg.Filter)
		if err != nil {
			return nil, err
		}
		return cs.guardiansOf(ctx, schoolID, students, channel)
	case KindAlumniParents:
		students, err := cs.Academic.EnrollmentsByStatus(ctx, schoolID, "graduated")
		if err != nil {
			return nil, err
		}
		return cs.guardiansOf(ctx, schoolID, students, channel)
	case KindPPDBApplicants:
		rows, err := cs.Admission.SegmentGuardians(ctx, schoolID, seg.Filter.PeriodID, seg.Filter.Statuses)
		if err != nil {
			return nil, err
		}
		out := make([]Candidate, 0, len(rows))
		for _, g := range rows {
			out = append(out, PPDBCandidate(g, channel))
		}
		return out, nil
	}
	return nil, fmt.Errorf("unknown segment kind %q", seg.Kind)
}

// PPDBCandidate maps an admission guardian to a candidate. WhatsApp/email use
// a contact subject (the PPDB form consent is keyed the same way); in-app needs
// an identity account.
func PPDBCandidate(g client.SegmentGuardian, channel string) Candidate {
	c := Candidate{Name: g.Name, UserID: g.IdentityUserID}
	kind := ContactKind(channel)
	contact := g.Email
	if kind == "phone" {
		contact = g.Phone
	}
	switch {
	case channel == ChannelInApp && g.IdentityUserID != "":
		c.SubjectKey = UserSubject(g.IdentityUserID)
	case channel != ChannelInApp && strings.TrimSpace(contact) != "":
		c.Contact = contact
		c.SubjectKey = ContactSubject(kind, contact)
	default:
		c.SubjectKey = "ppdb:" + g.GuardianID
		if channel == ChannelInApp {
			c.UserID = ""
		}
	}
	return c
}

func (cs ClientSource) classroomStudents(ctx context.Context, schoolID string, f SegmentFilter) ([]string, error) {
	classrooms, err := cs.Academic.Classrooms(ctx, schoolID)
	if err != nil {
		return nil, err
	}
	grades := toSet(f.GradeLevels)
	ids := toSet(f.ClassroomIDs)
	var out []string
	for _, cl := range classrooms {
		if len(grades) > 0 && !grades[cl.GradeLevel] {
			continue
		}
		if len(ids) > 0 && !ids[cl.ID] {
			continue
		}
		students, err := cs.Academic.ClassroomStudents(ctx, schoolID, cl.ID)
		if err != nil {
			return nil, err
		}
		out = append(out, students...)
	}
	return out, nil
}

func (cs ClientSource) guardiansOf(ctx context.Context, schoolID string, students []string, channel string) ([]Candidate, error) {
	seen := map[string]bool{}
	var out []Candidate
	for _, sid := range students {
		guardians, err := cs.Identity.StudentGuardians(ctx, schoolID, sid)
		if err != nil {
			return nil, err
		}
		for _, g := range guardians {
			if seen[g.GuardianUserID] {
				continue
			}
			seen[g.GuardianUserID] = true
			c := Candidate{SubjectKey: UserSubject(g.GuardianUserID), UserID: g.GuardianUserID}
			if u, err := cs.Identity.GetUser(ctx, g.GuardianUserID); err == nil && u != nil {
				c.Name = u.Name
				switch channel {
				case ChannelEmail:
					c.Contact = strings.TrimSpace(u.Email)
				case ChannelWhatsApp:
					c.Contact = strings.TrimSpace(u.Phone)
				}
			}
			out = append(out, c)
		}
	}
	return out, nil
}

func toSet(v []string) map[string]bool {
	m := make(map[string]bool, len(v))
	for _, s := range v {
		if s != "" {
			m[s] = true
		}
	}
	return m
}
