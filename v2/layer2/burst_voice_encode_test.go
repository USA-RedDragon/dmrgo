package layer2_test

import (
	"math/rand"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/internal/bits"
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
	t.Parallel()

	rng := rand.New(rand.NewSource(1))
	tx := must(layer2.NewVoiceTransmitter(&layer2.VoiceTransmitterOptions{LC: groupLC(enums.FIDStandardized), ColorCode: 1, Source: layer2.SyncSourceMS, Silence: silenceSlot()}))
	changed := 0
	for n := range 600 {
		var slot [9]byte
		for i := range 72 {
			bits.SetBit(slot[:], i, byte(rng.Intn(2)))
		}
		bursts := must(tx.AddSlots(slot))
		for _, raw := range bursts {
			b := must(layer2.NewBurstFromBytes(raw))
			if got := must(b.Encode()); got != raw {
				t.Fatalf("slot %d: decode then Encode changed the burst\ngot  %x\nwant %x", n, got, raw)
			}
			frames, _ := b.AMBEFrames()
			for i, f := range frames {
				if vocoder.EncodeAMBE72(f) != b.VoiceSlots[i] {
					changed++
				}
			}
		}
	}
	if changed == 0 {
		t.Fatal("no random slot differs from its AMBE+2 re-encode; the test proves nothing")
	}
}

func noisyAMBEBurst(t *testing.T) (clean, noisy [33]byte, info [3]vocoder.AMBEFrame) {
	t.Helper()
	tx := must(layer2.NewVoiceTransmitter(&layer2.VoiceTransmitterOptions{LC: groupLC(enums.FIDStandardized), ColorCode: 1, Source: layer2.SyncSourceMS, Silence: silenceSlot()}))
	for i := range info {
		for j := range 49 {
			bits.SetBit(info[i][:], j, byte((i+j*5)>>1&1))
		}
	}
	clean = must(tx.AddFrames(info[:]...))[0]
	noisy = flipBurstBits(clean, voiceBurstBit(0, 0), voiceBurstBit(1, 0), voiceBurstBit(2, 0))
	return clean, noisy, info
}

func TestBurstEncode_AMBEWithCorrectedErrors(t *testing.T) {
	t.Parallel()

	clean, noisy, info := noisyAMBEBurst(t)
	b := must(layer2.NewBurstFromBytes(noisy))
	frames, res := b.AMBEFrames()
	if frames != info || res.ErrorsCorrected != 3 || res.Uncorrectable {
		t.Fatalf("AMBEFrames = %v %+v, want the sent frames with 3 corrected bits", frames, res)
	}
	if got := must(b.Encode()); got != noisy {
		t.Errorf("decode then Encode: got %x, want the received %x", got, noisy)
	}
	b.SetAMBEFrames(frames)
	if got := must(b.Encode()); got != clean {
		t.Errorf("SetAMBEFrames: got %x, want the regenerated %x", got, clean)
	}
	if b.VoiceSlots != must(layer2.NewBurstFromBytes(clean)).VoiceSlots {
		t.Error("VoiceSlots after SetAMBEFrames are not the clean slots")
	}
}
