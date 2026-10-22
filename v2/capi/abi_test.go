package main

import (
	"testing"
)

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
	f.Add([]byte{0x75, 0x5f, 0xd7, 0xdf, 0x75, 0xf7}, []byte{1}, uint8(1))
	f.Add([]byte{0xd5, 0xd7, 0xf7, 0x7f, 0xd7, 0x57, 0x00}, []byte{3, 2}, uint8(2))
	f.Fuzz(func(t *testing.T, stream, cuts []byte, period uint8) {
		h := newHarness(0)
		defer h.free()
		if _, err := framerRun(h, stream, cuts, period); err != nil {
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
