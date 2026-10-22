package packet_test

import (
	"bytes"
	"errors"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/packet"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

func TestBursts(t *testing.T) {
	t.Parallel()

	for _, rate := range []enums.DataType{enums.DataTypeRate12, enums.DataTypeRate34, enums.DataTypeRate1} {
		for _, h := range []pdu.DataHeader{unconfirmed(), confirmed()} {
			data := payload(53)
			p, err := packet.NewPacket(h, data, rate)
			if err != nil {
				t.Fatal(err)
			}
			bursts, err := packet.Bursts(&p, 5, layer2.SyncSourceMS)
			if err != nil {
				t.Fatalf("%s: %v", rate, err)
			}
			if len(bursts) != 1+len(p.Blocks) {
				t.Fatalf("%s: %d bursts for %d blocks", rate, len(bursts), len(p.Blocks))
			}
			_, sync, _ := layer2.SyncSourceMS.SyncPatterns()
			a := packet.NewAssembler(127)
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

	p, _ := packet.NewPacket(unconfirmed(), payload(5), enums.DataTypeRate12)
	if _, err := packet.Bursts(&p, 1, layer2.SyncSource(9)); err == nil {
		t.Error("Bursts accepted sync source 9")
	}
	p.Blocks = append(p.Blocks, &pdu.CSBK{})
	if _, err := packet.Bursts(&p, 1, layer2.SyncSourceBS); !errors.Is(err, packet.ErrRate) {
		t.Errorf("CSBK block: %v", err)
	}
	p.Blocks[len(p.Blocks)-1] = nil
	if _, err := packet.Bursts(&p, 1, layer2.SyncSourceBS); err == nil {
		t.Error("Bursts accepted a nil block")
	}
	p.Blocks[len(p.Blocks)-1] = (*pdu.Rate12Data)(nil)
	if _, err := packet.Bursts(&p, 1, layer2.SyncSourceBS); err == nil {
		t.Error("Bursts accepted a nil *pdu.Rate12Data")
	}
	if _, err := packet.Bursts(nil, 1, layer2.SyncSourceBS); !errors.Is(err, dmrgo.ErrNilArgument) {
		t.Errorf("nil Packet: %v", err)
	}
	dst := make([]layer2.RawBurst, 1, 4)
	if out, err := packet.AppendBursts(dst, nil, 1, layer2.SyncSourceBS); !errors.Is(err, dmrgo.ErrNilArgument) || len(out) != 1 {
		t.Errorf("AppendBursts nil Packet: %d bursts, %v", len(out), err)
	}
}
