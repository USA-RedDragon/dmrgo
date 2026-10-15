package packet

import (
	"errors"
	"fmt"

	"github.com/USA-RedDragon/dmrgo/v2/fec"
)

// Errors returned by the package. Match them with errors.Is.
var (
	// ErrRate is returned for a data type other than rate 1/2, 3/4 or 1.
	ErrRate = errors.New("data type is not rate 1/2, 3/4 or 1 coded")
	// ErrFormat is returned by Segment for a header that is not U_HEAD,
	// C_HEAD, DD_HEAD or R_HEAD, or whose matching sub-header is nil.
	ErrFormat = errors.New("header format does not carry segmented data")
	// ErrTooLong is returned when the payload needs more than 127 blocks,
	// or more than the header can count.
	ErrTooLong = errors.New("payload needs too many blocks")
	// ErrNoHeader is returned for a data block that no header announced.
	ErrNoHeader = errors.New("data block without a header")
	// ErrRateChange is returned when the data type changes within a packet
	// (ETSI TS 102 361-1 clause 8.2.2.1). The partial packet is discarded.
	ErrRateChange = errors.New("data type changed within a packet")
	// ErrMessageCRC is returned with the Message whose message CRC failed.
	// It wraps fec.ErrUncorrectable.
	ErrMessageCRC = fmt.Errorf("message CRC-32 does not match: %w", fec.ErrUncorrectable)
	// ErrBlocksMissing is returned when the blocks of a confirmed packet
	// have all arrived but some failed their CRC-9. Missing lists them and
	// Response builds the SACK. The Assembler keeps the good blocks for the
	// retry. It wraps fec.ErrUncorrectable.
	ErrBlocksMissing = fmt.Errorf("confirmed blocks failed CRC-9: %w", fec.ErrUncorrectable)
	// ErrResponseData is returned for C_RDATA blocks of the wrong count or with
	// a bad CRC.
	ErrResponseData = errors.New("invalid response data blocks")

	errResponseLength = fmt.Errorf("%w: length is not 1 or 2 rate 1/2 blocks", ErrResponseData)
	errResponseCRC    = fmt.Errorf("%w: %w", ErrResponseData, fec.ErrUncorrectable)
	errNoConfirmed    = fmt.Errorf("%w: no confirmed packet in progress", ErrFormat)
)
