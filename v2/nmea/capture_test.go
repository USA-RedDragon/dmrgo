package nmea_test

import (
	"bufio"
	"encoding/hex"
	"os"
	"strings"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/ip"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/packet"
	"github.com/USA-RedDragon/dmrgo/v2/nmea"
)

func anytoneBursts(t testing.TB) []layer2.RawBurst {
	t.Helper()
	f, err := os.Open("../layer2/testdata/anytone_aprs.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var out []layer2.RawBurst
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		if len(fields) == 0 || strings.HasPrefix(fields[0], "#") {
			continue
		}
		b, err := hex.DecodeString(fields[1])
		if len(fields) != 2 || err != nil || len(b) != 33 {
			t.Fatalf("%s: %v", sc.Text(), err)
		}
		out = append(out, layer2.RawBurst(b))
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestAnyTonePositionReports(t *testing.T) {
	t.Parallel()

	bursts := anytoneBursts(t)
	if len(bursts) != 150 {
		t.Fatalf("%d bursts, want 150", len(bursts))
	}
	wantTimes := [][3]uint8{
		{20, 0, 37}, {20, 1, 39}, {20, 2, 41}, {20, 3, 43}, {20, 4, 45},
		{20, 5, 47}, {20, 6, 49}, {20, 7, 51}, {20, 8, 55}, {20, 9, 57},
	}
	a := packet.NewAssembler(127)
	var b layer2.Burst
	n := 0
	for i, raw := range bursts {
		if err := b.DecodeFromBytes(raw); err != nil {
			t.Fatalf("burst %d: %v", i, err)
		}
		m, done, err := a.AddBurst(&b)
		if err != nil {
			t.Fatalf("burst %d: %v", i, err)
		}
		if !done {
			continue
		}
		if n >= len(wantTimes) {
			t.Fatalf("more than %d reports", len(wantTimes))
		}
		checkReport(t, n, &m, wantTimes[n])
		n++
	}
	if n != len(wantTimes) {
		t.Fatalf("%d reports, want %d", n, len(wantTimes))
	}
}

func checkReport(t *testing.T, n int, m *packet.Message, w [3]uint8) {
	t.Helper()
	u, ok := m.Header.AsUnconfirmedDataHeader()
	if !ok || u.SAP != enums.SAPUDPIPHeaderCompression || u.LLIDDestination != 3140598 || u.LLIDSource != 3191868 || m.Header.AppendedBlocks() != 0 {
		t.Fatalf("report %d: header %s", n, m.Header.String())
	}
	udp, payload, ok := m.UDPIPv4()
	if !ok {
		t.Fatalf("report %d: no compressed UDP/IPv4 header in %q", n, m.Data)
	}
	src, dst := ip.CompressedPorts(&udp)
	if udp.IPv4Identification != 1 || udp.SAID != enums.SAIDRadioNetwork || udp.DAID != enums.DAIDRadioNetwork ||
		udp.HeaderCompressionOpcode != 0 || udp.Octets() != 9 || src != 10110 || dst != 10110 {
		t.Fatalf("report %d: %s, ports %d %d", n, udp.String(), src, dst)
	}
	r, err := nmea.ParseRMC(payload)
	if err != nil {
		t.Fatalf("report %d: %q: %v", n, payload, err)
	}
	if r.Hour != w[0] || r.Minute != w[1] || r.Second != w[2] || r.Millisecond != 0 || !r.Valid ||
		!near(r.Latitude, 34+12.73/60) || !near(r.Longitude, 108+50.0/60) || r.Speed != 0 || r.Course != 121 ||
		r.Day != 2 || r.Month != 12 || r.Year != 2025 || r.MagneticVariation != 0 || r.Mode != 'A' || r.Talker != [2]byte{'G', 'P'} {
		t.Errorf("report %d: %+v", n, r)
	}
}
