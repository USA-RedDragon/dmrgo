package privacy

import (
	"errors"

	"github.com/USA-RedDragon/dmrgo/v2"
)

// Errors of the package. Match them with errors.Is.

const pkgName = "privacy"

var (
	// ErrKeysFull is returned by Receiver.AddKey when the Receiver holds
	// MaxKeys keys.
	ErrKeysFull = errors.New("privacy: no room for another key")
	// ErrNotStarted is returned by Transmitter.Output before Start.
	ErrNotStarted = errors.New("privacy: transmitter not started")
	// ErrAmateur is returned by NewTransmitter for ServiceAmateur: the
	// amateur service forbids messages encoded to obscure their meaning.
	ErrAmateur = errors.New("privacy: the amateur service forbids encryption")
	// ErrDataAlgorithm is returned by Transmitter.Data for a key other than
	// ARC4: no public capture with a key proves the data framing of AES,
	// DES or Basic Privacy.
	ErrDataAlgorithm = errors.New("privacy: no proven data framing for the algorithm")
)

var (
	errAESKey  error = &dmrgo.ConfigError{Type: pkgName, Field: "key"}
	errService error = &dmrgo.ConfigError{Type: pkgName, Field: "Service"}
	errSAP     error = &dmrgo.ConfigError{Type: pkgName, Field: "sap"}
)
