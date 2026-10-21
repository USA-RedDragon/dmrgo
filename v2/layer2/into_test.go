package layer2_test

import (
	"errors"
	"reflect"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
	"github.com/USA-RedDragon/dmrgo/v2/layer3"
)

func TestReceiveInto_MatchesReceive(t *testing.T) {
	t.Parallel()

	lc := must(pdu.NewVoiceLC(true, 91, 3191868, layer3.MotorolaServiceOptions()))
	streams := [][]layer2.RawBurst{
		transmission(t, lc, 12),
		loadBursts(t, "testdata/voice.bin"),
		loadBursts(t, "testdata/parrot_kerchunk.bin"),
	}
	for n, raws := range streams {
		var byValue, into layer2.VoiceReceiver
		var dst pdu.FullLinkControl
		var p pdu.FullLinkControlPayloads
		var lcs int
		for i, raw := range raws {
			b1, _ := layer2.NewBurstFromBytes(raw)
			b2, _ := layer2.NewBurstFromBytes(raw)
			want, wantOK, wantErr := byValue.Receive(b1)
			gotOK, gotErr := into.ReceiveInto(&dst, &p, b2)
			if gotOK != wantOK || !errors.Is(gotErr, wantErr) || b1.VoiceBurst != b2.VoiceBurst {
				t.Fatalf("stream %d burst %d: ReceiveInto %t %v %s, Receive %t %v %s", n, i, gotOK, gotErr, b2.VoiceBurst, wantOK, wantErr, b1.VoiceBurst)
			}
			if wantOK {
				lcs++
				if !reflect.DeepEqual(dst, want) {
					t.Errorf("stream %d burst %d: ReceiveInto %s, Receive %s", n, i, dst.String(), want.String())
				}
			}
		}
		if lcs == 0 {
			t.Errorf("stream %d: no embedded LC", n)
		}
	}
}

func TestDecodeEmbeddedFullLinkControlInto_MatchesValue(t *testing.T) {
	t.Parallel()

	gv := must(pdu.NewVoiceLC(true, 91, 3191868, layer3.ServiceOptions{}))
	uu := must(pdu.NewVoiceLC(false, 3140598, 3191868, layer3.ServiceOptions{IsPrivacy: true}))
	ta := pdu.FullLinkControl{FLCO: enums.FLCOTalkerAliasBlock1, TalkerAliasBlock: &pdu.FLCTalkerAliasBlock{TalkerAliasData: [7]byte{'D', 'M', 'R', 'G', 'O'}}}
	var dst pdu.FullLinkControl
	var p pdu.FullLinkControlPayloads
	for _, lc := range []pdu.FullLinkControl{gv, uu, ta} {
		f := must(layer2.EncodeEmbeddedFullLinkControl(&lc))
		for _, strict := range []bool{false, true} {
			want := layer2.DecodeEmbeddedFullLinkControl(f, strict)
			layer2.DecodeEmbeddedFullLinkControlInto(&dst, &p, f, strict)
			if !reflect.DeepEqual(dst, want) {
				t.Errorf("%s strict %t: Into %s, value %s", lc.String(), strict, dst.String(), want.String())
			}
		}
	}
}

func TestShortLCAssembler_AddInto_MatchesAdd(t *testing.T) {
	t.Parallel()

	data := [3]byte{0x49, 0x24, 0x92}
	fragments := buildValidShortLCFragments(byte(enums.SLCOActivityUpdate), data)
	cachs := make([]layer2.CACH, 0, 6)
	for i, f := range fragments {
		cachs = append(cachs, layer2.CACH{TACT: pdu.TACT{LCSS: shortLCSequence[i]}, Payload: f})
	}
	bad := layer2.CACH{TACT: pdu.TACT{LCSS: enums.LCSSFirstFragment}}
	bad.TACT.FEC.Uncorrectable = true
	cachs = append(cachs, cachs[0], bad)

	var byValue, into layer2.ShortLCAssembler
	var dst pdu.ShortLC
	var p pdu.ShortLCPayloads
	var done int
	for i, c := range cachs {
		want, wantDone, wantErr := byValue.Add(c)
		gotDone, gotErr := into.AddInto(&dst, &p, c)
		if gotDone != wantDone || !errors.Is(gotErr, wantErr) {
			t.Fatalf("CACH %d: AddInto %t %v, Add %t %v", i, gotDone, gotErr, wantDone, wantErr)
		}
		if wantDone {
			done++
			if !reflect.DeepEqual(dst, want) {
				t.Errorf("CACH %d: AddInto %s, Add %s", i, dst.String(), want.String())
			}
		}
	}
	if done != 1 {
		t.Errorf("%d Short LCs, want 1", done)
	}
}
