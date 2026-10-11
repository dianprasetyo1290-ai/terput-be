package calc

import (
	"testing"
	"time"
)

func TestQuizScoreAndPassing(t *testing.T) {
	questions := []Question{
		{Prompt: "Q1", CorrectIndex: 0},
		{Prompt: "Q2", CorrectIndex: 1},
		{Prompt: "Q3", CorrectIndex: 2},
		{Prompt: "Q4", CorrectIndex: 3},
		{Prompt: "Q5", CorrectIndex: 0},
	}

	// 4 correct out of 5 -> 80%
	answers := []int{0, 1, 2, 3, 1}
	score := QuizScore(questions, answers)
	if score != 80 {
		t.Fatalf("expected score 80, got %d", score)
	}

	// AC-101-01: passing 70, score 80 -> passed
	if !Passed(score, 70) {
		t.Fatalf("AC-101-01: expected passed with score 80 and threshold 70")
	}

	// Score 60 with passing 70 -> not passed
	answersFail := []int{0, 1, 2, 0, 1}
	scoreFail := QuizScore(questions, answersFail)
	if scoreFail != 60 {
		t.Fatalf("expected score 60, got %d", scoreFail)
	}
	if Passed(scoreFail, 70) {
		t.Fatalf("expected failed with score 60 and threshold 70")
	}
}

func TestCompliance(t *testing.T) {
	// AC-101-02: 40 assigned, 30 completed -> 7500 bp (75%)
	bp := Compliance(40, 30)
	if bp != 7500 {
		t.Fatalf("AC-101-02: expected 7500 bp, got %d", bp)
	}

	if Compliance(0, 0) != 10000 {
		t.Fatalf("expected 10000 bp for 0 assigned")
	}

	if Compliance(10, 10) != 10000 {
		t.Fatal("expected 10000 bp for 100% completed")
	}
}

func TestCanEnroll(t *testing.T) {
	// AC-99-01: quota 20, taken 20 -> false, "Kuota penuh"
	q20 := 20
	can, reason := CanEnroll(&q20, 20)
	if can || reason != "Kuota penuh" {
		t.Fatalf("AC-99-01: expected false and Kuota penuh, got %v, %s", can, reason)
	}

	canOpen, _ := CanEnroll(&q20, 19)
	if !canOpen {
		t.Fatalf("expected can enroll when quota is not reached")
	}

	canUnlimited, _ := CanEnroll(nil, 500)
	if !canUnlimited {
		t.Fatalf("expected can enroll when quota is nil")
	}
}

func TestNeedsApproval(t *testing.T) {
	// AC-99-02: cost_idr > 0 -> true (e.g. 1.500.000)
	if !NeedsApproval(1500000) {
		t.Fatalf("AC-99-02: expected needs approval for paid course 1.500.000")
	}
	if NeedsApproval(0) {
		t.Fatalf("expected no approval needed for free course")
	}
}

func TestBuildOnboarding(t *testing.T) {
	// AC-100-01: template 8 tasks -> 8 tasks with owner + due date
	tasks := []TemplateTask{
		{Title: "Unggah KTP & NPWP", Category: "dokumen", OwnerKind: "employee", DueOffsetDays: 1},
		{Title: "Buat Akun Email Sekolah", Category: "akun", OwnerKind: "role", OwnerRole: "admin", DueOffsetDays: 0},
		{Title: "Serah Terima Laptop", Category: "perangkat", OwnerKind: "role", OwnerRole: "admin", DueOffsetDays: 2},
		{Title: "Orientasi Budaya Sekolah", Category: "orientasi", OwnerKind: "supervisor", DueOffsetDays: 3},
		{Title: "Review Kurikulum", Category: "pelatihan", OwnerKind: "supervisor", DueOffsetDays: 5},
		{Title: "Pengenalan Guru Rekan", Category: "orientasi", OwnerKind: "supervisor", DueOffsetDays: 2},
		{Title: "Setup Ruang Kerja", Category: "perangkat", OwnerKind: "employee", DueOffsetDays: 1},
		{Title: "Evaluasi Minggu Pertama", Category: "orientasi", OwnerKind: "supervisor", DueOffsetDays: 7},
	}

	startDate := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	supervisorUID := "sup-123"

	built := BuildOnboarding(tasks, startDate, supervisorUID)
	if len(built) != 8 {
		t.Fatalf("AC-100-01: expected 8 tasks built, got %d", len(built))
	}

	// Verify supervisor task
	if built[3].OwnerUserID == nil || *built[3].OwnerUserID != supervisorUID {
		t.Fatalf("expected supervisor user id assigned")
	}
	// Verify offset days
	expectedDue := startDate.AddDate(0, 0, 7)
	if !built[7].DueDate.Equal(expectedDue) {
		t.Fatalf("expected due date %v, got %v", expectedDue, built[7].DueDate)
	}
}
