package nir_test

import (
	"bytes"
	"errors"
	"strings"
	"testing"
	"tynel.local/tynel/compiler/nir"
	"tynel.local/tynel/internal/probe"
)

func TestProbeRoundTrip(t *testing.T) {
	m := probe.GCModule()
	var a, b bytes.Buffer
	if err := nir.Write(&a, m); err != nil {
		t.Fatal(err)
	}
	loaded, err := nir.Read(bytes.NewReader(a.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	if err := nir.Write(&b, loaded); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(a.Bytes(), b.Bytes()) {
		t.Fatal("probe IR round trip changed")
	}
}

func TestProbeRejects(t *testing.T) {
	cases := map[string]func(*nir.Module){
		"missing feature":              func(m *nir.Module) { m.Features = nil },
		"unknown feature":              func(m *nir.Module) { m.Features = []string{"probe.unknown/1"} },
		"duplicate feature":            func(m *nir.Module) { m.Features = append(m.Features, nir.ProbeGCFeature) },
		"old IR":                       func(m *nir.Module) { m.IRVersion = 1 },
		"self reference":               func(m *nir.Module) { e := m.Functions[0].Body[0].Value; e.Op = "local"; e.Name = "node" },
		"duplicate local":              func(m *nir.Module) { m.Functions[2].Body[2].Name = "root" },
		"wrong parameter kind":         func(m *nir.Module) { m.Functions[0].Body[1].Value.Op = "local" },
		"forged parameter type":        func(m *nir.Module) { m.Functions[0].Body[1].Value.Type = nir.ProbeRef },
		"reference arithmetic":         func(m *nir.Module) { e := m.Functions[2].Body[4].Value; e.Op = "add"; e.Name = ""; e.Type = nir.Number },
		"wrong call argument":          func(m *nir.Module) { m.Functions[2].Body[0].Value.Args[0] = m.Functions[0].Body[0].Value },
		"number in pointer field":      func(m *nir.Module) { m.Functions[1].Body[0].Value = m.Functions[2].Body[3].Value },
		"number as target":             func(m *nir.Module) { m.Functions[2].Body[3].Target = m.Functions[2].Body[3].Value },
		"number returned as reference": func(m *nir.Module) { m.Functions[0].Body[2].Value = m.Functions[0].Body[1].Value },
		"gc with operand":              func(m *nir.Module) { m.Functions[2].Body[1].Value = m.Functions[2].Body[3].Value },
		"gc with target":               func(m *nir.Module) { m.Functions[2].Body[1].Target = m.Functions[2].Body[3].Target },
		"extraneous binding":           func(m *nir.Module) { m.Functions[0].Body[2].Name = "extra" },
		"forged new type":              func(m *nir.Module) { m.Functions[0].Body[0].Value.Type = nir.Number },
		"unknown probe operation":      func(m *nir.Module) { m.Functions[2].Body[1].Op = "probe_unknown" },
		"nil field operand":            func(m *nir.Module) { m.Functions[2].Body[9].Value.Args[0] = nil },
		"reference printed as number":  func(m *nir.Module) { m.Functions[2].Body[9].Value = m.Functions[2].Body[9].Value.Args[0] },
	}
	for name, change := range cases {
		t.Run(name, func(t *testing.T) {
			m := probe.GCModule()
			change(m)
			err := nir.Validate(m)
			var d nir.Diagnostic
			if !errors.As(err, &d) || d.Code != "TY4001" {
				t.Fatalf("expected TY4001, got %v", err)
			}
		})
	}
}

func TestScalarLocalsAndFeatureGate(t *testing.T) {
	loc := nir.Span{File: "local.nir", Line: 1, Column: 1}
	n := &nir.Expr{Op: "number", Type: nir.Number, Bits: nir.EncodeNumber(42), Loc: loc}
	m := &nir.Module{IRVersion: nir.Version, Semantics: nir.Semantics, Package: "main", Functions: []nir.Function{{Name: "main", Result: nir.Void, Loc: loc, Body: []nir.Stmt{
		{Op: "let", Name: "answer", Value: n, Loc: loc},
		{Op: "print_number", Value: &nir.Expr{Op: "local", Name: "answer", Type: nir.Number, Loc: loc}, Loc: loc},
	}}}}
	if err := nir.Validate(m); err != nil {
		t.Fatal(err)
	}
	m.Functions[0].Body = append(m.Functions[0].Body, nir.Stmt{Op: "probe_gc", Loc: loc})
	if err := nir.Validate(m); err == nil || !strings.Contains(err.Error(), "requires feature") {
		t.Fatalf("ungated probe: %v", err)
	}
}
