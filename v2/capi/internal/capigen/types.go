package capigen

import (
	"fmt"
	"go/token"
	"go/types"
	"path"
	"slices"
	"strings"
	"unicode"
)

type kind int

const (
	dstName    = "dst"
	userParam  = "DMRGO_USER_T user"
	cFalse     = "false"
	cBool      = "bool"
	cFloat     = "float"
	cDouble    = "double"
	cDuration  = "dmrgo_duration_t"
	lockCode   = "code"
	methString = "String"
	lockType   = "type"
)

const (
	kBad kind = iota
	kEnum
	kNArray
	kValue
	kHandle
	kClosed
	kOpen
	kNFunc
)

type tinfo struct {
	obj   *types.TypeName
	named *types.Named
	kind  kind
	c     string
	base  string
	id    string
	impls []*types.Named
	why   string
	sized bool
}

func snake(s string) string {
	if rest, ok := strings.CutPrefix(s, "DD"); ok && rest != "" {
		switch {
		case unicode.IsDigit(rune(rest[0])):
			return "dd_" + strings.ToLower(rest)
		case unicode.IsUpper(rune(rest[0])):
			return "dd_" + snake(rest)
		}
	}
	for _, w := range []string{"AnyTone", "MHz", "KHz", "kHz"} {
		if before, after, ok := strings.Cut(s, w); ok {
			var parts []string
			if before != "" {
				parts = append(parts, snake(before))
			}
			parts = append(parts, strings.ToLower(w))
			if after != "" {
				parts = append(parts, snake(after))
			}
			return strings.Join(parts, "_")
		}
	}
	return snakeWords(s)
}

func snakeWords(s string) string {
	var words []string
	r := []rune(s)
	for i := 0; i < len(r); {
		j := i + 1
		switch {
		case unicode.IsUpper(r[i]):
			for j < len(r) && (unicode.IsUpper(r[j]) || unicode.IsDigit(r[j])) {
				j++
			}
			if j < len(r) && unicode.IsLower(r[j]) {
				if j-i > 1 && (r[j] != 'v' || j+1 >= len(r) || !unicode.IsDigit(r[j+1])) {
					j--
				} else {
					for j < len(r) && (unicode.IsLower(r[j]) || unicode.IsDigit(r[j])) {
						j++
					}
				}
			}
			if j-i > 1 && (unicode.IsUpper(r[i+1]) || unicode.IsDigit(r[i+1])) {
				words = append(words, acronyms(string(r[i:j]))...)
			} else {
				words = append(words, string(r[i:j]))
			}
		default:
			for j < len(r) && !unicode.IsUpper(r[j]) {
				j++
			}
			words = append(words, string(r[i:j]))
		}
		i = j
	}
	out := words[:0]
	for _, w := range words {
		if n := len(out); n > 0 {
			prev := out[n-1]
			if len(w) == 1 && unicode.IsUpper(rune(w[0])) && unicode.IsDigit(rune(prev[len(prev)-1])) || w == "Hz" && len(prev) == 1 {
				out[n-1] = prev + w
				continue
			}
		}
		out = append(out, w)
	}
	return strings.ToLower(strings.Join(out, "_"))
}

func acronyms(w string) []string {
	known := []string{"CRC", "OK", "FLC", "GPS", "UDP", "IP", "IPv4", "IPv6", "TSCC", "USBD", "UDT", "LIP", "WD", "LC", "C", "P"}
	if slices.Contains(known, w) || !strings.ContainsFunc(w, unicode.IsUpper) {
		return []string{w}
	}
	var seg func(string) []string
	seg = func(s string) []string {
		if s == "" {
			return []string{}
		}
		for _, k := range known {
			if !strings.HasPrefix(s, k) {
				continue
			}
			n := len(k)
			for n < len(s) && s[n] >= '0' && s[n] <= '9' {
				n++
			}
			if rest := seg(s[n:]); rest != nil {
				return append([]string{s[:n]}, rest...)
			}
		}
		return nil
	}
	if parts := seg(w); parts != nil {
		return parts
	}
	return []string{w}
}

func cKeyword(s string) string {
	switch s {
	case "auto", "break", "case", "char", "const", "continue", "default", "do", "double", "else", "enum", "extern",
		"float", "for", "goto", "if", "inline", "int", "long", "register", "restrict", "return", "short", "signed",
		"sizeof", "static", "struct", "switch", "typedef", "union", "unsigned", "void", "volatile", "while", "bool",
		"true", "false", "class", "new", "delete", "this", "template", "typename",
		"namespace", "operator", "private", "protected", "public", "virtual", "friend", "throw", "try", "catch", "using",
		"explicit", "mutable", "export", "and", "or", "not", "xor", "bitand", "bitor", "compl", "and_eq", "or_eq",
		"xor_eq", "not_eq", "nullptr", "decltype", "noexcept", "alignas", "alignof", "constexpr", "thread_local",
		"wchar_t", "char8_t", "char16_t", "char32_t", "asm", "concept", "requires", "typeid", "static_assert",
		"dynamic_cast", "static_cast", "const_cast", "reinterpret_cast", "co_await", "co_yield", "co_return":
		return s + "_"
	}
	return s
}

func comp(n, suffix string) string {
	return strings.TrimSuffix(n, "_") + suffix
}

func goField(n string) string {
	if token.Lookup(n).IsKeyword() {
		return "_" + n
	}
	return n
}

func (g *gen) prefix(p *types.Package) string {
	if c, ok := g.prefixes[p.Path()]; ok {
		return c
	}
	c, err := g.lock.prefix(p.Path(), func() string { return g.prefixRule(p) })
	if err != nil {
		g.collisions = append(g.collisions, err.Error())
	}
	g.prefixes[p.Path()] = c
	return c
}

func (g *gen) prefixRule(p *types.Package) string {
	if p.Path() == modulePath {
		return "dmrgo_"
	}
	n := 0
	for _, o := range g.pkgs {
		if o.Name() == p.Name() {
			n++
		}
	}
	if n > 1 && path.Dir(p.Path()) != modulePath {
		return "dmrgo_" + path.Base(path.Dir(p.Path())) + "_" + p.Name() + "_"
	}
	return "dmrgo_" + p.Name() + "_"
}

func (g *gen) typeBase(p *types.Package, name string) string {
	pfx := g.prefix(p)
	if pfx != "dmrgo_" && strings.HasSuffix(pfx, "_"+snake(name)+"_") {
		return strings.TrimSuffix(pfx, "_")
	}
	return pfx + snake(name)
}

func (g *gen) goID(p *types.Package) string {
	if p.Path() == modulePath {
		return "dmrgo"
	}
	return strings.ReplaceAll(strings.TrimPrefix(p.Path(), modulePath+"/"), "/", "_")
}

func (g *gen) inModule(p *types.Package) bool {
	return p != nil && (p.Path() == modulePath || strings.HasPrefix(p.Path(), modulePath+"/"))
}

func (g *gen) qual(p *types.Package) string {
	if p == nil {
		return ""
	}
	g.imports[p.Path()] = true
	if !g.inModule(p) {
		return p.Name()
	}
	return "g_" + g.goID(p)
}

func (g *gen) goType(t types.Type) string {
	return types.TypeString(t, g.qual)
}

func isDuration(t types.Type) bool {
	n, ok := t.(*types.Named)
	return ok && n.Obj().Pkg() != nil && n.Obj().Pkg().Path() == "time" && n.Obj().Name() == "Duration"
}

func isError(t types.Type) bool {
	return types.Identical(t, types.Universe.Lookup("error").Type())
}

func (g *gen) ptrReturns() map[*types.Named]bool {
	ptrRet := map[*types.Named]bool{}
	scan := func(sig *types.Signature) {
		for v := range sig.Results().Variables() {
			p, ok := v.Type().(*types.Pointer)
			if !ok {
				continue
			}
			if n, ok := p.Elem().(*types.Named); ok && g.inModule(n.Obj().Pkg()) && underStruct(n) != nil {
				ptrRet[n] = true
			}
		}
	}
	for _, p := range g.pkgs {
		sc := p.Scope()
		for _, name := range sc.Names() {
			switch o := sc.Lookup(name).(type) {
			case *types.Func:
				if o.Exported() {
					scan(o.Signature())
				}
			case *types.TypeName:
				n, ok := o.Type().(*types.Named)
				if !ok || !o.Exported() {
					continue
				}
				for m := range n.Methods() {
					if m.Exported() {
						scan(m.Signature())
					}
				}
			}
		}
	}
	return ptrRet
}

func (g *gen) classify() {
	ptrRet := g.ptrReturns()
	for _, p := range g.pkgs {
		sc := p.Scope()
		for _, name := range sc.Names() {
			tn, ok := sc.Lookup(name).(*types.TypeName)
			if !ok || !tn.Exported() {
				continue
			}
			n, _ := tn.Type().(*types.Named)
			ti := &tinfo{obj: tn, named: n, base: g.typeBase(p, name), id: g.goID(p) + "_" + name}
			ti.c = ti.base + "_t"
			g.infos[tn] = ti
			g.order = append(g.order, ti)
			g.classifyType(ti, ptrRet)
		}
	}
	for _, ti := range g.order {
		if ti.kind == kClosed || ti.kind == kOpen {
			g.implementers(ti)
		}
	}
	g.markSized()
}

// markSized marks the config and option structs that the API only takes
// as inputs, and the structs that the lock already has with a size field:
// their C form starts with size_t size, so they can grow at the end.
func (g *gen) markSized() {
	used := map[*types.Named]bool{}
	var mark func(t types.Type)
	mark = func(t types.Type) {
		switch u := t.(type) {
		case *types.Pointer:
			mark(u.Elem())
		case *types.Slice:
			mark(u.Elem())
		case *types.Array:
			mark(u.Elem())
		case *types.Named:
			used[u] = true
		}
	}
	results := func(sig *types.Signature) {
		for v := range sig.Results().Variables() {
			mark(v.Type())
		}
	}
	for _, ti := range g.order {
		if ti.named == nil {
			continue
		}
		if s := underStruct(ti.named); s != nil {
			for f := range s.Fields() {
				if f.Exported() {
					mark(f.Type())
				}
			}
		}
		if it := underInterface(ti.named); it != nil {
			for m := range it.Methods() {
				results(m.Signature())
			}
		}
		for m := range ti.named.Methods() {
			if m.Exported() {
				results(m.Signature())
			}
		}
	}
	for _, p := range g.pkgs {
		for _, n := range p.Scope().Names() {
			if f, ok := p.Scope().Lookup(n).(*types.Func); ok && f.Exported() {
				results(f.Signature())
			}
		}
	}
	for _, ti := range g.order {
		if ti.kind != kValue {
			continue
		}
		n := ti.obj.Name()
		ti.sized = (strings.HasSuffix(n, "Config") || strings.HasSuffix(n, "Options")) && !used[ti.named] || g.lock.sizedLine(ti.c) != ""
	}
}

func (g *gen) isSized(t types.Type) bool {
	ti := g.info(t)
	return ti != nil && ti.sized
}

func (g *gen) classifyType(ti *tinfo, ptrRet map[*types.Named]bool) {
	n := ti.named
	switch {
	case n == nil || ti.obj.IsAlias():
		ti.why = "type alias"
		return
	case n.TypeParams().Len() > 0:
		ti.why = "generic type"
		return
	}
	switch u := n.Underlying().(type) {
	case *types.Basic:
		if u.Info()&(types.IsNumeric|types.IsBoolean) != 0 {
			ti.kind = kEnum
		} else {
			ti.why = "named " + u.String()
		}
	case *types.Array:
		ti.kind = kNArray
	case *types.Struct:
		ti.kind = kValue
		if ptrRet[n] || hasUnexported(u) && (hasPtrMethod(n) || !hasExported(u)) {
			ti.kind = kHandle
		}
	case *types.Interface:
		ti.kind = kOpen
		if g.closed[ti.obj.Pkg().Name()+"."+ti.obj.Name()] {
			ti.kind = kClosed
		}
	case *types.Signature:
		ti.kind = kNFunc
	default:
		ti.why = fmt.Sprintf("named %T", u)
	}
}

func (g *gen) implementers(ti *tinfo) {
	iface := underInterface(ti.named)
	for _, o := range g.order {
		switch o.kind {
		case kBad, kOpen, kClosed, kNFunc:
			continue
		default:
		}
		if types.Implements(o.named, iface) || types.Implements(types.NewPointer(o.named), iface) {
			ti.impls = append(ti.impls, o.named)
		}
	}
	if ti.kind == kClosed && len(ti.impls) == 0 {
		ti.kind, ti.why = kBad, "closed interface without implementers"
	}
}

func hasUnexported(s *types.Struct) bool {
	for f := range s.Fields() {
		if !f.Exported() {
			return true
		}
	}
	return false
}

func hasExported(s *types.Struct) bool {
	for f := range s.Fields() {
		if f.Exported() {
			return true
		}
	}
	return false
}

func hasPtrMethod(n *types.Named) bool {
	for m := range n.Methods() {
		if _, ok := m.Signature().Recv().Type().(*types.Pointer); ok {
			return true
		}
	}
	return false
}

func (g *gen) info(t types.Type) *tinfo {
	n, ok := t.(*types.Named)
	if !ok || !g.inModule(n.Obj().Pkg()) {
		return nil
	}
	return g.infos[n.Obj()]
}

func (g *gen) kindOf(t types.Type) kind {
	if ti := g.info(t); ti != nil {
		return ti.kind
	}
	return kBad
}

func basicC(b *types.Basic) string {
	switch b.Kind() {
	case types.Bool:
		return cBool
	case types.Int, types.Int64:
		return "int64_t"
	case types.Int8:
		return "int8_t"
	case types.Int16:
		return "int16_t"
	case types.Int32:
		return "int32_t"
	case types.Uint, types.Uint64:
		return "uint64_t"
	case types.Uint8:
		return "uint8_t"
	case types.Uint16:
		return "uint16_t"
	case types.Uint32:
		return "uint32_t"
	case types.Uintptr:
		return "uintptr_t"
	case types.Float32:
		return cFloat
	case types.Float64:
		return cDouble
	case types.Complex64:
		return "dmrgo_complex64_t"
	case types.Complex128:
		return "dmrgo_complex128_t"
	default:
		return ""
	}
}

func (g *gen) scalar(t types.Type) (string, bool) {
	if isDuration(t) {
		return cDuration, true
	}
	if ti := g.info(t); ti != nil {
		if ti.kind == kEnum {
			return ti.c, true
		}
		return "", false
	}
	if b, ok := t.(*types.Basic); ok {
		c := basicC(b)
		return c, c != ""
	}
	return "", false
}

func (g *gen) alias(t types.Type) bool {
	if isDuration(t) {
		return true
	}
	switch u := t.(type) {
	case *types.Basic:
		switch u.Kind() {
		case types.Int8, types.Int16, types.Int32, types.Int64, types.Uint8, types.Uint16, types.Uint32, types.Uint64,
			types.Float32, types.Float64, types.Complex64, types.Complex128:
			return true
		default:
			return false
		}
	case *types.Array:
		return u.Len() > 0 && g.alias(u.Elem())
	case *types.Named:
		ti := g.info(u)
		if ti == nil || ti.sized {
			return false
		}
		switch ti.kind {
		case kEnum:
			return g.alias(u.Underlying())
		case kNArray:
			return g.alias(u.Underlying())
		case kValue:
			s := underStruct(u)
			if s.NumFields() == 0 || hasUnexported(s) {
				return false
			}
			for f := range s.Fields() {
				if !g.alias(f.Type()) {
					return false
				}
			}
			return true
		default:
			return false
		}
	default:
		return false
	}
}

func (g *gen) decl(t types.Type, name string) (string, error) {
	if a, ok := t.(*types.Array); ok {
		return g.decl(a.Elem(), fmt.Sprintf("%s[%d]", name, a.Len()))
	}
	c, err := g.ctype(t)
	if err != nil {
		return "", err
	}
	return c + " " + name, nil
}

func (g *gen) ctype(t types.Type) (string, error) {
	if c, ok := g.scalar(t); ok {
		return c, nil
	}
	if ti := g.info(t); ti != nil {
		switch ti.kind {
		case kNArray, kValue, kClosed:
			return ti.c, nil
		case kNFunc:
			f, err := g.fnType(underSignature(ti.named), ti.c)
			if err != nil {
				return "", err
			}
			return f.c, nil
		default:
			return "", fmt.Errorf("%s has no C value form", ti.obj.Name())
		}
	}
	if s, ok := t.(*types.Signature); ok {
		f, err := g.fnType(s, "")
		if err != nil {
			return "", err
		}
		return f.c, nil
	}
	return "", fmt.Errorf("type %s has no C form", t)
}

func (g *gen) handleC(t types.Type) (string, bool) {
	if ti := g.info(t); ti != nil && (ti.kind == kHandle || ti.kind == kOpen) {
		return ti.c, true
	}
	return "", false
}

func (g *gen) cgoType(c string) string {
	switch c {
	case cBool, cFloat, cDouble:
		return "C." + c
	case "char":
		return "C.char"
	}
	return "C." + c
}

func (g *gen) cgoDeclType(t types.Type) (string, error) {
	for {
		a, ok := t.(*types.Array)
		if !ok {
			break
		}
		t = a.Elem()
	}
	c, err := g.ctype(t)
	if err != nil {
		return "", err
	}
	return g.cgoType(c), nil
}

func underStruct(t types.Type) *types.Struct {
	s, _ := t.Underlying().(*types.Struct)
	return s
}

func underInterface(t types.Type) *types.Interface {
	s, _ := t.Underlying().(*types.Interface)
	return s
}

func underSignature(t types.Type) *types.Signature {
	s, _ := t.Underlying().(*types.Signature)
	return s
}

func underArray(t types.Type) *types.Array {
	s, _ := t.Underlying().(*types.Array)
	return s
}

func sliceElem(t types.Type) types.Type {
	s, _ := t.(*types.Slice)
	return s.Elem()
}

func errorIface() *types.Interface {
	return underInterface(types.Universe.Lookup("error").Type())
}
