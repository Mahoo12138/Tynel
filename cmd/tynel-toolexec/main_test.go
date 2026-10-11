package main

import (
	"strings"
	"testing"
)

func TestVersionIdentity(t *testing.T) {
	id := "tynel-compile version go1.26.7\n"
	hash := strings.Repeat("a", 64)
	first, err := versionLine(id, []byte("one"), hash)
	if err != nil || !strings.HasPrefix(first, "compile version go1.26.7 tynel:") {
		t.Fatalf("%q: %v", first, err)
	}
	same, _ := versionLine(id, []byte("one"), hash)
	changedInput, _ := versionLine(id, []byte("two"), hash)
	changedCompiler, _ := versionLine(id, []byte("one"), strings.Repeat("b", 64))
	if first != same || first == changedInput || first == changedCompiler {
		t.Fatal("cache identity does not track all inputs")
	}
	if _, err := versionLine("broken", nil, hash); err == nil {
		t.Fatal("bad tool identity accepted")
	}
	if _, err := versionLine(id, nil, ""); err == nil {
		t.Fatal("missing compiler hash accepted")
	}
}
