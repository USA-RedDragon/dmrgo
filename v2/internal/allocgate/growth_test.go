//go:build !tinygo

package allocgate_test

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"maps"
	"path/filepath"
	"slices"
	"strings"
)

// grower is a library function that takes a slice and returns a slice of
// the same type, such as AppendPack(dst []byte, bits []Bit) []byte or
// appendEvent(out []Event, ...) ([]Event, *Event). The gate proves from its
// source that every append in it grows that slice, or a value that it
// derives from that slice, and that it passes such a value as the slice of
// each grower it calls.
type grower struct {
	d *declFunc
	// param is the index of the slice parameter and result the index of
	// the result of the same type.
	param, result int
	// calls holds the growers that it calls by a static call.
	calls map[string]bool
	// appends is set when its body has an append.
	appends bool
}

// growerShape returns the index of the slice parameter of fn and of the
// result of the same type. A non-variadic parameter of slice type whose
// type is also a result type is the slice parameter; when there are more
// than one, the one named dst is.
func growerShape(fn *types.Func) (param, result int, ok bool) {
	sig := fn.Signature()
	var params []int
	for i := range sig.Params().Len() {
		v := sig.Params().At(i)
		if _, isSlice := v.Type().Underlying().(*types.Slice); !isSlice || sig.Variadic() && i == sig.Params().Len()-1 {
			continue
		}
		if sliceResult(sig, v.Type()) >= 0 {
			params = append(params, i)
		}
	}
	if len(params) > 1 {
		params = slices.DeleteFunc(params, func(i int) bool { return sig.Params().At(i).Name() != "dst" })
	}
	if len(params) != 1 {
		return 0, 0, false
	}
	return params[0], sliceResult(sig, sig.Params().At(params[0]).Type()), true
}

func sliceResult(sig *types.Signature, t types.Type) int {
	for i := range sig.Results().Len() {
		if types.Identical(sig.Results().At(i).Type(), t) {
			return i
		}
	}
	return -1
}

// growers computes the library functions that grow only their slice
// parameter, as the greatest set of functions of grower shape whose bodies
// pass growerCheck with the set. It returns them by symbol, and why each
// function of grower shape is not in the set.
func (gt *gate) growers() (map[string]*grower, map[string]string) {
	set := map[string]*grower{}
	for _, d := range gt.p.declaredFuncs() {
		if d.sym == "" || d.decl.Body == nil {
			continue
		}
		param, result, ok := growerShape(d.fn)
		if ok {
			set[d.sym] = &grower{d: &d, param: param, result: result}
		}
	}
	why := map[string]string{}
	for changed := true; changed; {
		changed = false
		for _, sym := range slices.Sorted(maps.Keys(set)) {
			if w := gt.growerCheck(set[sym], set, why); w != "" {
				why[sym] = w
				delete(set, sym)
				changed = true
			}
		}
	}
	return set, why
}

// derivation tracks the local variables of a function that hold its slice
// parameter or a value derived from it.
type derivation struct {
	gt      *gate
	info    *types.Info
	set     map[string]*grower
	derived map[*types.Var]bool
}

// expr reports whether e, or result idx of the call e, is the slice
// parameter or derived from it: a variable that holds such a value, a
// slice expression or an append of such a value, or the result of a
// grower called with such a value as its slice.
func (dv *derivation) expr(e ast.Expr, idx int) bool {
	switch e := ast.Unparen(e).(type) {
	case *ast.Ident:
		v, ok := dv.info.Uses[e].(*types.Var)
		return ok && idx == 0 && dv.derived[v]
	case *ast.SliceExpr:
		return idx == 0 && dv.expr(e.X, 0)
	case *ast.CallExpr:
		if isBuiltin(dv.info, e.Fun, "append") {
			return idx == 0 && len(e.Args) > 0 && dv.expr(e.Args[0], 0)
		}
		fn := calleeFunc(dv.info, e.Fun)
		if fn == nil {
			return false
		}
		g := dv.set[funcSym(fn)]
		return g != nil && g.result == idx && g.param < len(e.Args) && dv.expr(e.Args[g.param], 0)
	}
	return false
}

// assign is a value that a function stores in a variable: result idx of
// rhs, or a value that is not derived when rhs is nil.
type assign struct {
	rhs ast.Expr
	idx int
}

// growerCheck returns why the body of g breaks the shape of a grower with
// the growers of set, or "", with notGrower to say why a callee is not a
// grower. It records the growers that g calls.
func (gt *gate) growerCheck(g *grower, set map[string]*grower, notGrower map[string]string) string {
	sc := newBodyScan(gt, g, set, notGrower)
	sc.scan()
	if len(sc.why) > 0 {
		return strings.Join(sc.why, "; ")
	}
	g.appends = len(sc.appends) > 0
	sc.derive()
	sc.verify()
	return strings.Join(sc.why, "; ")
}

// bodyScan holds what growerCheck finds in the body of a grower.
type bodyScan struct {
	gt        *gate
	g         *grower
	info      *types.Info
	set       map[string]*grower
	notGrower map[string]string
	slice     *types.Var
	root      string
	// tracked holds the values stored in each variable of the type of the
	// slice parameter, and poisoned the variables whose address the body
	// takes or that a range statement sets.
	tracked  map[*types.Var][]assign
	poisoned map[*types.Var]bool
	invoked  map[*ast.FuncLit]bool
	appends  []*ast.CallExpr
	calls    []growerCall
	returns  []*ast.ReturnStmt
	dv       *derivation
	why      []string
}

// growerCall is a call of a grower.
type growerCall struct {
	g    *grower
	call *ast.CallExpr
}

func newBodyScan(gt *gate, g *grower, set map[string]*grower, notGrower map[string]string) *bodyScan {
	sig := g.d.fn.Signature()
	sc := &bodyScan{gt: gt, g: g, info: g.d.lib.info, set: set, notGrower: notGrower, slice: sig.Params().At(g.param),
		poisoned: map[*types.Var]bool{}, invoked: map[*ast.FuncLit]bool{}}
	sc.root, _ = filepath.Abs(moduleRoot)
	sc.tracked = map[*types.Var][]assign{sc.slice: nil}
	for i := range sig.Results().Len() {
		if r := sig.Results().At(i); r.Name() != "" && types.Identical(r.Type(), sc.slice.Type()) {
			sc.tracked[r] = []assign{{}}
		}
	}
	body := g.d.decl.Body
	for id, obj := range sc.info.Defs {
		v, ok := obj.(*types.Var)
		if ok && !v.IsField() && id.Pos() >= body.Pos() && id.Pos() < body.End() && types.Identical(v.Type(), sc.slice.Type()) {
			if _, ok := sc.tracked[v]; !ok {
				sc.tracked[v] = nil
			}
		}
	}
	g.calls = map[string]bool{}
	return sc
}

func (sc *bodyScan) at(n ast.Node) string {
	p := sc.gt.p.fset.Position(n.Pos())
	if rel, err := filepath.Rel(sc.root, p.Filename); err == nil && !strings.HasPrefix(rel, "..") {
		p.Filename = rel
	}
	return p.String()
}

// varOf returns the tracked variable that e names, or nil.
func (sc *bodyScan) varOf(e ast.Expr) *types.Var {
	id, ok := ast.Unparen(e).(*ast.Ident)
	if !ok {
		return nil
	}
	obj := sc.info.Defs[id]
	if obj == nil {
		obj = sc.info.Uses[id]
	}
	v, _ := obj.(*types.Var)
	if _, ok := sc.tracked[v]; !ok {
		return nil
	}
	return v
}

// store records the assignment of rhs to lhs.
func (sc *bodyScan) store(lhs, rhs []ast.Expr) {
	for i, l := range lhs {
		v := sc.varOf(l)
		if v == nil {
			continue
		}
		a := assign{}
		switch {
		case len(rhs) == len(lhs):
			a = assign{rhs[i], 0}
		case len(rhs) == 1:
			if _, ok := ast.Unparen(rhs[0]).(*ast.CallExpr); ok {
				a = assign{rhs[0], i}
			}
		}
		sc.tracked[v] = append(sc.tracked[v], a)
	}
}

// scan collects the assignments, appends, calls and returns of the body.
func (sc *bodyScan) scan() {
	body := sc.g.d.decl.Body
	ast.Inspect(body, func(n ast.Node) bool {
		if c, ok := n.(*ast.CallExpr); ok {
			if fl, ok := ast.Unparen(c.Fun).(*ast.FuncLit); ok {
				sc.invoked[fl] = true
			}
		}
		return true
	})
	ast.Inspect(body, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.FuncLit:
			// cgo turns a C call with pointer arguments into such a call of
			// a func literal, whose body is then checked as part of g.
			if !sc.invoked[n] {
				sc.why = append(sc.why, sc.at(n)+": it has a func literal that it does not call in place, whose code the gate does not follow as part of it")
				return false
			}
		case *ast.AssignStmt:
			sc.store(n.Lhs, n.Rhs)
		case *ast.ValueSpec:
			sc.valueSpec(n)
		case *ast.RangeStmt:
			for _, e := range []ast.Expr{n.Key, n.Value} {
				if v := sc.varOf(e); v != nil {
					sc.poisoned[v] = true
				}
			}
		case *ast.UnaryExpr:
			if v := sc.varOf(n.X); v != nil && n.Op == token.AND {
				sc.poisoned[v] = true
			}
		case *ast.ReturnStmt:
			if !sc.inFuncLit(n) {
				sc.returns = append(sc.returns, n)
			}
		case *ast.CallExpr:
			sc.call(n)
		}
		return true
	})
}

// inFuncLit reports whether n is in a func literal that the body calls in
// place, whose returns return from the literal.
func (sc *bodyScan) inFuncLit(n ast.Node) bool {
	for fl := range sc.invoked {
		if fl.Pos() <= n.Pos() && n.End() <= fl.End() {
			return true
		}
	}
	return false
}

func (sc *bodyScan) valueSpec(n *ast.ValueSpec) {
	lhs := make([]ast.Expr, len(n.Names))
	for i, id := range n.Names {
		lhs[i] = id
	}
	if len(n.Values) > 0 {
		sc.store(lhs, n.Values)
		return
	}
	for _, l := range lhs {
		if v := sc.varOf(l); v != nil {
			sc.tracked[v] = append(sc.tracked[v], assign{})
		}
	}
}

// call records an append or a call of a grower, and why the body breaks the
// shape for a call that the gate cannot follow or of a function that can
// allocate and is not a grower.
func (sc *bodyScan) call(n *ast.CallExpr) {
	tv := sc.info.Types[n.Fun]
	switch {
	case tv.IsType():
		return
	case isBuiltin(sc.info, n.Fun, "append"):
		sc.appends = append(sc.appends, n)
		return
	case tv.IsBuiltin(), isFuncLit(n.Fun):
		return
	}
	fn := calleeFunc(sc.info, n.Fun)
	if fn == nil {
		sc.why = append(sc.why, sc.at(n)+": it calls a func value or an interface method, which the gate cannot follow as part of it")
		return
	}
	if !sc.gt.mayAllocate(fn) {
		return
	}
	sym := funcSym(fn)
	callee := sc.set[sym]
	if callee == nil {
		w, ok := sc.notGrower[sym]
		if !ok {
			w = "it does not take and return a slice:" + sc.gt.chain(sc.gt.next, sym)
		}
		sc.why = append(sc.why, fmt.Sprintf("%s: it calls %s, which can allocate and is not a grower (%s)", sc.at(n), fn.FullName(), w))
		return
	}
	sc.g.calls[sym] = true
	sc.calls = append(sc.calls, growerCall{callee, n})
}

// derive computes the tracked variables that hold only the slice parameter
// or values derived from it, as the greatest set whose every stored value
// is derived with the set.
func (sc *bodyScan) derive() {
	sc.dv = &derivation{gt: sc.gt, info: sc.info, set: sc.set, derived: map[*types.Var]bool{}}
	for v := range sc.tracked {
		sc.dv.derived[v] = !sc.poisoned[v]
	}
	for changed := true; changed; {
		changed = false
		for v, as := range sc.tracked {
			if sc.dv.derived[v] && slices.ContainsFunc(as, func(a assign) bool { return a.rhs == nil || !sc.dv.expr(a.rhs, a.idx) }) {
				sc.dv.derived[v] = false
				changed = true
			}
		}
	}
}

// verify records why the body breaks the shape: a slice parameter that
// holds another value, an append to a value that is not derived, a call of
// a grower with such a value as its slice, or a return of one.
func (sc *bodyScan) verify() {
	name := sc.slice.Name()
	if !sc.dv.derived[sc.slice] {
		sc.why = append(sc.why, fmt.Sprintf("it stores in %s a value that the gate cannot prove is derived from %s", name, name))
	}
	for _, a := range sc.appends {
		if len(a.Args) == 0 || !sc.dv.expr(a.Args[0], 0) {
			sc.why = append(sc.why, fmt.Sprintf("%s: it appends to a slice that the gate cannot prove is %s or derived from it", sc.at(a), name))
		}
	}
	for _, c := range sc.calls {
		if c.g.param >= len(c.call.Args) || !sc.dv.expr(c.call.Args[c.g.param], 0) {
			sc.why = append(sc.why, fmt.Sprintf("%s: it passes %s a slice that the gate cannot prove is %s or derived from it", sc.at(c.call), c.g.d.fn.Name(), name))
		}
	}
	sig := sc.g.d.fn.Signature()
	for _, r := range sc.returns {
		var ok bool
		switch {
		case len(r.Results) == 0:
			ok = sc.dv.derived[sig.Results().At(sc.g.result)]
		case len(r.Results) == 1 && sig.Results().Len() > 1:
			ok = sc.dv.expr(r.Results[0], sc.g.result)
		default:
			ok = sc.dv.expr(r.Results[sc.g.result], 0)
		}
		if !ok {
			sc.why = append(sc.why, fmt.Sprintf("%s: it returns a slice that the gate cannot prove is %s or derived from it", sc.at(r), name))
		}
	}
}

// mayAllocate reports whether the machine code of fn, or of an
// instantiation of a generic fn, can reach an allocator. It reports true for
// a generic fn without instantiations, since the gate cannot see its code.
func (gt *gate) mayAllocate(fn *types.Func) bool {
	if sym := funcSym(fn); sym != "" {
		return gt.allocates(sym)
	}
	re := genericSymRE(fn)
	found := false
	for name := range gt.g.bodies {
		if re.MatchString(name) {
			found = true
			if gt.allocates(name) {
				return true
			}
		}
	}
	return !found
}

func isFuncLit(e ast.Expr) bool {
	_, ok := ast.Unparen(e).(*ast.FuncLit)
	return ok
}

// isBuiltin reports whether fun is the builtin function name.
func isBuiltin(info *types.Info, fun ast.Expr, name string) bool {
	id, ok := ast.Unparen(fun).(*ast.Ident)
	if !ok {
		return false
	}
	b, ok := info.Uses[id].(*types.Builtin)
	return ok && b.Name() == name
}

// calleeFunc returns the function or concrete method that the call of fun
// calls, or nil for a func value, an interface method or a method
// expression.
func calleeFunc(info *types.Info, fun ast.Expr) *types.Func {
	switch e := ast.Unparen(fun).(type) {
	case *ast.IndexExpr:
		return calleeFunc(info, e.X)
	case *ast.IndexListExpr:
		return calleeFunc(info, e.X)
	case *ast.Ident:
		fn, _ := info.Uses[e].(*types.Func)
		return fn
	case *ast.SelectorExpr:
		if s := info.Selections[e]; s != nil {
			fn, ok := s.Obj().(*types.Func)
			if !ok || s.Kind() != types.MethodVal || types.IsInterface(fn.Signature().Recv().Type()) {
				return nil
			}
			return fn
		}
		fn, _ := info.Uses[e.Sel].(*types.Func)
		return fn
	}
	return nil
}

// growsOnlyDst reports whether the machine code of the grower sym can
// allocate only by growing its slice parameter, and otherwise why. Every
// function on a path from sym to an allocator must be a grower that the
// one before it calls by a static call, directly or through growers that
// the compiler inlined into it, and a path must end in a runtime function
// of append in a grower whose body, or the body of a grower that it calls,
// appends. The source checks of growerCheck then prove that each such
// append grows the slice of sym.
func (gt *gate) growsOnlyDst(sym string) (bool, string) {
	if !gt.growsOnly(sym) {
		return false, "it allocates outside slice growth:" + gt.chain(gt.noGrow, sym)
	}
	if gt.grow == nil {
		gt.grow, gt.notGrower = gt.growers()
	}
	if gt.grow[sym] == nil {
		return false, "the gate cannot prove that it grows only its slice parameter: " + gt.notGrower[sym]
	}
	closure := func(from string) map[string]bool {
		seen := map[string]bool{from: true}
		q := []string{from}
		for len(q) > 0 {
			n := q[len(q)-1]
			q = q[:len(q)-1]
			for c := range gt.grow[n].calls {
				if !seen[c] {
					seen[c] = true
					q = append(q, c)
				}
			}
		}
		return seen
	}
	seen := map[string]bool{sym: true}
	q := []string{sym}
	for len(q) > 0 {
		x := q[len(q)-1]
		q = q[:len(q)-1]
		inl := closure(x)
		for _, y := range slices.Sorted(maps.Keys(gt.g.edges[x])) {
			if !gt.allocates(y) {
				continue
			}
			switch {
			case gt.growth[y]:
				if !slices.ContainsFunc(slices.Collect(maps.Keys(inl)), func(s string) bool { return gt.grow[s].appends }) {
					return false, fmt.Sprintf("%s calls %s, but neither it nor a grower that it calls appends, so the growth comes from code that the gate did not check", x, y)
				}
			case gt.grow[y] != nil && inl[y]:
				if !seen[y] {
					seen[y] = true
					q = append(q, y)
				}
			case gt.grow[y] != nil:
				return false, fmt.Sprintf("%s calls the grower %s other than by a static call in its source, so the gate cannot prove what slice it passes", x, y)
			default:
				w := gt.notGrower[y]
				if w == "" {
					w = "it does not take and return a slice"
				}
				return false, fmt.Sprintf("%s calls %s, which can grow a slice that the gate cannot prove is dst (%s):%s", x, y, w, gt.chain(gt.next, y))
			}
		}
	}
	return true, ""
}

// growsDst reports whether the root r is a grower whose slice parameter is
// named dst and that can allocate only by growing it, and why not when it
// has the shape but breaks the rule.
func (gt *gate) growsDst(r root) (bool, string) {
	if r.decl == nil {
		return false, ""
	}
	param, _, ok := growerShape(r.decl.fn)
	if !ok {
		return false, ""
	}
	if name := r.decl.fn.Signature().Params().At(param).Name(); name != "dst" {
		return false, fmt.Sprintf("it takes and returns a slice, but names the slice %s, not dst", name)
	}
	return gt.growsOnlyDst(r.sym)
}
