package dmrd

// TalkerAlias is a DMRA message of DialectMMDVM: one talker alias block that
// a repeater heard, in the 19-octet form that DMRGateway builds toward a
// master. DMRGateway ee79d38 sends only the first 15 octets of it, with the
// first 3 octets of Data; Size shows which form came.
//
//dmr:frame 15 19
//dmr:magic 0-31="DMRA"
type TalkerAlias struct {
	// Repeater is the repeater ID.
	Repeater uint32 `dmr:"bits:32-63"`
	// Source is the radio ID of the talker (24 bits).
	Source uint32 `dmr:"bits:64-87"`
	// Type is the talker alias block: 0 for the header, 1 to 3 for the blocks.
	Type uint8 `dmr:"bits:88-95"`
	// Data is the 7 octets of the block, after its FLCO and FID octets (ETSI TS 102 361-2 clauses 7.1.1.4 and 7.1.1.5).
	// A 15-octet packet holds the first 3, and the others are 0.
	Data [7]byte `dmr:"bits:96-151,packed,tail"`
	// Size is the length of the packet: 15 or 19, or 0 for 15.
	Size uint8 `dmr:"-"`
}

// LocalTalkerAlias is a DMRA message of DialectLocal, from MMDVMHost to
// DMRGateway: TalkerAlias without the repeater ID. It has the length of the
// short TalkerAlias, so decode a DMRA that a master receives with
// DecodeTalkerAlias, which reads the repeater ID.
//
//dmr:frame 15
//dmr:magic 0-31="DMRA"
type LocalTalkerAlias struct {
	// Source is the radio ID of the talker (24 bits).
	Source uint32 `dmr:"bits:32-55"`
	// Type is the talker alias block: 0 for the header, 1 to 3 for the blocks.
	Type uint8 `dmr:"bits:56-63"`
	// Data is the 7 octets of the block, after its FLCO and FID octets.
	Data [7]byte `dmr:"bits:64-119,packed"`
}

// Position is a DMRG message of DialectMMDVM: the GPS position that a radio
// sent, in the 18-octet form that DMRGateway builds toward a master.
// DMRGateway ee79d38 sends only the first 14 octets of it, with the first 3
// octets of Data; Size shows which form came.
//
//dmr:frame 14 18
//dmr:magic 0-31="DMRG"
type Position struct {
	// Repeater is the repeater ID.
	Repeater uint32 `dmr:"bits:32-63"`
	// Source is the radio ID (24 bits).
	Source uint32 `dmr:"bits:64-87"`
	// Data is the 7 octets of the GPS Info LC after its FLCO and FID octets (ETSI TS 102 361-2 clause 7.1.1.3).
	// A 14-octet packet holds the first 3, and the others are 0.
	Data [7]byte `dmr:"bits:88-143,packed,tail"`
	// Size is the length of the packet: 14 or 18, or 0 for 14.
	Size uint8 `dmr:"-"`
}

// LocalPosition is a DMRG message of DialectLocal, from MMDVMHost to
// DMRGateway: Position without the repeater ID. It has the length of the
// short Position, so decode a DMRG that a master receives with
// DecodePosition.
//
//dmr:frame 14
//dmr:magic 0-31="DMRG"
type LocalPosition struct {
	// Source is the radio ID (24 bits).
	Source uint32 `dmr:"bits:32-55"`
	// Data is the 7 octets of the GPS Info LC after its FLCO and FID octets.
	Data [7]byte `dmr:"bits:56-111,packed"`
}
