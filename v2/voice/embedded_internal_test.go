package voice

import (
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
	"github.com/USA-RedDragon/dmrgo/v2/layer3"
)

func collectSuperframe(r *Receiver, f [4][4]byte) (pdu.LCBytes, bool) {
	var lc pdu.LCBytes
	var ok bool
	for i := range f {
		b := layer2.Burst{Kind: layer2.KindVoice, SyncPattern: enums.SyncNone, EmbeddedSignallingData: f[i]}
		lc, ok = r.collect(&b, i+1, 0)
	}
	return lc, ok
}

func TestCollectNoStaleTalker(t *testing.T) {
	t.Parallel()
	a, err := pdu.NewVoiceLCBytes(true, 91, 3002068, layer3.ServiceOptions{})
	if err != nil {
		t.Fatal(err)
	}
	b, err := pdu.NewVoiceLCBytes(true, 91, 3002069, layer3.ServiceOptions{})
	if err != nil {
		t.Fatal(err)
	}
	var r Receiver
	for range 2 {
		if lc, ok := collectSuperframe(&r, layer2.EncodeEmbeddedLC(a)); !ok || lc != a {
			t.Fatalf("talker A: %x %t", lc, ok)
		}
	}
	fb := layer2.EncodeEmbeddedLC(b)
	for _, p := range []int{3, 6, 13, 20, 27} {
		fb[2][p/8] ^= 0x80 >> (p % 8)
	}
	if lc, ok := collectSuperframe(&r, fb); ok && lc == a {
		t.Fatalf("talker B with five bit errors gives the LC of talker A")
	}
}
