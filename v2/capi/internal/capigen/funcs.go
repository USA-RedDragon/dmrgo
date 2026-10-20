package capigen

import (
	"fmt"
	"go/types"
	"strings"
)

type sliceOut struct {
	cn  string
	app bool
	e   types.Type
}

type sigmap struct {
	arena      bool
	arenaTypes []types.Type
	slices     []sliceOut
	commit     strings.Builder
	cps        []string
	gps        []string
	pre        strings.Builder
	args       []string
	after      strings.Builder
	post       strings.Builder
}

func (g *gen) cgoElem(t types.Type) (string, error) {
	if isString(t) {
		return "C.dmrgo_string_t", nil
	}
	if p, ok := t.(*types.Pointer); ok {
		if c, isH := g.handleC(p.Elem()); isH {
			return "*C." + c, nil
		}
		t = p.Elem()
	}
	if c, ok := g.handleC(t); ok {
		return "*C." + c, nil
	}
	return g.cgoArray(t)
}

func (g *gen) cgoParamElem(t types.Type) (string, error) {
	if _, ok := t.(*types.Array); ok {
		return g.cgoDeclType(t)
	}
	return g.cgoElem(t)
}

func constDecl(d string) string { return "const " + d }

func (g *gen) arrayPtr(t types.Type, p string) (string, error) {
	if _, ok := t.(*types.Array); !ok {
		return "(*" + p + ")", nil
	}
	c, err := g.cgoArray(t)
	if err != nil {
		return "", err
	}
	return "(*(*" + c + ")(unsafe.Pointer(" + p + ")))", nil
}

func (g *gen) baseGo(t types.Type) (string, error) {
	if _, ok := t.(*types.Array); ok {
		c, err := g.cgoDeclType(t)
		return "*" + c, err
	}
	c, err := g.ctype(t)
	if err != nil {
		return "", err
	}
	return "*" + g.cgoType(c), nil
}

func (g *gen) mapParam(sm *sigmap, n string, t types.Type, dst bool, owner *tinfo, appendRes *int) error {
	cn := "c_" + n
	a := "a_" + n
	if c, ok := g.scalar(t); ok {
		sm.cps = append(sm.cps, c+" "+n)
		sm.gps = append(sm.gps, cn+" "+g.cgoType(c))
		sm.args = append(sm.args, g.goType(t)+"("+cn+")")
		return nil
	}
	if isString(t) {
		sm.cps = append(sm.cps, "const char *"+n, "size_t "+n+"_len")
		sm.gps = append(sm.gps, cn+" *C.char", cn+"_len C.size_t")
		lines(&sm.pre, "if "+cn+" == nil && "+cn+"_len > 0 {", "return fail(C.DMRGO_ERR_ARG)", "}")
		sm.args = append(sm.args, "C.GoStringN("+cn+", C.int("+cn+"_len))")
		return nil
	}
	if s, ok := t.(*types.Signature); ok {
		f, err := g.fnType(s, g.cur+"_"+n+"_fn_t")
		if err != nil {
			return err
		}
		sm.cps = append(sm.cps, f.c+" "+n)
		sm.gps = append(sm.gps, cn+" C."+f.c)
		sm.args = append(sm.args, f.in+"("+cn+")")
		return nil
	}
	if ti := g.info(t); ti != nil {
		switch ti.kind {
		default:
		case kHandle:
			sm.cps = append(sm.cps, ti.c+" *"+n)
			sm.gps = append(sm.gps, cn+" *C."+ti.c)
			sm.args = append(sm.args, "*"+g.getter(ti)+"("+cn+").v")
			return nil
		case kOpen:
			sm.cps = append(sm.cps, ti.c+" *"+n)
			sm.gps = append(sm.gps, cn+" *C."+ti.c)
			lines(&sm.pre, "var "+a+" "+g.goType(t), "if "+cn+" != nil {", a+" = "+g.getter(ti)+"("+cn+").v", "}")
			sm.args = append(sm.args, a)
			return nil
		case kNFunc:
			f, err := g.fnType(underSignature(ti.named), ti.c)
			if err != nil {
				return err
			}
			sm.cps = append(sm.cps, f.c+" "+n)
			sm.gps = append(sm.gps, cn+" C."+f.c)
			sm.args = append(sm.args, g.goType(t)+"("+f.in+"("+cn+"))")
			return nil
		}
	}
	if sl, ok := t.(*types.Slice); ok {
		return g.mapSliceParam(sm, n, sl, owner, appendRes)
	}
	if ti := g.info(t); ti != nil && ti.kind == kClosed {
		sm.cps = append(sm.cps, "const "+ti.c+" *"+n)
		sm.gps = append(sm.gps, cn+" *C."+ti.c)
		lines(&sm.pre, "if "+cn+" == nil {", "return fail(C.DMRGO_ERR_ARG)", "}", "var "+a+" "+g.goType(t))
		if err := g.unionInline(&sm.pre, ti, a, cn); err != nil {
			return err
		}
		sm.args = append(sm.args, a)
		return nil
	}
	if p, ok := t.(*types.Pointer); ok {
		return g.mapPtrParam(sm, n, p.Elem(), dst)
	}
	if err := g.needDeps(t); err != nil {
		return err
	}
	d, err := g.ptrDecl(t, n)
	if err != nil {
		return err
	}
	bt, err := g.baseGo(t)
	if err != nil {
		return err
	}
	sm.cps = append(sm.cps, constDecl(d))
	sm.gps = append(sm.gps, cn+" "+bt)
	lines(&sm.pre, "if "+cn+" == nil {", "return fail(C.DMRGO_ERR_ARG)", "}")
	if g.alias(t) {
		sm.args = append(sm.args, "*(*"+g.goType(t)+")(unsafe.Pointer("+cn+"))")
		return nil
	}
	src, err := g.arrayPtr(t, cn)
	if err != nil {
		return err
	}
	st, err := g.storage(t)
	if err != nil {
		return err
	}
	lines(&sm.pre, "var "+a+" "+g.goType(t))
	sv := ""
	if st != "" {
		lines(&sm.pre, "var s_"+n+" "+st)
		sv = "s_" + n
	}
	if err := g.inVal(&sm.pre, t, a, src, sv, 0); err != nil {
		return err
	}
	sm.args = append(sm.args, a)
	return nil
}

func (g *gen) ptrDecl(t types.Type, n string) (string, error) {
	if _, ok := t.(*types.Array); ok {
		return g.decl(t, n)
	}
	c, err := g.ctype(t)
	if err != nil {
		return "", err
	}
	return c + " *" + n, nil
}

func (g *gen) mapPtrParam(sm *sigmap, n string, e types.Type, dst bool) error {
	cn := "c_" + n
	a := "a_" + n
	if ti := g.info(e); ti != nil && ti.kind == kHandle {
		sm.cps = append(sm.cps, ti.c+" *"+n)
		sm.gps = append(sm.gps, cn+" *C."+ti.c)
		lines(&sm.pre, "var "+a+" *"+g.goType(e), "if "+cn+" != nil {", a+" = "+g.getter(ti)+"("+cn+").v", "}")
		sm.args = append(sm.args, a)
		return nil
	}
	if _, isH := g.handleC(e); isH {
		return fmt.Errorf("pointer to interface")
	}
	if arr, ok := e.(*types.Array); ok {
		if ti := g.info(arr.Elem()); ti != nil && ti.kind == kHandle {
			sm.cps = append(sm.cps, fmt.Sprintf("%s *const %s[%d]", ti.c, n, arr.Len()))
			sm.gps = append(sm.gps, cn+" **C."+ti.c)
			lines(&sm.pre, "if "+cn+" == nil {", "return fail(C.DMRGO_ERR_ARG)", "}", "hs_"+n+" := unsafe.Slice("+cn+", "+fmt.Sprint(arr.Len())+")",
				"var "+a+" "+g.goType(e), "for i := range "+a+" {", a+"[i] = *"+g.getter(ti)+"(hs_"+n+"[i]).v", "}")
			lines(&sm.after, "for i := range "+a+" {", "*"+g.getter(ti)+"(hs_"+n+"[i]).v = "+a+"[i]", "}")
			sm.args = append(sm.args, "&"+a)
			return nil
		}
	}
	if err := g.needDeps(e); err != nil {
		return err
	}
	d, err := g.ptrDecl(e, n)
	if err != nil {
		return err
	}
	bt, err := g.baseGo(e)
	if err != nil {
		return err
	}
	sm.gps = append(sm.gps, cn+" "+bt)
	if dst {
		sm.arena = sm.arena || g.needsArena(e)
	}
	if g.alias(e) {
		sm.cps = append(sm.cps, d)
		sm.args = append(sm.args, "(*"+g.goType(e)+")(unsafe.Pointer("+cn+"))")
		return nil
	}
	src, err := g.arrayPtr(e, cn)
	if err != nil {
		return err
	}
	lines(&sm.pre, "var "+a+" "+g.goType(e), "var p_"+n+" *"+g.goType(e))
	if dst {
		sm.cps = append(sm.cps, d)
		lines(&sm.pre, "if "+cn+" != nil {", "p_"+n+" = &"+a, "}")
		lines(&sm.after, "if "+cn+" != nil {")
		if err := g.outVal(&sm.after, e, src, a, 0); err != nil {
			return err
		}
		lines(&sm.after, "}")
		sm.args = append(sm.args, "p_"+n)
		return nil
	}
	sm.cps = append(sm.cps, constDecl(d))
	st, err := g.storage(e)
	if err != nil {
		return err
	}
	sv := ""
	if st != "" {
		lines(&sm.pre, "var s_"+n+" "+st)
		sv = "s_" + n
	}
	lines(&sm.pre, "if "+cn+" != nil {")
	if err := g.inVal(&sm.pre, e, a, src, sv, 0); err != nil {
		return err
	}
	lines(&sm.pre, "p_"+n+" = &"+a, "}")
	sm.args = append(sm.args, "p_"+n)
	return nil
}

func (g *gen) mapSliceParam(sm *sigmap, n string, sl *types.Slice, owner *tinfo, appendRes *int) error {
	cn := "c_" + n
	a := "a_" + n
	e := sl.Elem()
	if err := g.needDeps(e); err != nil {
		return err
	}
	ed, err := g.elemDecl(e, "*"+n)
	if err != nil {
		return err
	}
	ge, err := g.cgoParamElem(e)
	if err != nil {
		return err
	}
	full, err := g.cgoElem(e)
	if err != nil {
		return err
	}
	if appendRes != nil {
		sm.cps = append(sm.cps, ed, "size_t "+n+"_cap", "size_t *"+n+"_len", "size_t *"+n+"_need")
		sm.gps = append(sm.gps, cn+" *"+ge, cn+"_cap C.size_t", cn+"_len *C.size_t", cn+"_need *C.size_t")
		lines(&sm.pre, "if "+cn+"_len == nil || *"+cn+"_len > "+cn+"_cap || ("+cn+" == nil && "+cn+"_cap > 0) {", "return fail(C.DMRGO_ERR_ARG)", "}",
			"nf_"+n+" := int(*"+cn+"_len)")
		lines(&sm.commit, "*"+cn+"_len = C.size_t(nf_"+n+")")
		sm.slices = append(sm.slices, sliceOut{cn: cn, app: true, e: e})
		r := fmt.Sprintf("r%d", *appendRes)
		if g.alias(e) {
			lines(&sm.pre, "full_"+n+" := unsafe.Slice((*"+g.goType(e)+")(unsafe.Pointer("+cn+")), int("+cn+"_cap))", a+" := full_"+n+"[:*"+cn+"_len]")
			lines(&sm.post, "nf_"+n+" = putAppend(full_"+n+", nf_"+n+", "+cn+"_need, "+r+", oc)")
			sm.args = append(sm.args, a)
			return nil
		}
		if owner != nil {
			k := g.addScratch(owner, g.goType(sl))
			lines(&sm.pre, a+" := h."+k+"[:0]")
			lines(&sm.post, "h."+k+" = "+r+"[:0]")
		} else {
			lines(&sm.pre, "var "+a+" "+g.goType(sl))
		}
		lines(&sm.post, "{", "base := nf_"+n, "total := base + len("+r+")", "if "+cn+"_need != nil {", "*"+cn+"_need = C.size_t(total)", "}",
			"if total > int("+cn+"_cap) {", "oc.short = true", "} else {", "cs := unsafe.Slice((*"+full+")(unsafe.Pointer("+cn+")), int("+cn+"_cap))", "for i := range "+r+" {")
		sm.arena = sm.arena || g.needsArena(e)
		if err := g.outElem(&sm.post, e, "cs[base+i]", r+"[i]"); err != nil {
			return err
		}
		lines(&sm.post, "}", "nf_"+n+" = total", "}", "}")
		sm.args = append(sm.args, a)
		return nil
	}
	if g.fillBuf && g.alias(e) {
		sm.cps = append(sm.cps, ed, "size_t "+n+"_len")
	} else {
		sm.cps = append(sm.cps, constDecl(ed), "size_t "+n+"_len")
	}
	sm.gps = append(sm.gps, cn+" *"+ge, cn+"_len C.size_t")
	lines(&sm.pre, "if "+cn+" == nil && "+cn+"_len > 0 {", "return fail(C.DMRGO_ERR_ARG)", "}", "var "+a+" "+g.goType(sl))
	if err := g.inSlice(&sm.pre, sl, a, cn, cn+"_len"); err != nil {
		return err
	}
	sm.args = append(sm.args, a)
	return nil
}

func (g *gen) mapResult(sm *sigmap, n string, t types.Type, r string) error {
	cn := "c_" + n
	if c, ok := g.scalar(t); ok {
		sm.cps = append(sm.cps, c+" *"+n)
		sm.gps = append(sm.gps, cn+" *"+g.cgoType(c))
		lines(&sm.post, "if "+cn+" != nil {", "*"+cn+" = "+g.cgoType(c)+"("+r+")", "}")
		return nil
	}
	if isString(t) {
		sm.cps = append(sm.cps, "char *"+n, "size_t "+n+"_cap", "size_t *"+n+"_len", "size_t *"+n+"_need")
		sm.gps = append(sm.gps, cn+" *C.char", cn+"_cap C.size_t", cn+"_len *C.size_t", cn+"_need *C.size_t")
		lines(&sm.pre, "if "+cn+" == nil && "+cn+"_cap > 0 {", "return fail(C.DMRGO_ERR_ARG)", "}")
		lines(&sm.post, "putStr("+cn+", "+cn+"_cap, "+cn+"_len, "+cn+"_need, "+r+", oc)")
		sm.slices = append(sm.slices, sliceOut{cn: cn, e: types.Typ[types.Byte]})
		return nil
	}
	if s, ok := t.(*types.Signature); ok {
		f, err := g.fnType(s, g.cur+"_fn_t")
		if err != nil {
			return err
		}
		if f.out == "" {
			return fmt.Errorf("func result has no C form")
		}
		sm.cps = append(sm.cps, f.c+" *"+n)
		sm.gps = append(sm.gps, cn+" *C."+f.c)
		lines(&sm.post, "if "+cn+" != nil {", "*"+cn+" = "+f.out+"("+r+")", "}")
		return nil
	}
	if ti := g.info(t); ti != nil {
		switch ti.kind {
		default:
		case kHandle:
			sm.cps = append(sm.cps, ti.c+" *"+n)
			sm.gps = append(sm.gps, cn+" *C."+ti.c)
			lines(&sm.post, "if "+cn+" != nil {", "*"+g.getter(ti)+"("+cn+").v = "+r, "}")
			return nil
		case kOpen:
			sm.cps = append(sm.cps, ti.c+" **"+n)
			sm.gps = append(sm.gps, cn+" **C."+ti.c)
			lines(&sm.pre, "if "+cn+" == nil {", "return fail(C.DMRGO_ERR_ARG)", "}")
			lines(&sm.post, "*"+cn+" = nil", "if "+r+" != nil {", "*"+cn+" = "+g.newer(ti)+"("+r+")", "}")
			return nil
		case kNFunc:
			f, err := g.fnType(underSignature(ti.named), ti.c)
			if err != nil {
				return err
			}
			if f.out == "" {
				return fmt.Errorf("func result has no C form")
			}
			sm.cps = append(sm.cps, f.c+" *"+n)
			sm.gps = append(sm.gps, cn+" *C."+f.c)
			lines(&sm.post, "if "+cn+" != nil {", "*"+cn+" = "+f.out+"("+r+")", "}")
			return nil
		}
	}
	if sl, ok := t.(*types.Slice); ok {
		return g.mapSliceResult(sm, n, sl, r)
	}
	if p, ok := t.(*types.Pointer); ok {
		e := p.Elem()
		if ti := g.info(e); ti != nil && ti.kind == kHandle {
			sm.cps = append(sm.cps, ti.c+" **"+n)
			sm.gps = append(sm.gps, cn+" **C."+ti.c)
			lines(&sm.pre, "if "+cn+" == nil {", "return fail(C.DMRGO_ERR_ARG)", "}")
			lines(&sm.post, "*"+cn+" = nil", "if "+r+" != nil {", "*"+cn+" = "+g.newer(ti)+"("+r+")", "}")
			return nil
		}
		if err := g.needDeps(e); err != nil {
			return err
		}
		d, err := g.ptrDecl(e, n)
		if err != nil {
			return err
		}
		bt, err := g.baseGo(e)
		if err != nil {
			return err
		}
		if g.needsArena(e) {
			sm.arena = true
			sm.arenaTypes = append(sm.arenaTypes, e)
		}
		sm.cps = append(sm.cps, d, "bool *"+n+"_ok")
		sm.gps = append(sm.gps, cn+" "+bt, cn+"_ok *C.bool")
		dst, err := g.arrayPtr(e, cn)
		if err != nil {
			return err
		}
		lines(&sm.post, "if "+cn+"_ok != nil {", "*"+cn+"_ok = C.bool("+r+" != nil)", "}", "if "+cn+" != nil && "+r+" != nil {")
		if err := g.outVal(&sm.post, e, dst, "(*"+r+")", 0); err != nil {
			return err
		}
		lines(&sm.post, "}")
		return nil
	}
	if err := g.needDeps(t); err != nil {
		return err
	}
	d, err := g.ptrDecl(t, n)
	if err != nil {
		return err
	}
	bt, err := g.baseGo(t)
	if err != nil {
		return err
	}
	if g.needsArena(t) {
		sm.arena = true
		sm.arenaTypes = append(sm.arenaTypes, t)
	}
	sm.cps = append(sm.cps, d)
	sm.gps = append(sm.gps, cn+" "+bt)
	dst, err := g.arrayPtr(t, cn)
	if err != nil {
		return err
	}
	lines(&sm.post, "if "+cn+" != nil {")
	if err := g.outVal(&sm.post, t, dst, r, 0); err != nil {
		return err
	}
	lines(&sm.post, "}")
	return nil
}

type recvKind int

const (
	rNone recvKind = iota
	rHandle
	rOpen
	rValPtr
	rValVal
	rClosed
	rEnum
	rEnumPtr
	rArrPtr
	rArrVal
)

func paramName(v *types.Var, i int) string {
	n := v.Name()
	if n == "" || n == "_" {
		n = fmt.Sprintf("p%d", i)
	}
	return cKeyword(snake(n))
}

func resultNames(sig *types.Signature) []string {
	res := sig.Results()
	var out []string
	used := map[string]bool{}
	for i := 0; i < res.Len(); i++ {
		used[res.At(i).Name()] = true
	}
	for i := 0; i < res.Len(); i++ {
		n := res.At(i).Name()
		switch {
		case n != "" && n != "_":
			n = cKeyword(snake(n))
		case res.Len() == 1 || (res.Len() == 2 && isError(res.At(1).Type())):
			n = "out"
		default:
			n = fmt.Sprintf("out%d", i)
		}
		out = append(out, n)
	}
	return out
}

type lead struct {
	c, gop []string
	pre    string
	field  bool
	assign bool
}

func (g *gen) mapRecv(sm *sigmap, recv *tinfo, rk recvKind, call string) (string, error) {
	const self = "self"
	addSelf := func(c, gop string, check bool) {
		sm.cps = append(sm.cps, c)
		sm.gps = append(sm.gps, gop)
		if check {
			lines(&sm.pre, "if c_self == nil {", "return fail(C.DMRGO_ERR_ARG)", "}")
		}
	}
	ptr := "c_self *C." + recv.c
	switch rk {
	case rHandle, rOpen:
		addSelf(recv.c+" *"+self, ptr, false)
		lines(&sm.pre, "h := "+g.getter(recv)+"(c_self)")
		return "h.v." + call, nil
	case rClosed:
		addSelf("const "+recv.c+" *"+self, ptr, true)
		lines(&sm.pre, "var x "+g.goType(recv.named))
		return "x." + call, g.unionInline(&sm.pre, recv, "x", "c_self")
	case rValPtr, rValVal:
		if err := g.needDeps(recv.named); err != nil {
			return "", err
		}
		d := recv.c + " *" + self
		if rk != rValPtr {
			d = "const " + d
		}
		addSelf(d, ptr, true)
		lines(&sm.pre, "var x "+g.goType(recv.named))
		st, err := g.storage(recv.named)
		if err != nil {
			return "", err
		}
		sv := ""
		if st != "" {
			lines(&sm.pre, "var xs "+st)
			sv = "xs"
		}
		if err := g.inVal(&sm.pre, recv.named, "x", "(*c_self)", sv, 0); err != nil {
			return "", err
		}
		if rk == rValPtr {
			sm.arena = sm.arena || g.needsArena(recv.named)
			if err := g.outVal(&sm.after, recv.named, "(*c_self)", "x", 0); err != nil {
				return "", err
			}
		}
		return "x." + call, nil
	case rEnumPtr:
		addSelf(recv.c+" *"+self, ptr, true)
		if g.alias(recv.named) {
			return "(*" + g.goType(recv.named) + ")(unsafe.Pointer(c_self))." + call, nil
		}
		lines(&sm.pre, "x := "+g.goType(recv.named)+"(*c_self)")
		lines(&sm.after, "*c_self = C."+recv.c+"(x)")
		return "x." + call, nil
	case rEnum:
		addSelf(recv.c+" "+self, "c_self C."+recv.c, false)
		return g.goType(recv.named) + "(c_self)." + call, nil
	case rArrPtr:
		addSelf(recv.c+" *"+self, ptr, true)
		return "(*" + g.goType(recv.named) + ")(unsafe.Pointer(c_self))." + call, nil
	case rArrVal:
		addSelf("const "+recv.c+" *"+self, ptr, true)
		return "(*(*" + g.goType(recv.named) + ")(unsafe.Pointer(c_self)))." + call, nil
	default:
		return "", fmt.Errorf("no receiver form")
	}
}

func (g *gen) wrapper(cname, doc string, recv *tinfo, rk recvKind, call string, sig *types.Signature, ld *lead) (string, string, error) {
	var sm sigmap
	target := call
	if ld != nil {
		sm.cps = append(sm.cps, ld.c...)
		sm.gps = append(sm.gps, ld.gop...)
		sm.pre.WriteString(ld.pre)
	}
	if recv != nil {
		t, err := g.mapRecv(&sm, recv, rk, call)
		if err != nil {
			return "", "", err
		}
		target = t
	}
	params, res := sig.Params(), sig.Results()
	appendIdx, appendRes := appendForm(sig)
	owner := recv
	if rk != rHandle {
		owner = nil
	}
	for i := 0; i < params.Len(); i++ {
		p := params.At(i)
		var ar *int
		if i == appendIdx {
			ar = &appendRes
		}
		if err := g.mapParam(&sm, paramName(p, i), p.Type(), p.Name() == dstName, owner, ar); err != nil {
			return "", "", fmt.Errorf("param %s: %w", p.Name(), err)
		}
	}
	if sig.Variadic() {
		sm.args[len(sm.args)-1] += "..."
	}
	names := resultNames(sig)
	var rvars []string
	errVar := ""
	for i := 0; i < res.Len(); i++ {
		r := fmt.Sprintf("r%d", i)
		rvars = append(rvars, r)
		switch t := res.At(i).Type(); {
		case isError(t):
			errVar = r
		case i == appendRes:
		default:
			if err := g.mapResult(&sm, names[i], t, r); err != nil {
				return "", "", fmt.Errorf("result %d: %w", i, err)
			}
		}
	}
	if sm.arena {
		sm.cps = append(sm.cps, "void *arena", "size_t arena_cap", "size_t *arena_len", "size_t *arena_need")
		sm.gps = append(sm.gps, "c_arena unsafe.Pointer", "c_arena_cap C.size_t", "c_arena_len *C.size_t", "c_arena_need *C.size_t")
	}
	var w strings.Builder
	lines(&w, "//export "+cname, "func "+cname+"("+strings.Join(sm.gps, ", ")+") (rc C.int) {", "defer recoverTo(&rc)", "oc := &outc{}")
	if sm.arena {
		lines(&w, "if c_arena_len != nil {", "if *c_arena_len > c_arena_cap {", "return fail(C.DMRGO_ERR_ARG)", "}", "oc.need = int(*c_arena_len)", "}", "oc.setArena(c_arena, c_arena_cap)")
	}
	w.WriteString(sm.pre.String())
	bounded, note := g.stateful(&w, &sm, sig, cname, rk, ld == nil)
	if bounded {
		if err := g.needFunc(cname, recv, rk, sig); err != nil {
			return "", "", err
		}
	}
	doc += note
	callLine(&w, &sm, ld, rvars, target)
	w.WriteString(sm.after.String())
	for _, r := range rvars {
		if r != errVar {
			lines(&w, "_ = "+r)
		}
	}
	w.WriteString(sm.post.String())
	if sm.arena {
		lines(&w, "if c_arena_need != nil {", "*c_arena_need = C.size_t(oc.reported())", "}")
		lines(&sm.commit, "if c_arena_len != nil {", "*c_arena_len = C.size_t(oc.need)", "}")
	}
	if bounded {
		lines(&w, "if oc.short {", "return setLast(C.DMRGO_ERR_BOUND, \"dmrgo: an output passed the bound of its _need function; the call took effect\")", "}")
	}
	lines(&w, "if oc.short {", "return fail(C.DMRGO_ERR_SHORT)", "}")
	w.WriteString(sm.commit.String())
	if errVar != "" {
		lines(&w, "if "+errVar+" != nil {", "return setErr("+errVar+")", "}")
	}
	lines(&w, "return C.DMRGO_OK", "}", "")
	cps := sm.cps
	if len(cps) == 0 {
		cps = []string{"void"}
	}
	proto := "int " + cname + "(" + strings.Join(cps, ", ") + ");"
	return w.String(), g.protoDoc(doc) + proto + "\n", nil
}

func (g *gen) protoDoc(doc string) string {
	if doc == "" {
		return ""
	}
	return "/* " + doc + " */\n"
}

func (g *gen) getter(ti *tinfo) string {
	g.usedHandles[ti] = true
	return "get_" + ti.id
}

func (g *gen) newer(ti *tinfo) string {
	g.usedHandles[ti] = true
	return "new_" + ti.id
}

func (g *gen) addScratch(ti *tinfo, typ string) string {
	for i, s := range g.scratch[ti] {
		if s == typ {
			return fmt.Sprintf("sc%d", i)
		}
	}
	g.scratch[ti] = append(g.scratch[ti], typ)
	return fmt.Sprintf("sc%d", len(g.scratch[ti])-1)
}

func (g *gen) mapSliceResult(sm *sigmap, n string, sl *types.Slice, r string) error {
	cn := "c_" + n
	e := sl.Elem()
	if err := g.needDeps(e); err != nil {
		return err
	}
	ed, err := g.elemDecl(e, "*"+n)
	if err != nil {
		return err
	}
	ge, err := g.cgoParamElem(e)
	if err != nil {
		return err
	}
	sm.cps = append(sm.cps, ed, "size_t "+n+"_cap", "size_t *"+n+"_len", "size_t *"+n+"_need")
	sm.gps = append(sm.gps, cn+" *"+ge, cn+"_cap C.size_t", cn+"_len *C.size_t", cn+"_need *C.size_t")
	lines(&sm.pre, "if "+cn+"_len == nil || ("+cn+" == nil && "+cn+"_cap > 0) {", "return fail(C.DMRGO_ERR_ARG)", "}")
	sm.arena = sm.arena || g.needsArena(e)
	sm.slices = append(sm.slices, sliceOut{cn: cn, e: e})
	return g.outSlice(&sm.post, sl, cn, cn+"_cap", cn+"_len", cn+"_need", r)
}

func (g *gen) stateful(w *strings.Builder, sm *sigmap, sig *types.Signature, cname string, rk recvKind, plain bool) (bool, string) {
	if !plain || (rk != rHandle && rk != rOpen) || (!sm.arena && len(sm.slices) == 0) || g.curMethod == nil ||
		g.curMethod.Name() == "String" || g.curMethod.Name() == "Error" {
		return false, ""
	}
	note, err := g.precheck(w, sm, sig, cname)
	if err != nil {
		g.unbounded = append(g.unbounded, cname+": "+err.Error())
		return false, " It changes the state of the handle even when it returns DMRGO_ERR_SHORT."
	}
	return true, note
}

func callLine(w *strings.Builder, sm *sigmap, ld *lead, rvars []string, target string) {
	switch {
	case ld != nil && ld.assign:
		lines(w, target+" = "+sm.args[0])
	case ld != nil && ld.field:
		lines(w, strings.Join(rvars, ", ")+" := "+target)
	case len(rvars) > 0:
		lines(w, strings.Join(rvars, ", ")+" := "+target+"("+strings.Join(sm.args, ", ")+")")
	default:
		lines(w, target+"("+strings.Join(sm.args, ", ")+")")
	}
}
