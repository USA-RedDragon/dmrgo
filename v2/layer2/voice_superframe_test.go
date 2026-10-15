package layer2_test

import (
	"errors"
	"fmt"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/bit"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
	"github.com/USA-RedDragon/dmrgo/v2/vocoder"
)

func testSlot(n int) [72]bit.Bit {
	var slot [72]bit.Bit
	for i := range slot {
		slot[i] = bit.Bit(((i*7 + n*13) >> 2 & 1) ^ (n >> (i % 5) & 1))
	}
	return slot
}

func silenceSlot() [72]bit.Bit {
	var slot [72]bit.Bit
	for i := range slot {
		slot[i] = bit.Bit(i % 3 & 1)
	}
	return slot
}

// groupLC returns a Grp_V_Ch_Usr LC. A manufacturer FID gets the same
// payload bits in RawPayload, since only the SFID dispatches to GroupVoice.
func groupLC(fid enums.FeatureSetID) pdu.FullLinkControl {
	gv := &pdu.FLCGroupVoice{GroupAddress: 91, SourceAddress: 3191868}
	lc := pdu.FullLinkControl{FLCO: enums.FLCOGroupVoiceChannelUser, FeatureSetID: fid}
	if fid == enums.StandardizedFID {
		lc.GroupVoice = gv
	} else {
		lc.RawPayload = must(pdu.EncodeFLCGroupVoice(gv))
	}
	return lc
}

// lcBits returns the 96 on-air bits of lc as a burst of dataType carries
// them, so LCs decoded with RawPayload compare equal to the variant they
// were built from.
func lcBits(t *testing.T, lc pdu.FullLinkControl, dataType enums.DataType) [96]bit.Bit {
	t.Helper()
	lc.DataType = dataType
	return must(pdu.EncodeFullLinkControl(&lc))
}

func checkLCBurst(t *testing.T, raw [33]byte, lc pdu.FullLinkControl, dataType enums.DataType, cc int, sync enums.SyncPattern) {
	t.Helper()
	b, err := layer2.NewBurstFromBytes(raw)
	if err != nil {
		t.Fatal(err)
	}
	if !b.IsData || b.SyncPattern != sync || b.SyncDistance != 0 {
		t.Fatalf("IsData=%v SyncPattern=%s", b.IsData, enums.SyncPatternToName(b.SyncPattern))
	}
	if b.SlotType.DataType != dataType || b.SlotType.ColorCode != cc || b.SlotType.FEC.HasErrors() {
		t.Fatalf("slot type %s", b.SlotType.ToString())
	}
	got, ok := b.Data.(*pdu.FullLinkControl)
	if !ok {
		t.Fatalf("Data is %T", b.Data)
	}
	if b.FEC.PDU.HasErrors() || b.FEC.Payload.HasErrors() {
		t.Fatalf("FEC %+v", b.FEC)
	}
	if got.FeatureSetID != lc.FeatureSetID || lcBits(t, *got, dataType) != lcBits(t, lc, dataType) {
		t.Errorf("decoded %s, want %s", got.ToString(), lc.ToString())
	}
}

// checkVoiceBurst decodes voice burst i of a transmission and checks its
// slots and its SYNC or EMB for superframe position i mod 6.
func checkVoiceBurst(t *testing.T, i int, raw [33]byte, slots [3][72]bit.Bit, voiceSync enums.SyncPattern, cc int) *layer2.Burst {
	t.Helper()
	b, err := layer2.NewBurstFromBytes(raw)
	if err != nil {
		t.Fatal(err)
	}
	if b.IsData {
		t.Fatalf("burst %d is data", i)
	}
	if b.VoiceSlots() != slots {
		t.Errorf("burst %d: voice slots differ", i)
	}
	pos := i % 6
	if pos == 0 {
		if b.SyncPattern != voiceSync || b.HasEmbeddedSignalling || b.VoiceBurst != enums.VoiceBurstA {
			t.Fatalf("burst %d: SyncPattern=%s", i, enums.SyncPatternToName(b.SyncPattern))
		}
		return b
	}
	emb := b.EmbeddedSignalling
	if !b.HasEmbeddedSignalling || emb.FEC.HasErrors() || emb.ColorCode != cc || emb.PreemptionAndPowerControlIndicator {
		t.Fatalf("burst %d: %s", i, b.ToString())
	}
	wantLCSS := []enums.LCSS{0, enums.FirstFragmentLC, enums.ContinuationFragmentLCorCSBK,
		enums.ContinuationFragmentLCorCSBK, enums.LastFragmentLCorCSBK, enums.SingleFragmentLCorCSBK}[pos]
	if emb.LCSS != wantLCSS {
		t.Errorf("burst %d: LCSS %s, want %s", i, enums.LCSSToName(emb.LCSS), enums.LCSSToName(wantLCSS))
	}
	if pos == 5 && (!layer2.IsNullEmbeddedLC(b.EmbeddedSignallingData) || b.HasReverseChannel) {
		t.Errorf("burst %d: burst F carries %x, want the Null embedded message", i, b.PackEmbeddedSignallingData())
	}
	return b
}

// TestVoiceTransmitter_Transmission builds a transmission of two full
// superframes and one partial one and decodes every burst.
func TestVoiceTransmitter_Transmission(t *testing.T) {
	const cc = 5
	sources := []layer2.SyncSource{layer2.SyncSourceBS, layer2.SyncSourceMS, layer2.SyncSourceDirectTS1, layer2.SyncSourceDirectTS2}
	for _, source := range sources {
		for _, fid := range []enums.FeatureSetID{enums.StandardizedFID, enums.FeatureSetID(0xC2)} {
			t.Run(fmt.Sprintf("source%d/FID%#x", source, int(fid)), func(t *testing.T) {
				voiceSync, dataSync, ok := source.SyncPatterns()
				if !ok {
					t.Fatal("unknown source")
				}
				lc := groupLC(fid)
				tx := must(layer2.NewVoiceTransmitter(lc, cc, source, silenceSlot()))

				var slots [][72]bit.Bit
				var bursts [][33]byte
				for n := 0; n < 40; n++ {
					slots = append(slots, testSlot(n))
					bursts = append(bursts, must(tx.AddSlots(testSlot(n)))...)
				}
				if len(bursts) != 13 {
					t.Fatalf("%d bursts from 40 slots, want 13", len(bursts))
				}
				tail := must(tx.Terminate())
				if len(tail) != 6 {
					t.Fatalf("Terminate returned %d bursts, want 5 padding bursts and the terminator", len(tail))
				}
				bursts = append(bursts, tail[:5]...)
				for len(slots) < 18*3 {
					slots = append(slots, silenceSlot())
				}

				checkLCBurst(t, tx.Header(), lc, enums.DataTypeVoiceLCHeader, cc, dataSync)
				checkLCBurst(t, tail[5], lc, enums.DataTypeTerminatorWithLC, cc, dataSync)

				var fragments [4][32]bit.Bit
				for i, raw := range bursts {
					want := [3][72]bit.Bit{slots[3*i], slots[3*i+1], slots[3*i+2]}
					b := checkVoiceBurst(t, i, raw, want, voiceSync, cc)
					if pos := i % 6; pos >= 1 && pos <= 4 {
						fragments[pos-1] = b.EmbeddedSignallingData
					}
					if i%6 != 4 {
						continue
					}
					got, res := layer2.DecodeEmbeddedFullLinkControl(fragments)
					if res.HasErrors() || got.FeatureSetID != fid ||
						lcBits(t, got, enums.DataTypeVoiceLCHeader) != lcBits(t, lc, enums.DataTypeVoiceLCHeader) {
						t.Errorf("burst %d: embedded LC %s (%+v)", i, got.ToString(), res)
					}
					target, source, ok := got.Addresses()
					if !ok || target != 91 || source != 3191868 {
						t.Errorf("burst %d: addresses %d -> %d", i, source, target)
					}
				}
			})
		}
	}
}

func TestVoiceTransmitter_AddFramesUsesAMBE72(t *testing.T) {
	lc := groupLC(enums.StandardizedFID)
	frames := make([][49]bit.Bit, 3)
	slots := make([][72]bit.Bit, 3)
	for i := range frames {
		for j := range frames[i] {
			frames[i][j] = bit.Bit((i + j*j) & 1)
		}
		slots[i] = vocoder.EncodeAMBE72(frames[i])
	}
	a := must(layer2.NewVoiceTransmitter(lc, 1, layer2.SyncSourceBS, silenceSlot()))
	b := must(layer2.NewVoiceTransmitter(lc, 1, layer2.SyncSourceBS, silenceSlot()))
	got := must(a.AddFrames(frames...))
	want := must(b.AddSlots(slots...))
	if len(got) != 1 || len(want) != 1 || got[0] != want[0] {
		t.Fatalf("AddFrames %x, AddSlots %x", got, want)
	}
	burst := must(layer2.NewBurstFromBytes(got[0]))
	for i := range frames {
		if burst.VoiceData.Frames[i].DecodedBits != frames[i] || burst.VoiceData.Frames[i].FEC.HasErrors() {
			t.Errorf("frame %d: decoded %v", i, burst.VoiceData.Frames[i].DecodedBits)
		}
	}
}

func TestVoiceTransmitter_ReverseChannel(t *testing.T) {
	lc := groupLC(enums.StandardizedFID)
	tx := must(layer2.NewVoiceTransmitter(lc, 2, layer2.SyncSourceBS, silenceSlot()))
	if err := tx.SetReverseChannel(pdu.ReverseChannel{RCCommand: enums.RCCeaseTransmissionCommand}); err != nil {
		t.Fatal(err)
	}
	var bursts [][33]byte
	for n := 0; n < 12*3; n++ {
		bursts = append(bursts, must(tx.AddSlots(testSlot(n)))...)
	}
	f := must(layer2.NewBurstFromBytes(bursts[5]))
	emb := f.EmbeddedSignalling
	if !f.HasReverseChannel || !emb.PreemptionAndPowerControlIndicator || emb.LCSS != enums.SingleFragmentLCorCSBK || emb.ColorCode != 2 {
		t.Fatalf("first burst F: %s", f.ToString())
	}
	if f.ReverseChannel.RCCommand != enums.RCCeaseTransmissionCommand || f.FEC.RC.HasErrors() {
		t.Errorf("RC %s", f.ReverseChannel.ToString())
	}
	next := must(layer2.NewBurstFromBytes(bursts[11]))
	if next.HasReverseChannel || !layer2.IsNullEmbeddedLC(next.EmbeddedSignallingData) {
		t.Errorf("second burst F: %s", next.ToString())
	}

	for _, source := range []layer2.SyncSource{layer2.SyncSourceMS, layer2.SyncSourceDirectTS1, layer2.SyncSourceDirectTS2} {
		tx := must(layer2.NewVoiceTransmitter(lc, 2, source, silenceSlot()))
		if err := tx.SetReverseChannel(pdu.ReverseChannel{}); err == nil {
			t.Errorf("source %d: reverse channel accepted", source)
		}
	}
	if err := tx.SetReverseChannel(pdu.ReverseChannel{RCCommand: enums.RCCommand(16)}); err == nil {
		t.Error("RC command 16 accepted")
	}
}

func TestVoiceTransmitter_Terminate(t *testing.T) {
	lc := groupLC(enums.StandardizedFID)

	tx := must(layer2.NewVoiceTransmitter(lc, 1, layer2.SyncSourceMS, silenceSlot()))
	tail := must(tx.Terminate())
	if len(tail) != 1 {
		t.Fatalf("empty transmission: %d bursts, want the terminator alone", len(tail))
	}
	checkLCBurst(t, tail[0], lc, enums.DataTypeTerminatorWithLC, 1, enums.MsSourcedData)
	if _, err := tx.Terminate(); !errors.Is(err, layer2.ErrVoiceTerminated) {
		t.Errorf("second Terminate: %v", err)
	}
	if _, err := tx.AddSlots(testSlot(0)); !errors.Is(err, layer2.ErrVoiceTerminated) {
		t.Errorf("AddSlots after Terminate: %v", err)
	}
	if err := tx.SetReverseChannel(pdu.ReverseChannel{}); !errors.Is(err, layer2.ErrVoiceTerminated) {
		t.Errorf("SetReverseChannel after Terminate: %v", err)
	}

	tx = must(layer2.NewVoiceTransmitter(lc, 1, layer2.SyncSourceMS, silenceSlot()))
	slots := make([][72]bit.Bit, 18)
	if n := len(must(tx.AddSlots(slots...))); n != 6 {
		t.Fatalf("%d bursts from 18 slots", n)
	}
	if n := len(must(tx.Terminate())); n != 1 {
		t.Errorf("Terminate after a full superframe returned %d bursts, want the terminator alone", n)
	}
}

func TestVoiceTransmitter_RejectsBadInput(t *testing.T) {
	lc := groupLC(enums.StandardizedFID)
	if _, err := layer2.NewVoiceTransmitter(lc, 16, layer2.SyncSourceBS, silenceSlot()); err == nil {
		t.Error("color code 16 accepted")
	}
	if _, err := layer2.NewVoiceTransmitter(lc, 1, layer2.SyncSource(4), silenceSlot()); err == nil {
		t.Error("sync source 4 accepted")
	}
	lc.FLCO = enums.FLCO(64)
	if _, err := layer2.NewVoiceTransmitter(lc, 1, layer2.SyncSourceBS, silenceSlot()); err == nil {
		t.Error("FLCO 64 accepted")
	}
}

// captureSuperframes splits the voice bursts of a capture into complete
// superframes A to F. It returns the indices of the bursts in complete
// superframes and of the other voice bursts.
func captureSuperframes(t *testing.T, bursts []*layer2.Burst) (kept, skipped []int) {
	t.Helper()
	var run []int
	end := func() {
		if len(run) == 6 {
			kept = append(kept, run...)
		} else {
			skipped = append(skipped, run...)
		}
		run = nil
	}
	for i, b := range bursts {
		switch {
		case b.IsData:
			end()
		case b.VoiceBurst == enums.VoiceBurstA:
			end()
			run = append(run, i)
		case len(run) > 0 && len(run) < 6:
			run = append(run, i)
		default:
			end()
			skipped = append(skipped, i)
		}
	}
	end()
	return kept, skipped
}

// TestVoiceTransmitter_ReproducesCaptures rebuilds each capture from its
// Voice LC Header and voice slots. The captures drop bursts: voice.bin has a
// superframe without its burst A and parrot_kerchunk.bin a burst A without
// bursts B to F. Every other burst must match the capture bit for bit.
func TestVoiceTransmitter_ReproducesCaptures(t *testing.T) {
	tests := []struct {
		file    string
		kept    int
		skipped []int
	}{
		{"testdata/voice.bin", 72, []int{49, 50, 51, 52, 53}},
		{"testdata/parrot_kerchunk.bin", 12, []int{7}},
	}
	for _, tt := range tests {
		t.Run(tt.file, func(t *testing.T) {
			raws := loadBursts(t, tt.file)
			bursts := make([]*layer2.Burst, len(raws))
			for i, raw := range raws {
				bursts[i] = must(layer2.NewBurstFromBytes(raw))
			}
			last := len(raws) - 1
			if bursts[0].SlotType.DataType != enums.DataTypeVoiceLCHeader || bursts[last].SlotType.DataType != enums.DataTypeTerminatorWithLC {
				t.Fatal("capture does not start with a header and end with a terminator")
			}
			kept, skipped := captureSuperframes(t, bursts[1:last])
			for i := range kept {
				kept[i]++
			}
			for i := range skipped {
				skipped[i]++
			}
			if len(kept) != tt.kept || fmt.Sprint(skipped) != fmt.Sprint(tt.skipped) {
				t.Fatalf("%d bursts in complete superframes, want %d; others %v, want %v", len(kept), tt.kept, skipped, tt.skipped)
			}

			lc, ok := bursts[0].Data.(*pdu.FullLinkControl)
			if !ok {
				t.Fatalf("header Data is %T", bursts[0].Data)
			}
			cc := bursts[0].SlotType.ColorCode
			tx := must(layer2.NewVoiceTransmitter(*lc, uint8(cc), layer2.SyncSourceBS, silenceSlot()))
			frames := must(layer2.NewVoiceTransmitter(*lc, uint8(cc), layer2.SyncSourceBS, silenceSlot()))
			if tx.Header() != raws[0] {
				t.Errorf("header\ngot  %x\nwant %x", tx.Header(), raws[0])
			}
			for _, i := range kept {
				slots := bursts[i].VoiceSlots()
				got := must(tx.AddSlots(slots[:]...))
				if len(got) != 1 || got[0] != raws[i] {
					t.Errorf("burst %d\ngot  %x\nwant %x", i, got, raws[i])
				}

				var decoded [3][49]bit.Bit
				for j, f := range bursts[i].VoiceData.Frames {
					decoded[j] = f.DecodedBits
				}
				fromFrames := must(frames.AddFrames(decoded[:]...))
				if len(fromFrames) != 1 {
					t.Fatalf("burst %d: %d bursts from frames", i, len(fromFrames))
				}
				if got := must(bursts[i].Encode()); got != raws[i] {
					t.Errorf("burst %d: decode then Encode changed the burst\ngot  %x\nwant %x", i, got, raws[i])
				}
				if n := bursts[i].FEC.Voice.ErrorsCorrected; n != 0 {
					regen := must(layer2.NewBurstFromBytes(fromFrames[0]))
					for j, f := range regen.VoiceData.Frames {
						if f.DecodedBits != decoded[j] || f.FEC.ErrorsCorrected != 0 {
							t.Errorf("burst %d frame %d: the frame path did not regenerate a clean frame", i, j)
						}
					}
					t.Logf("burst %d: the capture has %d voice bit errors, which the frame path corrects", i, n)
				} else if fromFrames[0] != raws[i] {
					t.Errorf("burst %d from frames\ngot  %x\nwant %x", i, fromFrames[0], raws[i])
				}
			}
			tail := must(tx.Terminate())
			if len(tail) != 1 || tail[0] != raws[last] {
				t.Errorf("terminator\ngot  %x\nwant %x", tail, raws[last])
			}
		})
	}
}
