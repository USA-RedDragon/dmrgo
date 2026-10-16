package layer2

import (
	"errors"
	"fmt"
	"reflect"
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
	// errNoEncode is returned by Burst.Encode and BuildBurst for Data without
	// an Encode method that returns 96, 144 or 192 information bits.
	errNoEncode = errors.New("data has no Encode method that returns 96, 144 or 192 information bits")
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

// colorCodeMismatch returns ErrColorCodeMismatch with got and want, or
// ErrColorCodeMismatch alone when one of them does not fit in 4 bits.
func colorCodeMismatch(got, want enums.ColorCode) error {
	if got > 0x0F || want > 0x0F {
		return ErrColorCodeMismatch
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

// The errors of the package that are not exported. Each is a static value,
// so a call that fails does not allocate. They carry no values.
var (
	errDataBurstNoSync = errors.New("a data burst needs a data SYNC pattern; only a decoded data burst with an EMB has SyncNone")
	errNoData          = errors.New("data burst has no Data")
	errNilData         = errors.New("cannot build a burst from nil Data")
	errRCNotOutbound   = errors.New("only a BS-sourced voice burst F carries a reverse channel")
	errUnknownKind     = errors.New("unknown burst kind")
	errKindNoEncode    = errors.New("the burst kind does not encode")
	errKindSync        = errors.New("the burst kind cannot carry its SYNC pattern")
	errVoicePosition   = errors.New("voice burst position out of range")
	errNotAToF         = errors.New("the voice burst position is not A to F")
	errEMBColorCode    = errors.New("a data burst with an EMB needs the color code of its slot type")
	errPayloadType     = errors.New("the decoded payload has another data type than the slot type; set Data")
	errNilPointerData  = errors.New("the data burst cannot carry a nil Data pointer")
	errDataType        = errors.New("the data burst cannot carry Data of another data type")
	errInfoBits        = errors.New("the Data encodes another number of information bits than its data type carries")
	errNotBPTC         = errors.New("the data type is not BPTC(196,96) coded")
	errSyncSource      = errors.New("unknown sync source")
	errSyncPattern     = errors.New("not a Table 9.2 SYNC pattern")
	errMMDVMHostData   = errors.New("MMDVMHost discards bursts of this data type")
)

// The joins of a decode error with ErrColorCodeMismatch, which
// DecodeFromBytesWith returns when both happen. They are built once.
var (
	errReservedColorCode      = errors.Join(ErrReservedDataType, ErrColorCodeMismatch)
	errUnknownKindColorCode   = errors.Join(errUnknownKind, ErrColorCodeMismatch)
	errVoicePositionColorCode = errors.Join(errVoicePosition, ErrColorCodeMismatch)
)

// joinColorCode returns the static join of the decode error err with
// ErrColorCodeMismatch.
func joinColorCode(err error) error {
	if _, ok := err.(reservedDataTypeError); ok {
		return errReservedColorCode
	}
	switch err {
	case errUnknownKind:
		return errUnknownKindColorCode
	case errVoicePosition:
		return errVoicePositionColorCode
	}
	return err
}

// checkData returns the error for Data d that a data burst of dataType
// cannot carry, a nil pointer or Data of another data type, or nil.
func checkData(d Data, dataType enums.DataType) error {
	if rv := reflect.ValueOf(d); rv.Kind() == reflect.Pointer && rv.IsNil() {
		return errNilPointerData
	}
	if d.DataType() != dataType {
		return errDataType
	}
	return nil
}
