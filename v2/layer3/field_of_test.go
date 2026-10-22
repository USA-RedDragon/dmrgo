package layer3_test

import (
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer3"
)

func TestServiceOptionsOf_InvertsField(t *testing.T) {
	t.Parallel()
	v := layer3.VoiceServiceOptions{IsEmergency: true, PriorityLevel: 2}
	if f, err := v.Field(); err != nil || f != 0x42 || layer3.VoiceServiceOptionsOf(f) != v {
		t.Errorf("Voice: field %#x %v, back %+v", f, err, layer3.VoiceServiceOptionsOf(f))
	}
	if got := layer3.VoiceServiceOptionsOf(0x80 | 0x42); got != v {
		t.Errorf("Voice with bit 7: %+v", got)
	}
	d := layer3.DataServiceOptions{IsSuppData: true, MultiItem: true, PriorityLevel: 1}
	if f, _ := d.Field(); layer3.DataServiceOptionsOf(f) != d {
		t.Errorf("Data: %+v", layer3.DataServiceOptionsOf(f))
	}
	r := layer3.RegistrationServiceOptions{IPInform: true, PowerSave: 5, Register: true}
	if f, _ := r.Field(); layer3.RegistrationServiceOptionsOf(f) != r {
		t.Errorf("Registration: %+v", layer3.RegistrationServiceOptionsOf(f))
	}
	s := layer3.StatusServiceOptions{Group: true, StatusHigh: 0x15}
	if f, _ := s.Field(); layer3.StatusServiceOptionsOf(f) != s {
		t.Errorf("Status: %+v", layer3.StatusServiceOptionsOf(f))
	}
	sd := layer3.ShortDataServiceOptions{IsSuppData: true}
	if f, _ := sd.Field(); layer3.ShortDataServiceOptionsOf(f) != sd {
		t.Errorf("ShortData: %+v", layer3.ShortDataServiceOptionsOf(f))
	}
	p := layer3.PollingServiceOptions{IsPrivacy: true, Format: enums.UDTNMEA}
	if f, _ := p.Field(); layer3.PollingServiceOptionsOf(f) != p {
		t.Errorf("Polling: %+v", layer3.PollingServiceOptionsOf(f))
	}
	dv := layer3.DiversionServiceOptions{Set: true, Voice: true, Status: true}
	if f, _ := dv.Field(); layer3.DiversionServiceOptionsOf(f) != dv {
		t.Errorf("Diversion: %+v", layer3.DiversionServiceOptionsOf(f))
	}
}
