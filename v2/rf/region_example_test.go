package rf_test

import (
	"errors"
	"fmt"

	"github.com/USA-RedDragon/dmrgo/v2/rf"
)

// ExampleRegionError shows a transmitter setup that the limits refuse: no
// measurement keeps the limits with I/Q at 48 kHz.
func ExampleRegionError() {
	_, err := rf.NewTransmitter(&rf.TxConfig{Rate: 48000, Region: rf.RegionUS, Service: rf.ServiceLandMobile, Power: 30})
	var re *rf.RegionError
	if errors.As(err, &re) {
		fmt.Println(re.Field+":", re.Reason)
	}
	// Output:
	// TxConfig.Rate: no measurement keeps the limits at this rate
}
