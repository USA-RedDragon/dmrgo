package bs_test

import (
	"testing"
	"time"

	"github.com/USA-RedDragon/dmrgo/v2/bs"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
)

func TestDataBadBlockCounts(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"d-sms.bin", "h-sms.bin", "m-sms.bin"} {
		bursts := loadBin(t, name)
		bad := -1
		for i, b := range bursts {
			if dt := b.SlotType().DataType; dt == enums.DataTypeRate12 || dt == enums.DataTypeRate34 {
				bad = i
			}
		}
		if bad < 0 {
			t.Fatalf("%s: no data block", name)
		}
		b := &bursts[bad]
		for i := range 12 {
			b[i] ^= 0x5A
		}
		for i := 21; i < 33; i++ {
			b[i] ^= 0xA5
		}
		var ok bool
		if b.SlotType().DataType == enums.DataTypeRate34 {
			_, res := b.Rate34Payload()
			ok = res.Uncorrectable
		} else {
			_, res := b.BPTCPayload()
			ok = res.Uncorrectable
		}
		if !ok || b.SlotType().FEC.Uncorrectable {
			t.Fatalf("%s: block %d not made uncorrectable", name, bad)
		}
		in := timed(bursts, enums.Slot2, rf, 0)
		got := run(t, newRepeater(t, &bs.Config{ColorCode: 1, CACH: true}), in, 3*time.Second)
		ends := filter(got, bs.OutputCallEnd)
		if len(ends) != 1 || ends[0].End != 0 {
			t.Fatalf("%s: ends %+v, want one with End 0", name, ends)
		}
	}
}
