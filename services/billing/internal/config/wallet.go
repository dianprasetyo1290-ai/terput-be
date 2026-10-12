package config

import "os"

// Wallet holds the canteen/wallet (T12) settings. Business rules (balance
// caps, offline window, account codes) live in DB tables per school, not here.
type Wallet struct {
	// OpsURL is ops-service (T9) for room validation; empty = rooms are
	// stored without validation.
	OpsURL string
	// CardSecret derives student card codes (HMAC). Falls back to the JWT
	// secret, which is already mandatory in production.
	CardSecret string
}

func loadWallet(jwtSecret string) Wallet {
	w := Wallet{OpsURL: os.Getenv("OPS_URL"), CardSecret: os.Getenv("STUDENT_CARD_SECRET")}
	if w.CardSecret == "" {
		w.CardSecret = jwtSecret
	}
	return w
}
