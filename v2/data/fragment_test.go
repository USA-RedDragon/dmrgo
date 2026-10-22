package data_test

import (
	"bytes"
	"testing"
	"time"

	"github.com/USA-RedDragon/dmrgo/v2/data"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

func fragmentBursts(t *testing.T, fsn []uint8, n int) ([]timedBurst, []byte) {
	t.Helper()
	all := make([]timedBurst, 0, 8*len(fsn))
	joined := make([]byte, 0, n*len(fsn))
	for i, f := range fsn {
		h := confirmedHeader(t, uint8(i)&7)
		editConfirmed(t, &h, func(c *pdu.ConfirmedDataHeader) { c.FragmentSequenceNumber = f })
		p := payload(n)
		p[0] = byte(i)
		joined = append(joined, p...)
		all = append(all, timed(time.Duration(i)*time.Second, build(t, h, p, enums.DataTypeRate12))...)
	}
	return all, joined
}

func count(ev []data.Event, k data.EventKind) int {
	n := 0
	for _, e := range ev {
		if e.Kind == k {
			n++
		}
	}
	return n
}

func TestReceiverFragments(t *testing.T) {
	t.Parallel()
	fsn := []uint8{0, 1, 2, 3, 4, 5, 6, 7, 1, 2 | 8}
	bursts, joined := fragmentBursts(t, fsn, 30)
	for _, respond := range []bool{false, true} {
		ev := replay(t, newReceiver(t, &data.ReceiverConfig{Respond: respond, Address: dstID}), bursts)
		if count(ev, data.EventDelivered) != len(fsn) || count(ev, data.EventReassembled) != 1 {
			t.Fatalf("respond %v: %s", respond, kinds(ev))
		}
		resp, _ := responses(t, ev)
		if respond && resp != "ACK+ACK+ACK+ACK+ACK+ACK+ACK+ACK+ACK+ACK" {
			t.Errorf("responses %q", resp)
		}
		frag := 0
		for _, e := range ev {
			switch e.Kind {
			case data.EventDelivered:
				if e.Message.Fragmented {
					frag++
				}
			case data.EventReassembled:
				if !bytes.Equal(e.Message.Payload, joined) || e.Message.Fragments != uint16(len(fsn)) {
					t.Errorf("reassembled %d octets, %d fragments", len(e.Message.Payload), e.Message.Fragments)
				}
			case data.EventCallStart, data.EventResponse, data.EventRespond, data.EventCallEnd:
			}
		}
		if frag != len(fsn)-1 {
			t.Errorf("%d fragments marked, want %d", frag, len(fsn)-1)
		}
	}
}

func TestReceiverSingleFSN(t *testing.T) {
	t.Parallel()
	for _, fsn := range []uint8{0, 0b1000} {
		bursts, _ := fragmentBursts(t, []uint8{fsn}, 40)
		r := newReceiver(t, &data.ReceiverConfig{Respond: true, Address: dstID})
		ev := make([]data.Event, 0, data.MaxEvents)
		for i := range bursts {
			ev = r.Burst(bursts[i].at, &bursts[i].raw, layer2.Hint{}, ev[:0])
		}
		if count(ev, data.EventDelivered) != 1 || count(ev, data.EventReassembled) != 0 || ev[0].Message.Fragmented {
			t.Errorf("FSN %04b: last block gave %s", fsn, kinds(ev))
		}
	}
}

func TestReceiverFragmentErrors(t *testing.T) {
	t.Parallel()
	bursts, _ := fragmentBursts(t, []uint8{0, 2 | 8}, 30)
	ev := replay(t, newReceiver(t, &data.ReceiverConfig{Respond: true, Address: dstID}), bursts)
	if resp, _ := responses(t, ev); resp != "ACK+NACK FSN Out Of Sequence" || count(ev, data.EventDelivered) != 1 {
		t.Errorf("answering: %q, %s", resp, kinds(ev))
	}
	ev = replay(t, newReceiver(t, nil), bursts)
	if count(ev, data.EventDelivered) != 2 || count(ev, data.EventReassembled) != 0 {
		t.Errorf("bridge: %s", kinds(ev))
	}
	long, _ := fragmentBursts(t, []uint8{0b1000}, 60)
	ev = replay(t, newReceiver(t, &data.ReceiverConfig{Respond: true, Address: dstID, MaxFragment: 50}), long)
	if resp, _ := responses(t, ev); resp != "NACK Memory Full" || count(ev, data.EventDelivered) != 0 {
		t.Errorf("MaxFragment: %q, %s", resp, kinds(ev))
	}
	small, _ := fragmentBursts(t, []uint8{0, 1 | 8}, 30)
	ev = replay(t, newReceiver(t, &data.ReceiverConfig{MaxMessage: 40}), small)
	if count(ev, data.EventDelivered) != 2 || count(ev, data.EventReassembled) != 0 {
		t.Errorf("MaxMessage: %s", kinds(ev))
	}
	for _, cfg := range []data.ReceiverConfig{{MaxFragment: data.MaxMessage + 1}, {MaxMessage: -1}} {
		if _, err := data.NewReceiver(&cfg); err == nil {
			t.Errorf("%+v: no error", cfg)
		}
	}
}
