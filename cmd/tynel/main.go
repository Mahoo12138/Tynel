package main

import (
	"fmt"
	"io"
	"os"

	"tynel.local/tynel/compiler/nir"
)

const version = "0.0.1-dev"

func main() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr)) }

func run(args []string, out, errs io.Writer) int {
	if len(args) == 1 && args[0] == "version" {
		fmt.Fprintf(out, "Tynel %s (IR v%d; TypeScript frontend not integrated)\n", version, nir.Version)
		return 0
	}
	if len(args) == 2 && (args[0] == "check-ir" || args[0] == "fmt-ir") {
		f, err := os.Open(args[1])
		if err != nil {
			fmt.Fprintln(errs, err)
			return 1
		}
		defer f.Close()
		m, err := nir.Read(f)
		if err != nil {
			fmt.Fprintln(errs, err)
			return 1
		}
		if args[0] == "fmt-ir" {
			if err := nir.Write(out, m); err != nil {
				fmt.Fprintln(errs, err)
				return 1
			}
		} else {
			fmt.Fprintf(out, "IR valid: %d functions, schema v%d\n", len(m.Functions), m.IRVersion)
		}
		return 0
	}
	if len(args) > 0 && (args[0] == "build" || args[0] == "run" || args[0] == "check") {
		fmt.Fprintln(errs, "TY1001: TypeScript frontend is not implemented yet; use check-ir and scripts/compile-ir.ps1 for the M1 research bridge.")
		return 2
	}
	fmt.Fprintln(errs, "usage: tynel version | check-ir <module.json> | fmt-ir <module.json>")
	return 2
}
