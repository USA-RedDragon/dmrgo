//go:build tinygo

package fsktx

var hostTables bool //nolint:gochecknoglobals // set by the constructors of the higher tiers, so builds without one drop their tables

// EnableHost lets MinRate read the tables of the higher tiers. Under
// TinyGo, a program that never calls it links none of them.
func EnableHost() { hostTables = true }

func minRateHost(ti int) (minRateTab, bool) {
	if !hostTables {
		return minRateTab{}, false
	}
	return minRateHostTab(ti)
}
