package pdu_test

import (
	"errors"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

func TestByteViewsNilReceiver(t *testing.T) {
	t.Parallel()

	var l *pdu.LCBytes
	if l.FLCO() != 0 || l.ProtectFlag() || l.Reserved() || l.FID() != 0 {
		t.Error("nil LCBytes header readers")
	}
	if _, ok := l.ServiceOptions(); ok {
		t.Error("nil LCBytes ServiceOptions ok")
	}
	if _, _, ok := l.Addresses(); ok {
		t.Error("nil LCBytes Addresses ok")
	}
	if _, _, _, ok := l.Call(); ok {
		t.Error("nil LCBytes Call ok")
	}
	if _, err := l.Codeword(enums.DataTypeVoiceLCHeader); !errors.Is(err, dmrgo.ErrNilReceiver) {
		t.Errorf("nil LCBytes Codeword: %v", err)
	}

	var c *pdu.CSBKBytes
	if c.Opcode() != 0 || c.FID() != 0 || c.CRCOK() {
		t.Error("nil CSBKBytes readers")
	}
	if _, _, ok := c.Addresses(); ok {
		t.Error("nil CSBKBytes Addresses ok")
	}
	if _, ok := c.Group(); ok {
		t.Error("nil CSBKBytes Group ok")
	}

	var h *pdu.DataHeaderBytes
	if h.Format() != 0 || h.CRCOK() {
		t.Error("nil DataHeaderBytes readers")
	}
	if _, _, ok := h.Addresses(); ok {
		t.Error("nil DataHeaderBytes Addresses ok")
	}
	if _, ok := h.Group(); ok {
		t.Error("nil DataHeaderBytes Group ok")
	}
}
