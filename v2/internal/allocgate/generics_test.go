//go:build !tinygo

package allocgate_test

import (
	"cmp"
	"fmt"
	"go/ast"
	"go/types"
	"slices"
	"strings"
	"testing"
)

func TestNoUnfollowedGenerics(t *testing.T) {
	t.Parallel()

	for _, h := range genericHoles(loadLibrary(t, libraryPackages(t))) {
		t.Error(h)
	}
}

func TestGenericHolesCases(t *testing.T) {
	t.Parallel()

	p := loadLibrary(t, []string{"./internal/allocgate/testdata/generics"})
	holes := map[string]bool{}
	for _, h := range genericHoles(p) {
		holes[h.fn] = true
		t.Log(h)
	}
	n := 0
	for _, d := range p.declaredFuncs() {
		name := d.fn.Name()
		want := hasPrefixWord(name, "Hole") || hasPrefixWord(name, "hole")
		if !want && !hasPrefixWord(name, "Fine") && !hasPrefixWord(name, "fine") {
			continue
		}
		n++
		if holes[d.fn.FullName()] != want {
			t.Errorf("%s: reported is %v, want %v", d.fn.FullName(), holes[d.fn.FullName()], want)
		}
	}
	if n == 0 {
		t.Fatal("no cases")
	}
}

func loadLibrary(t *testing.T, patterns []string) *program {
	t.Helper()
	pkgs := listPackages(t, patterns)
	lib := map[string]bool{}
	for _, p := range pkgs {
		if !p.DepOnly && !p.Standard {
			lib[p.ImportPath] = true
		}
	}
	return loadProgram(t, pkgs, func(path string) bool { return lib[path] })
}

type genericHole struct {
	fn, why string
}

func (h genericHole) String() string { return h.fn + ": " + h.why }

func genericHoles(p *program) []genericHole {
	var holes []genericHole
	for _, d := range p.declaredFuncs() {
		sig := d.fn.Signature()
		generic := sig.TypeParams().Len() > 0
		if recv := sig.Recv(); recv != nil {
			rt := recv.Type()
			if pt, ok := rt.(*types.Pointer); ok {
				rt = pt.Elem()
			}
			if named, ok := rt.(*types.Named); ok && named.TypeParams().Len() > 0 {
				generic = true
			}
		}
		if !generic {
			continue
		}
		name := d.fn.FullName()
		if d.fn.Exported() {
			holes = append(holes, genericHole{name, "exported generic code, which has machine code only where a caller instantiates it"})
		}
		if d.decl.Body == nil {
			continue
		}
		info := d.lib.info
		ast.Inspect(d.decl.Body, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			pos := p.fset.Position(call.Pos())
			fun := ast.Unparen(call.Fun)
			if se, ok := fun.(*ast.SelectorExpr); ok {
				if s := info.Selections[se]; s != nil && s.Kind() == types.MethodVal && isTypeParam(s.Recv()) {
					holes = append(holes, genericHole{name, fmt.Sprintf("%s: it calls the method %s of a type parameter, through the dictionary", pos, se.Sel.Name)})
					return true
				}
			}
			tv := info.Types[call.Fun]
			if tv.IsType() || tv.IsBuiltin() || isStatic(info, fun) || isFuncLit(fun) {
				return true
			}
			if sig, ok := tv.Type.Underlying().(*types.Signature); ok && mentionsTypeParam(sig) {
				holes = append(holes, genericHole{name, fmt.Sprintf("%s: it calls a func value whose signature has a type parameter", pos)})
			}
			return true
		})
	}
	slices.SortFunc(holes, func(a, b genericHole) int {
		return cmp.Or(strings.Compare(a.fn, b.fn), strings.Compare(a.why, b.why))
	})
	return holes
}

func isTypeParam(t types.Type) bool {
	if pt, ok := t.(*types.Pointer); ok {
		t = pt.Elem()
	}
	_, ok := t.(*types.TypeParam)
	return ok
}

func mentionsTypeParam(sig *types.Signature) bool {
	var has func(t types.Type) bool
	has = func(t types.Type) bool {
		switch t := t.(type) {
		case *types.TypeParam:
			return true
		case *types.Pointer:
			return has(t.Elem())
		case *types.Slice:
			return has(t.Elem())
		case *types.Array:
			return has(t.Elem())
		case *types.Map:
			return has(t.Key()) || has(t.Elem())
		case *types.Chan:
			return has(t.Elem())
		case *types.Signature:
			return mentionsTypeParam(t)
		case *types.Named:
			for i := range t.TypeArgs().Len() {
				if has(t.TypeArgs().At(i)) {
					return true
				}
			}
		}
		return false
	}
	for _, tup := range []*types.Tuple{sig.Params(), sig.Results()} {
		for i := range tup.Len() {
			if has(tup.At(i).Type()) {
				return true
			}
		}
	}
	return false
}
