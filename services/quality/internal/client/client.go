package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

type Clients struct {
	httpClient  *http.Client
	internalKey string
	InsightURL  string
	CoreURL     string
	IdentityURL string
	AcademicURL string
	HRURL       string
	NotifURL    string
}

func NewClients(internalKey, insightURL, coreURL, identityURL, academicURL, hrURL, notifURL string) *Clients {
	return &Clients{
		httpClient:  &http.Client{Timeout: 10 * time.Second},
		internalKey: internalKey,
		InsightURL:  insightURL,
		CoreURL:     coreURL,
		IdentityURL: identityURL,
		AcademicURL: academicURL,
		HRURL:       hrURL,
		NotifURL:    notifURL,
	}
}

// ------------------- Insight Client (T24 metric dictionary contract) -------------------

type MetricDef struct {
	Key             string   `json:"key"`
	Name            string   `json:"name"`
	Unit            string   `json:"unit"`
	AllowedGroupBy  []string `json:"allowed_group_by"`
	OwnerService    string   `json:"owner_service"`
	Formula         string   `json:"formula"`
	Limitations     string   `json:"limitations"`
}

type MetricValueResult struct {
	Key         string     `json:"key"`
	Unit        string     `json:"unit"`
	Value       int64      `json:"value"`
	Numerator   *int64     `json:"numerator,omitempty"`
	Denominator *int64     `json:"denominator,omitempty"`
	AsOf        time.Time  `json:"as_of"`
	ComputedAt  time.Time  `json:"computed_at"`
}

func (c *Clients) ListMetrics(ctx context.Context) ([]MetricDef, error) {
	reqURL := fmt.Sprintf("%s/internal/metrics?status=active", c.InsightURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-Internal-Key", c.internalKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		// Fallback baseline dictionary if insight is not yet started
		return BaselineMetrics(), nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return BaselineMetrics(), nil
	}

	var res struct {
		Data []MetricDef `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return BaselineMetrics(), nil
	}
	return res.Data, nil
}

func (c *Clients) GetMetricValue(ctx context.Context, schoolID, key, from, to, groupBy string) (*MetricValueResult, error) {
	params := url.Values{}
	params.Set("school_id", schoolID)
	params.Set("from", from)
	params.Set("to", to)
	if groupBy != "" {
		params.Set("group_by", groupBy)
	}

	reqURL := fmt.Sprintf("%s/internal/metrics/%s/value?%s", c.InsightURL, url.PathEscape(key), params.Encode())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-Internal-Key", c.internalKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("insight service unavailable: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusServiceUnavailable {
		return nil, fmt.Errorf("sumber metrik tidak tersedia (503)")
	}
	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("kunci metrik tidak dikenal (404)")
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("insight error (%d): %s", resp.StatusCode, string(body))
	}

	var res struct {
		Data MetricValueResult `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return nil, err
	}
	return &res.Data, nil
}

func BaselineMetrics() []MetricDef {
	return []MetricDef{
		{Key: "attendance_rate", Name: "Tingkat Kehadiran Siswa", Unit: "percent_bp", OwnerService: "attendance", Formula: "(Hadir + Terlambat) / Total Hari * 100%"},
		{Key: "collection_rate", Name: "Tingkat Pengumpulan SPP", Unit: "percent_bp", OwnerService: "billing", Formula: "Terbayar / Total Tagihan * 100%"},
		{Key: "outstanding_idr", Name: "Total Piutang Belum Terbayar", Unit: "idr", OwnerService: "billing", Formula: "Sum(Tagihan Belum Lunas)"},
		{Key: "ppdb_conversion", Name: "Tingkat Konversi PPDB", Unit: "percent_bp", OwnerService: "admission", Formula: "Diterima / Pendaftar * 100%"},
		{Key: "ppdb_applicants", Name: "Jumlah Pendaftar PPDB", Unit: "count", OwnerService: "admission", Formula: "Count(Pendaftar)"},
		{Key: "active_students", Name: "Jumlah Siswa Aktif", Unit: "count", OwnerService: "identity", Formula: "Count(Siswa Aktif)"},
		{Key: "active_teachers", Name: "Jumlah Guru Aktif", Unit: "count", OwnerService: "hr", Formula: "Count(Guru Aktif)"},
		{Key: "active_staff", Name: "Jumlah Tenaga Kependidikan", Unit: "count", OwnerService: "hr", Formula: "Count(Pegawai Non-Guru)"},
		{Key: "billed_idr", Name: "Total Nominal Tagihan", Unit: "idr", OwnerService: "billing", Formula: "Sum(Nominal Tagihan Terbit)"},
		{Key: "paid_idr", Name: "Total Pembayaran Masuk", Unit: "idr", OwnerService: "billing", Formula: "Sum(Nominal Pembayaran Diterima)"},
	}
}

// ------------------- Core Client (Approvals & Audit) -------------------

type ApprovalRequest struct {
	SchoolID       string                 `json:"school_id"`
	RequestType    string                 `json:"request_type"`
	RequesterID    string                 `json:"requester_id"`
	SourceService  string                 `json:"source_service"`
	SourceID       string                 `json:"source_id"`
	Summary        string                 `json:"summary"`
	Payload        map[string]interface{} `json:"payload"`
	CallbackURL    string                 `json:"callback_url"`
	IdempotencyKey string                 `json:"idempotency_key"`
}

type ApprovalResponse struct {
	ID     string `json:"id"`
	Status string `json:"status"`
}

func (c *Clients) CreateApproval(ctx context.Context, req ApprovalRequest) (*ApprovalResponse, error) {
	b, _ := json.Marshal(req)
	reqURL := fmt.Sprintf("%s/internal/approvals", c.CoreURL)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, reqURL, bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("X-Internal-Key", c.internalKey)
	if req.IdempotencyKey != "" {
		httpReq.Header.Set("Idempotency-Key", req.IdempotencyKey)
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("core service error: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("approval creation failed (%d): %s", resp.StatusCode, string(body))
	}

	var res struct {
		Data ApprovalResponse `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return nil, err
	}
	return &res.Data, nil
}

func (c *Clients) ForwardAuditEvent(ctx context.Context, payload map[string]interface{}) error {
	b, _ := json.Marshal(payload)
	reqURL := fmt.Sprintf("%s/internal/audit", c.CoreURL)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, reqURL, bytes.NewReader(b))
	if err != nil {
		return err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("X-Internal-Key", c.internalKey)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("forward audit failed (%d): %s", resp.StatusCode, string(body))
	}
	return nil
}

// ------------------- Identity Client (Dapodik Students & Foundation Schools) -------------------

type DapodikStudent struct {
	UserID     string  `json:"user_id"`
	Name       string  `json:"name"`
	NISN       *string `json:"nisn"`
	BirthDate  *string `json:"birth_date"`
	BirthPlace *string `json:"birth_place"`
	Religion   *string `json:"religion"`
}

type FoundationSchool struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Code string `json:"code"`
}

func (c *Clients) GetDapodikStudents(ctx context.Context, schoolID string) ([]DapodikStudent, error) {
	reqURL := fmt.Sprintf("%s/internal/dapodik/students?school_id=%s", c.IdentityURL, url.QueryEscape(schoolID))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-Internal-Key", c.internalKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("identity service error: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("get dapodik students failed (%d): %s", resp.StatusCode, string(body))
	}

	var res struct {
		Data []DapodikStudent `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return nil, err
	}
	return res.Data, nil
}

func (c *Clients) GetFoundationSchools(ctx context.Context, schoolID string) ([]FoundationSchool, error) {
	reqURL := fmt.Sprintf("%s/internal/schools/%s/foundation-schools", c.IdentityURL, url.PathEscape(schoolID))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-Internal-Key", c.internalKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return []FoundationSchool{{ID: schoolID, Name: "Sekolah Ini", Code: "SELF"}}, nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return []FoundationSchool{{ID: schoolID, Name: "Sekolah Ini", Code: "SELF"}}, nil
	}

	var res struct {
		Data []FoundationSchool `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return []FoundationSchool{{ID: schoolID, Name: "Sekolah Ini", Code: "SELF"}}, nil
	}
	return res.Data, nil
}

// ------------------- Academic Client (Dapodik Rombel) -------------------

type DapodikRombelItem struct {
	ClassroomID           string  `json:"classroom_id"`
	Name                  string  `json:"name"`
	GradeLevel            int     `json:"grade_level"`
	HomeroomTeacherUserID *string `json:"homeroom_teacher_user_id"`
	StudentCount          int     `json:"student_count"`
}

type DapodikRombelData struct {
	Rombel          []DapodikRombelItem `json:"rombel"`
	StudentRombelMap map[string]string   `json:"student_rombel_map"`
}

func (c *Clients) GetDapodikRombel(ctx context.Context, schoolID, academicYearID string) (*DapodikRombelData, error) {
	params := url.Values{}
	params.Set("school_id", schoolID)
	if academicYearID != "" {
		params.Set("academic_year_id", academicYearID)
	}

	reqURL := fmt.Sprintf("%s/internal/dapodik/rombel?%s", c.AcademicURL, params.Encode())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-Internal-Key", c.internalKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("academic service error: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("get dapodik rombel failed (%d): %s", resp.StatusCode, string(body))
	}

	var res struct {
		Data DapodikRombelData `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return nil, err
	}
	return &res.Data, nil
}

// ------------------- HR Client (Dapodik PTK & Blind Index Match) -------------------

type DapodikPTK struct {
	EmployeeID       string  `json:"employee_id"`
	FullName         string  `json:"full_name"`
	Gender           string  `json:"gender"`
	BirthDate        string  `json:"birth_date"`
	Position         string  `json:"position"`
	EmploymentStatus string  `json:"employment_status"`
	NIKLast4         *string `json:"nik_last4"`
}

func (c *Clients) GetDapodikPTK(ctx context.Context, schoolID string) ([]DapodikPTK, error) {
	reqURL := fmt.Sprintf("%s/internal/dapodik/ptk?school_id=%s", c.HRURL, url.QueryEscape(schoolID))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-Internal-Key", c.internalKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("hr service error: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("get dapodik ptk failed (%d): %s", resp.StatusCode, string(body))
	}

	var res struct {
		Data []DapodikPTK `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return nil, err
	}
	return res.Data, nil
}

func (c *Clients) MatchPTKNIKs(ctx context.Context, schoolID string, niks []string) (map[string]string, error) {
	payload := map[string]interface{}{
		"school_id": schoolID,
		"niks":      niks,
	}
	b, _ := json.Marshal(payload)

	reqURL := fmt.Sprintf("%s/internal/dapodik/ptk/match", c.HRURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, reqURL, bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Internal-Key", c.internalKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("hr match service error: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("ptk match failed (%d): %s", resp.StatusCode, string(body))
	}

	var res struct {
		Data map[string]string `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return nil, err
	}
	return res.Data, nil
}

// ------------------- Notification Client -------------------

func (c *Clients) SendNotification(ctx context.Context, userID, title, body, link string) error {
	payload := map[string]interface{}{
		"user_id": userID,
		"title":   title,
		"body":    body,
		"link":    link,
	}
	b, _ := json.Marshal(payload)

	reqURL := fmt.Sprintf("%s/internal/notifications", c.NotifURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, reqURL, bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Internal-Key", c.internalKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		// Non-blocking notification failure
		return nil
	}
	defer resp.Body.Close()
	return nil
}
