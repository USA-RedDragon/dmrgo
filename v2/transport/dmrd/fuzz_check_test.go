package dmrd_test

import (
	"bytes"
	"encoding/hex"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/transport/dmrd"
)

func fuzzSeeds() [][]byte {
	seeds := make([][]byte, 0, 32)
	for _, v := range dmrdVectors() {
		p, _ := hex.DecodeString(v.hex)
		seeds = append(seeds, p, append(append([]byte{}, p...), 1, 2), append(append([]byte{}, p...), make([]byte, 20)...))
	}
	for _, s := range []string{"RPTL00040178", "MSTACK000401780A7ED498", "RPTRSSI01234567:1-120.0", "DMRT\x01\x81", "DMRP", "BCKA", "StartRef=4013;TS1_1=262;"} {
		seeds = append(seeds, []byte(s))
	}
	seeds = append(seeds, bin("RPTPING", 1), append([]byte("DMRE"), make([]byte, 81)...), append([]byte("RPTC"), make([]byte, 298)...))
	return seeds
}

func checkPacket(t *testing.T, p []byte) {
	t.Helper()
	if d, err := dmrd.DecodeData(p); err == nil {
		out, err := d.AppendBinary(nil)
		if err != nil || !bytes.Equal(out, p) {
			t.Fatalf("data round trip %x: %x %v", p, out, err)
		}
	}
	for _, dl := range []dmrd.Dialect{dmrd.DialectMMDVM, dmrd.DialectHBRP, dmrd.DialectLocal} {
		_ = dmrd.Classify(p, dl)
		c, err := dmrd.DecodeControl(p, dl)
		if err != nil {
			continue
		}
		out, err := c.AppendBinary(nil, dl)
		if err != nil || !bytes.EqualFold(out, p) {
			t.Fatalf("control %v round trip %q: %q %v", dl, p, out, err)
		}
	}
	if b, err := dmrd.DecodeBridge(p); err == nil {
		out, err := b.AppendBinary(nil)
		if err != nil || !bytes.Equal(out, p) {
			t.Fatalf("bridge round trip %x: %x %v", p, out, err)
		}
	}
	if b, err := dmrd.DecodeBridgeControl(p); err == nil {
		out, err := b.AppendBinary(nil)
		if err != nil || !bytes.Equal(out, p) {
			t.Fatalf("bridge control round trip %x: %x %v", p, out, err)
		}
	}
	if c, err := dmrd.MMDVMConfigOf(p); err == nil {
		_, _ = c.RXFrequency()
		_ = c.Callsign()
	}
	if c, err := dmrd.HBRPConfigOf(p); err == nil {
		_, _ = c.ID()
	}
	_, _ = dmrd.DecodeTalkerAlias(p)
	_, _ = dmrd.DecodeLocalTalkerAlias(p)
	_, _ = dmrd.DecodePosition(p)
	_, _ = dmrd.DecodeLocalPosition(p)
	if o, err := dmrd.ParseDMRPlusOptions(p); err == nil {
		again, err := dmrd.ParseDMRPlusOptions(o.AppendText(nil))
		if err != nil || again != o {
			t.Fatalf("options %q: %+v %v", p, again, err)
		}
	}
}
