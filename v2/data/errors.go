package data

import (
	"errors"

	"github.com/USA-RedDragon/dmrgo/v2"
)

// ErrBusy is returned by Sender.Send while a message is in progress. Match
// it with errors.Is.
var ErrBusy = errors.New("data: message in progress")

var errZero = errors.New("data: zero Sender; make it with NewSender")

var (
	errColorCode    error = &dmrgo.ConfigError{Type: "data", Field: "ColorCode"}
	errSource       error = &dmrgo.ConfigError{Type: "data", Field: "Source"}
	errMaxBlocks    error = &dmrgo.ConfigError{Type: "data", Field: "MaxBlocks"}
	errPeers        error = &dmrgo.ConfigError{Type: "data", Field: "Peers"}
	errMaxFragment  error = &dmrgo.ConfigError{Type: "data", Field: "MaxFragment"}
	errMaxMessage   error = &dmrgo.ConfigError{Type: "data", Field: "MaxMessage"}
	errAccess       error = &dmrgo.ConfigError{Type: "data", Field: "Access"}
	errPreambles    error = &dmrgo.ConfigError{Type: "data", Field: "Preambles"}
	errRetries      error = &dmrgo.ConfigError{Type: "data", Field: "Retries"}
	errHeader       error = &dmrgo.ConfigError{Type: "data", Field: "Header"}
	errWindow       error = &dmrgo.ConfigError{Type: "data", Field: "Window"}
	errMessageGroup error = &dmrgo.ConfigError{Type: "data", Field: "Header.Group"}
)
