// Package nilguard adds a nil receiver guard to the exported methods with a
// pointer receiver of a Go file, so that a call on a nil receiver returns
// dmrgo.ErrNilReceiver, dst or zero values instead of a panic.
package nilguard

import (
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
)

// Module is the import path of the library module.
const Module = "github.com/USA-RedDragon/dmrgo/v2"

type kind int

const (
	kNil kind = iota
	kComposite
	kZero
	kFalse
	kEmpty
)

// Guard rewrites files of one library tree.
type Guard struct {
	root  string
	pkgs  map[string]map[string]ast.Expr
	files map[string]*ast.File
}

// New returns a Guard for the v2 directory root.
func New(root string) *Guard {
	return &Guard{root: root, pkgs: map[string]map[string]ast.Expr{}, files: map[string]*ast.File{}}
}

func (g *Guard) decls(dir string, extra *ast.File) map[string]ast.Expr {
	m, ok := g.pkgs[dir]
	if !ok {
		m = map[string]ast.Expr{}
		ents, _ := os.ReadDir(dir)
		fset := token.NewFileSet()
		for _, e := range ents {
			n := e.Name()
			if !strings.HasSuffix(n, ".go") || strings.HasSuffix(n, "_test.go") {
				continue
			}
			f, err := parser.ParseFile(fset, filepath.Join(dir, n), nil, parser.SkipObjectResolution)
			if err != nil {
				continue
			}
			g.files[filepath.Join(dir, n)] = f
			addDecls(m, f)
		}
		g.pkgs[dir] = m
	}
	if extra != nil {
		addDecls(m, extra)
	}
	return m
}

func addDecls(m map[string]ast.Expr, f *ast.File) {
	for _, d := range f.Decls {
		gd, ok := d.(*ast.GenDecl)
		if !ok || gd.Tok != token.TYPE {
			continue
		}
		for _, s := range gd.Specs {
			if ts, ok := s.(*ast.TypeSpec); ok {
				m[ts.Name.Name] = ts.Type
			}
		}
	}
}

const rootPkg = "dmrgo"

func basicKind(name string) (kind, bool) {
	switch name {
	case "bool":
		return kFalse, true
	case "string":
		return kEmpty, true
	case "any", "error":
		return kNil, true
	case "int", "int8", "int16", "int32", "int64", "uint", "uint8", "uint16", "uint32", "uint64", "uintptr",
		"byte", "rune", "float32", "float64", "complex64", "complex128":
		return kZero, true
	}
	return 0, false
}

func stdKind(name string) (kind, bool) {
	switch name {
	case "time.Duration":
		return kZero, true
	case "time.Time":
		return kComposite, true
	case "unsafe.Pointer":
		return kNil, true
	}
	return 0, false
}

func (g *Guard) kindOf(t ast.Expr, dir string, f *ast.File, depth int) (kind, error) {
	if depth > 16 {
		return 0, fmt.Errorf("type too deep")
	}
	switch t := t.(type) {
	case *ast.StarExpr, *ast.MapType, *ast.FuncType, *ast.ChanType, *ast.InterfaceType:
		return kNil, nil
	case *ast.ArrayType:
		if t.Len == nil {
			return kNil, nil
		}
		return kComposite, nil
	case *ast.StructType:
		return kComposite, nil
	case *ast.ParenExpr:
		return g.kindOf(t.X, dir, f, depth+1)
	case *ast.IndexExpr, *ast.IndexListExpr:
		return kComposite, nil
	case *ast.Ident:
		if k, ok := basicKind(t.Name); ok {
			return k, nil
		}
		u, ok := g.decls(dir, nil)[t.Name]
		if !ok {
			return 0, fmt.Errorf("unknown type %s in %s", t.Name, dir)
		}
		return g.kindOf(u, dir, nil, depth+1)
	case *ast.SelectorExpr:
		x, ok := t.X.(*ast.Ident)
		if !ok {
			return 0, fmt.Errorf("unknown selector type in %s", dir)
		}
		pkg := x.Name
		path := importPath(f, pkg)
		if k, ok := stdKind(filepath.Base(path) + "." + t.Sel.Name); ok && !strings.HasPrefix(path, Module) {
			return k, nil
		}
		if !strings.HasPrefix(path, Module) {
			return 0, fmt.Errorf("unknown type %s.%s", pkg, t.Sel.Name)
		}
		d := filepath.Join(g.root, strings.TrimPrefix(strings.TrimPrefix(path, Module), "/"))
		u, ok := g.decls(d, nil)[t.Sel.Name]
		if !ok {
			return 0, fmt.Errorf("unknown type %s", path+"."+t.Sel.Name)
		}
		var df *ast.File
		for p, ff := range g.files {
			if filepath.Dir(p) == d && declares(ff, t.Sel.Name) {
				df = ff
			}
		}
		return g.kindOf(u, d, df, depth+1)
	}
	return 0, fmt.Errorf("unknown type expression %T", t)
}

func declares(f *ast.File, name string) bool {
	for _, d := range f.Decls {
		if gd, ok := d.(*ast.GenDecl); ok && gd.Tok == token.TYPE {
			for _, s := range gd.Specs {
				if ts, ok := s.(*ast.TypeSpec); ok && ts.Name.Name == name {
					return true
				}
			}
		}
	}
	return false
}

func importPath(f *ast.File, name string) string {
	if f == nil {
		return name
	}
	for _, im := range f.Imports {
		p, _ := strconv.Unquote(im.Path.Value)
		n := filepath.Base(p)
		if im.Name != nil {
			n = im.Name.Name
		}
		if n == name {
			return p
		}
	}
	return name
}

func rootName(f *ast.File) string {
	for _, im := range f.Imports {
		if p, _ := strconv.Unquote(im.Path.Value); p == Module {
			if im.Name != nil {
				return im.Name.Name
			}
			return rootPkg
		}
	}
	return ""
}

func text(fset *token.FileSet, src []byte, n ast.Node) string {
	return string(src[fset.Position(n.Pos()).Offset:fset.Position(n.End()).Offset])
}

// Apply returns src, a file of the package in dir, with a guard in each
// exported method with a pointer receiver that has none, and the number of
// guards it added.
func (g *Guard) Apply(src []byte, dir string) ([]byte, int, error) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "", src, parser.ParseComments|parser.SkipObjectResolution)
	if err != nil {
		return nil, 0, err
	}
	g.decls(dir, f)
	inRoot := f.Name.Name == rootPkg && filepath.Clean(dir) == filepath.Clean(g.root)
	errName := "ErrNilReceiver"
	needImport := false
	if !inRoot {
		n := rootName(f)
		if n == "" {
			n, needImport = rootPkg, true
		}
		errName = n + ".ErrNilReceiver"
	}
	type ins struct {
		off  int
		text string
	}
	var edits []ins
	count := 0
	usesErr := false
	for _, d := range f.Decls {
		fd, ok := d.(*ast.FuncDecl)
		if !ok || fd.Recv == nil || fd.Body == nil || !fd.Name.IsExported() {
			continue
		}
		rf := fd.Recv.List[0]
		star, ok := rf.Type.(*ast.StarExpr)
		if !ok {
			continue
		}
		tid, ok := star.X.(*ast.Ident)
		if !ok || !tid.IsExported() {
			continue
		}
		name := ""
		if len(rf.Names) > 0 && rf.Names[0].Name != "_" {
			name = rf.Names[0].Name
		}
		if name != "" && (guarded(fd.Body, name) || delegates(fd.Body, name)) {
			continue
		}
		if name == "" {
			name = "r"
			for _, p := range fd.Type.Params.List {
				for _, pn := range p.Names {
					if pn.Name == "r" {
						name = "rcv"
					}
				}
			}
			if len(rf.Names) > 0 {
				edits = append(edits, ins{fset.Position(rf.Names[0].Pos()).Offset, "\x00" + name})
			} else {
				edits = append(edits, ins{fset.Position(rf.Type.Pos()).Offset, name + " "})
			}
		}
		var vals []string
		if fd.Type.Results != nil {
			for _, r := range fd.Type.Results.List {
				n := max(1, len(r.Names))
				for range n {
					v, isErr, err := g.value(fset, src, f, dir, fd, r.Type, errName)
					if err != nil {
						return nil, 0, fmt.Errorf("%s.%s: %w", tid.Name, fd.Name.Name, err)
					}
					usesErr = usesErr || isErr
					vals = append(vals, v)
				}
			}
		}
		guard := "\n\tif " + name + " == nil {\n\t\treturn"
		if len(vals) > 0 {
			guard += " " + strings.Join(vals, ", ")
		}
		guard += "\n\t}\n"
		edits = append(edits, ins{fset.Position(fd.Body.Lbrace).Offset + 1, guard})
		count++
	}
	if count == 0 {
		return src, 0, nil
	}
	if usesErr && needImport {
		var off int
		var add string
		switch {
		case len(f.Imports) == 0:
			off, add = fset.Position(f.Name.End()).Offset, "\n\nimport \""+Module+"\""
		default:
			var last *ast.GenDecl
			for _, d := range f.Decls {
				if gd, ok := d.(*ast.GenDecl); ok && gd.Tok == token.IMPORT {
					last = gd
				}
			}
			if last.Rparen.IsValid() {
				off, add = fset.Position(last.Rparen).Offset, "\t\""+Module+"\"\n"
			} else {
				off, add = fset.Position(last.End()).Offset, "\n\nimport \""+Module+"\""
			}
		}
		edits = append(edits, ins{off, add})
	}
	slices.SortStableFunc(edits, func(a, b ins) int { return b.off - a.off })
	out := slices.Clone(src)
	for _, e := range edits {
		if strings.HasPrefix(e.text, "\x00") {
			out = slices.Concat(out[:e.off], []byte(e.text[1:]), out[e.off+1:])
			continue
		}
		out = slices.Concat(out[:e.off], []byte(e.text), out[e.off:])
	}
	res, err := format.Source(out)
	if err != nil {
		return nil, 0, fmt.Errorf("format: %w\n%s", err, out)
	}
	return res, count, nil
}

func guarded(b *ast.BlockStmt, name string) bool {
	if len(b.List) == 0 {
		return false
	}
	is, ok := b.List[0].(*ast.IfStmt)
	if !ok || is.Init != nil {
		return false
	}
	be, ok := is.Cond.(*ast.BinaryExpr)
	if !ok {
		return false
	}
	x, ok := be.X.(*ast.Ident)
	y, ok2 := be.Y.(*ast.Ident)
	return ok && ok2 && be.Op == token.EQL && x.Name == name && y.Name == "nil"
}

func (g *Guard) value(fset *token.FileSet, src []byte, f *ast.File, dir string, fd *ast.FuncDecl, t ast.Expr, errName string) (string, bool, error) {
	ts := text(fset, src, t)
	if ts == "error" {
		return errName, true, nil
	}
	if ts == "string" && (fd.Name.Name == "String" || fd.Name.Name == "Error" || fd.Name.Name == "GoString") {
		return `"<nil>"`, false, nil
	}
	if at, ok := t.(*ast.ArrayType); ok && at.Len == nil {
		for _, p := range fd.Type.Params.List {
			if text(fset, src, p.Type) != ts {
				continue
			}
			for _, pn := range p.Names {
				if pn.Name == "dst" || strings.HasPrefix(fd.Name.Name, "Append") {
					return pn.Name, false, nil
				}
			}
		}
	}
	k, err := g.kindOf(t, dir, f, 0)
	if err != nil {
		return "", false, err
	}
	switch k {
	case kNil:
		return "nil", false, nil
	case kComposite:
		return ts + "{}", false, nil
	case kFalse:
		return "false", false, nil
	case kEmpty:
		return `""`, false, nil
	case kZero:
	}
	return "0", false, nil
}

func delegates(b *ast.BlockStmt, name string) bool {
	if len(b.List) != 1 {
		return false
	}
	rs, ok := b.List[0].(*ast.ReturnStmt)
	if !ok || len(rs.Results) != 1 {
		return false
	}
	call, ok := rs.Results[0].(*ast.CallExpr)
	if !ok {
		return false
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	x, ok := sel.X.(*ast.Ident)
	return ok && x.Name == name && sel.Sel.IsExported()
}
