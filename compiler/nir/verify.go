package nir

import (
	"fmt"
	"regexp"
	"strings"
)

const MaxDepth = 64
const MaxFunctions = 128
const MaxNodes = 4096

var identifier = regexp.MustCompile(`^[a-z][A-Za-z0-9_]*$`)

type Diagnostic struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Loc     Span   `json:"loc"`
}

func (d Diagnostic) Error() string {
	return fmt.Sprintf("%s:%d:%d: %s: %s", d.Loc.File, d.Loc.Line, d.Loc.Column, d.Code, d.Message)
}

// Validate fails closed: every node is checked, including unused functions.
// This slice intentionally has no general reachability or dead-code exemption.
func Validate(m *Module) error {
	if m == nil {
		return fail(Span{}, "nil module")
	}
	if m.IRVersion != Version {
		return fail(Span{}, "unsupported IR version %d (want %d)", m.IRVersion, Version)
	}
	if m.Semantics != Semantics {
		return fail(Span{}, "unsupported semantics %q", m.Semantics)
	}
	if m.Package != "main" {
		return fail(Span{}, "M1 accepts only package main")
	}
	if len(m.Functions) == 0 || len(m.Functions) > MaxFunctions {
		return fail(Span{}, "function count must be 1..%d", MaxFunctions)
	}
	funcs := make(map[string]*Function)
	for i := range m.Functions {
		fn := &m.Functions[i]
		if !identifier.MatchString(fn.Name) || fn.Name == "init" || strings.HasPrefix(fn.Name, "tynel") {
			return fail(fn.Loc, "invalid or reserved function name %q", fn.Name)
		}
		if funcs[fn.Name] != nil {
			return fail(fn.Loc, "duplicate function %q", fn.Name)
		}
		if err := validSpan(fn.Loc); err != nil {
			return err
		}
		funcs[fn.Name] = fn
	}
	entry := funcs["main"]
	if entry == nil || len(entry.Params) != 0 || entry.Result != Void {
		return fail(Span{}, "main must have no parameters and return void")
	}
	count := 0
	for i := range m.Functions {
		fn := &m.Functions[i]
		if fn.Result != Number && fn.Result != Void {
			return fail(fn.Loc, "unsupported result type %q", fn.Result)
		}
		if len(fn.Params) > 32 || len(fn.Body) > MaxNodes {
			return fail(fn.Loc, "function exceeds resource limits")
		}
		params := make(map[string]Type)
		for _, p := range fn.Params {
			if !identifier.MatchString(p.Name) || p.Type != Number || params[p.Name] != "" {
				return fail(fn.Loc, "invalid or duplicate parameter %q", p.Name)
			}
			params[p.Name] = p.Type
		}
		returned := false
		for _, stmt := range fn.Body {
			count++
			if count > MaxNodes {
				return fail(stmt.Loc, "IR node budget exceeded")
			}
			if returned {
				return fail(stmt.Loc, "statement after return")
			}
			if err := validSpan(stmt.Loc); err != nil {
				return err
			}
			switch stmt.Op {
			case "return":
				returned = true
				if fn.Result == Void {
					if stmt.Value != nil {
						return fail(stmt.Loc, "void return has a value")
					}
					continue
				}
			case "print_number":
				// A probe host operation, deliberately not JS console.log.
			default:
				return fail(stmt.Loc, "unsupported statement %q", stmt.Op)
			}
			if err := checkExpr(stmt.Value, params, funcs, 0, &count); err != nil {
				return err
			}
		}
		if fn.Result != Void && !returned {
			return fail(fn.Loc, "number function %q has no return", fn.Name)
		}
	}
	return nil
}

func checkExpr(e *Expr, params map[string]Type, funcs map[string]*Function, depth int, count *int) error {
	if e == nil {
		return fail(Span{}, "missing expression")
	}
	*count++
	if depth > MaxDepth || *count > MaxNodes {
		return fail(e.Loc, "expression depth or node budget exceeded")
	}
	if err := validSpan(e.Loc); err != nil {
		return err
	}
	if e.Type != Number {
		return fail(e.Loc, "M1 expression type must be number, got %q", e.Type)
	}
	switch e.Op {
	case "number":
		if e.Name != "" || len(e.Args) != 0 {
			return fail(e.Loc, "number has extraneous operands")
		}
		if _, err := DecodeNumber(e.Bits); err != nil {
			return fail(e.Loc, "%v", err)
		}
	case "param":
		if e.Bits != "" || len(e.Args) != 0 || params[e.Name] != Number {
			return fail(e.Loc, "unknown parameter or malformed reference %q", e.Name)
		}
	case "add", "sub", "mul", "div":
		if e.Bits != "" || e.Name != "" || len(e.Args) != 2 {
			return fail(e.Loc, "%s needs exactly two operands", e.Op)
		}
	case "call":
		fn := funcs[e.Name]
		if e.Bits != "" || fn == nil || fn.Result != Number || len(fn.Params) != len(e.Args) {
			return fail(e.Loc, "unknown or mismatched call %q", e.Name)
		}
	default:
		return fail(e.Loc, "unsupported expression %q", e.Op)
	}
	for _, arg := range e.Args {
		if err := checkExpr(arg, params, funcs, depth+1, count); err != nil {
			return err
		}
	}
	return nil
}

func validSpan(s Span) error {
	if s.File == "" || len(s.File) > 1024 || strings.ContainsAny(s.File, "\x00\r\n") || s.Line < 1 || s.Line > (1<<20) || s.Column < 1 || s.Column > (1<<16) {
		return fail(s, "invalid source span")
	}
	return nil
}

func fail(loc Span, format string, args ...any) error {
	return Diagnostic{Code: "TY4001", Loc: loc, Message: fmt.Sprintf(format, args...)}
}
