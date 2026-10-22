package capigen

import (
	"bufio"
	"bytes"
	"fmt"
	"maps"
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

const sizedHead = "{ size_t size;"

// sizedLine returns the lock line of the struct c when it has a size field.
func (l *abiLock) sizedLine(c string) string {
	head := lockType + " struct " + c + " " + sizedHead
	for line := range l.lines {
		if strings.HasPrefix(line, head) {
			return line
		}
	}
	return ""
}

// grown reports whether line, a struct with a size field, is old with
// members added at the end, the first of them at or after the sizeof of
// old. A member in the tail padding of old would count as set by a caller
// built against old, whose size covers it.
func grown(old, line string, ct *cTypes) bool {
	if !strings.HasPrefix(old, lockType+" struct ") || !strings.Contains(old, sizedHead) {
		return false
	}
	if !strings.HasPrefix(line, strings.TrimSuffix(old, "};")) {
		return false
	}
	oldMembers := members(old)
	oldSize, _, ok := ct.layout(oldMembers)
	if !ok {
		return false
	}
	offset, ok := ct.offset(members(line), len(oldMembers))
	return ok && offset >= oldSize
}

// cTypes holds what the lock lines tell about C types, to lay out structs
// as the LP64 C ABI does.
type cTypes struct {
	scalars map[string]string
	structs map[string][]string
}

func newCTypes(scalars map[string]string, lines ...[]string) *cTypes {
	ct := &cTypes{scalars: scalars, structs: map[string][]string{}}
	for _, ls := range lines {
		for _, l := range ls {
			name, ok := strings.CutPrefix(l, lockType+" struct ")
			if !ok {
				continue
			}
			name, _, _ = strings.Cut(name, " ")
			if _, seen := ct.structs[name]; !seen {
				ct.structs[name] = members(l)
			}
		}
	}
	return ct
}

// members splits the body of a struct line into its member declarations.
func members(line string) []string {
	i, j := strings.Index(line, "{"), strings.LastIndex(line, "}")
	if i < 0 || j < i {
		return nil
	}
	var out []string
	depth, start := 0, i+1
	for k := i + 1; k < j; k++ {
		switch line[k] {
		case '{', '(':
			depth++
		case '}', ')':
			depth--
		case ';':
			if depth == 0 {
				if d := strings.TrimSpace(line[start:k]); d != "" {
					out = append(out, d)
				}
				start = k + 1
			}
		}
	}
	return out
}

func scalarSize(t string) uintptr {
	switch t {
	case "bool", "int8_t", "uint8_t":
		return 1
	case "int16_t", "uint16_t":
		return 2
	case "int", "int32_t", "uint32_t":
		return 4
	case "int64_t", "uint64_t", "size_t", "double", "DMRGO_USER_T":
		return 8
	}
	return 0
}

func (ct *cTypes) offset(ms []string, n int) (uintptr, bool) {
	var off uintptr
	for i, m := range ms {
		size, align, ok := ct.member(m)
		if !ok {
			return 0, false
		}
		off = (off + align - 1) &^ (align - 1)
		if i == n {
			return off, true
		}
		off += size
	}
	return 0, false
}

func (ct *cTypes) layout(ms []string) (size, align uintptr, ok bool) {
	align = 1
	for _, m := range ms {
		s, a, ok := ct.member(m)
		if !ok {
			return 0, 0, false
		}
		size = (size+a-1)&^(a-1) + s
		align = max(align, a)
	}
	return (size + align - 1) &^ (align - 1), align, true
}

func (ct *cTypes) member(d string) (size, align uintptr, ok bool) {
	d = strings.TrimPrefix(d, "const ")
	if body, ok := strings.CutPrefix(d, "union "); ok {
		var u, a uintptr = 0, 1
		for _, m := range members(body) {
			s, ma, ok := ct.member(m)
			if !ok {
				return 0, 0, false
			}
			u, a = max(u, s), max(a, ma)
		}
		return (u + a - 1) &^ (a - 1), a, true
	}
	if strings.Contains(d, "(*") {
		return 8, 8, true
	}
	if strings.Contains(d, "*") {
		size, align = 8, 8
	} else {
		t, _, _ := strings.Cut(d, " ")
		if size, align, ok = ct.typ(t, 0); !ok {
			return 0, 0, false
		}
	}
	for rest := d; ; {
		i := strings.Index(rest, "[")
		if i < 0 {
			break
		}
		j := strings.Index(rest[i:], "]")
		if j < 0 {
			return 0, 0, false
		}
		n, err := strconv.Atoi(rest[i+1 : i+j])
		if err != nil {
			return 0, 0, false
		}
		size *= uintptr(n)
		rest = rest[i+j:]
	}
	return size, align, true
}

func (ct *cTypes) typ(t string, depth int) (size, align uintptr, ok bool) {
	if depth > 32 {
		return 0, 0, false
	}
	if s := scalarSize(t); s != 0 {
		return s, s, true
	}
	if base, ok := ct.scalars[t]; ok {
		return ct.typ(base, depth+1)
	}
	if ms, ok := ct.structs[t]; ok {
		return ct.layout(ms)
	}
	return 0, 0, false
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
	ct := newCTypes(scalars, syms, slices.Collect(maps.Keys(l.lines)))
	for s := range l.lines {
		if !now[s] && !same[layoutOf(s, scalars)] && !slices.ContainsFunc(syms, func(n string) bool { return grown(s, n, ct) }) {
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
