package frontend

import (
	"bytes"
	"encoding/json"
	"reflect"
	"testing"

	"tynel.local/tynel/compiler/nir"
)

func shortFacts(op string) *Program {
	p := scalarFixture()
	s := p.Body[0].Loc
	p.Functions = []Function{
		{ID: "f0", Name: "left", Result: "boolean", Loc: s, Body: []Statement{{Op: "return", Value: booleanFact(false), Loc: s}}},
		{ID: "f1", Name: "right", Result: "boolean", Loc: s, Body: []Statement{{Op: "return", Value: booleanFact(true), Loc: s}}},
	}
	p.Body[0].Value = &Expression{Op: op, Type: "boolean", Loc: s, Args: []*Expression{
		{Op: "call", Type: "boolean", Ref: "f0", Loc: s}, {Op: "call", Type: "boolean", Ref: "f1", Loc: s},
	}}
	return p
}

func TestShortCircuitLoweringAndReplay(t *testing.T) {
	for _, op := range []string{"and", "or"} {
		t.Run(op, func(t *testing.T) {
			p := shortFacts(op)
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
			body := m.Functions[len(m.Functions)-1].Body
			if len(body) != 3 || body[0].Op != "let" || body[0].Value.Name != "f0" || body[1].Op != "short_"+op || body[2].Op != "print_boolean" {
				t.Fatalf("eager or missing steps: %+v", body)
			}
			s := body[1]
			if s.Condition.Name != body[0].Name || len(s.Body) != 1 || s.Body[0].Value.Op != "call" || s.Body[0].Value.Name != "f1" || s.Value.Name != s.Body[0].Name || body[2].Value.Name != s.Name {
				t.Fatal("incorrect scope/result wiring")
			}
			var out bytes.Buffer
			if err = nir.Write(&out, m); err != nil {
				t.Fatal(err)
			}
			again, err := nir.Read(&out)
			if err != nil || !reflect.DeepEqual(m, again) {
				t.Fatal("NIR round trip", err)
			}
		})
	}
}

func TestShortCircuitFactsRejected(t *testing.T) {
	cases := map[string]func(*Program){
		"old-version":   func(p *Program) { p.Version = 2 },
		"old-profile":   func(p *Program) { p.Profile = "tynel.scalar/2" },
		"wrong-result":  func(p *Program) { p.Body[0].Value.Type = "number" },
		"missing-arg":   func(p *Program) { p.Body[0].Value.Args = p.Body[0].Value.Args[:1] },
		"extra-arg":     func(p *Program) { p.Body[0].Value.Args = append(p.Body[0].Value.Args, booleanFact(true)) },
		"nil-left":      func(p *Program) { p.Body[0].Value.Args[0] = nil },
		"nil-right":     func(p *Program) { p.Body[0].Value.Args[1] = nil },
		"number-left":   func(p *Program) { p.Body[0].Value.Args[0] = scalarFixture().Body[0].Value },
		"number-right":  func(p *Program) { p.Body[0].Value.Args[1] = scalarFixture().Body[0].Value },
		"extra-ref":     func(p *Program) { p.Body[0].Value.Ref = "f0" },
		"extra-bits":    func(p *Program) { p.Body[0].Value.Bits = nir.EncodeNumber(0) },
		"extra-bool":    func(p *Program) { p.Body[0].Value.Bool = booleanFact(false).Bool },
		"unknown-right": func(p *Program) { p.Body[0].Value.Args[1].Ref = "f99" },
		"wrong-signature": func(p *Program) {
			p.Functions[1].Result = "number"
			p.Functions[1].Body[0].Value = scalarFixture().Body[0].Value
		},
		"skipped-invalid": func(p *Program) { p.Body[0].Value.Args[0] = booleanFact(false); p.Body[0].Value.Args[1].Op = "eval" },
		"right-span":      func(p *Program) { p.Body[0].Value.Args[1].Loc.File = "other.ts" },
		"cycle":           func(p *Program) { e := p.Body[0].Value; e.Args[1] = e },
	}
	for name, change := range cases {
		t.Run(name, func(t *testing.T) {
			p := shortFacts("and")
			change(p)
			if _, err := Lower(p); err == nil {
				t.Fatal("invalid short-circuit facts accepted")
			}
		})
	}
}

func TestNestedShortCircuitStaysInsideRight(t *testing.T) {
	p := shortFacts("and")
	original := p.Body[0].Value.Args[1]
	p.Body[0].Value.Args[1] = &Expression{Op: "or", Type: "boolean", Loc: original.Loc, Args: []*Expression{original, booleanFact(false)}}
	m, err := Lower(p)
	if err != nil {
		t.Fatal(err)
	}
	body := m.Functions[len(m.Functions)-1].Body
	if len(body) != 3 || len(body[1].Body) != 2 || body[1].Body[1].Op != "short_or" {
		t.Fatal("nested RHS was hoisted")
	}
	// Returned data owns its own boolean literal storage.
	*p.Body[0].Value.Args[1].Args[1].Bool = true
	if *body[1].Body[1].Value.Bool {
		t.Fatal("NIR aliases checker facts")
	}
}
