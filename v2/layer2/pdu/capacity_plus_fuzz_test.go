package pdu_test

import (
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

const (
	maxCapPlusOps = 32
	maxCapPlusRaw = 128
)

func FuzzCapacityPlus(f *testing.F) {
	pool := capPlusPool(f)
	capPlusSeeds(pool, func(ops, raw []byte) { f.Add(ops, raw) })
	f.Fuzz(func(t *testing.T, ops, raw []byte) { fuzzCapacityPlus(t, pool, ops, raw) })
}

func capPlusPool(tb testing.TB) [][8]byte {
	tb.Helper()
	var pool [][8]byte
	for _, v := range capPlusVectors(tb) {
		if v.kind == "csbk" {
			pool = append(pool, [8]byte(v.octets[2:10]))
		}
	}
	if len(pool) == 0 || len(pool) > 0x80 {
		tb.Fatalf("%d CSBKs in the pool", len(pool))
	}
	return pool
}

func capPlusSeeds(pool [][8]byte, add func(ops, raw []byte)) {
	all := make([]byte, 0, len(pool))
	for i := range pool {
		add([]byte{byte(i)}, nil)
		all = append(all, byte(i))
	}
	add(all, nil)
	add([]byte{0x80, 0x80, 0x80}, []byte{0x80, 0xFF, 1, 2, 3, 4, 5, 6, 0x00, 7, 8, 9, 10, 11, 12, 13, 0x40, 0x90, 0xFF, 0, 0, 0, 0, 0})
}

func fuzzCapacityPlus(t *testing.T, pool [][8]byte, ops, data []byte) {
	ops = ops[:min(len(ops), maxCapPlusOps)]
	data = data[:min(len(data), maxCapPlusRaw)]
	var asm pdu.CapacityPlusSiteStatusAssembler
	for _, op := range ops {
		raw := pool[int(op)%len(pool)]
		if op&0x80 != 0 {
			raw = [8]byte{}
			copy(raw[:], data)
			data = data[min(8, len(data)):]
		}
		capPlusChunk(t, raw, &asm)
	}
}

func capPlusChunk(t *testing.T, raw [8]byte, asm *pdu.CapacityPlusSiteStatusAssembler) {
	t.Helper()
	for _, op := range []pdu.CSBKOpcode{pdu.CSBKCapacityPlusSiteStatus, pdu.CSBKCapacityPlusNeighbors, pdu.CSBKCapacityPlusPreamble, pdu.CSBKCapacityPlusDataRevertAnnouncement, pdu.CSBKCapacityPlusDataRevertGrant} {
		c := pdu.CSBK{LastBlock: true, Opcode: op, FID: enums.FIDMotorola, RawPayload: raw}
		back := pdu.DecodeCSBK(must(c.Encode()))
		if back.RawPayload != raw {
			t.Fatalf("opcode %#x: payload %x, want %x", op, back.RawPayload, raw)
		}
		checkCapPlusPayload(t, &back, raw, asm)
	}
	var lcRaw [7]byte
	copy(lcRaw[:], raw[1:])
	lc := pdu.FullLinkControl{FLCO: pdu.FLCOCapacityPlusGroupVoiceGrant, FID: enums.FIDMotorola, RawPayload: lcRaw}
	if g, ok := lc.AsCapacityPlusGroupVoiceGrant(); !ok || must(pdu.EncodePayload(&g)) != lcRaw {
		t.Fatalf("group grant %x does not round trip", lcRaw)
	}
	lc.FLCO = pdu.FLCOCapacityPlusPrivateVoiceGrant
	if g, ok := lc.AsCapacityPlusPrivateVoiceGrant(); !ok || must(pdu.EncodePayload(&g)) != lcRaw {
		t.Fatalf("private grant %x does not round trip", lcRaw)
	}
	s := pdu.ShortLC{SLCO: pdu.SLCOCapacityPlusRestChannel, RawPayload: [3]byte{raw[0], raw[1], raw[2]}}
	r, ok := s.AsCapacityPlusRestChannel()
	if ok != (raw[0] == 0x10) || ok && must(pdu.EncodePayload(&r)) != s.RawPayload {
		t.Fatalf("rest channel %x: %+v %t", s.RawPayload, r, ok)
	}
}

func checkCapPlusPayload(t *testing.T, c *pdu.CSBK, raw [8]byte, asm *pdu.CapacityPlusSiteStatusAssembler) {
	t.Helper()
	if p, ok := c.AsCapacityPlusSiteStatus(); ok {
		if must(pdu.EncodePayload(&p)) != raw {
			t.Fatalf("site status %x does not round trip", raw)
		}
		if done, err := asm.Add(&p); done && err == nil {
			act, _ := asm.Activity()
			for lsn := uint8(0); lsn <= pdu.CapacityPlusMaxLSN+1; lsn++ {
				_, okT := act.Talkgroup(lsn)
				_, okR := act.RadioTarget(lsn)
				inRange := lsn >= 1 && lsn <= pdu.CapacityPlusMaxLSN
				if okT != (inRange && act.Voice&(1<<(lsn-1)) != 0) || okR != (inRange && act.Radio&(1<<(lsn-1)) != 0) {
					t.Fatalf("LSN %d: %t %t for %+v", lsn, okT, okR, act)
				}
			}
			if act.Radio != 0 && act.Options&0x80 == 0 {
				t.Fatalf("radios without the options flag: %+v", act)
			}
		}
	}
	if p, ok := c.AsCapacityPlusNeighbors(); ok {
		if must(pdu.EncodePayload(&p)) != raw {
			t.Fatalf("neighbors %x does not round trip", raw)
		}
		for i := range 7 {
			if _, _, ok := p.Neighbor(i); ok != (i < int(p.Count) && i < 6) {
				t.Fatalf("Neighbor(%d) %t with Count %d", i, ok, p.Count)
			}
		}
	}
	if p, ok := c.AsCapacityPlusPreamble(); ok && must(pdu.EncodePayload(&p)) != raw {
		t.Fatalf("preamble %x does not round trip", raw)
	}
	if p, ok := c.AsCapacityPlusDataRevertAnnouncement(); ok && must(pdu.EncodePayload(&p)) != raw {
		t.Fatalf("revert announcement %x does not round trip", raw)
	}
	if p, ok := c.AsCapacityPlusDataRevertGrant(); ok && must(pdu.EncodePayload(&p)) != raw {
		t.Fatalf("revert grant %x does not round trip", raw)
	}
}
