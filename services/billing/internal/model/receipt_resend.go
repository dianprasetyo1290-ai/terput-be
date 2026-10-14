package model

import "time"

// DefaultReceiptResendCooldown applies when a school has no gateway_settings row.
const DefaultReceiptResendCooldown = 10 * time.Minute

// ReceiptResendWait reports how long the caller must still wait before the
// receipt last sent at lastSent may be sent again (0 = allowed now). A
// receipt never sent yet can always be sent.
func ReceiptResendWait(lastSent *time.Time, cooldown time.Duration, now time.Time) time.Duration {
	if lastSent == nil || cooldown <= 0 {
		return 0
	}
	next := lastSent.Add(cooldown)
	if !now.Before(next) {
		return 0
	}
	return next.Sub(now)
}
