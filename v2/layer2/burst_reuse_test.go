package layer2_test

import (
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

// TestBurstDecode_ReusesData shows that a decode into a reused Burst
// overwrites the PDU of the previous decode, and that Encode codes an edit of
// that PDU.
func TestBurstDecode_ReusesData(t *testing.T) {
	t.Parallel()

	first := must(layer2.BuildBurst(&pdu.Rate12Data{Data: [12]byte{1}}, 1, layer2.SyncSourceBS))
	second := must(layer2.BuildBurst(&pdu.Rate12Data{Data: [12]byte{2}}, 1, layer2.SyncSourceBS))
	var b layer2.Burst
	if err := b.DecodeFromBytes(first); err != nil {
		t.Fatal(err)
	}
	kept, ok := b.Data.(*pdu.Rate12Data)
	if !ok {
		t.Fatalf("Data is %T", b.Data)
	}
	copied := *kept
	if err := b.DecodeFromBytes(second); err != nil {
		t.Fatal(err)
	}
	if b.Data != layer2.Data(kept) || kept.Data[0] != 2 || copied.Data[0] != 1 {
		t.Fatalf("Data %s, kept %x, copy %x", b.Data.String(), kept.Data, copied.Data)
	}
	kept.Data[0] = 3
	got := must(layer2.NewBurstFromBytes(must(b.Encode())))
	if r, ok := got.Data.(*pdu.Rate12Data); !ok || r.Data[0] != 3 {
		t.Errorf("Encode gives %s, want the edited block", got.String())
	}
}
