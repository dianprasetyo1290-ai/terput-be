# Sekoci Go+Vue Rewrite — Shared Conventions

Rewrite of the sekoci school-management platform (currently a Laravel monolith +
Vue admin, prod at sekoci domain on 103.92.215.3) into Go microservices + Vue,
built and run locally first. Source of truth for requirements:
`~/Downloads/Revamp Sekoci.docx` (4 roles: Superadmin, Admin, Guru, Siswa).

Each service is a **fully independent Go module** (own go.mod, own Postgres
database, own port, own docker-compose.yml for standalone dev). No shared Go
package — copy the small snippets below into each service's own
`internal/common` package. This is deliberate: it lets services be built in
parallel with zero file contention.

## Services & ports

| Service     | Dir                    | Port | DB name         | Owns |
|-------------|-------------------------|------|------------------|------|
| identity    | services/identity        | 8081 | skci_identity    | auth, users, roles, schools, foundations, organization/position, bulk school import |
| academic    | services/academic        | 8082 | skci_academic    | academic years/terms, classrooms, subjects, schedules, enrollments, subject matrix |
| attendance  | services/attendance      | 8083 | skci_attendance  | student/teacher presensi, daily teaching agenda |
| exam        | services/exam            | 8084 | skci_exam        | CBT/TBK: bank soal, exams, questions, tokens/vouchers, attempts, anti-cheat |
| evaluation  | services/evaluation      | 8085 | skci_evaluation  | RPM (lesson plans), Penilaian Guru (teacher evaluation) |
| report      | services/report          | 8086 | skci_report      | E-raport / report cards |

Postgres user/password for local dev: `sekoci` / `change-me` (matches each
service's own `docker-compose.yml`, host port = `54` + service port's last two
digits, e.g. identity → `5481`).

Cross-service data (e.g. exam-service needs a student's name from identity) is
fetched via plain internal HTTP calls to the other service's REST API — never
direct DB access across services. Each service authenticates its internal
calls with the same service-to-service header: `X-Internal-Key: <SEKOCI_INTERNAL_KEY env, default "change-me">`.

## Auth (JWT, HS256)

Shared dev secret: `SEKOCI_JWT_SECRET` env var, default
`"change-me"`. Identity-service issues the token on
login; every other service only verifies it (no login logic elsewhere).

Claims:
```json
{
  "sub": "<user_id>",
  "school_id": "<school_id or null for superadmin>",
  "active_role": "superadmin|admin|guru|siswa",
  "roles": ["guru", "walikelas"],
  "exp": 1234567890
}
```
`roles` carries the jabatan-only roles (Walikelas, Kepala Sekolah, Kepala
Yayasan) that ride alongside `active_role` — they don't get their own
dashboard, per the spec, but gate specific actions (e.g. Kepala Sekolah in
teacher-evaluation). `active_role` is what a Kepala Sekolah switches via the
"switch role without logout" feature — same token, identity-service exposes
`POST /auth/switch-role` to reissue a token with a different `active_role`
from the same underlying account.

Go middleware every service should implement (paste into
`internal/common/auth.go`):
```go
package common

import (
	"context"
	"net/http"
	"strings"

	"github.com/golang-jwt/jwt/v5"
)

type Claims struct {
	UserID     string   `json:"sub"`
	SchoolID   string   `json:"school_id"`
	ActiveRole string   `json:"active_role"`
	Roles      []string `json:"roles"`
	jwt.RegisteredClaims
}

type ctxKey string

const ClaimsCtxKey ctxKey = "claims"

func JWTMiddleware(secret string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authz := r.Header.Get("Authorization")
			tokenStr := strings.TrimPrefix(authz, "Bearer ")
			if tokenStr == "" {
				WriteError(w, http.StatusUnauthorized, "missing token")
				return
			}
			claims := &Claims{}
			token, err := jwt.ParseWithClaims(tokenStr, claims, func(t *jwt.Token) (interface{}, error) {
				return []byte(secret), nil
			})
			if err != nil || !token.Valid {
				WriteError(w, http.StatusUnauthorized, "invalid token")
				return
			}
			ctx := context.WithValue(r.Context(), ClaimsCtxKey, claims)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func FromCtx(r *http.Request) *Claims {
	c, _ := r.Context().Value(ClaimsCtxKey).(*Claims)
	return c
}
```

## Response envelope

Every JSON response, success or error, uses this shape (matches what the
existing admin-web Vue frontend already expects, so the new Vue app stays
consistent):
```json
{ "data": <payload or null>, "message": "string", "errors": <object or null> }
```
Paginated list responses put pagination in `meta` alongside `data`:
```json
{ "data": [...], "meta": { "page": 1, "per_page": 20, "total": 123 }, "message": "", "errors": null }
```
Paste into `internal/common/response.go`:
```go
package common

import (
	"encoding/json"
	"net/http"
)

type Envelope struct {
	Data    interface{} `json:"data"`
	Meta    interface{} `json:"meta,omitempty"`
	Message string      `json:"message"`
	Errors  interface{} `json:"errors"`
}

func WriteJSON(w http.ResponseWriter, status int, data interface{}, meta interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(Envelope{Data: data, Meta: meta, Message: "", Errors: nil})
}

func WriteError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(Envelope{Data: nil, Message: msg, Errors: map[string]string{"message": msg}})
}
```

## Tech stack (all services)

- Go 1.22+, `chi` router (`github.com/go-chi/chi/v5`)
- `github.com/jackc/pgx/v5` + `database/sql` (via `pgx/v5/stdlib`) for Postgres
- `github.com/golang-migrate/migrate/v4` for schema migrations, files in
  `services/<name>/migrations/*.sql` (up/down pairs, numbered)
- `github.com/golang-jwt/jwt/v5` for JWT
- `github.com/go-chi/cors` for CORS (allow `http://localhost:5173` — the Vue dev server)
- Standard layout: `cmd/server/main.go`, `internal/handler`, `internal/store`, `internal/model`, `migrations/`, `Dockerfile`, `docker-compose.yml`, `.env.example`, `README.md`
- Every service exposes `GET /healthz` (no auth) returning `{"status":"ok"}`
- All list endpoints support `?page=&per_page=` and relevant filters called out per-service
- IDs: UUID (`gen_random_uuid()`, enable `pgcrypto` extension in first migration)

## Multi-tenancy rule

Every table that isn't global (i.e. everything except identity's own
`schools`/`foundations`) has a `school_id` column, and every query is scoped
to the caller's `school_id` from the JWT claims — except when `active_role ==
"superadmin"` and an explicit `?school_id=` filter/impersonation header is
given. This is the rule that keeps "Sekolah A tidak bisa melihat data sekolah
B" (from the spec) true everywhere.

## Academic-year scoping rule

Every academic table also carries `academic_year_id`. Default queries filter
to the school's *current* academic year; a `?academic_year_id=` query param
lets admin view prior/archived years without mixing data (this is called out
explicitly in the spec as a pain point in the current system).
