package trunk

import (
	"errors"

	"github.com/USA-RedDragon/dmrgo/v2"
)

// Errors of the tables and machines. Match them with errors.Is.

const pkgName = "trunk"

var (
	// ErrChannelNumber is returned for a channel number that names no
	// channel: CHNULL, ChannelAbsolute or over 12 bits.
	ErrChannelNumber = errors.New("trunk: channel number out of range")
	// ErrPlanFull is returned when a ChannelPlan or HuntList has no room.
	ErrPlanFull = errors.New("trunk: table full")
	// ErrBusy is returned by MS.Call while a registration or call request
	// runs.
	ErrBusy = errors.New("trunk: request in progress")
	// ErrNotOnControl is returned by MS.Call when the MS is not on a
	// confirmed TSCC.
	ErrNotOnControl = errors.New("trunk: not on a TSCC")
	// ErrNotRegistered is returned by MS.Call and MS.CallData when the
	// TSCC asks for a registration that the MS does not hold.
	ErrNotRegistered = errors.New("trunk: not registered")
	// ErrStunned is returned for a user service of an MS that the TSCC
	// stunned.
	ErrStunned = errors.New("trunk: stunned")
	// ErrNoCall is returned by MS.Interrupt when the MS is not on the
	// payload channel of a voice call, and by TSCC.Interrupt for a slot
	// without a call.
	ErrNoCall = errors.New("trunk: no call")
)

// Errors of the appended data of a UDT. Match them with errors.Is.
var (
	// ErrUDTFormat is returned for appended data in another UDT Format
	// than the method reads or writes.
	ErrUDTFormat = errors.New("trunk: appended data in another UDT format")
	// ErrUDTLength is returned for data that does not fit in 1 to 4
	// appended blocks, or for blocks whose size does not match the format.
	ErrUDTLength = errors.New("trunk: appended data length out of range")
	// ErrUDTCharacter is returned for a character or digit that the UDT
	// Format cannot hold.
	ErrUDTCharacter = errors.New("trunk: character out of range for the UDT format")
)

var (
	errHunt          error = &dmrgo.ConfigError{Type: pkgName, Field: "Hunt"}
	errTalkgroups    error = &dmrgo.ConfigError{Type: pkgName, Field: "Talkgroups"}
	errSysCode       error = &dmrgo.ConfigError{Type: pkgName, Field: "SysCode"}
	errAddress       error = &dmrgo.ConfigError{Type: pkgName, Field: "Address"}
	errTarget        error = &dmrgo.ConfigError{Type: pkgName, Field: "target"}
	errDMRLA         error = &dmrgo.ConfigError{Type: pkgName, Field: "DMRLA"}
	errColorCode     error = &dmrgo.ConfigError{Type: pkgName, Field: "ColorCode"}
	errChannels      error = &dmrgo.ConfigError{Type: pkgName, Field: "Payload"}
	errRegistrations error = &dmrgo.ConfigError{Type: pkgName, Field: "MaxRegistrations"}
	errPowerSave     error = &dmrgo.ConfigError{Type: pkgName, Field: "PowerSave"}
	errStatus        error = &dmrgo.ConfigError{Type: pkgName, Field: "status"}
	errSupplementary error = &dmrgo.ConfigError{Type: pkgName, Field: "supplementary data"}
	errLead          error = &dmrgo.ConfigError{Type: pkgName, Field: "Lead"}
)
