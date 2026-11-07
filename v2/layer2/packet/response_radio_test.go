package packet_test

import (
	"encoding/hex"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/packet"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

func TestNewResponse_MatchesRadio(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name  string
		burst string
		ns    uint8
		resp  packet.Response
	}{
		{"ACK", "2d3e2cf14f68e99196c12f855dad5d7f77fd757e68c4ab0061b54ce4cb1f9fbd54", 2, packet.ResponseACK},
		{"NACK out of sequence", "2d922c374e64ef8992f12e655dad5d7f77fd757e6960ab4061b54a24c41fb43d04", 3, packet.ResponseNACKOutOfSequence},
	} {
		raw, err := hex.DecodeString(c.burst)
		if err != nil {
			t.Fatal(err)
		}
		var rb layer2.RawBurst
		copy(rb[:], raw)
		b, err := layer2.NewBurstFromBytes(rb)
		if err != nil {
			t.Fatalf("%s: decode: %v", c.name, err)
		}
		dh, ok := b.Data.(*pdu.DataHeader)
		if !ok {
			t.Fatalf("%s: burst carries %T, want a data header", c.name, b.Data)
		}
		want, err := dh.Encode()
		if err != nil {
			t.Fatalf("%s: encode radio header: %v", c.name, err)
		}
		to := &pdu.ConfirmedDataHeader{
			ResponseRequested:  true,
			SAP:                enums.SAPIPBasedPacketData,
			LLIDDestination:    3009999,
			LLIDSource:         3009998,
			FullMessageFlag:    true,
			BlocksToFollow:     9,
			SendSequenceNumber: c.ns,
		}
		p, err := packet.NewResponse(to, c.resp, nil)
		if err != nil {
			t.Fatalf("%s: NewResponse: %v", c.name, err)
		}
		got, err := p.Header.Encode()
		if err != nil {
			t.Fatalf("%s: encode: %v", c.name, err)
		}
		if got != want {
			t.Errorf("%s: NewResponse header %x, radio sent %x", c.name, got, want)
		}
	}
}
