// Package trellis encodes and decodes the rate 3/4 trellis code that protects
// rate 3/4 data blocks. A block is 196 information bits packed most
// significant bit first in 25 bytes, and carries 144 data bits packed in 18
// bytes. (ETSI TS 102 361-1 clause B.2.4)
package trellis

import (
	mathbits "math/bits"

	"github.com/USA-RedDragon/dmrgo/v2/fec"
)

const (
	tribits = 49
	dibits  = 98
	states  = 8

	lanes = 0x0101010101010101
	high  = 0x8080808080808080
)

func deinterleave(info *[25]byte) [tribits]byte {
	var d [dibits]byte
	i := 0
	for r := 0; r < 8; r += 2 {
		for j := r; j < dibits; j += 8 {
			b := info[i>>2] >> (4 - 2*(i&3))
			d[j], d[j+1] = b>>2&3, b&3
			i += 2
		}
	}
	var pairs [tribits]byte
	for k := range pairs {
		pairs[k] = d[2*k]<<2 | d[2*k+1]
	}
	return pairs
}

const branchSequences = 0xc30f965a4b871ed2

func branchDibits(state, tribit uint32) byte {
	shift := state&1*2 + state&4
	seq := (state>>1 ^ state>>2) & 1
	return byte(uint64(branchSequences) >> (32*seq + 4*((tribit+shift)&7)) & 0xF)
}

func less(a, b uint64) uint64 {
	return (^((a | high) - b) & high) >> 7 * 0xFF
}

// Decode decodes 196 received bits with a maximum likelihood Viterbi decoder
// over the 8-state trellis, whose path starts and ends in state 0, and
// returns the 144 data bits. The branch metric is the cost of each received
// dibit against the sent one, by the number of 4FSK deviation steps between
// them (see Trellis34BranchMetric in cmd/tablegen).
//
// ErrorsCorrected counts the received bits that differ from the decoded
// block. Uncorrectable is set only when another path that ends in state 0
// has the same metric, so that the decoded block is one of two equally
// likely blocks. A path that ends in another state and has a lower metric
// is no block, so it does not flag the decode, as MMDVMHost CDMRTrellis
// does not. A block with more errors than the code corrects can still
// decode to a wrong block without Uncorrectable, so callers check the block
// CRC-9 or the message CRC-32.
func Decode(info [25]byte) ([18]byte, fec.Result) {
	rx := deinterleave(&info)

	metric := trellis34BranchMetric[rx[0]][0]
	var tied uint8
	var prev [tribits]uint32
	for k := 1; k < tribits; k++ {
		bm := &trellis34BranchMetric[rx[k]]
		best := metric&0xFF*lanes + bm[0]
		var from, tie uint64
		for s := uint64(1); s < states; s++ {
			m := metric>>(8*s)&0xFF*lanes + bm[s]
			lt, gt := less(m, best), less(best, m)
			tie = tie&^lt | ^(lt | gt)
			best ^= (best ^ m) & lt
			from ^= (from ^ s*lanes) & lt
		}

		var nextTied uint8
		for t := range states {
			if tie>>(8*t)&1 != 0 || tied>>(from>>(8*t)&7)&1 != 0 {
				nextTied |= 1 << t
			}
		}
		least := best
		for sh := 8; sh < 64; sh <<= 1 {
			least ^= (least ^ least>>sh) & less(least>>sh, least)
		}
		metric = best - least&0xFF*lanes
		tied = nextTied

		from = (from | from>>5) & 0x003F003F003F003F
		from = (from | from>>10) & 0x00000FFF00000FFF
		prev[k] = uint32((from | from>>20&0xFFF000) & 0xFFFFFF)
	}

	result := fec.Result{BitsChecked: 196, Uncorrectable: tied&1 != 0}

	var out [18]byte
	state := uint32(0)
	for k := tribits - 1; k >= 0; k-- {
		from := prev[k] >> (3 * state) & 7
		result.ErrorsCorrected += mathbits.OnesCount8(rx[k] ^ branchDibits(from, state))
		if k < tribits-1 {
			bit := 3 * k
			v := uint16(state) << (13 - bit&7)
			out[bit>>3] |= byte(v >> 8)
			if bit>>3+1 < len(out) {
				out[bit>>3+1] |= byte(v & 0xFF)
			}
		}
		state = from
	}
	return out, result
}

// Encode encodes 144 data bits with the rate 3/4 trellis code and a tail
// tribit of 0 into 196 interleaved bits.
func Encode(data [18]byte) [25]byte {
	var d [dibits]byte
	state := uint32(0)
	for k := range tribits {
		var in uint32
		if k < tribits-1 {
			bit := 3 * k
			v := uint32(data[bit>>3]) << 8
			if bit>>3+1 < len(data) {
				v |= uint32(data[bit>>3+1])
			}
			in = v >> (13 - bit&7) & 7
		}
		pair := branchDibits(state, in)
		d[2*k], d[2*k+1] = pair>>2, pair&3
		state = in
	}
	var info [25]byte
	i := 0
	for r := 0; r < 8; r += 2 {
		for j := r; j < dibits; j += 8 {
			info[i>>2] |= (d[j]<<2 | d[j+1]) << (4 - 2*(i&3))
			i += 2
		}
	}
	return info
}
