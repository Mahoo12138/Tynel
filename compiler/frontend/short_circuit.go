package frontend

import (
	"fmt"
	"maps"

	"tynel.local/tynel/compiler/nir"
)

// A logical RHS has its own statement sink: eager materialization into g would
// run its calls even when the LHS already determines the result.
func (l *lowerer) shortCircuit(e *Expression, g *nir.Function, env map[string]localBinding, depth int) (*nir.Expr, error) {
	if e.Type != "boolean" || len(e.Args) != 2 {
		return nil, l.fail(e.Loc, "short-circuit operators require two boolean operands")
	}
	left, err := l.expr(e.Args[0], g, env, depth+1)
	if err != nil {
		return nil, err
	}
	if left.Type != nir.Boolean {
		return nil, l.fail(e.Loc, "short-circuit left operand must be boolean")
	}
	rightBody := nir.Function{Result: nir.Boolean}
	// Always validate/extract the RHS, even for false && rhs or true || rhs.
	// Only its *runtime execution* is conditional.
	right, err := l.expr(e.Args[1], &rightBody, maps.Clone(env), depth+1)
	if err != nil {
		return nil, err
	}
	if right.Type != nir.Boolean {
		return nil, l.fail(e.Loc, "short-circuit right operand must be boolean")
	}
	l.temp++
	name := fmt.Sprintf("t%d", l.temp)
	g.Body = append(g.Body, nir.Stmt{
		Op: "short_" + e.Op, Name: name, Condition: left,
		Body: rightBody.Body, Value: right, Loc: l.span(e.Loc),
	})
	return &nir.Expr{Op: "local", Type: nir.Boolean, Name: name, Loc: l.span(e.Loc)}, nil
}
