// Package campaign implements PRD menu 147 (segmented WhatsApp / email /
// in-app campaigns): saved segments, provider-approved WhatsApp templates,
// scheduling, marketing consent (BR-COM-01, OQ-43) and delivery statistics.
// The rules in this file are pure (no I/O) and table-tested.
package campaign

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// Channels.
const (
	ChannelWhatsApp = "whatsapp"
	ChannelEmail    = "email"
	ChannelInApp    = "in_app"
)

// ValidChannels is the closed channel set (matches the DB CHECK).
var ValidChannels = map[string]bool{ChannelWhatsApp: true, ChannelEmail: true, ChannelInApp: true}

// Consent statuses and sources.
const (
	ConsentOptedIn  = "opted_in"
	ConsentOptedOut = "opted_out"

	SourcePreferences     = "preferences"
	SourcePPDBForm        = "ppdb_form"
	SourceUnsubscribeLink = "unsubscribe_link"
	SourceWAKeyword       = "wa_keyword"
	SourceAdminImport     = "admin_import"
)

// ValidConsentSources is the closed source set.
var ValidConsentSources = map[string]bool{
	SourcePreferences: true, SourcePPDBForm: true, SourceUnsubscribeLink: true, SourceWAKeyword: true, SourceAdminImport: true,
}

// Recipient statuses and exclusion reasons.
const (
	RecipientQueued    = "queued"
	RecipientExcluded  = "excluded"
	RecipientSent      = "sent"
	RecipientDelivered = "delivered"
	RecipientRead      = "read"
	RecipientReplied   = "replied"
	RecipientFailed    = "failed"

	ExcludeNoConsent = "no_consent"
	ExcludeOptedOut  = "opted_out"
	ExcludeNoContact = "no_contact"
	ExcludeQuota     = "quota"
)

// Segment kinds.
const (
	KindParents        = "parents"
	KindPPDBApplicants = "ppdb_applicants"
	KindAlumniParents  = "alumni_parents"
)

// ValidSegmentKinds is the closed kind set.
var ValidSegmentKinds = map[string]bool{KindParents: true, KindPPDBApplicants: true, KindAlumniParents: true}

// Campaign statuses.
const (
	StatusDraft     = "draft"
	StatusScheduled = "scheduled"
	StatusSending   = "sending"
	StatusSent      = "sent"
	StatusCancelled = "cancelled"
	StatusFailed    = "failed"
)

// Template approval statuses.
const (
	TemplatePending  = "pending"
	TemplateApproved = "approved"
	TemplateRejected = "rejected"
)

// ValidTemplateStatuses is the closed approval set.
var ValidTemplateStatuses = map[string]bool{TemplatePending: true, TemplateApproved: true, TemplateRejected: true}

// Candidate is one resolved segment member before consent filtering. Contact
// is the raw address for the campaign channel (empty for in_app, which uses
// UserID); it is never persisted raw.
type Candidate struct {
	SubjectKey string
	UserID     string
	Name       string
	Contact    string
}

// Exclusion is a candidate that will not be sent, with the reason.
type Exclusion struct {
	Candidate
	Reason string
}

// HasContact reports whether the candidate is reachable on channel.
func (c Candidate) HasContact(channel string) bool {
	if channel == ChannelInApp {
		return c.UserID != ""
	}
	return strings.TrimSpace(c.Contact) != ""
}

// Eligible applies BR-COM-01 / OQ-43: only an explicit opted_in consent row
// for the channel is sent. A missing row is no_consent, an opted_out row is
// opted_out, and an unreachable candidate is no_contact (FR-147-003).
func Eligible(candidates []Candidate, consents map[string]string, channel string) (send []Candidate, excluded []Exclusion) {
	for _, c := range candidates {
		switch {
		case !c.HasContact(channel):
			excluded = append(excluded, Exclusion{c, ExcludeNoContact})
		case consents[c.SubjectKey] == ConsentOptedOut:
			excluded = append(excluded, Exclusion{c, ExcludeOptedOut})
		case consents[c.SubjectKey] != ConsentOptedIn:
			excluded = append(excluded, Exclusion{c, ExcludeNoConsent})
		default:
			send = append(send, c)
		}
	}
	return send, excluded
}

// ApplyQuota keeps at most the remaining monthly quota. quota nil = no quota.
func ApplyQuota(send []Candidate, usedThisMonth int, quota *int) (keep []Candidate, over []Exclusion) {
	if quota == nil {
		return send, nil
	}
	remaining := *quota - usedThisMonth
	if remaining < 0 {
		remaining = 0
	}
	if remaining >= len(send) {
		return send, nil
	}
	for _, c := range send[remaining:] {
		over = append(over, Exclusion{c, ExcludeQuota})
	}
	return send[:remaining], over
}

// Stats is the per-campaign report (FR-147-004).
type Stats struct {
	Total     int            `json:"total"`
	Queued    int            `json:"queued"`
	Sent      int            `json:"sent"`
	Delivered int            `json:"delivered"`
	Read      int            `json:"read"`
	Replied   int            `json:"replied"`
	Failed    int            `json:"failed"`
	Excluded  int            `json:"excluded"`
	ByReason  map[string]int `json:"excluded_by_reason"`
}

// RecipientState is the minimal input to Stats.
type RecipientState struct {
	Status        string
	ExcludeReason string
}

// ComputeStats aggregates recipient states. A later state implies the earlier
// ones: replied counts as read, delivered and sent; read as delivered and sent.
func ComputeStats(rows []RecipientState) Stats {
	s := Stats{ByReason: map[string]int{}}
	for _, r := range rows {
		s.Total++
		switch r.Status {
		case RecipientQueued:
			s.Queued++
		case RecipientExcluded:
			s.Excluded++
			s.ByReason[r.ExcludeReason]++
		case RecipientFailed:
			s.Failed++
		case RecipientSent:
			s.Sent++
		case RecipientDelivered:
			s.Sent++
			s.Delivered++
		case RecipientRead:
			s.Sent++
			s.Delivered++
			s.Read++
		case RecipientReplied:
			s.Sent++
			s.Delivered++
			s.Read++
			s.Replied++
		}
	}
	return s
}

// Template is the subset of wa_templates RenderTemplate needs.
type Template struct {
	ApprovalStatus string
	Variables      []string
	Body           string
}

// ErrTemplateNotApproved is returned for a template the provider has not approved.
var ErrTemplateNotApproved = errors.New("template WhatsApp belum disetujui penyedia")

var placeholder = regexp.MustCompile(`\{\{\s*([a-zA-Z0-9_]+)\s*\}\}`)

// RenderTemplate validates an approved template has every declared variable
// and returns the preview body with placeholders substituted (FR-147-002).
func RenderTemplate(tpl Template, vars map[string]string) (string, error) {
	if tpl.ApprovalStatus != TemplateApproved {
		return "", ErrTemplateNotApproved
	}
	var missing []string
	for _, v := range tpl.Variables {
		if strings.TrimSpace(vars[v]) == "" {
			missing = append(missing, v)
		}
	}
	if len(missing) > 0 {
		return "", fmt.Errorf("variabel template belum diisi: %s", strings.Join(missing, ", "))
	}
	return placeholder.ReplaceAllStringFunc(tpl.Body, func(m string) string {
		name := placeholder.FindStringSubmatch(m)[1]
		if v, ok := vars[name]; ok {
			return v
		}
		return m
	}), nil
}

// ---- subjects & contacts ----------------------------------------------------

// NormalizeContact canonicalizes an email (lower-case) or phone (digits
// only), matching admission's normalizeContact so hashes agree across services.
func NormalizeContact(kind, v string) string {
	v = strings.TrimSpace(strings.ToLower(v))
	if kind == "phone" {
		var b strings.Builder
		for _, r := range v {
			if r >= '0' && r <= '9' {
				b.WriteRune(r)
			}
		}
		return b.String()
	}
	return v
}

// HashContact is the sha256 hex of a normalized contact.
func HashContact(kind, v string) string {
	sum := sha256.Sum256([]byte(NormalizeContact(kind, v)))
	return hex.EncodeToString(sum[:])
}

// UserSubject is the consent subject for an identity user.
func UserSubject(userID string) string { return "user:" + userID }

// ContactSubject is the consent subject for a raw contact (PPDB applicants
// without an identity account).
func ContactSubject(kind, v string) string { return kind + ":" + HashContact(kind, v) }

// ContactKind maps a channel to the contact kind it needs.
func ContactKind(channel string) string {
	if channel == ChannelWhatsApp {
		return "phone"
	}
	return "email"
}

// MaskContact hides all but the edges of a contact for display.
func MaskContact(kind, v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return ""
	}
	if kind == "email" {
		at := strings.LastIndex(v, "@")
		if at <= 0 {
			return "***"
		}
		return v[:1] + "***" + v[at:]
	}
	if len(v) <= 4 {
		return "***"
	}
	return v[:2] + strings.Repeat("*", len(v)-4) + v[len(v)-2:]
}

// ---- unsubscribe token --------------------------------------------------------

// SignUnsubscribe builds the HMAC token for an unsubscribe link.
func SignUnsubscribe(secret, schoolID, subjectKey, channel string) string {
	payload := schoolID + "|" + subjectKey + "|" + channel
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(payload))
	return base64.RawURLEncoding.EncodeToString([]byte(payload)) + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

// VerifyUnsubscribe checks a token and returns its school, subject and channel.
func VerifyUnsubscribe(secret, token string) (schoolID, subjectKey, channel string, err error) {
	parts := strings.Split(token, ".")
	if len(parts) != 2 {
		return "", "", "", errors.New("invalid token")
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return "", "", "", errors.New("invalid token")
	}
	sig, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return "", "", "", errors.New("invalid token")
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	if !hmac.Equal(sig, mac.Sum(nil)) {
		return "", "", "", errors.New("invalid token")
	}
	f := strings.Split(string(payload), "|")
	if len(f) != 3 || !ValidChannels[f[2]] {
		return "", "", "", errors.New("invalid token")
	}
	return f[0], f[1], f[2], nil
}
