package store

import "testing"

// The two bulk callers pass a different number of args, so the appended
// classroom placeholder must be numbered from the arg count, never hardcoded.
func TestWithClassroomFilter(t *testing.T) {
	// BukaBlokirBulk shape: one arg (exam_id = $1).
	q, args := withClassroomFilter("UPDATE x WHERE exam_id=$1", "kelas-a", []interface{}{"exam-1"})
	if want := "UPDATE x WHERE exam_id=$1 AND classroom_id=$2"; q != want {
		t.Errorf("got %q, want %q", q, want)
	}
	if len(args) != 2 || args[1] != "kelas-a" {
		t.Errorf("classroom must be the last arg, got %v", args)
	}

	// BlokirBulk shape: three args (exam_id = $3).
	q, args = withClassroomFilter("UPDATE x WHERE exam_id=$3", "kelas-a", []interface{}{nil, "guru-1", "exam-1"})
	if want := "UPDATE x WHERE exam_id=$3 AND classroom_id=$4"; q != want {
		t.Errorf("got %q, want %q", q, want)
	}
	if len(args) != 4 {
		t.Errorf("want 4 args, got %d", len(args))
	}

	// "Semua Kelas" must leave the statement untouched.
	q, args = withClassroomFilter("UPDATE x WHERE exam_id=$1", "", []interface{}{"exam-1"})
	if q != "UPDATE x WHERE exam_id=$1" || len(args) != 1 {
		t.Errorf("empty classroom must not narrow: %q %v", q, args)
	}
}
