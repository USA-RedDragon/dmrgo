//go:build tinygo

package pdu_test

import "testing"

// TestFuzzSeeds runs the fuzz tests of the package on their seeds, which
// TinyGo does not do because its F.Fuzz does not run the fuzz function.
func TestFuzzSeeds(t *testing.T) {
	t.Parallel()

	bptcSeeds(t, func(kind uint8, data []byte) { fuzzBPTCPDU(t, kind, data) })
	smallSeeds(t, func(kind uint8, data []byte) { fuzzSmallPDU(t, kind, data) })
	editorSeeds(t, func(kind uint8, data []byte, target, source uint32, group bool, v uint8) {
		fuzzPDUEditors(t, kind, data, target, source, group, v)
	})
	talkerAliasSeeds(func(alias string, format, order uint8, extra []byte) {
		fuzzTalkerAlias(t, alias, format, order, extra)
	})
	sysCodeSeeds(func(v uint16, model uint8, net, site uint16, par uint8, id uint32) {
		fuzzSysCode(t, v, model, net, site, par, id)
	})
}
