// Package probe constructs test IR. It is not a parser or a Go-source emitter.
package probe

import "tynel.local/tynel/compiler/nir"

// GCModule exercises an escaping allocation, aliases, a heap pointer store,
// and caller roots across GC/stack growth. The child has no lasting local root.
func GCModule() *nir.Module {
	loc := nir.Span{File: "experiments/gc-probe.nir", Line: 1, Column: 1}
	expr := func(op string, typ nir.Type, args ...*nir.Expr) *nir.Expr {
		return &nir.Expr{Op: op, Type: typ, Args: args, Loc: loc}
	}
	num := func(v float64) *nir.Expr { e := expr("number", nir.Number); e.Bits = nir.EncodeNumber(v); return e }
	ref := func(op, name string, typ nir.Type) *nir.Expr { e := expr(op, typ); e.Name = name; return e }
	local := func(name string) *nir.Expr { return ref("local", name, nir.ProbeRef) }
	param := func(name string, typ nir.Type) *nir.Expr { return ref("param", name, typ) }
	call := func(name string, args ...*nir.Expr) *nir.Expr {
		e := expr("call", nir.ProbeRef, args...)
		e.Name = name
		return e
	}
	stmt := func(op string, value *nir.Expr) nir.Stmt { return nir.Stmt{Op: op, Value: value, Loc: loc} }
	let := func(name string, value *nir.Expr) nir.Stmt { s := stmt("let", value); s.Name = name; return s }
	pair := func(op string, target, value *nir.Expr) nir.Stmt { s := stmt(op, value); s.Target = target; return s }
	value := func(e *nir.Expr) *nir.Expr { return expr("probe_value", nir.Number, e) }
	return &nir.Module{IRVersion: nir.Version, Semantics: nir.Semantics, Package: "main", Features: []string{nir.ProbeGCFeature}, Functions: []nir.Function{
		{Name: "makeNode", Params: []nir.Param{{Name: "value", Type: nir.Number}}, Result: nir.ProbeRef, Loc: loc, Body: []nir.Stmt{
			let("node", expr("probe_new", nir.ProbeRef)),
			pair("probe_store_value", local("node"), param("value", nir.Number)),
			stmt("return", local("node")),
		}},
		{Name: "linkNode", Params: []nir.Param{{Name: "parent", Type: nir.ProbeRef}, {Name: "child", Type: nir.ProbeRef}}, Result: nir.ProbeRef, Loc: loc, Body: []nir.Stmt{
			pair("probe_store_next", param("parent", nir.ProbeRef), param("child", nir.ProbeRef)),
			stmt("return", param("parent", nir.ProbeRef)),
		}},
		{Name: "main", Result: nir.Void, Loc: loc, Body: []nir.Stmt{
			let("root", call("makeNode", num(40))),
			stmt("probe_gc", nil),
			let("alias", local("root")),
			pair("probe_store_value", local("alias"), num(42)),
			let("linked", call("linkNode", local("root"), call("makeNode", num(7)))),
			stmt("probe_gc", nil),
			stmt("probe_stack", nil),
			pair("probe_assert_same", local("root"), local("alias")),
			pair("probe_assert_same", local("root"), local("linked")),
			stmt("print_number", value(local("root"))),
			stmt("print_number", value(expr("probe_next", nir.ProbeRef, local("root")))),
			pair("probe_store_value", local("alias"), num(43)),
			stmt("probe_gc", nil),
			stmt("print_number", value(local("root"))),
		}},
	}}
}
