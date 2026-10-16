//go:build !tinygo

package allocgate_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
)

// moduleRoot is the v2 module directory, relative to this package.
const moduleRoot = "../.."

// skipDirs are the module directories that are not library code.
var skipDirs = []string{"cmd", "examples", "internal/allocgate", "internal/fecref", "internal/outputdigest"}

// libraryPackages returns the import paths of the library packages, relative
// to the module ("./bit" and so on).
func libraryPackages(t *testing.T) []string {
	t.Helper()
	var pkgs []string
	err := filepath.WalkDir(moduleRoot, func(path string, d os.DirEntry, err error) error {
		if err != nil || !d.IsDir() {
			return err
		}
		rel, _ := filepath.Rel(moduleRoot, path)
		rel = filepath.ToSlash(rel)
		if d.Name() == "testdata" || slices.ContainsFunc(skipDirs, func(s string) bool { return rel == s || strings.HasPrefix(rel, s+"/") }) {
			return filepath.SkipDir
		}
		matches, _ := filepath.Glob(filepath.Join(path, "*.go"))
		for _, m := range matches {
			if !strings.HasSuffix(m, "_test.go") {
				pkgs = append(pkgs, "./"+rel)
				break
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return pkgs
}

// pkgFuncs holds the declarations of one package directory.
type pkgFuncs struct {
	fset  *token.FileSet
	files map[string]*ast.File
	// names holds the function names, and the method names as
	// Recv.Method, of the package.
	names map[string]bool
}

func parsePackage(t *testing.T, dir string) *pkgFuncs {
	t.Helper()
	p := &pkgFuncs{fset: token.NewFileSet(), files: map[string]*ast.File{}, names: map[string]bool{}}
	matches, _ := filepath.Glob(filepath.Join(moduleRoot, dir, "*.go"))
	for _, m := range matches {
		if strings.HasSuffix(m, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(p.fset, m, nil, parser.ParseComments|parser.SkipObjectResolution)
		if err != nil {
			t.Fatal(err)
		}
		p.files[filepath.Base(m)] = f
		for _, d := range f.Decls {
			if fd, ok := d.(*ast.FuncDecl); ok {
				p.names[funcKey(fd)] = true
			}
		}
	}
	return p
}

func recvName(fd *ast.FuncDecl) string {
	if fd.Recv == nil || len(fd.Recv.List) == 0 {
		return ""
	}
	e := fd.Recv.List[0].Type
	if s, ok := e.(*ast.StarExpr); ok {
		e = s.X
	}
	if ix, ok := e.(*ast.IndexExpr); ok {
		e = ix.X
	}
	if id, ok := e.(*ast.Ident); ok {
		return id.Name
	}
	return ""
}

func funcKey(fd *ast.FuncDecl) string {
	if r := recvName(fd); r != "" {
		return r + "." + fd.Name.Name
	}
	return fd.Name.Name
}

// enclosing returns the function declaration that contains line of file, or
// nil for a package-level declaration.
func (p *pkgFuncs) enclosing(file string, line int) *ast.FuncDecl {
	f := p.files[file]
	if f == nil {
		return nil
	}
	for _, d := range f.Decls {
		fd, ok := d.(*ast.FuncDecl)
		if !ok {
			continue
		}
		if p.fset.Position(fd.Pos()).Line <= line && line <= p.fset.Position(fd.End()).Line {
			return fd
		}
	}
	return nil
}

// twinOK reports whether twin names a function or a Type.Method of the
// package, or a method of the receiver of fd, other than fd.
func twinOK(p *pkgFuncs, fd *ast.FuncDecl, twin string) bool {
	if twin == fd.Name.Name || twin == funcKey(fd) {
		return false
	}
	if r := recvName(fd); r != "" && p.names[r+"."+twin] {
		return true
	}
	return p.names[twin]
}

// twinRE matches the doc sentence of an allocating convenience wrapper, such
// as "It allocates the slice; AppendPack does not.", and captures the twin.
var twinRE = regexp.MustCompile(`allocates[^.;]*; ((?:\w+\.)?\w+) does not`)

// exemptRule is one rule that lets a function allocate. match reports
// whether the rule covers the function declaration fd of package p; fd is
// nil for a package-level declaration.
type exemptRule struct {
	name  string
	match func(p *pkgFuncs, fd *ast.FuncDecl) bool
}

// exemptRules lists every rule that lets a function allocate, one rule per
// entry, so that removing a rule is removing its entry.
var exemptRules = []exemptRule{
	// A constructor, a function or method named New* or new*, allocates the
	// value it returns.
	{"constructor", func(_ *pkgFuncs, fd *ast.FuncDecl) bool {
		return fd != nil && (strings.HasPrefix(fd.Name.Name, "New") || strings.HasPrefix(fd.Name.Name, "new"))
	}},
	// A convenience wrapper whose doc comment says "allocates ...; Twin does
	// not.", where Twin is a function or a Type.Method of the package, or a
	// method of the same receiver, such as AppendPack for Pack or
	// DecodeCSBKInto for DecodeCSBK.
	{"allocating wrapper", func(p *pkgFuncs, fd *ast.FuncDecl) bool {
		if fd == nil || fd.Doc == nil {
			return false
		}
		doc := strings.Join(strings.Fields(fd.Doc.Text()), " ")
		for _, m := range twinRE.FindAllStringSubmatch(doc, -1) {
			if twinOK(p, fd, m[1]) {
				return true
			}
		}
		return false
	}},
	// A String, GoString or Error method builds text for fmt or an error,
	// which allocates by design.
	{"fmt or error text", func(_ *pkgFuncs, fd *ast.FuncDecl) bool {
		return fd != nil && fd.Recv != nil && (fd.Name.Name == "String" || fd.Name.Name == "GoString" || fd.Name.Name == "Error")
	}},
	// An Append* or append* function grows the slice it appends to, which
	// allocates only when the caller's slice lacks the capacity.
	{"append", func(_ *pkgFuncs, fd *ast.FuncDecl) bool {
		return fd != nil && (strings.HasPrefix(fd.Name.Name, "Append") || strings.HasPrefix(fd.Name.Name, "append"))
	}},
	// A package-level declaration, such as a lookup table or a sentinel
	// error, is built once, when the program starts.
	{"package level", func(_ *pkgFuncs, fd *ast.FuncDecl) bool {
		return fd == nil
	}},
}

// exemptReason returns the name of the first rule of exemptRules that lets
// fd allocate, or "" when none does.
func exemptReason(p *pkgFuncs, fd *ast.FuncDecl) string {
	for _, r := range exemptRules {
		if r.match(p, fd) {
			return r.name
		}
	}
	return ""
}
