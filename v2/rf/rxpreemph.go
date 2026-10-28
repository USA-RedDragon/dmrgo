package rf

// A RxPreEmphasis is the pre-emphasis of an AudioReceiver for a radio that
// de-emphasizes its discriminator output from 300 Hz to 3 kHz, as the
// speaker and accessory outputs of FM radios do, which
// AudioRxConfig.PreEmphasis takes. It is the receive twin of PreEmphasis:
// the zero RxPreEmphasis is not set up, Init sets it up in memory of the
// caller, such as a package variable, and NewRxPreEmphasis allocates one.
// A program that sets up none links none of its filter. It holds the state
// of the filter, so each AudioReceiver takes a RxPreEmphasis of its own.
type RxPreEmphasis struct {
	f      rxPE
	start  func(f *rxPE, hz int)
	filter func(f *rxPE, x []int16)
}

type rxPE struct {
	x1, y1 int32
	g, k   int32
}

// NewRxPreEmphasis allocates a RxPreEmphasis and returns it set up.
func NewRxPreEmphasis() *RxPreEmphasis {
	return new(RxPreEmphasis).Init()
}

// Init sets up p, in place of any state it holds, and returns p, for
// AudioRxConfig.PreEmphasis. It does not allocate. A nil p returns nil.
func (p *RxPreEmphasis) Init() *RxPreEmphasis {
	if p == nil {
		return nil
	}
	*p = RxPreEmphasis{start: rxPEStart, filter: rxPEFilter}
	return p
}

func (p *RxPreEmphasis) set() bool {
	return p != nil && p.start != nil && p.filter != nil
}

func rxPEStart(f *rxPE, hz int) {
	q := [...][2]int32{{10768, 2137}, {12035, 1869}, {15221, 1168}}
	i := 2
	switch hz {
	case 2 * symbolRate:
		i = 0
	case discRate / 2:
		i = 1
	}
	*f = rxPE{g: q[i][0], k: q[i][1]}
}

func rxPEFilter(f *rxPE, x []int16) {
	x1, y1, g, k := f.x1, f.y1, f.g, f.k
	for i, v := range x {
		c := int32(v)
		y := y1 + (g*(c-x1)+1<<10)>>11 + (k*(x1-y1)+1<<10)>>11
		x[i] = int16(max(min(y, 32767), -32768))
		x1, y1 = c, y
	}
	f.x1, f.y1 = x1, y1
}

func (r *AudioReceiver) emit(x []int16) {
	if r.pe != nil {
		r.pe.filter(&r.pe.f, x)
	}
	r.write(x)
}

//go:noinline
func (r *AudioReceiver) emitCopy(x []int16) {
	b := r.d48.buf[:]
	for len(x) > 0 {
		m := copy(b, x)
		r.emit(b[:m])
		x = x[m:]
	}
}
