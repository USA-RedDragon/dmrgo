package ars_test

import (
	"bytes"
	"errors"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/mototrbo/ars"
)

func FuzzDecodeMessage(f *testing.F) {
	messageSeeds(func(data []byte) { f.Add(data) })
	f.Fuzz(fuzzDecodeMessage)
}

func messageSeeds(add func(data []byte)) {
	add([]byte{0x00, 0x0c, 0xf0, 0x20, 0x07, '3', '1', '9', '1', '8', '6', '8', 0x00, 0x00})
	add([]byte{0x00, 0x02, 0xbf, 0x01})
	add([]byte{0x00, 0x01, 0x14})
	add([]byte{0x00, 0x05, 0x05, 0x01, 'a', 0x00, 0x00})
}

func fuzzDecodeMessage(t *testing.T, data []byte) {
	m, err := ars.DecodeMessage(data)
	if err != nil {
		if !errors.Is(err, ars.ErrShort) && !errors.Is(err, ars.ErrLength) {
			t.Fatalf("DecodeMessage(%x): unexpected error %v", data, err)
		}
		return
	}
	out, err := m.AppendBinary(nil)
	if err != nil || !bytes.Equal(out, data) {
		t.Fatalf("DecodeMessage(%x) encodes to %x, %v", data, out, err)
	}
	_, _ = m.DeviceID()
}
