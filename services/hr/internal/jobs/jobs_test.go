package jobs

import (
	"testing"
)

func TestHashJobName(t *testing.T) {
	h1 := hashJobName("contract_reminders")
	h2 := hashJobName("contract_reminders")
	h3 := hashJobName("document_reminders")

	if h1 != h2 {
		t.Fatalf("job name hash not deterministic: %d vs %d", h1, h2)
	}
	if h1 == h3 {
		t.Fatalf("different job names produced identical hash: %d", h1)
	}
}
