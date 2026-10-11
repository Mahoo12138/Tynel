// Package nir defines Tynel's versioned, backend-independent research IR.
package nir

const Version = 1
const Semantics = "tynel.shared-core/0.1"

// Type is a semantic type, not a Go storage layout.
type Type string

const (
	Number Type = "number"
	Void   Type = "void"
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
	Op    string `json:"op"`
	Value *Expr  `json:"value,omitempty"`
	Loc   Span   `json:"loc"`
}
