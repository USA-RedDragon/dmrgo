package pdu_test

import (
	"errors"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
	"github.com/USA-RedDragon/dmrgo/v2/layer3"
)

func airRoundTrip(t *testing.T, lc pdu.FullLinkControl) pdu.FullLinkControl {
	t.Helper()
	lc.BurstDataType = enums.DataTypeVoiceLCHeader
	bits, err := lc.Encode()
	if err != nil {
		t.Fatal(err)
	}
	got := pdu.DecodeFullLinkControl(bits, enums.DataTypeVoiceLCHeader)
	res := got.FEC
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
		{"", layer3.TalkerAliasSevenBit, 0},
		{"DL1ABC", layer3.TalkerAliasSevenBit, 0},
		{"KI5VMF J", layer3.TalkerAliasSevenBit, 1},
		{"0123456789ABCDEFGHIJKLMNOPQRSTU", layer3.TalkerAliasSevenBit, 3},
		{"Müller", layer3.TalkerAliasISOEightBit, 0},
		{"N0CALL Jürgen Brück Ñ", layer3.TalkerAliasISOEightBit, 3},
		{"日本語", layer3.TalkerAliasUTF8, 1},
		{"ABCDEFGHIJKLMNOPQRSTUVWXYZ0", layer3.TalkerAliasUTF8, 3},
		{"АБВ", layer3.TalkerAliasUTF16, 0},
		{"Ω KI5VMF 日本語", layer3.TalkerAliasUTF16, 3},
		{"a😀", layer3.TalkerAliasUTF16, 0},
	}
	for _, tt := range tests {
		lcs, err := pdu.EncodeTalkerAlias(tt.alias, tt.format)
		if err != nil {
			t.Fatalf("%q: %v", tt.alias, err)
		}
		if len(lcs) != tt.blocks+1 {
			t.Errorf("%q: %d LCs, want %d", tt.alias, len(lcs), tt.blocks+1)
		}
		var a, b, c pdu.TalkerAliasAssembler
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
			var text [pdu.TalkerAliasMaxText]byte
			n, okInto, errInto := b.AddInto(&text, &rx)
			appended, okAppend, errAppend := c.AppendAdd([]byte("x"), &rx)
			if okInto != ok || errInto != nil || string(text[:n]) != alias || okAppend != ok || errAppend != nil || string(appended) != "x"+alias {
				t.Errorf("%q: AddInto gives %q %v %v and AppendAdd %q %v %v, want %q %v", tt.alias, text[:n], okInto, errInto, appended, okAppend, errAppend, alias, ok)
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
	lcs, err := pdu.EncodeTalkerAlias("D", layer3.TalkerAliasSevenBit)
	if err != nil {
		t.Fatal(err)
	}
	h, ok := lcs[0].AsTalkerAliasHeader()
	if !ok || lcs[0].FLCO != enums.FLCOTalkerAliasHeader || lcs[0].FID != enums.FIDStandardized {
		t.Fatalf("FLCO %v FID %v", lcs[0].FLCO, lcs[0].FID)
	}
	if h.TalkerAliasData>>42 != 0b1000100 || h.TalkerAliasDataLength != 1 {
		t.Errorf("header %+v", h)
	}
	lcs, _ = pdu.EncodeTalkerAlias("D", layer3.TalkerAliasUTF8)
	h, ok = lcs[0].AsTalkerAliasHeader()
	if !ok {
		t.Fatal("UTF-8 LC 0 is not a header")
	}
	if h.TalkerAliasData>>48 != 0 || h.TalkerAliasData>>40 != 'D' {
		t.Errorf("UTF-8 header %+v", h)
	}
}

func TestTalkerAliasBlocks_Table(t *testing.T) {
	t.Parallel()
	tests := []struct {
		format layer3.TalkerAliasDataFormat
		bounds [4]int
	}{
		{layer3.TalkerAliasSevenBit, [4]int{7, 15, 23, 31}},
		{layer3.TalkerAliasISOEightBit, [4]int{6, 13, 20, 27}},
		{layer3.TalkerAliasUTF8, [4]int{6, 13, 20, 27}},
		{layer3.TalkerAliasUTF16, [4]int{3, 6, 10, 13}},
	}
	for _, tt := range tests {
		for n, limit := range tt.bounds {
			if got := pdu.TalkerAliasBlocks(tt.format, limit); got != n {
				t.Errorf("format %d length %d: %d blocks, want %d", tt.format, limit, got, n)
			}
			if n < 3 {
				if got := pdu.TalkerAliasBlocks(tt.format, limit+1); got != n+1 {
					t.Errorf("format %d length %d: %d blocks, want %d", tt.format, limit+1, got, n+1)
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
		{"0123456789ABCDEFGHIJKLMNOPQRSTUV", layer3.TalkerAliasSevenBit},
		{"0123456789ABCDEFGHIJKLMNOPQR", layer3.TalkerAliasISOEightBit},
		{"0123456789ABCDEFGHIJKLMNOPQR", layer3.TalkerAliasUTF8},
		{"0123456789ABCD", layer3.TalkerAliasUTF16},
	} {
		if _, err := pdu.EncodeTalkerAlias(tc.alias, tc.format); !errors.Is(err, pdu.ErrTalkerAliasLength) {
			t.Errorf("%q: err %v", tc.alias, err)
		}
	}
	if _, err := pdu.EncodeTalkerAlias("é", layer3.TalkerAliasSevenBit); err == nil {
		t.Error("7-bit accepted é")
	}
	if _, err := pdu.EncodeTalkerAlias("Ā", layer3.TalkerAliasISOEightBit); !errors.Is(err, pdu.ErrTalkerAliasCharacter) {
		t.Errorf("ISO Ā: %v, want ErrTalkerAliasCharacter", err)
	}
	var fre *dmrgo.FieldRangeError
	if _, err := pdu.EncodeTalkerAlias("x", layer3.TalkerAliasDataFormat(4)); !errors.As(err, &fre) {
		t.Errorf("unknown format: %v, want a *dmrgo.FieldRangeError", err)
	}
	lcs, _ := pdu.EncodeTalkerAlias("KI5VMF Jacob", layer3.TalkerAliasUTF8)
	hdr, ok := lcs[0].AsTalkerAliasHeader()
	if !ok {
		t.Fatal("LC 0 is not a header")
	}
	if _, err := pdu.TalkerAlias(&hdr); !errors.Is(err, pdu.ErrTalkerAliasIncomplete) {
		t.Errorf("missing block: %v", err)
	}
	if _, err := pdu.TalkerAlias(&hdr, nil); !errors.Is(err, pdu.ErrTalkerAliasIncomplete) {
		t.Errorf("nil block: %v", err)
	}
	bad := hdr

	bad.TalkerAliasDataLength = 2
	bad.TalkerAliasData = 0xC341 << 32
	if _, err := pdu.TalkerAlias(&bad); err == nil {
		t.Error("invalid UTF-8 accepted")
	}
	if got, err := pdu.AppendTalkerAlias([]byte("x"), &bad); !errors.Is(err, pdu.ErrTalkerAliasInvalidUTF8) || string(got) != "x\xc3A" {
		t.Errorf("AppendTalkerAlias of invalid UTF-8: %q, %v", got, err)
	}
	var a pdu.TalkerAliasAssembler
	if _, ok, err := a.Add(&pdu.FullLinkControl{FLCO: enums.FLCOGroupVoiceChannelUser}); ok || err != nil {
		t.Error("non-TA LC not ignored")
	}
	if _, ok, err := a.Add(&lcs[1]); ok || err != nil {
		t.Error("block without header completed")
	}
	other, _ := pdu.EncodeTalkerAlias("Other", layer3.TalkerAliasUTF8)
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
