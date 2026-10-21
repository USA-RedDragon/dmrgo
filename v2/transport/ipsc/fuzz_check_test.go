package ipsc_test

import (
	"bytes"
	"encoding/hex"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/transport/ipsc"
)

func fuzzSeeds() [][]byte {
	seeds := make([][]byte, 0, 12)
	for _, h := range []string{
		"900004ce2e6a0000805c04080400",
		"93003010af0000",
		"850004ce2e000000010002",
		"800004ce2e1d30b43c0000090200000d9e60805e7b6c482e959e0000000013800000",
		"840004ce2e552febf630b43c0100007f1b40805e31d16224394d0000000008420010804a0090df56803c7bc828850bd5eadf4d860390a50c0078e5a6f28fc6965cf0",
		"800004ce2e3230b43c00000902000058f32080ddfb253bbefd0400000000018000081000006000000000000930b43c000000",
	} {
		p, _ := hex.DecodeString(h)
		seeds = append(seeds, p)
	}
	return seeds
}

func checkPacket(t *testing.T, p []byte) {
	t.Helper()
	if u, err := ipsc.DecodeUser(p); err == nil {
		out, err := u.AppendBinary(nil)
		if err != nil || !bytes.Equal(out, p) {
			t.Fatalf("user round trip %x: %x %v", p, out, err)
		}
		_ = u.Hint()
		if raw, err := u.RawBurst(1, layer2.SyncSourceMS); err == nil {
			var back ipsc.User
			h := u.Hint()
			_, _ = back.SetRawBurst(raw, h)
		}
	}
	if c, err := ipsc.DecodeControl(p); err == nil && c.Opcode != ipsc.OpPeerListReply {
		out, err := c.AppendBinary(nil)
		if err != nil || !bytes.Equal(out, p) {
			t.Fatalf("control round trip %x: %x %v", p, out, err)
		}
	}
	if l, err := ipsc.PeerListOf(p); err == nil {
		for i := range l.Len() {
			_ = l.Entry(i)
		}
	}
	if r, err := ipsc.DecodeRaw(p); err == nil && !bytes.Equal(r.AppendBinary(nil), p) {
		t.Fatalf("raw round trip %x", p)
	}
}
