package pdu_test

import (
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

func TestDataHeader_Readers(t *testing.T) {
	t.Parallel()

	sap := enums.SAPIPBasedPacketData
	tests := []struct {
		name         string
		set          func(h *pdu.DataHeader) error
		blocks       int
		blocksOK     bool
		sapOK        bool
		full, fullOK bool
	}{
		{"U_HEAD", func(h *pdu.DataHeader) error {
			return h.SetUnconfirmedDataHeader(pdu.UnconfirmedDataHeader{SAP: sap, BlocksToFollow: 5, FullMessageFlag: true})
		}, 5, true, true, true, true},
		{"C_HEAD", func(h *pdu.DataHeader) error {
			return h.SetConfirmedDataHeader(pdu.ConfirmedDataHeader{SAP: sap, BlocksToFollow: 7})
		}, 7, true, true, false, true},
		{"C_RHEAD", func(h *pdu.DataHeader) error {
			return h.SetResponsePacketHeader(pdu.ResponsePacketHeader{SAP: sap, BlocksToFollow: 1})
		}, 1, true, true, false, false},
		{"DD_HEAD", func(h *pdu.DataHeader) error {
			return h.SetDefinedDataHeader(pdu.DefinedDataHeader{SAP: sap, AppendedBlocks: 3, FullMessageFlag: true})
		}, 3, true, true, true, true},
		{"R_HEAD", func(h *pdu.DataHeader) error {
			return h.SetRawDataHeader(pdu.RawDataHeader{SAP: sap, AppendedBlocks: 2})
		}, 2, true, true, false, true},
		{"SP_HEAD", func(h *pdu.DataHeader) error {
			return h.SetStatusPrecodedHeader(pdu.StatusPrecodedHeader{SAP: sap})
		}, 0, true, true, false, false},
		{"UDT_HEAD", func(h *pdu.DataHeader) error {
			return h.SetUDTHeader(pdu.UDTHeader{SAP: sap, UAB: 3})
		}, 4, true, true, false, false},
		{"P_HEAD", func(h *pdu.DataHeader) error {
			return h.SetProprietaryHeader(pdu.ProprietaryHeader{SAP: sap})
		}, 0, false, false, false, false},
	}
	for _, tt := range tests {
		var h pdu.DataHeader
		if err := tt.set(&h); err != nil {
			t.Fatalf("%s: %v", tt.name, err)
		}
		if n, ok := h.BlocksToFollow(); n != tt.blocks || ok != tt.blocksOK {
			t.Errorf("%s: BlocksToFollow %d %v, want %d %v", tt.name, n, ok, tt.blocks, tt.blocksOK)
		}
		if got, ok := h.SAP(); ok != tt.sapOK || ok && got != sap {
			t.Errorf("%s: SAP %v %v", tt.name, got, ok)
		}
		if full, ok := h.FullMessage(); full != tt.full || ok != tt.fullOK {
			t.Errorf("%s: FullMessage %v %v, want %v %v", tt.name, full, ok, tt.full, tt.fullOK)
		}
	}
	var nilHeader *pdu.DataHeader
	if _, ok := nilHeader.BlocksToFollow(); ok {
		t.Error("nil BlocksToFollow ok")
	}
	if _, ok := nilHeader.SAP(); ok {
		t.Error("nil SAP ok")
	}
	if _, ok := nilHeader.FullMessage(); ok {
		t.Error("nil FullMessage ok")
	}
}
