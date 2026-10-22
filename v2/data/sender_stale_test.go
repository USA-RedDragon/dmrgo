package data_test

import (
	"testing"
	"time"

	"github.com/USA-RedDragon/dmrgo/v2/data"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/packet"
)

func responseNS(t *testing.T, r packet.Response, ns uint8) []layer2.RawBurst {
	t.Helper()
	ch := confirmedHeader(t, ns)
	h := confirmedOf(t, &ch)
	p, err := packet.NewResponse(&h, r, nil)
	if err != nil {
		t.Fatal(err)
	}
	b, err := packet.Bursts(&p, 0, layer2.SyncSourceBS)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestSenderIgnoresStaleACK(t *testing.T) {
	t.Parallel()
	s := newSender(t, &data.SenderConfig{Preambles: -1})
	if err := s.Send(0, message(t, 10)); err != nil {
		t.Fatal(err)
	}
	drain(t, s, time.Second)
	s.Cancel(time.Second, nil)
	if err := s.Send(2*time.Second, message(t, 10)); err != nil {
		t.Fatal(err)
	}
	out := drain(t, s, 3*time.Second)
	if s.State() != data.SenderWaitResponse || len(out) == 0 || out[0].Seq != 1 {
		t.Fatalf("state %v, outputs %+v", s.State(), out)
	}
	now := out[len(out)-1].Due + 60*time.Millisecond
	stale := responseNS(t, packet.ResponseACK, 0)
	if got := s.Burst(now, &stale[0], layer2.Hint{}, nil); len(got) != 0 {
		t.Fatalf("the ACK of N(S) 0 ended the message with N(S) 1: %+v", got)
	}
	ack := responseNS(t, packet.ResponseACK, 1)
	got := s.Burst(now, &ack[0], layer2.Hint{}, nil)
	if len(got) != 1 || got[0].Result != data.ResultDelivered || got[0].Seq != 1 {
		t.Fatalf("ACK of N(S) 1: %+v", got)
	}
}

func TestSenderIgnoresStaleNACK(t *testing.T) {
	t.Parallel()
	for _, code := range []packet.Response{packet.ResponseNACKUndeliverable, packet.ResponseNACKMemoryFull, packet.ResponseNACKInvalidUser, packet.ResponseNACKPacketCRC} {
		s := newSender(t, &data.SenderConfig{Preambles: -1})
		if err := s.Send(0, message(t, 10)); err != nil {
			t.Fatal(err)
		}
		drain(t, s, time.Second)
		s.Cancel(time.Second, nil)
		if err := s.Send(2*time.Second, message(t, 10)); err != nil {
			t.Fatal(err)
		}
		out := drain(t, s, 3*time.Second)
		now := out[len(out)-1].Due + 60*time.Millisecond
		stale := responseNS(t, code, 0)
		if got := s.Burst(now, &stale[0], layer2.Hint{}, nil); len(got) != 0 || s.State() != data.SenderWaitResponse {
			t.Fatalf("%v of N(S) 0 acted on the message with N(S) 1: %+v, state %v", code, got, s.State())
		}
		nack := responseNS(t, code, 1)
		got := s.Burst(now, &nack[0], layer2.Hint{}, nil)
		if code == packet.ResponseNACKPacketCRC {
			if len(got) != 0 || s.State() == data.SenderWaitResponse {
				t.Fatalf("%v of N(S) 1: %+v, state %v", code, got, s.State())
			}
			continue
		}
		if len(got) != 1 || got[0].Result != data.ResultRefused || got[0].Seq != 1 {
			t.Fatalf("%v of N(S) 1: %+v", code, got)
		}
	}
}
