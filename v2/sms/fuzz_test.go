package sms_test

import (
	"encoding/hex"
	"testing"
	"unicode/utf8"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/internal/appcapture"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/packet"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
	"github.com/USA-RedDragon/dmrgo/v2/sms"
)

func FuzzDecode(f *testing.F) {
	decodeSeeds(func(defined bool, code uint8, data []byte) { f.Add(defined, code, data) })
	f.Fuzz(fuzzDecode)
}

func decodeSeeds(add func(defined bool, code uint8, data []byte)) {
	for _, s := range []string{appcapture.TMS, appcapture.DMRText} {
		b, _ := hex.DecodeString(s)
		add(false, uint8(enums.SAPIPBasedPacketData), b)
	}
	h, _ := hex.DecodeString(appcapture.HSMS)
	add(true, uint8(enums.DDBCD), h)
	add(true, uint8(enums.DDUTF16BE), []byte{0, 'H', 0, 'i'})
	g, _ := hex.DecodeString(appcapture.GPS)
	add(false, uint8(enums.SAPUDPIPHeaderCompression), g)
}

func fuzzDecode(t *testing.T, defined bool, code uint8, data []byte) {
	var m packet.Message
	m.Data = data
	if defined {
		m.Header = pdu.DataHeader{Format: pdu.FormatShortDataDefined, DefinedDataHeader: &pdu.DefinedDataHeader{
			SAP: enums.SAPShortData, DefinedData: enums.DDFormat(code & 0x3F), LLIDDestination: 2, LLIDSource: 1, BitPadding: uint8(len(data) % 3 * 8),
		}}
	} else {
		m.Header = pdu.DataHeader{Format: pdu.FormatUnconfirmed, UnconfirmedDataHeader: &pdu.UnconfirmedDataHeader{
			SAP: enums.ServiceAccessPointID(code & 0xF), LLIDDestination: 2, LLIDSource: 1,
		}}
	}
	out, got, err := sms.Decode(nil, &m.Header, m.Data)
	if err != nil {
		return
	}
	if !utf8.Valid(got.Body) || len(out) != len(got.Body) || got.Source != 1 || got.Destination != 2 {
		t.Fatalf("Decode(%t, %d, %x) = %+v", defined, code, data, got)
	}
	var h sms.Header
	enc, err := sms.AppendPacket(nil, &got, false, &h)
	if err != nil {
		t.Fatalf("AppendPacket(%+v): %v", got, err)
	}
	back := packet.Message{Header: h.Data, Data: enc}
	_, again, err := sms.Decode(nil, &back.Header, back.Data)
	if err != nil || again.Format != got.Format || string(again.Body) != string(got.Body) {
		t.Fatalf("round trip of %+v gave %+v, %v", got, again, err)
	}
}
