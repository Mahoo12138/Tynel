// Package probesupport is a handwritten test host, not a TypeScript runtime.
package probesupport

import "runtime"

// Node fixes the layout of the explicitly gated M1 GC probe. Next is a real
// typed pointer visible to Go's GC; no pointer is encoded in an integer slot.
type Node struct {
	Value float64
	Next  *Node
}

// ABI_v1 is kept reachable by the generated program's noinline host wrapper.
//
//go:noinline
func ABI_v1() {}

func GrowStack() { grow(128) }

//go:noinline
func grow(depth int) {
	var frame [1024]byte
	for i := range frame {
		frame[i] = byte(i + depth)
	}
	if depth == 0 {
		runtime.GC()
	} else {
		grow(depth - 1)
	}
	checkFrame(&frame, depth)
}

//go:noinline
func checkFrame(frame *[1024]byte, depth int) {
	for i, value := range frame {
		if value != byte(i+depth) {
			panic("GC probe stack frame corrupted")
		}
	}
}
