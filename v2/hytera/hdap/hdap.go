package hdap

import (
	"encoding/binary"

	"github.com/USA-RedDragon/dmrgo/v2/internal/text"
)

// Service is the 7-bit service of a message.
type Service uint8

// The Service values.
const (
	ServiceRCP Service = 0x02 // radio control protocol
	ServiceLP  Service = 0x08 // location protocol
	ServiceTMP Service = 0x09 // text message protocol
	ServiceRRS Service = 0x11 // radio registration service
	ServiceTP  Service = 0x12 // telemetry protocol
	ServiceDTP Service = 0x13 // data transmit protocol
	ServiceDDS Service = 0x14 // data delivery states
)

// String returns the name of s. It allocates only for a value without a
// name.
func (s Service) String() string {
	switch s {
	case ServiceRCP:
		return "RCP"
	case ServiceLP:
		return "LP"
	case ServiceTMP:
		return "TMP"
	case ServiceRRS:
		return "RRS"
	case ServiceTP:
		return "TP"
	case ServiceDTP:
		return "DTP"
	case ServiceDDS:
		return "DDS"
	}
	return "Service(" + text.Int(int64(s)) + ")"
}

// Header is the first octet of a message.
//
//dmr:frame 1
type Header struct {
	// Reliable asks for a reliable transfer.
	Reliable bool `dmr:"bit:0"`
	// Service is the service of the message.
	Service Service `dmr:"bits:1-7,enum"`
}

// Message is a Hytera DMR Application Protocol (HDAP) message: the header,
// the opcode of the service, the payload, a checksum and the end octet
// 0x03. Its numbers are big-endian, as in every service but RCP, whose
// little-endian messages the package does not decode.
//
//dmr:stream
type Message struct {
	// Header is the first octet.
	Header Header `dmr:"frame"`
	// Opcode is the opcode of the service.
	Opcode Opcode `dmr:"u16"`
	// Payload is the payload of the opcode.
	Payload []byte `dmr:"lv16"`
	// Checksum covers the opcode, the length and the payload. Seal sets
	// it.
	Checksum uint8 `dmr:"u8"`
	// End is the end octet, 0x03.
	End uint8 `dmr:"u8,const:0x03"`
}

func (m *Message) sum() uint8 {
	var h [4]byte
	binary.BigEndian.PutUint16(h[0:], uint16(m.Opcode))
	n := len(m.Payload)
	if n <= 0xFFFF {
		binary.BigEndian.PutUint16(h[2:], uint16(n))
	}
	var s uint8
	for _, b := range h {
		s += b
	}
	for _, b := range m.Payload {
		s += b
	}
	return (s ^ 0xFF) + 0x33
}

// Seal sets Checksum: the sum of the octets of the opcode, the length and
// the payload, inverted, plus 0x33.
func (m *Message) Seal() {
	if m == nil {
		return
	}
	m.Checksum = m.sum()
}

// ChecksumOK reports whether Checksum matches the opcode, the length and
// the payload.
func (m *Message) ChecksumOK() bool {
	if m == nil {
		return false
	}
	return m.Checksum == m.sum()
}
