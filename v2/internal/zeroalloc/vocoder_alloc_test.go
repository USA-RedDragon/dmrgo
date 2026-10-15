package zeroalloc_test

import (
	"encoding/hex"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/bit"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/vocoder"
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

// vocoderEntries returns the entries of package vocoder.
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
	names := make([]string, 0, 8)
	out := []entry{
		{"vocoder.DecodeAMBE72", "voice.bin slots", 0, func() { frame, _ = vocoder.DecodeAMBE72(slots[i%len(slots)]); i++ }},
		{"vocoder.EncodeAMBE72", "voice.bin frames", 0, func() { sink = vocoder.EncodeAMBE72(frames[i%len(frames)]); i++ }},
		{"vocoder.PackAMBEVoice", "IPSC silence", 0, func() { payload = vocoder.PackAMBEVoice(vocoder.UnpackAMBEVoice(ipsc)) }},
		{"vocoder.UnpackAMBEVoice", "IPSC silence", 0, func() { voice = vocoder.UnpackAMBEVoice(ipsc) }},
		{"vocoder.AMBESilence", "", 0, func() { frame = vocoder.AMBESilence() }},
		{"vocoder.AMBESilenceSlot", "", 0, func() { sink = vocoder.AMBESilenceSlot() }},
		{"vocoder.Names", "empty registry", 0, func() { names = vocoder.Names() }},
		{"vocoder.AppendNames", "empty registry", 0, func() { names = vocoder.AppendNames(names[:0]) }},
	}
	_, _, _, _, _ = sink, frame, payload, voice, names
	return out
}
