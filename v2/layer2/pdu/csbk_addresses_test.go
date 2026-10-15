package pdu_test

import (
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

func TestCSBK_Addresses(t *testing.T) {
	t.Parallel()

	csbks := []pdu.CSBK{
		{Opcode: pdu.CSBKUnitToUnitVoiceServiceRequest, UnitToUnitVoiceServiceRequestPDU: &pdu.UnitToUnitVoiceServiceRequestPDU{}},
		{Opcode: pdu.CSBKUnitToUnitVoiceServiceAnswerResponse, UnitToUnitVoiceServiceAnswerResponsePDU: &pdu.UnitToUnitVoiceServiceAnswerResponsePDU{}},
		{Opcode: pdu.CSBKNegativeAcknowledgement, NegativeAcknowledgementPDU: &pdu.NegativeAcknowledgementPDU{}},
		{Opcode: pdu.CSBKPreamble, PreamblePDU: &pdu.PreamblePDU{}},
		{Opcode: pdu.CSBKPrivateVoiceGrant, PrivateVoiceGrantPDU: &pdu.PrivateVoiceGrantPDU{}},
		{Opcode: pdu.CSBKTalkgroupVoiceGrant, TalkgroupVoiceGrantPDU: &pdu.TalkgroupVoiceGrantPDU{}},
		{Opcode: pdu.CSBKBroadcastTalkgroupVoiceGrant, BroadcastTalkgroupVoiceGrantPDU: &pdu.BroadcastTalkgroupVoiceGrantPDU{}},
		{Opcode: pdu.CSBKPrivateDataGrant, PrivateDataGrantPDU: &pdu.PrivateDataGrantPDU{}},
		{Opcode: pdu.CSBKTalkgroupDataGrant, TalkgroupDataGrantPDU: &pdu.TalkgroupDataGrantPDU{}},
		{Opcode: pdu.CSBKDuplexPrivateVoiceGrant, DuplexPrivateVoiceGrantPDU: &pdu.DuplexPrivateVoiceGrantPDU{}},
		{Opcode: pdu.CSBKDuplexPrivateDataGrant, DuplexPrivateDataGrantPDU: &pdu.DuplexPrivateDataGrantPDU{}},
		{Opcode: pdu.CSBKPrivateDataGrantMultiItem, PrivateDataGrantMultiItemPDU: &pdu.PrivateDataGrantMultiItemPDU{}},
		{Opcode: pdu.CSBKClear, ClearPDU: &pdu.ClearPDU{}},
		{Opcode: pdu.CSBKProtect, ProtectPDU: &pdu.ProtectPDU{}},
		{Opcode: pdu.CSBKAhoy, AhoyPDU: &pdu.AhoyPDU{}},
		{Opcode: pdu.CSBKRandomAccess, RandomAccessPDU: &pdu.RandomAccessPDU{}},
		{Opcode: pdu.CSBKAckvitation, AckvitationPDU: &pdu.AckvitationPDU{}},
		{Opcode: pdu.CSBKMaintenance, MaintenancePDU: &pdu.MaintenancePDU{}},
		{Opcode: pdu.CSBKBSOutboundActivation},
	}
	for _, c := range csbks {
		c.LastBlock = true
		if !c.SetAddresses(3120101, 3120001) {
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
		if c.SetAddresses(1<<24, 1) {
			t.Errorf("%s: SetAddresses of a 25-bit target succeeded", c.Opcode)
		}
	}

	none := []pdu.CSBK{
		{},
		{Opcode: pdu.CSBKAloha, AlohaPDU: &pdu.AlohaPDU{MSAddress: 9}},
		{Opcode: pdu.CSBKMove, MovePDU: &pdu.MovePDU{MSAddress: 9}},
		{Opcode: pdu.CSBKAckOutbound, AckOutboundPDU: &pdu.AckOutboundPDU{TargetAddress: 9}},
		{Opcode: pdu.CSBKChannelTiming, ChannelTimingPDU: &pdu.ChannelTimingPDU{}},
		{Opcode: pdu.CSBKTalkgroupDataGrantMultiItem, FID: 0x10},
		{Opcode: pdu.CSBKPreamble, FID: 0x10},
	}
	for _, c := range none {
		if _, _, ok := c.Addresses(); ok || c.SetAddresses(1, 2) {
			t.Errorf("%s FID %#x: has addresses", c.Opcode, uint8(c.FID))
		}
	}
}
