package enums_test

import (
	"errors"
	"fmt"

	"github.com/USA-RedDragon/dmrgo/v2"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
)

func ExampleNearestSyncPattern() {
	field := [6]byte{0x75, 0x5F, 0xD7, 0xDF, 0x75, 0xF6}
	pattern, distance := enums.NearestSyncPattern(field)
	fmt.Println(pattern, distance)
	// Output:
	// BS-sourced voice 1
}

// ExampleNewColorCode shows the error of a value that does not fit in 4
// bits.
func ExampleNewColorCode() {
	_, err := enums.NewColorCode(16)
	var rangeErr *dmrgo.FieldRangeError
	if errors.As(err, &rangeErr) {
		fmt.Println(rangeErr.Type, rangeErr.Field, rangeErr.Bits)
	}
	// Output:
	// enums ColorCode 4
}
