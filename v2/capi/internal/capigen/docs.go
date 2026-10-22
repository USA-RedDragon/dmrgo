package capigen

import (
	"go/types"
	"regexp"
	"strings"
)

var (
	goRef  = regexp.MustCompile(`(?:\b[Aa]n )?\b[a-z][a-z0-9]*\.[A-Z]\w*(?:\.[A-Z]\w*)?`)
	goHead = regexp.MustCompile(`^Go: [\w.]+\.(?:\s|$)`)
	nilAt  = regexp.MustCompile(`(^|\. )Nil `)
	goSel  = regexp.MustCompile(`\b([a-z][a-z0-9]*)((?:\.[A-Z]\w*)+)(\(\))?`)
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
		body := h[i+3 : i+j]
		head := goHead.FindString(body)
		b.WriteString(h[:i+3] + head)
		b.WriteString(goRef.ReplaceAllStringFunc(body[len(head):], func(w string) string {
			art, name, ok := strings.Cut(w, " ")
			if !ok {
				art, name = "", w
			} else {
				art = art[:1] + " "
			}
			if c, ok := refs[name]; ok {
				return art + c
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
