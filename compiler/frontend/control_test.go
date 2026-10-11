package frontend

import (
	"bytes"
	"encoding/json"
	"testing"
	"tynel.local/tynel/compiler/nir"
)

func booleanFact(v bool) *Expression {
	return &Expression{Op: "bool", Type: "boolean", Bool: &v, Loc: scalarFixture().Body[0].Loc}
}
func branchFacts() *Program {
	p := scalarFixture()
	loc := p.Body[0].Loc
	n := p.Body[0].Value
	p.Functions = []Function{{ID: "f0", Name: "choose", Params: []Binding{{ID: "v1", Name: "flag", Type: "boolean", Loc: loc}}, Result: "number", Loc: loc, Body: []Statement{{
		Op: "if", Loc: loc, Condition: &Expression{Op: "ref", Type: "boolean", Ref: "v1", Loc: loc},
		Then: []Statement{{Op: "const", Binding: &Binding{ID: "v2", Name: "inside", Type: "number", Loc: loc}, Value: n, Loc: loc}, {Op: "return", Value: &Expression{Op: "ref", Type: "number", Ref: "v2", Loc: loc}, Loc: loc}},
		Else: []Statement{{Op: "return", Value: n, Loc: loc}},
	}}}}
	p.Body[0].Value = &Expression{Op: "call", Ref: "f0", Type: "number", Args: []*Expression{booleanFact(false)}, Loc: loc}
	return p
}
func TestControlFactsRoundTrip(t *testing.T) {
	p := branchFacts()
	data, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	var q Program
	if err = json.Unmarshal(data, &q); err != nil {
		t.Fatal(err)
	}
	m, err := Lower(&q)
	if err != nil {
		t.Fatal(err)
	}
	if m.Functions[0].Body[0].Op != "if" || len(m.Functions[0].Body[0].Then) != 2 {
		t.Fatal("branch structure changed")
	}
	var encoded bytes.Buffer
	if err = nir.Write(&encoded, m); err != nil {
		t.Fatal(err)
	}
	if _, err = nir.Read(&encoded); err != nil {
		t.Fatal(err)
	}
}
func TestControlFactsReject(t *testing.T) {
	cases := map[string]func(*Program){
		"old-facts":           func(p *Program) { p.Version = 1 },
		"old-profile":         func(p *Program) { p.Profile = "tynel.scalar/1" },
		"missing-bool":        func(p *Program) { p.Body[0].Value.Args[0].Bool = nil },
		"extra-bool":          func(p *Program) { p.Functions[0].Body[0].Else[0].Value.Bool = booleanFact(false).Bool },
		"bad-condition":       func(p *Program) { p.Functions[0].Body[0].Condition = p.Functions[0].Body[0].Else[0].Value },
		"missing-return-path": func(p *Program) { p.Functions[0].Body[0].Else = nil },
		"wrong-branch-result": func(p *Program) { p.Functions[0].Body[0].Else[0].Value = booleanFact(true) },
		"scope-escape":        func(p *Program) { p.Functions[0].Body[0].Else[0].Value = p.Functions[0].Body[0].Then[1].Value },
		"forged-ref-type":     func(p *Program) { p.Functions[0].Body[0].Condition.Type = "number" },
		"forged-binding-type": func(p *Program) { p.Functions[0].Body[0].Then[0].Binding.Type = "boolean" },
		"bool-arithmetic": func(p *Program) {
			p.Body[0].Value = &Expression{Op: "add", Type: "number", Args: []*Expression{booleanFact(false), booleanFact(true)}, Loc: p.Body[0].Loc}
		},
		"numeric-not": func(p *Program) {
			p.Body[0].Value = &Expression{Op: "not", Type: "boolean", Args: []*Expression{p.Functions[0].Body[0].Else[0].Value}, Loc: p.Body[0].Loc}
		},
		"branch-extra-value": func(p *Program) { p.Functions[0].Body[0].Value = booleanFact(true) },
		"foreign-branch":     func(p *Program) { p.Body[0].Then = []Statement{} },
		"foreign-block":      func(p *Program) { p.Body[0].Body = []Statement{} },
		"cycle": func(p *Program) {
			body := make([]Statement, 1)
			body[0] = Statement{Op: "block", Loc: p.Body[0].Loc, Body: body}
			p.Body = body
		},
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			p := branchFacts()
			mutate(p)
			if _, err := Lower(p); err == nil {
				t.Fatal("invalid control facts accepted")
			}
		})
	}
}
func TestBranchCallsStayInBranch(t *testing.T) {
	p := branchFacts()
	call := p.Body[0].Value
	p.Body = []Statement{{Op: "if", Condition: booleanFact(false), Then: []Statement{{Op: "print", Value: call, Loc: p.Body[0].Loc}}, Loc: p.Body[0].Loc}}
	m, err := Lower(p)
	if err != nil {
		t.Fatal(err)
	}
	main := m.Functions[len(m.Functions)-1]
	if len(main.Body) != 1 || main.Body[0].Op != "if" {
		t.Fatal("hoisted work from branch")
	}
	body := main.Body[0].Then
	if len(body) != 2 || body[0].Value.Op != "call" {
		t.Fatal("missing branch-local call temporary")
	}
}
func TestFalseLiteralDetachedFromFacts(t *testing.T) {
	p := scalarFixture()
	p.Body[0].Value = booleanFact(false)
	m, err := Lower(p)
	if err != nil {
		t.Fatal(err)
	}
	*p.Body[0].Value.Bool = true
	if m.Functions[0].Body[0].Op != "print_boolean" || *m.Functions[0].Body[0].Value.Bool {
		t.Fatal("lowered bool aliases input")
	}
}
