//go:build !tinygo

package rf

import (
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/internal/rftx"
)

func proofBurst(t *testing.T, cc enums.ColorCode, breakSlotType bool) Burst {
	t.Helper()
	raw, err := rftx.WakeupBurst(rftx.SeqConfig{ColorCode: cc, Source: 3191868, BS: 310123})
	if err != nil {
		t.Fatal(err)
	}
	flip := func(k int) { raw[k/8] ^= 0x80 >> (k % 8) }
	for k := 108; k < 156; k += 6 {
		flip(k)
	}
	if breakSlotType {
		for k := 98; k < 108 && !raw.SlotType().FEC.Uncorrectable; k++ {
			flip(k)
		}
		for k := 156; k < 166 && !raw.SlotType().FEC.Uncorrectable; k++ {
			flip(k)
		}
		if !raw.SlotType().FEC.Uncorrectable {
			t.Fatal("slot type still decodes")
		}
	}
	return Burst{Data: raw, Sync: enums.SyncMSSourcedData, Distance: syncDist(&raw, enums.SyncMSSourcedData)}
}

// TestExtWeakLockProof feeds bursts with a SYNC 8 bits off into a stream
// that only the weak verify path locked. A burst whose slot type does not
// decode never leaves the receiver; two bursts whose slot types decode with
// the same color code release the stream.
func TestExtWeakLockProof(t *testing.T) {
	t.Parallel()
	run := func(bs []Burst, votes int) int {
		n := 0
		c := &leanCore{unproven: true, onBurst: func(*Burst) { n++ }}
		if votes >= 0 {
			c.votes[votes] = 1
		}
		x := &softExt{}
		for i := range bs {
			if c.unproven {
				x.prove(c, &bs[i])
			}
			x.push(c, &bs[i], false)
			x.drain(c, false, 0)
		}
		x.end(c)
		return n
	}
	bad := proofBurst(t, 1, true)
	if bad.Distance != 8 {
		t.Fatalf("SYNC distance %d, want 8", bad.Distance)
	}
	good1, good2, other := proofBurst(t, 1, false), proofBurst(t, 1, false), proofBurst(t, 2, false)
	for _, c := range []struct {
		name  string
		bs    []Burst
		votes int
		want  int
	}{
		{"uncorrectable slot type", []Burst{bad}, -1, 0},
		{"many uncorrectable", []Burst{bad, bad, bad, bad, bad, bad, bad, bad, bad, bad}, -1, 0},
		{"one decoded slot type", []Burst{good1}, -1, 0},
		{"two disagreeing color codes", []Burst{good1, other}, -1, 0},
		{"two agreeing color codes", []Burst{bad, good1, good2}, -1, 3},
		{"known color code", []Burst{good1}, 1, 1},
		{"other known color code", []Burst{good1}, 2, 0},
	} {
		if got := run(c.bs, c.votes); got != c.want {
			t.Errorf("%s: %d bursts out, want %d", c.name, got, c.want)
		}
	}
}
