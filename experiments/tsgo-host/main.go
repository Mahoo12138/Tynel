// External consumer: imports only the public facade, never tsgo internals.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/microsoft/typescript-go/tynelapi"
)

func main() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr)) }

func run(args []string, out, errs io.Writer) int {
	fs := flag.NewFlagSet("tynel-tsgo-probe", flag.ContinueOnError)
	fs.SetOutput(errs)
	offset := fs.Int("offset", -1, "optional zero-based UTF-8 byte offset for one type query")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(errs, "usage: tynel-tsgo-probe [-offset byte] file.ts")
		return 2
	}
	f, err := os.Open(fs.Arg(0))
	if err != nil {
		fmt.Fprintln(errs, err)
		return 2
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, tynelapi.MaxSourceBytes+1))
	if err != nil {
		fmt.Fprintln(errs, err)
		return 2
	}
	s, err := tynelapi.New(string(data))
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
	result := struct {
		tynelapi.Snapshot
		Type *tynelapi.TypeFact `json:"type,omitempty"`
	}{Snapshot: snapshot}
	if *offset >= 0 && len(snapshot.Diagnostics) == 0 {
		fact, err := s.TypeAt(*offset)
		if err != nil {
			fmt.Fprintln(errs, err)
			return 2
		}
		result.Type = &fact
	}
	enc := json.NewEncoder(out)
	enc.SetIndent("", "  ")
	if err := enc.Encode(result); err != nil {
		fmt.Fprintln(errs, err)
		return 2
	}
	if len(snapshot.Diagnostics) != 0 {
		return 1
	}
	return 0
}
