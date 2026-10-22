//go:build tinygo

package pdu_test

import "testing"

func TestFuzzSeeds(t *testing.T) {
	bptcSeeds(t, func(kind uint8, data []byte) { fuzzBPTCPDU(t, kind, data) })
	smallSeeds(t, func(kind uint8, data []byte) { fuzzSmallPDU(t, kind, data) })
	editorSeeds(t, func(kind uint8, data []byte, target, source uint32, group bool, v uint8) {
		fuzzPDUEditors(t, kind, data, target, source, group, v)
	})
	talkerAliasSeeds(func(alias string, format, order uint8, extra []byte) {
		fuzzTalkerAlias(t, alias, format, order, extra)
	})
	pool := capPlusPool(t)
	capPlusSeeds(pool, func(ops, raw []byte) { fuzzCapacityPlus(t, pool, ops, raw) })
	broadcastSeeds(func(typ uint8, p1 uint16, reg bool, backoff uint8, sys uint16, p2 uint32) {
		fuzzBroadcastParms(t, typ, p1, reg, backoff, sys, p2)
	})
	sysCodeSeeds(func(v uint16, model uint8, net, site uint16, par uint8, id uint32) {
		fuzzSysCode(t, v, model, net, site, par, id)
	})
}
