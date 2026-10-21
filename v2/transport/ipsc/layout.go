package ipsc

//dmr:frame 5
type ctlID struct {
	Opcode Opcode `dmr:"bits:0-7,enum"`
	ID     uint32 `dmr:"bits:8-39"`
}

//dmr:frame 9
type ctlVersion struct {
	Opcode  Opcode  `dmr:"bits:0-7,enum"`
	ID      uint32  `dmr:"bits:8-39"`
	Version [4]byte `dmr:"bits:40-71,packed"`
}

//dmr:frame 10
type ctlModeFlags struct {
	Opcode Opcode  `dmr:"bits:0-7,enum"`
	ID     uint32  `dmr:"bits:8-39"`
	Mode   Mode    `dmr:"bits:40-47"`
	Flags  [4]byte `dmr:"bits:48-79,packed"`
}

//dmr:frame 14
type ctlFull struct {
	Opcode  Opcode  `dmr:"bits:0-7,enum"`
	ID      uint32  `dmr:"bits:8-39"`
	Mode    Mode    `dmr:"bits:40-47"`
	Flags   [4]byte `dmr:"bits:48-79,packed"`
	Version [4]byte `dmr:"bits:80-111,packed"`
}

//dmr:frame 16
type ctlRegisterReply struct {
	Opcode  Opcode  `dmr:"bits:0-7,enum"`
	ID      uint32  `dmr:"bits:8-39"`
	Mode    Mode    `dmr:"bits:40-47"`
	Flags   [4]byte `dmr:"bits:48-79,packed"`
	Peers   uint16  `dmr:"bits:80-95"`
	Version [4]byte `dmr:"bits:96-127,packed"`
}

//dmr:frame 11
type peerEntry struct {
	ID   uint32  `dmr:"bits:0-31"`
	IP   [4]byte `dmr:"bits:32-63,packed"`
	Port uint16  `dmr:"bits:64-79"`
	Mode Mode    `dmr:"bits:80-87"`
}

//dmr:frame 31
type userHeader struct {
	Opcode      Opcode   `dmr:"bits:0-7,enum"`
	ID          uint32   `dmr:"bits:8-39"`
	CallSeq     uint8    `dmr:"bits:40-47"`
	Source      uint32   `dmr:"bits:48-71"`
	Destination uint32   `dmr:"bits:72-95"`
	Priority    uint8    `dmr:"bits:96-103"`
	CallControl uint32   `dmr:"bits:104-135"`
	Info        CallInfo `dmr:"bits:136-143"`
	RTPFlags    uint8    `dmr:"bits:144-151"`
	Marker      bool     `dmr:"bit:152"`
	PayloadType uint8    `dmr:"bits:153-159"`
	Seq         uint16   `dmr:"bits:160-175"`
	Timestamp   uint32   `dmr:"bits:176-207"`
	SSRC        uint32   `dmr:"bits:208-239"`
	Burst       uint8    `dmr:"bits:240-247"`
}

//dmr:frame 21
type voiceA struct {
	Length  uint8    `dmr:"bits:0-7"`
	Control uint8    `dmr:"bits:8-15"`
	AMBE    [19]byte `dmr:"bits:16-167,packed"`
}

//dmr:frame 26
type voiceBCDF struct {
	Length   uint8    `dmr:"bits:0-7"`
	Control  uint8    `dmr:"bits:8-15"`
	AMBE     [19]byte `dmr:"bits:16-167,packed"`
	Embedded [4]byte  `dmr:"bits:168-199,packed"`
	EMB      uint8    `dmr:"bits:200-207"`
}

//dmr:frame 35
type voiceE struct {
	Length   uint8    `dmr:"bits:0-7"`
	Control  uint8    `dmr:"bits:8-15"`
	AMBE     [19]byte `dmr:"bits:16-167,packed"`
	Embedded [4]byte  `dmr:"bits:168-199,packed"`
	LC       [9]byte  `dmr:"bits:200-271,packed"`
	EMB      uint8    `dmr:"bits:272-279"`
}

//dmr:frame 3
type dataHead struct {
	Status uint8  `dmr:"bits:0-7"`
	Words  uint16 `dmr:"bits:8-23"`
}
