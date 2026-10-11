package nir

import (
	"bytes"
	"math"
	"strings"
	"testing"
)

var loc = Span{File: "test.tynel", Line: 1, Column: 1}

func num(v float64) *Expr { return &Expr{Op: "number", Type: Number, Bits: EncodeNumber(v), Loc: loc} }
func fixture() *Module {
	return &Module{IRVersion: Version, Semantics: Semantics, Package: "main", Functions: []Function{
		{Name: "main", Result: Void, Loc: loc, Body: []Stmt{{Op: "print_number", Value: num(42), Loc: loc}}},
	}}
}

func TestNumberBits(t *testing.T) {
	for _, bits := range []uint64{0, 1 << 63, 0x7ff0000000000000, 0xfff0000000000000, 0x7ff8000000000042, 1, 0x7fefffffffffffff, 0x4045000000000000} {
		v := math.Float64frombits(bits)
		got, err := DecodeNumber(EncodeNumber(v))
		if err != nil || math.Float64bits(got) != bits {
			t.Fatalf("%x: got %x, %v", bits, math.Float64bits(got), err)
		}
	}
}

func TestBadNumberBits(t *testing.T) {
	for _, s := range []string{"", "0", "000000000000000A", "zzzzzzzzzzzzzzzz", "-000000000000001", "00000000000000000"} {
		if _, err := DecodeNumber(s); err == nil {
			t.Errorf("accepted %q", s)
		}
	}
}

func TestRoundTrip(t *testing.T) {
	m := fixture()
	m.Functions[0].Body[0].Value = num(math.Copysign(0, -1))
	var b bytes.Buffer
	if err := Write(&b, m); err != nil {
		t.Fatal(err)
	}
	restored, err := Read(&b)
	if err != nil {
		t.Fatal(err)
	}
	if restored.Functions[0].Body[0].Value.Bits != "8000000000000000" {
		t.Fatal("lost negative zero")
	}
}

func TestValidateRejects(t *testing.T) {
	cases := map[string]func(*Module){
		"version":     func(m *Module) { m.IRVersion++ },
		"semantics":   func(m *Module) { m.Semantics = "other" },
		"package":     func(m *Module) { m.Package = "other" },
		"no entry":    func(m *Module) { m.Functions[0].Name = "entry" },
		"duplicate":   func(m *Module) { m.Functions = append(m.Functions, m.Functions[0]) },
		"reserved":    func(m *Module) { m.Functions[0].Name = "tynelF64" },
		"location":    func(m *Module) { m.Functions[0].Loc.Line = 0 },
		"unsupported": func(m *Module) { m.Functions[0].Body[0].Op = "eval" },
		"bad type":    func(m *Module) { m.Functions[0].Body[0].Value.Type = "any" },
		"unknown parameter": func(m *Module) {
			m.Functions[0].Body[0].Value = &Expr{Op: "param", Type: Number, Name: "missing", Loc: loc}
		},
		"unknown call": func(m *Module) {
			m.Functions[0].Body[0].Value = &Expr{Op: "call", Type: Number, Name: "missing", Loc: loc}
		},
		"nil expression": func(m *Module) { m.Functions[0].Body[0].Value = nil },
		"trailing statement": func(m *Module) {
			m.Functions[0].Body = append([]Stmt{{Op: "return", Loc: loc}}, m.Functions[0].Body...)
		},
		"cycle": func(m *Module) {
			e := &Expr{Op: "add", Type: Number, Loc: loc}
			e.Args = []*Expr{e, num(1)}
			m.Functions[0].Body[0].Value = e
		},
	}
	for name, change := range cases {
		t.Run(name, func(t *testing.T) {
			m := fixture()
			change(m)
			if Validate(m) == nil {
				t.Fatal("invalid IR accepted")
			}
		})
	}
}

func TestReadRejects(t *testing.T) {
	for _, data := range []string{"{", "null", `{"irVersion":1,"typo":0}`, strings.Repeat(" ", MaxArtifactBytes+1)} {
		if _, err := Read(strings.NewReader(data)); err == nil {
			t.Fatal("invalid artifact accepted")
		}
	}
	var b bytes.Buffer
	if err := Write(&b, fixture()); err != nil {
		t.Fatal(err)
	}
	if _, err := Read(strings.NewReader(b.String() + "{}")); err == nil {
		t.Fatal("accepted two documents")
	}
}

func TestReadUnknownField(t *testing.T) {
	var b bytes.Buffer
	if err := Write(&b, fixture()); err != nil {
		t.Fatal(err)
	}
	data := strings.Replace(b.String(), "{", `{"unknownField": 1,`, 1)
	_, err := Read(strings.NewReader(data))
	if err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("wrong refusal: %v", err)
	}
}

func TestVoidAndLimits(t *testing.T) {
	m := fixture()
	m.Functions[0].Body = nil
	if err := Validate(m); err != nil {
		t.Fatal(err)
	}
	m.Functions[0].Loc.Column = 1 << 30
	if Validate(m) == nil {
		t.Fatal("accepted unrepresentable source location")
	}
	if Validate(nil) == nil {
		t.Fatal("accepted nil module")
	}
	m = fixture()
	m.Functions[0].Body = make([]Stmt, MaxNodes+1)
	if Validate(m) == nil {
		t.Fatal("accepted excessive body")
	}
}

func TestWriteRejectsOversize(t *testing.T) {
	m := fixture()
	s := Stmt{Op: "print_number", Value: num(1), Loc: loc}
	s.Loc.File = strings.Repeat("x", 1024)
	s.Value.Loc.File = s.Loc.File
	m.Functions[0].Body = make([]Stmt, 600)
	for i := range m.Functions[0].Body {
		m.Functions[0].Body[i] = s
	}
	var out bytes.Buffer
	if err := Write(&out, m); err == nil || out.Len() != 0 {
		t.Fatalf("oversize output accepted or partially written: %v", err)
	}
}

func FuzzNumberBits(f *testing.F) {
	f.Add(uint64(0))
	f.Add(uint64(1 << 63))
	f.Add(uint64(0x7ff8000000000042))
	f.Fuzz(func(t *testing.T, u uint64) {
		got, err := DecodeNumber(EncodeNumber(math.Float64frombits(u)))
		if err != nil || math.Float64bits(got) != u {
			t.Fatalf("round-trip failed for %x", u)
		}
	})
}
