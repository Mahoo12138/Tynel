package frontend

import (
	"bytes"
	"strings"
	"testing"

	"tynel.local/tynel/compiler/nir"
)

func scalarFixture() *Program {
	loc := Span{File: "input.ts", StartByte: 0, EndByte: 2, Line: 1, Column: 1}
	return &Program{Version: Version, Profile: Profile, File: "input.ts", SourceSHA256: strings.Repeat("a", 64), Body: []Statement{{Op: "print", Loc: loc, Value: &Expression{Op: "number", Type: "number", Bits: nir.EncodeNumber(42), Loc: loc}}}}
}
func TestLowerRoundTrip(t *testing.T) {
	m, err := Lower(scalarFixture())
	if err != nil {
		t.Fatal(err)
	}
	var b bytes.Buffer
	if err = nir.Write(&b, m); err != nil {
		t.Fatal(err)
	}
	if _, err = nir.Read(&b); err != nil {
		t.Fatal(err)
	}
}
func TestLowerRejects(t *testing.T) {
	cases := map[string]func(*Program){
		"version":         func(p *Program) { p.Version++ },
		"profile":         func(p *Program) { p.Profile = "unreviewed" },
		"source":          func(p *Program) { p.SourceSHA256 = "" },
		"file":            func(p *Program) { p.File = "bad\nname" },
		"span":            func(p *Program) { p.Body[0].Value.Loc.File = "other.ts" },
		"bits":            func(p *Program) { p.Body[0].Value.Bits = "invalid" },
		"type":            func(p *Program) { p.Body[0].Value.Type = "any" },
		"call":            func(p *Program) { p.Body[0].Value.Op = "call"; p.Body[0].Value.Bits = ""; p.Body[0].Value.Ref = "f99" },
		"forward":         func(p *Program) { p.Body[0].Value.Op = "ref"; p.Body[0].Value.Bits = ""; p.Body[0].Value.Ref = "v99" },
		"operation":       func(p *Program) { p.Body[0].Value.Op = "eval" },
		"cycle":           func(p *Program) { e := p.Body[0].Value; e.Op = "neg"; e.Bits = ""; e.Args = []*Expression{e} },
		"top-return":      func(p *Program) { p.Body[0].Op = "return" },
		"missing-binding": func(p *Program) { p.Body[0].Op = "const" },
	}
	for name, change := range cases {
		t.Run(name, func(t *testing.T) {
			p := scalarFixture()
			change(p)
			if _, err := Lower(p); err == nil {
				t.Fatal("accepted invalid facts")
			}
		})
	}
	if _, err := Lower(nil); err == nil {
		t.Fatal("accepted nil")
	}
}
func TestDiscardIsNotPrint(t *testing.T) {
	p := scalarFixture()
	p.Body[0].Op = "discard"
	m, err := Lower(p)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range m.Functions[0].Body {
		if s.Op == "print_number" {
			t.Fatal("expression statement became print")
		}
	}
}
