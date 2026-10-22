package enums_test

import (
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
)

func TestProtectKind_Table7_82(t *testing.T) {
	t.Parallel()
	want := []string{"DIS_PTT", "EN_PTT", "ILLEGALLY_PARKED", "EN_PTT_ONE_MS", "ProtectKind(4)"}
	for k, name := range want {
		if got := enums.ProtectKind(k).String(); got != name {
			t.Errorf("ProtectKind(%d) = %q, want %q", k, got, name)
		}
	}
}
