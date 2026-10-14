package campaign

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"skci/notification/internal/common"
	"skci/notification/internal/provider"
)

type failingEmail struct{}

func (failingEmail) Name() string { return "failing" }
func (failingEmail) Send(context.Context, provider.Message) (string, error) {
	return "", errors.New("smtp ditolak")
}

// BUG-113: recipients that fail delivery end up as one dead_letter item per
// campaign in core's exception queue.
func TestCampaignFailuresReportDeadLetter(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	school, humas := uuid(t, db), uuid(t, db)
	st := NewStore(db)
	cands := make([]Candidate, 3)
	for i := range cands {
		uid := uuid(t, db)
		cands[i] = Candidate{SubjectKey: UserSubject(uid), UserID: uid, Name: fmt.Sprintf("Wali %d", i), Contact: fmt.Sprintf("dl%d@mail.test", i)}
		if err := st.SetConsent(ctx, school, cands[i].SubjectKey, uid, ChannelEmail, ConsentOptedIn, SourcePreferences, uid); err != nil {
			t.Fatal(err)
		}
	}
	var reps []common.ExceptionReport
	svc := baseService(db, fakeSource{cands})
	svc.Email = failingEmail{}
	svc.Exceptions = func(_ context.Context, e common.ExceptionReport) error {
		reps = append(reps, e)
		return nil
	}
	router := newServer(svc)
	bearer := tok(t, humas, school, "humas")

	rec := call(router, http.MethodPost, "/campaign-segments", bearer, `{"name":"Ortu","kind":"parents","filter":{}}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("segment: %d %s", rec.Code, rec.Body.String())
	}
	rec = call(router, http.MethodPost, "/campaigns", bearer, `{"name":"Info","segment_id":"`+dataID(t, rec)+`","channel":"email","subject":"Info","body":"Halo"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("campaign: %d %s", rec.Code, rec.Body.String())
	}
	campID := dataID(t, rec)
	rec = call(router, http.MethodPost, "/campaigns/"+campID+"/send-now", bearer, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("send-now: %d %s", rec.Code, rec.Body.String())
	}
	if len(reps) != 1 {
		t.Fatalf("reports = %d, want 1", len(reps))
	}
	r := reps[0]
	if r.Kind != common.ExceptionKindDeadLetter || r.SchoolID != school || r.IdempotencyKey != common.DeadLetterKey("notification", "campaign", campID) ||
		r.PayloadSummary["failed_recipients"] != 3 || !strings.Contains(r.ErrorMessage, "3 penerima gagal") {
		t.Fatalf("report = %+v", r)
	}
}
