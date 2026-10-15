package layer2_test

import (
	"fmt"
	"math/rand"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

func TestCSBKBytes_SMSCaptures(t *testing.T) {
	rng := rand.New(rand.NewSource(7))
	n := 0
	for _, name := range []string{"d-sms", "h-sms", "m-sms"} {
		for i, frame := range loadBursts(t, "testdata/"+name+".bin") {
			b, _ := layer2.NewBurstFromBytes(frame)
			dc, isCSBK := b.Data.(*pdu.CSBK)
			if !isCSBK || b.FEC.PDU.HasErrors() {
				continue
			}
			r := layer2.RawBurst(frame)
			p, _ := r.BPTCPayload()
			c := pdu.CSBKBytes(p)
			if !c.CRCOK() || c.Opcode() != pdu.CSBKPreamble || c.Opcode() != dc.Opcode || c.FID() != dc.FID {
				t.Fatalf("%s burst %d: CRC %t, opcode %s", name, i, c.CRCOK(), c.Opcode())
			}
			if d, _ := pdu.DecodeCSBKBytes(c); d.PreamblePDU == nil || *d.PreamblePDU != *dc.PreamblePDU {
				t.Fatalf("%s burst %d: DecodeCSBKBytes differs from the burst decode", name, i)
			}
			if must(dc.Bytes()) != p {
				t.Fatalf("%s burst %d: CSBK.Bytes %x, payload %x", name, i, must(dc.Bytes()), p)
			}
			n++
			wantT, wantS, _ := dc.Addresses()
			gotT, gotS, ok := c.Addresses()
			g, gok := c.Group()
			if !ok || !gok || gotT != wantT || gotS != wantS || g != dc.PreamblePDU.Group {
				t.Fatalf("%s burst %d: Addresses %d %d %t, Group %t %t", name, i, gotT, gotS, ok, g, gok)
			}
			editPreamble(t, fmt.Sprintf("%s burst %d", name, i), r, c, dc, rng)
		}
	}
	if n == 0 {
		t.Fatal("no CSBKs in the SMS captures")
	}
	t.Logf("%d preambles", n)
}

func editPreamble(t *testing.T, where string, r layer2.RawBurst, c pdu.CSBKBytes, dc *pdu.CSBK, rng *rand.Rand) {
	t.Helper()
	editable := [12]byte{2: 0x40, 4: 0xFF, 5: 0xFF, 6: 0xFF, 7: 0xFF, 8: 0xFF, 9: 0xFF, 10: 0xFF, 11: 0xFF}
	g, _ := c.Group()
	newT, newS, newG := uint32(rng.Intn(1<<24)), uint32(rng.Intn(1<<24)), !g
	e := c
	if !e.SetAddresses(newT, newS) || !e.SetGroup(newG) {
		t.Fatalf("%s: edit refused", where)
	}
	if e.CRCOK() {
		t.Fatalf("%s: CRC still passes before Reseal", where)
	}
	e.Reseal()
	if !e.CRCOK() || !layer2.MMDVMHostAccepts(enums.DataTypeCSBK, e) {
		t.Fatalf("%s: resealed CSBK fails its CRC", where)
	}
	for j := range e {
		if (e[j]^c[j])&^editable[j] != 0 {
			t.Fatalf("%s: octet %d changed %02x -> %02x", where, j, c[j], e[j])
		}
	}
	want := *dc
	pre := *dc.PreamblePDU
	want.PreamblePDU = &pre
	want.SetAddresses(newT, newS)
	want.PreamblePDU.Group = newG
	if must(want.Bytes()) != [12]byte(e) {
		t.Fatalf("%s: CSBKBytes edit %x, CSBK edit %x", where, e, must(want.Bytes()))
	}

	r.SetBPTCPayload(e)
	nb, err := layer2.NewBurstFromBytes(r)
	if err != nil {
		t.Fatalf("%s: %v", where, err)
	}
	nc, ok := nb.Data.(*pdu.CSBK)
	if !ok || nb.FEC.PDU.HasErrors() {
		t.Fatalf("%s: edited burst does not decode as a clean CSBK", where)
	}
	if dt, ds, _ := nc.Addresses(); dt != newT || ds != newS || nc.PreamblePDU.Group != newG {
		t.Fatalf("%s: edited CSBK decodes as %d %d %t", where, dt, ds, nc.PreamblePDU.Group)
	}
}
