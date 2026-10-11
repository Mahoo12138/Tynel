// tynel-gc-probe emits the explicitly experimental M1 fixture as NIR JSON.
package main

import (
	"fmt"
	"os"
	"tynel.local/tynel/compiler/nir"
	"tynel.local/tynel/internal/probe"
)

func main() {
	if len(os.Args) != 1 {
		fmt.Fprintln(os.Stderr, "usage: tynel-gc-probe (writes NIR to stdout)")
		os.Exit(2)
	}
	if err := nir.Write(os.Stdout, probe.GCModule()); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
