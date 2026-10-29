# Use an MMDVM modem

This guide shows how to talk to an MMDVM or MMDVM_HS modem over its serial port, as MMDVMHost does: read its version, send its config, switch it to DMR, and pass bursts in both directions.

## Goal

Run the host side of the serial protocol of an MMDVM modem, so that your program receives the DMR bursts that the modem hears and sends bursts for it to transmit.

## What you need

- Go and a checkout of dmrgo. The example runs offline: a simulated modem in memory stands in for the serial port.
- The package [transport/mmdvm](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/transport/mmdvm), and [layer2](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/layer2) for the bursts.
- For a real modem, a serial port package of your choice. Package mmdvm does no I/O: it builds and reads the octets that go over the port.

The serial protocol of a modem is not the MMDVM network protocol. The modem protocol links a host to its radio hardware; the network protocol links a host to a master, and the guide [Connect to a network with the MMDVM network protocol or HBRP](connect-dmrd.md) covers it.

Run the example from the top of the repository:

```sh
go test -C v2 -run '^Example$' -v ./transport/mmdvm
```

## Steps

### 1. Send a frame to the modem

Every message on the port is a frame: a start octet, a length, a command and a payload. [mmdvm.AppendFrame](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/transport/mmdvm#AppendFrame) builds one around a payload. Write the frame to the serial port.

<!-- dmrgo:code file=v2/transport/mmdvm/session_example_test.go lines=17-25 -->
```go
var fromModem []byte
toModem := func(c mmdvm.Command, payload []byte) {
	frame, err := mmdvm.AppendFrame(nil, c, payload)
	if err != nil {
		fmt.Println(err)
		return
	}
	fromModem = modem(fromModem, frame)
}
```
<!-- /dmrgo:code -->

### 2. Open the session

Request the version first. The reply gives the protocol version of the firmware: 1 for MMDVM_HS and 2 for MMDVM. [mmdvm.DecodeVersion](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/transport/mmdvm#DecodeVersion) reads it, and `Capabilities.Has` shows whether the modem has DMR. Then send the config: [ConfigV2](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/transport/mmdvm#ConfigV2) for protocol 2 and [ConfigV1](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/transport/mmdvm#ConfigV1) for protocol 1. It enables the modes and sets the color code, the levels and the delays. [SetMode](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/transport/mmdvm#SetMode) switches the modem to DMR. The modem answers the config and the mode with an Ack, or with a Nak and a reason code.

<!-- dmrgo:code file=v2/transport/mmdvm/session_example_test.go lines=27-33 -->
```go
toModem(mmdvm.CommandGetVersion, nil)
cfg := mmdvm.ConfigV2{DMR: true, DMRColorCode: 7, State: mmdvm.ModeIdle, RXLevel: 50, DMRTXLevel: 50}
payload, _ := cfg.AppendBinary(nil)
toModem(mmdvm.CommandSetConfig, payload)
mode := mmdvm.SetMode{Mode: mmdvm.ModeDMR}
payload, _ = mode.AppendBinary(nil)
toModem(mmdvm.CommandSetMode, payload)
```
<!-- /dmrgo:code -->

### 3. Read the frames from the modem

Append the octets that you read from the port to a buffer, and call [mmdvm.Next](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/transport/mmdvm#Next) on it. `NextFrame` returns a whole frame and its length; drop that many octets. `NextMore` means the buffer ends inside a frame: read more. `NextSkip` gives the number of octets to drop that do not start a frame.

A DMR burst comes as a `CommandDMRData1` or `CommandDMRData2` frame. [mmdvm.DecodeDMRData](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/transport/mmdvm#DecodeDMRData) decodes it, and [DMRData.Hint](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/transport/mmdvm#DMRData.Hint) gives the `layer2.Hint` for the voice and data machines. A `CommandDMRLost1` or `CommandDMRLost2` frame shows that the modem lost the signal of the slot.

<!-- dmrgo:code file=v2/transport/mmdvm/session_example_test.go lines=35-67 -->
```go
for len(fromModem) > 0 {
	f, n, r := mmdvm.Next(fromModem)
	if r == mmdvm.NextMore {
		break
	}
	fromModem = fromModem[n:]
	if r == mmdvm.NextSkip {
		continue
	}
	switch f.Command {
	case mmdvm.CommandGetVersion:
		v, _ := mmdvm.DecodeVersion(f.Payload)
		fmt.Printf("version: protocol %d, DMR %v, %s\n", v.Protocol, v.Capabilities.Has(mmdvm.CapDMR), v.Description)
	case mmdvm.CommandAck:
		a, _ := mmdvm.DecodeAck(f.Payload)
		fmt.Println("ack:", a.Command)
	case mmdvm.CommandDMRData1, mmdvm.CommandDMRData2:
		d, err := mmdvm.DecodeDMRData(f)
		if err != nil {
			fmt.Println(err)
			continue
		}
		h := d.Hint()
		b, err := layer2.NewBurstFromBytes(d.Burst)
		if err != nil {
			fmt.Println(err)
			continue
		}
		fmt.Println("burst:", h.Slot, h.Kind, h.DataType, "color code", b.SlotType.ColorCode)
	default:
		fmt.Println("frame:", f.Command)
	}
}
```
<!-- /dmrgo:code -->

### 4. Send bursts to the modem

Put each burst that a machine returns in a [DMRData](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/transport/mmdvm#DMRData). [DMRData.SetHint](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/transport/mmdvm#DMRData.SetHint) sets the slot and the control octet from the `Hint` of the output, and `DMRData.Command` gives the command of the slot. Wrap the payload of `DMRData.AppendBinary` with `AppendFrame`. On a duplex modem, [DMRStart](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/transport/mmdvm#DMRStart) starts and stops the transmitter, and [DMRShortLC](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/transport/mmdvm#DMRShortLC) sets the Short LC of the CACH.

The simulated modem of the example builds its frames the same way:

<!-- dmrgo:code file=v2/transport/mmdvm/session_example_test.go lines=80-97 -->
```go
switch f.Command {
case mmdvm.CommandGetVersion:
	v := mmdvm.Version{Protocol: 2, Capabilities: mmdvm.CapDMR, CPU: mmdvm.CPUST, Description: []byte("MMDVM simulated")}
	payload, _ := v.AppendBinary(nil)
	out, _ = mmdvm.AppendFrame(out, mmdvm.CommandGetVersion, payload)
case mmdvm.CommandSetConfig, mmdvm.CommandSetMode:
	a := mmdvm.Ack{Command: f.Command}
	payload, _ := a.AppendBinary(nil)
	out, _ = mmdvm.AppendFrame(out, mmdvm.CommandAck, payload)
	if f.Command == mmdvm.CommandSetMode {
		raw, _ := hex.DecodeString("20580b4015ec0f6871000d005c4d5d7f77fd757361a416203d5235242181850277")
		d := mmdvm.DMRData{Slot: enums.Slot2, Control: mmdvm.ControlDataSync | mmdvm.Control(enums.DataTypeVoiceLCHeader)}
		copy(d.Burst[:], raw)
		payload, _ = d.AppendBinary(nil)
		out, _ = mmdvm.AppendFrame(out, d.Command(), payload)
	}
default:
}
```
<!-- /dmrgo:code -->

## Output

The version, the two Acks and the received burst:

<!-- dmrgo:output test=mmdvm.Example -->
```text
version: protocol 2, DMR true, MMDVM simulated
ack: SetConfig
ack: SetMode
burst: TS2 Data Voice LC Header color code 7
```
<!-- /dmrgo:output -->

The frames that MMDVMHost sends to a modem, from a capture:

<!-- dmrgo:output test=mmdvm.ExampleNext -->
```text
GetStatus 0
DMRData2 34
TS2 MS-sourced data Voice LC Header
```
<!-- /dmrgo:output -->

## How it works

A frame starts with the octet 0xE0, then a length octet that counts the whole frame, then the command. A length of 0 is the long form, for frames of 256 octets or more: the next octet plus 255 is the length. A DMR data frame holds a control octet and the 33-octet burst, and from a modem built to send it, the RSSI after the burst. The control octet shows how the modem found the burst: a data SYNC with its data type, the voice SYNC of burst A, or the position of voice bursts B to F. MMDVMHost sends 0x00 toward the modem.

The decode functions never allocate and never panic. A field value that the firmware refuses, such as a color code above 15, gives a `*dmrgo.FieldRangeError`. (MMDVMHost 9384d68 Modem.cpp; MMDVM 046e1df SerialPort.cpp, DMRSlotRX.cpp, DMRIdleRX.cpp and DMRTX.cpp; MMDVM_HS bf66faf SerialPort.cpp)

The package encodes and decodes both roles, so a program can also act as the modem, as the example does. The bursts go to and from the service machines: [bs Repeater](../machines/bs-repeater.md), [voice Receiver](../machines/voice-receiver.md), [voice Transmitter](../machines/voice-transmitter.md), [data Receiver](../machines/data-receiver.md) and [data Sender](../machines/data-sender.md).

## Related guides

- [Clean up bursts for a hotspot](clean-up-bursts.md)
- [Connect to a network with the MMDVM network protocol or HBRP](connect-dmrd.md)
- [Rewrite and relay a voice call](relay-voice-call.md)
- [Receive with an SDR or an FM radio](receive-sdr-fm.md)
