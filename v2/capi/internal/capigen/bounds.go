package capigen

import (
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"regexp"
	"strings"
)

type bctx struct {
	pkg  *types.Package
	recv *tinfo
	args map[string]string
	cdef bool
}

func (g *gen) translate(expr string, c bctx) (string, string, error) {
	e, err := parser.ParseExpr(expr)
	if err != nil {
		return "", "", fmt.Errorf("bound %q: %w", expr, err)
	}
	c.cdef = true
	var cw strings.Builder
	goExpr, err := g.texpr(e, &c, &cw)
	if err != nil {
		return "", "", fmt.Errorf("bound %q: %w", expr, err)
	}
	if !c.cdef {
		return goExpr, "", nil
	}
	return goExpr, cw.String(), nil
}

func (g *gen) constRef(pkg *types.Package, name string, cw *strings.Builder) (string, bool) {
	k, ok := pkg.Scope().Lookup(name).(*types.Const)
	if !ok || !k.Exported() {
		return "", false
	}
	cw.WriteString(strings.ToUpper(g.prefix(pkg) + snake(name)))
	return "int(" + g.qual(pkg) + "." + name + ")", true
}

func (g *gen) texpr(e ast.Expr, c *bctx, cw *strings.Builder) (string, error) {
	switch e := e.(type) {
	case *ast.BasicLit:
		if e.Kind != token.INT {
			return "", errors.New("only integer literals")
		}
		cw.WriteString(e.Value)
		return e.Value, nil
	case *ast.ParenExpr:
		cw.WriteString("(")
		x, err := g.texpr(e.X, c, cw)
		cw.WriteString(")")
		return "(" + x + ")", err
	case *ast.BinaryExpr:
		switch e.Op {
		case token.ADD, token.SUB, token.MUL, token.QUO:
		default:
			return "", fmt.Errorf("operator %s", e.Op)
		}
		cw.WriteString("(")
		x, err := g.texpr(e.X, c, cw)
		if err != nil {
			return "", err
		}
		cw.WriteString(" " + e.Op.String() + " ")
		y, err := g.texpr(e.Y, c, cw)
		cw.WriteString(")")
		return "(" + x + " " + e.Op.String() + " " + y + ")", err
	case *ast.Ident:
		if a, ok := c.args[e.Name]; ok {
			c.cdef = false
			return "int(" + a + ")", nil
		}
		if c.recv != nil && g.recvMethod(c.recv, e.Name) {
			c.cdef = false
			return "int(h.v." + e.Name + "())", nil
		}
		if s, ok := g.constRef(c.pkg, e.Name, cw); ok {
			return s, nil
		}
		return "", fmt.Errorf("unknown name %s", e.Name)
	case *ast.SelectorExpr:
		x, ok := e.X.(*ast.Ident)
		if !ok {
			return "", errors.New("selector")
		}
		for _, p := range g.pkgs {
			if p.Name() == x.Name {
				if s, ok := g.constRef(p, e.Sel.Name, cw); ok {
					return s, nil
				}
			}
		}
		return "", fmt.Errorf("unknown constant %s.%s", x.Name, e.Sel.Name)
	case *ast.CallExpr:
		return g.tcall(e, c, cw)
	default:
		return "", fmt.Errorf("expression %T", e)
	}
}

func (g *gen) tcall(e *ast.CallExpr, c *bctx, cw *strings.Builder) (string, error) {
	if sel, ok := e.Fun.(*ast.SelectorExpr); ok {
		return g.tmethod(sel, e.Args, c, cw)
	}
	f, ok := e.Fun.(*ast.Ident)
	if !ok {
		return "", errors.New("call of a non-name")
	}
	c.cdef = false
	switch f.Name {
	case "len":
		if len(e.Args) != 1 {
			return "", errors.New("len takes one argument")
		}
		a, ok := e.Args[0].(*ast.Ident)
		if !ok || c.args[a.Name] == "" {
			return "", errors.New("len of a non-parameter")
		}
		return "len(" + c.args[a.Name] + ")", nil
	case "min", "max":
		var parts []string
		for _, a := range e.Args {
			x, err := g.texpr(a, c, cw)
			if err != nil {
				return "", err
			}
			parts = append(parts, x)
		}
		return f.Name + "(" + strings.Join(parts, ", ") + ")", nil
	}
	if c.recv == nil || len(e.Args) != 0 {
		return "", fmt.Errorf("call of %s", f.Name)
	}
	if !g.recvMethod(c.recv, f.Name) {
		return "", fmt.Errorf("%s is not a method without arguments", f.Name)
	}
	return "int(h.v." + f.Name + "())", nil
}

func (g *gen) tmethod(sel *ast.SelectorExpr, args []ast.Expr, c *bctx, cw *strings.Builder) (string, error) {
	x, ok := sel.X.(*ast.Ident)
	if !ok || c.recv == nil {
		return "", errors.New("call of a method of a non-receiver")
	}
	for _, p := range g.pkgs {
		if p.Name() == x.Name {
			return "", fmt.Errorf("call of %s.%s", x.Name, sel.Sel.Name)
		}
	}
	if !g.recvIntMethod(c.recv, sel.Sel.Name, len(args)) {
		return "", fmt.Errorf("%s is not a method of %d int arguments and one result", sel.Sel.Name, len(args))
	}
	c.cdef = false
	parts := make([]string, 0, len(args))
	for _, a := range args {
		v, err := g.texpr(a, c, cw)
		if err != nil {
			return "", err
		}
		parts = append(parts, "int("+v+")")
	}
	return "int(h.v." + sel.Sel.Name + "(" + strings.Join(parts, ", ") + "))", nil
}

func (g *gen) recvIntMethod(recv *tinfo, name string, n int) bool {
	o, _, _ := types.LookupFieldOrMethod(types.NewPointer(recv.named), true, recv.obj.Pkg(), name)
	m, ok := o.(*types.Func)
	if !ok || m.Signature().Params().Len() != n || m.Signature().Results().Len() != 1 {
		return false
	}
	for i := range n {
		if b, ok := m.Signature().Params().At(i).Type().(*types.Basic); !ok || b.Kind() != types.Int {
			return false
		}
	}
	return true
}

func (g *gen) recvMethod(recv *tinfo, name string) bool {
	var rt types.Type = types.NewPointer(recv.named)
	if _, ok := recv.named.Underlying().(*types.Interface); ok {
		rt = recv.named
	}
	o, _, _ := types.LookupFieldOrMethod(rt, true, recv.obj.Pkg(), name)
	m, ok := o.(*types.Func)
	return ok && m.Signature().Params().Len() == 0 && m.Signature().Results().Len() == 1
}

// boundPhrase returns the bound that doc states for the one output buffer
// of a call: "room for N more octets" or "returns at most N items", or 0
// for a call that writes into dst and "keeps the length of dst".
func boundPhrase(doc string) (string, bool) {
	f := regexp.MustCompile(`(?:room for|returns at most) (\S+?)(?: more)? [a-z]+`).FindStringSubmatch(doc)
	if f != nil {
		return f[1], true
	}
	if regexp.MustCompile(`keeps the length of \S+`).MatchString(doc) {
		return "0", true
	}
	return "", false
}

func (g *gen) elemArena(t types.Type, doc string, pkg *types.Package) (string, error) {
	switch u := t.(type) {
	case *types.Slice:
		f := regexp.MustCompile(`at most (\S+) [a-z]+`).FindStringSubmatch(doc)
		if f == nil {
			return "", errors.New("a slice field without a bound in its Go doc")
		}
		n, _, err := g.translate(f[1], bctx{pkg: pkg})
		if err != nil {
			return "", err
		}
		e := u.Elem()
		if g.alias(e) {
			return "(" + n + ")*int(unsafe.Sizeof(*new(" + g.goType(e) + "))) + 16", nil
		}
		ce, err := g.cgoElem(e)
		if err != nil {
			return "", err
		}
		sub, err := g.elemArena(e, "", pkg)
		if err != nil {
			return "", err
		}
		return "(" + n + ")*(int(unsafe.Sizeof(" + ce + "{})) + 16 + " + sub + ")", nil
	case *types.Basic:
		if u.Kind() == types.String {
			f := regexp.MustCompile(`at most (\S+) bytes`).FindStringSubmatch(doc)
			if f == nil {
				return "", errors.New("a string without a bound in its Go doc")
			}
			n, _, err := g.translate(f[1], bctx{pkg: pkg})
			return "(" + n + ") + 1", err
		}
		return "0", nil
	case *types.Array:
		n, err := g.elemArena(u.Elem(), doc, pkg)
		return fmt.Sprintf("%d*(%s)", u.Len(), n), err
	case *types.Pointer:
		if _, isH := g.handleC(u.Elem()); isH {
			return "0", nil
		}
		return g.elemArena(u.Elem(), doc, pkg)
	case *types.Named:
		return g.namedArena(u)
	default:
		return "0", nil
	}
}

func (g *gen) namedArena(u *types.Named) (string, error) {
	ti := g.info(u)
	if ti == nil || !g.needsArena(u) {
		return "0", nil
	}
	var parts []string
	switch ti.kind {
	case kValue:
		for f := range underStruct(u).Fields() {
			if !f.Exported() || !g.needsArena(f.Type()) {
				continue
			}
			n, err := g.elemArena(f.Type(), g.l.full[f], f.Pkg())
			if err != nil {
				return "", fmt.Errorf("%s.%s: %w", ti.obj.Name(), f.Name(), err)
			}
			parts = append(parts, n)
		}
		return strings.Join(parts, " + "), nil
	case kClosed:
		for _, im := range ti.impls {
			n, err := g.namedArena(im)
			if err != nil {
				return "", err
			}
			parts = append(parts, n)
		}
		return "max(" + strings.Join(parts, ", ") + ")", nil
	default:
		return "", fmt.Errorf("%s in the arena", ti.obj.Name())
	}
}

type needs struct {
	items, arena string
	itemsC       string
}

func (g *gen) callNeeds(sm *sigmap, sig *types.Signature) (needs, error) {
	var n needs
	m, recv := g.curMethod, g.curRecv
	args := map[string]string{}
	params := sig.Params()
	for i := 0; i < params.Len() && i < len(sm.args); i++ {
		args[params.At(i).Name()] = strings.TrimSuffix(sm.args[i], "...")
	}
	if len(sm.slices) > 1 {
		return n, errors.New("it has more than one output buffer")
	}
	if len(sm.slices) == 1 {
		phrase, ok := boundPhrase(g.l.full[m])
		if !ok {
			return n, errors.New("its Go doc gives no bound for one call")
		}
		goExpr, cExpr, err := g.translate(phrase, bctx{pkg: m.Pkg(), recv: recv, args: args})
		if err != nil {
			return n, err
		}
		n.items, n.itemsC = "max(0, "+goExpr+")", cExpr
	}
	var parts []string
	if sm.arena {
		if len(sm.slices) == 1 {
			per, err := g.elemArena(sm.slices[0].e, "", m.Pkg())
			if err != nil {
				return n, err
			}
			if per != "0" {
				parts = append(parts, n.items+"*("+per+")")
			}
		}
		for _, t := range sm.arenaTypes {
			a, err := g.elemArena(t, "", m.Pkg())
			if err != nil {
				return n, err
			}
			parts = append(parts, a)
		}
	}
	n.arena = "0"
	if len(parts) > 0 {
		n.arena = strings.Join(parts, " + ")
	}
	return n, nil
}

func (g *gen) precheck(w *strings.Builder, sm *sigmap, sig *types.Signature, cname string) (string, error) {
	nd, err := g.callNeeds(sm, sig)
	if err != nil {
		return "", err
	}
	if len(sm.slices) == 1 {
		so := sm.slices[0]
		name := strings.TrimPrefix(so.cn, "c_")
		fill, room := "0", "int("+so.cn+"_cap)"
		if so.app {
			fill = "nf_" + name
			room += " - " + fill
		}
		if isByte(so.e) && !so.app {
			room += " - 1"
		}
		extra := ""
		if isByte(so.e) && !so.app {
			extra = " + 1"
		}
		lines(w, "need_items := "+nd.items, "need_arena := "+nd.arena, "_ = need_arena",
			"if "+room+" < need_items {", "if "+so.cn+"_need != nil {", "*"+so.cn+"_need = C.size_t("+fill+" + need_items"+extra+")", "}")
		if sm.arena {
			lines(w, "if c_arena_need != nil {", "*c_arena_need = C.size_t(oc.need + need_arena)", "}")
		}
		lines(w, "return fail(C.DMRGO_ERR_SHORT)", "}")
	} else {
		lines(w, "need_arena := "+nd.arena)
	}
	if sm.arena {
		lines(w, "if int(c_arena_cap)-oc.need < need_arena {", "if c_arena_need != nil {", "*c_arena_need = C.size_t(oc.need + need_arena)", "}")
		for _, so := range sm.slices {
			fill := "0"
			if so.app {
				fill = "nf_" + strings.TrimPrefix(so.cn, "c_")
			}
			lines(w, "if "+so.cn+"_need != nil {", "*"+so.cn+"_need = C.size_t("+fill+" + need_items)", "}")
		}
		lines(w, "return fail(C.DMRGO_ERR_SHORT)", "}")
	}
	note := " It returns DMRGO_ERR_SHORT and changes nothing when the room is less than " + cname + "_need gives"
	if nd.itemsC != "" && !sm.arena {
		def := "#define " + strings.ToUpper(cname) + "_MAX_ITEMS (" + nd.itemsC + ")"
		g.defines = append(g.defines, def)
		g.abiSyms = append(g.abiSyms, "sym "+def)
		note += " (" + strings.ToUpper(cname) + "_MAX_ITEMS items)"
	}
	return note + ".", nil
}

func (g *gen) needFunc(cname string, recv *tinfo, rk recvKind, sig *types.Signature) error {
	var sm sigmap
	if _, err := g.mapRecv(&sm, recv, rk, ""); err != nil {
		return err
	}
	ai, _ := appendForm(sig)
	params := sig.Params()
	pnames, _ := g.sigNames(sig, g.curMethod, "arena", "items", "self")
	for i := 0; i < params.Len(); i++ {
		if i == ai {
			sm.args = append(sm.args, "nil")
			continue
		}
		p := params.At(i)
		if err := g.mapParam(&sm, pnames[i], p.Type(), false, nil, nil); err != nil {
			return err
		}
	}
	full := sm
	var probe sigmap
	g.probeOutputs(&probe, sig)
	full.slices, full.arena, full.arenaTypes = probe.slices, probe.arena, probe.arenaTypes
	nd, err := g.callNeeds(&full, sig)
	if err != nil {
		return err
	}
	items := nd.items
	if items == "" {
		items = "0"
	}
	name := cname + "_need"
	sm.cps = append(sm.cps, "size_t *items", "size_t *arena")
	sm.gps = append(sm.gps, "c_items *C.size_t", "c_arena *C.size_t")
	var w strings.Builder
	lines(&w, "//export "+name, "func "+name+"("+strings.Join(sm.gps, ", ")+") (rc C.int) {", "defer recoverTo(&rc)")
	w.WriteString(sm.pre.String())
	if rk == rHandle || rk == rOpen {
		lines(&w, "_ = h")
	}
	for _, a := range sm.args {
		if strings.HasPrefix(a, "a_") || strings.HasPrefix(a, "p_") {
			lines(&w, "_ = "+strings.TrimSuffix(a, "..."))
		}
	}
	lines(&w, "if c_items != nil {", "*c_items = C.size_t("+items+")", "}", "if c_arena != nil {", "*c_arena = C.size_t("+nd.arena+")", "}", "return C.DMRGO_OK", "}", "")
	if err := g.claim(name, cname); err != nil {
		return err
	}
	g.funcSyms = append(g.funcSyms, name)
	g.goFuncs.WriteString(w.String())
	proto := "int " + name + "(" + strings.Join(sm.cps, ", ") + ");"
	g.hProtos.WriteString("/* " + name + " sets *items and *arena to the room that " + cname + " needs for these arguments, without a change to the handle. */\n" + proto + "\n")
	g.abiSyms = append(g.abiSyms, "sym "+proto)
	return nil
}

func (g *gen) probeOutputs(sm *sigmap, sig *types.Signature) {
	ai, ar := appendForm(sig)
	if ai >= 0 {
		e := sliceElem(sig.Params().At(ai).Type())
		sm.slices = append(sm.slices, sliceOut{app: true, e: e})
		sm.arena = sm.arena || g.needsArena(e)
	}
	res := sig.Results()
	for i := 0; i < res.Len(); i++ {
		t := res.At(i).Type()
		switch {
		case i == ar || isError(t):
		case isString(t):
			sm.slices = append(sm.slices, sliceOut{e: types.Typ[types.Byte]})
		default:
			if sl, ok := t.(*types.Slice); ok {
				sm.slices = append(sm.slices, sliceOut{e: sl.Elem()})
				sm.arena = sm.arena || g.needsArena(sl.Elem())
			} else if g.needsArena(t) {
				sm.arena = true
				sm.arenaTypes = append(sm.arenaTypes, t)
			}
		}
	}
}

func isByte(t types.Type) bool {
	b, ok := t.(*types.Basic)
	return ok && (b.Kind() == types.Byte || b.Kind() == types.Uint8)
}
