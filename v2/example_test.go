package dmrgo_test

import (
	"errors"
	"fmt"

	"github.com/USA-RedDragon/dmrgo/v2"
	"github.com/USA-RedDragon/dmrgo/v2/data"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
	"github.com/USA-RedDragon/dmrgo/v2/voice"
)

// Example decodes a Voice LC Header burst, checks its FEC, reads the
// addresses of its Full LC and encodes it back to the same 33 bytes.
func Example() {
	raw := [33]byte{
		68, 75, 3, 135, 36, 66, 12, 240, 21, 240,
		12, 161, 196, 109, 255, 87, 215, 93, 245, 222,
		49, 168, 53, 24, 63, 48, 61, 97, 56, 82,
		151, 134, 91,
	}
	burst, err := layer2.NewBurstFromBytes(raw)
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println(burst.SlotType.DataType, burst.FEC.Aggregate().Err())
	if lc, ok := burst.Data.(*pdu.FullLinkControl); ok {
		target, source, _ := lc.Addresses()
		fmt.Println("call from", source, "to", target)
	}
	encoded, err := burst.Encode()
	fmt.Println(encoded == raw, err)
	// Output:
	// Voice LC Header <nil>
	// call from 3191868 to 9990
	// true <nil>
}

// ExampleFieldRangeError matches the error of a group address that does not
// fit in its 24 bits.
func ExampleFieldRangeError() {
	lc := pdu.FullLinkControl{BurstDataType: enums.DataTypeVoiceLCHeader}
	err := lc.SetGroupVoice(pdu.FLCGroupVoice{GroupAddress: 1 << 24})
	var rangeErr *dmrgo.FieldRangeError
	if errors.As(err, &rangeErr) {
		fmt.Println(rangeErr.Field, rangeErr.Bits)
	}
	// Output:
	// GroupAddress 24
}

// ExampleConfigError matches the error of a color code over 15 in a
// constructor config.
func ExampleConfigError() {
	_, err := voice.NewTransmitter(&voice.TransmitterConfig{Slot: enums.Slot1, ColorCode: 16})
	var configErr *dmrgo.ConfigError
	if errors.As(err, &configErr) {
		fmt.Println(configErr.Type, configErr.Field)
	}
	// Output:
	// voice ColorCode
}

// ExampleErrZeroValue sends on a zero data.Sender and on one that its
// constructor made. Only the constructor sets up the machine.
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
	// Output:
	// true
	// <nil>
}

// ExampleErrNilReceiver calls methods on a nil data.Sender. Send returns
// ErrNilReceiver, and Tick returns dst unchanged.
func ExampleErrNilReceiver() {
	var tx *data.Sender
	err := tx.Send(0, &data.Message{})
	fmt.Println(errors.Is(err, dmrgo.ErrNilReceiver))
	outputs := tx.Tick(0, make([]data.Output, 0, data.MaxOutputs))
	fmt.Println(len(outputs))
	// Output:
	// true
	// 0
}
