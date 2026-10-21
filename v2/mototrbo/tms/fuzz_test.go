package tms_test

import (
	"bytes"
	"errors"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/mototrbo/tms"
)

func FuzzDecodeMessage(f *testing.F) {
	messageSeeds(func(data []byte) { f.Add(data) })
	f.Fuzz(fuzzDecodeMessage)
}

func messageSeeds(add func(data []byte)) {
	add([]byte{0x00, 0x1c, 0xa0, 0x00, 0x81, 0x04, 0x0d, 0x00, 0x0a, 0x00, 0x4d, 0x00, 0x4f, 0x00, 0x54, 0x00, 0x4f, 0x00,
		0x20, 0x00, 0x54, 0x00, 0x45, 0x00, 0x53, 0x00, 0x54, 0x00, 0x00, 0x00})
	add([]byte{0x00, 0x03, 0x9f, 0x00, 0x01})
	add([]byte{0x00, 0x03, 0x90, 0x00, 0x03})
	add([]byte{0x00, 0x05, 0x20, 0x02, 0x41, 0x00, 0x42})
}

func fuzzDecodeMessage(t *testing.T, data []byte) {
	m, err := tms.DecodeMessage(data)
	if err != nil {
		if !errors.Is(err, tms.ErrShort) && !errors.Is(err, tms.ErrLength) {
			t.Fatalf("DecodeMessage(%x): unexpected error %v", data, err)
		}
		return
	}
	out, err := m.AppendBinary(nil)
	if err != nil || !bytes.Equal(out, data) {
		t.Fatalf("DecodeMessage(%x) encodes to %x, %v", data, out, err)
	}
	_ = m.TextUTF8(nil)
}
