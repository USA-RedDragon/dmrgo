package enums_test

import (
	"errors"
	"math/bits"
	"strings"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
)

// ETSI TS 102 361-1 Table 9.2
var tableSyncPatterns = []enums.SyncPattern{
	enums.SyncBSSourcedVoice, enums.SyncBSSourcedData, enums.SyncMSSourcedVoice,
	enums.SyncMSSourcedData, enums.SyncMSSourcedRC, enums.SyncTDMA1Voice,
	enums.SyncTDMA1Data, enums.SyncTDMA2Voice, enums.SyncTDMA2Data, enums.SyncReserved,
}

// Table 9.2
func FuzzEnums(f *testing.F) {
	enumSeeds(func(v uint64, i int) { f.Add(v, i) })
	f.Fuzz(fuzzEnums)
}

func enumSeeds(add func(v uint64, i int)) {
	add(uint64(enums.SyncBSSourcedVoice), 3)
	add(uint64(enums.SyncMSSourcedData^0x100401), 0)
	add(uint64(0xFFFFFFFFFFFFFFFF), -1)
}

func fuzzEnums(t *testing.T, v uint64, i int) {
	field := [6]byte{byte(v >> 40), byte(v >> 32), byte(v >> 24), byte(v >> 16), byte(v >> 8), byte(v)}
	p, d := enums.NearestSyncPattern(field)
	best := 49
	for _, q := range tableSyncPatterns {
		best = min(best, bits.OnesCount64(v&0xFFFFFFFFFFFF^uint64(q)))
	}
	if d != best || bits.OnesCount64(v&0xFFFFFFFFFFFF^uint64(p)) != d {
		t.Fatalf("NearestSyncPattern(%012x) = %s at %d, least distance %d", v&0xFFFFFFFFFFFF, p, d, best)
	}
	if enums.SyncPattern(v).String() == "" || p.String() == "" {
		t.Fatalf("empty SyncPattern name for %x", v)
	}
	checkVoiceBursts(t, uint8(v), i)
	checkByteEnums(t, uint8(v))
	checkNames(t, uint8(v))
}

func checkVoiceBursts(t *testing.T, b uint8, i int) {
	t.Helper()
	vb, ok := enums.VoiceBurstAt(i)
	if ok != (i >= 0 && i < 6) {
		t.Fatalf("VoiceBurstAt(%d) ok %t", i, ok)
	}
	if j, jok := vb.Index(); ok && (!jok || j != i) || !ok && (jok || vb != enums.VoiceBurstUnknown) {
		t.Fatalf("VoiceBurstAt(%d) = %s, Index %d %t", i, vb, j, jok)
	}
	bt := enums.VoiceBurstType(b)
	_, iok := bt.Index()
	if _, lok := bt.LCSS(); lok != (iok && bt != enums.VoiceBurstA) || bt.String() == "" {
		t.Fatalf("VoiceBurstType %d LCSS ok %t", b, lok)
	}
}

func checkByteEnums(t *testing.T, b uint8) {
	t.Helper()
	cc, err := enums.NewColorCode(b)
	var rangeErr *dmrgo.FieldRangeError
	if b < 16 && (err != nil || cc != enums.ColorCode(b)) || b >= 16 && (!errors.As(err, &rangeErr) || rangeErr.Bits != 4 || err.Error() == "") {
		t.Fatalf("NewColorCode(%d) = %d %v", b, cc, err)
	}
	s := enums.Slot(b)
	if o := s.Other(); (s == enums.Slot1 || s == enums.Slot2) != (o.Other() == s && o != s) || s.String() == "" {
		t.Fatalf("Slot %d Other %d", b, o)
	}
	dt := enums.DataType(b)
	if dt.IsReserved() && (dt.IsBPTC() || dt.CarriesFullLC() || dt.String() != "Reserved") || dt.CarriesFullLC() && !dt.IsBPTC() {
		t.Fatalf("DataType %d: reserved %t, BPTC %t, Full LC %t, %q", b, dt.IsReserved(), dt.IsBPTC(), dt.CarriesFullLC(), dt.String())
	}
}

func checkNames(t *testing.T, b uint8) {
	t.Helper()
	valid := []struct {
		name  string
		ok    bool
		label string
	}{
		{"FLCO", enums.FLCO(b).Valid(), enums.FLCO(b).String()},
		{"FeatureSetID", enums.FeatureSetID(b).Valid(), enums.FeatureSetID(b).String()},
		{"DAID", enums.DAID(b).Valid(), enums.DAID(b).String()},
		{"SAID", enums.SAID(b).Valid(), enums.SAID(b).String()},
		{"DPID", enums.DPID(b).Valid(), enums.DPID(b).String()},
		{"SPID", enums.SPID(b).Valid(), enums.SPID(b).String()},
	}
	for _, c := range valid {
		if c.ok == strings.HasPrefix(c.label, "Unknown") {
			t.Fatalf("%s %d: Valid %t, String %q", c.name, b, c.ok, c.label)
		}
	}
	_ = enums.DPIDToPort(enums.DPID(b))
	_ = enums.SPIDToPort(enums.SPID(b))
	for _, name := range []string{
		enums.Access(b).String(), enums.ActivityID(b).String(), enums.AnnouncementType(b).String(),
		enums.LCSS(b).String(), enums.RCCommand(b).String(), enums.ServiceAccessPointID(b).String(),
		enums.SLCO(b).String(), enums.DataType(b).String(),
	} {
		if name == "" {
			t.Fatalf("empty name for %d", b)
		}
	}
}
