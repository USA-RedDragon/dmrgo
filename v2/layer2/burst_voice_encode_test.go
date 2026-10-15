package layer2_test

import (
	"math/rand"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/bit"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/fec"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/vocoder"
)

// voiceBurstBit returns the burst bit of bit j of voice slot i.
func voiceBurstBit(i, j int) int {
	v := i*72 + j
	if v < 108 {
		return v
	}
	return v + 48
}

// Slots of another codec, here random bits, are not AMBE+2 codewords. The
// AMBE+2 decoder changes them, so decode then Encode must keep the slots as
// received.
func TestBurstEncode_KeepsNonAMBEVoiceSlots(t *testing.T) {
	rng := rand.New(rand.NewSource(1)) //nolint:gosec // pseudo-random adequate for tests
	tx := must(layer2.NewVoiceTransmitter(groupLC(enums.StandardizedFID), 1, layer2.SyncSourceMS, silenceSlot()))
	changed := 0
	for n := range 600 {
		var slot [72]bit.Bit
		for i := range slot {
			slot[i] = bit.Bit(rng.Intn(2)) //nolint:gosec // 0 or 1
		}
		bursts := must(tx.AddSlots(slot))
		for _, raw := range bursts {
			b := must(layer2.NewBurstFromBytes(raw))
			if got := must(b.Encode()); got != raw {
				t.Fatalf("slot %d: decode then Encode changed the burst\ngot  %x\nwant %x", n, got, raw)
			}
			for i, f := range b.VoiceData.Frames {
				if vocoder.EncodeAMBE72(f.DecodedBits) != b.VoiceSlots()[i] {
					changed++
				}
			}
		}
	}
	if changed == 0 {
		t.Fatal("no random slot differs from its AMBE+2 re-encode; the test proves nothing")
	}
}

func noisyAMBEBurst(t *testing.T) (clean, noisy [33]byte) {
	t.Helper()
	tx := must(layer2.NewVoiceTransmitter(groupLC(enums.StandardizedFID), 1, layer2.SyncSourceMS, silenceSlot()))
	var info [3][49]bit.Bit
	for i := range info {
		for j := range info[i] {
			info[i][j] = bit.Bit((i + j*5) >> 1 & 1)
		}
	}
	clean = must(tx.AddFrames(info[:]...))[0]
	noisy = flipBurstBits(clean, voiceBurstBit(0, 0), voiceBurstBit(1, 0), voiceBurstBit(2, 0))
	b := must(layer2.NewBurstFromBytes(noisy))
	for i, f := range b.VoiceData.Frames {
		if f.FEC.ErrorsCorrected != 1 || f.FEC.Uncorrectable || f.DecodedBits != info[i] {
			t.Fatalf("frame %d: FEC %+v; the flipped bit is not one the frame FEC corrects", i, f.FEC)
		}
	}
	return clean, noisy
}

func TestBurstEncode_AMBEWithCorrectedErrors(t *testing.T) {
	clean, noisy := noisyAMBEBurst(t)
	b := must(layer2.NewBurstFromBytes(noisy))
	if got := must(b.Encode()); got != noisy {
		t.Errorf("unchanged VoiceData: got %x, want the received %x", got, noisy)
	}

	regen := layer2.Burst{
		SyncPattern: b.SyncPattern,
		VoiceData:   b.VoiceData,
	}
	if got := must(regen.Encode()); got != clean {
		t.Errorf("VoiceData in a new Burst: got %x, want the regenerated %x", got, clean)
	}
	if regen.VoiceSlots() != must(layer2.NewBurstFromBytes(clean)).VoiceSlots() {
		t.Error("VoiceSlots of a hand-built burst are not the slots Encode writes")
	}

	b.VoiceData.Frames[1].FEC = fec.FECResult{}
	if got := must(b.Encode()); got != noisy {
		t.Error("a frame whose FEC changed but whose bits did not is re-coded")
	}

	b.VoiceData.Frames[0].DecodedBits[48] ^= 1
	got := must(layer2.NewBurstFromBytes(must(b.Encode())))
	slots := got.VoiceSlots()
	noisySlots := must(layer2.NewBurstFromBytes(noisy)).VoiceSlots()
	if slots[0] != vocoder.EncodeAMBE72(b.VoiceData.Frames[0].DecodedBits) {
		t.Error("edited frame 0 is not regenerated")
	}
	if slots[1] != noisySlots[1] || slots[2] != noisySlots[2] {
		t.Error("unedited frames 1 and 2 are not kept as received")
	}
	if b.VoiceSlots() != slots {
		t.Error("VoiceSlots of an edited burst are not the slots Encode writes")
	}
}
