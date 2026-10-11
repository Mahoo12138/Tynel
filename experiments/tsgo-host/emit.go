package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"

	"github.com/microsoft/typescript-go/tynelapi"
	"tynel.local/tynel/compiler/frontend"
	"tynel.local/tynel/compiler/nir"
)

func emitScalar(source, name, mode string, out, errs io.Writer) int {
	s, err := tynelapi.NewScalar(source)
	if err != nil {
		fmt.Fprintln(errs, err)
		return 2
	}
	defer s.Close()
	snapshot, err := s.Snapshot()
	if err != nil {
		fmt.Fprintln(errs, err)
		return 2
	}
	if len(snapshot.Diagnostics) != 0 {
		for _, d := range snapshot.Diagnostics {
			fmt.Fprintf(errs, "%s:%d:%d: TS%d: %s\n", name, d.Start.Line, d.Start.UTF16Column, d.Code, d.Message)
		}
		return 1
	}
	facts, err := s.ScalarFacts(filepath.ToSlash(name))
	// Only owned values survive this point. No checker/AST is available to the lowerer.
	s.Close()
	if err != nil {
		fmt.Fprintln(errs, err)
		return 1
	}
	m, err := frontend.Lower(facts)
	if err != nil {
		fmt.Fprintln(errs, err)
		return 1
	}
	var b bytes.Buffer
	if mode == "facts" {
		enc := json.NewEncoder(&b)
		enc.SetIndent("", "  ")
		err = enc.Encode(facts)
	} else {
		err = nir.Write(&b, m)
	}
	if err != nil {
		fmt.Fprintln(errs, err)
		return 2
	}
	if _, err = io.Copy(out, &b); err != nil {
		fmt.Fprintln(errs, err)
		return 2
	}
	return 0
}
