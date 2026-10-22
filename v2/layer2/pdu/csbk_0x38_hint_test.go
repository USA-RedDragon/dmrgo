package pdu_test

import (
	"strings"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

func TestCSBK_SetResetsHint0x38(t *testing.T) {
	t.Parallel()

	var c pdu.CSBK
	noErr(c.SetTalkgroupDataGrantMultiItem(pdu.TalkgroupDataGrantMultiItemPDU{PhysicalChannel: 5}))
	if c.Hint0x38 != pdu.Opcode0x38TalkgroupDataGrantMultiItem {
		t.Fatalf("Hint0x38 after SetTalkgroupDataGrantMultiItem: %d", c.Hint0x38)
	}
	noErr(c.SetPreamble(pdu.PreamblePDU{}))
	if c.Hint0x38 != pdu.Opcode0x38Unknown {
		t.Errorf("Hint0x38 after SetPreamble: %d", c.Hint0x38)
	}
}

func TestCSBK_Opcode0x38HintSelectsView(t *testing.T) {
	t.Parallel()

	var c pdu.CSBK
	noErr(c.SetBSOutboundActivation(pdu.BSOutboundActivationPDU{BSAddress: 9, SourceAddress: 1}))
	for _, tt := range []struct {
		hint       pdu.Opcode0x38Hint
		bs, td     bool
		stringType string
	}{
		{pdu.Opcode0x38Unknown, true, true, "BSOutboundActivationPDU{"},
		{pdu.Opcode0x38BSOutboundActivation, true, false, "BSOutboundActivationPDU{"},
		{pdu.Opcode0x38TalkgroupDataGrantMultiItem, false, true, "TalkgroupDataGrantMultiItemPDU{"},
	} {
		c.Hint0x38 = tt.hint
		_, bs := c.AsBSOutboundActivation()
		_, td := c.AsTalkgroupDataGrantMultiItem()
		if bs != tt.bs || td != tt.td {
			t.Errorf("hint %d: As BS_Dwn_Act %v, TD_GRANT_MI %v", tt.hint, bs, td)
		}
		if s := c.String(); !strings.Contains(s, tt.stringType) {
			t.Errorf("hint %d: String %s", tt.hint, s)
		}
	}
}

func TestMBCHeader_TalkgroupDataGrantMultiItem(t *testing.T) {
	t.Parallel()

	var h pdu.MBCHeader
	g := pdu.TalkgroupDataGrantMultiItemPDU{PhysicalChannel: 0xFFF, TargetAddress: 91, SourceAddress: 7}
	noErr(h.SetTalkgroupDataGrantMultiItem(g))
	if h.Opcode != pdu.CSBKTalkgroupDataGrantMultiItem {
		t.Fatalf("opcode %v", h.Opcode)
	}
	got := pdu.DecodeMBCHeader(must(h.Encode()))
	if p, ok := got.AsTalkgroupDataGrantMultiItem(); !ok || p != g {
		t.Errorf("TD_GRANT_MI MBC header round trip %+v %v", p, ok)
	}
	if !strings.Contains(got.String(), "TalkgroupDataGrantMultiItemPDU{") {
		t.Errorf("String %s", got.String())
	}
}
