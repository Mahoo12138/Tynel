// Same named type, wrong field order: the bridge must reject this layout.
package probesupport

type Node struct {
	Next  *Node
	Value float64
}

//go:noinline
func ABI_v1()    {}
func GrowStack() {}
