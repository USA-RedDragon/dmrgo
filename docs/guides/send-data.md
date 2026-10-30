# Send SMS and confirmed data with retries

This guide shows how to read the text of a received SMS, send unconfirmed and confirmed data packets, and resend only the blocks that the receiver did not get.

## Goal

Build data packets into bursts, reassemble them on the other side, and recover a lost block with a selective retry.

## What you need

- Go and a checkout of dmrgo. The example runs offline.
- The SMS captures in `v2/layer2/testdata`. They hold the same text in the formats of different radios.
- The packages [layer2/packet](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/layer2/packet), [layer2/pdu](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/layer2/pdu) and [sms](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/sms). For a radio or a gateway on a live channel, also [data](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/data).

Run the example from the top of the repository:

```sh
go run -C v2 ./examples/data
```

## Steps

### 1. Reassemble a received SMS

Give each decoded burst to a [packet.Assembler](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/layer2/packet#Assembler). `AddBurst` returns the [Message](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/layer2/packet#Message) when the packet is complete. [sms.Decode](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/sms#Decode) finds the text format from the header and the user data, and returns the text as UTF-8.

<!-- dmrgo:code file=v2/examples/data/main.go symbol=capture -->
```go
func capture(w io.Writer, path string) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if len(raw)%layer2.RawBurstSize != 0 {
		return fmt.Errorf("length %d is not a multiple of %d", len(raw), layer2.RawBurstSize)
	}
	a := packet.NewAssembler(127)
	n := 0
	b := layer2.NewBurst()
	for off := 0; off < len(raw); off += layer2.RawBurstSize {
		if err := b.DecodeFromBytes(layer2.RawBurst(raw[off:])); err != nil {
			return fmt.Errorf("burst %d: %w", off/layer2.RawBurstSize, err)
		}
		m, done, err := a.AddBurst(b)
		if err != nil {
			return fmt.Errorf("burst %d: %w", off/layer2.RawBurstSize, err)
		}
		if !done {
			continue
		}
		n++
		_, t, err := sms.Decode(nil, &m.Header, m.Data)
		if err != nil {
			return fmt.Errorf("message %d: %w", n, err)
		}
		fmt.Fprintf(w, "%s: %s %d -> %d, %s, %d octets, CRC %08x, text %q\n",
			filepath.Base(path), m.Header.Format(), t.Source, t.Destination, m.DataType, len(m.Data), m.CRC, t.Body)
	}
	if n != 1 {
		return fmt.Errorf("got %d messages, want 1", n)
	}
	return nil
}
```
<!-- /dmrgo:code -->

### 2. Send an unconfirmed packet

Put a compressed UDP/IPv4 header in front of the text, and make an unconfirmed data header with the SAP UDP/IP header compression. [packet.NewPacket](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/layer2/packet#NewPacket) cuts the payload into blocks of the rate that you pick and adds the pad octets and the message CRC. [packet.Bursts](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/layer2/packet#Bursts) codes the header and the blocks as bursts.

<!-- dmrgo:code file=v2/examples/data/main.go symbol=unconfirmed -->
```go
func unconfirmed(w io.Writer) error {
	udp := pdu.UDPIPv4CompressedHeader{IPv4Identification: 0x1234, SPID: enums.SPIDTextMessage, DPID: enums.DPIDTextMessage}
	udpHeader, err := udp.Encode()
	if err != nil {
		return err
	}
	text := "Hello from dmrgo, unconfirmed SMS test"
	payload := append(udpHeader[:], text...)

	h, err := pdu.UnconfirmedDataHeader{
		SAP: enums.SAPUDPIPHeaderCompression, LLIDDestination: dst, LLIDSource: src, FullMessageFlag: true}.DataHeader()
	if err != nil {
		return err
	}
	p, err := packet.NewPacket(h, payload, enums.DataTypeRate12)
	if err != nil {
		return err
	}
	bursts, err := packet.Bursts(&p, colorCode, layer2.SyncSourceMS)
	if err != nil {
		return err
	}
	a := packet.NewAssembler(127)
	m, err := receive(a, bursts)
	if err != nil {
		return err
	}
	if !bytes.Equal(m.Data, payload) {
		return fmt.Errorf("payload %x, want %x", m.Data, payload)
	}
	got := pdu.DecodeUDPIPv4CompressedHeader([9]byte(m.Data[:9]))
	u, ok := m.Header.AsUnconfirmed()
	if !ok {
		return fmt.Errorf("header format %s, want unconfirmed", m.Header.Format())
	}
	fmt.Fprintf(w, "unconfirmed: %d bursts, BF=%d POC=%d, port %d, text %q: ok\n",
		len(bursts), u.BlocksToFollow, u.PadOctetCount, uint16(ip.DPIDPort(got.DPID)), m.Data[9:])
	return nil
}
```
<!-- /dmrgo:code -->

[sms.AppendPacket](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/sms#AppendPacket) builds the user data and the data header of a text in each format that radios use, so you do not build the UDP header yourself:

<!-- dmrgo:code file=v2/sms/example_test.go symbol=Example -->
```go
func Example() {
	t := sms.Text{Format: sms.FormatTMS, Source: 3191868, Destination: 3140598, Body: []byte("Hello")}
	data, err := sms.AppendPacket(nil, &t, &header)
	if err != nil {
		fmt.Println(err)
		return
	}
	p, err := packet.NewPacket(header.Data, data, enums.DataTypeRate12)
	if err != nil {
		fmt.Println(err)
		return
	}
	m := packet.Message{Header: p.Header, Data: data}
	_, got, err := sms.Decode(nil, &m.Header, m.Data)
	fmt.Printf("%d blocks, %s from %d: %s %v\n", len(p.Blocks), got.Format, got.Source, got.Body, err)
}
```
<!-- /dmrgo:code -->

<!-- dmrgo:output test=sms.Example -->
```text
5 blocks, TMS from 3191868: Hello <nil>
```
<!-- /dmrgo:output -->

### 3. Send a confirmed packet and lose a block

A confirmed data header requests a response. Each block of a confirmed packet has its own sequence number (DBSN) and CRC-9, so the receiver can identify each lost block. The example corrupts one block on the air.

<!-- dmrgo:code file=v2/examples/data/main.go symbol=lossyConfirmed -->
```go
func lossyConfirmed(lip []byte) ([]layer2.RawBurst, layer2.RawBurst, error) {
	h, err := pdu.ConfirmedDataHeader{
		ResponseRequested: true, SAP: enums.SAPIPBasedPacketData, LLIDDestination: dst, LLIDSource: src,
		FullMessageFlag: true, ReSynchronizeFlag: true, SendSequenceNumber: 3, FragmentSequenceNumber: 8}.DataHeader()
	if err != nil {
		return nil, layer2.RawBurst{}, err
	}
	p, err := packet.NewPacket(h, lip, enums.DataTypeRate34)
	if err != nil {
		return nil, layer2.RawBurst{}, err
	}
	bursts, err := packet.Bursts(&p, colorCode, layer2.SyncSourceMS)
	if err != nil {
		return nil, layer2.RawBurst{}, err
	}
	good := bursts[2]
	blk, ok := p.Blocks[1].(*pdu.Rate34Data)
	if !ok {
		return nil, layer2.RawBurst{}, fmt.Errorf("block 1 is %T", p.Blocks[1])
	}
	bad := *blk
	bad.Data[7] ^= 0x10
	if bursts[2], err = layer2.BuildRate34DataBurst(bad.Data, colorCode, layer2.SyncSourceMS); err != nil {
		return nil, layer2.RawBurst{}, err
	}
	return bursts, good, nil
}
```
<!-- /dmrgo:code -->

### 4. Answer with a selective ACK and resend the missing block

The receiving Assembler returns `packet.ErrBlocksMissing`, and `Missing` returns the blocks that it did not get. `Response` builds the SACK packet. At the sender, `RetryBlocks` of the response returns the blocks to send again.

<!-- dmrgo:code file=v2/examples/data/main.go symbol=requestRetry -->
```go
func requestRetry(w io.Writer, bursts []layer2.RawBurst) (*packet.Assembler, *packet.Assembler, error) {
	rx := packet.NewAssembler(127)
	if _, err := receive(rx, bursts); !errors.Is(err, packet.ErrBlocksMissing) {
		return nil, nil, fmt.Errorf("corrupted packet: %w, want %w", err, packet.ErrBlocksMissing)
	}
	sack, err := rx.Response()
	if err != nil {
		return nil, nil, err
	}
	sackBursts, err := packet.Bursts(&sack, colorCode, layer2.SyncSourceMS)
	if err != nil {
		return nil, nil, err
	}
	tx := packet.NewAssembler(127)
	rm, err := receive(tx, sackBursts)
	if err != nil {
		return nil, nil, err
	}
	if rm.Response == nil {
		return nil, nil, errors.New("SACK carries no response data")
	}
	retry := rm.Response.RetryBlocks()
	sackCode, err := responseOf(&rm.Header)
	if err != nil {
		return nil, nil, err
	}
	fmt.Fprintf(w, "confirmed: %d bursts, missing %v, %s requests %v\n",
		len(bursts), rx.Missing(), sackCode, retry)
	if !slices.Equal(retry, []int{1}) {
		return nil, nil, fmt.Errorf("retry %v, want [1]", retry)
	}
	return rx, tx, nil
}
```
<!-- /dmrgo:code -->

### 5. Complete the packet and send the ACK

Send the header and the missing block again. The Assembler completes the packet, and [packet.NewResponse](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/layer2/packet#NewResponse) builds the ACK.

<!-- dmrgo:code file=v2/examples/data/main.go symbol=completeRetry -->
```go
func completeRetry(w io.Writer, rx, tx *packet.Assembler, lip []byte, header, good layer2.RawBurst) error {
	m, err := receive(rx, []layer2.RawBurst{header, good})
	if err != nil {
		return err
	}
	if !bytes.Equal(m.Data, lip) {
		return fmt.Errorf("payload %q, want %q", m.Data, lip)
	}
	ch, ok := m.Header.AsConfirmed()
	if !ok {
		return fmt.Errorf("header format %s, want confirmed", m.Header.Format())
	}
	ack, err := packet.NewResponse(&ch, packet.ResponseACK, nil)
	if err != nil {
		return err
	}
	ackBursts, err := packet.Bursts(&ack, colorCode, layer2.SyncSourceMS)
	if err != nil {
		return err
	}
	am, err := receive(tx, ackBursts)
	if err != nil {
		return err
	}
	ackDst, ackSrc, _ := am.Header.Addresses()
	ackCode, err := responseOf(&am.Header)
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "confirmed: retry done, %q, %s %d -> %d: ok\n",
		m.Data, ackCode, ackSrc, ackDst)
	return nil
}
```
<!-- /dmrgo:code -->

## Output

The first lines are the received SMS captures. Then come the unconfirmed packet, the confirmed packet with its SACK, and the retry with its ACK.

<!-- dmrgo:output example=data -->
```text
d-sms.bin: Unconfirmed 3191868 -> 9990, Rate 1/2, 56 octets, CRC bc5a0a35, text "TEST KI5VMF"
h-sms.bin: Defined Short 3191868 -> 9990, Rate 1/2, 26 octets, CRC 654553b8, text "TEST KI5VMF"
m-sms.bin: Unconfirmed 3191868 -> 9990, Rate 1/2, 62 octets, CRC 0b8c211b, text "TEST KI5VMF"
unconfirmed: 6 bursts, BF=5 POC=9, port 5016, text "Hello from dmrgo, unconfirmed SMS test": ok
confirmed: 4 bursts, missing [1], SACK requests [1]
confirmed: retry done, "LIP: 41.878,-87.629,hdg=90,v=12", ACK 3120002 -> 3120001: ok
```
<!-- /dmrgo:output -->

## Send on a live channel

The example above builds every packet and response by hand. On a live channel, a [data.Sender](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/data#Sender) sends the CSBK preambles, the header and the blocks at their times, waits for the response, and resends the blocks that a SACK requests. A [data.Receiver](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/data#Receiver) with `Respond` set reassembles the packet and returns the response bursts to send. Both do no I/O: each method takes the time now and appends outputs or events to a slice of the caller.

In this example the air loses one burst of the first try. The Receiver requests the block again with a SACK, and the Sender resends only that block.

<!-- dmrgo:code file=v2/data/sender_example_test.go symbol=ExampleSender -->
```go
func ExampleSender() {
	tx, err := data.NewSender(&data.SenderConfig{Slot: enums.Slot1, Source: layer2.SyncSourceMS, ColorCode: 1})
	if err != nil {
		fmt.Println(err)
		return
	}
	rx, err := data.NewReceiver(&data.ReceiverConfig{Address: 3120002, Respond: true})
	if err != nil {
		fmt.Println(err)
		return
	}
	m := &data.Message{Rate: enums.DataTypeRate12, Payload: []byte("a confirmed message of a few blocks")}
	if err := m.Header.SetConfirmed(pdu.ConfirmedDataHeader{
		SAP: enums.SAPIPBasedPacketData, LLIDDestination: 3120002, LLIDSource: 3120001,
	}); err != nil {
		fmt.Println(err)
		return
	}
	if err := tx.Send(0, m); err != nil {
		fmt.Println(err)
		return
	}
	outputs := make([]data.Output, 0, data.MaxOutputs)
	events := make([]data.Event, 0, data.MaxEvents)
	var replies []data.Event
	sent := 0
	for now := time.Duration(0); tx.State() != data.SenderIdle; now += 10 * time.Millisecond {
		for _, e := range replies {
			for _, o := range tx.Burst(e.Due, &e.Burst, layer2.Hint{}, outputs[:0]) {
				fmt.Printf("%v after %d tries (%v)\n", o.Result, o.Try, o.Response)
			}
		}
		replies = replies[:0]
		for _, o := range tx.Tick(now, outputs[:0]) {
			sent++
			if sent == data.DefaultPreambles+3 {
				continue
			}
			for _, e := range rx.Burst(o.Due, &o.Burst, layer2.Hint{}, events[:0]) {
				switch e.Kind {
				case data.EventDelivered:
					fmt.Printf("delivered %q\n", e.Message.Payload)
				case data.EventRespond:
					if e.Burst.SlotType().DataType == enums.DataTypeDataHeader {
						fmt.Printf("receiver sends %v\n", e.Response)
					}
					replies = append(replies, e)
				case data.EventCallStart, data.EventResponse, data.EventCallEnd, data.EventReassembled:
				}
			}
		}
	}
}
```
<!-- /dmrgo:code -->

<!-- dmrgo:output test=data.ExampleSender -->
```text
receiver sends SACK
delivered "a confirmed message of a few blocks"
receiver sends ACK
Delivered after 2 tries (ACK)
```
<!-- /dmrgo:output -->

The Sender sends <!-- dmrgo:const pkg=data name=DefaultPreambles -->`5`<!-- /dmrgo:const --> preamble CSBKs by default before a packet. The config of each machine:

<!-- dmrgo:config pkg=data type=SenderConfig -->
| Field | Type | Default | Description |
|---|---|---|---|
| `ColorCode` | `enums.ColorCode` | zero value | ColorCode is the color code of the bursts. |
| `Source` | `layer2.SyncSource` | zero value | Source selects the SYNC patterns: layer2.SyncSourceBS, the zero value, for a repeater outbound, layer2.SyncSourceMS for a radio. |
| `Access` | `enums.Access` | zero value | Access is the channel access policy. The zero value, enums.AccessPoliteOwnCC, sends only while Channel last reported the channel idle (ETSI TS 102 361-1 clause 5.2.1.6). |
| `Preambles` | `int` | `DefaultPreambles` (5) | Preambles is the number of preamble CSBKs before the header of each try, so that a radio in power save wakes up. A try that starts within DefaultHang of a response from the target sends at most one, as the target is still in the data call, and one Pre_CSBK is the ChanAuth PDU of data in channel hangtime (ETSI TS 102 361-2 clause 5.1.1.6.0). 0 means DefaultPreambles, and a negative value sends none; at most MaxPreambles. |
| `IdleSearch` | `time.Duration` | not machine-readable; see the description | IdleSearch is the time that a polite Sender waits for an idle channel before it holds off. 0 means DefaultIdleSearch, and a negative value holds off at once. |
| `Holdoff` | `time.Duration` | not machine-readable; see the description | Holdoff is the longest random holdoff T_Holdoff after a busy channel. 0 means constants.THoldoffMax (2 s), and a negative value turns the holdoff off. |
| `ResponseWait` | `time.Duration` | not machine-readable; see the description | ResponseWait is the time T_RspnsWait from the end of the last burst to the response of a confirmed packet. 0 means constants.TResponseWait (180 ms). A response that comes through a network can need more; constants.TResponseWaitSimulcast is 2 s. |
| `TxLimit` | `time.Duration` | not machine-readable; see the description | TxLimit is the time T_DataTxLmt from Send after which no new try starts. 0 means constants.TDataTxLimit (60 s), and a negative value turns the limit off. |
| `Retries` | `int` | not machine-readable; see the description | Retries is the most transmissions N_RtryLmt of a confirmed message, the first one included. 0 means constants.NRetryLimit (8); at most MaxRetries. |
| `MaxBlocks` | `int` | not machine-readable; see the description | MaxBlocks is the most data blocks of a packet. 0 means MaxBlocks. Each block takes about 110 bytes, so a microcontroller can set 16. |
| `Peers` | `int` | not machine-readable; see the description | Peers is the number of links whose send sequence number V(S) the Sender keeps. 0 means DefaultPeers; at most MaxPeers. |
| `Resync` | `bool` | zero value | Resync sets the S flag on the first packet to a link and on the resend after a NACK for N(S) out of sequence, so that the receiver takes the N(S). ETSI TS 102 361-1 clause 9.3.23 keeps S for special registration messages, so false, the default, never sets it. |
| `Window` | `int` | zero value | Window is the most C_HEAD messages of the IP bearer service in progress at once, for sliding window flow control. Send queues up to Window of them to one link, and the Sender sends them one after the other with the Response Requested flag set only on the last, then takes one response for all of them. 0 or 1, the default, means stop and wait: each message requests its own response. At most MaxWindow. Each message of the window takes its own packet.Builder. Use a window only with receivers known to keep every packet of it: the AnyTone D890UV firmware (also in the BTECH DA-7X2) sends an ACK for the last packet of a window and keeps only that packet, so the earlier ones are lost. (ETSI TS 102 361-3 clause 5.4.4) |
| `Seed` | `uint64` | zero value | Seed seeds the random holdoff. Two Senders with the same seed hold off for the same times, so give each radio its own seed. |
| `Slot` | `enums.Slot` | zero value | Slot is the TDMA slot of the Sender, which it puts in the Hint of each burst it sends. It must be Slot1 or Slot2. |
| `Lead` | `time.Duration` | not machine-readable; see the description | Lead makes Tick emit a burst this long before its Due time, so that an RF FIFO can be filled ahead. 0 means at Due; it must not be negative. The timers of the Sender keep to the Due times. |
<!-- /dmrgo:config -->

<!-- dmrgo:config pkg=data type=ReceiverConfig -->
| Field | Type | Default | Description |
|---|---|---|---|
| `ColorCode` | `*enums.ColorCode` | zero value | ColorCode, when set, drops bursts with another color code, and is the color code of the responses. Nil accepts every color code and answers with the color code of the packet. |
| `Address` | `uint32` | zero value | Address is the LLID of this station. A confirmed packet gets a response only when its destination is Address. 0 answers every confirmed packet to a radio, as a gateway does. |
| `Respond` | `bool` | zero value | Respond makes the Receiver answer confirmed packets with an ACK, NACK or SACK (EventRespond), and drop duplicate and out of sequence packets outside the IP bearer service, as the package doc describes. False delivers every complete packet and answers none, as a scanner or bridge needs. |
| `Source` | `layer2.SyncSource` | zero value | Source selects the SYNC of the responses: layer2.SyncSourceBS, the zero value, for a repeater outbound, layer2.SyncSourceMS for a radio. |
| `ResponseDelay` | `time.Duration` | not machine-readable; see the description | ResponseDelay is the time from the last block of a packet to the first response burst. 0 means one slot period (60 ms), the next burst of the logical channel, and a negative value means at once. |
| `MaxBlocks` | `int` | not machine-readable; see the description | MaxBlocks is the most data blocks of a packet. 0 means MaxBlocks; a larger packet is dropped. Each block takes 24 bytes, so a microcontroller can set 16. |
| `MaxFragment` | `int` | not machine-readable; see the description | MaxFragment is the largest payload of one packet. 0 means DefaultMaxFragment; at most MaxMessage. A larger confirmed packet gets a NACK for memory full, and any larger packet is dropped. |
| `MaxMessage` | `int` | not machine-readable; see the description | MaxMessage is the size of the buffer that joins the fragments of a confirmed message. 0 means DefaultMaxMessage; at most MaxMessage. A longer message is not joined; its fragments are still delivered. |
| `Peers` | `int` | not machine-readable; see the description | Peers is the number of links whose receive sequence number VI the Receiver keeps. 0 means DefaultPeers; at most MaxPeers. When the table is full, the link heard least recently is dropped (ETSI TS 102 361-1 clause 9.3.24). |
| `Lost` | `time.Duration` | not machine-readable; see the description | Lost is the time without a burst, while a packet waits for blocks, after which the slots that passed count as lost blocks, one each 60 ms. 0 means DefaultLost, and a negative value counts lost blocks only from the transport sequence number and at the end of the call. A network transport with jitter sets Hint.Seq and a longer Lost. |
| `Hang` | `time.Duration` | not machine-readable; see the description | Hang is the time without a burst after which a data call ends with EndQuiet. 0 means DefaultHang, and a negative value turns the timer off. |
| `AcceptBadCRC` | `bool` | zero value | AcceptBadCRC delivers packets whose CRC failed, with Message.BadCRC set, as packet.Assembler.AcceptBadCRC describes. A confirmed packet still gets a NACK, so that a good copy can follow. |
| `Scratch` | `*layer2.Burst` | `layer2.NewBurst()` | Scratch, when set, is the Burst that Burst decodes into. Machines that run on one goroutine can share one, because a Receiver is done with it when its method returns. Make it with layer2.NewBurst. Nil means the Receiver makes its own. |
<!-- /dmrgo:config -->

## How it works

A data packet is a header block, then the data blocks that the header announces in its blocks to follow (BF) field. The last block ends with the 32-bit message CRC, and the header gives the pad octet count (POC) before it. A confirmed packet has a DBSN and a CRC-9 in each block. The receiver answers with an ACK when it has every block and the message CRC passes, or with a SACK that lists the blocks to send again. A retry header has the Full Message Flag clear, and the Assembler adds the blocks of the retry to the packet in progress. (ETSI TS 102 361-1 clause 8.2, clause B.3.9, clause 8.2.2.3)

The text formats differ by radio: MOTOTRBO TMS, the ETSI IP text, the DMR Standard text and the H-SMS text of AnyTone radios, and ETSI defined data. `sms.Decode` reads all of them. (ETSI TS 102 361-3 clauses 5.7.1 and 6.1)

The state machines: [data Sender](../machines/data-sender.md) and [data Receiver](../machines/data-receiver.md). A program that only watches the channel can follow the preamble CSBKs of a transfer and the header that they announce with the [csbk Preambles](../machines/csbk-preambles.md).

## Related guides

- [Run TCP or UDP over DMR data](tcp-udp-over-dmr.md)
- [Report and request location: LIP, LRRP, NMEA](location.md)
- [Decode and re-encode captures](decode-captures.md)
- [Rewrite and relay a voice call](relay-voice-call.md)
