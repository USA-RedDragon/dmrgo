package ipsc

import (
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/internal/text"
)

// Opcode is octet 0 of an IPSC packet.
type Opcode uint8

// The Opcode values.
const (
	OpCallConfirmation    Opcode = 0x05 // confirmation from the receiver of a confirmed call
	OpTextMessageAck      Opcode = 0x54 // text message acknowledgement
	OpCallMonitorStatus   Opcode = 0x61 // call monitor status
	OpCallMonitorRepeat   Opcode = 0x62 // call monitor repeat
	OpCallMonitorNack     Opcode = 0x63 // call monitor NACK
	OpXNL                 Opcode = 0x70 // XCMP/XNL control message
	OpGroupVoice          Opcode = 0x80 // group voice
	OpPrivateVoice        Opcode = 0x81 // private voice
	OpGroupData           Opcode = 0x83 // group data
	OpPrivateData         Opcode = 0x84 // private data
	OpWakeup              Opcode = 0x85 // repeater wake-up
	OpCollision           Opcode = 0x86 // collision of two transmitters
	OpRegisterRequest     Opcode = 0x90 // a peer registers with the master
	OpRegisterReply       Opcode = 0x91 // the master accepts a registration
	OpPeerListRequest     Opcode = 0x92 // a peer asks for the peer list
	OpPeerListReply       Opcode = 0x93 // the master sends the peer list
	OpPeerRegisterRequest Opcode = 0x94 // a peer registers with another peer
	OpPeerRegisterReply   Opcode = 0x95 // a peer accepts a registration
	OpAliveRequest        Opcode = 0x96 // keepalive from a peer to the master
	OpAliveReply          Opcode = 0x97 // keepalive reply from the master
	OpPeerAliveRequest    Opcode = 0x98 // keepalive between peers
	OpPeerAliveReply      Opcode = 0x99 // keepalive reply between peers
	OpDeregisterRequest   Opcode = 0x9A // a peer leaves
	OpDeregisterReply     Opcode = 0x9B // the leave is accepted
	OpStatus              Opcode = 0xF0 // sent by an XPR 8400 after each keepalive; meaning not known
)

// String returns the name of the opcode. It allocates only for a value
// without a name.
func (o Opcode) String() string {
	switch o {
	case OpCallConfirmation:
		return "CallConfirmation"
	case OpTextMessageAck:
		return "TextMessageAck"
	case OpCallMonitorStatus:
		return "CallMonitorStatus"
	case OpCallMonitorRepeat:
		return "CallMonitorRepeat"
	case OpCallMonitorNack:
		return "CallMonitorNack"
	case OpXNL:
		return "XNL"
	case OpGroupVoice:
		return "GroupVoice"
	case OpPrivateVoice:
		return "PrivateVoice"
	case OpGroupData:
		return "GroupData"
	case OpPrivateData:
		return "PrivateData"
	case OpWakeup:
		return "Wakeup"
	case OpCollision:
		return "Collision"
	case OpRegisterRequest:
		return "RegisterRequest"
	case OpRegisterReply:
		return "RegisterReply"
	case OpPeerListRequest:
		return "PeerListRequest"
	case OpPeerListReply:
		return "PeerListReply"
	case OpPeerRegisterRequest:
		return "PeerRegisterRequest"
	case OpPeerRegisterReply:
		return "PeerRegisterReply"
	case OpAliveRequest:
		return "AliveRequest"
	case OpAliveReply:
		return "AliveReply"
	case OpPeerAliveRequest:
		return "PeerAliveRequest"
	case OpPeerAliveReply:
		return "PeerAliveReply"
	case OpDeregisterRequest:
		return "DeregisterRequest"
	case OpDeregisterReply:
		return "DeregisterReply"
	case OpStatus:
		return "Status"
	}
	return "Opcode(" + text.Int(int64(o)) + ")"
}

// Mode is the mode octet of a peer: operational, analog or digital, and the
// link state of each slot.
type Mode uint8

// Operational reports bit 6, the peer is operational.
func (m Mode) Operational() bool { return m&0x40 != 0 }

// Digital reports that bits 5 and 4 say digital.
func (m Mode) Digital() bool { return m&0x30 == 0x20 }

// Analog reports that bits 5 and 4 say analog.
func (m Mode) Analog() bool { return m&0x30 == 0x10 }

// TS1 returns the 2-bit link state of slot 1, bits 3 and 2.
func (m Mode) TS1() uint8 { return uint8(m>>2) & 3 }

// TS2 returns the 2-bit link state of slot 2, bits 1 and 0.
func (m Mode) TS2() uint8 { return uint8(m) & 3 }

// Flag is one bit of the 4 flag octets of a peer.
type Flag uint8

// The Flag values: octet index in the high nibble, bit in the low.
const (
	FlagCSBK            Flag = 0x27 // octet 2 bit 7: CSBK calls
	FlagRepeaterMonitor Flag = 0x26 // octet 2 bit 6: repeater call monitoring
	FlagConsoleApp      Flag = 0x25 // octet 2 bit 5: console application
	FlagXNLStatus       Flag = 0x37 // octet 3 bit 7: XNL connected
	FlagXNLMaster       Flag = 0x36 // octet 3 bit 6: XNL master
	FlagXNLSlave        Flag = 0x35 // octet 3 bit 5: XNL slave
	FlagAuth            Flag = 0x34 // octet 3 bit 4: packets carry an HMAC-SHA1 trailer
	FlagData            Flag = 0x33 // octet 3 bit 3: data calls
	FlagVoice           Flag = 0x32 // octet 3 bit 2: voice calls
	FlagMasterPeer      Flag = 0x30 // octet 3 bit 0: the master
)

// Flags is the 4 flag octets of a peer.
type Flags [4]byte

// Has reports whether flag f is set.
func (f Flags) Has(flag Flag) bool {
	return f[flag>>4&3]&(1<<(flag&7)) != 0
}

// Set sets or clears flag f.
func (f *Flags) Set(flag Flag, on bool) {
	if on {
		f[flag>>4&3] |= 1 << (flag & 7)
		return
	}
	f[flag>>4&3] &^= 1 << (flag & 7)
}

// CallInfo is octet 17 of a user packet.
type CallInfo uint8

// Privacy reports bit 7: the call is encrypted.
func (c CallInfo) Privacy() bool { return c&0x80 != 0 }

// End reports bit 6, the end flag. On voice bursts MOTOTRBO repeaters also
// set it in the middle of a weak call.
func (c CallInfo) End() bool { return c&0x40 != 0 }

// Slot returns Slot2 for bit 5 and Slot1 otherwise.
func (c CallInfo) Slot() enums.Slot {
	if c&0x20 != 0 {
		return enums.Slot2
	}
	return enums.Slot1
}

// BurstType is octet 30 of a user packet: the DMR data type for a data
// burst, BurstVoice for a voice burst, BurstRaw for the raw information
// bits of a burst that the repeater could not decode, and BurstSyncLost for
// a placeholder of a burst that it lost. Bit 7 is slot 2 on voice and
// BurstSyncLost.
type BurstType uint8

// The BurstType values other than the data types 0x00 to 0x08.
const (
	BurstVoice    BurstType = 0x0A
	BurstSyncLost BurstType = 0x13
	BurstRaw      BurstType = 0x1F
)

// VoiceLayout is the form of a voice packet, which tells some superframe
// positions.
type VoiceLayout uint8

// The VoiceLayout values.
const (
	VoiceLayoutA    VoiceLayout = iota + 1 // 52 octets: burst A, the AMBE frames only
	VoiceLayoutBCDF                        // 57 octets: bursts B, C, D and F, with embedded data and EMB
	VoiceLayoutE                           // 66 octets: burst E, also with the full LC
)
