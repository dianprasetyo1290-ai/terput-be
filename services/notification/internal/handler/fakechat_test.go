package handler

import (
	"context"
	"time"

	"skci/notification/internal/model"
	"skci/notification/internal/store"
)

// ---- T13 chat extension of the in-memory fake ---------------------------------

func (f *fakeStore) FindActiveTeacherParentThread(_ context.Context, schoolID, teacherID, guardianID, studentID string) (*model.ChatThread, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, t := range f.threads {
		if t.SchoolID == schoolID && t.Kind == model.ChatKindTeacherParent && t.ClosedAt == nil &&
			*t.TeacherUserID == teacherID && *t.GuardianUserID == guardianID && *t.StudentUserID == studentID {
			c := *t
			return &c, nil
		}
	}
	return nil, store.ErrNotFound
}

func (f *fakeStore) ArchiveChatThread(_ context.Context, threadID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if t, ok := f.threads[threadID]; ok && t.ArchivedAt == nil {
		now := time.Now()
		t.ArchivedAt = &now
	}
	return nil
}

func (f *fakeStore) GetChatMessage(_ context.Context, id, schoolID string) (*model.ChatMessage, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for threadID, list := range f.messages {
		for _, m := range list {
			if m.ID == id && f.threads[threadID].SchoolID == schoolID {
				c := *m
				return &c, nil
			}
		}
	}
	return nil, store.ErrNotFound
}

func (f *fakeStore) GetChatSettings(_ context.Context, schoolID string) (*model.ChatSettings, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if s, ok := f.chatSettings[schoolID]; ok {
		c := *s
		return &c, nil
	}
	return &model.ChatSettings{SchoolID: schoolID, ServiceDays: []int{}, MaxAttachmentBytes: model.DefaultMaxAttachmentBytes}, nil
}

func (f *fakeStore) UpsertChatSettings(_ context.Context, in model.ChatSettings) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.chatSettings == nil {
		f.chatSettings = map[string]*model.ChatSettings{}
	}
	c := in
	f.chatSettings[in.SchoolID] = &c
	return nil
}

func (f *fakeStore) CreateChatReport(_ context.Context, schoolID, messageID, reporterID, reason string) (*model.ChatMessageReport, error) {
	f.mu.Lock()
	if f.reports == nil {
		f.reports = map[string]*model.ChatMessageReport{}
	}
	for _, r := range f.reports {
		if r.MessageID == messageID && r.ReporterUserID == reporterID {
			f.mu.Unlock()
			return nil, store.ErrConflict
		}
	}
	var threadID string
	for tid, list := range f.messages {
		for _, m := range list {
			if m.ID == messageID {
				threadID = tid
			}
		}
	}
	r := &model.ChatMessageReport{ID: newID(), SchoolID: schoolID, MessageID: messageID, ThreadID: threadID,
		ReporterUserID: reporterID, Reason: reason, Status: model.ReportOpen, CreatedAt: time.Now()}
	f.reports[r.ID] = r
	f.mu.Unlock()
	return f.GetChatReport(context.Background(), r.ID, schoolID)
}

func (f *fakeStore) GetChatReport(_ context.Context, id, schoolID string) (*model.ChatMessageReport, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	r, ok := f.reports[id]
	if !ok || r.SchoolID != schoolID {
		return nil, store.ErrNotFound
	}
	c := *r
	return &c, nil
}

func (f *fakeStore) ListChatReports(_ context.Context, schoolID, status string, _, _ int) ([]model.ChatMessageReport, int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := []model.ChatMessageReport{}
	for _, r := range f.reports {
		if r.SchoolID == schoolID && (status == "" || r.Status == status) {
			out = append(out, *r)
		}
	}
	return out, len(out), nil
}

func (f *fakeStore) ReviewChatReport(_ context.Context, id, schoolID, status, note, reviewerID string) (*model.ChatMessageReport, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	r, ok := f.reports[id]
	if !ok || r.SchoolID != schoolID || r.Status != model.ReportOpen {
		return nil, store.ErrConflict
	}
	now := time.Now()
	r.Status, r.ReviewNote, r.ReviewedBy, r.ReviewedAt = status, &note, &reviewerID, &now
	c := *r
	return &c, nil
}
