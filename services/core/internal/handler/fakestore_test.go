package handler

import (
	"context"
	"fmt"
	"slices"
	"sync"
	"time"

	"skci/core/internal/model"
	"skci/core/internal/store"
)

type fakeStore struct {
	mu           sync.Mutex
	flows        map[string]model.ApprovalFlow
	auditGrants  []model.AuditAccessGrant
	requests     map[string]model.ApprovalRequest
	steps        map[string][]model.ApprovalStep
	delegations  map[string]model.ApprovalDelegation
	exceptions   map[string]model.IntegrationException
	audits       []model.AuditEvent
	srCategories map[string]*model.ServiceRequestCategory
	helpArticles []model.HelpArticle
	idCounter    int
}

func newFakeStore() *fakeStore {
	return &fakeStore{
		flows:       make(map[string]model.ApprovalFlow),
		requests:    make(map[string]model.ApprovalRequest),
		steps:       make(map[string][]model.ApprovalStep),
		delegations: make(map[string]model.ApprovalDelegation),
		exceptions:  make(map[string]model.IntegrationException),
		audits:      nil,
	}
}

func (f *fakeStore) nextID(prefix string) string {
	f.idCounter++
	return fmt.Sprintf("00000000-0000-0000-0000-%012d", f.idCounter)
}

func (f *fakeStore) ListFlows(ctx context.Context, schoolID string) ([]model.ApprovalFlow, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var res []model.ApprovalFlow
	for _, fl := range f.flows {
		if fl.SchoolID == schoolID && fl.DeletedAt == nil {
			res = append(res, fl)
		}
	}
	return res, nil
}

func (f *fakeStore) GetFlow(ctx context.Context, schoolID, id string) (*model.ApprovalFlow, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	fl, ok := f.flows[id]
	if !ok || fl.SchoolID != schoolID || fl.DeletedAt != nil {
		return nil, store.ErrNotFound
	}
	return &fl, nil
}

func (f *fakeStore) GetFlowByRequestType(ctx context.Context, schoolID, requestType string) (*model.ApprovalFlow, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, fl := range f.flows {
		if fl.SchoolID == schoolID && fl.RequestType == requestType && fl.Active && fl.DeletedAt == nil {
			return &fl, nil
		}
	}
	return nil, store.ErrNotFound
}

func (f *fakeStore) CreateFlow(ctx context.Context, schoolID, createdBy string, flow model.ApprovalFlow) (*model.ApprovalFlow, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	id := f.nextID("flow")
	flow.ID = id
	flow.SchoolID = schoolID
	flow.CreatedBy = createdBy
	flow.CreatedAt = time.Now()
	flow.UpdatedAt = time.Now()
	f.flows[id] = flow
	return &flow, nil
}

func (f *fakeStore) UpdateFlow(ctx context.Context, schoolID, id string, flow model.ApprovalFlow) (*model.ApprovalFlow, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	existing, ok := f.flows[id]
	if !ok || existing.SchoolID != schoolID || existing.DeletedAt != nil {
		return nil, store.ErrNotFound
	}
	existing.Name = flow.Name
	existing.Active = flow.Active
	existing.Steps = flow.Steps
	existing.UpdatedAt = time.Now()
	f.flows[id] = existing
	return &existing, nil
}

func (f *fakeStore) DeleteFlow(ctx context.Context, schoolID, id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	existing, ok := f.flows[id]
	if !ok || existing.SchoolID != schoolID || existing.DeletedAt != nil {
		return store.ErrNotFound
	}
	now := time.Now()
	existing.DeletedAt = &now
	f.flows[id] = existing
	return nil
}

func (f *fakeStore) CreateRequest(ctx context.Context, req model.ApprovalRequest, steps []model.ApprovalStep) (*model.ApprovalRequest, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, r := range f.requests {
		if r.SchoolID == req.SchoolID && r.IdempotencyKey == req.IdempotencyKey {
			return &r, store.ErrIdempotentHit
		}
	}
	id := f.nextID("req")
	req.ID = id
	req.Status = model.RequestStatusPending
	req.CurrentSeq = func(v int) *int { return &v }(1)
	req.CreatedAt = time.Now()
	req.UpdatedAt = time.Now()

	for i := range steps {
		steps[i].ID = f.nextID("step")
		steps[i].RequestID = id
		steps[i].SchoolID = req.SchoolID
		steps[i].CreatedAt = time.Now()
		steps[i].UpdatedAt = time.Now()
	}
	req.Steps = steps
	f.requests[id] = req
	f.steps[id] = steps
	return &req, nil
}

func (f *fakeStore) GetRequestByID(ctx context.Context, schoolID, id string) (*model.ApprovalRequest, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	r, ok := f.requests[id]
	if !ok || r.SchoolID != schoolID {
		return nil, store.ErrNotFound
	}
	r.Steps = f.steps[id]
	return &r, nil
}

func (f *fakeStore) GetRequestByIdempotency(ctx context.Context, schoolID, key string) (*model.ApprovalRequest, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, r := range f.requests {
		if r.SchoolID == schoolID && r.IdempotencyKey == key {
			r.Steps = f.steps[r.ID]
			return &r, nil
		}
	}
	return nil, store.ErrNotFound
}

func (f *fakeStore) GetRequestBySource(ctx context.Context, schoolID, sourceService, sourceID string) (*model.ApprovalRequest, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, r := range f.requests {
		if r.SchoolID == schoolID && r.SourceService == sourceService && r.SourceID == sourceID {
			r.Steps = f.steps[r.ID]
			return &r, nil
		}
	}
	return nil, store.ErrNotFound
}

func (f *fakeStore) ListInbox(ctx context.Context, filter store.InboxFilter, page, perPage int) ([]model.ApprovalRequest, int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var res []model.ApprovalRequest
	for _, r := range f.requests {
		if r.SchoolID != filter.SchoolID {
			continue
		}
		if filter.Status == "pending" && r.Status != model.RequestStatusPending {
			continue
		}
		if filter.Status == "decided" && r.Status == model.RequestStatusPending {
			continue
		}
		r.Steps = f.steps[r.ID]
		res = append(res, r)
	}
	return res, len(res), nil
}

func (f *fakeStore) ListMine(ctx context.Context, schoolID, userID string, page, perPage int) ([]model.ApprovalRequest, int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var res []model.ApprovalRequest
	for _, r := range f.requests {
		if r.SchoolID == schoolID && r.MakerUserID == userID {
			r.Steps = f.steps[r.ID]
			res = append(res, r)
		}
	}
	return res, len(res), nil
}

func (f *fakeStore) DecideStep(ctx context.Context, schoolID, reqID string, seq int, decision string, decidedBy string, delegateOf *string, comment string) (*model.ApprovalRequest, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	r, ok := f.requests[reqID]
	if !ok || r.SchoolID != schoolID {
		return nil, store.ErrNotFound
	}
	if r.Status != model.RequestStatusPending {
		return nil, fmt.Errorf("already decided")
	}

	steps := f.steps[reqID]
	now := time.Now()

	if decision == "approved" {
		for i := range steps {
			if steps[i].Seq == seq {
				steps[i].Status = model.StepStatusApproved
				steps[i].DecidedBy = &decidedBy
				steps[i].DecidedAsDelegateOf = delegateOf
				steps[i].Comment = &comment
				steps[i].DecidedAt = &now
				break
			}
		}

		nextIdx := -1
		for i := range steps {
			if steps[i].Seq > seq {
				nextIdx = i
				break
			}
		}

		if nextIdx >= 0 {
			steps[nextIdx].Status = model.StepStatusPending
			r.CurrentSeq = &steps[nextIdx].Seq
		} else {
			r.Status = model.RequestStatusApproved
			r.DecidedAt = &now
		}
	} else if decision == "rejected" {
		for i := range steps {
			if steps[i].Seq == seq {
				steps[i].Status = model.StepStatusRejected
				steps[i].DecidedBy = &decidedBy
				steps[i].DecidedAsDelegateOf = delegateOf
				steps[i].Comment = &comment
				steps[i].DecidedAt = &now
			} else if steps[i].Seq > seq {
				steps[i].Status = model.StepStatusSkipped
			}
		}
		r.Status = model.RequestStatusRejected
		r.DecidedAt = &now
	}

	f.steps[reqID] = steps
	f.requests[reqID] = r
	r.Steps = steps
	return &r, nil
}

func (f *fakeStore) CancelRequest(ctx context.Context, reqID string) (*model.ApprovalRequest, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	r, ok := f.requests[reqID]
	if !ok {
		return nil, store.ErrNotFound
	}
	if r.Status != model.RequestStatusPending {
		return nil, fmt.Errorf("only pending can be cancelled")
	}
	now := time.Now()
	r.Status = model.RequestStatusCancelled
	r.DecidedAt = &now
	steps := f.steps[reqID]
	for i := range steps {
		if steps[i].Status == model.StepStatusPending || steps[i].Status == model.StepStatusWaiting {
			steps[i].Status = model.StepStatusSkipped
		}
	}
	f.steps[reqID] = steps
	f.requests[reqID] = r
	r.Steps = steps
	return &r, nil
}

func (f *fakeStore) MarkCallbackDone(ctx context.Context, reqID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	r, ok := f.requests[reqID]
	if !ok {
		return store.ErrNotFound
	}
	r.CallbackStatus = "done"
	r.CallbackAttempts++
	f.requests[reqID] = r
	return nil
}

func (f *fakeStore) MarkCallbackFailed(ctx context.Context, reqID string, maxAttempts int) (int, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	r, ok := f.requests[reqID]
	if !ok {
		return 0, false, store.ErrNotFound
	}
	r.CallbackAttempts++
	exhausted := r.CallbackAttempts >= maxAttempts
	if exhausted {
		r.CallbackStatus = "failed"
	} else {
		r.CallbackStatus = "pending"
	}
	f.requests[reqID] = r
	return r.CallbackAttempts, exhausted, nil
}

func (f *fakeStore) ListPendingCallbacks(ctx context.Context, maxAttempts int) ([]model.ApprovalRequest, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var res []model.ApprovalRequest
	for _, r := range f.requests {
		if (r.Status == model.RequestStatusApproved || r.Status == model.RequestStatusRejected) && r.CallbackStatus != "done" && r.CallbackAttempts < maxAttempts {
			r.Steps = f.steps[r.ID]
			res = append(res, r)
		}
	}
	return res, nil
}

func (f *fakeStore) ListExpiredSteps(ctx context.Context) ([]model.ApprovalStep, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var res []model.ApprovalStep
	now := time.Now()
	for _, reqSteps := range f.steps {
		for _, st := range reqSteps {
			if st.Status == model.StepStatusPending && st.DueAt != nil && st.DueAt.Before(now) {
				res = append(res, st)
			}
		}
	}
	return res, nil
}

func (f *fakeStore) EscalateStep(ctx context.Context, stepID string) (*model.ApprovalStep, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for reqID, reqSteps := range f.steps {
		for i, st := range reqSteps {
			if st.ID == stepID {
				reqSteps[i].Status = model.StepStatusEscalated
				newSeq := st.Seq + 1
				newStep := model.ApprovalStep{
					ID:           f.nextID("step"),
					SchoolID:     st.SchoolID,
					RequestID:    reqID,
					Seq:          newSeq,
					ApproverKind: model.ApproverKindRole,
					ApproverRole: st.EscalateToRole,
					Status:       model.StepStatusPending,
					SLAHours:     st.SLAHours,
				}
				reqSteps = append(reqSteps, newStep)
				f.steps[reqID] = reqSteps

				req := f.requests[reqID]
				req.CurrentSeq = &newSeq
				f.requests[reqID] = req
				return &newStep, nil
			}
		}
	}
	return nil, store.ErrNotFound
}

func (f *fakeStore) ListDelegations(ctx context.Context, schoolID, userID string) ([]model.ApprovalDelegation, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var res []model.ApprovalDelegation
	for _, d := range f.delegations {
		if d.SchoolID == schoolID && (d.FromUserID == userID || d.ToUserID == userID) {
			res = append(res, d)
		}
	}
	return res, nil
}

func (f *fakeStore) CreateDelegation(ctx context.Context, del model.ApprovalDelegation) (*model.ApprovalDelegation, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	id := f.nextID("del")
	del.ID = id
	del.CreatedAt = time.Now()
	f.delegations[id] = del
	return &del, nil
}

func (f *fakeStore) RevokeDelegation(ctx context.Context, schoolID, id, userID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	d, ok := f.delegations[id]
	if !ok || d.SchoolID != schoolID || d.FromUserID != userID {
		return store.ErrNotFound
	}
	now := time.Now()
	d.RevokedAt = &now
	f.delegations[id] = d
	return nil
}

func (f *fakeStore) GetActiveDelegationsForUser(ctx context.Context, schoolID, userID string) ([]model.ActiveDelegation, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	now := time.Now()
	var res []model.ActiveDelegation
	for _, d := range f.delegations {
		if d.SchoolID == schoolID && d.ToUserID == userID && d.RevokedAt == nil && !d.ValidFrom.After(now) && !d.ValidTo.Before(now) {
			res = append(res, model.ActiveDelegation{
				FromUserID: d.FromUserID,
				ToUserID:   d.ToUserID,
			})
		}
	}
	return res, nil
}

func (f *fakeStore) UpsertException(ctx context.Context, exc model.IntegrationException) (*model.IntegrationException, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for id, e := range f.exceptions {
		if e.SchoolID == exc.SchoolID && e.IdempotencyKey == exc.IdempotencyKey {
			e.Attempts++
			e.ErrorMessage = exc.ErrorMessage
			e.PayloadSummary = exc.PayloadSummary
			e.UpdatedAt = time.Now()
			f.exceptions[id] = e
			return &e, false, nil
		}
	}
	id := f.nextID("exc")
	exc.ID = id
	exc.Attempts = 1
	exc.Status = model.ExceptionStatusOpen
	exc.CreatedAt = time.Now()
	exc.UpdatedAt = time.Now()
	f.exceptions[id] = exc
	return &exc, true, nil
}

func (f *fakeStore) GetExceptionByID(ctx context.Context, schoolID, id string) (*model.IntegrationException, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	e, ok := f.exceptions[id]
	if !ok || e.SchoolID != schoolID {
		return nil, store.ErrNotFound
	}
	return &e, nil
}

func (f *fakeStore) ListExceptions(ctx context.Context, filter store.ExceptionFilter, page, perPage int) ([]model.IntegrationException, int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var res []model.IntegrationException
	for _, e := range f.exceptions {
		if e.SchoolID != filter.SchoolID {
			continue
		}
		if filter.Kind != "" && e.Kind != filter.Kind {
			continue
		}
		if !slices.Contains(filter.Kinds, e.Kind) {
			continue
		}
		if filter.Status != "" && e.Status != filter.Status {
			continue
		}
		if filter.Severity != "" && e.Severity != filter.Severity {
			continue
		}
		if filter.SourceService != "" && e.SourceService != filter.SourceService {
			continue
		}
		res = append(res, e)
	}
	return res, len(res), nil
}

func (f *fakeStore) AssignException(ctx context.Context, schoolID, id, assigneeUserID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	e, ok := f.exceptions[id]
	if !ok || e.SchoolID != schoolID {
		return store.ErrNotFound
	}
	e.AssigneeUserID = &assigneeUserID
	e.UpdatedAt = time.Now()
	f.exceptions[id] = e
	return nil
}

func (f *fakeStore) ResolveException(ctx context.Context, schoolID, id, resolvedBy, reason string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	e, ok := f.exceptions[id]
	if !ok || e.SchoolID != schoolID {
		return store.ErrNotFound
	}
	now := time.Now()
	e.Status = model.ExceptionStatusResolved
	e.ResolvedBy = &resolvedBy
	e.ResolutionReason = &reason
	e.ResolvedAt = &now
	e.UpdatedAt = now
	f.exceptions[id] = e
	return nil
}

func (f *fakeStore) ClaimExceptionForRetry(ctx context.Context, schoolID, id string, staleAfter time.Duration) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	e, ok := f.exceptions[id]
	if !ok || e.SchoolID != schoolID {
		return store.ErrNotFound
	}
	switch {
	case e.Status == model.ExceptionStatusResolved:
		return store.ErrConflict
	case e.Status == model.ExceptionStatusRetrying && time.Since(e.UpdatedAt) < staleAfter:
		return store.ErrRetryInProgress
	}
	e.Status = model.ExceptionStatusRetrying
	e.Attempts++
	e.UpdatedAt = time.Now()
	f.exceptions[id] = e
	return nil
}

func (f *fakeStore) ReopenException(ctx context.Context, schoolID, id, lastError string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	e, ok := f.exceptions[id]
	if !ok || e.SchoolID != schoolID {
		return store.ErrNotFound
	}
	e.Status = model.ExceptionStatusOpen
	e.ErrorMessage = lastError
	e.UpdatedAt = time.Now()
	f.exceptions[id] = e
	return nil
}

func (f *fakeStore) InsertAudit(ctx context.Context, event model.AuditEvent) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	event.ID = int64(len(f.audits) + 1)
	event.CreatedAt = time.Now()
	f.audits = append(f.audits, event)
	return nil
}

func (f *fakeStore) ListAudit(ctx context.Context, filter store.AuditFilter, page, perPage int) ([]model.AuditEvent, int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var res []model.AuditEvent
	for _, a := range f.audits {
		if filter.SchoolID != nil && a.SchoolID != nil && *a.SchoolID != *filter.SchoolID {
			continue
		}
		if filter.UserID != "" && (a.ActorUserID == nil || *a.ActorUserID != filter.UserID) {
			continue
		}
		if filter.Module != "" && a.Module != filter.Module {
			continue
		}
		if filter.CorrelationID != "" && (a.CorrelationID == nil || *a.CorrelationID != filter.CorrelationID) {
			continue
		}
		if len(filter.Actions) > 0 && !slices.Contains(filter.Actions, a.Action) {
			continue
		}
		if filter.Service != "" && a.Service != filter.Service {
			continue
		}
		if filter.From != nil && a.CreatedAt.Before(*filter.From) {
			continue
		}
		if filter.To != nil && a.CreatedAt.After(*filter.To) {
			continue
		}
		res = append(res, a)
	}
	return res, len(res), nil
}

func (f *fakeStore) ListServiceRequestCategories(ctx context.Context, schoolID string, activeOnly bool) ([]model.ServiceRequestCategory, error) {
	return nil, nil
}
func (f *fakeStore) GetServiceRequestCategory(ctx context.Context, schoolID, id string) (*model.ServiceRequestCategory, error) {
	if cat, ok := f.srCategories[id]; ok && cat.SchoolID == schoolID {
		return cat, nil
	}
	return nil, store.ErrNotFound
}
func (f *fakeStore) CreateServiceRequestCategory(ctx context.Context, cat model.ServiceRequestCategory) (*model.ServiceRequestCategory, error) {
	cat.ID = f.nextID("cat")
	return &cat, nil
}
func (f *fakeStore) UpdateServiceRequestCategory(ctx context.Context, schoolID, id string, cat model.ServiceRequestCategory) (*model.ServiceRequestCategory, error) {
	return &cat, nil
}
func (f *fakeStore) DeleteServiceRequestCategory(ctx context.Context, schoolID, id string) error {
	return nil
}
func (f *fakeStore) CreateServiceRequest(ctx context.Context, req model.ServiceRequest) (*model.ServiceRequest, error) {
	req.ID = f.nextID("sr")
	return &req, nil
}
func (f *fakeStore) GetServiceRequest(ctx context.Context, schoolID, id string) (*model.ServiceRequest, error) {
	return nil, store.ErrNotFound
}
func (f *fakeStore) ListParentServiceRequests(ctx context.Context, schoolID, requesterUserID string) ([]model.ServiceRequest, error) {
	return nil, nil
}
func (f *fakeStore) ListHandlerServiceRequests(ctx context.Context, schoolID string, roles []string, status string) ([]model.ServiceRequest, error) {
	return nil, nil
}
func (f *fakeStore) UpdateServiceRequestStatus(ctx context.Context, schoolID, id, newStatus, note, actorUserID string) (*model.ServiceRequest, error) {
	return nil, nil
}
func (f *fakeStore) UpdateServiceRequestResult(ctx context.Context, schoolID, id string, resultAttachments []byte) (*model.ServiceRequest, error) {
	return nil, nil
}
func (f *fakeStore) ListServiceRequestEvents(ctx context.Context, schoolID, requestID string) ([]model.ServiceRequestEvent, error) {
	return nil, nil
}
func (f *fakeStore) CanUserHandleServiceRequests(ctx context.Context, schoolID string, roles []string) ([]string, error) {
	return nil, nil
}

func (f *fakeStore) CreateAuditGrant(ctx context.Context, g model.AuditAccessGrant) (*model.AuditAccessGrant, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	g.ID = fmt.Sprintf("00000000-0000-0000-0000-%012d", len(f.auditGrants)+1)
	g.CreatedAt = time.Now()
	f.auditGrants = append(f.auditGrants, g)
	return &g, nil
}

func (f *fakeStore) ListAuditGrants(ctx context.Context, schoolID string) ([]model.AuditAccessGrant, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []model.AuditAccessGrant
	for _, g := range f.auditGrants {
		if g.SchoolID == schoolID {
			out = append(out, g)
		}
	}
	return out, nil
}

func (f *fakeStore) RevokeAuditGrant(ctx context.Context, schoolID, id, revokedBy string) (*model.AuditAccessGrant, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i, g := range f.auditGrants {
		if g.ID == id && g.SchoolID == schoolID && g.RevokedAt == nil {
			now := time.Now()
			f.auditGrants[i].RevokedAt, f.auditGrants[i].RevokedBy = &now, &revokedBy
			out := f.auditGrants[i]
			return &out, nil
		}
	}
	return nil, store.ErrNotFound
}

func (f *fakeStore) HasActiveAuditGrant(ctx context.Context, schoolID, userID string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, g := range f.auditGrants {
		if g.SchoolID == schoolID && g.UserID == userID && g.Active(time.Now()) {
			return true, nil
		}
	}
	return false, nil
}

func (f *fakeStore) AuditFacets(_ context.Context, schoolID *string, since time.Time) (model.AuditFacets, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := model.AuditFacets{Services: []string{}, Modules: []string{}, Actions: []string{}}
	add := func(dst *[]string, v string) {
		if v != "" && !slices.Contains(*dst, v) {
			*dst = append(*dst, v)
		}
	}
	for _, a := range f.audits {
		if (schoolID != nil && (a.SchoolID == nil || *a.SchoolID != *schoolID)) || a.CreatedAt.Before(since) {
			continue
		}
		add(&out.Services, a.Service)
		add(&out.Modules, a.Module)
		add(&out.Actions, a.Action)
	}
	slices.Sort(out.Services)
	slices.Sort(out.Modules)
	slices.Sort(out.Actions)
	return out, nil
}

func (f *fakeStore) ListHelpArticles(_ context.Context, schoolID, audience string) ([]model.HelpArticle, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []model.HelpArticle
	for _, a := range f.helpArticles {
		if (a.SchoolID == nil || *a.SchoolID == schoolID) && a.Audience == audience {
			out = append(out, a)
		}
	}
	return out, nil
}

func (f *fakeStore) GetHelpArticle(_ context.Context, schoolID, id string) (*model.HelpArticle, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, a := range f.helpArticles {
		if a.ID == id && (a.SchoolID == nil || *a.SchoolID == schoolID) {
			return &a, nil
		}
	}
	return nil, store.ErrNotFound
}

func (f *fakeStore) CreateHelpArticle(_ context.Context, a model.HelpArticle, _ string) (*model.HelpArticle, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, x := range f.helpArticles {
		if a.ReplacesID != nil && x.ReplacesID != nil && *x.ReplacesID == *a.ReplacesID && *x.SchoolID == *a.SchoolID {
			return nil, store.ErrConflict
		}
	}
	a.ID = f.nextID("help")
	a.CreatedAt, a.UpdatedAt = time.Now(), time.Now()
	f.helpArticles = append(f.helpArticles, a)
	return &a, nil
}

func (f *fakeStore) UpdateHelpArticle(_ context.Context, schoolID, id string, a model.HelpArticle, _ string) (*model.HelpArticle, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i, x := range f.helpArticles {
		if x.ID == id && x.SchoolID != nil && *x.SchoolID == schoolID {
			x.Audience, x.Kind, x.Title, x.Body, x.SortOrder, x.Published = a.Audience, a.Kind, a.Title, a.Body, a.SortOrder, a.Published
			f.helpArticles[i] = x
			return &x, nil
		}
	}
	return nil, store.ErrNotFound
}

func (f *fakeStore) DeleteHelpArticle(_ context.Context, schoolID, id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i, x := range f.helpArticles {
		if x.ID == id && x.SchoolID != nil && *x.SchoolID == schoolID {
			f.helpArticles = append(f.helpArticles[:i], f.helpArticles[i+1:]...)
			return nil
		}
	}
	return store.ErrNotFound
}
