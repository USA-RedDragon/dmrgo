package trunk

import (
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

// MaxNeighbors is the number of adjacent sites that a Site keeps.
const MaxNeighbors = 8

// Access is the random access control of the last C_ALOHA of a TSCC (ETSI
// TS 102 361-4 clause 6.2.1.1, Table 7.19).
type Access struct {
	// Mask is the number of least significant address bits that must match
	// Address for an MS to make a random access, 0 to 24.
	Mask uint8
	// Address is the MS address of the C_ALOHA, for Mask.
	Address uint32
	// Function shows for which services random access is invited.
	Function enums.ServiceFunction
	// NRandWait is the code for the most TDMA frames that the TSCC may
	// take to answer a random access.
	NRandWait enums.NRandWait
	// Backoff is the code for the TDMA frames from which an MS picks the
	// frame of its next random access.
	Backoff enums.Backoff
}

// Applies reports whether the C_ALOHA population subdivision lets the MS
// with address ms make a random access (ETSI TS 102 361-4 clause
// 6.2.1.1.1). An emergency request is let through for every Mask but 24.
// A Mask of 24 with the address ADRNULL (0) lets no MS through.
func (a *Access) Applies(ms uint32, emergency bool) bool {
	if a == nil {
		return false
	}

	if a.Mask >= 24 {
		return a.Address != 0 && a.Address == ms&0xFFFFFF
	}
	if emergency {
		return true
	}
	m := uint32(1)<<a.Mask - 1
	return a.Address&m == ms&m
}

// Neighbor is an adjacent site that a TSCC announces with C_BCAST
// Adjacent_Site (ETSI TS 102 361-4 clause 7.2.19.7).
type Neighbor struct {
	// SysCode14 is the 14 most significant bits of the C_SYScode of the
	// adjacent TSCC, as pdu.SysCode.Unpack14 takes them.
	SysCode14 uint16
	// Channel is the channel number of the adjacent TSCC.
	Channel uint16
	// HasActiveConn reports whether ActiveConn is known.
	HasActiveConn bool
	// ActiveConn reports whether the adjacent site has a network
	// connection.
	ActiveConn bool
	// ConfirmedPriority is the priority of the confirmed channel.
	ConfirmedPriority uint8
	// AdjacentPriority is the priority of the adjacent channel.
	AdjacentPriority uint8
}

// Site holds the data that a Follower or an MS has about the TSCC it is on,
// from C_ALOHA, C_BCAST and the C_SYS_Parms of the CACH (ETSI TS 102 361-4
// clauses 5.1, 6.3.2 and 7.1.1.1). The zero Site holds no data.
type Site struct {
	// SysCode is the C_SYScode of the TSCC. Par is set only when a C_ALOHA
	// or C_BCAST gave the whole 16 bits.
	SysCode pdu.SysCode
	// HasSysCode reports whether SysCode is known.
	HasSysCode bool
	// ColorCode is the color code of the TSCC.
	ColorCode enums.ColorCode
	// Channel is the channel number of the TSCC, or CHNULL when the
	// consumer tuned to it without a hunt list.
	Channel uint16
	// Reg reports whether the TSCC requires MSs to register.
	Reg bool
	// Version is the version of ETSI TS 102 361-4 that the system follows,
	// from C_ALOHA.
	Version uint8
	// Offset reports whether the TSCC uses offset timing.
	Offset bool
	// ActiveConn reports whether the site has a network connection.
	ActiveConn bool
	// SiteTSSync reports whether all channels of the site are timeslot
	// synchronized.
	SiteTSSync bool
	// Access is the random access control of the last C_ALOHA. HasAccess
	// reports whether one arrived.
	Access Access
	// HasAccess reports whether Access holds a C_ALOHA.
	HasAccess bool
	// SlotCounter is the Common_Slot_Counter of the last C_SYS_Parms,
	// which counts 120 ms steps modulo 512 (ETSI TS 102 361-4 clause 5.1.3).
	SlotCounter uint16
	// Neighbors holds the first NeighborsLen adjacent sites announced.
	Neighbors [MaxNeighbors]Neighbor
	// NeighborsLen is the number of entries of Neighbors in use.
	NeighborsLen int
}
