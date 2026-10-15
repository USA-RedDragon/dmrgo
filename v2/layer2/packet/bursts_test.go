package packet_test

import (
	"bytes"
	"errors"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/packet"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

func TestBursts(t *testing.T) {
	for _, rate := range []enums.DataType{enums.DataTypeRate12, enums.DataTypeRate34, enums.DataTypeRate1} {
		for _, h := range []pdu.DataHeader{unconfirmed(), confirmed()} {
			data := payload(53)
			p, err := packet.Segment(h, data, rate)
			if err != nil {
				t.Fatal(err)
			}
			bursts, err := packet.Bursts(p, 5, layer2.SyncSourceMS)
			if err != nil {
				t.Fatalf("%s: %v", rate, err)
			}
			if len(bursts) != 1+len(p.Blocks) {
				t.Fatalf("%s: %d bursts for %d blocks", rate, len(bursts), len(p.Blocks))
			}
			_, sync, _ := layer2.SyncSourceMS.SyncPatterns()
			var a packet.Assembler
			var got packet.Message
			var done bool
			for i, raw := range bursts {
				b, err := layer2.NewBurstFromBytes(raw)
				if err != nil {
					t.Fatalf("%s burst %d: %v", rate, i, err)
				}
				if b.SyncPattern != sync || b.SlotType.ColorCode != 5 || b.FEC.Aggregate().ErrorsCorrected != 0 {
					t.Fatalf("%s burst %d: sync %s, slot type %+v, FEC %+v", rate, i, b.SyncPattern, b.SlotType, b.FEC)
				}
				enc, err := b.Encode()
				if err != nil || enc != raw {
					t.Fatalf("%s burst %d: Burst.Encode differs: %v", rate, i, err)
				}
				got, done, err = a.AddBurst(b)
				if err != nil {
					t.Fatalf("%s burst %d: %v", rate, i, err)
				}
			}
			if !done || !bytes.Equal(got.Data, data) {
				t.Errorf("%s: message %v %x, want %x", rate, done, got.Data, data)
			}
		}
	}

	p, _ := packet.Segment(unconfirmed(), payload(5), enums.DataTypeRate12)
	if _, err := packet.Bursts(p, 16, layer2.SyncSourceBS); err == nil {
		t.Error("Bursts accepted color code 16")
	}
	p.Blocks = append(p.Blocks, &pdu.CSBK{})
	if _, err := packet.Bursts(p, 1, layer2.SyncSourceBS); !errors.Is(err, packet.ErrRate) {
		t.Errorf("CSBK block: %v", err)
	}
}
