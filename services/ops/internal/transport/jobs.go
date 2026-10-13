package transport

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"skci/ops/internal/client"
)

type JobRunner struct {
	db          *sql.DB
	client      *client.Client
	billingURL  string
	internalKey string
	loc         *time.Location
	httpClient  *http.Client
}

func NewJobRunner(db *sql.DB, c *client.Client, billingURL, internalKey string, loc *time.Location) *JobRunner {
	if loc == nil {
		loc = time.UTC
	}
	return &JobRunner{
		db:          db,
		client:      c,
		billingURL:  billingURL,
		internalKey: internalKey,
		loc:         loc,
		httpClient:  &http.Client{Timeout: 10 * time.Second},
	}
}

// RunDailyTasks handles expiring document reminders and auto-creating daily trips (AC-120-01).
func (j *JobRunner) RunDailyTasks(ctx context.Context, schoolID string) error {
	store := NewStore(j.db, j.loc)
	st, err := store.GetSettings(ctx, schoolID)
	if err != nil {
		return err
	}

	today := time.Now().In(j.loc)

	// 1. Create daily jemput and antar trips for active routes
	_, _ = store.GetOrCreateDailyTrips(ctx, schoolID, today)

	// 2. Check expiring documents and send notification to transport_officer (AC-120-01)
	expiring, err := store.ListExpiring(ctx, schoolID, st.DocReminderDays)
	if err == nil && len(expiring) > 0 && j.client != nil {
		officers, offErr := j.client.UsersWithRole(ctx, schoolID, "transport_officer")
		if offErr == nil && len(officers) > 0 {
			for _, item := range expiring {
				idempKey := fmt.Sprintf("transport-doc:%s:%s", item.ID, item.ValidUntil.Format("2006-01-02"))
				body := fmt.Sprintf("%s akan kedaluwarsa dalam %d hari (%s). Segera perpanjang.",
					item.Title, item.DaysLeft, item.ValidUntil.Format("02 Jan 2006"))
				if item.DaysLeft < 0 {
					body = fmt.Sprintf("%s SUDAH KEDALUWARSA pada %s!", item.Title, item.ValidUntil.Format("02 Jan 2006"))
				}

				for _, off := range officers {
					_ = j.client.Notify(ctx, client.Notification{
						SchoolID: schoolID,
						UserID:   off.ID,
						Type:     "transport",
						Title:    "Peringatan Masa Berlaku Dokumen Transportasi",
						Body:     body,
						Data: map[string]any{
							"kind":        item.Kind,
							"id":          item.ID,
							"valid_until": item.ValidUntil.Format("2006-01-02"),
							"days_left":   item.DaysLeft,
						},
					}, idempKey)
				}
			}
		}
	}

	return nil
}

// RunDailyTasksForAll runs the daily transport job across all schools with settings or routes.
func (j *JobRunner) RunDailyTasksForAll(ctx context.Context) error {
	rows, err := j.db.QueryContext(ctx, `
		SELECT DISTINCT school_id FROM transport_settings
		UNION
		SELECT DISTINCT school_id FROM transport_routes
	`)
	if err != nil {
		return err
	}
	defer rows.Close()

	var schools []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err == nil {
			schools = append(schools, s)
		}
	}

	for _, s := range schools {
		_ = j.RunDailyTasks(ctx, s)
	}
	return nil
}

// IssueMonthlyCharges computes and issues ad-hoc charges to billing (AC-122-01).
func (j *JobRunner) IssueMonthlyCharges(ctx context.Context, schoolID string, year, month int) (int, int, error) {
	store := NewStore(j.db, j.loc)
	previews, err := store.PreviewBilling(ctx, schoolID, year, month)
	if err != nil {
		return 0, 0, err
	}

	issuedCount := 0
	failedCount := 0

	for _, p := range previews {
		if p.HasFareError || p.TotalAmount <= 0 {
			continue
		}

		itemsJSON, _ := json.Marshal(p.Items)
		charge := Charge{
			SchoolID:      schoolID,
			StudentUserID: p.StudentUserID,
			PeriodYear:    year,
			PeriodMonth:   month,
			Items:         itemsJSON,
			AmountIDR:     p.TotalAmount,
			Version:       1,
			Status:        "issued",
		}

		// Call Billing POST /internal/bills/adhoc (BILL-ADHOC)
		billID, billErr := j.callBillAdhoc(ctx, schoolID, p.StudentUserID, year, month, 1, p.Items)
		if billErr != nil {
			charge.Status = "failed"
			errStr := billErr.Error()
			charge.LastError = &errStr
			failedCount++
		} else {
			charge.BillID = &billID
			issuedCount++
		}

		_, _ = store.SaveCharge(ctx, charge)
	}

	return issuedCount, failedCount, nil
}

// AdjustChargeOnAllocationChange recomputes charge and calls billing adjust if amount changed (FR-122-03).
func (j *JobRunner) AdjustChargeOnAllocationChange(ctx context.Context, schoolID, studentUserID string, year, month int) error {
	store := NewStore(j.db, j.loc)
	st, err := store.GetSettings(ctx, schoolID)
	if err != nil {
		return err
	}

	fares, err := store.ListFares(ctx, schoolID)
	if err != nil {
		return err
	}

	allocs, err := store.ListAllocations(ctx, schoolID, "", studentUserID)
	if err != nil {
		return err
	}

	var segments []AllocationSegment
	for _, a := range allocs {
		segments = append(segments, AllocationSegment{
			AllocationID: a.ID,
			RouteID:      a.RouteID,
			RouteName:    a.RouteName,
			StartDate:    a.StartDate,
			EndDate:      a.EndDate,
		})
	}

	newItems, newTotal, err := ComputeCharge(segments, fares, st.ProrataPolicy, year, month)
	if err != nil {
		return err
	}

	// Fetch existing charge
	charges, err := store.ListCharges(ctx, schoolID, year, month, "")
	if err != nil {
		return err
	}

	var currentCharge *Charge
	for i := range charges {
		if charges[i].StudentUserID == studentUserID {
			currentCharge = &charges[i]
			break
		}
	}

	if currentCharge == nil || currentCharge.Status != "issued" {
		return nil // Not yet issued, no adjust needed
	}

	if !ChargeDiff(currentCharge, newItems, newTotal) {
		return nil // No change
	}

	newVersion := currentCharge.Version + 1
	adjustErr := j.callBillAdjust(ctx, schoolID, studentUserID, year, month, newVersion, newItems, "Perubahan rute atau periode alokasi")
	if adjustErr != nil {
		errStr := adjustErr.Error()
		currentCharge.LastError = &errStr
		_ = store.UpdateChargeStatus(ctx, schoolID, currentCharge.ID, "failed", "", errStr)
		return adjustErr
	}

	itemsJSON, _ := json.Marshal(newItems)
	currentCharge.Items = itemsJSON
	currentCharge.AmountIDR = newTotal
	currentCharge.Version = newVersion
	currentCharge.Status = "adjusted"
	currentCharge.LastError = nil
	_, err = store.SaveCharge(ctx, *currentCharge)
	return err
}

func (j *JobRunner) callBillAdhoc(ctx context.Context, schoolID, studentUserID string, year, month, version int, items []ChargeItem) (string, error) {
	if j.billingURL == "" {
		return fmt.Sprintf("fake-bill-%d", time.Now().UnixNano()), nil
	}

	type billLine struct {
		Code      string `json:"code"`
		Name      string `json:"name"`
		AmountIDR int64  `json:"amount_idr"`
	}
	var lines []billLine
	for _, it := range items {
		lines = append(lines, billLine{
			Code:      it.Code,
			Name:      it.Name,
			AmountIDR: it.AmountIDR,
		})
	}

	reqBody := map[string]any{
		"school_id":       schoolID,
		"student_user_id": studentUserID,
		"source":          "transport",
		"source_ref":      fmt.Sprintf("transport:%s:%04d-%02d", studentUserID, year, month),
		"period_year":     year,
		"period_month":    month,
		"items":           lines,
		"idempotency_key": fmt.Sprintf("transport:%s:%04d-%02d:v%d", studentUserID, year, month, version),
	}
	bodyBytes, _ := json.Marshal(reqBody)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, j.billingURL+"/internal/bills/adhoc", bytes.NewReader(bodyBytes))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Internal-Key", j.internalKey)

	resp, err := j.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		respBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return "", fmt.Errorf("billing error (%d): %s", resp.StatusCode, string(respBytes))
	}

	var res struct {
		BillID string `json:"bill_id"`
	}
	resBytes, _ := io.ReadAll(resp.Body)
	_ = json.Unmarshal(resBytes, &res)
	return res.BillID, nil
}

func (j *JobRunner) callBillAdjust(ctx context.Context, schoolID, studentUserID string, year, month, version int, items []ChargeItem, reason string) error {
	if j.billingURL == "" {
		return nil
	}

	type billLine struct {
		Code      string `json:"code"`
		Name      string `json:"name"`
		AmountIDR int64  `json:"amount_idr"`
	}
	var lines []billLine
	for _, it := range items {
		lines = append(lines, billLine{
			Code:      it.Code,
			Name:      it.Name,
			AmountIDR: it.AmountIDR,
		})
	}

	reqBody := map[string]any{
		"school_id":       schoolID,
		"source":          "transport",
		"source_ref":      fmt.Sprintf("transport:%s:%04d-%02d", studentUserID, year, month),
		"items":           lines,
		"reason":          reason,
		"idempotency_key": fmt.Sprintf("transport:%s:%04d-%02d:v%d", studentUserID, year, month, version),
	}
	bodyBytes, _ := json.Marshal(reqBody)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, j.billingURL+"/internal/bills/adhoc/adjust", bytes.NewReader(bodyBytes))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Internal-Key", j.internalKey)

	resp, err := j.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		respBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return fmt.Errorf("billing adjust error (%d): %s", resp.StatusCode, string(respBytes))
	}

	return nil
}
