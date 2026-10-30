# Run TCP or UDP over DMR data

This guide shows how to carry UDP datagrams and IPv4 packets over DMR confirmed data between two radios, with the results of two simulated sessions.

## Goal

Connect an IP application to a DMR channel with a data Sender and a data Receiver at each radio.

## What you need

- Go and a checkout of dmrgo. Both examples run offline, on a simulated channel with bit errors.
- The packages [data](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/data) and [ip](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/ip).
- For TCP, a TCP/IP stack. The tcp example uses the netstack of gVisor.

The udp and tcp examples are their own Go modules, so their dependencies (zstd for udp, gVisor for tcp) do not become dependencies of dmrgo. Run them from the top of the repository:

```sh
go run -C v2/examples/udp .
go run -C v2/examples/tcp .
```

The README of each example describes its parts, its flags, the columns of its tables and what the results show: [udp](../../v2/examples/udp/README.md), [tcp](../../v2/examples/tcp/README.md).

## Steps

### 1. Make a Sender and a Receiver for each radio

A [data.Sender](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/data#Sender) sends packets and a [data.Receiver](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/data#Receiver) receives them. Set `Address` to the radio ID of the radio and `Respond` so that the Receiver answers confirmed packets. Make the output and event slices with capacity `data.MaxOutputs` and `data.MaxEvents`, so that they never grow.

<!-- dmrgo:code file=v2/examples/udp/link.go symbol=newLink -->
```go
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
```
<!-- /dmrgo:code -->

### 2. Send a UDP datagram with a compressed header

[ip.AppendCompressed](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/ip#AppendCompressed) puts a compressed UDP/IPv4 header in front of the payload. Send it in a confirmed packet with the SAP UDP/IP header compression. A compressed header has no IPv4 addresses, so the LLIDs of the data header carry the addresses. The Sender takes one packet at a time: send the next one when its state is `data.SenderIdle` again. (ETSI TS 102 361-3 clause 7.2.3)

<!-- dmrgo:code file=v2/examples/udp/link.go symbol=link.pump -->
```go
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
	}
}
```
<!-- /dmrgo:code -->

### 3. Send an IPv4 packet

For TCP, send each IPv4 packet of the stack as one confirmed packet with the SAP IP based packet data. The MTU of the stack must fit one DMR packet. `Send` returns `data.ErrBusy` while the Sender has a packet in progress, so keep the packet in the queue and try again later. (ETSI TS 102 361-3 clause 5.5)

<!-- dmrgo:code file=v2/examples/tcp/link.go symbol=link.pump -->
```go
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
		}
		l.queue = l.queue[1:]
	}
}
```
<!-- /dmrgo:code -->

### 4. Drive the machines with the clock and the channel

The machines do no I/O. Call `Tick` at the time that `Deadline` returns, and pass each burst that the radio hears to `Burst` of both machines. Call `Channel` when the channel goes busy or idle. A polite Sender waits for an idle channel before it starts a try.

<!-- dmrgo:code file=v2/examples/udp/link.go symbol=link.deadline -->
```go
func (l *link) deadline() (time.Duration, bool) {
	at, ok := l.tx.Deadline()
	if r, rok := l.rx.Deadline(); rok && (!ok || r < at) {
		at, ok = r, true
	}
	return at, ok
}
```
<!-- /dmrgo:code -->

<!-- dmrgo:code file=v2/examples/udp/link.go symbol=link.tick -->
```go
func (l *link) tick(now time.Duration) {
	l.outputs(now, l.tx.Tick(now, l.outs[:0]))
	l.events(now, l.rx.Tick(now, l.evs[:0]))
	l.pump(now)
}
```
<!-- /dmrgo:code -->

<!-- dmrgo:code file=v2/examples/udp/link.go symbol=link.hear -->
```go
func (l *link) hear(now time.Duration, raw *layer2.RawBurst) {
	h := layer2.Hint{At: now}
	l.outputs(now, l.tx.Burst(now, raw, h, l.outs[:0]))
	l.events(now, l.rx.Burst(now, raw, h, l.evs[:0]))
	l.pump(now)
}
```
<!-- /dmrgo:code -->

<!-- dmrgo:code file=v2/examples/udp/link.go symbol=link.channel -->
```go
func (l *link) channel(now time.Duration, busy bool) {
	l.tx.Channel(now, busy)
}
```
<!-- /dmrgo:code -->

### 5. Send the outputs and deliver the packets

An `OutputBurst` is a burst to send at its `Due` time, and `OutputDone` ends a packet with its result and its count of tries. An `EventRespond` is a response burst that the Receiver sends, and `EventDelivered` holds a packet. [ip.Decode](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/ip#Decode) reads the datagram from the user data for the SAP of the header.

<!-- dmrgo:code file=v2/examples/udp/link.go symbol=link.outputs -->
```go
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
```
<!-- /dmrgo:code -->

<!-- dmrgo:code file=v2/examples/udp/link.go symbol=link.events -->
```go
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
```
<!-- /dmrgo:code -->

## Output

The udp example sends filled forms in different encodings, at each data rate and bit error rate, and prints the size of each encoding and the time from a form to its acknowledgement.

<!-- dmrgo:output example=udp -->
```text
Octets of the form body, 200 forms, with a 4096-octet dictionary. Each datagram adds a 5-octet form header and a 9-octet compressed UDP/IP header:

| encoding | mean | min | max |
|---|---|---|---|
| screen | 380.0 | 348 | 398 |
| screen+zstd | 202.6 | 180 | 217 |
| fields | 116.6 | 81 | 139 |
| fields+zstd | 108.2 | 84 | 126 |
| fields+zstd+dict | 57.4 | 49 | 65 |

Time from a form to its acknowledgement, 30 forms each, one at a time:

| encoding | rate | BER | acked | mean s | p95 s | max s | bursts/form | DMR tries/form | app resends | copies dropped |
|---|---|---|---|---|---|---|---|---|---|---|
| screen | Rate 1/2 | 0 | 30/30 | 3.4 | 3.5 | 3.5 | 56.3 | 1.00 | 0 | 0 |
| screen | Rate 1/2 | 0.001 | 30/30 | 3.4 | 3.5 | 3.5 | 56.3 | 1.00 | 0 | 0 |
| screen | Rate 1/2 | 0.005 | 30/30 | 3.4 | 3.5 | 3.5 | 56.3 | 1.00 | 0 | 0 |
| screen | Rate 1/2 | 0.01 | 30/30 | 3.4 | 3.5 | 3.5 | 56.3 | 1.00 | 0 | 0 |
| screen | Rate 1/2 | 0.02 | 30/30 | 4.0 | 7.2 | 11.5 | 66.1 | 1.40 | 0 | 2 |
| screen | Rate 3/4 | 0 | 30/30 | 2.4 | 2.5 | 2.5 | 40.4 | 1.00 | 0 | 0 |
| screen | Rate 3/4 | 0.001 | 30/30 | 2.4 | 2.8 | 2.8 | 40.8 | 1.07 | 0 | 0 |
| screen | Rate 3/4 | 0.005 | 30/30 | 2.7 | 2.9 | 3.2 | 44.9 | 1.80 | 0 | 0 |
| screen | Rate 3/4 | 0.01 | 30/30 | 3.3 | 4.0 | 4.7 | 53.0 | 2.73 | 0 | 0 |
| screen | Rate 3/4 | 0.02 | 30/30 | 5.9 | 10.5 | 14.2 | 91.5 | 4.91 | 3 | 0 |
| screen | Rate 1 | 0 | 30/30 | 2.0 | 2.0 | 2.0 | 33.6 | 1.00 | 0 | 0 |
| screen | Rate 1 | 0.001 | 30/30 | 2.9 | 4.0 | 4.1 | 46.1 | 2.77 | 0 | 0 |
| screen | Rate 1 | 0.005 | 27/30 | 6.9 | 11.2 | 15.4 | 124.3 | 6.63 | 11 | 0 |
| screen | Rate 1 | 0.01 | 0/30 | - | - | - | 374.7 | 8.00 | 60 | 0 |
| screen | Rate 1 | 0.02 | 0/30 | - | - | - | 510.8 | 8.00 | 60 | 0 |
| fields+zstd+dict | Rate 1/2 | 0 | 30/30 | 1.4 | 1.5 | 1.5 | 24.0 | 1.00 | 0 | 0 |
| fields+zstd+dict | Rate 1/2 | 0.001 | 30/30 | 1.4 | 1.5 | 1.5 | 24.0 | 1.00 | 0 | 0 |
| fields+zstd+dict | Rate 1/2 | 0.005 | 30/30 | 1.4 | 1.5 | 1.5 | 24.0 | 1.00 | 0 | 0 |
| fields+zstd+dict | Rate 1/2 | 0.01 | 30/30 | 1.4 | 1.5 | 1.5 | 24.0 | 1.00 | 0 | 0 |
| fields+zstd+dict | Rate 1/2 | 0.02 | 30/30 | 1.5 | 2.1 | 2.2 | 24.6 | 1.03 | 0 | 0 |
| fields+zstd+dict | Rate 3/4 | 0 | 30/30 | 1.2 | 1.3 | 1.3 | 20.1 | 1.00 | 0 | 0 |
| fields+zstd+dict | Rate 3/4 | 0.001 | 30/30 | 1.2 | 1.3 | 1.3 | 20.1 | 1.00 | 0 | 0 |
| fields+zstd+dict | Rate 3/4 | 0.005 | 30/30 | 1.3 | 1.6 | 1.6 | 21.8 | 1.33 | 0 | 0 |
| fields+zstd+dict | Rate 3/4 | 0.01 | 30/30 | 1.6 | 2.0 | 2.3 | 25.8 | 1.93 | 0 | 0 |
| fields+zstd+dict | Rate 3/4 | 0.02 | 30/30 | 2.3 | 4.2 | 5.2 | 36.2 | 3.00 | 0 | 0 |
| fields+zstd+dict | Rate 1 | 0 | 30/30 | 1.1 | 1.1 | 1.1 | 19.0 | 1.00 | 0 | 0 |
| fields+zstd+dict | Rate 1 | 0.001 | 30/30 | 1.5 | 2.3 | 2.3 | 24.1 | 1.83 | 0 | 0 |
| fields+zstd+dict | Rate 1 | 0.005 | 30/30 | 3.6 | 10.0 | 14.7 | 51.0 | 4.50 | 4 | 1 |
| fields+zstd+dict | Rate 1 | 0.01 | 10/30 | 5.9 | 10.2 | 10.2 | 143.9 | 7.63 | 45 | 0 |
| fields+zstd+dict | Rate 1 | 0.02 | 0/30 | - | - | - | 187.7 | 8.00 | 60 | 0 |
```
<!-- /dmrgo:output -->

The tcp example runs a TN3270 session over TCP, on a simplex and a duplex channel, with and without a DMR window, and prints the time of each screen.

<!-- dmrgo:output example=tcp -->
```text
TN3270 over TCP over DMR, MTU 576, 5 cycles of form submit and back. Records: negotiation 54 octets in 6 records, form screen 316, form input 160, list screen 1674, PF3 5:

| channel | window | rate | BER | setup s | submit mean s | submit p95 s | back mean s | TCP segments | TCP retransmits | RTO timeouts | DMR packets | bursts |
|---|---|---|---|---|---|---|---|---|---|---|---|---|
| simplex | 1 | Rate 1/2 | 0 | 22.3 | 20.5 | 21.6 | 4.7 | 95 | 1 | 0 | 95 | 2234 |
| simplex | 1 | Rate 1/2 | 0.001 | 22.3 | 20.5 | 21.6 | 4.7 | 95 | 1 | 0 | 95 | 2234 |
| simplex | 1 | Rate 1/2 | 0.005 | 22.3 | 20.5 | 21.6 | 4.7 | 95 | 1 | 0 | 95 | 2234 |
| simplex | 1 | Rate 1/2 | 0.01 | 22.3 | 20.1 | 22.2 | 5.3 | 96 | 1 | 0 | 96 | 2265 |
| simplex | 1 | Rate 1/2 | 0.02 | 22.8 | 21.7 | 22.3 | 4.9 | 95 | 1 | 0 | 95 | 2345 |
| simplex | 1 | Rate 3/4 | 0 | 20.4 | 15.5 | 16.6 | 3.5 | 97 | 2 | 0 | 97 | 1639 |
| simplex | 1 | Rate 3/4 | 0.001 | 20.4 | 15.6 | 16.6 | 3.5 | 97 | 2 | 0 | 97 | 1649 |
| simplex | 1 | Rate 3/4 | 0.005 | 21.1 | 17.6 | 19.0 | 4.0 | 95 | 0 | 0 | 95 | 1797 |
| simplex | 1 | Rate 3/4 | 0.01 | 23.0 | 24.1 | 36.0 | 4.9 | 93 | 0 | 0 | 93 | 2456 |
| simplex | 1 | Rate 3/4 | 0.02 | 54.7 | 41.3 | 62.9 | 10.3 | 103 | 7 | 2 | 103 | 4699 |
| simplex | 1 | Rate 1 | 0 | 11.7 | 13.3 | 14.5 | 2.9 | 91 | 0 | 0 | 91 | 1291 |
| simplex | 1 | Rate 1 | 0.001 | 21.3 | 19.6 | 27.5 | 6.9 | 97 | 3 | 0 | 97 | 2181 |
| simplex | 1 | Rate 1 | 0.005 | 80.9 | 124.4 | 193.3 | 20.9 | 126 | 27 | 10 | 126 | 6787 |
| simplex | 1 | Rate 1 | 0.01 | session stopped at 2m12.58s: terminal TCP error: operation timed out | | | | | | | | |
| simplex | 1 | Rate 1 | 0.02 | session stopped at 2m7s: terminal TCP error: operation timed out | | | | | | | | |
| simplex | 4 | Rate 1/2 | 0 | 16.6 | 16.0 | 17.6 | 7.5 | 101 | 4 | 0 | 101 | 2096 |
| simplex | 4 | Rate 1/2 | 0.001 | 16.6 | 16.0 | 17.6 | 7.5 | 101 | 4 | 0 | 101 | 2096 |
| simplex | 4 | Rate 1/2 | 0.005 | 16.6 | 16.0 | 17.6 | 7.5 | 101 | 4 | 0 | 101 | 2096 |
| simplex | 4 | Rate 1/2 | 0.01 | 16.6 | 31.0 | 91.0 | 7.0 | 105 | 8 | 1 | 105 | 3335 |
| simplex | 4 | Rate 1/2 | 0.02 | 136.3 | 65.8 | 101.1 | 87.1 | 125 | 24 | 14 | 125 | 9242 |
| simplex | 4 | Rate 3/4 | 0 | 14.3 | 10.6 | 11.7 | 4.7 | 99 | 4 | 0 | 99 | 1422 |
| simplex | 4 | Rate 3/4 | 0.001 | 14.3 | 21.2 | 62.2 | 3.6 | 100 | 6 | 2 | 100 | 2218 |
| simplex | 4 | Rate 3/4 | 0.005 | 649.7 | 150.4 | 304.6 | 228.7 | 152 | 53 | 32 | 152 | 15616 |
| simplex | 4 | Rate 3/4 | 0.01 | 40.3 | 311.2 | 493.1 | 11.2 | 153 | 55 | 24 | 153 | 16768 |
| simplex | 4 | Rate 3/4 | 0.02 | 364.7 | 285.5 | 401.3 | 87.6 | 141 | 38 | 7 | 141 | 11809 |
| simplex | 4 | Rate 1 | 0 | 13.4 | 8.8 | 12.2 | 3.9 | 103 | 6 | 0 | 103 | 1158 |
| simplex | 4 | Rate 1 | 0.001 | 225.7 | 183.6 | 290.4 | 10.5 | 157 | 64 | 31 | 157 | 14069 |
| simplex | 4 | Rate 1 | 0.005 | session stopped: not done after 1h10m0s, at record 12 of 27 | | | | | | | | |
| simplex | 4 | Rate 1 | 0.01 | session stopped at 2m25.42s: half open, the host dropped the handshake and the terminal waits | | | | | | | | |
| simplex | 4 | Rate 1 | 0.02 | session stopped at 2m7s: terminal TCP error: operation timed out | | | | | | | | |
| duplex | 1 | Rate 1/2 | 0 | 13.1 | 15.5 | 15.7 | 4.6 | 95 | 2 | 0 | 95 | 2192 |
| duplex | 1 | Rate 1/2 | 0.001 | 13.1 | 15.5 | 15.7 | 4.6 | 95 | 2 | 0 | 95 | 2192 |
| duplex | 1 | Rate 1/2 | 0.005 | 13.1 | 15.5 | 15.7 | 4.6 | 95 | 2 | 0 | 95 | 2192 |
| duplex | 1 | Rate 1/2 | 0.01 | 13.1 | 15.9 | 17.3 | 4.6 | 95 | 2 | 0 | 95 | 2223 |
| duplex | 1 | Rate 1/2 | 0.02 | 13.4 | 16.3 | 17.2 | 4.8 | 95 | 2 | 0 | 95 | 2288 |
| duplex | 1 | Rate 3/4 | 0 | 10.5 | 10.4 | 10.4 | 3.3 | 93 | 1 | 0 | 93 | 1530 |
| duplex | 1 | Rate 3/4 | 0.001 | 10.5 | 10.4 | 10.4 | 3.3 | 93 | 1 | 0 | 93 | 1530 |
| duplex | 1 | Rate 3/4 | 0.005 | 10.7 | 11.6 | 12.1 | 3.9 | 93 | 1 | 0 | 93 | 1685 |
| duplex | 1 | Rate 3/4 | 0.01 | 14.4 | 14.4 | 15.0 | 4.6 | 95 | 2 | 0 | 95 | 2060 |
| duplex | 1 | Rate 3/4 | 0.02 | 24.2 | 26.8 | 36.3 | 7.7 | 98 | 3 | 0 | 98 | 3580 |
| duplex | 1 | Rate 1 | 0 | 8.3 | 8.1 | 8.1 | 2.4 | 89 | 1 | 0 | 89 | 1199 |
| duplex | 1 | Rate 1 | 0.001 | 10.7 | 11.7 | 12.7 | 3.8 | 89 | 1 | 0 | 89 | 1660 |
| duplex | 1 | Rate 1 | 0.005 | 37.7 | 380.6 | 1230.7 | 34.5 | 144 | 47 | 25 | 143 | 8334 |
| duplex | 1 | Rate 1 | 0.01 | session stopped at 2m12.58s: half open, the host dropped the handshake and the terminal waits | | | | | | | | |
| duplex | 1 | Rate 1 | 0.02 | session stopped at 2m7s: terminal TCP error: operation timed out | | | | | | | | |
| duplex | 4 | Rate 1/2 | 0 | 12.0 | 14.8 | 15.2 | 4.2 | 95 | 2 | 0 | 95 | 2120 |
| duplex | 4 | Rate 1/2 | 0.001 | 12.0 | 14.8 | 15.2 | 4.2 | 95 | 2 | 0 | 95 | 2120 |
| duplex | 4 | Rate 1/2 | 0.005 | 12.0 | 14.8 | 15.2 | 4.2 | 95 | 2 | 0 | 95 | 2120 |
| duplex | 4 | Rate 1/2 | 0.01 | 12.0 | 17.0 | 25.7 | 4.2 | 97 | 2 | 0 | 97 | 2328 |
| duplex | 4 | Rate 1/2 | 0.02 | 110.1 | 72.7 | 117.5 | 4.2 | 118 | 23 | 10 | 118 | 8340 |
| duplex | 4 | Rate 3/4 | 0 | 8.9 | 9.7 | 9.7 | 2.9 | 93 | 1 | 0 | 93 | 1446 |
| duplex | 4 | Rate 3/4 | 0.001 | 8.9 | 44.2 | 182.5 | 3.0 | 105 | 13 | 7 | 105 | 4002 |
| duplex | 4 | Rate 3/4 | 0.005 | 53.6 | 298.7 | 522.7 | 120.5 | 149 | 53 | 31 | 149 | 14130 |
| duplex | 4 | Rate 3/4 | 0.01 | 335.9 | 313.5 | 514.8 | 7.7 | 155 | 59 | 27 | 155 | 17669 |
| duplex | 4 | Rate 3/4 | 0.02 | 55.4 | 201.0 | 248.8 | 28.7 | 137 | 39 | 13 | 137 | 11320 |
| duplex | 4 | Rate 1 | 0 | 7.3 | 7.4 | 7.4 | 2.1 | 89 | 1 | 0 | 89 | 1125 |
| duplex | 4 | Rate 1 | 0.001 | 56.9 | 173.7 | 303.6 | 7.6 | 146 | 55 | 22 | 146 | 12433 |
| duplex | 4 | Rate 1 | 0.005 | 125.9 | 388.7 | 543.8 | 48.7 | 158 | 58 | 26 | 158 | 13134 |
| duplex | 4 | Rate 1 | 0.01 | session stopped at 2m25.42s: terminal TCP error: operation timed out | | | | | | | | |
| duplex | 4 | Rate 1 | 0.02 | session stopped at 2m7s: terminal TCP error: operation timed out | | | | | | | | |
```
<!-- /dmrgo:output -->

## How it works

The DMR IP bearer service carries an IPv4 packet in the user data of a data packet. With the SAP IP based packet data, the packet has its full IPv4 header. With the SAP UDP/IP header compression, a short header replaces the IPv4 and UDP headers, and the ports <!-- dmrgo:const pkg=ip name=PortText -->`5016`<!-- /dmrgo:const --> and <!-- dmrgo:const pkg=ip name=PortLocation -->`5017`<!-- /dmrgo:const --> need no extended header. [ip.RadioAddress](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/ip#RadioAddress) gives the IPv4 address of a radio ID, for a stack that needs one. (ETSI TS 102 361-3 clauses 5.5, 5.6 and 7.2.3)

A DMR ACK covers one radio hop, and the IP bearer service delivers a packet again after its response is lost. So an application on UDP needs its own acknowledgement and its own check for copies, as the form protocol of the udp example has. (ETSI TS 102 361-3 clause 5.4.1.3)

By default a Sender uses stop and wait: each packet requests a response. `SenderConfig.Window` turns on a sliding window, where only the last packet of a window requests a response. The Receiver keeps the blocks of the last packet only, so one lost block makes the Sender send that packet and every later packet of the window again. The tcp results show when a window helps. (ETSI TS 102 361-3 clause 5.4.4)

The state machines: [data Sender](../machines/data-sender.md) and [data Receiver](../machines/data-receiver.md).

## Related guides

- [Send SMS and confirmed data with retries](send-data.md)
- [Report and request location: LIP, LRRP, NMEA](location.md)
