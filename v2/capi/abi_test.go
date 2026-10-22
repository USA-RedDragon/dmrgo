package main

import (
	"bytes"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

const (
	maxScript   = 128
	maxCuts     = 128
	maxStream   = 4096
	maxBitFlips = 64
)

func fuzzStream(pool [][33]byte, script, flips []byte) []byte {
	script = script[:min(len(script), maxScript)]
	var s []byte
	for i := 0; i < len(script) && len(s) < maxStream; {
		op := script[i]
		i++
		n := int(op&0x3f) + 1
		switch op >> 6 {
		case 0:
			end := min(i+n, len(script))
			s = append(s, script[i:end]...)
			i = end
		case 1:
			if i+2 > len(script) {
				i = len(script)
				break
			}
			count := (int(op&0x3f)<<8 | int(script[i])) + 1
			s = append(s, bytes.Repeat([]byte{script[i+1]}, min(count, maxStream-len(s)))...)
			i += 2
		case 2:
			if i >= len(script) {
				break
			}
			r := int(script[i]) + 1
			i++
			d := min(n, len(s))
			for k := 0; k < r && d > 0 && len(s) < maxStream; k++ {
				s = append(s, s[len(s)-d:]...)
			}
		case 3:
			if len(pool) > 0 {
				b := pool[int(op&0x3f)%len(pool)]
				s = append(s, b[:]...)
			}
		}
	}
	s = s[:min(len(s), maxStream)]
	flips = flips[:min(len(flips), maxBitFlips)]
	for k := 0; k+1 < len(flips) && len(s) > 0; k += 2 {
		i := (int(flips[k])<<8 | int(flips[k+1])) % (8 * len(s))
		s[i/8] ^= 0x80 >> (i % 8)
	}
	return s
}

func abiSeeds(t *testing.T, f func(seed uint64) error) {
	t.Helper()
	for seed := range uint64(4) {
		if err := f(seed); err != nil {
			t.Fatalf("seed %d: %v", seed, err)
		}
	}
}

func TestABIVoice(t *testing.T) {
	t.Parallel()
	abiSeeds(t, voiceABI)
}

func TestABIVoiceTransmitter(t *testing.T) {
	t.Parallel()
	abiSeeds(t, layer2txABI)
}

func TestABIFramer(t *testing.T) {
	t.Parallel()
	abiSeeds(t, framerABI)
}

func TestABITalkerAlias(t *testing.T) {
	t.Parallel()
	abiSeeds(t, talkerAliasABI)
}

func TestABICodec2(t *testing.T) {
	t.Parallel()
	abiSeeds(t, codec2ABI)
}

func TestABIPacket(t *testing.T) {
	t.Parallel()
	abiSeeds(t, packetABI)
}

func TestABIRepeater(t *testing.T) {
	t.Parallel()
	abiSeeds(t, bsABI)
}

func TestABICSBK(t *testing.T) {
	t.Parallel()
	abiSeeds(t, csbkABI)
}

func TestABITrunk(t *testing.T) {
	t.Parallel()
	most, err := trunkABI(1, 16, 28)
	if err != nil {
		t.Fatal(err)
	}
	if most <= 10 {
		t.Errorf("at most %d TSCC outputs per Tick; the test needs more than trunk.MaxOutputs", most)
	}
	if _, err := trunkABI(2, 3, 5); err != nil {
		t.Fatal(err)
	}
}

func FuzzABIFramer(f *testing.F) {
	b, err := layer2.BuildDataBurst(pdu.PRFillBits(), enums.DataTypeIdle, 1, layer2.SyncSourceBS)
	if err != nil {
		f.Fatal(err)
	}
	pool := [][33]byte{b}
	f.Add([]byte{0x05, 0x75, 0x5f, 0xd7, 0xdf, 0x75, 0xf7}, []byte{}, []byte{1}, uint8(1))
	f.Add([]byte{0x06, 0xd5, 0xd7, 0xf7, 0x7f, 0xd7, 0x57, 0x00}, []byte{}, []byte{3, 2}, uint8(2))
	f.Fuzz(func(t *testing.T, script, flips, cuts []byte, period uint8) {
		h := newHarness(0)
		defer h.free()
		if _, err := framerRun(h, fuzzStream(pool, script, flips), cuts[:min(len(cuts), maxCuts)], period); err != nil {
			t.Fatal(err)
		}
		if err := h.recheck(); err != nil {
			t.Fatal(err)
		}
	})
}

func FuzzABI(f *testing.F) {
	for k := range uint8(10) {
		f.Add(uint64(k), k)
	}
	families := []func(uint64) error{voiceABI, layer2txABI, framerABI, talkerAliasABI, codec2ABI, packetABI, bsABI, csbkABI}
	f.Fuzz(func(t *testing.T, seed uint64, family uint8) {
		if err := families[int(family)%len(families)](seed); err != nil {
			t.Fatal(err)
		}
	})
}
