package frontend

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

func TestFrontendContractHasNoUpstreamHandles(t *testing.T) {
	seen := map[reflect.Type]bool{}
	var walk func(reflect.Type)
	walk = func(typ reflect.Type) {
		if seen[typ] {
			return
		}
		seen[typ] = true
		if strings.Contains(typ.PkgPath(), "microsoft") || strings.Contains(typ.PkgPath(), "cmd/compile") {
			t.Fatalf("upstream handle: %v", typ)
		}
		switch typ.Kind() {
		case reflect.Struct:
			for i := 0; i < typ.NumField(); i++ {
				if !typ.Field(i).IsExported() {
					t.Fatalf("hidden state: %v", typ)
				}
				walk(typ.Field(i).Type)
			}
		case reflect.Pointer, reflect.Slice:
			walk(typ.Elem())
		case reflect.String, reflect.Int, reflect.Bool:
		default:
			t.Fatalf("not a plain data field: %v", typ)
		}
	}
	walk(reflect.TypeFor[Program]())
}

func TestCoreImportsStayBackendIndependent(t *testing.T) {
	for _, dir := range []string{".", "../nir"} {
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatal(err)
		}
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") {
				continue
			}
			file, err := parser.ParseFile(token.NewFileSet(), filepath.Join(dir, entry.Name()), nil, parser.ImportsOnly)
			if err != nil {
				t.Fatal(err)
			}
			for _, imp := range file.Imports {
				path, err := strconv.Unquote(imp.Path.Value)
				if err != nil {
					t.Fatal(err)
				}
				if strings.Contains(path, "microsoft/") || strings.Contains(path, "cmd/compile") || strings.Contains(path, "tynelapi") {
					t.Fatalf("forbidden import %s in %s", path, entry.Name())
				}
			}
		}
	}
}
