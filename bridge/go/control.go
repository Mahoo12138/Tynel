package tynelbridge

import (
	"cmd/compile/internal/ir"
	"cmd/compile/internal/tynelbridge/nir"
	"cmd/compile/internal/typecheck"
	"cmd/compile/internal/types"
	"maps"
)

// Build nested statement lists without hoisting branch-local declarations or
// calls. Go's later stages derive the CFG and SSA from these high-level nodes.
func (b *builder) statements(stmts []nir.Stmt, g *ir.Func, env map[string]*ir.Name) []ir.Node {
	var out []ir.Node
	for _, s := range stmts {
		pos := b.pos(s.Loc)
		var node ir.Node
		switch s.Op {
		case "short_and", "short_or":
			out = append(out, b.shortCircuit(s, g, env)...)
			continue
		case "if":
			condition := b.expr(s.Condition, env)
			yes := b.statements(s.Then, g, maps.Clone(env))
			no := b.statements(s.Else, g, maps.Clone(env))
			node = ir.NewIfStmt(pos, condition, yes, no)
		case "block":
			// Source scope was checked in NIR; fresh symbols preserve identity
			// even when the backend can flatten this purely lexical block.
			out = append(out, b.statements(s.Body, g, maps.Clone(env))...)
			continue
		case "let":
			rhs := b.expr(s.Value, env)
			name := g.NewLocal(pos, &types.Sym{Name: s.Name, Pkg: types.LocalPkg}, b.storage(s.Value.Type))
			env[s.Name] = name
			out = append(out, typecheck.Stmt(ir.NewDecl(pos, ir.ODCL, name)))
			node = ir.NewAssignStmt(pos, name, rhs)
			name.Defn = node
		case "probe_store_value", "probe_store_next":
			index := 0
			if s.Op == "probe_store_next" {
				index = 1
			}
			lhs := typecheck.DotField(pos, b.expr(s.Target, env), index)
			node = ir.NewAssignStmt(pos, lhs, b.expr(s.Value, env))
		case "probe_gc":
			node = b.host(pos, "tynelProbeGC")
		case "probe_stack":
			node = b.host(pos, "tynelProbeStack")
		case "probe_assert_same":
			node = b.host(pos, "tynelProbeAssertSame", b.expr(s.Target, env), b.expr(s.Value, env))
		case "return":
			var values []ir.Node
			if s.Value != nil {
				values = []ir.Node{b.expr(s.Value, env)}
			}
			node = ir.NewReturnStmt(pos, values)
		case "print_number":
			node = b.host(pos, "tynelPrint", b.expr(s.Value, env))
		case "print_boolean":
			node = b.host(pos, "tynelPrintBool", b.expr(s.Value, env))
		default:
			panic("validated statement missing in bridge")
		}
		out = append(out, typecheck.Stmt(node))
	}
	return out
}
