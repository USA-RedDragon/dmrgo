package dmrd

import (
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
)

//dmr:frame 85
//dmr:magic 0-31="DMRE"
type dmreV4 struct {
	Seq          uint8           `dmr:"bits:32-39"`
	Source       uint32          `dmr:"bits:40-63"`
	Destination  uint32          `dmr:"bits:64-87"`
	Repeater     uint32          `dmr:"bits:88-119"`
	Slot         enums.Slot      `dmr:"bits:120,enum,bias:1"`
	Private      bool            `dmr:"bit:121"`
	Frame        FrameType       `dmr:"bits:122-123,enum"`
	Value        uint8           `dmr:"bits:124-127"`
	Stream       uint32          `dmr:"bits:128-159"`
	Burst        layer2.RawBurst `dmr:"bits:160-423,packed"`
	BER          uint8           `dmr:"bits:424-431"`
	RSSI         uint8           `dmr:"bits:432-439"`
	Version      uint8           `dmr:"bits:440-447"`
	Time         uint64          `dmr:"bits:448-511"`
	SourceServer uint32          `dmr:"bits:512-543"`
	Hops         uint8           `dmr:"bits:544-551"`
	Hash         [16]byte        `dmr:"bits:552-679,packed"`
}

//dmr:frame 89
//dmr:magic 0-31="DMRE"
type dmreV5 struct {
	Seq            uint8           `dmr:"bits:32-39"`
	Source         uint32          `dmr:"bits:40-63"`
	Destination    uint32          `dmr:"bits:64-87"`
	Repeater       uint32          `dmr:"bits:88-119"`
	Slot           enums.Slot      `dmr:"bits:120,enum,bias:1"`
	Private        bool            `dmr:"bit:121"`
	Frame          FrameType       `dmr:"bits:122-123,enum"`
	Value          uint8           `dmr:"bits:124-127"`
	Stream         uint32          `dmr:"bits:128-159"`
	Burst          layer2.RawBurst `dmr:"bits:160-423,packed"`
	BER            uint8           `dmr:"bits:424-431"`
	RSSI           uint8           `dmr:"bits:432-439"`
	Version        uint8           `dmr:"bits:440-447"`
	Time           uint64          `dmr:"bits:448-511"`
	SourceServer   uint32          `dmr:"bits:512-543"`
	SourceRepeater uint32          `dmr:"bits:544-575"`
	Hops           uint8           `dmr:"bits:576-583"`
	Hash           [16]byte        `dmr:"bits:584-711,packed"`
}

//dmr:frame 24
type bcTag struct {
	Tag       [4]byte  `dmr:"bits:0-31,packed"`
	Signature [20]byte `dmr:"bits:32-191,packed"`
}

//dmr:frame 31
//dmr:magic 0-31="BCSQ"
type bcsq struct {
	TalkGroup uint32   `dmr:"bits:32-55"`
	Stream    uint32   `dmr:"bits:56-87"`
	Signature [20]byte `dmr:"bits:88-247,packed"`
}

//dmr:frame 25
//dmr:magic 0-31="BCVE"
type bcve struct {
	Version   uint8    `dmr:"bits:32-39"`
	Signature [20]byte `dmr:"bits:40-199,packed"`
}
