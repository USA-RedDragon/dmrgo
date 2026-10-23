package bs_test

import (
	"testing"
	"time"

	"github.com/USA-RedDragon/dmrgo/v2/bs"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
)

func TestExpressHeaderLate(t *testing.T) {
	t.Parallel()
	for _, mode := range []bs.Mode{bs.ModeNetwork, bs.ModeRF} {
		for _, late := range []time.Duration{170 * time.Millisecond, 330 * time.Millisecond} {
			d := netData(t, loadBin(t, "d-sms.bin"), enums.Slot1, 30, 5, time.Second)
			want := append([]advPkt(nil), d...)
			hdr := -1
			for i := range d {
				if d[i].raw.SlotType().DataType == enums.DataTypeDataHeader {
					hdr = i
					break
				}
			}
			if hdr < 0 || hdr+3 >= len(d) {
				t.Fatal("no data header with blocks after it")
			}
			d[hdr].at += late
			got := expressRun(t, newRepeater(t, &bs.Config{Mode: mode, ColorCode: 1, CACH: true}), d, 4*time.Second, false)
			slack := late
			if mode == bs.ModeRF {
				slack += time.Duration(len(want)) * 60 * time.Millisecond
			}
			checkData(t, mode.String(), dataOut(got, enums.Slot1), want, func(int) time.Duration { return slack })
		}
	}
}

func TestExpressHeaderLost(t *testing.T) {
	t.Parallel()
	d := netData(t, loadBin(t, "d-sms.bin"), enums.Slot1, 30, 5, time.Second)
	hdr := -1
	for i := range d {
		if d[i].raw.SlotType().DataType == enums.DataTypeDataHeader {
			hdr = i
			break
		}
	}
	next := netData(t, loadBin(t, "h-sms.bin"), enums.Slot1, 100, 6, d[len(d)-1].at+500*time.Millisecond)
	in := append(append(append([]advPkt(nil), d[:hdr]...), d[hdr+1:]...), next...)
	got := expressRun(t, newRepeater(t, &bs.Config{Mode: bs.ModeNetwork, ColorCode: 1}), in, 5*time.Second, false)
	out := dataOut(got, enums.Slot1)
	if len(out) < len(next) {
		t.Fatalf("%d data bursts out", len(out))
	}
	checkData(t, "next message", out[len(out)-len(next):], next, func(int) time.Duration { return 0 })
}
