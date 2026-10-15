package layer2_test

import (
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

func TestEncodeShortLCToCACH_RoundTrip(t *testing.T) {
	t.Parallel()
	slc := pdu.ShortLC{
		SLCO: enums.SLCOActivityUpdate,
		ActivityUpdate: &pdu.ShortLCActivityUpdate{
			TS1ActivityID: enums.ActivityGroupVoice,
			TS2ActivityID: enums.ActivityGroupCSBK,
			HashTS1:       0x5A,
			HashTS2:       0xC3,
		},
	}
	tact := pdu.TACT{AccessType: true, TDMAChannel: true}
	fields := must(layer2.EncodeShortLCToCACH(&slc, tact))

	var a layer2.ShortLCAssembler
	for i, field := range fields {
		c := layer2.DecodeCACH(layer2.CACHDeinterleave(field))
		if c.FEC.Uncorrectable || c.FEC.ErrorsCorrected != 0 {
			t.Fatalf("field %d: TACT FEC %+v", i, c.FEC)
		}
		if !c.TACT.AccessType || !c.TACT.TDMAChannel {
			t.Errorf("field %d: TACT %+v", i, c.TACT)
		}
		ready, err := a.AddFragment(c.TACT.LCSS, c.Payload)
		if err != nil {
			t.Fatalf("field %d: %v", i, err)
		}
		if ready != (i == 3) {
			t.Fatalf("field %d: ready=%v", i, ready)
		}
	}
	got, res, err := a.Complete()
	if err != nil || res.Uncorrectable || res.ErrorsCorrected != 0 {
		t.Fatalf("complete: %v %+v", err, res)
	}
	if got.SLCO != slc.SLCO || got.ActivityUpdate == nil || *got.ActivityUpdate != *slc.ActivityUpdate {
		t.Errorf("got %s", got.ToString())
	}
}

func TestEncodeShortLCToCACH_Error(t *testing.T) {
	t.Parallel()
	if _, err := layer2.EncodeShortLCToCACH(&pdu.ShortLC{SLCO: enums.SLCO(16)}, pdu.TACT{}); err == nil {
		t.Error("expected error for SLCO out of range")
	}
}
