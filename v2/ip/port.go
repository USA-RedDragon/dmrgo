package ip

import (
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/internal/text"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

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

// DPIDPort returns the UDP port of dpid: PortText for DPIDTextMessage and
// PortLocation for DPIDLocationProtocol. It returns 0 for
// DPIDExtendedHeader, whose port is in the Extended Header, and for a
// manufacturer specific or reserved DPID, whose port the radio configures.
// (ETSI TS 102 361-3 clause 7.2.1.2, Table 7.18)
func DPIDPort(dpid enums.DPID) Port {
	switch dpid {
	case enums.DPIDTextMessage:
		return PortText
	case enums.DPIDLocationProtocol:
		return PortLocation
	case enums.DPIDExtendedHeader:
	}
	return 0
}

// PortDPID returns the DPID of the UDP port p. ok is false for a port that
// has no DPID, which goes in the Extended Header with DPIDExtendedHeader.
func PortDPID(p Port) (dpid enums.DPID, ok bool) {
	switch p {
	case PortText:
		return enums.DPIDTextMessage, true
	case PortLocation:
		return enums.DPIDLocationProtocol, true
	default:
		return enums.DPIDExtendedHeader, false
	}
}

// SPIDPort returns the UDP port of spid: PortText for SPIDTextMessage and
// PortLocation for SPIDLocationProtocol. It returns 0 for
// SPIDExtendedHeader, whose port is in the Extended Header, and for a
// manufacturer specific or reserved SPID, whose port the radio configures.
// (ETSI TS 102 361-3 clause 7.2.1.1, Table 7.17)
func SPIDPort(spid enums.SPID) Port {
	switch spid {
	case enums.SPIDTextMessage:
		return PortText
	case enums.SPIDLocationProtocol:
		return PortLocation
	case enums.SPIDExtendedHeader:
	}
	return 0
}

// PortSPID returns the SPID of the UDP port p. ok is false for a port that
// has no SPID, which goes in the Extended Header with SPIDExtendedHeader.
func PortSPID(p Port) (spid enums.SPID, ok bool) {
	switch p {
	case PortText:
		return enums.SPIDTextMessage, true
	case PortLocation:
		return enums.SPIDLocationProtocol, true
	default:
		return enums.SPIDExtendedHeader, false
	}
}

// CompressedPorts returns the UDP source and destination ports of a
// compressed UDP/IPv4 header: the port of the SPID or DPID, or the port in
// an Extended Header when the SPID or DPID is 0. A manufacturer specific or
// reserved SPID or DPID gives port 0. A nil h gives 0 and 0. (ETSI TS 102
// 361-3 clauses 7.2.1.1 and 7.2.1.2)
func CompressedPorts(h *pdu.UDPIPv4CompressedHeader) (source, destination Port) {
	if h == nil {
		return 0, 0
	}
	source, destination = SPIDPort(h.SPID), DPIDPort(h.DPID)
	switch {
	case h.SPID == enums.SPIDExtendedHeader && h.DPID == enums.DPIDExtendedHeader:
		source, destination = Port(h.ExtendedHeader1), Port(h.ExtendedHeader2)
	case h.SPID == enums.SPIDExtendedHeader:
		source = Port(h.ExtendedHeader1)
	case h.DPID == enums.DPIDExtendedHeader:
		destination = Port(h.ExtendedHeader1)
	}
	return source, destination
}
