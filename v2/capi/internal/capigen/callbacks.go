package capigen

import (
	"fmt"
	"go/types"
	"regexp"
	"slices"
	"strings"
)

type fnInfo struct {
	sig     *types.Signature
	c       string
	in, out string
	k       int
	err     error
}

func cName(decl string) string {
	d := regexp.MustCompile(`\[[0-9]+\]`).ReplaceAllString(decl, "")
	d = strings.NewReplacer("(", " ", ")", " ", "*", " ").Replace(d)
	f := strings.Fields(d)
	return f[len(f)-1]
}

func (g *gen) fnType(sig *types.Signature, name string) (*fnInfo, error) {
	for _, f := range g.fns {
		if types.Identical(f.sig, sig) {
			return f, f.err
		}
	}
	k := len(g.fns)
	if name == "" {
		name = g.cur + "_fn_t"
		for i := 2; g.taken(name); i++ {
			name = fmt.Sprintf("%s_fn%d_t", g.cur, i)
		}
	}
	f := &fnInfo{sig: sig, c: name, k: k, in: fmt.Sprintf("fnin%d", k)}
	g.fns = append(g.fns, f)
	f.err = g.emitFn(f)
	return f, f.err
}

func (g *gen) taken(name string) bool {
	if _, ok := g.cnames[name]; ok {
		return true
	}
	for _, f := range g.fns {
		if f.c == name {
			return true
		}
	}
	return false
}

func (g *gen) cParams(sig *types.Signature) ([]string, error) {
	var sm sigmap
	params := sig.Params()
	res := sig.Results()
	appendIdx, appendRes := appendForm(sig)
	pnames, names := g.sigNames(sig, nil, "user")
	for i := 0; i < params.Len(); i++ {
		p := params.At(i)
		var ar *int
		if i == appendIdx {
			ar = &appendRes
		}
		if err := g.mapParam(&sm, pnames[i], p.Type(), p.Name() == dstName, nil, ar); err != nil {
			return nil, err
		}
	}
	for i := 0; i < res.Len(); i++ {
		if isError(res.At(i).Type()) || i == appendRes {
			continue
		}
		if err := g.mapResult(&sm, names[i], res.At(i).Type(), "x"); err != nil {
			return nil, err
		}
	}
	return sm.cps, nil
}

func appendForm(sig *types.Signature) (int, int) {
	params, res := sig.Params(), sig.Results()
	for i := 0; i < params.Len(); i++ {
		p := params.At(i)
		if p.Name() != dstName {
			continue
		}
		if _, ok := p.Type().(*types.Slice); !ok {
			continue
		}
		for j := 0; j < res.Len(); j++ {
			if types.Identical(res.At(j).Type(), p.Type()) {
				return i, j
			}
		}
	}
	return -1, -1
}

func (g *gen) emitFn(f *fnInfo) error {
	cps, err := g.cParams(f.sig)
	if err != nil {
		return err
	}
	all := append([]string{userParam}, cps...)
	g.fwd = append(g.fwd, "typedef struct "+f.c+" "+f.c+";")
	var b strings.Builder
	lines(&b, "struct "+f.c+" {", "\tint (*fn)("+strings.Join(all, ", ")+");", "\tDMRGO_USER_T user;", "};", "")
	g.hStructs.WriteString(b.String())
	g.abiTypes = append(g.abiTypes, "type "+strings.Join(strings.Fields(b.String()), " "))
	var names []string
	for _, p := range cps {
		names = append(names, cName(p))
	}
	tr := f.c + "__call"
	lines(&g.internal, "static inline int "+tr+"("+strings.Join(append([]string{f.c + " f"}, cps...), ", ")+") {",
		"\treturn f.fn("+strings.Join(append([]string{"f.user"}, names...), ", ")+");", "}", "")
	gs := g.goType(f.sig)
	body, fields, err := g.cbBody(f.sig, "C."+tr+"(c.f", "")
	if err != nil {
		return err
	}
	var w strings.Builder
	exp := fmt.Sprintf("dmrgo__fn%d", f.k)
	code, _, werr := g.wrapper(exp, "", nil, rNone, "fv", f.sig, &lead{c: []string{userParam}, gop: []string{"user C.uintptr_t"}, pre: "fv := hvalue[" + gs + "](uintptr(user))\n"})
	if werr == nil {
		f.out = fmt.Sprintf("fnout%d", f.k)
		g.goFuncs.WriteString(code)
		lines(&g.genC, fmt.Sprintf("void *dmrgo__fnptr%d(void) { return (void *)%s; }", f.k, exp), "")
		lines(&g.preamble, fmt.Sprintf("void *dmrgo__fnptr%d(void);", f.k))
		lines(&w, "func "+f.out+"(v "+gs+") C."+f.c+" {", "if v == nil {", "return C."+f.c+"{}", "}",
			fmt.Sprintf("return C.%s{fn: (*[0]byte)(C.dmrgo__fnptr%d()), user: newUser(v)}", f.c, f.k), "}", "")
	}
	lines(&w, fmt.Sprintf("type cb%d struct {", f.k), "f C."+f.c)
	for _, fl := range fields {
		lines(&w, fl)
	}
	lines(&w, "}", "", "func "+f.in+"(p C."+f.c+") "+gs+" {", "if p.fn == nil {", "return nil", "}")
	if werr == nil {
		lines(&w, fmt.Sprintf("if unsafe.Pointer(p.fn) == C.dmrgo__fnptr%d() {", f.k), "return hvalue["+gs+"](uintptr(p.user))", "}")
	}
	lines(&w, fmt.Sprintf("c := &cb%d{f: p}", f.k), "return c.call", "}", "")
	w.WriteString(fmt.Sprintf("func (c *cb%d) call", f.k) + body)
	g.goConv.WriteString(w.String())
	return nil
}

type cbuild struct {
	g                        *gen
	fp                       string
	appendIdx                int
	fields, gp, gr, cargs    []string
	pre, loopSet, grow, post strings.Builder
	errName                  string
	results                  []string
	resultType               map[string]string
}

func (b *cbuild) zeroed(field, typ string) {
	if b.resultType == nil {
		b.resultType = map[string]string{}
	}
	b.results = append(b.results, field)
	b.resultType[field] = typ
}

func (g *gen) cbBody(sig *types.Signature, call, fp string) (string, []string, error) {
	params, res := sig.Params(), sig.Results()
	appendIdx, appendRes := appendForm(sig)
	b := &cbuild{g: g, fp: fp, appendIdx: appendIdx}
	for i := 0; i < params.Len(); i++ {
		if err := b.param(i, params.At(i).Type(), sig.Variadic() && i == params.Len()-1); err != nil {
			return "", nil, err
		}
	}
	for i := 0; i < res.Len(); i++ {
		if err := b.result(i, res.At(i).Type(), i == appendRes); err != nil {
			return "", nil, err
		}
	}
	var w strings.Builder
	lines(&w, "("+strings.Join(b.gp, ", ")+") ("+strings.Join(b.gr, ", ")+") {", "oc := &outc{}", "_ = oc")
	w.WriteString(b.pre.String())
	c := call
	if len(b.cargs) > 0 {
		c += ", " + strings.Join(b.cargs, ", ")
	}
	lines(&w, "var code C.int", "for {")
	for _, r := range b.results {
		lines(&w, "c."+r+" = *new("+b.resultType[r]+")")
	}
	w.WriteString(b.loopSet.String())
	lines(&w, "code = "+c+")")
	if b.grow.Len() > 0 {
		lines(&w, "if code == C.DMRGO_ERR_SHORT {", "grown := false")
		w.WriteString(b.grow.String())
		lines(&w, "if grown {", "continue", "}", "}")
	}
	lines(&w, "break", "}")
	if b.errName != "" {
		lines(&w, "if code != C.DMRGO_OK {", b.errName+" = codeErr(code)", "return", "}")
	} else {
		lines(&w, "if code != C.DMRGO_OK {", "return", "}")
	}
	w.WriteString(b.post.String())
	lines(&w, "return", "}", "")
	return w.String(), b.fields, nil
}

func (b *cbuild) param(i int, t types.Type, variadic bool) error {
	g := b.g
	q := fmt.Sprintf("q%d", i)
	if variadic {
		b.gp = append(b.gp, q+" ..."+g.goType(sliceElem(t)))
	} else {
		b.gp = append(b.gp, q+" "+g.goType(t))
	}
	if i == b.appendIdx {
		e := sliceElem(t)
		if !g.alias(e) {
			return fmt.Errorf("append form callback of %s", e)
		}
		ge, err := g.cgoElem(e)
		if err != nil {
			return err
		}
		n := fmt.Sprintf("%sn%d", b.fp, i)
		b.fields = append(b.fields, n+" C.size_t", n+"need C.size_t")
		lines(&b.pre, "d := "+q)
		lines(&b.loopSet, "c."+n+" = C.size_t(len(d))", "c."+n+"need = 0")
		lines(&b.grow, "if growOK(c."+n+"need, cap(d)) {", "d = slices.Grow(d, int(c."+n+"need)-len(d))", "grown = true", "}")
		b.cargs = append(b.cargs, "(*"+ge+")(unsafe.Pointer(unsafe.SliceData(d[:cap(d)])))", "C.size_t(cap(d))", "&c."+n, "&c."+n+"need")
		g.imports["slices"] = true
		return nil
	}
	if c, ok := g.scalar(t); ok {
		b.cargs = append(b.cargs, g.cgoType(c)+"("+q+")")
		return nil
	}
	if isString(t) {
		b.cargs = append(b.cargs, "(*C.char)(unsafe.Pointer(unsafe.StringData("+q+")))", "C.size_t(len("+q+"))")
		return nil
	}
	if sl, ok := t.(*types.Slice); ok {
		if !g.alias(sl.Elem()) {
			return fmt.Errorf("callback slice of %s", sl.Elem())
		}
		ge, err := g.cgoElem(sl.Elem())
		if err != nil {
			return err
		}
		b.cargs = append(b.cargs, "(*"+ge+")(unsafe.Pointer(unsafe.SliceData("+q+")))", "C.size_t(len("+q+"))")
		return nil
	}
	e := t
	p, ptr := t.(*types.Pointer)
	if ptr {
		e = p.Elem()
	}
	_, isArr := e.(*types.Array)
	_, isSig := e.(*types.Signature)
	if _, isH := g.handleC(e); isH || isSig || (ptr && isArr) || g.needsArena(e) {
		return fmt.Errorf("callback param of %s", t)
	}
	ca, err := g.cgoArray(e)
	if err != nil {
		return err
	}
	a := fmt.Sprintf("%sa%d", b.fp, i)
	b.fields = append(b.fields, a+" "+ca)
	if !ptr {
		addr := "&c." + a
		if isArr {
			addr += "[0]"
		}
		b.cargs = append(b.cargs, addr)
		return g.outVal(&b.pre, e, "c."+a, q, 0)
	}
	lines(&b.pre, fmt.Sprintf("var pa%d *%s", i, ca), "if "+q+" != nil {")
	if err := g.outVal(&b.pre, e, "c."+a, "(*"+q+")", 0); err != nil {
		return err
	}
	lines(&b.pre, fmt.Sprintf("pa%d = &c.%s", i, a), "}")
	b.cargs = append(b.cargs, fmt.Sprintf("pa%d", i))
	return nil
}

func (b *cbuild) result(i int, t types.Type, isAppend bool) error {
	g := b.g
	o := fmt.Sprintf("o%d", i)
	b.gr = append(b.gr, o+" "+g.goType(t))
	r := fmt.Sprintf("%sr%d", b.fp, i)
	switch {
	case isError(t):
		b.errName = o
		return nil
	case isAppend:
		lines(&b.post, o+" = d[:min(c."+fmt.Sprintf("%sn%d", b.fp, b.appendIdx)+", C.size_t(cap(d)))]")
		return nil
	case isString(t) || isSliceAlias(g, t):
		return b.bufResult(i, t, o)
	}
	if c, ok := g.scalar(t); ok {
		b.fields = append(b.fields, r+" "+g.cgoType(c))
		b.zeroed(r, g.cgoType(c))
		b.cargs = append(b.cargs, "&c."+r)
		lines(&b.post, o+" = "+g.goType(t)+"(c."+r+")")
		return nil
	}
	if ti := g.info(t); ti != nil && ti.kind == kOpen {
		b.fields = append(b.fields, r+" *C."+ti.c)
		b.cargs = append(b.cargs, "&c."+r)
		lines(&b.post, "if c."+r+" != nil {", o+" = "+g.getter(ti)+"(c."+r+").v", "deleteHandle(unsafe.Pointer(c."+r+"))", "c."+r+" = nil", "}")
		return nil
	}
	e := t
	p, ptr := t.(*types.Pointer)
	if ptr {
		e = p.Elem()
	}
	if _, isH := g.handleC(e); isH {
		return fmt.Errorf("callback handle result")
	}
	ca, err := g.cgoArray(e)
	if err != nil {
		return err
	}
	b.fields = append(b.fields, r+" "+ca, fmt.Sprintf("%sg%d %s", b.fp, i, g.goType(e)))
	b.zeroed(r, ca)
	addr := "&c." + r
	if _, isArr := e.(*types.Array); isArr {
		addr += "[0]"
	}
	st, err := g.storage(e)
	if err != nil {
		return err
	}
	sv := ""
	if st != "" {
		b.fields = append(b.fields, fmt.Sprintf("%ss%d %s", b.fp, i, st))
		sv = fmt.Sprintf("c.%ss%d", b.fp, i)
	}
	gv := fmt.Sprintf("c.%sg%d", b.fp, i)
	if !ptr {
		b.cargs = append(b.cargs, addr)
		if err := g.inVal(&b.post, e, gv, "c."+r, sv, 0); err != nil {
			return err
		}
		lines(&b.post, o+" = "+gv)
		return nil
	}
	ok := fmt.Sprintf("%sok%d", b.fp, i)
	b.fields = append(b.fields, ok+" C.bool")
	b.cargs = append(b.cargs, addr, "&c."+ok)
	lines(&b.loopSet, "c."+ok+" = false")
	lines(&b.post, "if c."+ok+" {")
	if err := g.inVal(&b.post, e, gv, "c."+r, sv, 0); err != nil {
		return err
	}
	lines(&b.post, o+" = &"+gv, "}")
	return nil
}

func (b *cbuild) bufResult(i int, t types.Type, o string) error {
	g := b.g
	et, ge := "byte", "C.char"
	if !isString(t) {
		e := sliceElem(t)
		et = g.goType(e)
		var err error
		if ge, err = g.cgoElem(e); err != nil {
			return err
		}
	}
	bf := fmt.Sprintf("%sb%d", b.fp, i)
	n := fmt.Sprintf("%sn%d", b.fp, i)
	b.fields = append(b.fields, bf+" []"+et, n+" C.size_t", n+"need C.size_t")
	b.cargs = append(b.cargs, "(*"+ge+")(unsafe.Pointer(unsafe.SliceData(c."+bf+")))", "C.size_t(len(c."+bf+"))", "&c."+n, "&c."+n+"need")
	b.zeroed(n, "C.size_t")
	lines(&b.loopSet, "c."+n+"need = 0")
	lines(&b.grow, "if growOK(c."+n+"need, len(c."+bf+")) {", "c."+bf+" = make([]"+et+", c."+n+"need)", "grown = true", "}")
	if isString(t) {
		lines(&b.post, o+" = string(c."+bf+"[:min(c."+n+", C.size_t(len(c."+bf+")))])")
		return nil
	}
	lines(&b.post, o+" = slices.Clone(c."+bf+"[:min(c."+n+", C.size_t(len(c."+bf+")))])")
	g.imports["slices"] = true
	return nil
}

func isSliceAlias(g *gen, t types.Type) bool {
	s, ok := t.(*types.Slice)
	return ok && g.alias(s.Elem())
}

func (g *gen) vtable(ti *tinfo) (string, string, error) {
	iface := underInterface(ti.named)
	vt := ti.base + "_vtable_t"
	var hb, ib strings.Builder
	var gw strings.Builder
	var fields []string
	var methods strings.Builder
	lines(&hb, "struct "+vt+" {", "\t/* size is sizeof the struct, which the caller sets. A method that it does not cover, or whose function is NULL, fails with DMRGO_ERR_NO_METHOD, or gives zero results when it has no error. A function that returns an error gives zero results. */", "\tsize_t size;", "\tDMRGO_USER_T user;")
	var plain strings.Builder
	lines(&plain, "struct "+vt+" {", "\tsize_t size;", "\tDMRGO_USER_T user;")
	ms := make([]*types.Func, 0, iface.NumMethods())
	for i := 0; i < iface.NumMethods(); i++ {
		ms = append(ms, iface.Method(i))
	}
	if old := g.lock.sizedLine(vt); old != "" {
		at := func(m *types.Func) int {
			if i := strings.Index(old, "(*"+cKeyword(snake(m.Name()))+")("); i >= 0 {
				return i
			}
			return len(old)
		}
		slices.SortStableFunc(ms, func(a, b *types.Func) int { return at(a) - at(b) })
	}
	for i, m := range ms {
		sig := m.Signature()
		g.fillBuf = isRead(m)
		cps, err := g.cParams(sig)
		g.fillBuf = false
		if err != nil {
			return "", "", fmt.Errorf("method %s: %w", m.Name(), err)
		}
		mn := cKeyword(snake(m.Name()))
		if mn == "user" || mn == "size" {
			mn += "_fn"
		}
		all := append([]string{userParam}, cps...)
		if d := g.l.docs[m]; d != "" {
			lines(&hb, "\t/* "+d+" */")
		}
		lines(&hb, "\tint (*"+mn+")("+strings.Join(all, ", ")+");")
		lines(&plain, "\tint (*"+mn+")("+strings.Join(all, ", ")+");")
		var names []string
		for _, p := range cps {
			names = append(names, cName(p))
		}
		tr := vt + "__" + mn
		lines(&ib, "static inline int "+tr+"("+strings.Join(append([]string{"const " + vt + " *t"}, cps...), ", ")+") {",
			"\tif (!t->"+mn+")", "\t\treturn DMRGO_ERR_NO_METHOD;",
			"\treturn t->"+mn+"("+strings.Join(append([]string{"t->user"}, names...), ", ")+");", "}", "")
		body, fs, err := g.cbBody(sig, "C."+tr+"(&c.t", fmt.Sprintf("m%d", i))
		if err != nil {
			return "", "", fmt.Errorf("method %s: %w", m.Name(), err)
		}
		fields = append(fields, fs...)
		methods.WriteString("func (c *vt_" + ti.id + ") " + m.Name() + body)
	}
	lines(&hb, "};", "")
	lines(&plain, "};")
	lines(&gw, "type vt_"+ti.id+" struct {", "t C."+vt)
	for _, f := range fields {
		lines(&gw, f)
	}
	lines(&gw, "}", "")
	gw.WriteString(methods.String())
	cname := ti.base + "_from_vtable"
	lines(&gw, "//export "+cname, "func "+cname+"(c_vt *C."+vt+", c_out **C."+ti.c+") (rc C.int) {", "defer recoverTo(&rc)",
		"if c_vt == nil || c_out == nil {", "return fail(C.DMRGO_ERR_ARG)", "}", "var z C."+vt,
		"if !sizedIn(unsafe.Pointer(&z), unsafe.Pointer(c_vt), unsafe.Sizeof(z)) {", "return fail(C.DMRGO_ERR_ARG)", "}",
		"*c_out = "+g.newer(ti)+"(&vt_"+ti.id+"{t: z})", "return C.DMRGO_OK", "}", "")
	g.fwd = append(g.fwd, "typedef struct "+vt+" "+vt+";")
	g.hStructs.WriteString(hb.String())
	g.abiTypes = append(g.abiTypes, "type "+strings.Join(strings.Fields(plain.String()), " "))
	g.internal.WriteString(ib.String())
	g.goConv.WriteString(gw.String())
	proto := "/* " + cname + " makes a " + ti.obj.Name() + " whose methods call the functions of vt with vt->user. */\nint " + cname + "(const " + vt + " *vt, " + ti.c + " **out);\n"
	return cname, proto, nil
}

func isRead(m *types.Func) bool {
	sig := m.Signature()
	if m.Name() != "Read" || sig.Params().Len() != 1 || sig.Results().Len() != 2 {
		return false
	}
	sl, ok := sig.Params().At(0).Type().(*types.Slice)
	if !ok || !types.Identical(sl.Elem(), types.Typ[types.Byte]) {
		return false
	}
	n, ok := sig.Results().At(0).Type().(*types.Basic)
	return ok && n.Kind() == types.Int && isError(sig.Results().At(1).Type())
}
