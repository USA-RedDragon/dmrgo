package pdu_test

import (
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
	"github.com/USA-RedDragon/dmrgo/v2/layer3"
)

const (
	maxBoundsAlias = 64
	maxBoundsRaw   = 14 * 9
)

func runTalkerAliasBounds(t *testing.T, alias string, raw []byte) {
	t.Helper()
	var a pdu.TalkerAliasAssembler
	check := func(lc *pdu.FullLinkControl) {
		s, _, _ := a.Add(lc)
		if len(s) > pdu.TalkerAliasMaxText {
			t.Fatalf("Add returned %d bytes", len(s))
		}
		b, _, _ := a.AppendAdd(nil, lc)
		if len(b) > pdu.TalkerAliasMaxText {
			t.Fatalf("AppendAdd returned %d bytes", len(b))
		}
	}
	for _, f := range []layer3.TalkerAliasDataFormat{0, 1, 2, 3} {
		lcs, err := pdu.EncodeTalkerAlias(alias, f)
		if err != nil {
			continue
		}
		for i := range lcs {
			check(&lcs[i])
		}
	}
	for off := 0; off+9 <= len(raw); off += 9 {
		lc := pdu.DecodeFullLinkControlBytes([9]byte(raw[off : off+9]))
		check(&lc)
	}
}

func TestTalkerAliasBounds(t *testing.T) {
	t.Parallel()
	runTalkerAliasBounds(t, "dmrgo test alias", nil)
	runTalkerAliasBounds(t, "0123456789012345678901234567890123456789012345678901234567890123456789", nil)
}

func FuzzTalkerAliasBounds(f *testing.F) {
	f.Add("N0CALL", []byte{0x04, 0x00, 0xff, 0x41, 0x42, 0x43, 0x44, 0x45, 0x46})
	f.Add("ä alias", []byte{0x05, 0x00, 0x61, 0x62, 0x63, 0x64, 0x65, 0x66, 0x67, 0x06, 0x00, 1, 2, 3, 4, 5, 6, 7})
	f.Fuzz(func(t *testing.T, alias string, raw []byte) {
		runTalkerAliasBounds(t, alias[:min(len(alias), maxBoundsAlias)], raw[:min(len(raw), maxBoundsRaw)])
	})
}
