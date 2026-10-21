package capigen

import (
	"fmt"
	"go/types"
	"strings"
)

type member struct {
	name  string
	field *types.Var
	decls []string
}

func lines(w *strings.Builder, s ...string) {
	for _, l := range s {
		w.WriteString(l)
		w.WriteByte('\n')
	}
}

func (g *gen) members(ti *tinfo) ([]member, error) {
	s := underStruct(ti.named)
	var out []member
	for f := range s.Fields() {
		if !f.Exported() {
			continue
		}
		m := member{name: cKeyword(snake(f.Name())), field: f}
		cur := g.cur
		g.cur = ti.base + "_" + m.name
		d, err := g.fieldDecls(m.name, f.Type())
		g.cur = cur
		if err != nil {
			return nil, fmt.Errorf("field %s: %w", f.Name(), err)
		}
		m.decls = d
		out = append(out, m)
	}
	return out, nil
}

func (g *gen) fieldDecls(n string, t types.Type) ([]string, error) {
	switch u := t.(type) {
	case *types.Basic:
		if u.Kind() == types.String {
			return []string{"char *" + n, "size_t " + n + "_len"}, nil
		}
	case *types.Slice:
		e, err := g.elemDecl(u.Elem(), "*"+n)
		if err != nil {
			return nil, err
		}
		return []string{e, "size_t " + n + "_len"}, nil
	case *types.Pointer:
		if c, ok := g.handleC(u.Elem()); ok {
			return []string{c + " *" + n}, nil
		}
		d, err := g.decl(u.Elem(), n)
		if err != nil {
			return nil, err
		}
		return []string{"bool has_" + n, d}, nil
	case *types.Named:
		if c, ok := g.handleC(u); ok && g.kindOf(u) == kOpen {
			return []string{c + " *" + n}, nil
		}
	}
	d, err := g.decl(t, n)
	if err != nil {
		return nil, err
	}
	return []string{d}, nil
}

func (g *gen) elemDecl(t types.Type, n string) (string, error) {
	if b, ok := t.(*types.Basic); ok && b.Kind() == types.String {
		return "dmrgo_string_t " + n, nil
	}
	if p, ok := t.(*types.Pointer); ok {
		if _, isH := g.handleC(p.Elem()); !isH {
			t = p.Elem()
		}
	}
	if c, ok := g.handleC(t); ok {
		return c + " *" + n, nil
	}
	if _, ok := t.(*types.Array); ok {
		return g.decl(t, "("+n+")")
	}
	return g.decl(t, n)
}

func isString(t types.Type) bool {
	b, ok := t.(*types.Basic)
	return ok && b.Kind() == types.String
}

func isBool(t types.Type) bool {
	b, ok := t.Underlying().(*types.Basic)
	return ok && b.Kind() == types.Bool
}

func (g *gen) storage(t types.Type) (string, error) {
	switch u := t.(type) {
	case *types.Array:
		s, err := g.storage(u.Elem())
		if err != nil || s == "" {
			return "", err
		}
		return fmt.Sprintf("[%d]%s", u.Len(), s), nil
	case *types.Named:
		ti := g.info(u)
		if ti == nil {
			return "", nil
		}
		switch ti.kind {
		case kValue:
			return g.structStorage(ti)
		case kClosed:
			if err := g.unionStorage(ti); err != nil {
				return "", err
			}
			return "st_" + ti.id, nil
		case kNArray:
			return g.storage(u.Underlying())
		default:
			return "", nil
		}
	default:
		return "", nil
	}
}

func (g *gen) structStorage(ti *tinfo) (string, error) {
	key := "st:" + ti.id
	if v, ok := g.memo[key]; ok {
		return v.s, v.err
	}
	g.memo[key] = memo{}
	ms, err := g.members(ti)
	if err != nil {
		g.memo[key] = memo{err: err}
		return "", err
	}
	var fs []string
	for _, m := range ms {
		ft := m.field.Type()
		if p, ok := ft.(*types.Pointer); ok {
			if _, isH := g.handleC(p.Elem()); isH {
				continue
			}
			fs = append(fs, "p_"+m.field.Name()+" "+g.goType(p.Elem()))
			s, err := g.storage(p.Elem())
			if err != nil {
				g.memo[key] = memo{err: err}
				return "", err
			}
			if s != "" {
				fs = append(fs, "s_"+m.field.Name()+" "+s)
			}
			continue
		}
		if _, ok := ft.(*types.Slice); ok {
			continue
		}
		s, err := g.storage(ft)
		if err != nil {
			g.memo[key] = memo{err: err}
			return "", err
		}
		if s != "" {
			fs = append(fs, "f_"+m.field.Name()+" "+s)
		}
	}
	if len(fs) == 0 {
		return "", nil
	}
	name := "st_" + ti.id
	var w strings.Builder
	lines(&w, "type "+name+" struct {")
	for _, f := range fs {
		lines(&w, "\t"+f)
	}
	lines(&w, "}", "")
	g.goConv.WriteString(w.String())
	g.memo[key] = memo{s: name}
	return name, nil
}

func (g *gen) unionStorage(ti *tinfo) error {
	key := "st:" + ti.id
	if v, ok := g.memo[key]; ok {
		return v.err
	}
	g.memo[key] = memo{s: "st_" + ti.id}
	var w strings.Builder
	lines(&w, "type st_"+ti.id+" struct {")
	for i, im := range ti.impls {
		lines(&w, fmt.Sprintf("\tv%d %s", i, g.goType(im)))
		s, err := g.storage(im)
		if err != nil {
			g.memo[key] = memo{err: err}
			return err
		}
		if s != "" {
			lines(&w, fmt.Sprintf("\ts%d %s", i, s))
		}
	}
	lines(&w, "}", "")
	g.goConv.WriteString(w.String())
	return nil
}

func (g *gen) zeroLit(c string, t types.Type) string {
	if _, ok := g.scalar(t); ok {
		if isBool(t) {
			return cFalse
		}
		return "0"
	}
	return c + "{}"
}

func (g *gen) cgoArray(t types.Type) (string, error) {
	if a, ok := t.(*types.Array); ok {
		e, err := g.cgoArray(a.Elem())
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("[%d]%s", a.Len(), e), nil
	}
	c, err := g.ctype(t)
	if err != nil {
		return "", err
	}
	return g.cgoType(c), nil
}

func (g *gen) inVal(w *strings.Builder, t types.Type, dst, src, st string, depth int) error {
	if _, ok := g.scalar(t); ok {
		lines(w, dst+" = "+g.goType(t)+"("+src+")")
		return nil
	}
	if g.alias(t) {
		lines(w, dst+" = *(*"+g.goType(t)+")(unsafe.Pointer(&"+src+"))")
		return nil
	}
	if s, ok := t.(*types.Signature); ok {
		f, err := g.fnType(s, "")
		if err != nil {
			return err
		}
		lines(w, dst+" = "+f.in+"("+src+")")
		return nil
	}
	if a, ok := t.(*types.Array); ok {
		i := fmt.Sprintf("i%d", depth)
		sti := ""
		if st != "" {
			sti = st + "[" + i + "]"
		}
		lines(w, "for "+i+" := range "+dst+" {")
		if err := g.inVal(w, a.Elem(), dst+"["+i+"]", src+"["+i+"]", sti, depth+1); err != nil {
			return err
		}
		lines(w, "}")
		return nil
	}
	ti := g.info(t)
	if ti == nil {
		return fmt.Errorf("no C form for %s", t)
	}
	switch ti.kind {
	case kNArray:
		u := underArray(ti.named)
		return g.inVal(w, u, "(*"+g.goType(u)+")(&"+dst+")", src, st, depth)
	case kValue, kClosed:
		fn, err := g.inFn(ti)
		if err != nil {
			return err
		}
		if st != "" {
			lines(w, dst+" = "+fn+"(&"+src+", &"+st+")")
		} else {
			lines(w, dst+" = "+fn+"(&"+src+")")
		}
		return nil
	case kNFunc:
		f, err := g.fnType(underSignature(ti.named), ti.c)
		if err != nil {
			return err
		}
		lines(w, dst+" = "+g.goType(t)+"("+f.in+"("+src+"))")
		return nil
	default:
		return fmt.Errorf("no C value form for %s", t)
	}
}

func (g *gen) outVal(w *strings.Builder, t types.Type, dst, src string, depth int) error {
	if c, ok := g.scalar(t); ok {
		lines(w, dst+" = "+g.cgoType(c)+"("+src+")")
		return nil
	}
	if g.alias(t) {
		lines(w, "*(*"+g.goType(t)+")(unsafe.Pointer(&"+dst+")) = "+src)
		return nil
	}
	if s, ok := t.(*types.Signature); ok {
		f, err := g.fnType(s, "")
		if err != nil {
			return err
		}
		if f.out == "" {
			return fmt.Errorf("func %s has no C output form", s)
		}
		lines(w, dst+" = "+f.out+"("+src+")")
		return nil
	}
	if a, ok := t.(*types.Array); ok {
		i := fmt.Sprintf("i%d", depth)
		lines(w, "for "+i+" := range "+src+" {")
		if err := g.outVal(w, a.Elem(), dst+"["+i+"]", src+"["+i+"]", depth+1); err != nil {
			return err
		}
		lines(w, "}")
		return nil
	}
	ti := g.info(t)
	if ti == nil {
		return fmt.Errorf("no C form for %s", t)
	}
	switch ti.kind {
	case kNArray:
		u := underArray(ti.named)
		return g.outVal(w, u, dst, "(*"+g.goType(u)+")(&"+src+")", depth)
	case kValue:
		fn, err := g.outFn(ti)
		if err != nil {
			return err
		}
		lines(w, fn+"(&"+dst+", &"+src+", oc)")
		return nil
	case kClosed:
		fn, err := g.outFn(ti)
		if err != nil {
			return err
		}
		lines(w, fn+"(&"+dst+", "+src+", oc)")
		return nil
	case kNFunc:
		f, err := g.fnType(underSignature(ti.named), ti.c)
		if err != nil {
			return err
		}
		if f.out == "" {
			return fmt.Errorf("func %s has no C output form", ti.obj.Name())
		}
		lines(w, dst+" = "+f.out+"("+src+")")
		return nil
	default:
		return fmt.Errorf("no C value form for %s", t)
	}
}

func (g *gen) inField(w *strings.Builder, t types.Type, d, s, n, st string) error {
	switch u := t.(type) {
	case *types.Basic:
		if u.Kind() == types.String {
			lines(w, d+" = C.GoStringN("+s+"."+n+", C.int("+s+"."+n+"_len))")
			return nil
		}
	case *types.Slice:
		return g.inSlice(w, u, d, s+"."+n, s+"."+n+"_len")
	case *types.Pointer:
		if ti := g.info(u.Elem()); ti != nil && (ti.kind == kHandle || ti.kind == kOpen) {
			lines(w, d+" = nil", "if "+s+"."+n+" != nil {", d+" = "+g.getter(ti)+"("+s+"."+n+").v", "}")
			return nil
		}
		lines(w, "if "+s+".has_"+n+" {")
		if err := g.inVal(w, u.Elem(), st+".p_"+strings.TrimPrefix(d, "d."), s+"."+n, g.subStorage(st, "s_", strings.TrimPrefix(d, "d."), u.Elem()), 0); err != nil {
			return err
		}
		lines(w, d+" = &"+st+".p_"+strings.TrimPrefix(d, "d."), "} else {", d+" = nil", "}")
		return nil
	case *types.Named:
		if ti := g.info(u); ti != nil && ti.kind == kOpen {
			lines(w, d+" = nil", "if "+s+"."+n+" != nil {", d+" = "+g.getter(ti)+"("+s+"."+n+").v", "}")
			return nil
		}
	}
	return g.inVal(w, t, d, s+"."+n, g.subStorage(st, "f_", strings.TrimPrefix(d, "d."), t), 0)
}

func (g *gen) subStorage(st, pfx, field string, t types.Type) string {
	s, err := g.storage(t)
	if err != nil || s == "" || st == "" {
		return ""
	}
	return st + "." + pfx + field
}

func (g *gen) inSlice(w *strings.Builder, u *types.Slice, d, ptr, n string) error {
	e := u.Elem()
	if g.alias(e) {
		lines(w, d+" = unsafe.Slice((*"+g.goType(e)+")(unsafe.Pointer("+ptr+")), int("+n+"))")
		return nil
	}
	lines(w, d+" = nil", "if "+n+" > 0 {")
	ge, err := g.cgoElem(e)
	if err != nil {
		return err
	}
	lines(w, "cs := unsafe.Slice((*"+ge+")(unsafe.Pointer("+ptr+")), int("+n+"))")
	lines(w, d+" = make("+g.goType(u)+", len(cs))")
	if isString(e) {
		lines(w, "for i := range cs {", d+"[i] = C.GoStringN(cs[i].ptr, C.int(cs[i].len))", "}", "}")
		return nil
	}
	if p, ok := e.(*types.Pointer); ok {
		if ti := g.info(p.Elem()); ti != nil && (ti.kind == kHandle || ti.kind == kOpen) {
			lines(w, "for i := range cs {", d+"[i] = "+g.getter(ti)+"(cs[i]).v", "}", "}")
			return nil
		}
		lines(w, "vs := make([]"+g.goType(p.Elem())+", len(cs))")
		s, err := g.storage(p.Elem())
		if err != nil {
			return err
		}
		sti := ""
		if s != "" {
			lines(w, "ss := make([]"+s+", len(cs))")
			sti = "ss[i]"
		}
		lines(w, "for i := range cs {")
		if err := g.inVal(w, p.Elem(), "vs[i]", "cs[i]", sti, 1); err != nil {
			return err
		}
		lines(w, d+"[i] = &vs[i]", "}", "}")
		return nil
	}
	if ti := g.info(e); ti != nil && (ti.kind == kHandle || ti.kind == kOpen) {
		lines(w, "for i := range cs {", d+"[i] = *"+g.getter(ti)+"(cs[i]).v", "}", "}")
		return nil
	}
	s, err := g.storage(e)
	if err != nil {
		return err
	}
	sti := ""
	if s != "" {
		lines(w, "ss := make([]"+s+", len(cs))")
		sti = "ss[i]"
	}
	lines(w, "for i := range cs {")
	if err := g.inVal(w, e, d+"[i]", "cs[i]", sti, 1); err != nil {
		return err
	}
	lines(w, "}", "}")
	return nil
}

func (g *gen) outField(w *strings.Builder, t types.Type, d, n, s string) error {
	switch u := t.(type) {
	case *types.Basic:
		if u.Kind() == types.String {
			lines(w, d+"."+n+" = arenaStr(oc, "+s+")", d+"."+n+"_len = C.size_t(len("+s+"))")
			return nil
		}
	case *types.Slice:
		return g.outArena(w, u, d+"."+n, d+"."+n+"_len", s)
	case *types.Pointer:
		if _, ok := g.handleC(u.Elem()); ok {
			lines(w, "if "+s+" != nil {", "panic(argPanic{})", "}", d+"."+n+" = nil")
			return nil
		}
		c, err := g.cgoArray(u.Elem())
		if err != nil {
			return err
		}
		lines(w, "if "+s+" != nil {", d+".has_"+n+" = true")
		if err := g.outVal(w, u.Elem(), d+"."+n, "(*"+s+")", 0); err != nil {
			return err
		}
		lines(w, "} else {", d+".has_"+n+" = false", d+"."+n+" = "+g.zeroLit(c, u.Elem()), "}")
		return nil
	case *types.Named:
		if ti := g.info(u); ti != nil && ti.kind == kOpen {
			lines(w, "if "+s+" != nil {", "panic(argPanic{})", "}", d+"."+n+" = nil")
			return nil
		}
	}
	return g.outVal(w, t, d+"."+n, s, 0)
}

func (g *gen) outSlice(w *strings.Builder, u *types.Slice, ptr, capx, lenp, needp, s string) error {
	e := u.Elem()
	if g.alias(e) {
		lines(w, "putSlice(unsafe.Slice((*"+g.goType(e)+")(unsafe.Pointer("+ptr+")), int("+capx+")), "+lenp+", "+needp+", "+s+", oc)")
		return nil
	}
	ge, err := g.cgoElem(e)
	if err != nil {
		return err
	}
	lines(w, "{", "src := "+s, "*("+lenp+") = C.size_t(min(len(src), int("+capx+")))", "if "+needp+" != nil {", "*"+needp+" = C.size_t(len(src))", "}", "if len(src) > int("+capx+") {", "oc.short = true", "}",
		"if "+ptr+" != nil {", "cs := unsafe.Slice((*"+ge+")(unsafe.Pointer("+ptr+")), int("+capx+"))", "for i := range min(len(src), len(cs)) {")
	if err := g.outElem(w, e, "cs[i]", "src[i]"); err != nil {
		return err
	}
	lines(w, "}", "}", "}")
	return nil
}

func (g *gen) outElem(w *strings.Builder, e types.Type, dst, src string) error {
	if isString(e) {
		lines(w, dst+".ptr = arenaStr(oc, "+src+")", dst+".len = C.size_t(len("+src+"))")
		return nil
	}
	if p, ok := e.(*types.Pointer); ok {
		if _, isH := g.handleC(p.Elem()); isH {
			return fmt.Errorf("a slice of handles has no C output form")
		}
		e, src = p.Elem(), "(*"+src+")"
	}
	if _, isH := g.handleC(e); isH {
		return fmt.Errorf("a slice of handles has no C output form")
	}
	return g.outVal(w, e, dst, src, 1)
}

func (g *gen) outArena(w *strings.Builder, u *types.Slice, ptr, n, s string) error {
	e := u.Elem()
	ge, err := g.cgoElem(e)
	if err != nil {
		return err
	}
	if g.alias(e) {
		lines(w, ptr+" = (*"+ge+")(arenaCopy(oc, "+s+"))", n+" = C.size_t(len("+s+"))")
		return nil
	}
	lines(w, "{", "src := "+s, n+" = C.size_t(len(src))", ptr+" = nil", "cs := arenaTake["+ge+"](oc, len(src))", "if cs != nil {", ptr+" = &cs[0]", "for i := range src {")
	if err := g.outElem(w, e, "cs[i]", "src[i]"); err != nil {
		return err
	}
	lines(w, "}", "}", "}")
	return nil
}

func (g *gen) inFn(ti *tinfo) (string, error) {
	key := "in:" + ti.id
	if v, ok := g.memo[key]; ok {
		return v.s, v.err
	}
	name := "in_" + ti.id
	g.memo[key] = memo{s: name}
	var w strings.Builder
	var err error
	if ti.kind == kClosed {
		err = g.unionIn(&w, ti, name)
	} else {
		err = g.structIn(&w, ti, name)
	}
	if err != nil {
		g.memo[key] = memo{err: err}
		return "", err
	}
	g.goConv.WriteString(w.String())
	return name, nil
}

func (g *gen) structIn(w *strings.Builder, ti *tinfo, name string) error {
	ms, err := g.members(ti)
	if err != nil {
		return err
	}
	st, err := g.storage(ti.named)
	if err != nil {
		return err
	}
	if err := g.cStruct(ti); err != nil {
		return err
	}
	sig := "func " + name + "(s *C." + ti.c
	if st != "" {
		sig += ", st *" + st
	}
	lines(w, sig+") (d "+g.goType(ti.named)+") {")
	if len(ms) == 0 {
		lines(w, "_ = s")
	}
	for _, m := range ms {
		stv := ""
		if st != "" {
			stv = "st"
		}
		if err := g.inField(w, m.field.Type(), "d."+m.field.Name(), "s", m.name, stv); err != nil {
			return fmt.Errorf("field %s: %w", m.field.Name(), err)
		}
	}
	lines(w, "return", "}", "")
	return nil
}

func (g *gen) unionIn(w *strings.Builder, ti *tinfo, name string) error {
	if err := g.unionStorage(ti); err != nil {
		return err
	}
	if err := g.cUnion(ti); err != nil {
		return err
	}
	lines(w, "func "+name+"(s *C."+ti.c+", st *st_"+ti.id+") (d "+g.goType(ti.named)+") {", "switch s.tag {", "case 0:", "d = nil")
	for i, im := range ti.impls {
		ii := g.info(im)
		fn, err := g.inFn(ii)
		if err != nil {
			return err
		}
		s, err := g.storage(im)
		if err != nil {
			return err
		}
		lines(w, fmt.Sprintf("case %d:", g.lock.tag(ti.c, g.tagName(ti, ii))))
		arg := fmt.Sprintf("(*C.%s)(unsafe.Pointer(&s.v))", ii.c)
		if s != "" {
			arg += fmt.Sprintf(", &st.s%d", i)
		}
		lines(w, fmt.Sprintf("st.v%d = %s(%s)", i, fn, arg), fmt.Sprintf("d = &st.v%d", i))
	}
	lines(w, "default:", "panic(argPanic{})", "}", "return", "}", "")
	return nil
}

func (g *gen) unionInline(w *strings.Builder, ti *tinfo, dst, src string) error {
	if err := g.cUnion(ti); err != nil {
		return err
	}
	lines(w, "switch "+src+".tag {", "case 0:", dst+" = nil")
	for i, im := range ti.impls {
		ii := g.info(im)
		fn, err := g.inFn(ii)
		if err != nil {
			return err
		}
		s, err := g.storage(im)
		if err != nil {
			return err
		}
		lines(w, fmt.Sprintf("case %d:", g.lock.tag(ti.c, g.tagName(ti, ii))))
		arg := fmt.Sprintf("(*C.%s)(unsafe.Pointer(&%s.v))", ii.c, src)
		if s != "" {
			lines(w, fmt.Sprintf("var us%d %s", i, s))
			arg += fmt.Sprintf(", &us%d", i)
		}
		lines(w, fmt.Sprintf("uv%d := %s(%s)", i, fn, arg), fmt.Sprintf("%s = &uv%d", dst, i))
	}
	lines(w, "default:", "return fail(C.DMRGO_ERR_ARG)", "}")
	return nil
}

func (g *gen) tagName(u, im *tinfo) string {
	return strings.ToUpper(u.base + "_" + strings.TrimPrefix(im.base, "dmrgo_"))
}

func (g *gen) outFn(ti *tinfo) (string, error) {
	key := "out:" + ti.id
	if v, ok := g.memo[key]; ok {
		return v.s, v.err
	}
	name := "out_" + ti.id
	g.memo[key] = memo{s: name}
	var w strings.Builder
	var err error
	if ti.kind == kClosed {
		err = g.unionOut(&w, ti, name)
	} else {
		err = g.structOut(&w, ti, name)
	}
	if err != nil {
		g.memo[key] = memo{err: err}
		return "", err
	}
	g.goConv.WriteString(w.String())
	return name, nil
}

func (g *gen) structOut(w *strings.Builder, ti *tinfo, name string) error {
	ms, err := g.members(ti)
	if err != nil {
		return err
	}
	if err := g.cStruct(ti); err != nil {
		return err
	}
	lines(w, "func "+name+"(d *C."+ti.c+", s *"+g.goType(ti.named)+", oc *outc) {")
	lines(w, "_ = oc")
	if len(ms) == 0 {
		lines(w, "_ = d", "_ = s")
	}
	for _, m := range ms {
		if err := g.outField(w, m.field.Type(), "d", m.name, "s."+m.field.Name()); err != nil {
			return fmt.Errorf("field %s: %w", m.field.Name(), err)
		}
	}
	lines(w, "}", "")
	return nil
}

func (g *gen) unionOut(w *strings.Builder, ti *tinfo, name string) error {
	if err := g.cUnion(ti); err != nil {
		return err
	}
	lines(w, "func "+name+"(d *C."+ti.c+", s "+g.goType(ti.named)+", oc *outc) {", "switch x := s.(type) {", "case nil:", "d.tag = 0")
	for _, im := range ti.impls {
		ii := g.info(im)
		fn, err := g.outFn(ii)
		if err != nil {
			return err
		}
		tag := g.lock.tag(ti.c, g.tagName(ti, ii))
		lines(w, "case *"+g.goType(im)+":", fmt.Sprintf("d.tag = %d", tag), fn+"((*C."+ii.c+")(unsafe.Pointer(&d.v)), x, oc)")
		if types.Implements(im, underInterface(ti.named)) {
			lines(w, "case "+g.goType(im)+":", fmt.Sprintf("d.tag = %d", tag), fn+"((*C."+ii.c+")(unsafe.Pointer(&d.v)), &x, oc)")
		}
	}
	lines(w, "default:", "d.tag = 0", "}", "}", "")
	return nil
}

func (g *gen) cStruct(ti *tinfo) error {
	key := "h:" + ti.id
	if v, ok := g.memo[key]; ok {
		return v.err
	}
	g.memo[key] = memo{}
	var b strings.Builder
	switch ti.kind {
	default:
	case kNArray:
		d, err := g.decl(ti.named.Underlying(), "v")
		if err != nil {
			g.memo[key] = memo{err: err}
			return err
		}
		if err := g.needDeps(ti.named.Underlying()); err != nil {
			g.memo[key] = memo{err: err}
			return err
		}
		lines(&b, "struct "+ti.c+" {", "\t"+d+";", "};", "")
	case kValue:
		ms, err := g.members(ti)
		if err != nil {
			g.memo[key] = memo{err: err}
			return err
		}
		lines(&b, "struct "+ti.c+" {")
		if len(ms) == 0 {
			lines(&b, "\tuint8_t unused;")
		}
		for _, m := range ms {
			if err := g.needDeps(m.field.Type()); err != nil {
				g.memo[key] = memo{err: err}
				return err
			}
			for _, d := range m.decls {
				lines(&b, "\t"+d+";")
			}
		}
		lines(&b, "};", "")
	}
	g.hStructs.WriteString(b.String())
	g.abiTypes = append(g.abiTypes, "type "+strings.Join(strings.Fields(b.String()), " "))
	return nil
}

func (g *gen) cUnion(ti *tinfo) error {
	key := "h:" + ti.id
	if v, ok := g.memo[key]; ok {
		return v.err
	}
	g.memo[key] = memo{}
	var b strings.Builder
	lines(&b, "struct "+ti.c+" {", "\tuint32_t tag;", "\tunion {")
	for _, im := range ti.impls {
		ii := g.info(im)
		if err := g.cStruct(ii); err != nil {
			g.memo[key] = memo{err: err}
			return err
		}
		lines(&b, "\t\t"+ii.c+" "+strings.TrimPrefix(ii.base, "dmrgo_")+";")
	}
	lines(&b, "\t} v;", "};", "")
	for _, im := range ti.impls {
		ii := g.info(im)
		g.defines = append(g.defines, fmt.Sprintf("#define %s %d", g.tagName(ti, ii), g.lock.tag(ti.c, g.tagName(ti, ii))))
	}
	g.hStructs.WriteString(b.String())
	g.abiTypes = append(g.abiTypes, "type "+strings.Join(strings.Fields(b.String()), " "))
	return nil
}

func (g *gen) needDeps(t types.Type) error {
	switch u := t.(type) {
	case *types.Array:
		return g.needDeps(u.Elem())
	case *types.Pointer:
		if _, isH := g.handleC(u.Elem()); isH {
			return nil
		}
		return g.needDeps(u.Elem())
	case *types.Signature:
		_, err := g.fnType(u, "")
		return err
	case *types.Named:
		ti := g.info(u)
		if ti == nil {
			return nil
		}
		switch ti.kind {
		case kNArray, kValue:
			return g.cStruct(ti)
		case kClosed:
			return g.cUnion(ti)
		case kNFunc:
			_, err := g.fnType(underSignature(u), ti.c)
			return err
		default:
			return nil
		}
	default:
		return nil
	}
}
