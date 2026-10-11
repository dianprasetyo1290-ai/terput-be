package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"skci/notification/internal/model"
	"skci/notification/internal/store"
)

func decodeThreads(t *testing.T, rec *httptest.ResponseRecorder) ([]model.ChatThread, int) {
	t.Helper()
	var env struct {
		Data []model.ChatThread `json:"data"`
		Meta struct {
			Total int `json:"total"`
		} `json:"meta"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("decode threads: %v (body %s)", err, rec.Body.String())
	}
	return env.Data, env.Meta.Total
}

// TestThreadAuthorization is the core chat rule: only a named participant of a
// thread in the caller's own school may read or post in it. A same-school
// non-participant is 403; another school's thread is 404; no token is 401.
func TestThreadAuthorization(t *testing.T) {
	fs, router := newTestServer(t)
	ctx := context.Background()

	thread, err := fs.CreateChatThread(ctx, store.CreateThreadInput{
		SchoolID:  schoolA,
		Subject:   "Konsultasi nilai",
		CreatedBy: userA,
	})
	if err != nil {
		t.Fatalf("seed thread: %v", err)
	}

	tokA := token(t, userA, schoolA, "guru")
	tokB := token(t, userB, schoolA, "siswa") // same school, NOT a participant
	tokC := token(t, userC, schoolB, "guru")  // different school
	base := "/chat/threads/" + thread.ID

	cases := []struct {
		name, method, path, bearer string
		body                       string
		want                       int
	}{
		{"participant reads messages", http.MethodGet, base + "/messages", tokA, "", http.StatusOK},
		{"participant posts a message", http.MethodPost, base + "/messages", tokA, `{"body":"halo"}`, http.StatusCreated},
		{"participant marks read", http.MethodPost, base + "/read", tokA, "", http.StatusOK},
		{"same-school non-participant cannot read", http.MethodGet, base + "/messages", tokB, "", http.StatusForbidden},
		{"same-school non-participant cannot post", http.MethodPost, base + "/messages", tokB, `{"body":"x"}`, http.StatusForbidden},
		{"same-school non-participant cannot mark read", http.MethodPost, base + "/read", tokB, "", http.StatusForbidden},
		{"other school cannot read", http.MethodGet, base + "/messages", tokC, "", http.StatusNotFound},
		{"other school cannot post", http.MethodPost, base + "/messages", tokC, `{"body":"x"}`, http.StatusNotFound},
		{"no token", http.MethodGet, base + "/messages", "", "", http.StatusUnauthorized},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rec := do(router, c.method, c.path, c.bearer, nil, c.body)
			if rec.Code != c.want {
				t.Fatalf("status = %d, want %d (body %s)", rec.Code, c.want, rec.Body.String())
			}
		})
	}

	// The non-participant never got a message written on their behalf.
	messages, _, err := fs.ListChatMessages(ctx, thread.ID, 1, 50)
	if err != nil {
		t.Fatalf("list messages: %v", err)
	}
	if len(messages) != 1 {
		t.Fatalf("messages = %d, want 1 (only userA's post succeeded)", len(messages))
	}
}

// TestThreadListOnlyReturnsOwnThreads checks GET /chat/threads never leaks a
// thread the caller is not a participant of.
func TestThreadListOnlyReturnsOwnThreads(t *testing.T) {
	fs, router := newTestServer(t)
	ctx := context.Background()

	mine, err := fs.CreateChatThread(ctx, store.CreateThreadInput{SchoolID: schoolA, Subject: "Punya A", CreatedBy: userA})
	if err != nil {
		t.Fatalf("seed thread A: %v", err)
	}
	if _, err := fs.CreateChatThread(ctx, store.CreateThreadInput{SchoolID: schoolA, Subject: "Punya B", CreatedBy: userB}); err != nil {
		t.Fatalf("seed thread B: %v", err)
	}
	if _, err := fs.CreateChatThread(ctx, store.CreateThreadInput{SchoolID: schoolB, Subject: "Sekolah lain", CreatedBy: userA}); err != nil {
		t.Fatalf("seed cross-school thread: %v", err)
	}

	tokA := token(t, userA, schoolA, "guru")
	rec := do(router, http.MethodGet, "/chat/threads", tokA, nil, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("list threads: status = %d, want 200 (body %s)", rec.Code, rec.Body.String())
	}
	threads, total := decodeThreads(t, rec)
	if total != 1 || len(threads) != 1 {
		t.Fatalf("threads total=%d len=%d, want exactly 1 (only A's own, same-school thread)", total, len(threads))
	}
	if threads[0].ID != mine.ID {
		t.Errorf("listed thread %s, want %s", threads[0].ID, mine.ID)
	}
}

// TestCreateThreadAddsCreatorAsParticipant covers POST /chat/threads: the
// creator can immediately use the thread.
func TestCreateThreadAddsCreatorAsParticipant(t *testing.T) {
	_, router := newTestServer(t)

	tokA := token(t, userA, schoolA, "guru")
	rec := do(router, http.MethodPost, "/chat/threads", tokA, nil, `{"subject":"Nilai rapor","participant_user_ids":["`+userB+`"]}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create thread: status = %d, want 201 (body %s)", rec.Code, rec.Body.String())
	}
	var env struct {
		Data model.ChatThread `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("decode created thread: %v", err)
	}
	roleByUser := map[string]string{}
	for _, p := range env.Data.Participants {
		roleByUser[p.UserID] = p.Role
	}
	if roleByUser[userA] != model.ChatRoleOwner {
		t.Errorf("creator role = %q, want %q", roleByUser[userA], model.ChatRoleOwner)
	}
	if roleByUser[userB] != model.ChatRoleMember {
		t.Errorf("participant role = %q, want %q", roleByUser[userB], model.ChatRoleMember)
	}
}

// TestMarkThreadReadIdempotent checks read receipts are recorded once and a
// retry is a no-op.
func TestMarkThreadReadIdempotent(t *testing.T) {
	fs, router := newTestServer(t)
	ctx := context.Background()

	thread, err := fs.CreateChatThread(ctx, store.CreateThreadInput{
		SchoolID:     schoolA,
		Subject:      "Tanya",
		CreatedBy:    userA,
		Participants: []store.ParticipantInput{{UserID: userB, Role: model.ChatRoleMember}},
	})
	if err != nil {
		t.Fatalf("seed thread: %v", err)
	}
	for i := 0; i < 2; i++ {
		if _, err := fs.CreateChatMessage(ctx, store.CreateMessageInput{ThreadID: thread.ID, SenderUserID: userB, Body: "pesan"}); err != nil {
			t.Fatalf("seed message %d: %v", i, err)
		}
	}

	tokA := token(t, userA, schoolA, "guru")
	path := "/chat/threads/" + thread.ID + "/read"

	rec1 := do(router, http.MethodPost, path, tokA, nil, "")
	if rec1.Code != http.StatusOK {
		t.Fatalf("first mark-read: status = %d, want 200 (body %s)", rec1.Code, rec1.Body.String())
	}
	var env struct {
		Data struct {
			Marked int `json:"marked"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec1.Body.Bytes(), &env); err != nil {
		t.Fatalf("decode mark-read: %v", err)
	}
	if env.Data.Marked != 2 {
		t.Errorf("first mark-read marked = %d, want 2", env.Data.Marked)
	}

	rec2 := do(router, http.MethodPost, path, tokA, nil, "")
	if err := json.Unmarshal(rec2.Body.Bytes(), &env); err != nil {
		t.Fatalf("decode second mark-read: %v", err)
	}
	if env.Data.Marked != 0 {
		t.Errorf("second mark-read marked = %d, want 0 (idempotent)", env.Data.Marked)
	}
}
