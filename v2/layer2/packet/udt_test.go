package packet_test

import (
	"bytes"
	"errors"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/packet"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

func crcCCITT(data []byte) uint16 {
	var reg uint16
	for _, b := range data {
		for i := 7; i >= 0; i-- {
			bit := uint16(b>>i) & 1
			if (reg>>15)^bit == 1 {
				reg = reg<<1 ^ 0x1021
			} else {
				reg <<= 1
			}
		}
	}
	return ^reg
}

func udtBlocks(payload []byte, n int) [][12]byte {
	buf := bytes.Repeat([]byte{0xFF}, 12*n)
	copy(buf, payload)
	c := crcCCITT(buf[:12*n-2])
	buf[12*n-2], buf[12*n-1] = byte(c>>8), byte(c)
	out := make([][12]byte, n)
	for i := range out {
		out[i] = [12]byte(buf[12*i:])
	}
	return out
}

func TestSegment_UDT(t *testing.T) {
	t.Parallel()

	base := pdu.UDTHeader{
		Group: true, SAP: enums.SAPShortData, UDTFormat: enums.UDTISO8, LLIDDestination: 0x000102,
		LLIDSource: 0x030405, SupplementaryFlag: false, ProtectFlag: true, Opcode: pdu.UDTOpcodeInboundHeader,
	}
	for _, c := range []struct {
		name   string
		n      int
		pad    uint8
		blocks int
	}{
		{"empty", 0, 20, 1},
		{"one block", 10, 0, 1},
		{"two blocks", 11, 22, 2},
		{"odd pad nibble", 4, 13, 1},
		{"four blocks", 46, 0, 4},
		{"binary fill", 3, 0, 1},
	} {
		h := base
		h.PadNibble, h.UAB = c.pad, 3
		p, err := packet.NewPacket(header(t, (*pdu.DataHeader).SetUDT, h), payload(c.n), enums.DataTypeRate12)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		got := get(t, &p.Header, (*pdu.DataHeader).AsUDT)
		want := base
		want.PadNibble, want.UAB = c.pad, uint8(c.blocks-1)
		if got != want {
			t.Errorf("%s: header %+v, want %+v", c.name, got, want)
		}
		blocks := udtBlocks(payload(c.n), c.blocks)
		if len(p.Blocks) != c.blocks {
			t.Fatalf("%s: %d blocks, want %d", c.name, len(p.Blocks), c.blocks)
		}
		for i, b := range p.Blocks {
			r, ok := b.(*pdu.Rate12Data)
			if !ok || r.Data != blocks[i] {
				t.Errorf("%s: block %d % x, want % x", c.name, i, r.Data, blocks[i])
			}
		}
		if !pdu.MBCLastBlockCRCOK(blocks) {
			t.Errorf("%s: CRC", c.name)
		}
	}
}

func TestSegment_UDTErrors(t *testing.T) {
	t.Parallel()

	h := header(t, (*pdu.DataHeader).SetUDT, pdu.UDTHeader{UDTFormat: enums.UDTISO8, PadNibble: 3})
	for _, c := range []struct {
		name string
		h    pdu.DataHeader
		n    int
		rate enums.DataType
		want error
	}{
		{"rate 3/4", h, 1, enums.DataTypeRate34, packet.ErrRate},
		{"rate 1", h, 1, enums.DataTypeRate1, packet.ErrRate},
		{"too long", h, 47, enums.DataTypeRate12, packet.ErrTooLong},
		{"pad over the payload", h, 10, enums.DataTypeRate12, packet.ErrPadNibble},
	} {
		if _, err := packet.NewPacket(c.h, payload(c.n), c.rate); !errors.Is(err, c.want) {
			t.Errorf("%s: %v, want %v", c.name, err, c.want)
		}
	}
	if _, err := packet.NewPacket(h, payload(9), enums.DataTypeRate12); err != nil {
		t.Errorf("pad of 3 nibbles after 9 octets: %v", err)
	}
	b := packet.NewBuilder(1)
	if _, err := b.Segment(h, payload(11), enums.DataTypeRate12); !errors.Is(err, packet.ErrNoRoom) {
		t.Errorf("no room: %v", err)
	}
	if _, err := b.Segment(h, payload(8), enums.DataTypeRate12); err != nil {
		t.Errorf("one block: %v", err)
	}
	p, err := b.Segment(h, payload(8), enums.DataTypeRate12)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := b.Retry(&p, nil); !errors.Is(err, packet.ErrFormat) {
		t.Errorf("retry: %v", err)
	}
}
