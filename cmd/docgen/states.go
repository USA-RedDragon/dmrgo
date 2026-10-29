package main

import (
	"fmt"
	"go/ast"
	"go/constant"
	"go/types"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// A stateRun holds what the instrumented tests of one package recorded.
type stateRun struct {
	machines map[string]*machine
	err      error
}

// A machine is one view of the state of a machine type: the state type
// that one of its methods returns, or the results of its observers.
type machine struct {
	name      string
	typ       string
	stateType *types.Named
	accessor  string
	args      []string
	argType   types.Type
	observers []observer
	methods   []string
	skipped   []string
	initial   map[string]bool
	edges     map[[2]string]map[string]bool
}

type observer struct {
	name string
	kind string
}

const hookFile = "zz_docgen_states.go"

const (
	obsBool = "bool"
	obsOK   = "ok"
	obsLen  = "len"
)

func (g *gen) runStates() error {
	g.states = map[string]*stateRun{}
	pkgs := make([]string, 0, len(g.machines))
	for ip := range g.machines {
		pkgs = append(pkgs, ip)
	}
	sort.Strings(pkgs)
	for _, ip := range pkgs {
		if g.loadErr[ip] != nil {
			continue
		}
		run, err := g.recordStates(ip, g.machines[ip])
		if err != nil {
			run = &stateRun{err: fmt.Errorf("states of %s: %w", shortPath(ip), err)}
		}
		g.states[ip] = run
	}
	return nil
}

func integerNamed(t types.Type) *types.Named {
	n, ok := t.(*types.Named)
	if !ok {
		return nil
	}
	if b, ok := n.Underlying().(*types.Basic); !ok || b.Info()&types.IsInteger == 0 {
		return nil
	}
	return n
}

func stateMethod(tn *types.TypeName) *types.Named {
	ms := types.NewMethodSet(types.NewPointer(tn.Type()))
	sel := ms.Lookup(tn.Pkg(), "State")
	if sel == nil {
		return nil
	}
	sig, ok := sel.Type().(*types.Signature)
	if !ok || sig.Params().Len() != 0 || sig.Results().Len() != 1 {
		return nil
	}
	return integerNamed(sig.Results().At(0).Type())
}

func observersOf(tn *types.TypeName) []observer {
	var out []observer
	ms := types.NewMethodSet(types.NewPointer(tn.Type()))
	for i := 0; i < ms.Len(); i++ {
		f, ok := ms.At(i).Obj().(*types.Func)
		if !ok || !f.Exported() {
			continue
		}
		sig, ok := f.Type().(*types.Signature)
		if !ok || sig.Params().Len() != 0 {
			continue
		}
		r := sig.Results()
		isBasic := func(t types.Type, info types.BasicInfo) bool {
			b, ok := t.Underlying().(*types.Basic)
			return ok && b.Info()&info != 0
		}
		switch {
		case r.Len() == 1 && isBasic(r.At(0).Type(), types.IsBoolean):
			out = append(out, observer{f.Name(), obsBool})
		case r.Len() == 2 && isBasic(r.At(1).Type(), types.IsBoolean):
			out = append(out, observer{f.Name(), obsOK})
		case r.Len() == 1 && isBasic(r.At(0).Type(), types.IsInteger) && f.Name() == "Len":
			out = append(out, observer{f.Name(), obsLen})
		}
	}
	return out
}

type imports struct {
	here  *types.Package
	alias map[string]string
}

func (im *imports) qualify(c *types.Const) string {
	if c.Pkg() == im.here {
		return c.Name()
	}
	a, ok := im.alias[c.Pkg().Path()]
	if !ok {
		a = fmt.Sprintf("__dgp%d", len(im.alias))
		im.alias[c.Pkg().Path()] = a
	}
	return a + "." + c.Name()
}

// resolveView maps the type= of a marker to the machine type, the method
// that returns its state and the arguments of that method.
func resolveView(pkg *types.Package, name string, im *imports) (*machine, error) {
	tn, ok := pkg.Scope().Lookup(name).(*types.TypeName)
	if !ok {
		return nil, fmt.Errorf("%s is not a type", name)
	}
	m := &machine{name: name, typ: name, initial: map[string]bool{}, edges: map[[2]string]map[string]bool{}}
	if st := stateMethod(tn); st != nil {
		m.stateType, m.accessor = st, "State"
		return m, nil
	}
	if st := integerNamed(tn.Type()); st != nil {
		var found []string
		for _, n := range pkg.Scope().Names() {
			t, ok := pkg.Scope().Lookup(n).(*types.TypeName)
			if !ok || t == tn || !t.Exported() {
				continue
			}
			ms := types.NewMethodSet(types.NewPointer(t.Type()))
			for i := 0; i < ms.Len(); i++ {
				f, ok := ms.At(i).Obj().(*types.Func)
				if !ok {
					continue
				}
				sig, ok := f.Type().(*types.Signature)
				if !ok || !f.Exported() || sig.Results().Len() != 1 || !types.Identical(sig.Results().At(0).Type(), st) || sig.Params().Len() > 1 {
					continue
				}
				var args []string
				var argType types.Type
				if sig.Params().Len() == 1 {
					argType = sig.Params().At(0).Type()
					at, ok := sig.Params().At(0).Type().(*types.Named)
					if !ok || at.Obj().Pkg() == nil {
						continue
					}
					sc := at.Obj().Pkg().Scope()
					for _, cn := range sc.Names() {
						if c, ok := sc.Lookup(cn).(*types.Const); ok && c.Exported() && types.Identical(c.Type(), at) {
							args = append(args, im.qualify(c))
						}
					}
					if len(args) == 0 {
						continue
					}
				}
				found = append(found, t.Name()+"."+f.Name())
				m.typ, m.accessor, m.args, m.argType, m.stateType = t.Name(), f.Name(), args, argType, st
			}
		}
		switch len(found) {
		case 0:
			return nil, fmt.Errorf("no method of the package returns %s", name)
		case 1:
			return m, nil
		}
		return nil, fmt.Errorf("several methods return %s (%s); give the machine type", name, strings.Join(found, ", "))
	}
	m.observers = observersOf(tn)
	if len(m.observers) == 0 {
		return nil, fmt.Errorf("%s has no State method and no method without arguments that returns a bool or a count", name)
	}
	return m, nil
}

func (m *machine) stateExprs() []string {
	if m.stateType != nil {
		if len(m.args) == 0 {
			return []string{fmt.Sprintf("strconv.FormatUint(uint64(r.%s()), 10)", m.accessor)}
		}
		var out []string
		for _, a := range m.args {
			out = append(out, fmt.Sprintf("strconv.FormatUint(uint64(r.%s(%s)), 10)", m.accessor, a))
		}
		return out
	}
	var parts []string
	for _, o := range m.observers {
		switch o.kind {
		case obsBool:
			parts = append(parts, fmt.Sprintf("strconv.FormatBool(r.%s())", o.name))
		case obsOK:
			parts = append(parts, fmt.Sprintf("func() string { _, ok := r.%s(); return strconv.FormatBool(ok) }()", o.name))
		case obsLen:
			parts = append(parts, fmt.Sprintf("strconv.FormatBool(r.%s() > 0)", o.name))
		}
	}
	return []string{strings.Join(parts, ` + "," + `)}
}

type edit struct {
	off  int
	text string
}

const hookCommon = `var __dgMu sync.Mutex

var __dgSeen = map[string]bool{}

func __dgEmit(fields ...string) {
	line := ""
	for i, f := range fields {
		if i > 0 {
			line += "\t"
		}
		line += f
	}
	if __dgSeen[line] {
		return
	}
	__dgSeen[line] = true
	f, err := os.OpenFile(os.Getenv("DOCGEN_STATES_OUT"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	_, _ = f.WriteString(line + "\n")
	_ = f.Close()
}

var _ = strconv.Itoa
`

const hookType = `
var __dgDepthT_ = map[*T_]int{}

var __dgFromT_ = map[*T_][]string{}

var __dgKnownT_ = map[*T_]bool{}

var __dgViewsT_ = []string{VIEWS}

func __dgStateT_(r *T_) []string { return []string{COMPS} }

func __dgInT_(r *T_) bool {
	if r == nil {
		return false
	}
	__dgMu.Lock()
	defer __dgMu.Unlock()
	if __dgDepthT_[r] == 0 {
		s := __dgStateT_(r)
		__dgFromT_[r] = s
		if !__dgKnownT_[r] {
			__dgKnownT_[r] = true
			for i, v := range s {
				__dgEmit("init", __dgViewsT_[i], v)
			}
		}
	}
	__dgDepthT_[r]++
	return true
}

func __dgOutT_(r *T_, m string) {
	__dgMu.Lock()
	defer __dgMu.Unlock()
	__dgDepthT_[r]--
	if __dgDepthT_[r] > 0 {
		return
	}
	delete(__dgDepthT_, r)
	from := __dgFromT_[r]
	for i, v := range __dgStateT_(r) {
		if v != from[i] {
			__dgEmit("edge", __dgViewsT_[i], from[i], m, v)
		}
	}
}
`

type stateDef struct {
	key  string
	name string
	doc  string
}

func (g *gen) stateDefs(pi *pkgInfo, m *machine) []stateDef {
	if m.stateType == nil {
		seen := map[string]bool{}
		for k := range m.initial {
			seen[k] = true
		}
		for k := range m.edges {
			seen[k[0]], seen[k[1]] = true, true
		}
		out := make([]stateDef, 0, len(seen))
		for k := range seen {
			out = append(out, stateDef{key: k, name: observerName(m, k)})
		}
		sort.Slice(out, func(i, j int) bool { return out[i].key < out[j].key })
		return out
	}
	docs := map[string]string{}
	for _, t := range pi.doc.Types {
		if t.Name != m.stateType.Obj().Name() {
			continue
		}
		for _, c := range t.Consts {
			for _, s := range c.Decl.Specs {
				vs, ok := s.(*ast.ValueSpec)
				if !ok {
					continue
				}
				for _, n := range vs.Names {
					d := vs.Doc.Text()
					if d == "" {
						d = vs.Comment.Text()
					}
					docs[n.Name] = d
				}
			}
		}
	}
	type cv struct {
		name string
		v    uint64
	}
	var cs []cv
	scope := m.stateType.Obj().Pkg().Scope()
	for _, n := range scope.Names() {
		c, ok := scope.Lookup(n).(*types.Const)
		if !ok || !c.Exported() || !types.Identical(c.Type(), m.stateType) {
			continue
		}
		v, ok := constant.Uint64Val(c.Val())
		if !ok {
			continue
		}
		cs = append(cs, cv{n, v})
	}
	sort.Slice(cs, func(i, j int) bool { return cs[i].v < cs[j].v || cs[i].v == cs[j].v && cs[i].name < cs[j].name })
	var out []stateDef
	for _, c := range cs {
		out = append(out, stateDef{key: strconv.FormatUint(c.v, 10), name: c.name, doc: docs[c.name]})
	}
	return out
}

func observerName(m *machine, key string) string {
	vals := strings.Split(key, ",")
	var parts []string
	for i, o := range m.observers {
		v := i < len(vals) && vals[i] == "true"
		switch o.kind {
		case obsBool:
			parts = append(parts, fmt.Sprintf("%s() %s", o.name, map[bool]string{true: "true", false: "false"}[v]))
		case obsOK:
			parts = append(parts, fmt.Sprintf("%s() %s", o.name, map[bool]string{true: "ok", false: "not ok"}[v]))
		case obsLen:
			parts = append(parts, fmt.Sprintf("%s() %s", o.name, map[bool]string{true: "> 0", false: "0"}[v]))
		}
	}
	return strings.Join(parts, ", ")
}

var slugRe = regexp.MustCompile(`[^a-z0-9]+`)

func (g *gen) stateView(mk marker) (string, *machine, error) {
	ip, err := g.pkgPath(mk.args[argPkg])
	if err != nil {
		return "", nil, err
	}
	if err := g.loadErr[ip]; err != nil {
		return "", nil, err
	}
	run := g.states[ip]
	if run == nil {
		return "", nil, fmt.Errorf("no state run for %s", ip)
	}
	if run.err != nil {
		return "", nil, run.err
	}
	m := run.machines[mk.args["type"]]
	if m == nil {
		return "", nil, fmt.Errorf("no state view %s", mk.args["type"])
	}
	return ip, m, nil
}

func (g *gen) fillStates(pg *page, mk marker) (string, []outFile, error) {
	ip, m, err := g.stateView(mk)
	if err != nil {
		return "", nil, err
	}
	pi := g.loaded[ip]
	defs := g.stateDefs(pi, m)
	byKey := map[string]stateDef{}
	for _, d := range defs {
		byKey[d.key] = d
	}
	name := func(k string) string {
		if d, ok := byKey[k]; ok {
			return d.name
		}
		return "value " + k
	}
	reached := map[string]bool{}
	for k := range m.initial {
		reached[k] = true
	}
	for k := range m.edges {
		reached[k[0]], reached[k[1]] = true, true
	}
	if len(m.edges) == 0 {
		return "", nil, fmt.Errorf("the tests of %s recorded no transition of %s", shortPath(ip), m.name)
	}
	var b strings.Builder
	if m.stateType != nil {
		acc := m.typ + "." + m.accessor
		each := ""
		if len(m.args) > 0 {
			each = " for each `" + typeString(m.argType, pi.pkg.Types) + "`"
		}
		fmt.Fprintf(&b, "States of [`%s`](%s), from [`%s`](%s)%s:\n\n", m.typ, pkgLink(ip, m.typ), acc, pkgLink(ip, acc), each)
		b.WriteString("| State | Meaning | Seen in tests |\n|---|---|---|\n")
		for _, d := range defs {
			seen := "no"
			if reached[d.key] {
				seen = "yes"
			}
			if m.initial[d.key] {
				seen += ", first state"
			}
			fmt.Fprintf(&b, "| [`%s`](%s) | %s | %s |\n", d.name, pkgLink(ip, d.name), cell(d.doc), seen)
		}
	} else {
		var obs []string
		for _, o := range m.observers {
			obs = append(obs, "`"+o.name+"()`")
		}
		fmt.Fprintf(&b, "[`%s`](%s) has no State method. The state in this table is the result of %s. States seen in tests:\n\n", m.name, pkgLink(ip, m.name), strings.Join(obs, ", "))
		b.WriteString("| State | Seen in tests |\n|---|---|\n")
		for _, d := range defs {
			seen := "yes"
			if m.initial[d.key] {
				seen += ", first state"
			}
			fmt.Fprintf(&b, "| %s | %s |\n", cell(d.name), seen)
		}
	}
	type tr struct {
		from, to string
		calls    []string
	}
	var trs []tr
	for k, set := range m.edges {
		t := tr{from: k[0], to: k[1]}
		for c := range set {
			t.calls = append(t.calls, c)
		}
		sort.Strings(t.calls)
		trs = append(trs, t)
	}
	order := map[string]int{}
	for i, d := range defs {
		order[d.key] = i
	}
	sort.Slice(trs, func(i, j int) bool {
		if order[trs[i].from] != order[trs[j].from] {
			return order[trs[i].from] < order[trs[j].from]
		}
		return order[trs[i].to] < order[trs[j].to]
	})
	b.WriteString("\nTransitions seen in tests:\n\n| From | Method call | To |\n|---|---|---|\n")
	for _, t := range trs {
		var cs []string
		for _, c := range t.calls {
			cs = append(cs, "`"+c+"`")
		}
		fmt.Fprintf(&b, "| %s | %s | %s |\n", cell(name(t.from)), strings.Join(cs, ", "), cell(name(t.to)))
	}
	slug := slugRe.ReplaceAllString(strings.ToLower(shortPath(ip)+"-"+m.typ), "-")
	if m.name != m.typ {
		slug += "-" + strings.ToLower(m.name)
	}
	svgPath := filepath.Join(filepath.Dir(pg.path), "states", slug+".svg")
	var nodes, keys []string
	idx := map[string]int{}
	addNode := func(k string) {
		if _, ok := idx[k]; !ok {
			idx[k] = len(keys)
			keys = append(keys, k)
			nodes = append(nodes, name(k))
		}
	}
	for _, d := range defs {
		addNode(d.key)
	}
	for _, t := range trs {
		addNode(t.from)
		addNode(t.to)
	}
	var arcs []arc
	for _, t := range trs {
		arcs = append(arcs, arc{from: idx[t.from], to: idx[t.to], label: strings.Join(t.calls, ", ")})
	}
	svg := stateSVG(nodes, arcs, m.initial, keys)
	fmt.Fprintf(&b, "\n![Transitions of %s](states/%s.svg)\n", m.typ, slug)
	if len(m.skipped) > 0 {
		fmt.Fprintf(&b, "\nNot recorded (no receiver name): %s.\n", strings.Join(m.skipped, ", "))
	}
	return b.String(), []outFile{{path: svgPath, data: svg}}, nil
}
