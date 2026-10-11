package nir

import (
	"bytes"
	"errors"
	"reflect"
	"strings"
	"testing"
)

func truth(v bool) *Expr                { return &Expr{Op: "bool", Type: Boolean, Bool: &v, Loc: loc} }
func local(name string, typ Type) *Expr { return &Expr{Op: "local", Name: name, Type: typ, Loc: loc} }
func ret(e *Expr) Stmt                  { return Stmt{Op: "return", Value: e, Loc: loc} }
func branchFixture() *Module {
	m := fixture()
	f := Function{Name: "choose", Params: []Param{{Name: "flag", Type: Boolean}}, Result: Number, Loc: loc, Body: []Stmt{{
		Op: "if", Condition: &Expr{Op: "param", Name: "flag", Type: Boolean, Loc: loc}, Loc: loc,
		Then: []Stmt{{Op: "let", Name: "inside", Value: num(42), Loc: loc}, ret(local("inside", Number))},
		Else: []Stmt{ret(num(0))},
	}}}
	m.Functions = append(m.Functions, f)
	m.Functions[0].Body[0].Value = &Expr{Op: "call", Name: "choose", Type: Number, Args: []*Expr{truth(false)}, Loc: loc}
	return m
}

func TestBooleanBranchRoundTrip(t *testing.T) {
	m := branchFixture()
	var data bytes.Buffer
	if err := Write(&data, m); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(data.String(), `"bool": false`) {
		t.Fatal("false literal disappeared")
	}
	again, err := Read(&data)
	if err != nil || !reflect.DeepEqual(m, again) {
		t.Fatalf("round trip: %v", err)
	}
}

func TestControlRejects(t *testing.T) {
	cases := map[string]func(*Module){
		"old-schema":                  func(m *Module) { m.IRVersion = 2 },
		"missing-boolean":             func(m *Module) { m.Functions[0].Body[0].Value.Args[0].Bool = nil },
		"wrong-bool-result":           func(m *Module) { m.Functions[0].Body[0].Value.Args[0].Type = Number },
		"extra-bool":                  func(m *Module) { m.Functions[1].Body[0].Else[0].Value.Bool = truth(false).Bool },
		"missing-condition":           func(m *Module) { m.Functions[1].Body[0].Condition = nil },
		"numeric-condition":           func(m *Module) { m.Functions[1].Body[0].Condition = num(1) },
		"if-value":                    func(m *Module) { m.Functions[1].Body[0].Value = num(1) },
		"if-target":                   func(m *Module) { m.Functions[1].Body[0].Target = num(1) },
		"if-body":                     func(m *Module) { m.Functions[1].Body[0].Body = []Stmt{} },
		"foreign-branch":              func(m *Module) { m.Functions[1].Body[0].Else[0].Then = []Stmt{} },
		"missing-else-return":         func(m *Module) { m.Functions[1].Body[0].Else = nil },
		"wrong-return":                func(m *Module) { m.Functions[1].Body[0].Else[0].Value = truth(true) },
		"cross-arm-local":             func(m *Module) { m.Functions[1].Body[0].Else[0].Value = local("inside", Number) },
		"statement-after-both-return": func(m *Module) { m.Functions[1].Body = append(m.Functions[1].Body, ret(num(4))) },
		"invalid-untaken": func(m *Module) {
			m.Functions[1].Body[0].Condition = truth(true)
			m.Functions[1].Body[0].Else[0].Op = "eval"
		},
		"mixed-equality": func(m *Module) {
			m.Functions[1].Body[0].Condition = &Expr{Op: "eq", Type: Boolean, Args: []*Expr{num(1), truth(true)}, Loc: loc}
		},
		"nil-equality": func(m *Module) {
			m.Functions[1].Body[0].Condition = &Expr{Op: "eq", Type: Boolean, Args: []*Expr{nil, truth(true)}, Loc: loc}
		},
		"boolean-order": func(m *Module) {
			m.Functions[1].Body[0].Condition = &Expr{Op: "lt", Type: Boolean, Args: []*Expr{truth(false), truth(true)}, Loc: loc}
		},
		"numeric-not": func(m *Module) {
			m.Functions[1].Body[0].Condition = &Expr{Op: "not", Type: Boolean, Args: []*Expr{num(1)}, Loc: loc}
		},
		"forged-reference-type": func(m *Module) { m.Functions[1].Body[0].Condition.Type = Number },
		"wrong-printer":         func(m *Module) { m.Functions[0].Body[0].Op = "print_boolean" },
		"cycle": func(m *Module) {
			body := make([]Stmt, 1)
			body[0] = Stmt{Op: "block", Body: body, Loc: loc}
			m.Functions[0].Body = body
		},
		"nested-budget": func(m *Module) { m.Functions[0].Body = []Stmt{{Op: "block", Body: make([]Stmt, MaxNodes+1), Loc: loc}} },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			m := branchFixture()
			mutate(m)
			var d Diagnostic
			if !errors.As(Validate(m), &d) || d.Code != "TY4001" {
				t.Fatal("bad control IR accepted")
			}
		})
	}
}

func TestBranchFallthroughAndScope(t *testing.T) {
	m := branchFixture()
	f := &m.Functions[1]
	f.Body[0].Else = nil
	f.Body = append(f.Body, ret(num(0)))
	if err := Validate(m); err != nil {
		t.Fatal(err)
	}
	f.Body[1].Value = local("inside", Number)
	if err := Validate(m); err == nil {
		t.Fatal("branch local escaped")
	}
	m = fixture()
	m.Functions[0].Body = []Stmt{{Op: "block", Body: []Stmt{{Op: "let", Name: "inner", Value: truth(false), Loc: loc}}, Loc: loc}, {Op: "print_boolean", Value: local("inner", Boolean), Loc: loc}}
	if Validate(m) == nil {
		t.Fatal("block local escaped")
	}
}

func TestBooleanOperationsAndBlockReturn(t *testing.T) {
	for _, op := range []string{"eq", "ne", "lt", "le", "gt", "ge"} {
		m := fixture()
		m.Functions[0].Body = []Stmt{{Op: "print_boolean", Value: &Expr{Op: op, Type: Boolean, Args: []*Expr{num(1), num(2)}, Loc: loc}, Loc: loc}}
		if err := Validate(m); err != nil {
			t.Fatal(op, err)
		}
	}
	m := branchFixture()
	m.Functions[1].Body = []Stmt{{Op: "block", Body: []Stmt{ret(num(42))}, Loc: loc}}
	if err := Validate(m); err != nil {
		t.Fatal(err)
	}
}
