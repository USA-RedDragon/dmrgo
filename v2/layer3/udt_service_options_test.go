package layer3_test

import (
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer3"
)

func TestUDTServiceOptions_Tables7_54_7_58_7_60(t *testing.T) {
	t.Parallel()
	if v, err := (layer3.ShortDataServiceOptions{IsSuppData: true}).Field(); err != nil || v != 0b0010000 {
		t.Errorf("short data %07b %v", v, err)
	}
	if v, err := (layer3.PollingServiceOptions{Format: enums.UDTNMEA}).Field(); err != nil || v != 0b0000101 {
		t.Errorf("polling %07b %v", v, err)
	}
	if _, err := (layer3.PollingServiceOptions{Format: 16}).Field(); err == nil {
		t.Error("polling format 16 encoded")
	}
	d := layer3.DiversionServiceOptions{Set: true, Voice: true, Status: true}
	v, err := d.Field()
	if err != nil || v != 0b0011001 {
		t.Errorf("diversion %07b %v", v, err)
	}
	if back := layer3.DiversionServiceOptionsOf(v); back != d {
		t.Errorf("diversion %+v", back)
	}
	if back := layer3.PollingServiceOptionsOf(0b0000101); back.Format != enums.UDTNMEA {
		t.Errorf("polling %+v", back)
	}
	if back := layer3.DecodeShortDataServiceOptions([1]byte{0xFF}); !back.IsSuppData || !back.IsPrivacy {
		t.Errorf("short data %+v", back)
	}
}
