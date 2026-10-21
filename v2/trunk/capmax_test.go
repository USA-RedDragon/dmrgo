package trunk_test

import (
	"encoding/hex"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
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

func TestMS_AuthCapacityMaxChallenges(t *testing.T) {
	t.Parallel()
	for _, h := range []string{
		"9c00000e018a890000ca", "9c00000e018a8900021d", "9c00000e018a89000355", "9c00000e018a89000874",
		"9c00000e018a89002619", "9c00000e018a89005638", "9c00000e018a89006aef", "9c00000e002a540042de",
		"9c00000e014066006acf", "9c00000e0203a9007aeb", "9c00000e029c120051a1",
	} {
		var c pdu.CSBKBytes
		if _, err := hex.Decode(c[:10], []byte(h)); err != nil {
			t.Fatal(err)
		}
		c.Reseal()
		raw, err := layer2.BuildDataBurst([12]byte(c), enums.DataTypeCSBK, testCC, layer2.SyncSourceBS)
		if err != nil {
			t.Fatal(err)
		}
		target, source, _ := c.Addresses()
		r := newRig(t, &trunk.MSConfig{Address: target, NoRegistration: true})
		r.feed(aloha(t), &idle)
		out := r.feed(raw, &busy)
		wantKinds(t, h, out, trunk.OutputAuthChallenge)
		if out[0].Challenge != source || source >= 0x8000 {
			t.Errorf("%s: challenge %06X, want %06X", h, out[0].Challenge, source)
		}
	}
}
