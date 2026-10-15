package bit_test

import (
	"errors"
	"fmt"

	"github.com/USA-RedDragon/dmrgo/v2/bit"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

// ExampleUnpack converts packed bytes to bits, most significant bit first,
// and back.
func ExampleUnpack() {
	bits := bit.Unpack([]byte{0xA5})
	fmt.Printf("%v %d\n", bits, bit.Uint(bits[:4]))
	fmt.Printf("%x\n", bit.Pack(bits))
	// Output:
	// [1 0 1 0 0 1 0 1] 10
	// a5
}

// ExampleFieldRangeError finds the field that an Encode method rejected.
func ExampleFieldRangeError() {
	lc := pdu.FullLinkControl{
		BurstDataType: enums.DataTypeVoiceLCHeader,
		FLCO:          enums.FLCOGroupVoiceChannelUser,
		GroupVoice:    &pdu.FLCGroupVoice{GroupAddress: 1 << 24},
	}
	_, err := lc.Encode()
	var rangeErr *bit.FieldRangeError
	if errors.As(err, &rangeErr) {
		fmt.Println(rangeErr.Field, rangeErr.Bits)
	}
	// Output:
	// GroupAddress 24
}
