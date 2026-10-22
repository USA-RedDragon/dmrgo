//go:build !tinygo

package dmrgo_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestDocListsPackagesAndErrors(t *testing.T) {
	t.Parallel()

	raw, err := os.ReadFile("doc.go")
	if err != nil {
		t.Fatal(err)
	}
	doc := string(raw)
	err = filepath.WalkDir(".", func(path string, d fs.DirEntry, err error) error {
		if err != nil || !d.IsDir() {
			return err
		}
		switch d.Name() {
		case "internal", "examples", "testdata", "capi":
			return filepath.SkipDir
		}
		checkPackage(t, doc, path)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestDocErrorsExist(t *testing.T) {
	t.Parallel()

	raw, err := os.ReadFile("doc.go")
	if err != nil {
		t.Fatal(err)
	}
	errs := map[string]bool{}
	err = filepath.WalkDir(".", func(path string, d fs.DirEntry, err error) error {
		if err != nil || !d.IsDir() {
			return err
		}
		switch d.Name() {
		case "internal", "examples", "testdata", "capi":
			return filepath.SkipDir
		}
		pkgs, err := parser.ParseDir(token.NewFileSet(), path, func(fi fs.FileInfo) bool {
			return !strings.HasSuffix(fi.Name(), "_test.go")
		}, parser.SkipObjectResolution)
		if err != nil {
			return err
		}
		for name, pkg := range pkgs {
			for _, f := range pkg.Files {
				for _, decl := range f.Decls {
					gd, ok := decl.(*ast.GenDecl)
					if !ok || gd.Tok != token.VAR {
						continue
					}
					for _, spec := range gd.Specs {
						vs, ok := spec.(*ast.ValueSpec)
						if !ok {
							continue
						}
						for _, id := range vs.Names {
							errs[name+"."+id.Name] = true
						}
					}
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range regexp.MustCompile(`\b([a-z][a-z0-9]*)\.(Err[A-Za-z0-9]+)\b`).FindAllStringSubmatch(string(raw), -1) {
		if !errs[m[1]+"."+m[2]] {
			t.Errorf("doc.go names %s.%s, which the package does not export", m[1], m[2])
		}
	}
	for _, m := range regexp.MustCompile(`(?m)^//   - (Err[A-Za-z0-9]+):`).FindAllStringSubmatch(string(raw), -1) {
		if !errs["dmrgo."+m[1]] {
			t.Errorf("doc.go names %s, which package dmrgo does not export", m[1])
		}
	}
}

func checkPackage(t *testing.T, doc, dir string) {
	t.Helper()
	pkgs, err := parser.ParseDir(token.NewFileSet(), dir, func(fi fs.FileInfo) bool {
		return !strings.HasSuffix(fi.Name(), "_test.go")
	}, parser.SkipObjectResolution)
	if err != nil {
		t.Fatal(err)
	}
	for name, pkg := range pkgs {
		if dir != "." && !strings.Contains(doc, "//   - "+dir+":") && !strings.Contains(doc, " "+dir+",") && !strings.Contains(doc, " "+dir+":") {
			t.Errorf("doc.go # Packages does not list %s", dir)
		}
		prefix := name + "."
		if dir == "." {
			prefix = ""
		}
		for _, f := range pkg.Files {
			for _, decl := range f.Decls {
				gd, ok := decl.(*ast.GenDecl)
				if !ok || gd.Tok != token.VAR {
					continue
				}
				for _, spec := range gd.Specs {
					vs, ok := spec.(*ast.ValueSpec)
					if !ok {
						continue
					}
					for _, id := range vs.Names {
						if id.IsExported() && strings.HasPrefix(id.Name, "Err") && !strings.Contains(doc, prefix+id.Name) {
							t.Errorf("doc.go # Errors does not list %s%s", prefix, id.Name)
						}
					}
				}
			}
		}
	}
}
