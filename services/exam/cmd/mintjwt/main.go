// mintjwt is a dev-only helper that mints a JWT matching CONVENTIONS.md's
// claims shape, for curl-testing exam-service without a running
// identity-service. Usage: go run ./cmd/mintjwt <role> [sub] [school_id]
package main

import (
	"fmt"
	"os"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func main() {
	role := "admin"
	if len(os.Args) > 1 {
		role = os.Args[1]
	}
	sub := "11111111-1111-1111-1111-111111111111"
	if len(os.Args) > 2 {
		sub = os.Args[2]
	}
	schoolID := "22222222-2222-2222-2222-222222222222"
	if len(os.Args) > 3 {
		schoolID = os.Args[3]
	}
	claims := jwt.MapClaims{
		"sub":         sub,
		"school_id":   schoolID,
		"active_role": role,
		"roles":       []string{},
		"exp":         time.Now().Add(24 * time.Hour).Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	s, err := token.SignedString([]byte("change-me"))
	if err != nil {
		panic(err)
	}
	fmt.Println(s)
}
