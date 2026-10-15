package trellis34

import (
	"math/rand"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/bit"
)

// tableB7 returns ETSI TS 102 361-1 Table B.7, indexed by [FSM state][input tribit].
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

// tableB8 returns ETSI TS 102 361-1 Table B.8, constellation point to dibit pair.
func tableB8() [16][2]int8 {
	return [16][2]int8{
		{1, -1}, {-1, -1}, {3, -3}, {-3, -3}, {-3, -1}, {3, -1}, {-1, -3}, {1, -3},
		{-3, 3}, {3, 3}, {-1, 1}, {1, 1}, {1, 3}, {-1, 3}, {3, 1}, {-3, 1},
	}
}

func TestTrellis34_EncoderTablesMatchSpec(t *testing.T) {
	b7 := tableB7()
	for s := range b7 {
		for in := range b7[s] {
			if got := encoder_state_transition_table[s*8+in]; got != b7[s][in] {
				t.Errorf("state %d input %d: point %d, want %d", s, in, got, b7[s][in])
			}
		}
	}
	if trellis34_reverse_constellation != tableB8() {
		t.Errorf("constellation table %v, want %v", trellis34_reverse_constellation, tableB8())
	}
}

func randomTrellisBlock(r *rand.Rand) ([144]bit.Bit, [196]bit.Bit) {
	var data [144]bit.Bit
	for i := range data {
		data[i] = bit.Bit(r.Intn(2))
	}
	return data, New().Encode(data)
}

func TestTrellis34_CleanDecode(t *testing.T) {
	r := rand.New(rand.NewSource(1))
	for n := 0; n < 200; n++ {
		data, enc := randomTrellisBlock(r)
		got, res := New().Decode(enc)
		if got != data || res.ErrorsCorrected != 0 || res.Uncorrectable {
			t.Fatalf("clean block: data match=%v result=%+v", got == data, res)
		}
	}
}

func TestTrellis34_SingleBitErrorsCorrected(t *testing.T) {
	r := rand.New(rand.NewSource(2))
	tr := New()
	for n := 0; n < 20; n++ {
		data, enc := randomTrellisBlock(r)
		for pos := range enc {
			rx := enc
			rx[pos] ^= 1
			got, res := tr.Decode(rx)
			if got != data || res.ErrorsCorrected != 1 || res.Uncorrectable {
				t.Fatalf("bit %d: data match=%v result=%+v", pos, got == data, res)
			}
		}
	}
}

// The code has a free distance of 3 under the bit Hamming metric, so two bit
// errors are only guaranteed to be corrected when they fall in constellation
// points far enough apart (after deinterleaving) to be separate error events.
func TestTrellis34_SeparatedDoubleBitErrorsCorrected(t *testing.T) {
	var pointOf [196]int
	for i, j := range interleave_matrix {
		pointOf[2*i] = int(j) / 2
		pointOf[2*i+1] = int(j) / 2
	}
	r := rand.New(rand.NewSource(4))
	tr := New()
	for n := 0; n < 20000; n++ {
		data, enc := randomTrellisBlock(r)
		a, b := r.Intn(196), r.Intn(196)
		if d := pointOf[a] - pointOf[b]; d > -6 && d < 6 {
			continue
		}
		rx := enc
		rx[a] ^= 1
		rx[b] ^= 1
		got, res := tr.Decode(rx)
		if got != data || res.ErrorsCorrected != 2 || res.Uncorrectable {
			t.Fatalf("bits %d,%d: data match=%v result=%+v", a, b, got == data, res)
		}
	}
}

func TestTrellis34_NonzeroFinalStateFlagged(t *testing.T) {
	r := rand.New(rand.NewSource(5))
	tr := New()
	for n := 0; n < 200; n++ {
		var tribits [49]byte
		for i := range tribits {
			tribits[i] = byte(r.Intn(8))
		}
		tribits[48] = byte(r.Intn(7) + 1)

		var dibits [98]int8
		state := byte(0)
		for i, tb := range tribits {
			point := encoder_state_transition_table[state*8+tb]
			dibits[2*i] = trellis34_reverse_constellation[point][0]
			dibits[2*i+1] = trellis34_reverse_constellation[point][1]
			state = tb
		}
		var interleaved [98]int8
		for i := range interleaved {
			interleaved[i] = dibits[interleave_matrix[i]]
		}

		if _, res := tr.Decode(tr.dibitsToBits(interleaved)); !res.Uncorrectable {
			t.Fatalf("block ending in state %d not flagged: %+v", tribits[48], res)
		}
	}
}
