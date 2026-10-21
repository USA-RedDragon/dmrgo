//go:build !tinygo

package tms_test

import (
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/mototrbo/tms"
)

func TestAllocs(t *testing.T) {
	payload := captured(t)
	buf := make([]byte, 0, 128)
	n := testing.AllocsPerRun(50, func() {
		m, err := tms.DecodeMessage(payload)
		if err != nil {
			panic(err)
		}
		text := m.TextUTF8(buf[:0])
		r, err := tms.NewText(1, true)
		if err != nil {
			panic(err)
		}
		rest := r.SetTextUTF8(buf[len(text):len(text)], text)
		if _, err := r.AppendBinary(rest[len(rest):]); err != nil {
			panic(err)
		}
	})
	if n != 0 {
		t.Errorf("%.1f allocations per run, want 0", n)
	}
}
