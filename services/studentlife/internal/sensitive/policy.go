package sensitive

import (
	"encoding/json"
	"fmt"

	"skci/studentlife/internal/common"
)

// Module data sensitif yang dikenal (CHECK di migrasi 0001).
const (
	ModuleHealth     = "health"
	ModuleCounseling = "counseling"
)

// Modules lists every module accepted by sensitive_access_logs /
// confidential_access_policies.
var Modules = []string{ModuleHealth, ModuleCounseling}

// Level is how much of a sensitive record a viewer may see.
type Level int

const (
	None Level = iota
	AlertOnly
	StatusOnly
	Full
)

// OwnerRoles is the role that fully manages each module's records
// (BR-HLT-01: petugas UKS). T20 handles counseling per-case membership.
var OwnerRoles = map[string]string{
	ModuleHealth: common.RolePetugasUKS,
}

// Facts are relationship facts the handler fetched (identity/academic)
// before calling CanAccess; CanAccess itself does no I/O.
type Facts struct {
	IsGuardian       bool // verified guardian of the subject student
	IsHomeroom       bool // wali kelas of the subject student's classroom
	IsCaseMember     bool // active member of the counseling case / session counselor
	HasDisclosedCase bool // open/monitoring case with disclose_to_homeroom=true (AC-130-01)
}

// CanAccess applies BR-HLT-01, BR-CNS-01 & OQ-31.
// For health: petugas_uks & guardian -> Full; policy roles -> Full; homeroom -> AlertOnly.
// For counseling (T20, AC-130-01): case member -> Full; policy roles -> Full;
// homeroom teacher -> StatusOnly if HasDisclosedCase=true; others -> None.
func CanAccess(module, viewerRole string, f Facts, policyRoles []string) Level {
	for _, r := range policyRoles {
		if r == viewerRole {
			return Full
		}
	}
	if module == ModuleHealth {
		if owner, ok := OwnerRoles[module]; ok && viewerRole == owner {
			return Full
		}
		if viewerRole == common.RoleParent && f.IsGuardian {
			return Full
		}
		if viewerRole == common.RoleWalikelas && f.IsHomeroom {
			return AlertOnly
		}
	}
	if module == ModuleCounseling {
		if f.IsCaseMember {
			return Full
		}
		if viewerRole == common.RoleWalikelas && f.IsHomeroom && f.HasDisclosedCase {
			return StatusOnly
		}
	}
	return None
}

// AAD binds a ciphertext to one row: "<table>:<school_id>:<row id>".
func AAD(table, schoolID, rowID string) string {
	return fmt.Sprintf("%s:%s:%s", table, schoolID, rowID)
}

// SealJSON marshals v and encrypts it for one row. A nil v seals to nil.
func (s *Service) SealJSON(v any, aad string) ([]byte, error) {
	if v == nil {
		return nil, nil
	}
	raw, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	return s.Encrypt(string(raw), aad)
}

// OpenJSON decrypts and unmarshals a SealJSON payload into dst. Empty
// ciphertext leaves dst untouched.
func (s *Service) OpenJSON(raw []byte, aad string, dst any) error {
	if len(raw) == 0 {
		return nil
	}
	pt, err := s.Decrypt(raw, aad)
	if err != nil {
		return err
	}
	return json.Unmarshal([]byte(pt), dst)
}
