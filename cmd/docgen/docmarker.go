package main

import (
	"fmt"
	"go/ast"
	"go/doc"
	"go/doc/comment"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
)

// rootDir returns the directory that a pkg= names relative to the
// repository root, for the commands outside the v2 module.
func (g *gen) rootDir(p string) string {
	return filepath.Join(g.root, filepath.FromSlash(strings.Trim(p, "/")))
}

// rootPackage reads the package doc of a directory outside the v2 module.
func (g *gen) rootPackage(p string) (*pkgInfo, error) {
	dir := g.rootDir(p)
	fset := token.NewFileSet()
	es, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var files []*ast.File
	for _, e := range es {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") || strings.HasSuffix(e.Name(), "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, filepath.Join(dir, e.Name()), nil, parser.ParseComments)
		if err != nil {
			return nil, err
		}
		files = append(files, f)
	}
	ip := "github.com/USA-RedDragon/dmrgo/" + strings.Trim(p, "/")
	d, err := doc.NewFromFiles(fset, files, ip, doc.AllDecls)
	if err != nil {
		return nil, err
	}
	return &pkgInfo{doc: d, path: ip}, nil
}

func (g *gen) fillDoc(m marker) (string, []outFile, error) {
	pi, err := g.info(m)
	if err != nil {
		if _, perr := g.pkgPath(m.args[argPkg]); perr == nil || !hasGo(g.rootDir(m.args[argPkg])) {
			return "", nil, err
		}
		if pi, err = g.rootPackage(m.args[argPkg]); err != nil {
			return "", nil, err
		}
	}
	text, err := docText(pi, m.args["name"])
	if err != nil {
		return "", nil, err
	}
	d := pi.doc.Parser().Parse(text)
	if sec := m.args["section"]; sec != "" {
		var blocks []comment.Block
		in := false
		for _, b := range d.Content {
			if h, ok := b.(*comment.Heading); ok {
				if in {
					break
				}
				in = headingText(h) == sec
				continue
			}
			if in {
				blocks = append(blocks, b)
			}
		}
		if !in && len(blocks) == 0 {
			return "", nil, fmt.Errorf("no heading # %s in the doc of %s", sec, describe(pi, m.args["name"]))
		}
		d.Content = blocks
	}
	p := pi.doc.Printer()
	p.DocLinkURL = func(l *comment.DocLink) string {
		c := *l
		if c.ImportPath == "" {
			c.ImportPath = pi.path
		}
		return c.DefaultURL("https://pkg.go.dev")
	}
	p.HeadingLevel = 3
	return strings.TrimRight(string(p.Markdown(d)), "\n"), nil, nil
}

func headingText(h *comment.Heading) string {
	var b strings.Builder
	for _, t := range h.Text {
		inlineText(&b, t)
	}
	return b.String()
}

func inlineText(b *strings.Builder, t comment.Text) {
	switch x := t.(type) {
	case comment.Plain:
		b.WriteString(string(x))
	case comment.Italic:
		b.WriteString(string(x))
	case *comment.Link:
		for _, y := range x.Text {
			inlineText(b, y)
		}
	case *comment.DocLink:
		for _, y := range x.Text {
			inlineText(b, y)
		}
	}
}

func describe(pi *pkgInfo, name string) string {
	if name == "" {
		return "package " + pi.path
	}
	return name + " in " + pi.path
}

func docText(pi *pkgInfo, name string) (string, error) {
	if name == "" {
		return pi.doc.Doc, nil
	}
	typ, method, _ := strings.Cut(name, ".")
	for _, t := range pi.doc.Types {
		if t.Name != typ {
			continue
		}
		if method == "" {
			return t.Doc, nil
		}
		for _, f := range t.Methods {
			if f.Name == method {
				return f.Doc, nil
			}
		}
	}
	for _, f := range pi.doc.Funcs {
		if f.Name == name {
			return f.Doc, nil
		}
	}
	for _, t := range pi.doc.Types {
		for _, f := range t.Funcs {
			if f.Name == name {
				return f.Doc, nil
			}
		}
	}
	for _, vs := range [][]*docValue{values(pi.doc.Consts), values(pi.doc.Vars), typeValues(pi)} {
		for _, v := range vs {
			for _, n := range v.names {
				if n == name {
					return v.doc, nil
				}
			}
		}
	}
	return "", fmt.Errorf("no doc for %s", describe(pi, name))
}

type docValue struct {
	names []string
	doc   string
}

func values(vs []*doc.Value) []*docValue {
	out := make([]*docValue, 0, len(vs))
	for _, v := range vs {
		for _, s := range v.Decl.Specs {
			spec, ok := s.(*ast.ValueSpec)
			if !ok {
				continue
			}
			d := spec.Doc.Text()
			if d == "" {
				d = spec.Comment.Text()
			}
			if d == "" {
				d = v.Doc
			}
			dv := &docValue{doc: d}
			for _, n := range spec.Names {
				dv.names = append(dv.names, n.Name)
			}
			out = append(out, dv)
		}
	}
	return out
}

func typeValues(pi *pkgInfo) []*docValue {
	out := make([]*docValue, 0, len(pi.doc.Types))
	for _, t := range pi.doc.Types {
		out = append(out, values(t.Consts)...)
		out = append(out, values(t.Vars)...)
	}
	return out
}
