package capigen

import (
	"bufio"
	"bytes"
	"fmt"
	"slices"
	"strconv"
	"strings"
)

const firstCode = -100

const lockPre = "prerelease"

type abiLock struct {
	abi   int
	pre   bool
	codes map[string]int
	tags  map[string]int
	lines map[string]bool
	used  map[string]bool
	pfx   map[string]string
	upfx  map[string]bool
	gone  map[string]int
}

func parseLock(b []byte) (*abiLock, error) {
	l := &abiLock{abi: 1, codes: map[string]int{}, tags: map[string]int{}, lines: map[string]bool{}, used: map[string]bool{}, pfx: map[string]string{}, upfx: map[string]bool{}, gone: map[string]int{}}
	sc := bufio.NewScanner(bytes.NewReader(b))
	sc.Buffer(nil, 1<<24)
	for sc.Scan() {
		line := sc.Text()
		f := strings.Fields(line)
		if len(f) == 0 {
			continue
		}
		switch f[0] {
		case "abi", lockCode, "tag":
			n, err := strconv.Atoi(f[len(f)-1])
			if err != nil {
				return nil, fmt.Errorf("abi lock: %q: %w", line, err)
			}
			switch f[0] {
			case "abi":
				l.abi = n
			case lockCode:
				l.codes[f[1]] = n
			default:
				if len(f) != 4 {
					return nil, fmt.Errorf("abi lock: %q", line)
				}
				l.tags[f[1]+" "+f[2]] = n
			}
		case lockPre:
			l.pre = true
		case "retired":
			n, err := strconv.Atoi(f[len(f)-1])
			if err != nil || len(f) < 4 {
				return nil, fmt.Errorf("abi lock: %q", line)
			}
			l.gone[strings.Join(f[1:len(f)-1], " ")] = n
		case "prefix":
			if len(f) != 3 {
				return nil, fmt.Errorf("abi lock: %q", line)
			}
			l.pfx[f[1]] = f[2]
		case "sym", lockType:
			l.lines[line] = true
		default:
			return nil, fmt.Errorf("abi lock: unknown line %q", line)
		}
	}
	return l, sc.Err()
}

func (l *abiLock) code(name string) int {
	l.used["code "+name] = true
	if c, ok := l.codes[name]; ok {
		return c
	}
	c := firstCode
	for _, v := range l.codes {
		c = min(c, v-1)
	}
	for k, v := range l.gone {
		if strings.HasPrefix(k, lockCode+" ") {
			c = min(c, v-1)
		}
	}
	l.codes[name] = c
	return c
}

// renumber gives a lock with the prerelease marker gapless codes in the
// order of names, from firstCode down, and tags in name order from 1 for
// each union. After the release the numbers only grow.
func (l *abiLock) renumber(names []string, tags map[string][]string) {
	if !l.pre {
		return
	}
	l.codes = map[string]int{}
	for i, n := range names {
		l.codes[n] = firstCode - i
	}
	l.tags = map[string]int{}
	for u, ns := range tags {
		for i, n := range ns {
			l.tags[u+" "+n] = i + 1
		}
	}
	l.gone = map[string]int{}
}

func (l *abiLock) prefix(path string, rule func() string) (string, error) {
	l.upfx[path] = true
	if p, ok := l.pfx[path]; ok {
		return p, nil
	}
	p := rule()
	for o, q := range l.pfx {
		if q == p {
			delete(l.upfx, path)
			return "", fmt.Errorf("packages %s and %s have the C prefix %s", o, path, p)
		}
	}
	l.pfx[path] = p
	return p, nil
}

func (l *abiLock) tag(union, name string) int {
	key := union + " " + name
	l.used["tag "+key] = true
	if t, ok := l.tags[key]; ok {
		return t
	}
	t := 1
	for k, v := range l.tags {
		if strings.HasPrefix(k, union+" ") {
			t = max(t, v+1)
		}
	}
	for k, v := range l.gone {
		if strings.HasPrefix(k, "tag "+union+" ") {
			t = max(t, v+1)
		}
	}
	l.tags[key] = t
	return t
}

func (l *abiLock) render(syms []string, allowBreak bool) ([]byte, []string, error) {
	var broken []string
	now := map[string]bool{}
	for _, s := range syms {
		now[s] = true
	}
	scalars := scalarTypedefs(syms, l.lines)
	same := map[string]bool{}
	for s := range now {
		same[layoutOf(s, scalars)] = true
	}
	for s := range l.lines {
		if !now[s] && !same[layoutOf(s, scalars)] {
			broken = append(broken, s)
		}
	}
	for k := range l.codes {
		if !l.used["code "+k] {
			broken = append(broken, "code "+k)
		}
	}
	for k := range l.tags {
		if !l.used["tag "+k] {
			broken = append(broken, "tag "+k)
		}
	}
	slices.Sort(broken)
	if len(broken) > 0 {
		if !allowBreak && !l.pre {
			return nil, broken, fmt.Errorf("the C ABI changed in %d places; run the generator with -break to bump DMRGO_ABI_VERSION", len(broken))
		}
		if !l.pre {
			l.abi++
		}
		for _, b := range broken {
			f := strings.Fields(b)
			switch f[0] {
			case lockCode:
				if !l.pre {
					l.gone[b] = l.codes[f[1]]
				}
				delete(l.codes, f[1])
			case "tag":
				if !l.pre {
					l.gone[b] = l.tags[f[1]+" "+f[2]]
				}
				delete(l.tags, f[1]+" "+f[2])
			}
		}
	}
	var b bytes.Buffer
	fmt.Fprintf(&b, "abi %d\n", l.abi)
	if l.pre {
		b.WriteString(lockPre + "\n")
	}
	type kv struct {
		k string
		v int
	}
	var cs []kv
	for k, v := range l.codes {
		cs = append(cs, kv{k, v})
	}
	slices.SortFunc(cs, func(a, b kv) int { return b.v - a.v })
	for _, c := range cs {
		fmt.Fprintf(&b, "code %s %d\n", c.k, c.v)
	}
	var gs []string
	for k, v := range l.gone {
		gs = append(gs, fmt.Sprintf("retired %s %d", k, v))
	}
	slices.Sort(gs)
	for _, g := range gs {
		b.WriteString(g + "\n")
	}
	var ps []string
	for p := range l.upfx {
		ps = append(ps, "prefix "+p+" "+l.pfx[p])
	}
	slices.Sort(ps)
	for _, p := range ps {
		b.WriteString(p + "\n")
	}
	var ts []kv
	for k, v := range l.tags {
		ts = append(ts, kv{k, v})
	}
	slices.SortFunc(ts, func(a, b kv) int { return strings.Compare(a.k, b.k) })
	for _, t := range ts {
		fmt.Fprintf(&b, "tag %s %d\n", t.k, t.v)
	}
	s := slices.Clone(syms)
	slices.Sort(s)
	for _, x := range slices.Compact(s) {
		b.WriteString(x + "\n")
	}
	return b.Bytes(), broken, nil
}

func scalarTypedefs(syms []string, old map[string]bool) map[string]string {
	m := map[string]string{}
	add := func(s string) {
		f := strings.Fields(strings.TrimSuffix(s, ";"))
		if len(f) == 4 && f[0] == "sym" && f[1] == "typedef" && strings.HasSuffix(f[2], "_t") && !strings.HasPrefix(f[2], "dmrgo_") {
			m[f[3]] = f[2]
		}
	}
	for _, s := range syms {
		add(s)
	}
	for s := range old {
		add(s)
	}
	return m
}

func layoutOf(line string, scalars map[string]string) string {
	if !strings.HasPrefix(line, lockType+" ") && (!strings.HasPrefix(line, "sym ") || strings.HasPrefix(line, "sym typedef ") || strings.HasPrefix(line, "sym #define ")) {
		return line
	}
	var b strings.Builder
	for i := 0; i < len(line); {
		j := i
		for j < len(line) && (line[j] == '_' || line[j] >= 'a' && line[j] <= 'z' || line[j] >= 'A' && line[j] <= 'Z' || line[j] >= '0' && line[j] <= '9') {
			j++
		}
		if j == i {
			b.WriteByte(line[i])
			i++
			continue
		}
		if base, ok := scalars[line[i:j]]; ok {
			b.WriteString(base)
		} else {
			b.WriteString(line[i:j])
		}
		i = j
	}
	return b.String()
}
