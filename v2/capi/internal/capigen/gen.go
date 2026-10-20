// Package capigen generates the C API of dmrgo from the exported Go API of
// the library: api_gen.go (the cgo exports and the converters), dmrgo_gen.h
// (the C declarations), dmrgo_gen_internal.h and dmrgo_gen.c (trampolines),
// and abi.txt (the lock of the C ABI). Every exported type, field, func,
// method, const and error var of the packages under v2, except internal/,
// cmd/, examples/ and capi/, gets a C counterpart, and Exports reports any
// that could not be mapped.
package capigen

import (
	"bytes"
	"fmt"
	"go/constant"
	"go/format"
	"go/types"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
)

// Options holds the options of Generate.
type Options struct {
	// Break allows a change of the C ABI: Generate then bumps
	// DMRGO_ABI_VERSION instead of failing.
	Break bool
}

// Export is one exported Go identifier and its C counterparts.
type Export struct {
	// Go is the Go name, such as "pdu.DecodeCSBK" or "layer2.Burst.Data".
	Go string
	// Kind is "type", "field", "func", "method", "const" or "var".
	Kind string
	// C holds the C names that map it. It is empty when Err is set.
	C []string
	// Err says why the export has no C counterpart.
	Err string
}

// Output holds the generated files.
type Output struct {
	Go, H, Internal, C, Lock, Map []byte
	// Exports lists every export of the library in package order.
	Exports []Export
	// Broken lists the lock lines that the API no longer matches.
	Broken []string
	// Unbounded lists the stateful calls whose Go docs give no bound for
	// one call, so that their C form can change state and still return
	// DMRGO_ERR_SHORT.
	Unbounded []string
}

// Files maps the file names of Output, relative to the capi directory.
func (o *Output) Files() map[string][]byte {
	return map[string][]byte{
		"api_gen.go": o.Go, "dmrgo_gen.h": o.H, "dmrgo_gen_internal.h": o.Internal, "dmrgo_gen.c": o.C, "abi.txt": o.Lock,
		"libdmrgo.map": o.Map,
	}
}

type memo struct {
	s   string
	err error
}

type errEntry struct {
	name  string
	code  string
	goRef string
	ti    *tinfo
	ptr   bool
	val   bool
}

type gen struct {
	l           *loader
	pkgs        []*types.Package
	closed      map[string]bool
	infos       map[*types.TypeName]*tinfo
	order       []*tinfo
	imports     map[string]bool
	memo        map[string]memo
	fns         []*fnInfo
	cur         string
	cnames      map[string]string
	usedHandles map[*tinfo]bool
	scratch     map[*tinfo][]string
	curMethod   *types.Func
	curRecv     *tinfo
	unbounded   []string
	lock        *abiLock
	fwd         []string
	defines     []string
	errs        []errEntry
	exports     []Export
	vtableErrs  []string
	funcSyms    []string
	capiDir     string
	collisions  []string

	hScalars, hStructs, hConsts, hProtos strings.Builder
	goConv, goFuncs, internal, genC      strings.Builder
	preamble                             strings.Builder
	abiTypes, abiSyms                    []string
}

// Generate generates the C API for the capi directory capiDir, whose parent
// is the v2 module. lock is the current abi.txt, or nil.
func Generate(capiDir string, lock []byte, opt Options) (*Output, error) {
	l := newLoader(filepath.Join(capiDir, ".."))
	pkgs, err := l.packages()
	if err != nil {
		return nil, err
	}
	al, err := parseLock(lock)
	if err != nil {
		return nil, err
	}
	g := &gen{
		l: l, pkgs: pkgs, closed: map[string]bool{"layer2.Data": true}, infos: map[*types.TypeName]*tinfo{},
		imports: map[string]bool{}, memo: map[string]memo{}, cnames: map[string]string{},
		usedHandles: map[*tinfo]bool{}, scratch: map[*tinfo][]string{}, lock: al, capiDir: capiDir,
	}
	g.classify()
	for name := range g.closed {
		found := false
		for _, ti := range g.order {
			if ti.obj.Pkg().Name()+"."+ti.obj.Name() == name {
				found = true
			}
		}
		if !found {
			return nil, fmt.Errorf("closed interface %s does not exist", name)
		}
	}
	names := map[string]string{}
	for _, p := range pkgs {
		if o, ok := names[g.prefix(p)]; ok {
			return nil, fmt.Errorf("packages %s and %s have the same C prefix", o, p.Path())
		}
		names[g.prefix(p)] = p.Path()
	}
	for _, p := range pkgs {
		sc := p.Scope()
		for _, name := range sc.Names() {
			o := sc.Lookup(name)
			if !o.Exported() {
				continue
			}
			if err := g.object(p, o); err != nil {
				return nil, err
			}
		}
	}
	return g.finish(opt)
}

func (g *gen) goName(p *types.Package, n string) string {
	return p.Name() + "." + n
}

func (g *gen) doc(o types.Object, goName string) string {
	d := g.l.docs[o]
	if d == "" {
		return "Go: " + goName + "."
	}
	return "Go: " + goName + ". " + d
}

func (g *gen) claim(c, goName string) error {
	if o, ok := g.cnames[c]; ok {
		g.collisions = append(g.collisions, fmt.Sprintf("C name %s maps both %s and %s", c, o, goName))
		return nil
	}
	g.cnames[c] = goName
	return nil
}

func (g *gen) addFunc(e *Export, cname, doc string, recv *tinfo, rk recvKind, call string, sig *types.Signature, ld *lead) error {
	if err := g.claim(cname, e.Go); err != nil {
		return err
	}
	g.cur = cname
	code, proto, werr := g.wrapper(cname, doc, recv, rk, call, sig, ld)
	if werr == nil {
		g.goFuncs.WriteString(code)
		g.funcSyms = append(g.funcSyms, cname)
		g.hProtos.WriteString(proto)
		g.abiSyms = append(g.abiSyms, "sym "+strings.TrimSpace(proto[strings.LastIndex(strings.TrimSpace(proto), "\n")+1:]))
		e.C = append(e.C, cname)
	} else {
		e.Err = werr.Error()
		e.C = nil
	}
	return nil
}

func (g *gen) object(p *types.Package, o types.Object) error {
	gn := g.goName(p, o.Name())
	switch o := o.(type) {
	case *types.Const:
		e := Export{Go: gn, Kind: "const"}
		def, err := g.constant(p, o)
		if err != nil {
			e.Err = err.Error()
		} else {
			c := strings.Fields(def)[1]
			if err := g.claim(c, gn); err != nil {
				return err
			}
			e.C = []string{c}
			g.hConsts.WriteString("/* " + g.doc(o, gn) + " */\n" + def + "\n")
			g.abiSyms = append(g.abiSyms, "sym "+def)
		}
		g.exports = append(g.exports, e)
	case *types.Var:
		e := Export{Go: gn, Kind: "var"}
		if isError(o.Type()) {
			c := "DMRGO_ERR_" + strings.ToUpper(strings.TrimPrefix(g.prefix(p), "dmrgo_")+snake(strings.TrimPrefix(o.Name(), "Err")))
			if err := g.claim(c, gn); err != nil {
				return err
			}
			e.C = []string{c}
			g.errs = append(g.errs, errEntry{code: c, goRef: g.qual(p) + "." + o.Name(), name: p.Name() + "." + o.Name()})
		} else {
			e.Err = "exported var of type " + o.Type().String()
		}
		g.exports = append(g.exports, e)
	case *types.Func:
		e := Export{Go: gn, Kind: "func"}
		if o.Signature().TypeParams().Len() > 0 {
			e.Err = "generic func"
		} else if err := g.addFunc(&e, g.prefix(p)+snake(o.Name()), g.doc(o, gn), nil, rNone, g.qual(p)+"."+o.Name(), o.Signature(), nil); err != nil {
			return err
		}
		g.exports = append(g.exports, e)
	case *types.TypeName:
		return g.typeObject(o, gn)
	}
	return nil
}

func (g *gen) typeObject(o *types.TypeName, gn string) error {
	ti := g.infos[o]
	e := Export{Go: gn, Kind: lockType, C: []string{ti.c}}
	g.cur = ti.base
	if err := g.claim(ti.c, gn); err != nil {
		return err
	}
	doc := "/* " + g.doc(o, gn) + " */"
	var err error
	switch ti.kind {
	case kBad:
		err = fmt.Errorf("%s", ti.why)
	case kEnum:
		c, _ := g.scalar(ti.named.Underlying())
		g.hScalars.WriteString(doc + "\ntypedef " + c + " " + ti.c + ";\n")
		g.abiSyms = append(g.abiSyms, "sym typedef "+c+" "+ti.c+";")
	case kNArray, kValue:
		g.fwd = append(g.fwd, doc, "typedef struct "+ti.c+" "+ti.c+";")
		err = g.cStruct(ti)
	case kClosed:
		g.fwd = append(g.fwd, doc, "typedef struct "+ti.c+" "+ti.c+";")
		err = g.cUnion(ti)
	case kHandle, kOpen:
		g.fwd = append(g.fwd, doc, "typedef struct "+ti.c+" "+ti.c+";")
	case kNFunc:
		_, err = g.fnType(underSignature(ti.named), ti.c)
	}
	if err != nil {
		e.Err, e.C = err.Error(), nil
	}
	g.exports = append(g.exports, e)
	if ti.kind == kBad {
		return nil
	}
	if s, ok := ti.named.Underlying().(*types.Struct); ok {
		if err := g.fields(ti, s, gn, err); err != nil {
			return err
		}
	}
	if ti.kind == kHandle {
		if err := g.lifecycle(ti, gn, true); err != nil {
			return err
		}
	}
	if ti.kind == kOpen {
		if err := g.lifecycle(ti, gn, false); err != nil {
			return err
		}
	}
	var ms []*types.Func
	if iface, ok := ti.named.Underlying().(*types.Interface); ok {
		for i := 0; i < iface.NumMethods(); i++ {
			ms = append(ms, iface.Method(i))
		}
	} else {
		for m := range ti.named.Methods() {
			ms = append(ms, m)
		}
	}
	slices.SortFunc(ms, func(a, b *types.Func) int { return strings.Compare(a.Name(), b.Name()) })
	for _, m := range ms {
		if !m.Exported() {
			continue
		}
		me := Export{Go: gn + "." + m.Name(), Kind: "method"}
		rk, why := g.recvKind(ti, m)
		if why != "" {
			me.Err = why
		} else {
			g.curMethod, g.curRecv = m, ti
			err := g.addFunc(&me, ti.base+"_"+snake(m.Name()), g.doc(m, gn+"."+m.Name()), ti, rk, m.Name(), m.Signature(), nil)
			g.curMethod, g.curRecv = nil, nil
			if err != nil {
				return err
			}
		}
		g.exports = append(g.exports, me)
	}
	if ti.kind == kOpen {
		g.cur = ti.base + "_from_vtable"
		cname, proto, err := g.vtable(ti)
		if err == nil {
			if err := g.claim(cname, gn); err != nil {
				return err
			}
			g.funcSyms = append(g.funcSyms, cname)
			g.hProtos.WriteString(proto)
			g.abiSyms = append(g.abiSyms, "sym "+strings.TrimSpace(proto[strings.Index(proto, "\n")+1:]))
		} else {
			g.vtableErrs = append(g.vtableErrs, gn+": "+err.Error())
		}
	}
	return g.errorType(ti, gn)
}

func (g *gen) recvKind(ti *tinfo, m *types.Func) (recvKind, string) {
	_, ptr := m.Signature().Recv().Type().(*types.Pointer)
	switch ti.kind {
	case kHandle:
		return rHandle, ""
	case kOpen:
		return rOpen, ""
	case kClosed:
		return rClosed, ""
	case kValue:
		if ptr {
			return rValPtr, ""
		}
		return rValVal, ""
	case kEnum:
		if ptr {
			return rEnumPtr, ""
		}
		return rEnum, ""
	case kNArray:
		if ptr {
			return rArrPtr, ""
		}
		return rArrVal, ""
	default:
		return rNone, "methods of this kind of type"
	}
}

func (g *gen) fields(ti *tinfo, s *types.Struct, gn string, typeErr error) error {
	var ms []member
	if ti.kind == kValue && typeErr == nil {
		var err error
		if ms, err = g.members(ti); err != nil {
			typeErr = err
		}
	}
	mi := 0
	for f := range s.Fields() {
		if !f.Exported() {
			continue
		}
		fe := Export{Go: gn + "." + f.Name(), Kind: "field"}
		switch {
		case ti.kind == kHandle:
			if err := g.handleField(ti, f, &fe); err != nil {
				return err
			}
		case typeErr != nil:
			fe.Err = typeErr.Error()
		default:
			fe.C = []string{ti.c + "." + ms[mi].name}
			mi++
		}
		g.exports = append(g.exports, fe)
	}
	return nil
}

func (g *gen) handleField(ti *tinfo, f *types.Var, fe *Export) error {
	n := snake(f.Name())
	get := types.NewSignatureType(nil, nil, nil, nil, types.NewTuple(types.NewVar(0, nil, "", f.Type())), false)
	set := types.NewSignatureType(nil, nil, nil, types.NewTuple(types.NewVar(0, nil, "v", f.Type())), nil, false)
	doc := g.doc(f, fe.Go)
	if err := g.addFunc(fe, ti.base+"_get_"+n, doc, ti, rHandle, f.Name(), get, &lead{field: true}); err != nil {
		return err
	}
	if fe.Err != "" {
		return nil
	}
	return g.addFunc(fe, ti.base+"_set_"+n, doc, ti, rHandle, f.Name(), set, &lead{assign: true})
}

func (g *gen) lifecycle(ti *tinfo, gn string, zero bool) error {
	g.usedHandles[ti] = true
	if zero {
		c := ti.base + "_new"
		if err := g.claim(c, gn); err != nil {
			return err
		}
		g.funcSyms = append(g.funcSyms, c)
		lines(&g.goFuncs, "//export "+c, "func "+c+"(c_out **C."+ti.c+") (rc C.int) {", "defer recoverTo(&rc)",
			"if c_out == nil {", "return fail(C.DMRGO_ERR_ARG)", "}", "*c_out = "+g.newer(ti)+"(new("+g.goType(ti.named)+"))", "return C.DMRGO_OK", "}", "")
		p := "int " + c + "(" + ti.c + " **out);"
		g.hProtos.WriteString("/* " + c + " makes a zero " + gn + ". */\n" + p + "\n")
		g.abiSyms = append(g.abiSyms, "sym "+p)
	}
	c := ti.base + "_free"
	if err := g.claim(c, gn); err != nil {
		return err
	}
	g.funcSyms = append(g.funcSyms, c)
	lines(&g.goFuncs, "//export "+c, "func "+c+"(c_self *C."+ti.c+") (rc C.int) {", "defer recoverTo(&rc)",
		"if c_self == nil {", "return C.DMRGO_OK", "}", g.getter(ti)+"(c_self)", "deleteHandle(unsafe.Pointer(c_self))", "return C.DMRGO_OK", "}", "")
	p := "int " + c + "(" + ti.c + " *self);"
	g.hProtos.WriteString("/* " + c + " frees the handle. self may be NULL. */\n" + p + "\n")
	g.abiSyms = append(g.abiSyms, "sym "+p)
	return nil
}

func (g *gen) errorType(ti *tinfo, gn string) error {
	if ti.kind != kValue {
		return nil
	}
	et := errorIface()
	val := types.Implements(ti.named, et)
	ptr := types.Implements(types.NewPointer(ti.named), et)
	if !val && !ptr {
		return nil
	}
	code := "DMRGO_ERR_" + strings.ToUpper(strings.TrimPrefix(ti.base, "dmrgo_"))
	if err := g.claim(code, gn); err != nil {
		return err
	}
	g.errs = append(g.errs, errEntry{code: code, ti: ti, ptr: ptr && !val, val: val, name: ti.obj.Pkg().Name() + "." + ti.obj.Name()})
	for i := range g.exports {
		if g.exports[i].Go == gn && g.exports[i].Kind == lockType && g.exports[i].Err == "" {
			g.exports[i].C = append(g.exports[i].C, code)
		}
	}
	c := ti.base + "_last"
	if err := g.claim(c, gn); err != nil {
		return err
	}
	if _, ferr := g.outFn(ti); ferr != nil {
		return fmt.Errorf("%s: %w", gn, ferr)
	}
	n := g.lock.code(code)
	g.funcSyms = append(g.funcSyms, c)
	lines(&g.goFuncs, "//export "+c, "func "+c+"(c_out *C."+ti.c+", c_ok *C.bool) C.int {", "l := C.dmrgo__last()",
		fmt.Sprintf("ok := l.kind == %d", n), "if c_ok != nil {", "*c_ok = C.bool(ok)", "}", "if ok && c_out != nil {",
		"*c_out = *(*C."+ti.c+")(unsafe.Pointer(&l.d))", "}", "return C.DMRGO_OK", "}", "")
	p := "int " + c + "(" + ti.c + " *out, bool *ok);"
	g.hProtos.WriteString("/* " + c + " sets *ok when the last error of this thread holds a " + gn + " and copies it to *out. Its strings point into thread-local memory that the next failed call on the thread reuses. */\n" + p + "\n")
	g.abiSyms = append(g.abiSyms, "sym "+p)
	return nil
}

func (g *gen) constant(p *types.Package, o *types.Const) (string, error) {
	name := strings.ToUpper(g.prefix(p) + snake(o.Name()))
	v := o.Val()
	t := o.Type()
	cast := ""
	if ti := g.info(t); ti != nil && ti.kind == kEnum {
		cast = ti.c
	}
	var s string
	switch v.Kind() {
	case constant.Bool:
		s = cFalse
		if constant.BoolVal(v) {
			s = "true"
		}
	case constant.String:
		s = cString(constant.StringVal(v))
	case constant.Float:
		f, _ := constant.Float64Val(v)
		s = strconv.FormatFloat(f, 'g', 17, 64)
		if !strings.ContainsAny(s, ".eEn") {
			s += ".0"
		}
	case constant.Int:
		if i, ok := constant.Int64Val(v); ok {
			switch {
			case i >= -1<<31 && i < 1<<31:
				s = strconv.FormatInt(i, 10)
			default:
				s = "INT64_C(" + strconv.FormatInt(i, 10) + ")"
			}
		} else if u, ok := constant.Uint64Val(v); ok {
			s = "UINT64_C(0x" + strconv.FormatUint(u, 16) + ")"
		} else {
			return "", fmt.Errorf("constant %s does not fit 64 bits", v)
		}
		if b, ok := t.Underlying().(*types.Basic); ok && b.Info()&types.IsUnsigned != 0 {
			if u, ok := constant.Uint64Val(v); ok && u >= 1<<31 {
				s = "UINT64_C(0x" + strconv.FormatUint(u, 16) + ")"
			}
		}
	default:
		return "", fmt.Errorf("constant of kind %s", v.Kind())
	}
	if cast != "" {
		s = "((" + cast + ")" + s + ")"
	}
	return "#define " + name + " " + s, nil
}

func cString(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c == '"' || c == '\\':
			b.WriteByte('\\')
			b.WriteByte(c)
		case c < 0x20 || c >= 0x7f:
			fmt.Fprintf(&b, "\\%03o", c)
		default:
			b.WriteByte(c)
		}
	}
	b.WriteByte('"')
	return b.String()
}

func (g *gen) handles() {
	for _, ti := range g.order {
		if ti.kind != kHandle && ti.kind != kOpen {
			continue
		}
		v := "*" + g.goType(ti.named)
		if ti.kind == kOpen {
			v = g.goType(ti.named)
		}
		lines(&g.goConv, "type h_"+ti.id+" struct {", "v "+v)
		for i, s := range g.scratch[ti] {
			lines(&g.goConv, fmt.Sprintf("sc%d %s", i, s))
		}
		lines(&g.goConv, "}", "",
			"func get_"+ti.id+"(p *C."+ti.c+") *h_"+ti.id+" {", "return lookup[*h_"+ti.id+"](unsafe.Pointer(p))", "}", "",
			"func new_"+ti.id+"(v "+v+") *C."+ti.c+" {", "return (*C."+ti.c+")(newHandle(&h_"+ti.id+"{v: v}))", "}", "")
	}
}

func (g *gen) errorsGo() {
	var w strings.Builder
	var vars, typs []errEntry
	for _, e := range g.errs {
		if e.ti == nil {
			vars = append(vars, e)
		} else {
			typs = append(typs, e)
		}
	}
	lines(&w, "func errCode(err error) C.int {", "for e := err; e != nil; {")
	if len(vars) > 0 {
		lines(&w, "switch e {")
		for _, e := range vars {
			lines(&w, "case "+e.goRef+":", fmt.Sprintf("return %d", g.lock.code(e.code)))
		}
		lines(&w, "}")
	}
	if len(typs) > 0 {
		lines(&w, "switch e.(type) {")
		for _, e := range typs {
			lines(&w, "case "+strings.Join(g.errTypes(e), ", ")+":", fmt.Sprintf("return %d", g.lock.code(e.code)))
		}
		lines(&w, "}")
	}
	lines(&w, "switch u := e.(type) {", "case interface{ Unwrap() []error }:", "for _, x := range u.Unwrap() {",
		"if c := errCode(x); c != C.DMRGO_ERR_OTHER {", "return c", "}", "}", "return C.DMRGO_ERR_OTHER",
		"case interface{ Unwrap() error }:", "e = u.Unwrap()", "default:", "e = nil", "}", "}", "return C.DMRGO_ERR_OTHER", "}", "")
	lines(&w, "func setErr(err error) C.int {", "code := setLast(errCode(err), err.Error())")
	if len(typs) > 0 {
		g.imports["errors"] = true
		lines(&w, "l := C.dmrgo__last()", "oc := &outc{arena: unsafe.Slice((*byte)(unsafe.Pointer(&l.arena[0])), len(l.arena))}")
		for _, e := range typs {
			fn, _ := g.outFn(e.ti)
			t, arg := g.goType(e.ti.named), "&x"
			if e.ptr {
				t, arg = "*"+t, "x"
			}
			lines(&w, "{", "var x "+t, "if errors.As(err, &x) {", fmt.Sprintf("l.kind = %d", g.lock.code(e.code)),
				fn+"((*C."+e.ti.c+")(unsafe.Pointer(&l.d)), "+arg+", oc)", "return code", "}", "}")
		}
	}
	lines(&w, "return code", "}", "", "func codeErr(code C.int) error {", "switch code {")
	for _, e := range vars {
		lines(&w, fmt.Sprintf("case %d:", g.lock.code(e.code)), "return "+e.goRef)
	}
	lines(&w, "}", "return codeError(code)", "}", "")
	g.goFuncs.WriteString(w.String())
	lines(&g.genC, "const char *dmrgo__gen_strerror(int status) {", "\tswitch (status) {")
	for _, e := range g.errs {
		lines(&g.genC, fmt.Sprintf("\tcase %d:", g.lock.code(e.code)), "\t\treturn "+cString(e.name)+";")
	}
	lines(&g.genC, "\t}", "\treturn \"unknown status\";", "}", "")
	lines(&g.genC, "static _Thread_local dmrgo__last_t last;", "", "dmrgo__last_t *dmrgo__last(void) { return &last; }", "",
		"void dmrgo__fail(int code) {", "\tconst char *m = dmrgo_strerror(code);", "\tsize_t n = strlen(m);", "\tlast.code = code;",
		"\tlast.kind = 0;", "\tlast.msg_len = n;", "\tif (n > sizeof last.msg - 1)", "\t\tn = sizeof last.msg - 1;",
		"\tmemcpy(last.msg, m, n);", "\tlast.msg[n] = 0;", "}")
	lines(&g.internal, "typedef struct dmrgo__last_t {", "\tint code;", "\tint kind;", "\tsize_t msg_len;", "\tchar msg[512];", "\tunion {", "\t\tchar none;")
	for i, e := range typs {
		lines(&g.internal, fmt.Sprintf("\t\t%s k%d;", e.ti.c, i))
	}
	lines(&g.internal, "\t} d;", "\tunsigned char arena[512];", "} dmrgo__last_t;", "", "dmrgo__last_t *dmrgo__last(void);", "void dmrgo__fail(int code);", "")
	for _, e := range g.errs {
		def := fmt.Sprintf("#define %s (%d)", e.code, g.lock.code(e.code))
		g.defines = append(g.defines, def)
		g.abiSyms = append(g.abiSyms, "sym "+def)
	}
}

func (g *gen) errTypes(e errEntry) []string {
	var ts []string
	if e.val {
		ts = append(ts, g.goType(e.ti.named))
	}
	if types.Implements(types.NewPointer(e.ti.named), errorIface()) {
		ts = append(ts, "*"+g.goType(e.ti.named))
	}
	return ts
}

func (g *gen) convertAll() {
	for _, ti := range g.order {
		if ti.kind != kValue && ti.kind != kClosed {
			continue
		}
		fn, err := g.inFn(ti)
		if err != nil {
			continue
		}
		_, _ = g.outFn(ti)
		st, err := g.storage(ti.named)
		if err != nil {
			continue
		}
		lines(&g.goConv, "func fromC_"+ti.id+"(s *C."+ti.c+") "+g.goType(ti.named)+" {")
		if st != "" {
			lines(&g.goConv, "var st "+st, "return "+fn+"(s, &st)")
		} else {
			lines(&g.goConv, "return "+fn+"(s)")
		}
		lines(&g.goConv, "}", "")
	}
}

func (g *gen) finish(opt Options) (*Output, error) {
	g.convertAll()
	if len(g.collisions) > 0 {
		return nil, fmt.Errorf("%s", strings.Join(g.collisions, "\n"))
	}
	g.handles()
	g.errorsGo()
	var h bytes.Buffer
	lines2 := func(s ...string) {
		for _, l := range s {
			h.WriteString(l + "\n")
		}
	}
	lines2("/* Code generated by go run ./internal/capigen/gen; DO NOT EDIT. */", "", "#ifndef DMRGO_GEN_H", "#define DMRGO_GEN_H", "",
		"#include <stdbool.h>", "#include <stddef.h>", "#include <stdint.h>", "", "#ifdef __cplusplus", "extern \"C\" {", "#endif", "",
		fmt.Sprintf("#define DMRGO_ABI_VERSION %d", g.lock.abi), "",
		"typedef struct dmrgo_complex64_t {", "\tfloat re;", "\tfloat im;", "} dmrgo_complex64_t;", "",
		"typedef struct dmrgo_complex128_t {", "\tdouble re;", "\tdouble im;", "} dmrgo_complex128_t;", "",
		"typedef struct dmrgo_string_t {", "\tchar *ptr;", "\tsize_t len;", "} dmrgo_string_t;", "")
	for _, f := range g.fwd {
		lines2(f)
	}
	lines2("")
	h.WriteString(g.hScalars.String())
	lines2("")
	h.WriteString(g.hStructs.String())
	slices.Sort(g.defines)
	for _, d := range slices.Compact(g.defines) {
		lines2(d)
	}
	lines2("")
	h.WriteString(g.hConsts.String())
	lines2("", "#ifndef DMRGO_NO_PROTOTYPES", "")
	h.WriteString(g.hProtos.String())
	lines2("", "#endif", "", "#ifdef __cplusplus", "}", "#endif", "", "#endif")

	var ib bytes.Buffer
	ib.WriteString("/* Code generated by go run ./internal/capigen/gen; DO NOT EDIT. */\n\n#ifndef DMRGO_GEN_INTERNAL_H\n#define DMRGO_GEN_INTERNAL_H\n\n#include \"dmrgo_gen.h\"\n\n")
	ib.WriteString(g.internal.String())
	ib.WriteString("#endif\n")

	var cb bytes.Buffer
	cb.WriteString("/* Code generated by go run ./internal/capigen/gen; DO NOT EDIT. */\n\n#include <string.h>\n\n#include \"_cgo_export.h\"\n\nconst char *dmrgo_strerror(int status);\n\n")
	cb.WriteString(g.genC.String())

	body := g.goConv.String() + g.goFuncs.String()
	var gb bytes.Buffer
	gb.WriteString("// Code generated by go run ./internal/capigen/gen; DO NOT EDIT.\n\npackage main\n\n/*\n#define DMRGO_NO_PROTOTYPES\n#include \"dmrgo.h\"\n#include \"dmrgo_gen_internal.h\"\n\n")
	gb.WriteString(g.preamble.String())
	gb.WriteString("*/\nimport \"C\"\n\nimport (\n")
	var imps []string
	for p := range g.imports {
		imps = append(imps, p)
	}
	imps = append(imps, "unsafe")
	slices.Sort(imps)
	for _, p := range slices.Compact(imps) {
		alias := p[strings.LastIndex(p, "/")+1:]
		spec := fmt.Sprintf("%q", p)
		if g.inModule(types.NewPackage(p, "")) {
			alias = "g_" + strings.ReplaceAll(strings.TrimPrefix(strings.TrimPrefix(p, modulePath), "/"), "/", "_")
			if p == modulePath {
				alias = "g_dmrgo"
			}
			spec = alias + " " + spec
		}
		if !strings.Contains(body, alias+".") {
			continue
		}
		gb.WriteString("\t" + spec + "\n")
	}
	gb.WriteString(")\n\n")
	gb.WriteString(body)
	src, ferr := format.Source(gb.Bytes())
	if ferr != nil {
		src = gb.Bytes()
	}
	hand, handFuncs, err := handABI(filepath.Join(g.capiDir, "dmrgo.h"))
	if err != nil {
		return nil, err
	}
	syms := append(slices.Clone(g.abiSyms), g.abiTypes...)
	syms = append(syms, hand...)
	var mb bytes.Buffer
	mb.WriteString("{\n\tglobal:\n")
	funcs := append(slices.Clone(g.funcSyms), handFuncs...)
	slices.Sort(funcs)
	for _, f := range slices.Compact(funcs) {
		mb.WriteString("\t\t" + f + ";\n")
	}
	mb.WriteString("\tlocal:\n\t\t*;\n};\n")
	lockOut, broken, lerr := g.lock.render(syms, opt.Break)
	out := &Output{Go: src, H: h.Bytes(), Internal: ib.Bytes(), C: cb.Bytes(), Lock: lockOut, Map: mb.Bytes(), Exports: g.exports, Broken: broken, Unbounded: g.unbounded}
	if ferr != nil {
		return out, fmt.Errorf("gofmt: %w", ferr)
	}
	if lerr != nil {
		return out, lerr
	}
	return out, nil
}
