package layer2_test

import (
	"errors"
	"fmt"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/internal/bits"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
	"github.com/USA-RedDragon/dmrgo/v2/vocoder"
)

func testSlot(n int) [9]byte {
	var slot [9]byte
	for i := range 72 {
		bits.SetBit(slot[:], i, byte(((i*7+n*13)>>2&1)^(n>>(i%5)&1)))
	}
	return slot
}

func silenceSlot() [9]byte {
	var slot [9]byte
	for i := range 72 {
		bits.SetBit(slot[:], i, byte(i%3&1))
	}
	return slot
}

func groupLC(fid enums.FID) pdu.FullLinkControl {
	var lc pdu.FullLinkControl
	mustSet(lc.SetGroupVoice(pdu.FLCGroupVoice{GroupAddress: 91, SourceAddress: 3191868}))
	lc.FID = fid
	return lc
}

func lcBits(t *testing.T, lc pdu.FullLinkControl, dataType enums.DataType) [12]byte {
	t.Helper()
	lc.BurstDataType = dataType
	return must(lc.Encode())
}

func checkLCBurst(t *testing.T, raw [33]byte, lc pdu.FullLinkControl, dataType enums.DataType, cc int, sync enums.SyncPattern) {
	t.Helper()
	b, err := layer2.NewBurstFromBytes(raw)
	if err != nil {
		t.Fatal(err)
	}
	if b.Kind != layer2.KindData || b.SyncPattern != sync || b.SyncDistance != 0 {
		t.Fatalf("IsData=%v SyncPattern=%s", (b.Kind == layer2.KindData), b.SyncPattern.String())
	}
	if b.SlotType.DataType != dataType || b.SlotType.ColorCode != enums.ColorCode(cc) || b.SlotType.FEC.HasErrors() {
		t.Fatalf("slot type %s", b.SlotType.String())
	}
	got, ok := b.Data.(*pdu.FullLinkControl)
	if !ok {
		t.Fatalf("Data is %T", b.Data)
	}
	if b.FEC.PDU.HasErrors() || b.FEC.Payload.HasErrors() {
		t.Fatalf("FEC %+v", b.FEC)
	}
	if got.FID != lc.FID || lcBits(t, *got, dataType) != lcBits(t, lc, dataType) {
		t.Errorf("decoded %s, want %s", got.String(), lc.String())
	}
}

func checkVoiceBurst(t *testing.T, i int, raw [33]byte, slots [3][9]byte, voiceSync enums.SyncPattern, cc int) *layer2.Burst {
	t.Helper()
	b, err := layer2.NewBurstFromBytes(raw)
	if err != nil {
		t.Fatal(err)
	}
	if b.Kind == layer2.KindData {
		t.Fatalf("burst %d is data", i)
	}
	if b.VoiceSlots != slots {
		t.Errorf("burst %d: voice slots differ", i)
	}
	pos := i % 6
	if pos == 0 {
		if b.SyncPattern != voiceSync || b.HasEmbeddedSignalling() || b.VoiceBurst != enums.VoiceBurstA {
			t.Fatalf("burst %d: SyncPattern=%s", i, b.SyncPattern.String())
		}
		return b
	}
	emb := b.EmbeddedSignalling
	if !b.HasEmbeddedSignalling() || emb.FEC.HasErrors() || emb.ColorCode != enums.ColorCode(cc) || emb.PreemptionAndPowerControlIndicator {
		t.Fatalf("burst %d: %s", i, b.String())
	}
	wantLCSS := []enums.LCSS{0, enums.LCSSFirstFragment, enums.LCSSContinuationFragment,
		enums.LCSSContinuationFragment, enums.LCSSLastFragment, enums.LCSSSingleFragment}[pos]
	if emb.LCSS != wantLCSS {
		t.Errorf("burst %d: LCSS %s, want %s", i, emb.LCSS.String(), wantLCSS.String())
	}
	if pos == 5 && (b.EmbeddedSignallingData != [4]byte{} || b.HasReverseChannel()) {
		t.Errorf("burst %d: burst F carries %x, want the Null embedded message", i, b.EmbeddedSignallingData)
	}
	return b
}

func TestVoiceTransmitter_Transmission(t *testing.T) {
	t.Parallel()

	const cc = 5
	sources := []layer2.SyncSource{layer2.SyncSourceBS, layer2.SyncSourceMS, layer2.SyncSourceDirectTS1, layer2.SyncSourceDirectTS2}
	for _, source := range sources {
		for _, fid := range []enums.FID{enums.FIDStandardized, enums.FID(0xC2)} {
			t.Run(fmt.Sprintf("source%d/FID%#x", source, int(fid)), func(t *testing.T) {
				t.Parallel()

				voiceSync, dataSync, ok := source.SyncPatterns()
				if !ok {
					t.Fatal("unknown source")
				}
				lc := groupLC(fid)
				tx := must(layer2.NewVoiceTransmitter(&layer2.VoiceTransmitterOptions{LC: lc, ColorCode: cc, Source: source, Silence: silenceSlot()}))

				var slots [][9]byte
				var bursts []layer2.RawBurst
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

				var fragments [4][4]byte
				for i, raw := range bursts {
					want := [3][9]byte{slots[3*i], slots[3*i+1], slots[3*i+2]}
					b := checkVoiceBurst(t, i, raw, want, voiceSync, cc)
					if pos := i % 6; pos >= 1 && pos <= 4 {
						fragments[pos-1] = b.EmbeddedSignallingData
					}
					if i%6 != 4 {
						continue
					}
					got := layer2.DecodeEmbeddedFullLinkControl(fragments, false)
					if res := got.FEC; res.HasErrors() || got.FID != fid ||
						lcBits(t, got, enums.DataTypeVoiceLCHeader) != lcBits(t, lc, enums.DataTypeVoiceLCHeader) {
						t.Errorf("burst %d: embedded LC %s (%+v)", i, got.String(), res)
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
	t.Parallel()

	lc := groupLC(enums.FIDStandardized)
	frames := make([]vocoder.AMBEFrame, 3)
	slots := make([][9]byte, 3)
	for i := range frames {
		for j := range 49 {
			bits.SetBit(frames[i][:], j, byte((i+j*j)&1))
		}
		slots[i] = must(vocoder.EncodeAMBE72(frames[i]))
	}
	a := must(layer2.NewVoiceTransmitter(&layer2.VoiceTransmitterOptions{LC: lc, ColorCode: 1, Source: layer2.SyncSourceBS, Silence: silenceSlot()}))
	b := must(layer2.NewVoiceTransmitter(&layer2.VoiceTransmitterOptions{LC: lc, ColorCode: 1, Source: layer2.SyncSourceBS, Silence: silenceSlot()}))
	got := must(a.AddFrames(frames...))
	want := must(b.AddSlots(slots...))
	if len(got) != 1 || len(want) != 1 || got[0] != want[0] {
		t.Fatalf("AddFrames %x, AddSlots %x", got, want)
	}
	burst := must(layer2.NewBurstFromBytes(got[0]))
	decoded, res := burst.AMBEFrames()
	if res.HasErrors() {
		t.Errorf("AMBE FEC %+v", res)
	}
	for i := range frames {
		if decoded[i] != frames[i] {
			t.Errorf("frame %d: decoded %v", i, decoded[i])
		}
	}
}

func TestVoiceTransmitter_Append(t *testing.T) {
	t.Parallel()

	opts := &layer2.VoiceTransmitterOptions{
		LC: groupLC(enums.FIDStandardized), ColorCode: 1, Source: layer2.SyncSourceBS, Silence: silenceSlot(),
		PIHeader: &pdu.PIHeader{AlgorithmID: 1, KeyID: 7, Destination: 91},
	}
	a := must(layer2.NewVoiceTransmitter(opts))
	b := must(layer2.NewVoiceTransmitter(opts))
	frames := make([]vocoder.AMBEFrame, 4)
	slots := make([][9]byte, 4)
	for i := range slots {
		slots[i] = testSlot(i)
		bits.SetBit(frames[i][:], i, 1)
	}
	want := a.Headers()
	want = append(want, must(a.AddSlots(slots...))...)
	want = append(want, must(a.AddFrames(frames...))...)
	want = append(want, must(a.Terminate())...)

	prefix := layer2.RawBurst{0xAA}
	got := b.AppendHeaders([]layer2.RawBurst{prefix})
	got = must(b.AppendSlots(got, slots...))
	got = must(b.AppendFrames(got, frames...))
	got = must(b.AppendTerminate(got))
	if len(got) != len(want)+1 || got[0] != prefix {
		t.Fatalf("got %d bursts after the prefix %x, want %d", len(got)-1, got[0], len(want))
	}
	for i := range want {
		if got[i+1] != want[i] {
			t.Errorf("burst %d: %x, want %x", i, got[i+1], want[i])
		}
	}
	if out, err := b.AppendSlots(got[:1], slots...); !errors.Is(err, layer2.ErrVoiceTerminated) || len(out) != 1 {
		t.Errorf("AppendSlots after Terminate: %d bursts, %v", len(out), err)
	}
	if out, err := b.AppendTerminate(nil); !errors.Is(err, layer2.ErrVoiceTerminated) || out != nil {
		t.Errorf("AppendTerminate after Terminate: %d bursts, %v", len(out), err)
	}
}

func TestVoiceTransmitter_AddBurst(t *testing.T) {
	t.Parallel()

	opts := &layer2.VoiceTransmitterOptions{LC: groupLC(enums.FIDStandardized), ColorCode: 1, Source: layer2.SyncSourceBS, Silence: silenceSlot()}
	for _, lead := range []int{0, 1, 2} {
		a := must(layer2.NewVoiceTransmitter(opts))
		b := must(layer2.NewVoiceTransmitter(opts))
		var want, got []layer2.RawBurst
		for i := range lead {
			want = must(a.AppendSlots(want, testSlot(100+i)))
			got = must(b.AppendSlots(got, testSlot(100+i)))
		}
		for n := range 14 {
			var slots [layer2.VoiceSlotsPerBurst][9]byte
			for i := range slots {
				slots[i] = testSlot(3*n + i)
			}
			want = must(a.AppendSlots(want, slots[:]...))
			got = append(got, must(b.AddBurst(slots)))
		}
		if len(got) != len(want) {
			t.Fatalf("lead %d: %d bursts, want %d", lead, len(got), len(want))
		}
		for i := range want {
			if got[i] != want[i] {
				t.Errorf("lead %d: burst %d differs", lead, i)
			}
		}
		term := b.Terminator()
		tail := must(b.Terminate())
		if tail[len(tail)-1] != term {
			t.Errorf("lead %d: Terminator is not the last burst of Terminate", lead)
		}
		if _, err := b.AddBurst([layer2.VoiceSlotsPerBurst][9]byte{}); !errors.Is(err, layer2.ErrVoiceTerminated) {
			t.Errorf("lead %d: AddBurst after Terminate: %v", lead, err)
		}
	}
}

func TestVoiceTransmitter_ReverseChannel(t *testing.T) {
	t.Parallel()

	lc := groupLC(enums.FIDStandardized)
	tx := must(layer2.NewVoiceTransmitter(&layer2.VoiceTransmitterOptions{LC: lc, ColorCode: 2, Source: layer2.SyncSourceBS, Silence: silenceSlot()}))
	if err := tx.SetReverseChannel(pdu.ReverseChannel{RCCommand: enums.RCCeaseTransmissionCommand}); err != nil {
		t.Fatal(err)
	}
	var bursts []layer2.RawBurst
	for n := 0; n < 12*3; n++ {
		bursts = append(bursts, must(tx.AddSlots(testSlot(n)))...)
	}
	f := must(layer2.NewBurstFromBytes(bursts[5]))
	emb := f.EmbeddedSignalling
	if !f.HasReverseChannel() || !emb.PreemptionAndPowerControlIndicator || emb.LCSS != enums.LCSSSingleFragment || emb.ColorCode != 2 {
		t.Fatalf("first burst F: %s", f.String())
	}
	if f.ReverseChannel.RCCommand != enums.RCCeaseTransmissionCommand || f.FEC.RC.HasErrors() {
		t.Errorf("RC %s", f.ReverseChannel.String())
	}
	next := must(layer2.NewBurstFromBytes(bursts[11]))
	if next.HasReverseChannel() || next.EmbeddedSignallingData != [4]byte{} {
		t.Errorf("second burst F: %s", next.String())
	}

	for _, source := range []layer2.SyncSource{layer2.SyncSourceMS, layer2.SyncSourceDirectTS1, layer2.SyncSourceDirectTS2} {
		tx := must(layer2.NewVoiceTransmitter(&layer2.VoiceTransmitterOptions{LC: lc, ColorCode: 2, Source: source, Silence: silenceSlot()}))
		if err := tx.SetReverseChannel(pdu.ReverseChannel{}); err == nil {
			t.Errorf("source %d: reverse channel accepted", source)
		}
	}
	if err := tx.SetReverseChannel(pdu.ReverseChannel{RCCommand: enums.RCCommand(16)}); err == nil {
		t.Error("RC command 16 accepted")
	}
}

func TestVoiceTransmitter_Terminate(t *testing.T) {
	t.Parallel()

	lc := groupLC(enums.FIDStandardized)

	tx := must(layer2.NewVoiceTransmitter(&layer2.VoiceTransmitterOptions{LC: lc, ColorCode: 1, Source: layer2.SyncSourceMS, Silence: silenceSlot()}))
	tail := must(tx.Terminate())
	if len(tail) != 1 {
		t.Fatalf("empty transmission: %d bursts, want the terminator alone", len(tail))
	}
	checkLCBurst(t, tail[0], lc, enums.DataTypeTerminatorWithLC, 1, enums.SyncMSSourcedData)
	if _, err := tx.Terminate(); !errors.Is(err, layer2.ErrVoiceTerminated) {
		t.Errorf("second Terminate: %v", err)
	}
	if _, err := tx.AddSlots(testSlot(0)); !errors.Is(err, layer2.ErrVoiceTerminated) {
		t.Errorf("AddSlots after Terminate: %v", err)
	}
	if err := tx.SetReverseChannel(pdu.ReverseChannel{}); !errors.Is(err, layer2.ErrVoiceTerminated) {
		t.Errorf("SetReverseChannel after Terminate: %v", err)
	}

	tx = must(layer2.NewVoiceTransmitter(&layer2.VoiceTransmitterOptions{LC: lc, ColorCode: 1, Source: layer2.SyncSourceMS, Silence: silenceSlot()}))
	slots := make([][9]byte, 18)
	if n := len(must(tx.AddSlots(slots...))); n != 6 {
		t.Fatalf("%d bursts from 18 slots", n)
	}
	if n := len(must(tx.Terminate())); n != 1 {
		t.Errorf("Terminate after a full superframe returned %d bursts, want the terminator alone", n)
	}
}

func TestVoiceTransmitter_RejectsBadInput(t *testing.T) {
	t.Parallel()

	lc := groupLC(enums.FIDStandardized)
	_, err := layer2.NewVoiceTransmitter(&layer2.VoiceTransmitterOptions{LC: lc, ColorCode: 0x17, Source: layer2.SyncSourceBS, Silence: silenceSlot()})
	var rangeErr *dmrgo.FieldRangeError
	if !errors.As(err, &rangeErr) || rangeErr.Field != "ColorCode" {
		t.Errorf("color code 0x17: error %v, want the FieldRangeError of ColorCode", err)
	}
	if _, err := layer2.NewVoiceTransmitter(&layer2.VoiceTransmitterOptions{LC: lc, ColorCode: 1, Source: layer2.SyncSource(4), Silence: silenceSlot()}); err == nil {
		t.Error("sync source 4 accepted")
	}
	lc.FLCO = enums.FLCO(64)
	if _, err := layer2.NewVoiceTransmitter(&layer2.VoiceTransmitterOptions{LC: lc, ColorCode: 1, Source: layer2.SyncSourceBS, Silence: silenceSlot()}); err == nil {
		t.Error("FLCO 64 accepted")
	}
}

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
		case (b.Kind == layer2.KindData):
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

func TestVoiceTransmitter_ReproducesCaptures(t *testing.T) {
	t.Parallel()

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
			t.Parallel()

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
			tx := must(layer2.NewVoiceTransmitter(&layer2.VoiceTransmitterOptions{LC: *lc, ColorCode: cc, Source: layer2.SyncSourceBS, Silence: silenceSlot()}))
			frames := must(layer2.NewVoiceTransmitter(&layer2.VoiceTransmitterOptions{LC: *lc, ColorCode: cc, Source: layer2.SyncSourceBS, Silence: silenceSlot()}))
			if tx.Header() != raws[0] {
				t.Errorf("header\ngot  %x\nwant %x", tx.Header(), raws[0])
			}
			for _, i := range kept {
				slots := bursts[i].VoiceSlots
				got := must(tx.AddSlots(slots[:]...))
				if len(got) != 1 || got[0] != raws[i] {
					t.Errorf("burst %d\ngot  %x\nwant %x", i, got, raws[i])
				}

				decoded, voiceFEC := bursts[i].AMBEFrames()
				fromFrames := must(frames.AddFrames(decoded[:]...))
				if len(fromFrames) != 1 {
					t.Fatalf("burst %d: %d bursts from frames", i, len(fromFrames))
				}
				if got := must(bursts[i].Encode()); got != raws[i] {
					t.Errorf("burst %d: decode then Encode changed the burst\ngot  %x\nwant %x", i, got, raws[i])
				}
				if n := voiceFEC.ErrorsCorrected; n != 0 {
					regen := must(layer2.NewBurstFromBytes(fromFrames[0]))
					if f, r := regen.AMBEFrames(); f != decoded || r.ErrorsCorrected != 0 {
						t.Errorf("burst %d: the frame path did not regenerate clean frames", i)
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

func TestSyncSourceOf(t *testing.T) {
	t.Parallel()

	for _, src := range []layer2.SyncSource{layer2.SyncSourceBS, layer2.SyncSourceMS, layer2.SyncSourceDirectTS1, layer2.SyncSourceDirectTS2} {
		voice, data, _ := src.SyncPatterns()
		for p, kind := range map[enums.SyncPattern]layer2.Kind{voice: layer2.KindVoice, data: layer2.KindData} {
			gotSrc, gotKind, ok := layer2.SyncSourceOf(p)
			if !ok || gotSrc != src || gotKind != kind {
				t.Errorf("SyncSourceOf(%s) = %d, %s, %v; want %d, %s", p, gotSrc, gotKind, ok, src, kind)
			}
		}
	}
	if src, k, ok := layer2.SyncSourceOf(enums.SyncMSSourcedRC); !ok || src != layer2.SyncSourceMS || k != layer2.KindReverseChannel {
		t.Errorf("SyncSourceOf(RC) = %d, %s, %v", src, k, ok)
	}
	for _, p := range []enums.SyncPattern{enums.SyncNone, enums.SyncReserved, 0x123456789ABC} {
		if _, k, ok := layer2.SyncSourceOf(p); ok || k != layer2.KindUnknown {
			t.Errorf("SyncSourceOf(%s) = %s, %v", p, k, ok)
		}
	}
}
