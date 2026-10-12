package common

import (
	"crypto/rand"
	"encoding/json"
	"math/big"
	"net/http"
	"strconv"

	"golang.org/x/crypto/bcrypt"
)

// Meta is the pagination block that rides alongside `data` in list responses,
// per CONVENTIONS.md's paginated-list envelope shape.
type Meta struct {
	Page    int `json:"page"`
	PerPage int `json:"per_page"`
	Total   int `json:"total"`
}

// WriteValidationError writes the envelope shape with a populated `errors`
// object instead of the single {"message": msg} map WriteError uses — for
// per-field validation problems (e.g. dry-run import reports).
func WriteValidationError(w http.ResponseWriter, status int, msg string, fieldErrors interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(Envelope{Data: nil, Message: msg, Errors: fieldErrors})
}

// ParsePage reads ?page= and ?per_page= query params with sane defaults,
// used by every list endpoint per CONVENTIONS.md.
func ParsePage(r *http.Request) (page, perPage int) {
	page, _ = strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	perPage, _ = strconv.Atoi(r.URL.Query().Get("per_page"))
	if perPage < 1 {
		perPage = 20
	} else if perPage > 200 {
		perPage = 200
	}
	return page, perPage
}

func HashPassword(plain string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(plain), bcrypt.DefaultCost)
	return string(b), err
}

func CheckPassword(hash, plain string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(plain)) == nil
}

const passwordAlphabet = "ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnpqrstuvwxyz23456789"

// GenDefaultPassword generates a random default password (e.g. for newly
// imported/created accounts and admin-triggered resets).
func randomString(length int, alphabet string) string {
	b := make([]byte, length)
	for i := range b {
		n, _ := rand.Int(rand.Reader, big.NewInt(int64(len(alphabet))))
		b[i] = alphabet[n.Int64()]
	}
	return string(b)
}

func GenDefaultPassword() string {
	return randomString(12, passwordAlphabet)
}

// GenClaimCode creates a high-entropy human-readable code. Only its bcrypt
// hash is persisted; the plaintext is returned once by the issue endpoint.
func GenClaimCode() string {
	return randomString(16, "ABCDEFGHJKLMNPQRSTUVWXYZ23456789")
}

// System roles. The 4 dashboard roles have their own dashboard; the 3
// jabatan-only roles ride along as extra tags in the JWT `roles` claim and
// gate actions in other services (see CONVENTIONS.md).
const (
	RoleSuperadmin    = "superadmin"
	RoleAdmin         = "admin"
	RoleGuru          = "guru"
	RoleSiswa         = "siswa"
	RoleOrangTua      = "orang_tua"
	RoleWalikelas     = "walikelas"
	RoleKepalaSekolah = "kepala_sekolah"
	RoleKepalaYayasan = "kepala_yayasan"
	// RolePengawas (SEKOCI V11) is panitia ujian: it sets exam schedules,
	// watches the live monitor and can larang/blokir a peserta, and reads
	// the bank soal. It manages no users, no nilai and no settings, so it
	// is deliberately NOT a dashboard role -- the frontend lands it on
	// Jadwal Ujian instead (web/src/router/domains/exam.routes.ts).
	RolePengawas = "pengawas"

	// HR and finance roles (SEKOCI T1-T5, migration 0014)
	RoleHRAdmin        = "hr_admin"
	RoleHRManager      = "hr_manager"
	RolePayrollOfficer = "payroll_officer"
	RoleRecruiter      = "recruiter"
	RoleTrainingAdmin  = "training_admin"
	RoleAuditor        = "auditor"
	RoleFinanceManager = "finance_manager"
	RolePegawai        = "pegawai"

	// Operasional sekolah, PRD Bagian 3–4 (T9, migration 0017). Names are
	// the contract for T10–T26 (docs/prd-sekoci/tasks/T9 §2).
	RoleInventoryOfficer   = "inventory_officer"
	RoleProcurementOfficer = "procurement_officer"
	RoleAssetOfficer       = "asset_officer"
	RolePustakawan         = "pustakawan"
	RoleFacilityOfficer    = "facility_officer"
	RoleTeknisi            = "teknisi"
	RoleTransportOfficer   = "transport_officer"
	RolePendampingTrans    = "pendamping_transport"
	RolePengemudi          = "pengemudi"
	RoleCanteenAdmin       = "canteen_admin"
	RoleCanteenOperator    = "canteen_operator"
	RolePetugasUKS         = "petugas_uks"
	RoleKonselor           = "konselor"
	RoleKesiswaan          = "kesiswaan"
	RoleEventCoordinator   = "event_coordinator"
	RolePembinaEkskul      = "pembina_ekskul"
	RoleServiceDeskAgent   = "servicedesk_agent"
	RoleServiceDeskManager = "servicedesk_manager"
	RoleKoordinatorPKL     = "koordinator_pkl"
	RolePembimbingPKL      = "pembimbing_pkl"
	RolePembimbingIndustri = "pembimbing_industri"
	RoleQACoordinator      = "qa_coordinator"
	RoleOperatorDapodik    = "operator_dapodik"
	RoleHumas              = "humas"
	RoleBIAnalyst          = "bi_analyst"
)

// OpsRoles are the 25 Bagian 3–4 roles added by migration 0017.
var OpsRoles = []string{
	RoleInventoryOfficer, RoleProcurementOfficer, RoleAssetOfficer, RolePustakawan, RoleFacilityOfficer, RoleTeknisi,
	RoleTransportOfficer, RolePendampingTrans, RolePengemudi, RoleCanteenAdmin, RoleCanteenOperator, RolePetugasUKS,
	RoleKonselor, RoleKesiswaan, RoleEventCoordinator, RolePembinaEkskul, RoleServiceDeskAgent, RoleServiceDeskManager,
	RoleKoordinatorPKL, RolePembimbingPKL, RolePembimbingIndustri, RoleQACoordinator, RoleOperatorDapodik, RoleHumas,
	RoleBIAnalyst,
}

var AllRoles = append([]string{
	RoleSuperadmin, RoleAdmin, RoleGuru, RoleSiswa, RoleOrangTua, RoleWalikelas, RoleKepalaSekolah, RoleKepalaYayasan, RolePengawas,
	RoleHRAdmin, RoleHRManager, RolePayrollOfficer, RoleRecruiter, RoleTrainingAdmin, RoleAuditor, RoleFinanceManager, RolePegawai,
}, OpsRoles...)

var DashboardRoles = map[string]bool{
	RoleSuperadmin: true,
	RoleAdmin:      true,
	RoleGuru:       true,
	RoleSiswa:      true,
	RoleOrangTua:   true,
}

func IsValidRole(role string) bool {
	for _, r := range AllRoles {
		if r == role {
			return true
		}
	}
	return false
}

// JabatanOnlyRoleForPosition canonicalizes a free-text position/jabatan name
// to one of the 3 jabatan-only system roles, if it matches. Used to mirror a
// Data Jabatan assignment into user_school_roles so it shows up in the JWT
// `roles` claim (see README Assumptions).
func JabatanOnlyRoleForPosition(name string) (string, bool) {
	switch normalizeJabatan(name) {
	case "walikelas", "wali kelas":
		return RoleWalikelas, true
	case "kepala sekolah":
		return RoleKepalaSekolah, true
	case "kepala yayasan":
		return RoleKepalaYayasan, true
	}
	return "", false
}

func normalizeJabatan(name string) string {
	out := make([]byte, 0, len(name))
	lastSpace := false
	for _, r := range name {
		c := r
		if c >= 'A' && c <= 'Z' {
			c = c - 'A' + 'a'
		}
		if c == ' ' || c == '_' || c == '-' {
			if lastSpace || len(out) == 0 {
				continue
			}
			out = append(out, ' ')
			lastSpace = true
			continue
		}
		if c < 'a' || c > 'z' {
			continue
		}
		out = append(out, byte(c))
		lastSpace = false
	}
	for len(out) > 0 && out[len(out)-1] == ' ' {
		out = out[:len(out)-1]
	}
	return string(out)
}
