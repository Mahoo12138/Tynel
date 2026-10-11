package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/microsoft/typescript-go/tynelapi"
	"tynel.local/tynel/compiler/frontend"
	"tynel.local/tynel/compiler/nir"
)

func extract(t *testing.T, source string) *frontend.Program {
	t.Helper()
	s, err := tynelapi.NewScalar(source)
	if err != nil {
		t.Fatal(err)
	}
	snap, err := s.Snapshot()
	if err != nil || len(snap.Diagnostics) > 0 {
		t.Fatalf("diagnostics: %+v %v", snap.Diagnostics, err)
	}
	p, err := s.ScalarFacts("test.ts")
	s.Close()
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.ScalarFacts("test.ts"); !errors.Is(err, tynelapi.ErrClosed) {
		t.Fatalf("closed extraction: %v", err)
	}
	return p
}
func TestScalarFactsAndReplayAfterClose(t *testing.T) {
	p := extract(t, "function add(a: number, b: number): number { const sum=a+b; return sum; } console.log(add(40,2));")
	if len(p.Functions) != 1 || len(p.Functions[0].Params) != 2 || p.Functions[0].Result != "number" {
		t.Fatalf("bad facts %+v", p)
	}
	serialized, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	var q frontend.Program
	if err = json.Unmarshal(serialized, &q); err != nil {
		t.Fatal(err)
	}
	m, err := frontend.Lower(&q)
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err = nir.Write(&out, m); err != nil {
		t.Fatal(err)
	}
	again, err := nir.Read(bytes.NewReader(out.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(m, again) {
		t.Fatal("NIR replay changed data")
	}
}
func TestScalarAcceptedForms(t *testing.T) {
	for name, source := range map[string]string{
		"numeric-radices": "console.log(0x2a + 0b10 + 0o10 + 1_000);",
		"unary":           "function neg(x:number):number{return -x;} console.log(1 / neg(0));",
		"same-spelling":   "function a(x:number):number{return x;} function b(x:number):number{return x+1;} const x=2; console.log(a(x)+b(x));",
		"forward-call":    "console.log(a(2)); function a(x:number):number{return b(x);} function b(x:number):number{return x+1;}",
		"unicode":         "// 😀\r\nfunction 求和(甲:number,乙:number):number{return 甲+乙;} console.log(求和(40,2));",
		"discard":         "const answer=40+2; answer;",
		"effect-order":    "function mark(x:number):number{console.log(x);return x;} function sum(a:number,b:number):number{return a+b;} console.log(sum(mark(1),mark(2)));",
	} {
		t.Run(name, func(t *testing.T) {
			p := extract(t, source)
			if _, err := frontend.Lower(p); err != nil {
				t.Fatal(err)
			}
		})
	}
}
func TestScalarRejectsUnsupportedWithoutArtifact(t *testing.T) {
	for name, source := range map[string]string{
		"mutable":             "let x=1; console.log(x);",
		"loop":                "for(let i=0;i<1;i++)console.log(i);",
		"branch":              "function f(x:number):number{if(x>0)return x;return 0;} console.log(f(1));",
		"object":              "const x={value:42}; console.log(x.value);",
		"closure":             "function f(x:number):number{function g():number{return x;}return g();}",
		"top-capture":         "const x=42; function f():number{return x;} console.log(f());",
		"function-value":      "function f(x:number):number{return x;} const g=f; console.log(g(1));",
		"generic":             "function f<T>(x:T):T{return x;} console.log(f(42));",
		"default":             "function f(x:number=1):number{return x;} console.log(f());",
		"rest":                "function f(...x:number[]):number{return x[0];} console.log(f(1));",
		"assertion":           "console.log(('bad' as unknown) as number);",
		"never":               "function f():never{throw 1;} f();",
		"library-only":        "console.log(Math.abs(-1));",
		"unused":              "function unused(x:number):number{return x%2;} console.log(42);",
		"shadow-log":          "function f(console:number):number{return console;} console.log(f(1)); const alias=console.log; alias(2);",
		"too-many-print-args": "console.log(1,2);",
		"error":               "const x:number='bad'; console.log(x);",
		"after-return":        "function f(x:number):number{return x;console.log(x);} console.log(f(1));",
		"assignment":          "function f(x:number):number{x=2;return x;} console.log(f(1));",
		"optional-call":       "function f(x:number):number{return x;} console.log(f?.(1));",
		"module":              "export function f(x:number):number{return x;}",
		"number-union":        "function f(x:number|undefined):number{return x===undefined?0:x;} console.log(f(1));",
	} {
		t.Run(name, func(t *testing.T) {
			var out, errs bytes.Buffer
			code := emitScalar(source, "negative.ts", "nir", &out, &errs)
			if code == 0 || out.Len() != 0 || (!strings.Contains(errs.String(), "TY12") && !strings.Contains(errs.String(), "TS")) {
				t.Fatalf("exit=%d stdout=%s stderr=%s", code, &out, &errs)
			}
		})
	}
}
func TestScalarBindingsAreSymbolIdentities(t *testing.T) {
	p := extract(t, "function a(x:number):number{return x;} function b(x:number):number{return x;} console.log(a(1)+b(2));")
	a, b := p.Functions[0], p.Functions[1]
	if a.Params[0].ID == b.Params[0].ID || a.Body[0].Value.Ref != a.Params[0].ID || b.Body[0].Value.Ref != b.Params[0].ID {
		t.Fatal("same-spelling symbols merged")
	}
}
func TestScalarFactsDoNotAliasSession(t *testing.T) {
	s, err := tynelapi.NewScalar("const x=42;console.log(x);")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	p, err := s.ScalarFacts("a.ts")
	if err != nil {
		t.Fatal(err)
	}
	p.Body[0].Value.Bits = "mutated"
	q, err := s.ScalarFacts("a.ts")
	if err != nil || q.Body[0].Value.Bits == "mutated" {
		t.Fatal("facts leaked session storage")
	}
}
func TestScalarErrorLocation(t *testing.T) {
	var out, errs bytes.Buffer
	code := emitScalar("// 😀\nconst x=1;\nconsole.log(x % 2);", "真实.ts", "nir", &out, &errs)
	if code == 0 || !strings.Contains(errs.String(), "真实.ts:3:13: TY1201") {
		t.Fatalf("bad diagnostic %d %s", code, &errs)
	}
}
