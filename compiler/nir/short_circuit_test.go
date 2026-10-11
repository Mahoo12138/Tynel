package nir

import (
	"bytes"
	"errors"
	"reflect"
	"strings"
	"testing"
)

func shortFixture(op string) *Module {
	m := fixture()
	m.Functions[0].Body = []Stmt{
		{Op: "let", Name: "outer", Value: truth(false), Loc: loc},
		{Op: op, Name: "result", Condition: local("outer", Boolean), Loc: loc,
			Body: []Stmt{{Op: "let", Name: "right", Value: truth(true), Loc: loc}}, Value: local("right", Boolean)},
		{Op: "print_boolean", Value: local("result", Boolean), Loc: loc},
	}
	return m
}

func TestShortCircuitRoundTrip(t *testing.T) {
	for _, op := range []string{"short_and", "short_or"} {
		t.Run(op, func(t *testing.T) {
			m := shortFixture(op)
			var out bytes.Buffer
			if err := Write(&out, m); err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(out.String(), `"bool": false`) {
				t.Fatal("lost false")
			}
			got, err := Read(&out)
			if err != nil || !reflect.DeepEqual(m, got) {
				t.Fatalf("round trip: %v", err)
			}
		})
	}
}

func TestShortCircuitRejects(t *testing.T) {
	cases := map[string]func(*Module){
		"previous-version":  func(m *Module) { m.IRVersion = 3 },
		"missing-name":      func(m *Module) { m.Functions[0].Body[1].Name = "" },
		"duplicate-result":  func(m *Module) { m.Functions[0].Body[1].Name = "outer" },
		"unknown-operation": func(m *Module) { m.Functions[0].Body[1].Op = "short_xor" },
		"missing-left":      func(m *Module) { m.Functions[0].Body[1].Condition = nil },
		"number-left":       func(m *Module) { m.Functions[0].Body[1].Condition = num(0) },
		"missing-right":     func(m *Module) { m.Functions[0].Body[1].Value = nil },
		"number-right":      func(m *Module) { m.Functions[0].Body[1].Value = num(0) },
		"self-left":         func(m *Module) { m.Functions[0].Body[1].Condition = local("result", Boolean) },
		"self-right":        func(m *Module) { m.Functions[0].Body[1].Value = local("result", Boolean) },
		"forward-right":     func(m *Module) { m.Functions[0].Body[1].Body[0].Value = local("right", Boolean) },
		"bad-right-type":    func(m *Module) { m.Functions[0].Body[1].Value.Type = Number },
		"unknown-right":     func(m *Module) { m.Functions[0].Body[1].Value = local("missing", Boolean) },
		"rhs-local-leak":    func(m *Module) { m.Functions[0].Body[2].Value = local("right", Boolean) },
		"target":            func(m *Module) { m.Functions[0].Body[1].Target = truth(true) },
		"then":              func(m *Module) { m.Functions[0].Body[1].Then = []Stmt{} },
		"else":              func(m *Module) { m.Functions[0].Body[1].Else = []Stmt{} },
		"right-return":      func(m *Module) { m.Functions[0].Body[1].Body = []Stmt{ret(nil)} },
		"right-print": func(m *Module) {
			m.Functions[0].Body[1].Body = []Stmt{{Op: "print_boolean", Value: truth(true), Loc: loc}}
		},
		"right-if":    func(m *Module) { m.Functions[0].Body[1].Body = []Stmt{{Op: "if", Condition: truth(true), Loc: loc}} },
		"right-probe": func(m *Module) { m.Functions[0].Body[1].Body = []Stmt{{Op: "probe_gc", Loc: loc}} },
		"oversize":    func(m *Module) { m.Functions[0].Body[1].Body = make([]Stmt, MaxNodes+1) },
		"rhs-cycle": func(m *Module) {
			body := make([]Stmt, 1)
			body[0] = Stmt{Op: "short_or", Name: "recursive", Condition: truth(true), Body: body, Value: truth(false), Loc: loc}
			m.Functions[0].Body[1].Body = body
		},
		"expression-cycle": func(m *Module) {
			x := &Expr{Op: "not", Type: Boolean, Loc: loc}
			x.Args = []*Expr{x}
			m.Functions[0].Body[1].Value = x
		},
		"invalid-skipped-rhs": func(m *Module) {
			m.Functions[0].Body[1].Condition = truth(false)
			m.Functions[0].Body[1].Body[0].Value.Op = "eval"
		},
	}
	for name, change := range cases {
		t.Run(name, func(t *testing.T) {
			m := shortFixture("short_and")
			change(m)
			var d Diagnostic
			if !errors.As(Validate(m), &d) || d.Code != "TY4001" {
				t.Fatal("invalid short-circuit IR accepted")
			}
		})
	}
}

func TestShortCircuitNestedScope(t *testing.T) {
	m := shortFixture("short_or")
	s := &m.Functions[0].Body[1]
	s.Body = []Stmt{{Op: "short_and", Name: "inner", Condition: local("outer", Boolean), Body: []Stmt{{Op: "let", Name: "hidden", Value: truth(true), Loc: loc}}, Value: local("hidden", Boolean), Loc: loc}}
	s.Value = local("inner", Boolean)
	if err := Validate(m); err != nil {
		t.Fatal(err)
	}
	s.Value = local("hidden", Boolean)
	if Validate(m) == nil {
		t.Fatal("nested RHS local escaped")
	}
	m = shortFixture("short_and")
	second := m.Functions[0].Body[1]
	second.Name = "second"
	second.Condition = local("result", Boolean)
	m.Functions[0].Body = append(m.Functions[0].Body, second)
	if err := Validate(m); err != nil {
		t.Fatal("RHS scopes should be independent", err)
	}
	second.Body = nil
	second.Value = local("right", Boolean)
	m.Functions[0].Body[len(m.Functions[0].Body)-1] = second
	if Validate(m) == nil {
		t.Fatal("previous RHS local leaked into sibling")
	}
}
