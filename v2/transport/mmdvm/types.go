package mmdvm

import "github.com/USA-RedDragon/dmrgo/v2/internal/text"

// Command is octet 2 of a frame (octet 3 of a long frame): what the frame
// carries.
type Command uint8

// The Command values of the MMDVM and MMDVM_HS firmware.
const (
	CommandGetVersion   Command = 0x00
	CommandGetStatus    Command = 0x01
	CommandSetConfig    Command = 0x02
	CommandSetMode      Command = 0x03
	CommandSetFreq      Command = 0x04
	CommandCalData      Command = 0x08
	CommandRSSIData     Command = 0x09
	CommandSendCWID     Command = 0x0A
	CommandDStarHeader  Command = 0x10
	CommandDStarData    Command = 0x11
	CommandDStarLost    Command = 0x12
	CommandDStarEOT     Command = 0x13
	CommandDMRAloha     Command = 0x17
	CommandDMRData1     Command = 0x18
	CommandDMRLost1     Command = 0x19
	CommandDMRData2     Command = 0x1A
	CommandDMRLost2     Command = 0x1B
	CommandDMRShortLC   Command = 0x1C
	CommandDMRStart     Command = 0x1D
	CommandDMRAbort     Command = 0x1E
	CommandYSFData      Command = 0x20
	CommandYSFLost      Command = 0x21
	CommandP25Header    Command = 0x30
	CommandP25LDU       Command = 0x31
	CommandP25Lost      Command = 0x32
	CommandNXDNData     Command = 0x40
	CommandNXDNLost     Command = 0x41
	CommandM17LinkSetup Command = 0x45
	CommandM17Stream    Command = 0x46
	CommandM17Packet    Command = 0x47
	CommandM17Lost      Command = 0x48
	CommandM17EOT       Command = 0x49
	CommandPOCSAGData   Command = 0x50
	CommandFMParams1    Command = 0x60
	CommandFMParams2    Command = 0x61
	CommandFMParams3    Command = 0x62
	CommandFMParams4    Command = 0x63
	CommandFMData       Command = 0x65
	CommandFMStatus     Command = 0x66
	CommandFMEOT        Command = 0x67
	CommandFMRSSI       Command = 0x68
	CommandAck          Command = 0x70
	CommandNak          Command = 0x7F
	CommandSerialData   Command = 0x80
	CommandI2CData      Command = 0x81
	CommandTransparent  Command = 0x90
	CommandQSOInfo      Command = 0x91
	CommandDebug1       Command = 0xF1
	CommandDebug2       Command = 0xF2
	CommandDebug3       Command = 0xF3
	CommandDebug4       Command = 0xF4
	CommandDebug5       Command = 0xF5
	CommandDebugDump    Command = 0xFA
)

// String returns the name of the command. It allocates only for a value
// without a name.
func (c Command) String() string {
	if s, ok := commandName(c); ok {
		return s
	}
	return "Command(" + text.Int(int64(c)) + ")"
}

func commandName(c Command) (string, bool) {
	switch c {
	case CommandGetVersion:
		return "GetVersion", true
	case CommandGetStatus:
		return "GetStatus", true
	case CommandSetConfig:
		return "SetConfig", true
	case CommandSetMode:
		return "SetMode", true
	case CommandSetFreq:
		return "SetFreq", true
	case CommandCalData:
		return "CalData", true
	case CommandRSSIData:
		return "RSSIData", true
	case CommandSendCWID:
		return "SendCWID", true
	case CommandDMRAloha:
		return "DMRAloha", true
	case CommandDMRData1:
		return "DMRData1", true
	case CommandDMRLost1:
		return "DMRLost1", true
	case CommandDMRData2:
		return "DMRData2", true
	case CommandDMRLost2:
		return "DMRLost2", true
	case CommandDMRShortLC:
		return "DMRShortLC", true
	case CommandDMRStart:
		return "DMRStart", true
	case CommandDMRAbort:
		return "DMRAbort", true
	case CommandAck:
		return "Ack", true
	case CommandNak:
		return "Nak", true
	case CommandSerialData:
		return "SerialData", true
	case CommandI2CData:
		return "I2CData", true
	case CommandTransparent:
		return "Transparent", true
	case CommandQSOInfo:
		return "QSOInfo", true
	case CommandDebug1, CommandDebug2, CommandDebug3, CommandDebug4, CommandDebug5:
		return "Debug", true
	case CommandDebugDump:
		return "DebugDump", true
	default:
		return otherModeCommandName(c)
	}
}

func otherModeCommandName(c Command) (string, bool) {
	switch c {
	case CommandDStarHeader:
		return "DStarHeader", true
	case CommandDStarData:
		return "DStarData", true
	case CommandDStarLost:
		return "DStarLost", true
	case CommandDStarEOT:
		return "DStarEOT", true
	case CommandYSFData:
		return "YSFData", true
	case CommandYSFLost:
		return "YSFLost", true
	case CommandP25Header:
		return "P25Header", true
	case CommandP25LDU:
		return "P25LDU", true
	case CommandP25Lost:
		return "P25Lost", true
	case CommandNXDNData:
		return "NXDNData", true
	case CommandNXDNLost:
		return "NXDNLost", true
	case CommandM17LinkSetup:
		return "M17LinkSetup", true
	case CommandM17Stream:
		return "M17Stream", true
	case CommandM17Packet:
		return "M17Packet", true
	case CommandM17Lost:
		return "M17Lost", true
	case CommandM17EOT:
		return "M17EOT", true
	case CommandPOCSAGData:
		return "POCSAGData", true
	case CommandFMParams1, CommandFMParams2, CommandFMParams3, CommandFMParams4:
		return "FMParams", true
	case CommandFMData:
		return "FMData", true
	case CommandFMStatus:
		return "FMStatus", true
	case CommandFMEOT:
		return "FMEOT", true
	case CommandFMRSSI:
		return "FMRSSI", true
	default:
		return "", false
	}
}

// Mode is a mode of the modem, or a calibration state in a status reply.
type Mode uint8

// The Mode values.
const (
	ModeIdle   Mode = 0
	ModeDStar  Mode = 1
	ModeDMR    Mode = 2
	ModeYSF    Mode = 3
	ModeP25    Mode = 4
	ModeNXDN   Mode = 5
	ModePOCSAG Mode = 6
	ModeM17    Mode = 7
	ModeFM     Mode = 10
)

// String returns the name of the mode. It allocates only for a value
// without a name, such as a calibration state.
func (m Mode) String() string {
	switch m {
	case ModeIdle:
		return "Idle"
	case ModeDStar:
		return "D-Star"
	case ModeDMR:
		return "DMR"
	case ModeYSF:
		return "YSF"
	case ModeP25:
		return "P25"
	case ModeNXDN:
		return "NXDN"
	case ModePOCSAG:
		return "POCSAG"
	case ModeM17:
		return "M17"
	case ModeFM:
		return "FM"
	}
	return "Mode(" + text.Int(int64(m)) + ")"
}

// CPU is the processor of a modem of protocol 2.
type CPU uint8

// The CPU values.
const (
	CPUAtmel CPU = 0
	CPUNXP   CPU = 1
	CPUST    CPU = 2
)

// Capabilities is the set of modes that a modem has: octets 4 (low) and 5
// (high) of a protocol 2 version reply.
type Capabilities uint16

// The Capabilities bits.
const (
	CapDStar  Capabilities = 0x0001
	CapDMR    Capabilities = 0x0002
	CapYSF    Capabilities = 0x0004
	CapP25    Capabilities = 0x0008
	CapNXDN   Capabilities = 0x0010
	CapFM     Capabilities = 0x0040
	CapPOCSAG Capabilities = 0x0100
)

// CapabilitiesV1 is what MMDVMHost assumes for a modem of protocol 1, which
// does not send its capabilities.
const CapabilitiesV1 = CapDStar | CapDMR | CapYSF | CapP25 | CapNXDN | CapPOCSAG

// Has reports whether c has every bit of m.
func (c Capabilities) Has(m Capabilities) bool { return c&m == m }
