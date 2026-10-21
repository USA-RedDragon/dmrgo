package ipsc_test

import (
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/transport/dmrd"
	"github.com/USA-RedDragon/dmrgo/v2/transport/ipsc"
)

func TestXPR8400BridgeToDMRD(t *testing.T) {
	t.Parallel()

	var track [3]layer2.PositionTracker
	bridged, rebuilt := 0, 0
	for _, c := range readXPR(t) {
		u, err := ipsc.DecodeUser(c.p)
		if err != nil {
			continue
		}
		h := u.Hint()
		raw, err := u.RawBurst(7, layer2.SyncSourceBS)
		if err != nil {
			track[h.Slot].Place(nil, &h)
			continue
		}
		rebuilt++
		track[h.Slot].Place(&raw, &h)
		d := dmrd.Data{Source: u.Source, Destination: u.Destination, Repeater: u.ID, Stream: u.CallControl, Burst: raw,
			Private: u.Opcode == ipsc.OpPrivateVoice || u.Opcode == ipsc.OpPrivateData}
		if err := d.SetHint(h); err != nil {
			t.Errorf("%s %x: %+v: %v", c.tag, c.p[:31], h, err)
			continue
		}
		if h.Kind == layer2.KindVoice {
			var b layer2.Burst
			_ = b.DecodeFromBytes(raw)
			if lcss, ok := h.VoiceBurst.LCSS(); ok && h.VoiceBurst != enums.VoiceBurstA && b.EmbeddedSignalling.LCSS != lcss {
				t.Errorf("%s %x: placed at %v, LCSS %v", c.tag, c.p[:31], h.VoiceBurst, b.EmbeddedSignalling.LCSS)
			}
		}
		bridged++
	}
	if rebuilt != 146 || bridged != rebuilt {
		t.Fatalf("bridged %d of %d", bridged, rebuilt)
	}
}
