//go:build !tinygo

package zeroalloc_test

import (
	"slices"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/bit"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
	"github.com/USA-RedDragon/dmrgo/v2/layer3"
)

// dataHeaderEntries returns the entries of the data header decoders and
// encoders, for a header of each format and the data header of a capture.
func dataHeaderEntries(t testing.TB, capture []bit.Bit) []entry {
	t.Helper()
	const api = "layer2/pdu."
	cases := []struct {
		name    string
		h       pdu.DataHeader
		payload func(h *pdu.DataHeader) entry
	}{
		{"U_HEAD", pdu.DataHeader{Format: pdu.FormatUnconfirmed, UnconfirmedDataHeader: &pdu.UnconfirmedDataHeader{Group: true, SAP: 4, FullMessageFlag: true, BlocksToFollow: 3}}, func(h *pdu.DataHeader) entry {
			return encodeEntry(api+"UnconfirmedDataHeader.Encode", "U_HEAD", h.UnconfirmedDataHeader, (*pdu.UnconfirmedDataHeader).Encode)
		}},
		{"C_HEAD", pdu.DataHeader{Format: pdu.FormatConfirmed, ConfirmedDataHeader: &pdu.ConfirmedDataHeader{BlocksToFollow: 2}}, func(h *pdu.DataHeader) entry {
			return encodeEntry(api+"ConfirmedDataHeader.Encode", "C_HEAD", h.ConfirmedDataHeader, (*pdu.ConfirmedDataHeader).Encode)
		}},
		{"C_RHEAD", pdu.DataHeader{Format: pdu.FormatResponsePacket, ResponsePacketHeader: &pdu.ResponsePacketHeader{BlocksToFollow: 1}}, func(h *pdu.DataHeader) entry {
			return encodeEntry(api+"ResponsePacketHeader.Encode", "C_RHEAD", h.ResponsePacketHeader, (*pdu.ResponsePacketHeader).Encode)
		}},
		{"DD_HEAD", pdu.DataHeader{Format: pdu.FormatShortDataDefined, DefinedDataHeader: &pdu.DefinedDataHeader{AppendedBlocks: 1}}, func(h *pdu.DataHeader) entry {
			return encodeEntry(api+"DefinedDataHeader.Encode", "DD_HEAD", h.DefinedDataHeader, (*pdu.DefinedDataHeader).Encode)
		}},
		{"SP_HEAD", pdu.DataHeader{Format: pdu.FormatShortDataRawOrStatusPrecoded, StatusPrecodedHeader: &pdu.StatusPrecodedHeader{}}, func(h *pdu.DataHeader) entry {
			return encodeEntry(api+"StatusPrecodedHeader.Encode", "SP_HEAD", h.StatusPrecodedHeader, (*pdu.StatusPrecodedHeader).Encode)
		}},
		{"R_HEAD", pdu.DataHeader{Format: pdu.FormatShortDataRawOrStatusPrecoded, RawDataHeader: &pdu.RawDataHeader{AppendedBlocks: 2}}, func(h *pdu.DataHeader) entry {
			return encodeEntry(api+"RawDataHeader.Encode", "R_HEAD", h.RawDataHeader, (*pdu.RawDataHeader).Encode)
		}},
		{"UDT_HEAD", pdu.DataHeader{Format: pdu.FormatUnifiedDataTransport, UDTHeader: &pdu.UDTHeader{}}, func(h *pdu.DataHeader) entry {
			return encodeEntry(api+"UDTHeader.Encode", "UDT_HEAD", h.UDTHeader, (*pdu.UDTHeader).Encode)
		}},
		{"P_HEAD", pdu.DataHeader{Format: pdu.FormatProprietary, ProprietaryHeader: &pdu.ProprietaryHeader{}}, func(h *pdu.DataHeader) entry {
			return encodeEntry(api+"ProprietaryHeader.Encode", "P_HEAD", h.ProprietaryHeader, (*pdu.ProprietaryHeader).Encode)
		}},
	}
	var p pdu.DataHeaderPayloads
	var dst pdu.DataHeader
	var out []entry
	for _, c := range cases {
		h := c.h
		h.SetAddresses(9, 3191868)
		bits := must(h.Encode())
		pdu.DecodeDataHeaderInto(&dst, &p, bits)
		if dst.FEC.Uncorrectable || dst.Format != h.Format {
			t.Fatalf("data header %s does not decode", c.name)
		}
		out = append(out,
			entry{api + "DecodeDataHeader", c.name, 1, func() { _ = pdu.DecodeDataHeader(bits) }},
			entry{api + "DecodeDataHeaderInto", c.name, 0, func() { pdu.DecodeDataHeaderInto(&dst, &p, bits) }},
			encodeEntry(api+"DataHeader.Encode", c.name, &h, (*pdu.DataHeader).Encode),
		)
		decoded := pdu.DecodeDataHeader(bits)
		out = append(out, c.payload(&decoded))
	}
	bits := [96]bit.Bit(capture)
	h := pdu.DecodeDataHeader(bits)
	hb := pdu.DataHeaderBytes(bit.Pack96(bits))
	return append(out,
		entry{api + "DecodeDataHeader", "capture", 1, func() { _ = pdu.DecodeDataHeader(bits) }},
		entry{api + "DecodeDataHeaderInto", "capture", 0, func() { pdu.DecodeDataHeaderInto(&dst, &p, bits) }},
		entry{api + "DataHeader.Addresses", "capture", 0, func() { _, _, _ = h.Addresses() }},
		entry{api + "DataHeader.SetAddresses", "capture", 0, func() { _ = h.SetAddresses(9, 3191868) }},
		entry{api + "DataHeader.DataType", "capture", 0, func() { _ = h.DataType() }},
		entry{api + "DataHeaderBytes.Format", "capture", 0, func() { _ = hb.Format() }},
		entry{api + "DataHeaderBytes.Addresses", "capture", 0, func() { _, _, _ = hb.Addresses() }},
		entry{api + "DataHeaderBytes.SetAddresses", "capture", 0, func() { _ = hb.SetAddresses(9, 3191868) }},
		entry{api + "DataHeaderBytes.Group", "capture", 0, func() { _, _ = hb.Group() }},
		entry{api + "DataHeaderBytes.SetGroup", "capture", 0, func() { _ = hb.SetGroup(false) }},
		entry{api + "DataHeaderBytes.CRCOK", "capture", 0, func() { _ = hb.CRCOK() }},
		entry{api + "DataHeaderBytes.Reseal", "capture", 0, func() { hb.Reseal() }},
	)
}

// mbcEntries returns the entries of the MBC decoders and encoders, for a
// header of each payload and a two-block CG_AP continuation.
func mbcEntries(t testing.TB) []entry {
	t.Helper()
	const api = "layer2/pdu."
	opcodes := []struct {
		name string
		h    pdu.MBCHeader
	}{
		{"PV_GRANT", pdu.MBCHeader{Opcode: pdu.CSBKPrivateVoiceGrant, PrivateVoiceGrantPDU: &pdu.PrivateVoiceGrantPDU{}}},
		{"TV_GRANT", pdu.MBCHeader{Opcode: pdu.CSBKTalkgroupVoiceGrant, TalkgroupVoiceGrantPDU: &pdu.TalkgroupVoiceGrantPDU{PhysicalChannel: 0xFFF}}},
		{"BTV_GRANT", pdu.MBCHeader{Opcode: pdu.CSBKBroadcastTalkgroupVoiceGrant, BroadcastTalkgroupVoiceGrantPDU: &pdu.BroadcastTalkgroupVoiceGrantPDU{}}},
		{"PD_GRANT", pdu.MBCHeader{Opcode: pdu.CSBKPrivateDataGrant, PrivateDataGrantPDU: &pdu.PrivateDataGrantPDU{}}},
		{"TD_GRANT", pdu.MBCHeader{Opcode: pdu.CSBKTalkgroupDataGrant, TalkgroupDataGrantPDU: &pdu.TalkgroupDataGrantPDU{}}},
		{"PV_GRANT_DX", pdu.MBCHeader{Opcode: pdu.CSBKDuplexPrivateVoiceGrant, DuplexPrivateVoiceGrantPDU: &pdu.DuplexPrivateVoiceGrantPDU{}}},
		{"PD_GRANT_DX", pdu.MBCHeader{Opcode: pdu.CSBKDuplexPrivateDataGrant, DuplexPrivateDataGrantPDU: &pdu.DuplexPrivateDataGrantPDU{}}},
		{"PD_GRANT_MI", pdu.MBCHeader{Opcode: pdu.CSBKPrivateDataGrantMultiItem, PrivateDataGrantMultiItemPDU: &pdu.PrivateDataGrantMultiItemPDU{}}},
		{"C_MOVE", pdu.MBCHeader{Opcode: pdu.CSBKMove, MovePDU: &pdu.MovePDU{}}},
		{"C_BCAST", pdu.MBCHeader{Opcode: pdu.CSBKBroadcast, BroadcastPDU: &pdu.BroadcastPDU{}}},
		{"manufacturer FID", pdu.MBCHeader{Opcode: pdu.CSBKBroadcast, FID: 0x10, RawPayload: [8]byte{1, 2, 3}}},
	}
	var p pdu.MBCHeaderPayloads
	var dst pdu.MBCHeader
	var out []entry
	for _, c := range opcodes {
		h := c.h
		bits := must(h.Encode())
		pdu.DecodeMBCHeaderInto(&dst, &p, bits)
		if dst.FEC.Uncorrectable || dst.Opcode != h.Opcode {
			t.Fatalf("MBC header %s does not decode", c.name)
		}
		allocs := 1.0
		if h.FID != enums.FIDStandardized {
			allocs = 0
		}
		out = append(out,
			entry{api + "DecodeMBCHeader", c.name, allocs, func() { _ = pdu.DecodeMBCHeader(bits) }},
			entry{api + "DecodeMBCHeaderInto", c.name, 0, func() { pdu.DecodeMBCHeaderInto(&dst, &p, bits) }},
			encodeEntry(api+"MBCHeader.Encode", c.name, &h, (*pdu.MBCHeader).Encode),
		)
	}
	header := opcodes[1].h
	var cgap pdu.CGAPContinuation
	var mvap pdu.MVAPContinuation
	var bcap pdu.BCAPContinuation
	var vnap pdu.VNAPContinuation
	blocks := [][96]bit.Bit{must(cgap.Encode()), must(cgap.Encode())}
	pdu.SetMBCLastBlockCRC(blocks)
	cont := pdu.DecodeMBCContinuation(blocks[1])
	contBits := blocks[1]
	return append(out,
		entry{api + "DecodeMBCContinuation", "CG_AP", 0, func() { _ = pdu.DecodeMBCContinuation(contBits) }},
		encodeEntry(api+"MBCContinuation.Encode", "CG_AP", &cont, (*pdu.MBCContinuation).Encode),
		entry{api + "MBCContinuation.AsCGAP", "CG_AP", 0, func() { _ = cont.AsCGAP() }},
		entry{api + "MBCContinuation.AsMVAP", "CG_AP", 0, func() { _ = cont.AsMVAP() }},
		entry{api + "MBCContinuation.AsBCAP", "CG_AP", 0, func() { _ = cont.AsBCAP() }},
		entry{api + "MBCContinuation.AsVNAP", "CG_AP", 0, func() { _ = cont.AsVNAP() }},
		entry{api + "MBCContinuation.DataType", "CG_AP", 0, func() { _ = cont.DataType() }},
		entry{api + "MBCHeader.DataType", "TV_GRANT", 0, func() { _ = header.DataType() }},
		encodeEntry(api+"CGAPContinuation.Encode", "zero", &cgap, (*pdu.CGAPContinuation).Encode),
		encodeEntry(api+"MVAPContinuation.Encode", "zero", &mvap, (*pdu.MVAPContinuation).Encode),
		encodeEntry(api+"BCAPContinuation.Encode", "zero", &bcap, (*pdu.BCAPContinuation).Encode),
		encodeEntry(api+"VNAPContinuation.Encode", "zero", &vnap, (*pdu.VNAPContinuation).Encode),
		entry{api + "CGAPContinuation.DataType", "zero", 0, func() { _ = cgap.DataType() }},
		entry{api + "MVAPContinuation.DataType", "zero", 0, func() { _ = mvap.DataType() }},
		entry{api + "BCAPContinuation.DataType", "zero", 0, func() { _ = bcap.DataType() }},
		entry{api + "VNAPContinuation.DataType", "zero", 0, func() { _ = vnap.DataType() }},
		entry{api + "MBCLastBlockCRCOK", "two blocks", 0, func() { _ = pdu.MBCLastBlockCRCOK(blocks...) }},
		entry{api + "SetMBCLastBlockCRC", "two blocks", 0, func() { pdu.SetMBCLastBlockCRC(blocks) }},
	)
}

// shortLCEntries returns the entries of the Short LC decoders and encoders,
// for a Short LC of each payload, and of the system identity code helpers.
func shortLCEntries(t testing.TB) []entry {
	t.Helper()
	const api = "layer2/pdu."
	csys := &pdu.ShortLCCSysParms{Reg: true, CommonSlotCounter: 17}
	if err := csys.SetSysCode(pdu.SysCode{Model: pdu.NetworkModelSmall, Net: 5, Site: 3}); err != nil {
		t.Fatal(err)
	}
	psys := &pdu.ShortLCPSysParms{}
	if err := psys.SetSysCode(pdu.SysCode{Model: pdu.NetworkModelTiny, Net: 9, Site: 1}); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name    string
		s       pdu.ShortLC
		payload func(s *pdu.ShortLC) entry
	}{
		{"Nul_Msg", pdu.ShortLC{SLCO: enums.SLCONullMessage, NullMessage: &pdu.ShortLCNullMessage{}}, func(s *pdu.ShortLC) entry {
			return encodeEntry(api+"ShortLCNullMessage.Encode", "Nul_Msg", s.NullMessage, (*pdu.ShortLCNullMessage).Encode)
		}},
		{"Act_Updt", pdu.ShortLC{SLCO: enums.SLCOActivityUpdate, ActivityUpdate: &pdu.ShortLCActivityUpdate{HashTS1: pdu.AddressHash(91)}}, func(s *pdu.ShortLC) entry {
			return encodeEntry(api+"ShortLCActivityUpdate.Encode", "Act_Updt", s.ActivityUpdate, (*pdu.ShortLCActivityUpdate).Encode)
		}},
		{"C_SYS_Parms", pdu.ShortLC{SLCO: enums.SLCOCSysParms, CSysParms: csys}, func(s *pdu.ShortLC) entry {
			return encodeEntry(api+"ShortLCCSysParms.Encode", "C_SYS_Parms", s.CSysParms, (*pdu.ShortLCCSysParms).Encode)
		}},
		{"P_SYS_Parms", pdu.ShortLC{SLCO: enums.SLCOPSysParms, PSysParms: psys}, func(s *pdu.ShortLC) entry {
			return encodeEntry(api+"ShortLCPSysParms.Encode", "P_SYS_Parms", s.PSysParms, (*pdu.ShortLCPSysParms).Encode)
		}},
	}
	var p pdu.ShortLCPayloads
	var dst pdu.ShortLC
	var out []entry
	for _, c := range cases {
		s := c.s
		bits := must(s.Encode())
		pdu.DecodeShortLCInto(&dst, &p, bits)
		if dst.FEC.Uncorrectable || dst.SLCO != s.SLCO {
			t.Fatalf("Short LC %s does not decode", c.name)
		}
		out = append(out,
			entry{api + "DecodeShortLC", c.name, 1, func() { _ = pdu.DecodeShortLC(bits) }},
			entry{api + "DecodeShortLCInto", c.name, 0, func() { pdu.DecodeShortLCInto(&dst, &p, bits) }},
			encodeEntry(api+"ShortLC.Encode", c.name, &s, (*pdu.ShortLC).Encode),
		)
		decoded := pdu.DecodeShortLC(bits)
		out = append(out, c.payload(&decoded))
	}
	code := pdu.SysCode{Model: pdu.NetworkModelSmall, Net: 5, Site: 3, Par: 1}
	model := pdu.NetworkModelLarge
	return append(out,
		entry{api + "ShortLCCSysParms.NetID", "small", 0, func() { _ = csys.NetID() }},
		entry{api + "ShortLCCSysParms.SiteID", "small", 0, func() { _ = csys.SiteID() }},
		entry{api + "ShortLCCSysParms.SysCode", "small", 0, func() { _ = csys.SysCode() }},
		entry{api + "ShortLCCSysParms.SetSysCode", "small", 0, func() { _ = csys.SetSysCode(code) }},
		entry{api + "ShortLCPSysParms.NetID", "tiny", 0, func() { _ = psys.NetID() }},
		entry{api + "ShortLCPSysParms.SiteID", "tiny", 0, func() { _ = psys.SiteID() }},
		entry{api + "ShortLCPSysParms.SysCode", "tiny", 0, func() { _ = psys.SysCode() }},
		entry{api + "ShortLCPSysParms.SetSysCode", "tiny", 0, func() { _ = psys.SetSysCode(code) }},
		entry{api + "SysCode.Pack", "small", 0, func() { _, _ = code.Pack() }},
		entry{api + "SysCode.Pack14", "small", 0, func() { _, _ = code.Pack14() }},
		entry{api + "SysCode.NetSite", "small", 0, func() { _, _ = code.NetSite() }},
		entry{api + "SysCode.Unpack", "small", 0, func() { code.Unpack(0x4A35) }},
		entry{api + "SysCode.Unpack14", "small", 0, func() { code.Unpack14(0x128D) }},
		entry{api + "NetworkModel.NetBits", "large", 0, func() { _ = model.NetBits() }},
		entry{api + "NetworkModel.SiteBits", "large", 0, func() { _ = model.SiteBits() }},
		entry{api + "AddressHash", "talkgroup 91", 0, func() { _ = pdu.AddressHash(91) }},
	)
}

// blockEntries returns the entries of the PI header, data block, USBD,
// slot type, EMB, TACT, RC and UDP/IPv4 header decoders and encoders, and
// of the codeword helpers.
func blockEntries(t testing.TB, capture map[enums.DataType][]bit.Bit) []entry {
	t.Helper()
	const api = "layer2/pdu."
	piBits := bit.Unpack96([12]byte(hexBytes(t, realPIHeader, 12)))
	pi := pdu.DecodePIHeader(piBits)
	r12Bits := [96]bit.Bit(capture[enums.DataTypeRate12])
	r12 := pdu.DecodeRate12Data(r12Bits)
	var r34 pdu.Rate34Data
	r34.SetConfirmed(3, [16]byte{4, 5, 6})
	r34Bits := must(r34.Encode())
	var r1 pdu.Rate1Data
	r1.SetConfirmed(5, [22]byte{1, 2, 3})
	r1Bits := must(r1.Encode())
	usbd := pdu.UnifiedSingleBlockData{ServiceType: 1, Payload: [10]byte{0xAB, 0xCD}}
	usbdBits := must(usbd.Encode())
	st := pdu.SlotType{ColorCode: 1, DataType: enums.DataTypeCSBK}
	stBits := must(st.Encode())
	sti := st.Info()
	emb := pdu.EmbeddedSignalling{ColorCode: 1, LCSS: enums.LCSSFirstFragment}
	embBits := must(emb.Encode())
	embi := emb.Info()
	tact := pdu.TACT{AccessType: true, LCSS: enums.LCSSLastFragment}
	tactBits := must(tact.Encode())
	rc := pdu.ReverseChannel{RCCommand: 3}
	rcBits := must(rc.Encode())
	udp := pdu.UDPIPv4CompressedHeader{IPv4Identification: 0x1234}
	udpBits := must(udp.Encode())
	csbkCW := [12]byte(hexBytes(t, realPreambleCSBK, 12))
	prFill := pdu.PRFillBits()
	var fill pdu.PRFill
	var seal [12]byte
	return []entry{
		{api + "DecodePIHeader", "vector", 0, func() { _ = pdu.DecodePIHeader(piBits) }},
		encodeEntry(api+"PIHeader.Encode", "vector", &pi, (*pdu.PIHeader).Encode),
		{api + "PIHeader.DataType", "vector", 0, func() { _ = pi.DataType() }},
		{api + "DecodeRate12Data", "capture", 0, func() { _ = pdu.DecodeRate12Data(r12Bits) }},
		encodeEntry(api+"Rate12Data.Encode", "capture", &r12, (*pdu.Rate12Data).Encode),
		{api + "Rate12Data.DataType", "capture", 0, func() { _ = r12.DataType() }},
		{api + "Rate12Data.ConfirmedDBSN", "capture", 0, func() { _ = r12.ConfirmedDBSN() }},
		{api + "Rate12Data.ConfirmedCRC9", "capture", 0, func() { _ = r12.ConfirmedCRC9() }},
		{api + "Rate12Data.ConfirmedUserData", "capture", 0, func() { _ = r12.ConfirmedUserData() }},
		{api + "Rate12Data.ConfirmedCRC9OK", "capture", 0, func() { _ = r12.ConfirmedCRC9OK() }},
		{api + "Rate12Data.SetConfirmed", "capture", 0, func() { r12.SetConfirmed(1, [10]byte{1}) }},
		{api + "DecodeRate34Data", "confirmed", 0, func() { _ = pdu.DecodeRate34Data(r34Bits) }},
		encodeEntry(api+"Rate34Data.Encode", "confirmed", &r34, (*pdu.Rate34Data).Encode),
		{api + "Rate34Data.DataType", "confirmed", 0, func() { _ = r34.DataType() }},
		{api + "Rate34Data.ConfirmedDBSN", "confirmed", 0, func() { _ = r34.ConfirmedDBSN() }},
		{api + "Rate34Data.ConfirmedCRC9", "confirmed", 0, func() { _ = r34.ConfirmedCRC9() }},
		{api + "Rate34Data.ConfirmedUserData", "confirmed", 0, func() { _ = r34.ConfirmedUserData() }},
		{api + "Rate34Data.ConfirmedCRC9OK", "confirmed", 0, func() { _ = r34.ConfirmedCRC9OK() }},
		{api + "Rate34Data.SetConfirmed", "confirmed", 0, func() { r34.SetConfirmed(1, [16]byte{1}) }},
		{api + "DecodeRate1Data", "confirmed", 0, func() { _ = pdu.DecodeRate1Data(r1Bits) }},
		encodeEntry(api+"Rate1Data.Encode", "confirmed", &r1, (*pdu.Rate1Data).Encode),
		{api + "Rate1Data.DataType", "confirmed", 0, func() { _ = r1.DataType() }},
		{api + "Rate1Data.ConfirmedDBSN", "confirmed", 0, func() { _ = r1.ConfirmedDBSN() }},
		{api + "Rate1Data.ConfirmedCRC9", "confirmed", 0, func() { _ = r1.ConfirmedCRC9() }},
		{api + "Rate1Data.ConfirmedUserData", "confirmed", 0, func() { _ = r1.ConfirmedUserData() }},
		{api + "Rate1Data.ConfirmedCRC9OK", "confirmed", 0, func() { _ = r1.ConfirmedCRC9OK() }},
		{api + "Rate1Data.SetConfirmed", "confirmed", 0, func() { r1.SetConfirmed(5, [22]byte{1, 2, 3}) }},
		{api + "DecodeUnifiedSingleBlockData", "service 1", 0, func() { _ = pdu.DecodeUnifiedSingleBlockData(usbdBits) }},
		encodeEntry(api+"UnifiedSingleBlockData.Encode", "service 1", &usbd, (*pdu.UnifiedSingleBlockData).Encode),
		{api + "UnifiedSingleBlockData.DataType", "service 1", 0, func() { _ = usbd.DataType() }},
		{api + "DecodeSlotType", "CSBK", 0, func() { _ = pdu.DecodeSlotType(stBits) }},
		encodeEntry(api+"SlotType.Encode", "CSBK", &st, (*pdu.SlotType).Encode),
		{api + "SlotType.Info", "CSBK", 0, func() { _ = st.Info() }},
		{api + "SlotTypeInfo.ColorCode", "CSBK", 0, func() { _ = sti.ColorCode() }},
		{api + "SlotTypeInfo.SetColorCode", "CSBK", 0, func() { sti.SetColorCode(1) }},
		{api + "SlotTypeInfo.DataType", "CSBK", 0, func() { _ = sti.DataType() }},
		{api + "SlotTypeInfo.SetDataType", "CSBK", 0, func() { _ = sti.SetDataType(enums.DataTypeCSBK) }},
		{api + "SlotTypeInfo.SlotType", "CSBK", 0, func() { _ = sti.SlotType() }},
		{api + "SlotTypeInfo.Codeword", "CSBK", 0, func() { _ = sti.Codeword() }},
		{api + "DecodeSlotTypeCodeword", "CSBK", 0, func() { _, _ = pdu.DecodeSlotTypeCodeword(sti.Codeword()) }},
		{api + "DecodeEmbeddedSignalling", "first fragment", 0, func() { _ = pdu.DecodeEmbeddedSignalling(embBits) }},
		encodeEntry(api+"EmbeddedSignalling.Encode", "first fragment", &emb, (*pdu.EmbeddedSignalling).Encode),
		{api + "EmbeddedSignalling.Info", "first fragment", 0, func() { _ = emb.Info() }},
		{api + "EMBInfo.ColorCode", "first fragment", 0, func() { _ = embi.ColorCode() }},
		{api + "EMBInfo.SetColorCode", "first fragment", 0, func() { embi.SetColorCode(1) }},
		{api + "EMBInfo.PI", "first fragment", 0, func() { _ = embi.PI() }},
		{api + "EMBInfo.SetPI", "first fragment", 0, func() { embi.SetPI(false) }},
		{api + "EMBInfo.LCSS", "first fragment", 0, func() { _ = embi.LCSS() }},
		{api + "EMBInfo.SetLCSS", "first fragment", 0, func() { _ = embi.SetLCSS(enums.LCSSFirstFragment) }},
		{api + "EMBInfo.EmbeddedSignalling", "first fragment", 0, func() { _ = embi.EmbeddedSignalling() }},
		{api + "EMBInfo.Codeword", "first fragment", 0, func() { _ = embi.Codeword() }},
		{api + "DecodeEMBCodeword", "first fragment", 0, func() { _, _ = pdu.DecodeEMBCodeword(embi.Codeword()) }},
		{api + "DecodeTACT", "busy", 0, func() { _ = pdu.DecodeTACT(tactBits) }},
		encodeEntry(api+"TACT.Encode", "busy", &tact, (*pdu.TACT).Encode),
		{api + "DecodeReverseChannel", "command 3", 0, func() { _ = pdu.DecodeReverseChannel(rcBits) }},
		encodeEntry(api+"ReverseChannel.Encode", "command 3", &rc, (*pdu.ReverseChannel).Encode),
		{api + "DecodeUDPIPv4CompressedHeader", "id 0x1234", 0, func() { _ = pdu.DecodeUDPIPv4CompressedHeader(udpBits) }},
		encodeEntry(api+"UDPIPv4CompressedHeader.Encode", "id 0x1234", &udp, (*pdu.UDPIPv4CompressedHeader).Encode),
		{api + "PRFillBits", "idle", 0, func() { _ = pdu.PRFillBits() }},
		{api + "IsPRFill", "idle", 0, func() { _ = pdu.IsPRFill(prFill) }},
		{api + "PRFill.DataType", "idle", 0, func() { _ = fill.DataType() }},
		{api + "CRCMask", "CSBK", 0, func() { _, _, _ = pdu.CRCMask(enums.DataTypeCSBK) }},
		{api + "BlockCRCOK", "Pre_CSBK capture", 0, func() { _ = pdu.BlockCRCOK(csbkCW, enums.DataTypeCSBK) }},
		{api + "BlockCRCOK", "rate 1/2 data type", 0, func() { _ = pdu.BlockCRCOK(csbkCW, enums.DataTypeRate12) }},
		{api + "Seal", "CSBK", 0, func() { _ = pdu.Seal(&seal, enums.DataTypeCSBK) }},
		{api + "Seal", "Voice LC Header", 0, func() { _ = pdu.Seal(&seal, enums.DataTypeVoiceLCHeader) }},
		{api + "Seal", "rate 1/2 data type", 0, func() { _ = pdu.Seal(&seal, enums.DataTypeRate12) }},
	}
}

// layer3GenEntries returns the entries of the code that dmrgen generates in
// package layer3.
func layer3GenEntries(t testing.TB) []entry {
	t.Helper()
	so := layer3.ServiceOptions{IsEmergency: true, PriorityLevel: 2}
	soBits := must(so.Encode())
	cdef := layer3.CdefParms{Channel: 12, TXMHz: 438, TXKHz: 4000, RXMHz: 431, RXKHz: 4000}
	cdefBits := must(cdef.Encode())
	return []entry{
		{"layer3.DecodeServiceOptions", "emergency", 0, func() { _ = layer3.DecodeServiceOptions(soBits) }},
		encodeEntry("layer3.ServiceOptions.Encode", "emergency", &so, (*layer3.ServiceOptions).Encode),
		{"layer3.DecodeCdefParms", "UHF", 0, func() { _ = layer3.DecodeCdefParms(cdefBits) }},
		encodeEntry("layer3.CdefParms.Encode", "UHF", &cdef, (*layer3.CdefParms).Encode),
	}
}

// pduEntries returns the entries of package layer2/pdu and of the dmrgen
// output of package layer3.
func pduEntries(t testing.TB) []entry {
	t.Helper()
	capture := captureData(t)
	return slices.Concat(
		csbkEntries(t),
		flcEntries(t),
		dataHeaderEntries(t, capture[enums.DataTypeDataHeader]),
		mbcEntries(t),
		shortLCEntries(t),
		blockEntries(t, capture),
		layer3GenEntries(t),
	)
}
