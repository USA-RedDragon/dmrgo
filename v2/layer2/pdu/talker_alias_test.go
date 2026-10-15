package pdu_test

import (
	"errors"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/bit"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
	"github.com/USA-RedDragon/dmrgo/v2/layer3"
)

func airRoundTrip(t *testing.T, lc pdu.FullLinkControl) pdu.FullLinkControl {
	t.Helper()
	lc.DataType = enums.DataTypeVoiceLCHeader
	bits, err := pdu.EncodeFullLinkControl(&lc)
	if err != nil {
		t.Fatal(err)
	}
	got, res := pdu.DecodeFullLinkControl(bits, enums.DataTypeVoiceLCHeader)
	if res.Uncorrectable {
		t.Fatal("uncorrectable")
	}
	return got
}

func TestTalkerAlias_RoundTrip(t *testing.T) {
	t.Parallel()
	tests := []struct {
		alias  string
		format layer3.TalkerAliasDataFormat
		blocks int
	}{
		{"", layer3.SevenBitCharacters, 0},
		{"DL1ABC", layer3.SevenBitCharacters, 0},
		{"KI5VMF J", layer3.SevenBitCharacters, 1},
		{"0123456789ABCDEFGHIJKLMNOPQRSTU", layer3.SevenBitCharacters, 3},
		{"Müller", layer3.ISOEightBitCharacters, 0},
		{"N0CALL Jürgen Brück Ñ", layer3.ISOEightBitCharacters, 3},
		{"日本語", layer3.UTF8Characters, 1},
		{"ABCDEFGHIJKLMNOPQRSTUVWXYZ0", layer3.UTF8Characters, 3},
		{"АБВ", layer3.UTF16Characters, 0},
		{"Ω KI5VMF 日本語", layer3.UTF16Characters, 3},
		{"a😀", layer3.UTF16Characters, 0},
	}
	for _, tt := range tests {
		lcs, err := pdu.EncodeTalkerAlias(tt.alias, tt.format)
		if err != nil {
			t.Fatalf("%q: %v", tt.alias, err)
		}
		if len(lcs) != tt.blocks+1 {
			t.Errorf("%q: %d LCs, want %d", tt.alias, len(lcs), tt.blocks+1)
		}
		var a pdu.TalkerAliasAssembler
		var got string
		var done bool
		for i := len(lcs) - 1; i >= 0; i-- {
			rx := airRoundTrip(t, lcs[i])
			alias, ok, err := a.Add(&rx)
			if err != nil {
				t.Fatalf("%q: %v", tt.alias, err)
			}
			if ok {
				got, done = alias, true
			}
		}
		if !done || got != tt.alias {
			t.Errorf("assembled %q (done=%v), want %q", got, done, tt.alias)
		}
		if alias, ok, _ := a.Add(&lcs[0]); ok || alias != "" {
			t.Errorf("%q: repeated header reported again", tt.alias)
		}
	}
}

func TestTalkerAlias_HeaderBits7Bit(t *testing.T) {
	t.Parallel()
	lcs, err := pdu.EncodeTalkerAlias("D", layer3.SevenBitCharacters)
	if err != nil {
		t.Fatal(err)
	}
	h := lcs[0].TalkerAliasHeader
	if lcs[0].FLCO != enums.FLCOTalkerAliasHeader || lcs[0].FeatureSetID != enums.StandardizedFID {
		t.Errorf("FLCO %v FID %v", lcs[0].FLCO, lcs[0].FeatureSetID)
	}
	if !h.TalkerAliasDataMSB || bit.BitsToUint8(h.TalkerAliasData[:], 0, 6) != 0b000100 || h.TalkerAliasDataLength != 1 {
		t.Errorf("header %+v", h)
	}
	lcs, _ = pdu.EncodeTalkerAlias("D", layer3.UTF8Characters)
	h = lcs[0].TalkerAliasHeader
	if h.TalkerAliasDataMSB || bit.BitsToUint8(h.TalkerAliasData[:], 0, 8) != 'D' {
		t.Errorf("UTF-8 header %+v", h)
	}
}

func TestTalkerAliasBlocks_Table(t *testing.T) {
	t.Parallel()
	tests := []struct {
		format layer3.TalkerAliasDataFormat
		bounds [4]int
	}{
		{layer3.SevenBitCharacters, [4]int{7, 15, 23, 31}},
		{layer3.ISOEightBitCharacters, [4]int{6, 13, 20, 27}},
		{layer3.UTF8Characters, [4]int{6, 13, 20, 27}},
		{layer3.UTF16Characters, [4]int{3, 6, 10, 13}},
	}
	for _, tt := range tests {
		for n, max := range tt.bounds {
			if got := pdu.TalkerAliasBlocks(tt.format, max); got != n {
				t.Errorf("format %d length %d: %d blocks, want %d", tt.format, max, got, n)
			}
			if n < 3 {
				if got := pdu.TalkerAliasBlocks(tt.format, max+1); got != n+1 {
					t.Errorf("format %d length %d: %d blocks, want %d", tt.format, max+1, got, n+1)
				}
			}
		}
	}
}

func TestTalkerAlias_Errors(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		alias  string
		format layer3.TalkerAliasDataFormat
	}{
		{"0123456789ABCDEFGHIJKLMNOPQRSTUV", layer3.SevenBitCharacters},
		{"0123456789ABCDEFGHIJKLMNOPQR", layer3.ISOEightBitCharacters},
		{"0123456789ABCDEFGHIJKLMNOPQR", layer3.UTF8Characters},
		{"0123456789ABCD", layer3.UTF16Characters},
	} {
		if _, err := pdu.EncodeTalkerAlias(tc.alias, tc.format); !errors.Is(err, pdu.ErrTalkerAliasLength) {
			t.Errorf("%q: err %v", tc.alias, err)
		}
	}
	if _, err := pdu.EncodeTalkerAlias("é", layer3.SevenBitCharacters); err == nil {
		t.Error("7-bit accepted é")
	}
	if _, err := pdu.EncodeTalkerAlias("Ā", layer3.ISOEightBitCharacters); err == nil {
		t.Error("ISO accepted Ā")
	}
	if _, err := pdu.EncodeTalkerAlias("x", layer3.TalkerAliasDataFormat(4)); err == nil {
		t.Error("unknown format accepted")
	}
	lcs, _ := pdu.EncodeTalkerAlias("KI5VMF Jacob", layer3.UTF8Characters)
	if _, err := pdu.DecodeTalkerAlias(lcs[0].TalkerAliasHeader); !errors.Is(err, pdu.ErrTalkerAliasIncomplete) {
		t.Errorf("missing block: %v", err)
	}
	if _, err := pdu.DecodeTalkerAlias(lcs[0].TalkerAliasHeader, nil); !errors.Is(err, pdu.ErrTalkerAliasIncomplete) {
		t.Errorf("nil block: %v", err)
	}
	bad := *lcs[0].TalkerAliasHeader
	bad.TalkerAliasDataLength = 1
	copy(bad.TalkerAliasData[:8], bit.BitsFromUint8(0xC3, 8))
	if _, err := pdu.DecodeTalkerAlias(&bad); err == nil {
		t.Error("invalid UTF-8 accepted")
	}
	var a pdu.TalkerAliasAssembler
	if _, ok, err := a.Add(&pdu.FullLinkControl{FLCO: enums.FLCOGroupVoiceChannelUser}); ok || err != nil {
		t.Error("non-TA LC not ignored")
	}
	if _, ok, err := a.Add(&lcs[1]); ok || err != nil {
		t.Error("block without header completed")
	}
	other, _ := pdu.EncodeTalkerAlias("Other", layer3.UTF8Characters)
	if alias, ok, err := a.Add(&other[0]); !ok || err != nil || alias != "Other" {
		t.Errorf("got %q %v %v", alias, ok, err)
	}
	if _, ok, _ := a.Add(&lcs[0]); ok {
		t.Error("changed header kept old blocks")
	}
	if alias, ok, _ := a.Add(&lcs[1]); !ok || alias != "KI5VMF Jacob" {
		t.Errorf("got %q %v", alias, ok)
	}
	a.Reset()
	if _, ok, _ := a.Add(&lcs[0]); ok {
		t.Error("reset kept blocks")
	}
}
