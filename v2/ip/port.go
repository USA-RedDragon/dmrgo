package ip

import "github.com/USA-RedDragon/dmrgo/v2/internal/text"

// Protocol is the protocol number of an IPv4 packet (RFC 790).
type Protocol uint8

// The Protocol values.
const (
	ProtocolICMP Protocol = 1  // ICMP
	ProtocolTCP  Protocol = 6  // TCP
	ProtocolUDP  Protocol = 17 // UDP
)

// String returns the name of p. It allocates only for a value without a
// name.
func (p Protocol) String() string {
	switch p {
	case ProtocolICMP:
		return "ICMP"
	case ProtocolTCP:
		return "TCP"
	case ProtocolUDP:
		return "UDP"
	}
	return "Protocol(" + text.Int(int64(p)) + ")"
}

// Port is a UDP port. The named values are the ports of the application
// protocols that DMR radios send in data packets.
type Port uint16

// The Port values.
const (
	PortLRRP             Port = 4001  // MOTOTRBO Location Request and Response Protocol
	PortXCMP             Port = 4004  // MOTOTRBO Extensible Command Message Protocol
	PortARS              Port = 4005  // MOTOTRBO Automatic Registration Service
	PortTMS              Port = 4007  // MOTOTRBO Text Messaging Service
	PortTelemetry        Port = 4008  // MOTOTRBO telemetry
	PortOTAP             Port = 4009  // MOTOTRBO over the air programming
	PortBattery          Port = 4012  // MOTOTRBO battery management
	PortJobTicket        Port = 4013  // MOTOTRBO job ticket server
	PortHyteraRRS        Port = 3002  // Hytera radio registration service
	PortHyteraLocation   Port = 3003  // Hytera location protocol
	PortHyteraText       Port = 3004  // Hytera text message protocol
	PortHyteraCall       Port = 3005  // Hytera radio control protocol
	PortHyteraTelemetry  Port = 3006  // Hytera telemetry protocol
	PortHyteraData       Port = 3007  // Hytera data transmit protocol
	PortHyteraSelfDefine Port = 3009  // Hytera self-defined message protocol
	PortText             Port = 5016  // text messaging (ETSI TS 102 361-3 clause 5.7.1)
	PortLocation         Port = 5017  // location information protocol (ETSI TS 102 361-3 clause 5.7.2)
	PortNMEA             Port = 10110 // NMEA 0183 sentences
)

// String returns the name of p, or its number when it has no name. It
// allocates only for a value without a name.
func (p Port) String() string {
	switch p {
	case PortLRRP:
		return "LRRP"
	case PortXCMP:
		return "XCMP"
	case PortARS:
		return "ARS"
	case PortTMS:
		return "TMS"
	case PortTelemetry:
		return "Telemetry"
	case PortOTAP:
		return "OTAP"
	case PortBattery:
		return "Battery"
	case PortJobTicket:
		return "Job Ticket"
	case PortHyteraRRS:
		return "Hytera RRS"
	case PortHyteraLocation:
		return "Hytera Location"
	case PortHyteraText:
		return "Hytera Text"
	case PortHyteraCall:
		return "Hytera Radio Control"
	case PortHyteraTelemetry:
		return "Hytera Telemetry"
	case PortHyteraData:
		return "Hytera Data"
	case PortHyteraSelfDefine:
		return "Hytera Self-Defined"
	case PortText:
		return "Text"
	case PortLocation:
		return "Location"
	case PortNMEA:
		return "NMEA"
	}
	return text.Int(int64(p))
}
