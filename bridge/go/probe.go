package tynelbridge

import (
	"cmd/compile/internal/ir"
	"cmd/compile/internal/tynelbridge/nir"
	"cmd/compile/internal/types"
	"cmd/internal/src"
	"fmt"
)

func (b *builder) storage(t nir.Type) *types.Type {
	switch t {
	case nir.Number:
		return types.Types[types.TFLOAT64]
	case nir.ProbeRef:
		return b.probeRef
	default:
		panic("validated NIR type missing storage lowering")
	}
}

func (b *builder) host(pos src.XPos, name string, args ...ir.Node) *ir.CallExpr {
	return ir.NewCallExpr(pos, ir.OCALL, b.functions[name].Nname, args)
}

// Validate both identity and shape before constructing target nodes. Reading
// a host type is an explicit temporary bootstrap seam, not structural TS typing.
func (b *builder) checkHost() error {
	witness := b.functions["tynelProbeType"]
	if witness == nil || witness.Type().NumParams() != 0 || witness.Type().NumResults() != 1 {
		return fmt.Errorf("TY4002: missing or invalid GC probe type witness")
	}
	ref := witness.Type().Results()[0].Type
	if !ref.IsPtr() || !ref.Elem().IsStruct() || ref.Elem().NumFields() != 2 {
		return fmt.Errorf("TY4002: invalid GC probe record layout")
	}
	record := ref.Elem()
	sym := record.Sym()
	if sym == nil || sym.Name != "Node" || sym.Pkg.Path != "tynel.local/bootstrap/probesupport" {
		return fmt.Errorf("TY4002: unexpected GC probe type origin")
	}
	value, next := record.Field(0), record.Field(1)
	if value.Sym == nil || next.Sym == nil || value.Sym.Name != "Value" || next.Sym.Name != "Next" || value.Type.Kind() != types.TFLOAT64 || !types.Identical(next.Type, ref) {
		return fmt.Errorf("TY4002: GC probe fields must be Value float64, Next *Node")
	}
	b.probeRef = ref
	f64 := types.Types[types.TFLOAT64]
	for _, contract := range []struct {
		name            string
		params, results []*types.Type
	}{
		{"main", nil, nil},
		{"tynelF64", []*types.Type{types.Types[types.TUINT64]}, []*types.Type{f64}},
		{"tynelPrint", []*types.Type{f64}, nil},
		{"tynelProbeABI_v1", nil, nil},
		{"tynelProbeGC", nil, nil},
		{"tynelProbeStack", nil, nil},
		{"tynelProbeAssertSame", []*types.Type{ref, ref}, nil},
	} {
		fn := b.functions[contract.name]
		if fn == nil || fn.Type().NumParams() != len(contract.params) || fn.Type().NumResults() != len(contract.results) {
			return fmt.Errorf("TY4002: missing or incompatible host function %s", contract.name)
		}
		for i, t := range contract.params {
			if !types.Identical(fn.Type().Params()[i].Type, t) {
				return fmt.Errorf("TY4002: parameter mismatch in %s", contract.name)
			}
		}
		for i, t := range contract.results {
			if !types.Identical(fn.Type().Results()[i].Type, t) {
				return fmt.Errorf("TY4002: result mismatch in %s", contract.name)
			}
		}
	}
	return nil
}
