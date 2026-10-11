package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"unicode/utf16"

	"github.com/microsoft/typescript-go/tynelapi"
)

func session(t *testing.T, source string) *tynelapi.Session {
	t.Helper()
	s, err := tynelapi.New(source)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close)
	return s
}

func TestRealTypeQueries(t *testing.T) {
	for _, tc := range []struct{ name, source, needle, want string }{
		{"addition", "function add(a: number, b: number): number { return a + b; } const answer: number = add(40, 2); answer;", "answer;", "number"},
		{"string", "let value: string = 'ok'; value;", "value;", "string"},
		{"shadowed-binding", "let value: number = 1; function inner(): string { let value: string = 'ok'; return value; }", "value;", "string"},
		{"narrowed", "function f(value: number | undefined): number { if (value !== undefined) { return value; } return 0; }", "value;", "number"},
		{"library", "const values: number[] = [1, 2]; values;", "values;", "number[]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := session(t, tc.source)
			snap, err := s.Snapshot()
			if err != nil || len(snap.Diagnostics) != 0 {
				t.Fatalf("%v: %+v", err, snap.Diagnostics)
			}
			f, err := s.TypeAt(strings.LastIndex(tc.source, tc.needle))
			if err != nil || f.Display != tc.want {
				t.Fatalf("fact=%+v err=%v", f, err)
			}
		})
	}
}

func TestDeclaredAndNarrowedTypesDiffer(t *testing.T) {
	source := "function f(value: number | undefined): number { if (value !== undefined) { return value; } return 0; }"
	s := session(t, source)
	decl, err := s.TypeAt(strings.Index(source, "value:"))
	if err != nil {
		t.Fatal(err)
	}
	use, err := s.TypeAt(strings.LastIndex(source, "value;"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(decl.Display, "undefined") || !strings.Contains(decl.Display, "number") || use.Display != "number" {
		t.Fatalf("declaration=%+v use=%+v", decl, use)
	}
}

func TestDiagnosticsAreRealAndOwned(t *testing.T) {
	s := session(t, "const answer: number = 'bad';")
	snap, err := s.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if len(snap.Diagnostics) != 1 || snap.Diagnostics[0].Code != 2322 {
		t.Fatalf("unexpected diagnostics %+v", snap.Diagnostics)
	}
	snap.Diagnostics[0].Message = "modified by consumer"
	fresh, _ := s.Snapshot()
	if fresh.Diagnostics[0].Message == snap.Diagnostics[0].Message {
		t.Fatal("diagnostic aliases session storage")
	}
	if _, err := s.TypeAt(6); !errors.Is(err, tynelapi.ErrSourceDiagnostics) {
		t.Fatalf("query on invalid program: %v", err)
	}
}

func TestSyntaxAndUnresolvedImports(t *testing.T) {
	for _, source := range []string{"function broken( {", "import { missing } from './not-on-disk'; missing;"} {
		s := session(t, source)
		snap, err := s.Snapshot()
		if err != nil || len(snap.Diagnostics) == 0 {
			t.Fatalf("accepted invalid input %q: %v", source, err)
		}
	}
}

func TestUnicodeCoordinates(t *testing.T) {
	source := "// 😀\r\nconst 名: number = 1; /*😀*/ 名;"
	s := session(t, source)
	offset := strings.LastIndex(source, "名;")
	fact, err := s.TypeAt(offset)
	if err != nil {
		t.Fatal(err)
	}
	lineStart := strings.LastIndex(source[:offset], "\n") + 1
	wantColumn := len(utf16.Encode([]rune(source[lineStart:offset]))) + 1
	if fact.Display != "number" || fact.Start.ByteOffset != offset || fact.Start.Line != 2 || fact.Start.UTF16Column != wantColumn {
		t.Fatalf("bad coordinate %+v want column %d", fact, wantColumn)
	}
	if _, err := s.TypeAt(offset + 1); err == nil {
		t.Fatal("accepted offset inside UTF-8 character")
	}
}

func TestSessionLifetimeAndSnapshot(t *testing.T) {
	source := "let x: number = 1; x;"
	s := session(t, source)
	f, err := s.TypeAt(strings.LastIndex(source, "x;"))
	if err != nil {
		t.Fatal(err)
	}
	before, _ := json.Marshal(f)
	s.Close()
	s.Close()
	if _, err := s.TypeAt(0); !errors.Is(err, tynelapi.ErrClosed) {
		t.Fatalf("query after close: %v", err)
	}
	if _, err := s.Snapshot(); !errors.Is(err, tynelapi.ErrClosed) {
		t.Fatalf("snapshot after close: %v", err)
	}
	after, _ := json.Marshal(f)
	if !bytes.Equal(before, after) {
		t.Fatal("owned fact changed after close")
	}
	other := session(t, "let x: string = 'new'; x;")
	next, err := other.TypeAt(len("let x: string = 'new'; "))
	if err != nil || next.Display != "string" || next.SourceSHA256 == f.SourceSHA256 {
		t.Fatalf("cross-session contamination: %+v %v", next, err)
	}
}

func TestConcurrentQueriesSerialized(t *testing.T) {
	s := session(t, "let x: number = 1; x;")
	var wg sync.WaitGroup
	for range 12 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			f, err := s.TypeAt(19)
			if err != nil || f.Display != "number" {
				t.Errorf("query=%+v error=%v", f, err)
			}
		}()
	}
	wg.Wait()
}

func TestLimitsAndInvalidQueries(t *testing.T) {
	for _, source := range []string{strings.Repeat(" ", tynelapi.MaxSourceBytes+1), string([]byte{0xff})} {
		if _, err := tynelapi.New(source); err == nil {
			t.Fatal("invalid source accepted")
		}
	}
	s := session(t, "let x: number = 1; x;")
	for _, offset := range []int{-1, 1000, 3} {
		if _, err := s.TypeAt(offset); err == nil {
			t.Fatalf("invalid query accepted: %d", offset)
		}
	}
}

func TestPublicDTOsContainNoUpstreamHandles(t *testing.T) {
	var check func(reflect.Type)
	check = func(typ reflect.Type) {
		if strings.Contains(typ.PkgPath(), "/internal/") {
			t.Fatalf("upstream type leaked: %v", typ)
		}
		switch typ.Kind() {
		case reflect.Struct:
			for i := 0; i < typ.NumField(); i++ {
				check(typ.Field(i).Type)
			}
		case reflect.Slice, reflect.Array:
			check(typ.Elem())
		case reflect.String, reflect.Int, reflect.Int32:
		default:
			t.Fatalf("non-data field in DTO: %v", typ)
		}
	}
	check(reflect.TypeFor[tynelapi.Snapshot]())
	check(reflect.TypeFor[tynelapi.TypeFact]())
}

func TestExternalInternalImportRejected(t *testing.T) {
	goBin := os.Getenv("TYNEL_PROBE_GO")
	if goBin == "" {
		t.Fatal("run scripts/build-tsgo-probe.ps1 -Test to supply the exact SDK")
	}
	tmp := filepath.Join(t.TempDir(), "forbidden.go")
	if err := os.WriteFile(tmp, []byte("package main\nimport _ \"github.com/microsoft/typescript-go/internal/ast\"\nfunc main(){}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(goBin, "build", "-mod=readonly", "-o", filepath.Join(t.TempDir(), "forbidden.exe"), tmp)
	log, err := cmd.CombinedOutput()
	if err == nil || !bytes.Contains(log, []byte("use of internal package")) {
		t.Fatalf("wrong internal fence result: %v %s", err, log)
	}
}

func TestCLI(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
		want int
	}{
		{"add", []string{"../../examples/frontend/add.ts"}, 0},
		{"type-error", []string{"../../examples/frontend/type-error.ts"}, 1},
		{"missing", []string{"missing.ts"}, 2},
		{"usage", nil, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var out, errs bytes.Buffer
			if got := run(tc.args, &out, &errs); got != tc.want {
				t.Fatalf("exit %d expected %d: %s %s", got, tc.want, out.String(), errs.String())
			}
			if tc.want < 2 && !json.Valid(out.Bytes()) {
				t.Fatalf("not JSON: %s", out.String())
			}
		})
	}
}
