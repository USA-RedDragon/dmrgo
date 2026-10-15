package zeroalloc_test

import (
	"encoding/hex"
	"os"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/bit"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
	"github.com/USA-RedDragon/dmrgo/v2/layer3"
)

// Real codewords from the pdu tests: a Voice LC Header and Terminator with
// LC (UU_V_Ch_Usr, target 9990, source 3191868), a Pre_CSBK and a PI
// header.
const (
	realVoiceLCHeader = "03000000270630b43c5dcfc1"
	realTerminatorLC  = "03000000270630b43c52c0ce"
	realLC            = "03000000270630b43c"
	realPreambleCSBK  = "bd00801d23386323383b5889"
	realPIHeader      = "211002177afc730000090dda"
)

// hexBytes returns the n bytes of the hex string s.
func hexBytes(t testing.TB, s string, n int) []byte {
	t.Helper()
	b, err := hex.DecodeString(s)
	if err != nil || len(b) != n {
		t.Fatalf("bad hex %q: %v", s, err)
	}
	return b
}

// captureData returns the information bits of the first burst of each data
// type in the layer2 captures, as the decoded PDU encodes them.
func captureData(t testing.TB) map[enums.DataType][]bit.Bit {
	t.Helper()
	out := map[enums.DataType][]bit.Bit{}
	for _, name := range []string{"voice.bin", "d-sms.bin", "h-sms.bin", "m-sms.bin"} {
		raw, err := os.ReadFile("../../layer2/testdata/" + name)
		if err != nil || len(raw)%len(layer2.RawBurst{}) != 0 {
			t.Fatalf("%s: %v", name, err)
		}
		for len(raw) > 0 {
			var b layer2.Burst
			err := b.DecodeFromBytes(layer2.RawBurst(raw[:len(layer2.RawBurst{})]))
			raw = raw[len(layer2.RawBurst{}):]
			if err != nil || b.Data == nil || b.FEC.Aggregate().HasErrors() {
				continue
			}
			if _, ok := out[b.SlotType.DataType]; !ok {
				out[b.SlotType.DataType] = must(b.Data.EncodeBits())
			}
		}
	}
	return out
}

// encodeEntry returns an entry that calls encode on v.
func encodeEntry[T, B any](api, name string, v *T, encode func(*T) (B, error)) entry {
	if _, err := encode(v); err != nil {
		panic(api + " " + name + ": " + err.Error())
	}
	return entry{api, name, 0, func() { _, _ = encode(v) }}
}

// csbkCase is a CSBK with one payload, the name of the payload type and the
// entry of the payload's Encode.
type csbkCase struct {
	name    string
	c       pdu.CSBK
	payload func(c *pdu.CSBK) entry
}

func csbkCases() []csbkCase {
	const api = "layer2/pdu."
	return []csbkCase{
		{"UU_V_Req", pdu.CSBK{Opcode: pdu.CSBKUnitToUnitVoiceServiceRequest, UnitToUnitVoiceServiceRequestPDU: &pdu.UnitToUnitVoiceServiceRequestPDU{}}, func(c *pdu.CSBK) entry {
			return encodeEntry(api+"UnitToUnitVoiceServiceRequestPDU.Encode", "UU_V_Req", c.UnitToUnitVoiceServiceRequestPDU, (*pdu.UnitToUnitVoiceServiceRequestPDU).Encode)
		}},
		{"UU_Ans_Rsp", pdu.CSBK{Opcode: pdu.CSBKUnitToUnitVoiceServiceAnswerResponse, UnitToUnitVoiceServiceAnswerResponsePDU: &pdu.UnitToUnitVoiceServiceAnswerResponsePDU{}}, func(c *pdu.CSBK) entry {
			return encodeEntry(api+"UnitToUnitVoiceServiceAnswerResponsePDU.Encode", "UU_Ans_Rsp", c.UnitToUnitVoiceServiceAnswerResponsePDU, (*pdu.UnitToUnitVoiceServiceAnswerResponsePDU).Encode)
		}},
		{"NACK_Rsp", pdu.CSBK{Opcode: pdu.CSBKNegativeAcknowledgement, NegativeAcknowledgementPDU: &pdu.NegativeAcknowledgementPDU{}}, func(c *pdu.CSBK) entry {
			return encodeEntry(api+"NegativeAcknowledgementPDU.Encode", "NACK_Rsp", c.NegativeAcknowledgementPDU, (*pdu.NegativeAcknowledgementPDU).Encode)
		}},
		{"Pre_CSBK", pdu.CSBK{Opcode: pdu.CSBKPreamble, PreamblePDU: &pdu.PreamblePDU{Group: true, CSBKBlocksToFollow: 2}}, func(c *pdu.CSBK) entry {
			return encodeEntry(api+"PreamblePDU.Encode", "Pre_CSBK", c.PreamblePDU, (*pdu.PreamblePDU).Encode)
		}},
		{"CT_CSBK", pdu.CSBK{Opcode: pdu.CSBKChannelTiming, ChannelTimingPDU: &pdu.ChannelTimingPDU{}}, func(c *pdu.CSBK) entry {
			return encodeEntry(api+"ChannelTimingPDU.Encode", "CT_CSBK", c.ChannelTimingPDU, (*pdu.ChannelTimingPDU).Encode)
		}},
		{"PV_GRANT", pdu.CSBK{Opcode: pdu.CSBKPrivateVoiceGrant, PrivateVoiceGrantPDU: &pdu.PrivateVoiceGrantPDU{}}, func(c *pdu.CSBK) entry {
			return encodeEntry(api+"PrivateVoiceGrantPDU.Encode", "PV_GRANT", c.PrivateVoiceGrantPDU, (*pdu.PrivateVoiceGrantPDU).Encode)
		}},
		{"TV_GRANT", pdu.CSBK{Opcode: pdu.CSBKTalkgroupVoiceGrant, TalkgroupVoiceGrantPDU: &pdu.TalkgroupVoiceGrantPDU{PhysicalChannel: 0xFFF}}, func(c *pdu.CSBK) entry {
			return encodeEntry(api+"TalkgroupVoiceGrantPDU.Encode", "TV_GRANT", c.TalkgroupVoiceGrantPDU, (*pdu.TalkgroupVoiceGrantPDU).Encode)
		}},
		{"BTV_GRANT", pdu.CSBK{Opcode: pdu.CSBKBroadcastTalkgroupVoiceGrant, BroadcastTalkgroupVoiceGrantPDU: &pdu.BroadcastTalkgroupVoiceGrantPDU{}}, func(c *pdu.CSBK) entry {
			return encodeEntry(api+"BroadcastTalkgroupVoiceGrantPDU.Encode", "BTV_GRANT", c.BroadcastTalkgroupVoiceGrantPDU, (*pdu.BroadcastTalkgroupVoiceGrantPDU).Encode)
		}},
		{"PD_GRANT", pdu.CSBK{Opcode: pdu.CSBKPrivateDataGrant, PrivateDataGrantPDU: &pdu.PrivateDataGrantPDU{}}, func(c *pdu.CSBK) entry {
			return encodeEntry(api+"PrivateDataGrantPDU.Encode", "PD_GRANT", c.PrivateDataGrantPDU, (*pdu.PrivateDataGrantPDU).Encode)
		}},
		{"TD_GRANT", pdu.CSBK{Opcode: pdu.CSBKTalkgroupDataGrant, TalkgroupDataGrantPDU: &pdu.TalkgroupDataGrantPDU{}}, func(c *pdu.CSBK) entry {
			return encodeEntry(api+"TalkgroupDataGrantPDU.Encode", "TD_GRANT", c.TalkgroupDataGrantPDU, (*pdu.TalkgroupDataGrantPDU).Encode)
		}},
		{"PV_GRANT_DX", pdu.CSBK{Opcode: pdu.CSBKDuplexPrivateVoiceGrant, DuplexPrivateVoiceGrantPDU: &pdu.DuplexPrivateVoiceGrantPDU{}}, func(c *pdu.CSBK) entry {
			return encodeEntry(api+"DuplexPrivateVoiceGrantPDU.Encode", "PV_GRANT_DX", c.DuplexPrivateVoiceGrantPDU, (*pdu.DuplexPrivateVoiceGrantPDU).Encode)
		}},
		{"PD_GRANT_DX", pdu.CSBK{Opcode: pdu.CSBKDuplexPrivateDataGrant, DuplexPrivateDataGrantPDU: &pdu.DuplexPrivateDataGrantPDU{}}, func(c *pdu.CSBK) entry {
			return encodeEntry(api+"DuplexPrivateDataGrantPDU.Encode", "PD_GRANT_DX", c.DuplexPrivateDataGrantPDU, (*pdu.DuplexPrivateDataGrantPDU).Encode)
		}},
		{"PD_GRANT_MI", pdu.CSBK{Opcode: pdu.CSBKPrivateDataGrantMultiItem, PrivateDataGrantMultiItemPDU: &pdu.PrivateDataGrantMultiItemPDU{}}, func(c *pdu.CSBK) entry {
			return encodeEntry(api+"PrivateDataGrantMultiItemPDU.Encode", "PD_GRANT_MI", c.PrivateDataGrantMultiItemPDU, (*pdu.PrivateDataGrantMultiItemPDU).Encode)
		}},
		{"C_MOVE", pdu.CSBK{Opcode: pdu.CSBKMove, MovePDU: &pdu.MovePDU{}}, func(c *pdu.CSBK) entry {
			return encodeEntry(api+"MovePDU.Encode", "C_MOVE", c.MovePDU, (*pdu.MovePDU).Encode)
		}},
		{"C_ALOHA", pdu.CSBK{Opcode: pdu.CSBKAloha, AlohaPDU: &pdu.AlohaPDU{}}, func(c *pdu.CSBK) entry {
			return encodeEntry(api+"AlohaPDU.Encode", "C_ALOHA", c.AlohaPDU, (*pdu.AlohaPDU).Encode)
		}},
		{"C_BCAST", pdu.CSBK{Opcode: pdu.CSBKBroadcast, BroadcastPDU: &pdu.BroadcastPDU{}}, func(c *pdu.CSBK) entry {
			return encodeEntry(api+"BroadcastPDU.Encode", "C_BCAST", c.BroadcastPDU, (*pdu.BroadcastPDU).Encode)
		}},
		{"P_CLEAR", pdu.CSBK{Opcode: pdu.CSBKClear, ClearPDU: &pdu.ClearPDU{}}, func(c *pdu.CSBK) entry {
			return encodeEntry(api+"ClearPDU.Encode", "P_CLEAR", c.ClearPDU, (*pdu.ClearPDU).Encode)
		}},
		{"P_PROTECT", pdu.CSBK{Opcode: pdu.CSBKProtect, ProtectPDU: &pdu.ProtectPDU{}}, func(c *pdu.CSBK) entry {
			return encodeEntry(api+"ProtectPDU.Encode", "P_PROTECT", c.ProtectPDU, (*pdu.ProtectPDU).Encode)
		}},
		{"C_AHOY", pdu.CSBK{Opcode: pdu.CSBKAhoy, AhoyPDU: &pdu.AhoyPDU{}}, func(c *pdu.CSBK) entry {
			return encodeEntry(api+"AhoyPDU.Encode", "C_AHOY", c.AhoyPDU, (*pdu.AhoyPDU).Encode)
		}},
		{"C_ACKD", pdu.CSBK{Opcode: pdu.CSBKAckOutbound, AckOutboundPDU: &pdu.AckOutboundPDU{}}, func(c *pdu.CSBK) entry {
			return encodeEntry(api+"AckOutboundPDU.Encode", "C_ACKD", c.AckOutboundPDU, (*pdu.AckOutboundPDU).Encode)
		}},
		{"C_ACKU", pdu.CSBK{Opcode: pdu.CSBKAckInbound, AckInboundPDU: &pdu.AckInboundPDU{}}, func(c *pdu.CSBK) entry {
			return encodeEntry(api+"AckInboundPDU.Encode", "C_ACKU", c.AckInboundPDU, (*pdu.AckInboundPDU).Encode)
		}},
		{"P_ACKD", pdu.CSBK{Opcode: pdu.CSBKAckOutboundPayload, AckOutboundPayloadPDU: &pdu.AckOutboundPayloadPDU{}}, func(c *pdu.CSBK) entry {
			return encodeEntry(api+"AckOutboundPayloadPDU.Encode", "P_ACKD", c.AckOutboundPayloadPDU, (*pdu.AckOutboundPayloadPDU).Encode)
		}},
		{"P_ACKU", pdu.CSBK{Opcode: pdu.CSBKAckInboundPayload, AckInboundPayloadPDU: &pdu.AckInboundPayloadPDU{}}, func(c *pdu.CSBK) entry {
			return encodeEntry(api+"AckInboundPayloadPDU.Encode", "P_ACKU", c.AckInboundPayloadPDU, (*pdu.AckInboundPayloadPDU).Encode)
		}},
		{"C_RAND", pdu.CSBK{Opcode: pdu.CSBKRandomAccess, RandomAccessPDU: &pdu.RandomAccessPDU{}}, func(c *pdu.CSBK) entry {
			return encodeEntry(api+"RandomAccessPDU.Encode", "C_RAND", c.RandomAccessPDU, (*pdu.RandomAccessPDU).Encode)
		}},
		{"C_ACKVIT", pdu.CSBK{Opcode: pdu.CSBKAckvitation, AckvitationPDU: &pdu.AckvitationPDU{}}, func(c *pdu.CSBK) entry {
			return encodeEntry(api+"AckvitationPDU.Encode", "C_ACKVIT", c.AckvitationPDU, (*pdu.AckvitationPDU).Encode)
		}},
		{"P_MAINT", pdu.CSBK{Opcode: pdu.CSBKMaintenance, MaintenancePDU: &pdu.MaintenancePDU{}}, func(c *pdu.CSBK) entry {
			return encodeEntry(api+"MaintenancePDU.Encode", "P_MAINT", c.MaintenancePDU, (*pdu.MaintenancePDU).Encode)
		}},
	}
}

// csbkEntries returns the entries of the CSBK decoders and encoders, for
// a CSBK of each payload, BS_Dwn_Act and TD_GRANT_MI in RawPayload and the
// Pre_CSBK of a capture.
func csbkEntries(t testing.TB) []entry {
	t.Helper()
	cases := csbkCases()
	out := make([]entry, 0, 7*len(cases)+24)
	var p pdu.CSBKPayloads
	var dst pdu.CSBK
	for _, cc := range cases {
		c := cc.c
		c.LastBlock = true
		c.SetAddresses(3191868, 310123)
		bits := must(c.Encode())
		b := pdu.CSBKBytes(must(c.Bytes()))
		pdu.DecodeCSBKInto(&dst, &p, bits)
		if dst.FEC.Uncorrectable || dst.Opcode != c.Opcode {
			t.Fatalf("CSBK %s does not decode", cc.name)
		}
		out = append(out,
			entry{"layer2/pdu.DecodeCSBK", cc.name, 1, func() { _ = pdu.DecodeCSBK(bits) }},
			entry{"layer2/pdu.DecodeCSBKInto", cc.name, 0, func() { pdu.DecodeCSBKInto(&dst, &p, bits) }},
			entry{"layer2/pdu.DecodeCSBKBytes", cc.name, 1, func() { _ = pdu.DecodeCSBKBytes(b) }},
			entry{"layer2/pdu.DecodeCSBKBytesInto", cc.name, 0, func() { pdu.DecodeCSBKBytesInto(&dst, &p, b) }},
			encodeEntry("layer2/pdu.CSBK.Encode", cc.name, &c, (*pdu.CSBK).Encode),
			encodeEntry("layer2/pdu.CSBK.Bytes", cc.name, &c, (*pdu.CSBK).Bytes),
		)
		decoded := pdu.DecodeCSBK(bits)
		out = append(out, cc.payload(&decoded))
	}

	act := must(pdu.NewBSOutboundActivation(310123, 3191868))
	var grant pdu.CSBK
	if err := grant.SetTalkgroupDataGrantMultiItem(pdu.TalkgroupDataGrantMultiItemPDU{}); err != nil {
		t.Fatal(err)
	}
	pre := pdu.CSBKBytes(hexBytes(t, realPreambleCSBK, 12))
	preBits := bit.Unpack96(pre)
	for _, c := range []struct {
		name string
		c    pdu.CSBK
	}{{"BS_Dwn_Act", act}, {"TD_GRANT_MI", grant}} {
		bits := must(c.c.Encode())
		out = append(out,
			entry{"layer2/pdu.DecodeCSBK", c.name, 0, func() { _ = pdu.DecodeCSBK(bits) }},
			entry{"layer2/pdu.DecodeCSBKInto", c.name, 0, func() { pdu.DecodeCSBKInto(&dst, &p, bits) }},
		)
	}
	actPDU, _ := act.AsBSOutboundActivation()
	grantPDU, _ := grant.AsTalkgroupDataGrantMultiItem()
	out = append(out,
		entry{"layer2/pdu.DecodeCSBK", "Pre_CSBK capture", 1, func() { _ = pdu.DecodeCSBK(preBits) }},
		entry{"layer2/pdu.DecodeCSBKInto", "Pre_CSBK capture", 0, func() { pdu.DecodeCSBKInto(&dst, &p, preBits) }},
		entry{"layer2/pdu.DecodeCSBKBytes", "Pre_CSBK capture", 1, func() { _ = pdu.DecodeCSBKBytes(pre) }},
		entry{"layer2/pdu.DecodeCSBKBytesInto", "Pre_CSBK capture", 0, func() { pdu.DecodeCSBKBytesInto(&dst, &p, pre) }},
		entry{"layer2/pdu.CSBK.AsBSOutboundActivation", "BS_Dwn_Act", 0, func() { _, _ = act.AsBSOutboundActivation() }},
		entry{"layer2/pdu.CSBK.AsTalkgroupDataGrantMultiItem", "TD_GRANT_MI", 0, func() { _, _ = grant.AsTalkgroupDataGrantMultiItem() }},
		entry{"layer2/pdu.CSBK.SetBSOutboundActivation", "BS_Dwn_Act", 0, func() { _ = act.SetBSOutboundActivation(actPDU) }},
		entry{"layer2/pdu.CSBK.SetTalkgroupDataGrantMultiItem", "TD_GRANT_MI", 0, func() { _ = grant.SetTalkgroupDataGrantMultiItem(grantPDU) }},
		encodeEntry("layer2/pdu.BSOutboundActivationPDU.Encode", "BS_Dwn_Act", &actPDU, (*pdu.BSOutboundActivationPDU).Encode),
		encodeEntry("layer2/pdu.TalkgroupDataGrantMultiItemPDU.Encode", "TD_GRANT_MI", &grantPDU, (*pdu.TalkgroupDataGrantMultiItemPDU).Encode),
		entry{"layer2/pdu.NewBSOutboundActivation", "BS_Dwn_Act", 0, func() { _, _ = pdu.NewBSOutboundActivation(310123, 3191868) }},
		entry{"layer2/pdu.CSBK.DataType", "BS_Dwn_Act", 0, func() { _ = act.DataType() }},
		entry{"layer2/pdu.CSBK.Addresses", "BS_Dwn_Act", 0, func() { _, _, _ = act.Addresses() }},
		entry{"layer2/pdu.CSBK.SetAddresses", "BS_Dwn_Act", 0, func() { _ = act.SetAddresses(310123, 3191868) }},
		entry{"layer2/pdu.CSBKBytes.Opcode", "Pre_CSBK capture", 0, func() { _ = pre.Opcode() }},
		entry{"layer2/pdu.CSBKBytes.FID", "Pre_CSBK capture", 0, func() { _ = pre.FID() }},
		entry{"layer2/pdu.CSBKBytes.Addresses", "Pre_CSBK capture", 0, func() { _, _, _ = pre.Addresses() }},
		entry{"layer2/pdu.CSBKBytes.SetAddresses", "Pre_CSBK capture", 0, func() { _ = pre.SetAddresses(0x233863, 0x23383b) }},
		entry{"layer2/pdu.CSBKBytes.Group", "Pre_CSBK capture", 0, func() { _, _ = pre.Group() }},
		entry{"layer2/pdu.CSBKBytes.SetGroup", "Pre_CSBK capture", 0, func() { _ = pre.SetGroup(false) }},
		entry{"layer2/pdu.CSBKBytes.CRCOK", "Pre_CSBK capture", 0, func() { _ = pre.CRCOK() }},
		entry{"layer2/pdu.CSBKBytes.Reseal", "Pre_CSBK capture", 0, func() { pre.Reseal() }},
	)
	return out
}

// flcEntries returns the entries of the Full LC decoders and encoders, for
// an LC of each payload and the Voice LC Header and Terminator with LC of a
// capture.
func flcEntries(t testing.TB) []entry {
	t.Helper()
	var p pdu.FullLinkControlPayloads
	var dst pdu.FullLinkControl
	header := must(pdu.NewVoiceLC(true, 91, 3191868, layer3.ServiceOptions{}))
	uu := must(pdu.NewVoiceLC(false, 9990, 3191868, layer3.ServiceOptions{IsEmergency: true}))
	tas := must(pdu.EncodeTalkerAlias("N0CALL Jacob", layer3.TalkerAliasISOEightBit))
	cases := []struct {
		name    string
		lc      pdu.FullLinkControl
		payload func(lc *pdu.FullLinkControl) entry
	}{
		{"Grp_V_Ch_Usr", header, func(lc *pdu.FullLinkControl) entry {
			return encodeEntry("layer2/pdu.FLCGroupVoice.Encode", "Grp_V_Ch_Usr", lc.GroupVoice, (*pdu.FLCGroupVoice).Encode)
		}},
		{"UU_V_Ch_Usr", uu, func(lc *pdu.FullLinkControl) entry {
			return encodeEntry("layer2/pdu.FLCUnitToUnit.Encode", "UU_V_Ch_Usr", lc.UnitToUnit, (*pdu.FLCUnitToUnit).Encode)
		}},
		{"GPS_Info", pdu.FullLinkControl{FLCO: enums.FLCOGPSInfo, GPSInfo: &pdu.FLCGPSInfo{Longitude: -97.74, Latitude: 30.27}}, func(lc *pdu.FullLinkControl) entry {
			return encodeEntry("layer2/pdu.FLCGPSInfo.Encode", "GPS_Info", lc.GPSInfo, (*pdu.FLCGPSInfo).Encode)
		}},
		{"Talker_Alias_Hdr", tas[0], func(lc *pdu.FullLinkControl) entry {
			return encodeEntry("layer2/pdu.FLCTalkerAliasHeader.Encode", "Talker_Alias_Hdr", lc.TalkerAliasHeader, (*pdu.FLCTalkerAliasHeader).Encode)
		}},
		{"Talker_Alias_Blk1", tas[1], func(lc *pdu.FullLinkControl) entry {
			return encodeEntry("layer2/pdu.FLCTalkerAliasBlock.Encode", "Talker_Alias_Blk1", lc.TalkerAliasBlock, (*pdu.FLCTalkerAliasBlock).Encode)
		}},
		{"Term_Data_LC", pdu.FullLinkControl{FLCO: enums.FLCOTerminatorDataLinkControl, TerminatorDataLinkControl: &pdu.FLCTerminatorDataLinkControl{LLIDDestination: 9, LLIDSource: 3191868}}, func(lc *pdu.FullLinkControl) entry {
			return encodeEntry("layer2/pdu.FLCTerminatorDataLinkControl.Encode", "Term_Data_LC", lc.TerminatorDataLinkControl, (*pdu.FLCTerminatorDataLinkControl).Encode)
		}},
	}
	var out []entry
	for _, c := range cases {
		lc := c.lc
		lc.BurstDataType = enums.DataTypeTerminatorWithLC
		bits := must(lc.Encode())
		lcBytes := must(lc.Bytes())
		pdu.DecodeFullLinkControlInto(&dst, &p, bits, lc.BurstDataType)
		if dst.FEC.Uncorrectable || dst.FLCO != lc.FLCO {
			t.Fatalf("Full LC %s does not decode", c.name)
		}
		out = append(out,
			entry{"layer2/pdu.DecodeFullLinkControl", c.name, 1, func() { _ = pdu.DecodeFullLinkControl(bits, enums.DataTypeTerminatorWithLC) }},
			entry{"layer2/pdu.DecodeFullLinkControlInto", c.name, 0, func() { pdu.DecodeFullLinkControlInto(&dst, &p, bits, enums.DataTypeTerminatorWithLC) }},
			entry{"layer2/pdu.DecodeFullLinkControlBytes", c.name, 1, func() { _, _ = pdu.DecodeFullLinkControlBytes(lcBytes) }},
			entry{"layer2/pdu.DecodeFullLinkControlBytesInto", c.name, 0, func() { _ = pdu.DecodeFullLinkControlBytesInto(&dst, &p, lcBytes) }},
			encodeEntry("layer2/pdu.FullLinkControl.Encode", c.name, &lc, (*pdu.FullLinkControl).Encode),
			encodeEntry("layer2/pdu.FullLinkControl.Bytes", c.name, &lc, (*pdu.FullLinkControl).Bytes),
			encodeEntry("layer2/pdu.FullLinkControl.Codeword", c.name, &lc, (*pdu.FullLinkControl).Codeword),
		)
		decoded := pdu.DecodeFullLinkControl(bits, lc.BurstDataType)
		out = append(out, c.payload(&decoded))
	}

	headerBits := bit.Unpack96([12]byte(hexBytes(t, realVoiceLCHeader, 12)))
	termCW := [12]byte(hexBytes(t, realTerminatorLC, 12))
	termBits := bit.Unpack96(termCW)
	lcBytes := pdu.LCBytes(hexBytes(t, realLC, 9))
	so := layer3.MotorolaServiceOptions()
	captured := pdu.DecodeFullLinkControl(headerBits, enums.DataTypeVoiceLCHeader)
	out = append(out,
		entry{"layer2/pdu.DecodeFullLinkControl", "Voice LC Header capture", 1, func() { _ = pdu.DecodeFullLinkControl(headerBits, enums.DataTypeVoiceLCHeader) }},
		entry{"layer2/pdu.DecodeFullLinkControlInto", "Voice LC Header capture", 0, func() { pdu.DecodeFullLinkControlInto(&dst, &p, headerBits, enums.DataTypeVoiceLCHeader) }},
		entry{"layer2/pdu.DecodeFullLinkControlInto", "Terminator with LC capture", 0, func() { pdu.DecodeFullLinkControlInto(&dst, &p, termBits, enums.DataTypeTerminatorWithLC) }},
		entry{"layer2/pdu.DecodeFullLinkControlInto", "wrong mask", 0, func() { pdu.DecodeFullLinkControlInto(&dst, &p, termBits, enums.DataTypeVoiceLCHeader) }},
		entry{"layer2/pdu.DecodeFullLinkControlBytes", "capture", 1, func() { _, _ = pdu.DecodeFullLinkControlBytes(lcBytes) }},
		entry{"layer2/pdu.DecodeFullLinkControlBytesInto", "capture", 0, func() { _ = pdu.DecodeFullLinkControlBytesInto(&dst, &p, lcBytes) }},
		entry{"layer2/pdu.DecodeFullLCCodeword", "Terminator with LC capture", 0, func() { _, _, _ = pdu.DecodeFullLCCodeword(termCW, enums.DataTypeTerminatorWithLC) }},
		entry{"layer2/pdu.DecodeFullLCCodeword", "CSBK data type", 0, func() { _, _, _ = pdu.DecodeFullLCCodeword(termCW, enums.DataTypeCSBK) }},
		entry{"layer2/pdu.FullLCParityOK", "Terminator with LC capture", 0, func() { _ = pdu.FullLCParityOK(termCW, enums.DataTypeTerminatorWithLC) }},
		entry{"layer2/pdu.FullLCParityOK", "wrong mask", 0, func() { _ = pdu.FullLCParityOK(termCW, enums.DataTypeVoiceLCHeader) }},
		entry{"layer2/pdu.LCBytes.Codeword", "capture", 0, func() { _, _ = lcBytes.Codeword(enums.DataTypeVoiceLCHeader) }},
		entry{"layer2/pdu.LCBytes.Codeword", "CSBK data type", 0, func() { _, _ = lcBytes.Codeword(enums.DataTypeCSBK) }},
		entry{"layer2/pdu.NewVoiceLC", "group", 1, func() { _, _ = pdu.NewVoiceLC(true, 91, 3191868, so) }},
		entry{"layer2/pdu.NewVoiceLCBytes", "group", 0, func() { _, _ = pdu.NewVoiceLCBytes(true, 91, 3191868, so) }},
		entry{"layer2/pdu.FullLinkControl.Addresses", "capture", 0, func() { _, _, _ = captured.Addresses() }},
		entry{"layer2/pdu.FullLinkControl.SetAddresses", "capture", 0, func() { _ = captured.SetAddresses(9990, 3191868) }},
		entry{"layer2/pdu.FullLinkControl.ServiceOptions", "capture", 0, func() { _, _ = captured.ServiceOptions() }},
		entry{"layer2/pdu.FullLinkControl.DataType", "capture", 0, func() { _ = captured.DataType() }},
		entry{"layer2/pdu.LCBytes.FLCO", "capture", 0, func() { _ = lcBytes.FLCO() }},
		entry{"layer2/pdu.LCBytes.SetFLCO", "capture", 0, func() { lcBytes.SetFLCO(enums.FLCOUnitToUnitVoiceChannelUser) }},
		entry{"layer2/pdu.LCBytes.ProtectFlag", "capture", 0, func() { _ = lcBytes.ProtectFlag() }},
		entry{"layer2/pdu.LCBytes.SetProtectFlag", "capture", 0, func() { lcBytes.SetProtectFlag(false) }},
		entry{"layer2/pdu.LCBytes.Reserved", "capture", 0, func() { _ = lcBytes.Reserved() }},
		entry{"layer2/pdu.LCBytes.SetReserved", "capture", 0, func() { lcBytes.SetReserved(false) }},
		entry{"layer2/pdu.LCBytes.FID", "capture", 0, func() { _ = lcBytes.FID() }},
		entry{"layer2/pdu.LCBytes.SetFID", "capture", 0, func() { lcBytes.SetFID(enums.FIDStandardized) }},
		entry{"layer2/pdu.LCBytes.ServiceOptions", "capture", 0, func() { _ = lcBytes.ServiceOptions() }},
		entry{"layer2/pdu.LCBytes.SetServiceOptions", "capture", 0, func() { lcBytes.SetServiceOptions(0) }},
		entry{"layer2/pdu.LCBytes.Addresses", "capture", 0, func() { _, _, _ = lcBytes.Addresses() }},
		entry{"layer2/pdu.LCBytes.SetAddresses", "capture", 0, func() { _ = lcBytes.SetAddresses(9990, 3191868) }},
		entry{"layer2/pdu.LCBytes.Group", "capture", 0, func() { _, _ = lcBytes.Group() }},
		entry{"layer2/pdu.LCBytes.SetGroup", "capture", 0, func() { _ = lcBytes.SetGroup(false) }},
	)

	return append(out, talkerAliasEntries(t)...)
}

// talkerAliasEntries returns the entries of the talker alias encoders,
// decoders and assembler, for an alias of each format and a UTF-8 alias
// that is not valid UTF-8.
func talkerAliasEntries(t testing.TB) []entry {
	t.Helper()
	var out []entry
	var lcs [4]pdu.FullLinkControl
	var lcp [4]pdu.FullLinkControlPayloads
	text := make([]byte, 0, 96)
	var kept string
	for _, c := range []struct {
		name   string
		alias  string
		format layer3.TalkerAliasDataFormat
	}{
		{"7-bit", "KI5VMF J", layer3.TalkerAliasSevenBit},
		{"ISO 8-bit", "N0CALL Jürgen Brück Ñ", layer3.TalkerAliasISOEightBit},
		{"UTF-8", "日本語", layer3.TalkerAliasUTF8},
		{"UTF-16", "Ω KI5VMF 😀", layer3.TalkerAliasUTF16},
	} {
		tas := must(pdu.EncodeTalkerAlias(c.alias, c.format))
		header := tas[0].TalkerAliasHeader
		blocks := make([]*pdu.FLCTalkerAliasBlock, 0, len(tas)-1)
		for i := 1; i < len(tas); i++ {
			blocks = append(blocks, tas[i].TalkerAliasBlock)
		}
		if got := must(pdu.DecodeTalkerAlias(header, blocks...)); got != c.alias {
			t.Fatalf("%s alias decodes as %q", c.name, got)
		}
		var alias pdu.TalkerAliasAssembler
		out = append(out,
			entry{"layer2/pdu.TalkerAliasBlocks", c.name, 0, func() { _ = pdu.TalkerAliasBlocks(c.format, header.TalkerAliasDataLength) }},
			entry{"layer2/pdu.DecodeTalkerAlias", c.name, 1, func() { kept, _ = pdu.DecodeTalkerAlias(header, blocks...) }},
			entry{"layer2/pdu.AppendTalkerAlias", c.name, 0, func() { _, _ = pdu.AppendTalkerAlias(text[:0], header, blocks...) }},
			entry{"layer2/pdu.EncodeTalkerAlias", c.name, 2, func() { _, _ = pdu.EncodeTalkerAlias(c.alias, c.format) }},
			entry{"layer2/pdu.EncodeTalkerAliasInto", c.name, 0, func() { _, _ = pdu.EncodeTalkerAliasInto(&lcs, &lcp, c.alias, c.format) }},
			entry{"layer2/pdu.TalkerAliasAssembler.Reset", c.name, 0, func() { alias.Reset() }},
			entry{"layer2/pdu.TalkerAliasAssembler.Add", c.name + " header and blocks", 1, func() {
				alias.Reset()
				for i := range tas {
					kept, _, _ = alias.Add(&tas[i])
				}
			}},
			entry{"layer2/pdu.TalkerAliasAssembler.Add", c.name + " repeated header", 0, func() { kept, _, _ = alias.Add(&tas[0]) }},
			entry{"layer2/pdu.TalkerAliasAssembler.AppendAdd", c.name + " header and blocks", 0, func() {
				alias.Reset()
				for i := range tas {
					_, _, _ = alias.AppendAdd(text[:0], &tas[i])
				}
			}},
		)
	}
	bad := pdu.FLCTalkerAliasHeader{TalkerAliasDataFormat: layer3.TalkerAliasUTF8, TalkerAliasDataLength: 2, TalkerAliasData: 0xC328 << 32}
	if _, err := pdu.AppendTalkerAlias(nil, &bad); err == nil {
		t.Fatal("bad UTF-8 alias decodes")
	}
	_ = kept
	return append(out,
		entry{"layer2/pdu.AppendTalkerAlias", "invalid UTF-8", 0, func() { _, _ = pdu.AppendTalkerAlias(text[:0], &bad) }},
		entry{"layer2/pdu.DecodeTalkerAlias", "invalid UTF-8", 1, func() { kept, _ = pdu.DecodeTalkerAlias(&bad) }},
	)
}
