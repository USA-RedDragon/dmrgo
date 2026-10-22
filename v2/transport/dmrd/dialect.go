package dmrd

import "github.com/USA-RedDragon/dmrgo/v2/internal/text"

// Dialect selects the form of the control messages.
type Dialect uint8

// The Dialect values.
const (
	// DialectUnknown is the zero value. No call takes it.
	DialectUnknown Dialect = iota
	// DialectMMDVM is the protocol that DMRGateway speaks to a master and
	// that BrandMeister, DMR+, HBlink, FreeDMR and DMRHub masters accept:
	// binary repeater IDs, RPTACK, RPTPING and MSTPONG, RPTO, RPTG and a
	// 302-octet RPTC with the ID at octets 4 to 7.
	DialectMMDVM
	// DialectHBRP is the HomeBrew Repeater Protocol in the form of the
	// examples of its specification and of BrandMeister's Homebrew port:
	// repeater IDs, salt and token as hex text, MSTACK, MSTPING and RPTPONG,
	// and a 306-octet RPTC with the ID after the callsign.
	DialectHBRP
	// DialectLocal is the link between MMDVMHost and DMRGateway on one host:
	// no login, DMRP and DMRB, DMRT, and DMRA and DMRG without the repeater
	// ID.
	DialectLocal
)

// String returns the name of the dialect. It allocates only for a value
// without a name.
func (d Dialect) String() string {
	switch d {
	case DialectUnknown:
		return "Unknown"
	case DialectMMDVM:
		return "MMDVM"
	case DialectHBRP:
		return "HBRP"
	case DialectLocal:
		return "Local"
	}
	return "Dialect(" + text.Int(int64(d)) + ")"
}

// Command is the kind of a message, named by its tag.
type Command uint8

// The Command values.
const (
	CommandUnknown Command = iota // not a message of the dialect
	CommandDMRD                   // DMR data
	CommandDMRA                   // talker alias
	CommandDMRG                   // GPS position of a radio
	CommandRPTL                   // login request
	CommandRPTACK                 // acknowledgement from the master (DialectMMDVM)
	CommandMSTACK                 // acknowledgement from the master (DialectHBRP)
	CommandRPTK                   // login token
	CommandRPTC                   // configuration
	CommandRPTO                   // options
	CommandRPTPING                // keepalive request from the repeater (DialectMMDVM)
	CommandMSTPONG                // keepalive reply from the master (DialectMMDVM)
	CommandMSTPING                // keepalive request (DialectHBRP)
	CommandRPTPONG                // keepalive reply (DialectHBRP)
	CommandRPTCL                  // the repeater closes the connection
	CommandMSTCL                  // the master closes the connection
	CommandMSTNAK                 // refusal from the master
	CommandRPTSBKN                // the master asks for a site beacon
	CommandRPTG                   // home position of the repeater (DialectMMDVM)
	CommandRPTRSSI                // RSSI report (DialectHBRP)
	CommandRPTINTR                // request to interrupt the incoming call (DialectHBRP)
	CommandDMRP                   // ping between MMDVMHost and DMRGateway
	CommandDMRB                   // DMRGateway asks MMDVMHost for a beacon
	CommandDMRT                   // trunking command between MMDVMHost and DMRGateway
	CommandDMRE                   // FreeDMR OpenBridge data, versions 4 and 5
	CommandBCKA                   // FreeDMR OpenBridge keepalive
	CommandBCSQ                   // FreeDMR OpenBridge squelch of a stream
	CommandBCST                   // FreeDMR OpenBridge stun
	CommandBCVE                   // FreeDMR OpenBridge version
)

// String returns the tag of the command. It allocates only for a value
// without a name.
func (c Command) String() string {
	if c == CommandUnknown {
		return "Unknown"
	}
	if t := c.tag(); t != "" {
		return t
	}
	return "Command(" + text.Int(int64(c)) + ")"
}

func (c Command) tag() string {
	switch c {
	case CommandDMRD:
		return "DMRD"
	case CommandDMRA:
		return "DMRA"
	case CommandDMRG:
		return "DMRG"
	case CommandRPTL:
		return "RPTL"
	case CommandRPTACK:
		return "RPTACK"
	case CommandMSTACK:
		return "MSTACK"
	case CommandRPTK:
		return "RPTK"
	case CommandRPTC:
		return "RPTC"
	case CommandRPTO:
		return "RPTO"
	case CommandRPTPING:
		return "RPTPING"
	case CommandMSTPONG:
		return "MSTPONG"
	case CommandMSTPING:
		return "MSTPING"
	case CommandRPTPONG:
		return "RPTPONG"
	case CommandRPTCL:
		return "RPTCL"
	case CommandMSTCL:
		return "MSTCL"
	case CommandMSTNAK:
		return "MSTNAK"
	case CommandRPTSBKN:
		return "RPTSBKN"
	case CommandRPTG:
		return "RPTG"
	case CommandRPTRSSI:
		return "RPTRSSI"
	case CommandRPTINTR:
		return "RPTINTR"
	case CommandDMRP:
		return "DMRP"
	case CommandDMRB:
		return "DMRB"
	case CommandDMRT:
		return "DMRT"
	case CommandDMRE:
		return "DMRE"
	case CommandBCKA:
		return "BCKA"
	case CommandBCSQ:
		return "BCSQ"
	case CommandBCST:
		return "BCST"
	case CommandBCVE:
		return "BCVE"
	case CommandUnknown:
	}
	return ""
}

type shape uint8

const (
	shapeNone shape = iota
	shapeTag
	shapeText
	shapeBinID
	shapeBinIDText
	shapeBinIDToken
	shapeHexID
	shapeHexIDSalt
	shapeHexIDText
	shapeHexIDToken
)

func (c Command) shape(d Dialect) shape {
	switch d {
	case DialectMMDVM:
		switch c {
		case CommandRPTL, CommandRPTACK, CommandRPTPING, CommandMSTPONG, CommandRPTCL, CommandMSTCL, CommandMSTNAK, CommandRPTSBKN:
			return shapeBinID
		case CommandRPTK:
			return shapeBinIDToken
		case CommandRPTO, CommandRPTG:
			return shapeBinIDText
		case CommandDMRD, CommandDMRA, CommandDMRG, CommandRPTC:
			return shapeNone
		default:
		}
	case DialectHBRP:
		switch c {
		case CommandRPTL, CommandMSTNAK, CommandMSTPING, CommandRPTPONG, CommandRPTCL, CommandMSTCL, CommandRPTSBKN:
			return shapeHexID
		case CommandMSTACK:
			return shapeHexIDSalt
		case CommandRPTK:
			return shapeHexIDToken
		case CommandRPTRSSI, CommandRPTINTR:
			return shapeHexIDText
		case CommandDMRD, CommandRPTC:
			return shapeNone
		default:
		}
	case DialectLocal:
		switch c {
		case CommandDMRP, CommandDMRB:
			return shapeTag
		case CommandDMRT:
			return shapeText
		case CommandDMRD, CommandDMRA, CommandDMRG:
			return shapeNone
		default:
		}
	case DialectUnknown:
	}
	return shapeNone
}

func (c Command) inDialect(d Dialect) bool {
	if c == CommandUnknown || d == DialectUnknown || d > DialectLocal {
		return false
	}
	switch c {
	case CommandDMRD, CommandDMRE, CommandBCKA, CommandBCSQ, CommandBCST, CommandBCVE:
		return true
	case CommandDMRA, CommandDMRG:
		return d != DialectHBRP
	case CommandRPTC:
		return d != DialectLocal
	default:
	}
	return c.shape(d) != shapeNone
}

// Classify returns the command of packet p in dialect d by its tag, or
// CommandUnknown when no tag of d matches. A longer tag wins, so RPTCL is
// not read as RPTC. The OpenBridge tags DMRE, BCKA, BCSQ, BCST and BCVE
// match in every dialect.
func Classify(p []byte, d Dialect) Command {
	best, n := CommandUnknown, 0
	for c := CommandDMRD; c <= CommandBCVE; c++ {
		t := c.tag()
		if len(t) > n && hasTag(p, t) && c.inDialect(d) {
			best, n = c, len(t)
		}
	}
	return best
}

// RepeaterID returns the repeater ID of the message p of dialect d, by its
// command as Classify gives it: the repeater of a DMRD, DMRA or DMRG, the
// ID of an RPTC, and the ID of every control message that carries one. ok
// is false for a message without one, for one that does not decode, and
// for a DialectMMDVM RPTACK, whose 4 octets are the salt when it answers
// RPTL. It does not allocate.
func RepeaterID(p []byte, d Dialect) (id uint32, ok bool) {
	var err error
	switch c := Classify(p, d); c {
	case CommandDMRD:
		var m Data
		m, err = DecodeData(p)
		id = m.Repeater
	case CommandDMRA, CommandDMRG:
		if d == DialectLocal {
			return 0, false
		}
		if c == CommandDMRA {
			var m TalkerAlias
			m, err = DecodeTalkerAlias(p)
			id = m.Repeater
		} else {
			var m Position
			m, err = DecodePosition(p)
			id = m.Repeater
		}
	case CommandRPTC:
		if d == DialectHBRP {
			var c *HBRPConfig
			if c, err = HBRPConfigOf(p); err == nil {
				id, err = c.ID()
			}
		} else {
			var c *MMDVMConfig
			if c, err = MMDVMConfigOf(p); err == nil {
				id = c.ID()
			}
		}
	case CommandRPTACK, CommandUnknown, CommandDMRP, CommandDMRB, CommandDMRT, CommandDMRE, CommandBCKA, CommandBCSQ, CommandBCST, CommandBCVE:
		return 0, false
	default:
		var m Control
		m, err = DecodeControl(p, d)
		id = m.ID
	}
	if err != nil {
		return 0, false
	}
	return id, true
}

func hasTag(p []byte, t string) bool {
	if len(p) < len(t) {
		return false
	}
	for i := range len(t) {
		if p[i] != t[i] {
			return false
		}
	}
	return true
}
