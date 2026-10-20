//go:build !tinygo

package nmea_test

import (
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/packet"
	"github.com/USA-RedDragon/dmrgo/v2/nmea"
)

func TestAllocsPositionReports(t *testing.T) {
	bursts := anytoneBursts(t)
	a := packet.NewAssembler(127)
	var b layer2.Burst
	reports := 0
	n := testing.AllocsPerRun(20, func() {
		reports = 0
		for _, raw := range bursts {
			if b.DecodeFromBytes(raw) != nil {
				continue
			}
			m, done, err := a.AddBurst(&b)
			if !done || err != nil {
				continue
			}
			if _, payload, ok := m.UDPIPv4(); ok {
				if _, err := nmea.ParseRMC(payload); err == nil {
					reports++
				}
			}
		}
	})
	if n != 0 || reports != 10 {
		t.Errorf("%.1f allocations per run and %d reports, want 0 and 10", n, reports)
	}
}
