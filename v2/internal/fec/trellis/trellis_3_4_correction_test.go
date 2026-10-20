package trellis

import (
	"math/rand"
	"testing"

	reftrellis "github.com/USA-RedDragon/dmrgo/v2/internal/fecref/trellis"
	"github.com/USA-RedDragon/dmrgo/v2/internal/unpacked"
)

// ETSI TS 102 361-1 Table B.7
func tableB7() [8][8]byte {
	return [8][8]byte{
		{0, 8, 4, 12, 2, 10, 6, 14},
		{4, 12, 2, 10, 6, 14, 0, 8},
		{1, 9, 5, 13, 3, 11, 7, 15},
		{5, 13, 3, 11, 7, 15, 1, 9},
		{3, 11, 7, 15, 1, 9, 5, 13},
		{7, 15, 1, 9, 5, 13, 3, 11},
		{2, 10, 6, 14, 0, 8, 4, 12},
		{6, 14, 0, 8, 4, 12, 2, 10},
	}
}

// ETSI TS 102 361-1 Table B.8
func tableB8() [16][2]int8 {
	return [16][2]int8{
		{1, -1}, {-1, -1}, {3, -3}, {-3, -3}, {-3, -1}, {3, -1}, {-1, -3}, {1, -3},
		{-3, 3}, {3, 3}, {-1, 1}, {1, 1}, {1, 3}, {-1, 3}, {3, 1}, {-3, 1},
	}
}

// ETSI TS 102 361-1 Table 10.3
func symbolBits(d int8) byte {
	return map[int8]byte{3: 0b01, 1: 0b00, -1: 0b10, -3: 0b11}[d]
}

func level(d byte) int {
	return [4]int{1, 3, -1, -3}[d&3]
}

func TestTrellis34_TablesMatchSpec(t *testing.T) {
	t.Parallel()

	b7, b8 := tableB7(), tableB8()
	for s := range b7 {
		for in := range b7[s] {
			if got := encoder_state_transition_table[s*8+in]; got != b7[s][in] {
				t.Errorf("state %d input %d: point %d, want %d", s, in, got, b7[s][in])
			}
			pair := b8[b7[s][in]]
			if got, want := trellis34BranchDibits[s*8+in], symbolBits(pair[0])<<2|symbolBits(pair[1]); got != want {
				t.Errorf("state %d input %d: dibits %04b, want %04b", s, in, got, want)
			}
		}
	}
	if trellis34_reverse_constellation != b8 {
		t.Errorf("constellation table %v, want %v", trellis34_reverse_constellation, b8)
	}
}

func randomBlock(r *rand.Rand) ([18]byte, [25]byte) {
	var data [18]byte
	r.Read(data[:])
	return data, Encode(data)
}

func dibitAt(info *[25]byte, i int) byte {
	return info[i/4] >> (6 - 2*(i%4)) & 3
}

func setDibit(info *[25]byte, i int, d byte) {
	sh := 6 - 2*(i%4)
	info[i/4] = info[i/4]&^(3<<sh) | d<<sh
}

func cost(rx, sent *[25]byte) int {
	stepCost := [4]int{0, 7, 10, 9}
	total := 0
	for i := range 98 {
		d := level(dibitAt(rx, i)) - level(dibitAt(sent, i))
		if d < 0 {
			d = -d
		}
		total += stepCost[d/2]
	}
	return total
}

func bitErrors(a, b *[25]byte) int {
	n := 0
	for i := range a {
		x := a[i] ^ b[i]
		for ; x != 0; x &= x - 1 {
			n++
		}
	}
	return n
}

func TestTrellis34_EncodeMatchesReference(t *testing.T) {
	t.Parallel()

	r := rand.New(rand.NewSource(1))
	for range 20000 {
		data, got := randomBlock(r)
		var in [144]unpacked.Bit
		copy(in[:], unpacked.Unpack(data[:]))
		want := reftrellis.Encode(in)
		if [25]byte(unpacked.Pack(want[:])) != got {
			t.Fatalf("Encode(%x) = %x, want %x", data, got, unpacked.Pack(want[:]))
		}
	}
}

func TestTrellis34_CleanDecode(t *testing.T) {
	t.Parallel()

	r := rand.New(rand.NewSource(2))
	for range 2000 {
		data, enc := randomBlock(r)
		got, res := Decode(enc)
		if got != data || res.ErrorsCorrected != 0 || res.Uncorrectable || res.BitsChecked != 196 {
			t.Fatalf("clean block %x: got %x, result %+v", data, got, res)
		}
	}
}

func TestTrellis34_SingleOneStepErrorsCorrected(t *testing.T) {
	t.Parallel()

	r := rand.New(rand.NewSource(3))
	for range 200 {
		data, enc := randomBlock(r)
		for i := range 98 {
			for d := range byte(4) {
				diff := level(d) - level(dibitAt(&enc, i))
				if diff != 2 && diff != -2 {
					continue
				}
				rx := enc
				setDibit(&rx, i, d)
				got, res := Decode(rx)
				if got != data || res.ErrorsCorrected != 1 || res.Uncorrectable {
					t.Fatalf("dibit %d set to %02b: got %x, want %x, result %+v", i, d, got, data, res)
				}
			}
		}
	}
}

func TestTrellis34_MaximumLikelihood(t *testing.T) {
	t.Parallel()

	r := rand.New(rand.NewSource(4))
	for n := range 200000 {
		data, enc := randomBlock(r)
		rx := enc
		for range 1 + n%8 {
			setDibit(&rx, r.Intn(98), byte(r.Intn(4)))
		}
		got, res := Decode(rx)
		dec := Encode(got)
		if res.ErrorsCorrected != bitErrors(&rx, &dec) {
			t.Fatalf("ErrorsCorrected %d, want %d bits between received and decoded block", res.ErrorsCorrected, bitErrors(&rx, &dec))
		}
		mDec, mSent := cost(&rx, &dec), cost(&rx, &enc)
		switch {
		case mDec > mSent:
			t.Fatalf("block %x: decoded metric %d is above the sent block's %d", data, mDec, mSent)
		case got != data && mDec == mSent && !res.Uncorrectable:
			t.Fatalf("block %x: decoded block %x has the sent block's metric %d but no Uncorrectable", data, got, mDec)
		}
	}
}

func TestTrellis34_SingleDibitErrors(t *testing.T) {
	t.Parallel()

	r := rand.New(rand.NewSource(5))
	var ok, flagged, closer int
	for range 300 {
		data, enc := randomBlock(r)
		for i := range 98 {
			for d := range byte(4) {
				if d == dibitAt(&enc, i) {
					continue
				}
				rx := enc
				setDibit(&rx, i, d)
				got, res := Decode(rx)
				dec := Encode(got)
				switch {
				case got == data && !res.Uncorrectable:
					ok++
				case res.Uncorrectable:
					flagged++
				case cost(&rx, &dec) < cost(&rx, &enc):
					closer++
				default:
					t.Fatalf("dibit %d set to %02b: got %x, want %x, result %+v", i, d, got, data, res)
				}
			}
		}
	}
	t.Logf("single dibit errors: %d corrected, %d flagged, %d decoded to a closer block", ok, flagged, closer)
}

func TestTrellis34_TwoDibitDistance(t *testing.T) {
	t.Parallel()

	var a, b [18]byte
	b[0] = 0b010_00000
	ea, eb := Encode(a), Encode(b)
	diff := 0
	for i := range 98 {
		if dibitAt(&ea, i) != dibitAt(&eb, i) {
			diff++
		}
	}
	if diff != 2 {
		t.Fatalf("blocks differ in %d dibits, want 2", diff)
	}
}

func TestTrellis34_NonzeroFinalStateFlagged(t *testing.T) {
	t.Parallel()

	r := rand.New(rand.NewSource(6))
	for range 200 {
		var tribits [49]byte
		for i := range tribits {
			tribits[i] = byte(r.Intn(8))
		}
		tribits[48] = byte(r.Intn(7) + 1)

		var d [98]byte
		state := byte(0)
		for i, tb := range tribits {
			pair := trellis34BranchDibits[state*8+tb]
			d[2*i], d[2*i+1] = pair>>2, pair&3
			state = tb
		}
		var info [25]byte
		for i, j := range interleave_matrix {
			setDibit(&info, i, d[j])
		}

		if _, res := Decode(info); !res.Uncorrectable {
			t.Fatalf("block ending in state %d not flagged: %+v", tribits[48], res)
		}
	}
}
