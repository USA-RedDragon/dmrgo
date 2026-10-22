package pdu_test

import (
	"errors"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
	"github.com/USA-RedDragon/dmrgo/v2/layer3"
)

func cutUTF8Alias(t *testing.T, octets string) (*pdu.FLCTalkerAliasHeader, []*pdu.FLCTalkerAliasBlock) {
	t.Helper()
	lcs, err := pdu.EncodeTalkerAlias(octets, layer3.TalkerAliasUTF8)
	if err != nil {
		t.Fatal(err)
	}
	hdr, ok := lcs[0].AsTalkerAliasHeader()
	if !ok {
		t.Fatal("LC 0 is not a header")
	}
	blocks := make([]*pdu.FLCTalkerAliasBlock, 0, len(lcs)-1)
	for _, lc := range lcs[1:] {
		b, ok := lc.AsTalkerAliasBlock()
		if !ok {
			t.Fatal("LC is not a block")
		}
		blocks = append(blocks, &b)
	}
	return &hdr, blocks
}

func TestTalkerAliasCutUTF8(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		octets, want string
		err          error
	}{
		{"ТЕРНАВ\xd0", "ТЕРНАВ", nil},
		{"KI5VMF ✓\xe2\x9c", "KI5VMF ✓", nil},
		{"\xc3", "", nil},
		{"a\xf0\x9f\x98", "a", nil},
		{"a\xc3A", "a\xc3A", pdu.ErrTalkerAliasInvalidUTF8},
		{"\xffa\xc3", "\xffa\xc3", pdu.ErrTalkerAliasInvalidUTF8},
	} {
		hdr, blocks := cutUTF8Alias(t, tc.octets)
		got, err := pdu.TalkerAlias(hdr, blocks...)
		if got != tc.want || !errors.Is(err, tc.err) || (tc.err == nil) != (err == nil) {
			t.Errorf("TalkerAlias(%q) = %q, %v; want %q, %v", tc.octets, got, err, tc.want, tc.err)
		}
		var a pdu.TalkerAliasAssembler
		var text string
		var ok bool
		lcs, _ := pdu.EncodeTalkerAlias(tc.octets, layer3.TalkerAliasUTF8)
		for i := range lcs {
			if s, k, _ := a.Add(&lcs[i]); k {
				text, ok = s, k
			}
		}
		if wantOK := tc.err == nil; ok != wantOK || ok && text != tc.want {
			t.Errorf("Assembler(%q) = %q, %v; want %q, %v", tc.octets, text, ok, tc.want, wantOK)
		}
	}
}
