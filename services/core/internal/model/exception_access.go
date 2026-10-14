package model

import (
	"slices"

	"skci/core/internal/common"
)

// exceptionKindsByRole is the FR-80-005 matrix: which exception kinds each
// role sees. admin is the school's Finance Admin / unit admin (payments,
// notifications, imports, callbacks) and does NOT see journal postings, which
// belong to Accounting (finance_manager; payroll_officer for hr postings).
// auditor reads everything but acts on nothing.
var exceptionKindsByRole = map[string][]string{
	common.RoleSuperadmin: ExceptionKinds,
	common.RoleAuditor:    ExceptionKinds,
	common.RoleAdmin: {ExceptionKindWebhook, ExceptionKindNotification, ExceptionKindDeadLetter,
		ExceptionKindImport, ExceptionKindCallback},
	common.RoleFinanceManager: {ExceptionKindWebhook, ExceptionKindJournalPosting},
	common.RolePayrollOfficer: {ExceptionKindJournalPosting},
}

// ExceptionKindsFor returns the kinds a role may see (nil = no access).
func ExceptionKindsFor(role string) []string { return exceptionKindsByRole[role] }

// ExceptionResponsibleRoles are the roles notified when a critical item of a
// kind appears (FR-80-006): everyone who may act on it, auditors excluded.
// Sorted for a stable fan-out order.
func ExceptionResponsibleRoles(kind string) []string {
	var roles []string
	for role, kinds := range exceptionKindsByRole {
		if role != common.RoleAuditor && slices.Contains(kinds, kind) {
			roles = append(roles, role)
		}
	}
	slices.Sort(roles)
	return roles
}
