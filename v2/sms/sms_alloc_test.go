//go:build !tinygo

package sms_test

import (
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/internal/appcapture"
	"github.com/USA-RedDragon/dmrgo/v2/sms"
)

func TestAllocs(t *testing.T) {
	m := ipMessage(t, appcapture.TMS)
	buf := make([]byte, 0, 512)
	var h sms.Header
	n := testing.AllocsPerRun(50, func() {
		out, text, err := sms.Decode(buf[:0], &m.Header, m.Data)
		if err != nil {
			panic(err)
		}
		text.Confirmed = true
		if _, err := sms.AppendPacket(out, &text, &h); err != nil {
			panic(err)
		}
	})
	if n != 0 {
		t.Errorf("%.1f allocations per run, want 0", n)
	}
}
