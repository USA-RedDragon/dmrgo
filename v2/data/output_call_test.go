package data_test

import (
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/data"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
)

func TestOutputCall(t *testing.T) {
	t.Parallel()

	for _, group := range []bool{false, true} {
		s := newSender(t, &data.SenderConfig{Slot: enums.Slot1, Preambles: 2})
		m := &data.Message{Header: ipHeader(t, group, enums.SAPIPBasedPacketData), Rate: enums.DataTypeRate12, Payload: payload(40)}
		if err := s.Send(0, m); err != nil {
			t.Fatal(err)
		}
		want := layer2.Call{Group: group, Target: 9, Source: 3120001}
		bursts, done := 0, 0
		var out []data.Output
		for range 100 {
			at, ok := s.Deadline()
			if !ok {
				break
			}
			out = s.Tick(at, out[:0])
			for _, o := range out {
				if o.Call != want {
					t.Errorf("group %v: %v output with call %+v", group, o.Kind, o.Call)
				}
				switch o.Kind {
				case data.OutputBurst:
					bursts++
				case data.OutputDone:
					done++
				}
			}
		}
		if bursts < 4 || done != 1 {
			t.Errorf("group %v: %d bursts, %d done", group, bursts, done)
		}
	}
}
