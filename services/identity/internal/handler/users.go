package handler

import (
	"database/sql"
	"database/sql/driver"
	"encoding/csv"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/xuri/excelize/v2"

	"skci/identity/internal/common"
	"skci/identity/internal/model"
	"skci/identity/internal/rules"
	"skci/identity/internal/security"
	"skci/identity/internal/store"
	"skci/identity/internal/userimport"
)

// Data Pengguna: superadmin-scoped user directory (list/filter by role and
// optionally by school), used as-is by superadmin and re-mounted with the
// school_id forced from JWT claims for admin (see router in cmd/server).
//
// Data Siswa / Data Guru reuse this same query (role=siswa / role=guru) —
// see ListStudents/ListTeachers below — and PUT /users/{id} +
// POST /users/{id}/reset-password cover "admin can edit account data" /
// "admin can reset a student's password" for both, rather than duplicating
// separate edit/reset handlers per entity (see README Assumptions).

const userCols = `u.id, u.name, u.email, u.phone, u.default_password, u.password_changed, u.photo, u.birth_date, u.birth_place, u.address, u.religion, u.nisn, u.is_superadmin, u.created_at, u.updated_at, u.managed_by_hr, u.deactivate_at, u.nik`

func scanUser(row interface{ Scan(...interface{}) error }) (model.User, error) {
	var u model.User
	err := row.Scan(&u.ID, &u.Name, &u.Email, &u.Phone, &u.DefaultPassword, &u.PasswordChanged, &u.Photo, &u.BirthDate, &u.BirthPlace, &u.Address, &u.Religion, &u.NISN, &u.IsSuperadmin, &u.CreatedAt, &u.UpdatedAt, &u.ManagedByHR, &u.DeactivateAt, &u.NIK)
	if err == nil && u.PasswordChanged {
		u.DefaultPassword = nil // hide once the user has changed it from the default
	}
	return u, err
}

// ListUsers implements Data Pengguna. role filter is optional; school_id
// filter is optional ONLY for superadmin (resolveSchoolScope enforces the
// multi-tenancy rule: non-superadmin is always forced to their own
// school_id, superadmin may omit it to see all schools).
//
// Non-admin exception (added after a real guru-role 403 was caught via
// browser testing, 2026-07-18): any authenticated user may browse the guru
// directory (role=guru only, not siswa/admin lists) — evaluation-service's
// peer-evaluator picker and similar "pick a colleague" UI need this, and a
// list of guru names isn't sensitive the way siswa data or account/security
// fields are. listUsersByRole strips default_password/phone/address/
// birth info from the response in that case — see directoryOnly there.
//
// walikelas exception (Bug Report SEKOCI GPM V10, ROLE WALI KELAS: found
// live via browser testing 2026-08-19 -- Kenaikan Kelas and Matriks
// Pelajaran both name-lookup their own classroom's roster against
// listUsersByRole('siswa'), which 403'd for active_role=="walikelas" since
// only guru had this carve-out, leaving both pages permanently stuck on
// "Gagal memuat data..."). Same directoryOnly tradeoff as guru above --
// a walikelas already gets the full siswa roster of the school via any
// enrollment/promotion endpoint anyway, this only adds names.
func (a *App) ListUsers(w http.ResponseWriter, r *http.Request) {
	claims := common.FromCtx(r)
	role := r.URL.Query().Get("role")
	// A trusted sibling-service call (X-Internal-Key, no claims -- see
	// resolveSchoolScope's identical check) is privileged the same as
	// admin/superadmin: this route is only reachable via X-Internal-Key on
	// the internal-or-jwt group in main.go in the first place, so anything
	// that got here without claims already passed that gate. Missed
	// originally -- CountUsersByRole-style internal counts 401'd with
	// "missing token" even though IsInternal(r) was already true.
	privileged := common.IsInternal(r) || (claims != nil && (claims.ActiveRole == common.RoleAdmin || claims.ActiveRole == common.RoleSuperadmin))
	if !privileged {
		if claims == nil {
			common.WriteError(w, http.StatusUnauthorized, "Sesi tidak ditemukan. Silakan masuk kembali")
			return
		}
		// kepala_sekolah joins walikelas here: the principal's school dashboard
		// and roster views resolve names off this same siswa directory.
		directoryAllowed := role == common.RoleGuru || (role == common.RoleSiswa && (claims.ActiveRole == common.RoleWalikelas || claims.ActiveRole == common.RoleKepalaSekolah))
		if !directoryAllowed {
			common.WriteError(w, http.StatusForbidden, "Akses ditolak: hanya untuk admin")
			return
		}
	}
	a.listUsersByRole(w, r, role, !privileged)
}

// ListStudents backs Data Siswa (admin/superadmin, full fields) and also
// the guru daily attendance roster (name lookup only, directoryOnly=true —
// same non-sensitive-fields tradeoff as the guru directory exception in
// ListUsers above; a guru taking presensi for their own classroom needs
// student names, which /classrooms/{id}/students in academic-service alone
// can't supply since it only returns bare student_user_id).
func (a *App) ListStudents(w http.ResponseWriter, r *http.Request) {
	claims := common.FromCtx(r)
	privileged := claims != nil && (claims.ActiveRole == common.RoleAdmin || claims.ActiveRole == common.RoleSuperadmin)
	// walikelas (Presensi Siswa) needs the same directory-only list as guru,
	// and kepala_sekolah's school-wide dashboard/presensi summary does too.
	if !privileged && (claims == nil || (claims.ActiveRole != common.RoleGuru && claims.ActiveRole != common.RoleWalikelas && claims.ActiveRole != common.RoleKepalaSekolah)) {
		common.WriteError(w, http.StatusForbidden, "Akses ditolak: hanya untuk admin")
		return
	}
	a.listUsersByRole(w, r, common.RoleSiswa, !privileged)
}

// ListTeachers mirrors ListStudents' admin-or-guru pattern (Bug Report
// SEKOCI GPM, ROLE GURU "Assign Pengganti"): a guru picking a substitute
// needs this same list, so it can't stay admin-only. Previously hard-gated
// to RoleAdmin — every guru's own JWT got 403'd here, the frontend's
// unhandled promise left the substitute-teacher dropdown permanently
// empty, and the "Assign Pengganti" button (disabled while empty) looked
// like a dead click with no popup. Non-admin callers get directoryOnly=true
// same as ListStudents, so guru still can't see other teachers' default
// password / phone / address / etc.
func (a *App) ListTeachers(w http.ResponseWriter, r *http.Request) {
	claims := common.FromCtx(r)
	privileged := claims != nil && (claims.ActiveRole == common.RoleAdmin || claims.ActiveRole == common.RoleSuperadmin)
	// kepala_sekolah reads the teacher directory for the Presensi Guru view.
	if !privileged && (claims == nil || (claims.ActiveRole != common.RoleGuru && claims.ActiveRole != common.RoleKepalaSekolah)) {
		common.WriteError(w, http.StatusForbidden, "Akses ditolak: hanya untuk admin")
		return
	}
	a.listUsersByRole(w, r, common.RoleGuru, !privileged)
}

// listUsersByRole executes the shared Data Pengguna/Siswa/Guru query.
// directoryOnly strips account/security fields from the response — see
// ListUsers' doc comment for when that's set.
func (a *App) listUsersByRole(w http.ResponseWriter, r *http.Request, role string, directoryOnly bool) {
	schoolID, filtered := resolveSchoolScope(r)
	status := r.URL.Query().Get("status") // aktif/nonaktif filter for guru, optional
	q := strings.TrimSpace(r.URL.Query().Get("q"))

	where := `u.deleted_at IS NULL`
	args := []interface{}{}
	joinRole := role != "" || filtered || status != ""

	if joinRole {
		where += ` AND EXISTS (SELECT 1 FROM user_school_roles usr WHERE usr.user_id = u.id`
		if role != "" {
			args = append(args, role)
			where += ` AND usr.role = $` + placeholder(len(args))
		}
		if filtered {
			args = append(args, schoolID)
			where += ` AND usr.school_id = $` + placeholder(len(args))
		}
		if status != "" {
			args = append(args, status)
			where += ` AND usr.status = $` + placeholder(len(args))
		}
		where += `)`
	}
	// Search must run inside the same tenant-scoped WHERE above, not as a client-side filter
	// over one page — a query for an existing user must find them regardless of which page
	// they'd otherwise sort onto (name/email match, case-insensitive).
	if q != "" {
		args = append(args, "%"+q+"%")
		where += ` AND (u.name ILIKE $` + placeholder(len(args)) + ` OR u.email ILIKE $` + placeholder(len(args)) + `)`
	}
	// Data Siswa's "Filter Kelas" (classroom_id lives in academic-service, not here) resolves
	// a roster of student user ids client-side, then filters/paginates against that id set
	// server-side -- fixes the row-number-vs-page mismatch and "students beyond page 1
	// invisible" bugs a purely client-side post-filter over one page had (GPM School bug
	// report 2026-08-28: siswa order scrambled across classroom-filter switches).
	//
	// A "Belum ada kelas"/roster-exclude id set is the union of every classroom's roster
	// for the year, so it scales with enrolled-student count rather than one classroom's
	// -- a 220-siswa school already produced an 8KB comma list, and nginx's request-line
	// limit rejects it with 414 before this handler ever runs, surfacing as a blank Data
	// Siswa table / "Gagal memuat data pengguna." (and the same call from Detail Kelas's
	// siswa-picker as "Gagal memuat data kelas."). POST carries the same ids/exclude_ids
	// as a JSON body instead of a query string, off the URL entirely.
	idsParam := strings.TrimSpace(r.URL.Query().Get("ids"))
	excludeParam := strings.TrimSpace(r.URL.Query().Get("exclude_ids"))
	if r.Method == http.MethodPost {
		var body struct {
			IDs        string `json:"ids"`
			ExcludeIDs string `json:"exclude_ids"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err == nil {
			if body.IDs != "" {
				idsParam = body.IDs
			}
			if body.ExcludeIDs != "" {
				excludeParam = body.ExcludeIDs
			}
		}
	}
	if idsParam != "" {
		args = append(args, pqStringArray(strings.Split(idsParam, ",")))
		where += ` AND u.id = ANY($` + placeholder(len(args)) + `::uuid[])`
	}
	if excludeParam != "" {
		args = append(args, pqStringArray(strings.Split(excludeParam, ",")))
		where += ` AND u.id != ALL($` + placeholder(len(args)) + `::uuid[])`
	}

	var total int
	if err := a.DB.QueryRow(`SELECT count(*) FROM users u WHERE `+where, args...).Scan(&total); err != nil {
		common.WriteDBError(w, err, "Gagal memuat daftar pengguna")
		return
	}

	page, perPage := common.ParsePage(r)
	args2 := append(append([]interface{}{}, args...), perPage, (page-1)*perPage)
	// ORDER BY u.name alone ties non-deterministically across separate query
	// executions whenever two users share a name (common with Indonesian
	// names) -- OFFSET pagination then silently skips or repeats rows across
	// the page boundary depending on which order Postgres happens to return
	// tied rows in that run. Root-caused via "generate token dari nomer 201
	// gamau ke-generate": bulk-issue tokens pages through this endpoint
	// assuming it gets every student exactly once. u.id is unique, so it's a
	// safe deterministic tiebreaker.
	query := `SELECT ` + userCols + ` FROM users u WHERE ` + where + ` ORDER BY u.name, u.id LIMIT $` + placeholder(len(args2)-1) + ` OFFSET $` + placeholder(len(args2))

	rows, err := a.DB.Query(query, args2...)
	if err != nil {
		common.WriteDBError(w, err, "Gagal memuat daftar pengguna")
		return
	}
	defer rows.Close()

	out := []model.UserWithRoles{}
	ids := []string{}
	byID := map[string]*model.UserWithRoles{}
	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			common.WriteError(w, http.StatusInternalServerError, "Gagal memuat daftar pengguna")
			return
		}
		uwr := model.UserWithRoles{User: u, Roles: []model.UserSchoolRole{}}
		out = append(out, uwr)
		ids = append(ids, u.ID)
	}
	rows.Close()
	for i := range out {
		byID[out[i].ID] = &out[i]
	}
	if len(ids) > 0 {
		roleRows, err := a.DB.Query(`SELECT id, user_id, school_id, role, status, academic_year_id, student_status FROM user_school_roles WHERE user_id = ANY($1::uuid[])`, pqStringArray(ids))
		if err == nil {
			defer roleRows.Close()
			for roleRows.Next() {
				var ro model.UserSchoolRole
				if err := roleRows.Scan(&ro.ID, &ro.UserID, &ro.SchoolID, &ro.Role, &ro.Status, &ro.AcademicYearID, &ro.StudentStatus); err == nil {
					if u, ok := byID[ro.UserID]; ok {
						u.Roles = append(u.Roles, ro)
					}
				}
			}
		}
	}

	if directoryOnly {
		for i := range out {
			out[i].DefaultPassword = nil
			out[i].Phone = nil
			out[i].Address = nil
			out[i].BirthDate = nil
			out[i].BirthPlace = nil
			out[i].Religion = nil
			out[i].NISN = nil
			out[i].NIK = nil
			out[i].Photo = nil
			out[i].Roles = []model.UserSchoolRole{}
		}
	}

	common.WriteJSON(w, http.StatusOK, out, common.Meta{Page: page, PerPage: perPage, Total: total})
}

type createUserRequest struct {
	Name     string  `json:"name"`
	Email    string  `json:"email"`
	Phone    *string `json:"phone"`
	Password string  `json:"password"` // optional; auto-generated default password if omitted
	Role     string  `json:"role"`
	SchoolID string  `json:"school_id"` // forced to caller's school unless superadmin
}

// adminAssignable lists the roles a plain admin may hand out. Every role
// here is non-escalating: none can create users or grant roles, so an admin
// cannot reach admin/superadmin through them. Pengawas (V11, panitia ujian)
// qualifies -- it only touches exam scheduling, the live monitor and a
// read-only bank soal.
func adminAssignable(role string) bool {
	switch role {
	case common.RoleGuru, common.RoleSiswa, common.RoleOrangTua, common.RolePengawas:
		return true
	}
	return false
}

func (a *App) CreateUser(w http.ResponseWriter, r *http.Request) {
	if !requireRole(w, r, common.RoleAdmin) {
		return
	}
	claims := common.FromCtx(r)
	var req createUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Name == "" || req.Email == "" || req.Role == "" {
		common.WriteError(w, http.StatusBadRequest, "Nama, email, dan peran wajib diisi")
		return
	}
	req.Email = strings.TrimSpace(req.Email)
	if !rules.ValidEmail(req.Email) { // BUG-084
		common.WriteValidationError(w, http.StatusBadRequest, "Format email tidak valid", map[string]string{"email": "Format email tidak valid"})
		return
	}
	if req.Password != "" {
		if msg := security.ValidPassword(req.Password); msg != "" {
			common.WriteValidationError(w, http.StatusBadRequest, msg, map[string]string{"password": msg})
			return
		}
	}
	if !common.IsValidRole(req.Role) {
		common.WriteError(w, http.StatusBadRequest, "Peran tidak valid")
		return
	}
	// Privilege-escalation guard: an admin (as opposed to superadmin) may only
	// ever create guru/siswa accounts here — never another admin or a
	// superadmin. The frontend already hides those options from the role
	// picker when active_role=admin, but that's UI-only; enforce it
	// server-side too since this is a real escalation path otherwise.
	if claims.ActiveRole == common.RoleAdmin && !adminAssignable(req.Role) {
		common.WriteError(w, http.StatusForbidden, "Admin unit hanya dapat membuat akun guru, siswa, orang tua, atau pengawas")
		return
	}
	schoolID := claims.SchoolID
	if claims.ActiveRole == common.RoleSuperadmin && req.SchoolID != "" {
		schoolID = req.SchoolID
	}
	if req.Role != common.RoleSuperadmin && schoolID == "" {
		common.WriteError(w, http.StatusBadRequest, "Unit wajib dipilih untuk peran selain superadmin")
		return
	}

	plainPassword := req.Password
	if plainPassword == "" {
		plainPassword = common.GenDefaultPassword()
	}
	hash, err := common.HashPassword(plainPassword)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal membuat pengguna")
		return
	}

	tx, err := a.DB.Begin()
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal membuat pengguna")
		return
	}
	defer tx.Rollback()

	var u model.User
	row := tx.QueryRow(`INSERT INTO users AS u (name, email, phone, password_hash, default_password, password_changed, is_superadmin)
		VALUES ($1,$2,$3,$4,NULL,false,$5) RETURNING `+userCols,
		req.Name, req.Email, req.Phone, hash, req.Role == common.RoleSuperadmin)
	u, err = scanUser(row)
	if err != nil {
		common.WriteDBError(w, err, "Gagal membuat pengguna (email mungkin sudah dipakai)")
		return
	}
	var schoolIDArg interface{}
	if schoolID != "" {
		schoolIDArg = schoolID
	}
	if _, err := tx.Exec(`INSERT INTO user_school_roles (user_id, school_id, role, student_status)
		VALUES ($1,$2,$3, CASE WHEN $3 = 'siswa' THEN 'aktif' END)`, u.ID, schoolIDArg, req.Role); err != nil {
		common.WriteDBError(w, err, "Gagal menetapkan peran")
		return
	}
	if err := tx.Commit(); err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal membuat pengguna")
		return
	}
	u.DefaultPassword = &plainPassword // response-only; never persisted
	common.WriteJSON(w, http.StatusCreated, u, nil)
}

func (a *App) GetUser(w http.ResponseWriter, r *http.Request) {
	if !requireRole(w, r, common.RoleAdmin) {
		return
	}
	id := chi.URLParam(r, "id")
	row := a.DB.QueryRow(`SELECT `+userCols+` FROM users u WHERE u.id=$1 AND u.deleted_at IS NULL`, id)
	u, err := scanUser(row)
	if err == sql.ErrNoRows {
		common.WriteError(w, http.StatusNotFound, "Pengguna tidak ditemukan")
		return
	} else if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal memuat pengguna")
		return
	}
	roles, _ := aRolesFor(a, id)
	common.WriteJSON(w, http.StatusOK, model.UserWithRoles{User: u, Roles: roles}, nil)
}

func aRolesFor(a *App, userID string) ([]model.UserSchoolRole, error) {
	rows, err := a.DB.Query(`SELECT id, user_id, school_id, role, status, academic_year_id, student_status FROM user_school_roles WHERE user_id=$1`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.UserSchoolRole{}
	for rows.Next() {
		var ro model.UserSchoolRole
		if err := rows.Scan(&ro.ID, &ro.UserID, &ro.SchoolID, &ro.Role, &ro.Status, &ro.AcademicYearID, &ro.StudentStatus); err != nil {
			return nil, err
		}
		out = append(out, ro)
	}
	return out, nil
}

type updateUserRequest struct {
	Name       string  `json:"name"`
	Phone      *string `json:"phone"`
	Photo      *string `json:"photo"`
	BirthDate  *string `json:"birth_date"`
	BirthPlace *string `json:"birth_place"`
	Address    *string `json:"address"`
	Religion   *string `json:"religion"`
	NISN       *string `json:"nisn"`
	// NIK (migration 0020): omitted keeps the stored value, "" clears it.
	NIK *string `json:"nik"`
	// ConfirmDuplicate saves despite a possible-duplicate warning (BUG-016).
	ConfirmDuplicate bool `json:"confirm_duplicate"`
}

func trimPtr(p *string) *string {
	if p == nil {
		return nil
	}
	v := strings.TrimSpace(*p)
	return &v
}

func deref(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

// UpdateUser: "admin can edit account data" for Data Siswa / Data Guru /
// Data Pengguna alike.
func (a *App) UpdateUser(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if !a.requireUserInScope(w, r, id) {
		return
	}
	var req updateUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Name == "" {
		common.WriteError(w, http.StatusBadRequest, "Nama wajib diisi")
		return
	}
	// AC-83-02: while HRIS is on for one of the user's schools, the name of an
	// HR-managed account is owned by the hr service, not Data Guru/Pengguna.
	var hrLocked bool
	var currentName, curNISN, curNIK, curPhone, curReligion string
	err := a.DB.QueryRow(`SELECT u.managed_by_hr AND EXISTS (
			SELECT 1 FROM user_school_roles usr
			JOIN school_features sf ON sf.school_id = usr.school_id AND sf.feature_key = 'hris' AND sf.enabled
			WHERE usr.user_id = u.id), u.name, COALESCE(u.nisn,''), COALESCE(u.nik,''), COALESCE(u.phone,''), COALESCE(u.religion,'')
		FROM users u WHERE u.id = $1 AND u.deleted_at IS NULL`, id).Scan(&hrLocked, &currentName, &curNISN, &curNIK, &curPhone, &curReligion)
	if err == sql.ErrNoRows {
		common.WriteError(w, http.StatusNotFound, "Pengguna tidak ditemukan")
		return
	} else if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal memperbarui pengguna")
		return
	}
	if hrLocked && strings.TrimSpace(req.Name) != currentName {
		common.WriteError(w, http.StatusConflict, "Data dikelola dari HRIS")
		return
	}
	req.NISN, req.NIK = trimPtr(req.NISN), trimPtr(req.NIK)
	if errs := rules.ValidateIdentityNumbers(req.NISN, req.NIK); len(errs) > 0 { // BUG-074
		msg := errs["nisn"]
		if msg == "" {
			msg = errs["nik"]
		}
		common.WriteValidationError(w, http.StatusBadRequest, msg, errs)
		return
	}
	// BUG-073: a changed religion must come from the Data Referensi list
	// (stored as its label, which academic matches to Agama subjects).
	if rel := strings.TrimSpace(deref(req.Religion)); rel != "" && rel != curReligion {
		label, ok, err := a.referenceLabel(r.Context(), "agama", rel, common.FromCtx(r).SchoolID)
		if err != nil {
			common.WriteDBError(w, err, "Gagal memeriksa data referensi")
			return
		}
		if !ok {
			common.WriteValidationError(w, http.StatusBadRequest, "Agama tidak ada di Data Referensi", map[string]string{"religion": "Pilih agama dari daftar"})
			return
		}
		req.Religion = &label
	}
	// Only values that change are checked, so editing an unrelated field of
	// an existing record never trips over a known shared number.
	newIfChanged := func(v, cur string) string {
		if v == cur {
			return ""
		}
		return v
	}
	nikCheck := ""
	if req.NIK != nil {
		nikCheck = newIfChanged(*req.NIK, curNIK)
	}
	phoneCheck := ""
	if rules.NormalizePhone(deref(req.Phone)) != rules.NormalizePhone(curPhone) {
		phoneCheck = rules.NormalizePhone(deref(req.Phone))
	}
	if !req.ConfirmDuplicate && !a.checkDuplicates(w, r, id, newIfChanged(deref(req.NISN), curNISN), nikCheck, phoneCheck) {
		return
	}
	row := a.DB.QueryRow(`UPDATE users AS u SET name=$1, phone=$2, photo=$3, birth_date=$4, birth_place=$5, address=$6, religion=$7, nisn=NULLIF($8,''),
			nik = CASE WHEN $10::text IS NULL THEN nik ELSE NULLIF($10,'') END, updated_at=now()
		WHERE id=$9 AND deleted_at IS NULL RETURNING `+userCols,
		req.Name, req.Phone, req.Photo, req.BirthDate, req.BirthPlace, req.Address, req.Religion, req.NISN, id, req.NIK)
	u, err := scanUser(row)
	if err == sql.ErrNoRows {
		common.WriteError(w, http.StatusNotFound, "Pengguna tidak ditemukan")
		return
	} else if err != nil {
		common.WriteDBError(w, err, "Gagal memperbarui pengguna")
		return
	}
	if req.ConfirmDuplicate {
		claims := common.FromCtx(r)
		a.RecordSecurityAudit(nil, "duplicate_warning_overridden", id, &claims.UserID, &claims.ActiveRole, nil,
			map[string]any{"nisn": deref(req.NISN), "nik_set": deref(req.NIK) != ""}, "saved despite duplicate warning", r.Header.Get("X-Correlation-ID"))
	}
	common.WriteJSON(w, http.StatusOK, u, nil)
}

// DeleteUser: soft-delete (mirrors DeleteSchool's pattern in schools.go) —
// sets deleted_at rather than removing the row, so history (e.g. exam
// attempts, evaluation records) referencing this user_id stays intact.
//
// If the deleted account holds a siswa role, also tells academic-service to
// drop their class_enrollments rows (RemoveStudentEnrollments) — otherwise
// that row survives untouched (no FK from users into class_enrollments,
// cross-service) and keeps inflating DashboardCountsFor's siswa count even
// though Data Siswa now shows the student gone. See Report sekoci V3.docx.
func (a *App) DeleteUser(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	// An admin may only delete accounts of their own unit (by-ID reach
	// across units was possible before).
	if !a.requireUserInScope(w, r, id) {
		return
	}
	var siswaSchoolIDs []string
	roleRows, err := a.DB.Query(`SELECT school_id FROM user_school_roles WHERE user_id = $1 AND role = 'siswa' AND school_id IS NOT NULL`, id)
	if err == nil {
		for roleRows.Next() {
			var schoolID string
			if roleRows.Scan(&schoolID) == nil {
				siswaSchoolIDs = append(siswaSchoolIDs, schoolID)
			}
		}
		roleRows.Close()
	}
	res, err := a.DB.Exec(`UPDATE users SET deleted_at = now() WHERE id = $1 AND deleted_at IS NULL`, id)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal menghapus pengguna")
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		common.WriteError(w, http.StatusNotFound, "Pengguna tidak ditemukan")
		return
	}
	for _, schoolID := range siswaSchoolIDs {
		_ = a.Academic.RemoveStudentEnrollments(schoolID, id)
	}
	// BUG-022: a deleted account's tokens stop working immediately.
	a.revokeAll(r, id, "user_deleted", "")
	c := common.FromCtx(r)
	a.RecordSecurityAudit(nil, "user_deleted", id, &c.UserID, &c.ActiveRole, nil, nil, "user soft-deleted", r.Header.Get("X-Correlation-ID"))
	common.WriteJSON(w, http.StatusOK, map[string]string{"id": id}, nil)
}

// ResetPassword: admin/superadmin resets any (in-scope) user's password to
// a freshly generated default password, returned once in the response.
func (a *App) ResetPassword(w http.ResponseWriter, r *http.Request) {
	if !requireRole(w, r, common.RoleAdmin) {
		return
	}
	claims := common.FromCtx(r)
	id := chi.URLParam(r, "id")
	newPassword := common.GenDefaultPassword()
	hash, err := common.HashPassword(newPassword)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal mengatur ulang kata sandi")
		return
	}
	// A non-superadmin admin may only reset passwords for users who hold a role at their own
	// school — otherwise a School A admin could reset any user's password by ID alone.
	query := `UPDATE users SET password_hash=$1, default_password=NULL, password_changed=false, updated_at=now()
		WHERE id=$2 AND deleted_at IS NULL`
	args := []interface{}{hash, id}
	if claims.ActiveRole != common.RoleSuperadmin {
		args = append(args, claims.SchoolID)
		query += ` AND EXISTS (SELECT 1 FROM user_school_roles usr WHERE usr.user_id = users.id AND usr.school_id = $` + placeholder(len(args)) + `)`
	}
	res, err := a.DB.Exec(query, args...)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal mengatur ulang kata sandi")
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		common.WriteError(w, http.StatusNotFound, "Pengguna tidak ditemukan")
		return
	}
	a.revokeAll(r, id, "password_reset_by_admin", "")
	a.RecordSecurityAudit(nil, "password_reset_by_admin", id, &claims.UserID, &claims.ActiveRole, nil, nil, "admin reset password", r.Header.Get("X-Correlation-ID"))
	common.WriteJSON(w, http.StatusOK, map[string]string{"id": id, "new_password": newPassword}, nil)
}

type assignRoleRequest struct {
	Role     string `json:"role"`
	SchoolID string `json:"school_id"`
	// ExceptionReason (BUG-023): when the role forms a forbidden combination,
	// a reason turns the request into an SoD exception that goes to the
	// Approval Center instead of being refused. ValidUntil (YYYY-MM-DD) is
	// optional; the exception lapses after that date.
	ExceptionReason string `json:"exception_reason"`
	ValidUntil      string `json:"valid_until"`
}

// AssignUserRole: "Admin can assign additional roles/jabatan to a user here."
func (a *App) AssignUserRole(w http.ResponseWriter, r *http.Request) {
	if !requireRole(w, r, common.RoleAdmin) {
		return
	}
	claims := common.FromCtx(r)
	id := chi.URLParam(r, "id")
	var req assignRoleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || !common.IsValidRole(req.Role) {
		common.WriteError(w, http.StatusBadRequest, "Peran tidak valid")
		return
	}
	// Same privilege-escalation guard as CreateUser: an admin can't grant
	// themselves/anyone the admin or superadmin role via this endpoint either.
	if claims.ActiveRole == common.RoleAdmin && !adminAssignable(req.Role) {
		common.WriteError(w, http.StatusForbidden, "Admin unit hanya dapat menetapkan peran guru, siswa, orang tua, atau pengawas")
		return
	}
	schoolID := claims.SchoolID
	if claims.ActiveRole == common.RoleSuperadmin && req.SchoolID != "" {
		schoolID = req.SchoolID
	}
	// Superadmin's own JWT carries no school_id, and the "Tambah Peran" UI
	// (add a role to an existing user from the edit-user modal) never sends
	// one either -- it's adding a role to an account that already belongs to
	// a school, not creating a new tenancy link from scratch. Falling back to
	// that existing school (rather than silently inserting school_id=NULL,
	// which is what happened before this fix -- confirmed in prod: several
	// admin accounts stuck with no school, dashboard blank, everything
	// scoped by school_id coming back empty) keeps the new role in the same
	// tenant the account is already in.
	if schoolID == "" && req.Role != common.RoleSuperadmin {
		_ = a.DB.QueryRow(`SELECT school_id FROM user_school_roles WHERE user_id=$1 AND school_id IS NOT NULL ORDER BY created_at LIMIT 1`, id).Scan(&schoolID)
	}
	if schoolID == "" && req.Role != common.RoleSuperadmin {
		common.WriteError(w, http.StatusBadRequest, "Unit wajib dipilih untuk peran selain superadmin")
		return
	}
	if claims.ActiveRole != common.RoleSuperadmin {
		if ok, err := a.userInScope(r, id); err != nil || !ok {
			common.WriteError(w, http.StatusNotFound, "Pengguna tidak ditemukan")
			return
		}
	}
	// BUG-023 / AC-RBAC-08: refuse a forbidden role combination, or route it
	// to the Approval Center as an exception when a reason is given.
	found, err := a.findRoleConflicts(r.Context(), id, req.Role)
	if err != nil {
		common.WriteDBError(w, err, "gagal memeriksa kombinasi peran")
		return
	}
	if len(found) > 0 {
		if strings.TrimSpace(req.ExceptionReason) == "" {
			writeRoleConflict(w, found)
			return
		}
		a.requestRoleConflictException(w, r, schoolID, id, req, found)
		return
	}
	var schoolIDArg interface{}
	if schoolID != "" {
		schoolIDArg = schoolID
	}
	if _, err := a.DB.Exec(`INSERT INTO user_school_roles (user_id, school_id, role) VALUES ($1,$2,$3)
		ON CONFLICT (user_id, school_id, role) DO NOTHING`, id, schoolIDArg, req.Role); err != nil {
		common.WriteDBError(w, err, "gagal menambahkan peran")
		return
	}
	var sID *string
	if schoolID != "" {
		sID = &schoolID
	}
	a.RecordSecurityAudit(sID, "role_assigned", id, &claims.UserID, &claims.ActiveRole, nil, map[string]any{"assigned_role": req.Role}, "assigned role to user", r.Header.Get("X-Correlation-ID"))

	common.WriteJSON(w, http.StatusCreated, map[string]string{"user_id": id, "role": req.Role}, nil)
}

// RemoveUserRole: "Data Peran" — an admin managing role assignments (as opposed to Data
// Jabatan's org-chart positions) needs to be able to take a role back off an account, not just
// add one.
func (a *App) RemoveUserRole(w http.ResponseWriter, r *http.Request) {
	if !requireRole(w, r, common.RoleAdmin) {
		return
	}
	claims := common.FromCtx(r)
	id := chi.URLParam(r, "id")
	role := chi.URLParam(r, "role")
	query := `DELETE FROM user_school_roles WHERE user_id=$1 AND role=$2`
	args := []interface{}{id, role}
	if claims.ActiveRole != common.RoleSuperadmin {
		query += ` AND school_id=$3`
		args = append(args, claims.SchoolID)
	}
	res, err := a.DB.Exec(query, args...)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal mencabut peran")
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		common.WriteError(w, http.StatusNotFound, "Penetapan peran tidak ditemukan")
		return
	}

	var sID *string
	if claims.SchoolID != "" {
		sID = &claims.SchoolID
	}
	a.RecordSecurityAudit(sID, "role_removed", id, &claims.UserID, &claims.ActiveRole, map[string]any{"removed_role": role}, nil, "removed role from user", r.Header.Get("X-Correlation-ID"))

	common.WriteJSON(w, http.StatusOK, map[string]string{"user_id": id, "role": role}, nil)
}

func (a *App) setTeacherStatus(w http.ResponseWriter, r *http.Request, status string) {
	if !requireRole(w, r, common.RoleAdmin) {
		return
	}
	claims := common.FromCtx(r)
	id := chi.URLParam(r, "id")
	query := `UPDATE user_school_roles SET status=$1 WHERE user_id=$2 AND role='guru'`
	args := []interface{}{status, id}
	if claims.ActiveRole != common.RoleSuperadmin {
		query += ` AND school_id=$3`
		args = append(args, claims.SchoolID)
	}
	// Deactivating does NOT delete the account — other services filter
	// "guru aktif" lists by checking this status via GET /teachers.
	res, err := a.DB.Exec(query, args...)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal memperbarui status guru")
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		common.WriteError(w, http.StatusNotFound, "Pengguna ini tidak memiliki peran guru")
		return
	}
	common.WriteJSON(w, http.StatusOK, map[string]string{"id": id, "status": status}, nil)
}

func (a *App) ActivateTeacher(w http.ResponseWriter, r *http.Request) {
	a.setTeacherStatus(w, r, "active")
}
func (a *App) DeactivateTeacher(w http.ResponseWriter, r *http.Request) {
	a.setTeacherStatus(w, r, "inactive")
}

// ExportUsers: CSV export of the filtered Data Pengguna list.
func (a *App) ExportUsers(w http.ResponseWriter, r *http.Request) {
	if !requireRole(w, r, common.RoleAdmin) {
		return
	}
	schoolID, filtered := resolveSchoolScope(r)
	role := r.URL.Query().Get("role")
	// Sekoci revamp report (Mas Isal, 2026-09-07): "Export Token Ujian"
	// mode narrows the CSV to just NISN/Nama/Email/Password/Token; anything
	// else (including the empty default) is the existing full export.
	tokenOnly := r.URL.Query().Get("mode") == "token"

	where := `u.deleted_at IS NULL`
	args := []interface{}{}
	if role != "" || filtered {
		where += ` AND EXISTS (SELECT 1 FROM user_school_roles usr WHERE usr.user_id = u.id`
		if role != "" {
			args = append(args, role)
			where += ` AND usr.role = $` + placeholder(len(args))
		}
		if filtered {
			args = append(args, schoolID)
			where += ` AND usr.school_id = $` + placeholder(len(args))
		}
		where += `)`
	}
	// Same "Filter Kelas" id-set support as listUsersByRole — the report's
	// other complaint was that exporting after filtering by class still
	// dumped every student because this WHERE never looked at ids/exclude_ids.
	idsParam := strings.TrimSpace(r.URL.Query().Get("ids"))
	excludeParam := strings.TrimSpace(r.URL.Query().Get("exclude_ids"))
	if idsParam != "" {
		args = append(args, pqStringArray(strings.Split(idsParam, ",")))
		where += ` AND u.id = ANY($` + placeholder(len(args)) + `::uuid[])`
	}
	if excludeParam != "" {
		args = append(args, pqStringArray(strings.Split(excludeParam, ",")))
		where += ` AND u.id != ALL($` + placeholder(len(args)) + `::uuid[])`
	}
	rows, err := a.DB.Query(`SELECT `+userCols+` FROM users u WHERE `+where+` ORDER BY u.name`, args...)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal mengekspor pengguna")
		return
	}
	defer rows.Close()

	// Token Ujian only makes sense scoped to one school — an unfiltered
	// superadmin export spanning every school leaves this column blank
	// rather than guessing a school (filtered implies schoolID != "").
	tokensByUser := map[string]string{}
	if filtered {
		tokensByUser = a.Exam.TokensBySchool(schoolID)
	}

	w.Header().Set("Content-Type", "text/csv")
	filename := "users.csv"
	if tokenOnly {
		filename = "token_ujian.csv"
	}
	w.Header().Set("Content-Disposition", "attachment; filename="+filename)
	cw := csv.NewWriter(w)
	if tokenOnly {
		cw.Write([]string{"NISN", "Nama Siswa", "Email", "Password", "Token Ujian"})
	} else {
		cw.Write([]string{"ID", "Nama", "Email", "No. Telepon", "Password", "Token Ujian", "Sudah Ganti Password", "Superadmin", "Tanggal Dibuat"})
	}
	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			break
		}
		password := ""
		if u.DefaultPassword != nil {
			password = *u.DefaultPassword
		}
		if tokenOnly {
			nisn := ""
			if u.NISN != nil {
				nisn = *u.NISN
			}
			cw.Write([]string{nisn, u.Name, u.Email, password, tokensByUser[u.ID]})
			continue
		}
		phone := ""
		if u.Phone != nil {
			phone = *u.Phone
		}
		cw.Write([]string{u.ID, u.Name, u.Email, phone, password, tokensByUser[u.ID], boolStr(u.PasswordChanged), boolStr(u.IsSuperadmin), u.CreatedAt.Format("2006-01-02T15:04:05Z07:00")})
	}
	cw.Flush()
}

func boolStr(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

func placeholder(n int) string {
	// small helper to keep dynamic-where-clause building above readable
	digits := []byte{}
	if n == 0 {
		return "0"
	}
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	return string(digits)
}

func pqStringArray(ids []string) interface{} {
	return stringArray(ids)
}

// stringArray implements driver.Valuer to pass a Go []string as a Postgres
// text[] literal without adding a second SQL-array dependency.
type stringArray []string

func (a stringArray) Value() (driver.Value, error) {
	if len(a) == 0 {
		return "{}", nil
	}
	out := "{"
	for i, s := range a {
		if i > 0 {
			out += ","
		}
		out += `"` + s + `"`
	}
	out += "}"
	return out, nil
}

// DownloadUsersTemplate: "Download Template" button for bulk user import
func (a *App) DownloadUsersTemplate(w http.ResponseWriter, r *http.Request) {
	if !requireRole(w, r, common.RoleAdmin) {
		return
	}
	switch kind := r.URL.Query().Get("kind"); kind {
	case "", userimport.KindUsers:
	case userimport.KindOrangTua, userimport.KindKelas:
		writeKindTemplate(w, kind)
		return
	default:
		common.WriteValidationError(w, http.StatusBadRequest, "Jenis template tidak dikenal", map[string]string{"kind": "Pilih: users, orang_tua, atau kelas"})
		return
	}
	f := excelize.NewFile()
	defer f.Close()
	sheetName := "Data Pengguna"
	f.SetSheetName("Sheet1", sheetName)
	// Columns are matched by header (import_users.go / userimport), so
	// order does not matter. Nama/Email/Peran are required (Peran may be
	// omitted when importing from Data Siswa / Data Guru); Telepon and
	// ID Sumber (the id in the previous system, kept for traceability) are
	// optional. Mandatory columns are marked red. Max 1000 rows per file.
	headers := []string{"Nama *", "Email *", "Telepon", "Peran (guru/siswa) *", "ID Sumber"}
	f.SetSheetRow(sheetName, "A1", &headers)
	f.SetSheetRow(sheetName, "A2", &[]string{"Budi Santoso", "budi@example.com", "08123456789", "siswa", "S-2024-001"})
	mandatoryStyle, err := f.NewStyle(&excelize.Style{Font: &excelize.Font{Bold: true, Color: "C00000"}})
	if err == nil {
		for _, cellRef := range []string{"A1", "B1", "D1"} {
			f.SetCellStyle(sheetName, cellRef, cellRef, mandatoryStyle)
		}
	}

	w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	w.Header().Set("Content-Disposition", "attachment; filename=template_data_pengguna.xlsx")
	_ = f.Write(w)
}

// checkDuplicates answers 409 with the possible duplicates (BUG-016) and
// returns false, or returns true when there are none. Names of accounts
// outside the caller's unit are withheld (tenant isolation).
func (a *App) checkDuplicates(w http.ResponseWriter, r *http.Request, userID, nisn, nik, phoneNorm string) bool {
	matches, err := store.FindDuplicateUsers(r.Context(), a.DB, userID, nisn, nik, phoneNorm)
	if err != nil {
		common.CaptureError(err) // a failed lookup must not block saving
		return true
	}
	if len(matches) == 0 {
		return true
	}
	claims := common.FromCtx(r)
	type dup struct {
		Field  string `json:"field"`
		UserID string `json:"user_id,omitempty"`
		Name   string `json:"name"`
	}
	out := make([]dup, 0, len(matches))
	fields := map[string]string{}
	for _, m := range matches {
		d := dup{Field: m.Field, Name: "(pengguna di unit lain)"}
		visible := claims != nil && claims.ActiveRole == common.RoleSuperadmin
		for _, sid := range m.SchoolIDs {
			if claims != nil && sid == claims.SchoolID {
				visible = true
			}
		}
		if visible {
			d.UserID, d.Name = m.UserID, m.Name
		}
		out = append(out, d)
		fields[m.Field] = map[string]string{"nisn": "NISN", "nik": "NIK", "phone": "Nomor ponsel"}[m.Field] + " sama dengan data lain: kemungkinan duplikat"
	}
	common.WriteJSONMsg(w, http.StatusConflict, map[string]any{"requires_confirmation": true, "duplicates": out, "fields": fields}, nil,
		"Kemungkinan data duplikat. Periksa daftar berikut; kirim ulang dengan confirm_duplicate=true bila memang orang yang berbeda.")
	return false
}
