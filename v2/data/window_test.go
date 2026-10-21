package data_test

import (
	"errors"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/USA-RedDragon/dmrgo/v2"

	"github.com/USA-RedDragon/dmrgo/v2/data"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/ip"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/packet"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

type winLink struct {
	t     *testing.T
	tx    *data.Sender
	rx    *data.Receiver
	drop  func(seq, try uint8, i int) bool
	lose  func(i int) bool
	done  []data.Output
	got   []byte
	heads []string
	resp  []string
}

func header(t testing.TB, raw layer2.RawBurst) *pdu.DataHeader {
	t.Helper()
	b, err := layer2.NewBurstFromBytes(raw)
	if err != nil {
		t.Fatal(err)
	}
	dh, ok := b.Data.(*pdu.DataHeader)
	if !ok {
		t.Fatalf("burst carries %T", b.Data)
	}
	return dh
}

func (l *winLink) headerOf(o *data.Output) {
	if o.Burst.SlotType().DataType != enums.DataTypeDataHeader {
		return
	}
	c := header(l.t, o.Burst).ConfirmedDataHeader
	s := strconv.Itoa(int(c.SendSequenceNumber))
	if !c.FullMessageFlag {
		s += "r"
	}
	if c.ResponseRequested {
		s += "A"
	}
	l.heads = append(l.heads, s)
}

func (l *winLink) run(msgs []*data.Message, end time.Duration) {
	l.t.Helper()
	for _, m := range msgs {
		if err := l.tx.Send(0, m); err != nil {
			l.t.Fatal(err)
		}
	}
	type pending struct {
		at  time.Duration
		raw layer2.RawBurst
	}
	var queue []pending
	out := make([]data.Output, 0, data.MaxOutputs)
	ev := make([]data.Event, 0, data.MaxEvents)
	idx := map[[2]uint8]int{}
	respIdx := 0
	handle := func(ev []data.Event) {
		for _, e := range ev {
			switch e.Kind {
			case data.EventDelivered:
				l.got = append(l.got, e.Message.Payload[0])
			case data.EventRespond:
				respIdx++
				if l.lose == nil || !l.lose(respIdx-1) {
					queue = append(queue, pending{e.Due, e.Burst})
				}
				if e.Burst.SlotType().DataType == enums.DataTypeDataHeader {
					st := header(l.t, e.Burst).ResponsePacketHeader.ResponseStatus
					l.resp = append(l.resp, e.Response.String()+"("+strconv.Itoa(int(st))+")")
				}
			case data.EventCallStart, data.EventResponse, data.EventCallEnd, data.EventReassembled:
			}
		}
	}
	for now := time.Duration(0); now <= end; now += 10 * time.Millisecond {
		for len(queue) > 0 && queue[0].at <= now {
			out = l.tx.Burst(queue[0].at, &queue[0].raw, layer2.Hint{}, out[:0])
			if len(out) > data.MaxOutputs {
				l.t.Fatalf("%d outputs", len(out))
			}
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
				l.headerOf(&o)
				k := [2]uint8{o.Seq, o.Try}
				i := idx[k]
				idx[k]++
				if l.drop == nil || !l.drop(o.Seq, o.Try, i) {
					handle(l.rx.Burst(o.Due, &o.Burst, layer2.Hint{}, ev[:0]))
				}
			}
		}
		if l.tx.State() == data.SenderIdle && len(queue) == 0 {
			return
		}
	}
}

func (l *winLink) dones() string {
	var b strings.Builder
	for i, o := range l.done {
		if i > 0 {
			b.WriteByte(' ')
		}
		b.WriteString(strconv.Itoa(int(o.Seq)) + ":" + o.Result.String() + "/" + strconv.Itoa(int(o.Try)))
	}
	return b.String()
}

func windowMessages(n, size int) []*data.Message {
	msgs := make([]*data.Message, n)
	for i := range msgs {
		msgs[i] = message(size)
		msgs[i].Payload[0] = byte('a' + i)
	}
	return msgs
}

func TestSenderWindowLoopback(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name  string
		drop  func(seq, try uint8, i int) bool
		lose  func(i int) bool
		big   bool
		heads string
		resp  string
		dones string
		got   string
	}{
		{name: "clean", heads: "0 1 2A", resp: "ACK(2)", dones: "0:Delivered/1 1:Delivered/1 2:Delivered/1", got: "abc"},
		{
			name:  "lost block of the middle packet",
			drop:  func(seq, try uint8, i int) bool { return seq == 1 && try == 1 && i == 3 },
			heads: "0 1 2A 1r 2rA", resp: "SACK(1)+ACK(2)",
			dones: "0:Delivered/1 1:Delivered/2 2:Delivered/2", got: "abc",
		},
		{
			name:  "lost block of the last packet",
			drop:  func(seq, try uint8, i int) bool { return seq == 2 && try == 1 && i == 5 },
			heads: "0 1 2A 2rA", resp: "SACK(2)+ACK(2)",
			dones: "0:Delivered/1 1:Delivered/1 2:Delivered/2", got: "abc",
		},
		{
			name:  "lost header of the middle packet",
			drop:  func(seq, try uint8, i int) bool { return seq == 1 && try == 1 && i == 0 },
			heads: "0 1 2A 1r 2rA", resp: "SACK(1)+ACK(2)",
			dones: "0:Delivered/1 1:Delivered/2 2:Delivered/2", got: "abc",
		},
		{
			name:  "lost ACK",
			lose:  func(i int) bool { return i == 0 },
			heads: "0 1 2A 0r 1r 2rA", resp: "ACK(2)+ACK(2)",
			dones: "0:Delivered/2 1:Delivered/2 2:Delivered/2", got: "abcabc",
		},
		{
			name:  "all lost",
			drop:  func(_, try uint8, _ int) bool { return try > 0 },
			heads: "0 1 2A 0r 1r 2rA 0r 1r 2rA",
			dones: "0:RetryLimit/3 1:RetryLimit/3 2:RetryLimit/3",
		},
		{
			name: "memory full", big: true,
			heads: "0 1 2A 2rA", resp: "NACK Memory Full(1)+ACK(2)",
			dones: "0:Delivered/1 1:Refused/1 2:Delivered/2", got: "ac",
		},
	} {
		l := &winLink{
			t:    t,
			tx:   newSender(t, &data.SenderConfig{Source: layer2.SyncSourceMS, ColorCode: 1, Retries: 3, Window: 3}),
			rx:   newReceiver(t, &data.ReceiverConfig{Respond: true, Address: dstID, MaxFragment: 70}),
			drop: c.drop,
			lose: c.lose,
		}
		msgs := windowMessages(3, 60)
		if c.big {
			msgs[1] = message(90)
			msgs[1].Payload[0] = 'b'
		}
		l.run(msgs, 20*time.Second)
		if got := strings.Join(l.heads, " "); got != c.heads {
			t.Errorf("%s: headers %q, want %q", c.name, got, c.heads)
		}
		if got := strings.Join(l.resp, "+"); got != c.resp {
			t.Errorf("%s: responses %q, want %q", c.name, got, c.resp)
		}
		if got := l.dones(); got != c.dones {
			t.Errorf("%s: dones %q, want %q", c.name, got, c.dones)
		}
		if string(l.got) != c.got {
			t.Errorf("%s: delivered %q, want %q", c.name, l.got, c.got)
		}
		for _, o := range l.done {
			m, ok := o.ICMP()
			want := o.Result == data.ResultRetryLimit || o.Result == data.ResultRefused
			if ok != want || ok && m != ip.ICMPHostUnreachable {
				t.Errorf("%s: %v gives ICMP %v %v", c.name, o.Result, m, ok)
			}
		}
	}
}

func TestSenderWindowOfEight(t *testing.T) {
	t.Parallel()
	l := &winLink{
		t:  t,
		tx: newSender(t, &data.SenderConfig{Window: data.MaxWindow}),
		rx: newReceiver(t, &data.ReceiverConfig{Respond: true, Address: dstID}),
	}
	l.run(windowMessages(3, 20), 5*time.Second)
	l.rx = newReceiver(t, &data.ReceiverConfig{Respond: true, Address: dstID})
	l.heads, l.got, l.done, l.resp = nil, nil, nil, nil
	msgs := windowMessages(8, 20)
	l.run(msgs, 10*time.Second)
	if got := strings.Join(l.heads, " "); got != "3 4 5 6 7 0 1 2A" {
		t.Errorf("headers %q", got)
	}
	if got := strings.Join(l.resp, "+"); got != "ACK(2)" || string(l.got) != "abcdefgh" || len(l.done) != 8 {
		t.Errorf("responses %q, delivered %q, dones %s", got, l.got, l.dones())
	}
	for _, m := range msgs {
		if err := l.tx.Send(0, m); err != nil {
			t.Fatal(err)
		}
	}
	if err := l.tx.Send(0, message(10)); !errors.Is(err, data.ErrBusy) {
		t.Errorf("ninth Send: %v", err)
	}
	if out := l.tx.Cancel(0, nil); len(out) != 8 || out[7].Seq != 2 || out[7].Result != data.ResultCancelled {
		t.Errorf("Cancel: %+v", out)
	}
}

func TestSenderWindowJoin(t *testing.T) {
	t.Parallel()
	s := newSender(t, &data.SenderConfig{Window: 4, Preambles: 2})
	if err := s.Send(0, message(10)); err != nil {
		t.Fatal(err)
	}
	var heads []string
	out := make([]data.Output, 0, data.MaxOutputs)
	tick := func(n int) {
		for range n {
			at, _ := s.Deadline()
			for _, o := range s.Tick(at, out[:0]) {
				if o.Kind == data.OutputBurst && o.Burst.SlotType().DataType == enums.DataTypeDataHeader {
					c := header(t, o.Burst).ConfirmedDataHeader
					heads = append(heads, strconv.Itoa(int(c.SendSequenceNumber))+strconv.FormatBool(c.ResponseRequested))
				}
			}
		}
	}
	tick(1)
	if err := s.Send(0, message(10)); err != nil {
		t.Fatal(err)
	}
	other := message(10)
	other.Header.ConfirmedDataHeader.LLIDDestination = 9
	short := message(10)
	short.Header.ConfirmedDataHeader.SAP = enums.SAPShortData
	for _, m := range []*data.Message{other, short} {
		if err := s.Send(0, m); !errors.Is(err, data.ErrBusy) {
			t.Errorf("Send to another link or SAP: %v", err)
		}
	}
	tick(6)
	if err := s.Send(0, message(10)); err != nil {
		t.Fatal(err)
	}
	tick(1)
	if s.State() != data.SenderWaitResponse || strings.Join(heads, " ") != "0false 1true" {
		t.Fatalf("state %v, headers %v", s.State(), heads)
	}
	ack := response(t, packet.ResponseACK, nil)
	h := header(t, ack[0])
	h.ResponsePacketHeader.ResponseStatus = 1
	raw, err := layer2.BuildBurst(h, 0, layer2.SyncSourceBS)
	if err != nil {
		t.Fatal(err)
	}
	wait, _ := s.Deadline()
	done := s.Burst(wait-time.Millisecond, &raw, layer2.Hint{}, out[:0])
	if len(done) != 2 || done[0].Seq != 0 || done[1].Seq != 1 || done[1].Result != data.ResultDelivered {
		t.Fatalf("dones %+v", done)
	}
	tick(5)
	if strings.Join(heads, " ") != "0false 1true 2true" {
		t.Errorf("headers %v", heads)
	}
	short2 := newSender(t, &data.SenderConfig{Window: 4})
	if err := short2.Send(0, short); err != nil {
		t.Fatal(err)
	}
	if err := short2.Send(0, short); !errors.Is(err, data.ErrBusy) {
		t.Errorf("second short data C_HEAD: %v", err)
	}
	for _, w := range []int{-1, data.MaxWindow + 1} {
		var ce *dmrgo.ConfigError
		if _, err := data.NewSender(&data.SenderConfig{Window: w}); !errors.As(err, &ce) || ce.Field != "Window" {
			t.Errorf("Window %d: %v", w, err)
		}
	}
}

func windowResponse(t *testing.T, r packet.Response, status uint8, rd *packet.ResponseData) []layer2.RawBurst {
	t.Helper()
	h := confirmedHeader(status).ConfirmedDataHeader
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

func TestSenderWindowResponses(t *testing.T) {
	t.Parallel()
	var rd packet.ResponseData
	rd.Retry[2], rd.Retry[5] = true, true
	for _, c := range []struct {
		name   string
		resp   []layer2.RawBurst
		dones  string
		resend string
	}{
		{"ACK of all", windowResponse(t, packet.ResponseACK, 2, nil), "0:Delivered 1:Delivered 2:Delivered", ""},
		{"ACK of the first", windowResponse(t, packet.ResponseACK, 0, nil), "0:Delivered", "1r/7 2r/7A"},
		{"SACK", windowResponse(t, packet.ResponseSACK, 1, &rd), "0:Delivered", "1r/2 2r/7A"},
		{"NACK CRC", windowResponse(t, packet.ResponseNACKPacketCRC, 0, nil), "", "0r/7S 1r/7 2r/7A"},
		{"NACK undeliverable", windowResponse(t, packet.ResponseNACKUndeliverable, 1, nil), "0:Delivered 1:Refused", "2r/7A"},
		{"NACK out of sequence", windowResponse(t, packet.ResponseNACKOutOfSequence, 6, nil), "", "0/7S 1/7S 2/7SA"},
		{"other status", windowResponse(t, packet.ResponseACK, 5, nil), "", ""},
	} {
		s := newSender(t, &data.SenderConfig{Window: 3, Resync: true})
		for _, m := range windowMessages(3, 60) {
			if err := s.Send(0, m); err != nil {
				t.Fatal(err)
			}
		}
		out := drain(t, s, 24*60*time.Millisecond)
		if s.State() != data.SenderWaitResponse || len(out) != 24 {
			t.Fatalf("%s: state %v after %d outputs", c.name, s.State(), len(out))
		}
		now := out[len(out)-1].Due + 120*time.Millisecond
		var got []data.Output
		for i := range c.resp {
			got = append(got, s.Burst(now, &c.resp[i], layer2.Hint{}, nil)...)
		}
		var dones []string
		for _, o := range got {
			dones = append(dones, strconv.Itoa(int(o.Seq))+":"+o.Result.String())
		}
		if strings.Join(dones, " ") != c.dones {
			t.Errorf("%s: dones %v, want %q", c.name, dones, c.dones)
		}
		if c.name == "other status" {
			if s.State() != data.SenderWaitResponse {
				t.Errorf("%s: state %v", c.name, s.State())
			}
			continue
		}
		var heads []string
		blocks := 0
		for _, o := range drain(t, s, now+2*time.Second) {
			if o.Kind != data.OutputBurst || o.Try != 2 {
				continue
			}
			if o.Burst.SlotType().DataType != enums.DataTypeDataHeader {
				blocks++
				continue
			}
			ch := header(t, o.Burst).ConfirmedDataHeader
			h := strconv.Itoa(int(ch.SendSequenceNumber))
			if !ch.FullMessageFlag {
				h += "r"
			}
			h += "/" + strconv.Itoa(int(ch.BlocksToFollow))
			if ch.ReSynchronizeFlag {
				h += "S"
			}
			if ch.ResponseRequested {
				h += "A"
			}
			heads = append(heads, h)
		}
		if strings.Join(heads, " ") != c.resend {
			t.Errorf("%s: resend %v, want %q", c.name, heads, c.resend)
		}
	}
}

func TestSenderWindowTimeLimit(t *testing.T) {
	t.Parallel()
	s := newSender(t, &data.SenderConfig{Window: 3, TxLimit: 300 * time.Millisecond})
	s.Channel(0, true)
	if err := s.Send(0, message(10)); err != nil {
		t.Fatal(err)
	}
	if err := s.Send(100*time.Millisecond, message(60)); err != nil {
		t.Fatal(err)
	}
	out := s.Tick(300*time.Millisecond, nil)
	if len(out) != 1 || out[0].Result != data.ResultTimeLimit || out[0].Seq != 0 || s.State() != data.SenderWaiting {
		t.Fatalf("outputs %+v, state %v", out, s.State())
	}
	s.Channel(350*time.Millisecond, false)
	bursts := 0
	for _, o := range drain(t, s, 900*time.Millisecond) {
		if o.Kind != data.OutputBurst || o.Seq != 1 {
			t.Fatalf("output %+v", o)
		}
		bursts++
	}
	if bursts != 8 || s.State() != data.SenderWaitResponse {
		t.Errorf("%d bursts, state %v", bursts, s.State())
	}
}

func TestReceiverWindow(t *testing.T) {
	t.Parallel()
	packetBursts := func(ns uint8, last, partial bool) []layer2.RawBurst {
		h := confirmedHeader(ns)
		h.ConfirmedDataHeader.ResponseRequested = last
		h.ConfirmedDataHeader.FullMessageFlag = !partial
		return build(t, h, payload(60), enums.DataTypeRate12)
	}
	for _, c := range []struct {
		name    string
		ns      []uint8
		drop    [2]int
		partial int
		resp    string
		status  uint8
		retry   int
		rdata   int
		deliver int
	}{
		{"all good", []uint8{5, 6, 7}, [2]int{-1, 0}, -1, "ACK", 7, 0, 0, 3},
		{"bad block of the first", []uint8{5, 6, 7}, [2]int{0, 2}, -1, "SACK", 5, 7, 1, 0},
		{"bad block of the last", []uint8{5, 6, 7}, [2]int{2, 4}, -1, "SACK", 7, 1, 1, 2},
		{"lost packet", []uint8{5, 7}, [2]int{-1, 0}, -1, "SACK", 6, 127, 2, 1},
		{"bad block of a later try", []uint8{5, 6, 7}, [2]int{1, 2}, 1, "SACK", 6, 127, 2, 1},
		{"new window at a repeat", []uint8{5, 6, 5, 6}, [2]int{0, 2}, -1, "ACK", 6, 0, 0, 2},
		{"stop and wait", []uint8{1}, [2]int{0, 2}, -1, "SACK", 1, 1, 1, 0},
	} {
		var bursts []layer2.RawBurst
		drop := -1
		for i, ns := range c.ns {
			if c.drop[0] == i {
				drop = len(bursts) + c.drop[1]
			}
			bursts = append(bursts, packetBursts(ns, i == len(c.ns)-1, i == c.partial)...)
		}
		ev := replay(t, newReceiver(t, &data.ReceiverConfig{Respond: true, Address: dstID}), timed(0, bursts, drop))
		var resp []data.Event
		delivered := 0
		for _, e := range ev {
			switch e.Kind {
			case data.EventRespond:
				resp = append(resp, e)
			case data.EventDelivered:
				delivered++
			case data.EventCallStart, data.EventResponse, data.EventCallEnd, data.EventReassembled:
			}
		}
		if len(resp) != 1+c.rdata || resp[0].Response.String() != c.resp || delivered != c.deliver {
			t.Errorf("%s: %s", c.name, kinds(ev))
			continue
		}
		if st := header(t, resp[0].Burst).ResponsePacketHeader.ResponseStatus; st != c.status {
			t.Errorf("%s: status %d, want %d", c.name, st, c.status)
		}
		if c.rdata == 0 {
			continue
		}
		var blocks []*pdu.Rate12Data
		for _, e := range resp[1:] {
			b, err := layer2.NewBurstFromBytes(e.Burst)
			if err != nil {
				t.Fatal(err)
			}
			blocks = append(blocks, b.Data.(*pdu.Rate12Data))
		}
		rd, err := packet.DecodeResponseData(blocks...)
		if err != nil {
			t.Fatal(err)
		}
		if n := len(rd.RetryBlocks()); n != c.retry {
			t.Errorf("%s: %d blocks to retry, want %d", c.name, n, c.retry)
		}
	}
}
