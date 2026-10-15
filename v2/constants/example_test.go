package constants_test

import (
	"fmt"

	"github.com/USA-RedDragon/dmrgo/v2/constants"
)

// Example shows that the timers are time.Duration values and the counters
// are plain integers.
func Example() {
	fmt.Println(constants.BeaconInterval, constants.NRetryLimit)
	// Output:
	// 4m30s 8
}
