# I/O-free services

This page describes the method set that every state machine of dmrgo shares: each method takes the time and an input, and returns outputs, with no I/O, clock or goroutine of its own.

## The rule

A state machine of the packages [bs](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/bs), [csbk](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/csbk), [data](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/data), [trunk](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/trunk) and [voice](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/voice) does no I/O. It does not read a socket, a serial port or a clock, and it starts no goroutine and no timer. The session machines of [transport/dmrd/dmrdsession](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/transport/dmrd/dmrdsession) and [transport/ipsc/ipscsession](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/transport/ipsc/ipscsession) follow the same rule for packets in place of bursts, with a method set of their own. Your program does the I/O and keeps the clock:

1. Read a burst, a packet or a frame from your source.
2. Give it to the machine, with the time now.
3. Send the outputs that the machine returns.
4. At the time that `Deadline` returns, call `Tick`.

So a machine runs the same in a test, in a replay of a capture, on a server and on a microcontroller. A test can drive time forward in one step, as the examples of these pages do.

## The method set

Each method takes `now`, the time on a monotonic clock that you pick, and appends its outputs to a slice `dst` that you give it:

- `Burst(now, raw, h, dst)` decodes the [layer2.RawBurst](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/layer2#RawBurst) `raw` with the data in the [layer2.Hint](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/layer2#Hint) `h`. A nil `raw` is a transport frame without a burst: only `h` applies.
- `Decoded(now, b, h, dst)` takes a burst that you decoded once, so that one burst can go to several machines.
- `Lost(now, dst)` reports the loss of the signal.
- `Tick(now, dst)` runs what is due at `now`. `Deadline` returns when the next `Tick` is due. A `Tick` before `Deadline` does nothing.
- `State` returns the state, and `Reset` ends the work without an output.

The time is a `time.Duration` from any start that you pick, such as the start of the program. Use one monotonic clock for all the calls to one machine.

Three machines differ. [bs.Repeater](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/bs#Repeater) runs both slots: its `Inbound`, `Decoded` and `Lost` also take the source of the burst, and `Lost` the slot. [trunk.Follower](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/trunk#Follower), [trunk.MS](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/trunk#MS) and [trunk.CapacityPlusFollower](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/trunk#CapacityPlusFollower) also take the CACH in `Burst` and `Decoded`. [trunk.TSCC](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/trunk#TSCC) runs a whole site and has only `Inbound`, which also takes the carrier, `Tick` and `Deadline`.

## Drive the clock

This example sends a confirmed message to no one. Each loop calls `Deadline` and then `Tick` at that time, so the example does not wait for real time to pass. The Sender tries again at each timeout, and then gives up:

<!-- dmrgo:code file=v2/data/sender_example_test.go symbol=driveClock -->
```go
func driveClock(tx *data.Sender) {
	outputs := make([]data.Output, 0, data.MaxOutputs)
	for {
		at, ok := tx.Deadline()
		if !ok {
			return
		}
		for _, o := range tx.Tick(at, outputs[:0]) {
			if icmp, ok := o.ICMP(); ok {
				fmt.Printf("%v after %d tries: ICMP %v (type %d, code %d)\n", o.Result, o.Try, icmp, icmp.Type(), icmp.Code())
			}
		}
	}
}
```
<!-- /dmrgo:code -->

<!-- dmrgo:output test=data.ExampleOutput_ICMP -->
```text
RetryLimit after 2 tries: ICMP Host Unreachable (type 3, code 1)
```
<!-- /dmrgo:output -->

In a real program, set a timer to the time that `Deadline` returns, and call `Tick` when it fires or when an input comes first. Call `Deadline` again after each call, because each input can change it.

## Outputs with a Due time

A machine that sends bursts returns each one with its `Due` time, the start of its slot on air. A machine whose `Tick` sends bursts at their `Due` time has a `Lead` in its config: `Tick` returns a burst that long before its `Due` time, so that you can fill a transmit FIFO ahead, and `Deadline` is `Due` minus `Lead`.

The hint of each burst that a machine sends carries its slot. The `Call` of the output carries the addresses and the group flag of the call, as a gateway puts them in a packet: [dmrd.Data.SetCallHint](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/transport/dmrd#Data.SetCallHint) and [hytera.Frame.SetCallHint](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/transport/hytera#Frame.SetCallHint) take both.

## Make a machine with its constructor

A zero machine does nothing. Each method that starts work, such as `Start`, `Request` or `Send`, returns [dmrgo.ErrZeroValue](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2#ErrZeroValue) on a zero machine, and the other methods do nothing. Make each machine with its constructor, such as [data.NewSender](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/data#NewSender):

<!-- dmrgo:code file=v2/example_test.go symbol=ExampleErrZeroValue -->
```go
func ExampleErrZeroValue() {
	m := &data.Message{Payload: []byte("hello")}
	if err := m.Header.SetUnconfirmed(pdu.UnconfirmedDataHeader{LLIDDestination: 3120002, LLIDSource: 3120001}); err != nil {
		fmt.Println(err)
		return
	}
	var zero data.Sender
	err := zero.Send(0, m)
	fmt.Println(errors.Is(err, dmrgo.ErrZeroValue))
	tx, err := data.NewSender(&data.SenderConfig{Slot: enums.Slot1})
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println(tx.Send(0, m))
}
```
<!-- /dmrgo:code -->

<!-- dmrgo:output test=.ExampleErrZeroValue -->
```text
true
<nil>
```
<!-- /dmrgo:output -->

The constructor checks the config and returns a [*dmrgo.ConfigError](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2#ConfigError) for a field out of range. After the constructor, no method of the machine allocates when `dst` has room. See [Allocation](../allocation.md).

## Concurrency and ownership

- A machine is not safe for concurrent use. Use each machine from one goroutine at a time.
- Do not copy a machine after its first use. It holds pointers into its own storage, and `go vet` reports a copy.
- A constructor or method keeps a copy of each slice that it keeps, such as a password or a payload. It keeps a pointer, func or interface of a config as it is, such as a `Scratch` burst, a `trunk.Authenticator` or a `dvsi.Port`.
- The slice that a method returns is `dst` with the outputs appended. Some fields of an output point into the storage of the machine and are valid until its next method call, such as `Message.Payload` of a [data.Event](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/data#Event). The doc of each output type names them. Copy such a field to keep it.

The [state machine pages](../machines/voice-receiver.md) give the states, the config, the timers and the errors of each machine.

Next: [Errors and the never-panic rule](errors.md).
