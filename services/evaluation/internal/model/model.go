package model

import "time"

// ---------------- RPM (lesson plans) ----------------

type LessonPlan struct {
	ID             string                 `json:"id"`
	SchoolID       string                 `json:"school_id"`
	AcademicYearID string                 `json:"academic_year_id"`
	GuruUserID     string                 `json:"guru_user_id"`
	Title          string                 `json:"title"`
	SectionA       map[string]interface{} `json:"section_a"` // A. Identitas & Karakteristik
	SectionB       map[string]interface{} `json:"section_b"` // B. Analisis Kesiapan & Desain
	SectionC       map[string]interface{} `json:"section_c"` // C. Langkah Pembelajaran
	// DimensiLulusan holds the 8 graduate-profile dimensions (see
	// DimensiLulusanFields), keyed by dimension code -> guru-authored text.
	// A real "D. Indikator 8 Dimensi Lulusan" section on the manual create
	// form (Bug Report SEKOCI GPM V10, ROLE GURU) as well as
	// import/rpm-import -- both paths write it the same way.
	DimensiLulusan map[string]interface{} `json:"dimensi_lulusan"`
	Status         string                 `json:"status"`
	CreatedAt      time.Time              `json:"created_at"`
	UpdatedAt      time.Time              `json:"updated_at"`
}

// DimensiLulusanField is one of the 8 "Dimensi Lulusan" (graduate-profile
// dimensions) the client's RPM template asks a guru to fill per lesson --
// Kurikulum Merdeka's 2024 successor to the older 6-dimension "Profil
// Pelajar Pancasila", per Kepmendikdasmen. Code is the JSONB key; Label is
// the Excel template's row label (and the display label).
type DimensiLulusanField struct {
	Code  string
	Label string
}

// DimensiLulusanFields is the fixed set of 8 dimensions, in template order.
// Not school-configurable -- the client specified these by name.
//
// Corrected 2026-08-11 (Bug Report SEKOCI, 5 Agustus 2026): an earlier pass
// guessed these names without the real template and got them wrong. See
// migration 000004 for the data-side fix.
var DimensiLulusanFields = []DimensiLulusanField{
	{"beriman_bertakwa_berakhlak_mulia", "Beriman, Bertakwa, dan Berakhlak Mulia"},
	{"berkebinekaan_global", "Berkebinekaan Global"},
	{"bergotong_royong", "Bergotong Royong"},
	{"mandiri", "Mandiri"},
	{"bernalar_kritis", "Bernalar Kritis"},
	{"kreatif", "Kreatif"},
	{"literasi_digital", "Literasi & Digital"},
	{"kepemimpinan", "Kepemimpinan"},
}

// RpmField is a single labeled field within RPM section A, B, or C (the
// frontend's own field breakdown of the freeform section_a/b/c JSONB --
// see model.LessonPlan doc and README Assumption #9). Reused here so the
// combined A/B/C/D Excel template (rpm_template.go) and its import stay in
// lockstep with the guru-facing form (RpmMineFormView.vue) without
// duplicating the field list a third time.
type RpmField struct {
	Code  string
	Label string
}

// SectionAFields -- A. Identitas & Karakteristik.
var SectionAFields = []RpmField{
	{"alokasi_waktu", "Alokasi Waktu"},
	{"fase", "Fase"},
	{"karakteristik_peserta_didik", "Karakteristik Peserta Didik"},
	{"kelas", "Kelas"},
	{"mata_pelajaran", "Mata Pelajaran"},
}

// SectionBFields -- B. Analisis Kesiapan & Desain.
var SectionBFields = []RpmField{
	{"asesmen_diagnostik", "Asesmen Diagnostik"},
	{"capaian_pembelajaran", "Capaian Pembelajaran"},
	{"pemahaman_bermakna", "Pemahaman Bermakna"},
	{"pertanyaan_pemantik", "Pertanyaan Pemantik"},
	{"tujuan_pembelajaran", "Tujuan Pembelajaran"},
}

// SectionCFields -- C. Langkah Pembelajaran.
var SectionCFields = []RpmField{
	{"asesmen_formatif", "Asesmen Formatif"},
	{"kegiatan_awal", "Kegiatan Awal"},
	{"kegiatan_inti", "Kegiatan Inti"},
	{"kegiatan_penutup", "Kegiatan Penutup"},
	{"refleksi", "Refleksi"},
}

// ---------------- Program Tahunan (Prota) ----------------
// NEW FEATURE, Bug Report SEKOCI 5 Agustus 2026. Excel-import-driven parent
// + line-items, mirrors Program Semester below. One Prota = one (guru,
// mata pelajaran, kelas, tahun ajaran) curriculum plan; ProtaItem = one row
// of Template_Prota_LMS_Ready.xlsx sheet TEMPLATE_PROTA.

type Prota struct {
	ID             string      `json:"id"`
	SchoolID       string      `json:"school_id"`
	AcademicYearID string      `json:"academic_year_id"`
	GuruUserID     string      `json:"guru_user_id"`
	MataPelajaran  string      `json:"mata_pelajaran"`
	Kelas          string      `json:"kelas"`
	Items          []ProtaItem `json:"items,omitempty"`
	CreatedAt      time.Time   `json:"created_at"`
	UpdatedAt      time.Time   `json:"updated_at"`
}

type ProtaItem struct {
	ID                      string `json:"id"`
	No                      int    `json:"no"`
	Semester                string `json:"semester"`
	ElemenKodeBab           string `json:"elemen_kode_bab"`
	KompetensiDasarTP       string `json:"kompetensi_dasar_tp"`
	AlokasiWaktuJP          int    `json:"alokasi_waktu_jp"`
	KeteranganTargetAsesmen string `json:"keterangan_target_asesmen"`
}

// ---------------- Program Semester (Prosem) ----------------
// Same pattern as Prota. ProsemItem = one row of
// Template_Prosem_LMS_Ready.xlsx sheet TEMPLATE_PROSEM.

type Prosem struct {
	ID             string       `json:"id"`
	SchoolID       string       `json:"school_id"`
	AcademicYearID string       `json:"academic_year_id"`
	GuruUserID     string       `json:"guru_user_id"`
	MataPelajaran  string       `json:"mata_pelajaran"`
	Kelas          string       `json:"kelas"`
	Items          []ProsemItem `json:"items,omitempty"`
	CreatedAt      time.Time    `json:"created_at"`
	UpdatedAt      time.Time    `json:"updated_at"`
}

type ProsemItem struct {
	ID                   string `json:"id"`
	No                   int    `json:"no"`
	MateriPokokAktivitas string `json:"materi_pokok_aktivitas"`
	AlokasiJP            int    `json:"alokasi_jp"`
	BulanTarget          string `json:"bulan_target"`
	MingguKe             int    `json:"minggu_ke"`
	EstimasiTanggal      string `json:"estimasi_tanggal"` // YYYY-MM-DD, "" if unset
	TipeAktivitasLMS     string `json:"tipe_aktivitas_lms"`
	StatusInputGuru      string `json:"status_input_guru"`
}

// ---------------- Penilaian Guru ----------------

type Period struct {
	ID                     string    `json:"id"`
	SchoolID               string    `json:"school_id"`
	AcademicYearID         string    `json:"academic_year_id"`
	Title                  string    `json:"title"`
	Semester               string    `json:"semester"`
	PublishStartDate       string    `json:"publish_start_date"` // YYYY-MM-DD
	PublishEndDate         string    `json:"publish_end_date"`
	Published              bool      `json:"published"`
	HeadmasterUserID       *string   `json:"headmaster_user_id"`
	CurriculumVPUserID     *string   `json:"curriculum_vp_user_id"`
	StudentAffairsVPUserID *string   `json:"student_affairs_vp_user_id"`
	TargetTeacherIDs       []string  `json:"target_teacher_ids"`
	PeerEvaluateeIDs       []string  `json:"peer_evaluatee_ids"`
	PeerEvaluatorIDs       []string  `json:"peer_evaluator_ids"`
	StudentClassroomIDs    []string  `json:"student_classroom_ids"`
	LeadAssessorName       string    `json:"lead_assessor_name"`
	Status                 string    `json:"status"` // computed: Draft|Published|Sedang Berlangsung|Selesai
	CreatedBy              *string   `json:"created_by"`
	UpdatedBy              *string   `json:"updated_by"`
	CreatedAt              time.Time `json:"created_at"`
	UpdatedAt              time.Time `json:"updated_at"`
}

type EvalForm struct {
	ID            string    `json:"id"`
	SchoolID      string    `json:"school_id"`
	Code          string    `json:"code"`
	Title         string    `json:"title"`
	EvaluatorType string    `json:"evaluator_type"`
	Priority      int       `json:"priority"`
	Weight        float64   `json:"weight"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

type EvalItem struct {
	ID          string            `json:"id"`
	FormID      string            `json:"form_id"`
	SchoolID    string            `json:"school_id"`
	Code        string            `json:"code"`
	Indicator   string            `json:"indicator"`
	IndicatorEn string            `json:"indicator_en"`
	Weight      float64           `json:"weight"`
	MaxScore    int               `json:"max_score"`
	ScoreLabels map[string]string `json:"score_labels"`
	SortOrder   int               `json:"sort_order"`
}

type EvalTask struct {
	ID                  string     `json:"id"`
	PeriodID            string     `json:"period_id"`
	FormID              string     `json:"form_id"`
	SchoolID            string     `json:"school_id"`
	AcademicYearID      string     `json:"academic_year_id"`
	TargetTeacherUserID string     `json:"target_teacher_user_id"`
	EvaluatorUserID     string     `json:"evaluator_user_id"`
	EvaluatorType       string     `json:"evaluator_type"`
	ClassroomID         *string    `json:"classroom_id"`
	Status              string     `json:"status"`
	SubmittedAt         *time.Time `json:"submitted_at"`
	RawScore            *float64   `json:"raw_score"`
	CreatedAt           time.Time  `json:"created_at"`
	UpdatedAt           time.Time  `json:"updated_at"`
}

type EvalAnswer struct {
	ID        string    `json:"id"`
	TaskID    string    `json:"task_id"`
	ItemID    string    `json:"item_id"`
	SchoolID  string    `json:"school_id"`
	Score     int       `json:"score"`
	Notes     string    `json:"notes"`
	CreatedAt time.Time `json:"created_at"`
}

// Status constants for periods (computed, date-driven)
const (
	StatusDraft     = "Draft"
	StatusPublished = "Published"
	StatusOngoing   = "Sedang Berlangsung"
	StatusFinished  = "Selesai"
)

// Evaluator type constants (map to form.evaluator_type / task.evaluator_type)
const (
	EvalTypeKepalaSekolah = "kepala_sekolah"
	EvalTypeWakaKurikulum = "waka_kurikulum"
	EvalTypeWakaKesiswaan = "waka_kesiswaan"
	EvalTypeSelf          = "self"
	EvalTypePeer          = "peer"
	EvalTypeSiswa         = "siswa"

	// Upward-feedback track: guru rates a school leader. Separate from the
	// 6 categories above (self 25/siswa 22/kepsek 20/waka-kurikulum 18/
	// waka-kesiswaan 10/peer 5 = 100%) -- these are NOT blended into that
	// weighted score (no room in the 100%, and it's the opposite direction:
	// teacher rating their supervisor, not the supervisor's report card).
	// Reuses the same EvalTask row shape: for these types
	// target_teacher_user_id holds the LEADER being rated and
	// evaluator_user_id holds the rating guru (swapped vs. the
	// kepala_sekolah/waka_* types above, where the leader is the evaluator).
	EvalTypeGuruToKepsek        = "guru_to_kepsek"
	EvalTypeGuruToWakaKurikulum = "guru_to_waka_kurikulum"
	EvalTypeGuruToWakaKesiswaan = "guru_to_waka_kesiswaan"
)

// IsUpwardEvalType reports whether t is one of the guru-rates-a-leader
// upward-feedback types (see EvalTypeGuruToKepsek doc comment).
func IsUpwardEvalType(t string) bool {
	return t == EvalTypeGuruToKepsek || t == EvalTypeGuruToWakaKurikulum || t == EvalTypeGuruToWakaKesiswaan
}

const (
	TaskStatusPending   = "pending"
	TaskStatusSubmitted = "submitted"
)
