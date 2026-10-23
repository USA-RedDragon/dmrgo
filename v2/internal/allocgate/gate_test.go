//go:build !tinygo

package allocgate_test

import (
	"fmt"
	"go/ast"
	"go/types"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
	"unicode"
	"unicode/utf8"
)

const moduleRoot = "../.."

var skipDirs = []string{"cmd", "examples", "internal/allocgate", "internal/dvsisim", "internal/fecref", "internal/outputdigest", "internal/text/texttest", "internal/unpacked"}

func libraryPackages(t *testing.T) []string {
	t.Helper()
	var pkgs []string
	err := filepath.WalkDir(moduleRoot, func(path string, d os.DirEntry, err error) error {
		if err != nil || !d.IsDir() {
			return err
		}
		rel, _ := filepath.Rel(moduleRoot, path)
		rel = filepath.ToSlash(rel)
		if d.Name() == "testdata" || slices.ContainsFunc(skipDirs, func(s string) bool { return rel == s || strings.HasPrefix(rel, s+"/") }) {
			return filepath.SkipDir
		}
		if _, err := os.Stat(filepath.Join(path, "go.mod")); err == nil && rel != "." {
			return filepath.SkipDir
		}
		matches, _ := filepath.Glob(filepath.Join(path, "*.go"))
		for _, m := range matches {
			if !strings.HasSuffix(m, "_test.go") {
				pkgs = append(pkgs, "./"+rel)
				break
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return pkgs
}

type root struct {
	sym  string
	decl *declFunc
}

const textPkgPath = "github.com/USA-RedDragon/dmrgo/v2/internal/text"

type rule struct {
	name       string
	what, when string
	match      func(r root) (ok bool, broken string)
	used       int
}

type gate struct {
	t         *testing.T
	g         *callGraph
	p         *program
	isLib     func(string) bool
	reached   map[string]bool
	next      map[string]string
	noGrow    map[string]string
	declBy    map[string]*declFunc
	notes     []string
	unknown   map[string]bool
	callerFns map[string]bool
	growth    map[string]bool
	full      bool
	cuts      []*cut
	grow      map[string]*grower
	notGrower map[string]string
}

type cut struct {
	name       string
	funcs      map[string]bool
	allocates  bool
	where      string
	what, when string
	used       int
	without    map[string]string
}

func reachedOutsideWazero(pkgs []listedPackage, lib map[string]bool) map[string]bool {
	imports := map[string][]string{}
	for _, p := range pkgs {
		imports[p.ImportPath] = p.Imports
	}
	reached := map[string]bool{}
	var walk func(string)
	walk = func(path string) {
		if reached[path] || inWazero(path+".x") {
			return
		}
		reached[path] = true
		for _, i := range imports[path] {
			walk(i)
		}
	}
	for path := range lib {
		walk(path)
	}
	return reached
}

func newGate(t *testing.T, patterns []string, env ...string) *gate {
	t.Helper()
	pkgs := listPackages(t, patterns, env...)
	lib := map[string]bool{}
	full := false
	for _, p := range pkgs {
		if !p.DepOnly && !p.Standard {
			lib[p.ImportPath] = true
			full = full || len(p.CgoFiles) > 0
		}
	}
	full = full && os.Getenv("DMRGO_README") != ""
	isLib := func(path string) bool { return lib[path] }
	bodies := disassemble(t, pkgs)
	facts := readCompilerFacts(t)
	gt := &gate{t: t, g: newCallGraph(bodies, facts.noReturn), p: loadProgram(t, pkgs, isLib), isLib: isLib, reached: reachedOutsideWazero(pkgs, lib), full: full, declBy: map[string]*declFunc{}, unknown: map[string]bool{}, callerFns: map[string]bool{}}
	gt.resolveInterfaceCalls()
	gt.resolveFuncValueCalls()
	gt.growth = facts.growth
	gt.g.sinks[facts.newproc] = true
	caches, err := gt.runtimeCaches(facts.cached, facts.builtins)
	if err != nil {
		t.Fatalf("runtime cache rule: %v", err)
	}
	prints := map[string]bool{}
	for f := range facts.builtins {
		if strings.HasPrefix(f, "runtime.print") {
			prints[f] = true
		}
	}
	gt.cuts = []*cut{
		{
			name: "runtime cache", funcs: caches, allocates: true, where: "runtime cache",
			what: "A type assertion or type switch to an interface type, and a goroutine that blocks",
			when: "Once for each call site and dynamic type, as the runtime fills the cache of the assertion or switch, and when no sudog is free for a goroutine that blocks on a mutex or a channel. The runtime keeps both.",
		},
		{
			name: "caller method", funcs: gt.callerFns, allocates: true, where: "method of the caller",
			what: "A method of an interface that the library declares for the caller to implement, such as the Read and Write of a dvsi.Port",
			when: "When the method of the caller allocates. The code of the library on the path does not.",
		},
		{
			name: "debug print", funcs: prints,
			what: "The runtime print functions, which the runtime calls on its debug paths",
			when: "Never: they format into fixed buffers that the gate cannot prove are large enough.",
		},
	}
	for _, c := range gt.cuts {
		t.Logf("%s rule: cuts %s", c.name, strings.Join(slices.Sorted(maps.Keys(c.funcs)), ", "))
	}
	gt.paths()
	return gt
}

func (gt *gate) paths() {
	avoid := map[string]bool{}
	for _, c := range gt.cuts {
		maps.Copy(avoid, c.funcs)
	}
	gt.next = gt.g.pathsToSinks(avoid)
	gt.grow, gt.notGrower = nil, nil
	noGrow := maps.Clone(avoid)
	maps.Copy(noGrow, gt.growth)
	gt.noGrow = gt.g.pathsToSinks(noGrow)
	for _, c := range gt.cuts {
		rest := map[string]bool{}
		for _, o := range gt.cuts {
			if o != c {
				maps.Copy(rest, o.funcs)
			}
		}
		c.without = gt.g.pathsToSinks(rest)
	}
}

func (gt *gate) resolveInterfaceCalls() {
	byName := map[string][]string{}
	for name, bs := range gt.g.bodies {
		if pkg, m, ok := methodSym(name); ok && !gt.isLib(pkg) && gt.reached[pkg] && !ifaceWrapper(name, bs) {
			byName[m] = append(byName[m], name)
		}
	}
	cache := map[string][]string{}
	callerNode := map[string]string{}
	unresolved := map[string]bool{}
	for name, bs := range gt.g.bodies {
		if inWazero(name) {
			continue
		}
		for _, b := range bs {
			for arg := range b.ifaceMethods {
				targets, ok := cache[arg]
				if !ok && inWazero(strings.TrimPrefix(arg, "type:")) {
					ok = true
					cache[arg] = nil
				}
				if !ok {
					m, err := gt.p.resolveIfaceMethod(arg)
					switch {
					case err == nil:
						targets = gt.p.libraryImplementations(m)
						if !m.sealed {
							sym, _, _ := strings.Cut(strings.TrimPrefix(arg, "type:"), "+")
							if !strings.HasPrefix(sym, "interface {") && gt.isLib(symPackage(sym)) {
								via := "caller:" + sym + "." + m.method.Name()
								callerNode[arg] = via
								gt.callerFns[via] = true
								for _, to := range byName[m.method.Name()] {
									gt.g.addEdge(via, to)
								}
							} else {
								targets = append(targets, byName[m.method.Name()]...)
							}
						}
					case anonymousMethod(arg) != "":
						targets = byName[anonymousMethod(arg)]
					default:
						unresolved[arg] = true
					}
					cache[arg] = targets
				}
				if unresolved[arg] {
					gt.unknown[name] = true
				}
				if via, ok := callerNode[arg]; ok {
					gt.g.addEdge(name, via)
				}
				for _, to := range targets {
					gt.g.addEdge(name, to)
				}
			}
		}
	}
	if len(unresolved) > 0 {
		gt.notes = append(gt.notes, fmt.Sprintf("%d interface types not resolved, so calls of their methods reach no implementation: %s", len(unresolved), strings.Join(slices.Sorted(maps.Keys(unresolved)), ", ")))
	}
}

func ifaceWrapper(name string, bs []*body) bool {
	i := strings.LastIndexByte(name, '.')
	prefix := "type:" + name[:i] + "+"
	for _, b := range bs {
		for arg := range b.ifaceMethods {
			if strings.HasPrefix(arg, prefix) {
				return true
			}
		}
	}
	return false
}

var anonymousMethodRE = regexp.MustCompile(`(?:^|; )(\w+)\(`)

func anonymousMethod(arg string) string {
	sym, off, ok := strings.Cut(strings.TrimPrefix(arg, "type:"), "+")
	body, ok2 := strings.CutPrefix(sym, "interface { ")
	if !ok || !ok2 {
		return ""
	}
	var names []string
	for _, m := range anonymousMethodRE.FindAllStringSubmatch(strings.TrimSuffix(body, " }"), -1) {
		names = append(names, m[1])
	}
	slices.Sort(names)
	n, err := strconv.Atoi(off)
	if err != nil || n < 80 || (n-80)%8 != 0 || (n-80)/8 >= len(names) {
		return ""
	}
	return names[(n-80)/8]
}

func (gt *gate) resolveFuncValueCalls() {
	calls, values := gt.p.funcValues()
	closures := map[position][]string{}
	for name, bs := range gt.g.bodies {
		if !gt.isLib(symPackage(name)) {
			continue
		}
		for _, b := range bs {
			closures[b.insts[0].pos] = append(closures[b.insts[0].pos], name)
		}
	}
	for i, v := range values {
		if v.lit {
			for _, s := range closures[v.pos] {
				if strings.Contains(s, ".func") {
					values[i].syms = append(values[i].syms, s)
				}
			}
		}
	}
	for name, sites := range gt.g.indirect {
		for _, pos := range sites {
			for _, c := range calls[pos] {
				for _, v := range values {
					if v.sig != nil && types.Identical(v.sig, c.sig) {
						for _, s := range v.syms {
							gt.g.addEdge(name, s)
						}
					}
				}
			}
		}
	}
}

func (gt *gate) roots() []root {
	byPkg := map[string][]string{}
	for name := range gt.g.bodies {
		byPkg[symPackage(name)] = append(byPkg[symPackage(name)], name)
	}
	var roots []root
	for _, d := range gt.p.declaredFuncs() {
		gt.declBy[d.fn.FullName()] = &d
		if !d.fn.Exported() {
			continue
		}
		if d.sym != "" {
			if _, ok := gt.g.bodies[d.sym]; !ok {
				gt.t.Errorf("%s: exported, but the compiled package has no code for %s", d.fn.FullName(), d.sym)
				continue
			}
			roots = append(roots, root{d.sym, &d})
			continue
		}
		re := genericSymRE(d.fn)
		found := false
		for _, s := range byPkg[d.fn.Pkg().Path()] {
			if re.MatchString(s) {
				roots = append(roots, root{s, &d})
				found = true
			}
		}
		if !found {
			gt.notes = append(gt.notes, fmt.Sprintf("%s: generic, and the library instantiates it nowhere", d.fn.FullName()))
		}
	}
	for _, l := range gt.p.order {
		if _, ok := gt.g.bodies[l.path+".init"]; ok {
			roots = append(roots, root{sym: l.path + ".init"})
		}
	}
	slices.SortFunc(roots, func(a, b root) int { return strings.Compare(a.sym, b.sym) })
	return roots
}

func genericSymRE(fn *types.Func) *regexp.Regexp {
	pkg := regexp.QuoteMeta(fn.Pkg().Path())
	sig := fn.Signature()
	if sig.Recv() == nil {
		return regexp.MustCompile(`^` + pkg + `\.` + fn.Name() + `\[.*\]$`)
	}
	rt := sig.Recv().Type()
	if pt, ok := rt.(*types.Pointer); ok {
		rt = pt.Elem()
	}
	named, ok := rt.(*types.Named)
	if !ok {
		return regexp.MustCompile(`^$`)
	}
	tn := named.Obj().Name()
	return regexp.MustCompile(`^` + pkg + `\.(\(\*)?` + tn + `\[.*\]\)?\.` + fn.Name() + `$`)
}

func (gt *gate) allocates(sym string) bool {
	_, ok := gt.next[sym]
	return ok
}

func (gt *gate) growsOnly(sym string) bool {
	_, ok := gt.noGrow[sym]
	return !ok
}

func (gt *gate) chain(next map[string]string, sym string) string {
	return "\n\t\t" + strings.Join(chain(next, sym), "\n\t\t")
}

func hasPrefixWord(name, prefix string) bool {
	rest, ok := strings.CutPrefix(name, prefix)
	if !ok {
		return false
	}
	r, _ := utf8.DecodeRuneInString(rest)
	return rest == "" || unicode.IsUpper(r)
}

func (gt *gate) twinOf(d *declFunc) (*declFunc, string) {
	name := d.fn.Name()
	if tw := gt.sibling(d, "Append"+name); tw != nil {
		ts := tw.fn.Signature()
		ok := ts.Params().Len() > 0 && ts.Results().Len() > 0
		if ok {
			_, isSlice := ts.Params().At(0).Type().Underlying().(*types.Slice)
			ok = isSlice && types.Identical(ts.Params().At(0).Type(), ts.Results().At(0).Type())
		}
		if !ok {
			return tw, fmt.Sprintf("%s must take a slice dst first and return the extended slice first", tw.fn.Name())
		}
		return tw, ""
	}
	if tw := gt.sibling(d, name+"Into"); tw != nil {
		ts := tw.fn.Signature()
		ok := ts.Params().Len() > 0
		if ok {
			_, ok = ts.Params().At(0).Type().(*types.Pointer)
		}
		if !ok {
			return tw, fmt.Sprintf("%s must take a pointer dst first", tw.fn.Name())
		}
		return tw, ""
	}
	return nil, ""
}

func (gt *gate) sibling(d *declFunc, name string) *declFunc {
	full := d.fn.FullName()
	i := strings.LastIndexByte(full, '.')
	return gt.declBy[full[:i+1]+name]
}

var twinDocRE = regexp.MustCompile(`allocates[^.;]*; ((?:\w+\.)?\w+) does not`)

func docText(fd *ast.FuncDecl) string {
	if fd.Doc == nil {
		return ""
	}
	return strings.Join(strings.Fields(fd.Doc.Text()), " ")
}

func (gt *gate) rules() []*rule {
	isMethod := func(r root) bool { return r.decl != nil && r.decl.fn.Signature().Recv() != nil }
	return []*rule{
		{name: "package initialization", what: "The initialization of a package-level variable, such as a lookup table or a joined sentinel error", when: "Once, when the program starts.", match: func(r root) (bool, string) {
			return r.decl == nil, ""
		}},
		{name: "fmt and error text", what: "A String, GoString or Error method", when: "When the text is built, for a person to read.", match: func(r root) (bool, string) {
			if !isMethod(r) {
				return false, ""
			}
			switch r.decl.fn.Name() {
			case "String", "GoString", "Error":
			default:
				return false, ""
			}
			sig := r.decl.fn.Signature()
			if sig.Params().Len() != 0 || sig.Results().Len() != 1 || !types.Identical(sig.Results().At(0).Type(), types.Typ[types.String]) {
				return false, "a String, GoString or Error method must have the signature func() string"
			}
			return true, ""
		}},
		{name: "text", what: "A function of internal/text, which formats the values of String and Error methods", when: "When the text is built, for a person to read.", match: func(r root) (bool, string) {
			return r.decl != nil && r.decl.fn.Pkg() != nil && r.decl.fn.Pkg().Path() == textPkgPath, ""
		}},
		{name: "constructor", what: "An exported package-level New function", when: "Once, at construction: it allocates what it returns, sized up front.", match: func(r root) (bool, string) {
			if r.decl == nil || !hasPrefixWord(r.decl.fn.Name(), "New") {
				return false, ""
			}
			if isMethod(r) {
				return false, "a constructor is a package-level function; a New method is not one"
			}
			return true, ""
		}},
		{name: "dst growth", what: "An exported function or method that takes a slice dst and returns a slice of the same type", when: "Only to grow dst when dst has no room. The caller sizes dst. A machine built from other machines may also pass each of them an unexported field sliced to [:0] that its New makes with capacity MaxX of that machine's package, whose fuzz test checks that bound.", match: gt.growsDst},
		{name: "allocating wrapper", what: "An exported X with an exported twin AppendX or XInto that does not allocate", when: "X allocates the slice, string or struct it returns; the twin writes into storage of the caller.", match: func(r root) (bool, string) {
			if r.decl == nil {
				return false, ""
			}
			tw, shape := gt.twinOf(r.decl)
			if tw == nil {
				return false, ""
			}
			if shape != "" {
				return false, "its twin has the wrong shape: " + shape
			}
			if hasPrefixWord(tw.fn.Name(), "Append") {
				if ok, why := gt.growsDst(root{tw.sym, tw}); gt.allocates(tw.sym) && !ok {
					return false, fmt.Sprintf("its twin %s does not allocate only to grow dst: %s", tw.fn.Name(), why)
				}
			} else if gt.allocates(tw.sym) {
				return false, fmt.Sprintf("its twin %s allocates:%s", tw.fn.Name(), gt.chain(gt.next, tw.sym))
			}
			return true, ""
		}},
		{name: "nil-dst wrapper", what: "An exported X whose whole body is return Twin(nil, its parameters), where Twin allocates only to grow dst", when: "X allocates the slice it returns; Twin with room in dst does not.", match: func(r root) (bool, string) {
			if r.decl == nil {
				return false, ""
			}
			tw, broken := gt.nilDstTwin(r.decl)
			if tw == nil {
				return false, broken
			}
			if ok, why := gt.growsDst(root{tw.sym, tw}); gt.allocates(tw.sym) && !ok {
				return false, fmt.Sprintf("its twin %s does not allocate only to grow dst: %s", tw.fn.Name(), why)
			}
			return true, ""
		}},
	}
}

func (gt *gate) nilDstTwin(d *declFunc) (*declFunc, string) {
	body := d.decl.Body
	if body == nil || len(body.List) != 1 {
		return nil, ""
	}
	ret, ok := body.List[0].(*ast.ReturnStmt)
	if !ok || len(ret.Results) != 1 {
		return nil, ""
	}
	call, ok := ret.Results[0].(*ast.CallExpr)
	if !ok || len(call.Args) == 0 {
		return nil, ""
	}
	if id, ok := call.Args[0].(*ast.Ident); !ok || id.Name != "nil" {
		return nil, ""
	}
	var name string
	switch fun := call.Fun.(type) {
	case *ast.Ident:
		if d.decl.Recv != nil {
			return nil, ""
		}
		name = fun.Name
	case *ast.SelectorExpr:
		recv, ok := fun.X.(*ast.Ident)
		if !ok || d.decl.Recv == nil || len(d.decl.Recv.List[0].Names) != 1 || d.decl.Recv.List[0].Names[0].Name != recv.Name {
			return nil, ""
		}
		name = fun.Sel.Name
	default:
		return nil, ""
	}
	tw := gt.sibling(d, name)
	if tw == nil || !tw.fn.Exported() {
		return nil, fmt.Sprintf("it returns %s(nil, ...), which is not an exported function of its package or method of its receiver", name)
	}
	var params []string
	for _, f := range d.decl.Type.Params.List {
		for _, n := range f.Names {
			params = append(params, n.Name)
		}
	}
	args := call.Args[1:]
	ok = len(args) == len(params)
	for i := 0; ok && i < len(args); i++ {
		id, isIdent := args[i].(*ast.Ident)
		ok = isIdent && id.Name == params[i]
	}
	if ok && d.fn.Signature().Variadic() != call.Ellipsis.IsValid() {
		ok = false
	}
	if !ok {
		return nil, fmt.Sprintf("it returns %s(nil, ...), but not with its own parameters (%s) in order", name, strings.Join(params, ", "))
	}
	return tw, ""
}

func TestHeapAllocations(t *testing.T) {
	t.Parallel()

	gt := newGate(t, libraryPackages(t))
	checkReadme(t, gt.check(t), gt.cuts)
}

func saysAllocates(r root) bool {
	return r.decl == nil || strings.Contains(strings.ToLower(docText(r.decl.decl)), "allocat")
}

func (gt *gate) check(t *testing.T) []*rule {
	t.Helper()
	rules := gt.rules()
	roots := gt.roots()
	bad, allocating := gt.checkRoots(rules, roots)
	bad = append(bad, gt.checkCutDocs(roots)...)
	bad = append(bad, gt.checkTwinDocs()...)
	bad = append(bad, gt.checkDeadRules(t, rules, roots)...)
	bad = append(bad, gt.checkUnknown(roots)...)
	slices.Sort(bad)
	for _, b := range bad {
		t.Error(b)
	}
	t.Logf("%d roots, %d allocate, %d fail; %d functions in the call graph; %d cannot return", len(roots), allocating, len(bad), len(gt.g.bodies), len(gt.g.noReturn))
	for _, n := range gt.notes {
		t.Log(n)
	}
	return rules
}

func (gt *gate) checkRoots(rules []*rule, roots []root) (bad []string, allocating int) {
	for _, r := range roots {
		if !gt.allocates(r.sym) {
			continue
		}
		allocating++
		var why []string
		ok := false
		for _, ru := range rules {
			m, broken := ru.match(r)
			if m {
				ru.used++
				ok = true
				break
			}
			if broken != "" {
				why = append(why, ru.name+" rule: "+broken)
			}
		}
		if ok {
			if !saysAllocates(r) {
				bad = append(bad, fmt.Sprintf("%s: a rule allows it to allocate, but its doc has no sentence that says when it allocates", r.sym))
			}
			continue
		}
		msg := fmt.Sprintf("%s allocates:%s", r.sym, gt.chain(gt.next, r.sym))
		for _, w := range why {
			msg += "\n\t" + w
		}
		bad = append(bad, msg)
	}
	return bad, allocating
}

func (gt *gate) checkCutDocs(roots []root) []string {
	var bad []string
	for _, r := range roots {
		if gt.allocates(r.sym) {
			continue
		}
		for _, c := range gt.cuts {
			if _, ok := c.without[r.sym]; ok && c.allocates && !saysAllocates(r) {
				bad = append(bad, fmt.Sprintf("%s: it can allocate in a %s that the %s rule allows, but its doc has no sentence that says when it allocates:%s", r.sym, c.where, c.name, gt.chain(c.without, r.sym)))
			}
		}
	}
	return bad
}

func (gt *gate) checkTwinDocs() []string {
	var bad []string
	for _, d := range gt.p.declaredFuncs() {
		m := twinDocRE.FindStringSubmatch(docText(d.decl))
		if m == nil {
			continue
		}
		name := d.fn.FullName()
		switch {
		case !d.fn.Exported():
			bad = append(bad, fmt.Sprintf("%s: unexported, but its doc claims the allocating wrapper exemption (twin %s)", name, m[1]))
		case !gt.allocates(d.sym):
			bad = append(bad, fmt.Sprintf("%s: its doc says that it allocates, but its machine code reaches no allocator", name))
		default:
			tw, _ := gt.twinOf(&d)
			if tw == nil {
				tw, _ = gt.nilDstTwin(&d)
			}
			if tw == nil || m[1] != tw.fn.Name() {
				want := "none"
				if tw != nil {
					want = tw.fn.Name()
				}
				bad = append(bad, fmt.Sprintf("%s: its doc names the twin %s, but its twin by the naming or nil-dst rule is %s", name, m[1], want))
			}
		}
	}
	return bad
}

func (gt *gate) checkDeadRules(t *testing.T, rules []*rule, roots []root) []string {
	t.Helper()
	var bad []string
	for _, c := range gt.cuts {
		n := 0
		for _, r := range roots {
			if _, ok := c.without[r.sym]; ok && !gt.allocates(r.sym) {
				n++
			}
		}
		c.used = n
		t.Logf("%s rule: %d functions", c.name, n)
		if n == 0 && gt.full {
			bad = append(bad, fmt.Sprintf("the %s rule exempts no allocating function; remove it", c.name))
		}
	}
	for _, ru := range rules {
		if ru.used == 0 && gt.full {
			bad = append(bad, fmt.Sprintf("the %s rule exempts no allocating function; remove it", ru.name))
		}
		t.Logf("%s rule: %d functions", ru.name, ru.used)
	}
	if !gt.full {
		t.Log("not the configuration of the README table, so a rule that covers nothing is not reported")
	}
	return bad
}

func (gt *gate) checkUnknown(roots []root) []string {
	seen := map[string]bool{}
	var q []string
	for _, r := range roots {
		if !seen[r.sym] {
			seen[r.sym] = true
			q = append(q, r.sym)
		}
	}
	for len(q) > 0 {
		n := q[len(q)-1]
		q = q[:len(q)-1]
		for to := range gt.g.edges[n] {
			if !seen[to] {
				seen[to] = true
				q = append(q, to)
			}
		}
	}
	var bad []string
	for name := range gt.unknown {
		if seen[name] {
			bad = append(bad, fmt.Sprintf("%s: a root reaches it, and it calls a method of an interface type that the gate cannot resolve", name))
		}
	}
	return bad
}
