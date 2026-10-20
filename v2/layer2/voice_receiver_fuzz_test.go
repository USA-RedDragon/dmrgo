package layer2_test

import (
	"errors"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
	"github.com/USA-RedDragon/dmrgo/v2/layer3"
	"github.com/USA-RedDragon/dmrgo/v2/vocoder"
)

type voiceFuzzInput struct {
	opts    layer2.VoiceTransmitterOptions
	aliases []pdu.FullLinkControl
	slots   [][9]byte
	actions []byte
	junk    []byte
}

func parseVoiceFuzz(data []byte) (in voiceFuzzInput, ok bool) {
	next := func() byte {
		if len(data) == 0 {
			return 0
		}
		b := data[0]
		data = data[1:]
		return b
	}
	flags, cc, so := next(), next(), next()
	target := uint32(next())<<16 | uint32(next())<<8 | uint32(next())
	source := uint32(next())<<16 | uint32(next())<<8 | uint32(next())
	lc, err := pdu.NewVoiceLC(flags&1 != 0, target, source, layer3.DecodeServiceOptions([1]byte{so}))
	if err != nil {
		return in, false
	}
	in.opts = layer2.VoiceTransmitterOptions{LC: lc, ColorCode: enums.ColorCode(cc & 15),
		Source: layer2.SyncSource(flags >> 1 & 3), Silence: vocoder.AMBESilenceSlot()}
	alias := make([]byte, next()&0x1F)
	for i := range alias {
		alias[i] = 0x20 + next()%0x5F
	}
	if len(alias) > 0 {
		if in.aliases, err = pdu.EncodeTalkerAlias(string(alias), layer3.TalkerAliasDataFormat(flags>>3&3)); err != nil {
			return in, false
		}
	}
	in.slots = make([][9]byte, next())
	for i := range in.slots {
		for j := range in.slots[i] {
			in.slots[i][j] = next()
		}
	}
	n := int(next())
	in.actions = make([]byte, n)
	for i := range in.actions {
		in.actions[i] = next()
	}
	in.junk = data
	return in, true
}

func (in *voiceFuzzInput) embeddedOf(n int) *pdu.FullLinkControl {
	if len(in.aliases) == 0 || n%2 == 0 {
		return nil
	}
	return &in.aliases[n/2%len(in.aliases)]
}

func FuzzVoiceReceiver(f *testing.F) {
	for _, s := range voiceReceiverSeeds() {
		f.Add(s)
	}
	f.Fuzz(runVoiceReceiver)
}

func voiceReceiverSeeds() [][]byte {
	call := []byte{0x01, 0x01, 0x00, 0x00, 0x0C, 0x1C, 0x30, 0xB4, 0x3C}
	slots := make([]byte, 0, 1+36*9)
	slots = append(slots, 36)
	for i := range 36 * 9 {
		slots = append(slots, byte(i*7))
	}
	lossy := append(append(append([]byte{}, call...), 6, 'K', 'I', '5', 'V', 'M', 'F'), slots...)
	lossy = append(lossy, 20, 0x45, 0x7F, 0, 0x80, 0, 0x41, 0x52, 0, 0, 0xC0, 0, 0x63, 0, 0, 0x80, 0x80, 0, 0x4C, 0, 0)
	return [][]byte{
		append(append(append([]byte{}, call...), 0), slots...),
		append(append(append([]byte{}, call...), 6, 'K', 'I', '5', 'V', 'M', 'F'), slots...),
		append(lossy, make([]byte, 33)...),
	}
}

func runVoiceReceiver(t *testing.T, data []byte) {
	in, ok := parseVoiceFuzz(data)
	if !ok {
		return
	}
	in.opts.EmbeddedLC = in.embeddedOf
	tx, err := layer2.NewVoiceTransmitter(&in.opts)
	if err != nil {
		t.Fatalf("NewVoiceTransmitter: %v", err)
	}
	bursts := tx.AppendHeaders(nil)
	if bursts, err = tx.AppendSlots(bursts, in.slots...); err != nil {
		t.Fatal(err)
	}
	if bursts, err = tx.AppendTerminate(bursts); err != nil {
		t.Fatal(err)
	}
	checkVoiceReceiver(t, &in, bursts)
}

type voiceTrack struct {
	clean     bool
	fragments int
}

var broken = voiceTrack{fragments: -1}

func checkVoiceReceiver(t *testing.T, in *voiceFuzzInput, bursts []layer2.RawBurst) {
	t.Helper()
	var rx layer2.VoiceReceiver
	var lc pdu.FullLinkControl
	var p pdu.FullLinkControlPayloads
	b := layer2.NewBurst()
	var tr voiceTrack
	voiceIdx := -1
	junk := in.junk
	for i := range bursts {
		action := byte(0)
		if i < len(in.actions) {
			action = in.actions[i]
		}
		raw := bursts[i]
		pos := enums.VoiceBurstUnknown
		if i > 0 && i < len(bursts)-1 {
			voiceIdx++
			pos, _ = enums.VoiceBurstAt(voiceIdx % layer2.VoiceBurstsPerSuperframe)
		}
		switch action & 0xC0 {
		case 0x80:
			tr = broken
			continue
		case 0xC0:
			n := copy(raw[:], junk)
			junk = junk[n:]
			if err := b.DecodeFromBytes(raw); err != nil {
				continue
			}
			_, _ = rx.ReceiveInto(&lc, &p, b)
			tr = broken
			continue
		case 0x40:
			bit := damagedBit(pos, int(action&0x3F))
			raw[bit/8] ^= 0x80 >> (bit % 8)
		}
		if err := b.DecodeFromBytes(raw); err != nil {
			t.Fatalf("burst %d %x: %v", i, raw, err)
		}
		if pos == enums.VoiceBurstUnknown {
			if b.Kind != layer2.KindData {
				t.Fatalf("burst %d: kind %v, sent a data burst", i, b.Kind)
			}
			if done, err := rx.ReceiveInto(&lc, &p, b); done || err != nil && !errors.Is(err, layer2.ErrEmbeddedLCSequence) {
				t.Fatalf("burst %d: data burst done %v err %v", i, done, err)
			}
			tr = voiceTrack{}
			continue
		}
		if b.Kind != layer2.KindVoice || b.VoiceSlots != voiceSlotsOf(in, voiceIdx) {
			t.Fatalf("burst %d: kind %v slots %x, sent %v %x", i, b.Kind, b.VoiceSlots, pos, voiceSlotsOf(in, voiceIdx))
		}
		done, err := rx.ReceiveInto(&lc, &p, b)
		checkPosition(t, i, b.VoiceBurst, pos, tr.clean)
		if pos == enums.VoiceBurstA || pos == enums.VoiceBurstB || pos == enums.VoiceBurstE {
			tr.clean = true
		}
		switch {
		case pos == enums.VoiceBurstB:
			tr.fragments = 1
		case tr.fragments < 0:
			continue
		case pos >= enums.VoiceBurstC && pos <= enums.VoiceBurstE && tr.fragments > 0:
			tr.fragments++
		default:
			tr.fragments = 0
		}
		want := pos == enums.VoiceBurstE && tr.fragments == 4
		if done != want || want && err != nil {
			t.Fatalf("burst %d %v: embedded LC done %v err %v, want done %v", i, pos, done, err, want)
		}
		if done {
			checkEmbeddedLC(t, i, in, voiceIdx/layer2.VoiceBurstsPerSuperframe, &lc)
		}
	}
}

func damagedBit(pos enums.VoiceBurstType, k int) int {
	switch {
	case pos == enums.VoiceBurstA:
		return 108 + k%48
	case pos != enums.VoiceBurstUnknown && k%16 < 8:
		return 108 + k%16
	case pos != enums.VoiceBurstUnknown:
		return 148 + k%16 - 8
	case k*3 < 98:
		return k * 3
	}
	return 166 + k*3 - 98
}

func checkPosition(t *testing.T, i int, got, want enums.VoiceBurstType, clean bool) {
	t.Helper()
	switch {
	case want == enums.VoiceBurstA || want == enums.VoiceBurstB || want == enums.VoiceBurstE || clean:
		if got != want {
			t.Fatalf("burst %d: position %v, sent %v", i, got, want)
		}
	case want == enums.VoiceBurstF:
		if got != want && got != enums.VoiceBurstUnknown {
			t.Fatalf("burst %d: position %v, sent F", i, got)
		}
	case got != enums.VoiceBurstC && got != enums.VoiceBurstD && got != enums.VoiceBurstUnknown:
		t.Fatalf("burst %d: position %v, sent %v", i, got, want)
	}
}

func voiceSlotsOf(in *voiceFuzzInput, n int) [layer2.VoiceSlotsPerBurst][9]byte {
	var s [layer2.VoiceSlotsPerBurst][9]byte
	for i := range s {
		s[i] = vocoder.AMBESilenceSlot()
		if k := n*layer2.VoiceSlotsPerBurst + i; k < len(in.slots) {
			s[i] = in.slots[k]
		}
	}
	return s
}

func checkEmbeddedLC(t *testing.T, i int, in *voiceFuzzInput, n int, lc *pdu.FullLinkControl) {
	t.Helper()
	want := in.embeddedOf(n)
	if want == nil {
		want = &in.opts.LC
	}
	got, err := lc.Bytes()
	if err != nil {
		t.Fatalf("burst %d: embedded LC %+v: %v", i, lc, err)
	}
	sent, err := want.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	if got != sent {
		t.Fatalf("burst %d: embedded LC %x, sent %x in superframe %d", i, got, sent, n)
	}
}
