package trunk_test

import (
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
	"github.com/USA-RedDragon/dmrgo/v2/trunk"
)

func TestFollower_RealCapacityMax(t *testing.T) {
	t.Parallel()
	caps := readCapture(t, "testdata/capmax_cc.bits")
	for _, accept := range []bool{false, true} {
		f, err := trunk.NewFollower(&trunk.FollowerConfig{AcceptRAS: accept, NoFollow: true})
		if err != nil {
			t.Fatal(err)
		}
		out := make([]trunk.Output, 0, trunk.MaxOutputs)
		var grants []trunk.Grant
		sites := 0
		for i := range caps {
			c := &caps[i]
			out = f.Burst(c.at, &c.raw, c.hint, c.cach, out[:0])
			for j := range out {
				switch out[j].Kind {
				case trunk.OutputSite:
					sites++
				case trunk.OutputGrant:
					grants = append(grants, out[j].Grant)
				default:
				}
			}
		}
		if !accept {
			if sites != 0 || len(grants) != 0 {
				t.Errorf("without AcceptRAS: %d sites, %d grants, want none", sites, len(grants))
			}
			continue
		}
		s := f.Site()
		if sites == 0 || s.SysCode.Model != pdu.NetworkModelLarge || s.SysCode.Net != 6 || s.SysCode.Site != 9 || s.ColorCode != 6 {
			t.Errorf("Site = %+v after %d OutputSite", s, sites)
		}
		if len(grants) != 1 {
			t.Fatalf("%d grants, want 1: %+v", len(grants), grants)
		}
		g := grants[0]
		if g.Target != 7001 || g.Source != 72002 || g.Channel != 512 || g.Slot != enums.Slot2 || !g.Group {
			t.Errorf("Grant = %+v", g)
		}
		if s.NeighborsLen == 0 {
			t.Error("no Adjacent_Site neighbors")
		}
	}
}
