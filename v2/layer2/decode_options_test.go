package layer2_test

import (
	"encoding/hex"
	"errors"
	"math/rand"
	"reflect"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/bit"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
)

// dmrdVoiceBurstD is a DMRD voice frame (frame type 0, voice sequence 3,
// burst D) whose centre carries the MS-sourced RC SYNC instead of an EMB.
const dmrdVoiceBurstD = "085dc11e5e1d328f0cae4bc91da77d55f7dfd77ab808e70a096ad32de1f56d0f9e"

func flipSync(frame [33]byte, n int) [33]byte {
	for i := 0; i < n; i++ {
		p := layer2.SyncStart + 5*i
		frame[p/8] ^= 1 << (7 - p%8)
	}
	return frame
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func TestDecodeFromBytesWith_DefaultMatchesDecodeFromBytes(t *testing.T) {
	frames := captureFrames(t)
	frames = append(frames, buildStandaloneRCBurst(7, enums.RCCeaseTransmissionRequest))
	rng := rand.New(rand.NewSource(5))
	for _, f := range captureFrames(t) {
		for n := 0; n < 8; n++ {
			frames = append(frames, flipSync(f, n))
		}
	}
	for n := 0; n < 2000; n++ {
		var f [33]byte
		rng.Read(f[:])
		frames = append(frames, f)
	}
	for _, frame := range frames {
		var want layer2.Burst
		wantErr := want.DecodeFromBytes(frame)
		for _, opts := range []layer2.DecodeOptions{{}, {SyncThreshold: layer2.DefaultSyncThreshold}} {
			var got layer2.Burst
			err := got.DecodeFromBytesWith(frame, opts)
			if errString(err) != errString(wantErr) || !reflect.DeepEqual(got, want) {
				t.Fatalf("%x opts %+v: got %s (%v), want %s (%v)", frame, opts, got.String(), err, want.String(), wantErr)
			}
		}
	}
}

func TestDecodeFromBytesWith_KnownVoiceBurstOverridesRCSync(t *testing.T) {
	raw, err := hex.DecodeString(dmrdVoiceBurstD)
	if err != nil {
		t.Fatal(err)
	}
	frame := [33]byte(raw)

	def := must(layer2.NewBurstFromBytes(frame))
	if def.SyncPattern != enums.SyncMSSourcedRC || def.VoiceSlots != ([3][72]bit.Bit{}) {
		t.Fatalf("DecodeFromBytes no longer takes this burst as RC: %s", def.String())
	}

	var b layer2.Burst
	opts := layer2.DecodeOptions{Kind: layer2.KindVoice, VoiceBurst: enums.VoiceBurstD}
	if err := b.DecodeFromBytesWith(frame, opts); err != nil {
		t.Fatal(err)
	}
	if (b.Kind == layer2.KindData) || b.VoiceBurst != enums.VoiceBurstD || !b.HasEmbeddedSignalling() || b.SyncPattern != enums.SyncNone {
		t.Fatalf("not decoded as voice burst D: %s", b.String())
	}
	if _, res := b.AMBEFrames(); res.BitsChecked == 0 || res.ErrorsCorrected != 0 || res.Uncorrectable {
		t.Fatalf("AMBE not clean: %+v", res)
	}
	r := layer2.RawBurst(frame)
	if packSlots(b.VoiceSlots) != r.VoiceSlots() {
		t.Fatal("voice slots differ from the frame")
	}
	enc := must(b.Encode())
	if enc.VoiceSlots() != r.VoiceSlots() {
		t.Fatal("Encode lost the voice")
	}
}

func TestDecodeFromBytesWith_SyncThreshold(t *testing.T) {
	n := 0
	for _, frame := range captureFrames(t) {
		clean := must(layer2.NewBurstFromBytes(frame))
		if clean.Kind != layer2.KindData || clean.SyncDistance != 0 {
			continue
		}
		n++
		five := flipSync(frame, 5)
		if b, _ := layer2.NewBurstFromBytes(five); b.Kind == layer2.KindData {
			t.Fatal("default threshold matched a SYNC 5 bits off")
		}
		var b layer2.Burst
		if err := b.DecodeFromBytesWith(five, layer2.DecodeOptions{SyncThreshold: 5}); err != nil {
			t.Fatal(err)
		}
		if b.Kind != layer2.KindData || b.SyncPattern != clean.SyncPattern || b.SyncDistance != 5 || !reflect.DeepEqual(b.Data, clean.Data) {
			t.Fatalf("threshold 5: %s", b.String())
		}
		one := flipSync(frame, 1)
		if err := b.DecodeFromBytesWith(one, layer2.DecodeOptions{SyncThreshold: -1}); err != nil {
			t.Fatal(err)
		}
		if (b.Kind == layer2.KindData) || b.SyncPattern != enums.SyncNone || b.SyncDistance != 1 {
			t.Fatalf("exact match accepted a SYNC 1 bit off: %s", b.String())
		}
	}
	if n == 0 {
		t.Fatal("no clean data bursts")
	}
}

func TestDecodeFromBytesWith_KnownData(t *testing.T) {
	n := 0
	for _, frame := range captureFrames(t) {
		clean := must(layer2.NewBurstFromBytes(frame))
		if clean.Kind != layer2.KindData || clean.FEC.SlotType.HasErrors() || clean.Data == nil {
			continue
		}
		n++
		broken := frame
		for i := 13; i < 20; i++ {
			broken[i] = 0
		}
		broken[12] ^= 0x3F
		broken[20] ^= 0xFC
		dt := clean.SlotType.DataType
		var b layer2.Burst
		err := b.DecodeFromBytesWith(broken, layer2.DecodeOptions{Kind: layer2.KindData, DataType: &dt})
		if err != nil {
			t.Fatal(err)
		}
		if b.Kind != layer2.KindData || b.SlotType.DataType != dt || !reflect.DeepEqual(b.Data, clean.Data) {
			t.Fatalf("known data type: %s, want %s", b.String(), clean.String())
		}
		if err := b.DecodeFromBytesWith(frame, layer2.DecodeOptions{Kind: layer2.KindData}); err != nil || !reflect.DeepEqual(b.Data, clean.Data) {
			t.Fatalf("data kind without data type: %s (%v)", b.String(), err)
		}
	}
	if n == 0 {
		t.Fatal("no data bursts")
	}
}

func TestDecodeFromBytesWith_KnownVoiceA(t *testing.T) {
	n := 0
	for _, frame := range captureFrames(t) {
		clean := must(layer2.NewBurstFromBytes(frame))
		if clean.VoiceBurst != enums.VoiceBurstA || clean.SyncDistance != 0 {
			continue
		}
		n++
		six := flipSync(frame, 6)
		var b layer2.Burst
		if err := b.DecodeFromBytesWith(six, layer2.DecodeOptions{Kind: layer2.KindVoice, VoiceBurst: enums.VoiceBurstA}); err != nil {
			t.Fatal(err)
		}
		if b.VoiceBurst != enums.VoiceBurstA || b.SyncPattern != clean.SyncPattern || b.SyncDistance != 6 || b.HasEmbeddedSignalling() || b.VoiceSlots != clean.VoiceSlots {
			t.Fatalf("burst A: %s", b.String())
		}
		if err := b.DecodeFromBytesWith(frame, layer2.DecodeOptions{Kind: layer2.KindVoice}); err != nil || b.VoiceBurst != enums.VoiceBurstA {
			t.Fatalf("voice of unknown position: %s (%v)", b.String(), err)
		}
		if err := b.DecodeFromBytesWith(six, layer2.DecodeOptions{Kind: layer2.KindVoice}); err != nil || b.VoiceBurst != enums.VoiceBurstUnknown || !b.HasEmbeddedSignalling() {
			t.Fatalf("voice of unknown position, SYNC 6 bits off: %s (%v)", b.String(), err)
		}
	}
	if n == 0 {
		t.Fatal("no voice burst A")
	}
	var b layer2.Burst
	if err := b.DecodeFromBytesWith([33]byte{}, layer2.DecodeOptions{Kind: layer2.KindVoice, VoiceBurst: 7}); err == nil {
		t.Fatal("position 7 accepted")
	}
	if err := b.DecodeFromBytesWith([33]byte{}, layer2.DecodeOptions{Kind: 9}); err == nil {
		t.Fatal("kind 9 accepted")
	}
}

func TestDecodeFromBytesWith_KnownReverseChannel(t *testing.T) {
	frame := flipSync(buildStandaloneRCBurst(7, enums.RCCeaseTransmissionRequest), 6)
	if b, _ := layer2.NewBurstFromBytes(frame); b.SyncPattern == enums.SyncMSSourcedRC {
		t.Fatal("default decode matched a SYNC 6 bits off")
	}
	var b layer2.Burst
	if err := b.DecodeFromBytesWith(frame, layer2.DecodeOptions{Kind: layer2.KindReverseChannel}); err != nil {
		t.Fatal(err)
	}
	if b.SyncPattern != enums.SyncMSSourcedRC || b.SyncDistance != 6 || !b.HasReverseChannel() || b.ReverseChannel.RCCommand != enums.RCCeaseTransmissionRequest {
		t.Fatalf("RC: %s", b.String())
	}
}

func TestDecodeFromBytesWith_ColorCode(t *testing.T) {
	seen := map[string]bool{}
	for _, frame := range captureFrames(t) {
		clean := must(layer2.NewBurstFromBytes(frame))
		var cc int
		var kind string
		switch {
		case (clean.Kind == layer2.KindData):
			cc, kind = int(clean.SlotType.ColorCode), "data"
		case clean.HasEmbeddedSignalling():
			cc, kind = int(clean.EmbeddedSignalling.ColorCode), "EMB"
		default:
			kind = "voice A"
		}
		seen[kind] = true
		right := enums.ColorCode(cc)         //nolint:gosec // 4-bit color code
		wrong := enums.ColorCode(cc+1) & 0xF //nolint:gosec // 4-bit color code
		var b layer2.Burst
		if err := b.DecodeFromBytesWith(frame, layer2.DecodeOptions{ColorCode: &right}); err != nil {
			t.Fatalf("%s: %v", kind, err)
		}
		err := b.DecodeFromBytesWith(frame, layer2.DecodeOptions{ColorCode: &wrong})
		if kind == "voice A" {
			if err != nil {
				t.Fatalf("voice A checked: %v", err)
			}
			continue
		}
		if !errors.Is(err, layer2.ErrColorCodeMismatch) {
			t.Fatalf("%s: err %v", kind, err)
		}
		if b.String() != clean.String() {
			t.Fatalf("%s: burst not populated on mismatch", kind)
		}
	}
	for _, k := range []string{"data", "EMB", "voice A"} {
		if !seen[k] {
			t.Errorf("no %s bursts", k)
		}
	}
}
