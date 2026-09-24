package handler

import (
	"testing"

	"skci/exam/internal/model"
)

// Yadika13 item 2: a published/ongoing exam accepts a schedule change but
// nothing else. changedBesidesSchedule decides that, so it has to be exact —
// a false positive locks a guru out of moving the clock mid-exam, a false
// negative lets a field through that in-flight attempts depend on.
func TestChangedBesidesSchedule(t *testing.T) {
	existing := &model.Exam{
		Title:        "STS Matematika",
		TermID:       "term-1",
		ExamTypeID:   "type-sts",
		SubjectID:    "subj-mtk",
		TeacherID:    "guru-1",
		IsShuffled:   true,
		ClassroomID:  "kelas-a",
		ClassroomIDs: []string{"kelas-a", "kelas-b"},
	}
	base := examReq{
		Title:        "STS Matematika",
		TermID:       "term-1",
		ExamTypeID:   "type-sts",
		SubjectID:    "subj-mtk",
		TeacherID:    "guru-1",
		IsShuffled:   true,
		ClassroomIDs: []string{"kelas-a", "kelas-b"},
	}

	if got := changedBesidesSchedule(existing, base); got != "" {
		t.Errorf("identical payload must be schedule-only, got %q", got)
	}

	// Same classrooms, different order — not a change.
	reordered := base
	reordered.ClassroomIDs = []string{"kelas-b", "kelas-a"}
	if got := changedBesidesSchedule(existing, reordered); got != "" {
		t.Errorf("reordered kelas must not count as a change, got %q", got)
	}

	// An omitted field means "unchanged", not "cleared" — the frontend's
	// schedule-only save must not trip on fields it didn't send.
	if got := changedBesidesSchedule(existing, examReq{IsShuffled: true}); got != "" {
		t.Errorf("omitted fields must not count as changes, got %q", got)
	}

	for name, mutate := range map[string]func(*examReq){
		"Judul":          func(r *examReq) { r.Title = "STS Matematika (revisi)" },
		"Mata Pelajaran": func(r *examReq) { r.SubjectID = "subj-lain" },
		"Guru":           func(r *examReq) { r.TeacherID = "guru-2" },
		"Acak Soal":      func(r *examReq) { r.IsShuffled = false },
		"Tipe Ujian":     func(r *examReq) { r.ExamTypeID = "type-sas" },
	} {
		req := base
		mutate(&req)
		if got := changedBesidesSchedule(existing, req); got != name {
			t.Errorf("changing %s: want %q, got %q", name, name, got)
		}
	}

	// Dropping a classroom is a change — an in-flight attempt belongs to one.
	fewer := base
	fewer.ClassroomIDs = []string{"kelas-a"}
	if got := changedBesidesSchedule(existing, fewer); got != "Kelas" {
		t.Errorf("removing a kelas: want \"Kelas\", got %q", got)
	}

	// Swapping a classroom keeps the count but is still a change.
	swapped := base
	swapped.ClassroomIDs = []string{"kelas-a", "kelas-c"}
	if got := changedBesidesSchedule(existing, swapped); got != "Kelas" {
		t.Errorf("swapping a kelas: want \"Kelas\", got %q", got)
	}
}
