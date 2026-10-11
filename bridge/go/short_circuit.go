package tynelbridge

import (
	"maps"

	"cmd/compile/internal/ir"
	"cmd/compile/internal/tynelbridge/nir"
	"cmd/compile/internal/typecheck"
	"cmd/compile/internal/types"
)

func (b *builder) shortCircuit(s nir.Stmt, g *ir.Func, env map[string]*ir.Name) []ir.Node {
	pos := b.pos(s.Loc)
	left := b.expr(s.Condition, env)
	result := g.NewLocal(pos, &types.Sym{Name: s.Name, Pkg: types.LocalPkg}, b.storage(nir.Boolean))
	initial := ir.NewAssignStmt(pos, result, left)
	result.Defn = initial
	out := []ir.Node{
		typecheck.Stmt(ir.NewDecl(pos, ir.ODCL, result)),
		typecheck.Stmt(initial),
	}
	// Keep RHS preparations and its final assignment inside the conditional.
	// The result is absent from this environment until its initializer is built.
	rightScope := maps.Clone(env)
	rightBody := b.statements(s.Body, g, rightScope)
	rightBody = append(rightBody, typecheck.Stmt(ir.NewAssignStmt(b.pos(s.Value.Loc), result, b.expr(s.Value, rightScope))))
	var condition ir.Node = result
	if s.Op == "short_or" {
		condition = typecheck.Expr(ir.NewUnaryExpr(pos, ir.ONOT, result))
	}
	out = append(out, typecheck.Stmt(ir.NewIfStmt(pos, condition, rightBody, nil)))
	env[s.Name] = result
	return out
}
