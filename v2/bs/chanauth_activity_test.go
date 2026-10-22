package bs_test

import (
	"testing"
	"time"

	"github.com/USA-RedDragon/dmrgo/v2/bs"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

func chanAuthUpdates(t *testing.T, d *driver, mark int) []pdu.ShortLCActivityUpdate {
	t.Helper()
	var asm layer2.ShortLCAssembler
	var aus []pdu.ShortLCActivityUpdate
	for _, o := range d.all[mark:] {
		if o.Kind != bs.OutputBurst || !o.HasCACH {
			continue
		}
		slc, done, err := asm.Add(layer2.DecodeCACHField(o.CACH))
		if err != nil || !done {
			continue
		}
		if au, ok := slc.AsActivityUpdate(); ok {
			aus = append(aus, au)
		}
	}
	return aus
}

// ETSI TS 102 361-2 clause 5.1.1.6.1.1, ETSI TS 102 361-2 clause 7.1.2.5
func TestChanAuthActivityUpdate(t *testing.T) {
	t.Parallel()
	rf := bs.Source{RF: true}
	old, _ := pdu.AddressHash(9)
	hash, _ := pdu.AddressHash(92)
	for _, auth := range []string{"voice", "csbk"} {
		d := keyed(t, bs.Config{ChannelAuth: bs.ChannelAuthSingleSite, CallHang: -1})
		d.until(2 * time.Second)
		mark := len(d.all)
		burst := header(t, 92, 3120005)
		if auth == "csbk" {
			burst = preCSBK(t, 3120005)
		}
		d.in(2*time.Second, enums.Slot1, rf, burst)
		if st := d.r.SlotState(enums.Slot1); st != bs.SlotChanAuth {
			t.Fatalf("%s: state %v", auth, st)
		}
		d.until(2*time.Second + 600*time.Millisecond)
		aus := chanAuthUpdates(t, d, mark)
		if len(aus) == 0 {
			t.Fatalf("%s: no Activity Update", auth)
		}
		for _, au := range aus {
			if au.HashTS1 == old && au.TS1ActivityID == enums.ActivityGroupVoice {
				t.Fatalf("%s: Activity Update %+v carries the call that ended", auth, au)
			}
			if auth == "voice" && (au.TS1ActivityID != enums.ActivityGroupVoice || au.HashTS1 != hash) {
				t.Errorf("%s: Activity Update %+v, want the group voice call to 92", auth, au)
			}
		}
	}
}

// ETSI TS 102 361-2 clause 5.1.1.6.1.2
func TestChanAuthSelectActivityUpdate(t *testing.T) {
	t.Parallel()
	rf := bs.Source{RF: true}
	hash, _ := pdu.AddressHash(93)
	d := keyed(t, bs.Config{ChannelAuth: bs.ChannelAuthMultiSite, CallHang: -1})
	d.until(2 * time.Second)
	d.in(2*time.Second, enums.Slot1, rf, header(t, 92, 3120005))
	d.in(2*time.Second+60*time.Millisecond, enums.Slot1, rf, header(t, 93, 3120001))
	if st := d.r.SlotState(enums.Slot1); st != bs.SlotChanAuthSelect {
		t.Fatalf("state %v", st)
	}
	mark := len(d.all)
	d.until(2*time.Second + 400*time.Millisecond)
	aus := chanAuthUpdates(t, d, mark)
	if len(aus) < 2 {
		t.Fatalf("Activity Updates %v", aus)
	}
	for _, au := range aus[1:] {
		if au.TS1ActivityID != enums.ActivityGroupVoice || au.HashTS1 != hash {
			t.Errorf("Activity Update %+v, want the group voice call to 93", au)
		}
	}
}
