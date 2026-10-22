package enums_test

import (
	"flag"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
)

var update = flag.Bool("update", false, "rewrite testdata/strings.txt")

type stringer struct {
	name string
	max  int
	str  func(v int) string
}

func stringers() []stringer {
	return []stringer{
		{"Access", 3, func(v int) string { return enums.Access(v).String() }},
		{"AckReason", 255, func(v int) string { return enums.AckReason(v).String() }},
		{"AckType", 4, func(v int) string { return enums.AckType(v).String() }},
		{"ActivityID", 16, func(v int) string { return enums.ActivityID(v).String() }},
		{"AnnouncementType", 32, func(v int) string { return enums.AnnouncementType(v).String() }},
		{"DAID", 16, func(v int) string { return enums.DAID(v).String() }},
		{"DataType", 16, func(v int) string { return enums.DataType(v).String() }},
		{"DDFormat", 64, func(v int) string { return enums.DDFormat(v).String() }},
		{"DPID", 128, func(v int) string { return enums.DPID(v).String() }},
		{"FID", 255, func(v int) string { return enums.FID(v).String() }},
		{"FLCO", 64, func(v int) string { return enums.FLCO(v).String() }},
		{"LCSS", 4, func(v int) string { return enums.LCSS(v).String() }},
		{"ProtectKind", 8, func(v int) string { return enums.ProtectKind(v).String() }},
		{"RCCommand", 16, func(v int) string { return enums.RCCommand(v).String() }},
		{"SAID", 16, func(v int) string { return enums.SAID(v).String() }},
		{"SAP", 16, func(v int) string { return enums.SAP(v).String() }},
		{"ServiceFunction", 4, func(v int) string { return enums.ServiceFunction(v).String() }},
		{"ServiceKind", 16, func(v int) string { return enums.ServiceKind(v).String() }},
		{"SLCO", 16, func(v int) string { return enums.SLCO(v).String() }},
		{"Slot", 3, func(v int) string { return enums.Slot(v).String() }},
		{"SPID", 128, func(v int) string { return enums.SPID(v).String() }},
		{"UDTFormat", 16, func(v int) string { return enums.UDTFormat(v).String() }},
		{"VoiceBurstType", 7, func(v int) string { return enums.VoiceBurstType(v).String() }},
	}
}

func TestStrings(t *testing.T) {
	t.Parallel()

	var b strings.Builder
	for _, e := range stringers() {
		for v := 0; v <= e.max; v++ {
			s := e.str(v)
			if s == "" || strings.HasPrefix(s, e.name+"(") && s != e.name+"("+strconv.Itoa(v)+")" {
				t.Errorf("%s(%d).String() = %q", e.name, v, s)
			}
			b.WriteString(e.name + " " + strconv.Itoa(v) + " " + s + "\n")
		}
	}
	for _, p := range append(tableSyncPatterns, enums.SyncNone, 0x123456789ABC) {
		b.WriteString("SyncPattern " + strconv.FormatUint(uint64(p), 16) + " " + p.String() + "\n")
	}
	const path = "testdata/strings.txt"
	if *update {
		if err := os.WriteFile(path, []byte(b.String()), 0o600); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := b.String(); got != string(want) {
		g, w := strings.Split(got, "\n"), strings.Split(string(want), "\n")
		for i := range min(len(g), len(w)) {
			if g[i] != w[i] {
				t.Fatalf("line %d: got %q, want %q; run go test -update", i+1, g[i], w[i])
			}
		}
		t.Fatalf("got %d lines, want %d; run go test -update", len(g), len(w))
	}
}
