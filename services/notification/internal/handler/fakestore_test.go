package handler

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"sort"
	"sync"
	"time"

	"skci/notification/internal/model"
	"skci/notification/internal/store"
)

// fakeStore is an in-memory DataStore. It exists so the authorization and
// idempotency rules are exercised at the HTTP layer without a database; the
// SQL that backs each rule is covered separately by the (skippable) DB tests in
// internal/store.
type fakeStore struct {
	mu sync.Mutex

	notifications map[string]*model.Notification
	idempotency   map[string]string // schoolID + "|" + key -> notification id

	prefs map[string]map[string]bool // schoolID + "|" + userID -> channel -> enabled

	templates map[string]*model.NotificationTemplate

	deliveries map[string][]*model.NotificationDelivery // notificationID -> rows

	threads      map[string]*model.ChatThread
	participants map[string]map[string]string // threadID -> userID -> role
	messages     map[string][]*model.ChatMessage
	reads        map[string]map[string]bool // messageID -> userID -> read

	// T13 (lazily initialised in fakechat_test.go)
	chatSettings map[string]*model.ChatSettings
	reports      map[string]*model.ChatMessageReport
}

func newFakeStore() *fakeStore {
	return &fakeStore{
		notifications: map[string]*model.Notification{},
		idempotency:   map[string]string{},
		prefs:         map[string]map[string]bool{},
		templates:     map[string]*model.NotificationTemplate{},
		deliveries:    map[string][]*model.NotificationDelivery{},
		threads:       map[string]*model.ChatThread{},
		participants:  map[string]map[string]string{},
		messages:      map[string][]*model.ChatMessage{},
		reads:         map[string]map[string]bool{},
	}
}

func newID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func userKey(schoolID, userID string) string { return schoolID + "|" + userID }

func cloneNotification(n *model.Notification) *model.Notification {
	c := *n
	if n.Data != nil {
		c.Data = append([]byte(nil), n.Data...)
	}
	if n.ReadAt != nil {
		t := *n.ReadAt
		c.ReadAt = &t
	}
	return &c
}

// ---- notifications ---------------------------------------------------------

func (f *fakeStore) CreateNotification(_ context.Context, in store.CreateNotificationInput) (*model.Notification, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if in.IdempotencyKey != nil {
		key := in.SchoolID + "|" + *in.IdempotencyKey
		if id, ok := f.idempotency[key]; ok {
			return cloneNotification(f.notifications[id]), false, nil
		}
	}
	n := &model.Notification{
		ID:             newID(),
		SchoolID:       in.SchoolID,
		UserID:         in.UserID,
		Type:           in.Type,
		Title:          in.Title,
		Body:           in.Body,
		Data:           in.Data,
		IdempotencyKey: in.IdempotencyKey,
		CreatedAt:      time.Now(),
	}
	f.notifications[n.ID] = n
	if in.IdempotencyKey != nil {
		f.idempotency[in.SchoolID+"|"+*in.IdempotencyKey] = n.ID
	}
	return cloneNotification(n), true, nil
}

func (f *fakeStore) GetNotification(_ context.Context, id, schoolID, userID string) (*model.Notification, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	n, ok := f.notifications[id]
	if !ok || n.SchoolID != schoolID || n.UserID != userID {
		return nil, store.ErrNotFound
	}
	return cloneNotification(n), nil
}

func (f *fakeStore) GetNotificationByID(_ context.Context, id, schoolID string) (*model.Notification, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	n, ok := f.notifications[id]
	if !ok || n.SchoolID != schoolID {
		return nil, store.ErrNotFound
	}
	return cloneNotification(n), nil
}

func (f *fakeStore) ListNotifications(_ context.Context, filter store.NotificationFilter) ([]model.Notification, int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var matching []*model.Notification
	for _, n := range f.notifications {
		if n.SchoolID != filter.SchoolID || n.UserID != filter.UserID {
			continue
		}
		if filter.Type != "" && n.Type != filter.Type {
			continue
		}
		if filter.Unread && n.ReadAt != nil {
			continue
		}
		matching = append(matching, n)
	}
	sort.Slice(matching, func(i, j int) bool { return matching[i].CreatedAt.After(matching[j].CreatedAt) })
	total := len(matching)

	perPage := filter.PerPage
	if perPage < 1 {
		perPage = 20
	}
	page := filter.Page
	if page < 1 {
		page = 1
	}
	start := (page - 1) * perPage
	if start > total {
		start = total
	}
	end := start + perPage
	if end > total {
		end = total
	}
	out := make([]model.Notification, 0, end-start)
	for _, n := range matching[start:end] {
		out = append(out, *cloneNotification(n))
	}
	return out, total, nil
}

func (f *fakeStore) CountUnreadNotifications(_ context.Context, schoolID, userID string) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	count := 0
	for _, n := range f.notifications {
		if n.SchoolID == schoolID && n.UserID == userID && n.ReadAt == nil {
			count++
		}
	}
	return count, nil
}

func (f *fakeStore) MarkNotificationRead(_ context.Context, id, schoolID, userID string) (*model.Notification, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	n, ok := f.notifications[id]
	if !ok || n.SchoolID != schoolID || n.UserID != userID {
		return nil, store.ErrNotFound
	}
	if n.ReadAt == nil {
		t := time.Now()
		n.ReadAt = &t
	}
	return cloneNotification(n), nil
}

func (f *fakeStore) MarkAllNotificationsRead(_ context.Context, schoolID, userID string) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	count := 0
	for _, n := range f.notifications {
		if n.SchoolID == schoolID && n.UserID == userID && n.ReadAt == nil {
			t := time.Now()
			n.ReadAt = &t
			count++
		}
	}
	return count, nil
}

// ---- preferences -----------------------------------------------------------

func (f *fakeStore) ListNotificationPreferences(_ context.Context, schoolID, userID string) ([]model.NotificationPreference, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := []model.NotificationPreference{}
	for channel, enabled := range f.prefs[userKey(schoolID, userID)] {
		out = append(out, model.NotificationPreference{SchoolID: schoolID, UserID: userID, Channel: channel, Enabled: enabled})
	}
	return out, nil
}

func (f *fakeStore) UpsertNotificationPreferences(_ context.Context, schoolID, userID string, prefs []store.PreferenceInput) ([]model.NotificationPreference, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	key := userKey(schoolID, userID)
	if f.prefs[key] == nil {
		f.prefs[key] = map[string]bool{}
	}
	for _, p := range prefs {
		f.prefs[key][p.Channel] = p.Enabled
	}
	return f.ListNotificationPreferences(context.Background(), schoolID, userID)
}

func (f *fakeStore) IsChannelEnabled(_ context.Context, schoolID, userID, channel string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if enabled, ok := f.prefs[userKey(schoolID, userID)][channel]; ok {
		return enabled, nil
	}
	return model.DefaultPreference(channel), nil
}

// ---- templates -------------------------------------------------------------

func (f *fakeStore) ListNotificationTemplates(_ context.Context, schoolID, key string) ([]model.NotificationTemplate, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := []model.NotificationTemplate{}
	for _, t := range f.templates {
		if t.SchoolID == schoolID && (key == "" || t.Key == key) {
			out = append(out, *t)
		}
	}
	return out, nil
}

func (f *fakeStore) GetNotificationTemplate(_ context.Context, id, schoolID string) (*model.NotificationTemplate, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	t, ok := f.templates[id]
	if !ok || t.SchoolID != schoolID {
		return nil, store.ErrNotFound
	}
	return t, nil
}

func (f *fakeStore) CreateNotificationTemplate(_ context.Context, in store.TemplateInput) (*model.NotificationTemplate, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, t := range f.templates {
		if t.SchoolID == in.SchoolID && t.Key == in.Key && t.Channel == in.Channel && t.Version == in.Version && in.Version > 0 {
			return nil, store.ErrConflict
		}
	}
	version := in.Version
	if version <= 0 {
		version = 1
		for _, t := range f.templates {
			if t.SchoolID == in.SchoolID && t.Key == in.Key && t.Channel == in.Channel && t.Version >= version {
				version = t.Version + 1
			}
		}
	}
	t := &model.NotificationTemplate{
		ID: newID(), SchoolID: in.SchoolID, Key: in.Key, Version: version,
		Channel: in.Channel, Subject: in.Subject, Body: in.Body,
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	f.templates[t.ID] = t
	return t, nil
}

func (f *fakeStore) UpdateNotificationTemplate(_ context.Context, id, schoolID string, in store.TemplateInput) (*model.NotificationTemplate, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	t, ok := f.templates[id]
	if !ok || t.SchoolID != schoolID {
		return nil, store.ErrNotFound
	}
	t.Subject, t.Body, t.UpdatedAt = in.Subject, in.Body, time.Now()
	return t, nil
}

func (f *fakeStore) DeleteNotificationTemplate(_ context.Context, id, schoolID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	t, ok := f.templates[id]
	if !ok || t.SchoolID != schoolID {
		return store.ErrNotFound
	}
	delete(f.templates, id)
	return nil
}

func (f *fakeStore) FindNotificationTemplate(_ context.Context, schoolID, key, channel string) (*model.NotificationTemplate, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var best *model.NotificationTemplate
	for _, t := range f.templates {
		if t.SchoolID != schoolID || t.Key != key || t.Channel != channel {
			continue
		}
		if best == nil || t.Version > best.Version {
			best = t
		}
	}
	if best == nil {
		return nil, store.ErrNotFound
	}
	return best, nil
}

// ---- deliveries ------------------------------------------------------------

func (f *fakeStore) CreateDelivery(_ context.Context, notificationID, channel string) (*model.NotificationDelivery, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, d := range f.deliveries[notificationID] {
		if d.Channel == channel {
			return d, nil
		}
	}
	d := &model.NotificationDelivery{
		ID: newID(), NotificationID: notificationID, Channel: channel,
		Status: model.DeliveryPending, CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	f.deliveries[notificationID] = append(f.deliveries[notificationID], d)
	return d, nil
}

func (f *fakeStore) ListDeliveries(_ context.Context, notificationID string) ([]model.NotificationDelivery, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := []model.NotificationDelivery{}
	for _, d := range f.deliveries[notificationID] {
		out = append(out, *d)
	}
	return out, nil
}

func (f *fakeStore) UpdateDelivery(_ context.Context, id string, in store.UpdateDeliveryInput) (*model.NotificationDelivery, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, rows := range f.deliveries {
		for _, d := range rows {
			if d.ID != id {
				continue
			}
			d.Status = in.Status
			d.UpdatedAt = time.Now()
			if in.IncrementAttempts {
				d.Attempts++
			}
			if in.Status == model.DeliverySent {
				d.LastError = nil
			} else if in.LastError != "" {
				d.LastError = &in.LastError
			}
			if in.ProviderMessageID != "" {
				d.ProviderMessageID = &in.ProviderMessageID
			}
			return d, nil
		}
	}
	return nil, store.ErrNotFound
}

// ---- chat ------------------------------------------------------------------

func (f *fakeStore) CreateChatThread(_ context.Context, in store.CreateThreadInput) (*model.ChatThread, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	t := &model.ChatThread{
		ID: newID(), SchoolID: in.SchoolID, Subject: in.Subject,
		CreatedBy: in.CreatedBy, CreatedAt: time.Now(), Kind: in.Kind,
	}
	if t.Kind == "" {
		t.Kind = model.ChatKindGeneral
	}
	if in.Kind == model.ChatKindTeacherParent {
		t.StudentUserID, t.TeacherUserID, t.GuardianUserID = &in.StudentUserID, &in.TeacherUserID, &in.GuardianUserID
	}
	f.threads[t.ID] = t
	f.participants[t.ID] = map[string]string{}
	order := []string{}
	add := func(userID, role string) {
		if userID == "" {
			return
		}
		if _, exists := f.participants[t.ID][userID]; exists {
			return
		}
		if role == "" {
			role = model.ChatRoleMember
		}
		f.participants[t.ID][userID] = role
		order = append(order, userID)
	}
	add(in.CreatedBy, model.ChatRoleOwner)
	for _, p := range in.Participants {
		add(p.UserID, p.Role)
	}
	for _, userID := range order {
		t.Participants = append(t.Participants, model.ChatParticipant{
			ThreadID: t.ID, UserID: userID, Role: f.participants[t.ID][userID], JoinedAt: t.CreatedAt,
		})
	}
	return t, nil
}

func (f *fakeStore) ListChatThreadsForUser(_ context.Context, schoolID, userID string, page, perPage int) ([]model.ChatThread, int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var matching []*model.ChatThread
	for id, t := range f.threads {
		if t.SchoolID != schoolID {
			continue
		}
		if _, ok := f.participants[id][userID]; ok {
			matching = append(matching, t)
		}
	}
	sort.Slice(matching, func(i, j int) bool { return matching[i].CreatedAt.After(matching[j].CreatedAt) })
	total := len(matching)
	if perPage < 1 {
		perPage = 20
	}
	if page < 1 {
		page = 1
	}
	start := (page - 1) * perPage
	if start > total {
		start = total
	}
	end := start + perPage
	if end > total {
		end = total
	}
	out := make([]model.ChatThread, 0, end-start)
	for _, t := range matching[start:end] {
		out = append(out, *t)
	}
	return out, total, nil
}

func (f *fakeStore) GetChatThread(_ context.Context, id, schoolID string) (*model.ChatThread, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	t, ok := f.threads[id]
	if !ok || t.SchoolID != schoolID {
		return nil, store.ErrNotFound
	}
	c := *t
	return &c, nil
}

func (f *fakeStore) IsChatParticipant(_ context.Context, threadID, userID string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	_, ok := f.participants[threadID][userID]
	return ok, nil
}

func (f *fakeStore) ListChatMessages(_ context.Context, threadID string, page, perPage int) ([]model.ChatMessage, int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	all := f.messages[threadID]
	total := len(all)
	if perPage < 1 {
		perPage = 20
	}
	if page < 1 {
		page = 1
	}
	start := (page - 1) * perPage
	if start > total {
		start = total
	}
	end := start + perPage
	if end > total {
		end = total
	}
	out := make([]model.ChatMessage, 0, end-start)
	for _, m := range all[start:end] {
		c := *m
		for userID := range f.reads[m.ID] {
			c.ReadBy = append(c.ReadBy, userID)
		}
		c.ReadCount = len(c.ReadBy)
		out = append(out, c)
	}
	return out, total, nil
}

func (f *fakeStore) CreateChatMessage(_ context.Context, in store.CreateMessageInput) (*model.ChatMessage, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	m := &model.ChatMessage{
		ID: newID(), ThreadID: in.ThreadID, SenderUserID: in.SenderUserID,
		Body: in.Body, AttachmentKey: in.AttachmentKey, CreatedAt: time.Now(),
		AttachmentName: in.AttachmentName, AttachmentMime: in.AttachmentMime, AttachmentSize: in.AttachmentSize,
		OutsideServiceHours: in.OutsideServiceHours,
	}
	f.messages[in.ThreadID] = append(f.messages[in.ThreadID], m)
	c := *m
	return &c, nil
}

func (f *fakeStore) MarkChatThreadRead(_ context.Context, threadID, userID string) ([]string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	marked := []string{}
	for _, m := range f.messages[threadID] {
		if m.SenderUserID == userID {
			continue
		}
		if f.reads[m.ID] == nil {
			f.reads[m.ID] = map[string]bool{}
		}
		if f.reads[m.ID][userID] {
			continue
		}
		f.reads[m.ID][userID] = true
		marked = append(marked, m.ID)
	}
	return marked, nil
}
