package main

import (
	"fmt"
	"go/ast"
	"go/doc"
	"go/types"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"golang.org/x/tools/go/packages"
)

const modPath = "github.com/USA-RedDragon/dmrgo/v2"

type gen struct {
	root    string
	v2      string
	log     io.Writer
	pkgs    map[string]bool
	loaded  map[string]*pkgInfo
	loadErr map[string]error
	dirs    []string

	examples map[string]bool
	exOut    map[string]string
	exErr    map[string]error
	tests    map[string]map[string]bool
	testsOK  map[string]error
	machines map[string]map[string]string
	states   map[string]*stateRun
}

type pkgInfo struct {
	pkg  *packages.Package
	doc  *doc.Package
	exs  []*doc.Example
	path string
}

func newGen(root string, log io.Writer) *gen {
	return &gen{
		root:     root,
		v2:       filepath.Join(root, "v2"),
		log:      log,
		pkgs:     map[string]bool{},
		examples: map[string]bool{},
		tests:    map[string]map[string]bool{},
		machines: map[string]map[string]string{},
	}
}

func importPath(p string) string {
	p = strings.Trim(p, "/")
	switch {
	case p == "" || p == "." || p == modPath:
		return modPath
	case strings.HasPrefix(p, modPath+"/"):
		return p
	}
	return modPath + "/" + p
}

func shortPath(ip string) string {
	if ip == modPath {
		return "."
	}
	return strings.TrimPrefix(ip, modPath+"/")
}

func splitTest(s string) (pkg, name string) {
	dot := strings.LastIndex(s, ".")
	if dot < 0 {
		return "", s
	}
	return s[:dot], s[dot+1:]
}

// pkgPath returns the import path that the pkg= of a marker names: a path
// under v2, or the last elements of exactly one package path under v2
// ("dmrdsession" for transport/dmrd/dmrdsession).
func (g *gen) pkgPath(p string) (string, error) {
	ip := importPath(p)
	if hasGo(filepath.Join(g.v2, filepath.FromSlash(shortPath(ip)))) {
		return ip, nil
	}
	if g.dirs == nil {
		if err := filepath.WalkDir(g.v2, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if !d.IsDir() {
				return nil
			}
			name := d.Name()
			if path != g.v2 && (name == "testdata" || strings.HasPrefix(name, ".") || strings.HasPrefix(name, "_")) {
				return filepath.SkipDir
			}
			if hasGo(path) {
				rel, _ := filepath.Rel(g.v2, path)
				g.dirs = append(g.dirs, filepath.ToSlash(rel))
			}
			return nil
		}); err != nil {
			return "", err
		}
	}
	var found []string
	for _, d := range g.dirs {
		if strings.HasSuffix("/"+d, "/"+strings.Trim(p, "/")) {
			found = append(found, d)
		}
	}
	switch len(found) {
	case 0:
		return "", fmt.Errorf("no package %s under v2", p)
	case 1:
		return modPath + "/" + found[0], nil
	}
	return "", fmt.Errorf("pkg=%s names several packages: %s", p, strings.Join(found, ", "))
}

func hasGo(dir string) bool {
	es, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	for _, e := range es {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".go") {
			return true
		}
	}
	return false
}

func (g *gen) plan(m marker) {
	if p, ok := m.args[argPkg]; ok {
		if ip, err := g.pkgPath(p); err == nil {
			m.args[argPkg] = shortPath(ip)
		}
	}
	if t, ok := m.args["test"]; ok {
		pkg, name := splitTest(t)
		if ip, err := g.pkgPath(pkg); err == nil {
			m.args["test"] = shortPath(ip) + "." + name
		}
	}
	switch m.kind {
	case kindDoc:
		if _, err := g.pkgPath(m.args[argPkg]); err != nil && hasGo(g.rootDir(m.args[argPkg])) {
			return
		}
		g.pkgs[importPath(m.args[argPkg])] = true
	case kindConfig, kindConst, kindErrors:
		g.pkgs[importPath(m.args[argPkg])] = true
		if m.kind == kindErrors {
			g.pkgs[modPath] = true
		}
	case kindStates:
		ip := importPath(m.args[argPkg])
		g.pkgs[ip] = true
		if g.machines[ip] == nil {
			g.machines[ip] = map[string]string{}
		}
		g.machines[ip][m.args["type"]] = m.args["run"]
	case kindOutput:
		if ex := m.args["example"]; ex != "" {
			g.examples[ex] = true
			return
		}
		pkg, name := splitTest(m.args["test"])
		ip := importPath(pkg)
		g.pkgs[ip] = true
		if g.tests[ip] == nil {
			g.tests[ip] = map[string]bool{}
		}
		g.tests[ip][strings.TrimPrefix(name, "Example")] = true
	}
}

func (g *gen) prepare() error {
	if err := g.load(); err != nil {
		return err
	}
	if err := g.runExamples(); err != nil {
		return err
	}
	g.runTests()
	return g.runStates()
}

func (g *gen) load() error {
	g.loaded = map[string]*pkgInfo{}
	if len(g.pkgs) == 0 {
		return nil
	}
	var pats []string
	for p := range g.pkgs {
		pats = append(pats, p)
	}
	sort.Strings(pats)
	cfg := &packages.Config{
		Mode:  packages.NeedName | packages.NeedFiles | packages.NeedSyntax | packages.NeedTypes | packages.NeedTypesInfo | packages.NeedImports | packages.NeedDeps,
		Dir:   g.v2,
		Tests: true,
		Env:   offlineEnv(),
	}
	ps, err := packages.Load(cfg, pats...)
	if err != nil {
		return err
	}
	g.loadErr = map[string]error{}
	for _, p := range ps {
		if strings.HasSuffix(p.ID, ".test") {
			continue
		}
		base := strings.TrimSuffix(p.PkgPath, "_test")
		var errs []string
		for _, e := range p.Errors {
			errs = append(errs, e.Error())
		}
		if len(errs) > 0 {
			g.loadErr[base] = fmt.Errorf("loading %s: %s", shortPath(base), strings.Join(errs, "; "))
			continue
		}
		test := strings.Contains(p.ID, "[")
		ext := strings.HasSuffix(p.PkgPath, "_test")
		info := g.loaded[base]
		if info == nil {
			info = &pkgInfo{path: base}
			g.loaded[base] = info
		}
		var tf []*ast.File
		for _, f := range p.Syntax {
			if strings.HasSuffix(p.Fset.File(f.Pos()).Name(), "_test.go") {
				tf = append(tf, f)
			}
		}
		info.exs = append(info.exs, doc.Examples(tf...)...)
		if !test && !ext {
			info.pkg = p
			d, err := doc.NewFromFiles(p.Fset, p.Syntax, p.PkgPath, doc.PreserveAST|doc.AllDecls)
			if err != nil {
				return err
			}
			info.doc = d
		}
	}
	for _, p := range pats {
		if g.loadErr[p] == nil && (g.loaded[p] == nil || g.loaded[p].pkg == nil) {
			g.loadErr[p] = fmt.Errorf("package %s not found", shortPath(p))
		}
	}
	return nil
}

func dedupeExamples(in []*doc.Example) map[string]*doc.Example {
	out := map[string]*doc.Example{}
	for _, e := range in {
		out[e.Name] = e
	}
	return out
}

func (g *gen) info(m marker) (*pkgInfo, error) {
	ip, err := g.pkgPath(m.args[argPkg])
	if err != nil {
		return nil, err
	}
	if err := g.loadErr[ip]; err != nil {
		return nil, err
	}
	pi := g.loaded[ip]
	if pi == nil || pi.pkg == nil {
		return nil, fmt.Errorf("package %s not loaded", ip)
	}
	return pi, nil
}

func (g *gen) fill(pg *page, m marker) (string, []outFile, error) {
	switch m.kind {
	case kindOutput:
		if ex := m.args["example"]; ex != "" {
			if err := g.exErr[ex]; err != nil {
				return "", nil, err
			}
			out, ok := g.exOut[ex]
			if !ok {
				return "", nil, fmt.Errorf("no example %s under v2/examples", ex)
			}
			return fence("text", out), nil, nil
		}
		return g.fillTest(m)
	case kindCode:
		return g.fillCode(pg, m)
	case kindConst:
		return g.fillConst(m)
	case kindConfig:
		return g.fillConfig(m)
	case kindErrors:
		return g.fillErrors(m)
	case kindStates:
		return g.fillStates(pg, m)
	case kindDoc:
		return g.fillDoc(m)
	case kindGoMod:
		return g.fillGoMod(m)
	case kindFlags:
		return g.fillFlags(m)
	}
	return "", nil, fmt.Errorf("unknown kind %s", m.kind)
}

func fence(lang, s string) string {
	s = strings.TrimRight(s, "\n")
	f := "```"
	for strings.Contains(s, f) {
		f += "`"
	}
	return f + lang + "\n" + s + "\n" + f
}

func (g *gen) fillTest(m marker) (string, []outFile, error) {
	pkg, name := splitTest(m.args["test"])
	ip, err := g.pkgPath(pkg)
	if err != nil {
		return "", nil, err
	}
	if err := g.loadErr[ip]; err != nil {
		return "", nil, err
	}
	pi := g.loaded[ip]
	name = strings.TrimPrefix(name, "Example")
	e := dedupeExamples(pi.exs)[name]
	if e == nil {
		return "", nil, fmt.Errorf("no func Example%s in %s", name, ip)
	}
	if e.Output == "" && !e.EmptyOutput {
		return "", nil, fmt.Errorf("func Example%s has no // Output: comment", name)
	}
	if err := g.testsOK[ip]; err != nil {
		return "", nil, err
	}
	return fence("text", e.Output), nil, nil
}

func (g *gen) fillConst(m marker) (string, []outFile, error) {
	pi, err := g.info(m)
	if err != nil {
		return "", nil, err
	}
	name := m.args["name"]
	switch obj := pi.pkg.Types.Scope().Lookup(name).(type) {
	case *types.Const:
		return "`" + constString(obj.Type(), obj.Val().ExactString()) + "`", nil, nil
	case *types.Var:
		for _, f := range pi.pkg.Syntax {
			for _, d := range f.Decls {
				gd, ok := d.(*ast.GenDecl)
				if !ok {
					continue
				}
				for _, s := range gd.Specs {
					vs, ok := s.(*ast.ValueSpec)
					if !ok {
						continue
					}
					for i, n := range vs.Names {
						if n.Name != name || pi.pkg.TypesInfo.Defs[n] != obj {
							continue
						}
						if i >= len(vs.Values) {
							return "", nil, fmt.Errorf("var %s has no initial value", name)
						}
						tv := pi.pkg.TypesInfo.Types[vs.Values[i]]
						if tv.Value == nil {
							return "", nil, fmt.Errorf("the initial value of var %s is not a constant expression", name)
						}
						return "`" + constString(obj.Type(), tv.Value.ExactString()) + "`", nil, nil
					}
				}
			}
		}
	}
	return "", nil, fmt.Errorf("%s is not a constant or a package-level var of %s", name, pi.path)
}

func cell(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	return strings.ReplaceAll(s, "|", `\|`)
}

func pkgLink(ip, name string) string {
	return "https://pkg.go.dev/" + ip + "#" + name
}

func typeString(t types.Type, here *types.Package) string {
	return types.TypeString(t, func(p *types.Package) string {
		if p == here {
			return ""
		}
		return p.Name()
	})
}

func lookupType(pi *pkgInfo, name string) (*types.TypeName, *doc.Type, error) {
	tn, ok := pi.pkg.Types.Scope().Lookup(name).(*types.TypeName)
	if !ok {
		return nil, nil, fmt.Errorf("%s is not a type of %s", name, pi.path)
	}
	for _, t := range pi.doc.Types {
		if t.Name == name {
			return tn, t, nil
		}
	}
	return tn, nil, nil
}

func fieldDocs(dt *doc.Type) map[string]string {
	out := map[string]string{}
	if dt == nil {
		return out
	}
	for _, spec := range dt.Decl.Specs {
		ts, ok := spec.(*ast.TypeSpec)
		if !ok {
			continue
		}
		st, ok := ts.Type.(*ast.StructType)
		if !ok {
			continue
		}
		for _, f := range st.Fields.List {
			for _, n := range f.Names {
				out[n.Name] = f.Doc.Text()
			}
		}
	}
	return out
}

func (g *gen) fillConfig(m marker) (string, []outFile, error) {
	pi, err := g.info(m)
	if err != nil {
		return "", nil, err
	}
	tn, dt, err := lookupType(pi, m.args["type"])
	if err != nil {
		return "", nil, err
	}
	st, ok := tn.Type().Underlying().(*types.Struct)
	if !ok {
		return "", nil, fmt.Errorf("%s is not a struct", tn.Name())
	}
	docs := fieldDocs(dt)
	defs := findDefaults(pi, st)
	var b strings.Builder
	b.WriteString("| Field | Type | Default | Description |\n|---|---|---|---|\n")
	for i := 0; i < st.NumFields(); i++ {
		f := st.Field(i)
		if !f.Exported() {
			continue
		}
		def := defs[f]
		if len(def) == 0 {
			if claimsDefault(docs[f.Name()]) {
				def = []string{"not machine-readable; see the description"}
			} else {
				def = []string{"zero value"}
			}
		}
		fmt.Fprintf(&b, "| `%s` | `%s` | %s | %s |\n", f.Name(), cell(typeString(f.Type(), pi.pkg.Types)), cell(strings.Join(def, " or ")), cell(docs[f.Name()]))
	}
	return b.String(), nil, nil
}

var defaultClaim = regexp.MustCompile(`(?i)\b(0|nil|zero|empty|an? empty \w+|a zero \w+) means\b|defaults to|by default`)

func claimsDefault(d string) bool {
	return defaultClaim.MatchString(strings.Join(strings.Fields(d), " "))
}

func errorType() *types.Interface {
	i, _ := types.Universe.Lookup("error").Type().Underlying().(*types.Interface)
	return i
}

type errInfo struct {
	name string
	ip   string
	doc  string
	by   []string
}

func (g *gen) errorIndex() map[string]*errInfo {
	idx := map[string]*errInfo{}
	for ip, pi := range g.loaded {
		if pi.pkg == nil {
			continue
		}
		add := func(name, d string) {
			key := pi.pkg.Name + "." + name
			idx[key] = &errInfo{name: name, ip: ip, doc: d}
		}
		for _, v := range pi.doc.Vars {
			for _, s := range v.Decl.Specs {
				vs, ok := s.(*ast.ValueSpec)
				if !ok {
					continue
				}
				d := vs.Doc.Text()
				if d == "" {
					d = vs.Comment.Text()
				}
				if d == "" {
					d = v.Doc
				}
				for _, n := range vs.Names {
					obj := pi.pkg.Types.Scope().Lookup(n.Name)
					if obj != nil && obj.Exported() && types.Implements(obj.Type(), errorType()) {
						add(n.Name, d)
					}
				}
			}
		}
		for _, t := range pi.doc.Types {
			obj := pi.pkg.Types.Scope().Lookup(t.Name)
			if obj == nil || !obj.Exported() {
				continue
			}
			if types.Implements(types.NewPointer(obj.Type()), errorType()) {
				add(t.Name, t.Doc)
			}
		}
	}
	return idx
}

func (g *gen) fillErrors(m marker) (string, []outFile, error) {
	pi, err := g.info(m)
	if err != nil {
		return "", nil, err
	}
	_, dt, err := lookupType(pi, m.args["type"])
	if err != nil {
		return "", nil, err
	}
	if dt == nil {
		return "", nil, fmt.Errorf("no doc for %s", m.args["type"])
	}
	idx := g.errorIndex()
	type fn struct{ name, doc string }
	var fns []fn
	for _, f := range dt.Funcs {
		fns = append(fns, fn{f.Name, f.Doc})
	}
	for _, f := range dt.Methods {
		fns = append(fns, fn{dt.Name + "." + f.Name, f.Doc})
	}
	found := map[string]*errInfo{}
	for _, f := range fns {
		for _, w := range docWords(f.doc) {
			key := w
			if !strings.Contains(w, ".") {
				key = pi.pkg.Name + "." + w
			}
			e := idx[key]
			if e == nil {
				continue
			}
			if found[key] == nil {
				found[key] = &errInfo{name: e.name, ip: e.ip, doc: e.doc}
			}
			fe := found[key]
			if len(fe.by) == 0 || fe.by[len(fe.by)-1] != f.name {
				fe.by = append(fe.by, f.name)
			}
		}
	}
	if len(found) == 0 {
		return fmt.Sprintf("The doc comments of [`%s`](%s) and its methods name no error.", dt.Name, pkgLink(pi.path, dt.Name)), nil, nil
	}
	keys := make([]string, 0, len(found))
	for k := range found {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	b.WriteString("| Error | Returned by | Meaning |\n|---|---|---|\n")
	for _, k := range keys {
		e := found[k]
		var by []string
		for _, f := range e.by {
			by = append(by, "`"+f+"`")
		}
		fmt.Fprintf(&b, "| [`%s`](%s) | %s | %s |\n", k, pkgLink(e.ip, e.name), strings.Join(by, ", "), cell(firstSentence(e.doc)))
	}
	return b.String(), nil, nil
}

func docWords(d string) []string {
	var out []string
	for _, w := range strings.FieldsFunc(d, func(r rune) bool {
		return r != '.' && r != '_' && (r < '0' || r > '9') && (r < 'a' || r > 'z') && (r < 'A' || r > 'Z')
	}) {
		w = strings.Trim(w, ".")
		if w != "" {
			out = append(out, w)
		}
	}
	return out
}

func firstSentence(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if i := strings.Index(s, ". "); i >= 0 {
		return s[:i+1]
	}
	return s
}

func constString(t types.Type, exact string) string {
	if n, ok := t.(*types.Named); ok && n.Obj().Pkg() != nil && n.Obj().Pkg().Path() == "time" && n.Obj().Name() == "Duration" {
		var v int64
		if _, err := fmt.Sscan(exact, &v); err == nil {
			return durationString(v)
		}
	}
	return exact
}
