package data_test

import (
	"testing"
	"time"

	"github.com/USA-RedDragon/dmrgo/v2/data"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/ip"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/packet"
)

func TestSenderSourceSDL(t *testing.T) {
	t.Parallel()
	ack := response(t, packet.ResponseACK, nil)
	var rd packet.ResponseData
	rd.Retry[1] = true
	sack := response(t, packet.ResponseSACK, &rd)
	type step struct {
		at    time.Duration
		do    string
		state data.SenderState
	}
	ms := time.Millisecond
	for _, c := range []struct {
		name   string
		cfg    data.SenderConfig
		steps  []step
		result data.Result
		icmp   bool
	}{
		{
			name: "idle channel, ACK", cfg: data.SenderConfig{Slot: enums.Slot1, Preambles: -1},
			steps: []step{{0, "send", data.SenderSending}, {150 * ms, "tick", data.SenderWaitResponse},
				{200 * ms, "ack", data.SenderIdle}},
			result: data.ResultDelivered,
		},
		{
			name: "busy, idle search, holdoff, idle", cfg: data.SenderConfig{Slot: enums.Slot1, Preambles: -1, Holdoff: 100 * ms},
			steps: []step{
				{0, "busy", data.SenderIdle}, {0, "send", data.SenderWaiting},
				{540 * ms, "tick", data.SenderHoldoff}, {560 * ms, "idle", data.SenderHoldoff},
				{640 * ms, "tick", data.SenderSending}, {800 * ms, "tick", data.SenderWaitResponse},
				{850 * ms, "ack", data.SenderIdle},
			},
			result: data.ResultDelivered,
		},
		{
			name: "holdoff back to the idle search", cfg: data.SenderConfig{Slot: enums.Slot1, Preambles: -1, Holdoff: 100 * ms},
			steps: []step{
				{0, "busy", data.SenderIdle}, {0, "send", data.SenderWaiting},
				{540 * ms, "tick", data.SenderHoldoff}, {640 * ms, "tick", data.SenderWaiting},
				{640 * ms, "cancel", data.SenderIdle},
			},
			result: data.ResultCancelled,
		},
		{
			name: "T_DataTxLmt in Qualify_Idle", cfg: data.SenderConfig{Slot: enums.Slot1, Preambles: -1, TxLimit: 300 * ms},
			steps: []step{{0, "busy", data.SenderIdle}, {0, "send", data.SenderWaiting},
				{300 * ms, "tick", data.SenderIdle}},
			result: data.ResultTimeLimit, icmp: true,
		},
		{
			name: "T_DataTxLmt in Holdoff", cfg: data.SenderConfig{Slot: enums.Slot1, Preambles: -1, TxLimit: 600 * ms, Holdoff: time.Second, Seed: 1},
			steps: []step{
				{0, "busy", data.SenderIdle}, {0, "send", data.SenderWaiting},
				{540 * ms, "tick", data.SenderHoldoff}, {590 * ms, "tick", data.SenderHoldoff},
				{600 * ms, "tick", data.SenderIdle},
			},
			result: data.ResultTimeLimit, icmp: true,
		},
		{
			name: "SACK with tries left", cfg: data.SenderConfig{Slot: enums.Slot1, Preambles: -1, Retries: 2},
			steps: []step{
				{0, "send", data.SenderSending}, {150 * ms, "tick", data.SenderWaitResponse},
				{160 * ms, "busy", data.SenderWaitResponse}, {200 * ms, "sack", data.SenderWaiting},
				{210 * ms, "idle", data.SenderSending}, {400 * ms, "tick", data.SenderWaitResponse},
				{450 * ms, "ack", data.SenderIdle},
			},
			result: data.ResultDelivered,
		},
		{
			name: "SACK at N_RtryLmt", cfg: data.SenderConfig{Slot: enums.Slot1, Preambles: -1, Retries: 1},
			steps: []step{{0, "send", data.SenderSending}, {150 * ms, "tick", data.SenderWaitResponse},
				{200 * ms, "sack", data.SenderIdle}},
			result: data.ResultRetryLimit, icmp: true,
		},
		{
			name: "T_RspnsWait with tries left", cfg: data.SenderConfig{Slot: enums.Slot1, Preambles: -1, Retries: 2},
			steps: []step{
				{0, "send", data.SenderSending}, {150 * ms, "tick", data.SenderWaitResponse},
				{150 * ms, "busy", data.SenderWaitResponse}, {360 * ms, "tick", data.SenderWaiting},
				{360 * ms, "cancel", data.SenderIdle},
			},
			result: data.ResultCancelled,
		},
		{
			name: "T_RspnsWait at N_RtryLmt", cfg: data.SenderConfig{Slot: enums.Slot1, Preambles: -1, Retries: 1},
			steps: []step{{0, "send", data.SenderSending}, {150 * ms, "tick", data.SenderWaitResponse},
				{360 * ms, "tick", data.SenderIdle}},
			result: data.ResultRetryLimit, icmp: true,
		},
	} {
		s := newSender(t, &c.cfg)
		var done []data.Output
		for i, st := range c.steps {
			var out []data.Output
			switch st.do {
			case "send":
				if err := s.Send(st.at, message(t, 10)); err != nil {
					t.Fatal(err)
				}
			case "busy", "idle":
				s.Channel(st.at, st.do == "busy")
			case "tick":
				for {
					at, ok := s.Deadline()
					if !ok || at > st.at {
						break
					}
					out = append(out, s.Tick(at, nil)...)
				}
			case "ack":
				out = s.Burst(st.at, &ack[0], layer2.Hint{}, nil)
			case "sack":
				for j := range sack {
					out = append(out, s.Burst(st.at, &sack[j], layer2.Hint{}, nil)...)
				}
			case "cancel":
				out = s.Cancel(st.at, nil)
			}
			for _, o := range out {
				if o.Kind == data.OutputDone {
					done = append(done, o)
				}
			}
			if s.State() != st.state {
				t.Errorf("%s: step %d (%s at %v): state %v, want %v", c.name, i, st.do, st.at, s.State(), st.state)
			}
		}
		if len(done) != 1 || done[0].Result != c.result {
			t.Errorf("%s: dones %+v", c.name, done)
			continue
		}
		if m, ok := done[0].ICMP(); ok != c.icmp || ok && m != ip.ICMPHostUnreachable {
			t.Errorf("%s: ICMP %v %v", c.name, m, ok)
		}
	}
	if m, ok := (&data.Output{Kind: data.OutputBurst}).ICMP(); ok {
		t.Errorf("burst gives ICMP %v", m)
	}
	var none *data.Output
	if _, ok := none.ICMP(); ok {
		t.Error("nil Output gives an ICMP message")
	}
}
