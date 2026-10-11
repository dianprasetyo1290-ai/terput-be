package calc

import (
	"fmt"
	"time"
)

type Question struct {
	ID           string   `json:"id"`
	Prompt       string   `json:"prompt"`
	Options      []string `json:"options"`
	CorrectIndex int      `json:"correct_index"`
}

type TemplateTask struct {
	Title          string `json:"title"`
	Category       string `json:"category"`
	OwnerKind      string `json:"owner_kind"` // role, supervisor, employee
	OwnerRole      string `json:"owner_role"`
	DueOffsetDays  int    `json:"due_offset_days"`
}

type BuiltTask struct {
	Title       string
	Category    string
	OwnerUserID *string
	OwnerRole   *string
	DueDate     time.Time
}

// QuizScore calculates percentage score (0-100) based on correct answers.
// AC-101-01: passing 70, score 80 -> completed
func QuizScore(questions []Question, answers []int) int {
	if len(questions) == 0 {
		return 100
	}
	correct := 0
	for i, q := range questions {
		if i < len(answers) && answers[i] == q.CorrectIndex {
			correct++
		}
	}
	return (correct * 100) / len(questions)
}

// Passed returns whether the score meets or exceeds the passing score threshold.
func Passed(score int, passingScore int) bool {
	return score >= passingScore
}

// Compliance returns compliance percentage in basis points (100 bp = 1%).
// AC-101-02: 40 assigned & 30 completed -> 7500 bp (75%).
func Compliance(assigned int, completed int) int {
	if assigned <= 0 {
		return 10000 // 100% compliant if nothing assigned
	}
	if completed >= assigned {
		return 10000
	}
	return int((int64(completed) * 10000) / int64(assigned))
}

// CanEnroll checks whether a course/session still has available quota.
// AC-99-01: quota 20, taken 20 -> false, "Kuota penuh"
func CanEnroll(quota *int, taken int) (bool, string) {
	if quota != nil && taken >= *quota {
		return false, "Kuota penuh"
	}
	return true, ""
}

// NeedsApproval determines if training enrollment needs approval from direct supervisor.
// AC-99-02: cost_idr > 0 -> true (needs approval, e.g. 1.500.000 -> pending_approval)
func NeedsApproval(costIDR int64) bool {
	return costIDR > 0
}

// BuildOnboarding generates concrete onboarding tasks from a template.
// AC-100-01: template 8 tasks -> 8 tasks with assigned owner & due date
func BuildOnboarding(templateTasks []TemplateTask, startDate time.Time, supervisorUserID string) []BuiltTask {
	var built []BuiltTask
	for _, t := range templateTasks {
		dueDate := startDate.AddDate(0, 0, t.DueOffsetDays)
		var ownerUID *string
		var ownerRole *string

		switch t.OwnerKind {
		case "supervisor":
			if supervisorUserID != "" {
				s := supervisorUserID
				ownerUID = &s
			}
		case "role":
			if t.OwnerRole != "" {
				r := t.OwnerRole
				ownerRole = &r
			}
		case "employee":
			// Handled dynamically by the employee themselves
		}

		built = append(built, BuiltTask{
			Title:       t.Title,
			Category:    t.Category,
			OwnerUserID: ownerUID,
			OwnerRole:   ownerRole,
			DueDate:     dueDate,
		})
	}
	return built
}

// DisplayStatus formats assignment status taking into account due date.
func DisplayStatus(status string, dueDate *time.Time, today time.Time) string {
	if status == "completed" {
		return "selesai"
	}
	if dueDate != nil && today.After(*dueDate) {
		return "terlambat"
	}
	switch status {
	case "in_progress":
		return "berjalan"
	case "not_started":
		return "belum mulai"
	case "pending_approval":
		return "menunggu approval"
	default:
		return status
	}
}

func FormatCompliancePercent(bp int) string {
	return fmt.Sprintf("%.1f%%", float64(bp)/100.0)
}
