//go:build !tinygo

package allocgate_test

import (
	"fmt"
	"slices"
	"strings"
)

// semacquirePrefix starts the name of each runtime function that sync calls
// to block a goroutine on a contended Mutex or RWMutex, such as
// internal/sync.runtime_SemacquireMutex or sync.runtime_SemacquireRWMutexR.
const semacquirePrefix = "runtime_Semacquire"

// runtimeCaches returns the runtime functions that allocate only to fill a
// cache whose size the program bounds, not the number of calls, so the gate
// does not follow calls into them. It starts from these runtime functions:
//
//   - runtime.typeAssert and runtime.interfaceSwitch, which the compiler
//     calls for a type assertion or a type switch to an interface type when
//     its inline cache misses. They build the cache, and an itab in the
//     global itab table, once for each call site and dynamic type.
//   - the runtime_Semacquire functions of sync, which block a goroutine on a
//     contended mutex. The goroutine takes a sudog from a per-P cache and
//     returns it there, so the runtime allocates a sudog only when the
//     caches are empty, at most once for each goroutine that waits at the
//     same time. The cut also covers the other blocking operations that
//     take a sudog, such as a channel operation.
//
// From each, it follows the runtime callees that reach an allocator, and are
// not entries, to the builders: the functions whose allocating callees are
// all entries. The entries are the allocators and the runtime functions that
// compiled code calls, the builtins of the compiler, such as runtime.mallocgc
// and runtime.newobject. The builders are the cut. It returns an error, which
// fails the gate, when a start function is missing or calls an entry that
// allocates itself, since the runtime then no longer has that shape.
func (gt *gate) runtimeCaches(cached []string, builtins map[string]bool) (map[string]bool, error) {
	next := gt.g.pathsToSinks(nil)
	entry := func(f string) bool { return builtins[f] || gt.g.sinks[f] }
	allocating := func(f string) []string {
		var cs []string
		for c := range gt.g.edges[f] {
			if _, ok := next[c]; ok && inRuntime(c) {
				cs = append(cs, c)
			}
		}
		slices.Sort(cs)
		return cs
	}
	starts := slices.Clone(cached)
	sema := 0
	for name := range gt.g.bodies {
		if i := strings.LastIndexByte(name, '.'); i > 0 && strings.HasPrefix(name[i+1:], semacquirePrefix) {
			starts = append(starts, name)
			sema++
		}
	}
	if sema == 0 {
		return nil, fmt.Errorf("no runtime function named %s*", semacquirePrefix)
	}
	cut := map[string]bool{}
	for _, s := range starts {
		if _, ok := gt.g.bodies[s]; !ok {
			return nil, fmt.Errorf("%s has no machine code", s)
		}
		seen := map[string]bool{s: true}
		q := []string{s}
		for len(q) > 0 {
			f := q[0]
			q = q[1:]
			cs := allocating(f)
			if f != s && !slices.ContainsFunc(cs, func(c string) bool { return !entry(c) }) {
				cut[f] = true
				continue
			}
			for _, c := range cs {
				if entry(c) {
					return nil, fmt.Errorf("%s reaches the allocator entry %s through %s, not through a private builder", s, c, f)
				}
				if !seen[c] {
					seen[c] = true
					q = append(q, c)
				}
			}
		}
	}
	if len(cut) == 0 {
		return nil, fmt.Errorf("the runtime cache rule found no builder from %s", strings.Join(starts, ", "))
	}
	return cut, nil
}
