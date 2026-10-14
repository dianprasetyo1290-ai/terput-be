package model

import (
	"errors"
	"fmt"
	"sort"
	"time"
)

// ReminderKind classifies an automatic bill reminder (FR-21 pengingat).
type ReminderKind string

const (
	ReminderBeforeDue ReminderKind = "before_due" // H-x
	ReminderOnDue     ReminderKind = "on_due"     // H
	ReminderOverdue   ReminderKind = "overdue"    // H+x
	ReminderManual    ReminderKind = "manual"
)

// ReminderPolicy is one school's reminder schedule, stored in
// billing_reminder_settings. Day offsets are calendar days in the school's
// time zone.
type ReminderPolicy struct {
	Enabled    bool     `json:"enabled"`
	DaysBefore []int    `json:"days_before"`
	OnDue      bool     `json:"on_due"`
	DaysAfter  []int    `json:"days_after"`
	SendHour   int      `json:"send_hour"`
	Channels   []string `json:"channels"`
}

// SuggestedReminderPolicy is what the settings screen proposes before a
// school saves its own schedule; it is never applied implicitly.
func SuggestedReminderPolicy() ReminderPolicy {
	return ReminderPolicy{Enabled: true, DaysBefore: []int{3}, OnDue: true, DaysAfter: []int{3}, SendHour: 8, Channels: []string{"in_app", "email"}}
}

var reminderChannels = map[string]bool{"in_app": true, "email": true, "whatsapp": true, "push": true}

const maxReminderOffset = 90

// Normalize sorts and de-duplicates the offsets and validates the policy.
func (p *ReminderPolicy) Normalize() error {
	var err error
	if p.DaysBefore, err = normalizeOffsets(p.DaysBefore, "days_before"); err != nil {
		return err
	}
	if p.DaysAfter, err = normalizeOffsets(p.DaysAfter, "days_after"); err != nil {
		return err
	}
	if p.SendHour < 0 || p.SendHour > 23 {
		return errors.New("send_hour harus 0–23")
	}
	if len(p.Channels) == 0 {
		return errors.New("minimal satu kanal pengingat")
	}
	seen := map[string]bool{}
	var ch []string
	for _, c := range p.Channels {
		if !reminderChannels[c] {
			return fmt.Errorf("kanal %q tidak dikenal", c)
		}
		if !seen[c] {
			seen[c] = true
			ch = append(ch, c)
		}
	}
	p.Channels = ch
	if p.Enabled && len(p.DaysBefore) == 0 && !p.OnDue && len(p.DaysAfter) == 0 {
		return errors.New("pengingat aktif membutuhkan minimal satu jadwal (H-x, H, atau H+x)")
	}
	return nil
}

func normalizeOffsets(in []int, field string) ([]int, error) {
	seen := map[int]bool{}
	out := []int{}
	for _, d := range in {
		if d < 1 || d > maxReminderOffset {
			return nil, fmt.Errorf("%s harus 1–%d hari", field, maxReminderOffset)
		}
		if !seen[d] {
			seen[d] = true
			out = append(out, d)
		}
	}
	sort.Ints(out)
	return out, nil
}

// Window returns how many days before and after the due date the policy
// can fire, so the job only scans bills inside that range.
func (p ReminderPolicy) Window() (before, after int) {
	if n := len(p.DaysBefore); n > 0 {
		before = p.DaysBefore[n-1]
	}
	if n := len(p.DaysAfter); n > 0 {
		after = p.DaysAfter[n-1]
	}
	return
}

// CalendarDaysBetween counts whole calendar days from a to b in loc
// (negative when b is before a).
func CalendarDaysBetween(a, b time.Time, loc *time.Location) int {
	ay, am, ad := a.In(loc).Date()
	by, bm, bd := b.In(loc).Date()
	da := time.Date(ay, am, ad, 0, 0, 0, 0, time.UTC)
	db := time.Date(by, bm, bd, 0, 0, 0, 0, time.UTC)
	return int(db.Sub(da).Hours() / 24)
}

// ReminderFor reports which reminder, if any, is due today for a bill due
// on due. offset is the signed day distance (negative = before due).
func ReminderFor(p ReminderPolicy, today, due time.Time, loc *time.Location) (ReminderKind, int, bool) {
	if !p.Enabled {
		return "", 0, false
	}
	offset := CalendarDaysBetween(due, today, loc)
	switch {
	case offset < 0:
		for _, d := range p.DaysBefore {
			if d == -offset {
				return ReminderBeforeDue, offset, true
			}
		}
	case offset == 0:
		if p.OnDue {
			return ReminderOnDue, 0, true
		}
	default:
		for _, d := range p.DaysAfter {
			if d == offset {
				return ReminderOverdue, offset, true
			}
		}
	}
	return "", 0, false
}

// ReminderText builds the guardian-facing title and body.
func ReminderText(kind ReminderKind, offset int, label, amount, due string) (string, string) {
	switch kind {
	case ReminderBeforeDue:
		return "Pengingat tagihan", fmt.Sprintf("Tagihan %s sebesar %s jatuh tempo %d hari lagi (%s).", label, amount, -offset, due)
	case ReminderOnDue:
		return "Tagihan jatuh tempo hari ini", fmt.Sprintf("Tagihan %s sebesar %s jatuh tempo hari ini (%s).", label, amount, due)
	case ReminderOverdue:
		return "Tagihan lewat jatuh tempo", fmt.Sprintf("Tagihan %s sebesar %s telah lewat jatuh tempo %d hari (jatuh tempo %s).", label, amount, offset, due)
	}
	return "Pengingat tagihan", fmt.Sprintf("Sisa tagihan %s sebesar %s.", label, amount)
}
