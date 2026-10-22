package layer2

import (
	"errors"

	"github.com/USA-RedDragon/dmrgo/v2"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/fec"
	"github.com/USA-RedDragon/dmrgo/v2/internal/text"
)

// Errors returned by the package. Match them with errors.Is.
var (
	// ErrReservedDataType is returned, wrapped, by DecodeFromBytes for a data
	// burst whose slot type carries a reserved data type. The burst has no Data
	// (ETSI TS 102 361-1 Table 9.22).
	ErrReservedDataType = errors.New("layer2: reserved data type")
	// ErrEmbeddedLCSequence is returned by VoiceReceiver.Receive and
	// EmbeddedLCAssembler.Add when a burst does not continue the partial
	// embedded LC, which is discarded.
	ErrEmbeddedLCSequence = errors.New("layer2: embedded LC fragment out of sequence")
	// ErrEmbeddedLCUncorrectable is returned by VoiceReceiver.Receive and
	// EmbeddedLCAssembler.Add when the embedded LC of bursts B to E fails
	// its FEC or its checksum. It wraps fec.ErrUncorrectable.
	ErrEmbeddedLCUncorrectable error = &wrapError{"embedded LC: " + fec.ErrUncorrectable.Error(), fec.ErrUncorrectable}
	// ErrShortLCSequence is returned by ShortLCAssembler.Add when a CACH
	// fragment arrives out of sequence or has an LCSS that is not valid for
	// Short LC. The partial Short LC is discarded.
	ErrShortLCSequence = errors.New("layer2: short LC fragment out of sequence")
	// ErrColorCodeMismatch is returned, wrapped, by DecodeFromBytesWith when the
	// burst's color code differs from DecodeOptions.ColorCode.
	ErrColorCodeMismatch = errors.New("layer2: color code mismatch")
	// ErrVoiceTerminated is returned by VoiceTransmitter methods called after
	// Terminate.
	ErrVoiceTerminated = errors.New("layer2: voice transmission already terminated")
	// ErrDataType is returned, wrapped or as is, by the builders and by the
	// SetHint methods of the transports for a data type that the call cannot
	// carry: Data of another data type than the slot type, or a data type
	// that is not BPTC(196,96) coded for BuildDataBurst.
	ErrDataType = errors.New("layer2: the data burst cannot carry this data type")
	// ErrSyncSource is returned by the builders and the voice transmitter
	// for a SyncSource that is not one of its values.
	ErrSyncSource = errors.New("layer2: unknown sync source")
	// ErrVoicePosition is returned, wrapped or as is, for a voice burst
	// position that is not one of its values, or that is not A to F where
	// the call needs a burst of a superframe.
	ErrVoicePosition = errors.New("layer2: voice burst position out of range")
	errNotPDU        = errors.New("layer2: data is not a PDU type of package pdu")
)

type wrapError struct {
	msg string
	err error
}

func (e *wrapError) Error() string {
	return e.msg
}

func (e *wrapError) Unwrap() error {
	return e.err
}

type reservedDataTypeError struct {
	dt enums.DataType
}

// Error returns the message of e. It allocates the message.
func (e reservedDataTypeError) Error() string {
	return ErrReservedDataType.Error() + " " + text.Int(int64(e.dt))
}

func (e reservedDataTypeError) Unwrap() error {
	return ErrReservedDataType
}

type colorCodeMismatchError struct {
	gotWant uint8
}

func colorCodeMismatch(got, want enums.ColorCode) error {
	if got > 0x0F || want > 0x0F {
		return ErrColorCodeMismatch
	}
	return colorCodeMismatchError{gotWant: uint8(got)<<4 | uint8(want)}
}

// Error returns the message of e. It allocates the message.
func (e colorCodeMismatchError) Error() string {
	return ErrColorCodeMismatch.Error() + ": burst has " + text.Int(int64(e.gotWant>>4)) +
		", want " + text.Int(int64(e.gotWant&0x0F))
}

func (e colorCodeMismatchError) Unwrap() error {
	return ErrColorCodeMismatch
}

var (
	errDataBurstNoSync       = errors.New("layer2: a data burst needs a data SYNC pattern; only a decoded data burst with an EMB has SyncNone")
	errNoData                = errors.New("layer2: data burst has no Data")
	errRCNotOutbound         = errors.New("layer2: only a BS-sourced voice burst F carries a reverse channel")
	errUnknownKind           = errors.New("layer2: unknown burst kind")
	errKindNoEncode          = errors.New("layer2: the burst kind does not encode")
	errKindSync              = errors.New("layer2: the burst kind cannot carry its SYNC pattern")
	errNotAToF         error = &wrapError{"the voice burst position is not A to F", ErrVoicePosition}
	errEMBColorCode          = errors.New("layer2: a data burst with an EMB needs the color code of its slot type")
	errPayloadType           = errors.New("layer2: the decoded payload has another data type than the slot type; set Data")
	errNilPointerData        = errors.New("layer2: the data burst cannot carry a nil Data pointer")
	errDataPDUType     error = &wrapError{"the data burst cannot carry Data of another data type", ErrDataType}
	errNotBPTC         error = &wrapError{"the data type is not BPTC(196,96) coded", ErrDataType}
	errSyncPattern           = errors.New("layer2: not a Table 9.2 SYNC pattern")
	errMMDVMHostData         = errors.New("layer2: MMDVMHost discards bursts of this data type")

	errBurstBPTCReservedRange    = &dmrgo.FieldRangeError{Type: "Burst", Field: "BPTCReserved", Bits: 3}
	errRawBurstBPTCReservedRange = &dmrgo.FieldRangeError{Type: "RawBurst", Field: "BPTCReserved", Bits: 3}
	errRawBurstInfoRange         = &dmrgo.FieldRangeError{Type: "RawBurst", Field: "SetInfo", Bits: 196}
)

var (
	errReservedColorCode      = errors.Join(ErrReservedDataType, ErrColorCodeMismatch)
	errUnknownKindColorCode   = errors.Join(errUnknownKind, ErrColorCodeMismatch)
	errVoicePositionColorCode = errors.Join(ErrVoicePosition, ErrColorCodeMismatch)
)

func joinColorCode(err error) error {
	if _, ok := err.(reservedDataTypeError); ok {
		return errReservedColorCode
	}
	switch err {
	case errUnknownKind:
		return errUnknownKindColorCode
	case ErrVoicePosition:
		return errVoicePositionColorCode
	}
	return err
}
