//go:build !tinygo

package zeroalloc_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// moduleRoot is the v2 module directory, relative to this package.
const moduleRoot = "../.."

// skipDirs are the module directories whose exported API is not library API.
// internal/fecref holds reference codecs that only tests call, and
// internal/errs the error constructors of the generated Encode methods.
var skipDirs = []string{"cmd", "examples", "internal/errs", "internal/fecref", "internal/zeroalloc"}

// exemptMethods are method names that no type needs an entry for, with the
// reason.
var exemptMethods = map[string]string{
	"String":     "fmt output, which allocates by design",
	"GoString":   "fmt output, which allocates by design",
	"Error":      "error text, which allocates by design",
	"Unwrap":     "returns a stored error",
	"EncodeBits": "returns a fresh slice by the layer2.Data contract; Encode returns the same bits as an array and has entries",
}

// exempt are the exported functions and methods that have no entry, each
// with the reason.
var exempt = map[string]string{
	"vocoder.New":                "setup: looks up a codec and builds its state once",
	"vocoder.Register":           "setup: called from the init function of a codec package",
	"vocoder/codec2.New":         "constructor: allocates the libcodec2 state once",
	"vocoder/codec2.Codec.Close": "Close",
}

// libraryAPI returns the exported functions, and the exported methods of
// exported types, of the library packages, in the form of entry.api, and
// the names among them that need no entry: methods in exemptMethods, and
// deprecated functions and methods, which forward to API with entries.
func libraryAPI(t *testing.T) (api []string, noEntry map[string]bool) {
	t.Helper()
	seen := map[string]bool{}
	noEntry = map[string]bool{}
	err := filepath.WalkDir(moduleRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil || !d.IsDir() {
			return err
		}
		rel, err := filepath.Rel(moduleRoot, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if d.Name() == "testdata" || slices.ContainsFunc(skipDirs, func(s string) bool { return rel == s || strings.HasPrefix(rel, s+"/") }) {
			return filepath.SkipDir
		}
		pkgs, err := parser.ParseDir(token.NewFileSet(), path, func(fi fs.FileInfo) bool {
			return !strings.HasSuffix(fi.Name(), "_test.go")
		}, parser.SkipObjectResolution|parser.ParseComments)
		if err != nil {
			return err
		}
		for _, pkg := range pkgs {
			for _, f := range pkg.Files {
				for _, decl := range f.Decls {
					fn, isFunc := decl.(*ast.FuncDecl)
					if !isFunc {
						continue
					}
					a, ok := apiName(rel, fn)
					if !ok {
						continue
					}
					seen[a] = true
					_, exemptMethod := exemptMethods[fn.Name.Name]
					if fn.Recv != nil && exemptMethod || fn.Doc != nil && strings.Contains("\n"+fn.Doc.Text(), "\nDeprecated: ") {
						noEntry[a] = true
					}
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	api = make([]string, 0, len(seen))
	for a := range seen {
		api = append(api, a)
	}
	slices.Sort(api)
	return api, noEntry
}

// apiName returns the entry.api of fn in package directory dir, and false
// when fn is not an exported function or an exported method of an exported
// type.
func apiName(dir string, fn *ast.FuncDecl) (string, bool) {
	if !fn.Name.IsExported() {
		return "", false
	}
	pkg := dir
	if pkg == "." {
		return "", false
	}
	if fn.Recv == nil {
		return pkg + "." + fn.Name.Name, true
	}
	typ := fn.Recv.List[0].Type
	if star, ok := typ.(*ast.StarExpr); ok {
		typ = star.X
	}
	switch g := typ.(type) {
	case *ast.IndexExpr:
		typ = g.X
	case *ast.IndexListExpr:
		typ = g.X
	}
	id, ok := typ.(*ast.Ident)
	if !ok || !id.IsExported() {
		return "", false
	}
	return pkg + "." + id.Name + "." + fn.Name.Name, true
}

// TestAllocsCoverAPI fails for an exported function or method without an
// entry and not in exempt, and for an exempt name that is not library API or
// that has an entry.
func TestAllocsCoverAPI(t *testing.T) {
	covered := map[string]bool{}
	for _, e := range entries(t) {
		covered[e.api] = true
	}
	api, noEntry := libraryAPI(t)
	for _, a := range api {
		_, isExempt := exempt[a]
		switch {
		case noEntry[a] && isExempt:
			t.Errorf("%s needs no entry: remove it from exempt in guard_alloc_test.go", a)
		case noEntry[a]:
		case covered[a] && isExempt:
			t.Errorf("%s has an entry and is in exempt: remove it from exempt in guard_alloc_test.go", a)
		case !covered[a] && !isExempt:
			t.Errorf("%s has no allocation entry: add an entry that calls it to the table of its package in v2/internal/zeroalloc, or add it to exempt in guard_alloc_test.go with a one-line reason", a)
		}
	}
	for a := range exempt {
		if !slices.Contains(api, a) {
			t.Errorf("exempt names %s, which is not library API: remove it from exempt in guard_alloc_test.go", a)
		}
	}
	for a := range covered {
		if !slices.Contains(api, a) {
			t.Errorf("an entry names %s, which is not library API: fix its api field", a)
		}
	}
}
