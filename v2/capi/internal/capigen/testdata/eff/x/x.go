package x

type M struct {
	N    int
	Data []byte
	Sub  S
	P    *S
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
