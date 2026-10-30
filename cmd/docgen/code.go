package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

func langOf(path string) string {
	switch ext := filepath.Ext(path); ext {
	case ".h":
		return "c"
	case ".yml":
		return "yaml"
	case ".py":
		return "python"
	case ".go", ".c", ".sh", ".yaml", ".json", ".js", ".html", ".css", ".lua", ".toml":
		return ext[1:]
	}
	return ""
}

func parseRange(s string) (int, int, error) {
	a, b, ok := strings.Cut(s, "-")
	x, err1 := strconv.Atoi(a)
	y, err2 := strconv.Atoi(b)
	if !ok || err1 != nil || err2 != nil || x < 1 || y < x {
		return 0, 0, fmt.Errorf("lines=%s is not a range a-b with 1 <= a <= b", s)
	}
	return x, y, nil
}

func (g *gen) fillCode(pg *page, m marker) (string, []outFile, error) {
	rel := m.args["file"]
	path := filepath.Join(g.root, filepath.FromSlash(rel))
	src, err := os.ReadFile(path)
	if err != nil {
		return "", nil, err
	}
	if sym := m.args["symbol"]; sym != "" {
		ex, err := symbolExcerpt(path, src, sym)
		if err != nil {
			return "", nil, err
		}
		return fence(langOf(path), ex), nil, nil
	}
	if m.args["lines"] == "" {
		return fence(langOf(path), strings.TrimRight(string(src), "\n")), nil, nil
	}
	fmt.Fprintf(g.log, "docgen: %s:%d: dmrgo:code uses lines=; symbol= does not break when the file changes\n", pg.rel, m.line)
	a, b, err := parseRange(m.args["lines"])
	if err != nil {
		return "", nil, err
	}
	lines := strings.Split(string(src), "\n")
	if b > len(lines) {
		return "", nil, fmt.Errorf("%s has %d lines, the range ends at %d", rel, len(lines), b)
	}
	excerpt := dedent(lines[a-1 : b])
	lang := langOf(path)
	want := fence(lang, excerpt)
	old := strings.Trim(m.body, "\n")
	if old != "" && old != want {
		if oa, ob, ok := findExcerpt(lines, old); ok {
			return "", nil, fmt.Errorf("range drifted: the excerpt in the page is now at lines=%d-%d of %s", oa, ob, rel)
		}
		return "", nil, fmt.Errorf("lines %d-%d of %s changed; check the range, then empty the marker body and run -update", a, b, rel)
	}
	if lang == "go" {
		if err := checkGoRange(path, src, a, b); err != nil {
			return "", nil, err
		}
	}
	return want, nil, nil
}

func dedent(lines []string) string {
	prefix := ""
	first := true
	for _, l := range lines {
		if strings.TrimSpace(l) == "" {
			continue
		}
		ws := l[:len(l)-len(strings.TrimLeft(l, " \t"))]
		if first {
			prefix, first = ws, false
			continue
		}
		for !strings.HasPrefix(ws, prefix) {
			prefix = prefix[:len(prefix)-1]
		}
	}
	out := make([]string, len(lines))
	for i, l := range lines {
		out[i] = strings.TrimPrefix(l, prefix)
		if strings.TrimSpace(out[i]) == "" {
			out[i] = ""
		}
	}
	return strings.Join(out, "\n")
}

func findExcerpt(lines []string, body string) (int, int, bool) {
	bl := strings.Split(body, "\n")
	if len(bl) < 3 {
		return 0, 0, false
	}
	inner := bl[1 : len(bl)-1]
	n := len(inner)
	for i := 0; i+n <= len(lines); i++ {
		if dedent(lines[i:i+n]) == strings.Join(inner, "\n") {
			return i + 1, i + n, true
		}
	}
	return 0, 0, false
}

// checkGoRange fails unless lines a to b hold whole syntax nodes that share
// one parent: one or more declarations, statements, specs or fields, each
// with its doc comment.
func checkGoRange(path string, src []byte, a, b int) error {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path, src, parser.ParseComments)
	if err != nil {
		return err
	}
	line := func(p token.Pos) int { return fset.Position(p).Line }
	starts, ends := map[int][]ast.Node{}, map[int][]ast.Node{}
	var stack []ast.Node
	ast.Inspect(f, func(n ast.Node) bool {
		if n == nil {
			stack = stack[:len(stack)-1]
			return true
		}
		var parent ast.Node
		if len(stack) > 0 {
			parent = stack[len(stack)-1]
		}
		stack = append(stack, n)
		switch x := n.(type) {
		case ast.Decl, ast.Stmt, ast.Spec, *ast.Field, *ast.KeyValueExpr:
			starts[line(n.Pos())] = append(starts[line(n.Pos())], parent)
			ends[line(n.End())] = append(ends[line(n.End())], parent)
			var d *ast.CommentGroup
			switch y := x.(type) {
			case *ast.FuncDecl:
				d = y.Doc
			case *ast.GenDecl:
				d = y.Doc
			case *ast.Field:
				d = y.Doc
			case *ast.ValueSpec:
				d = y.Doc
			case *ast.TypeSpec:
				d = y.Doc
			}
			if d != nil {
				starts[line(d.Pos())] = append(starts[line(d.Pos())], parent)
			}
		}
		return true
	})
	for _, p := range starts[a] {
		for _, q := range ends[b] {
			if p == q {
				return nil
			}
		}
	}
	return fmt.Errorf("lines %d-%d of %s do not hold whole declarations or statements; the range may have drifted", a, b, filepath.Base(path))
}

// symbolExcerpt returns the source of the func, method (Type.Method), type,
// or const or var block named sym, without its doc comment. For an Example
// func it leaves out the output comment.
func symbolExcerpt(path string, src []byte, sym string) (string, error) {
	switch filepath.Ext(path) {
	case ".go":
	case ".yaml", ".yml":
		return yamlExcerpt(src, sym)
	case ".c", ".h":
		return cExcerpt(src, sym)
	default:
		return "", fmt.Errorf("symbol= needs a Go, YAML or C file")
	}
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path, src, parser.ParseComments)
	if err != nil {
		return "", err
	}
	node := findSymbol(f, sym)
	if node == nil {
		return "", fmt.Errorf("no func, type, const or var %s in %s", sym, filepath.Base(path))
	}
	start := fset.Position(node.Pos()).Offset
	_, spec := node.(*ast.TypeSpec)
	if spec {
		start = lineStart(src, start)
	}
	text := string(src[start:fset.Position(node.End()).Offset])
	if fd, ok := node.(*ast.FuncDecl); ok && strings.HasPrefix(fd.Name.Name, "Example") && fd.Body != nil {
		outs := outputComments(f, fd)
		for i := len(outs) - 1; i >= 0; i-- {
			cs := lineStart(src, fset.Position(outs[i].Pos()).Offset) - start
			ce := fset.Position(outs[i].End()).Offset - start
			for ce < len(text) && text[ce] != '\n' {
				ce++
			}
			if ce < len(text) {
				ce++
			}
			text = text[:cs] + text[ce:]
		}
	}
	if spec {
		return dedent(strings.Split(text, "\n")), nil
	}
	return text, nil
}

func findSymbol(f *ast.File, sym string) ast.Node {
	recv, name, isMethod := strings.Cut(sym, ".")
	if !isMethod {
		name = sym
	}
	for _, d := range f.Decls {
		switch x := d.(type) {
		case *ast.FuncDecl:
			if x.Name.Name == name && (x.Recv != nil) == isMethod && (!isMethod || recvType(x) == recv) {
				return x
			}
		case *ast.GenDecl:
			if isMethod {
				continue
			}
			if n := findSpec(x, name); n != nil {
				return n
			}
		}
	}
	return nil
}

func recvType(fd *ast.FuncDecl) string {
	t := fd.Recv.List[0].Type
	if st, ok := t.(*ast.StarExpr); ok {
		t = st.X
	}
	if ix, ok := t.(*ast.IndexExpr); ok {
		t = ix.X
	}
	if id, ok := t.(*ast.Ident); ok {
		return id.Name
	}
	return ""
}

func findSpec(x *ast.GenDecl, name string) ast.Node {
	for _, s := range x.Specs {
		switch y := s.(type) {
		case *ast.TypeSpec:
			if y.Name.Name != name {
				continue
			}
			if len(x.Specs) > 1 {
				return y
			}
			return x
		case *ast.ValueSpec:
			for _, n := range y.Names {
				if n.Name == name {
					return x
				}
			}
		}
	}
	return nil
}

func outputComments(f *ast.File, fd *ast.FuncDecl) []*ast.CommentGroup {
	var out []*ast.CommentGroup
	for _, c := range f.Comments {
		if c.Pos() > fd.Body.Lbrace && c.End() < fd.Body.Rbrace {
			t := strings.TrimSpace(c.Text())
			if strings.HasPrefix(t, "Output:") || strings.HasPrefix(t, "Unordered output:") {
				out = append(out, c)
			}
		}
	}
	return out
}

func lineStart(src []byte, off int) int {
	for off > 0 && src[off-1] != '\n' {
		off--
	}
	return off
}

// yamlExcerpt returns the block of the key at the dotted path sym, such as
// jobs.test, with the key line and without the blank lines at its end. A
// last segment key[A:B] or key[A] selects named items of the list at key.
func yamlExcerpt(src []byte, sym string) (string, error) {
	path, items := sym, ""
	if j := strings.IndexByte(sym, '['); j >= 0 && strings.HasSuffix(sym, "]") {
		path, items = sym[:j], sym[j+1:len(sym)-1]
	}
	lines := strings.Split(string(src), "\n")
	start, end, indent := 0, len(lines), -1
	at := -1
	for _, key := range strings.Split(path, ".") {
		at = -1
		child := -1
		for i := start; i < end; i++ {
			t := strings.TrimLeft(lines[i], " ")
			if t == "" || strings.HasPrefix(t, "#") {
				continue
			}
			ind := len(lines[i]) - len(t)
			if ind <= indent {
				break
			}
			if child < 0 {
				child = ind
			}
			if ind == child && (t == key+":" || strings.HasPrefix(t, key+": ")) {
				at = i
				break
			}
		}
		if at < 0 {
			return "", fmt.Errorf("no key %s in the YAML file", path)
		}
		indent = len(lines[at]) - len(strings.TrimLeft(lines[at], " "))
		start, end = at+1, yamlBlockEnd(lines, at+1, end, indent)
	}
	if items != "" {
		return yamlItems(lines, start, end, sym, items)
	}
	for end > at+1 && strings.TrimSpace(lines[end-1]) == "" {
		end--
	}
	return dedent(lines[at:end]), nil
}

func yamlItems(lines []string, start, end int, sym, items string) (string, error) {
	first, last, ok := strings.Cut(items, ":")
	if !ok {
		last = first
	}
	var starts []int
	var names []string
	for i := start; i < end; i++ {
		t := strings.TrimLeft(lines[i], " ")
		if !strings.HasPrefix(t, "- ") {
			continue
		}
		if len(starts) > 0 && len(lines[i])-len(t) != len(lines[starts[0]])-len(strings.TrimLeft(lines[starts[0]], " ")) {
			continue
		}
		starts = append(starts, i)
		names = append(names, yamlItemName(lines, i, end))
	}
	a, b := -1, -1
	for k, n := range names {
		if n == first && a < 0 {
			a = k
		}
		if n == last && a >= 0 {
			b = k
			break
		}
	}
	if a < 0 || b < 0 {
		return "", fmt.Errorf("no items %s in the YAML file", sym)
	}
	stop := end
	if b+1 < len(starts) {
		stop = starts[b+1]
	}
	for stop > starts[a]+1 && strings.TrimSpace(lines[stop-1]) == "" {
		stop--
	}
	return dedent(lines[starts[a]:stop]), nil
}

func yamlItemName(lines []string, at, end int) string {
	ind := len(lines[at]) - len(strings.TrimLeft(lines[at], " "))
	for i := at; i < end; i++ {
		t := strings.TrimLeft(lines[i], " ")
		if i > at && t != "" && len(lines[i])-len(t) <= ind {
			break
		}
		t = strings.TrimPrefix(t, "- ")
		if n, ok := strings.CutPrefix(t, "name: "); ok {
			return strings.TrimSpace(n)
		}
	}
	return ""
}

// cExcerpt returns the #define of sym, with its continuation lines, or the
// definition of the function sym, from its first line at column 0 to the
// closing brace at column 0.
func cExcerpt(src []byte, sym string) (string, error) {
	lines := strings.Split(string(src), "\n")
	for i, l := range lines {
		if rest, ok := strings.CutPrefix(l, "#define "+sym); ok && (rest == "" || rest[0] == ' ' || rest[0] == '\t' || rest[0] == '(') {
			j := i
			for j < len(lines)-1 && strings.HasSuffix(lines[j], "\\") {
				j++
			}
			return strings.Join(lines[i:j+1], "\n"), nil
		}
	}
	for i, l := range lines {
		if l == "" || l[0] == ' ' || l[0] == '\t' || l[0] == '#' || l[0] == '/' || l[0] == '*' || strings.HasSuffix(strings.TrimSpace(l), ";") {
			continue
		}
		k := strings.Index(l, sym+"(")
		if k < 0 || (k > 0 && (isIdent(l[k-1]))) {
			continue
		}
		for j := i + 1; j < len(lines); j++ {
			if lines[j] == "}" {
				return strings.Join(lines[i:j+1], "\n"), nil
			}
		}
	}
	return "", fmt.Errorf("no #define or function %s in the C file", sym)
}

func isIdent(c byte) bool {
	return c == '_' || c >= '0' && c <= '9' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z'
}

func yamlBlockEnd(lines []string, from, end, indent int) int {
	for i := from; i < end; i++ {
		t := strings.TrimLeft(lines[i], " ")
		if t != "" && len(lines[i])-len(t) <= indent {
			return i
		}
	}
	return end
}
