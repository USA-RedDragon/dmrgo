//go:build !tinygo

package zeroalloc_test

import (
	"encoding/hex"
	"errors"
	"slices"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/bit"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/vocoder"
	"github.com/USA-RedDragon/dmrgo/v2/vocoder/codec2"
)

// ipscSilence is the AMBE_DATA of voice burst A in DMRlink
// documents/voice_burst_decoding.txt: three AMBE+2 silence frames.
const ipscSilence = "f801a99f8ce0be006a67e3382f801a99f8ce08"

// voiceSlots returns the 72-bit vocoder slots of the voice bursts of
// layer2/testdata/voice.bin.
func voiceSlots(t testing.TB) [][72]bit.Bit {
	t.Helper()
	var slots [][72]bit.Bit
	for _, raw := range voiceBursts(t) {
		var b layer2.Burst
		if err := b.DecodeFromBytes(raw); err != nil {
			t.Fatal(err)
		}
		if b.Kind == layer2.KindData {
			continue
		}
		bits := bit.Unpack264(raw)
		var voice [216]bit.Bit
		copy(voice[:108], bits[:108])
		copy(voice[108:], bits[156:])
		for i := range 3 {
			slots = append(slots, [72]bit.Bit(voice[i*72:(i+1)*72]))
		}
	}
	if len(slots) == 0 {
		t.Fatal("no voice slots in voice.bin")
	}
	return slots
}

// vocoderEntries returns the entries of packages vocoder and vocoder/codec2.
func vocoderEntries(t testing.TB) []entry {
	t.Helper()
	slots := voiceSlots(t)
	frames := make([]vocoder.AMBEFrame, len(slots))
	for i, s := range slots {
		frames[i], _ = vocoder.DecodeAMBE72(s)
	}
	var ipsc [19]byte
	if _, err := hex.Decode(ipsc[:], []byte(ipscSilence)); err != nil {
		t.Fatal(err)
	}
	var i int
	var sink [72]bit.Bit
	var frame vocoder.AMBEFrame
	var payload [19]byte
	var voice [3]vocoder.AMBEFrame
	var bad [3]bool
	names := make([]string, 0, 8)
	out := slices.Concat([]entry{
		{"vocoder.DecodeAMBE72", "voice.bin slots", 0, func() { frame, _ = vocoder.DecodeAMBE72(slots[i%len(slots)]); i++ }},
		{"vocoder.EncodeAMBE72", "voice.bin frames", 0, func() { sink = vocoder.EncodeAMBE72(frames[i%len(frames)]); i++ }},
		{"vocoder.PackAMBEVoice", "IPSC silence", 0, func() { payload = vocoder.PackAMBEVoice(vocoder.UnpackAMBEVoice(ipsc)) }},
		{"vocoder.UnpackAMBEVoice", "IPSC silence", 0, func() { voice = vocoder.UnpackAMBEVoice(ipsc) }},
		{"vocoder.PackAMBEVoiceFlags", "IPSC silence", 0, func() {
			payload = vocoder.PackAMBEVoiceFlags(vocoder.UnpackAMBEVoice(ipsc), [3]bool{false, true, true})
		}},
		{"vocoder.UnpackAMBEVoiceFlags", "IPSC silence", 0, func() { voice, bad = vocoder.UnpackAMBEVoiceFlags(ipsc) }},
		{"vocoder.AMBESilence", "", 0, func() { frame = vocoder.AMBESilence() }},
		{"vocoder.AMBESilenceSlot", "", 0, func() { sink = vocoder.AMBESilenceSlot() }},
		{"vocoder.Names", "codec2 registered", 1, func() { names = vocoder.Names() }},
		{"vocoder.AppendNames", "codec2 registered", 0, func() { names = vocoder.AppendNames(names[:0]) }},
	}, codec2Entries(t))
	_, _, _, _, _, _ = sink, frame, payload, voice, bad, names
	return out
}

// codec2Entries returns the entries of package vocoder/codec2: the libcodec2
// codec when the build has it, and the stub otherwise.
func codec2Entries(t testing.TB) []entry {
	t.Helper()
	pcm := make([]int16, codec2.FrameSamples)
	for i := range pcm {
		pcm[i] = int16((i%40 - 20) * 800)
	}
	dst := make([]int16, 0, codec2.FrameSamples)
	c, err := codec2.New(codec2.Config{})
	name, decodeAllocs := "libcodec2", 1.0
	if errors.Is(err, codec2.ErrUnavailable) {
		name, decodeAllocs = "stub", 0
		c = &codec2.Codec{}
	} else if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Close() })
	slot := c.Silence()
	if name == "libcodec2" {
		slot = must(c.Encode(pcm))
	}
	var sink [72]bit.Bit
	var samples []int16
	var n int
	out := []entry{
		{"vocoder/codec2.Codec.FrameSamples", name, 0, func() { n = c.FrameSamples() }},
		{"vocoder/codec2.Codec.Mode", name, 0, func() { n = int(c.Mode()) }},
		{"vocoder/codec2.Codec.FID", name, 0, func() { n = int(c.FID()) }},
		{"vocoder/codec2.Codec.Silence", name, 0, func() { sink = c.Silence() }},
		{"vocoder/codec2.Codec.Encode", name, 0, func() { sink, _ = c.Encode(pcm) }},
		{"vocoder/codec2.Codec.Decode", name, decodeAllocs, func() { samples, _ = c.Decode(slot) }},
		{"vocoder/codec2.Codec.AppendDecode", name, 0, func() { samples, _ = c.AppendDecode(dst[:0], slot) }},
	}
	_, _, _ = sink, samples, n
	return out
}
