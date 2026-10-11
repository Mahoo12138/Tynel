package nir

import (
	"maps"
	"strings"
)

// checkBody returns whether every path through this sequence returns. There is
// no constant-condition pruning: both branches must be valid even for `false`.
// Each branch has a copied lexical environment; its declarations cannot escape.
func checkBody(body []Stmt, values map[string]binding, funcs map[string]*Function, probe bool, result Type, depth int, count *int) (bool, error) {
	if depth > MaxDepth || len(body) > MaxNodes {
		return false, fail(Span{}, "statement depth or node budget exceeded")
	}
	returned := false
	for _, s := range body {
		(*count)++
		if *count > MaxNodes {
			return false, fail(s.Loc, "IR node budget exceeded")
		}
		if returned {
			return false, fail(s.Loc, "statement after return")
		}
		if err := validSpan(s.Loc); err != nil {
			return false, err
		}
		if s.Op != "let" && s.Name != "" {
			return false, fail(s.Loc, "extraneous statement binding")
		}
		paired := s.Op == "probe_store_value" || s.Op == "probe_store_next" || s.Op == "probe_assert_same"
		if !paired && s.Target != nil {
			return false, fail(s.Loc, "extraneous statement target")
		}
		if s.Op != "if" && (s.Condition != nil || s.Then != nil || s.Else != nil) {
			return false, fail(s.Loc, "extraneous branch fields")
		}
		if s.Op != "block" && s.Body != nil {
			return false, fail(s.Loc, "extraneous block body")
		}
		if strings.HasPrefix(s.Op, "probe_") && !probe {
			return false, fail(s.Loc, "%s requires feature %s", s.Op, ProbeGCFeature)
		}
		if s.Op == "if" || s.Op == "block" {
			if s.Value != nil {
				return false, fail(s.Loc, "control statement has extraneous value")
			}
			if s.Op == "block" {
				done, err := checkBody(s.Body, maps.Clone(values), funcs, probe, result, depth+1, count)
				if err != nil {
					return false, err
				}
				returned = done
				continue
			}
			if err := checkExpr(s.Condition, values, funcs, probe, depth+1, count); err != nil {
				return false, err
			}
			if s.Condition.Type != Boolean {
				return false, fail(s.Loc, "if condition must be boolean; no implicit truthiness")
			}
			yes, err := checkBody(s.Then, maps.Clone(values), funcs, probe, result, depth+1, count)
			if err != nil {
				return false, err
			}
			no, err := checkBody(s.Else, maps.Clone(values), funcs, probe, result, depth+1, count)
			if err != nil {
				return false, err
			}
			returned = yes && no
			continue
		}
		want := Number
		switch s.Op {
		case "let":
			if !identifier.MatchString(s.Name) || values[s.Name].typ != "" {
				return false, fail(s.Loc, "invalid or duplicate local %q", s.Name)
			}
			if err := checkExpr(s.Value, values, funcs, probe, depth+1, count); err != nil {
				return false, err
			}
			// Publish only after validating the initializer, excluding self/forward reads.
			values[s.Name] = binding{s.Value.Type, false}
			continue
		case "return":
			returned = true
			if result == Void {
				if s.Value != nil {
					return false, fail(s.Loc, "void return has a value")
				}
				continue
			}
			want = result
		case "print_number":
		case "print_boolean":
			want = Boolean
		case "probe_gc", "probe_stack":
			if s.Value != nil {
				return false, fail(s.Loc, "%s takes no operands", s.Op)
			}
			continue
		case "probe_store_value", "probe_store_next", "probe_assert_same":
			if err := checkExpr(s.Target, values, funcs, probe, depth+1, count); err != nil {
				return false, err
			}
			if s.Target.Type != ProbeRef {
				return false, fail(s.Loc, "probe target must be probe_ref")
			}
			if s.Op != "probe_store_value" {
				want = ProbeRef
			}
		default:
			return false, fail(s.Loc, "unsupported statement %q", s.Op)
		}
		if err := checkExpr(s.Value, values, funcs, probe, depth+1, count); err != nil {
			return false, err
		}
		if s.Value.Type != want {
			return false, fail(s.Loc, "%s expects %s, got %s", s.Op, want, s.Value.Type)
		}
	}
	return returned, nil
}
