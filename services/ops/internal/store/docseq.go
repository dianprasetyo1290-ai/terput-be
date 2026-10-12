package store

import (
	"context"
	"fmt"
)

// NextDocNumber allocates the next document number for (school, prefix,
// year), e.g. WO-2026-0001. Call inside the transaction that inserts the
// document so a rollback does not burn a number.
func NextDocNumber(ctx context.Context, q Q, schoolID, prefix string, year int) (string, error) {
	var last int
	err := q.QueryRowContext(ctx, `INSERT INTO doc_sequences (school_id, prefix, year, last)
		VALUES ($1, $2, $3, 1)
		ON CONFLICT (school_id, prefix, year) DO UPDATE SET last = doc_sequences.last + 1
		RETURNING last`, schoolID, prefix, year).Scan(&last)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s-%d-%04d", prefix, year, last), nil
}
