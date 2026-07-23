// Package client holds best-effort internal HTTP clients to sibling services
// (identity, academic). Calls are defensive (short timeout, fail soft) so
// evaluation-service still works standalone for local dev/testing even when
// a sibling isn't up.
//
// Reconciled 2026-07-18 against the real, running services (the original
// URLs here were pre-integration guesses at a nonexistent "/internal/"
// prefix — there is no such prefix; identity/academic mount their
// internal-consumable routes at the same paths browsers use, just behind
// InternalOrJWTMiddleware, see docs/CONVENTIONS.md):
//
//	GET  identity:  /users/{id}                    -> {id, name, ...} (nuptk not yet a field, NUPTK stays "-")
//	GET  identity:  /users?role=guru&school_id=...  -> list of guru accounts
//	GET  identity:  /users?jabatan=...&school_id=... -> NOT SUPPORTED by identity yet (no jabatan filter on
//	                                                     /users); GetUsersByJabatan is unused dead code as of
//	                                                     this writing — the actual Kepala Sekolah/Waka
//	                                                     dropdowns in the frontend resolve via identity's
//	                                                     /organization instead, see FRONTEND_CONVENTIONS.md.
//	GET  identity:  /academic-years?school_id=...   -> list of academic years (identity owns this table, not academic)
//	GET  academic:  /classrooms/{id}/students        -> enrollment rows ({student_user_id}, no name — enriched via GetUser)
//	GET  academic:  /teaching-assignments?guru_id=...  -> classroom+subject pairs a guru teaches (implemented
//	                                                       2026-07-18; previously missing, so Kelas yang Diampu/
//	                                                       Mata Pelajaran were always blank on the printed report).
//
// All internal calls carry header X-Internal-Key per CONVENTIONS.md.
package client

import (
	"encoding/json"
	"net/http"
	"time"
)

type Clients struct {
	IdentityBaseURL string
	AcademicBaseURL string
	InternalKey     string
	http            *http.Client
}

func New(identityBaseURL, academicBaseURL, internalKey string) *Clients {
	return &Clients{
		IdentityBaseURL: identityBaseURL,
		AcademicBaseURL: academicBaseURL,
		InternalKey:     internalKey,
		http:            &http.Client{Timeout: 2 * time.Second},
	}
}

type UserSummary struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	NUPTK string `json:"nuptk,omitempty"`
}

func (c *Clients) doGet(url string, out interface{}) error {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("X-Internal-Key", c.InternalKey)
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return errNonOK(resp.StatusCode)
	}
	var env struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		return err
	}
	return json.Unmarshal(env.Data, out)
}

type httpStatusErr int

func (e httpStatusErr) Error() string { return "internal call: non-200 status" }
func errNonOK(code int) error         { return httpStatusErr(code) }

// GetUser fetches a single user's summary (name, nuptk) from identity-service.
// Best-effort: returns a placeholder UserSummary if identity is unreachable.
func (c *Clients) GetUser(id string) UserSummary {
	var u UserSummary
	url := c.IdentityBaseURL + "/users/" + id
	if err := c.doGet(url, &u); err != nil || u.ID == "" {
		return UserSummary{ID: id, Name: "(unknown - identity-service unreachable)", NUPTK: "-"}
	}
	return u
}

// GetGuruList fetches all guru accounts for a school from identity-service.
func (c *Clients) GetGuruList(schoolID string) []UserSummary {
	var users []UserSummary
	url := c.IdentityBaseURL + "/users?role=guru&school_id=" + schoolID
	if err := c.doGet(url, &users); err != nil {
		return nil
	}
	return users
}

// GetUsersByJabatan fetches users tagged with a given jabatan (e.g. kepala_sekolah).
func (c *Clients) GetUsersByJabatan(schoolID, jabatan string) []UserSummary {
	var users []UserSummary
	url := c.IdentityBaseURL + "/users?jabatan=" + jabatan + "&school_id=" + schoolID
	if err := c.doGet(url, &users); err != nil {
		return nil
	}
	return users
}

type ClassroomStudent struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// classEnrollment mirrors academic-service's actual /classrooms/{id}/students
// response (student_user_id, no name — academic-service doesn't own student
// display names, identity-service does). GetClassroomStudents enriches each
// enrollment with a name via identity's GetUser.
type classEnrollment struct {
	StudentUserID string `json:"student_user_id"`
}

// GetClassroomStudents fetches the student roster of a classroom from
// academic-service, enriched with names from identity-service.
func (c *Clients) GetClassroomStudents(classroomID string) []ClassroomStudent {
	var enrollments []classEnrollment
	url := c.AcademicBaseURL + "/classrooms/" + classroomID + "/students"
	if err := c.doGet(url, &enrollments); err != nil {
		return nil
	}
	students := make([]ClassroomStudent, 0, len(enrollments))
	for _, e := range enrollments {
		students = append(students, ClassroomStudent{ID: e.StudentUserID, Name: c.GetUser(e.StudentUserID).Name})
	}
	return students
}

type TeachingAssignment struct {
	ClassroomID   string `json:"classroom_id"`
	ClassroomName string `json:"classroom_name"`
	SubjectName   string `json:"subject_name"`
}

// GetTeachingAssignments fetches the classrooms/subjects a guru teaches, from academic-service.
func (c *Clients) GetTeachingAssignments(guruID string) []TeachingAssignment {
	var out []TeachingAssignment
	url := c.AcademicBaseURL + "/teaching-assignments?guru_id=" + guruID
	if err := c.doGet(url, &out); err != nil {
		return nil
	}
	return out
}

type AcademicYear struct {
	ID      string `json:"id"`
	Label   string `json:"label"`
	Current bool   `json:"is_current"`
}

// GetAcademicYears fetches the list of academic years for a school, from academic-service.
func (c *Clients) GetAcademicYears(schoolID string) []AcademicYear {
	var out []AcademicYear
	url := c.IdentityBaseURL + "/academic-years?school_id=" + schoolID
	if err := c.doGet(url, &out); err != nil {
		return nil
	}
	return out
}

type SchoolSummary struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// GetSchool fetches a school's name from identity-service, for the printed
// Rapor Kinerja Guru title ("... — Nama Sekolah").
func (c *Clients) GetSchool(id string) SchoolSummary {
	var s SchoolSummary
	url := c.IdentityBaseURL + "/schools/" + id
	if err := c.doGet(url, &s); err != nil || s.ID == "" {
		return SchoolSummary{ID: id, Name: "(Nama Sekolah)"}
	}
	return s
}
