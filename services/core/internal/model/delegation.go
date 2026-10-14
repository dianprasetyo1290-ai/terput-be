package model

// SharedApproverRoles returns the approver roles both the delegator and the
// delegate hold in the school (FR-79-005: delegation only to a user with an
// equivalent role). A portal role (siswa, orang_tua) is never in
// approverRoles, so a student or parent can never receive authority.
func SharedApproverRoles(fromRoles, toRoles, approverRoles []string) []string {
	approver := make(map[string]bool, len(approverRoles))
	for _, r := range approverRoles {
		approver[r] = true
	}
	has := make(map[string]bool, len(toRoles))
	for _, r := range toRoles {
		has[r] = true
	}
	var shared []string
	seen := map[string]bool{}
	for _, r := range fromRoles {
		if approver[r] && has[r] && !seen[r] {
			seen[r] = true
			shared = append(shared, r)
		}
	}
	return shared
}
