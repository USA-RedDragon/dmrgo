package bs_test

import (
	"testing"
	"time"

	"github.com/USA-RedDragon/dmrgo/v2/bs"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/voice"
)

func TestRepeaterLateEntryOneHeader(t *testing.T) {
	t.Parallel()
	for _, from := range []bs.Source{{RF: true}, {Peer: 1}} {
		for _, late := range []bool{false, true} {
			in := call(t, 1, 9, 3191868, 3, enums.Slot1, from, 0)
			if late {
				in = in[1:]
			}
			got := run(t, newRepeater(t, &bs.Config{ColorCode: 1, Headers: 3}), in, 3*time.Second)
			starts := filter(got, bs.OutputCallStart)
			if len(starts) == 0 || (starts[0].Call.Origin == voice.OriginHeader) == late {
				t.Fatalf("from %+v late %v: call starts %v", from, late, starts)
			}
			headers, last, first := 0, time.Duration(0), time.Duration(-1)
			for _, o := range filter(got, bs.OutputBurst) {
				if o.To&bs.ToRF == 0 || o.Slot != enums.Slot1 {
					continue
				}
				if o.Hint.Kind == layer2.KindVoice {
					first = o.Due
					break
				}
				if o.Hint.HasDataType && o.Hint.DataType == enums.DataTypeVoiceLCHeader {
					headers, last = headers+1, o.Due
				}
			}
			want := 3
			if late {
				want = 1
			}
			if headers != want || first != last+60*time.Millisecond {
				t.Errorf("from %+v late %v: %d headers, first voice due %v after the last, want %d and 60ms", from, late, headers, first-last, want)
			}
		}
	}
}
