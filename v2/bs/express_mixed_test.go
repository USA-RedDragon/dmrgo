package bs_test

import (
	"testing"
	"time"

	"github.com/USA-RedDragon/dmrgo/v2/bs"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
)

func TestExpressCSBKInVoiceStream(t *testing.T) {
	t.Parallel()
	for _, mode := range []bs.Mode{bs.ModeNetwork, bs.ModeRF} {
		v := advNetCall(t, 3, 0, 0, 1, time.Second, 3191868)
		c := advPkt{at: v[9].at + 10*time.Millisecond, raw: testCSBK(t), peer: 1, h: v[9].h}
		for i := 10; i < len(v); i++ {
			v[i].h.Seq = (v[i].h.Seq + 1) & 0xFF
		}
		c.h.Seq = (c.h.Seq + 1) & 0xFF
		cfg := &bs.Config{Mode: mode, ColorCode: 1, CACH: true}
		got := expressRun(t, newRepeater(t, cfg), append(append([]advPkt(nil), v...), c), 6*time.Second, false)
		checkVoice(t, "csbk in voice", newRepeater(t, cfg), v, got, enums.Slot1)
		if d := dataOut(got, enums.Slot1); len(d) != 1 || payloadOf(d[0].Burst) != payloadOf(c.raw) {
			t.Fatalf("%v: %d CSBKs out", mode, len(d))
		}
	}
}

func TestExpressSharedCounter(t *testing.T) {
	t.Parallel()
	v := advNetCall(t, 3, 0, 1, 1, time.Second, 3191868)
	raws := call(t, 1, 9, 3191868, 3, enums.Slot2, bs.Source{Peer: 1}, 0)
	for i := range v {
		v[i].h.Slot = enums.Slot2
		v[i].raw = raws[i].raw
		v[i].h.Seq = uint16(2*i) & 0xFF
	}
	d := netData(t, loadBin(t, "d-sms.bin"), enums.Slot1, 0, 2, v[2].at+30*time.Millisecond)
	for i := range d {
		d[i].h.Seq = uint16(2*i+5) & 0xFF
	}
	wantD := append([]advPkt(nil), d...)
	cfg := &bs.Config{Mode: bs.ModeNetwork, ColorCode: 1}
	got := expressRun(t, newRepeater(t, cfg), append(append([]advPkt(nil), v...), d...), 6*time.Second, false)
	checkData(t, "shared counter", dataOut(got, enums.Slot1), wantD, func(i int) time.Duration {
		if i == 1 {
			return 60 * time.Millisecond
		}
		return 0
	})
	checkVoice(t, "shared counter", newRepeater(t, cfg), v, got, enums.Slot2)
}
