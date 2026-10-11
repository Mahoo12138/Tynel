package nir

import "maps"

func isShortCircuit(op string) bool { return op == "short_and" || op == "short_or" }

func checkShortCircuit(s Stmt, values map[string]binding, funcs map[string]*Function, probe bool, depth int, count *int) error {
	if !identifier.MatchString(s.Name) || values[s.Name].typ != "" {
		return fail(s.Loc, "invalid or duplicate short-circuit result %q", s.Name)
	}
	if err := checkExpr(s.Condition, values, funcs, probe, depth+1, count); err != nil {
		return err
	}
	if s.Condition.Type != Boolean {
		return fail(s.Loc, "short-circuit left operand must be boolean")
	}
	if len(s.Body) > MaxNodes {
		return fail(s.Loc, "short-circuit body exceeds node budget")
	}
	// An RHS computes a value, not a function exit or arbitrary source block.
	// Restrict its preparation to immutable temporaries and nested logicals.
	for _, step := range s.Body {
		if step.Op != "let" && !isShortCircuit(step.Op) {
			return fail(step.Loc, "short-circuit RHS accepts only let or nested short-circuit steps")
		}
	}
	rightScope := maps.Clone(values)
	if _, err := checkBody(s.Body, rightScope, funcs, probe, Void, depth+1, count); err != nil {
		return err
	}
	if err := checkExpr(s.Value, rightScope, funcs, probe, depth+1, count); err != nil {
		return err
	}
	if s.Value.Type != Boolean {
		return fail(s.Loc, "short-circuit right result must be boolean")
	}
	// Neither RHS locals nor a partially initialized result enter the outer scope.
	values[s.Name] = binding{Boolean, false}
	return nil
}
