package capigen

import (
	"regexp"
	"strings"
)

var (
	goRef  = regexp.MustCompile(`(?:\b[Aa]n )?\b[a-z][a-z0-9]*\.[A-Z]\w*(?:\.[A-Z]\w*)?`)
	goHead = regexp.MustCompile(`^Go: [\w.]+\.(?:\s|$)`)
	nilAt  = regexp.MustCompile(`(^|\. )Nil `)
)

// cRefs maps the Go name of each export that has one C name, or a C type
// name first, to that C name. A field of a handle, which has a getter and
// a setter, has none.
func cRefs(exports []Export) map[string]string {
	m := map[string]string{}
	for _, e := range exports {
		if e.Err != "" || len(e.C) == 0 || (len(e.C) > 1 && e.Kind != "type") {
			continue
		}
		m[e.Go] = e.C[0]
	}
	return m
}

// cDocs replaces each qualified Go name, such as layer2.NewBurst or
// voice.ReceiverConfig.Scratch, in the comments of the header h with its C
// name from refs, and an "an" before it with "a". The "Go: pkg.Name." lead
// of a comment keeps its Go name.
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
