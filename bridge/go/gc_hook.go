// This file is overlaid into cmd/compile/internal/gc of the pinned Go tree.
package gc

import (
	"cmd/compile/internal/base"
	"cmd/compile/internal/tynelbridge"
	"flag"
)

var tynelIRPath = flag.String("tynel-ir", "", "load a validated Tynel IR module (research bridge)")

func loadTynelIR() {
	if *tynelIRPath == "" {
		return
	}
	if err := tynelbridge.Load(*tynelIRPath); err != nil {
		base.Errorf("Tynel bridge: %v", err)
		base.ErrorExit()
	}
}
