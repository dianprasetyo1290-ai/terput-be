package store

import "testing"

// TestDistributeEqualPoints asserts the sum is ALWAYS exactly 100 — including
// counts (3, 7) that a naive 100.0/n float division would leave at
// 99.99999999999999 — and that no question gets a negative or zero share
// for any n up to 100.
func TestDistributeEqualPoints(t *testing.T) {
	for _, n := range []int{1, 2, 3, 4, 5, 6, 7, 9, 10, 11, 13, 17, 20, 33, 50, 99, 100} {
		got := distributeEqualPoints(n)
		if len(got) != n {
			t.Fatalf("n=%d: got %d points, want %d", n, len(got), n)
		}
		sum := 0.0
		for _, p := range got {
			if p <= 0 {
				t.Fatalf("n=%d: non-positive point value %v in %v", n, p, got)
			}
			sum += p
		}
		if sum != 100 {
			t.Fatalf("n=%d: sum = %v, want exactly 100 (points: %v)", n, sum, got)
		}
	}
}

func TestDistributeEqualPointsZero(t *testing.T) {
	if got := distributeEqualPoints(0); got != nil {
		t.Fatalf("n=0: got %v, want nil", got)
	}
}
