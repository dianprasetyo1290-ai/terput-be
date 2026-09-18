// Package cache is a generic HTTP-response cache for GET requests, backed
// by Redis (client request 2026-08-29: "cache semua di redis"). Fails open
// by design: any Redis error (unreachable, timeout, bad REDIS_URL) just
// skips caching for that request rather than breaking the API -- caching
// is an optimization, never a hard dependency.
//
// Cache key scoping (security-critical): every key includes the caller's
// school_id AND user_id, not just the URL. Two different schools (or two
// different users within the same school) must never share a cache entry
// -- keying on URL alone would risk serving one tenant's or one user's
// cached response to another. This costs some cache-sharing opportunity
// (each user gets their own cached copies rather than sharing with every
// other user at their school) in exchange for being safe by construction.
//
// Content-Type/Content-Disposition are captured and replayed too, not
// just the body -- some GET endpoints here return file downloads
// (question import template) rather than JSON; a cache hit that forced
// application/json on those would corrupt the download.
package cache

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"

	"skci/exam/internal/common"
)

const keyPrefix = "httpcache:exam:"

type Cache struct {
	rdb *redis.Client
	ttl time.Duration
}

// New parses redisURL (e.g. "redis://redis:6379/0"). An empty URL or a
// parse/connect failure yields a Cache with a nil client -- Middleware
// then no-ops on every request (fail open, see package doc).
func New(redisURL string, ttl time.Duration) *Cache {
	if redisURL == "" {
		return &Cache{ttl: ttl}
	}
	opt, err := redis.ParseURL(redisURL)
	if err != nil {
		return &Cache{ttl: ttl}
	}
	return &Cache{rdb: redis.NewClient(opt), ttl: ttl}
}

type recorder struct {
	http.ResponseWriter
	status int
	body   strings.Builder
}

func (r *recorder) WriteHeader(status int) {
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}

func (r *recorder) Write(b []byte) (int, error) {
	r.body.Write(b)
	return r.ResponseWriter.Write(b)
}

func schoolScope(r *http.Request) string {
	if c := common.FromCtx(r); c != nil {
		return c.SchoolID
	}
	return "anon"
}

// versionKey holds a per-school counter that is part of every cache key
// and is bumped on every mutating request. Bumping it orphans that
// school's whole cached set at once (the orphans then die on their own
// TTL), which is what makes a write immediately visible to the very next
// read instead of up to `ttl` later.
//
// Without this, a guru who saved a third ujian and opened Bank Soal within
// the 15s window got the cached two-row list back, while the dashboard's
// own (differently-keyed) count already said three -- reported as "di
// dashboard ada 3 total ujian, tapi di bank soal cuma ada 2". Same class of
// bug report-service already fixed this same way in SEKOCI V8.
func versionKey(schoolID string) string { return keyPrefix + "ver:" + schoolID }

func (c *Cache) version(ctx context.Context, schoolID string) string {
	v, err := c.rdb.Get(ctx, versionKey(schoolID)).Result()
	if err != nil {
		return "0"
	}
	return v
}

func cacheKey(r *http.Request, version string) string {
	scope := "anon"
	if c := common.FromCtx(r); c != nil {
		scope = c.SchoolID + ":" + c.UserID
	}
	sum := sha256.Sum256([]byte(r.Method + " " + r.URL.Path + "?" + r.URL.RawQuery + " " + scope + " v" + version))
	return keyPrefix + hex.EncodeToString(sum[:])
}

func isMutation(method string) bool {
	switch method {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return true
	}
	return false
}

// Middleware caches 200 responses to GET requests as a Redis hash (body +
// Content-Type + Content-Disposition, so file downloads replay correctly
// from cache too). skip lets a route opt out (live/polling data that must
// never be stale -- see each service's main.go/router.go for its own
// bypass list) — return true to bypass the cache entirely for that, and
// to leave the invalidation counter alone on writes to that path,
// request.
func (c *Cache) Middleware(skip func(*http.Request) bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if c.rdb == nil {
				next.ServeHTTP(w, r)
				return
			}
			if isMutation(r.Method) {
				next.ServeHTTP(w, r)
				// Bumped unconditionally, including for a write that
				// failed: the cost is one extra cache miss, versus
				// serving a stale body if a status check ever got the
				// success/failure call wrong.
				//
				// ...except on the skip paths. Those are /attempt and
				// /monitor: a siswa answering a question writes on every
				// keystroke-ish interval and changes nothing a guru's
				// exam/folder/question list shows, so bumping there would
				// flush the whole school's cache continuously for the
				// entire duration of every exam -- the one moment the
				// cache actually earns its keep.
				if skip == nil || !skip(r) {
					c.rdb.Incr(context.Background(), versionKey(schoolScope(r)))
				}
				return
			}
			if r.Method != http.MethodGet || (skip != nil && skip(r)) {
				next.ServeHTTP(w, r)
				return
			}
			key := cacheKey(r, c.version(r.Context(), schoolScope(r)))
			if fields, err := c.rdb.HGetAll(r.Context(), key).Result(); err == nil && len(fields) > 0 {
				if ct := fields["ct"]; ct != "" {
					w.Header().Set("Content-Type", ct)
				}
				if cd := fields["cd"]; cd != "" {
					w.Header().Set("Content-Disposition", cd)
				}
				w.Header().Set("X-Cache", "HIT")
				_, _ = io.WriteString(w, fields["body"])
				return
			}
			rec := &recorder{ResponseWriter: w, status: http.StatusOK}
			next.ServeHTTP(rec, r)
			if rec.status != http.StatusOK || rec.body.Len() == 0 {
				return
			}
			fields := map[string]interface{}{"body": rec.body.String()}
			if ct := rec.Header().Get("Content-Type"); ct != "" {
				fields["ct"] = ct
			}
			if cd := rec.Header().Get("Content-Disposition"); cd != "" {
				fields["cd"] = cd
			}
			// Best-effort: a failed Set means the next request just misses
			// the cache too, not an error the caller ever sees.
			ctx := context.Background()
			pipe := c.rdb.Pipeline()
			pipe.HSet(ctx, key, fields)
			pipe.Expire(ctx, key, c.ttl)
			_, _ = pipe.Exec(ctx)
		})
	}
}
