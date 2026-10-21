package data_test

import (
	"bytes"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/USA-RedDragon/dmrgo/v2"

	"github.com/USA-RedDragon/dmrgo/v2/data"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/packet"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

func newSender(t testing.TB, cfg *data.SenderConfig) *data.Sender {
	t.Helper()
	s, err := data.NewSender(cfg)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func drain(t testing.TB, s *data.Sender, stop time.Duration) []data.Output {
	t.Helper()
	var all []data.Output
	out := make([]data.Output, 0, data.MaxOutputs)
	for {
		at, ok := s.Deadline()
		if !ok || at > stop {
			return all
		}
		out = s.Tick(at, out[:0])
		if len(out) > data.MaxOutputs {
			t.Fatalf("%d outputs", len(out))
		}
		all = append(all, out...)
	}
}

func TestSenderMatchesAnyTone(t *testing.T) {
	t.Parallel()
	bursts := readTimed(t, "testdata/anytone_sms_xpr.txt")
	ev := replay(t, newReceiver(t, nil), bursts[:14])
	i := slices.IndexFunc(ev, func(e data.Event) bool { return e.Kind == data.EventDelivered })
	if i < 0 {
		t.Fatal("no message")
	}
	m := ev[i].Message
	s := newSender(t, &data.SenderConfig{ColorCode: 7, Preambles: 5})
	if err := s.Send(0, &m); err != nil {
		t.Fatal(err)
	}
	out := drain(t, s, 0)
	out = append(out, drain(t, s, time.Second)...)
	n := 0
	for _, o := range out {
		if o.Kind != data.OutputBurst {
			continue
		}
		if o.Burst != bursts[n].raw {
			t.Errorf("burst %d: %x, radio sent %x", n, o.Burst, bursts[n].raw)
		}
		if o.Due != time.Duration(n)*60*time.Millisecond || o.Try != 1 {
			t.Errorf("burst %d: due %v, try %d", n, o.Due, o.Try)
		}
		if st := o.Burst.SlotType(); o.Hint.Kind != layer2.KindData || !o.Hint.HasDataType || o.Hint.DataType != st.DataType {
			t.Errorf("burst %d: hint %+v for %v", n, o.Hint, st.DataType)
		}
		n++
	}
	if n != 14 {
		t.Errorf("%d bursts, want 14", n)
	}
	if s.State() != data.SenderWaitResponse {
		t.Errorf("state %v", s.State())
	}
}

type link struct {
	t    *testing.T
	tx   *data.Sender
	rx   *data.Receiver
	drop func(try uint8, i int) bool
	done []data.Output
	got  [][]byte
	resp []string
}

func (l *link) run(m *data.Message, end time.Duration) {
	l.t.Helper()
	if err := l.tx.Send(0, m); err != nil {
		l.t.Fatal(err)
	}
	type pending struct {
		at  time.Duration
		raw layer2.RawBurst
	}
	var queue []pending
	out := make([]data.Output, 0, data.MaxOutputs)
	ev := make([]data.Event, 0, data.MaxEvents)
	idx := map[uint8]int{}
	respIdx := 0
	handle := func(ev []data.Event) {
		for _, e := range ev {
			switch e.Kind {
			case data.EventDelivered:
				l.got = append(l.got, append([]byte(nil), e.Message.Payload...))
			case data.EventRespond:
				respIdx++
				if l.drop == nil || !l.drop(0, respIdx-1) {
					queue = append(queue, pending{e.Due, e.Burst})
				}
				if e.Burst.SlotType().DataType == enums.DataTypeDataHeader {
					l.resp = append(l.resp, e.Response.String())
				}
			case data.EventCallStart, data.EventResponse, data.EventCallEnd, data.EventReassembled:
			}
		}
	}
	for now := time.Duration(0); now <= end; now += 10 * time.Millisecond {
		for len(queue) > 0 && queue[0].at <= now {
			out = l.tx.Burst(queue[0].at, &queue[0].raw, layer2.Hint{}, out[:0])
			l.done = append(l.done, out...)
			queue = queue[1:]
		}
		if at, ok := l.rx.Deadline(); ok && at <= now {
			handle(l.rx.Tick(now, ev[:0]))
		}
		if at, ok := l.tx.Deadline(); ok && at <= now {
			for _, o := range l.tx.Tick(now, out[:0]) {
				if o.Kind == data.OutputDone {
					l.done = append(l.done, o)
					continue
				}
				i := idx[o.Try]
				idx[o.Try]++
				if l.drop == nil || !l.drop(o.Try, i) {
					handle(l.rx.Burst(o.Due, &o.Burst, layer2.Hint{}, ev[:0]))
				}
			}
		}
		if len(l.done) > 0 && l.tx.State() == data.SenderIdle {
			return
		}
	}
}

func message(n int) *data.Message {
	return &data.Message{Header: confirmedHeader(0), Rate: enums.DataTypeRate12, Payload: payload(n)}
}

func TestSenderLoopback(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name   string
		drop   func(try uint8, i int) bool
		result data.Result
		tries  uint8
		resp   string
		ipDups int
	}{
		{"clean", nil, data.ResultDelivered, 1, "ACK", 0},
		{"lost block", func(try uint8, i int) bool { return try == 1 && i == 3 }, data.ResultDelivered, 2, "SACK+ACK", 0},
		{"lost last block", func(try uint8, i int) bool { return try == 1 && i == 6 }, data.ResultDelivered, 2, "SACK+ACK", 0},
		{"lost header", func(try uint8, i int) bool { return try == 1 && i == 0 }, data.ResultDelivered, 2, "ACK", 0},
		{"lost ACK", func(try uint8, i int) bool { return try == 0 && i == 0 }, data.ResultDelivered, 2, "ACK+ACK", 1},
		{"all lost", func(try uint8, _ int) bool { return try > 0 }, data.ResultRetryLimit, 3, "", 0},
	} {
		for _, sap := range []enums.ServiceAccessPointID{enums.SAPIPBasedPacketData, enums.SAPShortData} {
			l := &link{
				t:    t,
				tx:   newSender(t, &data.SenderConfig{Source: layer2.SyncSourceMS, ColorCode: 1, Retries: 3}),
				rx:   newReceiver(t, &data.ReceiverConfig{Respond: true, Address: dstID}),
				drop: c.drop,
			}
			m := message(60)
			m.Header = sapHeader(sap, 0, false)
			l.run(m, 10*time.Second)
			if len(l.done) != 1 || l.done[0].Result != c.result || l.done[0].Try != c.tries {
				t.Errorf("%s, %v: done %+v", c.name, sap, l.done)
				continue
			}
			if r := strings.Join(l.resp, "+"); r != c.resp {
				t.Errorf("%s, %v: responses %q, want %q", c.name, sap, r, c.resp)
			}
			want := 1
			if sap == enums.SAPIPBasedPacketData {
				want += c.ipDups
			}
			if c.result != data.ResultDelivered {
				want = 0
			}
			if len(l.got) != want || want > 0 && !bytes.HasPrefix(l.got[0], payload(60)) {
				t.Errorf("%s, %v: %d deliveries, want %d", c.name, sap, len(l.got), want)
			}
		}
	}
}

func TestSenderSequence(t *testing.T) {
	t.Parallel()
	for _, resync := range []bool{false, true} {
		s := newSender(t, &data.SenderConfig{Resync: resync})
		ns, sflag := make([]uint8, 0, 3), make([]bool, 0, 3)
		for range 3 {
			if err := s.Send(0, message(10)); err != nil {
				t.Fatal(err)
			}
			out := drain(t, s, 0)
			b, err := layer2.NewBurstFromBytes(out[0].Burst)
			if err != nil {
				t.Fatal(err)
			}
			dh, ok := b.Data.(*pdu.DataHeader)
			if !ok {
				t.Fatalf("burst carries %T", b.Data)
			}
			c := dh.ConfirmedDataHeader
			ns, sflag = append(ns, c.SendSequenceNumber), append(sflag, c.ReSynchronizeFlag)
			if c.FragmentSequenceNumber != 0b1000 || !c.FullMessageFlag || !c.ResponseRequested {
				t.Errorf("header %+v", c)
			}
			s.Cancel(0, nil)
		}
		if !slices.Equal(ns, []uint8{0, 1, 2}) || !slices.Equal(sflag, []bool{resync, false, false}) {
			t.Errorf("resync %v: N(S) %v, S %v", resync, ns, sflag)
		}
	}
}

func response(t *testing.T, r packet.Response, rd *packet.ResponseData) []layer2.RawBurst {
	t.Helper()
	h := confirmedHeader(0).ConfirmedDataHeader
	h.BlocksToFollow = 7
	p, err := packet.NewResponse(h, r, rd)
	if err != nil {
		t.Fatal(err)
	}
	b, err := packet.Bursts(&p, 0, layer2.SyncSourceBS)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestSenderResponses(t *testing.T) {
	t.Parallel()
	var rd packet.ResponseData
	rd.Retry[2], rd.Retry[5] = true, true
	for _, c := range []struct {
		name   string
		resp   []layer2.RawBurst
		result data.Result
		resend int
		full   bool
	}{
		{"ACK", response(t, packet.ResponseACK, nil), data.ResultDelivered, 0, false},
		{"SACK", response(t, packet.ResponseSACK, &rd), 0, 2, false},
		{"NACK CRC", response(t, packet.ResponseNACKPacketCRC, nil), 0, 7, false},
		{"NACK out of sequence", response(t, packet.ResponseNACKOutOfSequence, nil), 0, 7, true},
		{"NACK memory full", response(t, packet.ResponseNACKMemoryFull, nil), data.ResultRefused, 0, false},
		{"NACK undeliverable", response(t, packet.ResponseNACKUndeliverable, nil), data.ResultRefused, 0, false},
	} {
		s := newSender(t, nil)
		if err := s.Send(0, message(60)); err != nil {
			t.Fatal(err)
		}
		out := drain(t, s, 7*60*time.Millisecond)
		if s.State() != data.SenderWaitResponse || len(out) != 8 {
			t.Fatalf("%s: state %v after %d outputs", c.name, s.State(), len(out))
		}
		now := out[len(out)-1].Due + 120*time.Millisecond
		got := make([]data.Output, 0, data.MaxOutputs)
		for i := range c.resp {
			got = append(got, s.Burst(now, &c.resp[i], layer2.Hint{}, nil)...)
		}
		if c.result != 0 {
			if len(got) != 1 || got[0].Result != c.result || !got[0].HasResponse {
				t.Errorf("%s: %+v", c.name, got)
			}
			continue
		}
		var next []data.Output
		for _, o := range drain(t, s, now+time.Second) {
			if o.Try == 2 {
				next = append(next, o)
			}
		}
		if len(next) != c.resend+1 {
			t.Errorf("%s: %d bursts in try 2, want %d", c.name, len(next)-1, c.resend)
			continue
		}
		b, err := layer2.NewBurstFromBytes(next[0].Burst)
		if err != nil {
			t.Fatal(err)
		}
		dh, ok := b.Data.(*pdu.DataHeader)
		if !ok {
			t.Fatalf("%s: burst carries %T", c.name, b.Data)
		}
		h := dh.ConfirmedDataHeader
		if h.FullMessageFlag != c.full || int(h.BlocksToFollow) != c.resend || h.SendSequenceNumber != 0 {
			t.Errorf("%s: resend header %+v", c.name, h)
		}
	}
}

func TestSenderOtherResponse(t *testing.T) {
	t.Parallel()
	s := newSender(t, nil)
	if err := s.Send(0, message(10)); err != nil {
		t.Fatal(err)
	}
	drain(t, s, 60*time.Millisecond)
	h := pdu.ConfirmedDataHeader{LLIDDestination: 5, LLIDSource: srcID}
	p, err := packet.NewResponse(&h, packet.ResponseACK, nil)
	if err != nil {
		t.Fatal(err)
	}
	b, err := packet.Bursts(&p, 0, layer2.SyncSourceBS)
	if err != nil {
		t.Fatal(err)
	}
	if out := s.Burst(70*time.Millisecond, &b[0], layer2.Hint{}, nil); len(out) != 0 {
		t.Errorf("outputs %+v", out)
	}
}

func TestSenderUnconfirmed(t *testing.T) {
	t.Parallel()
	s := newSender(t, &data.SenderConfig{Preambles: 3, Source: layer2.SyncSourceMS, ColorCode: 1})
	m := &data.Message{Header: pdu.DataHeader{Format: pdu.FormatUnconfirmed, UnconfirmedDataHeader: &pdu.UnconfirmedDataHeader{
		Group: true, SAP: enums.SAPIPBasedPacketData, LLIDDestination: 9, LLIDSource: srcID}}, Rate: enums.DataTypeRate34, Payload: payload(40)}
	if err := s.Send(time.Second, m); err != nil {
		t.Fatal(err)
	}
	out := drain(t, s, 2*time.Second)
	if len(out) != 3+1+3+1 || out[len(out)-1].Result != data.ResultSent {
		t.Fatalf("%d outputs, last %+v", len(out), out[len(out)-1])
	}
	ev := replay(t, newReceiver(t, nil), func() []timedBurst {
		b := make([]timedBurst, 0, len(out))
		for _, o := range out[:len(out)-1] {
			b = append(b, timedBurst{o.Due, o.Burst})
		}
		return b
	}())
	if got := kinds(ev); got != "CallStart Delivered CallEnd(Quiet)" || !ev[0].Call.Group || string(ev[1].Message.Payload) != string(payload(40)) {
		t.Errorf("receiver: %s %+v", got, ev[0].Call)
	}
	if ev[0].Call.Target != 9 || ev[2].Stats.Duration != 6*60*time.Millisecond+data.DefaultHang {
		t.Errorf("call %+v, stats %+v", ev[0].Call, ev[2].Stats)
	}
}

func TestSenderAccess(t *testing.T) {
	t.Parallel()
	first := func(cfg *data.SenderConfig, idleAt time.Duration) (time.Duration, data.Result) {
		s := newSender(t, cfg)
		s.Channel(0, true)
		if err := s.Send(0, message(10)); err != nil {
			t.Fatal(err)
		}
		out := make([]data.Output, 0, data.MaxOutputs)
		for now := time.Duration(0); now < 10*time.Second; now += 10 * time.Millisecond {
			if now == idleAt {
				s.Channel(now, false)
			}
			out = s.Tick(now, out[:0])
			if len(out) > 0 {
				return out[0].Due, out[0].Result
			}
		}
		return -1, 0
	}
	if at, _ := first(&data.SenderConfig{}, 300*time.Millisecond); at != 300*time.Millisecond {
		t.Errorf("idle during the search: first burst at %v", at)
	}
	if at, _ := first(&data.SenderConfig{Access: enums.AccessImpolite}, -1); at != 0 {
		t.Errorf("impolite: first burst at %v", at)
	}
	a, _ := first(&data.SenderConfig{Seed: 1, Holdoff: time.Second}, 2*time.Second)
	b, _ := first(&data.SenderConfig{Seed: 1, Holdoff: time.Second}, 2*time.Second)
	if a != b || a < 2*time.Second {
		t.Errorf("holdoff: first bursts at %v and %v", a, b)
	}
	if _, r := first(&data.SenderConfig{TxLimit: 3 * time.Second}, -1); r != data.ResultTimeLimit {
		t.Errorf("busy channel: result %v", r)
	}
}

func TestSenderErrors(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		cfg   data.SenderConfig
		field string
	}{
		{data.SenderConfig{ColorCode: 16}, "ColorCode"},
		{data.SenderConfig{Source: 7}, "Source"},
		{data.SenderConfig{Access: 3}, "Access"},
		{data.SenderConfig{Preambles: data.MaxPreambles + 1}, "Preambles"},
		{data.SenderConfig{Retries: -1}, "Retries"},
		{data.SenderConfig{MaxBlocks: 128}, "MaxBlocks"},
		{data.SenderConfig{Peers: -2}, "Peers"},
	} {
		_, err := data.NewSender(&c.cfg)
		var ce *dmrgo.ConfigError
		if !errors.As(err, &ce) || ce.Field != c.field {
			t.Errorf("%+v: %v, want a ConfigError for %s", c.cfg, err, c.field)
		}
	}
	s := newSender(t, &data.SenderConfig{MaxBlocks: 2})
	group := message(10)
	group.Header.ConfirmedDataHeader.Group = true
	udt := &data.Message{Header: pdu.DataHeader{Format: pdu.FormatUnifiedDataTransport, UDTHeader: &pdu.UDTHeader{}}}
	prop := message(10)
	prop.Proprietary = &pdu.ProprietaryHeader{}
	for _, c := range []struct {
		name string
		m    *data.Message
		want error
	}{
		{"group", group, &dmrgo.ConfigError{Type: "data", Field: "Header.Group"}},
		{"UDT", udt, &dmrgo.ConfigError{Type: "data", Field: "Header"}},
		{"P_HEAD", prop, &dmrgo.ConfigError{Type: "data", Field: "Header"}},
		{"too long", message(200), packet.ErrTooLong},
		{"rate", &data.Message{Header: confirmedHeader(0), Rate: enums.DataTypeCSBK}, packet.ErrRate},
	} {
		err := s.Send(0, c.m)
		var ce, we *dmrgo.ConfigError
		switch {
		case errors.As(c.want, &we):
			if !errors.As(err, &ce) || ce.Field != we.Field {
				t.Errorf("%s: %v", c.name, err)
			}
		case !errors.Is(err, c.want):
			t.Errorf("%s: %v, want %v", c.name, err, c.want)
		}
	}
	if err := s.Send(0, message(10)); err != nil {
		t.Fatal(err)
	}
	if err := s.Send(0, message(10)); !errors.Is(err, data.ErrBusy) {
		t.Errorf("second Send: %v", err)
	}
	out := s.Cancel(0, nil)
	if len(out) != 1 || out[0].Result != data.ResultCancelled || s.State() != data.SenderIdle {
		t.Errorf("Cancel: %+v", out)
	}
	if out := s.Cancel(0, nil); len(out) != 0 {
		t.Errorf("Cancel when idle: %+v", out)
	}
}

func TestSenderStrings(t *testing.T) {
	t.Parallel()
	for _, c := range []struct{ got, want string }{
		{data.OutputBurst.String(), "Burst"},
		{data.OutputDone.String(), "Done"},
		{data.OutputKind(7).String(), "OutputKind(7)"},
		{data.ResultDelivered.String(), "Delivered"},
		{data.ResultCancelled.String(), "Cancelled"},
		{data.Result(0).String(), "Result(0)"},
		{data.SenderWaitResponse.String(), "WaitResponse"},
		{data.SenderState(9).String(), "SenderState(9)"},
	} {
		if c.got != c.want {
			t.Errorf("%q, want %q", c.got, c.want)
		}
	}
}
