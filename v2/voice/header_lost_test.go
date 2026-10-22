package voice_test

import (
	"strconv"
	"testing"
	"time"

	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/voice"
)

func headerCount(t *testing.T, bursts []layer2.RawBurst) int {
	t.Helper()
	for i := range bursts {
		b, err := layer2.NewBurstFromBytes(bursts[i])
		if err != nil {
			t.Fatal(err)
		}
		if b.Kind == layer2.KindVoice {
			return i
		}
	}
	t.Fatal("no voice burst")
	return 0
}

func TestReceiver_LostAfterHeader(t *testing.T) {
	t.Parallel()

	bursts := transmission(t, groupLC(t, 91, 3191868), 3, nil)
	headers := headerCount(t, bursts)
	for _, lost := range []int{0, 1, 2, 4, 6, 8} {
		t.Run(strconv.Itoa(lost), func(t *testing.T) {
			t.Parallel()

			in := timed(bursts, 0, layer2.Hint{})
			in = append(in[:headers], in[headers+lost:]...)
			got := feed(t, newReceiver(t, nil), in, -1)
			voices := find(got, voice.EventVoice)
			if len(voices) != 18-lost {
				t.Fatalf("events %s", kinds(got))
			}
			want := "ABCDEFABCDEFABCDEF"[lost:]
			if want[0] == 'C' {
				want = "??" + want[2:]
			}
			if positions(got) != want {
				t.Errorf("positions %s, want %s", positions(got), want)
			}
			if voices[0].Lost != uint8(lost) {
				t.Errorf("first voice burst: Lost %d, want %d", voices[0].Lost, lost)
			}
			if n := lostCounts(got); len(n) > 1 || lost == 0 && len(n) != 0 {
				t.Errorf("lost counts %v", n)
			}
			end := find(got, voice.EventCallEnd)
			if len(end) != 1 || end[0].Stats.Lost != uint32(lost) || end[0].Stats.Bursts != uint32(18-lost) {
				t.Errorf("end %+v", end)
			}
		})
	}
}

func TestReceiver_LostAfterHeaderSeq(t *testing.T) {
	t.Parallel()

	bursts := transmission(t, groupLC(t, 91, 3191868), 2, nil)
	headers := headerCount(t, bursts)
	tests := []struct {
		name  string
		lost  int
		delay time.Duration
	}{
		{"LateNoLoss", 0, 5 * slot},
		{"EarlyLoss", 3, -3 * slot},
		{"OnTimeLoss", 2, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			in := timed(bursts, 0, layer2.Hint{})
			for i := range in {
				in[i].hint = layer2.Hint{Seq: uint16(1000 + i), HasSeq: true}
				if i >= headers {
					in[i].at += tt.delay
				}
			}
			in = append(in[:headers], in[headers+tt.lost:]...)
			got := feed(t, newReceiver(t, nil), in, -1)
			voices := find(got, voice.EventVoice)
			if len(voices) != 12-tt.lost || voices[0].Lost != uint8(tt.lost) {
				t.Fatalf("events %s, lost counts %v, want %d lost first", kinds(got), lostCounts(got), tt.lost)
			}
			if want := "ABCDEFABCDEF"[tt.lost:]; positions(got) != want {
				t.Errorf("positions %s, want %s", positions(got), want)
			}
			if end := find(got, voice.EventCallEnd); len(end) != 1 || end[0].Stats.Lost != uint32(tt.lost) {
				t.Errorf("end %+v", end)
			}
		})
	}
}
