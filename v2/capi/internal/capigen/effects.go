package capigen

import (
	"go/ast"
	"go/token"
	"go/types"
	"strings"
)

type effKey struct {
	fn    *types.Func
	param int
}

const (
	eSet uint8 = 1 << iota
	eElem
)

type effect struct {
	all bool
	ret bool
	f   map[string]uint8
}

func (e *effect) mark(field string, bits uint8) {
	if bits == 0 {
		return
	}
	if e.f == nil {
		e.f = map[string]uint8{}
	}
	e.f[field] |= bits
}

func (e *effect) get(field string) uint8 {
	if e.all {
		return eSet | eElem
	}
	return e.f[field]
}

func (e *effect) bits() uint8 {
	if e.all {
		return eSet | eElem
	}
	var b uint8
	for _, v := range e.f {
		b |= v
	}
	return b
}

func (e *effect) any() bool {
	return e.all || len(e.f) > 0
}

type apath struct {
	field  string
	deeper bool
	copy   bool
	box    bool
}

type analyzer struct {
	l       *loader
	info    *types.Info
	eff     *effect
	root    types.Object
	aliases map[types.Object]apath
	changed bool
	parent  map[ast.Node]ast.Node
	resVars map[types.Object]bool
}

// effects reports what fn can change, as seen by its caller, through its
// parameter param (-1 for the receiver): for a struct or a pointer to one,
// the fields that it sets (eSet) or whose elements it writes in place
// (eElem); for a slice or another pointer, eElem under "" when it writes
// through it. A body that the analysis cannot read changes everything.
func (l *loader) effects(fn *types.Func, param int) *effect {
	fn = fn.Origin()
	k := effKey{fn, param}
	if e, ok := l.eff[k]; ok {
		return e
	}
	all := &effect{all: true, ret: true}
	l.eff[k] = all
	fd := l.decl[fn]
	if fd == nil {
		return all
	}
	info := l.info[fn.Pkg()]
	id := paramIdent(fd, param)
	if id == nil && param >= 0 && fn.Signature().Variadic() && param >= fn.Signature().Params().Len() {
		return l.effects(fn, fn.Signature().Params().Len()-1)
	}
	if id == nil || id.Name == "_" {
		l.eff[k] = &effect{}
		return l.eff[k]
	}
	obj := info.Defs[id]
	if obj == nil {
		return all
	}
	a := &analyzer{l: l, info: info, eff: &effect{}, root: obj, aliases: map[types.Object]apath{obj: {}}, parent: map[ast.Node]ast.Node{}, resVars: map[types.Object]bool{}}
	if fd.Type.Results != nil {
		for _, f := range fd.Type.Results.List {
			for _, n := range f.Names {
				if o := info.Defs[n]; o != nil {
					a.resVars[o] = true
				}
			}
		}
	}
	ast.Inspect(fd.Body, func(n ast.Node) bool {
		if n == nil {
			return false
		}
		ast.Inspect(n, func(c ast.Node) bool {
			if c == n {
				return true
			}
			if c != nil {
				a.parent[c] = n
			}
			return false
		})
		return true
	})
	for {
		a.changed = false
		a.eff = &effect{}
		ast.Inspect(fd.Body, func(n ast.Node) bool {
			if id, ok := n.(*ast.Ident); ok {
				if o := info.Uses[id]; o != nil {
					if p, ok := a.aliases[o]; ok {
						a.use(id, p)
					}
				}
			}
			return !a.eff.all
		})
		if !a.changed || a.eff.all {
			break
		}
	}
	l.eff[k] = a.eff
	return a.eff
}

// overwrites reports whether fn replaces all of *param (-1 for the
// receiver) before it reads it: after its nil checks, its first statement
// that uses param is *param = v, and v does not use param.
func (l *loader) overwrites(fn *types.Func, param int) bool {
	fn = fn.Origin()
	fd := l.decl[fn]
	if fd == nil {
		return false
	}
	info := l.info[fn.Pkg()]
	id := paramIdent(fd, param)
	if id == nil {
		return false
	}
	obj := info.Defs[id]
	if obj == nil {
		return false
	}
	uses := func(n ast.Node) bool {
		found := false
		ast.Inspect(n, func(x ast.Node) bool {
			if i, ok := x.(*ast.Ident); ok && info.Uses[i] == obj {
				found = true
			}
			return !found
		})
		return found
	}
	for _, st := range fd.Body.List {
		if !uses(st) {
			continue
		}
		if ifs, ok := st.(*ast.IfStmt); ok && ifs.Init == nil && ifs.Else == nil && nilCheck(ifs.Cond, obj, info) && len(ifs.Body.List) > 0 {
			if _, ok := ifs.Body.List[len(ifs.Body.List)-1].(*ast.ReturnStmt); ok && !uses(ifs.Body) {
				continue
			}
		}
		as, ok := st.(*ast.AssignStmt)
		if !ok || len(as.Lhs) != 1 || len(as.Rhs) != 1 || as.Tok != token.ASSIGN {
			return false
		}
		star, ok := ast.Unparen(as.Lhs[0]).(*ast.StarExpr)
		if !ok {
			return false
		}
		if x, ok := ast.Unparen(star.X).(*ast.Ident); !ok || info.Uses[x] != obj {
			return false
		}
		return !uses(as.Rhs[0])
	}
	return false
}

func nilCheck(e ast.Expr, obj types.Object, info *types.Info) bool {
	if x, ok := ast.Unparen(e).(*ast.BinaryExpr); ok {
		switch x.Op {
		case token.LOR, token.LAND:
			return nilCheck(x.X, obj, info) && nilCheck(x.Y, obj, info)
		case token.EQL, token.NEQ:
			l, lok := ast.Unparen(x.X).(*ast.Ident)
			r, rok := ast.Unparen(x.Y).(*ast.Ident)
			if lok && rok && (info.Uses[l] == obj && r.Name == "nil" || info.Uses[r] == obj && l.Name == "nil") {
				return true
			}
		default:
		}
	}
	found := false
	ast.Inspect(e, func(n ast.Node) bool {
		if i, ok := n.(*ast.Ident); ok && info.Uses[i] == obj {
			found = true
		}
		return !found
	})
	return !found
}

func paramIdent(fd *ast.FuncDecl, param int) *ast.Ident {
	if param < 0 {
		if fd.Recv == nil || len(fd.Recv.List) == 0 || len(fd.Recv.List[0].Names) == 0 {
			return nil
		}
		return fd.Recv.List[0].Names[0]
	}
	i := 0
	for _, f := range fd.Type.Params.List {
		if len(f.Names) == 0 {
			i++
			continue
		}
		for _, n := range f.Names {
			if i == param {
				return n
			}
			i++
		}
	}
	return nil
}

func isRef(t types.Type) bool {
	switch u := t.Underlying().(type) {
	case *types.Slice, *types.Pointer, *types.Map, *types.Chan, *types.Signature, *types.Interface:
		return true
	case *types.Struct:
		for f := range u.Fields() {
			if isRef(f.Type()) {
				return true
			}
		}
	case *types.Array:
		return isRef(u.Elem())
	case *types.Tuple:
		for v := range u.Variables() {
			if isRef(v.Type()) {
				return true
			}
		}
	}
	return false
}

func isPtr(t types.Type) bool {
	_, ok := t.Underlying().(*types.Pointer)
	return ok
}

func (a *analyzer) typeOf(e ast.Expr) types.Type {
	if tv, ok := a.info.Types[e]; ok {
		return tv.Type
	}
	if id, ok := e.(*ast.Ident); ok {
		if o := a.info.Uses[id]; o != nil {
			return o.Type()
		}
		if o := a.info.Defs[id]; o != nil {
			return o.Type()
		}
	}
	return nil
}

func (a *analyzer) isType(e ast.Expr) bool {
	tv, ok := a.info.Types[e]
	return ok && tv.IsType()
}

func structOf(t types.Type) *types.Struct {
	if t == nil {
		return nil
	}
	if p, ok := t.Underlying().(*types.Pointer); ok {
		t = p.Elem()
	}
	s, _ := t.Underlying().(*types.Struct)
	return s
}

func (a *analyzer) rootStruct() *types.Struct {
	return structOf(a.root.Type())
}

func (a *analyzer) use(id *ast.Ident, p apath) {
	var top ast.Expr = id
	structRoot := p.field == "" && !p.deeper && !p.box && structOf(a.info.Uses[id].Type()) != nil
	for top != nil {
		for {
			next, climbed, stop := a.step(top, &p, &structRoot)
			if stop {
				return
			}
			if !climbed {
				break
			}
			top = next
		}
		next := a.context(top, id, p)
		if next == nil {
			return
		}
		top = next
		p.deeper = true
		structRoot = false
	}
}

// step climbs from top to its parent when the parent still names the
// same memory, such as a field, an index or a conversion of top, and
// updates p. stop is set when a method call ends the use.
func (a *analyzer) step(top ast.Expr, p *apath, structRoot *bool) (ast.Expr, bool, bool) {
	switch x := a.parent[top].(type) {
	case *ast.ParenExpr:
		return x, true, false
	case *ast.SelectorExpr:
		if x.X != top {
			return nil, false, false
		}
		sel := a.info.Selections[x]
		if sel == nil {
			return nil, false, false
		}
		if sel.Kind() == types.FieldVal {
			if *structRoot {
				p.field = a.topField(top, sel, x.Sel.Name)
				p.deeper = p.deeper || len(sel.Index()) > 1
			} else {
				p.deeper = true
			}
			*structRoot = false
			return x, true, false
		}
		if *structRoot && len(sel.Index()) > 1 {
			p.field = a.topField(top, sel, x.Sel.Name)
		}
		next := a.methodCall(x, sel, *p)
		if next == nil {
			return nil, false, true
		}
		p.deeper = true
		*structRoot = false
		return next, true, false
	case *ast.IndexExpr:
		if x.X == top {
			p.deeper = true
			*structRoot = false
			return x, true, false
		}
	case *ast.SliceExpr:
		if x.X == top {
			return x, true, false
		}
	case *ast.StarExpr:
		return x, true, false
	case *ast.TypeAssertExpr:
		if x.X == top {
			return x, true, false
		}
	case *ast.UnaryExpr:
		if x.Op == token.AND && x.X == top {
			return x, true, false
		}
	case *ast.CallExpr:
		if len(x.Args) == 1 && x.Args[0] == top && a.isType(x.Fun) {
			*structRoot = false
			return x, true, false
		}
	case *ast.KeyValueExpr, *ast.CompositeLit:
		return a.litStep(top, p, structRoot)
	}
	return nil, false, false
}

// litStep climbs from a reference in a composite literal to the literal,
// which then holds the reference as a box.
func (a *analyzer) litStep(top ast.Expr, p *apath, structRoot *bool) (ast.Expr, bool, bool) {
	t := a.typeOf(top)
	switch x := a.parent[top].(type) {
	case *ast.KeyValueExpr:
		if x.Value == top && t != nil && isRef(t) {
			return x, true, false
		}
	case *ast.CompositeLit:
		if _, kv := top.(*ast.KeyValueExpr); !kv && (t == nil || !isRef(t)) {
			return nil, false, false
		}
		p.copy, p.box = true, true
		*structRoot = false
		return x, true, false
	}
	return nil, false, false
}

// topField is the field of the struct of top through which sel goes: the
// field itself, or the embedded field that holds a promoted field or
// method.
func (a *analyzer) topField(top ast.Expr, sel *types.Selection, name string) string {
	if len(sel.Index()) < 2 {
		return name
	}
	if s := structOf(a.typeOf(top)); s != nil && sel.Index()[0] < s.NumFields() {
		return s.Field(sel.Index()[0]).Name()
	}
	return name
}

// refArg reports whether the argument arg, which a copy alias of id
// starts, reaches memory that the copy shares with the original: a slice
// or pointer that the copy holds, not the copy itself.
func (a *analyzer) refArg(arg ast.Expr, id *ast.Ident, box bool) bool {
	t := a.typeOf(arg)
	if t == nil {
		return true
	}
	switch x := ast.Unparen(arg).(type) {
	case *ast.SliceExpr:
		if xt := a.typeOf(x.X); xt != nil {
			if _, ok := xt.Underlying().(*types.Array); ok {
				return a.throughRef(x.X, id, box)
			}
		}
	case *ast.UnaryExpr:
		if xt := a.typeOf(x.X); x.Op == token.AND && xt != nil && !isRef(xt) {
			return a.throughRef(x.X, id, box)
		}
	}
	return isRef(t) || a.throughRef(arg, id, box)
}

func (a *analyzer) throughRef(e ast.Expr, id *ast.Ident, box bool) bool {
	for e != nil && e != ast.Expr(id) {
		switch x := e.(type) {
		case *ast.ParenExpr:
			e = x.X
		case *ast.SelectorExpr:
			if t := a.typeOf(x.X); t != nil && isPtr(t) && (!box || ast.Unparen(x.X) != ast.Expr(id)) {
				return true
			}
			e = x.X
		case *ast.IndexExpr:
			if t := a.typeOf(x.X); t != nil {
				switch t.Underlying().(type) {
				case *types.Slice, *types.Map, *types.Pointer:
					return true
				}
			}
			e = x.X
		case *ast.SliceExpr:
			if t := a.typeOf(x.X); t != nil {
				if _, ok := t.Underlying().(*types.Slice); ok {
					return true
				}
			}
			e = x.X
		case *ast.StarExpr:
			if box && ast.Unparen(x.X) == ast.Expr(id) {
				e = x.X
				continue
			}
			return true
		case *ast.UnaryExpr:
			e = x.X
		default:
			return true
		}
	}
	return false
}

func (a *analyzer) write(p apath, lhs ast.Expr, id *ast.Ident) {
	if p.copy {
		if lhs != nil && !a.throughRef(lhs, id, p.box) {
			return
		}
		if p.field == "" && a.rootStruct() != nil {
			a.sharedRefs()
			return
		}
		a.eff.mark(p.field, eElem)
		return
	}
	if lhs != nil && ast.Unparen(lhs) == ast.Expr(id) {
		return
	}
	if p.field == "" {
		if a.rootStruct() != nil {
			a.eff.all = true
			return
		}
		a.eff.mark("", eElem)
		return
	}
	if !p.deeper {
		a.eff.mark(p.field, eSet)
		return
	}
	if a.fieldIsSlice(p.field) {
		a.eff.mark(p.field, eElem)
		return
	}
	a.eff.mark(p.field, eSet)
}

func (a *analyzer) fieldIsSlice(name string) bool {
	s := a.rootStruct()
	if s == nil {
		return false
	}
	for f := range s.Fields() {
		if f.Name() == name {
			_, ok := f.Type().Underlying().(*types.Slice)
			return ok
		}
	}
	return false
}

// sharedRefs marks what a copy of the root struct shares with the
// original: the elements of its reference fields.
func (a *analyzer) sharedRefs() {
	for f := range a.rootStruct().Fields() {
		if isRef(f.Type()) {
			a.eff.mark(f.Name(), eElem)
		}
	}
}

func (a *analyzer) escape(p apath) {
	if p.field == "" {
		if a.rootStruct() != nil && p.copy {
			a.sharedRefs()
			return
		}
		if a.rootStruct() != nil {
			a.eff.all = true
			return
		}
		a.eff.mark("", eElem)
		return
	}
	if p.copy {
		a.eff.mark(p.field, eElem)
		return
	}
	a.eff.mark(p.field, eSet|eElem)
}

func (a *analyzer) localRoot(e ast.Expr) (*types.Var, bool) {
	direct := true
	for {
		switch x := e.(type) {
		case *ast.ParenExpr:
			e = x.X
			continue
		case *ast.SelectorExpr:
			if t := a.typeOf(x.X); t == nil || isPtr(t) {
				return nil, false
			}
			e, direct = x.X, false
			continue
		case *ast.IndexExpr:
			if t := a.typeOf(x.X); t == nil {
				return nil, false
			} else if _, ok := t.Underlying().(*types.Array); !ok {
				return nil, false
			}
			e, direct = x.X, false
			continue
		case *ast.Ident:
			if x.Name == "_" {
				return nil, false
			}
			o := a.info.Defs[x]
			if o == nil {
				o = a.info.Uses[x]
			}
			v, ok := o.(*types.Var)
			if !ok || v.Parent() == nil || v.Pkg() == nil || v.Parent() == v.Pkg().Scope() {
				return nil, false
			}
			return v, direct
		}
		return nil, false
	}
}

func (a *analyzer) alias(lhs, rhs ast.Expr, p apath) {
	t := a.typeOf(rhs)
	if t == nil || !isRef(t) {
		return
	}
	v, direct := a.localRoot(lhs)
	if v == nil {
		return
	}
	if a.resVars[v] {
		a.eff.ret = true
	}
	if !direct {
		p.copy, p.box = true, true
	} else {
		switch v.Type().Underlying().(type) {
		case *types.Struct, *types.Array:
			if p.field == "" && !p.deeper && !p.box && types.Identical(v.Type(), derefType(a.root.Type())) {
				p.copy = true
			} else {
				p.copy, p.box = true, true
			}
		}
	}
	a.addAlias(v, p)
}

func (a *analyzer) addAlias(v types.Object, p apath) {
	old, ok := a.aliases[v]
	if !ok {
		a.aliases[v] = p
		a.changed = true
		return
	}
	if old == p {
		return
	}
	m := apath{deeper: old.deeper || p.deeper, copy: old.copy && p.copy, box: old.box && p.box}
	if old.field == p.field {
		m.field = old.field
	}
	if m != old {
		a.aliases[v] = m
		a.changed = true
	}
}

func (a *analyzer) switchAlias(ts *ast.TypeSwitchStmt, p apath) {
	for _, c := range ts.Body.List {
		if v, ok := a.info.Implicits[c].(*types.Var); ok && isRef(v.Type()) {
			q := p
			switch v.Type().Underlying().(type) {
			case *types.Struct, *types.Array:
				q.copy, q.box = true, true
			}
			a.addAlias(v, q)
		}
	}
}

func derefType(t types.Type) types.Type {
	if p, ok := t.Underlying().(*types.Pointer); ok {
		return p.Elem()
	}
	return t
}

func (a *analyzer) context(top ast.Expr, id *ast.Ident, p apath) ast.Expr {
	for n := a.parent[top]; n != nil; n = a.parent[n] {
		if _, ok := n.(*ast.FuncLit); ok {
			a.escape(p)
			return nil
		}
	}
	switch x := a.parent[top].(type) {
	case *ast.AssignStmt:
		for _, l := range x.Lhs {
			if l == top {
				a.write(p, top, id)
				return nil
			}
		}
		for i, r := range x.Rhs {
			if r != top {
				continue
			}
			if len(x.Lhs) == len(x.Rhs) {
				if ts, ok := a.parent[x].(*ast.TypeSwitchStmt); ok && ts.Assign == x {
					a.switchAlias(ts, p)
					return nil
				}
				a.alias(x.Lhs[i], top, p)
				return nil
			}
			if len(x.Rhs) == 1 && len(x.Lhs) == 2 {
				a.alias(x.Lhs[0], top, p)
				return nil
			}
		}
	case *ast.IncDecStmt:
		a.write(p, top, id)
	case *ast.ValueSpec:
		for i, v := range x.Values {
			if v == top && i < len(x.Names) {
				a.alias(x.Names[i], top, p)
			}
		}
	case *ast.RangeStmt:
		if x.X == top {
			if x.Value != nil {
				q := p
				q.deeper = true
				a.alias(x.Value, x.Value, q)
			}
			return nil
		}
		if x.Tok == token.ASSIGN {
			a.write(p, top, id)
		}
	case *ast.CallExpr:
		for i, arg := range x.Args {
			if arg != top {
				continue
			}
			if a.argCall(x, i, p, id) {
				return a.results(x, p)
			}
			return nil
		}
	case *ast.ReturnStmt:
		a.eff.ret = true
	case *ast.SendStmt:
		if x.Value == top {
			a.escape(p)
		}
	}
	return nil
}

func (a *analyzer) results(call *ast.CallExpr, p apath) ast.Expr {
	t := a.typeOf(call)
	if t == nil || !isRef(t) {
		return nil
	}
	if tu, ok := t.(*types.Tuple); ok {
		q := p
		q.deeper = true
		switch x := a.parent[call].(type) {
		case *ast.AssignStmt:
			if len(x.Rhs) == 1 {
				for i, l := range x.Lhs {
					if i < tu.Len() && isRef(tu.At(i).Type()) {
						a.alias(l, l, q)
					}
				}
			}
		case *ast.ValueSpec:
			if len(x.Values) == 1 {
				for i, l := range x.Names {
					if i < tu.Len() && isRef(tu.At(i).Type()) {
						a.alias(l, l, q)
					}
				}
			}
		}
		return nil
	}
	return call
}

func (a *analyzer) callee(call *ast.CallExpr) (*types.Func, *types.Builtin) {
	switch f := ast.Unparen(call.Fun).(type) {
	case *ast.Ident:
		switch o := a.info.Uses[f].(type) {
		case *types.Func:
			return o, nil
		case *types.Builtin:
			return nil, o
		}
	case *ast.SelectorExpr:
		if sel := a.info.Selections[f]; sel != nil {
			if fn, ok := sel.Obj().(*types.Func); ok {
				return fn, nil
			}
			return nil, nil
		}
		if fn, ok := a.info.Uses[f.Sel].(*types.Func); ok {
			return fn, nil
		}
	case *ast.IndexExpr:
		if id, ok := f.X.(*ast.Ident); ok {
			if fn, ok := a.info.Uses[id].(*types.Func); ok {
				return fn, nil
			}
		}
	}
	return nil, nil
}

func (a *analyzer) argCall(call *ast.CallExpr, i int, p apath, id *ast.Ident) bool {
	if t := a.typeOf(call.Args[i]); t != nil && !isRef(t) {
		return false
	}
	if p.copy && id != nil && !a.refArg(call.Args[i], id, p.box) {
		return false
	}
	fn, b := a.callee(call)
	if b != nil {
		switch b.Name() {
		case "copy", "append", "clear", "delete":
			if i == 0 {
				q := p
				q.deeper = true
				a.write(q, nil, nil)
			}
		}
		return b.Name() == "append" && i == 0
	}
	if fn == nil {
		a.escape(p)
		return true
	}
	if sel, ok := ast.Unparen(call.Fun).(*ast.SelectorExpr); ok {
		if s := a.info.Selections[sel]; s != nil && types.IsInterface(s.Recv()) {
			a.escape(p)
			return true
		}
	}
	params := fn.Signature().Params()
	pt := params.At(min(i, params.Len()-1)).Type()
	if a.l.decl[fn.Origin()] != nil {
		e := a.l.effects(fn, i)
		a.apply(e, p, pt)
		return e.ret
	}
	if stdWrites(fn, i) {
		a.escape(p)
		return true
	}
	return i == 0
}

func (a *analyzer) methodCall(x *ast.SelectorExpr, sel *types.Selection, p apath) ast.Expr {
	fn, ok := sel.Obj().(*types.Func)
	call, isCall := a.parent[x].(*ast.CallExpr)
	if !ok || !isCall || call.Fun != x {
		a.escape(p)
		return nil
	}
	rt := fn.Signature().Recv().Type()
	ret := true
	switch {
	case types.IsInterface(sel.Recv()):
		a.escape(p)
	case !isRef(rt):
		ret = false
	case a.l.decl[fn.Origin()] != nil:
		e := a.l.effects(fn, -1)
		a.apply(e, p, rt)
		ret = e.ret
	case stdWrites(fn, -1):
		a.escape(p)
	}
	if !ret {
		return nil
	}
	return a.results(call, p)
}

func (a *analyzer) apply(e *effect, p apath, pt types.Type) {
	if !e.any() {
		return
	}
	byRef := isPtr(pt)
	if _, ok := pt.Underlying().(*types.Slice); ok {
		if e.get("")&eElem != 0 {
			q := p
			q.deeper = true
			a.write(q, nil, nil)
		}
		return
	}
	mask := eSet | eElem
	if !byRef || p.copy {
		mask = eElem
	}
	if p.field == "" && !p.deeper && a.rootStruct() != nil && structOf(pt) != nil {
		if e.all {
			if mask == eSet|eElem {
				a.eff.all = true
				return
			}
			a.sharedRefs()
			return
		}
		for f, bits := range e.f {
			if bits&mask == 0 {
				continue
			}
			if p.copy {
				a.eff.mark(f, eElem)
			} else {
				a.eff.mark(f, bits&mask)
			}
		}
		return
	}
	if e.bits()&mask != 0 {
		q := p
		q.deeper = true
		a.write(q, nil, nil)
	}
}

func stdWrites(fn *types.Func, i int) bool {
	pkg := ""
	if fn.Pkg() != nil {
		pkg = fn.Pkg().Path()
	}
	name := fn.Name()
	recv := fn.Signature().Recv() != nil
	switch pkg {
	case "fmt":
		return strings.Contains(name, "scan") || strings.Contains(name, "Scan") || strings.HasPrefix(name, "Append") && i == 0
	case "errors":
		return name == "As" && i == 1
	case "strings", "bytes":
		if recv && i == -1 {
			switch name {
			case "Len", "Cap", methString, "Bytes", "Available", "AvailableBuffer", "Size":
				return false
			}
			return true
		}
		if recv {
			return strings.HasPrefix(name, "Read") && i == 0
		}
		return false
	case "math", "math/bits", "unicode", "hash/crc32", "hash/crc64", "hash/adler32", "time", "unicode/utf16":
		if recv {
			return i == -1 && name != methString || strings.HasPrefix(name, "Read") && i == 0
		}
		return false
	case "strconv":
		return strings.HasPrefix(name, "Append") && i == 0
	case "unicode/utf8":
		return (name == "EncodeRune" || name == "AppendRune") && i == 0
	case "encoding/binary":
		if name == "Read" {
			return true
		}
		return (strings.HasPrefix(name, "Put") || strings.HasPrefix(name, "Append")) && i == 0
	case "slices":
		switch name {
		case "Equal", "EqualFunc", "Compare", "CompareFunc", "Index", "IndexFunc", "Contains", "ContainsFunc", "Clone", "Max", "MaxFunc",
			"Min", "MinFunc", "IsSorted", "IsSortedFunc", "BinarySearch", "BinarySearchFunc", "All", "Values", "Backward":
			return false
		}
		return i == 0
	}
	return true
}
