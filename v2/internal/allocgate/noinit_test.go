//go:build !tinygo

package allocgate_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
	"testing"
)

func TestNoInit(t *testing.T) {
	t.Parallel()

	for _, pkg := range libraryPackages(t) {
		dir := filepath.Join(moduleRoot, pkg)
		matches, err := filepath.Glob(filepath.Join(dir, "*.go"))
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range matches {
			if strings.HasSuffix(m, "_test.go") {
				continue
			}
			fset := token.NewFileSet()
			f, err := parser.ParseFile(fset, m, nil, parser.SkipObjectResolution)
			if err != nil {
				t.Fatal(err)
			}
			for _, d := range f.Decls {
				if fd, ok := d.(*ast.FuncDecl); ok && fd.Recv == nil && fd.Name.Name == "init" {
					t.Errorf("%s: init function in library code", fset.Position(fd.Pos()))
				}
			}
		}
	}
}
