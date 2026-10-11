package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestCLI(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
		code int
		text string
	}{
		{"version", []string{"version"}, 0, "frontend not integrated"},
		{"no arguments", nil, 2, "usage:"},
		{"missing file", []string{"check-ir", "missing.json"}, 1, "missing.json"},
		{"not fake TypeScript", []string{"build", "main.ts"}, 2, "TY1001"},
		{"valid example", []string{"check-ir", "../../examples/add.nir.json"}, 0, "2 functions"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var out, errs bytes.Buffer
			code := run(tc.args, &out, &errs)
			if code != tc.code || !strings.Contains(out.String()+errs.String(), tc.text) {
				t.Fatalf("code=%d stdout=%q stderr=%q", code, out.String(), errs.String())
			}
		})
	}
}
