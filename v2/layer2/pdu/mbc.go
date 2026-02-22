package pdu

import (
	"github.com/USA-RedDragon/dmrgo/v2/bit"
	"github.com/USA-RedDragon/dmrgo/v2/fec"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/elements"
)

// dmr:crc crc_ccitt
// dmr:crc_mask 0xAAAA
// dmr:input_size 96
// ETSI TS 102 361-1 - 7.4.1 Multi Block Control (MBC) Header
type MBCHeader struct {
	DataType    elements.DataType `dmr:"-"`
	crc         uint16            `dmr:"-"` //nolint:unused
	FEC         fec.FECResult     `dmr:"-"`
	LastBlock   bool              `dmr:"bit:0"`
	ProtectFlag bool              `dmr:"bit:1"`
	CSBKOpcode  CSBKOpcode        `dmr:"bits:2-7"`
	FID         byte              `dmr:"bits:8-15"`
	Data        [64]bit.Bit       `dmr:"bits:16-79,raw"`
}

func (m *MBCHeader) GetDataType() elements.DataType {
	return m.DataType
}

// ETSI TS 102 361-1 - 7.4.1 Multi Block Control (MBC) Continuation
//
// The continuation block carries either intermediate or last-block data.
// LB=0: intermediate block (95 data bits, no CRC)
// LB=1: last block (79 data bits + 16-bit CRC over all continuation block data)
//
// NOTE: The last-block CRC covers data across ALL continuation blocks
// (intermediate + last), so it cannot be verified in a single-burst decode.
// The CRC-CCITT of the last block has NO mask per §B.3.12 NOTE 1.
type MBCContinuation struct {
	DataType elements.DataType `dmr:"-"`
	FEC      fec.FECResult     `dmr:"-"`

	// LastBlock indicates whether this is the final continuation block.
	// LB=1 means last block (has CRC), LB=0 means intermediate (no CRC).
	LastBlock bool `dmr:"bit:0"`

	// Data holds the continuation block's data payload.
	// For intermediate blocks (LB=0): 95 data bits (bits 1-95).
	// For last blocks (LB=1): 79 data bits (bits 1-79) + 16-bit CRC (bits 80-95).
	// We store the raw 95 bits and let higher layers interpret them.
	Data [95]bit.Bit `dmr:"bits:1-95,raw"`
}

func (m *MBCContinuation) GetDataType() elements.DataType {
	return m.DataType
}
