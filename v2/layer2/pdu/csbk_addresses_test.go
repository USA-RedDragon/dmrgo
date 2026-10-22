package pdu_test

import (
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

func TestCSBK_Addresses(t *testing.T) {
	t.Parallel()

	csbks := []pdu.CSBK{
		addrCSBK((*pdu.CSBK).SetUnitToUnitVoiceServiceRequest, pdu.UnitToUnitVoiceServiceRequestPDU{}),
		addrCSBK((*pdu.CSBK).SetUnitToUnitVoiceServiceAnswerResponse, pdu.UnitToUnitVoiceServiceAnswerResponsePDU{}),
		addrCSBK((*pdu.CSBK).SetNegativeAcknowledgement, pdu.NegativeAcknowledgementPDU{}),
		addrCSBK((*pdu.CSBK).SetPreamble, pdu.PreamblePDU{}),
		addrCSBK((*pdu.CSBK).SetPrivateVoiceGrant, pdu.PrivateVoiceGrantPDU{}),
		addrCSBK((*pdu.CSBK).SetTalkgroupVoiceGrant, pdu.TalkgroupVoiceGrantPDU{}),
		addrCSBK((*pdu.CSBK).SetBroadcastTalkgroupVoiceGrant, pdu.BroadcastTalkgroupVoiceGrantPDU{}),
		addrCSBK((*pdu.CSBK).SetPrivateDataGrant, pdu.PrivateDataGrantPDU{}),
		addrCSBK((*pdu.CSBK).SetTalkgroupDataGrant, pdu.TalkgroupDataGrantPDU{}),
		addrCSBK((*pdu.CSBK).SetDuplexPrivateVoiceGrant, pdu.DuplexPrivateVoiceGrantPDU{}),
		addrCSBK((*pdu.CSBK).SetDuplexPrivateDataGrant, pdu.DuplexPrivateDataGrantPDU{}),
		addrCSBK((*pdu.CSBK).SetPrivateDataGrantMultiItem, pdu.PrivateDataGrantMultiItemPDU{}),
		addrCSBK((*pdu.CSBK).SetClear, pdu.ClearPDU{}),
		addrCSBK((*pdu.CSBK).SetProtect, pdu.ProtectPDU{}),
		addrCSBK((*pdu.CSBK).SetAhoy, pdu.AhoyPDU{}),
		addrCSBK((*pdu.CSBK).SetRandomAccess, pdu.RandomAccessPDU{}),
		addrCSBK((*pdu.CSBK).SetAckvitation, pdu.AckvitationPDU{}),
		addrCSBK((*pdu.CSBK).SetMaintenance, pdu.MaintenancePDU{}),
		{Opcode: pdu.CSBKBSOutboundActivation},
	}
	for _, c := range csbks {
		c.LastBlock = true
		if c.SetAddresses(3120101, 3120001) != nil {
			t.Fatalf("%s: SetAddresses failed", c.Opcode)
		}
		decoded := pdu.DecodeCSBK(must(c.Encode()))
		res := decoded.FEC
		if res.Uncorrectable {
			t.Fatalf("%s: CRC fails", c.Opcode)
		}
		if target, source, ok := decoded.Addresses(); !ok || target != 3120101 || source != 3120001 {
			t.Errorf("%s: Addresses = %d %d %v", c.Opcode, target, source, ok)
		}
		if c.SetAddresses(1<<24, 1) == nil {
			t.Errorf("%s: SetAddresses of a 25-bit target succeeded", c.Opcode)
		}
	}

	none := []pdu.CSBK{
		{},
		addrCSBK((*pdu.CSBK).SetAloha, pdu.AlohaPDU{MSAddress: 9}),
		addrCSBK((*pdu.CSBK).SetMove, pdu.MovePDU{MSAddress: 9}),
		addrCSBK((*pdu.CSBK).SetAckOutbound, pdu.AckOutboundPDU{TargetAddress: 9}),
		addrCSBK((*pdu.CSBK).SetChannelTiming, pdu.ChannelTimingPDU{}),
		{Opcode: pdu.CSBKTalkgroupDataGrantMultiItem, FID: 0x10},
		{Opcode: pdu.CSBKPreamble, FID: 0x10},
	}
	for _, c := range none {
		if _, _, ok := c.Addresses(); ok || c.SetAddresses(1, 2) == nil {
			t.Errorf("%s FID %#x: has addresses", c.Opcode, uint8(c.FID))
		}
	}
}

func addrCSBK[P any](set func(*pdu.CSBK, P) error, p P) pdu.CSBK {
	var c pdu.CSBK
	noErr(set(&c, p))
	return c
}
