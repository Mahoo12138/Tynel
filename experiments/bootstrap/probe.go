package main

import (
	"runtime"
	"tynel.local/bootstrap/probesupport"
)

// These helpers contain no input-specific algorithm. The bridge constructs
// allocation, field stores, aliases and reference-returning functions from NIR.
//
//go:noinline
func tynelProbeABI_v1() { probesupport.ABI_v1() }

// The bridge reads this signature, then validates the fixed Node field layout.
func tynelProbeType() *probesupport.Node { return nil }

// No object argument/global root: generated caller liveness must keep it alive.
//
//go:noinline
func tynelProbeGC() { runtime.GC() }

//go:noinline
func tynelProbeStack() { probesupport.GrowStack() }

//go:noinline
func tynelProbeAssertSame(a, b *probesupport.Node) {
	if a != b {
		panic("GC probe reference identity changed")
	}
}
