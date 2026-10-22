package pdu_test

import (
	"math/rand"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

func csbkGroup(c *pdu.CSBK) (group, ok bool) {
	if p, ok := c.AsPreamble(); ok {
		return p.Group, true
	}
	if p, ok := c.AsClear(); ok {
		return p.Group, true
	}
	if p, ok := c.AsProtect(); ok {
		return p.Group, true
	}
	if p, ok := c.AsAhoy(); ok {
		return p.Group, true
	}
	return false, false
}

func setCSBKGroup(c *pdu.CSBK, group bool) {
	if p, ok := c.AsPreamble(); ok {
		p.Group = group
		noErr(c.SetPreamble(p))
	} else if p, ok := c.AsClear(); ok {
		p.Group = group
		noErr(c.SetClear(p))
	} else if p, ok := c.AsProtect(); ok {
		p.Group = group
		noErr(c.SetProtect(p))
	} else if p, ok := c.AsAhoy(); ok {
		p.Group = group
		noErr(c.SetAhoy(p))
	}
}

func TestCSBKBytes_ReproducerPreamble(t *testing.T) {
	t.Parallel()

	c := pdu.CSBKBytes{0xbd, 0x00, 0x80, 0x1d, 0x23, 0x38, 0x63, 0x23, 0x38, 0x3b, 0x58, 0x89}
	if !c.CRCOK() || c.Opcode() != pdu.CSBKPreamble || c.FID() != enums.FIDStandardized {
		t.Fatalf("%x: CRC %t opcode %s FID %d", c, c.CRCOK(), c.Opcode(), c.FID())
	}
	tgt, src, ok := c.Addresses()
	if g, gok := c.Group(); !ok || tgt != 0x233863 || src != 0x23383b || g || !gok {
		t.Fatalf("Addresses %x %x %t, Group %t %t", tgt, src, ok, g, gok)
	}
	want := pdu.DecodeCSBK(c)
	res := want.FEC
	if res.Uncorrectable {
		t.Fatal("DecodeCSBK fails the CRC")
	}
	if c.SetAddresses(9, 0x23383B) != nil || !c.SetGroup(true) {
		t.Fatal("edit refused")
	}
	c.Reseal()
	noErr(want.SetAddresses(9, 0x23383B))
	setCSBKGroup(&want, true)
	if b := must(want.Encode()); b != [12]byte(c) || !c.CRCOK() {
		t.Fatalf("CSBKBytes %x, CSBK.Encode %x", c, b)
	}
}

func TestCSBKBytes_MatchesCSBK(t *testing.T) {
	t.Parallel()

	addrBits := [12]byte{4: 0xFF, 5: 0xFF, 6: 0xFF, 7: 0xFF, 8: 0xFF, 9: 0xFF}
	rng := rand.New(rand.NewSource(17))
	addressed, grouped := map[pdu.CSBKOpcode]bool{}, map[pdu.CSBKOpcode]bool{}
	for range 20000 {
		var raw [12]byte
		rng.Read(raw[:])
		if rng.Intn(4) != 0 {
			raw[1] = 0
		}
		orig := pdu.CSBKBytes(raw)
		orig.Reseal()
		dec := pdu.DecodeCSBK(orig)
		res := dec.FEC
		if res.Uncorrectable {
			t.Fatalf("%x: resealed CSBK fails its CRC", orig)
		}
		if must(dec.Encode()) != [12]byte(orig) {
			t.Fatalf("%x: CSBK.Encode does not round trip", orig)
		}
		wt, ws, wok := dec.Addresses()
		gt, gs, gok := orig.Addresses()
		if gt != wt || gs != ws || gok != wok {
			t.Fatalf("%x: CSBKBytes.Addresses %x %x %t, CSBK %x %x %t", orig, gt, gs, gok, wt, ws, wok)
		}
		wg, wgok := csbkGroup(&dec)
		if g, ok := orig.Group(); g != wg || ok != wgok {
			t.Fatalf("%x: CSBKBytes.Group %t %t, CSBK %t %t", orig, g, ok, wg, wgok)
		}

		c := orig
		tgt, src := uint32(rng.Intn(1<<24)), uint32(rng.Intn(1<<24))
		want := dec
		if (c.SetAddresses(tgt, src) == nil) != (want.SetAddresses(tgt, src) == nil) {
			t.Fatalf("%x: SetAddresses ok differs from CSBK", orig)
		}
		if c.SetAddresses(1<<24, 0) == nil || c.SetAddresses(0, 1<<24) == nil {
			t.Fatalf("%x: SetAddresses accepts 25 bits", orig)
		}
		g := rng.Intn(2) == 0
		if c.SetGroup(g) != wgok {
			t.Fatalf("%x: SetGroup ok differs from Group", orig)
		}
		setCSBKGroup(&want, g)
		if !wok && !wgok && c != orig {
			t.Fatalf("%x: refused edits changed the CSBK", orig)
		}
		if wok {
			addressed[orig.Opcode()] = true
		}
		if wgok {
			grouped[orig.Opcode()] = true
		}
		o, m := groupMask(orig)
		allowed := addrBits
		allowed[o] |= m
		for i := range 10 {
			if (c[i]^orig[i])&^allowed[i] != 0 {
				t.Fatalf("%x: octet %d changed %02x -> %02x", orig, i, orig[i], c[i])
			}
		}
		c.Reseal()
		if b := must(want.Encode()); b != [12]byte(c) {
			t.Fatalf("%x: CSBKBytes edit %x, CSBK edit %x", orig, c, b)
		}
	}
	if len(addressed) != 19 || len(grouped) != 4 {
		t.Fatalf("covered %d addressed and %d grouped opcodes", len(addressed), len(grouped))
	}
}

func groupMask(c pdu.CSBKBytes) (int, byte) {
	g, ok := map[pdu.CSBKOpcode]struct {
		octet int
		mask  byte
	}{
		pdu.CSBKPreamble: {2, 0x40},
		pdu.CSBKClear:    {3, 0x01},
		pdu.CSBKProtect:  {3, 0x01},
		pdu.CSBKAhoy:     {3, 0x40},
	}[c.Opcode()]
	if !ok || c.FID() != enums.FIDStandardized {
		return 0, 0
	}
	return g.octet, g.mask
}
