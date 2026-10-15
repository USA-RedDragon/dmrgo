package layer2

import (
	"errors"
	"fmt"
	"strconv"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/fec"
)

// Errors returned by the package. Match them with errors.Is.
var (
	// ErrReservedDataType is returned, wrapped, by DecodeFromBytes for a data
	// burst whose slot type carries a reserved data type. The burst has no Data
	// (ETSI TS 102 361-1 Table 9.22).
	ErrReservedDataType = errors.New("reserved data type")
	// ErrEmbeddedLCSequence is returned by VoiceReceiver.Receive and
	// EmbeddedLCAssembler.Add when a burst does not continue the partial
	// embedded LC, which is discarded.
	ErrEmbeddedLCSequence = errors.New("embedded LC fragment out of sequence")
	// ErrEmbeddedLCUncorrectable is returned by VoiceReceiver.Receive and
	// EmbeddedLCAssembler.Add when the embedded LC of bursts B to E fails
	// its FEC or its checksum. It wraps fec.ErrUncorrectable.
	ErrEmbeddedLCUncorrectable = fmt.Errorf("embedded LC: %w", fec.ErrUncorrectable)
	// ErrShortLCSequence is returned by ShortLCAssembler.Add and AddFragment
	// when a CACH fragment arrives out of sequence or has an LCSS that is not
	// valid for Short LC. The partial Short LC is discarded.
	ErrShortLCSequence = errors.New("short LC fragment out of sequence")
	// ErrShortLCIncomplete is returned by ShortLCAssembler.Complete when the
	// assembler does not hold 4 fragments in sequence.
	ErrShortLCIncomplete = errors.New("short LC needs 4 fragments in sequence")
	// ErrColorCodeMismatch is returned, wrapped, by DecodeFromBytesWith when the
	// burst's color code differs from DecodeOptions.ColorCode.
	ErrColorCodeMismatch = errors.New("color code mismatch")
	// ErrVoiceTerminated is returned by VoiceTransmitter methods called after
	// Terminate.
	ErrVoiceTerminated = errors.New("voice transmission already terminated")
)

// reservedDataTypeError is ErrReservedDataType for a burst of data type dt.
// It is one byte, so it converts to an error without allocating.
type reservedDataTypeError struct {
	dt enums.DataType
}

func (e reservedDataTypeError) Error() string {
	return ErrReservedDataType.Error() + " " + strconv.Itoa(int(e.dt))
}

func (e reservedDataTypeError) Unwrap() error {
	return ErrReservedDataType
}

// colorCodeMismatchError is ErrColorCodeMismatch for a burst of color code
// got, in the high 4 bits, and the expected color code, in the low 4 bits.
// It is one byte, so it converts to an error without allocating.
type colorCodeMismatchError struct {
	gotWant uint8
}

// colorCodeMismatch returns ErrColorCodeMismatch with got and want.
func colorCodeMismatch(got, want enums.ColorCode) error {
	if got > 0x0F || want > 0x0F {
		return fmt.Errorf("%w: burst has %d, want %d", ErrColorCodeMismatch, got, want)
	}
	return colorCodeMismatchError{gotWant: uint8(got)<<4 | uint8(want)}
}

func (e colorCodeMismatchError) Error() string {
	return ErrColorCodeMismatch.Error() + ": burst has " + strconv.Itoa(int(e.gotWant>>4)) +
		", want " + strconv.Itoa(int(e.gotWant&0x0F))
}

func (e colorCodeMismatchError) Unwrap() error {
	return ErrColorCodeMismatch
}
