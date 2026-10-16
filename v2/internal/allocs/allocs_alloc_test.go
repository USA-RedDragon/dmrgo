//go:build !tinygo

package allocs_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/bit"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/framer"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/packet"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
	"github.com/USA-RedDragon/dmrgo/v2/layer3"
	"github.com/USA-RedDragon/dmrgo/v2/vocoder"
)

const testdata = "../../layer2/testdata"

// captures returns the bursts of each capture in layer2/testdata, by name.
func captures(t *testing.T) map[string][]layer2.RawBurst {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(testdata, "*.bin"))
	if err != nil || len(files) == 0 {
		t.Fatalf("no captures: %v", err)
	}
	out := map[string][]layer2.RawBurst{}
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil || len(data)%33 != 0 {
			t.Fatalf("read %s: %v", f, err)
		}
		var bursts []layer2.RawBurst
		for i := 0; i < len(data); i += 33 {
			bursts = append(bursts, layer2.RawBurst(data[i:i+33]))
		}
		out[filepath.Base(f)] = bursts
	}
	return out
}

func assertZero(t *testing.T, name string, f func()) {
	t.Helper()
	f()
	if n := testing.AllocsPerRun(20, f); n != 0 {
		t.Errorf("%s: %.1f allocations per run, want 0", name, n)
	}
}

// TestAllocsDecodeEncodeCaptures decodes and encodes every burst of every capture
// with one reused Burst.
func TestAllocsDecodeEncodeCaptures(t *testing.T) {
	for name, bursts := range captures(t) {
		var b layer2.Burst
		assertZero(t, name, func() {
			for _, raw := range bursts {
				if b.DecodeFromBytes(raw) == nil {
					_, _ = b.Encode()
				}
			}
		})
	}
}

// TestAllocsPacketAssembly reassembles the packets of the SMS captures with a
// reused Burst and an Assembler with Reuse set.
func TestAllocsPacketAssembly(t *testing.T) {
	all := captures(t)
	for _, name := range []string{"d-sms.bin", "h-sms.bin", "m-sms.bin"} {
		bursts := all[name]
		var b layer2.Burst
		a := packet.Assembler{Reuse: true}
		assertZero(t, name, func() {
			for _, raw := range bursts {
				if b.DecodeFromBytes(raw) == nil {
					_, _, _ = a.AddBurst(&b)
				}
			}
		})
	}
}

// TestAllocsPacketRetry builds the retry of two blocks of a confirmed packet
// with a reused Builder.
func TestAllocsPacketRetry(t *testing.T) {
	var b packet.Builder
	h := pdu.DataHeader{Format: pdu.FormatConfirmed, ConfirmedDataHeader: &pdu.ConfirmedDataHeader{
		SAP: 4, LLIDDestination: 2, LLIDSource: 1, FullMessageFlag: true}}
	p, err := b.Segment(h, make([]byte, 100), enums.DataTypeRate12)
	if err != nil {
		t.Fatal(err)
	}
	var rd packet.ResponseData
	rd.Retry[2], rd.Retry[5] = true, true
	assertZero(t, "retry", func() {
		if r, err := b.Retry(&p, &rd); err != nil || len(r.Blocks) != 2 {
			t.Fatalf("retry: %v", err)
		}
	})
}

// TestAllocsVoiceSuperframe sends a superframe of the voice frames of the parrot
// capture with a VoiceTransmitter and receives it with a VoiceReceiver.
func TestAllocsVoiceSuperframe(t *testing.T) {
	var frames []vocoder.AMBEFrame
	var b layer2.Burst
	for _, raw := range captures(t)["parrot_kerchunk.bin"] {
		if b.DecodeFromBytes(raw) == nil && b.Kind == layer2.KindVoice {
			f, _ := b.AMBEFrames()
			frames = append(frames, f[:]...)
		}
	}
	if len(frames) < 3*layer2.VoiceBurstsPerSuperframe {
		t.Fatalf("parrot capture has %d frames", len(frames))
	}
	frames = frames[:3*layer2.VoiceBurstsPerSuperframe]
	lc, err := pdu.NewVoiceLC(true, 91, 3191868, layer3.ServiceOptions{})
	if err != nil {
		t.Fatal(err)
	}
	tx, err := layer2.NewVoiceTransmitter(&layer2.VoiceTransmitterOptions{LC: lc, ColorCode: 1, Silence: vocoder.AMBESilenceSlot()})
	if err != nil {
		t.Fatal(err)
	}
	out := make([]layer2.RawBurst, 0, 2*layer2.VoiceBurstsPerSuperframe)
	var rx layer2.VoiceReceiver
	var dst pdu.FullLinkControl
	var p pdu.FullLinkControlPayloads
	var rb layer2.Burst
	assertZero(t, "superframe", func() {
		out, _ = tx.AppendFrames(out[:0], frames...)
		for _, raw := range out {
			if rb.DecodeFromBytesWith(raw, &layer2.DecodeOptions{Kind: layer2.KindVoice}) == nil {
				_, _ = rx.ReceiveInto(&dst, &p, &rb)
			}
		}
	})
}

// TestAllocsVoiceTransmitterReset starts a new call on a reused
// VoiceTransmitter, with a PI header.
func TestAllocsVoiceTransmitterReset(t *testing.T) {
	lc, err := pdu.NewVoiceLC(true, 91, 3191868, layer3.ServiceOptions{})
	if err != nil {
		t.Fatal(err)
	}
	opts := &layer2.VoiceTransmitterOptions{LC: lc, ColorCode: 1, Silence: vocoder.AMBESilenceSlot(), PIHeader: &pdu.PIHeader{AlgorithmID: 1}}
	var tx layer2.VoiceTransmitter
	assertZero(t, "reset", func() {
		if err := tx.Reset(opts); err != nil {
			t.Fatal(err)
		}
	})
}

// TestAllocsVocoder decodes and encodes the AMBE+2 frames of the voice capture and
// packs and unpacks them as IPSC voice payloads.
func TestAllocsVocoder(t *testing.T) {
	var slots [][72]bit.Bit
	var b layer2.Burst
	for _, raw := range captures(t)["voice.bin"] {
		if b.DecodeFromBytes(raw) == nil && b.Kind == layer2.KindVoice {
			slots = append(slots, b.VoiceSlots[:]...)
		}
	}
	assertZero(t, "voice.bin", func() {
		for i := 0; i+3 <= len(slots); i += 3 {
			var frames [3]vocoder.AMBEFrame
			for j := range frames {
				frames[j], _ = vocoder.DecodeAMBE72(slots[i+j])
				_ = vocoder.EncodeAMBE72(frames[j])
			}
			payload := vocoder.PackAMBEVoice(frames)
			_ = vocoder.UnpackAMBEVoice(payload)
		}
	})
}

// TestAllocsFramer finds the bursts of the voice capture in its bit stream, fed in
// chunks of 100 bits.
func TestAllocsFramer(t *testing.T) {
	var stream []bit.Bit
	for _, raw := range captures(t)["voice.bin"] {
		stream = bit.AppendUnpack(stream, raw[:])
	}
	f := framer.New(framer.Config{})
	dst := make([]framer.Burst, 0, 16)
	assertZero(t, "voice.bin", func() {
		for i := 0; i < len(stream); i += 100 {
			dst = f.AppendBursts(dst[:0], stream[i:min(i+100, len(stream))]...)
		}
	})
}
