//go:build !tinygo

package allocgate_test

import (
	"fmt"
	"slices"
	"strings"
)

const semacquirePrefix = "runtime_Semacquire"

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
