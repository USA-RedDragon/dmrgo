package x

import (
	"errors"
	"strings"
)

type M struct {
	N    int
	Data []byte
	Sub  S
	S2   S
	P    *S
	Any  any
	Map  map[int]*S
	E    E
	Sb   strings.Builder
	Err  error
	Ch   chan []byte
}

type E struct {
	S
	K int
}

func (s *S) SetA() { s.A = 1 }

func (s S) Poke() { s.B[0] = 1 }

func (m *M) CommaOK() {
	if v, ok := m.Any.(*S); ok {
		v.A = 1
	}
}

func (m *M) MapOK() {
	if v, ok := m.Map[1]; ok {
		v.A = 1
	}
}

func (m *M) TypeSwitch() {
	switch v := m.Any.(type) {
	case *S:
		v.A = 1
	}
}

func (m *M) Reassign(c bool) {
	d := m.Data
	if c {
		d = m.Sub.B
	}
	d[0] = 1
}

func (m *M) CopyFill() {
	c := *m
	copy(c.Data, "ab")
}

func (m *M) BoxFill() {
	v := S{B: m.Data}
	copy(v.B, "x")
}

func (m *M) Promoted() { m.E.A = 1 }

func (m *M) PromotedMethod() { m.E.SetA() }

func (m *M) Builder() { m.Sb.WriteString("x") }

func (m *M) As(err error) bool { return errors.As(err, &m.Err) }

func (m *M) Del() { delete(m.Map, 1) }

func (m *M) Send() {
	m.Ch <- m.Data
}

func (m M) ValuePoke() { m.Data[0] = 1 }

type pair struct{ d, s []byte }

func Fill(dst, src []byte) {
	pr := pair{dst, src}
	copy(pr.d, pr.s)
}

type S struct {
	A int
	B []byte
}

func (m *M) Read() int { return m.N + len(m.Data) + m.Sub.A }

func (m *M) SetN(n int) { m.N = n }

func (m *M) Fill() {
	for i := range m.Data {
		m.Data[i] = 1
	}
}

func (m *M) Grow() { m.Data = append(m.Data, 1) }

func (m *M) Copy() int {
	c := *m
	c.N = 5
	return c.N
}

func (m *M) CopyWrite() {
	c := *m
	c.Data[0] = 1
}

func (m *M) Via() { m.SetN(3) }

func (m *M) Alias() {
	d := m.Data[1:]
	copy(d, "ab")
}

func (m *M) Sub2() { m.Sub.A = 1 }

func (m *M) Ptr() { m.P.A = 2 }

func (m *M) Unknown(f func(*M)) { f(m) }

func (m *M) Closure() func() { return func() { m.N++ } }

func (m *M) Box() int {
	v := S{B: m.Data}
	v.A = 1
	return v.A
}

func Decode(data []byte) S { return S{B: data[1:]} }

func Sign(p []byte) {
	_, sig := span(p)
	copy(sig, "x")
}

func span(p []byte) ([]byte, []byte) { return p[:1], p[1:] }

func Into(m *M, data []byte) { m.Data = data }

func Look(m *M) int { return Helper(m) }

func Helper(m *M) int { return m.N }

func DecodeInto(m *M, data []byte) {
	if m == nil {
		return
	}
	*m = M{Data: data}
}

func ReadFirst(m *M) {
	n := m.N
	*m = M{N: n + 1}
}

type holder struct {
	m M
	n int
}

func wrap(m M) *holder { return &holder{m: m} }

func Wrapped(m *M) {
	c := *m
	h := wrap(c)
	h.n = 1
}
