//go:build !tinygo

package shortdata_test

import (
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/internal/appcapture"
	"github.com/USA-RedDragon/dmrgo/v2/shortdata"
)

func TestAllocs(t *testing.T) {
	data := unhex(t, appcapture.HSMS)
	buf := make([]byte, 0, 64)
	n := testing.AllocsPerRun(50, func() {
		h, err := shortdata.DecodeHyteraText(data)
		if err != nil {
			panic(err)
		}
		text := h.TextUTF8(buf[:0])
		enc, err := shortdata.AppendText(buf[len(text):len(text)], enums.DDUTF16LE, text)
		if err != nil {
			panic(err)
		}
		if _, err := shortdata.DecodeText(enc[len(enc):], enums.DDUTF16LE, enc); err != nil {
			panic(err)
		}
		if _, err := h.AppendBinary(buf[:0]); err != nil {
			panic(err)
		}
	})
	if n != 0 {
		t.Errorf("%.1f allocations per run, want 0", n)
	}
}
