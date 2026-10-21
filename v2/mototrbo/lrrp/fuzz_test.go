package lrrp_test

import (
	"bytes"
	"encoding/hex"
	"errors"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/mototrbo/lrrp"
)

func FuzzDecodeMessage(f *testing.F) {
	messageSeeds(func(data []byte) { f.Add(data) })
	f.Fuzz(fuzzDecodeMessage)
}

func messageSeeds(add func(data []byte)) {
	for _, s := range []string{specReport, "070c22042468ace0390503515355", "090922042468ace034313c", "0d0f22042468ace066118ecd8d118ad47b",
		"110722042468ace038", "0509220300000751546257", "090c22030000033431817a788374"} {
		b, _ := hex.DecodeString(s)
		add(b)
	}
	add([]byte{0x0d, 0x0e, 0x69, 0x10, 0x00, 0x00, 0x00, 0xf0, 0x00, 0x00, 0x00, 0x81, 0x00, 0x20, 0x70, 0x40, 0x10})
}

var msg lrrp.Message

func fuzzDecodeMessage(t *testing.T, data []byte) {
	m := &msg
	err := lrrp.DecodeMessageInto(m, data)
	if err != nil {
		if !errors.Is(err, lrrp.ErrShort) && !errors.Is(err, lrrp.ErrLength) && !errors.Is(err, lrrp.ErrToken) && !errors.Is(err, lrrp.ErrVarint) {
			t.Fatalf("DecodeMessageInto(%x): unexpected error %v", data, err)
		}
		return
	}
	out, err := m.AppendBinary(nil)
	if err != nil {
		t.Fatalf("AppendBinary of %x: %v", data, err)
	}
	if err := lrrp.DecodeMessageInto(m, out); err != nil {
		t.Fatalf("DecodeMessageInto(%x) of %x: %v", out, data, err)
	}
	again, err := m.AppendBinary(nil)
	if err != nil || !bytes.Equal(again, out) {
		t.Fatalf("%x encodes to %x, then %x, %v", data, out, again, err)
	}
}
