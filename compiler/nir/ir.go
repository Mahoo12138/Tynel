// Package nir defines Tynel's versioned, backend-independent research IR.
package nir

// Version 2 adds explicitly gated GC-probe references and local bindings.
const Version = 2
const ProbeGCFeature = "probe.gc/1"
const Semantics = "tynel.shared-core/0.1"

// Type is a semantic type, not a Go storage layout.
type Type string

const (
	Number Type = "number"
	Void   Type = "void"
	// ProbeRef is a nullable pointer to the fixed experimental node, not a TS object.
	ProbeRef Type = "probe_ref"
)

type Span struct {
	File   string `json:"file"`
	Line   int    `json:"line"`
	Column int    `json:"column"`
}

type Module struct {
	IRVersion int        `json:"irVersion"`
	Semantics string     `json:"semantics"`
	Package   string     `json:"package"`
	Features  []string   `json:"features,omitempty"`
	Functions []Function `json:"functions"`
}

type Function struct {
	Name   string  `json:"name"`
	Params []Param `json:"params"`
	Result Type    `json:"result"`
	Body   []Stmt  `json:"body"`
	Loc    Span    `json:"loc"`
}

type Param struct {
	Name string `json:"name"`
	Type Type   `json:"type"`
}

// Expr uses IDs/names and data only: checker pointers must not cross this seam.
// Numbers use exactly 16 lowercase hexadecimal digits of IEEE-754 binary64 bits.
type Expr struct {
	Op   string  `json:"op"`
	Type Type    `json:"type"`
	Bits string  `json:"bits,omitempty"`
	Name string  `json:"name,omitempty"`
	Args []*Expr `json:"args,omitempty"`
	Loc  Span    `json:"loc"`
}

type Stmt struct {
	Name   string `json:"name,omitempty"`
	Target *Expr  `json:"target,omitempty"`
	Op     string `json:"op"`
	Value  *Expr  `json:"value,omitempty"`
	Loc    Span   `json:"loc"`
}
