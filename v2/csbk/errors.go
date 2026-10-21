package csbk

import (
	"errors"

	"github.com/USA-RedDragon/dmrgo/v2"
)

// Errors of the machines. Match them with errors.Is.

const pkgName = "csbk"

var (
	// ErrBusy is returned by a request or a wakeup while another one runs.
	ErrBusy = errors.New("csbk: request in progress")
	// ErrNoAddresses is returned by Requester.Request for a standard CSBK
	// without both a target and a source address.
	ErrNoAddresses = errors.New("csbk: CSBK without target and source addresses")
)

var (
	errColorCode  error = &dmrgo.ConfigError{Type: pkgName, Field: "ColorCode"}
	errSource     error = &dmrgo.ConfigError{Type: pkgName, Field: "Source"}
	errRetries    error = &dmrgo.ConfigError{Type: pkgName, Field: "Retries"}
	errAttempts   error = &dmrgo.ConfigError{Type: pkgName, Field: "Attempts"}
	errAddress    error = &dmrgo.ConfigError{Type: pkgName, Field: "Address"}
	errTarget     error = &dmrgo.ConfigError{Type: pkgName, Field: "Target"}
	errSourceAddr error = &dmrgo.ConfigError{Type: pkgName, Field: "SourceAddress"}
	errPreambles  error = &dmrgo.ConfigError{Type: pkgName, Field: "Preambles"}
	errAccess     error = &dmrgo.ConfigError{Type: pkgName, Field: "Access"}
	errMatch      error = &dmrgo.ConfigError{Type: pkgName, Field: "Match"}
	errOACSU      error = &dmrgo.ConfigError{Type: pkgName, Field: "OACSU"}
	errID         error = &dmrgo.ConfigError{Type: pkgName, Field: "ID"}
	errDynamicID  error = &dmrgo.ConfigError{Type: pkgName, Field: "DynamicID"}
	errSlot       error = &dmrgo.ConfigError{Type: pkgName, Field: "Slot"}
)
