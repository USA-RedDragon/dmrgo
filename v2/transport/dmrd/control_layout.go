package dmrd

//dmr:frame 8
type binID4 struct {
	Tag [4]byte `dmr:"bits:0-31,packed"`
	ID  uint32  `dmr:"bits:32-63"`
}

//dmr:frame 9
type binID5 struct {
	Tag [5]byte `dmr:"bits:0-39,packed"`
	ID  uint32  `dmr:"bits:40-71"`
}

//dmr:frame 10
type binID6 struct {
	Tag [6]byte `dmr:"bits:0-47,packed"`
	ID  uint32  `dmr:"bits:48-79"`
}

//dmr:frame 11
type binID7 struct {
	Tag [7]byte `dmr:"bits:0-55,packed"`
	ID  uint32  `dmr:"bits:56-87"`
}

//dmr:frame 40
type binIDToken struct {
	Tag   [4]byte  `dmr:"bits:0-31,packed"`
	ID    uint32   `dmr:"bits:32-63"`
	Token [32]byte `dmr:"bits:64-319,packed"`
}

//dmr:frame 12
type hexID4 struct {
	Tag [4]byte `dmr:"bits:0-31,packed"`
	ID  uint32  `dmr:"bits:32-95,hex"`
}

//dmr:frame 13
type hexID5 struct {
	Tag [5]byte `dmr:"bits:0-39,packed"`
	ID  uint32  `dmr:"bits:40-103,hex"`
}

//dmr:frame 14
type hexID6 struct {
	Tag [6]byte `dmr:"bits:0-47,packed"`
	ID  uint32  `dmr:"bits:48-111,hex"`
}

//dmr:frame 15
type hexID7 struct {
	Tag [7]byte `dmr:"bits:0-55,packed"`
	ID  uint32  `dmr:"bits:56-119,hex"`
}

//dmr:frame 22
type hexIDSalt struct {
	Tag  [6]byte `dmr:"bits:0-47,packed"`
	ID   uint32  `dmr:"bits:48-111,hex"`
	Salt uint32  `dmr:"bits:112-175,hex"`
}

//dmr:frame 76
type hexIDToken struct {
	Tag   [4]byte  `dmr:"bits:0-31,packed"`
	ID    uint32   `dmr:"bits:32-95,hex"`
	Token [32]byte `dmr:"bits:96-607,packed,hex"`
}
