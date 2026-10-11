// Handwritten package/bootstrap scaffold for the M1 compiler experiment.
// It contains NO input-specific business code. The fork fills main and creates
// every business function directly as Go high-level IR after noding.
package main

import (
	"fmt"
	"math"
)

func main() {}

// Preserve complete binary64 values without Go's exact constant arithmetic.
func tynelF64(bits uint64) float64 { return math.Float64frombits(bits) }

// This probe printer is not a claim of JavaScript console.log compatibility.
func tynelPrint(value float64) { fmt.Println(value) }
