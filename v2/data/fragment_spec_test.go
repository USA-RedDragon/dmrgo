package data_test

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/USA-RedDragon/dmrgo/v2/data"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
)

// ETSI TS 102 361-1 V2.6.1 clause 8.2.0, Table 8.2.
func TestReceiverSpecFragmentExample(t *testing.T) {
	t.Parallel()
	fsn := []uint8{0b0000, 0b0001, 0b0010, 0b0011, 0b0100, 0b0101, 0b0110, 0b0111, 0b0001, 0b0010, 0b0011, 0b0100, 0b0101, 0b1110}
	const fragMax = 1500
	all := make([]timedBurst, 0, 70*len(fsn))
	joined := make([]byte, 0, fragMax*len(fsn))
	for i, f := range fsn {
		h := confirmedHeader(uint8(i) & 7)
		h.ConfirmedDataHeader.FragmentSequenceNumber = f
		p := payload(fragMax)
		p[0], p[1] = byte(i), f
		joined = append(joined, p...)
		all = append(all, timed(time.Duration(i)*10*time.Second, build(t, h, p, enums.DataTypeRate1))...)
	}
	r := newReceiver(t, &data.ReceiverConfig{Respond: true, Address: dstID, MaxMessage: fragMax * len(fsn)})
	ev := replay(t, r, all)
	if count(ev, data.EventDelivered) != len(fsn) || count(ev, data.EventReassembled) != 1 {
		t.Fatalf("%d delivered, %d reassembled", count(ev, data.EventDelivered), count(ev, data.EventReassembled))
	}
	for _, e := range ev {
		if e.Kind == data.EventReassembled && (!bytes.Equal(e.Message.Payload, joined) || e.Message.Fragments != uint16(len(fsn))) {
			t.Errorf("reassembled %d octets of %d fragments, want %d of %d", len(e.Message.Payload), e.Message.Fragments, len(joined), len(fsn))
		}
	}
	if resp, _ := responses(t, ev); resp != strings.Repeat("ACK+", len(fsn)-1)+"ACK" {
		t.Errorf("responses %q", resp)
	}
}
