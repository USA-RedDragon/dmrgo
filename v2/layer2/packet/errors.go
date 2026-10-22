package packet

import (
	"errors"
	"fmt"

	"github.com/USA-RedDragon/dmrgo/v2"
	"github.com/USA-RedDragon/dmrgo/v2/fec"
)

// Errors returned by the package. Match them with errors.Is.
var (
	// ErrRate is returned for a data type other than rate 1/2, 3/4 or 1.
	ErrRate = errors.New("packet: data type is not rate 1/2, 3/4 or 1 coded")
	// ErrFormat is returned by NewPacket and Builder.Segment for a header
	// that is not U_HEAD, C_HEAD, DD_HEAD or R_HEAD, and by Bursts and
	// AppendBursts for a nil Packet.
	ErrFormat = errors.New("packet: header format does not carry segmented data")
	// ErrTooLong is returned when the payload needs more than 127 blocks,
	// or more than the header can count.
	ErrTooLong = errors.New("packet: payload needs too many blocks")
	// ErrNoRoom is returned by Assembler.Add and AddBurst, and by
	// Builder.Segment and Retry, for a packet with more data blocks than
	// the storage that NewAssembler or NewBuilder allocated. A zero
	// Assembler or Builder has room for none.
	ErrNoRoom = errors.New("packet: packet has more blocks than the storage holds")
	// ErrNoHeader is returned for a data block that no header announced.
	ErrNoHeader = errors.New("packet: data block without a header")
	// ErrRateChange is returned when the data type changes within a packet. The
	// partial packet is discarded. (ETSI TS 102 361-1 clause 8.2.2.1)
	ErrRateChange = errors.New("packet: data type changed within a packet")
	// ErrHeaderCRC is returned for a data header whose CRC-CCITT failed. The
	// Assembler drops the header and keeps the packet in progress, as MMDVMHost
	// does. It wraps fec.ErrUncorrectable. (ETSI TS 102 361-1 clause B.3.8)
	ErrHeaderCRC = fmt.Errorf("packet: data header CRC does not match: %w", fec.ErrUncorrectable)
	// ErrMessageCRC is returned with the Message whose message CRC failed,
	// or whose Pad Octet Count is more than the octets before the CRC, as
	// when a damaged slot type gives the blocks the wrong rate. It wraps
	// fec.ErrUncorrectable. (ETSI TS 102 361-1 clause 9.3.22)
	ErrMessageCRC = fmt.Errorf("packet: message CRC-32 does not match: %w", fec.ErrUncorrectable)
	// ErrBlocksMissing is returned when the blocks of a confirmed packet
	// have all arrived but some failed their CRC-9. Missing lists them and
	// Response builds the SACK. The Assembler keeps the good blocks for the
	// retry. It wraps fec.ErrUncorrectable.
	ErrBlocksMissing = fmt.Errorf("packet: confirmed blocks failed CRC-9: %w", fec.ErrUncorrectable)
	// ErrUnexpectedRetry is returned for a header of a later try (Full
	// Message Flag 0) that is not a retry of the packet in progress, while
	// that packet still waits for blocks of its attempt. The Assembler drops
	// the header and keeps the packet in progress.
	ErrUnexpectedRetry = errors.New("packet: later try of a packet that is not in progress")
	// ErrNoFirstTry is returned by Assembler.Response for a packet that a
	// later try started, because its first header was lost. The size of
	// the packet is not known, so a SACK cannot name its blocks; answer
	// with a NACK instead, which asks for the whole packet again.
	ErrNoFirstTry = errors.New("packet: packet started from a later try")
	// ErrNoRetryBlocks is returned by Builder.Retry when the response data
	// flags no block of the packet.
	ErrNoRetryBlocks = errors.New("packet: response data flags no block to resend")
	// ErrResponseData is returned for C_RDATA blocks of the wrong count or with
	// a bad CRC.
	ErrResponseData = errors.New("packet: invalid response data blocks")

	errResponseLength  = fmt.Errorf("%w: length is not 1 or 2 rate 1/2 blocks", ErrResponseData)
	errResponseCRC     = fmt.Errorf("%w: %w", ErrResponseData, fec.ErrUncorrectable)
	errNoConfirmed     = fmt.Errorf("%w: no confirmed packet in progress", ErrFormat)
	errRetryFormat     = fmt.Errorf("%w: only a confirmed packet has a later try", ErrFormat)
	errSelectiveFormat = fmt.Errorf("%w: only a C_HEAD or a packet with SARQ set has a selective retry", ErrFormat)

	errResponseClassRange = &dmrgo.FieldRangeError{Type: "Response", Field: "Class", Bits: 2}
	errResponseTypeRange  = &dmrgo.FieldRangeError{Type: "Response", Field: "Type", Bits: 3}
)
