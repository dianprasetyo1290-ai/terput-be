package common

import (
	"log"
	"strings"
)

// technicalMarkers are fragments of driver, SQL and Go runtime errors. A
// handler that writes `"gagal ...: " + err.Error()` would otherwise send
// them to the browser (QA putaran 2: "tidak ada temuan di server").
var technicalMarkers = []string{
	"sqlstate", "pq: ", "pgx", "pgconn", "error: ", "violates", "constraint \"",
	"relation \"", "column \"", "syntax error", "invalid input syntax",
	"no rows in result set", "sql: ", "dial tcp", "connection refused",
	"deadline exceeded", "unexpected eof", "json: ", "strconv.", "runtime error",
	"duplicate key", "<nil>", "context canceled", "nil pointer", "i/o timeout",
	"cannot unmarshal", "conn closed", "conn busy", "operator does not exist",
	"out of range for type", "invalid uuid", "uuid: ", "failed to connect",
	"tls:", "x509:", "unexpected end of json", "goroutine ", "panic:",
	"permission denied for", "function ", "cannot scan", "cannot convert",
	"expected ", "got ", "parsing time", "no such file", "is not a valid",
}

func looksTechnical(msg string) bool {
	m := strings.ToLower(msg)
	for _, k := range technicalMarkers {
		if strings.Contains(m, k) {
			return true
		}
	}
	return false
}

const (
	genericServerMessage  = "Terjadi kesalahan pada server. Silakan coba lagi."
	genericRequestMessage = "Permintaan tidak dapat diproses. Periksa kembali data yang dikirim."
)

// SafeMessage is what an error response may show the user. A message that
// carries a raw technical error is logged and cut back to the handler's own
// prefix ("gagal memuat data: pq: ..." -> "gagal memuat data") or, when the
// whole text is technical, a generic sentence. 5xx responses always drop
// the part after the first ": ", which is where handlers append err.Error().
func SafeMessage(status int, msg string) string {
	technical := looksTechnical(msg)
	if !technical && (status < 500 || !strings.Contains(msg, ": ")) {
		if msg == "" && status >= 500 {
			return genericServerMessage
		}
		return msg
	}
	log.Printf("error response %d (sanitized): %s", status, msg)
	if i := strings.Index(msg, ": "); i > 0 {
		if prefix := strings.TrimSpace(msg[:i]); len(prefix) >= 6 && !looksTechnical(prefix) {
			return prefix
		}
	}
	if status >= 500 {
		return genericServerMessage
	}
	return genericRequestMessage
}
