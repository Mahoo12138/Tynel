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
		if !strings.HasPrefix(f.ID, "f") || !idPattern.MatchString(f.ID) || l.ids[f.ID] || f.Result != "number" || len(f.Params) > 32 {
			return nil, l.fail(f.Loc, "invalid or duplicate scalar function")
		}
		l.ids[f.ID] = true
		l.funcs[f.ID] = f
	}
	m := &nir.Module{IRVersion: nir.Version, Semantics: nir.Semantics, Package: "main"}
	for _, f := range p.Functions {
		g := nir.Function{Name: f.ID, Result: nir.Number, Loc: l.span(f.Loc)}
		env := make(map[string]string)
		for _, b := range f.Params {
			if err := l.bind(b, env, "param"); err != nil {
				return nil, err
			}
			g.Params = append(g.Params, nir.Param{Name: b.ID, Type: nir.Number})
		}
		if err := l.body(f.Body, &g, env, true); err != nil {
			return nil, err
		}
		m.Functions = append(m.Functions, g)
	}
	main := nir.Function{Name: "main", Result: nir.Void, Loc: nir.Span{File: p.File, Line: 1, Column: 1}}
	if err := l.body(p.Body, &main, make(map[string]string), false); err != nil {
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
func (l *lowerer) bind(b Binding, env map[string]string, kind string) error {
	if err := l.check(b.Loc); err != nil {
		return err
	}
	if b.Type != "number" || !strings.HasPrefix(b.ID, "v") || !idPattern.MatchString(b.ID) || l.ids[b.ID] || b.Name == "" {
		return l.fail(b.Loc, "invalid or duplicate scalar binding")
	}
	l.ids[b.ID] = true
	env[b.ID] = kind
	return nil
}
func (l *lowerer) body(stmts []Statement, g *nir.Function, env map[string]string, mustReturn bool) error {
	if len(stmts) > nir.MaxNodes {
		return l.fail(Span{File: l.file}, "statement budget exceeded")
	}
	returned := false
	for _, s := range stmts {
		if err := l.check(s.Loc); err != nil {
			return err
		}
		if returned {
			return l.fail(s.Loc, "statements after return are not supported")
		}
		if s.Op != "const" && s.Binding != nil {
			return l.fail(s.Loc, "unexpected binding on statement")
		}
		x, err := l.expr(s.Value, g, env, 0)
		if err != nil {
			return err
		}
		switch s.Op {
		case "const":
			if s.Binding == nil {
				return l.fail(s.Loc, "missing const binding")
			}
			if err := l.bind(*s.Binding, env, "local"); err != nil {
				return err
			}
			g.Body = append(g.Body, nir.Stmt{Op: "let", Name: s.Binding.ID, Value: x, Loc: l.span(s.Loc)})
		case "return":
			if !mustReturn {
				return l.fail(s.Loc, "top-level return is not supported")
			}
			g.Body = append(g.Body, nir.Stmt{Op: "return", Value: x, Loc: l.span(s.Loc)})
			returned = true
		case "print":
			g.Body = append(g.Body, nir.Stmt{Op: "print_number", Value: x, Loc: l.span(s.Loc)})
		case "discard":
			// Evaluate without printing. A bare `answer;` is not console.log.
			l.materialize(x, g)
		default:
			return l.fail(s.Loc, "unsupported scalar statement "+s.Op)
		}
	}
	if mustReturn && !returned {
		return l.fail(Span{File: l.file}, "number function must return a value")
	}
	return nil
}
func (l *lowerer) materialize(x *nir.Expr, g *nir.Function) *nir.Expr {
	l.temp++
	name := fmt.Sprintf("t%d", l.temp)
	g.Body = append(g.Body, nir.Stmt{Op: "let", Name: name, Value: x, Loc: x.Loc})
	return &nir.Expr{Op: "local", Type: nir.Number, Name: name, Loc: x.Loc}
}
func (l *lowerer) expr(e *Expression, g *nir.Function, env map[string]string, depth int) (*nir.Expr, error) {
	if e == nil {
		return nil, l.fail(Span{File: l.file}, "missing scalar expression")
	}
	if err := l.check(e.Loc); err != nil {
		return nil, err
	}
	if depth > nir.MaxDepth || e.Type != "number" {
		return nil, l.fail(e.Loc, "invalid scalar expression type or depth")
	}
	if e.Op != "number" && e.Bits != "" {
		return nil, l.fail(e.Loc, "extraneous literal bits")
	}
	if e.Op != "ref" && e.Op != "call" && e.Ref != "" {
		return nil, l.fail(e.Loc, "extraneous declaration reference")
	}
	x := &nir.Expr{Op: e.Op, Type: nir.Number, Loc: l.span(e.Loc)}
	arity := 0
	switch e.Op {
	case "number":
		if _, err := nir.DecodeNumber(e.Bits); err != nil {
			return nil, l.fail(e.Loc, err.Error())
		}
		x.Bits = e.Bits
	case "ref":
		kind, ok := env[e.Ref]
		if !ok {
			return nil, l.fail(e.Loc, "unknown, captured, or not-yet-initialized binding")
		}
		x.Op, x.Name = kind, e.Ref
	case "add", "sub", "mul", "div":
		arity = 2
	case "neg", "pos":
		arity = 1
	case "call":
		fn, ok := l.funcs[e.Ref]
		if !ok {
			return nil, l.fail(e.Loc, "unknown direct function target")
		}
		arity, x.Name = len(fn.Params), fn.ID
	default:
		return nil, l.fail(e.Loc, "unsupported scalar expression "+e.Op)
	}
	if len(e.Args) != arity {
		return nil, l.fail(e.Loc, "operand count does not match signature")
	}
	for _, a := range e.Args {
		arg, err := l.expr(a, g, env, depth+1)
		if err != nil {
			return nil, err
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
	if e.Op == "number" || e.Op == "ref" {
		return x, nil
	}
	return l.materialize(x, g), nil
}
