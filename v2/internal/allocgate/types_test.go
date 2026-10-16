//go:build !tinygo

package allocgate_test

import (
	"errors"
	"fmt"
	"go/ast"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// libPackage is a library package, type-checked from the files that the
// compiler compiled.
type libPackage struct {
	path  string
	files []*ast.File
	pkg   *types.Package
	info  *types.Info
}

// program holds the type information of the library and its dependencies.
type program struct {
	fset *token.FileSet
	lib  map[string]*libPackage
	// order lists the library packages in dependency order.
	order []*libPackage
	gc    types.Importer
}

type gateImporter struct{ p *program }

func (i gateImporter) Import(path string) (*types.Package, error) {
	if lp := i.p.lib[path]; lp != nil {
		if lp.pkg == nil {
			return nil, fmt.Errorf("import cycle at %s", path)
		}
		return lp.pkg, nil
	}
	return i.p.gc.Import(path)
}

// loadProgram type-checks the library packages of pkgs from source and
// imports the other packages from their export data.
func loadProgram(t *testing.T, pkgs []listedPackage, isLib func(string) bool) *program {
	t.Helper()
	p := &program{fset: token.NewFileSet(), lib: map[string]*libPackage{}}
	exports := map[string]string{}
	for _, lp := range pkgs {
		exports[lp.ImportPath] = lp.Export
	}
	p.gc = importer.ForCompiler(p.fset, "gc", func(path string) (io.ReadCloser, error) {
		f, ok := exports[path]
		if !ok || f == "" {
			return nil, fmt.Errorf("no export data for %s", path)
		}
		return os.Open(f)
	})
	for _, lp := range pkgs {
		if isLib(lp.ImportPath) {
			p.lib[lp.ImportPath] = &libPackage{path: lp.ImportPath}
		}
	}
	for _, lp := range pkgs {
		l := p.lib[lp.ImportPath]
		if l == nil {
			continue
		}
		for _, f := range lp.CompiledGoFiles {
			if !filepath.IsAbs(f) {
				f = filepath.Join(lp.Dir, f)
			}
			af, err := parser.ParseFile(p.fset, f, nil, parser.ParseComments|parser.SkipObjectResolution)
			if err != nil {
				t.Fatal(err)
			}
			l.files = append(l.files, af)
		}
		l.info = &types.Info{
			Types:      map[ast.Expr]types.TypeAndValue{},
			Defs:       map[*ast.Ident]types.Object{},
			Uses:       map[*ast.Ident]types.Object{},
			Selections: map[*ast.SelectorExpr]*types.Selection{},
		}
		conf := types.Config{Importer: gateImporter{p}, Sizes: types.SizesFor("gc", "amd64")}
		pkg, err := conf.Check(lp.ImportPath, p.fset, l.files, l.info)
		if err != nil {
			t.Fatalf("type-check %s: %v", lp.ImportPath, err)
		}
		l.pkg = pkg
		p.order = append(p.order, l)
	}
	return p
}

// lookupType returns the named type path.name, from the library or from
// export data.
func (p *program) lookupType(path, name string) types.Type {
	if path == "" {
		if obj := types.Universe.Lookup(name); obj != nil {
			return obj.Type()
		}
		return nil
	}
	var pkg *types.Package
	if l := p.lib[path]; l != nil {
		pkg = l.pkg
	} else {
		var err error
		if pkg, err = p.gc.Import(path); err != nil {
			return nil
		}
	}
	if obj, ok := pkg.Scope().Lookup(name).(*types.TypeName); ok {
		return obj.Type()
	}
	return nil
}

// symPackage returns the package path of a symbol name, such as
// "github.com/x/y" for "github.com/x/y.(*T).M", or "" when the name has no
// package.
func symPackage(sym string) string {
	end := len(sym)
	if i := strings.IndexAny(sym, "[("); i >= 0 {
		end = i
	}
	slash := strings.LastIndexByte(sym[:end], '/')
	dot := strings.IndexByte(sym[slash+1:end], '.')
	if dot < 0 {
		return ""
	}
	return sym[:slash+1+dot]
}

// methodSym splits a method symbol such as "pkg.(*T).M" or "pkg.T.M" into
// its package and method name. ok is false for a symbol that is not a
// method.
func methodSym(sym string) (pkg, method string, ok bool) {
	pkg = symPackage(sym)
	if pkg == "" {
		return "", "", false
	}
	rest := sym[len(pkg)+1:]
	if strings.HasPrefix(rest, "(") {
		i := strings.Index(rest, ").")
		if i < 0 {
			return "", "", false
		}
		rest = rest[i+2:]
	} else {
		depth := 0
		i := strings.IndexFunc(rest, func(r rune) bool {
			switch r {
			case '[':
				depth++
			case ']':
				depth--
			case '.':
				return depth == 0
			}
			return false
		})
		if i < 0 {
			return "", "", false
		}
		rest = rest[i+1:]
	}
	if !token.IsIdentifier(rest) {
		return "", "", false
	}
	return pkg, rest, true
}

// funcSym returns the symbol of the code of a declared function or method,
// or "" for a generic one, which has only instantiations.
func funcSym(fn *types.Func) string {
	sig := fn.Signature()
	if sig.TypeParams().Len() > 0 {
		return ""
	}
	recv := sig.Recv()
	if recv == nil {
		if fn.Pkg() == nil {
			return ""
		}
		return fn.Pkg().Path() + "." + fn.Name()
	}
	rt := recv.Type()
	ptr := false
	if pt, ok := rt.(*types.Pointer); ok {
		rt, ptr = pt.Elem(), true
	}
	named, ok := rt.(*types.Named)
	if !ok {
		return ""
	}
	if named.TypeParams().Len() > 0 || named.TypeArgs().Len() > 0 {
		return ""
	}
	if types.IsInterface(named) {
		return ""
	}
	obj := named.Obj()
	if obj.Pkg() == nil {
		return ""
	}
	if ptr {
		return obj.Pkg().Path() + ".(*" + obj.Name() + ")." + fn.Name()
	}
	return obj.Pkg().Path() + "." + obj.Name() + "." + fn.Name()
}

// symLess orders method names as the compiler orders the methods of an
// interface type: exported before unexported, then by name.
func symLess(a, b *types.Func) int {
	if a.Exported() != b.Exported() {
		if a.Exported() {
			return -1
		}
		return 1
	}
	if c := strings.Compare(a.Name(), b.Name()); c != 0 {
		return c
	}
	pa, pb := "", ""
	if a.Pkg() != nil {
		pa = a.Pkg().Path()
	}
	if b.Pkg() != nil {
		pb = b.Pkg().Path()
	}
	return strings.Compare(pa, pb)
}

// ifaceMethod is a method of an interface type that code calls.
type ifaceMethod struct {
	iface  *types.Interface
	method *types.Func
	// sealed is set when the interface has an unexported method of a
	// library package, so that only library types implement it.
	sealed bool
	// open is set when a type of the program that uses the library can
	// implement the interface.
	open bool
}

// errUnresolved marks an interface method relocation whose type the gate
// cannot find.
var errUnresolved = errors.New("unresolved interface type")

// resolveIfaceMethod resolves a R_USEIFACEMETHOD target such as
// "type:pkg.I+104" to the method that it names.
func (p *program) resolveIfaceMethod(arg string) (ifaceMethod, error) {
	sym, offStr, ok := strings.Cut(strings.TrimPrefix(arg, "type:"), "+")
	if !ok {
		return ifaceMethod{}, fmt.Errorf("%s: no offset", arg)
	}
	off, err := strconv.Atoi(offStr)
	if err != nil {
		return ifaceMethod{}, fmt.Errorf("%s: %w", arg, err)
	}
	var typ types.Type
	// An interface type descriptor is the 80-byte abi.InterfaceType, the
	// 16-byte abi.UncommonType of a named type, and then the methods, 8
	// bytes each.
	base := 80
	if strings.HasPrefix(sym, "interface {") {
		tv, err := types.Eval(p.fset, nil, token.NoPos, sym)
		if err != nil {
			return ifaceMethod{}, fmt.Errorf("%s: %w: %w", arg, errUnresolved, err)
		}
		typ = tv.Type
	} else {
		base += 16
		pkg := symPackage(sym)
		name := sym
		if pkg != "" {
			name = sym[len(pkg)+1:]
		}
		if typ = p.lookupType(pkg, name); typ == nil {
			return ifaceMethod{}, fmt.Errorf("%s: %w", arg, errUnresolved)
		}
	}
	it, ok := typ.Underlying().(*types.Interface)
	if !ok {
		return ifaceMethod{}, fmt.Errorf("%s: not an interface", arg)
	}
	methods := make([]*types.Func, it.NumMethods())
	for i := range methods {
		methods[i] = it.Method(i)
	}
	slices.SortFunc(methods, symLess)
	idx := (off - base) / 8
	if (off-base)%8 != 0 || idx < 0 || idx >= len(methods) {
		return ifaceMethod{}, fmt.Errorf("%s: offset %d is not a method of %d", arg, off, len(methods))
	}
	m := ifaceMethod{iface: it, method: methods[idx], open: true}
	for _, f := range methods {
		if !f.Exported() {
			m.open = false
			if f.Pkg() != nil && p.lib[f.Pkg().Path()] != nil {
				m.sealed = true
			}
		}
	}
	return m, nil
}

// libraryImplementations returns the symbols of the methods of the library
// types that implement m.iface, for method m.method.
func (p *program) libraryImplementations(m ifaceMethod) []string {
	var syms []string
	for _, l := range p.order {
		scope := l.pkg.Scope()
		for _, name := range scope.Names() {
			tn, ok := scope.Lookup(name).(*types.TypeName)
			if !ok || tn.IsAlias() {
				continue
			}
			named, ok := tn.Type().(*types.Named)
			if !ok || types.IsInterface(named) || named.TypeParams().Len() > 0 {
				continue
			}
			for _, t := range []types.Type{named, types.NewPointer(named)} {
				if !types.Implements(t, m.iface) {
					continue
				}
				obj, _, _ := types.LookupFieldOrMethod(t, true, m.method.Pkg(), m.method.Name())
				if fn, ok := obj.(*types.Func); ok {
					if s := funcSym(fn); s != "" {
						syms = append(syms, s)
					}
				}
				break
			}
		}
	}
	return syms
}

// declFunc is a function or method declared in a library package.
type declFunc struct {
	lib  *libPackage
	decl *ast.FuncDecl
	fn   *types.Func
	sym  string
}

// declaredFuncs returns the functions and methods declared in the library.
func (p *program) declaredFuncs() []declFunc {
	var fns []declFunc
	for _, l := range p.order {
		for _, f := range l.files {
			for _, d := range f.Decls {
				fd, ok := d.(*ast.FuncDecl)
				if !ok {
					continue
				}
				fn, ok := l.info.Defs[fd.Name].(*types.Func)
				if !ok {
					continue
				}
				fns = append(fns, declFunc{lib: l, decl: fd, fn: fn, sym: funcSym(fn)})
			}
		}
	}
	return fns
}

// dynamicCall is a call of a func value in library source.
type dynamicCall struct {
	sig *types.Signature
}

// funcValue is a function that library code uses as a value, which a
// dynamic call can then call.
type funcValue struct {
	sig *types.Signature
	// syms holds the symbols of its code; a func literal can have more
	// than one when the compiler inlined its enclosing function.
	syms []string
	// pos is the position of a func literal, whose symbols the gate
	// finds from the first instruction of each closure.
	pos position
	lit bool
}

// funcValues collects the dynamic calls of the library, by position, and
// the functions that the library uses as values.
func (p *program) funcValues() (map[position][]dynamicCall, []funcValue) {
	calls := map[position][]dynamicCall{}
	var values []funcValue
	for _, l := range p.order {
		for _, f := range l.files {
			callee := map[ast.Expr]bool{}
			ast.Inspect(f, func(n ast.Node) bool {
				switch n := n.(type) {
				case *ast.CallExpr:
					fun := ast.Unparen(n.Fun)
					callee[fun] = true
					if ix, ok := fun.(*ast.IndexExpr); ok {
						callee[ast.Unparen(ix.X)] = true
					}
					if ix, ok := fun.(*ast.IndexListExpr); ok {
						callee[ast.Unparen(ix.X)] = true
					}
					tv := l.info.Types[fun]
					if tv.IsType() || tv.IsBuiltin() {
						return true
					}
					sig, ok := tv.Type.Underlying().(*types.Signature)
					if !ok || isStatic(l.info, fun) {
						return true
					}
					pos := p.fset.Position(n.Lparen)
					k := position{filepath.Base(pos.Filename), pos.Line}
					calls[k] = append(calls[k], dynamicCall{sig})
				case *ast.FuncLit:
					sig, _ := l.info.Types[n].Type.(*types.Signature)
					pos := p.fset.Position(n.Pos())
					values = append(values, funcValue{sig: sig, pos: position{filepath.Base(pos.Filename), pos.Line}, lit: true})
				case *ast.Ident, *ast.SelectorExpr:
					e, _ := n.(ast.Expr)
					if callee[e] {
						return true
					}
					var obj types.Object
					if id, ok := e.(*ast.Ident); ok {
						obj = l.info.Uses[id]
					} else if se, ok := e.(*ast.SelectorExpr); ok {
						callee[se.Sel] = true
						if s := l.info.Selections[se]; s != nil {
							obj = s.Obj()
						} else {
							obj = l.info.Uses[se.Sel]
						}
					}
					fn, ok := obj.(*types.Func)
					if !ok {
						return true
					}
					sig, ok := l.info.Types[e].Type.(*types.Signature)
					if !ok {
						return true
					}
					if s := funcSym(fn); s != "" {
						values = append(values, funcValue{sig: sig, syms: []string{s}})
					}
				}
				return true
			})
		}
	}
	return calls, values
}

// isStatic reports whether fun names a function or a concrete method, so
// that the call is direct, or an interface method, which the gate resolves
// from the relocations of the machine code.
func isStatic(info *types.Info, fun ast.Expr) bool {
	switch e := fun.(type) {
	case *ast.IndexExpr:
		return isStatic(info, ast.Unparen(e.X))
	case *ast.IndexListExpr:
		return isStatic(info, ast.Unparen(e.X))
	case *ast.Ident:
		_, ok := info.Uses[e].(*types.Func)
		return ok
	case *ast.SelectorExpr:
		if s := info.Selections[e]; s != nil {
			_, ok := s.Obj().(*types.Func)
			return ok
		}
		_, ok := info.Uses[e.Sel].(*types.Func)
		return ok
	}
	return false
}
