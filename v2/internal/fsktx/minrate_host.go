//go:build !tinygo

package fsktx

// EnableHost lets MinRate read the tables of the higher tiers. Under gc it
// does nothing, as they are always there.
func EnableHost() {}

func minRateHost(ti int) (minRateTab, bool) { return minRateHostTab(ti) }
