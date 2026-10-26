// Package capigen generates the C API of dmrgo from the exported Go API of
// the library: api_gen.go (the cgo exports and the converters), dmrgo_gen.h
// (the C declarations), dmrgo_gen_internal.h and dmrgo_gen.c (trampolines),
// abi.txt (the lock of the C ABI) and api_gen_test.go (the list of the entry
// points for the tests). Every exported type, field, func, method, const and
// error var of the packages under v2, except internal/, cmd/, examples/ and
// capi/, gets a C counterpart, and Exports reports any that could not be
// mapped.
package capigen

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/constant"
	"go/format"
	"go/token"
	"go/types"
	"path/filepath"
	"regexp"
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
	// Test lists every generated entry point for the tests of the C API.
	Test []byte
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
		"libdmrgo.map": o.Map, "api_gen_test.go": o.Test,
	}
}

type memo struct {
	s   string
	err error
}

type errEntry struct {
	name  string
	text  string
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
	curFn       *types.Func
	fillBuf     bool
	ioCount     bool
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
	prefixes    map[string]string
	unions      []string

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
		usedHandles: map[*tinfo]bool{}, scratch: map[*tinfo][]string{}, lock: al, capiDir: capiDir, prefixes: map[string]string{},
	}
	for _, p := range pkgs {
		g.prefix(p)
	}
	g.classify()
	al.renumber(g.codeNames())
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

var errWord = regexp.MustCompile(`\*?(?:\b[a-z][a-z0-9]*\.)?\b(?:Err[A-Z]\w*|[A-Z]\w*Error)\b`)

// fnDoc is the doc of a func or method: the first sentence of its Go doc,
// a C form of a view sentence, and its sentences about errors, with the
// Go error names as their C codes.
func (g *gen) fnDoc(o *types.Func, goName string) string {
	d := g.doc(o, goName)
	if strings.Contains(d, "without a copy") {
		arg := "its argument"
		if ps := o.Signature().Params(); ps.Len() > 0 && ps.At(0).Name() != "" {
			arg = ps.At(0).Name()
		}
		i := strings.Index(d, ". ") + 2
		d = d[:i] + "It copies " + arg + " into *out, which does not keep " + arg + "."
	}
	head := "Go: " + goName + ". "
	if strings.HasPrefix(d, head) {
		d = head + errWord.ReplaceAllStringFunc(d[len(head):], func(w string) string { return g.errC(o.Pkg(), w) })
	}
	full := g.l.full[o]
	first := g.l.docs[o]
	var errs []string
	for _, sent := range strings.SplitAfter(full, ". ") {
		sent = strings.TrimSpace(sent)
		if sent == "" || strings.HasPrefix(first, strings.TrimSuffix(sent, ".")) || !errWord.MatchString(sent) {
			continue
		}
		errs = append(errs, errWord.ReplaceAllStringFunc(sent, func(w string) string { return g.errC(o.Pkg(), w) }))
	}
	if len(errs) > 0 {
		d += " " + strings.ReplaceAll(strings.Join(errs, " "), "*/", "* /")
	}
	if r := o.Signature().Recv(); r != nil && r.Name() != "" {
		code := "DMRGO_ERR_ARG"
		if k := g.kindOf(derefType(r.Type())); k == kHandle || k == kOpen {
			code = "DMRGO_ERR_HANDLE"
		}
		d = regexp.MustCompile(`DMRGO_ERR_NIL_RECEIVER for a nil `+regexp.QuoteMeta(r.Name())+`\b`).ReplaceAllString(d, code+" for a NULL self")
	}
	if strings.HasPrefix(d, head) {
		d = head + g.selDocs(d[len(head):], o)
	}
	return d
}

// errC maps a Go error name of a doc, such as ErrBusy, sms.ErrShort or
// *dmrgo.ConfigError, to its C code, or returns w.
func (g *gen) errC(p *types.Package, w string) string {
	name := strings.TrimPrefix(w, "*")
	pkg := p
	if q, n, ok := strings.Cut(name, "."); ok {
		pkg = nil
		for _, c := range g.pkgs {
			if c.Name() == q {
				pkg = c
				break
			}
		}
		name = n
	}
	if pkg == nil {
		return w
	}
	switch o := pkg.Scope().Lookup(name).(type) {
	case *types.Var:
		if isError(o.Type()) && o.Exported() {
			return g.varCode(pkg, name)
		}
	case *types.TypeName:
		if ti := g.infos[o]; ti != nil && isErrType(ti) {
			return g.typeCode(ti)
		}
	}
	return w
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
			c := g.varCode(p, o.Name())
			if err := g.claim(c, gn); err != nil {
				return err
			}
			e.C = []string{c}
			g.errs = append(g.errs, errEntry{code: c, goRef: g.qual(p) + "." + o.Name(), name: p.Name() + "." + o.Name(), text: g.errText(o, 0)})
		} else {
			e.Err = "exported var of type " + o.Type().String()
		}
		g.exports = append(g.exports, e)
	case *types.Func:
		e := Export{Go: gn, Kind: "func"}
		if o.Signature().TypeParams().Len() > 0 {
			e.Err = "generic func"
		} else {
			g.curFn = o
			err := g.addFunc(&e, g.funcName(p, o.Name()), g.fnDoc(o, gn), nil, rNone, g.qual(p)+"."+o.Name(), o.Signature(), nil)
			g.curFn = nil
			if err != nil {
				return err
			}
		}
		g.exports = append(g.exports, e)
	case *types.TypeName:
		return g.typeObject(o, gn)
	}
	return nil
}

func (g *gen) varCode(p *types.Package, name string) string {
	return "DMRGO_ERR_" + strings.ToUpper(strings.TrimPrefix(g.prefix(p), "dmrgo_")+snake(strings.TrimPrefix(name, "Err")))
}

func (g *gen) typeCode(ti *tinfo) string {
	return "DMRGO_ERR_" + strings.TrimSuffix(strings.ToUpper(strings.TrimPrefix(ti.base, "dmrgo_")), "_ERROR")
}

func isErrType(ti *tinfo) bool {
	return ti.kind == kValue && (types.Implements(ti.named, errorIface()) || types.Implements(types.NewPointer(ti.named), errorIface()))
}

// codeNames lists the error codes of the API in package order, and by name
// in a package, and the tags of each union by name.
func (g *gen) codeNames() ([]string, map[string][]string) {
	names := make([]string, 0, 2*len(g.pkgs))
	for _, p := range g.pkgs {
		var ns []string
		sc := p.Scope()
		for _, n := range sc.Names() {
			switch o := sc.Lookup(n).(type) {
			case *types.Var:
				if o.Exported() && isError(o.Type()) {
					ns = append(ns, g.varCode(p, n))
				}
			case *types.TypeName:
				if ti := g.infos[o]; ti != nil && o.Exported() && isErrType(ti) {
					ns = append(ns, g.typeCode(ti))
				}
			}
		}
		slices.Sort(ns)
		names = append(names, ns...)
	}
	tags := map[string][]string{}
	for _, ti := range g.order {
		if ti.kind != kClosed {
			continue
		}
		ts := make([]string, 0, len(ti.impls))
		for _, im := range ti.impls {
			ts = append(ts, g.tagName(ti, g.info(im)))
		}
		slices.Sort(ts)
		tags[ti.c] = ts
	}
	return names, tags
}

// errText is the message of the error var o, from its errors.New or
// fmt.Errorf literal, or its package and name.
func (g *gen) errText(o types.Object, depth int) string {
	fallback := o.Pkg().Name() + ": " + strings.ReplaceAll(snake(strings.TrimPrefix(o.Name(), "Err")), "_", " ")
	call, ok := g.l.vals[o].(*ast.CallExpr)
	if !ok || len(call.Args) == 0 || depth > 4 {
		return fallback
	}
	lit, ok := call.Args[0].(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		return fallback
	}
	f, err := strconv.Unquote(lit.Value)
	if err != nil {
		return fallback
	}
	info := g.l.info[o.Pkg()]
	args := call.Args[1:]
	var b strings.Builder
	for i := 0; i < len(f); i++ {
		if f[i] != '%' || i+1 >= len(f) {
			b.WriteByte(f[i])
			continue
		}
		i++
		if f[i] == '%' {
			b.WriteByte('%')
			continue
		}
		if len(args) == 0 {
			continue
		}
		a := args[0]
		args = args[1:]
		var id *ast.Ident
		switch x := a.(type) {
		case *ast.Ident:
			id = x
		case *ast.SelectorExpr:
			id = x.Sel
		}
		if id != nil {
			if v, ok := info.Uses[id].(*types.Var); ok && isError(v.Type()) && v.Pkg() != nil {
				b.WriteString(g.errText(v, depth+1))
			}
		}
	}
	return b.String()
}

func (g *gen) funcName(p *types.Package, name string) string {
	c := g.prefix(p) + snake(name)
	sc := p.Scope()
	for _, t := range sc.Names() {
		tn, ok := sc.Lookup(t).(*types.TypeName)
		if !ok || !tn.Exported() || !strings.HasPrefix(name, t) {
			continue
		}
		if u, ok := sc.Lookup(name[len(t):]).(*types.TypeName); ok && u.Exported() {
			return g.prefix(p) + "func_" + snake(name)
		}
	}
	return c
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
		if err := g.lifecycle(ti, gn, (zeroReady(g.l.plain[o], o.Name()) || g.valueResult(ti)) && !hasNew(o)); err != nil {
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
			g.curMethod, g.curRecv, g.fillBuf, g.curFn = m, ti, isRead(m), m
			if ti.kind == kOpen {
				g.curFn = nil
			}
			err := g.addFunc(&me, ti.base+"_"+snake(m.Name()), g.fnDoc(m, gn+"."+m.Name()), ti, rk, m.Name(), m.Signature(), nil)
			g.curMethod, g.curRecv, g.fillBuf, g.curFn = nil, nil, false, nil
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
		if err := g.asIface(ti, gn); err != nil {
			return err
		}
	}
	return g.errorType(ti, gn)
}

// asIface emits, for each value or handle type of the library that
// implements the open interface ti, a function that makes an interface
// handle from it, such as dmrgo_trunk_auth_key_authenticator.
func (g *gen) asIface(ti *tinfo, gn string) error {
	iface := underInterface(ti.named)
	for _, im := range ti.impls {
		ii := g.info(im)
		if ii == nil || ii.kind != kValue && ii.kind != kNArray && ii.kind != kHandle {
			continue
		}
		c := ii.base + "_as_" + snake(ti.obj.Name())
		if _, taken := g.cnames[c]; taken {
			continue
		}
		byVal := types.Implements(im, iface)
		g.cur = c
		var w strings.Builder
		var conv []string
		switch ii.kind {
		case kHandle:
			v := "h.v"
			if byVal {
				v = "*h.v"
			}
			conv = []string{"h := " + g.getter(ii) + "(c_self)", "*c_out = " + g.newer(ti) + "(" + v + ")"}
		default:
			st, err := g.storage(im)
			if err != nil {
				continue
			}
			var pre strings.Builder
			lines(&pre, "var x "+g.goType(im))
			sv := ""
			if st != "" {
				lines(&pre, "var xs "+st)
				sv = "xs"
			}
			src := g.sizedSrc(&pre, im, "c_self", "(*c_self)")
			if g.alias(im) {
				lines(&pre, "x = *(*"+g.goType(im)+")(unsafe.Pointer(c_self))")
			} else if err := g.inVal(&pre, im, "x", src, sv, 0); err != nil {
				continue
			}
			lines(&pre, "x = cloneDeep(x)")
			v := "&x"
			if byVal {
				v = "x"
			}
			conv = []string{strings.TrimSuffix(pre.String(), "\n"), "*c_out = " + g.newer(ti) + "(" + v + ")"}
		}
		if err := g.claim(c, gn); err != nil {
			return err
		}
		self := "const " + ii.c + " *self"
		if ii.kind == kHandle {
			self = ii.c + " *self"
		}
		lines(&w, "//export "+c, "func "+c+"(c_self *C."+ii.c+", c_out **C."+ti.c+") (rc C.int) {", "defer recoverTo(&rc)",
			"if c_self == nil || c_out == nil {", "return fail(C.DMRGO_ERR_ARG)", "}")
		lines(&w, conv...)
		lines(&w, "return C.DMRGO_OK", "}", "")
		g.goFuncs.WriteString(w.String())
		g.funcSyms = append(g.funcSyms, c)
		p := "int " + c + "(" + self + ", " + ti.c + " **out);"
		doc := "/* " + c + " makes a " + gn + " from self"
		if ii.kind != kHandle {
			doc += ", which it copies"
		} else {
			doc += ", which it shares; free both handles"
		}
		g.hProtos.WriteString(doc + ". */\n" + p + "\n")
		g.abiSyms = append(g.abiSyms, "sym "+p)
	}
	return nil
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
			i := slices.IndexFunc(ms, func(m member) bool { return m.field == f })
			fe.C = []string{ti.c + "." + ms[i].name}
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

// zeroReady reports whether the Go doc of the type name says that its zero
// value is ready to use, which is when the C API makes it with a _new
// function.
func zeroReady(doc, name string) bool {
	for _, lead := range []string{"The zero value is ", "The zero " + name + " is "} {
		if i := strings.Index(doc, lead); i >= 0 {
			rest := doc[i+len(lead):]
			if strings.HasPrefix(rest, "ready") || strings.HasPrefix(rest, "empty") || strings.HasPrefix(rest, "an empty") {
				return true
			}
		}
	}
	return false
}

// valueResult reports whether a func or method returns the handle type of
// ti by value, which the C form copies into a handle of the caller.
func (g *gen) valueResult(ti *tinfo) bool {
	check := func(sig *types.Signature) bool {
		for v := range sig.Results().Variables() {
			if types.Identical(v.Type(), ti.named) {
				return true
			}
		}
		return false
	}
	for _, p := range g.pkgs {
		for _, n := range p.Scope().Names() {
			switch o := p.Scope().Lookup(n).(type) {
			case *types.Func:
				if o.Exported() && check(o.Signature()) {
					return true
				}
			case *types.TypeName:
				nt, ok := o.Type().(*types.Named)
				if !ok || !o.Exported() {
					continue
				}
				for m := range nt.Methods() {
					if m.Exported() && check(m.Signature()) {
						return true
					}
				}
			}
		}
	}
	return false
}

func hasNew(o *types.TypeName) bool {
	f, ok := o.Pkg().Scope().Lookup("New" + o.Name()).(*types.Func)
	if !ok || f.Signature().Params().Len() != 0 || f.Signature().Results().Len() != 1 {
		return false
	}
	r := f.Signature().Results().At(0).Type()
	if p, ok := r.(*types.Pointer); ok {
		r = p.Elem()
	}
	return types.Identical(r, o.Type())
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
	code := g.typeCode(ti)
	if err := g.claim(code, gn); err != nil {
		return err
	}
	words := strings.ReplaceAll(snake(strings.TrimSuffix(ti.obj.Name(), "Error")), "_", " ")
	g.errs = append(g.errs, errEntry{code: code, ti: ti, ptr: ptr && !val, val: val, name: ti.obj.Pkg().Name() + "." + ti.obj.Name(),
		text: ti.obj.Pkg().Name() + ": " + words + " error"})
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
	lines(&w, "func errCode1(e error) C.int {")
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
	lines(&w, "return C.DMRGO_ERR_OTHER", "}", "")
	lines(&w, "func errWalk(err error, f func(C.int) bool) bool {", "for e := err; e != nil; {", "c := errCode1(e)", "if ce, ok := e.(codeError); ok {", "c = C.int(ce)", "}", "if c != C.DMRGO_ERR_OTHER && f(c) {", "return true", "}", "switch u := e.(type) {", "case interface{ Unwrap() []error }:", "for _, x := range u.Unwrap() {", "if errWalk(x, f) {", "return true", "}", "}", "return false", "case interface{ Unwrap() error }:", "e = u.Unwrap()", "default:", "return false", "}", "}", "return false", "}", "")
	lines(&w, "func setErr(err error) C.int {", "code := setLast(errCode(err), err.Error())", "setChain(err)")
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
	lines(&w, "}", "if code > 0 || !knownCode(code) {", "return statusError(code)", "}", "return codeError(code)", "}", "")
	lines(&w, "func knownCode(code C.int) bool {", "switch code {",
		"case C.DMRGO_ERR_HANDLE, C.DMRGO_ERR_ARG, C.DMRGO_ERR_SHORT, C.DMRGO_ERR_PANIC, C.DMRGO_ERR_OTHER, C.DMRGO_ERR_BOUND, C.DMRGO_ERR_NO_METHOD:",
		"return true")
	for _, e := range g.errs {
		lines(&w, fmt.Sprintf("case %d:", g.lock.code(e.code)), "return true")
	}
	lines(&w, "}", "return false", "}", "")
	g.goFuncs.WriteString(w.String())
	lines(&g.genC, "const char *dmrgo__gen_strerror(int status) {", "\tswitch (status) {")
	for _, e := range g.errs {
		lines(&g.genC, fmt.Sprintf("\tcase %d:", g.lock.code(e.code)), "\t\treturn "+cString(e.text)+";")
	}
	lines(&g.genC, "\t}", "\treturn \"unknown status\";", "}", "")
	lines(&g.genC, "static _Thread_local dmrgo__last_t last;", "", "dmrgo__last_t *dmrgo__last(void) { return &last; }", "",
		"void dmrgo__fail(int code) {", "\tconst char *m = dmrgo_strerror(code);", "\tsize_t n = strlen(m);", "\tlast.code = code;",
		"\tlast.kind = 0;", "\tlast.chain_len = 0;", "\tlast.msg_len = n;", "\tif (n > sizeof last.msg - 1)", "\t\tn = sizeof last.msg - 1;",
		"\tmemcpy(last.msg, m, n);", "\tlast.msg[n] = 0;", "}")
	lines(&g.internal, "typedef struct dmrgo__last_t {", "\tint code;", "\tint kind;", "\tint chain_len;", "\tint chain[16];", "\tsize_t msg_len;", "\tchar msg[512];", "\tunion {", "\t\tchar none;")
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

func (g *gen) alignsGo() {
	slices.Sort(g.unions)
	var w strings.Builder
	lines(&w, "func cAlign(p any) uintptr {", "switch p.(type) {")
	for _, c := range g.unions {
		fn := "dmrgo__align_" + strings.TrimSuffix(strings.TrimPrefix(c, "dmrgo_"), "_t")
		lines(&g.internal, "enum { "+fn+" = _Alignof("+c+") };", "")
		lines(&w, "case *C."+c+":", "return C."+fn)
	}
	lines(&w, "}", "return 0", "}", "")
	g.goFuncs.WriteString(w.String())
}

func (g *gen) finish(opt Options) (*Output, error) {
	g.convertAll()
	if len(g.collisions) > 0 {
		return nil, fmt.Errorf("%s", strings.Join(g.collisions, "\n"))
	}
	g.handles()
	g.errorsGo()
	g.alignsGo()
	var h bytes.Buffer
	lines2 := func(s ...string) {
		for _, l := range s {
			h.WriteString(l + "\n")
		}
	}
	lines2("/* Code generated by go run ./internal/capigen/gen; DO NOT EDIT. */", "", "#ifndef DMRGO_GEN_H", "#define DMRGO_GEN_H", "",
		"#include <stdbool.h>", "#include <stddef.h>", "#include <stdint.h>", "", "#ifdef __cplusplus", "extern \"C\" {", "#endif", "",
		abiVersionLine(g.lock.abi), "",
		"/* DMRGO_USER_T is the type of the user value of a callback or vtable. The library builds with uintptr_t, which has the same size and passing. */",
		"#ifndef DMRGO_USER_T", "#define DMRGO_USER_T void *", "#endif", "",
		"typedef struct dmrgo_complex64_t {", "\tfloat re;", "\tfloat im;", "} dmrgo_complex64_t;", "",
		"typedef struct dmrgo_complex128_t {", "\tdouble re;", "\tdouble im;", "} dmrgo_complex128_t;", "",
		"typedef struct dmrgo_string_t {", "\tchar *ptr;", "\tsize_t len;", "} dmrgo_string_t;", "",
		"/* dmrgo_duration_t is a time in nanoseconds. A now argument is the time since any fixed start, such as a monotonic clock. */",
		"typedef int64_t "+cDuration+";", "")
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
	gb.WriteString("// Code generated by go run ./internal/capigen/gen; DO NOT EDIT.\n\npackage main\n\n/*\n#define DMRGO_NO_PROTOTYPES\n#define DMRGO_USER_T uintptr_t\n#include \"dmrgo.h\"\n#include \"dmrgo_gen_internal.h\"\n\n")
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
	syms := append(slices.Clone(g.abiSyms), "sym typedef int64_t "+cDuration+";")
	syms = append(syms, g.abiTypes...)
	syms = append(syms, hand...)
	abi := g.lock.abi
	lockOut, broken, lerr := g.lock.render(syms, opt.Break)
	header := strings.Replace(h.String(), abiVersionLine(abi), abiVersionLine(g.lock.abi), 1)
	var mb bytes.Buffer
	fmt.Fprintf(&mb, "DMRGO_%d {\n\tglobal:\n", g.lock.abi)
	funcs := append(slices.Clone(g.funcSyms), handFuncs...)
	slices.Sort(funcs)
	for _, f := range slices.Compact(funcs) {
		mb.WriteString("\t\t" + f + ";\n")
	}
	mb.WriteString("\tlocal:\n\t\t*;\n};\n")
	var tb bytes.Buffer
	tb.WriteString("// Code generated by go run ./internal/capigen/gen; DO NOT EDIT.\n\npackage main\n\nvar entryPoints = map[string]any{\n")
	for _, m := range regexp.MustCompile(`(?m)^//export (dmrgo_\w+)$`).FindAllStringSubmatch(body, -1) {
		tb.WriteString("\t" + strconv.Quote(m[1]) + ": " + m[1] + ",\n")
	}
	tb.WriteString("}\n")
	test, terr := format.Source(tb.Bytes())
	if terr != nil {
		return nil, terr
	}
	refs := cRefs(g.exports)
	chainRefs(refs, g.l.refs)
	out := &Output{Test: test, Go: src, H: []byte(cDocs(header, refs)), Internal: ib.Bytes(), C: cb.Bytes(), Lock: lockOut, Map: mb.Bytes(), Exports: g.exports, Broken: broken, Unbounded: g.unbounded}
	if ferr != nil {
		return out, fmt.Errorf("gofmt: %w", ferr)
	}
	if lerr != nil {
		return out, lerr
	}
	return out, nil
}

func abiVersionLine(abi int) string {
	return fmt.Sprintf("#define DMRGO_ABI_VERSION %d", abi)
}
