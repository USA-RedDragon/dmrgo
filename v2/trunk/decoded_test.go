package trunk_test

import (
	"reflect"
	"testing"
	"time"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/trunk"
)

type burstMachine interface {
	Burst(now time.Duration, raw *layer2.RawBurst, h layer2.Hint, cach *[3]byte, dst []trunk.Output) []trunk.Output
	Decoded(now time.Duration, b *layer2.Burst, h layer2.Hint, cach *[3]byte, dst []trunk.Output) []trunk.Output
}

func decodeOnce(c *capture, b *layer2.Burst) *layer2.Burst {
	opts := layer2.DecodeOptions{Kind: c.hint.Kind, VoiceBurst: enums.VoiceBurstUnknown}
	if c.hint.Kind == layer2.KindVoice {
		opts.VoiceBurst = c.hint.VoiceBurst
	}
	if b.DecodeFromBytesWith(c.raw, &opts) != nil {
		return nil
	}
	return b
}

func checkDecoded(t *testing.T, name string, caps []capture, viaBurst, viaDecoded burstMachine) {
	t.Helper()
	var b layer2.Burst
	n := 0
	for i := range caps {
		c := &caps[i]
		want := viaBurst.Burst(c.at, &c.raw, c.hint, c.cach, nil)
		got := viaDecoded.Decoded(c.at, decodeOnce(c, &b), c.hint, c.cach, nil)
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("%s burst %d: Decoded %+v, Burst %+v", name, i, got, want)
		}
		n += len(want)
	}
	if n == 0 {
		t.Fatalf("%s: no outputs", name)
	}
}

func TestDecoded_MatchesBurst(t *testing.T) {
	t.Parallel()
	for _, path := range []string{"testdata/aselsan_udt.bits", "testdata/capmax_cc.bits", "testdata/t3cc.bits"} {
		caps := readCapture(t, path)
		f1, _ := trunk.NewFollower(&trunk.FollowerConfig{NoFollow: true, AcceptRAS: true})
		f2, _ := trunk.NewFollower(&trunk.FollowerConfig{NoFollow: true, AcceptRAS: true})
		checkDecoded(t, "Follower "+path, caps, f1, f2)
		m1, _ := trunk.NewMS(&trunk.MSConfig{Address: msAddr, NoRegistration: true, AcceptRAS: true})
		m2, _ := trunk.NewMS(&trunk.MSConfig{Address: msAddr, NoRegistration: true, AcceptRAS: true})
		checkDecoded(t, "MS "+path, caps, m1, m2)
	}
	for _, path := range []string{"testdata/capplus_busy.txt", "testdata/capplus_lcp_beacon.txt"} {
		caps := readCapPlusCapture(t, path)
		c1, _ := trunk.NewCapacityPlusFollower(nil)
		c2, _ := trunk.NewCapacityPlusFollower(nil)
		checkDecoded(t, "CapacityPlusFollower "+path, caps, c1, c2)
	}
}
