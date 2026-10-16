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

// newColorCodeMismatch returns ErrColorCodeMismatch with got and want.
//
//go:noinline
func newColorCodeMismatch(got, want enums.ColorCode) error {
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

// Errors of misuse that carry no values.
var (
	errDataBurstNoSync   = errors.New("a data burst needs a data SYNC pattern; only a decoded data burst with an EMB has SyncNone")
	errNoData            = errors.New("data burst has no Data")
	errNilData           = errors.New("cannot build a burst from nil Data")
	errRCNotOutbound     = errors.New("only a BS-sourced voice burst F carries a reverse channel")
	errUnknownSyncSource = errors.New("unknown sync source")
)

// The new*Error functions build the errors of the package that carry
// values. They are not inlined, so the functions that return the errors have
// no heap allocation.

// newWrappedError returns err with the prefix "prefix: ".
//
//go:noinline
func newWrappedError(prefix string, err error) error {
	return fmt.Errorf("%s: %w", prefix, err)
}

// newJoinedError returns errors.Join(a, b).
//
//go:noinline
func newJoinedError(a, b error) error {
	return errors.Join(a, b)
}

// newReservedDataTypeError returns ErrReservedDataType for data type dt.
//
//go:noinline
func newReservedDataTypeError(dt enums.DataType) error {
	return reservedDataTypeError{dt}
}

// newUnknownKindError returns the error for a burst kind out of range.
//
//go:noinline
func newUnknownKindError(k Kind) error {
	return fmt.Errorf("unknown burst kind %d", k)
}

// newKindError returns the error for a burst kind that does not encode.
//
//go:noinline
func newKindError(k Kind) error {
	return fmt.Errorf("burst kind %s does not encode", k)
}

// newKindSyncError returns the error for a burst of kind k with a SYNC
// pattern that does not fit it.
//
//go:noinline
func newKindSyncError(k Kind, p enums.SyncPattern) error {
	return fmt.Errorf("a %s burst cannot carry SYNC pattern %s", k, p)
}

// newVoicePositionError returns the error for a voice burst position out of
// range.
//
//go:noinline
func newVoicePositionError(pos enums.VoiceBurstType) error {
	return fmt.Errorf("voice burst position %d out of range", pos)
}

// newNotAToFError returns the error for a voice burst position that is not A
// to F.
//
//go:noinline
func newNotAToFError(pos enums.VoiceBurstType) error {
	return fmt.Errorf("voice burst position %s is not A to F", pos.String())
}

// newEMBColorCodeError returns the error for a data burst with an EMB whose
// color code differs from that of its slot type.
//
//go:noinline
func newEMBColorCodeError(emb, slotType enums.ColorCode) error {
	return fmt.Errorf("a data burst with an EMB needs the color code of its slot type: EMB %d, slot type %d", emb, slotType)
}

// newPayloadTypeError returns the error for a decoded payload of data type
// decoded in a burst whose slot type has data type dt.
//
//go:noinline
func newPayloadTypeError(decoded, dt enums.DataType) error {
	return fmt.Errorf("decoded payload is a %s but the slot type data type is %s; set Data", decoded.String(), dt.String())
}

// newDataError returns the error for Data d that a data burst of dataType
// cannot carry, a nil pointer or Data of another data type, or nil.
//
//go:noinline
func newDataError(d Data, dataType enums.DataType) error {
	if rv := reflect.ValueOf(d); rv.Kind() == reflect.Pointer && rv.IsNil() {
		return fmt.Errorf("data type %s cannot carry a nil %T", dataType.String(), d)
	}
	if d.DataType() != dataType {
		return fmt.Errorf("data type %s cannot carry Data of type %T with data type %s",
			dataType.String(), d, d.DataType().String())
	}
	return nil
}

// newInfoBitsError returns the error for Data d that encodes n information
// bits where a burst of dataType carries want.
//
//go:noinline
func newInfoBitsError(dataType enums.DataType, want int, d Data, n int) error {
	return fmt.Errorf("data type %s carries %d information bits, but %T encodes %d", dataType.String(), want, d, n)
}

// newNotBPTCError returns the error for a data type that is not BPTC(196,96)
// coded.
//
//go:noinline
func newNotBPTCError(dt enums.DataType) error {
	return fmt.Errorf("data type %s is not BPTC(196,96) coded", dt.String())
}

// newSyncSourceError returns the error for an unknown sync source.
//
//go:noinline
func newSyncSourceError(src SyncSource) error {
	return fmt.Errorf("unknown sync source %d", src)
}

// newSyncPatternError returns the error for a value that is not a SYNC
// pattern.
//
//go:noinline
func newSyncPatternError(p enums.SyncPattern) error {
	return fmt.Errorf("%#x is not a Table 9.2 SYNC pattern", uint64(p))
}

// newMMDVMHostDiscardsError returns the error for a data type that MMDVMHost
// discards.
//
//go:noinline
func newMMDVMHostDiscardsError(dt enums.DataType) error {
	return fmt.Errorf("MMDVMHost discards %s bursts", dt.String())
}

// newSuperframeLCError returns err for the embedded LC of superframe n.
//
//go:noinline
func newSuperframeLCError(n int, err error) error {
	return fmt.Errorf("superframe %d embedded LC: %w", n, err)
}
