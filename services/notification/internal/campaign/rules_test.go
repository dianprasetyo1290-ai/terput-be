package campaign

import (
	"fmt"
	"strings"
	"testing"
)

func parents(n int) []Candidate {
	out := make([]Candidate, n)
	for i := range out {
		id := fmt.Sprintf("u%03d", i)
		out[i] = Candidate{SubjectKey: UserSubject(id), UserID: id, Contact: id + "@mail.test"}
	}
	return out
}

// AC-147-01: 500 parents, 480 opted in, 20 opted out → 480 sent, 20 excluded.
func TestEligibleAC14701(t *testing.T) {
	cands := parents(500)
	consents := map[string]string{}
	for i, c := range cands {
		if i < 20 {
			consents[c.SubjectKey] = ConsentOptedOut
		} else {
			consents[c.SubjectKey] = ConsentOptedIn
		}
	}
	send, excluded := Eligible(cands, consents, ChannelEmail)
	if len(send) != 480 || len(excluded) != 20 {
		t.Fatalf("send=%d excluded=%d, want 480/20", len(send), len(excluded))
	}
	for _, e := range excluded {
		if e.Reason != ExcludeOptedOut {
			t.Fatalf("reason=%s want opted_out", e.Reason)
		}
	}
}

func TestEligibleReasons(t *testing.T) {
	cases := []struct {
		name    string
		cand    Candidate
		consent string
		channel string
		want    string // "" = sent
	}{
		{"no consent row is not sent (OQ-43)", Candidate{SubjectKey: "user:a", UserID: "a", Contact: "a@x"}, "", ChannelEmail, ExcludeNoConsent},
		{"opted out", Candidate{SubjectKey: "user:a", UserID: "a", Contact: "a@x"}, ConsentOptedOut, ChannelEmail, ExcludeOptedOut},
		{"empty contact", Candidate{SubjectKey: "user:a", UserID: "a"}, ConsentOptedIn, ChannelWhatsApp, ExcludeNoContact},
		{"in_app needs a user id", Candidate{SubjectKey: "phone:x", Contact: "0812"}, ConsentOptedIn, ChannelInApp, ExcludeNoContact},
		{"in_app with user id", Candidate{SubjectKey: "user:a", UserID: "a"}, ConsentOptedIn, ChannelInApp, ""},
		{"opted in with contact", Candidate{SubjectKey: "user:a", UserID: "a", Contact: "0812"}, ConsentOptedIn, ChannelWhatsApp, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			consents := map[string]string{}
			if c.consent != "" {
				consents[c.cand.SubjectKey] = c.consent
			}
			send, excluded := Eligible([]Candidate{c.cand}, consents, c.channel)
			if c.want == "" {
				if len(send) != 1 {
					t.Fatalf("want sent, got excluded %+v", excluded)
				}
				return
			}
			if len(excluded) != 1 || excluded[0].Reason != c.want {
				t.Fatalf("want %s, got send=%d excluded=%+v", c.want, len(send), excluded)
			}
		})
	}
}

func TestApplyQuota(t *testing.T) {
	ptr := func(v int) *int { return &v }
	cases := []struct {
		name               string
		n, used            int
		quota              *int
		wantKeep, wantOver int
	}{
		{"nil quota keeps all", 15, 999, nil, 15, 0},
		{"remaining 10 of 15", 15, 90, ptr(100), 10, 5},
		{"exhausted", 3, 100, ptr(100), 0, 3},
		{"over-used clamps to zero", 3, 120, ptr(100), 0, 3},
		{"exactly fits", 10, 90, ptr(100), 10, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			keep, over := ApplyQuota(parents(c.n), c.used, c.quota)
			if len(keep) != c.wantKeep || len(over) != c.wantOver {
				t.Fatalf("keep=%d over=%d want %d/%d", len(keep), len(over), c.wantKeep, c.wantOver)
			}
			for _, o := range over {
				if o.Reason != ExcludeQuota {
					t.Fatalf("reason=%s", o.Reason)
				}
			}
		})
	}
}

func TestComputeStats(t *testing.T) {
	s := ComputeStats([]RecipientState{
		{Status: RecipientSent}, {Status: RecipientDelivered}, {Status: RecipientRead}, {Status: RecipientReplied},
		{Status: RecipientFailed}, {Status: RecipientExcluded, ExcludeReason: ExcludeOptedOut},
		{Status: RecipientExcluded, ExcludeReason: ExcludeNoConsent}, {Status: RecipientQueued},
	})
	if s.Total != 8 || s.Sent != 4 || s.Delivered != 3 || s.Read != 2 || s.Replied != 1 || s.Failed != 1 || s.Excluded != 2 || s.Queued != 1 {
		t.Fatalf("stats = %+v", s)
	}
	if s.ByReason[ExcludeOptedOut] != 1 || s.ByReason[ExcludeNoConsent] != 1 {
		t.Fatalf("by reason = %+v", s.ByReason)
	}
}

func TestRenderTemplate(t *testing.T) {
	tpl := Template{ApprovalStatus: TemplateApproved, Variables: []string{"nama"}, Body: "Halo {{nama}}, PPDB dibuka {{ tanggal }}"}
	if _, err := RenderTemplate(tpl, map[string]string{}); err == nil || !strings.Contains(err.Error(), "nama") {
		t.Fatalf("missing variable must error, got %v", err)
	}
	got, err := RenderTemplate(tpl, map[string]string{"nama": "Budi", "tanggal": "1 Juli"})
	if err != nil || got != "Halo Budi, PPDB dibuka 1 Juli" {
		t.Fatalf("render = %q, %v", got, err)
	}
	for _, st := range []string{TemplatePending, TemplateRejected} {
		tpl.ApprovalStatus = st
		if _, err := RenderTemplate(tpl, map[string]string{"nama": "x"}); err != ErrTemplateNotApproved {
			t.Fatalf("status %s: want ErrTemplateNotApproved, got %v", st, err)
		}
	}
}

func TestContactHelpers(t *testing.T) {
	if HashContact("phone", "+62 812-3456") != HashContact("phone", "628123456") {
		t.Fatal("phone normalization must ignore punctuation")
	}
	if HashContact("email", " A@B.id ") != HashContact("email", "a@b.id") {
		t.Fatal("email normalization must be case-insensitive")
	}
	if got := MaskContact("email", "budi@mail.id"); got != "b***@mail.id" {
		t.Fatalf("mask email = %s", got)
	}
	if got := MaskContact("phone", "081234567"); got != "08*****67" {
		t.Fatalf("mask phone = %s", got)
	}
}

func TestUnsubscribeToken(t *testing.T) {
	tok := SignUnsubscribe("s3cret", "school", "user:abc", ChannelEmail)
	school, subject, channel, err := VerifyUnsubscribe("s3cret", tok)
	if err != nil || school != "school" || subject != "user:abc" || channel != ChannelEmail {
		t.Fatalf("verify = %s %s %s %v", school, subject, channel, err)
	}
	if _, _, _, err := VerifyUnsubscribe("other", tok); err == nil {
		t.Fatal("wrong secret must fail")
	}
	if _, _, _, err := VerifyUnsubscribe("s3cret", tok+"x"); err == nil {
		t.Fatal("tampered token must fail")
	}
}
