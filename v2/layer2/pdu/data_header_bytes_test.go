package pdu_test

import (
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/bit"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

func TestDataHeaderBytes_MatchesGeneratedCodec(t *testing.T) {
	t.Parallel()

	headers := []pdu.DataHeader{
		{Format: pdu.FormatUnifiedDataTransport, UDTHeader: &pdu.UDTHeader{LLIDDestination: 1, LLIDSource: 2, Opcode: pdu.UDTOpcodeOutboundHeader, AppendedBlocks: 1}},
		{Format: pdu.FormatResponsePacket, ResponsePacketHeader: &pdu.ResponsePacketHeader{LLIDDestination: 1, LLIDSource: 2, BlocksToFollow: 1}},
		{Format: pdu.FormatUnconfirmed, UnconfirmedDataHeader: &pdu.UnconfirmedDataHeader{Group: true, LLIDDestination: 1, LLIDSource: 2, BlocksToFollow: 3}},
		{Format: pdu.FormatConfirmed, ConfirmedDataHeader: &pdu.ConfirmedDataHeader{LLIDDestination: 1, LLIDSource: 2, BlocksToFollow: 3}},
		{Format: pdu.FormatShortDataDefined, DefinedDataHeader: &pdu.DefinedDataHeader{LLIDDestination: 1, LLIDSource: 2, AppendedBlocks: 2}},
		{Format: pdu.FormatShortDataRawOrStatusPrecoded, RawDataHeader: &pdu.RawDataHeader{Group: true, LLIDDestination: 1, LLIDSource: 2, AppendedBlocks: 2}},
		{Format: pdu.FormatShortDataRawOrStatusPrecoded, StatusPrecodedHeader: &pdu.StatusPrecodedHeader{LLIDDestination: 1, LLIDSource: 2, StatusPrecoded: 0x155}},
	}
	for _, dh := range headers {
		enc := must(dh.Encode())
		h := pdu.DataHeaderBytes(bit.Pack96(enc))
		if !h.CRCOK() || h.Format() != dh.Format {
			t.Fatalf("%s: CRC %t format %s", dh.Format, h.CRCOK(), h.Format())
		}
		wantT, wantS, _ := dh.Addresses()
		if gotT, gotS, ok := h.Addresses(); !ok || gotT != wantT || gotS != wantS {
			t.Fatalf("%s: Addresses %d %d %t", dh.Format, gotT, gotS, ok)
		}
		group := dh.Format != pdu.FormatResponsePacket
		if !h.SetAddresses(0xABCDEF, 0x123456) || h.SetGroup(true) != group {
			t.Fatalf("%s: SetAddresses or SetGroup refused", dh.Format)
		}
		h.Reseal()
		got := pdu.DecodeDataHeader(bit.Unpack96(h))
		res := got.FEC
		if res.HasErrors() {
			t.Fatalf("%s: edited header %+v", dh.Format, res)
		}
		want := dh
		want.SetAddresses(0xABCDEF, 0x123456)
		switch {
		case want.UnconfirmedDataHeader != nil:
			want.UnconfirmedDataHeader.Group = group
		case want.ConfirmedDataHeader != nil:
			want.ConfirmedDataHeader.Group = group
		case want.DefinedDataHeader != nil:
			want.DefinedDataHeader.Group = group
		case want.RawDataHeader != nil:
			want.RawDataHeader.Group = group
		case want.StatusPrecodedHeader != nil:
			want.StatusPrecodedHeader.Group = group
		case want.UDTHeader != nil:
			want.UDTHeader.Group = group
		}
		if must(got.Encode()) != must(want.Encode()) {
			t.Fatalf("%s: edited header decodes as %s, want %s", dh.Format, got.String(), want.String())
		}
	}
}
