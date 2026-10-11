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

type binding struct {
	typ       Type
	parameter bool
}

func valueType(t Type, probe bool) bool {
	return t == Number || t == Boolean || probe && t == ProbeRef
}

// Validate checks every body, including unused functions. Probe operations
// require an explicit feature; they are not source-language object support.
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
	probe := len(m.Features) == 1 && m.Features[0] == ProbeGCFeature
	if len(m.Features) != 0 && !probe {
		return fail(Span{}, "unknown or duplicate experimental features")
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
		if fn.Result != Void && !valueType(fn.Result, probe) {
			return fail(fn.Loc, "unsupported result type %q", fn.Result)
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
		if len(fn.Params) > 32 || len(fn.Body) > MaxNodes {
			return fail(fn.Loc, "function exceeds resource limits")
		}
		values := make(map[string]binding)
		for _, p := range fn.Params {
			if !identifier.MatchString(p.Name) || !valueType(p.Type, probe) || values[p.Name].typ != "" {
				return fail(fn.Loc, "invalid or duplicate parameter %q", p.Name)
			}
			values[p.Name] = binding{p.Type, true}
		}
		returned, err := checkBody(fn.Body, values, funcs, probe, fn.Result, 0, &count)
		if err != nil {
			return err
		}
		if fn.Result != Void && !returned {
			return fail(fn.Loc, "not all paths return a value in %q", fn.Name)
		}
	}
	return nil
}

func checkExpr(e *Expr, values map[string]binding, funcs map[string]*Function, probe bool, depth int, count *int) error {
	if e == nil {
		return fail(Span{}, "missing expression")
	}
	(*count)++
	if depth > MaxDepth || *count > MaxNodes {
		return fail(e.Loc, "expression depth or node budget exceeded")
	}
	if err := validSpan(e.Loc); err != nil {
		return err
	}
	if !valueType(e.Type, probe) {
		return fail(e.Loc, "unsupported expression type %q (probe_ref requires %s)", e.Type, ProbeGCFeature)
	}
	if e.Op != "number" && e.Bits != "" {
		return fail(e.Loc, "extraneous number bits")
	}
	if e.Op != "bool" && e.Bool != nil {
		return fail(e.Loc, "extraneous boolean literal")
	}
	if e.Op != "param" && e.Op != "local" && e.Op != "call" && e.Name != "" {
		return fail(e.Loc, "extraneous expression name")
	}
	if strings.HasPrefix(e.Op, "probe_") && !probe {
		return fail(e.Loc, "%s requires %s", e.Op, ProbeGCFeature)
	}
	want := Number
	var operands []Type
	switch e.Op {
	case "number":
		if e.Name != "" {
			return fail(e.Loc, "number has extraneous name")
		}
		if _, err := DecodeNumber(e.Bits); err != nil {
			return fail(e.Loc, "%v", err)
		}
	case "bool":
		if e.Bool == nil {
			return fail(e.Loc, "missing boolean literal")
		}
		want = Boolean
	case "not":
		want, operands = Boolean, []Type{Boolean}
	case "lt", "le", "gt", "ge":
		want, operands = Boolean, []Type{Number, Number}
	case "eq", "ne":
		if len(e.Args) != 2 || e.Args[0] == nil || (e.Args[0].Type != Number && e.Args[0].Type != Boolean) {
			return fail(e.Loc, "strict equality requires two same-kind scalar operands")
		}
		want, operands = Boolean, []Type{e.Args[0].Type, e.Args[0].Type}
	case "param", "local":
		v, ok := values[e.Name]
		if !ok || v.parameter != (e.Op == "param") {
			return fail(e.Loc, "unknown %s or malformed reference %q", e.Op, e.Name)
		}
		want = v.typ
	case "add", "sub", "mul", "div":
		operands = []Type{Number, Number}
	case "call":
		fn := funcs[e.Name]
		if fn == nil || !valueType(fn.Result, probe) || len(fn.Params) != len(e.Args) {
			return fail(e.Loc, "unknown or mismatched call %q", e.Name)
		}
		want = fn.Result
		for _, p := range fn.Params {
			operands = append(operands, p.Type)
		}
	case "probe_new":
		want = ProbeRef
	case "probe_value":
		operands = []Type{ProbeRef}
	case "probe_next":
		want, operands = ProbeRef, []Type{ProbeRef}
	default:
		return fail(e.Loc, "unsupported expression %q", e.Op)
	}
	if e.Type != want || len(e.Args) != len(operands) {
		return fail(e.Loc, "%s has wrong result type or operand count", e.Op)
	}
	for i, arg := range e.Args {
		if err := checkExpr(arg, values, funcs, probe, depth+1, count); err != nil {
			return err
		}
		if arg.Type != operands[i] {
			return fail(arg.Loc, "%s operand %d expects %s, got %s", e.Op, i, operands[i], arg.Type)
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
