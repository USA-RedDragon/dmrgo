//go:build !tinygo

package rf

import (
	"go/ast"
	"go/build"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
)

type goSource struct {
	docs   map[string]string
	consts map[string][]string
	fields map[string][]string
	errs   []string
	pkg    []string
}

func oneLine(s string) string { return strings.Join(strings.Fields(s), " ") }

func loadGoSource(dir string) (*goSource, error) {
	src := &goSource{docs: map[string]string{}, consts: map[string][]string{}, fields: map[string][]string{}}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	fset := token.NewFileSet()
	var names []string
	for _, e := range entries {
		n := e.Name()
		if !strings.HasSuffix(n, ".go") || strings.HasSuffix(n, "_test.go") {
			continue
		}
		if ok, err := build.Default.MatchFile(dir, n); err != nil || !ok {
			continue
		}
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		f, err := parser.ParseFile(fset, filepath.Join(dir, n), nil, parser.ParseComments)
		if err != nil {
			return nil, err
		}
		src.file(f)
	}
	return src, nil
}

func (s *goSource) put(key string, cg *ast.CommentGroup) {
	if cg == nil || key == "" {
		return
	}
	if _, ok := s.docs[key]; !ok {
		s.docs[key] = oneLine(cg.Text())
	}
}

func (s *goSource) file(f *ast.File) {
	if f.Doc != nil && s.pkg == nil {
		for _, para := range strings.Split(f.Doc.Text(), "\n\n") {
			if strings.HasPrefix(para, "\t") || strings.HasPrefix(para, "(") {
				continue
			}
			s.pkg = append(s.pkg, oneLine(para))
		}
	}
	for _, d := range f.Decls {
		switch d := d.(type) {
		case *ast.FuncDecl:
			name := d.Name.Name
			if d.Recv != nil && len(d.Recv.List) == 1 {
				t := d.Recv.List[0].Type
				if st, ok := t.(*ast.StarExpr); ok {
					t = st.X
				}
				if id, ok := t.(*ast.Ident); ok {
					name = id.Name + "." + name
				}
			}
			s.put(name, d.Doc)
		case *ast.GenDecl:
			s.genDecl(d)
		}
	}
	ast.Inspect(f, func(n ast.Node) bool {
		cl, ok := n.(*ast.CompositeLit)
		if !ok {
			return true
		}
		sel, ok := cl.Type.(*ast.SelectorExpr)
		if !ok || sel.Sel.Name != "ConfigError" {
			return true
		}
		for _, e := range cl.Elts {
			kv, ok := e.(*ast.KeyValueExpr)
			if !ok {
				continue
			}
			if k, ok := kv.Key.(*ast.Ident); ok && k.Name == "Field" {
				if lit, ok := kv.Value.(*ast.BasicLit); ok {
					if v, err := strconv.Unquote(lit.Value); err == nil {
						s.errs = append(s.errs, v)
					}
				}
			}
		}
		return true
	})
}

func (s *goSource) genDecl(d *ast.GenDecl) {
	var typ string
	for _, spec := range d.Specs {
		switch sp := spec.(type) {
		case *ast.TypeSpec:
			doc := sp.Doc
			if doc == nil && len(d.Specs) == 1 {
				doc = d.Doc
			}
			s.put(sp.Name.Name, doc)
			if st, ok := sp.Type.(*ast.StructType); ok {
				for _, fl := range st.Fields.List {
					for _, n := range fl.Names {
						s.put(sp.Name.Name+"."+n.Name, fl.Doc)
						if n.IsExported() {
							s.fields[sp.Name.Name] = append(s.fields[sp.Name.Name], n.Name)
						}
					}
				}
			}
		case *ast.ValueSpec:
			if id, ok := sp.Type.(*ast.Ident); ok {
				typ = id.Name
			} else if sp.Type != nil || len(sp.Values) > 0 {
				typ = ""
			}
			doc := sp.Doc
			if doc == nil && len(d.Specs) == 1 {
				doc = d.Doc
			}
			for _, n := range sp.Names {
				s.put(n.Name, doc)
				if d.Tok == token.CONST && typ != "" && n.IsExported() {
					s.consts[typ] = append(s.consts[typ], n.Name)
				}
			}
		}
	}
}

func (s *goSource) doc(key string) string { return s.docs[key] }

func (s *goSource) configErrors(prefixes ...string) []string {
	seen := map[string]bool{}
	var out []string
	for _, e := range s.errs {
		for _, p := range prefixes {
			if strings.HasPrefix(e, p) && !seen[e] && slices.Contains(s.fields[strings.TrimSuffix(p, ".")], strings.TrimPrefix(e, p)) {
				seen[e] = true
				out = append(out, e)
			}
		}
	}
	sort.Strings(out)
	return out
}
