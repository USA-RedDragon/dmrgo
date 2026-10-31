# Errors and the never-panic rule

This page describes how dmrgo reports bad input: it never panics, it returns static error values that you match with errors.Is and errors.As, and it reports an FEC failure in a result beside the decoded value.

## No panic on any input

No function or method of the library panics on its input. A burst of random bytes, a packet that a network cut short, a config with every field out of range, and a nil pointer each give an error or a zero result. A program that takes input from the air or from a network can give it to the library as it comes.

A method called on a nil pointer does nothing:

- it returns [dmrgo.ErrNilReceiver](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2#ErrNilReceiver) where it returns an error;
- it returns `dst` unchanged where it appends to `dst`;
- it returns `"<nil>"` from `String` and `Error`;
- it returns zero values otherwise.

<!-- dmrgo:code file=v2/example_test.go symbol=ExampleErrNilReceiver -->
```go
func ExampleErrNilReceiver() {
	var tx *data.Sender
	err := tx.Send(0, &data.Message{})
	fmt.Println(errors.Is(err, dmrgo.ErrNilReceiver))
	outputs := tx.Tick(0, make([]data.Output, 0, data.MaxOutputs))
	fmt.Println(len(outputs))
}
```
<!-- /dmrgo:code -->

<!-- dmrgo:output test=.ExampleErrNilReceiver -->
```text
true
0
```
<!-- /dmrgo:output -->

The fuzz tests of the packages feed random input to the decoders, the encoders and the state machines, and fail on a panic. The C API, libdmrgo, also returns a status code for each failure. See [C API](../targets/c.md).

## Match errors with errors.Is

Each error that the library returns is a static value, or wraps one. Match it with `errors.Is`. Because the values are static, a call that fails does not allocate. Only the `Error` method allocates, to build the message text.

The root package holds the errors that every package can return:

- [dmrgo.ErrNilReceiver](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2#ErrNilReceiver): a method was called on a nil pointer.
- [dmrgo.ErrNilArgument](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2#ErrNilArgument): a function or method got a nil pointer, func or interface argument that it needs.
- [dmrgo.ErrZeroValue](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2#ErrZeroValue): a method that starts work was called on a zero state machine. See [I/O-free services](services.md).

Each package holds its own errors, such as [data.ErrBusy](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/data#ErrBusy) and [layer2.ErrColorCodeMismatch](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/layer2#ErrColorCodeMismatch). The "Errors" section of the [package documentation](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2) lists every error of every package, with the calls that return it. The doc comment of each function gives the errors that it returns. For example, the methods of a [data.Sender](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/data#Sender) return these errors:

<!-- dmrgo:errors pkg=data type=Sender -->
| Error | Returned by | Meaning |
|---|---|---|
| [`data.ErrBusy`](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/data#ErrBusy) | `Sender.Send` | ErrBusy is returned by Sender.Send while a message is in progress. |
| [`dmrgo.ConfigError`](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2#ConfigError) | `Sender.Send` | ConfigError is the error of a constructor, and of a machine method that takes a config, option or argument field, for a field with a value out of range. |
| [`dmrgo.ErrNilArgument`](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2#ErrNilArgument) | `Sender.Send` | ErrNilArgument is returned by a function or method of the library that returns an error when a pointer, func or interface argument it needs is nil. |
| [`dmrgo.ErrZeroValue`](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2#ErrZeroValue) | `Sender.Send` | ErrZeroValue is returned by a method of a state machine that starts work, such as Start, Request or Send, when the machine is a zero value and not made with its constructor. |
<!-- /dmrgo:errors -->

## Match error types with errors.As

Two error types carry the field that failed. Match them with `errors.As`:

- [*dmrgo.FieldRangeError](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2#FieldRangeError): an encoder, setter, getter or function got a value that does not fit in its bits, or a field with a padding bit set. `Type` and `Field` name the field, and `Bits` gives its width.
- [*dmrgo.ConfigError](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2#ConfigError): a constructor or a machine method got a config, option or argument field out of range. `Type` names the package and `Field` the field.

<!-- dmrgo:code file=v2/example_test.go symbol=ExampleFieldRangeError -->
```go
func ExampleFieldRangeError() {
	lc := pdu.FullLinkControl{BurstDataType: enums.DataTypeVoiceLCHeader}
	err := lc.SetGroupVoice(pdu.FLCGroupVoice{GroupAddress: 1 << 24})
	var rangeErr *dmrgo.FieldRangeError
	if errors.As(err, &rangeErr) {
		fmt.Println(rangeErr.Field, rangeErr.Bits)
	}
}
```
<!-- /dmrgo:code -->

<!-- dmrgo:output test=.ExampleFieldRangeError -->
```text
GroupAddress 24
```
<!-- /dmrgo:output -->

<!-- dmrgo:code file=v2/example_test.go symbol=ExampleConfigError -->
```go
func ExampleConfigError() {
	_, err := voice.NewTransmitter(&voice.TransmitterConfig{Slot: enums.Slot1, ColorCode: 16})
	var configErr *dmrgo.ConfigError
	if errors.As(err, &configErr) {
		fmt.Println(configErr.Type, configErr.Field)
	}
}
```
<!-- /dmrgo:code -->

<!-- dmrgo:output test=.ExampleConfigError -->
```text
voice ColorCode
```
<!-- /dmrgo:output -->

A setter that returns a `*FieldRangeError` changes nothing.

## FEC failures go in the result

Bit errors on air are normal. A decoder corrects the errors that its FEC can correct, and reports the rest in an FEC result, a [fec.Result](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/fec#Result), beside the decoded value. The decode itself does not fail. For example, [layer2.NewBurstFromBytes](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/layer2#NewBurstFromBytes) returns no error for a burst whose CRC fails. `burst.FEC` reports the failure, and `burst.Data` still holds the decoded PDU. `Err` on a result returns [fec.ErrUncorrectable](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/fec#ErrUncorrectable) for an uncorrectable result. This example prints the failure of a burst and skips the burst:

<!-- dmrgo:code file=v2/layer2/example_test.go symbol=ExampleNewBurstFromBytes_capture -->
```go
func ExampleNewBurstFromBytes_capture() {
	capture, err := os.ReadFile("testdata/parrot_kerchunk.bin")
	if err != nil {
		fmt.Println(err)
		return
	}
	voiceBursts := 0
	for off := 0; off+33 <= len(capture); off += 33 {
		burst, err := layer2.NewBurstFromBytes([33]byte(capture[off : off+33]))
		if err != nil {
			fmt.Println(err)
			return
		}
		if err := burst.FEC.Aggregate().Err(); err != nil {
			fmt.Println("burst", off/33, err)
			continue
		}
		switch burst.Kind {
		case layer2.KindVoice:
			voiceBursts++
		case layer2.KindData:
			fmt.Print(burst.SlotType.DataType)
			if lc, ok := burst.Data.(*pdu.FullLinkControl); ok {
				target, source, _ := lc.Addresses()
				fmt.Printf(": call from %d to %d", source, target)
			}
			fmt.Println()
		case layer2.KindUnknown, layer2.KindReverseChannel:
		}
	}
	fmt.Println(voiceBursts, "voice bursts")
}
```
<!-- /dmrgo:code -->

You pick what to do with a failed burst: drop it, count it, or use what it holds.

Some calls return an error together with what they decoded. [layer2.VoiceReceiver](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/layer2#VoiceReceiver) and [packet.Assembler](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/layer2/packet#Assembler) return [layer2.ErrEmbeddedLCUncorrectable](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/layer2#ErrEmbeddedLCUncorrectable), [packet.ErrMessageCRC](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/layer2/packet#ErrMessageCRC) and [packet.ErrBlocksMissing](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/layer2/packet#ErrBlocksMissing) with the decoded value. Each of these wraps `fec.ErrUncorrectable`, so one `errors.Is` check finds every FEC failure.

Next: [Allocation](../allocation.md).
