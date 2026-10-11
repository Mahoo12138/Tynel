// Package tynelbridge runs INSIDE the forked Go compiler. It consumes NIR,
// never TypeScript checker objects and never generated Go business source.
package tynelbridge

import (
	"fmt"
	"go/constant"
	"os"
	"strconv"

	"cmd/compile/internal/base"
	"cmd/compile/internal/ir"
	"cmd/compile/internal/tynelbridge/nir"
	"cmd/compile/internal/typecheck"
	"cmd/compile/internal/types"
	"cmd/internal/src"
)

func Load(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	m, err := nir.Read(f)
	if err != nil {
		return err
	}
	if types.LocalPkg.Name != "main" {
		return fmt.Errorf("bridge requires the research main-package harness")
	}
	b := &builder{functions: make(map[string]*ir.Func), files: make(map[string]*src.PosBase)}
	for _, fn := range typecheck.Target.Funcs {
		b.functions[fn.Sym().Name] = fn
	}
	if err := b.checkHost(); err != nil {
		return err
	}
	for i := range m.Functions {
		fn := &m.Functions[i]
		if fn.Name == "main" {
			continue
		}
		if b.functions[fn.Name] != nil {
			return fmt.Errorf("function conflicts with bootstrap: %s", fn.Name)
		}
		pos := b.pos(fn.Loc)
		params := make([]*types.Field, len(fn.Params))
		for j, p := range fn.Params {
			params[j] = types.NewField(pos, types.LocalPkg.Lookup(p.Name), b.storage(p.Type))
		}
		var results []*types.Field
		if fn.Result != nir.Void {
			results = []*types.Field{types.NewField(pos, nil, b.storage(fn.Result))}
		}
		sym := types.LocalPkg.Lookup(fn.Name)
		g := ir.NewFunc(pos, pos, sym, types.NewSignature(nil, params, results))
		g.Endlineno = pos
		sym.Def = g.Nname
		typecheck.DeclFunc(g)
		typecheck.FinishFuncBody()
		b.functions[fn.Name] = g
	}
	for i := range m.Functions {
		fn := &m.Functions[i]
		g := b.functions[fn.Name]
		old := ir.CurFunc
		ir.CurFunc = g
		params := make(map[string]*ir.Name)
		for j, p := range fn.Params {
			params[p.Name] = g.Type().Params()[j].Nname.(*ir.Name)
		}
		g.Body = nil
		// A parsed zero-parameter main can have no declarations yet.
		if g.Dcl == nil {
			g.Dcl = make([]*ir.Name, 0)
		}
		if fn.Name == "main" {
			g.Body = append(g.Body, typecheck.Stmt(b.host(b.pos(fn.Loc), "tynelProbeABI_v1")))
		}
		g.Body = append(g.Body, b.statements(fn.Body, g, params)...)
		// nil Body denotes an external declaration to the Go backend, not
		// an empty function. Materialize void fallthrough explicitly.
		if fn.Result == nir.Void && (len(fn.Body) == 0 || fn.Body[len(fn.Body)-1].Op != "return") {
			g.Body = append(g.Body, typecheck.Stmt(ir.NewReturnStmt(b.pos(fn.Loc), nil)))
		}
		ir.CurFunc = old
	}
	return nil
}

type builder struct {
	probeRef  *types.Type
	functions map[string]*ir.Func
	files     map[string]*src.PosBase
}

func (b *builder) pos(s nir.Span) src.XPos {
	file := b.files[s.File]
	if file == nil {
		file = src.NewFileBase(s.File, s.File)
		b.files[s.File] = file
	}
	return base.Ctxt.PosTable.XPos(src.MakePos(file, uint(s.Line), uint(s.Column)))
}

func (b *builder) expr(e *nir.Expr, params map[string]*ir.Name) ir.Node {
	pos := b.pos(e.Loc)
	var node ir.Node
	switch e.Op {
	case "bool":
		node = ir.NewBasicLit(pos, types.Types[types.TBOOL], constant.MakeBool(*e.Bool))
	case "not":
		node = ir.NewUnaryExpr(pos, ir.ONOT, b.expr(e.Args[0], params))
	case "lt", "le", "gt", "ge", "eq", "ne":
		op := map[string]ir.Op{"lt": ir.OLT, "le": ir.OLE, "gt": ir.OGT, "ge": ir.OGE, "eq": ir.OEQ, "ne": ir.ONE}[e.Op]
		// Keep NaN-aware comparisons as comparisons, not negated opposites.
		node = ir.NewBinaryExpr(pos, op, b.expr(e.Args[0], params), b.expr(e.Args[1], params))
	case "number":
		bits, err := strconv.ParseUint(e.Bits, 16, 64)
		if err != nil {
			panic("validated binary64 bits changed")
		}
		u := ir.NewBasicLit(pos, types.Types[types.TUINT64], constant.MakeUint64(bits))
		node = ir.NewCallExpr(pos, ir.OCALL, b.functions["tynelF64"].Nname, []ir.Node{u})
	case "param", "local":
		return params[e.Name]
	case "probe_new":
		// Normal typed new: escape analysis and Go's allocator lowering remain active.
		node = ir.NewUnaryExpr(pos, ir.ONEW, ir.TypeNode(b.probeRef.Elem()))
	case "probe_value", "probe_next":
		index := 0
		if e.Op == "probe_next" {
			index = 1
		}
		return typecheck.DotField(pos, b.expr(e.Args[0], params), index)
	case "add", "sub", "mul", "div":
		op := map[string]ir.Op{"add": ir.OADD, "sub": ir.OSUB, "mul": ir.OMUL, "div": ir.ODIV}[e.Op]
		node = ir.NewBinaryExpr(pos, op, b.expr(e.Args[0], params), b.expr(e.Args[1], params))
		// Explicit same-type float conversion records the source operation's
		// rounding boundary (Round64F in Go SSA); temporaries alone do not
		// prevent Go from fusing multiply/add across statements.
		node = ir.NewConvExpr(pos, ir.OCONV, types.Types[types.TFLOAT64], typecheck.Expr(node))
	case "call":
		args := make([]ir.Node, len(e.Args))
		for i, arg := range e.Args {
			args[i] = b.expr(arg, params)
		}
		node = ir.NewCallExpr(pos, ir.OCALL, b.functions[e.Name].Nname, args)
	default:
		panic("validated expression missing in bridge")
	}
	return typecheck.Expr(node)
}
