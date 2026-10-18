package bits

import "testing"

// fieldByBits returns the n bits of b from bit start, read one at a time.
func fieldByBits(b []byte, start, n int) uint64 {
	var v uint64
	for i := range n {
		v = v<<1 | uint64(Bit(b, start+i))
	}
	return v
}

// fieldSeed is one seed of FuzzField.
type fieldSeed struct {
	buf   []byte
	start uint16
	width uint8
	v     uint64
	dst   uint16
}

// fieldSeeds returns the seeds of FuzzField.
func fieldSeeds() []fieldSeed {
	return []fieldSeed{
		{[]byte{0x12, 0x34, 0x56, 0x78, 0x9A, 0xBC, 0xDE, 0xF0, 0x0F}, uint16(3), uint8(57), uint64(0xFFFFFFFFFFFFFFFF), uint16(5)},
		{[]byte{0xFF, 0x00, 0xFF, 0x00, 0xFF, 0x00, 0xFF, 0x00, 0xFF, 0x00}, uint16(7), uint8(64), uint64(0x0123456789ABCDEF), uint16(1)},
		{[]byte{0x80}, uint16(0), uint8(1), uint64(0), uint16(0)},
	}
}

// checkField checks the properties of FuzzField for one input.
func checkField(t testing.TB, buf []byte, start uint16, width uint8, v uint64, dst uint16) {
	t.Helper()
	total := 8 * len(buf)
	if total == 0 {
		return
	}
	s := int(start) % total
	n := min(int(width)%65, total-s)
	mask := uint64(1)<<n - 1
	if n == 64 {
		mask = ^uint64(0)
	}

	if got, want := Field64(buf, s, n), fieldByBits(buf, s, n); got != want {
		t.Fatalf("Field64(%x, %d, %d) = %#x, want %#x", buf, s, n, got, want)
	}
	if n <= 57 {
		if got, want := Field(buf, s, n), fieldByBits(buf, s, n); got != want {
			t.Fatalf("Field(%x, %d, %d) = %#x, want %#x", buf, s, n, got, want)
		}
	}

	setters := []struct {
		name string
		set  func([]byte, int, int, uint64)
		max  int
	}{{"SetField", SetField, 57}, {"SetField64", SetField64, 64}}
	for _, w := range setters {
		if n > w.max {
			continue
		}
		out := append([]byte(nil), buf...)
		w.set(out, s, n, v)
		if got := fieldByBits(out, s, n); got != v&mask {
			t.Fatalf("%s(%x, %d, %d, %#x) wrote %#x", w.name, buf, s, n, v, got)
		}
		for i := range total {
			if (i < s || i >= s+n) && Bit(out, i) != Bit(buf, i) {
				t.Fatalf("%s(%x, %d, %d, %#x) changed bit %d", w.name, buf, s, n, v, i)
			}
		}
	}

	out := append([]byte(nil), buf...)
	SetBit(out, s, byte(v))
	for i := range total {
		want := Bit(buf, i)
		if i == s {
			want = byte(v & 1)
		}
		if Bit(out, i) != want {
			t.Fatalf("SetBit(%x, %d, %d) = %x", buf, s, v&1, out)
		}
	}

	d := int(dst) % total
	k := min(n, total-d)
	copied := make([]byte, len(buf))
	for i := range copied {
		copied[i] = ^buf[i]
	}
	before := append([]byte(nil), copied...)
	Copy(copied, d, buf, s, k)
	for i := range total {
		want := Bit(before, i)
		if i >= d && i < d+k {
			want = Bit(buf, s+i-d)
		}
		if Bit(copied, i) != want {
			t.Fatalf("Copy(%d bits from %d of %x to %d) bit %d = %d, want %d", k, s, buf, d, i, Bit(copied, i), want)
		}
	}
}

// FuzzField reads, writes and copies fuzzed fields of fuzzed bytes. Field
// and Field64 read what Bit reads one bit at a time, SetField and
// SetField64 write the low bits of the value and no other bit, SetBit sets
// one bit, and Copy copies the bits that Bit reads from the source and no
// other bit.
func FuzzField(f *testing.F) {
	for _, s := range fieldSeeds() {
		f.Add(s.buf, s.start, s.width, s.v, s.dst)
	}
	f.Fuzz(func(t *testing.T, buf []byte, start uint16, width uint8, v uint64, dst uint16) {
		checkField(t, buf, start, width, v, dst)
	})
}
