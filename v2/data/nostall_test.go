package data_test

import (
	"testing"
	"time"

	"github.com/USA-RedDragon/dmrgo/v2/data"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/packet"
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

func TestSenderResendAfterResponseSendsOnePreamble(t *testing.T) {
	t.Parallel()
	ms := time.Millisecond
	var rd packet.ResponseData
	rd.Retry[1] = true
	for _, c := range []struct {
		name      string
		resp      []layer2.RawBurst
		busy      time.Duration
		preambles int
	}{
		{"SACK", response(t, packet.ResponseSACK, &rd), 0, 1},
		{"NACK CRC", response(t, packet.ResponseNACKPacketCRC, nil), 0, 1},
		{"SACK, busy within the hang time", response(t, packet.ResponseSACK, &rd), data.DefaultHang, 1},
		{"SACK, busy after the hang time", response(t, packet.ResponseSACK, &rd), time.Second, data.DefaultPreambles},
		{"no response", nil, 0, data.DefaultPreambles},
	} {
		s := newSender(t, &data.SenderConfig{Slot: enums.Slot1, Retries: 2, Holdoff: -1})
		if err := s.Send(0, message(t, 30)); err != nil {
			t.Fatal(err)
		}
		var last time.Duration
		for s.State() == data.SenderSending {
			at, _ := s.Deadline()
			for _, o := range s.Tick(at, nil) {
				last = o.Due
			}
		}
		if s.State() != data.SenderWaitResponse {
			t.Fatalf("%s: state %v", c.name, s.State())
		}
		now := last + 120*ms
		if c.busy > 0 {
			s.Channel(now, true)
		}
		for i := range c.resp {
			s.Burst(now, &c.resp[i], layer2.Hint{}, nil)
		}
		if c.resp == nil {
			now, _ = s.Deadline()
		}
		var all []data.Output
		if c.busy > 0 {
			all = drain(t, s, now+c.busy)
			now += c.busy
			s.Channel(now, false)
		}
		csbks, header := 0, time.Duration(-1)
		for _, o := range append(all, drain(t, s, now+time.Second)...) {
			if o.Try != 2 || header >= 0 {
				continue
			}
			switch o.Burst.SlotType().DataType {
			case enums.DataTypeCSBK:
				csbks++
			case enums.DataTypeDataHeader:
				header = o.Due
			default:
			}
		}
		want := time.Duration(c.preambles) * 60 * ms
		if csbks != c.preambles || header != now+want {
			t.Errorf("%s: try 2 sends %d preambles and its header %v after the response, want %d and %v", c.name, csbks, header-now, c.preambles, want)
		}
	}
}
