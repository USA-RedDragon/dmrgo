//go:build !tinygo

package hytera_test

import (
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/hytera"
)

func TestAllocs(t *testing.T) {
	data := append([]byte{0x08, 0xa0, 0x02, 0x00, 0x32, 0, 0, 0, 7, 10, 0, 0, 80, 0, 0}, append([]byte(gps), 0xa5, 0x03)...)
	buf := make([]byte, 0, 128)
	n := testing.AllocsPerRun(50, func() {
		m, err := hytera.DecodeMessage(data)
		if err != nil {
			panic(err)
		}
		a, err := hytera.DecodeLPAnswer(m.Payload)
		if err != nil {
			panic(err)
		}
		if _, err := a.AppendBinary(buf[:0]); err != nil {
			panic(err)
		}
		m.Seal()
		if _, err := m.AppendBinary(buf[:0]); err != nil {
			panic(err)
		}
	})
	if n != 0 {
		t.Errorf("%.1f allocations per run, want 0", n)
	}
}
