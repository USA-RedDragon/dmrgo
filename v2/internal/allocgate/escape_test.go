//go:build !tinygo

package allocgate_test

import (
	"bufio"
	"bytes"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// moduleRoot is the v2 module directory, relative to this package.
const moduleRoot = "../.."

// skipDirs are the module directories that are not library code.
var skipDirs = []string{"cmd", "examples", "internal/allocgate", "internal/fecref", "internal/outputdigest", "internal/zeroalloc"}

// pendingDirs are library directories that the gate does not check yet,
// because their code still allocates. Empty it when they are clean.
var pendingDirs = []string{"layer2", "layer2/framer"}

// makeRE matches a make call that -gcflags=-m reports as not escaping. The
// runtime puts such a slice on the stack only when its size is a constant
// or at most 32 bytes, so a make with a size that is not a literal can still
// allocate.
var makeRE = regexp.MustCompile(`^(.+\.go):(\d+):(\d+): (make\(\[\][^,]+, (.*)\) does not escape)$`)

// literalArgsRE matches make arguments that are integer literals only.
var literalArgsRE = regexp.MustCompile(`^\d+(, \d+)?$`)

// siteRE matches a line of -gcflags=-m output that reports a heap
// allocation: a value that escapes, or a variable that moves, to the heap.
var siteRE = regexp.MustCompile(`^(.+\.go):(\d+):(\d+): (.*(?:escapes to heap|moved to heap).*)$`)

type site struct {
	file string
	line int
	msg  string
}

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

// escapeSites builds the library packages with -gcflags=-m and returns the
// reported heap allocations in files of the module.
func escapeSites(t *testing.T) []site {
	t.Helper()
	goTool, err := exec.LookPath("go")
	if err != nil {
		t.Skip("no go command")
	}
	root, err := filepath.Abs(moduleRoot)
	if err != nil {
		t.Fatal(err)
	}
	args := append([]string{"build", "-gcflags=-m=1"}, libraryPackages(t)...)
	cmd := exec.CommandContext(t.Context(), goTool, args...)
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "GOFLAGS=")
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	if err := cmd.Run(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out.String())
	}
	var sites []site
	seen := map[site]bool{}
	sc := bufio.NewScanner(&out)
	for sc.Scan() {
		m := siteRE.FindStringSubmatch(sc.Text())
		if m == nil || strings.Contains(m[4], "does not escape") {
			mm := makeRE.FindStringSubmatch(sc.Text())
			if mm == nil || literalArgsRE.MatchString(mm[5]) {
				continue
			}
			m = mm[:5]
			m[4] += " (variable size)"
		}
		file := m[1]
		if !filepath.IsAbs(file) {
			file = filepath.Join(root, file)
		}
		rel, err := filepath.Rel(root, file)
		if err != nil || strings.HasPrefix(rel, "..") {
			continue
		}
		line, _ := strconv.Atoi(m[2])
		s := site{filepath.ToSlash(rel), line, m[4]}
		if !seen[s] {
			seen[s] = true
			sites = append(sites, s)
		}
	}
	return sites
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

func TestNoHeapAllocations(t *testing.T) {
	t.Parallel()

	pkgs := map[string]*pkgFuncs{}
	var bad []string
	for _, s := range escapeSites(t) {
		dir := filepath.ToSlash(filepath.Dir(s.file))
		if slices.Contains(pendingDirs, dir) {
			continue
		}
		p, ok := pkgs[dir]
		if !ok {
			p = parsePackage(t, dir)
			pkgs[dir] = p
		}
		fd := p.enclosing(filepath.Base(s.file), s.line)
		if exemptReason(p, fd) != "" {
			continue
		}
		name := "package level"
		if fd != nil {
			name = funcKey(fd)
		}
		bad = append(bad, fmt.Sprintf("%s:%d: %s: %s", s.file, s.line, name, s.msg))
	}
	slices.Sort(bad)
	for _, b := range bad {
		t.Error(b)
	}
	if len(bad) > 0 {
		t.Logf("%d heap allocation sites outside the exemption rules", len(bad))
	}
}
