package main

import (
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"
	"strings"
	"time"
)

type tracer struct {
	pkg   *types.Package
	info  *types.Info
	alias map[types.Object]*types.Var
	files []*ast.File
	out   map[*types.Var][]string
}

// findDefaults returns, for each field of st, the values that the
// package assigns when the field holds its zero value. It follows the field
// through plain copies (assignments, composite literal keys and call
// arguments to functions of the package) and then looks for
//
//	if x == 0 { x = V }   and   switch { case x == 0: x = V }
//
// where x is the field or a copy of it. Fields whose default the code
// computes in another way get no entry.
func findDefaults(pi *pkgInfo, st *types.Struct) map[*types.Var][]string {
	t := &tracer{pkg: pi.pkg.Types, info: pi.pkg.TypesInfo, alias: map[types.Object]*types.Var{}, out: map[*types.Var][]string{}}
	for i := 0; i < st.NumFields(); i++ {
		t.alias[st.Field(i)] = st.Field(i)
	}
	for _, f := range pi.pkg.Syntax {
		if !strings.HasSuffix(pi.pkg.Fset.File(f.Pos()).Name(), "_test.go") {
			t.files = append(t.files, f)
		}
	}
	for t.propagate() {
	}
	for _, f := range t.files {
		ast.Inspect(f, t.findTests)
	}
	return t.out
}

func (t *tracer) objOf(e ast.Expr) types.Object {
	switch x := ast.Unparen(e).(type) {
	case *ast.Ident:
		if o := t.info.Uses[x]; o != nil {
			return o
		}
		return t.info.Defs[x]
	case *ast.SelectorExpr:
		if s := t.info.Selections[x]; s != nil {
			return s.Obj()
		}
		return t.info.Uses[x.Sel]
	}
	return nil
}

func (t *tracer) fieldOf(e ast.Expr) *types.Var {
	if o := t.objOf(e); o != nil {
		return t.alias[o]
	}
	return nil
}

func (t *tracer) link(dst types.Object, src ast.Expr) bool {
	if dst == nil || t.alias[dst] != nil {
		return false
	}
	f := t.fieldOf(src)
	if f == nil {
		return false
	}
	t.alias[dst] = f
	return true
}

func (t *tracer) propagate() bool {
	changed := false
	for _, f := range t.files {
		ast.Inspect(f, func(n ast.Node) bool {
			switch x := n.(type) {
			case *ast.AssignStmt:
				if len(x.Lhs) == len(x.Rhs) {
					for i := range x.Lhs {
						changed = t.link(t.objOf(x.Lhs[i]), x.Rhs[i]) || changed
					}
				}
			case *ast.ValueSpec:
				if len(x.Names) == len(x.Values) {
					for i := range x.Names {
						changed = t.link(t.info.Defs[x.Names[i]], x.Values[i]) || changed
					}
				}
			case *ast.KeyValueExpr:
				if k, ok := x.Key.(*ast.Ident); ok {
					changed = t.link(t.info.Uses[k], x.Value) || changed
				}
			case *ast.CallExpr:
				changed = t.linkCall(x) || changed
			}
			return true
		})
	}
	return changed
}

func (t *tracer) linkCall(x *ast.CallExpr) bool {
	fn, ok := t.objOf(x.Fun).(*types.Func)
	if !ok || fn.Pkg() != t.pkg {
		return false
	}
	sig, ok := fn.Type().(*types.Signature)
	if !ok {
		return false
	}
	changed := false
	for i, a := range x.Args {
		if i < sig.Params().Len() {
			changed = t.link(sig.Params().At(i), a) || changed
		}
	}
	return changed
}

func (t *tracer) zeroTest(cond ast.Expr) *types.Var {
	be, ok := ast.Unparen(cond).(*ast.BinaryExpr)
	if !ok || (be.Op != token.EQL && be.Op != token.LEQ) {
		return nil
	}
	f := t.fieldOf(be.X)
	if f == nil {
		return nil
	}
	tv := t.info.Types[be.Y]
	if tv.IsNil() {
		return f
	}
	if tv.Value == nil {
		return nil
	}
	switch tv.Value.Kind() {
	case constant.Int, constant.Float:
		if constant.Sign(tv.Value) == 0 {
			return f
		}
	case constant.String:
		if constant.StringVal(tv.Value) == "" {
			return f
		}
	default:
	}
	return nil
}

func (t *tracer) findTests(n ast.Node) bool {
	switch x := n.(type) {
	case *ast.IfStmt:
		if fv := t.zeroTest(x.Cond); fv != nil {
			t.record(fv, x.Body.List)
		}
	case *ast.SwitchStmt:
		if x.Tag != nil {
			return true
		}
		for _, c := range x.Body.List {
			cc, ok := c.(*ast.CaseClause)
			if !ok {
				continue
			}
			for _, e := range cc.List {
				if fv := t.zeroTest(e); fv != nil {
					t.record(fv, cc.Body)
				}
			}
		}
	}
	return true
}

func (t *tracer) record(f *types.Var, body []ast.Stmt) {
	for _, s := range body {
		ast.Inspect(s, func(n ast.Node) bool {
			switch n.(type) {
			case *ast.FuncLit, *ast.ForStmt, *ast.RangeStmt:
				return false
			}
			as, ok := n.(*ast.AssignStmt)
			if !ok || len(as.Lhs) != 1 || len(as.Rhs) != 1 || as.Tok != token.ASSIGN || t.fieldOf(as.Lhs[0]) != f {
				return true
			}
			v := valueString(t.info, as.Rhs[0])
			for _, o := range t.out[f] {
				if o == v {
					return true
				}
			}
			t.out[f] = append(t.out[f], v)
			return true
		})
	}
}

func valueString(info *types.Info, e ast.Expr) string {
	src := "`" + types.ExprString(e) + "`"
	tv := info.Types[e]
	if tv.Value == nil {
		return src
	}
	v := constString(tv.Type, tv.Value.ExactString())
	if v == types.ExprString(e) {
		return src
	}
	return src + " (" + v + ")"
}

func durationString(ns int64) string {
	return time.Duration(ns).String()
}
