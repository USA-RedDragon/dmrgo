package sms_test

import (
	"errors"
	"testing"
	"unicode/utf8"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
	"github.com/USA-RedDragon/dmrgo/v2/sms"
)

func FuzzDecodeUDT(f *testing.F) {
	udtSeeds(func(format, uab, pad uint8, data []byte) {
		f.Add(format, uab, pad, data)
	})
	f.Fuzz(fuzzDecodeUDT)
}

func udtSeeds(add func(format, uab, pad uint8, data []byte)) {
	add(uint8(enums.UDTISO7), 0, 13, []byte{0x83, 0x0F, 0x1C, 0x7F})
	add(uint8(enums.UDTISO8), 1, 2, []byte("dmrgo UDT text, two blocks"))
	add(uint8(enums.UDTUTF16BE), 0, 8, []byte{0xD8, 0x3D, 0xDC, 0xE1, 0, 'k'})
	add(uint8(enums.UDTUTF16BE), 0, 12, []byte{0xDC, 0x00, 0, 0, 0, 'a'})
	add(uint8(enums.UDTISO8), 3, 31, []byte("x"))
}

func fuzzDecodeUDT(t *testing.T, format, uab, pad uint8, data []byte) {
	h := setHeader(t, (*pdu.DataHeader).SetUDT, pdu.UDTHeader{
		UDTFormat: enums.UDTFormat(format & 0xF), UAB: uab & 3, PadNibble: pad & 0x1F, LLIDDestination: 2, LLIDSource: 1,
	})
	out, got, err := sms.Decode(nil, &h, data)
	if err != nil {
		return
	}
	if !utf8.Valid(got.Body) || len(out) != len(got.Body) || got.Format != sms.FormatUDT || got.Source != 1 || got.Destination != 2 {
		t.Fatalf("Decode(%d, %d, %d, %x) = %+v", format, uab, pad, data, got)
	}
	var back sms.Header
	enc, err := sms.AppendPacket(nil, &got, &back)
	if errors.Is(err, sms.ErrTextLength) && len(got.Body) == 0 {
		return
	}
	if err != nil {
		t.Fatalf("AppendPacket(%+v): %v", got, err)
	}
	_, again, err := sms.Decode(nil, &back.Data, enc)
	if err != nil || string(again.Body) != string(got.Body) || again.UDT.UDTFormat != got.UDT.UDTFormat {
		t.Fatalf("round trip of %+v gave %+v, %v", got, again, err)
	}
}
