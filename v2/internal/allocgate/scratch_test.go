//go:build !tinygo

package allocgate_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"os"
	"path/filepath"
	"strings"
)

func (gt *gate) boundedScratch(info *types.Info, arg ast.Expr, callee *grower) string {
	se, ok := ast.Unparen(arg).(*ast.SliceExpr)
	if !ok || se.Low != nil || se.Max != nil || se.Slice3 || !isZeroLit(se.High) {
		return "it is not a field sliced to [:0]"
	}
	sel, ok := ast.Unparen(se.X).(*ast.SelectorExpr)
	if !ok {
		return "it is not a field sliced to [:0]"
	}
	s := info.Selections[sel]
	if s == nil || s.Kind() != types.FieldVal {
		return "it is not a field sliced to [:0]"
	}
	f, ok := s.Obj().(*types.Var)
	if !ok || f.Pkg() == nil || f.Exported() {
		return "it is not an unexported field sliced to [:0]"
	}
	l := gt.p.lib[f.Pkg().Path()]
	if l == nil {
		return "the field is not in the library"
	}
	c, why := gt.scratchCap(l, f, callee.d.fn.Pkg())
	if why != "" {
		return why
	}
	if !gt.boundTested(callee, c) {
		return "no fuzz test of " + callee.d.fn.Pkg().Name() + " checks a len against " + c.Name()
	}
	return ""
}

func isZeroLit(e ast.Expr) bool {
	b, ok := ast.Unparen(e).(*ast.BasicLit)
	return ok && b.Kind == token.INT && b.Value == "0"
}

func (gt *gate) scratchCap(l *libPackage, f *types.Var, from *types.Package) (*types.Const, string) {
	var c *types.Const
	why := ""
	writes := 0
	isField := func(e ast.Expr) bool {
		sel, ok := ast.Unparen(e).(*ast.SelectorExpr)
		if !ok {
			return false
		}
		s := l.info.Selections[sel]
		return s != nil && s.Obj() == f
	}
	for _, file := range l.files {
		for _, d := range file.Decls {
			fd, ok := d.(*ast.FuncDecl)
			if !ok || fd.Body == nil {
				continue
			}
			inNew := fd.Recv == nil && strings.HasPrefix(fd.Name.Name, "New")
			ast.Inspect(fd.Body, func(n ast.Node) bool {
				switch n := n.(type) {
				case *ast.UnaryExpr:
					if n.Op == token.AND && isField(n.X) {
						why = "its address is taken"
					}
				case *ast.KeyValueExpr:
					if id, ok := n.Key.(*ast.Ident); ok && l.info.Uses[id] == f {
						writes++
						if w := setIn(inNew, l.info, n.Value, from, &c); w != "" {
							why = w
						}
					}
				case *ast.IncDecStmt:
					if isField(n.X) {
						why = "it is changed outside New"
					}
				case *ast.AssignStmt:
					for i, lhs := range n.Lhs {
						if !isField(lhs) {
							continue
						}
						writes++
						if len(n.Rhs) != len(n.Lhs) {
							why = "it is set from a call with more results"
							continue
						}
						if w := setIn(inNew, l.info, n.Rhs[i], from, &c); w != "" {
							why = w
						}
					}
				}
				return true
			})
		}
	}
	if why == "" && writes == 0 {
		why = "nothing sets it"
	}
	return c, why
}

func setIn(inNew bool, info *types.Info, e ast.Expr, from *types.Package, c **types.Const) string {
	if !inNew {
		return "it is set outside New"
	}
	k, w := makeCap(info, e, from)
	switch {
	case w != "":
		return w
	case *c != nil && *c != k:
		return "New sets it with two capacities"
	}
	*c = k
	return ""
}

func makeCap(info *types.Info, e ast.Expr, from *types.Package) (*types.Const, string) {
	call, ok := ast.Unparen(e).(*ast.CallExpr)
	if !ok || !isBuiltin(info, call.Fun, "make") || len(call.Args) != 3 || !isZeroLit(call.Args[1]) {
		return nil, "New does not set it with make(T, 0, Max)"
	}
	var id *ast.Ident
	switch x := ast.Unparen(call.Args[2]).(type) {
	case *ast.Ident:
		id = x
	case *ast.SelectorExpr:
		id = x.Sel
	}
	if id == nil {
		return nil, "its capacity is not a named constant"
	}
	c, ok := info.Uses[id].(*types.Const)
	if !ok || !strings.HasPrefix(c.Name(), "Max") || !c.Exported() || c.Pkg() != from {
		return nil, "its capacity is not an exported Max constant of the package of the callee"
	}
	return c, ""
}

func (gt *gate) boundTested(callee *grower, c *types.Const) bool {
	dir := filepath.Dir(gt.p.fset.Position(callee.d.decl.Pos()).Filename)
	files, _ := filepath.Glob(filepath.Join(dir, "*_test.go"))
	for _, name := range files {
		src, err := os.ReadFile(name)
		if err != nil {
			continue
		}
		af, err := parser.ParseFile(token.NewFileSet(), name, src, parser.SkipObjectResolution)
		if err != nil {
			continue
		}
		fuzz, checked := false, false
		ast.Inspect(af, func(n ast.Node) bool {
			switch n := n.(type) {
			case *ast.FuncDecl:
				fuzz = fuzz || n.Recv == nil && strings.HasPrefix(n.Name.Name, "Fuzz")
			case *ast.BinaryExpr:
				if n.Op == token.GTR || n.Op == token.GEQ || n.Op == token.LSS || n.Op == token.LEQ {
					checked = checked || isLen(n.X) && names(n.Y, c) || isLen(n.Y) && names(n.X, c)
				}
			}
			return true
		})
		if fuzz && checked {
			return true
		}
	}
	return false
}

func isLen(e ast.Expr) bool {
	call, ok := ast.Unparen(e).(*ast.CallExpr)
	if !ok {
		return false
	}
	id, ok := call.Fun.(*ast.Ident)
	return ok && id.Name == "len"
}

func names(e ast.Expr, c *types.Const) bool {
	switch x := ast.Unparen(e).(type) {
	case *ast.Ident:
		return x.Name == c.Name()
	case *ast.SelectorExpr:
		pkg, ok := x.X.(*ast.Ident)
		return ok && pkg.Name == c.Pkg().Name() && x.Sel.Name == c.Name()
	}
	return false
}
