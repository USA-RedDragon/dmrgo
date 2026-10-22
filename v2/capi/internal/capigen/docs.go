package capigen

import (
	"go/types"
	"regexp"
	"strings"
)

var (
	goRef     = regexp.MustCompile(`(?:\b[Aa]n )?\b[a-z][a-z0-9]*(?:\.[A-Z]\w*)+`)
	typeSel   = regexp.MustCompile(`(^|[^\w.])((?:[a-z][a-z0-9]*\.)?)([A-Z]\w*)((?:\.[A-Z]\w*)+)`)
	goHead    = regexp.MustCompile(`^Go: [\w.]+\.(?:\s|$)`)
	nilAt     = regexp.MustCompile(`(^|\. )Nil `)
	goSel     = regexp.MustCompile(`\b([a-z][a-z0-9]*)((?:\.[A-Z]\w*)+)(\(\))?`)
	goroutine = regexp.MustCompile(`\bgoroutine\b`)
)

func stdRef(name string) (string, bool) {
	if name == "net.UDPAddr" {
		return "a UDP socket address", true
	}
	return "", false
}

// cRefs maps the Go name of each export that has one C name, or a C type
// name first, to that C name, and a field of a handle, which has a getter
// and a setter, to its setter.
func cRefs(exports []Export) map[string]string {
	m := map[string]string{}
	for _, e := range exports {
		switch {
		case e.Err != "" || len(e.C) == 0:
		case len(e.C) == 1 || e.Kind == "type":
			m[e.Go] = e.C[0]
		case e.Kind == "field":
			for _, c := range e.C {
				if strings.Contains(c, "_set_") {
					m[e.Go] = c
				}
			}
		}
	}
	return m
}

// cDocs replaces each qualified Go name, such as layer2.NewBurst or
// voice.ReceiverConfig.Scratch, in the comments of the header h with its C
// name from refs, and an "an" before it with "a", and a standard library
// type, such as net.UDPAddr, with words. The "Go: pkg.Name." lead of a
// comment keeps its Go name.
func cDocs(h string, refs map[string]string) string {
	var b strings.Builder
	for {
		i := strings.Index(h, "/* ")
		if i < 0 {
			break
		}
		j := strings.Index(h[i:], "*/")
		if j < 0 {
			break
		}
		body := goroutine.ReplaceAllString(h[i+3:i+j], "thread")
		head := goHead.FindString(body)
		b.WriteString(h[:i+3] + head)
		b.WriteString(goRef.ReplaceAllStringFunc(body[len(head):], func(w string) string {
			art, name, ok := strings.Cut(w, " ")
			if !ok {
				art, name = "", w
			} else {
				art = art[:1] + " "
			}
			for n, rest := name, ""; strings.Count(n, ".") > 1 || n == name; {
				if c, ok := refs[n]; ok {
					return art + c + rest
				}
				i := strings.LastIndex(n, ".")
				n, rest = n[:i], n[i:]+rest
			}
			if c, ok := stdRef(name); ok {
				return c
			}
			return w
		}))
		b.WriteString("*/")
		h = h[i+j+2:]
	}
	b.WriteString(h)
	return b.String()
}

// nilC gives the C form of a "Nil" that starts a sentence of the doc d of
// a member with the declarations decls: the has_ flag false for an
// optional value, or NULL for a pointer to a handle.
func nilC(d string, decls []string) string {
	var c string
	switch {
	case len(decls) == 2 && strings.HasPrefix(decls[0], "bool has_"):
		c = cName(decls[0]) + " false "
	case len(decls) == 1 && strings.Contains(decls[0], " *"):
		c = "NULL "
	default:
		return d
	}
	return nilAt.ReplaceAllString(d, "${1}"+c)
}

// selDocs gives the C form of each selector on the receiver or a parameter
// of fn in the doc d: a field of a struct, such as h.Slot, as h->slot, a
// field of a handle, such as b.VoiceBurst, as the voice_burst of b, and a
// method, such as slc.Encode, as its qualified Go name on slc, which cDocs
// then names in C. The receiver is self in C.
func (g *gen) selDocs(d string, fn *types.Func) string {
	sig := fn.Signature()
	vars := map[string]*types.Var{}
	cn := map[string]string{}
	for i := range sig.Params().Len() {
		v := sig.Params().At(i)
		vars[v.Name()], cn[v.Name()] = v, cKeyword(v.Name())
	}
	if r := sig.Recv(); r != nil && r.Name() != "" {
		vars[r.Name()], cn[r.Name()] = r, "self"
	}
	return goSel.ReplaceAllStringFunc(d, func(w string) string {
		m := goSel.FindStringSubmatch(w)
		v, ok := vars[m[1]]
		if !ok {
			return w
		}
		sels := strings.Split(m[2][1:], ".")
		expr, t := cn[m[1]], v.Type()
		for i, sel := range sels {
			obj, _, _ := types.LookupFieldOrMethod(t, true, nil, sel)
			last := i == len(sels)-1
			switch o := obj.(type) {
			case *types.Var:
				if g.kindOf(derefType(t)) == kHandle {
					if !last {
						return w
					}
					return "the " + cKeyword(snake(sel)) + " of " + expr
				}
				conn := "."
				if i == 0 {
					conn = "->"
				}
				expr, t = expr+conn+cKeyword(snake(sel)), o.Type()
			case *types.Func:
				if !last {
					return w
				}
				n, ok := derefType(t).(*types.Named)
				if !ok || n.Obj().Pkg() == nil {
					return w
				}
				return n.Obj().Pkg().Name() + "." + n.Obj().Name() + "." + sel + " on " + expr
			default:
				return w
			}
		}
		return expr
	})
}

// qualify gives each Go selector in the doc d its qualified Go name, such
// as dmrdsession.Output.ID, which cDocs then names in C. A selector starts
// at an exported type of pkg, such as Output.ID, or of the one package
// that pkg imports with that name, or at a type that a package name
// qualifies, such as pdu.DataHeader.FEC.Uncorrectable, or at a field of
// in, the struct of a field or the receiver of a method, such as
// FEC.ErrorsCorrected. It keeps in l.refs the field and method exports
// that each such name goes through.
func (l *loader) qualify(d string, pkg *types.Package, in *types.Named) string {
	return typeSel.ReplaceAllStringFunc(d, func(w string) string {
		m := typeSel.FindStringSubmatch(w)
		sels := strings.Split(m[4][1:], ".")
		var name string
		var chains [][]string
		if m[2] != "" {
			if tn := typeOf(pkg, strings.TrimSuffix(m[2], "."), m[3]); tn != nil {
				if keys, ok := selKeys(tn.Type(), sels); ok {
					name, chains = m[2]+m[3], [][]string{keys}
				}
			}
		} else {
			if in != nil && directField(in, m[3]) {
				if keys, ok := selKeys(in, append([]string{m[3]}, sels...)); ok {
					name, chains = pkg.Name()+"."+in.Obj().Name()+"."+m[3], [][]string{keys}
				}
			}
			if tn := typeIn(pkg, m[3]); tn != nil {
				if keys, ok := selKeys(tn.Type(), sels); ok {
					if name == "" {
						name = tn.Pkg().Name() + "." + m[3]
					}
					chains = append(chains, keys)
				}
			}
		}
		if name == "" {
			return w
		}
		name += m[4]
		l.refs[name] = append(l.refs[name], chains...)
		return m[1] + name
	})
}

// typeOf finds the exported type name in pkg or in a package that pkg
// imports, by the name of the package.
func typeOf(pkg *types.Package, pkgName, name string) *types.TypeName {
	for _, p := range append([]*types.Package{pkg}, pkg.Imports()...) {
		if p.Name() == pkgName {
			if tn, ok := p.Scope().Lookup(name).(*types.TypeName); ok && tn.Exported() {
				return tn
			}
		}
	}
	return nil
}

// typeIn finds the exported type name in pkg, or else in the one package
// that pkg imports that has it.
func typeIn(pkg *types.Package, name string) *types.TypeName {
	if tn, ok := pkg.Scope().Lookup(name).(*types.TypeName); ok {
		return tn
	}
	var found *types.TypeName
	for _, p := range pkg.Imports() {
		if tn, ok := p.Scope().Lookup(name).(*types.TypeName); ok && tn.Exported() {
			if found != nil {
				return nil
			}
			found = tn
		}
	}
	return found
}

func directField(n *types.Named, name string) bool {
	st, ok := n.Underlying().(*types.Struct)
	if !ok {
		return false
	}
	for f := range st.Fields() {
		if f.Name() == name {
			return true
		}
	}
	return false
}

// selKeys gives the Go names of the exports that the selectors sels on a
// value of type t go through, such as dmrdsession.Output.Grant and
// trunk.Grant.Source, with a method only last.
func selKeys(t types.Type, sels []string) ([]string, bool) {
	var keys []string
	for i, sel := range sels {
		n, ok := types.Unalias(derefType(t)).(*types.Named)
		if !ok || n.Obj().Pkg() == nil {
			return nil, false
		}
		obj, idx, _ := types.LookupFieldOrMethod(n, true, n.Obj().Pkg(), sel)
		if len(idx) != 1 {
			return nil, false
		}
		keys = append(keys, n.Obj().Pkg().Name()+"."+n.Obj().Name()+"."+sel)
		switch o := obj.(type) {
		case *types.Var:
			t = o.Type()
		case *types.Func:
			if i != len(sels)-1 {
				return nil, false
			}
		default:
			return nil, false
		}
	}
	return keys, true
}

// chainRefs adds to refs the C name of each qualified Go name of chains
// that goes through more than one export, from the first of its chains
// that has one: the C member path, such as
// dmrgo_trunk_output_t.grant.source, when each export is a field of a
// struct, with the C function of a method last on it.
func chainRefs(refs map[string]string, chains map[string][][]string) {
	for name, cs := range chains {
		for _, keys := range cs {
			if c, ok := chainC(refs, keys); ok {
				refs[name] = c
				break
			}
		}
	}
}

func chainC(refs map[string]string, keys []string) (string, bool) {
	c, ok := refs[keys[0]]
	for j, k := range keys[1:] {
		next, found := refs[k]
		i := strings.Index(next, ".")
		if !ok || !found || !strings.Contains(c, ".") || (i < 0 && j < len(keys)-2) {
			return "", false
		}
		if i < 0 {
			c = next + " on " + c
		} else {
			c += next[i:]
		}
	}
	return c, ok
}
