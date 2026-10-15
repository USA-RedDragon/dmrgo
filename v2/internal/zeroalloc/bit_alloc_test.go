//go:build !tinygo

package zeroalloc_test

import (
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/bit"
	"github.com/USA-RedDragon/dmrgo/v2/internal/lowbit"
)

// bitEntries returns the entries of package bit and internal/lowbit, with
// the bursts of layer2/testdata/voice.bin.
func bitEntries(t testing.TB) []entry {
	t.Helper()
	voice := captureRawBursts(t, "voice")
	header, voiceB := voice[0], voice[2]
	bits264 := bit.Unpack264(header)
	if bit.Pack264(bits264) != header {
		t.Fatal("voice LC header does not pack back to itself")
	}
	emb := [4]byte(voiceB[14:18])
	bits32 := bit.Unpack32(emb)
	lc := [12]byte(header[:12])
	bits96 := bit.Unpack96(lc)
	soft := bits264
	for i := range soft {
		soft[i] |= 0xFE
	}
	bytes := make([]byte, 0, len(header))
	bits := make([]bit.Bit, 0, len(bits264))
	var (
		out264 [264]bit.Bit
		out33  [33]byte
		out4   [4]byte
		out12  [12]byte
		u64    uint64
	)
	out := []entry{
		{"bit.Uint", "voice LC header", 0, func() { u64 = bit.Uint(bits264[:48]) }},
		{"bit.PutUint", "voice LC header", 0, func() { bit.PutUint(out264[:48], u64) }},
		{"bit.Pack", "voice LC header, fresh slice", 1, func() { out33 = [33]byte(bit.Pack(bits264[:])) }},
		{"bit.AppendPack", "voice LC header", 0, func() { bytes = bit.AppendPack(bytes[:0], bits264[:]) }},
		{"bit.Unpack", "voice LC header, fresh slice", 1, func() { out264 = [264]bit.Bit(bit.Unpack(header[:])) }},
		{"bit.AppendUnpack", "voice LC header", 0, func() { bits = bit.AppendUnpack(bits[:0], header[:]) }},
		{"bit.Unpack32", "voice B embedded signalling", 0, func() { bits32 = bit.Unpack32(emb) }},
		{"bit.Pack32", "voice B embedded signalling", 0, func() { out4 = bit.Pack32(bits32) }},
		{"bit.Unpack96", "voice LC header", 0, func() { bits96 = bit.Unpack96(lc) }},
		{"bit.Pack96", "voice LC header", 0, func() { out12 = bit.Pack96(bits96) }},
		{"bit.Unpack264", "voice LC header", 0, func() { out264 = bit.Unpack264(header) }},
		{"bit.Pack264", "voice LC header", 0, func() { out33 = bit.Pack264(bits264) }},
		{"internal/lowbit.Mask", "soft bits", 0, func() { lowbit.Mask(soft[:]) }},
	}
	_, _, _, _, _ = out264, out33, out4, out12, u64
	return out
}
