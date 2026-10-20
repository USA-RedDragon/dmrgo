package bs_test

import (
	"errors"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/bs"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

func TestCACH(t *testing.T) {
	t.Parallel()
	var c bs.CACH
	au := pdu.ShortLCActivityUpdate{TS2ActivityID: enums.ActivityGroupVoice, HashTS2: 63}
	slc := pdu.ShortLC{SLCO: enums.SLCOActivityUpdate, ActivityUpdate: &au}
	var asm layer2.ShortLCAssembler
	for i := range 8 {
		if c.First() != (i%4 == 0) {
			t.Fatalf("field %d: First %v", i, c.First())
		}
		tc := enums.Slot(i%2 + 1)
		f, err := c.Next(tc, i%3 == 0, &slc)
		if err != nil {
			t.Fatal(err)
		}
		got := layer2.DecodeCACHField(f)
		if got.TACT.TDMAChannel != (tc == enums.Slot2) || got.TACT.AccessType != (i%3 == 0) || got.TACT.FEC.ErrorsCorrected != 0 {
			t.Fatalf("field %d: TACT %+v", i, got.TACT)
		}
		out, done, err := asm.Add(got)
		if done != (i%4 == 3) || err != nil {
			t.Fatalf("field %d: done %v, %v", i, done, err)
		}
		if done && (out.ActivityUpdate == nil || *out.ActivityUpdate != au) {
			t.Fatalf("field %d: Short LC %v", i, out)
		}
	}
	if _, err := c.Next(enums.SlotUnknown, false, &slc); !errors.Is(err, bs.ErrSlot) {
		t.Errorf("Next of SlotUnknown: %v", err)
	}
	c.Reset()
	if !c.First() {
		t.Error("Reset did not restart the Short LC")
	}
}
