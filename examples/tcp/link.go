package main

import (
	"errors"
	"time"

	"github.com/USA-RedDragon/dmrgo/v2/data"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

type linkStats struct {
	packets, octets, tries, failed int
}

// link carries IPv4 packets to and from one radio over DMR confirmed data
// with the SAP IP based packet data (ETSI TS 102 361-3 clause 5.5).
type link struct {
	id      uint32
	peer    uint32
	rate    enums.DataType
	tx      *data.Sender
	rx      *data.Receiver
	outs    []data.Output
	evs     []data.Event
	queue   [][]byte
	inbound func(now time.Duration, pkt []byte)
	emitTx  func(at time.Duration, raw *layer2.RawBurst)
	emitRx  func(at time.Duration, raw *layer2.RawBurst)
	stats   linkStats
}

func newLink(id, peer uint32, rate enums.DataType, window int, seed uint64, emitTx, emitRx func(time.Duration, *layer2.RawBurst)) (*link, error) {
	tx, err := data.NewSender(&data.SenderConfig{Source: layer2.SyncSourceMS, Seed: seed, Window: window})
	if err != nil {
		return nil, err
	}
	rx, err := data.NewReceiver(&data.ReceiverConfig{Address: id, Respond: true, Source: layer2.SyncSourceMS})
	if err != nil {
		return nil, err
	}
	return &link{
		id: id, peer: peer, rate: rate, tx: tx, rx: rx,
		outs:   make([]data.Output, 0, data.MaxOutputs),
		evs:    make([]data.Event, 0, data.MaxEvents),
		emitTx: emitTx, emitRx: emitRx,
	}, nil
}

func (l *link) send(now time.Duration, pkt []byte) {
	l.queue = append(l.queue, pkt)
	l.pump(now)
}

func (l *link) pump(now time.Duration) {
	for len(l.queue) > 0 {
		var h pdu.DataHeader
		if err := h.SetConfirmed(pdu.ConfirmedDataHeader{SAP: enums.SAPIPBasedPacketData, LLIDDestination: l.peer, LLIDSource: l.id}); err != nil {
			l.queue, l.stats.failed = l.queue[1:], l.stats.failed+1
			continue
		}
		err := l.tx.Send(now, &data.Message{Header: h, Rate: l.rate, Payload: l.queue[0]})
		if errors.Is(err, data.ErrBusy) {
			return
		}
		if err != nil {
			l.stats.failed++
		} else {
			l.stats.packets++
			l.stats.octets += len(l.queue[0])
		}
		l.queue = l.queue[1:]
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
	l.outputs(l.tx.Tick(now, l.outs[:0]))
	l.events(now, l.rx.Tick(now, l.evs[:0]))
	l.pump(now)
}

func (l *link) hear(now time.Duration, raw *layer2.RawBurst, toTx, toRx bool) {
	h := layer2.Hint{At: now}
	if toTx {
		l.outputs(l.tx.Burst(now, raw, h, l.outs[:0]))
	}
	if toRx {
		l.events(now, l.rx.Burst(now, raw, h, l.evs[:0]))
	}
	l.pump(now)
}

func (l *link) channel(now time.Duration, busy bool) {
	l.tx.Channel(now, busy)
}

func (l *link) outputs(outs []data.Output) {
	for i := range outs {
		o := &outs[i]
		switch o.Kind {
		case data.OutputBurst:
			l.emitTx(o.Due, &o.Burst)
		case data.OutputDone:
			l.stats.tries += int(o.Try)
			if o.Result != data.ResultDelivered {
				l.stats.failed++
			}
		}
	}
}

func (l *link) events(now time.Duration, evs []data.Event) {
	for i := range evs {
		e := &evs[i]
		switch e.Kind {
		case data.EventRespond:
			l.emitRx(e.Due, &e.Burst)
		case data.EventDelivered:
			if sap, _ := e.Message.Header.SAP(); sap == enums.SAPIPBasedPacketData && !e.Message.Fragmented {
				l.inbound(now, e.Message.Payload)
			}
		case data.EventCallStart, data.EventResponse, data.EventCallEnd, data.EventReassembled:
		}
	}
}
