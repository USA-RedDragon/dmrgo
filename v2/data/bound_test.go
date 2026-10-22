package data_test

import (
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/data"
)

func payloadOctets(ev []data.Event) int {
	n := 0
	for i := range ev {
		n += len(ev[i].Message.Payload)
	}
	return n
}

func TestMaxOutputOctets(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		cfg  *data.ReceiverConfig
		want int
	}{
		{nil, data.DefaultMaxFragment + data.DefaultMaxMessage},
		{&data.ReceiverConfig{MaxFragment: 100, MaxMessage: 200}, 300},
		{&data.ReceiverConfig{MaxFragment: 10, MaxMessage: 20}, 48 + 20},
	} {
		if got := newReceiver(t, c.cfg).MaxOutputOctets(); got != c.want {
			t.Errorf("%+v: MaxOutputOctets %d, want %d", c.cfg, got, c.want)
		}
	}
	var r *data.Receiver
	if r.MaxOutputOctets() != 0 {
		t.Fatal("nil Receiver")
	}
}

func TestMaxOutputOctetsReassembled(t *testing.T) {
	t.Parallel()
	bursts, joined := fragmentBursts(t, []uint8{0, 1, 2, 3, 4, 5, 6, 7, 1, 2 | 8}, 30)
	r := newReceiver(t, &data.ReceiverConfig{MaxFragment: 30, MaxMessage: len(joined)})
	if ev := replay(t, r, bursts); count(ev, data.EventReassembled) != 1 {
		t.Fatalf("%s", kinds(ev))
	}
}
