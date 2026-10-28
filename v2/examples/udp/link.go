package main

import (
	"time"

	"github.com/USA-RedDragon/dmrgo/v2/data"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/ip"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

const appPort ip.Port = 3270

type pending struct {
	dst     uint32
	payload []byte
	tag     int
}

type linkHandler interface {
	datagram(now time.Duration, src uint32, payload []byte)
	sent(now time.Duration, tag int, r data.Result)
}

// link carries UDP datagrams to and from one radio over DMR confirmed data
// with compressed UDP/IP headers (ETSI TS 102 361-3 clause 7.2.3).
type link struct {
	id      uint32
	rate    enums.DataType
	tx      *data.Sender
	rx      *data.Receiver
	outs    []data.Output
	evs     []data.Event
	pkt     []byte
	queue   []pending
	current int
	ipID    uint16
	h       linkHandler
	emit    func(at time.Duration, raw *layer2.RawBurst)
	stats   linkStats
}

type linkStats struct {
	datagrams, octets, tries int
}

func newLink(id uint32, rate enums.DataType, seed uint64, h linkHandler, emit func(time.Duration, *layer2.RawBurst)) (*link, error) {
	tx, err := data.NewSender(&data.SenderConfig{Slot: enums.Slot1, Source: layer2.SyncSourceMS, Seed: seed})
	if err != nil {
		return nil, err
	}
	rx, err := data.NewReceiver(&data.ReceiverConfig{Address: id, Respond: true, Source: layer2.SyncSourceMS})
	if err != nil {
		return nil, err
	}
	return &link{
		id: id, rate: rate, tx: tx, rx: rx,
		outs: make([]data.Output, 0, data.MaxOutputs),
		evs:  make([]data.Event, 0, data.MaxEvents),
		h:    h, emit: emit,
	}, nil
}

func (l *link) send(now time.Duration, dst uint32, payload []byte, tag int) {
	l.queue = append(l.queue, pending{dst: dst, payload: append([]byte(nil), payload...), tag: tag})
	l.pump(now)
}

func (l *link) pump(now time.Duration) {
	for len(l.queue) > 0 && l.tx.State() == data.SenderIdle {
		p := l.queue[0]
		l.queue = l.queue[1:]
		d := ip.Datagram{SourcePort: appPort, DestinationPort: appPort, ID: l.ipID, Payload: p.payload}
		l.ipID++
		var err error
		if l.pkt, err = ip.AppendCompressed(l.pkt[:0], &d); err != nil {
			l.h.sent(now, p.tag, data.ResultRefused)
			continue
		}
		var h pdu.DataHeader
		if err := h.SetConfirmed(pdu.ConfirmedDataHeader{SAP: enums.SAPUDPIPHeaderCompression, LLIDDestination: p.dst, LLIDSource: l.id}); err != nil {
			l.h.sent(now, p.tag, data.ResultRefused)
			continue
		}
		if err := l.tx.Send(now, &data.Message{Header: h, Rate: l.rate, Payload: l.pkt}); err != nil {
			l.h.sent(now, p.tag, data.ResultRefused)
			continue
		}
		l.current = p.tag
		l.stats.datagrams++
		l.stats.octets += len(l.pkt)
	}
}

func (l *link) deadline() (time.Duration, bool) {
	at, ok := l.tx.Deadline()
	if r, rok := l.rx.Deadline(); rok && (!ok || r < at) {
		at, ok = r, true
	}
	return at, ok
}

func (l *link) tick(now time.Duration) {
	l.outputs(now, l.tx.Tick(now, l.outs[:0]))
	l.events(now, l.rx.Tick(now, l.evs[:0]))
	l.pump(now)
}

func (l *link) hear(now time.Duration, raw *layer2.RawBurst) {
	h := layer2.Hint{At: now}
	l.outputs(now, l.tx.Burst(now, raw, h, l.outs[:0]))
	l.events(now, l.rx.Burst(now, raw, h, l.evs[:0]))
	l.pump(now)
}

func (l *link) channel(now time.Duration, busy bool) {
	l.tx.Channel(now, busy)
}

func (l *link) outputs(now time.Duration, outs []data.Output) {
	for i := range outs {
		o := &outs[i]
		switch o.Kind {
		case data.OutputBurst:
			l.emit(o.Due, &o.Burst)
		case data.OutputDone:
			l.stats.tries += int(o.Try)
			l.h.sent(now, l.current, o.Result)
		}
	}
}

func (l *link) events(now time.Duration, evs []data.Event) {
	for i := range evs {
		e := &evs[i]
		switch e.Kind {
		case data.EventRespond:
			l.emit(e.Due, &e.Burst)
		case data.EventDelivered:
			if e.Message.Fragmented {
				continue
			}
			sap, _ := e.Message.Header.SAP()
			d, err := ip.Decode(sap, e.Message.Payload)
			if err != nil || d.DestinationPort != appPort {
				continue
			}
			l.h.datagram(now, e.Call.Source, d.Payload)
		case data.EventCallStart, data.EventResponse, data.EventCallEnd, data.EventReassembled:
		}
	}
}
