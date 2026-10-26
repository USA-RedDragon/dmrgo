package dvsi

import (
	"errors"
	"fmt"
)

// Errors returned by the package. Match them with errors.Is.
var (
	// ErrIncomplete is returned by Parse when the bytes hold only the start
	// of a packet.
	ErrIncomplete = errors.New("dvsi: incomplete packet")
	// ErrStart is returned by Parse when the first byte is not StartByte.
	ErrStart = errors.New("dvsi: no start byte")
	// ErrType is returned by Parse for a packet type other than control,
	// channel or speech.
	ErrType = errors.New("dvsi: unknown packet type")
	// ErrLength is returned for a packet whose LENGTH is over MaxLength or
	// does not hold its fields.
	ErrLength = errors.New("dvsi: bad packet length")
	// ErrParity is returned by Parse when a packet that must have a parity
	// field has none or a wrong parity byte.
	ErrParity = errors.New("dvsi: bad parity")
	// ErrField is returned for a field that is unknown, cut short or not
	// the one that was expected.
	ErrField = errors.New("dvsi: bad field")
	// ErrChannel is returned for a channel outside 0 to MaxChannel.
	ErrChannel = errors.New("dvsi: bad channel")
	// ErrSamples is returned for a number of speech samples outside
	// MinSamples to MaxSamples, and by Codec.Encode for other than
	// FrameSamples samples.
	ErrSamples = errors.New("dvsi: bad number of samples")
	// ErrBits is returned for a number of channel bits outside MinBits to
	// MaxBits, or channel data of another length.
	ErrBits = errors.New("dvsi: bad number of channel bits")
	// ErrStatus is returned when the chip reports an error in a response
	// field.
	ErrStatus = errors.New("dvsi: device reported an error")
	// ErrSpeechFormat is returned by New when the chip rejects the
	// PKT_SPCHFMT field that turns on its decoder status.
	ErrSpeechFormat = errors.New("dvsi: device rejected the decoder status format (PKT_SPCHFMT)")
	// ErrTimeout is returned when the chip does not reply within
	// Config.Timeout.
	ErrTimeout = errors.New("dvsi: device timed out")
	// ErrPort is returned when a Port returns a byte count outside 0 to the
	// length of the buffer, or a Write returns 0 bytes and no error.
	ErrPort = errors.New("dvsi: port returned a bad byte count")
	// ErrClosed is returned when a closed Codec is used.
	ErrClosed = errors.New("dvsi: codec is closed")
)

var errDCMode = fmt.Errorf("%w: rejected the decoder mode reset (PKT_DCMODE)", ErrStatus)
