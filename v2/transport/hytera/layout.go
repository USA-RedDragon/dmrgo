package hytera

//dmr:frame 72
type frameLayout struct {
	Port        uint16   `dmr:"bits:0-15"`
	Mark        uint16   `dmr:"bits:16-31"`
	Seq         uint8    `dmr:"bits:32-39"`
	R5          [3]byte  `dmr:"bits:40-63,packed"`
	PacketType  uint8    `dmr:"bits:64-71"`
	R9          [7]byte  `dmr:"bits:72-127,packed"`
	Slot        uint16   `dmr:"bits:128-143"`
	SlotType    uint16   `dmr:"bits:144-159"`
	ColorCode   uint16   `dmr:"bits:160-175"`
	FrameType   uint16   `dmr:"bits:176-191"`
	R24         [2]byte  `dmr:"bits:192-207,packed"`
	Payload     [34]byte `dmr:"bits:208-479,packed,swap16"`
	R60         [2]byte  `dmr:"bits:480-495,packed"`
	CallType    uint8    `dmr:"bits:496-503"`
	R63         uint8    `dmr:"bits:504-511"`
	Destination uint32   `dmr:"bits:512-535,le"`
	R67         uint8    `dmr:"bits:536-543"`
	Source      uint32   `dmr:"bits:544-567,le"`
	R71         uint8    `dmr:"bits:568-575"`
}

//dmr:frame 20
//dmr:magic 0-31="ZZZZ"
type heartbeatLayout struct {
	Code   uint8   `dmr:"bits:32-39"`
	R5     [7]byte `dmr:"bits:40-95,packed"`
	Seq    uint8   `dmr:"bits:96-103"`
	Status [3]byte `dmr:"bits:104-127,packed"`
	R16    [4]byte `dmr:"bits:128-159,packed"`
}

//dmr:frame 20
//dmr:magic 0-23="P2P"
type p2pLayout struct {
	Op     uint8   `dmr:"bits:24-31"`
	Code   uint8   `dmr:"bits:32-39"`
	R5     [7]byte `dmr:"bits:40-95,packed"`
	Seq    uint8   `dmr:"bits:96-103"`
	Status [3]byte `dmr:"bits:104-127,packed"`
	R16    [4]byte `dmr:"bits:128-159,packed"`
}
