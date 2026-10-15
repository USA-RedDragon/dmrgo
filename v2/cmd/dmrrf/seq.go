package main

import (
	"encoding/binary"
	"fmt"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/packet"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

// SeqConfig describes a transmission.
type SeqConfig struct {
	ColorCode uint8
	Source    uint32
	Dest      uint32
	BS        uint32
	Group     bool
	Wakeups   int
	Message   string
}

func putAddr(b []byte, a uint32) {
	var t [4]byte
	binary.BigEndian.PutUint32(t[:], a)
	copy(b, t[1:])
}

// WakeupBurst returns an MS-sourced BS_Dwn_Act CSBK burst (ETSI TS 102 361-2
// clause 7.1.2.1, Table 7.5a) that activates a BS outbound.
func WakeupBurst(cfg SeqConfig) ([33]byte, error) {
	var c pdu.CSBKBytes
	c[0] = 0x80 | 0x38
	putAddr(c[4:7], cfg.BS)
	putAddr(c[7:10], cfg.Source)
	c.Reseal()
	r, err := layer2.BuildDataBurst([12]byte(c), enums.DataTypeCSBK, cfg.ColorCode, layer2.SyncSourceMS)
	return [33]byte(r), err
}

func rateOf(name string) (enums.DataType, error) {
	switch name {
	case "data12":
		return enums.DataTypeRate12, nil
	case "data34":
		return enums.DataTypeRate34, nil
	case "data1":
		return enums.DataTypeRate1, nil
	}
	return 0, fmt.Errorf("unknown data sequence %q", name)
}

// DataPacket segments the test message into a confirmed packet at the rate
// the sequence name selects.
func DataPacket(name string, cfg SeqConfig) (packet.Packet, error) {
	rate, err := rateOf(name)
	if err != nil {
		return packet.Packet{}, err
	}
	h := pdu.DataHeader{
		Format: pdu.FormatConfirmed,
		ConfirmedDataHeader: &pdu.ConfirmedDataHeader{
			Group:             cfg.Group,
			ResponseRequested: !cfg.Group,
			SAP:               enums.SAPShortData,
			LLIDDestination:   cfg.Dest,
			LLIDSource:        cfg.Source,
			FullMessageFlag:   true,
		},
	}
	return packet.Segment(h, []byte(cfg.Message), rate)
}

// Sequence builds the bursts of a named transmission: "wakeup" sends only
// wakeup CSBKs, "data12", "data34" and "data1" send wakeups followed by a
// confirmed data packet at that rate.
func Sequence(name string, cfg SeqConfig) ([][33]byte, error) {
	wake, err := WakeupBurst(cfg)
	if err != nil {
		return nil, err
	}
	out := make([][33]byte, 0, cfg.Wakeups)
	for range cfg.Wakeups {
		out = append(out, wake)
	}
	if name == "wakeup" {
		return out, nil
	}
	p, err := DataPacket(name, cfg)
	if err != nil {
		return nil, err
	}
	bursts, err := packet.Bursts(p, cfg.ColorCode, layer2.SyncSourceMS)
	if err != nil {
		return nil, err
	}
	return append(out, bursts...), nil
}
