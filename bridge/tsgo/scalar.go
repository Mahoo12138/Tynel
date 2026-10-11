package tynelapi

import (
	"context"
	"fmt"
	"strings"

	"github.com/microsoft/typescript-go/internal/ast"
	"github.com/microsoft/typescript-go/internal/checker"
	"github.com/microsoft/typescript-go/internal/jsnum"
	"github.com/microsoft/typescript-go/internal/scanner"
	"tynel.local/tynel/compiler/frontend"
	"tynel.local/tynel/compiler/nir"
)

// ScalarFacts copies a bounded, symbol-resolved projection. The caller may close
// the session before native validation/lowering. It never parses TypeToString.
func (s *Session) ScalarFacts(filename string) (*frontend.Program, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.program == nil {
		return nil, ErrClosed
	}
	if !s.scalar {
		return nil, fmt.Errorf("TY1201: scalar extraction requires NewScalar")
	}
	if len(s.snapshot.Diagnostics) != 0 {
		return nil, ErrSourceDiagnostics
	}
	if filename == "" || len(filename) > 1024 || strings.ContainsAny(filename, "\x00\r\n") {
		return nil, fmt.Errorf("TY1201: invalid source name")
	}
	c, release := s.program.GetCheckerPool().GetChecker(context.Background(), s.file)
	defer release()
	e := &extractor{s: s, c: c, file: strings.ReplaceAll(filename, "\\", "/"), functions: map[*ast.Symbol]frontend.Function{}, declarations: map[string]*ast.Node{}}
	p := &frontend.Program{Version: frontend.Version, Profile: frontend.Profile, File: e.file, SourceSHA256: s.snapshot.SourceSHA256}
	for _, n := range s.file.Statements.Nodes {
		if n.Kind != ast.KindFunctionDeclaration {
			continue
		}
		f, err := e.signature(n)
		if err != nil {
			return nil, err
		}
		p.Functions = append(p.Functions, f)
	}
	for i := range p.Functions {
		f := &p.Functions[i]
		n := e.declarations[f.ID]
		env := map[*ast.Symbol]string{}
		for j, arg := range n.AsFunctionDeclaration().Parameters.Nodes {
			env[c.GetSymbolAtLocation(arg.Name())] = f.Params[j].ID
		}
		body, err := e.statements(n.AsFunctionDeclaration().Body.AsBlock().Statements.Nodes, env, false, 0)
		if err != nil {
			return nil, err
		}
		f.Body = body
	}
	body, err := e.statements(s.file.Statements.Nodes, map[*ast.Symbol]string{}, true, 0)
	if err != nil {
		return nil, err
	}
	p.Body = body
	return p, nil
}

type extractor struct {
	s            *Session
	c            *checker.Checker
	file         string
	functions    map[*ast.Symbol]frontend.Function
	declarations map[string]*ast.Node
	serial       int
	nodes        int
}

func (e *extractor) loc(n *ast.Node) frontend.Span {
	start := scanner.GetTokenPosOfNode(n, e.s.file, false)
	p := position(e.s.text, start)
	return frontend.Span{File: e.file, StartByte: start, EndByte: n.End(), Line: p.Line, Column: p.UTF16Column}
}
func (e *extractor) fail(n *ast.Node, message string) error {
	return frontend.Error{Code: "TY1201", Message: message, Loc: e.loc(n)}
}
func (e *extractor) count(n *ast.Node, depth int) error {
	e.nodes++
	if e.nodes > nir.MaxNodes || depth > nir.MaxDepth {
		return e.fail(n, "scalar AST budget exceeded")
	}
	return nil
}
func numeric(t *checker.Type) bool {
	return t != nil && (t.Flags() == checker.TypeFlagsNumber || t.Flags() == checker.TypeFlagsNumberLiteral)
}
func (e *extractor) scalar(n *ast.Node) (string, error) {
	t := scalarKind(e.c.GetTypeAtLocation(n))
	if t == "" {
		return "", e.fail(n, "scalar/2 requires a checked number or boolean (not any, heterogeneous union, enum or object)")
	}
	return t, nil
}
func hasModifiers(n *ast.Node) bool { return n.Modifiers() != nil && len(n.Modifiers().Nodes) != 0 }
func (e *extractor) binding(n *ast.Node) (frontend.Binding, error) {
	if n == nil || n.Kind != ast.KindIdentifier {
		return frontend.Binding{}, fmt.Errorf("TY1201: only identifier bindings are supported")
	}
	sym := e.c.GetSymbolAtLocation(n)
	if sym == nil || len(sym.Declarations) != 1 {
		return frontend.Binding{}, e.fail(n, "ambiguous or merged declaration")
	}
	typ, err := e.scalar(n)
	if err != nil {
		return frontend.Binding{}, err
	}
	e.serial++
	return frontend.Binding{ID: fmt.Sprintf("v%d", e.serial), Name: n.Text(), Type: typ, Loc: e.loc(n)}, nil
}
func (e *extractor) signature(n *ast.Node) (frontend.Function, error) {
	f := n.AsFunctionDeclaration()
	if hasModifiers(n) || f.AsteriskToken != nil || f.TypeParameters != nil || f.FullSignature != nil || f.Body == nil || f.Body.Kind != ast.KindBlock || n.Name() == nil || n.Name().Kind != ast.KindIdentifier || f.Type == nil || !scalarAnnotation(f.Type) {
		return frontend.Function{}, e.fail(n, "only plain functions with an explicit number/boolean result are supported; no async, generator, generic, export or overload")
	}
	sym := e.c.GetSymbolAtLocation(n.Name())
	if sym == nil || len(sym.Declarations) != 1 || e.functions[sym].ID != "" {
		return frontend.Function{}, e.fail(n, "duplicate or ambiguous function")
	}
	sig := e.c.GetSignatureFromDeclaration(n)
	if sig == nil || scalarKind(e.c.GetReturnTypeOfSignature(sig)) == "" {
		return frontend.Function{}, e.fail(n, "function result signature is not scalar")
	}
	out := frontend.Function{ID: fmt.Sprintf("f%d", len(e.functions)), Name: n.Name().Text(), Result: scalarKind(e.c.GetReturnTypeOfSignature(sig)), Loc: e.loc(n)}
	if len(f.Parameters.Nodes) > 32 || len(e.functions) >= nir.MaxFunctions-1 {
		return out, e.fail(n, "function/parameter budget exceeded")
	}
	for _, pn := range f.Parameters.Nodes {
		p := pn.AsParameterDeclaration()
		if hasModifiers(pn) || p.DotDotDotToken != nil || p.QuestionToken != nil || p.Initializer != nil || p.Type == nil || !scalarAnnotation(p.Type) || pn.Name().Kind != ast.KindIdentifier || pn.Name().Text() == "this" {
			return out, e.fail(pn, "parameters must be required number/boolean identifiers without defaults or rest")
		}
		b, err := e.binding(pn.Name())
		if err != nil {
			return out, err
		}
		out.Params = append(out.Params, b)
	}
	e.functions[sym], e.declarations[out.ID] = out, n
	return out, nil
}
func (e *extractor) statements(nodes []*ast.Node, env map[*ast.Symbol]string, top bool, depth int) ([]frontend.Statement, error) {
	var out []frontend.Statement
	for _, n := range nodes {
		if err := e.count(n, depth); err != nil {
			return nil, err
		}
		s := frontend.Statement{Loc: e.loc(n)}
		switch n.Kind {
		case ast.KindFunctionDeclaration:
			if top && depth == 0 {
				continue
			}
			return nil, e.fail(n, "nested functions and closures are not supported")
		case ast.KindEmptyStatement:
			continue
		case ast.KindBlock:
			body, err := e.branch(n, env, top, depth+1)
			if err != nil {
				return nil, err
			}
			s.Op, s.Body = "block", body
		case ast.KindIfStatement:
			branch := n.AsIfStatement()
			condition, err := e.expr(branch.Expression, env, depth+1)
			if err != nil {
				return nil, err
			}
			if condition.Type != "boolean" {
				return nil, e.fail(branch.Expression, "if condition must be boolean; implicit truthiness is not supported")
			}
			yes, err := e.branch(branch.ThenStatement, env, top, depth+1)
			if err != nil {
				return nil, err
			}
			var no []frontend.Statement
			if branch.ElseStatement != nil {
				no, err = e.branch(branch.ElseStatement, env, top, depth+1)
				if err != nil {
					return nil, err
				}
			}
			s.Op, s.Condition, s.Then, s.Else = "if", condition, yes, no
		case ast.KindVariableStatement:
			v := n.AsVariableStatement()
			list := v.DeclarationList.AsVariableDeclarationList()
			if hasModifiers(n) || v.DeclarationList.Flags&ast.NodeFlagsBlockScoped != ast.NodeFlagsConst {
				return nil, e.fail(n, "only const declarations are supported (not let/var/using)")
			}
			for _, dn := range list.Declarations.Nodes {
				d := dn.AsVariableDeclaration()
				if dn.Name().Kind != ast.KindIdentifier || d.Initializer == nil || d.ExclamationToken != nil || (d.Type != nil && !scalarAnnotation(d.Type)) {
					return nil, e.fail(dn, "const requires an identifier, scalar initializer and optional number/boolean annotation")
				}
				x, err := e.expr(d.Initializer, env, depth+1)
				if err != nil {
					return nil, err
				}
				b, err := e.binding(dn.Name())
				if err != nil {
					return nil, err
				}
				sym := e.c.GetSymbolAtLocation(dn.Name())
				if env[sym] != "" {
					return nil, e.fail(dn, "duplicate local")
				}
				env[sym] = b.ID
				out = append(out, frontend.Statement{Op: "const", Binding: &b, Value: x, Loc: e.loc(dn)})
			}
			continue
		case ast.KindReturnStatement:
			if top || n.AsReturnStatement().Expression == nil {
				return nil, e.fail(n, "a scalar function must return a value")
			}
			x, err := e.expr(n.AsReturnStatement().Expression, env, depth+1)
			if err != nil {
				return nil, err
			}
			s.Op, s.Value = "return", x
		case ast.KindExpressionStatement:
			x := n.AsExpressionStatement().Expression
			if e.isPrint(x) {
				call := x.AsCallExpression()
				if call.QuestionDotToken != nil || call.TypeArguments != nil || len(call.Arguments.Nodes) != 1 {
					return nil, e.fail(x, "native console.log requires exactly one number or boolean argument")
				}
				arg, err := e.expr(call.Arguments.Nodes[0], env, depth+1)
				if err != nil {
					return nil, err
				}
				s.Op, s.Value = "print", arg
			} else {
				arg, err := e.expr(x, env, depth+1)
				if err != nil {
					return nil, err
				}
				s.Op, s.Value = "discard", arg
			}
		default:
			return nil, e.fail(n, "unsupported scalar statement: "+n.Kind.String())
		}
		out = append(out, s)
	}
	return out, nil
}
func (e *extractor) isPrint(n *ast.Node) bool {
	if n.Kind != ast.KindCallExpression {
		return false
	}
	fn := n.AsCallExpression().Expression
	if fn.Kind != ast.KindPropertyAccessExpression {
		return false
	}
	p := fn.AsPropertyAccessExpression()
	if p.QuestionDotToken != nil || p.Expression.Kind != ast.KindIdentifier || p.Expression.Text() != "console" || fn.Name().Text() != "log" {
		return false
	}
	sym := e.c.GetSymbolAtLocation(p.Expression)
	if sym == nil || len(sym.Declarations) != 1 {
		return false
	}
	origin := ast.GetSourceFileOfNode(sym.Declarations[0])
	sig := e.c.GetResolvedSignature(n)
	if origin == nil || origin.FileName() != printDeclarationFile || sig == nil || sig.Declaration() == nil {
		return false
	}
	decl := ast.GetSourceFileOfNode(sig.Declaration())
	return decl != nil && decl.FileName() == printDeclarationFile
}
func (e *extractor) expr(n *ast.Node, env map[*ast.Symbol]string, depth int) (*frontend.Expression, error) {
	if err := e.count(n, depth); err != nil {
		return nil, err
	}
	typ, err := e.scalar(n)
	if err != nil {
		return nil, err
	}
	x := &frontend.Expression{Type: typ, Loc: e.loc(n)}
	var args []*ast.Node
	switch n.Kind {
	case ast.KindTrueKeyword, ast.KindFalseKeyword:
		value := n.Kind == ast.KindTrueKeyword
		x.Op, x.Bool = "bool", &value
	case ast.KindNumericLiteral:
		t := e.c.GetTypeAtLocation(n)
		if t.Flags() != checker.TypeFlagsNumberLiteral {
			return nil, e.fail(n, "numeric literal has unexpected checker representation")
		}
		value, ok := t.AsLiteralType().Value().(jsnum.Number)
		if !ok {
			return nil, e.fail(n, "numeric literal is not a binary64 value")
		}
		x.Op, x.Bits = "number", nir.EncodeNumber(float64(value))
	case ast.KindIdentifier:
		id := env[e.c.GetSymbolAtLocation(n)]
		if id == "" {
			return nil, e.fail(n, "unknown/captured/forward value: "+n.Text()+"; only in-scope scalar bindings are supported")
		}
		x.Op, x.Ref = "ref", id
	case ast.KindBinaryExpression:
		b := n.AsBinaryExpression()
		x.Op = map[ast.Kind]string{
			ast.KindPlusToken: "add", ast.KindMinusToken: "sub", ast.KindAsteriskToken: "mul", ast.KindSlashToken: "div",
			ast.KindLessThanToken: "lt", ast.KindLessThanEqualsToken: "le", ast.KindGreaterThanToken: "gt", ast.KindGreaterThanEqualsToken: "ge",
			ast.KindEqualsEqualsEqualsToken: "eq", ast.KindExclamationEqualsEqualsToken: "ne",
		}[b.OperatorToken.Kind]
		if x.Op == "" || b.Type != nil || hasModifiers(n) {
			return nil, e.fail(n, "unsupported operator: scalar/2 accepts arithmetic, ordered number comparisons and same-kind strict equality; no coercive equality, logical operators or assignment")
		}
		args = []*ast.Node{b.Left, b.Right}
	case ast.KindPrefixUnaryExpression:
		p := n.AsPrefixUnaryExpression()
		x.Op = map[ast.Kind]string{ast.KindPlusToken: "pos", ast.KindMinusToken: "neg", ast.KindExclamationToken: "not"}[p.Operator]
		if x.Op == "" {
			return nil, e.fail(n, "only numeric unary + / - and boolean ! are supported")
		}
		args = []*ast.Node{p.Operand}
	case ast.KindParenthesizedExpression:
		return e.expr(n.AsParenthesizedExpression().Expression, env, depth+1)
	case ast.KindCallExpression:
		call := n.AsCallExpression()
		if call.Expression.Kind != ast.KindIdentifier || call.QuestionDotToken != nil || call.TypeArguments != nil {
			return nil, e.fail(n, "only direct, non-generic function calls are supported")
		}
		f, ok := e.functions[e.c.GetSymbolAtLocation(call.Expression)]
		sig := e.c.GetResolvedSignature(n)
		if !ok || sig == nil || sig.Declaration() != e.declarations[f.ID] || len(sig.Parameters()) != len(f.Params) || len(call.Arguments.Nodes) != len(f.Params) {
			return nil, e.fail(n, "call target or signature is outside the scalar function table")
		}
		x.Op, x.Ref = "call", f.ID
		args = call.Arguments.Nodes
	default:
		return nil, e.fail(n, "unsupported scalar expression: "+n.Kind.String())
	}
	for _, a := range args {
		value, err := e.expr(a, env, depth+1)
		if err != nil {
			return nil, err
		}
		x.Args = append(x.Args, value)
	}
	return x, nil
}
