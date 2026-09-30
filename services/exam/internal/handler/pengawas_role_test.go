package handler

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// handlerRoles reads this package's own source and returns, per exported
// handler method, the roles its first requireRole call allows. Reading the
// source rather than restating the lists is the point: a test that merely
// repeated them would still pass after someone widened a handler.
func handlerRoles(t *testing.T) map[string][]string {
	t.Helper()
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	out := map[string][]string{}
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(token.NewFileSet(), f, nil, 0)
		if err != nil {
			t.Fatalf("%s: %v", f, err)
		}
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Recv == nil || fn.Body == nil {
				continue
			}
			var roles []string
			var found bool
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				if found {
					return false
				}
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				id, ok := call.Fun.(*ast.Ident)
				if !ok || id.Name != "requireRole" {
					return true
				}
				for _, arg := range call.Args[2:] {
					lit, ok := arg.(*ast.BasicLit)
					if !ok || lit.Kind != token.STRING {
						continue
					}
					v, _ := strconv.Unquote(lit.Value)
					roles = append(roles, v)
				}
				found = true
				return false
			})
			if found {
				sort.Strings(roles)
				out[fn.Name.Name] = roles
			}
		}
	}
	if len(out) < 20 {
		t.Fatalf("only found %d gated handlers -- the AST scan is broken, not the code", len(out))
	}
	return out
}

// TestPengawasRoleBoundary pins SEKOCI V11's whole point: Pengawas (panitia
// ujian) may set exam schedules, watch the live monitor and larang/blokir a
// peserta -- and nothing else. Bank soal stays read-only for it (ListFolders
// /GetFolder/ListQuestions/GetQuestion carry no requireRole at all, so they
// are reachable; every write does and must exclude pengawas), and it must
// never touch a soal, a poin or a nilai.
func has(list []string, role string) bool {
	for _, r := range list {
		if r == role {
			return true
		}
	}
	return false
}

func TestPengawasRoleBoundary(t *testing.T) {
	roles := handlerRoles(t)

	mustAllow := []string{
		"CreateExam", "UpdateExam", "DeleteExam", "PublishExam", "UnpublishExam", "DuplicateExam",
		"Monitor", "AttemptDetail",
		"LarangUjian", "LarangUjianByUser", "BukaLarangan", "Blokir", "BukaBlokir",
	}
	mustDeny := []string{
		"CreateQuestion", "UpdateQuestion", "DeleteQuestion", "PublishQuestion",
		"CreateFolder", "UpdateFolder", "DeleteFolder",
		"AddExamQuestion", "RemoveExamQuestion", "UpdateExamQuestionPoints",
		"NormalizeDraftPoints", "ImportQuestions", "OverrideAnswer",
		"CreateExamType", "UpdateExamType", "DeleteExamType",
		"BulkIssueTokens", "IssueToken", "RegenerateToken",
	}

	for _, name := range mustAllow {
		got, ok := roles[name]
		if !ok {
			t.Errorf("%s: no requireRole found -- handler renamed or gate dropped", name)
			continue
		}
		if !has(got, "pengawas") {
			t.Errorf("%s: pengawas should be allowed, got %v", name, got)
		}
	}
	for _, name := range mustDeny {
		got, ok := roles[name]
		if !ok {
			t.Errorf("%s: no requireRole found -- handler renamed or gate dropped", name)
			continue
		}
		if has(got, "pengawas") {
			t.Errorf("%s: pengawas must NOT be allowed, got %v", name, got)
		}
		if has(got, "siswa") {
			t.Errorf("%s: siswa must never be allowed, got %v", name, got)
		}
	}
}

// TestGuruMayActOnOwnExamMonitor pins V23: Pantau Ujian Langsung shows
// "Larang Ujian" and "Buka Larangan" to a guru with no role gating in the
// template, but the handlers excluded guru -- every click came back 403 and
// the UI rendered it as "Gagal memproses aksi. Cek status siswa saat ini".
// Guru belongs here: it already has Blokir/BukaBlokir/Reset/AccNilai, which
// are heavier, and loadAttemptForAction/loadExamForAction pin a guru to
// their own exam.
func TestGuruMayActOnOwnExamMonitor(t *testing.T) {
	roles := handlerRoles(t)
	for _, name := range []string{"LarangUjian", "LarangUjianByUser", "BukaLarangan"} {
		got, ok := roles[name]
		if !ok {
			t.Errorf("%s: no requireRole found -- handler renamed or gate dropped", name)
			continue
		}
		if !has(got, "guru") {
			t.Errorf("%s: guru not allowed, got %v -- the V23 403 is back", name, got)
		}
	}
}
