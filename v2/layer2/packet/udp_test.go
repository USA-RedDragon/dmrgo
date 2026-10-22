package packet_test

import (
	"bytes"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/packet"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

func TestMessageUDPIPv4(t *testing.T) {
	t.Parallel()

	unconfirmed := func(sap enums.SAP) pdu.DataHeader {
		return pdu.DataHeader{Format: pdu.FormatUnconfirmed, UnconfirmedDataHeader: &pdu.UnconfirmedDataHeader{SAP: sap}}
	}
	confirmed := pdu.DataHeader{Format: pdu.FormatConfirmed, ConfirmedDataHeader: &pdu.ConfirmedDataHeader{SAP: enums.SAPUDPIPHeaderCompression}}
	defined := pdu.DataHeader{Format: pdu.FormatShortDataDefined, DefinedDataHeader: &pdu.DefinedDataHeader{SAP: enums.SAPUDPIPHeaderCompression}}
	tests := []struct {
		name    string
		header  pdu.DataHeader
		data    []byte
		ok      bool
		src     uint16
		dst     uint16
		payload []byte
	}{
		{"both ports in extended headers", unconfirmed(enums.SAPUDPIPHeaderCompression), []byte{0, 1, 0, 0, 0, 0x27, 0x7e, 0x27, 0x7e, 'x'}, true, 10110, 10110, []byte("x")},
		{"no extended header", unconfirmed(enums.SAPUDPIPHeaderCompression), []byte{0, 1, 0, 1, 2, 'a', 'b'}, true, 5016, 5017, []byte("ab")},
		{"source port only", unconfirmed(enums.SAPUDPIPHeaderCompression), []byte{0, 1, 0, 0, 1, 0x12, 0x34, 'a'}, true, 0x1234, 5016, []byte("a")},
		{"destination port only", confirmed, []byte{0, 1, 0, 2, 0, 0x12, 0x34}, true, 5017, 0x1234, []byte{}},
		{"short header", unconfirmed(enums.SAPUDPIPHeaderCompression), []byte{0, 1, 0, 0}, false, 0, 0, nil},
		{"short extended header", unconfirmed(enums.SAPUDPIPHeaderCompression), []byte{0, 1, 0, 0, 0, 0x27, 0x7e, 0x27}, false, 0, 0, nil},
		{"other SAP", unconfirmed(enums.SAPIPBasedPacketData), []byte{0, 1, 0, 1, 2, 'a'}, false, 0, 0, nil},
		{"short data header", defined, []byte{0, 1, 0, 1, 2, 'a'}, false, 0, 0, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			m := packet.Message{Header: tt.header, Data: tt.data}
			h, payload, ok := m.UDPIPv4()
			if ok != tt.ok {
				t.Fatalf("ok = %t, want %t", ok, tt.ok)
			}
			if !ok {
				if payload != nil || h != (pdu.UDPIPv4CompressedHeader{}) {
					t.Fatalf("not ok but header %+v payload %q", h, payload)
				}
				return
			}
			if src, dst := h.Ports(); src != tt.src || dst != tt.dst || h.IPv4Identification != 1 {
				t.Errorf("header %+v ports %d %d, want %d %d", h, src, dst, tt.src, tt.dst)
			}
			if !bytes.Equal(payload, tt.payload) {
				t.Errorf("payload %q, want %q", payload, tt.payload)
			}
		})
	}
}
