package model

import (
	"errors"
	"slices"
)

// ErrNoIndependentApprover: a step's only possible approver is the maker
// and no escalation role with another holder is configured.
var ErrNoIndependentApprover = errors.New("tidak ada penyetuju selain pembuat permintaan")

// hasOtherThan reports whether ids contains someone other than makerID.
func hasOtherThan(ids []string, makerID string) bool {
	return slices.ContainsFunc(ids, func(id string) bool { return id != "" && id != makerID })
}

// RerouteMakerOnlyStep keeps a step that someone other than the maker can
// decide; when the maker is its only approver (BR-GEN-09 would leave it stuck
// forever, BUG-111) the step is routed to its escalate_to_role, provided that
// role has a holder other than the maker. approvers are the users able to
// decide the step as configured; escalateApprovers the holders of
// escalate_to_role.
func RerouteMakerOnlyStep(st ApprovalStep, makerID string, approvers, escalateApprovers []string) (ApprovalStep, bool, error) {
	if hasOtherThan(approvers, makerID) {
		return st, false, nil
	}
	if st.EscalateToRole == nil || *st.EscalateToRole == "" || !hasOtherThan(escalateApprovers, makerID) {
		return st, false, ErrNoIndependentApprover
	}
	role := *st.EscalateToRole
	st.ApproverKind = ApproverKindRole
	st.ApproverRole = &role
	st.ApproverUserID = nil
	st.EscalateToRole = nil
	return st, true, nil
}
