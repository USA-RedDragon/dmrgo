//go:build !tinygo

package allocgate_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
)

var allocPrimitives = []string{"runtime.(*mcache).nextFree", "runtime.(*mcache).allocLarge"}

type compilerFacts struct {
	noReturn []string
	growth   map[string]bool
	cached   []string
	newproc  string
	builtins map[string]bool
}

var panicRE = regexp.MustCompile(`LookupRuntimeFunc\("(\w*[pP]anic\w*)"\)`)

var newprocRE = regexp.MustCompile(`ir\.Syms\.Newproc = typecheck\.LookupRuntimeFunc\("(\w+)"\)`)

var cachedRE = regexp.MustCompile(`ir\.Syms\.(?:TypeAssert|InterfaceSwitch) = typecheck\.LookupRuntimeFunc\("(\w+)"\)`)

var growthRE = regexp.MustCompile(`ir\.Syms\.(?:Growslice|MoveSlice)\w* = typecheck\.LookupRuntimeFunc\("(\w+)"\)`)

func readCompilerFacts(t *testing.T) compilerFacts {
	t.Helper()
	out, err := exec.CommandContext(t.Context(), "go", "env", "GOROOT").Output()
	if err != nil {
		t.Fatalf("go env GOROOT: %v", err)
	}
	file := filepath.Join(strings.TrimSpace(string(out)), "src", "cmd", "compile", "internal", "ssagen", "ssa.go")
	src, err := os.ReadFile(file)
	if err != nil {
		t.Fatalf("read the compiler: %v", err)
	}
	facts := compilerFacts{growth: map[string]bool{}}
	for _, m := range growthRE.FindAllStringSubmatch(string(src), -1) {
		facts.growth["runtime."+m[1]] = true
	}
	if !facts.growth["runtime.growslice"] {
		t.Fatalf("%s: found no lookup of runtime.growslice", file)
	}
	for _, m := range cachedRE.FindAllStringSubmatch(string(src), -1) {
		facts.cached = append(facts.cached, "runtime."+m[1])
	}
	if len(facts.cached) != 2 {
		t.Fatalf("%s: found %q as the runtime functions of the cached type assertion and type switch, want two", file, facts.cached)
	}
	if m := newprocRE.FindStringSubmatch(string(src)); m != nil {
		facts.newproc = "runtime." + m[1]
	} else {
		t.Fatalf("%s: found no lookup of the runtime function of a go statement", file)
	}
	facts.builtins = readBuiltins(t, filepath.Join(filepath.Dir(filepath.Dir(file)), "typecheck", "_builtin", "runtime.go"))
	f, err := parser.ParseFile(token.NewFileSet(), file, src, parser.SkipObjectResolution)
	if err != nil {
		t.Fatalf("read the compiler: %v", err)
	}
	names := blockExitNames(f)
	if !slices.Contains(names, "runtime.gopanic") {
		t.Fatalf("%s: found no list of calls that end a block, or it lacks gopanic: %q", file, names)
	}
	for _, m := range panicRE.FindAllStringSubmatch(string(src), -1) {
		names = append(names, "runtime."+m[1])
	}
	facts.noReturn = names
	return facts
}

func readBuiltins(t *testing.T, file string) map[string]bool {
	t.Helper()
	bf, err := parser.ParseFile(token.NewFileSet(), file, nil, parser.SkipObjectResolution)
	if err != nil {
		t.Fatalf("read the compiler: %v", err)
	}
	builtins := map[string]bool{}
	for _, d := range bf.Decls {
		if fd, ok := d.(*ast.FuncDecl); ok {
			builtins["runtime."+fd.Name.Name] = true
		}
	}
	if !builtins["runtime.mallocgc"] || !builtins["runtime.newobject"] {
		t.Fatalf("%s: found no runtime.mallocgc or runtime.newobject", file)
	}
	return builtins
}

func blockExitNames(f *ast.File) []string {
	var names []string
	ast.Inspect(f, func(n ast.Node) bool {
		is, ok := n.(*ast.IfStmt)
		if !ok || is.Init == nil {
			return true
		}
		as, ok := is.Init.(*ast.AssignStmt)
		if !ok || len(as.Lhs) != 1 {
			return true
		}
		if id, ok := as.Lhs[0].(*ast.Ident); !ok || id.Name != "fn" {
			return true
		}
		exit := false
		ast.Inspect(is.Body, func(n ast.Node) bool {
			if se, ok := n.(*ast.SelectorExpr); ok && se.Sel.Name == "BlockExit" {
				exit = true
			}
			return true
		})
		if !exit {
			return true
		}
		ast.Inspect(is.Cond, func(n ast.Node) bool {
			be, ok := n.(*ast.BinaryExpr)
			if !ok || be.Op != token.EQL {
				return true
			}
			id, ok := be.X.(*ast.Ident)
			lit, ok2 := be.Y.(*ast.BasicLit)
			if ok && ok2 && id.Name == "fn" && lit.Kind == token.STRING {
				if s, err := strconv.Unquote(lit.Value); err == nil {
					names = append(names, "runtime."+s)
				}
			}
			return true
		})
		return false
	})
	return names
}

type liveness struct {
	live     []bool
	noReturn bool
}

const (
	exitReturn = -2
	exitDead   = -3
)

func successors(b *body, noReturn map[string]bool) (succ [][]int, jumpTable bool) {
	n := len(b.insts)
	byAddr := make(map[int64]int, n)
	for i, in := range b.insts {
		byAddr[in.addr] = i
	}
	succ = make([][]int, n)
	for i, in := range b.insts {
		next := exitDead
		if i+1 < n {
			next = i + 1
		}
		switch {
		case in.op == "RET":
			succ[i] = []int{exitReturn}
		case in.op == "INT" || in.op == "UD2" || in.op == "HLT":
			succ[i] = []int{exitDead}
		case in.op[0] == 'J' || strings.HasPrefix(in.op, "LOOP"):
			var jt bool
			succ[i], jt = jumpSuccessors(in, byAddr, next, noReturn)
			jumpTable = jumpTable || jt
		case in.op == "CALL" && in.call != "" && noReturn[in.call]:
			succ[i] = []int{exitDead}
		case in.op == "CALL" && strings.HasPrefix(in.call, "runtime.morestack"):
			succ[i] = []int{exitDead}
		default:
			succ[i] = []int{next}
		}
	}
	return succ, jumpTable
}

func jumpSuccessors(in inst, byAddr map[int64]int, next int, noReturn map[string]bool) ([]int, bool) {
	switch {
	case in.call != "":
		if noReturn[in.call] {
			return []int{exitDead}, false
		}
		return []int{exitReturn}, false
	case in.indirect:
		return []int{exitReturn}, true
	}
	to, ok := byAddr[in.target]
	if !ok {
		to = exitReturn
	}
	if in.op == "JMP" {
		return []int{to}, false
	}
	return []int{to, next}, false
}

func analyzeBody(b *body, noReturn map[string]bool) liveness {
	n := len(b.insts)
	succ, jumpTable := successors(b, noReturn)
	pred := make([][]int, n)
	open := make([]int, n)
	doomed := make([]bool, n)
	var q []int
	for i, ss := range succ {
		for _, s := range ss {
			switch s {
			case exitReturn:
				open[i]++
			case exitDead:
			default:
				pred[s] = append(pred[s], i)
				open[i]++
			}
		}
		if open[i] == 0 {
			doomed[i] = true
			q = append(q, i)
		}
	}
	for len(q) > 0 {
		i := q[len(q)-1]
		q = q[:len(q)-1]
		for _, p := range pred[i] {
			if open[p]--; open[p] == 0 && !doomed[p] {
				doomed[p] = true
				q = append(q, p)
			}
		}
	}
	entered := make([]bool, n)
	if jumpTable {
		for i := range entered {
			entered[i] = true
		}
	} else {
		q := []int{0}
		entered[0] = true
		for len(q) > 0 {
			i := q[len(q)-1]
			q = q[:len(q)-1]
			for _, s := range succ[i] {
				if s >= 0 && !entered[s] {
					entered[s] = true
					q = append(q, s)
				}
			}
		}
	}
	l := liveness{live: make([]bool, n), noReturn: doomed[0]}
	for i := range n {
		l.live[i] = entered[i] && !doomed[i]
	}
	return l
}

type callGraph struct {
	bodies   map[string][]*body
	noReturn map[string]bool
	edges    map[string]map[string]bool
	indirect map[string][]position
	sinks    map[string]bool
}

func newCallGraph(bodies map[string][]*body, noReturn []string) *callGraph {
	g := &callGraph{bodies: bodies, noReturn: map[string]bool{}, edges: map[string]map[string]bool{}, indirect: map[string][]position{}, sinks: map[string]bool{}}
	for _, n := range noReturn {
		g.noReturn[n] = true
	}
	callers := map[string][]string{}
	for name, bs := range bodies {
		for _, b := range bs {
			for _, in := range b.insts {
				if in.call != "" {
					callers[in.call] = append(callers[in.call], name)
				}
			}
		}
	}
	lives := map[*body]liveness{}
	work := make([]string, 0, len(bodies))
	for name := range bodies {
		work = append(work, name)
	}
	queued := map[string]bool{}
	for len(work) > 0 {
		name := work[len(work)-1]
		work = work[:len(work)-1]
		queued[name] = false
		all := true
		for _, b := range bodies[name] {
			l := analyzeBody(b, g.noReturn)
			lives[b] = l
			all = all && l.noReturn
		}
		if all && !g.noReturn[name] {
			g.noReturn[name] = true
			for _, c := range callers[name] {
				if !queued[c] {
					queued[c] = true
					work = append(work, c)
				}
			}
		}
	}
	for name, bs := range bodies {
		e := map[string]bool{}
		rt := inRuntime(name)
		for _, b := range bs {
			l := lives[b]
			for i, in := range b.insts {
				if !l.live[i] {
					continue
				}
				if in.call != "" {
					e[in.call] = true
				}
				for _, r := range in.refs {
					if _, ok := bodies[r]; ok && !rt {
						e[r] = true
					}
				}
				if in.indirect && in.call == "" {
					g.indirect[name] = append(g.indirect[name], in.pos)
				}
			}
		}
		delete(e, name)
		g.edges[name] = e
	}
	for name, e := range g.edges {
		for _, p := range allocPrimitives {
			if e[p] {
				g.sinks[name] = true
			}
		}
	}
	for _, p := range allocPrimitives {
		g.sinks[p] = true
	}
	return g
}

func inRuntime(sym string) bool {
	pkg := symPackage(sym)
	return pkg == "runtime" || strings.HasPrefix(pkg, "runtime/") || strings.HasPrefix(pkg, "internal/runtime/")
}

func (g *callGraph) addEdge(from, to string) {
	if from == to {
		return
	}
	e := g.edges[from]
	if e == nil {
		e = map[string]bool{}
		g.edges[from] = e
	}
	e[to] = true
}

func (g *callGraph) pathsToSinks(avoid map[string]bool) map[string]string {
	rev := map[string][]string{}
	for from, e := range g.edges {
		if avoid[from] {
			continue
		}
		for to := range e {
			rev[to] = append(rev[to], from)
		}
	}
	for _, r := range rev {
		slices.Sort(r)
	}
	next := map[string]string{}
	var q []string
	for s := range g.sinks {
		next[s] = ""
		q = append(q, s)
	}
	slices.Sort(q)
	for len(q) > 0 {
		s := q[0]
		q = q[1:]
		for _, p := range rev[s] {
			if _, ok := next[p]; !ok {
				next[p] = s
				q = append(q, p)
			}
		}
	}
	return next
}

func chain(next map[string]string, name string) []string {
	var c []string
	for n := name; n != ""; n = next[n] {
		c = append(c, n)
	}
	return c
}
