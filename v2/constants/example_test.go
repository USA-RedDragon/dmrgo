package constants_test

import (
	"fmt"

	"github.com/USA-RedDragon/dmrgo/v2/constants"
)

// Example prints two constants of the Annexes: a timer and a retry limit.
func Example() {
	fmt.Println(constants.BeaconInterval, constants.NRetryLimit)
	// Output:
	// 4m30s 8
}
