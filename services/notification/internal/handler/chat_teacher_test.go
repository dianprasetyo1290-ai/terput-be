package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"skci/notification/internal/client"
	"skci/notification/internal/model"
	"skci/notification/internal/provider"
	"skci/notification/internal/storage"
	"skci/notification/internal/store"
)

const (
	mathTeacher = "dddddddd-dddd-dddd-dddd-dddddddddddd"
	waliKelas9B = "eeeeeeee-eeee-eeee-eeee-eeeeeeeeeeee"
	parent9B    = "ffffffff-ffff-ffff-ffff-ffffffffffff"
	student9A   = "9a9a9a9a-9a9a-9a9a-9a9a-9a9a9a9a9a9a"
	student9B   = "9b9b9b9b-9b9b-9b9b-9b9b-9b9b9b9b9b9b"
	headUser    = "12121212-1212-1212-1212-121212121212"
)

// chatServer builds a router with a scenario: the math teacher teaches 9A
// only, the wali kelas of 9B teaches 9B, parent9B is the guardian of student9B.
func chatServer(t *testing.T, now time.Time) (*fakeStore, *fakeRelations, http.Handler) {
	t.Helper()
	fs := newFakeStore()
	rel := newFakeRelations()
	rel.roles[parent9B] = []string{roleOrangTua}
	rel.roles[mathTeacher] = []string{roleGuru}
	rel.roles[waliKelas9B] = []string{roleWalikelas}
	rel.teacherStudents[mathTeacher] = map[string]bool{student9A: true}
	rel.teacherStudents[waliKelas9B] = map[string]bool{student9B: true}
	rel.guardianOf[parent9B] = []string{student9B}
	rel.studentTeachers[student9B] = []client.StudentTeacher{{UserID: waliKelas9B, Label: "Wali kelas"}}
	rel.heads = []client.UserProfile{{ID: headUser, Name: "Kepala"}}
	h := New(fs, client.New("", ""), provider.NoopProvider{}, false)
	h.Relations = rel
	h.Attachments = storage.NewLocal(t.TempDir(), 50<<20)
	loc, _ := time.LoadLocation("Asia/Jakarta")
	h.Location = loc
	h.Now = func() time.Time { return now }
	return fs, rel, NewRouter(h, RouterConfig{JWTSecret: testJWTSecret, InternalKey: testInternalKey})
}

func threadCount(fs *fakeStore) int {
	fs.mu.Lock()
	defer fs.mu.Unlock()
	return len(fs.threads)
}

// AC-148-01: a math teacher who does not teach 9B cannot start a chat with a
// 9B parent — neither through the teacher–parent route nor the general one.
func TestAC14801TeacherMustTeachStudent(t *testing.T) {
	fs, _, router := chatServer(t, time.Now())
	tok := token(t, mathTeacher, schoolA, roleGuru)

	rec := do(router, http.MethodPost, "/chat/teacher-parent/threads", tok, nil, `{"student_user_id":"`+student9B+`","guardian_user_id":"`+parent9B+`"}`)
	if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), "tidak mengajar atau mewalikan") {
		t.Fatalf("teacher-parent: %d %s", rec.Code, rec.Body.String())
	}
	rec = do(router, http.MethodPost, "/chat/threads", tok, nil, `{"subject":"Nilai","participant_user_ids":["`+parent9B+`"]}`)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("general thread bypass: %d %s", rec.Code, rec.Body.String())
	}
	rec = do(router, http.MethodGet, "/chat/teacher-parent/contacts?student_user_id="+student9B, tok, nil, "")
	if rec.Code != http.StatusForbidden {
		t.Fatalf("contacts: %d %s", rec.Code, rec.Body.String())
	}
	if n := threadCount(fs); n != 0 {
		t.Fatalf("%d threads created, want 0", n)
	}
}

func TestTeacherParentThreadCreateAndReuse(t *testing.T) {
	fs, _, router := chatServer(t, time.Now())
	tok := token(t, waliKelas9B, schoolA, roleWalikelas)
	body := `{"student_user_id":"` + student9B + `"}` // guardian resolved from identity
	rec := do(router, http.MethodPost, "/chat/teacher-parent/threads", tok, nil, body)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", rec.Code, rec.Body.String())
	}
	var env struct {
		Data model.ChatThread `json:"data"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &env)
	if env.Data.Kind != model.ChatKindTeacherParent || env.Data.GuardianUserID == nil || *env.Data.GuardianUserID != parent9B {
		t.Fatalf("thread = %+v", env.Data)
	}
	// The parent starting the same trio gets the existing thread back.
	ptok := token(t, parent9B, schoolA, roleOrangTua)
	rec = do(router, http.MethodPost, "/chat/teacher-parent/threads", ptok, nil, `{"student_user_id":"`+student9B+`","teacher_user_id":"`+waliKelas9B+`"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("reuse: %d %s", rec.Code, rec.Body.String())
	}
	if threadCount(fs) != 1 {
		t.Fatalf("threads = %d, want 1", threadCount(fs))
	}
	// The parent may not reach the math teacher about 9B.
	rec = do(router, http.MethodPost, "/chat/teacher-parent/threads", ptok, nil, `{"student_user_id":"`+student9B+`","teacher_user_id":"`+mathTeacher+`"}`)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("parent → unrelated teacher: %d", rec.Code)
	}
	// Parent contacts list only the 9B wali kelas.
	rec = do(router, http.MethodGet, "/chat/teacher-parent/contacts?student_user_id="+student9B, ptok, nil, "")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), waliKelas9B) || strings.Contains(rec.Body.String(), mathTeacher) {
		t.Fatalf("contacts: %d %s", rec.Code, rec.Body.String())
	}
	// A parent who is not the guardian is refused.
	other := token(t, userC, schoolA, roleOrangTua)
	if rec := do(router, http.MethodGet, "/chat/teacher-parent/contacts?student_user_id="+student9B, other, nil, ""); rec.Code != http.StatusForbidden {
		t.Fatalf("non-guardian contacts: %d", rec.Code)
	}
}

func TestRelationLookupFailureFailsClosed(t *testing.T) {
	fs, rel, router := chatServer(t, time.Now())
	rel.fail = true
	tok := token(t, waliKelas9B, schoolA, roleWalikelas)
	if rec := do(router, http.MethodPost, "/chat/teacher-parent/threads", tok, nil, `{"student_user_id":"`+student9B+`","guardian_user_id":"`+parent9B+`"}`); rec.Code != http.StatusForbidden {
		t.Fatalf("teacher-parent with downstream down: %d", rec.Code)
	}
	if rec := do(router, http.MethodPost, "/chat/threads", tok, nil, `{"subject":"x","participant_user_ids":["`+parent9B+`"]}`); rec.Code != http.StatusForbidden {
		t.Fatalf("general with downstream down: %d", rec.Code)
	}
	if threadCount(fs) != 0 {
		t.Fatal("thread created despite failed relation lookup")
	}
}

func seedTeacherParentThread(t *testing.T, fs *fakeStore) *model.ChatThread {
	t.Helper()
	th, err := fs.CreateChatThread(context.Background(), store.CreateThreadInput{
		SchoolID: schoolA, Subject: "Ananda", CreatedBy: waliKelas9B,
		Participants: []store.ParticipantInput{{UserID: parent9B}},
		Kind:         model.ChatKindTeacherParent, StudentUserID: student9B, TeacherUserID: waliKelas9B, GuardianUserID: parent9B,
	})
	if err != nil {
		t.Fatal(err)
	}
	return th
}

// AC-148-02: service hours 07–17, parent sends at 21:00 WIB → delivered with
// a service notice; the other participant gets an in-app notification.
func TestAC14802OutsideServiceHours(t *testing.T) {
	loc, _ := time.LoadLocation("Asia/Jakarta")
	fs, _, router := chatServer(t, time.Date(2026, 10, 12, 21, 0, 0, 0, loc))
	start, end := "07:00", "17:00"
	_ = fs.UpsertChatSettings(context.Background(), model.ChatSettings{SchoolID: schoolA, ServiceStart: &start, ServiceEnd: &end, MaxAttachmentBytes: model.DefaultMaxAttachmentBytes})
	th := seedTeacherParentThread(t, fs)

	rec := do(router, http.MethodPost, "/chat/threads/"+th.ID+"/messages", token(t, parent9B, schoolA, roleOrangTua), nil, `{"body":"Selamat malam Bu"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("post: %d %s", rec.Code, rec.Body.String())
	}
	var env struct {
		Data model.ChatMessage `json:"data"`
		Meta struct {
			ServiceNotice string `json:"service_notice"`
		} `json:"meta"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &env)
	if !env.Data.OutsideServiceHours || !strings.Contains(env.Meta.ServiceNotice, "07:00–17:00") {
		t.Fatalf("message=%+v notice=%q", env.Data, env.Meta.ServiceNotice)
	}
	n, _ := fs.CountUnreadNotifications(context.Background(), schoolA, waliKelas9B)
	if n != 1 {
		t.Fatalf("teacher unread notifications = %d, want 1", n)
	}
	if s, _ := fs.CountUnreadNotifications(context.Background(), schoolA, parent9B); s != 0 {
		t.Fatalf("sender notified about own message")
	}
}

func multipartBody(t *testing.T, name string, content []byte) (*bytes.Buffer, string) {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, _ := mw.CreateFormFile("file", name)
	_, _ = fw.Write(content)
	_ = mw.WriteField("body", "rapor terlampir")
	_ = mw.Close()
	return &buf, mw.FormDataContentType()
}

func TestAttachmentUploadLimitAndDownload(t *testing.T) {
	fs, _, router := chatServer(t, time.Now())
	_ = fs.UpsertChatSettings(context.Background(), model.ChatSettings{SchoolID: schoolA, MaxAttachmentBytes: 1024})
	th := seedTeacherParentThread(t, fs)
	tok := token(t, waliKelas9B, schoolA, roleWalikelas)
	pdf := append([]byte("%PDF-1.4\n"), bytes.Repeat([]byte("x"), 100)...)

	post := func(content []byte, name, bearer string) *httptest.ResponseRecorder {
		body, ctype := multipartBody(t, name, content)
		req := httptest.NewRequest(http.MethodPost, "/chat/threads/"+th.ID+"/attachments", body)
		req.Header.Set("Content-Type", ctype)
		req.Header.Set("Authorization", "Bearer "+bearer)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		return rec
	}
	if rec := post(append([]byte("%PDF-1.4\n"), bytes.Repeat([]byte("x"), 2048)...), "big.pdf", tok); rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("oversize: %d %s", rec.Code, rec.Body.String())
	}
	if rec := post([]byte("MZ executable"), "virus.exe", tok); rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("bad mime: %d %s", rec.Code, rec.Body.String())
	}
	rec := post(pdf, "rapor.pdf", tok)
	if rec.Code != http.StatusCreated {
		t.Fatalf("upload: %d %s", rec.Code, rec.Body.String())
	}
	var env struct {
		Data model.ChatMessage `json:"data"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &env)
	if env.Data.AttachmentName == nil || *env.Data.AttachmentName != "rapor.pdf" {
		t.Fatalf("message = %+v", env.Data)
	}
	path := "/chat/messages/" + env.Data.ID + "/attachment"
	if rec := do(router, http.MethodGet, path, token(t, parent9B, schoolA, roleOrangTua), nil, ""); rec.Code != http.StatusOK || !bytes.Equal(rec.Body.Bytes(), pdf) {
		t.Fatalf("participant download: %d", rec.Code)
	}
	if rec := do(router, http.MethodGet, path, token(t, mathTeacher, schoolA, roleGuru), nil, ""); rec.Code != http.StatusForbidden {
		t.Fatalf("non-participant download: %d", rec.Code)
	}
	// A JSON message may not point at another thread's file.
	if rec := do(router, http.MethodPost, "/chat/threads/"+th.ID+"/messages", tok, nil, `{"attachment_key":"chat/other/x.pdf"}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("foreign attachment key: %d", rec.Code)
	}
}

func TestReportModerationAndNoDelete(t *testing.T) {
	fs, _, router := chatServer(t, time.Now())
	th := seedTeacherParentThread(t, fs)
	msg, _ := fs.CreateChatMessage(context.Background(), store.CreateMessageInput{ThreadID: th.ID, SenderUserID: waliKelas9B, Body: "pesan tidak pantas"})
	ptok := token(t, parent9B, schoolA, roleOrangTua)
	head := token(t, headUser, schoolA, roleKepalaSekolah)

	// Without any report the head cannot read the thread.
	if rec := do(router, http.MethodGet, "/chat/threads/"+th.ID+"/messages", head, nil, ""); rec.Code != http.StatusForbidden {
		t.Fatalf("moderator without report: %d", rec.Code)
	}
	if rec := do(router, http.MethodPost, "/chat/messages/"+msg.ID+"/report", token(t, mathTeacher, schoolA, roleGuru), nil, `{"reason":"x"}`); rec.Code != http.StatusForbidden {
		t.Fatalf("non-participant report: %d", rec.Code)
	}
	rec := do(router, http.MethodPost, "/chat/messages/"+msg.ID+"/report", ptok, nil, `{"reason":"kata kasar"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("report: %d %s", rec.Code, rec.Body.String())
	}
	if rec := do(router, http.MethodPost, "/chat/messages/"+msg.ID+"/report", ptok, nil, `{"reason":"lagi"}`); rec.Code != http.StatusConflict {
		t.Fatalf("duplicate report: %d", rec.Code)
	}
	if n, _ := fs.CountUnreadNotifications(context.Background(), schoolA, headUser); n != 1 {
		t.Fatalf("kepala sekolah notifications = %d, want 1", n)
	}
	var env struct {
		Data []model.ChatMessageReport `json:"data"`
	}
	rec = do(router, http.MethodGet, "/chat/moderation/reports?status=open", head, nil, "")
	_ = json.Unmarshal(rec.Body.Bytes(), &env)
	if len(env.Data) != 1 {
		t.Fatalf("reports: %s", rec.Body.String())
	}
	if rec := do(router, http.MethodGet, "/chat/moderation/reports/"+env.Data[0].ID, ptok, nil, ""); rec.Code != http.StatusForbidden {
		t.Fatalf("parent on moderation: %d", rec.Code)
	}
	rec = do(router, http.MethodGet, "/chat/moderation/reports/"+env.Data[0].ID, head, nil, "")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "pesan tidak pantas") {
		t.Fatalf("moderation detail: %d %s", rec.Code, rec.Body.String())
	}
	if rec := do(router, http.MethodPost, "/chat/moderation/reports/"+env.Data[0].ID+"/review", head, nil, `{"status":"reviewed","note":"sudah ditegur"}`); rec.Code != http.StatusOK {
		t.Fatalf("review: %d", rec.Code)
	}
	// Messages can never be deleted (no route), archiving keeps them.
	if rec := do(router, http.MethodDelete, "/chat/messages/"+msg.ID, ptok, nil, ""); rec.Code != http.StatusMethodNotAllowed && rec.Code != http.StatusNotFound {
		t.Fatalf("delete message: %d", rec.Code)
	}
	if rec := do(router, http.MethodPost, "/chat/threads/"+th.ID+"/archive", ptok, nil, ""); rec.Code != http.StatusOK {
		t.Fatalf("archive: %d", rec.Code)
	}
	msgs, _, _ := fs.ListChatMessages(context.Background(), th.ID, 1, 50)
	if len(msgs) != 1 {
		t.Fatalf("messages after archive = %d, want 1", len(msgs))
	}
}

func TestChatSettingsAdminOnly(t *testing.T) {
	_, _, router := chatServer(t, time.Now())
	body := `{"service_start":"07:00","service_end":"17:00","service_days":[1,2,3,4,5],"max_attachment_bytes":5242880}`
	if rec := do(router, http.MethodPut, "/chat-settings", token(t, waliKelas9B, schoolA, roleWalikelas), nil, body); rec.Code != http.StatusForbidden {
		t.Fatalf("non-admin: %d", rec.Code)
	}
	if rec := do(router, http.MethodPut, "/chat-settings", token(t, userA, schoolA, roleAdmin), nil, body); rec.Code != http.StatusOK {
		t.Fatalf("admin: %d %s", rec.Code, rec.Body.String())
	}
	if rec := do(router, http.MethodPut, "/chat-settings", token(t, userA, schoolA, roleAdmin), nil, `{"service_start":"07:00"}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("half pair: %d", rec.Code)
	}
}
