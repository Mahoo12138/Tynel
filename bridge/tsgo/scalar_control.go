package tynelapi

import (
	"github.com/microsoft/typescript-go/internal/ast"
	"github.com/microsoft/typescript-go/internal/checker"
	"maps"
	"tynel.local/tynel/compiler/frontend"
)

// The pinned checker may represent `boolean` as the union false | true.
// Inspect flags and members; never classify types by their display strings.
func scalarKind(t *checker.Type) string {
	if numeric(t) {
		return "number"
	}
	if t == nil {
		return ""
	}
	switch t.Flags() {
	case checker.TypeFlagsBoolean, checker.TypeFlagsBooleanLiteral:
		return "boolean"
	case checker.TypeFlagsUnion, checker.TypeFlagsUnion | checker.TypeFlagsBoolean:
		members := t.Types()
		if len(members) != 2 {
			return ""
		}
		values := [2]bool{}
		for i, member := range members {
			if member.Flags() != checker.TypeFlagsBooleanLiteral {
				return ""
			}
			value, ok := member.AsLiteralType().Value().(bool)
			if !ok {
				return ""
			}
			values[i] = value
		}
		if values[0] != values[1] {
			return "boolean"
		}
	}
	return ""
}

func scalarAnnotation(n *ast.Node) bool {
	return n != nil && (n.Kind == ast.KindNumberKeyword || n.Kind == ast.KindBooleanKeyword)
}

func (e *extractor) branch(n *ast.Node, env map[*ast.Symbol]string, top bool, depth int) ([]frontend.Statement, error) {
	if n.Kind == ast.KindBlock {
		if err := e.count(n, depth); err != nil {
			return nil, err
		}
		return e.statements(n.AsBlock().Statements.Nodes, maps.Clone(env), top, depth)
	}
	return e.statements([]*ast.Node{n}, maps.Clone(env), top, depth)
}
