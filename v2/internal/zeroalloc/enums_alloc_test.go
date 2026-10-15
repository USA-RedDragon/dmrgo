//go:build !tinygo

package zeroalloc_test

import (
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/bit"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
)

// enumsEntries returns the entries of package enums.
func enumsEntries(t testing.TB) []entry {
	t.Helper()
	voiceA := captureRawBursts(t, "voice")[1]
	bits := bit.Unpack264(voiceA)
	sync := [6]byte(bit.Pack(bits[centerStart:centerEnd]))
	if p, d := enums.NearestSyncPattern(sync); d != 0 {
		t.Fatalf("voice A SYNC: nearest %v at distance %d", p, d)
	}
	var (
		cc   enums.ColorCode
		err  error
		ok   bool
		port uint16
		sp   enums.SyncPattern
		n    int
		vb   enums.VoiceBurstType
		lcss enums.LCSS
	)
	dt := enums.DataTypeVoiceLCHeader
	burst := enums.VoiceBurstB
	out := []entry{
		{"enums.NewColorCode", "valid", 0, func() { cc, err = enums.NewColorCode(1) }},
		{"enums.NearestSyncPattern", "voice A", 0, func() { sp, n = enums.NearestSyncPattern(sync) }},
		{"enums.DPIDToPort", "", 0, func() { port = enums.DPIDToPort(enums.DPIDTextMessage) }},
		{"enums.SPIDToPort", "", 0, func() { port = enums.SPIDToPort(enums.SPIDLocationProtocol) }},
		{"enums.DAID.Valid", "", 0, func() { ok = enums.DAIDGroupNetwork.Valid() }},
		{"enums.SAID.Valid", "", 0, func() { ok = enums.SAIDRadioNetwork.Valid() }},
		{"enums.DPID.Valid", "", 0, func() { ok = enums.DPIDTextMessage.Valid() }},
		{"enums.SPID.Valid", "", 0, func() { ok = enums.SPIDTextMessage.Valid() }},
		{"enums.FLCO.Valid", "", 0, func() { ok = enums.FLCOGroupVoiceChannelUser.Valid() }},
		{"enums.FeatureSetID.Valid", "", 0, func() { ok = enums.FIDMotorola.Valid() }},
		{"enums.DataType.IsReserved", "", 0, func() { ok = dt.IsReserved() }},
		{"enums.DataType.IsBPTC", "", 0, func() { ok = dt.IsBPTC() }},
		{"enums.DataType.CarriesFullLC", "", 0, func() { ok = dt.CarriesFullLC() }},
		{"enums.VoiceBurstAt", "", 0, func() { vb, ok = enums.VoiceBurstAt(4) }},
		{"enums.VoiceBurstType.Index", "", 0, func() { n, ok = burst.Index() }},
		{"enums.VoiceBurstType.LCSS", "", 0, func() { lcss, ok = burst.LCSS() }},
	}
	_, _, _, _, _, _, _, _ = cc, err, ok, port, sp, n, vb, lcss
	return out
}
