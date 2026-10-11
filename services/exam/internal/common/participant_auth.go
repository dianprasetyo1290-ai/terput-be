package common

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"strings"

	"github.com/golang-jwt/jwt/v5"

	"skci/exam/internal/model"
)

// ParticipantScopeExamAttempt is the only scope an applicant CBT token may
// carry. A token with any other scope is refused by the middleware.
const ParticipantScopeExamAttempt = "exam:attempt"

type ParticipantClaims struct {
	SchoolID        string `json:"school_id"`
	ExamID          string `json:"exam_id"`
	ParticipantType string `json:"participant_type"`
	Scope           string `json:"scope"`
	jwt.RegisteredClaims
}

const ParticipantClaimsCtxKey ctxKey = "participant_claims"

// ParticipantJWTMiddleware accepts only admission applicant tokens with the
// exam:attempt scope. Normal identity JWTs cannot be replayed on these routes.
//
// participant_type is checked through model.ValidParticipantType (an exact
// match on the closed 'student'|'applicant' union) rather than a hand-written
// comparison, so the claim is validated in the same place the DB CHECK and the
// audit trail agree on — see model.ParticipantTypeLen.
func ParticipantJWTMiddleware(secret string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			tokenStr := bearerToken(r.Header.Get("Authorization"))
			claims := &ParticipantClaims{}
			token, err := jwt.ParseWithClaims(tokenStr, claims, func(t *jwt.Token) (interface{}, error) {
				if t.Method.Alg() != jwt.SigningMethodHS256.Alg() {
					return nil, fmt.Errorf("unexpected signing method")
				}
				return []byte(secret), nil
			})
			if tokenStr == "" || err != nil || !token.Valid ||
				claims.ParticipantType != model.ParticipantApplicant || claims.Scope != ParticipantScopeExamAttempt {
				WriteError(w, http.StatusUnauthorized, "invalid or expired participant token")
				return
			}
			ctx := context.WithValue(r.Context(), ParticipantClaimsCtxKey, claims)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// bearerToken extracts the credential from an Authorization header, accepting
// any casing of the "Bearer" scheme. Returns "" when the header is absent or
// carries a different scheme — never a half-stripped value, which would turn a
// malformed header into a lookup of some other token's hash.
func bearerToken(header string) string {
	h := strings.TrimSpace(header)
	if len(h) < 7 || !strings.EqualFold(h[:7], "Bearer ") {
		return ""
	}
	return strings.TrimSpace(h[7:])
}

func ParticipantFromCtx(r *http.Request) *ParticipantClaims {
	c, _ := r.Context().Value(ParticipantClaimsCtxKey).(*ParticipantClaims)
	return c
}

func TokenHash(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}
