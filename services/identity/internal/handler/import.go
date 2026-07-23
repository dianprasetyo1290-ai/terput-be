package handler

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/xuri/excelize/v2"

	"skci/identity/internal/common"
)

// Import Template Sekoci: a single .xlsx that registers a brand-new school
// in one shot. 8 sheets, matching the client-provided reference file
// verbatim (Indonesian sheet names + headers):
//
//  1. Data Identitas Sekolah — the school itself.
//  2. Data Guru               — creates guru accounts.
//  3. Data Orang Tua          — no dedicated model yet; staged as JSON
//     (import_staged_rows, kind='orang_tua') alongside Data Kelas / Data
//     Mata Pelajaran below, same "not this service's domain yet" pattern.
//  4. Data Kelas              — academic-service's domain; staged.
//  5. Data Siswa              — creates siswa accounts.
//  6. Struktur Organisasi     — real model (positions/position_assignments),
//     resolved by matching "Nama" against this file's Data Guru sheet.
//  7. Data Mata Pelajaran     — academic-service's domain; staged.
//  8. Data Ekstrakurikuler    — no dedicated model yet; staged.
//
// Guru/Siswa fields that don't have a first-class users column (NUPTK,
// NISN, Pendidikan Terakhir, Asal Sekolah, Nama Orang Tua, the per-row
// Tahun Akademik) go into users.extra (JSONB catch-all, migration 0006)
// rather than one narrow column per field.

const (
	sheetIdentitas       = "Data Identitas Sekolah"
	sheetGuru            = "Data Guru"
	sheetOrangTua        = "Data Orang Tua"
	sheetKelas           = "Data Kelas"
	sheetSiswa           = "Data Siswa"
	sheetOrganisasi      = "Struktur Organisasi"
	sheetMataPelajaran   = "Data Mata Pelajaran"
	sheetEkstrakurikuler = "Data Ekstrakurikuler"
)

// genericStagedSheets: sheets with no dedicated backend model — parsed
// verbatim (row -> header-keyed map) and stored in import_staged_rows under
// `kind`, same handling for all four regardless of which domain eventually
// consumes them.
var genericStagedSheets = []struct{ sheet, kind string }{
	{sheetKelas, "kelas"},
	{sheetMataPelajaran, "mata_pelajaran"},
	{sheetOrangTua, "orang_tua"},
	{sheetEkstrakurikuler, "ekstrakurikuler"},
}

type importRowError struct {
	Sheet string `json:"sheet"`
	Row   int    `json:"row"` // 1-based, excludes header
	Field string `json:"field"`
	Issue string `json:"issue"`
}

type importReport struct {
	OK                bool             `json:"ok"`
	SchoolName        string           `json:"school_name"`
	Counts            map[string]int   `json:"counts"`             // sheet -> valid unique row count
	DuplicatesDropped map[string]int   `json:"duplicates_dropped"` // sheet -> count of rows dropped as duplicates (first kept)
	Errors            []importRowError `json:"errors"`
}

type parsedImport struct {
	school struct {
		name, address, subdistrict, district, city, province string
		registrationNumber, yearFounded, phoneNumber, email  string
		logo                                                 string
	}
	guru              []map[string]string // fields incl. name/email/phone/birth_date/birth_place/address/religion + extra
	guruOrder         []string
	guruEmailByName   map[string]string // lowercased trimmed guru name -> email, for Struktur Organisasi lookup
	siswa             []map[string]string
	siswaOrder        []string
	org               []map[string]string // jabatan, name (Struktur Organisasi's "Nama" column)
	staged            map[string][]map[string]string // kind -> raw rows, see genericStagedSheets
	report            importReport
}

func parseImportFile(f *excelize.File, schoolNameOverride string) *parsedImport {
	pi := &parsedImport{
		guruEmailByName: map[string]string{},
		staged:          map[string][]map[string]string{},
	}
	pi.report.Counts = map[string]int{}
	pi.report.DuplicatesDropped = map[string]int{}

	// Data Identitas Sekolah: header row + one data row.
	if rows, err := f.GetRows(sheetIdentitas); err == nil && len(rows) >= 2 {
		hdr := indexHeader(rows[0])
		row := rows[1]
		get := func(col string) string { return cell(row, hdr, col) }
		pi.school.name = get("nama sekolah")
		pi.school.address = get("alamat")
		pi.school.subdistrict = get("kelurahan")
		pi.school.district = get("kecamatan")
		pi.school.city = get("kota/kabupaten")
		pi.school.province = get("provinsi")
		pi.school.registrationNumber = get("nomor induk")
		pi.school.yearFounded = get("tahun berdiri")
		pi.school.phoneNumber = get("nomor telpon")
		pi.school.email = get("email")
		pi.school.logo = get("link logo sekolah (opsional)")
	} else {
		pi.report.Errors = append(pi.report.Errors, importRowError{Sheet: sheetIdentitas, Row: 0, Issue: "sheet missing or has no data row"})
	}
	if schoolNameOverride != "" {
		pi.school.name = schoolNameOverride
	}
	pi.report.SchoolName = pi.school.name
	if pi.school.name == "" {
		pi.report.Errors = append(pi.report.Errors, importRowError{Sheet: sheetIdentitas, Row: 1, Field: "nama sekolah", Issue: "school name is required (from sheet or the 'Buat sekolah' field)"})
	}

	// Data Guru: dedup by email, first kept.
	seenEmail := map[string]bool{}
	if rows, err := f.GetRows(sheetGuru); err == nil && len(rows) >= 1 {
		hdr := indexHeader(rows[0])
		for i, row := range rows[1:] {
			name := cell(row, hdr, "nama guru")
			email := strings.ToLower(strings.TrimSpace(cell(row, hdr, "email")))
			if name == "" && email == "" {
				continue // blank row
			}
			if email == "" {
				pi.report.Errors = append(pi.report.Errors, importRowError{Sheet: sheetGuru, Row: i + 1, Field: "email", Issue: "email is required"})
				continue
			}
			if seenEmail[email] {
				pi.report.DuplicatesDropped[sheetGuru]++
				continue
			}
			seenEmail[email] = true
			pi.guru = append(pi.guru, map[string]string{
				"name": name, "email": email, "phone": cell(row, hdr, "no telepon"),
				"birth_date": cell(row, hdr, "tanggal lahir"), "birth_place": cell(row, hdr, "tempat lahir"),
				"address": cell(row, hdr, "alamat"), "religion": cell(row, hdr, "agama"),
				"nuptk": cell(row, hdr, "nuptk"), "tahun_akademik": cell(row, hdr, "tahun akademik"),
				"pendidikan_terakhir": cell(row, hdr, "pendidikan terakhir"),
			})
			pi.guruOrder = append(pi.guruOrder, email)
			if name != "" {
				pi.guruEmailByName[strings.ToLower(strings.TrimSpace(name))] = email
			}
		}
	}
	pi.report.Counts[sheetGuru] = len(pi.guru)

	// Data Siswa: dedup by email.
	seenEmail = map[string]bool{}
	if rows, err := f.GetRows(sheetSiswa); err == nil && len(rows) >= 1 {
		hdr := indexHeader(rows[0])
		for i, row := range rows[1:] {
			name := cell(row, hdr, "nama")
			email := strings.ToLower(strings.TrimSpace(cell(row, hdr, "email")))
			if name == "" && email == "" {
				continue
			}
			if email == "" {
				pi.report.Errors = append(pi.report.Errors, importRowError{Sheet: sheetSiswa, Row: i + 1, Field: "email", Issue: "email is required"})
				continue
			}
			if seenEmail[email] {
				pi.report.DuplicatesDropped[sheetSiswa]++
				continue
			}
			seenEmail[email] = true
			pi.siswa = append(pi.siswa, map[string]string{
				"name": name, "email": email, "phone": cell(row, hdr, "no telepon"),
				"birth_date": cell(row, hdr, "tanggal lahir"), "birth_place": cell(row, hdr, "tempat lahir"),
				"address": cell(row, hdr, "alamat"), "religion": cell(row, hdr, "agama"),
				"nisn": cell(row, hdr, "nisn"), "kelas": cell(row, hdr, "kelas"),
				"tahun_akademik": cell(row, hdr, "tahun akademik"), "asal_sekolah": cell(row, hdr, "asal sekolah"),
				"nama_orang_tua": cell(row, hdr, "nama orang tua"),
			})
			pi.siswaOrder = append(pi.siswaOrder, email)
		}
	}
	pi.report.Counts[sheetSiswa] = len(pi.siswa)

	// Struktur Organisasi: Jabatan | Nama. Dedup by (jabatan, name) pair.
	// "Nama" is matched against Data Guru's names in Import() below.
	if rows, err := f.GetRows(sheetOrganisasi); err == nil && len(rows) >= 1 {
		hdr := indexHeader(rows[0])
		seenPair := map[string]bool{}
		for i, row := range rows[1:] {
			jabatan := cell(row, hdr, "jabatan")
			name := cell(row, hdr, "nama")
			if jabatan == "" && name == "" {
				continue
			}
			if jabatan == "" || name == "" {
				pi.report.Errors = append(pi.report.Errors, importRowError{Sheet: sheetOrganisasi, Row: i + 1, Issue: "jabatan and nama are both required"})
				continue
			}
			key := jabatan + "|" + strings.ToLower(strings.TrimSpace(name))
			if seenPair[key] {
				pi.report.DuplicatesDropped[sheetOrganisasi]++
				continue
			}
			seenPair[key] = true
			pi.org = append(pi.org, map[string]string{"jabatan": jabatan, "name": name})
		}
	}
	pi.report.Counts[sheetOrganisasi] = len(pi.org)

	// Data Kelas / Data Mata Pelajaran / Data Orang Tua / Data Ekstrakurikuler:
	// no per-field extraction needed, staged verbatim (dedup by full-row content).
	for _, gs := range genericStagedSheets {
		rows, dropped := parseGenericStagedSheet(f, gs.sheet)
		pi.staged[gs.kind] = rows
		pi.report.Counts[gs.sheet] = len(rows)
		if dropped > 0 {
			pi.report.DuplicatesDropped[gs.sheet] = dropped
		}
	}

	pi.report.OK = len(pi.report.Errors) == 0
	return pi
}

// parseGenericStagedSheet reads a sheet with no dedicated model, keeping the
// whole row as a header-keyed map (dedup on full-row content — no natural
// key at this layer).
func parseGenericStagedSheet(f *excelize.File, sheet string) (rows []map[string]string, duplicatesDropped int) {
	allRows, err := f.GetRows(sheet)
	if err != nil || len(allRows) < 1 {
		return nil, 0
	}
	hdr := indexHeader(allRows[0])
	seenRow := map[string]bool{}
	for _, row := range allRows[1:] {
		if isBlankRow(row) {
			continue
		}
		key := strings.Join(row, "|")
		if seenRow[key] {
			duplicatesDropped++
			continue
		}
		seenRow[key] = true
		rows = append(rows, rowAsMap(row, hdr))
	}
	return rows, duplicatesDropped
}

func indexHeader(header []string) map[string]int {
	idx := map[string]int{}
	for i, h := range header {
		idx[strings.ToLower(strings.TrimSpace(h))] = i
	}
	return idx
}

func cell(row []string, hdr map[string]int, col string) string {
	i, ok := hdr[col]
	if !ok || i >= len(row) {
		return ""
	}
	return strings.TrimSpace(row[i])
}

func rowAsMap(row []string, hdr map[string]int) map[string]string {
	m := map[string]string{}
	for name, i := range hdr {
		if i < len(row) {
			m[name] = strings.TrimSpace(row[i])
		}
	}
	return m
}

func isBlankRow(row []string) bool {
	for _, c := range row {
		if strings.TrimSpace(c) != "" {
			return false
		}
	}
	return true
}

// DryRunImport validates and reports problems without writing anything.
func (a *App) DryRunImport(w http.ResponseWriter, r *http.Request) {
	if !requireRole(w, r, common.RoleSuperadmin) {
		return
	}
	pi, err := a.readImportUpload(r)
	if err != nil {
		common.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, pi.report, nil)
}

// Import: runs the same parse+validate as dry-run; if clean, writes the
// school/academic-year/guru/siswa/organisasi rows in one transaction and
// stages the model-less sheets (Data Kelas, Data Mata Pelajaran, Data Orang
// Tua, Data Ekstrakurikuler) for whichever service ends up owning them.
func (a *App) Import(w http.ResponseWriter, r *http.Request) {
	if !requireRole(w, r, common.RoleSuperadmin) {
		return
	}
	pi, err := a.readImportUpload(r)
	if err != nil {
		common.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	academicYearLabel := strings.TrimSpace(r.FormValue("academic_year"))
	if academicYearLabel == "" {
		common.WriteError(w, http.StatusBadRequest, "academic_year is required")
		return
	}
	if !pi.report.OK {
		common.WriteValidationError(w, http.StatusUnprocessableEntity, "dry run found problems; fix them before importing", pi.report.Errors)
		return
	}

	tx, err := a.DB.Begin()
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "import failed")
		return
	}
	defer tx.Rollback()

	var schoolID string
	err = tx.QueryRow(`INSERT INTO schools (name, address, subdistrict, district, city, province,
			registration_number, year_founded, phone_number, email, logo)
		VALUES ($1,NULLIF($2,''),NULLIF($3,''),NULLIF($4,''),NULLIF($5,''),NULLIF($6,''),
			NULLIF($7,''),NULLIF($8,''),NULLIF($9,''),NULLIF($10,''),NULLIF($11,''))
		RETURNING id`,
		pi.school.name, pi.school.address, pi.school.subdistrict, pi.school.district,
		pi.school.city, pi.school.province, pi.school.registrationNumber, pi.school.yearFounded,
		pi.school.phoneNumber, pi.school.email, pi.school.logo).Scan(&schoolID)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "failed to create school: "+err.Error())
		return
	}

	var academicYearID string
	if err := tx.QueryRow(`INSERT INTO academic_years (school_id, label, is_current) VALUES ($1,$2,true) RETURNING id`,
		schoolID, academicYearLabel).Scan(&academicYearID); err != nil {
		common.WriteError(w, http.StatusInternalServerError, "failed to create academic year: "+err.Error())
		return
	}
	if _, err := tx.Exec(`UPDATE schools SET current_academic_year_id=$1 WHERE id=$2`, academicYearID, schoolID); err != nil {
		common.WriteError(w, http.StatusInternalServerError, "failed to link academic year: "+err.Error())
		return
	}

	emailToUserID := map[string]string{}
	guruCreated := 0
	for _, g := range pi.guru {
		userID, plainPW, err := createImportedUser(tx, g["name"], g["email"], g["phone"], g["birth_date"], g["birth_place"], g["address"], g["religion"],
			extraJSON(map[string]string{"nuptk": g["nuptk"], "tahun_akademik": g["tahun_akademik"], "pendidikan_terakhir": g["pendidikan_terakhir"]}))
		if err != nil {
			common.WriteError(w, http.StatusInternalServerError, "failed to create guru "+g["email"]+": "+err.Error())
			return
		}
		_ = plainPW
		emailToUserID[g["email"]] = userID
		if _, err := tx.Exec(`INSERT INTO user_school_roles (user_id, school_id, role, academic_year_id) VALUES ($1,$2,'guru',$3)`,
			userID, schoolID, academicYearID); err != nil {
			common.WriteError(w, http.StatusInternalServerError, "failed to assign guru role: "+err.Error())
			return
		}
		guruCreated++
	}

	siswaCreated := 0
	for _, s := range pi.siswa {
		userID, _, err := createImportedUser(tx, s["name"], s["email"], s["phone"], s["birth_date"], s["birth_place"], s["address"], s["religion"],
			extraJSON(map[string]string{"nisn": s["nisn"], "kelas": s["kelas"], "tahun_akademik": s["tahun_akademik"], "asal_sekolah": s["asal_sekolah"], "nama_orang_tua": s["nama_orang_tua"]}))
		if err != nil {
			common.WriteError(w, http.StatusInternalServerError, "failed to create siswa "+s["email"]+": "+err.Error())
			return
		}
		if _, err := tx.Exec(`INSERT INTO user_school_roles (user_id, school_id, role, academic_year_id) VALUES ($1,$2,'siswa',$3)`,
			userID, schoolID, academicYearID); err != nil {
			common.WriteError(w, http.StatusInternalServerError, "failed to assign siswa role: "+err.Error())
			return
		}
		siswaCreated++
	}

	positionsAssigned := 0
	for _, o := range pi.org {
		email, found := pi.guruEmailByName[strings.ToLower(strings.TrimSpace(o["name"]))]
		guruUserID, foundUser := emailToUserID[email]
		if !found || !foundUser {
			// jabatan assigned to a name not present in this file's Data Guru sheet — skip, not fatal for import.
			continue
		}
		var positionID string
		if err := tx.QueryRow(`INSERT INTO positions (school_id, name) VALUES ($1,$2)
			ON CONFLICT (school_id, name) DO UPDATE SET name = EXCLUDED.name RETURNING id`,
			schoolID, o["jabatan"]).Scan(&positionID); err != nil {
			common.WriteError(w, http.StatusInternalServerError, "failed to create position: "+err.Error())
			return
		}
		if _, err := tx.Exec(`INSERT INTO position_assignments (position_id, user_id, school_id) VALUES ($1,$2,$3)
			ON CONFLICT (position_id, user_id) DO NOTHING`, positionID, guruUserID, schoolID); err != nil {
			common.WriteError(w, http.StatusInternalServerError, "failed to assign position: "+err.Error())
			return
		}
		if role, ok := common.JabatanOnlyRoleForPosition(o["jabatan"]); ok {
			if _, err := tx.Exec(`INSERT INTO user_school_roles (user_id, school_id, role) VALUES ($1,$2,$3)
				ON CONFLICT (user_id, school_id, role) DO NOTHING`, guruUserID, schoolID, role); err != nil {
				common.WriteError(w, http.StatusInternalServerError, "failed to mirror jabatan role: "+err.Error())
				return
			}
		}
		positionsAssigned++
	}

	// Data Kelas / Data Mata Pelajaran / Data Orang Tua / Data Ekstrakurikuler
	// have no owning model yet — staged verbatim as JSON rows. Once
	// academic-service (kelas/mata_pelajaran) or a parent/ekstrakurikuler
	// model exists, dispatch these instead of / in addition to staging them
	// here (same TODO as before, see README Assumptions).
	stagedCounts := map[string]int{}
	for kind, rows := range pi.staged {
		for _, row := range rows {
			payload, _ := json.Marshal(row)
			if _, err := tx.Exec(`INSERT INTO import_staged_rows (school_id, kind, payload) VALUES ($1,$2,$3)`, schoolID, kind, payload); err != nil {
				common.WriteError(w, http.StatusInternalServerError, "failed to stage "+kind+" row: "+err.Error())
				return
			}
			stagedCounts[kind]++
		}
	}

	if err := tx.Commit(); err != nil {
		common.WriteError(w, http.StatusInternalServerError, "import failed to commit: "+err.Error())
		return
	}

	common.WriteJSON(w, http.StatusCreated, map[string]interface{}{
		"school_id":              schoolID,
		"academic_year_id":       academicYearID,
		"guru_created":           guruCreated,
		"siswa_created":          siswaCreated,
		"positions_assigned":     positionsAssigned,
		"kelas_staged":           stagedCounts["kelas"],
		"mata_pelajaran_staged":  stagedCounts["mata_pelajaran"],
		"orang_tua_staged":       stagedCounts["orang_tua"],
		"ekstrakurikuler_staged": stagedCounts["ekstrakurikuler"],
	}, nil)
}

// extraJSON marshals the non-empty entries of m into a JSON object for
// users.extra, or returns nil (-> SQL NULL) if every value is blank.
func extraJSON(m map[string]string) []byte {
	clean := map[string]string{}
	for k, v := range m {
		if v != "" {
			clean[k] = v
		}
	}
	if len(clean) == 0 {
		return nil
	}
	b, _ := json.Marshal(clean)
	return b
}

func createImportedUser(tx *sql.Tx, name, email, phone, birthDate, birthPlace, address, religion string, extra []byte) (userID, plainPassword string, err error) {
	plainPassword = common.GenDefaultPassword()
	hash, err := common.HashPassword(plainPassword)
	if err != nil {
		return "", "", err
	}
	nullable := func(s string) interface{} {
		if s == "" {
			return nil
		}
		return s
	}
	var extraPtr interface{}
	if extra != nil {
		extraPtr = extra
	}
	err = tx.QueryRow(`INSERT INTO users (name, email, phone, password_hash, default_password, password_changed,
			birth_date, birth_place, address, religion, extra)
		VALUES ($1,$2,$3,$4,$5,false,$6,$7,$8,$9,$10) RETURNING id`,
		name, email, nullable(phone), hash, plainPassword,
		nullable(birthDate), nullable(birthPlace), nullable(address), nullable(religion), extraPtr).Scan(&userID)
	return userID, plainPassword, err
}

func (a *App) readImportUpload(r *http.Request) (*parsedImport, error) {
	if err := r.ParseMultipartForm(20 << 20); err != nil {
		return nil, fmt.Errorf("failed to parse multipart form: %w", err)
	}
	file, _, err := r.FormFile("file")
	if err != nil {
		return nil, fmt.Errorf("file is required (multipart field 'file')")
	}
	defer file.Close()
	f, err := excelize.OpenReader(file)
	if err != nil {
		return nil, fmt.Errorf("invalid .xlsx file: %w", err)
	}
	defer f.Close()
	schoolNameOverride := strings.TrimSpace(r.FormValue("school_name")) // the "Buat sekolah" field
	return parseImportFile(f, schoolNameOverride), nil
}

// DownloadTemplate serves a blank .xlsx with the 8 required sheets, header
// rows, and one example data row each, for the "Download Template" button.
// Sheet names/headers match the reference file verbatim — see task notes.
func (a *App) DownloadTemplate(w http.ResponseWriter, r *http.Request) {
	if !requireRole(w, r, common.RoleSuperadmin) {
		return
	}
	f := excelize.NewFile()
	defer f.Close()

	type sheetDef struct {
		headers []string
		example []string
	}
	sheets := []struct {
		name string
		def  sheetDef
	}{
		{sheetIdentitas, sheetDef{
			headers: []string{"Nama Sekolah", "Alamat", "Kelurahan", "Kecamatan", "Kota/Kabupaten", "Provinsi", "Nomor Induk", "Tahun Berdiri", "Nomor telpon", "Email", "Link Logo sekolah (opsional)"},
			example: []string{"SD Negeri 1 Menteng", "Jl. Cikini Raya No. 10", "Menteng", "Menteng", "Jakarta Pusat", "DKI Jakarta", "20100001", "1985", "021-31923456", "info@sdn1menteng.sch.id", "https://sdn1menteng.sch.id/logo.png"},
		}},
		{sheetGuru, sheetDef{
			headers: []string{"Nama Guru", "NUPTK", "Tahun Akademik", "Tanggal Lahir", "Tempat Lahir", "Alamat", "No Telepon", "Email", "Pendidikan Terakhir", "Agama"},
			example: []string{"Siti Rahmawati, S.Pd", "1234567890123456", "2026/2027", "1985-04-12", "Jakarta", "Jl. Kebon Sirih No. 5, Jakarta Pusat", "081234567890", "siti.rahmawati@sdn1menteng.sch.id", "S1 Pendidikan Guru SD", "Islam"},
		}},
		{sheetOrangTua, sheetDef{
			headers: []string{"Nama", "Email", "No Telepon", "Alamat"},
			example: []string{"Budi Santoso", "budi.santoso@gmail.com", "081298765432", "Jl. Mangga No. 3, Jakarta Pusat"},
		}},
		{sheetKelas, sheetDef{
			headers: []string{"Kelas", "Sub Kelas", "Wali Kelas"},
			example: []string{"X", "A", "Siti Rahmawati, S.Pd"},
		}},
		{sheetSiswa, sheetDef{
			headers: []string{"Nama", "NISN", "Kelas", "Tahun Akademik", "Tanggal Lahir", "Tempat Lahir", "Alamat", "No Telepon", "Email", "Asal Sekolah", "Agama", "Nama Orang Tua"},
			example: []string{"Ahmad Fauzan", "0051234567", "X A", "2026/2027", "2012-08-20", "Jakarta", "Jl. Mangga No. 3, Jakarta Pusat", "081298765432", "ahmad.fauzan@gmail.com", "TK Islam Bunga Bangsa", "Islam", "Budi Santoso"},
		}},
		{sheetOrganisasi, sheetDef{
			headers: []string{"Jabatan", "Nama"},
			example: []string{"Kepala Sekolah", "Siti Rahmawati, S.Pd"},
		}},
		{sheetMataPelajaran, sheetDef{
			headers: []string{"Mata Pelajaran", "Guru Mata Pelajaran"},
			example: []string{"Matematika", "Siti Rahmawati, S.Pd"},
		}},
		{sheetEkstrakurikuler, sheetDef{
			headers: []string{"Nama Ekstrakurikuler", "Nama Pembina"},
			example: []string{"Pramuka", "Siti Rahmawati, S.Pd"},
		}},
	}

	first := true
	for _, s := range sheets {
		name := s.name
		if first {
			f.SetSheetName("Sheet1", name)
			first = false
		} else {
			f.NewSheet(name)
		}
		for i, h := range s.def.headers {
			cellRef, _ := excelize.CoordinatesToCellName(i+1, 1)
			f.SetCellValue(name, cellRef, h)
		}
		for i, v := range s.def.example {
			cellRef, _ := excelize.CoordinatesToCellName(i+1, 2)
			f.SetCellValue(name, cellRef, v)
		}
	}

	w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	w.Header().Set("Content-Disposition", "attachment; filename=sekoci_import_template.xlsx")
	if err := f.Write(w); err != nil {
		common.WriteError(w, http.StatusInternalServerError, "failed to generate template")
	}
}
