package enums_test

import (
	"errors"
	"fmt"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
)

func ExampleNearestSyncPattern() {
	field := [6]byte{0x75, 0x5F, 0xD7, 0xDF, 0x75, 0xF6}
	pattern, distance := enums.NearestSyncPattern(field)
	fmt.Println(pattern, distance)
	// Output:
	// BS-sourced voice 1
}

func ExampleColorCodeError() {
	_, err := enums.NewColorCode(16)
	var ccErr *enums.ColorCodeError
	if errors.As(err, &ccErr) {
		fmt.Println(ccErr.Value)
	}
	// Output:
	// 16
}
