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

// allocPrimitives are the methods of the runtime that take memory for a heap
// object from the cache of the P. Every heap allocation of the gc runtime
// goes through one of them; the gate derives the allocator entry points,
// such as runtime.mallocgc and its size-class variants, as the functions
// that call them.
var allocPrimitives = []string{"runtime.(*mcache).nextFree", "runtime.(*mcache).allocLarge"}

// compilerFacts holds what the gate reads from the source of the compiler of
// the go command, cmd/compile/internal/ssagen/ssa.go.
type compilerFacts struct {
	// noReturn holds the runtime functions that the compiler treats as
	// never returning: it ends the block of a call to one of them, so that
	// no code follows the call. They are the names that the condition of
	// the statement that sets the block kind to ssa.BlockExit compares
	// with, and the panic functions of the failed checks.
	noReturn []string
	// growth holds the runtime functions that the compiler calls for
	// append: growslice and its variants, and the moveSlice functions
	// that move a slice that append built on the stack to the heap.
	growth map[string]bool
	// cached holds the runtime functions that the compiler calls for a
	// type assertion or a type switch to an interface type after a miss of
	// the inline cache: typeAssert and interfaceSwitch.
	cached []string
	// newproc is the runtime function of a go statement, which allocates
	// the goroutine on a path that runs on the system stack, out of sight
	// of the call graph, so the gate counts it as an allocator.
	newproc string
	// builtins holds the runtime functions that compiled code calls: the
	// functions of cmd/compile/internal/typecheck/_builtin/runtime.go.
	builtins map[string]bool
}

// panicRE matches the lines of ssa.go that look up a runtime function that
// panics, such as panicBounds or goPanicIndex, which the compiler calls at
// the end of a block of kind ssa.BlockExit for a failed bounds, nil, shift
// or division check.
var panicRE = regexp.MustCompile(`LookupRuntimeFunc\("(\w*[pP]anic\w*)"\)`)

// newprocRE matches the line of ssa.go that looks up the runtime function
// of a go statement.
var newprocRE = regexp.MustCompile(`ir\.Syms\.Newproc = typecheck\.LookupRuntimeFunc\("(\w+)"\)`)

// cachedRE matches the lines of ssa.go that look up the runtime functions of
// the cached type assertion and type switch.
var cachedRE = regexp.MustCompile(`ir\.Syms\.(?:TypeAssert|InterfaceSwitch) = typecheck\.LookupRuntimeFunc\("(\w+)"\)`)

// growthRE matches the line of ssa.go that looks up a runtime function of
// append, such as ir.Syms.Growslice = typecheck.LookupRuntimeFunc("growslice").
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

// readBuiltins returns the functions that the builtin file of the compiler
// declares, as runtime symbols.
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

// blockExitNames returns the runtime functions that the condition of the
// statement of f that sets the block kind to ssa.BlockExit compares the
// called name fn with.
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

// liveness holds, for each instruction of a body, whether a path from it can
// return to the caller.
type liveness struct {
	// live reports, for each instruction, that the entry reaches it and
	// that it is not doomed: a doomed instruction is one whose every path
	// ends in a call that cannot return or a trap, without a cycle. It runs
	// only on the way to a panic, a throw or a switch to another goroutine,
	// so a call there is not a call of the function.
	live []bool
	// noReturn reports that the entry is doomed.
	noReturn bool
}

// The successors of an instruction that leaves the function: by a return,
// or by a call that cannot return or a trap.
const (
	exitReturn = -2
	exitDead   = -3
)

// successors returns the successors of each instruction of b, and whether b
// jumps through a register, given the functions that cannot return.
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
			// The stack growth block: it calls morestack and jumps back
			// to the entry, so the paths through it are the paths of the
			// function.
			succ[i] = []int{exitDead}
		default:
			succ[i] = []int{next}
		}
	}
	return succ, jumpTable
}

// jumpSuccessors returns the successors of the jump in, whose next
// instruction is next, and whether it jumps through a register.
func jumpSuccessors(in inst, byAddr map[int64]int, next int, noReturn map[string]bool) ([]int, bool) {
	switch {
	case in.call != "":
		if noReturn[in.call] {
			return []int{exitDead}, false
		}
		return []int{exitReturn}, false
	case in.indirect:
		// A jump through a register is a tail call through a func value
		// or an interface, or a jump table of a switch, whose targets the
		// gate does not know, so it counts every instruction as reachable.
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

// analyzeBody computes the liveness of b, given the functions that cannot
// return.
func analyzeBody(b *body, noReturn map[string]bool) liveness {
	n := len(b.insts)
	succ, jumpTable := successors(b, noReturn)
	// An instruction is doomed when every successor is doomed or a dead
	// exit. This is the least fixpoint from no doomed instruction, so a
	// cycle, such as a loop with a panic exit, is never doomed.
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

// callGraph is the static call graph of the machine code.
type callGraph struct {
	bodies   map[string][]*body
	noReturn map[string]bool
	// edges holds, for each function, the functions that its live code
	// calls, tail-calls or takes the address of.
	edges map[string]map[string]bool
	// indirect holds, for each function, the positions of its live
	// indirect calls.
	indirect map[string][]position
	// sinks holds the allocator entry points.
	sinks map[string]bool
}

// newCallGraph computes the functions that cannot return, as a fixed point,
// and then the edges of the live code.
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

// inRuntime reports whether sym is code of the runtime. The runtime takes
// the address of a function to run it on another stack or at another time:
// the system stack, the scheduler, the profilers and the cleanup goroutine.
// That code works for the runtime, not for the caller, so the address of a
// function in runtime code is not an edge. A direct call is.
func inRuntime(sym string) bool {
	pkg := symPackage(sym)
	return pkg == "runtime" || strings.HasPrefix(pkg, "runtime/") || strings.HasPrefix(pkg, "internal/runtime/")
}

// addEdge adds an edge that the machine code does not show, such as the
// call of an interface method.
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

// pathsToSinks returns, for every function from which a sink is reachable
// without passing through a function of avoid, the next function on a
// shortest path to a sink. A sink maps to "".
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

// chain returns the call chain from name to a sink.
func chain(next map[string]string, name string) []string {
	var c []string
	for n := name; n != ""; n = next[n] {
		c = append(c, n)
	}
	return c
}
