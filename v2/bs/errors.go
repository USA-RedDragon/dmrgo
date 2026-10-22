package bs

import (
	"errors"

	"github.com/USA-RedDragon/dmrgo/v2"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
)

// ErrSlot is returned for a slot other than enums.Slot1 or enums.Slot2.
// Match it with errors.Is.
var ErrSlot = errors.New("bs: slot must be 1 or 2")

// CheckSlot returns ErrSlot for a slot other than enums.Slot1 or
// enums.Slot2, the slots that Repeater.Inbound and Repeater.Lost drop.
func CheckSlot(s enums.Slot) error {
	if s != enums.Slot1 && s != enums.Slot2 {
		return ErrSlot
	}
	return nil
}

var (
	errMode        error = &dmrgo.ConfigError{Type: "bs", Field: "Mode"}
	errColorCode   error = &dmrgo.ConfigError{Type: "bs", Field: "ColorCode"}
	errActivation  error = &dmrgo.ConfigError{Type: "bs", Field: "Activation"}
	errPending     error = &dmrgo.ConfigError{Type: "bs", Field: "Pending"}
	errHeaders     error = &dmrgo.ConfigError{Type: "bs", Field: "Headers"}
	errDataQueue   error = &dmrgo.ConfigError{Type: "bs", Field: "DataQueue"}
	errTerminators error = &dmrgo.ConfigError{Type: "bs", Field: "Terminators"}
	errChanAuth    error = &dmrgo.ConfigError{Type: "bs", Field: "ChannelAuth"}
)
