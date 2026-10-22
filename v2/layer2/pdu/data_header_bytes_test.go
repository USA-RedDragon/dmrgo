package pdu_test

import (
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

func TestDataHeaderBytes_MatchesGeneratedCodec(t *testing.T) {
	t.Parallel()

	headers := []pdu.DataHeader{
		dataHeaderBytesBuild(func(h *pdu.DataHeader) error {
			return h.SetUDT(pdu.UDTHeader{LLIDDestination: 1, LLIDSource: 2, Opcode: pdu.UDTOpcodeOutboundHeader, UAB: 1})
		}),
		dataHeaderBytesBuild(func(h *pdu.DataHeader) error {
			return h.SetResponse(pdu.ResponsePacketHeader{LLIDDestination: 1, LLIDSource: 2, BlocksToFollow: 1})
		}),
		dataHeaderBytesBuild(func(h *pdu.DataHeader) error {
			return h.SetUnconfirmed(pdu.UnconfirmedDataHeader{Group: true, LLIDDestination: 1, LLIDSource: 2, BlocksToFollow: 3})
		}),
		dataHeaderBytesBuild(func(h *pdu.DataHeader) error {
			return h.SetConfirmed(pdu.ConfirmedDataHeader{LLIDDestination: 1, LLIDSource: 2, BlocksToFollow: 3})
		}),
		dataHeaderBytesBuild(func(h *pdu.DataHeader) error {
			return h.SetDefinedData(pdu.DefinedDataHeader{LLIDDestination: 1, LLIDSource: 2, AppendedBlocks: 2})
		}),
		dataHeaderBytesBuild(func(h *pdu.DataHeader) error {
			return h.SetRawData(pdu.RawDataHeader{Group: true, LLIDDestination: 1, LLIDSource: 2, AppendedBlocks: 2})
		}),
		dataHeaderBytesBuild(func(h *pdu.DataHeader) error {
			return h.SetStatusPrecoded(pdu.StatusPrecodedHeader{LLIDDestination: 1, LLIDSource: 2, StatusPrecoded: 0x155})
		}),
	}
	for _, dh := range headers {
		enc := must(dh.Encode())
		h := pdu.DataHeaderBytes(enc)
		if !h.CRCOK() || h.Format() != dh.Format() {
			t.Fatalf("%s: CRC %t format %s", dh.Format(), h.CRCOK(), h.Format())
		}
		wantT, wantS, _ := dh.Addresses()
		if gotT, gotS, ok := h.Addresses(); !ok || gotT != wantT || gotS != wantS {
			t.Fatalf("%s: Addresses %d %d %t", dh.Format(), gotT, gotS, ok)
		}
		group := dh.Format() != pdu.FormatResponsePacket
		if h.SetAddresses(0xABCDEF, 0x123456) != nil || h.SetGroup(true) != group {
			t.Fatalf("%s: SetAddresses or SetGroup refused", dh.Format())
		}
		h.Reseal()
		got := pdu.DecodeDataHeader(h)
		res := got.FEC
		if res.HasErrors() {
			t.Fatalf("%s: edited header %+v", dh.Format(), res)
		}
		want := dh
		noErr(want.SetAddresses(0xABCDEF, 0x123456))
		if p, ok := want.AsUnconfirmed(); ok {
			p.Group = group
			noErr(want.SetUnconfirmed(p))
		} else if p, ok := want.AsConfirmed(); ok {
			p.Group = group
			noErr(want.SetConfirmed(p))
		} else if p, ok := want.AsDefinedData(); ok {
			p.Group = group
			noErr(want.SetDefinedData(p))
		} else if p, ok := want.AsRawData(); ok {
			p.Group = group
			noErr(want.SetRawData(p))
		} else if p, ok := want.AsStatusPrecoded(); ok {
			p.Group = group
			noErr(want.SetStatusPrecoded(p))
		} else if p, ok := want.AsUDT(); ok {
			p.Group = group
			noErr(want.SetUDT(p))
		}
		if must(got.Encode()) != must(want.Encode()) {
			t.Fatalf("%s: edited header decodes as %s, want %s", dh.Format(), got.String(), want.String())
		}
	}
}

func dataHeaderBytesBuild(set func(*pdu.DataHeader) error) pdu.DataHeader {
	var h pdu.DataHeader
	noErr(set(&h))
	return h
}
