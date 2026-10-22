package data

import (
	"errors"

	"github.com/USA-RedDragon/dmrgo/v2"
)

// ErrBusy is returned by Sender.Send while a message is in progress. Match
// it with errors.Is.
var ErrBusy = errors.New("data: message in progress")

var errZero = errors.New("data: zero Sender; make it with NewSender")

const pkgName = "data"

var (
	errColorCode    error = &dmrgo.ConfigError{Type: pkgName, Field: "ColorCode"}
	errSource       error = &dmrgo.ConfigError{Type: pkgName, Field: "Source"}
	errMaxBlocks    error = &dmrgo.ConfigError{Type: pkgName, Field: "MaxBlocks"}
	errPeers        error = &dmrgo.ConfigError{Type: pkgName, Field: "Peers"}
	errMaxFragment  error = &dmrgo.ConfigError{Type: pkgName, Field: "MaxFragment"}
	errMaxMessage   error = &dmrgo.ConfigError{Type: pkgName, Field: "MaxMessage"}
	errAccess       error = &dmrgo.ConfigError{Type: pkgName, Field: "Access"}
	errPreambles    error = &dmrgo.ConfigError{Type: pkgName, Field: "Preambles"}
	errRetries      error = &dmrgo.ConfigError{Type: pkgName, Field: "Retries"}
	errHeader       error = &dmrgo.ConfigError{Type: pkgName, Field: "Header"}
	errProprietary  error = &dmrgo.ConfigError{Type: pkgName, Field: "Proprietary"}
	errWindow       error = &dmrgo.ConfigError{Type: pkgName, Field: "Window"}
	errMessageGroup error = &dmrgo.ConfigError{Type: pkgName, Field: "Header.Group"}
)
