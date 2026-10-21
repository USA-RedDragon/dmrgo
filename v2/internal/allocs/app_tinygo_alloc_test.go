//go:build tinygo

package allocs_test

import (
	"encoding/hex"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/internal/appcapture"
	"github.com/USA-RedDragon/dmrgo/v2/ip"
	"github.com/USA-RedDragon/dmrgo/v2/shortdata"
)

func tinyAppOps(t *testing.T) []tinyOp {
	t.Helper()
	tms := tinyOK(hex.DecodeString(appcapture.TMS))
	gps := tinyOK(hex.DecodeString(appcapture.GPS))
	hsms := tinyOK(hex.DecodeString(appcapture.HSMS))
	d := tinyOK(ip.Decode(enums.SAPIPBasedPacketData, tms))
	c := tinyOK(ip.Decode(enums.SAPUDPIPHeaderCompression, gps))
	h := tinyOK(shortdata.DecodeHyteraText(hsms))
	buf := make([]byte, 0, 256)
	var err error
	ops := []tinyOp{
		{"ip.Decode IPv4", func() { _, err = ip.Decode(enums.SAPIPBasedPacketData, tms) }},
		{"ip.Decode compressed", func() { _, err = ip.Decode(enums.SAPUDPIPHeaderCompression, gps) }},
		{"ip.AppendIPv4", func() { _, err = ip.AppendIPv4(buf[:0], &d) }},
		{"ip.AppendCompressed", func() { _, err = ip.AppendCompressed(buf[:0], &c) }},
		{"shortdata.DecodeHyteraText", func() { _, err = shortdata.DecodeHyteraText(hsms) }},
		{"shortdata.HyteraText.TextUTF8", func() { _ = h.TextUTF8(buf[:0]) }},
		{"shortdata.HyteraText.AppendBinary", func() { _, err = h.AppendBinary(buf[:0]) }},
		{"shortdata.Text", func() { _, err = shortdata.Text(buf[:0], enums.DDUTF16LE, h.Text) }},
		{"shortdata.AppendText", func() { _, err = shortdata.AppendText(buf[:0], enums.DDUTF16LE, []byte("HYT TEST")) }},
	}
	_ = err
	return ops
}
