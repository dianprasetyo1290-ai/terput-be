// Package client holds internal service-to-service HTTP clients.
package client

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"
)

// ExamClient talks to exam-service's GET /vouchers via X-Internal-Key, the
// same trusted-sibling-call shape academic-service's IdentityClient uses.
// Only used by Export CSV to join each student's exam token — see
// handler/users.go ExportUsers.
type ExamClient struct {
	BaseURL     string
	InternalKey string
	HTTPClient  *http.Client
}

func NewExamClient() *ExamClient {
	base := os.Getenv("EXAM_SERVICE_URL")
	if base == "" {
		base = "http://exam:8084"
	}
	key := os.Getenv("SEKOCI_INTERNAL_KEY")
	if key == "" {
		key = "change-me"
	}
	return &ExamClient{
		BaseURL:     base,
		InternalKey: key,
		HTTPClient:  &http.Client{Timeout: 3 * time.Second},
	}
}

type ExamToken struct {
	UserID string `json:"user_id"`
	Code   string `json:"code"`
}

type examTokensEnvelope struct {
	Data []ExamToken `json:"data"`
}

// TokensBySchool returns every voucher code issued in a school, keyed by
// student user_id (a student only ever has one active voucher — see
// ExamToken doc comment in exam-service's model). Returns an empty map (not
// an error) if exam-service is unreachable or the school has no tokens, so
// Export CSV degrades to a blank Token Ujian column rather than failing the
// whole export.
func (c *ExamClient) TokensBySchool(schoolID string) map[string]string {
	out := map[string]string{}
	if schoolID == "" {
		return out
	}
	url := fmt.Sprintf("%s/vouchers?school_id=%s", c.BaseURL, schoolID)
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return out
	}
	req.Header.Set("X-Internal-Key", c.InternalKey)
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return out
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return out
	}
	var env examTokensEnvelope
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		return out
	}
	for _, t := range env.Data {
		out[t.UserID] = t.Code
	}
	return out
}
