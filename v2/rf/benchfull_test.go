//go:build !tinygo

package rf

import "testing"

func benchFullChecks(t *testing.T) {
	benchLeanClockRateNull(t)
	benchLockErrors(t)
	benchELCTalkerChange(t)
}
