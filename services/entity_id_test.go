package services

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// cmd/generate emits fields typed services.<IDType> using the values in
// DomainIDTypes. If a value names a type that does not exist here, the generated
// code does not compile, and the failure lands in the user's build rather than
// this module's. This asserts every mapped type is really declared.
func TestDomainIDTypesNameRealTypes(t *testing.T) {
	declared := exportedTypeNames(t)

	for domain, idType := range DomainIDTypes {
		assert.Contains(t, declared, idType,
			"DomainIDTypes[%q] = %q, which is not a type declared in package services", domain, idType)
	}

	// The generator falls back to this for any unmapped domain.
	assert.Contains(t, declared, "EntityID")
}

// exportedTypeNames returns the exported type names declared in the package's
// non-test source.
func exportedTypeNames(t *testing.T) map[string]bool {
	t.Helper()

	fset := token.NewFileSet()
	names := map[string]bool{}

	entries, err := os.ReadDir(".")
	require.NoError(t, err)

	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}

		file, err := parser.ParseFile(fset, filepath.Join(".", name), nil, 0)
		require.NoError(t, err)

		for _, decl := range file.Decls {
			gen, ok := decl.(*ast.GenDecl)
			if !ok {
				continue
			}
			for _, spec := range gen.Specs {
				if ts, ok := spec.(*ast.TypeSpec); ok && ts.Name.IsExported() {
					names[ts.Name.Name] = true
				}
			}
		}
	}

	return names
}
