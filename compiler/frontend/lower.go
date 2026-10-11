package frontend

import (
	"fmt"
	"regexp"
	"strings"

	"tynel.local/tynel/compiler/nir"
)

var idPattern = regexp.MustCompile(`^[fv][0-9]+$`)
var digestPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

// Lower checks the detached facts and builds NIR. All bodies are checked,
// including unused functions. Calls and arithmetic are materialized in source
// order so later effectful functions cannot be reordered by tree reconstruction.
func Lower(p *Program) (*nir.Module, error) {
	if p == nil || p.Version != Version || p.Profile != Profile || !digestPattern.MatchString(p.SourceSHA256) {
		return nil, Error{Code: "TY1202", Message: "invalid scalar facts identity"}
	}
	if p.File == "" || len(p.File) > 1024 || strings.ContainsAny(p.File, "\x00\r\n") || len(p.Functions) >= nir.MaxFunctions {
		return nil, Error{Code: "TY1202", Message: "invalid source name or function budget"}
	}
	l := &lowerer{file: p.File, funcs: make(map[string]Function), ids: make(map[string]bool)}
	for _, f := range p.Functions {
		if err := l.check(f.Loc); err != nil {
			return nil, err
		}
		if !strings.HasPrefix(f.ID, "f") || !idPattern.MatchString(f.ID) || l.ids[f.ID] || !scalarType(f.Result) || len(f.Params) > 32 {
			return nil, l.fail(f.Loc, "invalid or duplicate scalar function")
		}
		l.ids[f.ID] = true
		l.funcs[f.ID] = f
	}
	m := &nir.Module{IRVersion: nir.Version, Semantics: nir.Semantics, Package: "main"}
	for _, f := range p.Functions {
		g := nir.Function{Name: f.ID, Result: nir.Type(f.Result), Loc: l.span(f.Loc)}
		env := make(map[string]localBinding)
		for _, b := range f.Params {
			if err := l.bind(b, env, "param"); err != nil {
				return nil, err
			}
			g.Params = append(g.Params, nir.Param{Name: b.ID, Type: nir.Type(b.Type)})
		}
		if err := l.body(f.Body, &g, env, true); err != nil {
			return nil, err
		}
		m.Functions = append(m.Functions, g)
	}
	main := nir.Function{Name: "main", Result: nir.Void, Loc: nir.Span{File: p.File, Line: 1, Column: 1}}
	if err := l.body(p.Body, &main, make(map[string]localBinding), false); err != nil {
		return nil, err
	}
	m.Functions = append(m.Functions, main)
	if err := nir.Validate(m); err != nil {
		return nil, err
	}
	return m, nil
}

type lowerer struct {
	file  string
	funcs map[string]Function
	ids   map[string]bool
	count int
	temp  int
}

func (l *lowerer) fail(s Span, text string) error {
	return Error{Code: "TY1202", Message: text, Loc: s}
}
func (l *lowerer) span(s Span) nir.Span {
	return nir.Span{File: s.File, Line: s.Line, Column: s.Column}
}
func (l *lowerer) check(s Span) error {
	l.count++
	if l.count > nir.MaxNodes || s.File != l.file || s.StartByte < 0 || s.EndByte < s.StartByte || s.EndByte > 64<<10 || s.Line < 1 || s.Column < 1 {
		return l.fail(s, "invalid source span or scalar node budget exceeded")
	}
	return nil
}
func (l *lowerer) bind(b Binding, env map[string]localBinding, kind string) error {
	if err := l.check(b.Loc); err != nil {
		return err
	}
	if !scalarType(b.Type) || !strings.HasPrefix(b.ID, "v") || !idPattern.MatchString(b.ID) || l.ids[b.ID] || b.Name == "" {
		return l.fail(b.Loc, "invalid or duplicate scalar binding")
	}
	l.ids[b.ID] = true
	env[b.ID] = localBinding{kind: kind, typ: b.Type}
	return nil
}
func (l *lowerer) body(stmts []Statement, g *nir.Function, env map[string]localBinding, mustReturn bool) error {
	returned, err := l.sequence(stmts, g, env, mustReturn, 0)
	if err != nil {
		return err
	}
	if mustReturn && !returned {
		return l.fail(Span{File: l.file, Line: g.Loc.Line, Column: g.Loc.Column}, "not all paths return a value")
	}
	return nil
}
func (l *lowerer) materialize(x *nir.Expr, g *nir.Function) *nir.Expr {
	l.temp++
	name := fmt.Sprintf("t%d", l.temp)
	g.Body = append(g.Body, nir.Stmt{Op: "let", Name: name, Value: x, Loc: x.Loc})
	return &nir.Expr{Op: "local", Type: x.Type, Name: name, Loc: x.Loc}
}
func (l *lowerer) expr(e *Expression, g *nir.Function, env map[string]localBinding, depth int) (*nir.Expr, error) {
	if e == nil {
		return nil, l.fail(Span{File: l.file}, "missing scalar expression")
	}
	if err := l.check(e.Loc); err != nil {
		return nil, err
	}
	if depth > nir.MaxDepth || !scalarType(e.Type) {
		return nil, l.fail(e.Loc, "invalid scalar expression type or depth")
	}
	if e.Op != "number" && e.Bits != "" {
		return nil, l.fail(e.Loc, "extraneous literal bits")
	}
	if e.Op != "bool" && e.Bool != nil {
		return nil, l.fail(e.Loc, "extraneous boolean literal")
	}
	if e.Op != "ref" && e.Op != "call" && e.Ref != "" {
		return nil, l.fail(e.Loc, "extraneous declaration reference")
	}
	if e.Op == "and" || e.Op == "or" {
		return l.shortCircuit(e, g, env, depth)
	}
	x := &nir.Expr{Op: e.Op, Type: nir.Type(e.Type), Loc: l.span(e.Loc)}
	want := "number"
	var operands []string
	switch e.Op {
	case "number":
		if _, err := nir.DecodeNumber(e.Bits); err != nil {
			return nil, l.fail(e.Loc, err.Error())
		}
		x.Bits = e.Bits
	case "bool":
		if e.Bool == nil {
			return nil, l.fail(e.Loc, "missing boolean literal")
		}
		value := *e.Bool
		x.Bool = &value
		want = "boolean"
	case "ref":
		binding, ok := env[e.Ref]
		if !ok {
			return nil, l.fail(e.Loc, "unknown, captured, or not-yet-initialized binding")
		}
		x.Op, x.Name, want = binding.kind, e.Ref, binding.typ
	case "add", "sub", "mul", "div":
		operands = []string{"number", "number"}
	case "neg", "pos":
		operands = []string{"number"}
	case "not":
		want, operands = "boolean", []string{"boolean"}
	case "lt", "le", "gt", "ge":
		want, operands = "boolean", []string{"number", "number"}
	case "eq", "ne":
		if len(e.Args) != 2 || e.Args[0] == nil || !scalarType(e.Args[0].Type) {
			return nil, l.fail(e.Loc, "invalid strict equality operands")
		}
		want, operands = "boolean", []string{e.Args[0].Type, e.Args[0].Type}
	case "call":
		fn, ok := l.funcs[e.Ref]
		if !ok {
			return nil, l.fail(e.Loc, "unknown direct function target")
		}
		want, x.Name = fn.Result, fn.ID
		for _, p := range fn.Params {
			operands = append(operands, p.Type)
		}
	default:
		return nil, l.fail(e.Loc, "unsupported scalar expression "+e.Op)
	}
	if e.Type != want || len(e.Args) != len(operands) {
		return nil, l.fail(e.Loc, "result type or operand count does not match signature")
	}
	for i, a := range e.Args {
		arg, err := l.expr(a, g, env, depth+1)
		if err != nil {
			return nil, err
		}
		if string(arg.Type) != operands[i] {
			return nil, l.fail(a.Loc, "operand type does not match signature")
		}
		x.Args = append(x.Args, arg)
	}
	if e.Op == "pos" {
		return x.Args[0], nil
	}
	if e.Op == "neg" {
		// Multiplication by -1 negates every binary64 numerical value exactly,
		// including signed zero. `0 - x` would incorrectly turn +0 into +0.
		// No promise is made about NaN payload/sign preservation by arithmetic.
		x.Op = "mul"
		x.Args = append([]*nir.Expr{{Op: "number", Type: nir.Number, Bits: nir.EncodeNumber(-1), Loc: x.Loc}}, x.Args...)
	}
	if e.Op == "number" || e.Op == "bool" || e.Op == "ref" {
		return x, nil
	}
	return l.materialize(x, g), nil
}
