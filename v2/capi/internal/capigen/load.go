package capigen

import (
	"errors"
	"fmt"
	"go/ast"
	"go/build"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
)

const modulePath = "github.com/USA-RedDragon/dmrgo/v2"

type loader struct {
	ctx  build.Context
	root string
	fset *token.FileSet
	std  types.Importer
	pkgs map[string]*types.Package
	docs map[types.Object]string
	full map[types.Object]string
	busy map[string]bool
	decl map[*types.Func]*ast.FuncDecl
	info map[*types.Package]*types.Info
	eff  map[effKey]*effect
	vals map[types.Object]ast.Expr
	refs map[string][][]string
}

func newLoader(root string) *loader {
	ctx := build.Default
	ctx.CgoEnabled = true
	ctx.GOOS, ctx.GOARCH = "linux", "amd64"
	fset := token.NewFileSet()
	return &loader{
		ctx: ctx, root: root, fset: fset, std: importer.ForCompiler(fset, "source", nil),
		pkgs: map[string]*types.Package{}, docs: map[types.Object]string{}, full: map[types.Object]string{}, busy: map[string]bool{},
		decl: map[*types.Func]*ast.FuncDecl{}, info: map[*types.Package]*types.Info{}, eff: map[effKey]*effect{}, vals: map[types.Object]ast.Expr{}, refs: map[string][][]string{},
	}
}

func (l *loader) Import(p string) (*types.Package, error) { return l.ImportFrom(p, "", 0) }

func (l *loader) ImportFrom(p, _ string, _ types.ImportMode) (*types.Package, error) {
	if p != modulePath && !strings.HasPrefix(p, modulePath+"/") {
		return l.std.Import(p)
	}
	return l.load(p)
}

func (l *loader) load(p string) (*types.Package, error) {
	if pkg, ok := l.pkgs[p]; ok {
		return pkg, nil
	}
	if l.busy[p] {
		return nil, fmt.Errorf("%s: import cycle", p)
	}
	l.busy[p] = true
	dir := filepath.Join(l.root, filepath.FromSlash(strings.TrimPrefix(strings.TrimPrefix(p, modulePath), "/")))
	bp, err := l.ctx.ImportDir(dir, 0)
	if err != nil {
		return nil, err
	}
	var files []*ast.File
	for _, f := range append(bp.GoFiles, bp.CgoFiles...) {
		af, err := parser.ParseFile(l.fset, filepath.Join(dir, f), nil, parser.ParseComments|parser.SkipObjectResolution)
		if err != nil {
			return nil, err
		}
		files = append(files, af)
	}
	info := &types.Info{Defs: map[*ast.Ident]types.Object{}, Uses: map[*ast.Ident]types.Object{},
		Types: map[ast.Expr]types.TypeAndValue{}, Selections: map[*ast.SelectorExpr]*types.Selection{}, Implicits: map[ast.Node]types.Object{}}
	conf := types.Config{Importer: l, Sizes: types.SizesFor("gc", "amd64"), FakeImportC: true}
	pkg, err := conf.Check(p, l.fset, files, info)
	if err != nil {
		return nil, err
	}
	l.collectDocs(files, info)
	for _, f := range files {
		for _, d := range f.Decls {
			if fd, ok := d.(*ast.FuncDecl); ok && fd.Body != nil {
				if fn, ok := info.Defs[fd.Name].(*types.Func); ok {
					l.decl[fn] = fd
				}
			}
		}
	}
	l.info[pkg] = info
	l.pkgs[p] = pkg
	return pkg, nil
}

func (l *loader) collectDocs(files []*ast.File, info *types.Info) {
	var in *types.Named
	set := func(id *ast.Ident, docs ...*ast.CommentGroup) {
		for _, d := range docs {
			if d != nil {
				if o := info.Defs[id]; o != nil {
					text := l.qualify(d.Text(), o.Pkg(), in)
					l.docs[o] = firstSentence(text)
					l.full[o] = strings.Join(strings.Fields(text), " ")
				}
				return
			}
		}
	}
	for _, f := range files {
		for _, d := range f.Decls {
			switch d := d.(type) {
			case *ast.FuncDecl:
				if fn, ok := info.Defs[d.Name].(*types.Func); ok && fn.Signature().Recv() != nil {
					in, _ = derefType(fn.Signature().Recv().Type()).(*types.Named)
				}
				set(d.Name, d.Doc)
				in = nil
			case *ast.GenDecl:
				for _, s := range d.Specs {
					switch s := s.(type) {
					case *ast.TypeSpec:
						set(s.Name, s.Doc, d.Doc)
						if st, ok := s.Type.(*ast.StructType); ok {
							in, _ = info.Defs[s.Name].Type().(*types.Named)
							for _, fl := range st.Fields.List {
								for _, n := range fl.Names {
									set(n, fl.Doc, fl.Comment)
								}
							}
							in = nil
						}
						if it, ok := s.Type.(*ast.InterfaceType); ok {
							for _, m := range it.Methods.List {
								for _, n := range m.Names {
									set(n, m.Doc, m.Comment)
								}
							}
						}
					case *ast.ValueSpec:
						for i, n := range s.Names {
							set(n, s.Doc, s.Comment, d.Doc)
							if len(s.Values) == len(s.Names) {
								if o := info.Defs[n]; o != nil {
									l.vals[o] = s.Values[i]
								}
							}
						}
					}
				}
			}
		}
	}
}

func firstSentence(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if i := strings.Index(s, ". "); i >= 0 {
		s = s[:i+1]
	}
	return strings.ReplaceAll(s, "*/", "* /")
}

func (l *loader) packages() ([]*types.Package, error) {
	skipDirs := []string{"capi", "cmd", "examples", "internal"}
	var paths []string
	err := filepath.WalkDir(l.root, func(p string, d os.DirEntry, err error) error {
		if err != nil || !d.IsDir() {
			return err
		}
		rel, _ := filepath.Rel(l.root, p)
		rel = filepath.ToSlash(rel)
		if d.Name() == "testdata" || slices.ContainsFunc(skipDirs, func(s string) bool { return rel == s || strings.HasPrefix(rel, s+"/") }) {
			return filepath.SkipDir
		}
		bp, err := l.ctx.ImportDir(p, 0)
		if err != nil {
			if errors.As(err, new(*build.NoGoError)) {
				return nil
			}
			return err
		}
		if bp.Name == "main" {
			return nil
		}
		ip := modulePath
		if rel != "." {
			ip = path.Join(modulePath, rel)
		}
		paths = append(paths, ip)
		return nil
	})
	if err != nil {
		return nil, err
	}
	slices.Sort(paths)
	var out []*types.Package
	for _, p := range paths {
		pkg, err := l.load(p)
		if err != nil {
			return nil, err
		}
		out = append(out, pkg)
	}
	return out, nil
}
