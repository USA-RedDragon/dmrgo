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
		{"ACK", "200f36eb706ce1f995e3ce27ddad5d7f77fd757e68f8fae870342926881a063c20", 2, packet.ResponseACK},
		{"NACK out of sequence", "20a3362d7160e7e191d3cfc7ddad5d7f77fd757e695cfaa870342fe6871a2dbc70", 3, packet.ResponseNACKOutOfSequence},
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
			LLIDDestination:    3191868,
			LLIDSource:         3140598,
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
