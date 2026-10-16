package vocoder_test

import (
	"encoding/hex"
	"math/rand"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/internal/bits"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/vocoder"
)

// ipscSilence is the AMBE_DATA of voice burst A in DMRlink
// documents/voice_burst_decoding.txt: three AMBE+2 silence frames.
const ipscSilence = "f801a99f8ce0be006a67e3382f801a99f8ce08"

func TestAMBEVoice_IPSCCapture(t *testing.T) {
	t.Parallel()

	raw, err := hex.DecodeString(ipscSilence)
	if err != nil {
		t.Fatal(err)
	}
	var data [19]byte
	copy(data[:], raw)
	frames := vocoder.UnpackAMBEVoice(data)
	silence := vocoder.AMBEFrame{0xF8, 0x01, 0xA9, 0x9F, 0x8C, 0xE0, 0x80}
	for k, f := range frames {
		if f != silence {
			t.Errorf("frame %d: % X, want the AMBE+2 silence frame", k, f)
		}
	}
	if got := vocoder.PackAMBEVoice(frames); got != data {
		t.Errorf("pack % X, want % X", got, data)
	}
}

func TestAMBEVoice_RoundTrip(t *testing.T) {
	t.Parallel()

	r := rand.New(rand.NewSource(4))
	for n := 0; n < 1000; n++ {
		var frames [3]vocoder.AMBEFrame
		for k := range frames {
			for i := range 49 {
				bits.SetBit(frames[k][:], i, byte(r.Intn(2)))
			}
		}
		data := vocoder.PackAMBEVoice(frames)
		for _, i := range []int{49, 99, 149, 150, 151} {
			if bits.Bit(data[:], i) != 0 {
				t.Fatalf("bit %d is set", i)
			}
		}
		if got := vocoder.UnpackAMBEVoice(data); got != frames {
			t.Fatal("round trip changed the frames")
		}
		for _, i := range []int{49, 99, 149, 150, 151} {
			data[i/8] ^= 0x80 >> (i % 8)
		}
		if got := vocoder.UnpackAMBEVoice(data); got != frames {
			t.Fatal("unpack used a bit that carries no frame bits")
		}
	}
}

// TestAMBEVoiceFlags_BadBits checks bits 49 and 99, the bad flags of frames
// 2 and 3 (ipsc2hbpc GV_BAD2_BIT and GV_BAD3_BIT).
func TestAMBEVoiceFlags_BadBits(t *testing.T) {
	t.Parallel()

	var data [19]byte
	data[6] |= 0x40
	data[12] |= 0x10
	frames, bad := vocoder.UnpackAMBEVoiceFlags(data)
	if bad != [3]bool{false, true, true} {
		t.Fatalf("bad %v, want [false true true]", bad)
	}
	if frames != ([3]vocoder.AMBEFrame{}) {
		t.Fatal("the flag bits changed the frames")
	}
	if got := vocoder.PackAMBEVoiceFlags(frames, bad); got != data {
		t.Fatalf("pack % X, want % X", got, data)
	}
	if got := vocoder.PackAMBEVoiceFlags(frames, [3]bool{true, false, false}); got != ([19]byte{}) {
		t.Fatalf("bad[0] set bits % X", got)
	}
}

func TestAMBEVoiceFlags_RoundTrip(t *testing.T) {
	t.Parallel()

	raw, err := hex.DecodeString(ipscSilence)
	if err != nil {
		t.Fatal(err)
	}
	frames := vocoder.UnpackAMBEVoice([19]byte(raw))
	for n := range 4 {
		want := [3]bool{false, n&1 != 0, n&2 != 0}
		data := vocoder.PackAMBEVoiceFlags(frames, want)
		gotFrames, got := vocoder.UnpackAMBEVoiceFlags(data)
		if got != want || gotFrames != frames {
			t.Errorf("flags %v: got %v", want, got)
		}
		if vocoder.UnpackAMBEVoice(data) != frames {
			t.Errorf("flags %v: UnpackAMBEVoice changed the frames", want)
		}
	}
}

// TestAMBEVoice_VoiceBurst checks the payload of the first voice burst of
// layer2/testdata/voice.bin against the PackAMBEVoice of dmrgo v0.0.1.
func TestAMBEVoice_VoiceBurst(t *testing.T) {
	t.Parallel()

	raw, err := hex.DecodeString("fba8e556272f8079d5daa8c6760755fd7df75f760ea149e1da8ac676077bf549f5")
	if err != nil {
		t.Fatal(err)
	}
	var frame [33]byte
	copy(frame[:], raw)
	burst, err := layer2.NewBurstFromBytes(frame)
	if err != nil || (burst.Kind == layer2.KindData) {
		t.Fatalf("voice burst: %v", err)
	}
	frames, _ := burst.AMBEFrames()
	payload := vocoder.PackAMBEVoice(frames)
	if got := hex.EncodeToString(payload[:]); got != "f813d9918ca7be04f6314102af81329c504fb8" {
		t.Errorf("AMBE payload %s", got)
	}
}
