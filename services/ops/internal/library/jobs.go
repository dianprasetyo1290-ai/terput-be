package library

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"skci/ops/internal/client"
)

type JobRunner struct {
	db         *sql.DB
	client     *client.Client
	billingURL string
	internalKey string
	loc        *time.Location
	httpClient *http.Client
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
		httpClient:  &http.Client{Timeout: 8 * time.Second},
	}
}

// RunMemberSync synchronizes active students and staff into library members (FR-113-001/003).
func (j *JobRunner) RunMemberSync(ctx context.Context, schoolID string) (int, error) {
	// 1. Get default categories
	store := NewStore(j.db, j.loc)
	studentCat, err := store.GetDefaultCategory(ctx, schoolID, "student")
	if err != nil {
		return 0, fmt.Errorf("kategori default siswa belum diatur: %w", err)
	}
	employeeCat, err := store.GetDefaultCategory(ctx, schoolID, "employee")
	if err != nil {
		return 0, fmt.Errorf("kategori default pegawai belum diatur: %w", err)
	}

	// 2. Query users from identity via client
	syncedCount := 0
	if j.client != nil {
		// Sync students
		students, err := j.client.UsersWithRole(ctx, schoolID, "siswa")
		if err == nil {
			for _, st := range students {
				memberCode := fmt.Sprintf("LIB-S-%s", st.ID[:8])
				m := Member{
					SchoolID:   schoolID,
					Kind:       "student",
					UserID:     st.ID,
					CategoryID: studentCat.ID,
					MemberCode: memberCode,
					Status:     "active",
				}
				if _, err := store.UpsertMember(ctx, m); err == nil {
					syncedCount++
				}
			}
		}

		// Sync staff (guru, admin, etc.)
		staff, err := j.client.UsersWithRole(ctx, schoolID, "guru")
		if err == nil {
			for _, st := range staff {
				memberCode := fmt.Sprintf("LIB-E-%s", st.ID[:8])
				m := Member{
					SchoolID:   schoolID,
					Kind:       "employee",
					UserID:     st.ID,
					CategoryID: employeeCat.ID,
					MemberCode: memberCode,
					Status:     "active",
				}
				if _, err := store.UpsertMember(ctx, m); err == nil {
					syncedCount++
				}
			}
		}
	}

	return syncedCount, nil
}

// RunDailyTasks handles reminders, expired reservations, and billed fines sync (Section 4.4).
func (j *JobRunner) RunDailyTasks(ctx context.Context, schoolID string) error {
	now := time.Now().In(j.loc)
	store := NewStore(j.db, j.loc)

	// 1. Get settings
	st, err := store.GetSettings(ctx, schoolID)
	if err != nil {
		return err
	}

	// 2. Loan reminders
	rows, err := j.db.QueryContext(ctx, `
		SELECT l.id, l.member_id, l.due_date, m.user_id, t.title
		FROM lib_loans l
		JOIN lib_members m ON m.id = l.member_id
		JOIN lib_copies cp ON cp.id = l.copy_id
		JOIN lib_titles t ON t.id = cp.title_id
		WHERE l.school_id = $1 AND l.status = 'active'
	`, schoolID)
	if err == nil {
		type reminderCandidate struct {
			loanID  string
			userID  string
			dueDate time.Time
			title   string
		}
		var candidates []reminderCandidate
		for rows.Next() {
			var c reminderCandidate
			if err := rows.Scan(&c.loanID, &c.userID, &c.dueDate, &c.title); err == nil {
				candidates = append(candidates, c)
			}
		}
		rows.Close()

		for _, c := range candidates {
			loan := Loan{DueDate: c.dueDate}
			kind := ReminderKind(loan, now, st.ReminderDaysBefore, j.loc)
			if kind != "none" && j.client != nil {
				idempKey := fmt.Sprintf("lib:loan:%s:%s", c.loanID, kind)
				msg := fmt.Sprintf("Buku '%s' akan jatuh tempo pada %s.", c.title, c.dueDate.Format("02 Jan 2006"))
				if kind == "overdue" {
					msg = fmt.Sprintf("Pemberitahuan: Peminjaman buku '%s' telah melewati batas jatuh tempo.", c.title)
				}
				_ = j.client.Notify(ctx, client.Notification{
					SchoolID: schoolID,
					UserID:   c.userID,
					Title:    "Pengingat Peminjaman Perpustakaan",
					Body:     msg,
				}, idempKey)
			}
		}
	}

	// 3. Expired reservations
	expRows, err := j.db.QueryContext(ctx, `
		SELECT id, title_id, held_copy_id
		FROM lib_reservations
		WHERE school_id = $1 AND status = 'ready' AND ready_until < $2::date
	`, schoolID, now)
	if err == nil {
		type expRes struct {
			id         string
			titleID    string
			heldCopyID *string
		}
		var expiredList []expRes
		for expRows.Next() {
			var er expRes
			if err := expRows.Scan(&er.id, &er.titleID, &er.heldCopyID); err == nil {
				expiredList = append(expiredList, er)
			}
		}
		expRows.Close()

		for _, er := range expiredList {
			_, _ = j.db.ExecContext(ctx, `
				UPDATE lib_reservations SET status = 'expired', updated_at = now()
				WHERE id = $1
			`, er.id)

			if er.heldCopyID != nil {
				// Check if another waiting reservation exists for this title
				var nextID, nextMemberID string
				err := j.db.QueryRowContext(ctx, `
					SELECT id, member_id FROM lib_reservations
					WHERE school_id = $1 AND title_id = $2 AND status = 'waiting'
					ORDER BY created_at ASC LIMIT 1
				`, schoolID, er.titleID).Scan(&nextID, &nextMemberID)
				if err == nil {
					// Transfer held copy to next member
					readyUntil := now.AddDate(0, 0, 2)
					_, _ = j.db.ExecContext(ctx, `
						UPDATE lib_reservations SET status = 'ready', held_copy_id = $1, ready_until = $2, updated_at = now()
						WHERE id = $3
					`, *er.heldCopyID, readyUntil, nextID)
				} else {
					// Release copy to available
					_, _ = j.db.ExecContext(ctx, `
						UPDATE lib_copies SET status = 'available', updated_at = now()
						WHERE id = $1
					`, *er.heldCopyID)
				}
			}
		}
	}

	// 4. Sync billed fines with Billing service
	fRows, err := j.db.QueryContext(ctx, `
		SELECT id, bill_id FROM lib_fines
		WHERE school_id = $1 AND status = 'billed' AND bill_id IS NOT NULL
		LIMIT 50
	`, schoolID)
	if err == nil {
		type billedFine struct {
			fineID string
			billID string
		}
		var billedList []billedFine
		for fRows.Next() {
			var bf billedFine
			if err := fRows.Scan(&bf.fineID, &bf.billID); err == nil {
				billedList = append(billedList, bf)
			}
		}
		fRows.Close()

		if len(billedList) > 0 && j.billingURL != "" {
			var fineIDs []string
			for _, bf := range billedList {
				fineIDs = append(fineIDs, bf.fineID)
			}
			// Call GET {BILLING_URL}/internal/bills/by-source
			url := fmt.Sprintf("%s/internal/bills/by-source?school_id=%s&source_module=library&source_refs=%s", j.billingURL, schoolID, fineIDs[0])
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
			if err == nil {
				req.Header.Set("X-Internal-Key", j.internalKey)
				resp, err := j.httpClient.Do(req)
				if err == nil && resp.StatusCode == http.StatusOK {
					var bySourceResp []struct {
						SourceRef string `json:"source_ref"`
						Status    string `json:"status"`
					}
					bodyBytes, _ := io.ReadAll(resp.Body)
					resp.Body.Close()
					if err := json.Unmarshal(bodyBytes, &bySourceResp); err == nil {
						for _, r := range bySourceResp {
							if r.Status == "paid" {
								_ = store.PayFine(ctx, schoolID, r.SourceRef, "billing_sync")
							}
						}
					}
				}
			}
		}
	}

	return nil
}

// BillFineToAccounting sends fine to billing adhoc endpoint (Section 4.3).
func (j *JobRunner) BillFineToAccounting(ctx context.Context, f Fine, studentUserID, titleName string) error {
	if j.billingURL == "" {
		return errors.New("BILLING_URL not configured")
	}

	bodyMap := map[string]any{
		"school_id":       f.SchoolID,
		"student_user_id": studentUserID,
		"source_module":   "library",
		"source_ref":      f.ID,
		"title":           fmt.Sprintf("Denda Perpustakaan: %s", titleName),
		"due_at":          time.Now().AddDate(0, 0, 7).Format("2006-01-02"),
		"items": []map[string]any{
			{
				"name":           fmt.Sprintf("Denda keterlambatan: %s (%d hari)", titleName, *f.LateDays),
				"component_code": "library_fine",
				"amount_idr":     f.AmountIDR,
			},
		},
	}

	jsonBytes, _ := json.Marshal(bodyMap)
	url := fmt.Sprintf("%s/internal/bills/adhoc", j.billingURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(jsonBytes))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Internal-Key", j.internalKey)
	req.Header.Set("Idempotency-Key", fmt.Sprintf("library:%s", f.ID))

	resp, err := j.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		respBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return fmt.Errorf("billing error (%d): %s", resp.StatusCode, string(respBytes))
	}

	var res struct {
		BillID string `json:"bill_id"`
	}
	bodyBytes, _ := io.ReadAll(resp.Body)
	_ = json.Unmarshal(bodyBytes, &res)

	store := NewStore(j.db, j.loc)
	return store.UpdateFineBilled(ctx, f.SchoolID, f.ID, res.BillID)
}

// RunDailyTasksForAll runs daily library background tasks across all known schools.
func (j *JobRunner) RunDailyTasksForAll(ctx context.Context) error {
	rows, err := j.db.QueryContext(ctx, `SELECT DISTINCT school_id FROM lib_settings`)
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

// RunMemberSyncForAll synchronizes student and staff members across all known schools.
func (j *JobRunner) RunMemberSyncForAll(ctx context.Context) error {
	rows, err := j.db.QueryContext(ctx, `SELECT DISTINCT school_id FROM lib_settings`)
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
		_, _ = j.RunMemberSync(ctx, s)
	}
	return nil
}

