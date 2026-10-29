# voice Jitter

This page covers the voice Jitter, the network jitter buffer that puts the packets of one slot back in order before a Receiver.

## What it does

A [Jitter](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/voice#Jitter) takes the voice packets of one slot from a network and plays them in the order of their sequence numbers, a fixed delay after they were sent, one per 60 ms burst period. A packet that comes late or out of order within the delay plays in its place, so network jitter adds no fills and reordering loses no voice. A packet that comes after its time to play is dropped as late, and a copy is dropped as a duplicate. The Jitter follows a new stream after the one that plays, and starts its playout again after a stall. It takes the time only from the now of each call and from the Hint of each packet, and does no I/O. No ETSI clause covers it: it works as the Jitter setting of the DMR Network section of MMDVMHost does.

## When to use it

Put a Jitter in front of a [Receiver](voice-receiver.md) for each slot of a network stream: an MMDVM network protocol or HBRP master or peer, IPSC, or RTP. A Receiver counts a packet that comes after a later one as late and fills the gap, so it needs the packets in order. Do not use a Jitter for RF bursts, which come in order, or for data, CSBK and MBC bursts: the Jitter delays every packet by the playout delay, which only voice needs. Pass those around it, as the bs Repeater does.

## Config

Pass a [JitterConfig](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/voice#JitterConfig) to [NewJitter](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/voice#NewJitter). A nil config gives the default delay. A longer delay rides out more jitter and adds more latency. When a [Transmitter](voice-transmitter.md) relays the packets with a Lead, set the same Lead here, so each packet is there before the Transmitter sends its burst.

<!-- dmrgo:config pkg=voice type=JitterConfig -->
| Field | Type | Default | Description |
|---|---|---|---|
| `Delay` | `time.Duration` | `DefaultJitter` (360ms) | Delay is the playout delay: the time from the arrival of the packet that came first for its place in the stream to the Pop that returns it. A packet that comes up to Delay later than that, in any order, plays in its place. 0 means DefaultJitter, and a negative value turns the buffer off: Push then returns JitterBypass for every packet. At most MaxJitter. |
| `Lead` | `time.Duration` | zero value | Lead is the time from a Pop to the Due time of the packet it returns, for a Transmitter that relays the call with this TransmitterConfig.Lead, so that the packet is there before the Transmitter sends its burst. It must not be negative. |
<!-- /dmrgo:config -->

## States and transitions

The Jitter has no State type. It holds up to two streams: the one that plays and the next one, which waits. The package doc of [Jitter](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/voice#Jitter) gives the full rules. In short:

- Each packet of a stream gives the time of the first sequence number: its arrival less 60 ms for each sequence number it is ahead. The earliest of these sets the clock of the stream. The lowest sequence number plays one delay after that time, and each later one 60 ms after the one before.
- A missing sequence number is skipped when a later packet plays. The Receiver counts it from the sequence number and fills it.
- A packet so early that the buffer cannot hold it moves the playout earlier and counts a resync. A stall longer than the delay empties the buffer, and the next packets start the playout again on the same 60 ms grid.
- A new stream, another source or another Hint.Stream, waits behind the one that plays. A third stream passes the Jitter: [Push](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/voice#Jitter.Push) returns bypass for it.
- After a Terminator with LC or a strong end flag has played, a Voice LC Header or PI Header starts a new stream, for transports whose sequence number starts again with each call.
- The sequence number wraps at Hint.SeqBits bits: 8 for DMRD packets, 0 or 16 for RTP.

## Inputs and outputs

Inputs:

- [Push](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/voice#Jitter.Push) takes a packet: the raw burst, its [layer2.Hint](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/layer2#Hint) with the sequence number, and the source, such as the peer ID. A nil burst is a transport frame without a burst, which plays in its place. Push returns a [JitterResult](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/voice#JitterResult). Handle a bypass packet at once.
- [Pop](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/voice#Jitter.Pop) fills a [JitterPacket](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/voice#JitterPacket) with the next packet when its time has come. Call it at the time [Deadline](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/voice#Jitter.Deadline) returns. Each Pop returns at most one packet, so a late caller pops until Deadline is in the future.
- [Reset](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/voice#Jitter.Reset) drops every packet and the counts.

Outputs:

- The popped packet. Give it to the Receiver, or to a bs Repeater, with its Due time as now.
- [Len](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/voice#Jitter.Len) returns the number of packets that wait.
- [Stats](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/voice#Jitter.Stats) returns the [JitterStats](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/voice#JitterStats) counts of late, duplicate and overflow packets and of resyncs.

## Timers

- Playout delay: the default is <!-- dmrgo:const pkg=voice name=DefaultJitter -->`360ms`<!-- /dmrgo:const -->, the default Jitter of MMDVMHost, and the longest is <!-- dmrgo:const pkg=voice name=MaxJitter -->`2s`<!-- /dmrgo:const -->. A negative delay turns the buffer off.
- Burst period: one packet each 60 ms after the first.
- Lead: Deadline and Pop work this long before the Due time of each packet.

A now that jumps back by more than the delay, the Lead and one second is a clock reset, and the Jitter moves its times by the jump.

## Worked example

This example pushes four packets of one stream out of order, and a copy of the last one. It then pops each packet at the time that Deadline returns.

<!-- dmrgo:code file=v2/voice/example_test.go symbol=ExampleJitter -->
```go
func ExampleJitter() {
	j, err := voice.NewJitter(nil)
	if err != nil {
		fmt.Println(err)
		return
	}
	arrivals := []struct {
		at  time.Duration
		seq uint16
	}{
		{0, 0},
		{120 * time.Millisecond, 2},
		{150 * time.Millisecond, 1},
		{180 * time.Millisecond, 3},
		{190 * time.Millisecond, 3},
	}
	for _, a := range arrivals {
		var raw layer2.RawBurst
		h := layer2.Hint{Slot: enums.Slot1, Kind: layer2.KindVoice, Seq: a.seq, HasSeq: true, SeqBits: 8}
		fmt.Println("push", a.seq, "at", a.at, j.Push(a.at, &raw, h, 3120001))
	}
	var p voice.JitterPacket
	for {
		at, ok := j.Deadline()
		if !ok {
			break
		}
		if j.Pop(at, &p) {
			fmt.Println("pop", p.Hint.Seq, "due", p.Due)
		}
	}
	fmt.Printf("%+v\n", j.Stats())
}
```
<!-- /dmrgo:code -->

Output:

<!-- dmrgo:output test=voice.ExampleJitter -->
```text
push 0 at 0s Buffered
push 2 at 120ms Buffered
push 1 at 150ms Buffered
push 3 at 180ms Buffered
push 3 at 190ms Duplicate
pop 0 due 360ms
pop 1 due 420ms
pop 2 due 480ms
pop 3 due 540ms
{Late:0 Duplicates:1 Overflows:0 Resyncs:0}
```
<!-- /dmrgo:output -->

The packets play in the order of their sequence numbers, one burst period apart, starting one playout delay after the first packet arrived. The copy is dropped and counted.

## Errors

Push and Pop return no errors: Push reports what it did in its JitterResult. NewJitter returns a configuration error for a negative Lead or a delay above the longest.

<!-- dmrgo:errors pkg=voice type=Jitter -->
The doc comments of [`Jitter`](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/voice#Jitter) and its methods name no error.
<!-- /dmrgo:errors -->

## Related machines and guides

Machines:

- [voice Receiver](voice-receiver.md): takes the packets that Pop returns.
- [voice Transmitter](voice-transmitter.md): relays the call with the same Lead.
- [dmrdsession Peer](dmrdsession-peer.md), [dmrdsession Master](dmrdsession-master.md), [ipscsession Peer](ipscsession-peer.md) and [ipscsession Master](ipscsession-master.md): the network sessions whose voice packets go through a Jitter.
- bs Repeater (docs/machines/bs-repeater.md): runs a Jitter for each slot.

Guides:

- Connect to a network (MMDVM network protocol, HBRP, IPSC, Hytera)
- Rewrite and relay a voice call
