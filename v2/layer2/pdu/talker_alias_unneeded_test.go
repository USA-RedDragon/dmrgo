package pdu_test

import (
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
	"github.com/USA-RedDragon/dmrgo/v2/layer3"
)

func blockLC(t *testing.T, flco enums.FLCO, fill byte) pdu.FullLinkControl {
	t.Helper()
	var b pdu.FLCTalkerAliasBlock
	for i := range b.TalkerAliasData {
		b.TalkerAliasData[i] = fill
	}
	var lc pdu.FullLinkControl
	noErr(lc.SetTalkerAliasBlock(flco, b))
	return lc
}

// ETSI TS 102 361-2 clause 5.4.3
func TestTalkerAliasAssembler_UnneededBlocksReportOnce(t *testing.T) {
	t.Parallel()
	lcs, err := pdu.EncodeTalkerAlias("KN4OQW", layer3.TalkerAliasISOEightBit)
	if err != nil {
		t.Fatal(err)
	}
	if len(lcs) != 1 {
		t.Fatalf("%d LCs, want a header only", len(lcs))
	}
	seq := []pdu.FullLinkControl{
		lcs[0],
		blockLC(t, enums.FLCOTalkerAliasBlock1, 0),
		blockLC(t, enums.FLCOTalkerAliasBlock2, 0),
		blockLC(t, enums.FLCOTalkerAliasBlock3, 0),
		lcs[0],
		blockLC(t, enums.FLCOTalkerAliasBlock1, 0x55),
		blockLC(t, enums.FLCOTalkerAliasBlock3, 0xAA),
	}
	var a pdu.TalkerAliasAssembler
	reports := 0
	for i := range seq {
		alias, ok, err := a.Add(&seq[i])
		if err != nil {
			t.Fatalf("LC %d: %v", i, err)
		}
		if ok {
			reports++
			if alias != "KN4OQW" {
				t.Errorf("LC %d: alias %q", i, alias)
			}
		}
	}
	if reports != 1 {
		t.Errorf("alias reported %d times, want 1", reports)
	}
}

func TestTalkerAliasAssembler_NeededBlockChangeReports(t *testing.T) {
	t.Parallel()
	first, err := pdu.EncodeTalkerAlias("KN4OQW Clint", layer3.TalkerAliasISOEightBit)
	if err != nil {
		t.Fatal(err)
	}
	second, err := pdu.EncodeTalkerAlias("KN4OQW Chris", layer3.TalkerAliasISOEightBit)
	if err != nil {
		t.Fatal(err)
	}
	var a pdu.TalkerAliasAssembler
	var got []string
	for _, lc := range []pdu.FullLinkControl{first[0], first[1], second[1]} {
		alias, ok, err := a.Add(&lc)
		if err != nil {
			t.Fatal(err)
		}
		if ok {
			got = append(got, alias)
		}
	}
	if len(got) != 2 || got[0] != "KN4OQW Clint" || got[1] != "KN4OQW Chris" {
		t.Errorf("reports %q, want KN4OQW Clint then KN4OQW Chris", got)
	}
}

func TestTalkerAliasAssembler_BlockBeforeHeaderKept(t *testing.T) {
	t.Parallel()
	lcs, err := pdu.EncodeTalkerAlias("KN4OQW Clint", layer3.TalkerAliasISOEightBit)
	if err != nil {
		t.Fatal(err)
	}
	var a pdu.TalkerAliasAssembler
	reports := 0
	for _, lc := range []pdu.FullLinkControl{lcs[1], blockLC(t, enums.FLCOTalkerAliasBlock2, 0), lcs[0], blockLC(t, enums.FLCOTalkerAliasBlock3, 0)} {
		alias, ok, err := a.Add(&lc)
		if err != nil {
			t.Fatal(err)
		}
		if ok {
			reports++
			if alias != "KN4OQW Clint" {
				t.Errorf("alias %q", alias)
			}
		}
	}
	if reports != 1 {
		t.Errorf("alias reported %d times, want 1", reports)
	}
}
