package rf

import (
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

func TestLeanEMBAfterData(t *testing.T) {
	t.Parallel()
	const cc = 7
	burst := func(p enums.SyncPattern, dt enums.DataType) Burst {
		var b Burst
		if err := b.Data.SetSync(p); err != nil {
			t.Fatal(err)
		}
		if err := b.Data.SetSlotType(pdu.SlotType{ColorCode: cc, DataType: dt}); err != nil {
			t.Fatal(err)
		}
		return b
	}
	var emb Burst
	if err := emb.Data.SetEMB(pdu.EmbeddedSignalling{ColorCode: cc, LCSS: enums.LCSSLastFragment}); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name   string
		bs     bool
		before []enums.DataType
		want   bool
	}{
		{"late entry", false, nil, true},
		{"after data", false, []enums.DataType{enums.DataTypeRate12}, false},
		{"after terminator", false, []enums.DataType{enums.DataTypeTerminatorWithLC}, false},
		{"after header", false, []enums.DataType{enums.DataTypeRate12, enums.DataTypeVoiceLCHeader}, true},
		{"bs after idle", true, []enums.DataType{enums.DataTypeIdle}, true},
	}
	for _, c := range cases {
		r := &leanCore{src: 1, bs: c.bs}
		if c.bs {
			r.src = 0
		}
		_, data, _ := sourceAt(int(r.src)).SyncPatterns()
		r.resetChans()
		r.votes[cc] = 1
		j := int32(1)
		for _, dt := range c.before {
			b := burst(data, dt)
			if !r.classify(&b, j) || b.Sync != data {
				t.Fatalf("%s: data burst %v not placed", c.name, dt)
			}
			j += 2
		}
		j += 2
		b := emb
		got := r.classify(&b, j)
		if got != c.want || got && b.Voice != enums.VoiceBurstE {
			t.Errorf("%s: placed %v as %v, want %v", c.name, got, b.Voice, c.want)
		}
	}
}
