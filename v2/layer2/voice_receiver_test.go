package layer2_test

import (
	"bytes"
	"errors"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/fec"
	"github.com/USA-RedDragon/dmrgo/v2/internal/unpacked"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

func transmission(t *testing.T, lc pdu.FullLinkControl, n int) []layer2.RawBurst {
	t.Helper()
	tx := must(layer2.NewVoiceTransmitter(&layer2.VoiceTransmitterOptions{LC: lc, ColorCode: 3, Source: layer2.SyncSourceMS, Silence: silenceSlot()}))
	out := []layer2.RawBurst{tx.Header()}
	for i := 0; i < 3*n; i++ {
		out = append(out, must(tx.AddSlots(testSlot(i)))...)
	}
	return append(out, must(tx.Terminate())...)
}

type received struct {
	positions []enums.VoiceBurstType
	lcs       map[int]pdu.FullLinkControl
	errs      map[int]error
	badLCs    map[int]pdu.FullLinkControl
}

func receiveAll(t *testing.T, raws []layer2.RawBurst) received {
	t.Helper()
	var r layer2.VoiceReceiver
	got := received{lcs: map[int]pdu.FullLinkControl{}, errs: map[int]error{}, badLCs: map[int]pdu.FullLinkControl{}}
	for i, raw := range raws {
		b, _ := layer2.NewBurstFromBytes(raw)
		lc, ok, err := r.Receive(b, nil)
		got.positions = append(got.positions, b.VoiceBurst)
		if ok {
			got.lcs[i] = lc
		}
		if err != nil {
			got.errs[i] = err
		}
		if errors.Is(err, layer2.ErrEmbeddedLCUncorrectable) {
			got.badLCs[i] = lc
		}
	}
	return got
}

func positions(s string) []enums.VoiceBurstType {
	var out []enums.VoiceBurstType
	for _, c := range s {
		if c == '-' {
			out = append(out, enums.VoiceBurstUnknown)
			continue
		}
		out = append(out, enums.VoiceBurstA+enums.VoiceBurstType(c-'A'))
	}
	return out
}

func checkPositions(t *testing.T, got []enums.VoiceBurstType, want string) {
	t.Helper()
	w := positions(want)
	if len(got) != len(w) {
		t.Fatalf("%d positions, want %d", len(got), len(w))
	}
	for i := range w {
		if got[i] != w[i] {
			t.Errorf("burst %d: position %s, want %s", i, got[i].String(), w[i].String())
		}
	}
}

func TestVoiceReceiver_Transmission(t *testing.T) {
	t.Parallel()

	lc := groupLC(enums.FID(0xC2))
	raws := transmission(t, lc, 14)
	got := receiveAll(t, raws)
	checkPositions(t, got.positions, "-ABCDEFABCDEFABCDEF-")
	if len(got.errs) != 0 {
		t.Errorf("errors %v", got.errs)
	}
	if len(got.lcs) != 3 {
		t.Fatalf("LCs at %v, want bursts 5, 11 and 17", got.lcs)
	}
	for i, l := range got.lcs {
		if (i-5)%6 != 0 || l.FEC.HasErrors() || l.FID != 0xC2 ||
			lcBits(t, l, enums.DataTypeVoiceLCHeader) != lcBits(t, lc, enums.DataTypeVoiceLCHeader) {
			t.Errorf("burst %d: LC %s", i, l.String())
		}
	}
}

func TestVoiceReceiver_LateEntry(t *testing.T) {
	t.Parallel()

	raws := transmission(t, groupLC(enums.FIDStandardized), 12)[3:]
	got := receiveAll(t, raws)
	checkPositions(t, got.positions, "--EFABCDEF-")
	if len(got.errs) != 0 || len(got.lcs) != 1 {
		t.Errorf("LCs %v, errors %v; want one LC at burst 8", got.lcs, got.errs)
	}
	if _, ok := got.lcs[8]; !ok {
		t.Errorf("no LC at burst 8")
	}
}

func TestVoiceReceiver_LostBurst(t *testing.T) {
	t.Parallel()

	raws := transmission(t, groupLC(enums.FIDStandardized), 12)
	raws = append(raws[:3:3], raws[4:]...)
	got := receiveAll(t, raws)
	checkPositions(t, got.positions, "-ABCEFABCDEF-")
	if len(got.lcs) != 1 {
		t.Errorf("LCs at %v, want burst 10 only", got.lcs)
	}
	if !errors.Is(got.errs[4], layer2.ErrEmbeddedLCSequence) || len(got.errs) != 1 {
		t.Errorf("errors %v, want ErrEmbeddedLCSequence at burst 4", got.errs)
	}
}

func TestVoiceReceiver_UncorrectableLC(t *testing.T) {
	t.Parallel()

	raws := transmission(t, groupLC(enums.FIDStandardized), 6)
	all := [264]unpacked.Bit(unpacked.Unpack(raws[3][:]))
	for i := layer2.EmbeddedDataStart; i < layer2.EmbeddedDataStart+12; i++ {
		all[i] ^= 1
	}
	raws[3] = [33]byte(unpacked.Pack(all[:]))
	got := receiveAll(t, raws)
	checkPositions(t, got.positions, "-ABCDEF-")
	if !errors.Is(got.errs[5], layer2.ErrEmbeddedLCUncorrectable) || !errors.Is(got.errs[5], fec.ErrUncorrectable) || len(got.lcs) != 0 {
		t.Errorf("LCs %v, errors %v; want ErrEmbeddedLCUncorrectable at burst 5", got.lcs, got.errs)
	}
	if lc, ok := got.badLCs[5]; !ok || !lc.FEC.Uncorrectable {
		t.Errorf("uncorrectable embedded LC %+v: want the decoded LC with FEC.Uncorrectable", lc)
	}
}

func TestVoiceReceiver_EmbeddedLCBitError(t *testing.T) {
	t.Parallel()

	lc := groupLC(enums.FIDStandardized)
	raws := transmission(t, lc, 6)
	all := [264]unpacked.Bit(unpacked.Unpack(raws[2][:]))
	all[layer2.EmbeddedDataStart+7] ^= 1
	raws[2] = [33]byte(unpacked.Pack(all[:]))
	var r layer2.VoiceReceiver
	var lcs, uncorrectable int
	for _, raw := range raws {
		b, _ := layer2.NewBurstFromBytes(raw)
		_, ok, err := r.Receive(b, nil)
		if ok {
			lcs++
		}
		if errors.Is(err, layer2.ErrEmbeddedLCUncorrectable) {
			uncorrectable++
		}
	}
	if lcs != 1 || uncorrectable != 0 {
		t.Errorf("%d LCs, %d uncorrectable", lcs, uncorrectable)
	}
}

func TestVoiceReceiver_SequenceOverWrongEMB(t *testing.T) {
	t.Parallel()

	raws := transmission(t, groupLC(enums.FIDStandardized), 6)
	c := raws[3]
	emb := c.EMB()
	emb.LCSS = enums.LCSSFirstFragment
	if err := c.SetEMB(emb); err != nil {
		t.Fatal(err)
	}
	seq := []layer2.RawBurst{raws[1], raws[2], c, raws[4], raws[5], raws[6]}
	var r layer2.VoiceReceiver
	got := make([]enums.VoiceBurstType, 0, len(seq))
	lcs := 0
	for i, raw := range seq {
		b := must(layer2.NewBurstFromBytes(raw))
		h := layer2.Hint{Seq: uint16(10 + i), HasSeq: true}
		_, ok, err := r.Receive(b, &h)
		if err != nil {
			t.Errorf("burst %d: %v", i, err)
		}
		if ok {
			lcs++
		}
		got = append(got, b.VoiceBurst)
	}
	checkPositions(t, got, "ABCDEF")
	if lcs != 1 {
		t.Errorf("%d LCs, want 1", lcs)
	}
}

func TestVoiceReceiver_DataBurstResets(t *testing.T) {
	t.Parallel()

	raws := transmission(t, groupLC(enums.FIDStandardized), 6)
	seq := []layer2.RawBurst{raws[1], raws[2], raws[7], raws[3], raws[6]}
	got := receiveAll(t, seq)
	checkPositions(t, got.positions, "AB--F")
	if !errors.Is(got.errs[2], layer2.ErrEmbeddedLCSequence) || len(got.errs) != 1 {
		t.Errorf("errors %v, want ErrEmbeddedLCSequence at the terminator", got.errs)
	}
}

func TestVoiceReceiver_UncorrectableEMB(t *testing.T) {
	t.Parallel()

	raws := transmission(t, groupLC(enums.FIDStandardized), 6)
	all := [264]unpacked.Bit(unpacked.Unpack(raws[5][:]))
	for i := layer2.EMBFirstStart; i < layer2.EMBFirstStart+3; i++ {
		all[i] ^= 1
	}
	raws[5] = [33]byte(unpacked.Pack(all[:]))
	if b := must(layer2.NewBurstFromBytes(raws[5])); !b.HasEmbeddedSignalling() || !b.EmbeddedSignalling.FEC.Uncorrectable {
		t.Fatalf("corrupted EMB decodes as %s", b.String())
	}
	got := receiveAll(t, raws)
	checkPositions(t, got.positions, "-ABCDEF-")
	if _, ok := got.lcs[5]; !ok || len(got.errs) != 0 {
		t.Errorf("LCs %v, errors %v; want an LC at burst 5", got.lcs, got.errs)
	}
}

func TestVoiceReceiver_Captures(t *testing.T) {
	t.Parallel()

	tests := []struct {
		file      string
		positions string
		lcs       int
	}{
		{"testdata/voice.bin", "-" + "ABCDEF" + "ABCDEF" + "ABCDEF" + "ABCDEF" + "ABCDEF" + "ABCDEF" + "ABCDEF" + "ABCDEF" +
			"BCDEF" + "ABCDEF" + "ABCDEF" + "ABCDEF" + "ABCDEF" + "-", 13},
		{"testdata/parrot_kerchunk.bin", "-ABCDEFAABCDEF-", 2},
	}
	for _, tt := range tests {
		t.Run(tt.file, func(t *testing.T) {
			t.Parallel()

			got := receiveAll(t, loadBursts(t, tt.file))
			checkPositions(t, got.positions, tt.positions)
			if len(got.errs) != 0 {
				t.Errorf("errors %v", got.errs)
			}
			if len(got.lcs) != tt.lcs {
				t.Errorf("%d LCs, want %d", len(got.lcs), tt.lcs)
			}
			for i, lc := range got.lcs {
				if got.positions[i] != enums.VoiceBurstE || lc.FEC.HasErrors() {
					t.Errorf("burst %d: LC at %s, FEC %+v", i, got.positions[i].String(), lc.FEC)
				}
				info := lcBits(t, lc, enums.DataTypeVoiceLCHeader)
				if !bytes.Equal(info[:9], capturedEmbeddedLC) {
					t.Errorf("burst %d: LC % X, want % X", i, info[:9], capturedEmbeddedLC)
				}
				target, source, ok := lc.Addresses()
				if !ok || target != 9990 || source != 3191868 {
					t.Errorf("burst %d: %s", i, lc.String())
				}
			}
		})
	}
}

func TestVoiceReceiver_IgnoresStandaloneRC(t *testing.T) {
	t.Parallel()

	raws := transmission(t, groupLC(enums.FIDStandardized), 6)
	rc := buildStandaloneRCBurst(3, enums.RCCeaseTransmissionCommand)
	seq := []layer2.RawBurst{raws[1], raws[2], rc, raws[3], raws[4], raws[5]}
	got := receiveAll(t, seq)
	checkPositions(t, got.positions, "AB-CDE")
	if len(got.lcs) != 1 || len(got.errs) != 0 {
		t.Errorf("LCs %v, errors %v", got.lcs, got.errs)
	}
}
