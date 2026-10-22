package pdu_test

import (
	"errors"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

func TestNewBSOutboundActivation(t *testing.T) {
	t.Parallel()

	c := must(pdu.NewBSOutboundActivation(310123, 3191868))
	b := must(c.Encode())
	if b[0] != 0x80|0x38 || b[1] != 0 || b[2] != 0 || b[3] != 0 {
		t.Fatalf("octets 0 to 3 %x, want b8000000", b[:4])
	}
	cb := pdu.CSBKBytes(b)
	if tgt, src, ok := cb.Addresses(); !ok || tgt != 310123 || src != 3191868 || !cb.CRCOK() {
		t.Fatalf("CSBKBytes.Addresses %d %d %t, CRC %t", tgt, src, ok, cb.CRCOK())
	}
	got := pdu.DecodeCSBK(b)
	p, ok := got.AsBSOutboundActivation()
	if !ok || p.BSAddress != 310123 || p.SourceAddress != 3191868 || got.FEC.HasErrors() {
		t.Fatalf("decoded %+v %t", p, ok)
	}
	if tgt, src, ok := got.Addresses(); !ok || tgt != 310123 || src != 3191868 {
		t.Fatalf("CSBK.Addresses %d %d %t", tgt, src, ok)
	}
	if got.SetAddresses(9, 3120001) != nil {
		t.Fatal("SetAddresses on opcode 0x38")
	}
	if p, _ := got.AsBSOutboundActivation(); p.BSAddress != 9 || p.SourceAddress != 3120001 {
		t.Fatalf("after SetAddresses %+v", p)
	}
	if cb.SetAddresses(9, 3120001) != nil {
		t.Fatal("CSBKBytes.SetAddresses on opcode 0x38")
	}
	cb.Reseal()
	if back := must(got.Encode()); back != [12]byte(cb) {
		t.Fatalf("CSBK %x, CSBKBytes %x", back, cb)
	}
	var rangeErr *dmrgo.FieldRangeError
	if _, err := pdu.NewBSOutboundActivation(1<<24, 1); !errors.As(err, &rangeErr) {
		t.Fatalf("BS address 1<<24: %v", err)
	}
}

func TestCSBKSetTalkgroupDataGrantMultiItem(t *testing.T) {
	t.Parallel()

	c := pdu.CSBK{ProtectFlag: true}
	noErr(c.SetAloha(pdu.AlohaPDU{}))
	mi := pdu.TalkgroupDataGrantMultiItemPDU{PhysicalChannel: 77, HiRate: true, TargetAddress: 9, SourceAddress: 3120001}
	if err := c.SetTalkgroupDataGrantMultiItem(mi); err != nil {
		t.Fatal(err)
	}
	if _, isAloha := c.AsAloha(); isAloha || !c.ProtectFlag || c.Hint0x38 != pdu.Opcode0x38TalkgroupDataGrantMultiItem || c.FID != enums.FIDStandardized {
		t.Fatalf("CSBK %s", c.String())
	}
	got := pdu.DecodeCSBK(must(c.Encode()))
	if back, ok := got.AsTalkgroupDataGrantMultiItem(); !ok || back != mi {
		t.Fatalf("TD_GRANT_MI %+v %t, want %+v", back, ok, mi)
	}
	before := c
	if err := c.SetTalkgroupDataGrantMultiItem(pdu.TalkgroupDataGrantMultiItemPDU{PhysicalChannel: 0x1000}); err == nil {
		t.Fatal("physical channel 0x1000 accepted")
	}
	if c.RawPayload != before.RawPayload {
		t.Fatal("a failed set changed the CSBK")
	}
}

func TestMBCHeaderDispatch(t *testing.T) {
	t.Parallel()

	grant := pdu.TalkgroupVoiceGrantPDU{PhysicalChannel: 0xFFF, TargetAddress: 9, SourceAddress: 3120001}
	var h pdu.MBCHeader
	noErr(h.SetTalkgroupVoiceGrant(grant))
	if h.Opcode != pdu.CSBKTalkgroupVoiceGrant || h.FID != enums.FIDStandardized {
		t.Fatalf("SetTalkgroupVoiceGrant made opcode %s FID %#x", h.Opcode, uint8(h.FID))
	}
	got := pdu.DecodeMBCHeader(must(h.Encode()))
	if back, ok := got.AsTalkgroupVoiceGrant(); got.FEC.HasErrors() || !ok || back != grant || got.RawPayload != h.RawPayload {
		t.Fatalf("MBC header %s", got.String())
	}
	raw := pdu.MBCHeader{Opcode: pdu.CSBKTalkgroupVoiceGrant, FID: 0x10, RawPayload: [8]byte{1, 2, 3}}
	got = pdu.DecodeMBCHeader(must(raw.Encode()))
	if _, ok := got.AsTalkgroupVoiceGrant(); ok || got.RawPayload != raw.RawPayload {
		t.Fatalf("manufacturer FID header %s", got.String())
	}
}
