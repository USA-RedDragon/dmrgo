package sms_test

import (
	"bytes"
	"encoding/hex"
	"errors"
	"testing"
	"unicode/utf8"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/internal/appcapture"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/packet"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
	"github.com/USA-RedDragon/dmrgo/v2/shortdata"
	"github.com/USA-RedDragon/dmrgo/v2/sms"
)

const (
	maxData    = 128
	maxMessage = 127 * 24
)

func FuzzDecode(f *testing.F) {
	decodeSeeds(func(defined bool, code uint8, data []byte, from, to uint8, repeat uint16) {
		f.Add(defined, code, data, from, to, repeat)
	})
	f.Fuzz(fuzzDecode)
}

func decodeSeeds(add func(defined bool, code uint8, data []byte, from, to uint8, repeat uint16)) {
	for _, s := range []string{appcapture.TMS, appcapture.DMRText} {
		b, _ := hex.DecodeString(s)
		add(false, uint8(enums.SAPIPBasedPacketData), b, 0, 0, 0)
	}
	h, _ := hex.DecodeString(appcapture.HSMS)
	add(true, uint8(enums.DDBCD), h, 0, 0, 0)
	add(true, uint8(enums.DDBCD), h, 2, 4, 200)
	add(true, uint8(enums.DDUTF16BE), []byte{0, 'H', 0, 'i'}, 0, 0, 0)
	add(true, uint8(enums.DDUTF16BE), []byte{0, 'H', 0, 'i'}, 0, 4, 300)
	g, _ := hex.DecodeString(appcapture.GPS)
	add(false, uint8(enums.SAPUDPIPHeaderCompression), g, 0, 0, 0)
}

func expandMessage(data []byte, from, to uint8, repeat uint16) []byte {
	data = data[:min(len(data), maxData)]
	j := min(int(to), len(data))
	i := min(int(from), j)
	n := 0
	if i < j {
		n = min(int(repeat), (maxMessage-len(data))/(j-i))
	}
	msg := make([]byte, 0, len(data)+n*(j-i))
	msg = append(msg, data[:j]...)
	for range n {
		msg = append(msg, data[i:j]...)
	}
	return append(msg, data[j:]...)
}

func fuzzDecode(t *testing.T, defined bool, code uint8, raw []byte, from, to uint8, repeat uint16) {
	data := expandMessage(raw, from, to, repeat)
	var m packet.Message
	m.Data = data
	if defined {
		m.Header = setHeader(t, (*pdu.DataHeader).SetDefinedData, pdu.DefinedDataHeader{
			SAP: enums.SAPShortData, DefinedData: enums.DDFormat(code & 0x3F), LLIDDestination: 2, LLIDSource: 1, BitPadding: uint8(len(data) % 3 * 8),
		})
	} else {
		m.Header = setHeader(t, (*pdu.DataHeader).SetUnconfirmed, pdu.UnconfirmedDataHeader{
			SAP: enums.SAP(code & 0xF), LLIDDestination: 2, LLIDSource: 1,
		})
	}
	out, got, err := sms.Decode(nil, &m.Header, m.Data)
	if err != nil {
		return
	}
	if !utf8.Valid(got.Body) || len(out) != len(got.Body) || got.Source != 1 || got.Destination != 2 {
		t.Fatalf("Decode(%t, %d, %x) = %+v", defined, code, data, got)
	}
	var h sms.Header
	enc, err := sms.AppendPacket(nil, &got, &h)
	if errors.Is(err, shortdata.ErrCharacter) && bytes.ContainsRune(got.Body, utf8.RuneError) {
		return
	}
	if errors.Is(err, sms.ErrFormat) && len(got.Body) == 0 && got.Format == sms.FormatHytera {
		return
	}
	if err != nil {
		t.Fatalf("AppendPacket(%+v): %v", got, err)
	}
	back := packet.Message{Header: h.Data, Data: enc}
	_, again, err := sms.Decode(nil, &back.Header, back.Data)
	if err != nil || again.Format != got.Format || string(again.Body) != string(got.Body) {
		t.Fatalf("round trip of %+v gave %+v, %v", got, again, err)
	}
}
