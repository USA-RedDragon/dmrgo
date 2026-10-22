package data_test

import (
	"testing"
	"time"

	"github.com/USA-RedDragon/dmrgo/v2/data"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
)

func TestReceiverSACKAfterLostLastBlockAtNextSlot(t *testing.T) {
	t.Parallel()
	ms := time.Millisecond
	bursts := build(t, confirmedHeader(t, 0), payload(30), enums.DataTypeRate12)
	last := len(bursts) - 1
	for _, c := range []struct {
		name  string
		delay time.Duration
		want  time.Duration
	}{
		{"next slot", 0, time.Duration(last+1) * 60 * ms},
		{"at once", -1, time.Duration(last-1)*60*ms + data.DefaultLost},
	} {
		ev := replay(t, newReceiver(t, &data.ReceiverConfig{Respond: true, ResponseDelay: c.delay}), timed(0, bursts, last))
		var due []time.Duration
		for _, e := range ev {
			if e.Kind == data.EventRespond {
				due = append(due, e.Due)
			}
		}
		if got, _ := responses(t, ev); got != "SACK" || len(due) == 0 {
			t.Fatalf("%s: responses %q, want one SACK", c.name, got)
		}
		if due[0] != c.want {
			t.Errorf("%s: SACK due %v, want %v: the slot after the lost block %v", c.name, due[0], c.want, time.Duration(last)*60*ms)
		}
	}
}
