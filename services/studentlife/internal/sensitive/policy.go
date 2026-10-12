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
	Full
)

// OwnerRoles is the role that fully manages each module's records
// (BR-HLT-01: petugas UKS). T20 adds the counseling owner here.
var OwnerRoles = map[string]string{
	ModuleHealth: common.RolePetugasUKS,
}

// Facts are relationship facts the handler fetched (identity/academic)
// before calling CanAccess; CanAccess itself does no I/O.
type Facts struct {
	IsGuardian bool // verified guardian of the subject student
	IsHomeroom bool // wali kelas of the subject student's classroom
}

// CanAccess applies BR-HLT-01 / OQ-31. For the health module: the owner role
// (petugas_uks) and verified guardians see everything; a role with an active
// confidential_access_policies row sees everything; the homeroom teacher
// sees only medical alerts + emergency contacts; anyone else (kepala_sekolah
// without a policy, admin, guru, superadmin) sees nothing.
func CanAccess(module, viewerRole string, f Facts, policyRoles []string) Level {
	if owner, ok := OwnerRoles[module]; ok && viewerRole == owner {
		return Full
	}
	for _, r := range policyRoles {
		if r == viewerRole {
			return Full
		}
	}
	if module == ModuleHealth {
		if viewerRole == common.RoleParent && f.IsGuardian {
			return Full
		}
		if viewerRole == common.RoleWalikelas && f.IsHomeroom {
			return AlertOnly
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
