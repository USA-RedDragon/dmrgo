//go:build tinygo

package allocs_test

import (
	"encoding/hex"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/hytera/hdap"
	"github.com/USA-RedDragon/dmrgo/v2/internal/appcapture"
	"github.com/USA-RedDragon/dmrgo/v2/ip"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/packet"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
	"github.com/USA-RedDragon/dmrgo/v2/location"
	"github.com/USA-RedDragon/dmrgo/v2/mototrbo/ars"
	"github.com/USA-RedDragon/dmrgo/v2/mototrbo/lrrp"
	"github.com/USA-RedDragon/dmrgo/v2/mototrbo/tms"
	"github.com/USA-RedDragon/dmrgo/v2/shortdata"
	"github.com/USA-RedDragon/dmrgo/v2/sms"
)

var tinyLRRP lrrp.Message

var tinySMSHeader sms.Header

var tinyLocation location.Decoder

func tinyAppOps(t *testing.T) []tinyOp {
	t.Helper()
	tmsIP := tinyOK(hex.DecodeString(appcapture.TMS))
	gps := tinyOK(hex.DecodeString(appcapture.GPS))
	hsms := tinyOK(hex.DecodeString(appcapture.HSMS))
	d := tinyOK(ip.Decode(enums.SAPIPBasedPacketData, tmsIP))
	c := tinyOK(ip.Decode(enums.SAPUDPIPHeaderCompression, gps))
	h := tinyOK(shortdata.DecodeHyteraText(hsms))
	m := tinyOK(tms.DecodeMessage(d.Payload))
	reg := []byte{0x00, 0x0c, 0xf0, 0x20, 0x07, '3', '1', '9', '1', '8', '6', '8', 0x00, 0x00}
	a := tinyOK(ars.DecodeMessage(reg))
	loc := tinyOK(hex.DecodeString("071a22042468ace0341f4dbc778051118ecd8d118ad47b00636c0006"))
	smsMsg := packet.Message{Header: tinyUnconfirmed(pdu.UnconfirmedDataHeader{SAP: enums.SAPIPBasedPacketData, LLIDDestination: 3140598, LLIDSource: 3191868}), Data: tmsIP}
	smsText := sms.Text{Format: sms.FormatTMS, Source: 1, Destination: 2, Body: []byte("Hello")}
	gpsMsg := packet.Message{Header: tinyUnconfirmed(pdu.UnconfirmedDataHeader{SAP: enums.SAPUDPIPHeaderCompression}), Data: gps}
	hdapMsg := []byte{0x11, 0x00, 0x03, 0x00, 0x04, 0x0a, 0x00, 0x00, 0x50, 0xd1, 0x03}
	hm := tinyOK(hdap.DecodeMessage(hdapMsg))
	buf := make([]byte, 0, 256)
	var err error
	ops := []tinyOp{
		{"ip.Decode IPv4", func() { _, err = ip.Decode(enums.SAPIPBasedPacketData, tmsIP) }},
		{"ip.Decode compressed", func() { _, err = ip.Decode(enums.SAPUDPIPHeaderCompression, gps) }},
		{"ip.AppendIPv4", func() { _, err = ip.AppendIPv4(buf[:0], &d) }},
		{"ip.AppendCompressed", func() { _, err = ip.AppendCompressed(buf[:0], &c) }},
		{"shortdata.DecodeHyteraText", func() { _, err = shortdata.DecodeHyteraText(hsms) }},
		{"shortdata.HyteraText.TextUTF8", func() { _ = h.TextUTF8(buf[:0]) }},
		{"shortdata.HyteraText.AppendBinary", func() { _, err = h.AppendBinary(buf[:0]) }},
		{"shortdata.Text", func() { _, err = shortdata.Text(buf[:0], enums.DDUTF16LE, h.Text) }},
		{"shortdata.AppendText", func() { _, err = shortdata.AppendText(buf[:0], enums.DDUTF16LE, []byte("HYT TEST")) }},
		{"tms.DecodeMessage", func() { _, err = tms.DecodeMessage(d.Payload) }},
		{"tms.Message.AppendBinary", func() { _, err = m.AppendBinary(buf[:0]) }},
		{"tms.Message.TextUTF8", func() { _ = m.TextUTF8(buf[:0]) }},
		{"ars.DecodeMessage", func() { _, err = ars.DecodeMessage(reg) }},
		{"ars.Message.AppendBinary", func() { _, err = a.AppendBinary(buf[:0]) }},
		{"lrrp.DecodeMessageInto", func() { err = lrrp.DecodeMessageInto(&tinyLRRP, loc) }},
		{"lrrp.Message.AppendBinary", func() { _, err = tinyLRRP.AppendBinary(buf[:0]) }},
		{"sms.Decode", func() { _, _, err = sms.Decode(buf[:0], &smsMsg.Header, smsMsg.Data) }},
		{"sms.AppendPacket", func() { _, err = sms.AppendPacket(buf[:0], &smsText, true, &tinySMSHeader) }},
		{"location.Decoder.Decode", func() { _, err = tinyLocation.Decode(&gpsMsg.Header, gpsMsg.Data) }},
		{"hdap.DecodeMessage", func() { _, err = hdap.DecodeMessage(hdapMsg) }},
		{"hdap.Message.AppendBinary", func() { _, err = hm.AppendBinary(buf[:0]) }},
	}
	_ = err
	return ops
}
