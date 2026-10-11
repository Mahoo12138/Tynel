package frontend

import (
	"maps"
	"tynel.local/tynel/compiler/nir"
)

type localBinding struct{ kind, typ string }

func scalarType(t string) bool { return t == "number" || t == "boolean" }

// sequence emits each branch into its own sink. Calls in an unchosen branch
// remain there; extracting all temporaries before the if would change effects.
func (l *lowerer) sequence(stmts []Statement, g *nir.Function, env map[string]localBinding, canReturn bool, depth int) (bool, error) {
	if depth > nir.MaxDepth || len(stmts) > nir.MaxNodes {
		return false, l.fail(Span{File: l.file}, "statement depth or node budget exceeded")
	}
	returned := false
	for _, s := range stmts {
		if err := l.check(s.Loc); err != nil {
			return false, err
		}
		if returned {
			return false, l.fail(s.Loc, "statements after return are not supported")
		}
		if s.Op != "const" && s.Binding != nil {
			return false, l.fail(s.Loc, "unexpected binding on statement")
		}
		if s.Op != "if" && (s.Condition != nil || s.Then != nil || s.Else != nil) {
			return false, l.fail(s.Loc, "extraneous branch fields")
		}
		if s.Op != "block" && s.Body != nil {
			return false, l.fail(s.Loc, "extraneous block body")
		}
		if s.Op == "if" || s.Op == "block" {
			if s.Value != nil {
				return false, l.fail(s.Loc, "control statement has extraneous value")
			}
			if s.Op == "block" {
				block := nir.Function{Result: g.Result}
				done, err := l.sequence(s.Body, &block, maps.Clone(env), canReturn, depth+1)
				if err != nil {
					return false, err
				}
				g.Body = append(g.Body, nir.Stmt{Op: "block", Body: block.Body, Loc: l.span(s.Loc)})
				returned = done
				continue
			}
			condition, err := l.expr(s.Condition, g, env, depth+1)
			if err != nil {
				return false, err
			}
			if condition.Type != nir.Boolean {
				return false, l.fail(s.Loc, "if condition must be boolean; no implicit truthiness")
			}
			yes, no := nir.Function{Result: g.Result}, nir.Function{Result: g.Result}
			yesReturn, err := l.sequence(s.Then, &yes, maps.Clone(env), canReturn, depth+1)
			if err != nil {
				return false, err
			}
			noReturn, err := l.sequence(s.Else, &no, maps.Clone(env), canReturn, depth+1)
			if err != nil {
				return false, err
			}
			g.Body = append(g.Body, nir.Stmt{Op: "if", Condition: condition, Then: yes.Body, Else: no.Body, Loc: l.span(s.Loc)})
			returned = yesReturn && noReturn
			continue
		}
		x, err := l.expr(s.Value, g, env, depth+1)
		if err != nil {
			return false, err
		}
		switch s.Op {
		case "const":
			if s.Binding == nil {
				return false, l.fail(s.Loc, "missing const binding")
			}
			if string(x.Type) != s.Binding.Type {
				return false, l.fail(s.Loc, "initializer does not match binding type")
			}
			if err := l.bind(*s.Binding, env, "local"); err != nil {
				return false, err
			}
			g.Body = append(g.Body, nir.Stmt{Op: "let", Name: s.Binding.ID, Value: x, Loc: l.span(s.Loc)})
		case "return":
			if !canReturn {
				return false, l.fail(s.Loc, "top-level return is not supported")
			}
			if x.Type != g.Result {
				return false, l.fail(s.Loc, "return value does not match function result")
			}
			g.Body = append(g.Body, nir.Stmt{Op: "return", Value: x, Loc: l.span(s.Loc)})
			returned = true
		case "print":
			op := "print_number"
			if x.Type == nir.Boolean {
				op = "print_boolean"
			}
			g.Body = append(g.Body, nir.Stmt{Op: op, Value: x, Loc: l.span(s.Loc)})
		case "discard":
			l.materialize(x, g)
		default:
			return false, l.fail(s.Loc, "unsupported scalar statement "+s.Op)
		}
	}
	return returned, nil
}
