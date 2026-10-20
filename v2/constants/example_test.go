package constants_test

import (
	"fmt"

	"github.com/USA-RedDragon/dmrgo/v2/constants"
)

func Example() {
	fmt.Println(constants.BeaconInterval, constants.NRetryLimit)
	// Output:
	// 4m30s 8
}
